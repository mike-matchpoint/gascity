package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
)

type runtimeReadinessPingStore struct {
	*beads.MemStore
	pingCalls        int
	listCalls        int
	runtimeListCalls int
	runtimeListErr   error
	runtimePolicy    beads.ReadPolicy
	runtimeQuery     beads.ListQuery
	sawDeadline      bool
}

type runtimeReadinessCountStore struct {
	*runtimeReadinessPingStore
	countIndexedCalls int
	countIndexedErr   error
	countQuery        beads.ListQuery
	countSawDeadline  bool
}

func (s *runtimeReadinessCountStore) CountIndexed(ctx context.Context, query beads.ListQuery) (int, error) {
	s.countIndexedCalls++
	s.countQuery = query
	_, s.countSawDeadline = ctx.Deadline()
	return 0, s.countIndexedErr
}

func (s *runtimeReadinessPingStore) Ping() error {
	s.pingCalls++
	return errors.New("foreground ping should not be used for runtime readiness")
}

func (s *runtimeReadinessPingStore) List(query beads.ListQuery) ([]beads.Bead, error) {
	s.listCalls++
	return s.MemStore.List(query)
}

func (s *runtimeReadinessPingStore) RuntimeList(ctx context.Context, query beads.ListQuery, policy beads.ReadPolicy) ([]beads.Bead, error) {
	s.runtimeListCalls++
	s.runtimePolicy = policy
	s.runtimeQuery = query
	_, s.sawDeadline = ctx.Deadline()
	if s.runtimeListErr != nil {
		return nil, s.runtimeListErr
	}
	return s.MemStore.RuntimeList(ctx, query, policy)
}

func TestCheckStoreForRuntimeReadinessUsesIndexedCount(t *testing.T) {
	store := &runtimeReadinessCountStore{
		runtimeReadinessPingStore: &runtimeReadinessPingStore{MemStore: beads.NewMemStore()},
	}
	err := checkStoreForRuntimeReadiness(store, time.Now().Add(5*time.Second), "test.ready")
	if err != nil {
		t.Fatalf("checkStoreForRuntimeReadiness() error = %v", err)
	}
	if store.pingCalls != 0 {
		t.Fatalf("foreground Ping calls = %d, want 0", store.pingCalls)
	}
	if store.listCalls != 0 {
		t.Fatalf("foreground List calls = %d, want 0", store.listCalls)
	}
	if store.countIndexedCalls != 1 {
		t.Fatalf("CountIndexed calls = %d, want 1", store.countIndexedCalls)
	}
	if store.runtimeListCalls != 0 {
		t.Fatalf("RuntimeList calls = %d, want 0", store.runtimeListCalls)
	}
	if store.countQuery.Status != "open" {
		t.Fatalf("count query = %+v, want open status", store.countQuery)
	}
	if !store.countSawDeadline {
		t.Fatal("CountIndexed context had no deadline")
	}
}

func TestCheckStoreForRuntimeReadinessUsesRuntimeListWithoutIndexedCount(t *testing.T) {
	store := &runtimeReadinessPingStore{MemStore: beads.NewMemStore()}
	err := checkStoreForRuntimeReadiness(store, time.Now().Add(5*time.Second), "test.ready")
	if err != nil {
		t.Fatalf("checkStoreForRuntimeReadiness() error = %v", err)
	}
	if store.pingCalls != 0 {
		t.Fatalf("foreground Ping calls = %d, want 0", store.pingCalls)
	}
	if store.listCalls != 0 {
		t.Fatalf("foreground List calls = %d, want 0", store.listCalls)
	}
	if store.runtimeListCalls != 1 {
		t.Fatalf("RuntimeList calls = %d, want 1", store.runtimeListCalls)
	}
	if store.runtimePolicy.Class != beads.ReadClassHotDegradedOK {
		t.Fatalf("policy class = %q, want %q", store.runtimePolicy.Class, beads.ReadClassHotDegradedOK)
	}
	if store.runtimePolicy.Timeout > beads.HotDegradedOKBudget {
		t.Fatalf("policy timeout = %s, want <= %s", store.runtimePolicy.Timeout, beads.HotDegradedOKBudget)
	}
	if store.runtimePolicy.MaxRows != 1 {
		t.Fatalf("policy MaxRows = %d, want 1", store.runtimePolicy.MaxRows)
	}
	if store.runtimeQuery.Status != "open" || store.runtimeQuery.Limit != 1 {
		t.Fatalf("runtime query = %+v, want open limit 1", store.runtimeQuery)
	}
	if !store.sawDeadline {
		t.Fatal("RuntimeList context had no deadline")
	}
}

func TestCheckStoreForRuntimeReadinessPropagatesIndexedCountError(t *testing.T) {
	wantErr := errors.New("indexed count unavailable")
	store := &runtimeReadinessCountStore{
		runtimeReadinessPingStore: &runtimeReadinessPingStore{MemStore: beads.NewMemStore()},
		countIndexedErr:           wantErr,
	}
	err := checkStoreForRuntimeReadiness(store, time.Now().Add(5*time.Second), "test.ready")
	if !errors.Is(err, wantErr) {
		t.Fatalf("checkStoreForRuntimeReadiness() error = %v, want %v", err, wantErr)
	}
	if store.pingCalls != 0 {
		t.Fatalf("foreground Ping calls = %d, want 0", store.pingCalls)
	}
	if store.listCalls != 0 {
		t.Fatalf("foreground List calls = %d, want 0", store.listCalls)
	}
	if store.countIndexedCalls != 1 {
		t.Fatalf("CountIndexed calls = %d, want 1", store.countIndexedCalls)
	}
	if store.runtimeListCalls != 0 {
		t.Fatalf("RuntimeList calls = %d, want 0", store.runtimeListCalls)
	}
}
