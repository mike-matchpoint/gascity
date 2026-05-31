package beads

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/fsys"
)

func TestRuntimeWritePolicyDefaults(t *testing.T) {
	tests := []struct {
		class WriteClass
		want  time.Duration
	}{
		{WriteClassReservation, time.Second},
		{WriteClassCursorReservation, time.Second},
		{WriteClassPostActionCritical, 2 * time.Second},
		{WriteClassAuditRepair, time.Second},
		{WriteClassHotState, 5 * time.Second},
	}
	for _, tt := range tests {
		t.Run(string(tt.class), func(t *testing.T) {
			got := RuntimeWritePolicy(tt.class, "test", "key")
			if got.Timeout != tt.want {
				t.Fatalf("Timeout = %s, want %s", got.Timeout, tt.want)
			}
			if got.AllowFallback {
				t.Fatalf("AllowFallback = true, want false")
			}
		})
	}
}

func TestBdStoreRuntimeCreateTimeoutReturnsDegradedWithinBudget(t *testing.T) {
	var runnerCalls atomic.Int64
	store := NewBdStore(t.TempDir(), func(string, string, ...string) ([]byte, error) {
		t.Fatal("foreground runner must not be used by RuntimeCreate")
		return nil, nil
	}).WithContextRunner(func(ctx context.Context, _ string, name string, args ...string) ([]byte, error) {
		runnerCalls.Add(1)
		if name != "bd" || len(args) == 0 || args[0] != "create" {
			t.Fatalf("command = %s %v, want bd create", name, args)
		}
		<-ctx.Done()
		return nil, ctx.Err()
	})

	policy := RuntimeWritePolicy(WriteClassReservation, "test.reservation", "reservation-1")
	policy.Timeout = 20 * time.Millisecond
	start := time.Now()
	_, err := store.RuntimeCreate(context.Background(), Bead{
		ID:    "gc-order-test",
		Title: "tracking",
		Metadata: map[string]string{
			"gc.order.reservation_hash": "abc",
		},
	}, policy)
	elapsed := time.Since(start)
	if !IsDegradedWrite(err) {
		t.Fatalf("err = %v, want degraded write", err)
	}
	var degraded *DegradedWriteError
	if !errors.As(err, &degraded) || degraded.Outcome != WriteOutcomeAmbiguousTimeout {
		t.Fatalf("degraded = %#v, want ambiguous timeout", degraded)
	}
	if elapsed > 250*time.Millisecond {
		t.Fatalf("elapsed = %s, want bounded runtime write", elapsed)
	}
	if runnerCalls.Load() != 1 {
		t.Fatalf("runner calls = %d, want 1", runnerCalls.Load())
	}
	waitForRuntimeWriteManagerIdle(t, store, time.Second)
}

func TestRuntimeWriteBypassesCachingStoreRefresh(t *testing.T) {
	backing := &runtimeWriteCountingStore{Store: NewMemStore()}
	cache := NewCachingStoreForTest(backing, nil)
	policy := RuntimeWritePolicy(WriteClassReservation, "test.cache-bypass", "key")

	created, err := RuntimeCreate(context.Background(), cache, Bead{ID: "gc-fixed", Title: "fixed"}, policy)
	if err != nil {
		t.Fatalf("RuntimeCreate: %v", err)
	}
	if created.ID != "gc-fixed" {
		t.Fatalf("created ID = %q, want caller-provided ID", created.ID)
	}
	if backing.runtimeCreateCalls != 1 {
		t.Fatalf("runtime create calls = %d, want 1", backing.runtimeCreateCalls)
	}
	if backing.getCalls != 0 {
		t.Fatalf("backing Get calls = %d, want 0 runtime writes must bypass CachingStore refresh", backing.getCalls)
	}
	if err := RuntimeUpdate(context.Background(), cache, "gc-fixed", UpdateOpts{Metadata: map[string]string{"seen": "true"}}, policy); err != nil {
		t.Fatalf("RuntimeUpdate: %v", err)
	}
	if _, err := RuntimeCloseAll(context.Background(), cache, []string{"gc-fixed"}, map[string]string{"close_reason": "done"}, policy); err != nil {
		t.Fatalf("RuntimeCloseAll: %v", err)
	}
	if backing.runtimeUpdateCalls != 1 {
		t.Fatalf("runtime update calls = %d, want 1", backing.runtimeUpdateCalls)
	}
	if backing.runtimeCloseAllCalls != 1 {
		t.Fatalf("runtime close calls = %d, want 1", backing.runtimeCloseAllCalls)
	}
	if backing.getCalls != 0 {
		t.Fatalf("backing Get calls = %d, want 0 runtime writes must bypass CachingStore readbacks", backing.getCalls)
	}
}

func TestFileStoreRuntimeWritesPreservePersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "beads.json")
	store, err := OpenFileStore(fsys.OSFS{}, path)
	if err != nil {
		t.Fatalf("OpenFileStore: %v", err)
	}
	policy := RuntimeWritePolicy(WriteClassReservation, "test.file-runtime", "file-runtime")

	created, err := RuntimeCreate(context.Background(), store, Bead{ID: "gc-file-runtime", Title: "file runtime"}, policy)
	if err != nil {
		t.Fatalf("RuntimeCreate: %v", err)
	}
	if created.ID != "gc-file-runtime" {
		t.Fatalf("created ID = %q, want caller-provided ID", created.ID)
	}
	if err := RuntimeUpdate(context.Background(), store, created.ID, UpdateOpts{Labels: []string{"runtime-file"}}, policy); err != nil {
		t.Fatalf("RuntimeUpdate: %v", err)
	}
	if _, err := RuntimeCloseAll(context.Background(), store, []string{created.ID}, map[string]string{"close_reason": "done"}, policy); err != nil {
		t.Fatalf("RuntimeCloseAll: %v", err)
	}

	reopened, err := OpenFileStore(fsys.OSFS{}, path)
	if err != nil {
		t.Fatalf("reopen FileStore: %v", err)
	}
	got, err := reopened.Get(created.ID)
	if err != nil {
		t.Fatalf("Get(%s): %v", created.ID, err)
	}
	if got.Status != "closed" {
		t.Fatalf("status = %q, want closed", got.Status)
	}
	if !slices.Contains(got.Labels, "runtime-file") {
		t.Fatalf("labels = %v, want runtime-file", got.Labels)
	}
	if got.Metadata["close_reason"] != "done" {
		t.Fatalf("close_reason = %q, want done", got.Metadata["close_reason"])
	}
}

func TestFileStoreRuntimeCreateTimeoutReturnsDegradedWithinBudget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "beads.json")
	store, err := OpenFileStore(fsys.OSFS{}, path)
	if err != nil {
		t.Fatalf("OpenFileStore: %v", err)
	}
	locker := &blockingRuntimeLocker{
		locked:   make(chan struct{}),
		release:  make(chan struct{}),
		unlocked: make(chan struct{}),
	}
	store.SetLocker(locker)

	policy := RuntimeWritePolicy(WriteClassReservation, "test.file-runtime-timeout", "file-timeout")
	policy.Timeout = 20 * time.Millisecond
	start := time.Now()
	_, err = RuntimeCreate(context.Background(), store, Bead{ID: "gc-file-runtime-timeout", Title: "timeout"}, policy)
	elapsed := time.Since(start)
	if !IsDegradedWrite(err) {
		t.Fatalf("RuntimeCreate error = %v, want degraded write", err)
	}
	var degraded *DegradedWriteError
	if !errors.As(err, &degraded) || degraded.Outcome != WriteOutcomeAmbiguousTimeout {
		t.Fatalf("degraded = %#v, want ambiguous timeout", degraded)
	}
	if elapsed > 250*time.Millisecond {
		t.Fatalf("elapsed = %s, want bounded file-store runtime write", elapsed)
	}
	select {
	case <-locker.locked:
	case <-time.After(time.Second):
		t.Fatal("file-store runtime write did not reach locker")
	}
	close(locker.release)
	select {
	case <-locker.unlocked:
	case <-time.After(time.Second):
		t.Fatal("file-store runtime write did not release locker")
	}
}

type blockingRuntimeLocker struct {
	locked     chan struct{}
	release    chan struct{}
	unlocked   chan struct{}
	once       sync.Once
	unlockOnce sync.Once
}

func (l *blockingRuntimeLocker) Lock() error {
	l.once.Do(func() { close(l.locked) })
	<-l.release
	return nil
}

func (l *blockingRuntimeLocker) Unlock() error {
	l.unlockOnce.Do(func() { close(l.unlocked) })
	return nil
}

func TestInProcessRuntimeCreateDuplicateMatchingReservationSucceeds(t *testing.T) {
	fileStore, err := OpenFileStore(fsys.OSFS{}, filepath.Join(t.TempDir(), "beads.json"))
	if err != nil {
		t.Fatalf("OpenFileStore: %v", err)
	}
	tests := []struct {
		name  string
		store Store
	}{
		{name: "memory", store: NewMemStore()},
		{name: "file", store: fileStore},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := RuntimeWritePolicy(WriteClassReservation, "test.duplicate", "reservation-key")
			seed := Bead{
				ID:    "gc-order-fixed-" + tt.name,
				Title: "fixed",
				Metadata: map[string]string{
					"gc.order.reservation_hash": "hash-1",
				},
			}
			first, err := RuntimeCreate(context.Background(), tt.store, seed, policy)
			if err != nil {
				t.Fatalf("first RuntimeCreate: %v", err)
			}
			duplicate := seed
			duplicate.Title = "ignored duplicate title"
			second, err := RuntimeCreate(context.Background(), tt.store, duplicate, policy)
			if err != nil {
				t.Fatalf("duplicate RuntimeCreate: %v", err)
			}
			if second.ID != first.ID || second.Title != first.Title {
				t.Fatalf("duplicate returned %#v, want existing %#v", second, first)
			}
		})
	}
}

