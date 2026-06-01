package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/beads/closeorder"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/events"
	"github.com/gastownhall/gascity/internal/execenv"
	"github.com/gastownhall/gascity/internal/formula"
	"github.com/gastownhall/gascity/internal/fsys"
	"github.com/gastownhall/gascity/internal/molecule"
	"github.com/gastownhall/gascity/internal/orderdiscovery"
	"github.com/gastownhall/gascity/internal/orders"
	"github.com/gastownhall/gascity/internal/processgroup"
	"github.com/gastownhall/gascity/internal/routedwork"
)

const (
	labelOrderTracking    = "order-tracking"
	labelTriggerEnvFailed = "trigger-env-failed"

	orderTrackingSweepOrder                = "order-tracking-sweep"
	defaultOrderTrackingSweepStaleAfter    = 10 * time.Minute
	defaultOrderWispSweepStaleAfter        = 24 * time.Hour
	orderTrackingSweepWatchdogInterval     = 30 * time.Second
	orderTrackingSweepWatchdogStaleAfter   = 2 * time.Minute
	orderTrackingSweepMetadataReason       = "stale-order-tracking"
	orderTrackingSweepMetadataInitiator    = "order-tracking-sweep"
	orderTrackingWatchdogMetadataInitiator = "controller-watchdog"
	orderTrackingCloseVerifyAttempts       = 3
	orderTrackingCloseVerifyRetryDelay     = 25 * time.Millisecond
	orderDispatchSnapshotBudget            = 12 * time.Second
	orderDispatchSingleReadBudget          = 500 * time.Millisecond
	orderDispatchOpenReadLimit             = 1000
	orderDispatchLastRunReadLimit          = 1
	orderDispatchEventCursorReadLimit      = 200
	orderDispatchDescendantMaxDepth        = 4
	orderDispatchDescendantMaxRows         = 500

	// orphanedOrderTrackingCloseReason is the canonical close_reason
	// stamped on orphan-sweep closes. It satisfies bd's
	// validation.on-close=error validator (which rejects closes without
	// an explicit --reason of >=20 characters) and provides a meaningful
	// audit trail in the closed bead's metadata. Without this, the close
	// is rejected, the bead stays open, and the next sweep tick re-stamps
	// identical metadata — generating one bead.updated event per tick per
	// bead.
	orphanedOrderTrackingCloseReason = "order-tracking sweep: orphaned by prior controller"

	// staleOrderTrackingCloseReason is the canonical close_reason stamped
	// on stale-sweep closes (both the periodic order-tracking-sweep order
	// and the controller's runtime watchdog). Same rationale as
	// orphanedOrderTrackingCloseReason — without an explicit reason of
	// >=20 chars, validation.on-close=error rejects every close, the
	// watchdog retries every 30s, and the order-firing pipeline silently
	// wedges (no bead.created/closed events, only metadata churn).
	staleOrderTrackingCloseReason = "order-tracking sweep: stale tracking bead exceeded retention window"
	staleOrderWispCloseReason     = "order-tracking sweep: stale order wisp subtree exceeded retention window"

	completedOrderTrackingCloseReason = "order dispatch completed: tracking bead lifecycle finished"
)

var (
	orderDispatchMaxCreatesPerTick        = 4
	orderDispatchMaxExecCreatesPerTick    = 4
	orderDispatchMaxFormulaCreatesPerTick = 1
)

// orderDispatchStartupThrottleWindow bounds how long the default dispatch caps
// apply after controller construction. The defaults protect city reloads from a
// due-order burst; steady state relies on runtime writer backpressure so due
// orders can keep cadence without permanent static throttles.
var orderDispatchStartupThrottleWindow = 10 * time.Minute

// orderDispatchTrackingWriteBudget is longer than the generic runtime-write
// default because one due order produces a small write chain: reservation,
// outcome labels, then close. The per-tick create cap below keeps caller latency
// bounded without opening a startup or steady-state write storm.
var orderDispatchTrackingWriteBudget = 5 * time.Second

func orderRunLabel(scopedName string) string {
	return "order-run:" + scopedName
}

func scopedOrderTrackingLabel(scopedName string) string {
	return labelOrderTracking + ":" + scopedName
}

func orderTrackingLabels(scopedName string) []string {
	return []string{orderRunLabel(scopedName), labelOrderTracking, scopedOrderTrackingLabel(scopedName)}
}

var (
	// shellExecPostCancelWaitDelay is os/exec's pipe-close wait after
	// Cancel returns; the TERM and KILL waits each use shellExecSignalGrace.
	shellExecPostCancelWaitDelay = 2 * time.Second
	shellExecSignalGrace         = 2 * time.Second
)

// orderDispatcher evaluates order trigger conditions and dispatches due
// orders as wisps or exec scripts. Follows the nil-guard tracker pattern:
// nil means no auto-dispatchable orders exist.
//
// dispatch runs trigger evaluation synchronously, then spawns a goroutine
// per due order's dispatch action. The tracking bead is created before the
// goroutine launches to prevent re-fire on the next tick.
//
// drain waits for all in-flight dispatch goroutines spawned by prior
// dispatch calls to complete, bounded by ctx. It returns true when all
// tracked dispatches completed. Callers use this on controller exit and
// config reload to ensure tracking bead outcome metadata is persisted
// before the dispatcher is replaced or discarded.
type orderDispatcher interface {
	dispatch(ctx context.Context, cityPath string, now time.Time)
	drain(ctx context.Context) bool
}

// ExecRunner runs a shell command with context, working directory, and
// environment variables. Returns combined stdout or an error. When context
// cancellation stops a command, the returned error is ctx.Err(), not an
// *exec.ExitError, and the returned output may be partial.
type ExecRunner func(ctx context.Context, command, dir string, env []string) ([]byte, error)

// shellExecRunner is the production ExecRunner using os/exec.
func shellExecRunner(ctx context.Context, command, dir string, env []string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = dir
	cmd.Env = mergeOrderExecEnv(cmd.Environ(), env)
	processgroup.StartCommandInNewGroup(cmd)

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	var cleanupMu sync.Mutex
	var cleanupErr error
	var cleanupOnce sync.Once
	startedPGID := 0
	canceled := false
	cleanupProcess := func() error {
		cleanupOnce.Do(func() {
			cleanupMu.Lock()
			pgid := startedPGID
			cleanupMu.Unlock()
			err := cancelShellExecProcessGroup(cmd, pgid)
			cleanupMu.Lock()
			cleanupErr = err
			cleanupMu.Unlock()
		})
		cleanupMu.Lock()
		defer cleanupMu.Unlock()
		return cleanupErr
	}
	cmd.Cancel = func() error {
		cleanupMu.Lock()
		canceled = true
		cleanupMu.Unlock()
		_ = cleanupProcess()
		return nil
	}
	cmd.WaitDelay = shellExecPostCancelWaitDelay

	if err := cmd.Start(); err != nil {
		return output.Bytes(), err
	}
	cleanupMu.Lock()
	startedPGID = cmd.Process.Pid
	if pgid, err := syscall.Getpgid(cmd.Process.Pid); err == nil {
		startedPGID = pgid
	}
	cleanupMu.Unlock()

	err := cmd.Wait()
	cleanupMu.Lock()
	wasCanceled := canceled
	cleanupMu.Unlock()
	if errors.Is(err, exec.ErrWaitDelay) || wasCanceled {
		_ = cleanupProcess()
	}

	cleanupMu.Lock()
	wasCanceled = canceled
	errCleanup := cleanupErr
	cleanupMu.Unlock()
	if wasCanceled {
		if err := ctx.Err(); err != nil {
			if errCleanup != nil {
				return output.Bytes(), errors.Join(err, errCleanup)
			}
			return output.Bytes(), err
		}
	}
	if errCleanup != nil {
		if err != nil {
			return output.Bytes(), errors.Join(err, errCleanup)
		}
		return output.Bytes(), errCleanup
	}
	return output.Bytes(), err
}

func cancelShellExecProcessGroup(cmd *exec.Cmd, pgid int) error {
	return processgroup.TerminateCommand(cmd, pgid, shellExecSignalGrace, processgroup.Options{})
}

func mergeOrderExecEnv(environ, env []string) []string {
	out := mergeRuntimeEnv(environ, nil)
	for _, entry := range env {
		key, _, ok := strings.Cut(entry, "=")
		if ok {
			out = removeEnvKey(out, key)
		}
	}
	return append(out, env...)
}

func logDispatchError(stderr io.Writer, format string, args ...any) {
	msg := execenv.RedactText(fmt.Sprintf(format, args...), os.Environ())
	log.Print(msg)
	if stderr != nil {
		fmt.Fprintln(stderr, msg) //nolint:errcheck // best-effort stderr
	}
}

// lockedWriter serializes Write calls so concurrent dispatchOne goroutines
// logging via logDispatchError(m.stderr, ...) do not interleave bytes.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (lw *lockedWriter) Write(p []byte) (int, error) {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	return lw.w.Write(p)
}

// lockedStderr wraps w for storage on memoryOrderDispatcher.stderr. Returns
// nil unchanged so logDispatchError's nil-guard keeps its original semantics.
func lockedStderr(w io.Writer) io.Writer {
	if w == nil {
		return nil
	}
	return &lockedWriter{w: w}
}

type orderStoreFunc func(execStoreTarget) (beads.Store, error)

type orderSetSnapshot struct {
	Orders    []orders.Order
	Signature string
}

// memoryOrderDispatcher is the production implementation.
//
// inflightN + inflightDone together track dispatchOne goroutines so
// drain can select on either completion or ctx.Done without spawning an
// orphaned waiter goroutine. dispatch is only ever called from the tick
// goroutine, so addInflight's check-and-create happens-before any
// concurrent drain call on the same instance.
//
// dispatchCtx is the parent context for every dispatchOne goroutine. The
// per-goroutine ctx is derived to cancel when EITHER the caller's tick
// ctx OR dispatchCtx is done (see launchDispatchOne). cancel() cancels
// dispatchCtx.
type memoryOrderDispatcher struct {
	aa                 []orders.Order
	storeFn            orderStoreFunc
	ep                 events.Provider
	execRun            ExecRunner
	rec                events.Recorder
	stderr             io.Writer
	maxTimeout         time.Duration
	cfg                *config.City
	cityName           string
	cacheMu            sync.Mutex
	lastRunCache       map[string]time.Time
	degradedRunCache   map[string]time.Time
	nextCandidateStart int
	startedAt          time.Time
	leaseStore         *orderRuntimeLeaseStore

	dispatchCtx    context.Context
	dispatchCancel context.CancelFunc

	inflightMu   sync.Mutex
	inflightN    int
	inflightDone chan struct{} // closed when inflightN returns to 0; nil when idle
}

type orderDispatchCandidate struct {
	order               orders.Order
	target              execStoreTarget
	store               beads.Store
	storeKey            string
	reservationStoreKey string
	gateStores          []beads.Store
	gateStoreKeys       []string
}

type orderDispatchSnapshotInput struct {
	storeKey     string
	store        beads.Store
	orders       map[string]struct{}
	needsLastRun map[string]bool
	needsCursor  map[string]bool
	eventLastRun map[string]time.Time
}

type orderDispatchSnapshots struct {
	byStore map[string]*orderDispatchStoreSnapshot
}

type orderDispatchStoreSnapshot struct {
	storeKey              string
	capturedAt            time.Time
	lastRunByOrder        map[string]time.Time
	durableLastRunByOrder map[string]time.Time
	cursorByOrder         map[string]uint64
	openByOrder           map[string]bool
	degraded              map[string][]error
}

type orderDispatchTickStats struct {
	startedAt             time.Time
	ordersConsidered      int
	storesTouched         map[string]struct{}
	dispatchesCreated     int
	execDispatchesCreated int
	wispDispatchesCreated int
	ordersDeferred        int
	deferReasons          map[string]int
	trackingWriteFailures int
	writeDegraded         int
}

type orderDispatchCreateCaps struct {
	total int
	exec  int
	wisp  int
}

type orderDispatchRuntimeWriteStatsStore interface {
	RuntimeWriteManagerStatsForClass(beads.WriteClass) beads.RuntimeWriteClassStats
}

type orderDispatchBackingStore interface {
	Backing() beads.Store
}

