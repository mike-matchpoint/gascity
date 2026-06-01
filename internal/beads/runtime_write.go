package beads

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// WriteClass names the runtime contract for a bead write. Hot runtime classes
// are bounded and report degraded/ambiguous outcomes instead of falling back to
// foreground Beads CLI behavior.
type WriteClass string

// Runtime write classes used by hot, foreground, and maintenance callers.
const (
	// WriteClassHotState covers low-risk runtime state writes that must never
	// block controller progress.
	WriteClassHotState                WriteClass = "hot-state"
	WriteClassReservation             WriteClass = "reservation"
	WriteClassCursorReservation       WriteClass = "cursor-reservation"
	WriteClassPostActionCritical      WriteClass = "post-action-critical"
	WriteClassAuditRepair             WriteClass = "audit-repair"
	WriteClassForegroundAuthoritative WriteClass = "foreground-authoritative"
	WriteClassMaintenance             WriteClass = "maintenance"
)

// Runtime writer queue and budget defaults.
const (
	// RuntimeWriteQueueLimit bounds queued writes per canonical store key.
	RuntimeWriteQueueLimit = 128

	// RuntimeWriteTraceRelativePath is the default per-scope trace inspected by
	// doctor/status when GC_BD_TRACE is not explicitly set.
	RuntimeWriteTraceRelativePath = ".gc/runtime/beads/runtime-write.trace"

	RuntimeWriteReservationBudget       = 10 * time.Second
	RuntimeWriteCursorReservationBudget = 10 * time.Second
	RuntimeWritePostActionBudget        = 30 * time.Second
	RuntimeWriteAuditRepairBudget       = 10 * time.Second
	RuntimeWriteHotStateBudget          = 10 * time.Second
	RuntimeWritePingBudget              = time.Second
	RuntimeWriteQueueWaitBudget         = 3 * time.Second

	// RuntimeWriteBreakerRecoveryAfter bounds how long a process-local writer
	// can stay open-loop before it admits one bounded recovery probe.
	RuntimeWriteBreakerRecoveryAfter = 30 * time.Second

	// RuntimeWriteDeadlinePriorityWindow avoids reordering close-deadline jobs
	// by insignificant clock skew while still letting short-budget reservation
	// writes run before long-budget post-action work.
	RuntimeWriteDeadlinePriorityWindow = time.Second

	// RuntimeWriteStartBudgetMax bounds how much of a runtime write's timeout
	// must remain when the queued job reaches the head of the writer. Starting a
	// bd subprocess with only a sliver of budget left creates ambiguous outcomes
	// even when the backing store is healthy.
	RuntimeWriteStartBudgetMax = 5 * time.Second

	// RuntimeWriteBreakerRecoverySuccesses is the number of consecutive
	// bounded recovery probes required before normal runtime writes resume.
	RuntimeWriteBreakerRecoverySuccesses = 3
)

// RuntimeWriteTracePath returns the default runtime-write trace path under
// root. The relative fallback is useful for callers that have not resolved a
// city/rig root yet but still need a stable path.
func RuntimeWriteTracePath(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return RuntimeWriteTraceRelativePath
	}
	return filepath.Join(root, RuntimeWriteTraceRelativePath)
}

// WriteOutcome names the caller-visible result class for degraded runtime
// writes.
type WriteOutcome string

// Runtime write outcome constants surfaced through DegradedWriteError.
const (
	// WriteOutcomeNotStarted means the runtime write was rejected before the
	// Beads command started.
	WriteOutcomeNotStarted       WriteOutcome = "not-started"
	WriteOutcomeNotFound         WriteOutcome = "not-found"
	WriteOutcomeAmbiguousTimeout WriteOutcome = "ambiguous-timeout"
	WriteOutcomeFailed           WriteOutcome = "failed"
	WriteOutcomePartial          WriteOutcome = "partial"
	WriteOutcomeUnsupported      WriteOutcome = "unsupported"
)

// WritePolicy declares how a runtime caller is allowed to mutate bead state.
type WritePolicy struct {
	Class          WriteClass
	Caller         string
	Timeout        time.Duration
	IdempotencyKey string
	AllowFallback  bool
	// DetachCallerDeadline lets the runtime writer apply the policy timeout
	// even when the caller is running inside a shorter scheduling tick. Context
	// values are preserved and explicit context.Canceled shutdown still
	// propagates, but context.DeadlineExceeded from the caller does not shrink
	// the write class budget.
	DetachCallerDeadline bool
}

// RuntimeWritePolicy returns a policy with the documented default budget and
// fallback behavior for class.
func RuntimeWritePolicy(class WriteClass, caller, idempotencyKey string) WritePolicy {
	p := WritePolicy{Class: class, Caller: caller, IdempotencyKey: idempotencyKey}
	switch class {
	case WriteClassHotState:
		p.Timeout = RuntimeWriteHotStateBudget
	case WriteClassReservation:
		p.Timeout = RuntimeWriteReservationBudget
	case WriteClassCursorReservation:
		p.Timeout = RuntimeWriteCursorReservationBudget
	case WriteClassPostActionCritical:
		p.Timeout = RuntimeWritePostActionBudget
	case WriteClassAuditRepair:
		p.Timeout = RuntimeWriteAuditRepairBudget
	case WriteClassMaintenance:
		p.Timeout = 10 * time.Second
	default:
		p.Class = WriteClassForegroundAuthoritative
		p.Timeout = bdCommandTimeout
		p.AllowFallback = true
	}
	return p
}

// DegradedWriteError reports a bounded runtime write degradation. Callers must
// treat ambiguous outcomes as unresolved until reconciliation confirms state.
type DegradedWriteError struct {
	Class          WriteClass
	Caller         string
	Operation      string
	Outcome        WriteOutcome
	IdempotencyKey string
	StoreKey       string
	Err            error
}