func TestInProcessRuntimeCreateDuplicateMismatchedReservationFails(t *testing.T) {
	store := NewMemStore()
	policy := RuntimeWritePolicy(WriteClassReservation, "test.duplicate", "reservation-key")
	if _, err := RuntimeCreate(context.Background(), store, Bead{
		ID:    "gc-order-fixed",
		Title: "fixed",
		Metadata: map[string]string{
			"gc.order.reservation_hash": "hash-1",
		},
	}, policy); err != nil {
		t.Fatalf("first RuntimeCreate: %v", err)
	}
	_, err := RuntimeCreate(context.Background(), store, Bead{
		ID:    "gc-order-fixed",
		Title: "fixed",
		Metadata: map[string]string{
			"gc.order.reservation_hash": "hash-2",
		},
	}, policy)
	if !IsDegradedWrite(err) {
		t.Fatalf("duplicate RuntimeCreate error = %v, want degraded write", err)
	}
}

type runtimeWriteCountingStore struct {
	Store
	runtimeCreateCalls   int
	runtimeUpdateCalls   int
	runtimeCloseAllCalls int
	getCalls             int
}

func (s *runtimeWriteCountingStore) RuntimeCreate(_ context.Context, b Bead, _ WritePolicy) (Bead, error) {
	s.runtimeCreateCalls++
	return s.Create(b)
}

func (s *runtimeWriteCountingStore) RuntimeUpdate(_ context.Context, id string, opts UpdateOpts, _ WritePolicy) error {
	s.runtimeUpdateCalls++
	return s.Update(id, opts)
}

func (s *runtimeWriteCountingStore) RuntimeCloseAll(_ context.Context, ids []string, metadata map[string]string, _ WritePolicy) (int, error) {
	s.runtimeCloseAllCalls++
	return s.CloseAll(ids, metadata)
}

func (s *runtimeWriteCountingStore) RuntimePing(_ context.Context, _ WritePolicy) error {
	return s.Ping()
}

func (s *runtimeWriteCountingStore) Get(id string) (Bead, error) {
	s.getCalls++
	return s.Store.Get(id)
}

func TestMemStoreRuntimeCreatePreservesCallerProvidedID(t *testing.T) {
	store := NewMemStore()
	policy := RuntimeWritePolicy(WriteClassReservation, "test.mem", "fixed")
	created, err := store.RuntimeCreate(context.Background(), Bead{ID: "gc-order-fixed", Title: "fixed"}, policy)
	if err != nil {
		t.Fatalf("RuntimeCreate: %v", err)
	}
	if created.ID != "gc-order-fixed" {
		t.Fatalf("ID = %q, want caller-provided ID", created.ID)
	}
}

func TestBdStoreRuntimeWriterRegistryReusesManagerForSameStoreKey(t *testing.T) {
	dir := t.TempDir()
	first := NewBdStore(dir, nil)
	second := NewBdStore(dir, nil)
	if first.RuntimeWriteStoreKey() == "" {
		t.Fatal("empty store key")
	}
	if first.RuntimeWriteStoreKey() != second.RuntimeWriteStoreKey() {
		t.Fatalf("store keys differ: %q vs %q", first.RuntimeWriteStoreKey(), second.RuntimeWriteStoreKey())
	}
	if first.runtimeWriteManager() != second.runtimeWriteManager() {
		t.Fatal("same canonical store key did not reuse one runtime write manager")
	}
}

func TestBdStoreRuntimeWriterSerializesPerStore(t *testing.T) {
	var active atomic.Int64
	var maxActive atomic.Int64
	store := NewBdStore(t.TempDir(), nil).WithContextRunner(func(ctx context.Context, _ string, _ string, _ ...string) ([]byte, error) {
		now := active.Add(1)
		for {
			old := maxActive.Load()
			if now <= old || maxActive.CompareAndSwap(old, now) {
				break
			}
		}
		defer active.Add(-1)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(30 * time.Millisecond):
			return []byte(`[]`), nil
		}
	})
	policy := RuntimeWritePolicy(WriteClassAuditRepair, "test.serialize", "same")
	policy.Timeout = time.Second

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := store.RuntimeUpdate(context.Background(), "gc-1", UpdateOpts{Metadata: map[string]string{"i": string(rune('0' + i))}}, policy); err != nil {
				t.Errorf("RuntimeUpdate(%d): %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	if maxActive.Load() != 1 {
		t.Fatalf("max active bd commands = %d, want 1", maxActive.Load())
	}
}

