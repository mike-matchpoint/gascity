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

	RuntimeWriteReservationBudget       = time.Second
	RuntimeWriteCursorReservationBudget = time.Second
	RuntimeWritePostActionBudget        = 2 * time.Second
	RuntimeWriteAuditRepairBudget       = time.Second
	RuntimeWriteHotStateBudget          = 2 * time.Second
	RuntimeWritePingBudget              = time.Second

	// RuntimeWriteBreakerRecoveryAfter bounds how long a process-local writer
	// can stay open-loop before it admits one bounded recovery probe.
	RuntimeWriteBreakerRecoveryAfter = 30 * time.Second
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

// RuntimeWriter is implemented by stores that support bounded runtime writes.
type RuntimeWriter interface {
	RuntimeCreate(context.Context, Bead, WritePolicy) (Bead, error)
	RuntimeUpdate(context.Context, string, UpdateOpts, WritePolicy) error
	RuntimeCloseAll(context.Context, []string, map[string]string, WritePolicy) (int, error)
	RuntimePing(context.Context, WritePolicy) error
}

// RuntimeCreate executes a create under policy. CachingStore is deliberately
// unwrapped so hot writes bypass foreground post-write readbacks.
func RuntimeCreate(ctx context.Context, store Store, b Bead, policy WritePolicy) (Bead, error) {
	if store == nil {
		return Bead{}, degradedWrite(policy, "", "create", WriteOutcomeUnsupported, errors.New("nil bead store"))
	}
	if cache, ok := store.(*CachingStore); ok {
		store = cache.Backing()
	}
	policy = normalizeWritePolicy(policy)
	if policy.AllowFallback {
		return store.Create(b)
	}
	if writer, ok := store.(RuntimeWriter); ok {
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

// RuntimeUpdate executes an update under policy, bypassing CachingStore
// foreground refreshes.
func RuntimeUpdate(ctx context.Context, store Store, id string, opts UpdateOpts, policy WritePolicy) error {
	if store == nil {
		return degradedWrite(policy, "", "update", WriteOutcomeUnsupported, errors.New("nil bead store"))
	}
	if cache, ok := store.(*CachingStore); ok {
		store = cache.Backing()
	}
	policy = normalizeWritePolicy(policy)
	if policy.AllowFallback {
		return store.Update(id, opts)
	}
	if writer, ok := store.(RuntimeWriter); ok {
		return writer.RuntimeUpdate(ctx, id, opts, policy)
	}
	return degradedWrite(policy, "", "update", WriteOutcomeUnsupported, ErrRuntimeWriteUnsupported)
}

// RuntimeCloseAll executes a batch close under policy, bypassing CachingStore
// foreground refreshes.
func RuntimeCloseAll(ctx context.Context, store Store, ids []string, metadata map[string]string, policy WritePolicy) (int, error) {
	if store == nil {
		return 0, degradedWrite(policy, "", "close-all", WriteOutcomeUnsupported, errors.New("nil bead store"))
	}
	if cache, ok := store.(*CachingStore); ok {
		store = cache.Backing()
	}
	policy = normalizeWritePolicy(policy)
	if policy.AllowFallback {
		return store.CloseAll(ids, metadata)
	}
	if writer, ok := store.(RuntimeWriter); ok {
		return writer.RuntimeCloseAll(ctx, ids, metadata, policy)
	}
	return 0, degradedWrite(policy, "", "close-all", WriteOutcomeUnsupported, ErrRuntimeWriteUnsupported)
}

// RuntimePing probes write-health with a runtime budget.
func RuntimePing(ctx context.Context, store Store, policy WritePolicy) error {
	if store == nil {
		return degradedWrite(policy, "", "ping", WriteOutcomeUnsupported, errors.New("nil bead store"))
	}
	if cache, ok := store.(*CachingStore); ok {
		store = cache.Backing()
	}
	policy = normalizeWritePolicy(policy)
	if policy.AllowFallback {
		return store.Ping()
	}
	if writer, ok := store.(RuntimeWriter); ok {
		return writer.RuntimePing(ctx, policy)
	}
	return degradedWrite(policy, "", "ping", WriteOutcomeUnsupported, ErrRuntimeWriteUnsupported)
}

// ErrRuntimeWriteUnsupported is returned when a hot write reaches a store that
// does not implement RuntimeWriter.
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
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= policy.Timeout {
			return context.WithCancel(ctx)
		}
	}
	if policy.Timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, policy.Timeout)
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

type runtimeWriteManager struct {
	storeKey string
	queue    chan *runtimeWriteJob
	hotQueue chan *runtimeWriteJob

	mu        sync.Mutex
	pending   map[string]*runtimeWriteJob
	active    int
	dropped   int64
	collapsed int64
	timeouts  int64
	failures  int64
	completed int64
	oldest    time.Time

	breakerState  RuntimeWriteBreakerState
	breakerOpened time.Time
	recoveryProbe bool
	recent        []runtimeWriteFailure
}