func (e *DegradedWriteError) Error() string {
	if e == nil {
		return "<nil>"
	}
	msg := "degraded runtime write"
	if e.Caller != "" {
		msg += " caller=" + e.Caller
	}
	if e.Operation != "" {
		msg += " op=" + e.Operation
	}
	if e.Class != "" {
		msg += " class=" + string(e.Class)
	}
	if e.Outcome != "" {
		msg += " outcome=" + string(e.Outcome)
	}
	if e.IdempotencyKey != "" {
		msg += " idempotency_key=" + e.IdempotencyKey
	}
	if e.StoreKey != "" {
		msg += " store_key=" + e.StoreKey
	}
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	return msg
}

func (e *DegradedWriteError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// IsDegradedWrite reports whether err is a runtime degraded-write error.
func IsDegradedWrite(err error) bool {
	var degraded *DegradedWriteError
	return errors.As(err, &degraded)
}

type runtimeCreator interface {
	RuntimeCreate(context.Context, Bead, WritePolicy) (Bead, error)
}

type runtimeUpdater interface {
	RuntimeUpdate(context.Context, string, UpdateOpts, WritePolicy) error
}

type runtimeCloser interface {
	RuntimeCloseAll(context.Context, []string, map[string]string, WritePolicy) (int, error)
}

type runtimePinger interface {
	RuntimePing(context.Context, WritePolicy) error
}

// RuntimeWriter is implemented by stores that support every bounded runtime
// write operation.
type RuntimeWriter interface {
	runtimeCreator
	runtimeUpdater
	runtimeCloser
	runtimePinger
}

// RuntimeWriteExecutor exposes the shared bounded runtime-write scheduler to
// store adapters outside this package. Adapters still own their concrete write
// operation; the executor owns queueing, collapse, priority, and breaker policy.
type RuntimeWriteExecutor struct {
	storeKey string
	manager  *runtimeWriteManager
}

// NewRuntimeWriteExecutor returns a shared bounded runtime-write executor for
// storeKey. Callers should pass the same canonical key used in degraded-write
// errors and status/doctor summaries.
func NewRuntimeWriteExecutor(storeKey string) *RuntimeWriteExecutor {
	storeKey = strings.TrimSpace(storeKey)
	if storeKey == "" {
		storeKey = "unknown"
	}
	return &RuntimeWriteExecutor{
		storeKey: storeKey,
		manager:  runtimeWriteManagerForKey(storeKey, RuntimeWriteQueueWaitBudget),
	}
}

// Do runs a runtime write under the executor's queue and breaker policy.
func (e *RuntimeWriteExecutor) Do(ctx context.Context, policy WritePolicy, op, objectKey string, run func(context.Context) (any, error)) (any, error) {
	if e == nil || e.manager == nil {
		return nil, degradedWrite(policy, "", op, WriteOutcomeUnsupported, errors.New("nil runtime write executor"))
	}
	return e.manager.do(ctx, policy, op, objectKey, run)
}

// DoWithPayload runs a collapsible runtime write under the executor's queue and
// breaker policy. merge is applied to queued, not-yet-started jobs that share a
// collapse key.
func (e *RuntimeWriteExecutor) DoWithPayload(ctx context.Context, policy WritePolicy, op, objectKey string, payload any, merge func(existing, incoming any) any, run func(context.Context, any) (any, error)) (any, error) {
	if e == nil || e.manager == nil {
		return nil, degradedWrite(policy, "", op, WriteOutcomeUnsupported, errors.New("nil runtime write executor"))
	}
	return e.manager.doWithPayload(ctx, policy, op, objectKey, payload, merge, run)
}

// Stats exposes bounded writer health for stores that compose an executor.
func (e *RuntimeWriteExecutor) Stats() RuntimeWriteManagerStats {
	if e == nil || e.manager == nil {
		return RuntimeWriteManagerStats{}
	}
	return e.manager.stats()
}

// StatsForClass exposes queue and breaker health for one write class.
func (e *RuntimeWriteExecutor) StatsForClass(class WriteClass) RuntimeWriteClassStats {
	if e == nil || e.manager == nil {
		return RuntimeWriteClassStats{Class: normalizeWritePolicy(WritePolicy{Class: class}).Class}
	}
	return e.manager.statsForClass(class)
}

// RuntimeCreate executes a create under policy.
func RuntimeCreate(ctx context.Context, store Store, b Bead, policy WritePolicy) (Bead, error) {
	if store == nil {
		return Bead{}, degradedWrite(policy, "", "create", WriteOutcomeUnsupported, errors.New("nil bead store"))
	}
	policy = normalizeWritePolicy(policy)
	if policy.AllowFallback {
		return store.Create(b)
	}
	if writer, ok := store.(runtimeCreator); ok {
		return writer.RuntimeCreate(ctx, b, policy)
	}
	return Bead{}, degradedWrite(policy, "", "create", WriteOutcomeUnsupported, ErrRuntimeWriteUnsupported)
}

func runtimeCreateDuplicateResult(store Store, b Bead, createErr error, policy WritePolicy, storeKey string) (Bead, bool, error) {
	if createErr == nil || store == nil || strings.TrimSpace(b.ID) == "" || !isBdDuplicateID(createErr) {
		return Bead{}, false, nil
	}
	policy = normalizeWritePolicy(policy)
	existing, getErr := store.Get(b.ID)
	if getErr != nil {
		return Bead{}, true, degradedWrite(policy, storeKey, "create", WriteOutcomeFailed, errors.Join(createErr, getErr))
	}
	if runtimeCreateDuplicateMatches(existing, b) {
		return existing, true, nil
	}
	return Bead{}, true, degradedWrite(policy, storeKey, "create", WriteOutcomeFailed,
		fmt.Errorf("duplicate bead id %q has mismatched reservation metadata", b.ID))
}

// RuntimeUpdate executes an update under policy.
func RuntimeUpdate(ctx context.Context, store Store, id string, opts UpdateOpts, policy WritePolicy) error {
	if store == nil {
		return degradedWrite(policy, "", "update", WriteOutcomeUnsupported, errors.New("nil bead store"))
	}
	policy = normalizeWritePolicy(policy)
	if policy.AllowFallback {
		return store.Update(id, opts)
	}
	if writer, ok := store.(runtimeUpdater); ok {
		return writer.RuntimeUpdate(ctx, id, opts, policy)
	}
	return degradedWrite(policy, "", "update", WriteOutcomeUnsupported, ErrRuntimeWriteUnsupported)
}

// RuntimeCloseAll executes a batch close under policy.
func RuntimeCloseAll(ctx context.Context, store Store, ids []string, metadata map[string]string, policy WritePolicy) (int, error) {
	if store == nil {
		return 0, degradedWrite(policy, "", "close-all", WriteOutcomeUnsupported, errors.New("nil bead store"))
	}
	policy = normalizeWritePolicy(policy)
	if policy.AllowFallback {
		return store.CloseAll(ids, metadata)
	}
	if writer, ok := store.(runtimeCloser); ok {
		return writer.RuntimeCloseAll(ctx, ids, metadata, policy)
	}
	return 0, degradedWrite(policy, "", "close-all", WriteOutcomeUnsupported, ErrRuntimeWriteUnsupported)
}

// RuntimePing probes write-health with a runtime budget.
func RuntimePing(ctx context.Context, store Store, policy WritePolicy) error {
	if store == nil {
		return degradedWrite(policy, "", "ping", WriteOutcomeUnsupported, errors.New("nil bead store"))
	}
	policy = normalizeWritePolicy(policy)
	if policy.AllowFallback {
		return store.Ping()
	}
	if writer, ok := store.(runtimePinger); ok {
		return writer.RuntimePing(ctx, policy)
	}
	return degradedWrite(policy, "", "ping", WriteOutcomeUnsupported, ErrRuntimeWriteUnsupported)
}

// ErrRuntimeWriteUnsupported is returned when a hot write reaches a store that
// does not implement the requested runtime write operation.
var ErrRuntimeWriteUnsupported = errors.New("runtime write unsupported")

func normalizeWritePolicy(policy WritePolicy) WritePolicy {
	if policy.Class == "" {
		policy = RuntimeWritePolicy(WriteClassAuditRepair, policy.Caller, policy.IdempotencyKey)
	}
	defaulted := RuntimeWritePolicy(policy.Class, policy.Caller, policy.IdempotencyKey)
	if policy.Timeout <= 0 {
		policy.Timeout = defaulted.Timeout
	}
	policy.AllowFallback = policy.AllowFallback || defaulted.AllowFallback
	return policy
}

func contextWithWritePolicy(ctx context.Context, policy WritePolicy) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	callerCtx, releaseCallerCtx := runtimeWriteCallerContext(ctx, policy)
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= policy.Timeout && !policy.DetachCallerDeadline {
			child, cancel := context.WithCancel(callerCtx)
			return child, func() {
				cancel()
				releaseCallerCtx()
			}
		}
	}
	if policy.Timeout <= 0 {
		child, cancel := context.WithCancel(callerCtx)
		return child, func() {
			cancel()
			releaseCallerCtx()
		}
	}
	child, cancel := context.WithTimeout(callerCtx, policy.Timeout)
	return child, func() {
		cancel()
		releaseCallerCtx()
	}
}