func TestBdStoreRuntimeWriterPrioritizesHotState(t *testing.T) {
	holdStarted := make(chan struct{})
	releaseHold := make(chan struct{})
	var holdOnce sync.Once
	var mu sync.Mutex
	var order []string
	store := NewBdStore(t.TempDir(), nil).WithContextRunner(func(ctx context.Context, _ string, _ string, args ...string) ([]byte, error) {
		if len(args) < 3 || args[0] != "update" {
			return []byte(`[]`), nil
		}
		id := args[2]
		if id == "gc-hold" {
			holdOnce.Do(func() { close(holdStarted) })
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-releaseHold:
				return []byte(`[]`), nil
			}
		}
		mu.Lock()
		order = append(order, id)
		mu.Unlock()
		return []byte(`[]`), nil
	})

	holdPolicy := RuntimeWritePolicy(WriteClassAuditRepair, "test.priority-hold", "hold")
	holdPolicy.Timeout = 5 * time.Second
	errs := make(chan error, 3)
	go func() {
		errs <- store.RuntimeUpdate(context.Background(), "gc-hold", UpdateOpts{Metadata: map[string]string{"hold": "true"}}, holdPolicy)
	}()
	<-holdStarted

	normalPolicy := RuntimeWritePolicy(WriteClassAuditRepair, "test.priority-normal", "normal")
	normalPolicy.Timeout = 5 * time.Second
	go func() {
		errs <- store.RuntimeUpdate(context.Background(), "gc-normal", UpdateOpts{Metadata: map[string]string{"normal": "true"}}, normalPolicy)
	}()
	waitForRuntimeWriteQueueDepth(t, store, 1)

	hotPolicy := RuntimeWritePolicy(WriteClassHotState, "test.priority-hot", "hot")
	hotPolicy.Timeout = 5 * time.Second
	go func() {
		errs <- store.RuntimeUpdate(context.Background(), "gc-hot", UpdateOpts{Metadata: map[string]string{"hot": "true"}}, hotPolicy)
	}()
	waitForRuntimeWriteQueueDepth(t, store, 2)
	close(releaseHold)

	for i := 0; i < 3; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("RuntimeUpdate error: %v", err)
		}
	}
	mu.Lock()
	got := append([]string(nil), order...)
	mu.Unlock()
	if len(got) < 2 || got[0] != "gc-hot" || got[1] != "gc-normal" {
		t.Fatalf("runtime write order = %v, want hot-state before normal audit write", got)
	}
}

func TestBdStoreRuntimeWriteExecutionBudgetStartsWhenDequeued(t *testing.T) {
	holdStarted := make(chan struct{})
	releaseHold := make(chan struct{})
	var holdOnce sync.Once
	store := NewBdStore(t.TempDir(), nil).WithContextRunner(func(ctx context.Context, _ string, _ string, args ...string) ([]byte, error) {
		if len(args) < 3 || args[0] != "update" {
			return []byte(`[]`), nil
		}
		switch args[2] {
		case "gc-hold":
			holdOnce.Do(func() { close(holdStarted) })
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-releaseHold:
				return []byte(`[]`), nil
			}
		case "gc-hot":
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(20 * time.Millisecond):
				return []byte(`[]`), nil
			}
		default:
			return []byte(`[]`), nil
		}
	})

	holdPolicy := RuntimeWritePolicy(WriteClassAuditRepair, "test.dequeue-hold", "hold")
	holdPolicy.Timeout = 500 * time.Millisecond
	errs := make(chan error, 2)
	go func() {
		errs <- store.RuntimeUpdate(context.Background(), "gc-hold", UpdateOpts{Metadata: map[string]string{"hold": "true"}}, holdPolicy)
	}()
	<-holdStarted

	hotPolicy := RuntimeWritePolicy(WriteClassHotState, "test.dequeue-hot", "hot")
	hotPolicy.Timeout = 50 * time.Millisecond
	go func() {
		errs <- store.RuntimeUpdate(context.Background(), "gc-hot", UpdateOpts{Metadata: map[string]string{"hot": "true"}}, hotPolicy)
	}()
	waitForRuntimeWriteQueueDepth(t, store, 1)
	time.Sleep(hotPolicy.Timeout + 25*time.Millisecond)
	close(releaseHold)

	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("RuntimeUpdate error: %v", err)
		}
	}
}