type runtimeWriteFailure struct {
	at      time.Time
	timeout bool
}

type runtimeWriteJob struct {
	ctx         context.Context
	policy      WritePolicy
	operation   string
	objectKey   string
	collapseKey string
	enqueued    time.Time
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

func newRuntimeWriteManager(storeKey string) *runtimeWriteManager {
	m := &runtimeWriteManager{
		storeKey:     storeKey,
		queue:        make(chan *runtimeWriteJob, RuntimeWriteQueueLimit),
		hotQueue:     make(chan *runtimeWriteJob, RuntimeWriteQueueLimit),
		pending:      make(map[string]*runtimeWriteJob),
		breakerState: RuntimeWriteBreakerClosed,
	}
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
	writeCtx, cancel := contextWithWritePolicy(ctx, policy)
	defer cancel()
	if err := m.admit(policy, op); err != nil {
		return nil, err
	}
	result := make(chan runtimeWriteResult, 1)
	collapseKey := runtimeWriteCollapseKey(policy, op, objectKey)
	job := &runtimeWriteJob{
		ctx:         writeCtx,
		policy:      policy,
		operation:   op,
		objectKey:   objectKey,
		collapseKey: collapseKey,
		enqueued:    time.Now(),
		payload:     payload,
		merge:       merge,
		run:         run,
		waiters:     []chan runtimeWriteResult{result},
	}
	if collapseKey != "" {
		m.mu.Lock()
		tracked := false
		if existing := m.pending[collapseKey]; existing != nil {
			switch {
			case existing.started:
				// A started write already captured its payload; make this job the
				// follow-up merge target so newer updates are not lost or fanned out.
				m.pending[collapseKey] = job
				tracked = true
			case existing.merge != nil:
				existing.payload = existing.merge(existing.payload, payload)
				existing.waiters = append(existing.waiters, result)
				m.collapsed++
				m.mu.Unlock()
				return m.awaitResult(writeCtx, policy, op, result)
			default:
				existing.waiters = append(existing.waiters, result)
				m.collapsed++
				m.mu.Unlock()
				return m.awaitResult(writeCtx, policy, op, result)
			}
		}
		if collapseKey != "" && !tracked {
			m.pending[collapseKey] = job
		}
		m.mu.Unlock()
	}
	queue := m.queue
	if runtimeWriteUsesHotQueue(policy, op) {
		queue = m.hotQueue
	}
	select {
	case queue <- job:
		m.noteEnqueued(job.enqueued)
	case <-writeCtx.Done():
		m.removePending(collapseKey, job)
		return nil, degradedWrite(policy, m.storeKey, op, WriteOutcomeNotStarted, writeCtx.Err())
	default:
		m.mu.Lock()
		m.dropped++
		m.mu.Unlock()
		m.removePending(collapseKey, job)
		return nil, degradedWrite(policy, m.storeKey, op, WriteOutcomeNotStarted, fmt.Errorf("runtime write queue full: %d", RuntimeWriteQueueLimit))
	}
	return m.awaitResult(writeCtx, policy, op, result)
}

func (m *runtimeWriteManager) awaitResult(ctx context.Context, policy WritePolicy, op string, result <-chan runtimeWriteResult) (any, error) {
	select {
	case res := <-result:
		return res.value, res.err
	case <-ctx.Done():
		return nil, degradedWrite(policy, m.storeKey, op, WriteOutcomeAmbiguousTimeout, ctx.Err())
	}
}

func (m *runtimeWriteManager) admit(policy WritePolicy, op string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	switch m.breakerState {
	case RuntimeWriteBreakerClosed:
		return nil
	case RuntimeWriteBreakerHalfOpen:
		if policy.Class == WriteClassHotState || op == "ping" {
			return nil
		}
		return degradedWrite(policy, m.storeKey, op, WriteOutcomeNotStarted, errors.New("runtime write circuit breaker recovery probe in progress"))
	default:
		switch {
		case policy.Class == WriteClassHotState:
			return nil
		case op == "ping" && !m.recoveryProbe:
			m.breakerState = RuntimeWriteBreakerHalfOpen
			m.recoveryProbe = true
			return nil
		case !m.recoveryProbe && !m.breakerOpened.IsZero() && now.Sub(m.breakerOpened) >= RuntimeWriteBreakerRecoveryAfter:
			m.breakerState = RuntimeWriteBreakerHalfOpen
			m.recoveryProbe = true
			return nil
		default:
			return degradedWrite(policy, m.storeKey, op, WriteOutcomeNotStarted, errors.New("runtime write circuit breaker open"))
		}
	}
}

func (m *runtimeWriteManager) removePending(collapseKey string, job *runtimeWriteJob) {
	if collapseKey == "" || job == nil {
		return
	}
	m.mu.Lock()
	if m.pending[collapseKey] == job {
		delete(m.pending, collapseKey)
	}
	m.mu.Unlock()
}

func (m *runtimeWriteManager) noteEnqueued(enqueued time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.oldest.IsZero() || enqueued.Before(m.oldest) {
		m.oldest = enqueued
	}
}

func (m *runtimeWriteManager) loop() {
	for {
		if hot := m.nextHotJob(); hot != nil {
			m.runJob(hot)
			continue
		}
		var job *runtimeWriteJob
		select {
		case job = <-m.hotQueue:
		case job = <-m.queue:
		}
		m.runJob(job)
	}
}

func (m *runtimeWriteManager) nextHotJob() *runtimeWriteJob {
	select {
	case job := <-m.hotQueue:
		return job
	default:
		return nil
	}
}

func (m *runtimeWriteManager) runJob(job *runtimeWriteJob) {
	if job == nil {
		return
	}
	m.mu.Lock()
	m.active++
	m.oldest = time.Now()
	job.started = true
	payload := job.payload
	m.mu.Unlock()
	value, err := job.run(job.ctx, payload)
	res := runtimeWriteResult{value: value, err: err}
	m.recordResult(err)
	m.mu.Lock()
	m.active--
	m.completed++
	if len(m.queue) == 0 && len(m.hotQueue) == 0 {
		m.oldest = time.Time{}
	}
	if job.collapseKey != "" && m.pending[job.collapseKey] == job {
		delete(m.pending, job.collapseKey)
	}
	waiters := append([]chan runtimeWriteResult(nil), job.waiters...)
	m.mu.Unlock()
	for _, waiter := range waiters {
		select {
		case waiter <- res:
		default:
		}
	}
}

func (m *runtimeWriteManager) recordResult(err error) {
	if err == nil {
		m.mu.Lock()
		if m.breakerState != RuntimeWriteBreakerClosed {
			m.breakerState = RuntimeWriteBreakerClosed
			m.breakerOpened = time.Time{}
			m.recoveryProbe = false
			m.recent = nil
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
	if m.breakerState == RuntimeWriteBreakerHalfOpen {
		m.breakerState = RuntimeWriteBreakerOpen
		m.breakerOpened = time.Now()
		m.recoveryProbe = false
	}
	if timeout {
		m.timeouts++
	} else {
		m.failures++
	}
	now := time.Now()
	windowStart := now.Add(-60 * time.Second)
	filtered := m.recent[:0]
	for _, failure := range m.recent {
		if failure.at.After(windowStart) {
			filtered = append(filtered, failure)
		}
	}
	filtered = append(filtered, runtimeWriteFailure{at: now, timeout: timeout})
	m.recent = filtered
	timeouts := 0
	failures := len(m.recent)
	for _, failure := range m.recent {
		if failure.timeout {
			timeouts++
		}
	}
	if timeouts >= 3 || failures >= 5 {
		m.breakerState = RuntimeWriteBreakerOpen
		m.breakerOpened = now
		m.recoveryProbe = false
	}
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
		QueueDepth:     len(m.queue) + len(m.hotQueue),
		QueueLimit:     RuntimeWriteQueueLimit,
		Active:         m.active,
		BreakerState:   m.breakerState,
		Dropped:        m.dropped,
		Collapsed:      m.collapsed,
		Timeouts:       m.timeouts,
		Failures:       m.failures,
		Completed:      m.completed,
		OldestQueueAge: oldestAge,
	}
}

var runtimeWriteManagers = struct {
	mu       sync.Mutex
	managers map[string]*runtimeWriteManager
}{
	managers: make(map[string]*runtimeWriteManager),
}

func runtimeWriteManagerForKey(storeKey string) *runtimeWriteManager {
	runtimeWriteManagers.mu.Lock()
	defer runtimeWriteManagers.mu.Unlock()
	if m := runtimeWriteManagers.managers[storeKey]; m != nil {
		return m
	}
	m := newRuntimeWriteManager(storeKey)
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

func runtimeWriteUsesHotQueue(policy WritePolicy, op string) bool {
	return policy.Class == WriteClassHotState || strings.TrimSpace(op) == "ping"
}

func mergeRuntimeUpdatePayload(existing, incoming any) any {
	existingOpts, _ := existing.(UpdateOpts)
	incomingOpts, _ := incoming.(UpdateOpts)
	return mergeRuntimeUpdateOpts(existingOpts, incomingOpts)
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
