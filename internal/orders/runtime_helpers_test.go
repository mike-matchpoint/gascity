package orders

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
)

type rowsErrorStore struct {
	*beads.MemStore
	rows []beads.Bead
	err  error
}

func (s *rowsErrorStore) List(_ beads.ListQuery) ([]beads.Bead, error) {
	return s.rows, s.err
}

type runtimeRowsStore struct {
	*rowsErrorStore
	query  beads.ListQuery
	policy beads.ReadPolicy
	calls  int
}

func (s *runtimeRowsStore) RuntimeList(_ context.Context, query beads.ListQuery, policy beads.ReadPolicy) ([]beads.Bead, error) {
	s.calls++
	s.query = query
	s.policy = policy
	return s.rows, s.err
}

func TestLastRunFuncForStoreReturnsLatestRun(t *testing.T) {
	store := beads.NewMemStore()

	first, err := store.Create(beads.Bead{
		Title:  "order:digest",
		Status: "closed",
		Labels: []string{"order-run:digest"},
	})
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Millisecond)

	second, err := store.Create(beads.Bead{
		Title:  "order:digest",
		Status: "closed",
		Labels: []string{"order-run:digest", "wisp-failed"},
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := LastRunFuncForStore(store)("digest")
	if err != nil {
		t.Fatalf("LastRunFuncForStore(): %v", err)
	}
	if !got.Equal(second.CreatedAt) {
		t.Fatalf("LastRunFuncForStore() = %s, want %s (latest run should remain authoritative)", got, second.CreatedAt)
	}
	if !second.CreatedAt.After(first.CreatedAt) {
		t.Fatalf("test setup invalid: second.CreatedAt=%s, first.CreatedAt=%s", second.CreatedAt, first.CreatedAt)
	}
}

func TestLastRunFuncForStoreReturnsZeroWhenNoRunsExist(t *testing.T) {
	store := beads.NewMemStore()

	got, err := LastRunFuncForStore(store)("digest")
	if err != nil {
		t.Fatalf("LastRunFuncForStore(): %v", err)
	}
	if !got.IsZero() {
		t.Fatalf("LastRunFuncForStore() = %s, want zero time", got)
	}
}

func TestLastRunFuncForStoreReturnsListError(t *testing.T) {
	wantErr := errors.New("store unavailable")
	store := &rowsErrorStore{
		MemStore: beads.NewMemStore(),
		rows: []beads.Bead{{
			ID:        "run-1",
			Title:     "digest",
			CreatedAt: time.Date(2026, 5, 15, 7, 0, 0, 0, time.UTC),
			Labels:    []string{"order-run:digest"},
		}},
		err: wantErr,
	}

	got, err := LastRunFuncForStore(store)("digest")
	if !errors.Is(err, wantErr) {
		t.Fatalf("LastRunFuncForStore() err = %v, want %v", err, wantErr)
	}
	if !got.IsZero() {
		t.Fatalf("LastRunFuncForStore() = %s, want zero time on list error", got)
	}
}

func TestRuntimeLastRunFuncForStoreUsesBoundedRuntimeList(t *testing.T) {
	want := time.Date(2026, 5, 15, 7, 0, 0, 0, time.UTC)
	store := &runtimeRowsStore{rowsErrorStore: &rowsErrorStore{
		MemStore: beads.NewMemStore(),
		rows: []beads.Bead{{
			ID:        "run-1",
			Title:     "digest",
			CreatedAt: want,
			Labels:    []string{"order-run:digest"},
		}},
	}}

	got, err := RuntimeLastRunFuncForStore(store, time.Second, "test.runtime-last-run")("digest")
	if err != nil {
		t.Fatalf("RuntimeLastRunFuncForStore(): %v", err)
	}
	if !got.Equal(want) {
		t.Fatalf("RuntimeLastRunFuncForStore() = %s, want %s", got, want)
	}
	if store.calls != 1 {
		t.Fatalf("RuntimeList calls = %d, want 1", store.calls)
	}
	if store.query.Label != "order-run:digest" || !store.query.IncludeClosed || store.query.Limit != 1 || store.query.TierMode != beads.TierBoth {
		t.Fatalf("RuntimeList query = %+v, want bounded order-run history query", store.query)
	}
	if store.policy.MaxRows != 2 {
		t.Fatalf("RuntimeList policy MaxRows = %d, want 2 so one returned row is not treated as a row-cap", store.policy.MaxRows)
	}
}

func TestRuntimeLastRunFuncForStoreAllowsOneRowWithoutRowCapDegradation(t *testing.T) {
	store := beads.NewMemStore()
	created, err := store.Create(beads.Bead{
		Title:  "order:digest",
		Labels: []string{"order-run:digest"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(created.ID); err != nil {
		t.Fatal(err)
	}

	got, err := RuntimeLastRunFuncForStore(store, time.Second, "test.runtime-last-run")("digest")
	if err != nil {
		t.Fatalf("RuntimeLastRunFuncForStore(): %v", err)
	}
	if !got.Equal(created.CreatedAt) {
		t.Fatalf("RuntimeLastRunFuncForStore() = %s, want %s", got, created.CreatedAt)
	}
}

func TestCursorFuncForStoreReturnsZeroOnListError(t *testing.T) {
	store := &rowsErrorStore{
		MemStore: beads.NewMemStore(),
		rows: []beads.Bead{{
			ID:     "run-1",
			Labels: []string{"order-run:digest", "seq:42"},
		}},
		err: errors.New("store unavailable"),
	}

	got := CursorFuncForStore(store)("digest")
	if got != 0 {
		t.Fatalf("CursorFuncForStore() = %d, want 0 on list error", got)
	}
}