type orderDispatchRun struct {
	TrackingID       string
	LeaseID          string
	TrackingReserved bool
	EventSeq         uint64
}

// buildOrderDispatcher scans formula layers for orders and returns a
// dispatcher. Returns nil if no auto-dispatchable orders are found.
// Scans both city-level and per-rig orders. Rig orders get their Rig
// field stamped so they use independent scoped labels.
func buildOrderDispatcher(cityPath string, cfg *config.City, rec events.Recorder, stderr io.Writer) orderDispatcher {
	od, _ := buildOrderDispatcherWithSnapshot(cityPath, cfg, rec, stderr, "gc start: order scan")
	return od
}

func buildOrderDispatcherWithSnapshot(cityPath string, cfg *config.City, rec events.Recorder, stderr io.Writer, cmdName string) (orderDispatcher, orderSetSnapshot) {
	snapshot, err := scanOrderSetSnapshotFS(fsys.OSFS{}, cityPath, cfg, stderr, cmdName)
	if err != nil {
		logDispatchError(stderr, "%s: %v", cmdName, err)
		return nil, orderSetSnapshot{}
	}
	return buildOrderDispatcherFromOrderSet(cityPath, cfg, snapshot.Orders, rec, stderr), snapshot
}

func scanOrderSetSnapshotFS(fs fsys.FS, cityPath string, cfg *config.City, stderr io.Writer, cmdName string) (orderSetSnapshot, error) {
	if cfg == nil {
		cfg = &config.City{}
	}
	allAA, err := orderdiscovery.ScanAll(cityPath, cfg, orderdiscovery.ScanOptions{
		FS: fs,
		OnRigScanError: func(rigName string, err error) error {
			fmt.Fprintf(stderr, "%s: rig %s: %v\n", cmdName, rigName, err) //nolint:errcheck // best-effort stderr
			return nil
		},
		OnOverrideError: func(err error) error {
			logDispatchError(stderr, "%s: order overrides: %v", cmdName, err)
			return nil
		},
	})
	if err != nil {
		return orderSetSnapshot{}, err
	}
	return orderSetSnapshot{
		Orders:    append([]orders.Order(nil), allAA...),
		Signature: orderSetSignature(allAA),
	}, nil
}

func orderSetSignature(aa []orders.Order) string {
	normalized := append([]orders.Order(nil), aa...)
	sort.Slice(normalized, func(i, j int) bool {
		left, right := normalized[i].ScopedName(), normalized[j].ScopedName()
		if left != right {
			return left < right
		}
		return normalized[i].Source < normalized[j].Source
	})
	data, err := json.Marshal(normalized)
	if err != nil {
		data = []byte(fmt.Sprintf("%#v", normalized))
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:])
}

func buildOrderDispatcherFromOrderSet(cityPath string, cfg *config.City, allAA []orders.Order, rec events.Recorder, stderr io.Writer) orderDispatcher {
	if cfg == nil {
		cfg = &config.City{}
	}
	allAA = orders.FilterEnabled(allAA)

	// Filter out manual-trigger orders — they are never auto-dispatched.
	var auto []orders.Order
	for _, a := range allAA {
		if a.Trigger != "manual" {
			auto = append(auto, a)
		}
	}
	if len(auto) == 0 {
		return nil
	}

	// Extract events.Provider from recorder if available.
	// FileRecorder implements Provider; Discard does not.
	var ep events.Provider
	if p, ok := rec.(events.Provider); ok {
		ep = p
	}

	dispatchCtx, dispatchCancel := context.WithCancel(context.Background())
	return &memoryOrderDispatcher{
		aa: auto,
		storeFn: func(target execStoreTarget) (beads.Store, error) {
			return openStoreAtForCity(target.ScopeRoot, cityPath)
		},
		ep:             ep,
		execRun:        shellExecRunner,
		rec:            rec,
		stderr:         lockedStderr(stderr),
		maxTimeout:     cfg.Orders.MaxTimeoutDuration(),
		cfg:            cfg,
		cityName:       loadedCityName(cfg, cityPath),
		startedAt:      time.Now(),
		leaseStore:     newOrderRuntimeLeaseStore(cityPath),
		dispatchCtx:    dispatchCtx,
		dispatchCancel: dispatchCancel,
	}
}

