package main

import (
	"io"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/events"
)

// TestEmitBeadClosedMirrorsHookEmit asserts the consolidated on-close
// emit produces the same bead.closed event the hook's standalone
// `gc event emit` call produced: subject = bead ID, message = issue
// title, payload = {"bead":<issue JSON>}.
func TestEmitBeadClosedMirrorsHookEmit(t *testing.T) {
	fake := events.NewFake()
	raw := []byte(`{"id":"gc-1","title":"fix the flux capacitor"}`)

	emitBeadClosed(fake, "gc-1", raw, io.Discard)

	evts, err := fake.List(events.Filter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(evts) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evts))
	}
	e := evts[0]
	if e.Type != "bead.closed" {
		t.Errorf("type = %q, want bead.closed", e.Type)
	}
	if e.Subject != "gc-1" {
		t.Errorf("subject = %q, want gc-1", e.Subject)
	}
	if e.Message != "fix the flux capacitor" {
		t.Errorf("message = %q, want issue title", e.Message)
	}
	if !strings.Contains(string(e.Payload), `"bead":{"id":"gc-1"`) {
		t.Errorf("payload missing wrapped bead JSON: %s", e.Payload)
	}
}

// TestEmitBeadClosedDropsInvalidIssueJSON mirrors the previous hook
// behavior: malformed stdin made the --payload invalid, and
// doEventEmit's payload validation dropped the event entirely.
func TestEmitBeadClosedDropsInvalidIssueJSON(t *testing.T) {
	fake := events.NewFake()

	emitBeadClosed(fake, "gc-1", []byte("not json"), io.Discard)
	emitBeadClosed(fake, "gc-1", nil, io.Discard)

	evts, err := fake.List(events.Filter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(evts) != 0 {
		t.Fatalf("expected invalid issue JSON to drop the event, got %d events", len(evts))
	}
}

// TestDoBeadsOnCloseAutoclosersClosesMoleculeRoot asserts the
// consolidated path still reaches molecule autoclose — the last step of
// the old four-process chain — with a shared store and recorder.
func TestDoBeadsOnCloseAutoclosersClosesMoleculeRoot(t *testing.T) {
	store := beads.NewMemStore()
	root, err := store.Create(beads.Bead{Title: "mol-focus-review", Type: "molecule"})
	if err != nil {
		t.Fatalf("Create(root): %v", err)
	}
	step, err := store.Create(beads.Bead{Title: "Run tests", Type: "step", ParentID: root.ID})
	if err != nil {
		t.Fatalf("Create(step): %v", err)
	}
	if err := store.Close(step.ID); err != nil {
		t.Fatalf("Close(step): %v", err)
	}

	rec := events.NewFake()
	doBeadsOnCloseAutoclosers(store, rec, step.ID, io.Discard, io.Discard)

	got, err := store.Get(root.ID)
	if err != nil {
		t.Fatalf("Get(root): %v", err)
	}
	if got.Status != "closed" {
		t.Errorf("molecule root status = %q, want closed (autoclose did not run)", got.Status)
	}
}
