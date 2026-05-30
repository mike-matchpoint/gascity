package main

import (
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gastownhall/gascity/internal/beads"
)

func TestControllerDemandListDoesNotFallbackToBdList(t *testing.T) {
	var runnerCalls atomic.Int64
	backing := beads.NewBdStore("/city", func(string, string, ...string) ([]byte, error) {
		runnerCalls.Add(1)
		return nil, errors.New("runner must not be called")
	})
	store := beads.NewCachingStoreForTest(backing, nil)

	rows, err := listForControllerDemand(store, beads.ListQuery{Status: "open"})
	if len(rows) != 0 {
		t.Fatalf("rows = %+v, want none when runtime read degrades", rows)
	}
	if !beads.IsDegradedRead(err) {
		t.Fatalf("err = %v, want degraded read", err)
	}
	if runnerCalls.Load() != 0 {
		t.Fatalf("runner calls = %d, want 0", runnerCalls.Load())
	}
}

func TestRuntimeReadStaticGuardForControllerHotPaths(t *testing.T) {
	for _, path := range []string{"build_desired_state.go", "session_reconciler.go"} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		text := string(body)
		forbidden := []string{
			"beads.ReadyLive(",
			"Live: true",
		}
		for _, needle := range forbidden {
			if strings.Contains(text, needle) {
				t.Fatalf("%s contains %q; hot controller/session reads must use beads.RuntimeList/RuntimeReady", path, needle)
			}
		}
	}
}

func TestSessionHotPathStaticGuardForRuntimeStoreAccess(t *testing.T) {
	for _, path := range []string{
		"session_beads.go",
		"session_lifecycle_parallel.go",
		"session_reconciler.go",
	} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		text := string(body)
		forbidden := []string{
			"store.Get(",
			"store.Update(",
			"store.SetMetadata(",
			"store.SetMetadataBatch(",
			"store.Close(",
		}
		for _, needle := range forbidden {
			if strings.Contains(text, needle) {
				t.Fatalf("%s contains %q; hot session writes/ID reads must use runtime policies", path, needle)
			}
		}
	}

	body, err := os.ReadFile("session_bead_snapshot.go")
	if err != nil {
		t.Fatalf("reading session_bead_snapshot.go: %v", err)
	}
	if strings.Contains(string(body), "ListAllSessionBeads(") {
		t.Fatal("session_bead_snapshot.go calls ListAllSessionBeads; hot session snapshots must use ListAllSessionBeadsRuntime")
	}
}

func TestControllerDemandReadyDoesNotFallbackToBdReady(t *testing.T) {
	var runnerCalls atomic.Int64
	backing := beads.NewBdStore("/city", func(string, string, ...string) ([]byte, error) {
		runnerCalls.Add(1)
		return nil, errors.New("runner must not be called")
	})
	store := beads.NewCachingStoreForTest(backing, nil)

	rows, err := readyForControllerDemandQuery(store, beads.ReadyQuery{Assignee: "worker"})
	if len(rows) != 0 {
		t.Fatalf("rows = %+v, want none when runtime ready degrades", rows)
	}
	if !beads.IsDegradedRead(err) {
		t.Fatalf("err = %v, want degraded read", err)
	}
	if runnerCalls.Load() != 0 {
		t.Fatalf("runner calls = %d, want 0", runnerCalls.Load())
	}
}