func (m *memoryOrderDispatcher) dispatch(ctx context.Context, cityPath string, now time.Time) {
	// Skip all order dispatch when the city is suspended.
	if m.cfg != nil && citySuspended(m.cfg) {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	stats := &orderDispatchTickStats{
		startedAt:     time.Now(),
		storesTouched: make(map[string]struct{}),
		deferReasons:  make(map[string]int),
	}
	defer m.recordOrderDispatchTick(stats)

	stores := make(map[string]beads.Store)
	var candidates []orderDispatchCandidate
	snapshotInputs := make(map[string]*orderDispatchSnapshotInput)

	for _, a := range m.aa {
		// Skip orders targeting suspended rigs.
		if m.orderRigSuspended(a) {
			continue
		}
		stats.ordersConsidered++

		target, err := resolveOrderStoreTarget(cityPath, m.cfg, a)
		if err != nil {
			logDispatchError(m.stderr, "gc: order dispatch: resolving target for %s: %v", a.ScopedName(), err)
			stats.recordDeferred("resolve_target")
			continue
		}

		storeKey := orderStoreTargetKey(target)
		store, ok := stores[storeKey]
		if !ok {
			store, err = m.storeFn(target)
			if err != nil {
				logDispatchError(m.stderr, "gc: order dispatch: opening %s store for %s: %v", target.ScopeKind, a.ScopedName(), err)
				stats.recordDeferred("open_store")
				continue
			}
			stores[storeKey] = store
		}
		stats.touchStore(storeKey)

		storesForGate := []beads.Store{store}
		legacyStore, legacyOK := m.legacyCityStoreForTarget(cityPath, target, stores)
		if !legacyOK {
			stats.recordDeferred("legacy_store")
			continue
		}
		if legacyStore != nil {
			storesForGate = append(storesForGate, legacyStore)
		}
		storeKeysForGate := []string{storeKey}
		if legacyStore != nil {
			legacyKey := orderStoreTargetKey(legacyOrderCityTarget(cityPath, m.cfg))
			storeKeysForGate = append(storeKeysForGate, legacyKey)
			stats.touchStore(legacyKey)
		}
		scoped := a.ScopedName()
		for idx, gateStore := range storesForGate {
			if gateStore == nil {
				continue
			}
			key := storeKeysForGate[idx]
			input := snapshotInputs[key]
			if input == nil {
				input = &orderDispatchSnapshotInput{
					storeKey:     key,
					store:        gateStore,
					orders:       make(map[string]struct{}),
					needsLastRun: make(map[string]bool),
					needsCursor:  make(map[string]bool),
					eventLastRun: make(map[string]time.Time),
				}
				snapshotInputs[key] = input
			}
			input.orders[scoped] = struct{}{}
			if orderTriggerUsesLastRun(a) {
				if cached, ok := m.cachedLastRunOnly(scoped, storeKeysForGate); ok && orderCachedLastRunSuppresses(a, now, cached) {
					continue
				}
				input.needsLastRun[scoped] = true
			}
			if a.Trigger == "event" {
				input.needsCursor[scoped] = true
			}
		}
		candidates = append(candidates, orderDispatchCandidate{
			order:               a,
			target:              target,
			store:               store,
			storeKey:            storeKey,
			reservationStoreKey: orderRuntimeStoreKeyForTarget(target),
			gateStores:          storesForGate,
			gateStoreKeys:       storeKeysForGate,
		})
	}

	if len(candidates) == 0 {
		return
	}

	m.populateOrderDispatchEventLastRuns(snapshotInputs)
	snapshotCtx, cancel := context.WithTimeout(ctx, orderDispatchSnapshotBudget)
	snapshots := buildOrderDispatchSnapshots(snapshotCtx, snapshotInputs, now)
	cancel()

	limits := m.effectiveCreateCaps(now)
	candidates, rotateStart := m.rotateDispatchCandidates(candidates)
	candidateCount := len(candidates)
	advanceTo := rotateStart
	for j, candidate := range candidates {
		a := candidate.order
		scoped := a.ScopedName()
		if err := snapshots.degradation(scoped, candidate.gateStoreKeys); err != nil {
			m.recordOrderDispatchDeferred(scoped, fmt.Errorf("snapshot degraded: %w", err), stats)
			continue
		}
		if snapshots.hasOpenWork(scoped, candidate.gateStoreKeys) {
			continue
		}

		lastRunFn := func(orderName string) (time.Time, error) {
			if err := snapshots.degradation(orderName, candidate.gateStoreKeys); err != nil {
				return time.Time{}, err
			}
			last := snapshots.lastRun(orderName, candidate.gateStoreKeys)
			if cached, ok := m.cachedLastRunOnly(orderName, candidate.gateStoreKeys); ok && cached.After(last) {
				last = cached
			}
			return last, nil
		}
		cursorFn := func(orderName string) uint64 {
			if err := snapshots.degradation(orderName, candidate.gateStoreKeys); err != nil {
				return 0
			}
			return snapshots.cursor(orderName, candidate.gateStoreKeys)
		}
		triggerOpts, err := orderTriggerOptionsForTarget(cityPath, m.cfg, candidate.target, a)
		if err != nil {
			redacted := redactOrderEnvError(err, os.Environ())
			msg := fmt.Sprintf("building trigger env: %s", redacted)
			logDispatchError(m.stderr, "gc: order dispatch: building trigger env for %s: %s", a.ScopedName(), redacted)
			run, reservedAt, ok := m.reserveOrderForDispatch(ctx, candidate.store, candidate.reservationStoreKey, candidate.target.Prefix, a, now, 0, append(orderTrackingLabels(scoped), labelTriggerEnvFailed), stats)
			if !ok {
				continue
			}
			m.rememberLastRun(scoped, candidate.gateStoreKeys, reservedAt)
			if err := m.runtimeTrackingUpdate(ctx, candidate.store, run.TrackingID, beads.UpdateOpts{Labels: []string{labelTriggerEnvFailed}}, beads.WriteClassAuditRepair, "order.dispatch.trigger-env-failed", run.TrackingID); err != nil {
				m.recordOrderTrackingDegraded(scoped, run.TrackingID, "trigger-env-failed", err, stats)
			}
			m.leaseStore.markReservationDegraded(run.LeaseID, errors.New("trigger env failed"))
			stats.recordDeferred("trigger_env")
			m.rec.Record(events.Event{
				Type:    events.OrderFailed,
				Actor:   "controller",
				Subject: a.ScopedName(),
				Message: msg,
			})
			continue
		}
		result := orders.CheckTriggerWithOptions(a, now, lastRunFn, m.ep, cursorFn, triggerOpts)
		if err := snapshots.degradation(scoped, candidate.gateStoreKeys); err != nil {
			m.recordOrderDispatchDeferred(scoped, fmt.Errorf("snapshot degraded after trigger evaluation: %w", err), stats)
			continue
		}
		if orderTriggerUsesLastRun(a) && !result.LastRun.IsZero() {
			m.rememberLastRun(scoped, candidate.gateStoreKeys, result.LastRun)
		}
		if !result.Due {
			continue
		}
		if m.degradedCadenceSuppresses(scoped, a, now) {
			continue
		}
		if writerStats, ok := orderDispatchRuntimeWriterStats(candidate.store, beads.WriteClassReservation); ok && orderDispatchRuntimeWriterBackpressured(writerStats) {
			stats.recordDeferred("runtime_writer_backpressure")
			logDispatchError(m.stderr, "gc: order dispatch: deferring %s because runtime writer class %s is backpressured: active=%d queue_depth=%d queue_limit=%d breaker=%s oldest_queue_age=%s", scoped, writerStats.Class, writerStats.Active, writerStats.QueueDepth, writerStats.QueueLimit, writerStats.BreakerState, writerStats.OldestQueueAge)
			continue
		}
		limitKind := m.dispatchCreateLimitReached(stats, candidate, limits)
		if limitKind == "total" {
			// Cap reached before dispatching this due candidate. Resume the
			// round-robin here next tick so this order — and the ones after
			// it — get first crack at the budget, instead of being starved
			// by front-of-list orders.
			if candidateCount > 0 {
				m.nextCandidateStart = (rotateStart + j) % candidateCount
			}
			return
		}
		if limitKind != "" {
			continue
		}

		eventSeq, err := m.eventReservationSeq(a)
		if err != nil {
			m.recordOrderDispatchDeferred(scoped, fmt.Errorf("event cursor head: %w", err), stats)
			continue
		}
		run, reservedAt, ok := m.reserveOrderForDispatch(ctx, candidate.store, candidate.reservationStoreKey, candidate.target.Prefix, a, now, eventSeq, orderTrackingLabels(scoped), stats)
		if !ok {
			continue
		}
		m.rememberLastRun(scoped, candidate.gateStoreKeys, reservedAt)
		stats.recordDispatchCreated(a)
		if candidateCount > 0 {
			advanceTo = (rotateStart + j + 1) % candidateCount
		}

		// Fire with timeout; inflight tracks the spawned goroutine so
		// drain can wait for tracking-bead outcome persistence before
		// controller exit or config reload.
		orderToDispatch := a
		m.addInflight()
		m.launchDispatchOne(ctx, candidate.store, candidate.target, orderToDispatch, cityPath, run)
	}

	// Loop completed without hitting the cap. If anything dispatched, resume
	// the round-robin after the last dispatched candidate next tick.
	if stats.dispatchesCreated > 0 {
		m.nextCandidateStart = advanceTo
	}
}

func (m *memoryOrderDispatcher) effectiveCreateCaps(now time.Time) orderDispatchCreateCaps {
	caps := orderDispatchCreateCaps{
		total: orderDispatchMaxCreatesPerTick,
		exec:  orderDispatchMaxExecCreatesPerTick,
		wisp:  orderDispatchMaxFormulaCreatesPerTick,
	}
	explicitTotalCap := false
	explicitExecCap := false
	explicitWispCap := false
	if m != nil && m.cfg != nil {
		explicitTotalCap = m.cfg.Orders.MaxDispatchesPerTick != nil
		explicitExecCap = m.cfg.Orders.MaxExecDispatchesPerTick != nil
		explicitWispCap = m.cfg.Orders.MaxFormulaDispatchesPerTick != nil
		caps.total = m.cfg.Orders.MaxDispatchesPerTickOrDefault(caps.total)
		caps.exec = m.cfg.Orders.MaxExecDispatchesPerTickOrDefault(caps.exec)
		caps.wisp = m.cfg.Orders.MaxFormulaDispatchesPerTickOrDefault(caps.wisp)
	}
	if m.defaultDispatchStartupThrottleElapsed(now) {
		if !explicitTotalCap {
			caps.total = 0
		}
		if !explicitExecCap {
			caps.exec = 0
		}
		if !explicitWispCap {
			caps.wisp = 0
		}
	}
	return caps
}

func (m *memoryOrderDispatcher) defaultDispatchStartupThrottleElapsed(now time.Time) bool {
	if m == nil || orderDispatchStartupThrottleWindow <= 0 || m.startedAt.IsZero() {
		return false
	}
	return !now.Before(m.startedAt.Add(orderDispatchStartupThrottleWindow))
}

// rotateDispatchCandidates rotates candidates so the order at the persistent
// round-robin cursor (nextCandidateStart) leads, returning the rotated slice
// and the original index that now leads (rotateStart). It does NOT advance the
// cursor — the dispatch loop owns cursor bookkeeping so it can resume exactly
// after the last candidate that consumed budget (or at the one blocked by the
// cap/backpressure), which is what keeps front-of-list orders from starving.
func (m *memoryOrderDispatcher) rotateDispatchCandidates(candidates []orderDispatchCandidate) ([]orderDispatchCandidate, int) {
	if m == nil || m.cfg == nil || len(candidates) <= 1 {
		return candidates, 0
	}
	n := len(candidates)
	start := ((m.nextCandidateStart % n) + n) % n
	if start == 0 {
		return candidates, 0
	}
	rotated := make([]orderDispatchCandidate, 0, n)
	rotated = append(rotated, candidates[start:]...)
	rotated = append(rotated, candidates[:start]...)
	return rotated, start
}

func (m *memoryOrderDispatcher) dispatchCreateLimitReached(stats *orderDispatchTickStats, candidate orderDispatchCandidate, limits orderDispatchCreateCaps) string {
	if m == nil || stats == nil {
		return ""
	}
	if orderDispatchCapReached(stats.dispatchesCreated, limits.total) {
		return "total"
	}
	if candidate.order.IsExec() {
		if orderDispatchCapReached(stats.execDispatchesCreated, limits.exec) {
			return "exec"
		}
		return ""
	}
	if orderDispatchCapReached(stats.wispDispatchesCreated, limits.wisp) {
		return "wisp"
	}
	return ""
}

func orderDispatchCapReached(used, limit int) bool {
	return limit > 0 && used >= limit
}

func orderDispatchRuntimeWriterStats(store beads.Store, class beads.WriteClass) (beads.RuntimeWriteClassStats, bool) {
	for store != nil {
		if statsStore, ok := store.(orderDispatchRuntimeWriteStatsStore); ok {
			return statsStore.RuntimeWriteManagerStatsForClass(class), true
		}
		backing, ok := store.(orderDispatchBackingStore)
		if !ok {
			break
		}
		next := backing.Backing()
		if next == nil || next == store {
			break
		}
		store = next
	}
	return beads.RuntimeWriteClassStats{}, false
}

func orderDispatchRuntimeWriterBackpressured(stats beads.RuntimeWriteClassStats) bool {
	if stats.BreakerState != "" && stats.BreakerState != beads.RuntimeWriteBreakerClosed {
		return true
	}
	return stats.QueueLimit > 0 && stats.QueueDepth >= stats.QueueLimit
}

func (m *memoryOrderDispatcher) eventReservationSeq(a orders.Order) (uint64, error) {
	if a.Trigger != "event" || m.ep == nil {
		return 0, nil
	}
	seq, err := m.ep.LatestSeq()
	if err != nil {
		return 0, err
	}
	return seq, nil
}

func (m *memoryOrderDispatcher) reserveOrderForDispatch(ctx context.Context, store beads.Store, storeKey, storePrefix string, a orders.Order, now time.Time, eventSeq uint64, labels []string, stats *orderDispatchTickStats) (orderDispatchRun, time.Time, bool) {
	scoped := a.ScopedName()
	reservation := orderRuntimeReservationFor(a, storeKey, storePrefix, now, eventSeq)
	reservation.Lease.ControllerStartID = m.controllerStartID()
	lease, acquired, err := m.leaseStore.acquire(reservation.Lease)
	if err != nil {
		logDispatchError(m.stderr, "gc: order dispatch: acquiring runtime lease for %s: %v", scoped, err)
		stats.recordDeferred("local_lease")
		return orderDispatchRun{}, time.Time{}, false
	}
	if !acquired {
		stats.recordDeferred("local_lease")
		logDispatchError(m.stderr, "gc: order dispatch: suppressing %s behind runtime lease %s state=%s", scoped, lease.LeaseID, lease.State)
		return orderDispatchRun{}, time.Time{}, false
	}

	run := orderDispatchRun{
		TrackingID: reservation.TrackingID,
		LeaseID:    reservation.Lease.LeaseID,
		EventSeq:   eventSeq,
	}
	bead := beads.Bead{
		ID:     reservation.TrackingID,
		Title:  "order:" + scoped,
		Labels: labels,
		Metadata: map[string]string{
			"gc.order.runtime_write_isolation":  orderRuntimeLeaseVersion,
			"gc.order.scoped":                   scoped,
			"gc.order.store_key":                storeKey,
			"gc.order.trigger_kind":             a.Trigger,
			"gc.order.trigger_fingerprint":      reservation.Fingerprint,
			"gc.order.reservation_input":        orderRuntimeReservationMetadataInput(reservation.Input),
			"gc.order.reservation_input_format": "base64url-raw-v2",
			"gc.order.reservation_hash":         reservation.Hash,
			"gc.order.lease_id":                 reservation.Lease.LeaseID,
			"gc.idempotency_key":                reservation.Hash,
		},
	}
	policy := orderDispatchTrackingWritePolicy(beads.WriteClassReservation, "order.dispatch.tracking-reservation", reservation.Hash)
	created, err := beads.RuntimeCreate(ctx, store, bead, policy)
	if err == nil {
		run.TrackingReserved = true
		reservedAt := created.CreatedAt
		if reservedAt.IsZero() {
			reservedAt = now
		}
		m.leaseStore.markActive(run.LeaseID, effectiveTimeout(a, m.maxTimeout))
		return run, reservedAt, true
	}
	m.recordOrderTrackingDegraded(scoped, reservation.TrackingID, "reservation", err, stats)
	if !a.TrackingDegradedAllowed {
		logDispatchError(m.stderr, "gc: order dispatch: deferring %s because tracking reservation is degraded: %v", scoped, err)
		stats.recordTrackingWriteFailure()
		stats.recordDeferred("tracking_reservation")
		if orderTrackingReservationNotStarted(err) {
			m.leaseStore.completeAndRemove(run.LeaseID)
		} else {
			m.leaseStore.markReservationDegraded(run.LeaseID, err)
		}
		return orderDispatchRun{}, time.Time{}, false
	}
	m.leaseStore.markActive(run.LeaseID, effectiveTimeout(a, m.maxTimeout))
	m.rememberDegradedRun(scoped, now)
	return run, now, true
}

func orderTrackingReservationNotStarted(err error) bool {
	var degraded *beads.DegradedWriteError
	return errors.As(err, &degraded) && degraded.Outcome == beads.WriteOutcomeNotStarted
}

func orderDispatchTrackingWritePolicy(class beads.WriteClass, caller, idempotencyKey string) beads.WritePolicy {
	policy := beads.RuntimeWritePolicy(class, caller, idempotencyKey)
	if class != beads.WriteClassPostActionCritical && policy.Timeout > orderDispatchTrackingWriteBudget {
		policy.Timeout = orderDispatchTrackingWriteBudget
	}
	return policy
}

func (m *memoryOrderDispatcher) controllerStartID() string {
	if m == nil || m.startedAt.IsZero() {
		return ""
	}
	return m.startedAt.UTC().Format(time.RFC3339Nano)
}

func (s *orderDispatchTickStats) touchStore(key string) {
	if s == nil || key == "" {
		return
	}
	if s.storesTouched == nil {
		s.storesTouched = make(map[string]struct{})
	}
	s.storesTouched[key] = struct{}{}
}

func (s *orderDispatchTickStats) recordDeferred(reason string) {
	if s == nil {
		return
	}
	s.ordersDeferred++
	if reason == "" {
		reason = "error"
	}
	if s.deferReasons == nil {
		s.deferReasons = make(map[string]int)
	}
	s.deferReasons[reason]++
}

func (s *orderDispatchTickStats) recordDispatchCreated(a orders.Order) {
	if s == nil {
		return
	}
	s.dispatchesCreated++
	if a.IsExec() {
		s.execDispatchesCreated++
		return
	}
	s.wispDispatchesCreated++
}

func (s *orderDispatchTickStats) recordTrackingWriteFailure() {
	if s == nil {
		return
	}
	s.trackingWriteFailures++
}

func (s *orderDispatchTickStats) recordWriteDegraded() {
	if s == nil {
		return
	}
	s.writeDegraded++
}

func buildOrderDispatchSnapshots(ctx context.Context, inputs map[string]*orderDispatchSnapshotInput, now time.Time) orderDispatchSnapshots {
	if ctx == nil {
		ctx = context.Background()
	}
	keys := make([]string, 0, len(inputs))
	for key := range inputs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := orderDispatchSnapshots{byStore: make(map[string]*orderDispatchStoreSnapshot, len(keys))}
	for _, key := range keys {
		input := inputs[key]
		if input == nil || input.store == nil {
			continue
		}
		result.byStore[key] = buildOrderDispatchStoreSnapshot(ctx, input, now)
	}
	return result
}

func buildOrderDispatchStoreSnapshot(ctx context.Context, input *orderDispatchSnapshotInput, now time.Time) *orderDispatchStoreSnapshot {
	snapshot := &orderDispatchStoreSnapshot{
		storeKey:              input.storeKey,
		capturedAt:            now,
		lastRunByOrder:        make(map[string]time.Time),
		durableLastRunByOrder: make(map[string]time.Time),
		cursorByOrder:         make(map[string]uint64),
		openByOrder:           make(map[string]bool),
		degraded:              make(map[string][]error),
	}
	names := make([]string, 0, len(input.orders))
	for name := range input.orders {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if last := input.eventLastRun[name]; last.After(snapshot.lastRunByOrder[name]) {
			snapshot.lastRunByOrder[name] = last
		}
	}
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			snapshot.addDegradation(name, "snapshot deadline", err)
			continue
		}
		snapshot.captureOrder(ctx, input.store, name, input.needsLastRun[name], input.needsCursor[name])
	}
	return snapshot
}