func TestBdStoreRuntimeCloseAllCompositeBudgetCoversMetadataAndClose(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	store := NewBdStore(t.TempDir(), nil).WithContextRunner(func(ctx context.Context, _ string, _ string, args ...string) ([]byte, error) {
		if len(args) == 0 {
			return nil, errors.New("empty bd args")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
		mu.Lock()
		calls = append(calls, args[0])
		mu.Unlock()
		return []byte(`[]`), nil
	})

	policy := RuntimeWritePolicy(WriteClassHotState, "test.close-budget", "session:gc-1:close")
	policy.Timeout = 40 * time.Millisecond
	n, err := store.RuntimeCloseAll(context.Background(), []string{"gc-1"}, map[string]string{
		"state":        "drained",
		"closed_at":    "2026-05-31T08:15:53Z",
		"close_reason": "session drained: pool slot retired by reconciler",
	}, policy)
	if err != nil {
		t.Fatalf("RuntimeCloseAll: %v", err)
	}
	if n != 1 {
		t.Fatalf("closed = %d, want 1", n)
	}

	mu.Lock()
	got := append([]string(nil), calls...)
	mu.Unlock()
	want := []string{"update", "close"}
	if !slices.Equal(got, want) {
		t.Fatalf("bd calls = %v, want %v", got, want)
	}
}

func TestBdStoreRuntimeWriteCircuitBreakerOpensAfterTimeouts(t *testing.T) {
	store := NewBdStore(t.TempDir(), nil).WithContextRunner(func(ctx context.Context, _ string, _ string, _ ...string) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	for i := 0; i < 3; i++ {
		policy := RuntimeWritePolicy(WriteClassAuditRepair, "test.breaker", "key-"+string(rune('0'+i)))
		policy.Timeout = 5 * time.Millisecond
		_ = store.RuntimeUpdate(context.Background(), "gc-1", UpdateOpts{Metadata: map[string]string{"i": "1"}}, policy)
	}
	var stats RuntimeWriteManagerStats
	for range 100 {
		stats = store.RuntimeWriteManagerStats()
		if stats.BreakerState == RuntimeWriteBreakerOpen {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if stats.BreakerState != RuntimeWriteBreakerOpen {
		t.Fatalf("breaker state = %q, want open (stats=%+v)", stats.BreakerState, stats)
	}
}

func TestBdStoreRuntimeUpdateRetriesTransientWriteError(t *testing.T) {
	tracePath := filepath.Join(t.TempDir(), "bd.trace")
	t.Setenv("GC_BD_TRACE", tracePath)
	var calls atomic.Int64
	store := NewBdStore(t.TempDir(), nil).WithContextRunner(func(_ context.Context, _ string, _ string, _ ...string) ([]byte, error) {
		if calls.Add(1) == 1 {
			return nil, fmt.Errorf("exit status 1: Error updating gc-1: dolt commit: Error 1213 (40001): serialization failure: this transaction conflicts with a committed transaction from another client, try restarting transaction")
		}
		return []byte(`[]`), nil
	})
	policy := RuntimeWritePolicy(WriteClassHotState, "test.runtime-retry", "session:gc-1")
	if err := store.RuntimeUpdate(context.Background(), "gc-1", UpdateOpts{Metadata: map[string]string{"state": "active"}}, policy); err != nil {
		t.Fatalf("RuntimeUpdate: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("runtime update calls = %d, want retry success on second attempt", got)
	}
	traceBytes, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	trace := string(traceBytes)
	if !strings.Contains(trace, "outcome=success") {
		t.Fatalf("trace missing success outcome:\n%s", trace)
	}
	if strings.Contains(trace, "outcome=failed") || strings.Contains(trace, "outcome=ambiguous-timeout") {
		t.Fatalf("trace recorded transient retry as degraded:\n%s", trace)
	}
}

func TestBdStoreRuntimeWriteHotStateSuccessClosesOpenBreaker(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	store := NewBdStore(t.TempDir(), nil).WithContextRunner(func(ctx context.Context, _ string, _ string, _ ...string) ([]byte, error) {
		if fail.Load() {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return []byte(`[]`), nil
	})
	for i := 0; i < 3; i++ {
		policy := RuntimeWritePolicy(WriteClassAuditRepair, "test.breaker", "key-"+string(rune('0'+i)))
		policy.Timeout = 5 * time.Millisecond
		_ = store.RuntimeUpdate(context.Background(), "gc-1", UpdateOpts{Metadata: map[string]string{"i": "1"}}, policy)
	}
	waitForRuntimeWriteManagerState(t, store, RuntimeWriteBreakerOpen)

	fail.Store(false)
	policy := RuntimeWritePolicy(WriteClassHotState, "test.recover-hot", "session:gc-1")
	policy.Timeout = time.Second
	if err := store.RuntimeUpdate(context.Background(), "gc-1", UpdateOpts{Metadata: map[string]string{"state": "running"}}, policy); err != nil {
		t.Fatalf("hot-state RuntimeUpdate: %v", err)
	}
	waitForRuntimeWriteManagerState(t, store, RuntimeWriteBreakerClosed)
}

func TestBdStoreRuntimeWriteOpenBreakerAdmitsTimedRecoveryProbe(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	store := NewBdStore(t.TempDir(), nil).WithContextRunner(func(ctx context.Context, _ string, _ string, args ...string) ([]byte, error) {
		if fail.Load() {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		if len(args) > 0 && args[0] == "create" {
			return []byte(`{"id":"gc-order-probe","title":"probe","status":"open","created_at":"2026-05-31T00:00:00Z"}`), nil
		}
		return []byte(`[]`), nil
	})
	for i := 0; i < 3; i++ {
		policy := RuntimeWritePolicy(WriteClassAuditRepair, "test.breaker", "key-"+string(rune('0'+i)))
		policy.Timeout = 5 * time.Millisecond
		_ = store.RuntimeUpdate(context.Background(), "gc-1", UpdateOpts{Metadata: map[string]string{"i": "1"}}, policy)
	}
	waitForRuntimeWriteManagerState(t, store, RuntimeWriteBreakerOpen)

	manager := store.runtimeWriteManager()
	manager.mu.Lock()
	manager.breakerOpened = time.Now().Add(-RuntimeWriteBreakerRecoveryAfter - time.Second)
	manager.mu.Unlock()
	fail.Store(false)

	policy := RuntimeWritePolicy(WriteClassReservation, "test.recover-probe", "probe")
	policy.Timeout = time.Second
	if _, err := store.RuntimeCreate(context.Background(), Bead{ID: "gc-order-probe", Title: "probe"}, policy); err != nil {
		t.Fatalf("RuntimeCreate recovery probe: %v", err)
	}
	waitForRuntimeWriteManagerState(t, store, RuntimeWriteBreakerClosed)
}

func TestBdStoreRuntimeWriteDoesNotCoalesceStartedUpdate(t *testing.T) {
	var runnerCalls atomic.Int64
	started := make(chan struct{})
	release := make(chan struct{})
	store := NewBdStore(t.TempDir(), nil).WithContextRunner(func(ctx context.Context, _ string, _ string, _ ...string) ([]byte, error) {
		if runnerCalls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-release:
			return []byte(`[]`), nil
		}
	})
	policy := RuntimeWritePolicy(WriteClassAuditRepair, "test.coalesce", "same-key")
	policy.Timeout = time.Second

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(1)
	go func() {
		defer wg.Done()
		errs <- store.RuntimeUpdate(context.Background(), "gc-1", UpdateOpts{Metadata: map[string]string{"same": "true"}}, policy)
	}()
	<-started
	wg.Add(1)
	go func() {
		defer wg.Done()
		errs <- store.RuntimeUpdate(context.Background(), "gc-1", UpdateOpts{Metadata: map[string]string{"same": "true"}}, policy)
	}()
	waitForRuntimeWriteQueueDepth(t, store, 1)
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("RuntimeUpdate: %v", err)
		}
	}
	if runnerCalls.Load() != 2 {
		t.Fatalf("runner calls = %d, want started update to run follow-up write separately", runnerCalls.Load())
	}
	if got := store.RuntimeWriteManagerStats().Collapsed; got != 0 {
		t.Fatalf("collapsed = %d, want 0 for started update", got)
	}
}

func TestBdStoreRuntimeWriteMergesFollowUpAfterStartedUpdate(t *testing.T) {
	var runnerCalls atomic.Int64
	started := make(chan struct{})
	release := make(chan struct{})
	var mu sync.Mutex
	var followUpArgs []string
	store := NewBdStore(t.TempDir(), nil).WithContextRunner(func(ctx context.Context, _ string, _ string, args ...string) ([]byte, error) {
		if runnerCalls.Add(1) == 1 {
			close(started)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-release:
				return []byte(`[]`), nil
			}
		}
		mu.Lock()
		followUpArgs = append([]string(nil), args...)
		mu.Unlock()
		return []byte(`[]`), nil
	})
	policy := RuntimeWritePolicy(WriteClassHotState, "test.coalesce-follow-up", "session:gc-1")
	policy.Timeout = 5 * time.Second

	var wg sync.WaitGroup
	errs := make(chan error, 3)
	wg.Add(1)
	go func() {
		defer wg.Done()
		errs <- store.RuntimeUpdate(context.Background(), "gc-1", UpdateOpts{Metadata: map[string]string{"first": "true"}}, policy)
	}()
	<-started
	wg.Add(1)
	go func() {
		defer wg.Done()
		errs <- store.RuntimeUpdate(context.Background(), "gc-1", UpdateOpts{Metadata: map[string]string{"second": "true"}}, policy)
	}()
	waitForRuntimeWriteQueueDepth(t, store, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		errs <- store.RuntimeUpdate(context.Background(), "gc-1", UpdateOpts{Metadata: map[string]string{"third": "true"}}, policy)
	}()
	waitForRuntimeWriteCollapsed(t, store, 1)
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("RuntimeUpdate: %v", err)
		}
	}
	if runnerCalls.Load() != 2 {
		t.Fatalf("runner calls = %d, want started update plus one merged follow-up", runnerCalls.Load())
	}
	mu.Lock()
	gotArgs := strings.Join(followUpArgs, " ")
	mu.Unlock()
	if !strings.Contains(gotArgs, "second") || !strings.Contains(gotArgs, "third") {
		t.Fatalf("follow-up args = %q, want merged second and third updates", gotArgs)
	}
}

func TestBdStoreRuntimeUpdateMergesQueuedSessionHotState(t *testing.T) {
	holdStarted := make(chan struct{})
	releaseHold := make(chan struct{})
	var holdOnce sync.Once
	var mu sync.Mutex
	var sessionArgs []string
	store := NewBdStore(t.TempDir(), nil).WithContextRunner(func(ctx context.Context, _ string, _ string, args ...string) ([]byte, error) {
		if len(args) >= 3 && args[0] == "update" && args[2] == "gc-hold" {
			holdOnce.Do(func() { close(holdStarted) })
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-releaseHold:
				return []byte(`[]`), nil
			}
		}
		if len(args) >= 3 && args[0] == "update" && args[2] == "gc-1" {
			mu.Lock()
			sessionArgs = append([]string(nil), args...)
			mu.Unlock()
		}
		return []byte(`[]`), nil
	})
	holdPolicy := RuntimeWritePolicy(WriteClassAuditRepair, "test.hold", "hold")
	holdPolicy.Timeout = time.Second
	var holdWG sync.WaitGroup
	holdWG.Add(1)
	go func() {
		defer holdWG.Done()
		if err := store.RuntimeUpdate(context.Background(), "gc-hold", UpdateOpts{Metadata: map[string]string{"hold": "true"}}, holdPolicy); err != nil {
			t.Errorf("hold RuntimeUpdate: %v", err)
		}
	}()
	<-holdStarted

	policy := RuntimeWritePolicy(WriteClassHotState, "test.session-coalesce", "session:gc-1")
	policy.Timeout = time.Second
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(1)
	go func() {
		defer wg.Done()
		errs <- store.RuntimeUpdate(context.Background(), "gc-1", UpdateOpts{Metadata: map[string]string{
			"state":        "orphaned",
			"close_reason": "session orphaned",
			"generation":   "1",
		}}, policy)
	}()
	queueDeadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(queueDeadline) {
		if store.RuntimeWriteManagerStats().QueueDepth >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		errs <- store.RuntimeUpdate(context.Background(), "gc-1", UpdateOpts{
			Labels: []string{"synced"},
			Metadata: map[string]string{
				"state":        "active",
				"close_reason": "",
				"instance":     "2",
			},
		}, policy)
	}()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if store.RuntimeWriteManagerStats().Collapsed > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	close(releaseHold)
	holdWG.Wait()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("RuntimeUpdate: %v", err)
		}
	}
	mu.Lock()
	args := append([]string(nil), sessionArgs...)
	mu.Unlock()
	joined := strings.Join(args, "\n")
	for _, want := range []string{
		"--set-metadata\ngeneration=1",
		"--set-metadata\ninstance=2",
		"--set-metadata\nstate=orphaned",
		"--set-metadata\nclose_reason=session orphaned",
		"--add-label\nsynced",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("merged update args missing %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "state=active") || strings.Contains(joined, "\nclose_reason=\n") {
		t.Fatalf("merged update lost terminal session state:\n%s", joined)
	}
}