func runtimeWriteCallerContext(ctx context.Context, policy WritePolicy) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return context.Background(), func() {}
	}
	if !policy.DetachCallerDeadline {
		return ctx, func() {}
	}
	base := context.WithoutCancel(ctx)
	if ctx.Done() == nil {
		return base, func() {}
	}
	detached, cancel := context.WithCancel(base)
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) {
				cancel()
			}
		case <-done:
		}
	}()
	var once sync.Once
	return detached, func() {
		once.Do(func() {
			close(done)
			cancel()
		})
	}
}

func contextWithRuntimeWriteWaitPolicy(ctx context.Context, policy WritePolicy, waitBudget time.Duration) (context.Context, context.CancelFunc) {
	policy = normalizeWritePolicy(policy)
	if policy.Timeout > 0 && waitBudget > 0 {
		policy.Timeout += waitBudget
	}
	return contextWithWritePolicy(ctx, policy)
}

func ctxDone(ctx context.Context) <-chan struct{} {
	if ctx == nil {
		return nil
	}
	return ctx.Done()
}

func degradedWrite(policy WritePolicy, storeKey, op string, outcome WriteOutcome, err error) error {
	if err == nil {
		err = errors.New("runtime write degraded")
	}
	return &DegradedWriteError{
		Class:          policy.Class,
		Caller:         policy.Caller,
		Operation:      op,
		Outcome:        outcome,
		IdempotencyKey: strings.TrimSpace(policy.IdempotencyKey),
		StoreKey:       storeKey,
		Err:            err,
	}
}

// RuntimeWriteBreakerState names the per-store runtime write circuit breaker
// state.
type RuntimeWriteBreakerState string

// Runtime write breaker states exposed through RuntimeWriteManagerStats.
const (
	// RuntimeWriteBreakerClosed admits normal runtime writes.
	RuntimeWriteBreakerClosed   RuntimeWriteBreakerState = "closed"
	RuntimeWriteBreakerOpen     RuntimeWriteBreakerState = "open"
	RuntimeWriteBreakerHalfOpen RuntimeWriteBreakerState = "half-open"
)