func (m *memoryOrderDispatcher) populateOrderDispatchEventLastRuns(inputs map[string]*orderDispatchSnapshotInput) {
	if m == nil || m.ep == nil || len(inputs) == 0 {
		return
	}
	needed := map[string]struct{}{}
	for _, input := range inputs {
		if input == nil {
			continue
		}
		for name := range input.needsLastRun {
			needed[name] = struct{}{}
		}
	}
	if len(needed) == 0 {
		return
	}
	evts, err := orderDispatchRecentOrderFiredEvents(m.ep)
	if err != nil {
		return
	}
	lastByOrder := map[string]time.Time{}
	for _, evt := range evts {
		if evt.Type != events.OrderFired {
			continue
		}
		if _, ok := needed[evt.Subject]; !ok {
			continue
		}
		if evt.Ts.After(lastByOrder[evt.Subject]) {
			lastByOrder[evt.Subject] = evt.Ts
		}
	}
	if len(lastByOrder) == 0 {
		return
	}
	for _, input := range inputs {
		if input == nil {
			continue
		}
		if input.eventLastRun == nil {
			input.eventLastRun = make(map[string]time.Time)
		}
		for name := range input.needsLastRun {
			if last := lastByOrder[name]; last.After(input.eventLastRun[name]) {
				input.eventLastRun[name] = last
			}
		}
	}
}

func orderDispatchRecentOrderFiredEvents(ep events.Provider) ([]events.Event, error) {
	if ep == nil {
		return nil, nil
	}
	const orderDispatchEventLastRunTailLimit = 5000
	if tail, ok := ep.(events.TailProvider); ok {
		return tail.ListTail(events.Filter{Type: events.OrderFired}, orderDispatchEventLastRunTailLimit)
	}
	return ep.List(events.Filter{Type: events.OrderFired})
}

func (s *orderDispatchStoreSnapshot) captureOpenWorkRow(ctx context.Context, store beads.Store, scopedName string, b beads.Bead) {
	if b.Metadata["gc.order.runtime_write_isolation"] == orderRuntimeLeaseVersion && !beadLabelsContain(b.Labels, labelTriggerEnvFailed) {
		return
	}
	if beadLabelsContain(b.Labels, labelOrderTracking) {
		s.openByOrder[scopedName] = true
		return
	}
	if !isOrderWispRootCandidate(b) {
		return
	}
	if isOrderRootOnlyWispCandidate(b) {
		s.openByOrder[scopedName] = true
		return
	}
	hasOpen, err := orderDispatchHasOpenDescendants(ctx, store, b.ID)
	if err != nil {
		s.addDegradation(scopedName, "order wisp descendants", err)
		return
	}
	if hasOpen {
		s.openByOrder[scopedName] = true
	}
}

func (s *orderDispatchStoreSnapshot) captureOrder(ctx context.Context, store beads.Store, scopedName string, needsLastRun, needsCursor bool) {
	if s.openByOrder[scopedName] {
		return
	}
	s.captureOpenOrderWork(ctx, store, scopedName, needsLastRun)
	if s.openByOrder[scopedName] {
		return
	}

	if needsLastRun {
		history, err := orderDispatchRuntimeList(ctx, store, beads.ListQuery{
			Label:         orderRunLabel(scopedName),
			Limit:         orderDispatchLastRunReadLimit,
			IncludeClosed: true,
			Sort:          beads.SortCreatedDesc,
			TierMode:      beads.TierBoth,
		}, "order.dispatch.order-run-history")
		if err != nil {
			s.addDegradation(scopedName, "order-run history", err)
			return
		}
		for _, b := range history {
			if b.CreatedAt.After(s.lastRunByOrder[scopedName]) {
				s.lastRunByOrder[scopedName] = b.CreatedAt
			}
			if b.CreatedAt.After(s.durableLastRunByOrder[scopedName]) {
				s.durableLastRunByOrder[scopedName] = b.CreatedAt
			}
			if b.Status != "closed" {
				s.captureOpenWorkRow(ctx, store, scopedName, b)
			}
		}
	}

	if !needsCursor {
		return
	}
	cursorRows, err := orderDispatchRuntimeList(ctx, store, beads.ListQuery{
		Label:         "order:" + scopedName,
		Limit:         orderDispatchEventCursorReadLimit,
		IncludeClosed: true,
		Sort:          beads.SortCreatedDesc,
		TierMode:      beads.TierBoth,
	}, "order.dispatch.event-cursor")
	if err != nil {
		s.addDegradation(scopedName, "event cursor", err)
		return
	}
	if len(cursorRows) >= orderDispatchEventCursorReadLimit {
		s.addDegradation(scopedName, "event cursor", fmt.Errorf("cursor read hit cap %d", orderDispatchEventCursorReadLimit))
	}
	labelSets := make([][]string, 0, len(cursorRows))
	for _, b := range cursorRows {
		labelSets = append(labelSets, b.Labels)
	}
	s.cursorByOrder[scopedName] = orders.MaxSeqFromLabels(labelSets)
}

func (s *orderDispatchStoreSnapshot) captureOpenOrderWork(ctx context.Context, store beads.Store, scopedName string, needsLastRun bool) {
	openWork, err := orderDispatchRuntimeList(ctx, store, beads.ListQuery{
		Label:    orderRunLabel(scopedName),
		Limit:    orderDispatchOpenReadLimit,
		Sort:     beads.SortCreatedDesc,
		TierMode: beads.TierBoth,
	}, "order.dispatch.open-work")
	if err != nil {
		s.addDegradation(scopedName, "checking open work", err)
		return
	}
	if len(openWork) >= orderDispatchOpenReadLimit {
		s.addDegradation(scopedName, "checking open work", fmt.Errorf("open work read hit cap %d", orderDispatchOpenReadLimit))
	}
	for _, b := range openWork {
		if needsLastRun && b.CreatedAt.After(s.lastRunByOrder[scopedName]) {
			s.lastRunByOrder[scopedName] = b.CreatedAt
		}
		if b.CreatedAt.After(s.durableLastRunByOrder[scopedName]) {
			s.durableLastRunByOrder[scopedName] = b.CreatedAt
		}
		if b.Status == "closed" {
			continue
		}
		s.captureOpenWorkRow(ctx, store, scopedName, b)
	}
}

func orderDispatchHasOpenDescendants(ctx context.Context, store beads.Store, parentID string) (bool, error) {
	if parentID == "" {
		return false, nil
	}
	type queuedParent struct {
		id    string
		depth int
	}
	seen := map[string]struct{}{parentID: {}}
	queue := []queuedParent{{id: parentID}}
	rows := 0
	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		next := queue[0]
		queue = queue[1:]
		if next.depth >= orderDispatchDescendantMaxDepth {
			return false, fmt.Errorf("descendant depth hit cap %d", orderDispatchDescendantMaxDepth)
		}
		children, err := orderDispatchRuntimeList(ctx, store, beads.ListQuery{
			ParentID:      next.id,
			IncludeClosed: true,
			Limit:         orderDispatchOpenReadLimit,
			TierMode:      beads.TierBoth,
		}, "order.dispatch.descendants")
		if err != nil {
			return false, err
		}
		rows += len(children)
		if rows > orderDispatchDescendantMaxRows {
			return false, fmt.Errorf("descendant rows hit cap %d", orderDispatchDescendantMaxRows)
		}
		for _, child := range children {
			if child.ID == "" {
				continue
			}
			if _, ok := seen[child.ID]; ok {
				continue
			}
			seen[child.ID] = struct{}{}
			if child.Status != "closed" {
				return true, nil
			}
			queue = append(queue, queuedParent{id: child.ID, depth: next.depth + 1})
		}
	}
	return false, nil
}

