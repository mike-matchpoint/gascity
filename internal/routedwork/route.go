// Package routedwork centralizes routed work metadata, target normalization,
// and route planning shared by CLI, API, and controller demand paths.
package routedwork

import (
	"fmt"
	"strings"

	"github.com/gastownhall/gascity/internal/agentutil"
	"github.com/gastownhall/gascity/internal/config"
)

const (
	// RoutedToMetadataKey stores the normalized target agent or pool.
	RoutedToMetadataKey = "gc.routed_to"
	// PoolDemandMetadataKey marks routed formula-order roots that represent pool demand.
	PoolDemandMetadataKey = "gc.pool_demand"
	// PoolDemandOrderValue is the durable formula-order demand sentinel value.
	PoolDemandOrderValue = "order"
)

// DemandKind identifies the demand contract a route is expected to satisfy.
type DemandKind string

const (
	// DemandGeneric is ordinary routed work claimable by an ephemeral session.
	DemandGeneric DemandKind = "generic"
	// DemandFormulaOrder is routed formula-order root demand.
	DemandFormulaOrder DemandKind = "formula_order"
)

// Scope identifies the bead store scope that a route target claims from.
type Scope string

const (
	// ScopeCity routes to the city bead store.
	ScopeCity Scope = "city"
	// ScopeRig routes to a rig bead store.
	ScopeRig Scope = "rig"
)

// RoutePlan is the resolved routing contract for a user-supplied target.
type RoutePlan struct {
	RequestedTarget       string
	Target                string
	Scope                 Scope
	Rig                   string
	ClaimStoreRef         string
	Demand                DemandKind
	GenericDemandEligible bool
}

// NormalizeTarget collapses pool slot aliases to their shared pool template target.
func NormalizeTarget(cfg *config.City, target string) string {
	target = strings.TrimSpace(target)
	if cfg == nil || target == "" {
		return target
	}
	return agentutil.NormalizePoolRouteTarget(cfg, target)
}

// RouteMetadata returns the metadata required for ordinary routed work.
func RouteMetadata(target string) map[string]string {
	return map[string]string{RoutedToMetadataKey: strings.TrimSpace(target)}
}

// PoolDemandMetadataPair returns the formula-order pool-demand sentinel metadata.
func PoolDemandMetadataPair() map[string]string {
	return map[string]string{PoolDemandMetadataKey: PoolDemandOrderValue}
}

// FormulaOrderPoolDemandMetadata returns route plus formula-order demand metadata.
func FormulaOrderPoolDemandMetadata(target string) map[string]string {
	metadata := RouteMetadata(target)
	metadata[PoolDemandMetadataKey] = PoolDemandOrderValue
	return metadata
}

// PlanRoute resolves a user target to the normalized claim scope and demand plan.
func PlanRoute(cfg *config.City, target string, demand DemandKind) (RoutePlan, error) {
	requested := strings.TrimSpace(target)
	if requested == "" {
		return RoutePlan{}, fmt.Errorf("route target is required")
	}
	if cfg == nil {
		return RoutePlan{}, fmt.Errorf("route target %q cannot be resolved without city config", requested)
	}
	normalized := NormalizeTarget(cfg, requested)
	agentCfg, ok := agentutil.ResolveAgent(cfg, normalized, agentutil.ResolveOpts{AllowPoolMembers: true})
	if !ok {
		return RoutePlan{}, fmt.Errorf("route target %q not found", requested)
	}
	plan := RoutePlan{
		RequestedTarget:       requested,
		Target:                agentCfg.QualifiedName(),
		Scope:                 ScopeCity,
		ClaimStoreRef:         "city",
		Demand:                demand,
		GenericDemandEligible: agentCfg.SupportsGenericEphemeralSessions(),
	}
	if agentCfg.Dir != "" {
		plan.Scope = ScopeRig
		plan.Rig = agentCfg.Dir
		plan.ClaimStoreRef = "rig:" + agentCfg.Dir
	}
	return plan, nil
}
