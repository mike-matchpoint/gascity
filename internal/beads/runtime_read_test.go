package beads

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type runtimeReadIndexedStub struct {
	result IndexedListResult
	err    error
}

func (s runtimeReadIndexedStub) ListIndexed(context.Context, ListQuery) (IndexedListResult, error) {
	return s.result, s.err
}

type runtimeReadIndexedGetStub struct {
	runtimeReadIndexedStub
	getResult Bead
	getErr    error
	getCalls  atomic.Int64
	getID     string
}

func (s *runtimeReadIndexedGetStub) GetIndexed(_ context.Context, id string) (Bead, error) {
	s.getCalls.Add(1)
	s.getID = id
	return s.getResult, s.getErr
}

type slowRuntimeReadIndexedStub struct{}

func (slowRuntimeReadIndexedStub) ListIndexed(ctx context.Context, _ ListQuery) (IndexedListResult, error) {
	select {
	case <-ctx.Done():
		return IndexedListResult{}, ctx.Err()
	case <-time.After(time.Second):
		return IndexedListResult{DependencyCoverage: true, LabelsCoverage: true}, nil
	}
}

func TestRuntimeListHotDoesNotFallbackToBdListOnIndexedError(t *testing.T) {
	var runnerCalls atomic.Int64
	store := NewBdStore("/city", func(string, string, ...string) ([]byte, error) {
		runnerCalls.Add(1)
		return nil, errors.New("runner must not be called")
	}).WithIndexedReader(runtimeReadIndexedStub{err: context.DeadlineExceeded})

	rows, err := RuntimeList(context.Background(), store, ListQuery{Status: "open"},
		RuntimeReadPolicy(ReadClassHotAuthoritative, "test.hot-list"))
	if len(rows) != 0 {
		t.Fatalf("rows = %v, want none on indexed error", rows)
	}
	if !IsDegradedRead(err) {
		t.Fatalf("err = %v, want degraded read", err)
	}
	if runnerCalls.Load() != 0 {
		t.Fatalf("runner calls = %d, want 0", runnerCalls.Load())
	}
}

func TestRuntimeGetHotUsesIndexedGetterWithoutBDShow(t *testing.T) {
	var runnerCalls atomic.Int64
	indexed := &runtimeReadIndexedGetStub{
		getResult: Bead{ID: "gc-1", Title: "one", Status: "open", Type: "task"},
	}
	store := NewBdStore("/city", func(string, string, ...string) ([]byte, error) {
		runnerCalls.Add(1)
		return nil, errors.New("runner must not be called")
	}).WithIndexedReader(indexed)

	got, err := RuntimeGet(context.Background(), store, "gc-1",
		RuntimeReadPolicy(ReadClassHotAuthoritative, "test.hot-get"))
	if err != nil {
		t.Fatalf("RuntimeGet: %v", err)
	}
	if got.ID != "gc-1" {
		t.Fatalf("RuntimeGet ID = %q, want gc-1", got.ID)
	}
	if indexed.getCalls.Load() != 1 || indexed.getID != "gc-1" {
		t.Fatalf("indexed get calls=%d id=%q, want 1/gc-1", indexed.getCalls.Load(), indexed.getID)
	}
	if runnerCalls.Load() != 0 {
		t.Fatalf("runner calls = %d, want 0", runnerCalls.Load())
	}
}

func TestRuntimeGetHotDegradesWhenIndexedGetterUnavailable(t *testing.T) {
	var runnerCalls atomic.Int64
	store := NewBdStore("/city", func(string, string, ...string) ([]byte, error) {
		runnerCalls.Add(1)
		return nil, errors.New("runner must not be called")
	}).WithIndexedReader(runtimeReadIndexedStub{})

	_, err := RuntimeGet(context.Background(), store, "gc-1",
		RuntimeReadPolicy(ReadClassHotAuthoritative, "test.hot-get"))
	if !IsDegradedRead(err) {
		t.Fatalf("err = %v, want degraded read", err)
	}
	if runnerCalls.Load() != 0 {
		t.Fatalf("runner calls = %d, want 0", runnerCalls.Load())
	}
}

func TestRuntimeListHonorsHotReadBudget(t *testing.T) {
	var runnerCalls atomic.Int64
	store := NewBdStore("/city", func(string, string, ...string) ([]byte, error) {
		runnerCalls.Add(1)
		return nil, errors.New("runner must not be called")
	}).WithIndexedReader(slowRuntimeReadIndexedStub{})

	policy := RuntimeReadPolicy(ReadClassHotDegradedOK, "test.budget")
	policy.Timeout = 20 * time.Millisecond
	start := time.Now()
	_, err := RuntimeList(context.Background(), store, ListQuery{Status: "open"}, policy)
	elapsed := time.Since(start)
	if !IsDegradedRead(err) {
		t.Fatalf("err = %v, want degraded read", err)
	}
	if elapsed > 250*time.Millisecond {
		t.Fatalf("elapsed = %s, want bounded hot read", elapsed)
	}
	if runnerCalls.Load() != 0 {
		t.Fatalf("runner calls = %d, want 0", runnerCalls.Load())
	}
}

