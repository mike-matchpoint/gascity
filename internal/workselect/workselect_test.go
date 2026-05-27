package workselect

import (
	"testing"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
)

func TestListAppliesSameSelectorForCountAndNext(t *testing.T) {
	store := beads.NewMemStore()
	if _, err := store.Create(beads.Bead{Title: "first", Type: "task", Metadata: map[string]string{"gc.routed_to": "rig/worker"}}); err != nil {
		t.Fatalf("create first: %v", err)
	}
	if _, err := store.Create(beads.Bead{Title: "assigned", Type: "task", Assignee: "other", Metadata: map[string]string{"gc.routed_to": "rig/worker"}}); err != nil {
		t.Fatalf("create assigned: %v", err)
	}
	if _, err := store.Create(beads.Bead{Title: "other route", Type: "task", Metadata: map[string]string{"gc.routed_to": "rig/other"}}); err != nil {
		t.Fatalf("create other route: %v", err)
	}
	selector := config.WorkSelector{
		Type:       "task",
		Unassigned: true,
		Metadata:   map[string]string{"gc.routed_to": "rig/worker"},
	}
	count, err := Count(store, selector)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 1 {
		t.Fatalf("Count = %d, want 1", count)
	}
	next, ok, err := Next(store, selector)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if !ok || next.Title != "first" {
		t.Fatalf("Next = (%+v, %v), want first", next, ok)
	}
}

func TestSelectorIncludesInfrastructureOnlyWhenExplicit(t *testing.T) {
	store := beads.NewMemStore()
	if _, err := store.Create(beads.Bead{Title: "step", Type: "step", Metadata: map[string]string{"gc.routed_to": "rig/worker"}}); err != nil {
		t.Fatalf("create step: %v", err)
	}
	if _, err := store.Create(beads.Bead{Title: "task", Type: "task", Metadata: map[string]string{"gc.routed_to": "rig/worker"}}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	selector := config.WorkSelector{Metadata: map[string]string{"gc.routed_to": "rig/worker"}}
	count, err := Count(store, selector)
	if err != nil {
		t.Fatalf("Count default: %v", err)
	}
	if count != 1 {
		t.Fatalf("default Count = %d, want 1 task only", count)
	}
	selector.Type = "step"
	count, err = Count(store, selector)
	if err != nil {
		t.Fatalf("Count step: %v", err)
	}
	if count != 1 {
		t.Fatalf("step Count = %d, want explicit step", count)
	}
	next, ok, err := Next(store, selector)
	if err != nil {
		t.Fatalf("Next step: %v", err)
	}
	if !ok || next.Type != "step" {
		t.Fatalf("Next explicit step = (%+v, %v), want step", next, ok)
	}
}
