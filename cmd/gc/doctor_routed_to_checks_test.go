package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/doctor"
	"github.com/gastownhall/gascity/internal/routedwork"
)

func TestV2RoutedToNamespaceCheckWarnsOnShortBoundRoutes(t *testing.T) {
	cityDir := t.TempDir()
	rigDir := t.TempDir()
	cfg := &config.City{
		Agents: []config.Agent{
			{Name: "dog", BindingName: "gastown"},
			{Name: "polecat", Dir: "repo", BindingName: "gastown"},
		},
		Rigs: []config.Rig{
			{Name: "repo", Path: rigDir},
		},
	}
	cityStore := beads.NewMemStoreFrom(0, []beads.Bead{
		{ID: "CITY-1", Title: "warrant", Type: "task", Status: "open", Metadata: map[string]string{"gc.routed_to": "dog"}},
	}, nil)
	rigStore := beads.NewMemStoreFrom(0, []beads.Bead{
		{ID: "RIG-1", Title: "work", Type: "task", Status: "open", Metadata: map[string]string{"gc.routed_to": "repo/polecat"}},
	}, nil)
	stores := map[string]beads.Store{
		cityDir: cityStore,
		rigDir:  rigStore,
	}

	result := newV2RoutedToNamespaceCheck(cfg, cityDir, func(path string) (beads.Store, error) {
		store, ok := stores[path]
		if !ok {
			return nil, fmt.Errorf("unexpected store path %q", path)
		}
		return store, nil
	}).Run(&doctor.CheckContext{})

	if result.Status != doctor.StatusWarning {
		t.Fatalf("status = %v, want warning: %#v", result.Status, result)
	}
	details := strings.Join(result.Details, "\n")
	for _, want := range []string{
		`city bead CITY-1 has gc.routed_to="dog"; use "gastown.dog"`,
		`rig repo bead RIG-1 has gc.routed_to="repo/polecat"; use "repo/gastown.polecat"`,
	} {
		if !strings.Contains(details, want) {
			t.Fatalf("details missing %q:\n%s", want, details)
		}
	}
}

func TestV2RoutedToNamespaceCheckUsesTargetedRouteQueries(t *testing.T) {
	cityDir := t.TempDir()
	cfg := &config.City{
		Agents: []config.Agent{{Name: "dog", BindingName: "gastown"}},
	}
	store := &routeQuerySpyStore{Store: beads.NewMemStoreFrom(0, []beads.Bead{
		{ID: "CITY-1", Title: "warrant", Type: "task", Status: "open", Metadata: map[string]string{"gc.routed_to": "dog"}},
	}, nil)}

	result := newV2RoutedToNamespaceCheck(cfg, cityDir, func(path string) (beads.Store, error) {
		if path != cityDir {
			return nil, fmt.Errorf("unexpected store path %q", path)
		}
		return store, nil
	}).Run(&doctor.CheckContext{})

	if result.Status != doctor.StatusWarning {
		t.Fatalf("status = %v, want warning: %#v", result.Status, result)
	}
	if len(store.queries) == 0 {
		t.Fatal("expected at least one route query")
	}
	for _, query := range store.queries {
		if query.AllowScan {
			t.Fatalf("query %+v used AllowScan; route namespace check should use targeted metadata lookups", query)
		}
		if got := query.Metadata["gc.routed_to"]; got == "" {
			t.Fatalf("query %+v missing gc.routed_to metadata filter", query)
		}
	}
}

func TestV2RoutedToNamespaceCheckAllowsCanonicalRoutes(t *testing.T) {
	cityDir := t.TempDir()
	cfg := &config.City{
		Agents: []config.Agent{
			{Name: "dog", BindingName: "gastown"},
			{Name: "human"},
		},
	}
	cityStore := beads.NewMemStoreFrom(0, []beads.Bead{
		{ID: "CITY-1", Title: "warrant", Type: "task", Status: "open", Metadata: map[string]string{"gc.routed_to": "gastown.dog"}},
		{ID: "CITY-2", Title: "human", Type: "task", Status: "open", Metadata: map[string]string{"gc.routed_to": "human"}},
	}, nil)

	result := newV2RoutedToNamespaceCheck(cfg, cityDir, func(path string) (beads.Store, error) {
		if path != cityDir {
			return nil, fmt.Errorf("unexpected store path %q", path)
		}
		return cityStore, nil
	}).Run(&doctor.CheckContext{})

	if result.Status != doctor.StatusOK {
		t.Fatalf("status = %v, want ok: %#v", result.Status, result)
	}
}