// RuntimeWriteManagerStats exposes bounded writer health for tests, status,
// and doctor surfaces.
type RuntimeWriteManagerStats struct {
	StoreKey       string
	QueueDepth     int
	QueueLimit     int
	Active         int
	BreakerState   RuntimeWriteBreakerState
	Dropped        int64
	Collapsed      int64
	Timeouts       int64
	Failures       int64
	Completed      int64
	OldestQueueAge time.Duration
}

// RuntimeWriteClassStats exposes bounded writer health for one write class.
type RuntimeWriteClassStats struct {
	StoreKey       string
	Class          WriteClass
	QueueDepth     int
	QueueLimit     int
	Active         int
	BreakerState   RuntimeWriteBreakerState
	OldestQueueAge time.Duration
}

type runtimeWriteManager struct {
	storeKey   string
	waitBudget time.Duration

	mu            sync.Mutex
	cond          *sync.Cond
	queues        map[WriteClass][]*runtimeWriteJob
	queueDepth    int
	pending       map[string]*runtimeWriteJob
	active        int
	activeByClass map[WriteClass]int
	dropped       int64
	collapsed     int64
	timeouts      int64
	failures      int64
	completed     int64
	oldest        time.Time

	breakers map[WriteClass]*runtimeWriteBreaker
}

type runtimeWriteFailure struct {
	at      time.Time
	timeout bool
}

type runtimeWriteBreaker struct {
	state             RuntimeWriteBreakerState
	opened            time.Time
	recoveryProbe     bool
	recoverySuccesses int
	recent            []runtimeWriteFailure
}

type runtimeWriteJob struct {
	ctx         context.Context
	policy      WritePolicy
	operation   string
	objectKey   string
	collapseKey string
	recovery    bool
	enqueued    time.Time
	deadline    time.Time
	payload     any
	merge       func(existing, incoming any) any
	run         func(context.Context, any) (any, error)
	waiters     []chan runtimeWriteResult
	started     bool
}

type runtimeWriteResult struct {
	value any
	err   error
}

func newRuntimeWriteManager(storeKey string, waitBudget time.Duration) *runtimeWriteManager {
	m := &runtimeWriteManager{
		storeKey:      storeKey,
		waitBudget:    waitBudget,
		queues:        make(map[WriteClass][]*runtimeWriteJob),
		pending:       make(map[string]*runtimeWriteJob),
		activeByClass: make(map[WriteClass]int),
		breakers:      make(map[WriteClass]*runtimeWriteBreaker),
	}
	m.cond = sync.NewCond(&m.mu)
	go m.loop()
	return m
}

func (m *runtimeWriteManager) do(ctx context.Context, policy WritePolicy, op, objectKey string, run func(context.Context) (any, error)) (any, error) {
	return m.doWithPayload(ctx, policy, op, objectKey, nil, nil, func(runCtx context.Context, _ any) (any, error) {
		return run(runCtx)
	})
}

func (m *runtimeWriteManager) doWithPayload(ctx context.Context, policy WritePolicy, op, objectKey string, payload any, merge func(existing, incoming any) any, run func(context.Context, any) (any, error)) (any, error) {
	if m == nil {
		return nil, degradedWrite(policy, "", op, WriteOutcomeUnsupported, errors.New("nil runtime write manager"))
	}
	policy = normalizeWritePolicy(policy)
	if ctx == nil {
		ctx = context.Background()
	}
	waitCtx, cancel := contextWithRuntimeWriteWaitPolicy(ctx, policy, m.waitBudget)
	defer cancel()
	recovery, err := m.admit(policy, op)
	if err != nil {
		return nil, err
	}
	result := make(chan runtimeWriteResult, 1)
	collapseKey := runtimeWriteCollapseKey(policy, op, objectKey)
	job := &runtimeWriteJob{
		ctx:         ctx,
		policy:      policy,
		operation:   op,
		objectKey:   objectKey,
		collapseKey: collapseKey,
		recovery:    recovery,
		enqueued:    time.Now(),
		payload:     payload,
		merge:       merge,
		run:         run,
		waiters:     []chan runtimeWriteResult{result},
	}
	job.deadline, _ = waitCtx.Deadline()
	select {
	case <-waitCtx.Done():
		return nil, degradedWrite(policy, m.storeKey, op, WriteOutcomeNotStarted, waitCtx.Err())
	default:
	}

	m.mu.Lock()
	if waitCtx.Err() != nil {
		m.mu.Unlock()
		return nil, degradedWrite(policy, m.storeKey, op, WriteOutcomeNotStarted, waitCtx.Err())
	}
	if collapseKey != "" {
		if existing := m.pending[collapseKey]; existing != nil {
			switch {
			case existing.started:
				// A started write already captured its payload; make this job the
				// follow-up merge target so newer updates are not lost or fanned out.
				// The pending pointer is installed after capacity checks below.
			case existing.merge != nil:
				existing.payload = existing.merge(existing.payload, payload)
				existing.waiters = append(existing.waiters, result)
				m.collapsed++
				m.mu.Unlock()
				return m.awaitResult(waitCtx, policy, op, existing, result)
			default:
				existing.waiters = append(existing.waiters, result)
				m.collapsed++
				m.mu.Unlock()
				return m.awaitResult(waitCtx, policy, op, existing, result)
			}
		}
	}
	if !m.canEnqueueLocked(job) {
		m.dropped++
		m.mu.Unlock()
		return nil, degradedWrite(policy, m.storeKey, op, WriteOutcomeNotStarted, fmt.Errorf("runtime write queue full: %d", RuntimeWriteQueueLimit))
	}
	var evicted []*runtimeWriteJob
	if m.queueDepth >= RuntimeWriteQueueLimit {
		if evictedJob := m.evictMaintenanceForLocked(job); evictedJob != nil {
			evicted = append(evicted, evictedJob)
		}
	}
	if m.queueDepth >= RuntimeWriteQueueLimit {
		m.dropped++
		m.mu.Unlock()
		m.finishEvictedJobs(evicted)
		return nil, degradedWrite(policy, m.storeKey, op, WriteOutcomeNotStarted, fmt.Errorf("runtime write queue full: %d", RuntimeWriteQueueLimit))
	}
	if collapseKey != "" {
		m.pending[collapseKey] = job
	}
	m.enqueueLocked(job)
	m.mu.Unlock()
	m.finishEvictedJobs(evicted)
	return m.awaitResult(waitCtx, policy, op, job, result)
}

