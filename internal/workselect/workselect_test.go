package workselect

import (
	"testing"
	"time"

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

func TestSelectorReadyFiltersExplicitSteps(t *testing.T) {
	store := beads.NewMemStore()
	root, err := store.Create(beads.Bead{Title: "root", Type: "molecule"})
	if err != nil {
		t.Fatalf("create root: %v", err)
	}
	first, err := store.Create(beads.Bead{Title: "first", Type: "step", Metadata: map[string]string{"gc.routed_to": "rig/cartographer", "formula": "spec-cartographer"}})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := store.Create(beads.Bead{Title: "second", Type: "step", Metadata: map[string]string{"gc.routed_to": "rig/cartographer", "formula": "spec-cartographer"}})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	if err := store.DepAdd(first.ID, root.ID, "parent-child"); err != nil {
		t.Fatalf("parent dep first: %v", err)
	}
	if err := store.DepAdd(second.ID, root.ID, "parent-child"); err != nil {
		t.Fatalf("parent dep second: %v", err)
	}
	if err := store.DepAdd(second.ID, first.ID, "blocks"); err != nil {
		t.Fatalf("blocks dep second: %v", err)
	}
	selector := config.WorkSelector{
		Status:     "open",
		Type:       "step",
		Unassigned: true,
		Ready:      true,
		Metadata:   map[string]string{"gc.routed_to": "rig/cartographer", "formula": "spec-cartographer"},
	}
	count, err := Count(store, selector)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 1 {
		t.Fatalf("Count = %d, want only the unblocked first step", count)
	}
	next, ok, err := Next(store, selector)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if !ok || next.ID != first.ID {
		t.Fatalf("Next = (%+v, %v), want %s", next, ok, first.ID)
	}
}

func TestWorkSelectorAnyUnionDedupsAndOrders(t *testing.T) {
	store := beads.NewMemStore()
	first, err := store.Create(beads.Bead{
		Title: "warrant",
		Type:  "task",
		Labels: []string{
			"warrant",
		},
		Metadata: map[string]string{
			"gc.routed_to":        "gastown.dog",
			"gc.attached_formula": "mol-shutdown-dance",
		},
	})
	if err != nil {
		t.Fatalf("create warrant: %v", err)
	}
	time.Sleep(time.Millisecond)
	second, err := store.Create(beads.Bead{
		Title: "step",
		Type:  "step",
		Metadata: map[string]string{
			"gc.routed_to": "gastown.dog",
		},
	})
	if err != nil {
		t.Fatalf("create step: %v", err)
	}
	selector := config.WorkSelector{Any: []config.WorkSelector{
		{
			Type:       "step",
			Unassigned: true,
			Metadata:   map[string]string{"gc.routed_to": "gastown.dog"},
		},
		{
			Type:       "task",
			Label:      "warrant",
			Unassigned: true,
			Metadata: map[string]string{
				"gc.routed_to":        "gastown.dog",
				"gc.attached_formula": "mol-shutdown-dance",
			},
		},
		{
			Type:       "task",
			Unassigned: true,
			Metadata:   map[string]string{"gc.routed_to": "gastown.dog"},
		},
	}}
	count, err := Count(store, selector)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 2 {
		t.Fatalf("Count = %d, want 2 unique beads", count)
	}
	items, err := List(store, selector, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 2 || items[0].ID != first.ID || items[1].ID != second.ID {
		t.Fatalf("List IDs = %v, want [%s %s]", beadIDs(items), first.ID, second.ID)
	}
	next, ok, err := Next(store, selector)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if !ok || next.ID != first.ID {
		t.Fatalf("Next = (%+v, %v), want %s", next, ok, first.ID)
	}
}

func beadIDs(items []beads.Bead) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}
