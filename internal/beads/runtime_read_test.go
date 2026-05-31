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

type slowRuntimeReadIndexedStub struct{}

func (slowRuntimeReadIndexedStub) ListIndexed(ctx context.Context, _ ListQuery) (IndexedListResult, error) {
	select {
	case <-ctx.Done():
		return IndexedListResult{}, ctx.Err()
	case <-time.After(time.Second):
		return IndexedListResult{DependencyCoverage: true, LabelsCoverage: true}, nil
	}
}

type runtimeReadNoFallbackStore struct {
	Store
	listCalls int
}

func (s *runtimeReadNoFallbackStore) List(query ListQuery) ([]Bead, error) {
	s.listCalls++
	return s.Store.List(query)
}

func (s *runtimeReadNoFallbackStore) RuntimeHotFallbackDisabled() bool {
	return true
}

func TestRuntimeListHotWithoutRuntimeListerReturnsDegradedWithoutFallback(t *testing.T) {
	base := NewMemStore()
	store := &runtimeReadNoFallbackStore{Store: base}
	_, err := RuntimeList(context.Background(), store, ListQuery{Status: "open"},
		RuntimeReadPolicy(ReadClassHotAuthoritative, "test.no-runtime-list"))
	if !IsDegradedRead(err) {
		t.Fatalf("err = %v, want degraded read", err)
	}
	if store.listCalls != 0 {
		t.Fatalf("List calls = %d, want 0", store.listCalls)
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