func (m *runtimeWriteManager) awaitResult(ctx context.Context, policy WritePolicy, op string, job *runtimeWriteJob, result <-chan runtimeWriteResult) (any, error) {
	select {
	case res := <-result:
		return res.value, res.err
	case <-ctx.Done():
		if m.cancelQueuedWaiter(job, result) {
			return nil, degradedWrite(policy, m.storeKey, op, WriteOutcomeNotStarted, ctx.Err())
		}
		if !m.jobStarted(job) {
			return nil, degradedWrite(policy, m.storeKey, op, WriteOutcomeNotStarted, ctx.Err())
		}
		return nil, degradedWrite(policy, m.storeKey, op, WriteOutcomeAmbiguousTimeout, ctx.Err())
	}
}

func (m *runtimeWriteManager) jobStarted(job *runtimeWriteJob) bool {
	if m == nil || job == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return job.started
}

func (m *runtimeWriteManager) cancelQueuedWaiter(job *runtimeWriteJob, waiter <-chan runtimeWriteResult) bool {
	if m == nil || job == nil || waiter == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if job.started {
		return false
	}
	removedWaiter := false
	waiters := job.waiters[:0]
	for _, candidate := range job.waiters {
		if !removedWaiter && (<-chan runtimeWriteResult)(candidate) == waiter {
			removedWaiter = true
			continue
		}
		waiters = append(waiters, candidate)
	}
	job.waiters = waiters
	if !removedWaiter || len(job.waiters) > 0 {
		return false
	}
	if !m.removeQueuedJobLocked(job) {
		return false
	}
	m.dropped++
	return true
}

func (m *runtimeWriteManager) admit(policy WritePolicy, op string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	breaker := m.breakerForClassLocked(policy.Class)
	switch breaker.state {
	case RuntimeWriteBreakerClosed:
		return false, nil
	case RuntimeWriteBreakerHalfOpen:
		if breaker.recoveryProbe {
			if policy.Class == WriteClassHotState || op == "ping" {
				return true, nil
			}
			return false, degradedWrite(policy, m.storeKey, op, WriteOutcomeNotStarted, errors.New("runtime write circuit breaker recovery probe in progress"))
		}
		breaker.recoveryProbe = true
		return true, nil
	default:
		switch {
		case op == "ping" && !breaker.recoveryProbe:
			breaker.state = RuntimeWriteBreakerHalfOpen
			breaker.recoveryProbe = true
			return true, nil
		case policy.Class == WriteClassHotState && !breaker.recoveryProbe:
			breaker.state = RuntimeWriteBreakerHalfOpen
			breaker.recoveryProbe = true
			return true, nil
		case !breaker.recoveryProbe && !breaker.opened.IsZero() && now.Sub(breaker.opened) >= RuntimeWriteBreakerRecoveryAfter:
			breaker.state = RuntimeWriteBreakerHalfOpen
			breaker.recoveryProbe = true
			return true, nil
		default:
			return false, degradedWrite(policy, m.storeKey, op, WriteOutcomeNotStarted, fmt.Errorf("runtime write circuit breaker open for class %s", policy.Class))
		}
	}
}

func (m *runtimeWriteManager) canEnqueueLocked(job *runtimeWriteJob) bool {
	if m == nil || job == nil {
		return false
	}
	limit := runtimeWriteClassQueueLimit(job.policy.Class)
	if limit <= 0 {
		return false
	}
	return len(m.queues[job.policy.Class]) < limit
}

func (m *runtimeWriteManager) enqueueLocked(job *runtimeWriteJob) {
	if m == nil || job == nil {
		return
	}
	class := job.policy.Class
	m.queues[class] = append(m.queues[class], job)
	m.queueDepth++
	if m.oldest.IsZero() || job.enqueued.Before(m.oldest) {
		m.oldest = job.enqueued
	}
	m.cond.Signal()
}

func (m *runtimeWriteManager) popNextLocked() *runtimeWriteJob {
	var best *runtimeWriteJob
	var bestClass WriteClass
	bestIndex := -1
	for class, queue := range m.queues {
		for i, job := range queue {
			if job == nil {
				continue
			}
			if best == nil || runtimeWriteJobLess(job, best) {
				best = job
				bestClass = class
				bestIndex = i
			}
		}
	}
	if best == nil || bestIndex < 0 {
		return nil
	}
	queue := m.queues[bestClass]
	copy(queue[bestIndex:], queue[bestIndex+1:])
	queue = queue[:len(queue)-1]
	if len(queue) == 0 {
		delete(m.queues, bestClass)
	} else {
		m.queues[bestClass] = queue
	}
	m.queueDepth--
	m.active++
	if m.activeByClass == nil {
		m.activeByClass = make(map[WriteClass]int)
	}
	m.activeByClass[best.policy.Class]++
	if best.collapseKey != "" && m.pending[best.collapseKey] == best {
		delete(m.pending, best.collapseKey)
	}
	m.recomputeOldestLocked()
	return best
}