func TestRuntimeListHotReturnsDegradedPartialCoverageWithoutFallback(t *testing.T) {
	var runnerCalls atomic.Int64
	store := NewBdStore("/city", func(string, string, ...string) ([]byte, error) {
		runnerCalls.Add(1)
		return nil, errors.New("runner must not be called")
	}).WithIndexedReader(runtimeReadIndexedStub{result: IndexedListResult{
		Beads:          []Bead{{ID: "bd-1", Title: "partial", Status: "open", Type: "task"}},
		LabelsCoverage: true,
	}})

	rows, err := RuntimeList(context.Background(), store, ListQuery{Status: "open"},
		RuntimeReadPolicy(ReadClassHotAuthoritative, "test.partial-coverage"))
	if len(rows) != 1 || rows[0].ID != "bd-1" {
		t.Fatalf("rows = %+v, want indexed partial row", rows)
	}
	if !IsDegradedRead(err) {
		t.Fatalf("err = %v, want degraded read", err)
	}
	if runnerCalls.Load() != 0 {
		t.Fatalf("runner calls = %d, want 0", runnerCalls.Load())
	}
}

func TestRuntimeListForegroundKeepsAuthoritativeFallback(t *testing.T) {
	var runnerCalls atomic.Int64
	store := NewBdStore("/city", func(_ string, name string, args ...string) ([]byte, error) {
		runnerCalls.Add(1)
		got := name + " " + strings.Join(args, " ")
		if !strings.HasPrefix(got, "bd list ") {
			t.Fatalf("runner command = %q, want bd list", got)
		}
		return []byte(`[{"id":"bd-1","title":"one","status":"open","issue_type":"task","created_at":"2026-05-27T00:00:00Z"}]`), nil
	})

	rows, err := RuntimeList(context.Background(), store, ListQuery{Status: "open"},
		RuntimeReadPolicy(ReadClassForegroundAuthoritative, "test.foreground"))
	if err != nil {
		t.Fatalf("RuntimeList foreground: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != "bd-1" {
		t.Fatalf("rows = %+v, want foreground bd row", rows)
	}
	if runnerCalls.Load() != 1 {
		t.Fatalf("runner calls = %d, want 1", runnerCalls.Load())
	}
}

func TestCachingStoreRuntimeReadyDoesNotFallbackToBdReady(t *testing.T) {
	var runnerCalls atomic.Int64
	backing := NewBdStore("/city", func(string, string, ...string) ([]byte, error) {
		runnerCalls.Add(1)
		return nil, errors.New("runner must not be called")
	})
	cache := NewCachingStoreForTest(backing, nil)

	rows, err := RuntimeReady(context.Background(), cache, ReadyQuery{},
		RuntimeReadPolicy(ReadClassHotDegradedOK, "test.ready"))
	if len(rows) != 0 {
		t.Fatalf("rows = %+v, want none when cache cannot prove ready state", rows)
	}
	if !IsDegradedRead(err) {
		t.Fatalf("err = %v, want degraded read", err)
	}
	if runnerCalls.Load() != 0 {
		t.Fatalf("runner calls = %d, want 0", runnerCalls.Load())
	}
}

func TestCachingStoreRuntimeReadyUsesIndexedActiveRowsWhenCacheDepsIncomplete(t *testing.T) {
	var runnerCalls atomic.Int64
	backing := NewBdStore("/city", func(string, string, ...string) ([]byte, error) {
		runnerCalls.Add(1)
		return nil, errors.New("runner must not be called")
	}).WithIndexedReader(runtimeReadIndexedStub{result: IndexedListResult{
		Beads: []Bead{
			{ID: "blocker", Title: "blocker", Status: "in_progress", Type: "task"},
			{
				ID:     "blocked",
				Title:  "blocked",
				Status: "open",
				Type:   "bug",
				Dependencies: []Dep{{
					IssueID:     "blocked",
					DependsOnID: "blocker",
					Type:        "blocks",
				}},
			},
			{ID: "ready", Title: "ready", Status: "open", Type: "bug"},
			{ID: "workflow-step", Title: "step", Status: "open", Type: "step"},
		},
		DependencyCoverage: true,
		LabelsCoverage:     true,
	}})
	cache := NewCachingStoreForTest(backing, nil)

	rows, err := RuntimeReady(context.Background(), cache, ReadyQuery{},
		RuntimeReadPolicy(ReadClassHotDegradedOK, "test.ready"))
	if err != nil {
		t.Fatalf("RuntimeReady: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != "ready" {
		t.Fatalf("rows = %+v, want only indexed ready row", rows)
	}
	if runnerCalls.Load() != 0 {
		t.Fatalf("runner calls = %d, want 0", runnerCalls.Load())
	}
}

func TestCachingStoreRuntimeReadyDegradesBeforeComputingFromCappedIndexedActiveRows(t *testing.T) {
	var runnerCalls atomic.Int64
	backing := NewBdStore("/city", func(string, string, ...string) ([]byte, error) {
		runnerCalls.Add(1)
		return nil, errors.New("runner must not be called")
	}).WithIndexedReader(runtimeReadIndexedStub{result: IndexedListResult{
		Beads: []Bead{
			{
				ID:     "candidate",
				Title:  "would be unsafe if blocker is outside the cap",
				Status: "open",
				Type:   "bug",
				Dependencies: []Dep{{
					IssueID:     "candidate",
					DependsOnID: "open-blocker-outside-active-window",
					Type:        "blocks",
				}},
			},
			{ID: "cap-row", Title: "second row reaches cap", Status: "open", Type: "bug"},
		},
		DependencyCoverage: true,
		LabelsCoverage:     true,
	}})
	cache := NewCachingStoreForTest(backing, nil)
	policy := RuntimeReadPolicy(ReadClassHotDegradedOK, "test.ready.capped")
	policy.MaxRows = 2

	rows, err := RuntimeReady(context.Background(), cache, ReadyQuery{}, policy)
	if len(rows) != 0 {
		t.Fatalf("rows = %+v, want none when indexed active rows hit the cap", rows)
	}
	if !IsDegradedRead(err) {
		t.Fatalf("err = %v, want degraded read before readiness is computed from a capped active set", err)
	}
	if runnerCalls.Load() != 0 {
		t.Fatalf("runner calls = %d, want 0", runnerCalls.Load())
	}
}

func TestCachingStoreRuntimeReadyDegradesBeforeComputingFromPartialIndexedActiveRows(t *testing.T) {
	var runnerCalls atomic.Int64
	backing := NewBdStore("/city", func(string, string, ...string) ([]byte, error) {
		runnerCalls.Add(1)
		return nil, errors.New("runner must not be called")
	}).WithIndexedReader(runtimeReadIndexedStub{
		result: IndexedListResult{
			Beads: []Bead{{
				ID:     "candidate",
				Title:  "partial row must not be treated as ready",
				Status: "open",
				Type:   "bug",
				Dependencies: []Dep{{
					IssueID:     "candidate",
					DependsOnID: "missing-from-partial-active-set",
					Type:        "blocks",
				}},
			}},
			DependencyCoverage: true,
			LabelsCoverage:     true,
		},
		err: context.DeadlineExceeded,
	})
	cache := NewCachingStoreForTest(backing, nil)

	rows, err := RuntimeReady(context.Background(), cache, ReadyQuery{},
		RuntimeReadPolicy(ReadClassHotDegradedOK, "test.ready.partial"))
	if len(rows) != 0 {
		t.Fatalf("rows = %+v, want none when indexed active rows are partial", rows)
	}
	if !IsDegradedRead(err) {
		t.Fatalf("err = %v, want degraded read before readiness is computed from partial active rows", err)
	}
	if runnerCalls.Load() != 0 {
		t.Fatalf("runner calls = %d, want 0", runnerCalls.Load())
	}
}

func TestCachingStoreRuntimeReadyListUsesBackingRuntimeReadyListWhenCacheDirty(t *testing.T) {
	backing := NewMemStore()
	ready, err := backing.Create(Bead{
		Title:    "assigned formula step",
		Type:     "step",
		Status:   "open",
		Assignee: "cartographer",
	})
	if err != nil {
		t.Fatal(err)
	}
	blocker, err := backing.Create(Bead{
		Title:  "open blocker",
		Type:   "task",
		Status: "open",
	})
	if err != nil {
		t.Fatal(err)
	}
	blocked, err := backing.Create(Bead{
		Title:    "blocked assigned formula step",
		Type:     "step",
		Status:   "open",
		Assignee: "cartographer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := backing.DepAdd(blocked.ID, blocker.ID, "blocks"); err != nil {
		t.Fatalf("DepAdd: %v", err)
	}

	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	markCacheDirtyForTest(cache, ready.ID)

	rows, err := RuntimeReadyList(context.Background(), cache, ListQuery{
		Type:     "step",
		Assignee: "cartographer",
	}, RuntimeReadPolicy(ReadClassHotDegradedOK, "test.ready-list.dirty-cache"))
	if err != nil {
		t.Fatalf("RuntimeReadyList: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != ready.ID {
		t.Fatalf("rows = %+v, want only backing-proven ready step %s", rows, ready.ID)
	}
}

func TestHasUsableReadRowsPreservesRuntimeDegradedRowsOnlyWhenPresent(t *testing.T) {
	err := &DegradedReadError{
		Class:     ReadClassHotDegradedOK,
		Caller:    "test.usable-rows",
		Operation: "list",
		Route:     "indexed",
		Coverage:  "row-cap",
		Err:       context.DeadlineExceeded,
	}
	if !HasUsableReadRows(err, 1) {
		t.Fatal("HasUsableReadRows(degraded, 1) = false, want true")
	}
	if HasUsableReadRows(err, 0) {
		t.Fatal("HasUsableReadRows(degraded, 0) = true, want false")
	}
	if !HasUsableReadRows(&PartialResultError{Op: "bd list", Err: context.DeadlineExceeded}, 0) {
		t.Fatal("HasUsableReadRows(partial, 0) = false, want legacy partial rows usable")
	}
}