func TestV2RoutedToNamespaceCheckWarnsOnBoundNamedSessionShortRoutes(t *testing.T) {
	cityDir := t.TempDir()
	cfg := &config.City{
		NamedSessions: []config.NamedSession{
			{Name: "mayor", BindingName: "gastown"},
		},
	}
	cityStore := beads.NewMemStoreFrom(0, []beads.Bead{
		{ID: "CITY-1", Title: "mail", Type: "task", Status: "open", Metadata: map[string]string{"gc.routed_to": "mayor"}},
	}, nil)

	result := newV2RoutedToNamespaceCheck(cfg, cityDir, func(path string) (beads.Store, error) {
		if path != cityDir {
			return nil, fmt.Errorf("unexpected store path %q", path)
		}
		return cityStore, nil
	}).Run(&doctor.CheckContext{})

	if result.Status != doctor.StatusWarning {
		t.Fatalf("status = %v, want warning: %#v", result.Status, result)
	}
	details := strings.Join(result.Details, "\n")
	want := `city bead CITY-1 has gc.routed_to="mayor"; use "gastown.mayor"`
	if !strings.Contains(details, want) {
		t.Fatalf("details missing %q:\n%s", want, details)
	}
}

func TestV2RoutedToNamespaceCheckAllowsAmbiguousShortRouteForUnboundAgent(t *testing.T) {
	cityDir := t.TempDir()
	cfg := &config.City{
		Agents: []config.Agent{
			{Name: "dog"},
			{Name: "dog", BindingName: "gastown"},
		},
	}
	cityStore := beads.NewMemStoreFrom(0, []beads.Bead{
		{ID: "CITY-1", Title: "warrant", Type: "task", Status: "open", Metadata: map[string]string{"gc.routed_to": "dog"}},
	}, nil)

	result := newV2RoutedToNamespaceCheck(cfg, cityDir, func(path string) (beads.Store, error) {
		if path != cityDir {
			return nil, fmt.Errorf("unexpected store path %q", path)
		}
		return cityStore, nil
	}).Run(&doctor.CheckContext{})

	if result.Status != doctor.StatusOK {
		t.Fatalf("status = %v, want ok: %#v", result.Status, result)
	}
}

func TestV2RoutedToNamespaceCheckWarnsOnSkippedStoreScopes(t *testing.T) {
	cityDir := t.TempDir()
	rigDir := t.TempDir()
	cfg := &config.City{
		Agents: []config.Agent{
			{Name: "dog", BindingName: "gastown"},
		},
		Rigs: []config.Rig{
			{Name: "repo", Path: rigDir},
		},
	}

	result := newV2RoutedToNamespaceCheck(cfg, cityDir, func(path string) (beads.Store, error) {
		switch path {
		case cityDir:
			return nil, errors.New("city offline")
		case rigDir:
			return routeListErrorStore{err: errors.New("rig offline")}, nil
		default:
			return nil, fmt.Errorf("unexpected store path %q", path)
		}
	}).Run(&doctor.CheckContext{})

	if result.Status != doctor.StatusWarning {
		t.Fatalf("status = %v, want warning: %#v", result.Status, result)
	}
	details := strings.Join(result.Details, "\n")
	for _, want := range []string{
		"city skipped: opening bead store: city offline",
		"rig repo skipped: listing beads: rig offline",
	} {
		if !strings.Contains(details, want) {
			t.Fatalf("details missing %q:\n%s", want, details)
		}
	}
}

type routeListErrorStore struct {
	beads.Store
	err error
}

func (s routeListErrorStore) List(beads.ListQuery) ([]beads.Bead, error) {
	return nil, s.err
}

type routeQuerySpyStore struct {
	beads.Store
	queries []beads.ListQuery
}