func runtimeWriteJobLess(a, b *runtimeWriteJob) bool {
	if a == nil {
		return false
	}
	if b == nil {
		return true
	}
	if !a.deadline.IsZero() || !b.deadline.IsZero() {
		switch {
		case a.deadline.IsZero():
			return false
		case b.deadline.IsZero():
			return true
		case a.deadline.Before(b.deadline.Add(-RuntimeWriteDeadlinePriorityWindow)):
			return a.deadline.Before(b.deadline)
		case b.deadline.Before(a.deadline.Add(-RuntimeWriteDeadlinePriorityWindow)):
			return false
		}
	}
	aRank := runtimeWriteClassRank(a.policy.Class)
	bRank := runtimeWriteClassRank(b.policy.Class)
	if aRank != bRank {
		return aRank < bRank
	}
	if !a.enqueued.Equal(b.enqueued) {
		return a.enqueued.Before(b.enqueued)
	}
	return a.collapseKey < b.collapseKey
}

func (m *runtimeWriteManager) finishJobLocked(job *runtimeWriteJob) {
	if m == nil || job == nil {
		return
	}
	m.active--
	if m.activeByClass != nil {
		class := job.policy.Class
		if m.activeByClass[class] <= 1 {
			delete(m.activeByClass, class)
		} else {
			m.activeByClass[class]--
		}
	}
	if job.collapseKey != "" && m.pending[job.collapseKey] == job {
		delete(m.pending, job.collapseKey)
	}
	if m.queueDepth == 0 {
		m.recomputeOldestLocked()
	}
}

func (m *runtimeWriteManager) removeQueuedJobLocked(job *runtimeWriteJob) bool {
	if m == nil || job == nil {
		return false
	}
	queue := m.queues[job.policy.Class]
	for i, candidate := range queue {
		if candidate != job {
			continue
		}
		copy(queue[i:], queue[i+1:])
		queue = queue[:len(queue)-1]
		if len(queue) == 0 {
			delete(m.queues, job.policy.Class)
		} else {
			m.queues[job.policy.Class] = queue
		}
		m.queueDepth--
		if job.collapseKey != "" && m.pending[job.collapseKey] == job {
			delete(m.pending, job.collapseKey)
		}
		m.recomputeOldestLocked()
		return true
	}
	return false
}

func (m *runtimeWriteManager) evictMaintenanceForLocked(incoming *runtimeWriteJob) *runtimeWriteJob {
	if incoming == nil || runtimeWriteClassRank(incoming.policy.Class) >= runtimeWriteClassRank(WriteClassMaintenance) {
		return nil
	}
	queue := m.queues[WriteClassMaintenance]
	if len(queue) == 0 {
		return nil
	}
	job := queue[len(queue)-1]
	queue = queue[:len(queue)-1]
	if len(queue) == 0 {
		delete(m.queues, WriteClassMaintenance)
	} else {
		m.queues[WriteClassMaintenance] = queue
	}
	m.queueDepth--
	if job.collapseKey != "" && m.pending[job.collapseKey] == job {
		delete(m.pending, job.collapseKey)
	}
	m.recomputeOldestLocked()
	m.dropped++
	return job
}

func (m *runtimeWriteManager) finishEvictedJobs(jobs []*runtimeWriteJob) {
	for _, job := range jobs {
		if job == nil {
			continue
		}
		err := degradedWrite(job.policy, m.storeKey, job.operation, WriteOutcomeNotStarted, errors.New("runtime write queue preempted by higher-priority runtime write"))
		res := runtimeWriteResult{err: err}
		for _, waiter := range job.waiters {
			select {
			case waiter <- res:
			default:
			}
		}
	}
}

func (m *runtimeWriteManager) recomputeOldestLocked() {
	m.oldest = time.Time{}
	for _, queue := range m.queues {
		for _, job := range queue {
			if job == nil {
				continue
			}
			if m.oldest.IsZero() || job.enqueued.Before(m.oldest) {
				m.oldest = job.enqueued
			}
		}
	}
}

func (m *runtimeWriteManager) loop() {
	for {
		m.mu.Lock()
		for m.queueDepth == 0 {
			m.cond.Wait()
		}
		job := m.popNextLocked()
		if job == nil {
			m.cond.Wait()
			m.mu.Unlock()
			continue
		}
		payload := job.payload
		if err := runtimeWriteJobStartError(job, time.Now()); err != nil {
			m.finishJobLocked(job)
			m.dropped++
			waiters := append([]chan runtimeWriteResult(nil), job.waiters...)
			m.mu.Unlock()
			res := runtimeWriteResult{
				err: degradedWrite(job.policy, m.storeKey, job.operation, WriteOutcomeNotStarted, err),
			}
			for _, waiter := range waiters {
				select {
				case waiter <- res:
				default:
				}
			}
			continue
		}
		job.started = true
		m.mu.Unlock()
		runCtx, cancel := contextWithWritePolicy(job.ctx, job.policy)
		value, err := job.run(runCtx, payload)
		cancel()
		res := runtimeWriteResult{value: value, err: err}
		m.recordResult(job.policy.Class, job.recovery, err)
		m.mu.Lock()
		m.finishJobLocked(job)
		m.completed++
		waiters := append([]chan runtimeWriteResult(nil), job.waiters...)
		m.mu.Unlock()
		for _, waiter := range waiters {
			select {
			case waiter <- res:
			default:
			}
		}
	}
}

