package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
)

func setupTypedWorkCommandCity(t *testing.T) beads.Store {
	t.Helper()
	clearGCEnv(t)
	disableManagedDoltRecoveryForTest(t)
	resetFlags(t)
	t.Setenv("GC_BEADS", "file")
	t.Setenv("GC_BEADS_SCOPE_ROOT", "")
	cityDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cityDir, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ensureScopedFileStoreLayout(cityDir); err != nil {
		t.Fatalf("ensure scoped file store layout: %v", err)
	}
	if err := ensurePersistedScopeLocalFileStore(cityDir); err != nil {
		t.Fatalf("ensure file store: %v", err)
	}
	cityToml := `[workspace]
name = "typed-work-test"

[[agent]]
name = "worker"

[agent.work_selector]
type = "task"
unassigned = true
ready = true

[agent.work_selector.metadata]
"gc.routed_to" = "worker"
`
	if err := os.WriteFile(filepath.Join(cityDir, "city.toml"), []byte(cityToml), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GC_CITY", cityDir)
	store, err := openCityStoreAt(cityDir)
	if err != nil {
		t.Fatalf("open city store: %v", err)
	}
	return store
}

func setupRoutedWorkCommandCity(t *testing.T) beads.Store {
	t.Helper()
	clearGCEnv(t)
	disableManagedDoltRecoveryForTest(t)
	resetFlags(t)
	t.Setenv("GC_BEADS", "file")
	t.Setenv("GC_BEADS_SCOPE_ROOT", "")
	cityDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cityDir, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ensureScopedFileStoreLayout(cityDir); err != nil {
		t.Fatalf("ensure scoped file store layout: %v", err)
	}
	if err := ensurePersistedScopeLocalFileStore(cityDir); err != nil {
		t.Fatalf("ensure file store: %v", err)
	}
	cityToml := `[workspace]
name = "routed-work-test"

[[agent]]
name = "worker"
`
	if err := os.WriteFile(filepath.Join(cityDir, "city.toml"), []byte(cityToml), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GC_CITY", cityDir)
	store, err := openCityStoreAt(cityDir)
	if err != nil {
		t.Fatalf("open city store: %v", err)
	}
	return store
}

func TestCmdHookTypedSelectorReturnsAssignedInProgressWork(t *testing.T) {
	store := setupTypedWorkCommandCity(t)
	work, err := store.Create(beads.Bead{
		Title:    "resume assigned work",
		Type:     "task",
		Assignee: "worker-session",
	})
	if err != nil {
		t.Fatalf("create work: %v", err)
	}
	if err := store.Update(work.ID, beads.UpdateOpts{Status: stringPtr("in_progress")}); err != nil {
		t.Fatalf("mark in_progress: %v", err)
	}
	t.Setenv("GC_TEMPLATE", "worker")
	t.Setenv("GC_SESSION_NAME", "worker-session")

	var stdout, stderr bytes.Buffer
	code := cmdHook(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("cmdHook() = %d, want 0; stderr=%s", code, stderr.String())
	}
	if out := stdout.String(); !strings.Contains(out, work.ID) {
		t.Fatalf("stdout = %q, want assigned work %s", out, work.ID)
	}
}

func TestCmdWorkNextFallsBackToRoutedToWhenSelectorAbsent(t *testing.T) {
	store := setupRoutedWorkCommandCity(t)
	work, err := store.Create(beads.Bead{
		Title: "routed task",
		Type:  "task",
		Metadata: map[string]string{
			"gc.routed_to": "worker",
		},
	})
	if err != nil {
		t.Fatalf("create work: %v", err)
	}
	epic, err := store.Create(beads.Bead{
		Title: "parent epic",
		Type:  "epic",
		Metadata: map[string]string{
			"gc.routed_to": "worker",
		},
	})
	if err != nil {
		t.Fatalf("create epic: %v", err)
	}
	t.Setenv("GC_TEMPLATE", "worker")

	var stdout, stderr bytes.Buffer
	code := cmdWorkNext(workCommandOptions{JSON: true}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("cmdWorkNext() = %d, want 0; stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, work.ID) {
		t.Fatalf("stdout = %q, want routed work %s", out, work.ID)
	}
	if strings.Contains(out, epic.ID) {
		t.Fatalf("stdout = %q, should not claim parent epic %s", out, epic.ID)
	}
}

func TestCmdWorkCountFallsBackToRoutedToWhenSelectorAbsent(t *testing.T) {
	store := setupRoutedWorkCommandCity(t)
	if _, err := store.Create(beads.Bead{
		Title: "first routed task",
		Type:  "task",
		Metadata: map[string]string{
			"gc.routed_to": "worker",
		},
	}); err != nil {
		t.Fatalf("create first work: %v", err)
	}
	if _, err := store.Create(beads.Bead{
		Title:    "assigned work",
		Type:     "task",
		Assignee: "worker-session",
	}); err != nil {
		t.Fatalf("create assigned work: %v", err)
	}
	if _, err := store.Create(beads.Bead{
		Title: "other route",
		Type:  "task",
		Metadata: map[string]string{
			"gc.routed_to": "other",
		},
	}); err != nil {
		t.Fatalf("create other work: %v", err)
	}
	t.Setenv("GC_TEMPLATE", "worker")
	t.Setenv("GC_SESSION_NAME", "worker-session")

	var stdout, stderr bytes.Buffer
	code := cmdWorkCount(workCommandOptions{JSON: true}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("cmdWorkCount() = %d, want 0; stderr=%s", code, stderr.String())
	}
	if out := stdout.String(); !strings.Contains(out, `"count":2`) {
		t.Fatalf("stdout = %q, want count 2", out)
	}
}

func TestCmdWorkClaimFallsBackToRoutedToWhenSelectorAbsent(t *testing.T) {
	store := setupRoutedWorkCommandCity(t)
	work, err := store.Create(beads.Bead{
		Title: "claim routed task",
		Type:  "task",
		Metadata: map[string]string{
			"gc.routed_to": "worker",
		},
	})
	if err != nil {
		t.Fatalf("create work: %v", err)
	}
	other, err := store.Create(beads.Bead{
		Title: "other route",
		Type:  "task",
		Metadata: map[string]string{
			"gc.routed_to": "other",
		},
	})
	if err != nil {
		t.Fatalf("create other work: %v", err)
	}
	t.Setenv("GC_TEMPLATE", "worker")
	t.Setenv("GC_SESSION_NAME", "worker-session")

	var stdout, stderr bytes.Buffer
	code := cmdWorkClaim(workCommandOptions{JSON: true, Status: "in_progress"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("cmdWorkClaim() = %d, want 0; stderr=%s", code, stderr.String())
	}
	if out := stdout.String(); !strings.Contains(out, work.ID) {
		t.Fatalf("stdout = %q, want routed work %s", out, work.ID)
	}
	final, err := store.Get(work.ID)
	if err != nil {
		t.Fatalf("get final: %v", err)
	}
	if final.Status != "in_progress" || final.Assignee != "worker-session" {
		t.Fatalf("final status=%q assignee=%q, want in_progress worker-session", final.Status, final.Assignee)
	}
	otherFinal, err := store.Get(other.ID)
	if err != nil {
		t.Fatalf("get other final: %v", err)
	}
	if otherFinal.Status != "open" || otherFinal.Assignee != "" {
		t.Fatalf("other route status=%q assignee=%q, want untouched open/unassigned", otherFinal.Status, otherFinal.Assignee)
	}
}

func TestCmdWorkClaimTypedSelectorIsIdempotentForAssignedInProgressWork(t *testing.T) {
	store := setupTypedWorkCommandCity(t)
	work, err := store.Create(beads.Bead{
		Title:    "already claimed",
		Type:     "task",
		Assignee: "worker-session",
	})
	if err != nil {
		t.Fatalf("create work: %v", err)
	}
	if err := store.Update(work.ID, beads.UpdateOpts{Status: stringPtr("in_progress")}); err != nil {
		t.Fatalf("mark in_progress: %v", err)
	}
	t.Setenv("GC_TEMPLATE", "worker")
	t.Setenv("GC_SESSION_NAME", "worker-session")
	t.Setenv("GC_ALIAS", "worker-alias")

	var stdout, stderr bytes.Buffer
	code := cmdWorkClaim(workCommandOptions{JSON: true, Status: "in_progress"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("cmdWorkClaim() = %d, want 0; stderr=%s", code, stderr.String())
	}
	if out := stdout.String(); !strings.Contains(out, work.ID) {
		t.Fatalf("stdout = %q, want assigned work %s", out, work.ID)
	}
	final, err := store.Get(work.ID)
	if err != nil {
		t.Fatalf("get final: %v", err)
	}
	if final.Status != "in_progress" || final.Assignee != "worker-session" {
		t.Fatalf("final status=%q assignee=%q, want in_progress worker-session", final.Status, final.Assignee)
	}
}

func TestCmdWorkNextTypedSelectorReturnsReadyAssignedStep(t *testing.T) {
	store := setupTypedWorkCommandCity(t)
	work, err := store.Create(beads.Bead{
		Title:    "assigned formula step",
		Type:     "step",
		Assignee: "worker-session",
	})
	if err != nil {
		t.Fatalf("create work: %v", err)
	}
	t.Setenv("GC_TEMPLATE", "worker")
	t.Setenv("GC_SESSION_NAME", "worker-session")

	var stdout, stderr bytes.Buffer
	code := cmdWorkNext(workCommandOptions{JSON: true}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("cmdWorkNext() = %d, want 0; stderr=%s", code, stderr.String())
	}
	if out := stdout.String(); !strings.Contains(out, work.ID) {
		t.Fatalf("stdout = %q, want assigned step %s", out, work.ID)
	}
}

func TestClaimNextWorkSingleWinner(t *testing.T) {
	store := beads.NewMemStore()
	created, err := store.Create(beads.Bead{
		Title: "claim me",
		Type:  "task",
		Metadata: map[string]string{
			"gc.routed_to": "rig/worker",
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	selector := config.WorkSelector{
		Type:       "task",
		Unassigned: true,
		Metadata:   map[string]string{"gc.routed_to": "rig/worker"},
	}

	var wg sync.WaitGroup
	type result struct {
		bead beads.Bead
		err  error
	}
	results := make(chan result, 2)
	for _, assignee := range []string{"worker-1", "worker-2"} {
		wg.Add(1)
		go func(assignee string) {
			defer wg.Done()
			claimed, claimErr := claimNextWork(store, selector, assignee, map[string]string{"claimed_by": assignee})
			results <- result{bead: claimed, err: claimErr}
		}(assignee)
	}
	wg.Wait()
	close(results)

	winners := 0
	losers := 0
	winner := ""
	for res := range results {
		if res.err == nil {
			winners++
			winner = res.bead.Assignee
			if res.bead.Status != "in_progress" {
				t.Fatalf("claimed status = %q, want in_progress", res.bead.Status)
			}
			continue
		}
		if errors.Is(res.err, beads.ErrClaimLost) || errors.Is(res.err, errWorkNotFound) {
			losers++
			continue
		}
		t.Fatalf("claim error = %v, want ErrClaimLost or no work", res.err)
	}
	if winners != 1 || losers != 1 {
		t.Fatalf("winners=%d losers=%d, want 1/1", winners, losers)
	}
	final, err := store.Get(created.ID)
	if err != nil {
		t.Fatalf("get final: %v", err)
	}
	if final.Status != "in_progress" || final.Assignee != winner {
		t.Fatalf("final = status %q assignee %q, want winner %q in_progress", final.Status, final.Assignee, winner)
	}
	if final.Metadata["claimed_by"] != winner {
		t.Fatalf("claimed_by metadata = %q, want winner %q", final.Metadata["claimed_by"], winner)
	}
}

func TestClaimNextWorkAnyUsesUnionOrdering(t *testing.T) {
	store := beads.NewMemStore()
	step, err := store.Create(beads.Bead{
		Title: "first step",
		Type:  "step",
		Metadata: map[string]string{
			"gc.routed_to": "gastown.dog",
		},
	})
	if err != nil {
		t.Fatalf("create step: %v", err)
	}
	task, err := store.Create(beads.Bead{
		Title:  "second warrant",
		Type:   "task",
		Labels: []string{"warrant"},
		Metadata: map[string]string{
			"gc.routed_to":        "gastown.dog",
			"gc.attached_formula": "mol-shutdown-dance",
		},
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	selector := config.WorkSelector{Any: []config.WorkSelector{
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
			Type:       "step",
			Unassigned: true,
			Metadata:   map[string]string{"gc.routed_to": "gastown.dog"},
		},
	}}
	count, err := workSelectorCountForController(store, selector)
	if err != nil {
		t.Fatalf("workSelectorCountForController: %v", err)
	}
	if count != 2 {
		t.Fatalf("workSelectorCountForController = %d, want 2", count)
	}
	claimed, err := claimNextWork(store, selector, "dog-1", nil)
	if err != nil {
		t.Fatalf("claimNextWork: %v", err)
	}
	if claimed.ID != step.ID {
		t.Fatalf("claimed ID = %s, want earliest union member %s; task was %s", claimed.ID, step.ID, task.ID)
	}
}

func TestNormalizeWorkClaimStatus(t *testing.T) {
	for _, raw := range []string{"", "in_progress", "  in_progress  "} {
		got, err := normalizeWorkClaimStatus(raw)
		if err != nil {
			t.Fatalf("normalizeWorkClaimStatus(%q): %v", raw, err)
		}
		if got != "in_progress" {
			t.Fatalf("normalizeWorkClaimStatus(%q) = %q, want in_progress", raw, got)
		}
	}

	if _, err := normalizeWorkClaimStatus("open"); err == nil {
		t.Fatal("normalizeWorkClaimStatus(open) error = nil, want unsupported status error")
	}
}