func (s *routeQuerySpyStore) List(query beads.ListQuery) ([]beads.Bead, error) {
	s.queries = append(s.queries, query)
	return s.Store.List(query)
}

func TestDoctorRoutedWorkDemandContractWarnsOnUnclaimableRoutedWork(t *testing.T) {
	cityDir := t.TempDir()
	cfg := &config.City{
		Workspace: config.Workspace{Name: "test-city"},
		Agents: []config.Agent{{
			Name:              "dog",
			BindingName:       "gastown",
			MaxActiveSessions: intPtr(3),
			WorkSelector: config.WorkSelector{Any: []config.WorkSelector{
				{Type: "step", Unassigned: true, Metadata: map[string]string{routedwork.RoutedToMetadataKey: "gastown.dog"}},
				{Type: "task", Label: "warrant", Unassigned: true, Metadata: map[string]string{
					routedwork.RoutedToMetadataKey:        "gastown.dog",
					routedwork.AttachedFormulaMetadataKey: "mol-shutdown-dance",
				}},
			}},
		}},
	}
	store := beads.NewMemStoreFrom(0, []beads.Bead{{
		ID:       "CITY-RAW",
		Title:    "raw warrant",
		Type:     "task",
		Status:   "open",
		Labels:   []string{"warrant"},
		Metadata: map[string]string{routedwork.RoutedToMetadataKey: "gastown.dog"},
	}}, nil)

	result := newRoutedWorkDemandContractCheck(cfg, cityDir, func(path string) (beads.Store, error) {
		if path != cityDir {
			return nil, fmt.Errorf("unexpected store path %q", path)
		}
		return store, nil
	}).Run(&doctor.CheckContext{})

	if result.Status != doctor.StatusWarning {
		t.Fatalf("status = %v, want warning: %#v", result.Status, result)
	}
	details := strings.Join(result.Details, "\n")
	for _, want := range []string{
		"unclaimable routed work",
		"CITY-RAW",
		`gc.routed_to="gastown.dog"`,
		"does not match work_selector",
	} {
		if !strings.Contains(details, want) {
			t.Fatalf("details missing %q:\n%s", want, details)
		}
	}
}

func TestDoctorRoutedWorkDemandContractWarnsOnWrongClaimStore(t *testing.T) {
	cityDir := t.TempDir()
	rigDir := t.TempDir()
	cfg := &config.City{
		Workspace: config.Workspace{Name: "test-city"},
		Rigs:      []config.Rig{{Name: "frontend", Path: rigDir}},
		Agents: []config.Agent{{
			Name:              "dog",
			BindingName:       "gastown",
			MaxActiveSessions: intPtr(3),
			WorkSelector: config.WorkSelector{
				Type:       "task",
				Unassigned: true,
				Metadata:   map[string]string{routedwork.RoutedToMetadataKey: "gastown.dog"},
			},
		}},
	}
	cityStore := beads.NewMemStore()
	rigStore := beads.NewMemStoreFrom(0, []beads.Bead{{
		ID:       "RIG-WRONG",
		Title:    "wrong store",
		Type:     "task",
		Status:   "open",
		Metadata: map[string]string{routedwork.RoutedToMetadataKey: "gastown.dog"},
	}}, nil)
	stores := map[string]beads.Store{cityDir: cityStore, rigDir: rigStore}

	result := newRoutedWorkDemandContractCheck(cfg, cityDir, func(path string) (beads.Store, error) {
		store, ok := stores[path]
		if !ok {
			return nil, fmt.Errorf("unexpected store path %q", path)
		}
		return store, nil
	}).Run(&doctor.CheckContext{})

	if result.Status != doctor.StatusWarning {
		t.Fatalf("status = %v, want warning: %#v", result.Status, result)
	}
	details := strings.Join(result.Details, "\n")
	for _, want := range []string{
		"wrong claim store",
		"RIG-WRONG",
		"rig frontend",
		"city",
	} {
		if !strings.Contains(details, want) {
			t.Fatalf("details missing %q:\n%s", want, details)
		}
	}
}

