package routedwork

import (
	"testing"

	"github.com/gastownhall/gascity/internal/config"
)

func TestRoutePlanNormalizesPoolSlotAndResolvesClaimStore(t *testing.T) {
	cfg := &config.City{Agents: []config.Agent{
		{Name: "polecat", Dir: "repo", MaxActiveSessions: intPtr(3)},
		{Name: "dog", BindingName: "gastown", MaxActiveSessions: intPtr(3)},
	}}

	plan, err := PlanRoute(cfg, "repo/polecat-2", DemandGeneric)
	if err != nil {
		t.Fatalf("PlanRoute: %v", err)
	}
	if plan.Target != "repo/polecat" {
		t.Fatalf("Target = %q, want normalized repo/polecat", plan.Target)
	}
	if plan.Scope != ScopeRig || plan.Rig != "repo" || plan.ClaimStoreRef != "rig:repo" {
		t.Fatalf("scope = (%q, %q, %q), want rig/repo/rig:repo", plan.Scope, plan.Rig, plan.ClaimStoreRef)
	}
	if !plan.GenericDemandEligible {
		t.Fatal("GenericDemandEligible = false, want true for multi-session pool")
	}

	meta := RouteMetadata(plan.Target)
	if got := meta[RoutedToMetadataKey]; got != "repo/polecat" {
		t.Fatalf("RouteMetadata gc.routed_to = %q", got)
	}
}

func TestFormulaOrderPoolDemandMetadata(t *testing.T) {
	got := FormulaOrderPoolDemandMetadata("gastown.dog")
	if got[RoutedToMetadataKey] != "gastown.dog" {
		t.Fatalf("gc.routed_to = %q", got[RoutedToMetadataKey])
	}
	if got[PoolDemandMetadataKey] != PoolDemandOrderValue {
		t.Fatalf("gc.pool_demand = %q, want %q", got[PoolDemandMetadataKey], PoolDemandOrderValue)
	}
}

func TestRoutePlanRejectsUnknownTarget(t *testing.T) {
	cfg := &config.City{Agents: []config.Agent{{Name: "dog", BindingName: "gastown", MaxActiveSessions: intPtr(3)}}}
	if _, err := PlanRoute(cfg, "ghost", DemandGeneric); err == nil {
		t.Fatal("PlanRoute unknown target error = nil")
	}
}

func TestRoutePlanMaxZeroNotGenericDemandEligible(t *testing.T) {
	zero := 0
	cfg := &config.City{Agents: []config.Agent{{Name: "parked", MaxActiveSessions: &zero}}}
	plan, err := PlanRoute(cfg, "parked", DemandGeneric)
	if err != nil {
		t.Fatalf("PlanRoute parked: %v", err)
	}
	if plan.GenericDemandEligible {
		t.Fatal("GenericDemandEligible = true, want false for max_active_sessions=0")
	}
}

func intPtr(v int) *int {
	return &v
}