func runtimeWriteJobStartError(job *runtimeWriteJob, now time.Time) error {
	if job == nil {
		return errors.New("nil runtime write job")
	}
	if err := job.ctx.Err(); err != nil {
		return err
	}
	if job.deadline.IsZero() || job.policy.Timeout <= 0 {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	remaining := job.deadline.Sub(now)
	if remaining <= 0 {
		return context.DeadlineExceeded
	}
	minStartBudget := runtimeWriteMinStartBudget(job.policy)
	if minStartBudget > 0 && remaining < minStartBudget {
		return fmt.Errorf("runtime write start budget exhausted: %s remaining below %s minimum", remaining.Round(time.Millisecond), minStartBudget)
	}
	return nil
}

func runtimeWriteMinStartBudget(policy WritePolicy) time.Duration {
	if policy.Timeout <= 0 {
		return 0
	}
	minStartBudget := policy.Timeout / 2
	if minStartBudget > RuntimeWriteStartBudgetMax {
		return RuntimeWriteStartBudgetMax
	}
	return minStartBudget
}

func (m *runtimeWriteManager) recordResult(class WriteClass, recovery bool, err error) {
	if err == nil {
		m.mu.Lock()
		if recovery {
			breaker := m.breakerForClassLocked(class)
			breaker.recoverySuccesses++
			breaker.recoveryProbe = false
			breaker.recent = nil
			if breaker.recoverySuccesses >= RuntimeWriteBreakerRecoverySuccesses {
				breaker.state = RuntimeWriteBreakerClosed
				breaker.opened = time.Time{}
				breaker.recoverySuccesses = 0
			} else if breaker.state != RuntimeWriteBreakerClosed {
				breaker.state = RuntimeWriteBreakerOpen
				breaker.opened = time.Now()
			}
		}
		m.mu.Unlock()
		return
	}
	timeout := false
	var degraded *DegradedWriteError
	if errors.As(err, &degraded) && degraded.Outcome == WriteOutcomeAmbiguousTimeout {
		timeout = true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	breaker := m.breakerForClassLocked(class)
	if breaker.state != RuntimeWriteBreakerClosed && !recovery {
		return
	}
	if breaker.state == RuntimeWriteBreakerHalfOpen && recovery {
		breaker.state = RuntimeWriteBreakerOpen
		breaker.opened = time.Now()
		breaker.recoveryProbe = false
		breaker.recoverySuccesses = 0
	}
	if timeout {
		m.timeouts++
	} else {
		m.failures++
	}
	now := time.Now()
	windowStart := now.Add(-60 * time.Second)
	filtered := breaker.recent[:0]
	for _, failure := range breaker.recent {
		if failure.at.After(windowStart) {
			filtered = append(filtered, failure)
		}
	}
	filtered = append(filtered, runtimeWriteFailure{at: now, timeout: timeout})
	breaker.recent = filtered
	timeouts := 0
	failures := len(breaker.recent)
	for _, failure := range breaker.recent {
		if failure.timeout {
			timeouts++
		}
	}
	if timeouts >= 3 || failures >= 5 {
		breaker.state = RuntimeWriteBreakerOpen
		breaker.opened = now
		breaker.recoveryProbe = false
		breaker.recoverySuccesses = 0
	}
}

func (m *runtimeWriteManager) breakerForClassLocked(class WriteClass) *runtimeWriteBreaker {
	if m.breakers == nil {
		m.breakers = make(map[WriteClass]*runtimeWriteBreaker)
	}
	if class == "" {
		class = WriteClassAuditRepair
	}
	breaker := m.breakers[class]
	if breaker == nil {
		breaker = &runtimeWriteBreaker{state: RuntimeWriteBreakerClosed}
		m.breakers[class] = breaker
	}
	return breaker
}

func (m *runtimeWriteManager) breakerStateLocked() RuntimeWriteBreakerState {
	state := RuntimeWriteBreakerClosed
	for _, breaker := range m.breakers {
		if breaker == nil {
			continue
		}
		if breaker.state == RuntimeWriteBreakerOpen {
			return RuntimeWriteBreakerOpen
		}
		if breaker.state == RuntimeWriteBreakerHalfOpen {
			state = RuntimeWriteBreakerHalfOpen
		}
	}
	return state
}

func (m *runtimeWriteManager) stats() RuntimeWriteManagerStats {
	if m == nil {
		return RuntimeWriteManagerStats{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	oldestAge := time.Duration(0)
	if !m.oldest.IsZero() {
		oldestAge = time.Since(m.oldest)
	}
	return RuntimeWriteManagerStats{
		StoreKey:       m.storeKey,
		QueueDepth:     m.queueDepth,
		QueueLimit:     RuntimeWriteQueueLimit,
		Active:         m.active,
		BreakerState:   m.breakerStateLocked(),
		Dropped:        m.dropped,
		Collapsed:      m.collapsed,
		Timeouts:       m.timeouts,
		Failures:       m.failures,
		Completed:      m.completed,
		OldestQueueAge: oldestAge,
	}
}

func (m *runtimeWriteManager) statsForClass(class WriteClass) RuntimeWriteClassStats {
	if m == nil {
		return RuntimeWriteClassStats{Class: normalizeWritePolicy(WritePolicy{Class: class}).Class}
	}
	policy := normalizeWritePolicy(WritePolicy{Class: class})
	class = policy.Class
	m.mu.Lock()
	defer m.mu.Unlock()
	oldestAge := time.Duration(0)
	for _, job := range m.queues[class] {
		if job == nil || job.enqueued.IsZero() {
			continue
		}
		age := time.Since(job.enqueued)
		if oldestAge == 0 || age > oldestAge {
			oldestAge = age
		}
	}
	breaker := m.breakerForClassLocked(class)
	return RuntimeWriteClassStats{
		StoreKey:       m.storeKey,
		Class:          class,
		QueueDepth:     len(m.queues[class]),
		QueueLimit:     runtimeWriteClassQueueLimit(class),
		Active:         m.activeByClass[class],
		BreakerState:   breaker.state,
		OldestQueueAge: oldestAge,
	}
}

var runtimeWriteManagers = struct {
	mu       sync.Mutex
	managers map[string]*runtimeWriteManager
}{
	managers: make(map[string]*runtimeWriteManager),
}

func runtimeWriteManagerForKey(storeKey string, waitBudget time.Duration) *runtimeWriteManager {
	runtimeWriteManagers.mu.Lock()
	defer runtimeWriteManagers.mu.Unlock()
	if m := runtimeWriteManagers.managers[storeKey]; m != nil {
		if waitBudget > m.waitBudget {
			m.waitBudget = waitBudget
		}
		return m
	}
	m := newRuntimeWriteManager(storeKey, waitBudget)
	runtimeWriteManagers.managers[storeKey] = m
	return m
}

func runtimeWriteStoreKey(root, providerName, backendName, envFingerprint string) string {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}
	input := absRoot + "\x00" + providerName + "\x00" + backendName + "\x00" + envFingerprint
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])
}

// RuntimeStoreKey builds the canonical runtime-writer store key from the store
// root, provider, backend, and non-secret environment fingerprint.
func RuntimeStoreKey(root, providerName, backendName, envFingerprint string) string {
	return runtimeWriteStoreKey(root, providerName, backendName, envFingerprint)
}

func runtimeWriteCollapseKey(policy WritePolicy, op, objectKey string) string {
	idempotencyKey := strings.TrimSpace(policy.IdempotencyKey)
	if idempotencyKey == "" {
		return ""
	}
	return strings.Join([]string{
		string(policy.Class),
		strings.TrimSpace(op),
		strings.TrimSpace(objectKey),
		idempotencyKey,
	}, "\x00")
}

func runtimeWriteClassPriority() []WriteClass {
	return []WriteClass{
		WriteClassHotState,
		WriteClassPostActionCritical,
		WriteClassReservation,
		WriteClassCursorReservation,
		WriteClassAuditRepair,
		WriteClassForegroundAuthoritative,
		WriteClassMaintenance,
	}
}

func runtimeWriteClassRank(class WriteClass) int {
	for i, candidate := range runtimeWriteClassPriority() {
		if candidate == class {
			return i
		}
	}
	return len(runtimeWriteClassPriority())
}

func runtimeWriteClassQueueLimit(class WriteClass) int {
	switch class {
	case WriteClassMaintenance:
		return RuntimeWriteQueueLimit / 8
	case WriteClassAuditRepair:
		return RuntimeWriteQueueLimit / 2
	default:
		return RuntimeWriteQueueLimit
	}
}

func mergeRuntimeUpdatePayload(existing, incoming any) any {
	existingOpts, _ := existing.(UpdateOpts)
	incomingOpts, _ := incoming.(UpdateOpts)
	return mergeRuntimeUpdateOpts(existingOpts, incomingOpts)
}

// MergeRuntimeUpdatePayload is the shared coalescing merge function for queued
// runtime UpdateOpts payloads.
func MergeRuntimeUpdatePayload(existing, incoming any) any {
	return mergeRuntimeUpdatePayload(existing, incoming)
}

func mergeRuntimeUpdateOpts(existing, incoming UpdateOpts) UpdateOpts {
	out := cloneRuntimeUpdateOpts(existing)
	if incoming.Title != nil {
		out.Title = cloneStringPtr(incoming.Title)
	}
	if incoming.Status != nil {
		out.Status = cloneStringPtr(incoming.Status)
	}
	if incoming.Type != nil {
		out.Type = cloneStringPtr(incoming.Type)
	}
	if incoming.Priority != nil {
		out.Priority = cloneIntPtr(incoming.Priority)
	}
	if incoming.Description != nil {
		out.Description = cloneStringPtr(incoming.Description)
	}
	if incoming.ParentID != nil {
		out.ParentID = cloneStringPtr(incoming.ParentID)
	}
	if incoming.Assignee != nil {
		out.Assignee = cloneStringPtr(incoming.Assignee)
	}
	out.Labels = appendDedupStrings(out.Labels, incoming.Labels...)
	out.RemoveLabels = appendDedupStrings(out.RemoveLabels, incoming.RemoveLabels...)
	if len(incoming.Metadata) > 0 {
		if out.Metadata == nil {
			out.Metadata = make(map[string]string, len(incoming.Metadata))
		}
		for key, value := range incoming.Metadata {
			if key == "state" && runtimeSessionStateTerminal(out.Metadata[key]) && !runtimeSessionStateTerminal(value) {
				continue
			}
			if key == "close_reason" && strings.TrimSpace(out.Metadata[key]) != "" && strings.TrimSpace(value) == "" {
				continue
			}
			out.Metadata[key] = value
		}
	}
	return out
}

func cloneRuntimeUpdateOpts(in UpdateOpts) UpdateOpts {
	out := UpdateOpts{
		Title:        cloneStringPtr(in.Title),
		Status:       cloneStringPtr(in.Status),
		Type:         cloneStringPtr(in.Type),
		Priority:     cloneIntPtr(in.Priority),
		Description:  cloneStringPtr(in.Description),
		ParentID:     cloneStringPtr(in.ParentID),
		Assignee:     cloneStringPtr(in.Assignee),
		Labels:       append([]string(nil), in.Labels...),
		RemoveLabels: append([]string(nil), in.RemoveLabels...),
	}
	if len(in.Metadata) > 0 {
		out.Metadata = make(map[string]string, len(in.Metadata))
		for key, value := range in.Metadata {
			out.Metadata[key] = value
		}
	}
	return out
}

func cloneStringPtr(v *string) *string {
	if v == nil {
		return nil
	}
	cloned := *v
	return &cloned
}

func appendDedupStrings(dst []string, values ...string) []string {
	if len(values) == 0 {
		return dst
	}
	seen := make(map[string]struct{}, len(dst)+len(values))
	for _, value := range dst {
		seen[value] = struct{}{}
	}
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		dst = append(dst, value)
	}
	return dst
}

func runtimeSessionStateTerminal(state string) bool {
	switch strings.TrimSpace(state) {
	case "archived", "closed", "drained", "duplicate", "duplicate-repair", "failed-create", "gc_swept", "orphaned", "quarantined", "suspended":
		return true
	default:
		return false
	}
}