func TestDoctorRoutedWorkDemandContractWarnsOnOrderRootMissingSentinel(t *testing.T) {
	cityDir := t.TempDir()
	cfg := &config.City{
		Workspace: config.Workspace{Name: "test-city"},
		Agents: []config.Agent{{
			Name:              "dog",
			BindingName:       "gastown",
			MaxActiveSessions: intPtr(3),
			WorkSelector: config.WorkSelector{
				Type:       "molecule",
				Unassigned: true,
				Tier:       "both",
				Metadata: map[string]string{
					routedwork.RoutedToMetadataKey: "gastown.dog",
				},
			},
		}},
	}
	store := beads.NewMemStoreFrom(0, []beads.Bead{{
		ID:        "MOL-ORDER",
		Title:     "order root",
		Type:      "molecule",
		Status:    "open",
		Labels:    []string{"order-run:nightly"},
		Ephemeral: true,
		Metadata:  map[string]string{routedwork.RoutedToMetadataKey: "gastown.dog"},
	}}, nil)

	result := newRoutedWorkDemandContractCheck(cfg, cityDir, func(path string) (beads.Store, error) {
		if path != cityDir {
			return nil, fmt.Errorf("unexpected store path %q", path)
		}
		return store, nil
	}).Run(&doctor.CheckContext{})

	if result.Status != doctor.StatusWarning {
		t.Fatalf("status = %v, want warning: %#v", result.Status, result)
	}
	details := strings.Join(result.Details, "\n")
	for _, want := range []string{
		"missing order demand sentinel",
		"MOL-ORDER",
		`gc.pool_demand="order"`,
	} {
		if !strings.Contains(details, want) {
			t.Fatalf("details missing %q:\n%s", want, details)
		}
	}
}

func TestDoctorRoutedWorkDemandContractAllowsFormulaBackedWarrant(t *testing.T) {
	cityDir := t.TempDir()
	cfg := &config.City{
		Workspace: config.Workspace{Name: "test-city"},
		Agents: []config.Agent{{
			Name:              "dog",
			BindingName:       "gastown",
			MaxActiveSessions: intPtr(3),
			WorkSelector: config.WorkSelector{Any: []config.WorkSelector{
				{Type: "step", Unassigned: true, Metadata: map[string]string{routedwork.RoutedToMetadataKey: "gastown.dog"}},
				{Type: "task", Label: "warrant", Unassigned: true, Metadata: map[string]string{
					routedwork.RoutedToMetadataKey:        "gastown.dog",
					routedwork.AttachedFormulaMetadataKey: "mol-shutdown-dance",
				}},
			}},
		}},
	}
	store := beads.NewMemStoreFrom(0, []beads.Bead{{
		ID:     "CITY-WARRANT",
		Title:  "formula warrant",
		Type:   "task",
		Status: "open",
		Labels: []string{"warrant"},
		Metadata: map[string]string{
			routedwork.RoutedToMetadataKey:        "gastown.dog",
			routedwork.AttachedFormulaMetadataKey: "mol-shutdown-dance",
		},
	}}, nil)

	result := newRoutedWorkDemandContractCheck(cfg, cityDir, func(path string) (beads.Store, error) {
		if path != cityDir {
			return nil, fmt.Errorf("unexpected store path %q", path)
		}
		return store, nil
	}).Run(&doctor.CheckContext{})

	if result.Status != doctor.StatusOK {
		t.Fatalf("status = %v, want ok: %#v", result.Status, result)
	}
}

func TestDoctorJSONIncludesRoutedWorkDemandContractKey(t *testing.T) {
	var out bytes.Buffer
	report := &doctor.Report{
		Passed: 1,
		Results: []*doctor.CheckResult{{
			Name:    newRoutedWorkDemandContractCheck(&config.City{}, t.TempDir(), nil).Name(),
			Status:  doctor.StatusOK,
			Message: "ok",
		}},
	}
	if err := writeDoctorJSON(&out, report); err != nil {
		t.Fatalf("writeDoctorJSON: %v", err)
	}
	var decoded doctorJSONReport
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &decoded); err != nil {
		t.Fatalf("decode doctor json: %v\n%s", err, out.String())
	}
	if len(decoded.Results) != 1 || decoded.Results[0].Name != "routed-work-demand-contract" {
		t.Fatalf("doctor JSON results = %#v, want routed-work-demand-contract key", decoded.Results)
	}
}
