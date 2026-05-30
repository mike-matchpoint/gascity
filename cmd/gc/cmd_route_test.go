package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/routedwork"
	"github.com/gastownhall/gascity/internal/runtime"
)

func TestRouteCreateOnFormulaCreatesWarrantAndAttachesShutdownDance(t *testing.T) {
	dir := testFormulaDir(t)
	writeShutdownDanceRequiredVarsFormula(t, dir)
	cfg := &config.City{
		Workspace:     config.Workspace{Name: "test-city"},
		FormulaLayers: config.FormulaLayers{City: []string{dir}},
		Agents: []config.Agent{
			{Name: "dog", BindingName: "gastown", MaxActiveSessions: intPtr(3)},
		},
	}
	store := beads.NewMemStore()
	deps := routeCreateDeps{
		Sling: slingDeps{
			CityName: "test-city",
			CityPath: sharedTestCityDir,
			Cfg:      cfg,
			SP:       runtime.NewFake(),
			Runner:   newFakeRunner().run,
			Store:    store,
			StoreRef: "city:test-city",
		},
	}
	var stdout, stderr bytes.Buffer

	code := doRouteCreate(routeCreateOptions{
		Target: "gastown.dog",
		On:     "mol-shutdown-dance",
		Type:   "task",
		Labels: []string{"warrant"},
		Title:  "Stuck: gastown.deacon",
		Metadata: []string{
			"target=gastown.deacon",
			"reason=stale patrol",
			"requester=deacon",
		},
	}, deps, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doRouteCreate returned %d, want 0; stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	warrants, err := store.List(beads.ListQuery{Label: "warrant"})
	if err != nil {
		t.Fatalf("list warrants: %v", err)
	}
	if len(warrants) != 1 {
		t.Fatalf("warrant count = %d, want 1: %#v", len(warrants), warrants)
	}
	warrant := warrants[0]
	for key, want := range map[string]string{
		"target":                              "gastown.deacon",
		"reason":                              "stale patrol",
		"requester":                           "deacon",
		routedwork.RoutedToMetadataKey:        "gastown.dog",
		routedwork.AttachedFormulaMetadataKey: "mol-shutdown-dance",
		routeCreateResolvedTargetMetadataKey:  "gastown.dog",
		routeCreateRequestedTargetMetadataKey: "gastown.dog",
		routeCreateClaimStoreRefMetadataKey:   "city:test-city",
	} {
		if got := warrant.Metadata[key]; got != want {
			t.Fatalf("warrant metadata %s = %q, want %q; metadata=%v", key, got, want, warrant.Metadata)
		}
	}
	rootID := warrant.Metadata["molecule_id"]
	if rootID == "" {
		t.Fatal("warrant missing molecule_id after formula attachment")
	}
	children, err := store.List(beads.ListQuery{ParentID: rootID})
	if err != nil {
		t.Fatalf("list attached children: %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("attached children = %d, want 1: %#v", len(children), children)
	}
	for key, want := range map[string]string{
		"warrant_id": warrant.ID,
		"target":     "gastown.deacon",
		"reason":     "stale patrol",
		"requester":  "deacon",
	} {
		if got := children[0].Metadata[key]; got != want {
			t.Fatalf("attached child metadata %s = %q, want %q; metadata=%v", key, got, want, children[0].Metadata)
		}
	}
}

func TestRouteCreateUnknownTargetDoesNotCreateBead(t *testing.T) {
	cfg := &config.City{
		Workspace: config.Workspace{Name: "test-city"},
		Agents:    []config.Agent{{Name: "dog", BindingName: "gastown", MaxActiveSessions: intPtr(3)}},
	}
	store := beads.NewMemStore()
	deps := routeCreateDeps{Sling: slingDeps{
		CityName: "test-city",
		CityPath: sharedTestCityDir,
		Cfg:      cfg,
		SP:       runtime.NewFake(),
		Runner:   newFakeRunner().run,
		Store:    store,
		StoreRef: "city:test-city",
	}}
	var stdout, stderr bytes.Buffer

	code := doRouteCreate(routeCreateOptions{
		Target: "ghost.dog",
		On:     "mol-shutdown-dance",
		Type:   "task",
		Labels: []string{"warrant"},
		Title:  "Stuck: ghost",
		Metadata: []string{
			"target=ghost",
			"reason=missing",
			"requester=test",
		},
	}, deps, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("doRouteCreate returned 0, want failure; stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "route target") {
		t.Fatalf("stderr = %q, want route target error", stderr.String())
	}
	beads, err := store.List(beads.ListQuery{AllowScan: true})
	if err != nil {
		t.Fatalf("list beads: %v", err)
	}
	if len(beads) != 0 {
		t.Fatalf("created %d beads on unknown target: %#v", len(beads), beads)
	}
}

func TestRouteCreatePrevalidatesRequiredFormulaVarsBeforeCreate(t *testing.T) {
	dir := testFormulaDir(t)
	writeShutdownDanceRequiredVarsFormula(t, dir)
	cfg := &config.City{
		Workspace:     config.Workspace{Name: "test-city"},
		FormulaLayers: config.FormulaLayers{City: []string{dir}},
		Agents:        []config.Agent{{Name: "dog", BindingName: "gastown", MaxActiveSessions: intPtr(3)}},
	}
	store := beads.NewMemStore()
	deps := routeCreateDeps{Sling: slingDeps{
		CityName: "test-city",
		CityPath: sharedTestCityDir,
		Cfg:      cfg,
		SP:       runtime.NewFake(),
		Runner:   newFakeRunner().run,
		Store:    store,
		StoreRef: "city:test-city",
	}}
	var stdout, stderr bytes.Buffer

	code := doRouteCreate(routeCreateOptions{
		Target: "gastown.dog",
		On:     "mol-shutdown-dance",
		Type:   "task",
		Labels: []string{"warrant"},
		Title:  "Stuck: gastown.deacon",
		Metadata: []string{
			"target=gastown.deacon",
			"requester=deacon",
		},
	}, deps, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("doRouteCreate returned 0, want failure; stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), `variable "reason" is required`) {
		t.Fatalf("stderr = %q, want missing reason validation", stderr.String())
	}
	beads, err := store.List(beads.ListQuery{AllowScan: true})
	if err != nil {
		t.Fatalf("list beads: %v", err)
	}
	if len(beads) != 0 {
		t.Fatalf("created %d beads despite failed prevalidation: %#v", len(beads), beads)
	}
}

func TestRouteCreateStoreRootUsesTargetClaimStore(t *testing.T) {
	cityDir := t.TempDir()
	cfg := &config.City{
		Workspace: config.Workspace{Name: "test-city"},
		Rigs: []config.Rig{{
			Name: "frontend",
			Path: "frontend",
		}},
		Agents: []config.Agent{
			{Name: "dog", BindingName: "gastown", MaxActiveSessions: intPtr(3)},
			{Name: "polecat", Dir: "frontend", MaxActiveSessions: intPtr(3)},
		},
	}
	resolveRigPaths(cityDir, cfg.Rigs)

	cityPlan, err := routedwork.PlanRoute(cfg, "gastown.dog", routedwork.DemandGeneric)
	if err != nil {
		t.Fatalf("PlanRoute city target: %v", err)
	}
	if got := routeCreateStoreRoot(cfg, cityDir, cityPlan); got != filepath.Clean(cityDir) {
		t.Fatalf("city target store root = %q, want %q", got, filepath.Clean(cityDir))
	}

	rigPlan, err := routedwork.PlanRoute(cfg, "frontend/polecat", routedwork.DemandGeneric)
	if err != nil {
		t.Fatalf("PlanRoute rig target: %v", err)
	}
	wantRig := filepath.Join(cityDir, "frontend")
	if got := routeCreateStoreRoot(cfg, cityDir, rigPlan); got != filepath.Clean(wantRig) {
		t.Fatalf("rig target store root = %q, want %q", got, filepath.Clean(wantRig))
	}
}