func TestBdStoreRuntimeCreateDuplicateMatchingReservationSucceeds(t *testing.T) {
	var createCalls atomic.Int64
	var showCalls atomic.Int64
	store := NewBdStore(t.TempDir(), nil).WithContextRunner(func(_ context.Context, _ string, _ string, args ...string) ([]byte, error) {
		switch args[0] {
		case "create":
			createCalls.Add(1)
			return nil, errors.New("duplicate id already exists")
		case "show":
			showCalls.Add(1)
			return []byte(`[{"id":"gc-order-fixed","title":"fixed","metadata":{"gc.order.reservation_hash":"hash-1"}}]`), nil
		default:
			t.Fatalf("unexpected bd args: %v", args)
			return nil, nil
		}
	})
	policy := RuntimeWritePolicy(WriteClassReservation, "test.duplicate", "reservation-key")
	created, err := store.RuntimeCreate(context.Background(), Bead{
		ID:    "gc-order-fixed",
		Title: "fixed",
		Metadata: map[string]string{
			"gc.order.reservation_hash": "hash-1",
		},
	}, policy)
	if err != nil {
		t.Fatalf("RuntimeCreate duplicate: %v", err)
	}
	if created.ID != "gc-order-fixed" {
		t.Fatalf("created ID = %q, want existing duplicate bead", created.ID)
	}
	if createCalls.Load() != 1 || showCalls.Load() != 1 {
		t.Fatalf("calls create=%d show=%d, want 1/1", createCalls.Load(), showCalls.Load())
	}
}