func orderDispatchRuntimeList(ctx context.Context, store beads.Store, query beads.ListQuery, caller string) ([]beads.Bead, error) {
	if store == nil {
		return nil, fmt.Errorf("nil store")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	readCtx, cancel := context.WithTimeout(ctx, orderDispatchSingleReadBudget)
	defer cancel()
	policy := beads.RuntimeReadPolicy(beads.ReadClassHotAuthoritative, caller)
	policy.Timeout = orderDispatchSingleReadBudget
	if query.Limit > 0 {
		policy.MaxRows = query.Limit + 1
	}
	type listResult struct {
		rows []beads.Bead
		err  error
	}
	done := make(chan listResult, 1)
	go func() {
		rows, err := beads.RuntimeList(readCtx, store, query, policy)
		done <- listResult{rows: rows, err: err}
	}()
	select {
	case result := <-done:
		return result.rows, result.err
	case <-readCtx.Done():
		return nil, readCtx.Err()
	}
}

func (s *orderDispatchStoreSnapshot) addDegradation(orderName, shape string, err error) {
	if err == nil {
		return
	}
	s.degraded[orderName] = append(s.degraded[orderName], fmt.Errorf("%s %s: %w", s.storeKey, shape, err))
}

func (s orderDispatchSnapshots) degradation(orderName string, storeKeys []string) error {
	var errs []error
	for _, key := range storeKeys {
		snapshot := s.byStore[key]
		if snapshot == nil {
			errs = append(errs, fmt.Errorf("%s snapshot missing", key))
			continue
		}
		errs = append(errs, snapshot.degraded[orderName]...)
	}
	return errors.Join(errs...)
}

func (s orderDispatchSnapshots) hasOpenWork(orderName string, storeKeys []string) bool {
	for _, key := range storeKeys {
		if snapshot := s.byStore[key]; snapshot != nil && snapshot.openByOrder[orderName] {
			return true
		}
	}
	return false
}

func (s orderDispatchSnapshots) lastRun(orderName string, storeKeys []string) time.Time {
	var latest time.Time
	for _, key := range storeKeys {
		if snapshot := s.byStore[key]; snapshot != nil && snapshot.lastRunByOrder[orderName].After(latest) {
			latest = snapshot.lastRunByOrder[orderName]
		}
	}
	return latest
}

func (s orderDispatchSnapshots) cursor(orderName string, storeKeys []string) uint64 {
	var latest uint64
	for _, key := range storeKeys {
		if snapshot := s.byStore[key]; snapshot != nil && snapshot.cursorByOrder[orderName] > latest {
			latest = snapshot.cursorByOrder[orderName]
		}
	}
	return latest
}

func (m *memoryOrderDispatcher) cachedLastRunOnly(orderName string, storeKeys []string) (time.Time, bool) {
	key := orderHistoryCacheKey(orderName, storeKeys)
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()
	if m.lastRunCache == nil {
		return time.Time{}, false
	}
	last, ok := m.lastRunCache[key]
	return last, ok
}

func (m *memoryOrderDispatcher) recordOrderDispatchDeferred(scoped string, err error, stats *orderDispatchTickStats) {
	logDispatchError(m.stderr, "gc: order dispatch: deferring %s: %v", scoped, err)
	if stats != nil {
		stats.recordDeferred("snapshot_degraded")
	}
	if m.rec != nil {
		m.rec.Record(events.Event{
			Type:    events.OrderFailed,
			Actor:   "controller",
			Subject: scoped,
			Message: fmt.Sprintf("deferred order dispatch: %v", err),
		})
	}
}

func (m *memoryOrderDispatcher) recordOrderDispatchTick(stats *orderDispatchTickStats) {
	if m.rec == nil || stats == nil {
		return
	}
	deferReasons := make(map[string]int, len(stats.deferReasons))
	for reason, count := range stats.deferReasons {
		deferReasons[reason] = count
	}
	if len(deferReasons) == 0 {
		deferReasons = nil
	}
	payload, err := json.Marshal(events.OrderDispatchTickPayload{
		StartedAt:          stats.startedAt.UTC().Format(time.RFC3339Nano),
		DurationSeconds:    time.Since(stats.startedAt).Seconds(),
		OrdersConsidered:   stats.ordersConsidered,
		StoresTouched:      len(stats.storesTouched),
		DispatchesCreated:  stats.dispatchesCreated,
		OrdersDeferred:     stats.ordersDeferred,
		DeferReasons:       deferReasons,
		TrackingWriteFails: stats.trackingWriteFailures,
		LocalLeaseCount:    m.leaseStore.count(),
		WriteDegraded:      stats.writeDegraded,
		InFlight:           m.currentInflight(),
	})
	if err != nil {
		logDispatchError(m.stderr, "gc: order dispatch: encoding tick telemetry: %v", err)
		return
	}
	m.rec.Record(events.Event{
		Type:    events.OrderDispatchTick,
		Actor:   "controller",
		Subject: m.cityName,
		Payload: payload,
	})
}

// launchDispatchOne spawns dispatchOne with a context that cancels when
// EITHER the caller's tick ctx OR m.dispatchCtx is done — required so
// cancel() reaches goroutines whose tick ctx was context.Background().
// Falls back to the bare caller ctx when m.dispatchCtx is nil (test
// sites that don't initialize the cancel fields).
func (m *memoryOrderDispatcher) launchDispatchOne(ctx context.Context, store beads.Store, target execStoreTarget, a orders.Order, cityPath string, run orderDispatchRun) {
	if m.dispatchCtx == nil {
		go m.dispatchOne(ctx, store, target, a, cityPath, run)
		return
	}
	mergedCtx, cancelMerged := context.WithCancel(ctx)
	stopAfter := context.AfterFunc(m.dispatchCtx, cancelMerged)
	go func() {
		defer stopAfter()
		defer cancelMerged()
		m.dispatchOne(mergedCtx, store, target, a, cityPath, run)
	}()
}

// cancel signals all in-flight dispatchOne goroutines to terminate. Safe
// to call multiple times. Caller should follow with drain to wait for
// goroutine completion; dispatchOne's deferred cleanup writes the
// tracking-bead outcome before doneInflight signals drain.
func (m *memoryOrderDispatcher) cancel() {
	if m.dispatchCancel != nil {
		m.dispatchCancel()
	}
}

// addInflight increments the in-flight count and lazily creates the done
// signal. Called synchronously from dispatch on the tick goroutine.
func (m *memoryOrderDispatcher) addInflight() {
	m.inflightMu.Lock()
	m.inflightN++
	if m.inflightN == 1 {
		m.inflightDone = make(chan struct{})
	}
	m.inflightMu.Unlock()
}

// doneInflight decrements the count and signals completion when the last
// goroutine finishes. Called from dispatchOne's deferred cleanup.
func (m *memoryOrderDispatcher) doneInflight() {
	m.inflightMu.Lock()
	m.inflightN--
	if m.inflightN == 0 && m.inflightDone != nil {
		close(m.inflightDone)
		m.inflightDone = nil
	}
	m.inflightMu.Unlock()
}

func (m *memoryOrderDispatcher) currentInflight() int {
	m.inflightMu.Lock()
	defer m.inflightMu.Unlock()
	return m.inflightN
}

// drain blocks until all in-flight dispatchOne goroutines complete or ctx
// expires. It returns true when no work remains and returns immediately if
// nothing is in flight. When ctx expires, any still-running dispatches keep
// running (they will still write tracking-bead outcomes via ctx-unaware store
// calls); the startup sweep closes orphaned tracking beads on the next boot if
// drain did not have enough time to let them finish. The channel-signal design
// spawns no waiter goroutine and cannot leak state past return.
func (m *memoryOrderDispatcher) drain(ctx context.Context) bool {
	m.inflightMu.Lock()
	done := m.inflightDone
	m.inflightMu.Unlock()
	if done == nil {
		return true
	}
	select {
	case <-done:
		return true
	case <-ctx.Done():
		return false
	}
}

func (m *memoryOrderDispatcher) legacyCityStoreForTarget(cityPath string, target execStoreTarget, stores map[string]beads.Store) (beads.Store, bool) {
	if !legacyOrderCityFallbackNeeded(cityPath, target) {
		return nil, true
	}
	legacyTarget := legacyOrderCityTarget(cityPath, m.cfg)
	key := orderStoreTargetKey(legacyTarget)
	if store, ok := stores[key]; ok {
		return store, true
	}
	store, err := m.storeFn(legacyTarget)
	if err != nil {
		logDispatchError(m.stderr, "gc: order dispatch: opening legacy city store for rig order fallback: %v", err)
		return nil, false
	}
	stores[key] = store
	return store, true
}

func (m *memoryOrderDispatcher) rememberLastRun(orderName string, storeKeys []string, last time.Time) {
	key := orderHistoryCacheKey(orderName, storeKeys)
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()
	if m.lastRunCache == nil {
		m.lastRunCache = make(map[string]time.Time)
	}
	if existing, ok := m.lastRunCache[key]; !ok || existing.IsZero() || last.After(existing) {
		m.lastRunCache[key] = last
	}
}

func (m *memoryOrderDispatcher) rememberDegradedRun(orderName string, at time.Time) {
	if m == nil || orderName == "" || at.IsZero() {
		return
	}
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()
	if m.degradedRunCache == nil {
		m.degradedRunCache = make(map[string]time.Time)
	}
	if existing, ok := m.degradedRunCache[orderName]; !ok || at.After(existing) {
		m.degradedRunCache[orderName] = at
	}
}

func (m *memoryOrderDispatcher) degradedCadenceSuppresses(orderName string, a orders.Order, now time.Time) bool {
	minInterval := orderDegradedMinInterval(a)
	if m == nil || minInterval <= 0 {
		return false
	}
	m.cacheMu.Lock()
	last := m.degradedRunCache[orderName]
	m.cacheMu.Unlock()
	if last.IsZero() {
		return false
	}
	return now.Sub(last) < minInterval
}

func orderHistoryCacheKey(orderName string, storeKeys []string) string {
	return orderName + "\x00" + strings.Join(storeKeys, "\x00")
}

func orderTriggerUsesLastRun(a orders.Order) bool {
	return a.Trigger == "cooldown" || a.Trigger == "cron"
}

func orderCachedLastRunSuppresses(a orders.Order, now time.Time, cached time.Time) bool {
	if cached.IsZero() {
		return false
	}
	lastRunFn := func(string) (time.Time, error) {
		return cached, nil
	}
	result := orders.CheckTriggerWithOptions(a, now, lastRunFn, nil, nil, orders.TriggerOptions{})
	return !result.Due
}

func eventCursorLabels(scoped string, headSeq uint64) []string {
	return []string{
		fmt.Sprintf("order:%s", scoped),
		fmt.Sprintf("seq:%d", headSeq),
	}
}

// dispatchOne runs a single order dispatch in its own goroutine.
// For exec orders, runs the script directly. For formula orders,
// instantiates a wisp. Emits events and updates the tracking bead.
func (m *memoryOrderDispatcher) dispatchOne(ctx context.Context, store beads.Store, target execStoreTarget, a orders.Order, cityPath string, run orderDispatchRun) {
	// Defer order matters: doneInflight runs last, after Close makes the
	// tracking bead outcome observable to a waiting drain.
	defer m.doneInflight()
	defer func() {
		if !run.TrackingReserved {
			m.leaseStore.markCompletedPendingAudit(run.LeaseID, "tracking-reservation", errors.New("tracking reservation degraded"))
			return
		}
		if lease, ok := m.leaseStore.load(run.LeaseID); ok && lease.State == orderRuntimeLeaseCompletedPendingCritical {
			return
		}
		if err := m.closeOrderTrackingRuntime(ctx, store, run); err != nil {
			logDispatchError(m.stderr, "gc: order %s: closing tracking bead %s: %v", a.ScopedName(), run.TrackingID, err)
			m.recordOrderTrackingDegraded(a.ScopedName(), run.TrackingID, "tracking-close", err, nil)
			m.leaseStore.markCompletedPendingAudit(run.LeaseID, "tracking-close", err)
			return
		}
		m.leaseStore.completeAndRemove(run.LeaseID)
	}()

	timeout := effectiveTimeout(a, m.maxTimeout)
	childCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stopHeartbeat := m.startOrderLeaseHeartbeat(childCtx, run.LeaseID)
	defer stopHeartbeat()

	scoped := a.ScopedName()
	if a.Trigger == "event" && run.EventSeq > 0 {
		if err := m.runtimeTrackingUpdate(childCtx, store, run.TrackingID, beads.UpdateOpts{Labels: eventCursorLabels(scoped, run.EventSeq)}, beads.WriteClassCursorReservation, "order.dispatch.event-cursor", run.TrackingID); err != nil {
			logDispatchError(m.stderr, "gc: order %s: failed to persist event cursor seq=%d on tracking bead %s: %v", scoped, run.EventSeq, run.TrackingID, err)
			m.recordOrderTrackingDegraded(scoped, run.TrackingID, "event-cursor", err, nil)
			m.leaseStore.markAbandoned(run.LeaseID, err)
			return
		}
	}

	if a.IsExec() {
		m.dispatchExec(childCtx, store, target, a, cityPath, run)
	} else {
		m.dispatchWisp(childCtx, store, a, cityPath, run)
	}
}

func (m *memoryOrderDispatcher) closeOrderTrackingRuntime(ctx context.Context, store beads.Store, run orderDispatchRun) error {
	if !run.TrackingReserved || run.TrackingID == "" {
		return nil
	}
	writeCtx := orderDispatchPostActionWriteContext(ctx)
	_, err := beads.RuntimeCloseAll(writeCtx, store, []string{run.TrackingID}, map[string]string{
		"close_reason": completedOrderTrackingCloseReason,
	}, orderDispatchTrackingWritePolicy(beads.WriteClassPostActionCritical, "order.dispatch.tracking-close", run.TrackingID))
	return err
}

func (m *memoryOrderDispatcher) runtimeTrackingUpdate(ctx context.Context, store beads.Store, id string, opts beads.UpdateOpts, class beads.WriteClass, caller, idempotencyKey string) error {
	if id == "" {
		return fmt.Errorf("runtime tracking update: empty id")
	}
	policy := orderDispatchTrackingWritePolicy(class, caller, idempotencyKey)
	return beads.RuntimeUpdate(ctx, store, id, opts, policy)
}

func orderDispatchPostActionWriteContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return context.WithoutCancel(ctx)
}

func (m *memoryOrderDispatcher) recordOrderTrackingDegraded(scoped, trackingID, operation string, err error, stats *orderDispatchTickStats) {
	if err == nil {
		return
	}
	if stats != nil {
		stats.recordWriteDegraded()
	}
	if m != nil && m.rec != nil {
		msg := fmt.Sprintf("%s tracking bead %s degraded: %v", operation, trackingID, err)
		m.rec.Record(events.Event{
			Type:    events.OrderTrackingDegraded,
			Actor:   "controller",
			Subject: scoped,
			Message: msg,
		})
	}
}

func closeOrderTrackingBead(ctx context.Context, store beads.Store, trackingID string) error {
	_, err := closeAndVerifyOrderTrackingBeads(ctx, store, []string{trackingID}, map[string]string{
		"close_reason": completedOrderTrackingCloseReason,
	})
	return err
}

func (m *memoryOrderDispatcher) startOrderLeaseHeartbeat(ctx context.Context, leaseID string) func() {
	if m == nil || m.leaseStore == nil || leaseID == "" {
		return func() {}
	}
	done := make(chan struct{})
	var ctxDone <-chan struct{}
	if ctx != nil {
		ctxDone = ctx.Done()
	}
	go func() {
		ticker := time.NewTicker(orderRuntimeLeaseHeartbeatEvery)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctxDone:
				return
			case <-ticker.C:
				m.leaseStore.heartbeat(leaseID)
			}
		}
	}()
	return func() { close(done) }
}

