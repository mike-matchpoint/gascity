package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/events"
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

func TestRouteCreateJSONSchemaDeclared(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"route", "create", "--json-schema=result"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(route create --json-schema=result) = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if !json.Valid(stdout.Bytes()) {
		t.Fatalf("stdout is not valid JSON schema: %s", stdout.String())
	}
}

func TestRouteCreateJSONOutputValidatesSchema(t *testing.T) {
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
		JSON: true,
	}, deps, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doRouteCreate returned %d, want 0; stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	validateJSONAgainstResultSchema(t, []string{"route", "create"}, stdout.Bytes())

	var payload routeCreateJSONResult
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode route create JSON: %v\n%s", err, stdout.String())
	}
	if !payload.OK || payload.ID == "" || payload.Target != "gastown.dog" || payload.Formula != "mol-shutdown-dance" {
		t.Fatalf("payload = %#v, want routed formula-backed source evidence", payload)
	}
}

func TestRouteCreateEmitsFormulaBackedEvents(t *testing.T) {
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
	rec := events.NewFake()
	deps := routeCreateDeps{
		Rec: rec,
		Sling: slingDeps{
			CityName:   "test-city",
			CityPath:   sharedTestCityDir,
			Cfg:        cfg,
			SP:         runtime.NewFake(),
			Runner:     newFakeRunner().run,
			Store:      store,
			StoreRef:   "city:test-city",
			Recorder:   rec,
			EventActor: "test",
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

	wantTypes := []string{
		events.RouteCreateSourceCreated,
		events.SlingRouted,
		events.SlingFormulaAttached,
		events.RouteCreateFormulaAttached,
		events.RouteCreateRouted,
	}
	for _, want := range wantTypes {
		if !fakeEventsContainType(rec, want) {
			t.Fatalf("missing event type %q; events=%v", want, fakeEventTypes(rec))
		}
	}
	ev, ok := firstFakeEventOfType(rec, events.RouteCreateRouted)
	if !ok {
		t.Fatal("missing route create routed event")
	}
	var payload events.RouteWorkEventPayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v; raw=%s", err, ev.Payload)
	}
	if payload.BeadID == "" || payload.Target != "gastown.dog" || payload.Formula != "mol-shutdown-dance" || payload.Method != "on-formula" {
		t.Fatalf("payload = %#v, want bead/target/formula/method evidence", payload)
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

func TestRouteCreateValidationFailureEmitsEventBeforeCreate(t *testing.T) {
	dir := testFormulaDir(t)
	writeShutdownDanceRequiredVarsFormula(t, dir)
	cfg := &config.City{
		Workspace:     config.Workspace{Name: "test-city"},
		FormulaLayers: config.FormulaLayers{City: []string{dir}},
		Agents:        []config.Agent{{Name: "dog", BindingName: "gastown", MaxActiveSessions: intPtr(3)}},
	}
	store := beads.NewMemStore()
	rec := events.NewFake()
	deps := routeCreateDeps{Rec: rec, Sling: slingDeps{
		CityName:   "test-city",
		CityPath:   sharedTestCityDir,
		Cfg:        cfg,
		SP:         runtime.NewFake(),
		Runner:     newFakeRunner().run,
		Store:      store,
		StoreRef:   "city:test-city",
		Recorder:   rec,
		EventActor: "test",
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
	if fakeEventsContainType(rec, events.RouteCreateSourceCreated) {
		t.Fatalf("source-created event emitted despite validation failure: %v", fakeEventTypes(rec))
	}
	ev, ok := firstFakeEventOfType(rec, events.RouteCreateValidationFailed)
	if !ok {
		t.Fatalf("missing validation failed event; events=%v", fakeEventTypes(rec))
	}
	var payload events.RouteWorkEventPayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v; raw=%s", err, ev.Payload)
	}
	if payload.ErrorCode != "formula_validation_failed" || !strings.Contains(payload.ErrorMessage, `variable "reason" is required`) {
		t.Fatalf("payload = %#v, want missing reason validation evidence", payload)
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

func fakeEventTypes(rec *events.Fake) []string {
	types := make([]string, 0, len(rec.Events))
	for _, event := range rec.Events {
		types = append(types, event.Type)
	}
	return types
}

func fakeEventsContainType(rec *events.Fake, eventType string) bool {
	_, ok := firstFakeEventOfType(rec, eventType)
	return ok
}

func firstFakeEventOfType(rec *events.Fake, eventType string) (events.Event, bool) {
	for _, event := range rec.Events {
		if event.Type == eventType {
			return event, true
		}
	}
	return events.Event{}, false
}