func TestBdStoreRuntimeCreateDuplicateMismatchedReservationFails(t *testing.T) {
	store := NewBdStore(t.TempDir(), nil).WithContextRunner(func(_ context.Context, _ string, _ string, args ...string) ([]byte, error) {
		switch args[0] {
		case "create":
			return nil, errors.New("duplicate id already exists")
		case "show":
			return []byte(`[{"id":"gc-order-fixed","title":"fixed","metadata":{"gc.order.reservation_hash":"other"}}]`), nil
		default:
			t.Fatalf("unexpected bd args: %v", args)
			return nil, nil
		}
	})
	policy := RuntimeWritePolicy(WriteClassReservation, "test.duplicate", "reservation-key")
	_, err := store.RuntimeCreate(context.Background(), Bead{
		ID:    "gc-order-fixed",
		Title: "fixed",
		Metadata: map[string]string{
			"gc.order.reservation_hash": "hash-1",
		},
	}, policy)
	if !IsDegradedWrite(err) || !strings.Contains(err.Error(), "mismatched reservation metadata") {
		t.Fatalf("err = %v, want degraded mismatched reservation", err)
	}
}

func TestBdStoreRuntimePingUsesRuntimeBudget(t *testing.T) {
	var sawDeadline atomic.Bool
	store := NewBdStore(t.TempDir(), nil).WithContextRunner(func(ctx context.Context, _ string, name string, args ...string) ([]byte, error) {
		if name != "bd" || strings.Join(args, " ") != "list --json --limit 0" {
			t.Fatalf("command = %s %v, want bd list --json --limit 0", name, args)
		}
		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < time.Second {
			sawDeadline.Store(true)
		}
		<-ctx.Done()
		return nil, ctx.Err()
	})
	policy := RuntimeWritePolicy(WriteClassAuditRepair, "test.ping", "ping")
	policy.Timeout = 20 * time.Millisecond
	start := time.Now()
	err := store.RuntimePing(context.Background(), policy)
	elapsed := time.Since(start)
	if !IsDegradedWrite(err) {
		t.Fatalf("err = %v, want degraded ping timeout", err)
	}
	if elapsed > 250*time.Millisecond {
		t.Fatalf("elapsed = %s, want runtime ping budget", elapsed)
	}
	if !sawDeadline.Load() {
		t.Fatal("runtime ping runner did not receive a short context deadline")
	}
	deadline := time.Now().Add(time.Second)
	for {
		stats := store.RuntimeWriteManagerStats()
		if stats.Active == 0 && stats.QueueDepth == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("runtime writer remained active after timeout: %+v", stats)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestBdStoreRuntimeWriteTraceIncludesPolicy(t *testing.T) {
	tracePath := filepath.Join(t.TempDir(), "bd.trace")
	t.Setenv("GC_BD_TRACE", tracePath)
	store := NewBdStore(t.TempDir(), nil).WithContextRunner(func(_ context.Context, _ string, _ string, _ ...string) ([]byte, error) {
		return []byte(`[]`), nil
	})
	policy := RuntimeWritePolicy(WriteClassAuditRepair, "test.trace", "trace-key")
	if err := store.RuntimeUpdate(context.Background(), "gc-1", UpdateOpts{Metadata: map[string]string{"safe": "true"}}, policy); err != nil {
		t.Fatalf("RuntimeUpdate: %v", err)
	}
	data, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	trace := string(data)
	for _, want := range []string{
		"runtime_write",
		"caller=test.trace",
		"class=audit-repair",
		"op=update",
		"command=bd:update",
		"outcome=success",
		"store_key=",
	} {
		if !strings.Contains(trace, want) {
			t.Fatalf("trace missing %q:\n%s", want, trace)
		}
	}
}

func TestExecContextCommandRunnerCancellationKillsDoltRemoteChild(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh unavailable")
	}
	binDir := t.TempDir()
	pidFile := filepath.Join(binDir, "dolt.pid")
	writeRuntimeWriteExecutable(t, filepath.Join(binDir, "dolt"), `#!/bin/sh
echo "$$" > "`+pidFile+`"
sleep 30
`)
	writeRuntimeWriteExecutable(t, filepath.Join(binDir, "bd"), `#!/bin/sh
dolt remote -v &
while [ ! -s "`+pidFile+`" ]; do sleep 0.01; done
	wait
`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := ExecContextCommandRunnerWithEnv(nil)(ctx, t.TempDir(), "bd", "show", "gc-1")
		done <- err
	}()
	childPID := waitForRuntimeWriteTestPID(t, pidFile)
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("runner unexpectedly succeeded after context cancellation")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runner did not return after context cancellation")
	}
	for range 50 {
		if exec.Command("kill", "-0", childPID).Run() != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = exec.Command("kill", "-KILL", childPID).Run()
	t.Fatalf("dolt remote child pid %s survived context timeout", childPID)
}

func writeRuntimeWriteExecutable(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write executable %s: %v", path, err)
	}
}