func closeAndVerifyOrderTrackingBeads(ctx context.Context, store beads.Store, ids []string, metadata map[string]string) (int, error) {
	ids = uniqueNonEmptyOrderTrackingIDs(ids)
	if len(ids) == 0 {
		return 0, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if store == nil {
		return 0, fmt.Errorf("order-tracking close: nil store")
	}

	closed := 0
	var lastErr error
	for attempt := 1; attempt <= orderTrackingCloseVerifyAttempts; attempt++ {
		n, err := runtimeCloseOrderTrackingBeads(ctx, store, ids, metadata)
		closed += n
		if closed > len(ids) {
			closed = len(ids)
		}
		if err != nil {
			lastErr = fmt.Errorf("closing order-tracking beads %s: %w", strings.Join(ids, ", "), err)
			if attempt < orderTrackingCloseVerifyAttempts {
				if waitErr := waitOrderTrackingCloseRetry(ctx); waitErr != nil {
					return closed, errors.Join(lastErr, waitErr)
				}
			}
			continue
		}
		openIDs, err := openOrderTrackingIDs(ctx, store, ids)
		if err != nil {
			lastErr = fmt.Errorf("verifying order-tracking close for %s: %w", strings.Join(ids, ", "), err)
			if attempt < orderTrackingCloseVerifyAttempts {
				if waitErr := waitOrderTrackingCloseRetry(ctx); waitErr != nil {
					return closed, errors.Join(lastErr, waitErr)
				}
			}
			continue
		}
		if len(openIDs) == 0 {
			return closed, nil
		}
		lastErr = fmt.Errorf("verifying order-tracking close: still open: %s", strings.Join(openIDs, ", "))
		if attempt < orderTrackingCloseVerifyAttempts {
			if waitErr := waitOrderTrackingCloseRetry(ctx); waitErr != nil {
				return closed, errors.Join(lastErr, waitErr)
			}
		}
	}
	return closed, lastErr
}

func runtimeCloseOrderTrackingBeads(ctx context.Context, store beads.Store, ids []string, metadata map[string]string) (int, error) {
	if _, ok := store.(beads.RuntimeWriter); !ok {
		return store.CloseAll(ids, metadata)
	}
	policy := beads.RuntimeWritePolicy(beads.WriteClassMaintenance, "order.tracking.sweep-close", "order-tracking-close:"+strings.Join(ids, ","))
	return beads.RuntimeCloseAll(ctx, store, ids, metadata, policy)
}

func waitOrderTrackingCloseRetry(ctx context.Context) error {
	timer := time.NewTimer(orderTrackingCloseVerifyRetryDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func uniqueNonEmptyOrderTrackingIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func openOrderTrackingIDs(ctx context.Context, store beads.Store, ids []string) ([]string, error) {
	var openIDs []string
	for _, id := range ids {
		b, err := runtimeGetOrderTrackingBead(ctx, store, id)
		if errors.Is(err, beads.ErrNotFound) {
			continue
		}
		if err != nil {
			return openIDs, err
		}
		if b.Status != "closed" {
			openIDs = append(openIDs, id)
		}
	}
	return openIDs, nil
}

func runtimeGetOrderTrackingBead(ctx context.Context, store beads.Store, id string) (beads.Bead, error) {
	if runtimeStore, ok := store.(interface {
		RuntimeGet(context.Context, string, beads.ReadPolicy) (beads.Bead, error)
	}); ok {
		policy := beads.RuntimeReadPolicy(beads.ReadClassHotAuthoritative, "order.tracking.close-verify")
		return runtimeStore.RuntimeGet(ctx, id, policy)
	}
	return store.Get(id)
}

// dispatchExec runs an exec order's shell command.
func (m *memoryOrderDispatcher) dispatchExec(ctx context.Context, store beads.Store, target execStoreTarget, a orders.Order, cityPath string, run orderDispatchRun) {
	scoped := a.ScopedName()
	labels := []string{"exec"}

	env, err := orderExecEnvWithError(cityPath, m.cfg, target, a)
	var output []byte
	var execErrMsg string
	if err != nil {
		redactionEnv := append(os.Environ(), env...)
		redacted := redactOrderEnvError(err, redactionEnv)
		execErrMsg = "exec env failed: " + redacted
		labels = []string{"exec-env-failed"}
		logDispatchError(m.stderr, "gc: order exec %s env failed: %s", scoped, redacted)
	} else {
		m.rec.Record(events.Event{
			Type:    events.OrderFired,
			Actor:   "controller",
			Subject: scoped,
		})
		output, err = m.execRun(ctx, a.Exec, target.ScopeRoot, env)
		if err != nil {
			redactionEnv := append(os.Environ(), env...)
			execErrMsg = execenv.RedactText(err.Error(), redactionEnv)
			labels = []string{"exec-failed"}
			logDispatchError(m.stderr, "gc: order exec %s failed: %s", scoped, execErrMsg)
			if len(output) > 0 {
				logDispatchError(m.stderr, "gc: order exec %s output: %s", scoped, execenv.RedactText(string(output), redactionEnv))
			}
		}
	}

	// Label tracking bead with outcome via store (not CLI). For event execs,
	// cursor labels were already persisted before the command ran.
	if run.TrackingReserved {
		writeCtx := orderDispatchPostActionWriteContext(ctx)
		if err := m.runtimeTrackingUpdate(writeCtx, store, run.TrackingID, beads.UpdateOpts{Labels: labels}, beads.WriteClassPostActionCritical, "order.dispatch.exec-outcome", run.TrackingID); err != nil {
			logDispatchError(m.stderr, "gc: order %s: failed to label exec tracking bead %s: %v", scoped, run.TrackingID, err)
			m.recordOrderTrackingDegraded(scoped, run.TrackingID, "exec-outcome", err, nil)
		}
	}
	if execErrMsg != "" {
		if run.EventSeq > 0 {
			execErrMsg = fmt.Sprintf("seq=%d: %s", run.EventSeq, execErrMsg)
		}
		m.rec.Record(events.Event{
			Type:    events.OrderFailed,
			Actor:   "controller",
			Subject: scoped,
			Message: execErrMsg,
		})
		return
	}
	m.rec.Record(events.Event{
		Type:    events.OrderCompleted,
		Actor:   "controller",
		Subject: scoped,
	})
}

func redactOrderEnvError(err error, env []string) string {
	if err == nil {
		return ""
	}
	return execenv.RedactText(err.Error(), env)
}

// dispatchWisp instantiates a wisp from the order's formula.
func (m *memoryOrderDispatcher) dispatchWisp(ctx context.Context, store beads.Store, a orders.Order, cityPath string, run orderDispatchRun) {
	scoped := a.ScopedName()

	if err := ctx.Err(); err != nil {
		m.rec.Record(events.Event{
			Type:    events.OrderFailed,
			Actor:   "controller",
			Subject: scoped,
			Message: err.Error(),
		})
		if run.TrackingReserved {
			writeCtx := orderDispatchPostActionWriteContext(ctx)
			_ = m.runtimeTrackingUpdate(writeCtx, store, run.TrackingID, beads.UpdateOpts{Labels: []string{"wisp", "wisp-canceled"}}, beads.WriteClassPostActionCritical, "order.dispatch.wisp-canceled", run.TrackingID)
		}
		return
	}

	var searchPaths []string
	if a.FormulaLayer != "" {
		searchPaths = []string{a.FormulaLayer}
	}
	recipe, err := formula.CompileWithoutRuntimeVarValidation(ctx, a.Formula, searchPaths, nil)
	if err != nil {
		m.rec.Record(events.Event{
			Type:    events.OrderFailed,
			Actor:   "controller",
			Subject: scoped,
			Message: err.Error(),
		})
		m.markTrackingFailure(ctx, store, run, scoped, a)
		return
	}
	if err := molecule.ValidateRecipeRuntimeVars(recipe, molecule.Options{}); err != nil {
		m.rec.Record(events.Event{
			Type:    events.OrderFailed,
			Actor:   "controller",
			Subject: scoped,
			Message: err.Error(),
		})
		m.markTrackingFailure(ctx, store, run, scoped, a)
		return
	}

	var pool string
	if a.Pool != "" {
		pool, err = qualifyOrderPool(a, m.cfg)
		if err != nil {
			logDispatchError(m.stderr, "gc: order %s: %v", scoped, err)
			m.rec.Record(events.Event{
				Type:    events.OrderFailed,
				Actor:   "controller",
				Subject: scoped,
				Message: err.Error(),
			})
			m.markTrackingFailure(ctx, store, run, scoped, a)
			return
		}
	}

	// Decorate graph workflow recipes with routing metadata so child step
	// beads get gc.routed_to set before instantiation.
	if a.Pool != "" {
		if err := applyGraphRouting(recipe, nil, pool, nil, "", "", "", "", store, m.cityName, cityPath, m.cfg); err != nil {
			logDispatchError(m.stderr, "gc: order %s: routing decoration failed: %v", scoped, err)
			// Non-fatal — molecule still works, just without step-level routing.
		}
	}

	m.rec.Record(events.Event{
		Type:    events.OrderFired,
		Actor:   "controller",
		Subject: scoped,
	})

	cookResult, err := molecule.Instantiate(ctx, store, recipe, molecule.Options{})
	if err != nil {
		m.rec.Record(events.Event{
			Type:    events.OrderFailed,
			Actor:   "controller",
			Subject: scoped,
			Message: err.Error(),
		})
		m.markTrackingFailure(ctx, store, run, scoped, a)
		return
	}

	rootID := cookResult.RootID

	// Stamp the created wisp through the store contract rather than a raw
	// bd subprocess so controller dispatch stays provider-aware.
	update := beads.UpdateOpts{Labels: []string{"order-run:" + scoped}}
	if a.Trigger == "event" && m.ep != nil {
		update.Labels = append(update.Labels,
			fmt.Sprintf("order:%s", scoped),
			fmt.Sprintf("seq:%d", run.EventSeq),
		)
	}
	if a.Pool != "" {
		update.Metadata = routedwork.FormulaOrderPoolDemandMetadata(pool)
	}
	writeCtx := orderDispatchPostActionWriteContext(ctx)
	if err := m.runtimeTrackingUpdate(writeCtx, store, rootID, update, beads.WriteClassPostActionCritical, "order.dispatch.wisp-root-critical", rootID); err != nil {
		// Label failure is critical for duplicate-dispatch prevention.
		// Log and emit an event so operators can investigate.
		logDispatchError(m.stderr, "gc: order %s: failed to label wisp %s: %v", scoped, rootID, err)
		m.rec.Record(events.Event{
			Type:    events.OrderFailed,
			Actor:   "controller",
			Subject: scoped,
			Message: fmt.Sprintf("wisp %s created but label failed: %v", rootID, err),
		})
		m.leaseStore.markCompletedPendingCritical(run.LeaseID, "wisp-root-label:"+rootID, err)
		m.markTrackingFailure(ctx, store, run, scoped, a)
		return
	}

	m.rec.Record(events.Event{
		Type:    events.OrderCompleted,
		Actor:   "controller",
		Subject: scoped,
	})

	// Label tracking bead with outcome.
	if run.TrackingReserved {
		if err := m.runtimeTrackingUpdate(writeCtx, store, run.TrackingID, beads.UpdateOpts{Labels: []string{"wisp"}}, beads.WriteClassPostActionCritical, "order.dispatch.wisp-outcome", run.TrackingID); err != nil {
			m.recordOrderTrackingDegraded(scoped, run.TrackingID, "wisp-outcome", err, nil)
		}
	}
}

// orderRigSuspended reports whether the order targets a suspended rig.
// It derives the effective target rig from the qualified pool (after
// rig-prefix resolution) using the canonical ParseQualifiedName parser,
// then checks whether that rig is suspended.
func (m *memoryOrderDispatcher) orderRigSuspended(a orders.Order) bool {
	if m.cfg == nil {
		return false
	}
	qualified, err := qualifyOrderPool(a, m.cfg)
	if err != nil {
		return m.rigSuspendedByName(a.Rig)
	}
	rigName, _ := config.ParseQualifiedName(qualified)
	if rigName == "" {
		rigName = a.Rig
	}
	return m.rigSuspendedByName(rigName)
}

func (m *memoryOrderDispatcher) markTrackingFailure(ctx context.Context, store beads.Store, run orderDispatchRun, scoped string, a orders.Order) {
	if !run.TrackingReserved {
		return
	}
	labels := []string{"wisp", "wisp-failed"}
	if a.Trigger == "event" && run.EventSeq > 0 {
		labels = append(labels, eventCursorLabels(scoped, run.EventSeq)...)
	}
	writeCtx := orderDispatchPostActionWriteContext(ctx)
	if err := m.runtimeTrackingUpdate(writeCtx, store, run.TrackingID, beads.UpdateOpts{Labels: labels}, beads.WriteClassPostActionCritical, "order.dispatch.mark-failed", run.TrackingID); err != nil {
		logDispatchError(m.stderr, "gc: order %s: failed to mark tracking bead %s as failed: %v", scoped, run.TrackingID, err)
		m.recordOrderTrackingDegraded(scoped, run.TrackingID, "mark-failed", err, nil)
	}
}

func (m *memoryOrderDispatcher) rigSuspendedByName(rigName string) bool {
	if rigName == "" {
		return false
	}
	for _, r := range m.cfg.Rigs {
		if r.Name == rigName {
			return r.Suspended
		}
	}
	return false
}

// hasOpenWorkStrict reports whether any in-flight work exists for this order.
//
// Tracking beads carry order-tracking:<scoped> as the hot single-flight label.
// If that scoped label is present, the gate can avoid scanning the historical
// order-run:<scoped> label. The order-run fallback is retained for legacy
// tracking beads and wisp roots created before the scoped label existed.
//
// Wisp root beads also carry order-run:<scoped> so history/API feeds can
// attribute the wisp to its order, but they never carry labelOrderTracking.
// Molecule roots never auto-close when their step beads finish, so a leftover
// open root with all-closed children is orphan state and must not permanently
// block re-dispatch. A root whose children are still open is in-flight work and
// must block duplicate pours.
func (m *memoryOrderDispatcher) hasOpenWorkStrict(store beads.Store, scopedName string) (bool, error) {
	tracking, err := store.List(beads.ListQuery{
		Label: scopedOrderTrackingLabel(scopedName),
		Limit: 1,
	})
	if err != nil {
		return false, fmt.Errorf("listing scoped order-tracking beads: %w", err)
	}
	for _, b := range tracking {
		if b.Status != "closed" {
			return true, nil
		}
	}

	results, err := store.List(beads.ListQuery{
		Label: orderRunLabel(scopedName),
		Sort:  beads.SortCreatedDesc,
	})
	if err != nil {
		return false, fmt.Errorf("listing order work beads: %w", err)
	}
	for _, b := range results {
		if b.Status == "closed" {
			continue
		}
		if beadLabelsContain(b.Labels, labelOrderTracking) {
			return true, nil
		}
		if !isOrderWispRootCandidate(b) {
			continue
		}
		if isOrderRootOnlyWispCandidate(b) {
			return true, nil
		}
		hasOpenDescendants, err := storeHasOpenDescendants(store, b.ID, false)
		if err != nil {
			return false, fmt.Errorf("checking open descendants of wisp %s: %w", b.ID, err)
		}
		if hasOpenDescendants {
			return true, nil
		}
	}
	return false, nil
}

func isOrderWispRootCandidate(b beads.Bead) bool {
	if beads.IsMoleculeType(b.Type) {
		return true
	}
	return b.Metadata["gc.kind"] == "workflow" || b.Metadata["gc.kind"] == "wisp"
}

func isOrderRootOnlyWispCandidate(b beads.Bead) bool {
	return b.Metadata["gc.kind"] == "wisp" && !beads.IsMoleculeType(b.Type)
}

// storeHasOpenDescendants reports whether any transitive child of parentID is
// non-closed. It includes closed intermediate nodes so nested molecule work
// remains visible after a direct child step has completed.
func storeHasOpenDescendants(store beads.Store, parentID string, includeWisps bool) (bool, error) {
	seen := map[string]struct{}{parentID: {}}
	queue := []string{parentID}
	for len(queue) > 0 {
		parentID := queue[0]
		queue = queue[1:]

		query := beads.ListQuery{
			ParentID:      parentID,
			IncludeClosed: true,
		}
		if includeWisps {
			query.TierMode = beads.TierBoth
		}
		children, err := store.List(query)
		if err != nil {
			return false, err
		}
		for _, c := range children {
			if c.ID == "" {
				continue
			}
			if _, ok := seen[c.ID]; ok {
				continue
			}
			seen[c.ID] = struct{}{}
			if c.Status != "closed" {
				return true, nil
			}
			queue = append(queue, c.ID)
		}
	}
	return false, nil
}

// sweepOrphanedOrderTracking closes any open order-tracking beads left
// behind by a previous controller instance. Returns the count of beads
// closed. This is non-fatal: dispatch proceeds even if the sweep fails.
func sweepOrphanedOrderTracking(store beads.Store) (int, error) {
	// ListByLabel without IncludeClosed returns only open beads.
	all, err := store.ListByLabel(labelOrderTracking, 0, beads.WithBothTiers)
	if err != nil {
		return 0, fmt.Errorf("listing order-tracking beads: %w", err)
	}
	if len(all) == 0 {
		return 0, nil
	}
	ids := make([]string, 0, len(all))
	for _, b := range all {
		if beadLabelsContain(b.Labels, labelTriggerEnvFailed) {
			continue
		}
		ids = append(ids, b.ID)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	n, err := closeAndVerifyOrderTrackingBeads(context.Background(), store, ids, map[string]string{
		"close_reason": orphanedOrderTrackingCloseReason,
	})
	if err != nil {
		return n, fmt.Errorf("closing orphaned order-tracking beads: %w", err)
	}
	return n, nil
}

func beadLabelsContain(labels []string, want string) bool {
	for _, label := range labels {
		if label == want {
			return true
		}
	}
	return false
}

type orderTrackingSweepResult struct {
	trackingClosed int
	wispClosed     int
	storesSwept    int
	sweptStoreKeys map[string]struct{}
}

// sweepStaleOrderTracking closes open order-tracking beads whose creation
// timestamp is older than staleAfter. When onlyOrders is non-empty, it only
// closes tracking beads for those scoped order names.
func sweepStaleOrderTracking(store beads.Store, now time.Time, staleAfter time.Duration, onlyOrders map[string]struct{}, initiator string) (int, error) {
	result, err := sweepStaleOrderTrackingWithOptions(store, now, staleAfter, onlyOrders, initiator, false)
	return result.trackingClosed, err
}

func sweepStaleOrderTrackingAcrossStores(stores []beads.Store, now time.Time, staleAfter time.Duration, onlyOrders map[string]struct{}, initiator string, includeWispSubtrees bool) (orderTrackingSweepResult, error) {
	return sweepStaleOrderTrackingAcrossStoresWithOptions(stores, now, staleAfter, onlyOrders, initiator, includeWispSubtrees, false)
}

func sweepStaleOrderTrackingAcrossStoresWithOptions(stores []beads.Store, now time.Time, staleAfter time.Duration, onlyOrders map[string]struct{}, initiator string, includeWispSubtrees, requireAbandonedWisps bool) (orderTrackingSweepResult, error) {
	if staleAfter <= 0 {
		return orderTrackingSweepResult{}, fmt.Errorf("stale-after must be positive")
	}
	if includeWispSubtrees && len(onlyOrders) == 0 {
		return orderTrackingSweepResult{}, fmt.Errorf("include-wisps requires at least one order name")
	}
	result := orderTrackingSweepResult{}
	var errs []error
	for i, store := range stores {
		if store == nil {
			continue
		}
		partial, err := sweepStaleOrderTrackingWithWispOptions(store, now, staleAfter, onlyOrders, initiator, includeWispSubtrees, requireAbandonedWisps)
		result.trackingClosed += partial.trackingClosed
		result.wispClosed += partial.wispClosed
		if err != nil {
			errs = append(errs, fmt.Errorf("sweeping order-tracking %s: %w", orderTrackingSweepStoreLabel(store, i), err))
			continue
		}
		result.storesSwept++
		if key := orderTrackingSweepStoreKey(store); key != "" {
			if result.sweptStoreKeys == nil {
				result.sweptStoreKeys = make(map[string]struct{})
			}
			result.sweptStoreKeys[key] = struct{}{}
		}
	}
	return result, errors.Join(errs...)
}

func orderTrackingSweepStoreKey(store beads.Store) string {
	type keyed interface {
		orderTrackingSweepKey() string
	}
	if keyedStore, ok := store.(keyed); ok {
		return strings.TrimSpace(keyedStore.orderTrackingSweepKey())
	}
	return ""
}

func orderTrackingSweepStoreLabel(store beads.Store, index int) string {
	type labeled interface {
		orderTrackingSweepLabel() string
	}
	if labeledStore, ok := store.(labeled); ok {
		if label := strings.TrimSpace(labeledStore.orderTrackingSweepLabel()); label != "" {
			return label
		}
	}
	return fmt.Sprintf("store %d", index+1)
}

func sweepStaleOrderTrackingWithOptions(store beads.Store, now time.Time, staleAfter time.Duration, onlyOrders map[string]struct{}, initiator string, includeWispSubtrees bool) (orderTrackingSweepResult, error) {
	return sweepStaleOrderTrackingWithWispOptions(store, now, staleAfter, onlyOrders, initiator, includeWispSubtrees, false)
}

func sweepStaleOrderTrackingWithWispOptions(store beads.Store, now time.Time, staleAfter time.Duration, onlyOrders map[string]struct{}, initiator string, includeWispSubtrees, requireAbandonedWisps bool) (orderTrackingSweepResult, error) {
	if staleAfter <= 0 {
		return orderTrackingSweepResult{}, fmt.Errorf("stale-after must be positive")
	}
	if includeWispSubtrees && len(onlyOrders) == 0 {
		return orderTrackingSweepResult{}, fmt.Errorf("include-wisps requires at least one order name")
	}
	all, err := listOrderTrackingCandidates(store, onlyOrders)
	if err != nil {
		return orderTrackingSweepResult{}, fmt.Errorf("listing order-tracking beads: %w", err)
	}

	cutoff := now.Add(-staleAfter)
	result := orderTrackingSweepResult{}
	var ids []string
	for _, b := range all {
		if len(onlyOrders) > 0 {
			name, ok := orderNameFromTrackingBead(b)
			if !ok {
				continue
			}
			if _, ok := onlyOrders[name]; !ok {
				continue
			}
		}
		if b.CreatedAt.IsZero() || b.CreatedAt.After(cutoff) {
			continue
		}
		ids = append(ids, b.ID)
	}
	if len(ids) == 0 {
		if !includeWispSubtrees {
			return result, nil
		}
	} else {
		metadata := map[string]string{
			"order_tracking_sweep": orderTrackingSweepMetadataReason,
			"close_reason":         staleOrderTrackingCloseReason,
		}
		if initiator != "" {
			metadata["order_tracking_sweep_by"] = initiator
		}
		n, err := closeAndVerifyOrderTrackingBeads(context.Background(), store, ids, metadata)
		result.trackingClosed = n
		if err != nil {
			return result, fmt.Errorf("closing stale order-tracking beads: %w", err)
		}
	}

	if includeWispSubtrees {
		n, err := sweepStaleOrderWispSubtreesWithOptions(store, cutoff, onlyOrders, initiator, requireAbandonedWisps)
		result.wispClosed = n
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

func listOrderTrackingCandidates(store beads.Store, onlyOrders map[string]struct{}) ([]beads.Bead, error) {
	if len(onlyOrders) == 0 {
		return store.ListByLabel(labelOrderTracking, 0, beads.WithBothTiers)
	}
	seen := make(map[string]struct{})
	var out []beads.Bead
	var errs []error
	for orderName := range onlyOrders {
		for _, query := range []beads.ListQuery{
			{Label: scopedOrderTrackingLabel(orderName), TierMode: beads.TierBoth},
			{Label: orderRunLabel(orderName), TierMode: beads.TierBoth},
		} {
			items, err := store.List(query)
			if err != nil {
				errs = append(errs, fmt.Errorf("listing %s: %w", query.Label, err))
				continue
			}
			for _, b := range items {
				if b.ID == "" {
					continue
				}
				if _, ok := seen[b.ID]; ok {
					continue
				}
				if !beadLabelsContain(b.Labels, labelOrderTracking) {
					continue
				}
				seen[b.ID] = struct{}{}
				out = append(out, b)
			}
		}
	}
	return out, errors.Join(errs...)
}

func sweepStaleOrderWispSubtrees(store beads.Store, cutoff time.Time, onlyOrders map[string]struct{}, initiator string) (int, error) {
	return sweepStaleOrderWispSubtreesWithOptions(store, cutoff, onlyOrders, initiator, false)
}

func sweepStaleOrderWispSubtreesWithOptions(store beads.Store, cutoff time.Time, onlyOrders map[string]struct{}, initiator string, requireAbandoned bool) (int, error) {
	roots, err := staleOrderWispRoots(store, cutoff, onlyOrders)
	if err != nil {
		return 0, err
	}
	ids := make([]string, 0, len(roots))
	seen := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		if root.ID == "" || root.Status == "closed" {
			continue
		}
		if beadLabelsContain(root.Labels, labelOrderTracking) {
			continue
		}
		if !isOrderWispRootCandidate(root) {
			continue
		}
		if !isOrderRootOnlyWispCandidate(root) {
			openDescendants, err := storeHasOpenDescendants(store, root.ID, true)
			if err != nil {
				return 0, fmt.Errorf("checking stale wisp descendants of %s: %w", root.ID, err)
			}
			if !openDescendants {
				continue
			}
		}
		subtree, err := collectOrderWispSubtree(store, root)
		if err != nil {
			return 0, fmt.Errorf("collecting stale wisp subtree %s: %w", root.ID, err)
		}
		if !openSubtreeOlderThan(subtree, cutoff) {
			continue
		}
		if requireAbandoned && orderWispSubtreeHasActiveOwner(subtree) {
			continue
		}
		for _, id := range staleOrderWispSubtreeCloseIDs(subtree) {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return 0, nil
	}
	ordered, err := closeorder.Order(store, ids)
	if err != nil {
		return 0, fmt.Errorf("ordering stale order wisp closes: %w", err)
	}
	metadata := map[string]string{
		"order_tracking_sweep": orderTrackingSweepMetadataReason,
		"order_wisp_sweep":     "stale-order-wisp",
		"close_reason":         staleOrderWispCloseReason,
	}
	if initiator != "" {
		metadata["order_tracking_sweep_by"] = initiator
	}
	n, err := store.CloseAll(ordered, metadata)
	if err != nil {
		return n, fmt.Errorf("closing stale order wisp subtrees: %w", err)
	}
	return n, nil
}

func orderWispSubtreeHasActiveOwner(subtree []beads.Bead) bool {
	for _, b := range subtree {
		if b.Status == "closed" {
			continue
		}
		if strings.TrimSpace(b.Assignee) != "" {
			return true
		}
		if status := strings.TrimSpace(b.Status); status != "" && status != "open" {
			return true
		}
	}
	return false
}

func staleOrderWispRoots(store beads.Store, cutoff time.Time, onlyOrders map[string]struct{}) ([]beads.Bead, error) {
	if len(onlyOrders) == 0 {
		return nil, fmt.Errorf("include-wisps requires at least one order name")
	}
	var roots []beads.Bead
	for orderName := range onlyOrders {
		matches, err := store.List(beads.ListQuery{
			Label:         "order-run:" + orderName,
			CreatedBefore: cutoff,
			TierMode:      beads.TierBoth,
		})
		if err != nil {
			return nil, fmt.Errorf("listing stale order wisps for %s: %w", orderName, err)
		}
		roots = append(roots, matches...)
	}
	return roots, nil
}

func collectOrderWispSubtree(store beads.Store, root beads.Bead) ([]beads.Bead, error) {
	if root.ID == "" {
		return nil, nil
	}
	seen := map[string]struct{}{root.ID: {}}
	out := []beads.Bead{root}
	queue := []string{root.ID}
	for len(queue) > 0 {
		parentID := queue[0]
		queue = queue[1:]

		children, err := store.List(beads.ListQuery{
			ParentID:      parentID,
			IncludeClosed: true,
			TierMode:      beads.TierBoth,
		})
		if err != nil {
			return nil, err
		}
		for _, child := range children {
			if child.ID == "" {
				continue
			}
			if _, ok := seen[child.ID]; ok {
				continue
			}
			seen[child.ID] = struct{}{}
			out = append(out, child)
			queue = append(queue, child.ID)
		}
	}
	return out, nil
}

func staleOrderWispSubtreeCloseIDs(subtree []beads.Bead) []string {
	if len(subtree) == 0 {
		return nil
	}
	byID := make(map[string]beads.Bead, len(subtree))
	for _, bead := range subtree {
		if bead.ID != "" {
			byID[bead.ID] = bead
		}
	}
	depthMemo := make(map[string]int, len(subtree))
	const visitingDepth = -1
	var depth func(string) int
	depth = func(id string) int {
		if d, ok := depthMemo[id]; ok {
			if d == visitingDepth {
				return 0
			}
			return d
		}
		bead, ok := byID[id]
		if !ok {
			return 0
		}
		parentID := strings.TrimSpace(bead.ParentID)
		if parentID == "" || parentID == id {
			depthMemo[id] = 0
			return 0
		}
		parent, ok := byID[parentID]
		if !ok || parent.ID == "" {
			depthMemo[id] = 0
			return 0
		}
		depthMemo[id] = visitingDepth
		d := depth(parentID) + 1
		depthMemo[id] = d
		return d
	}

	ordered := append([]beads.Bead(nil), subtree...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if da, db := depth(ordered[i].ID), depth(ordered[j].ID); da != db {
			return da > db
		}
		return ordered[i].ID < ordered[j].ID
	})

	ids := make([]string, 0, len(ordered))
	for _, bead := range ordered {
		if bead.ID == "" || bead.Status == "closed" {
			continue
		}
		ids = append(ids, bead.ID)
	}
	return ids
}

func openSubtreeOlderThan(subtree []beads.Bead, cutoff time.Time) bool {
	for _, b := range subtree {
		if b.Status == "closed" {
			continue
		}
		if b.CreatedAt.IsZero() || !b.CreatedAt.Before(cutoff) {
			return false
		}
	}
	return true
}

func orderNameFromTrackingBead(b beads.Bead) (string, bool) {
	for _, label := range b.Labels {
		if name, ok := strings.CutPrefix(label, labelOrderTracking+":"); ok && name != "" {
			return name, true
		}
		if name, ok := strings.CutPrefix(label, "order-run:"); ok && name != "" {
			return name, true
		}
	}
	if name, ok := strings.CutPrefix(b.Title, "order:"); ok && name != "" {
		return name, true
	}
	return "", false
}

// sweepOrphanedOrderTrackingRetry calls sweepOrphanedOrderTracking with
// bounded retries. On startup the bead store's backing server may not be
// query-ready yet (dolt cold-start race, #753). Errors are retried; the
// total count of beads closed across attempts is returned. Retrying on
// partial closes is safe because beads.Store.CloseAll skips already-closed
// beads (see internal/beads/beads.go). The wrapper sleeps for up to
// attempts*backoff in the worst case.
func sweepOrphanedOrderTrackingRetry(store beads.Store, attempts int, backoff time.Duration) (int, error) { //nolint:unparam // attempts is configurable for testability
	if attempts <= 0 {
		attempts = 1
	}
	total := 0
	var err error
	for i := range attempts {
		var n int
		n, err = sweepOrphanedOrderTracking(store)
		total += n
		if err == nil {
			return total, nil
		}
		if i == attempts-1 {
			return total, fmt.Errorf("sweep failed after %d attempts: %w", attempts, err)
		}
		time.Sleep(backoff)
	}
	return total, err
}

// effectiveTimeout returns the timeout to use for an order dispatch.
// Uses the order's configured timeout (or default), capped by maxTimeout.
func effectiveTimeout(a orders.Order, maxTimeout time.Duration) time.Duration {
	t := a.TimeoutOrDefault()
	if maxTimeout > 0 && t > maxTimeout {
		return maxTimeout
	}
	return t
}

// rigExclusiveLayers returns the suffix of rigLayers that is not in
// cityLayers. Since rig layers are built as [cityLayers..., rigTopoLayers...,
// rigLocalLayer], we strip the city prefix to avoid double-scanning city
// orders.
func rigExclusiveLayers(rigLayers, cityLayers []string) []string {
	return orderdiscovery.RigExclusiveLayers(rigLayers, cityLayers)
}

// qualifyPool resolves a raw pool name from an order TOML to the qualified
// form used by Agent.QualifiedName() — the same string the scaler queries
// via gc.routed_to. Three layers of qualification stack:
//
//  1. If pool already contains "/" it is rig-qualified — pass through.
//  2. If pool exactly matches a configured binding-qualified target
//     ("binding.name"), preserve that target and still stack the rig prefix
//     when present.
//  3. If the order came from an imported pack, prefer same-source agents when
//     resolving a bare pool name so pack-local orders stay pack-local even if
//     other scopes also export the same bare agent name.
//  4. Otherwise look up agents in cfg.Agents whose Dir matches rig
//     (city orders use rig=="") and Name matches pool. If exactly one target
//     resolves, swap pool for the binding-qualified form ("binding.name")
//     before any rig prefixing. This handles V2 pack imports where the
//     dispatched wisp must carry "binding.name" so the agent's default
//     scale_check matches its own qualified name.
//
// Ambiguity is a hard failure: silently stamping the bare pool string would
// recreate the exact route/scaler mismatch this helper exists to prevent.
// nil cfg preserves the rig-only behavior so call sites without a loaded
// city remain stable. Dotted values that do not match a configured bound
// target are preserved for backward compatibility.
func qualifyOrderPool(a orders.Order, cfg *config.City) (string, error) {
	return qualifyPool(a.Pool, a.Rig, cfg, orderPoolSourceDirHint(a))
}

func orderPoolSourceDirHint(a orders.Order) string {
	if a.FormulaLayer == "" {
		return ""
	}
	return filepath.Clean(filepath.Dir(a.FormulaLayer))
}

func qualifyPool(pool, rig string, cfg *config.City, sourceDirHint string) (string, error) {
	if strings.Contains(pool, "/") {
		return pool, nil
	}
	if cfg == nil {
		if rig == "" {
			return pool, nil
		}
		return rig + "/" + pool, nil
	}

	qualified := pool
	scope := "city order"
	if rig != "" {
		scope = fmt.Sprintf("rig %q", rig)
	}

	var exactQualified []string
	var sourceScopedMatches []string
	var localBareMatches []string
	var bareMatches []string
	cleanHint := ""
	if sourceDirHint != "" {
		cleanHint = filepath.Clean(sourceDirHint)
	}
	for i := range cfg.Agents {
		a := &cfg.Agents[i]
		if a.Dir != rig {
			continue
		}
		switch {
		case strings.Contains(pool, ".") && a.BindingQualifiedName() == pool:
			exactQualified = appendUniquePoolTarget(exactQualified, a.BindingQualifiedName())
		case a.Name == pool:
			bareMatches = appendUniquePoolTarget(bareMatches, a.BindingQualifiedName())
			if a.BindingName == "" {
				localBareMatches = appendUniquePoolTarget(localBareMatches, a.BindingQualifiedName())
			}
			if cleanHint != "" && filepath.Clean(a.SourceDir) == cleanHint {
				sourceScopedMatches = appendUniquePoolTarget(sourceScopedMatches, a.BindingQualifiedName())
			}
		}
	}

	switch {
	case len(exactQualified) == 1:
		qualified = exactQualified[0]
	case len(exactQualified) > 1:
		return "", fmt.Errorf("ambiguous pool %q for %s: matches %s", pool, scope, strings.Join(exactQualified, ", "))
	case len(sourceScopedMatches) == 1:
		qualified = sourceScopedMatches[0]
	case len(sourceScopedMatches) > 1:
		return "", fmt.Errorf("ambiguous pool %q for %s: matches %s", pool, scope, strings.Join(sourceScopedMatches, ", "))
	case len(localBareMatches) == 1:
		qualified = localBareMatches[0]
	case len(localBareMatches) > 1:
		return "", fmt.Errorf("ambiguous pool %q for %s: matches %s", pool, scope, strings.Join(localBareMatches, ", "))
	case len(bareMatches) == 1:
		qualified = bareMatches[0]
	case len(bareMatches) > 1:
		return "", fmt.Errorf("ambiguous pool %q for %s: matches %s", pool, scope, strings.Join(bareMatches, ", "))
	}

	if rig == "" {
		return qualified, nil
	}
	return rig + "/" + qualified, nil
}

func appendUniquePoolTarget(values []string, want string) []string {
	for _, value := range values {
		if value == want {
			return values
		}
	}
	return append(values, want)
}