func waitForRuntimeWriteTestPID(t *testing.T, path string) string {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			pid := strings.TrimSpace(string(data))
			if pid != "" {
				return pid
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("pid file %s was not written", path)
	return ""
}

func waitForRuntimeWriteManagerIdle(t *testing.T, store *BdStore, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		stats := store.RuntimeWriteManagerStats()
		if stats.Active == 0 && stats.QueueDepth == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	stats := store.RuntimeWriteManagerStats()
	t.Fatalf("runtime write manager still active after %s: active=%d queue_depth=%d", timeout, stats.Active, stats.QueueDepth)
}

func waitForRuntimeWriteQueueDepth(t *testing.T, store *BdStore, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if store.RuntimeWriteManagerStats().QueueDepth >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("runtime write queue depth < %d after wait: %+v", want, store.RuntimeWriteManagerStats())
}

func waitForRuntimeWriteCollapsed(t *testing.T, store *BdStore, want int64) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if store.RuntimeWriteManagerStats().Collapsed >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("runtime write collapsed < %d after wait: %+v", want, store.RuntimeWriteManagerStats())
}

func waitForRuntimeWriteManagerState(t *testing.T, store *BdStore, want RuntimeWriteBreakerState) {
	t.Helper()
	const timeout = time.Second
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if got := store.RuntimeWriteManagerStats().BreakerState; got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	stats := store.RuntimeWriteManagerStats()
	t.Fatalf("runtime write breaker state = %q after %s, want %q (stats=%+v)", stats.BreakerState, timeout, want, stats)
}
