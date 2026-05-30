package main

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/doctor"
	"github.com/gastownhall/gascity/internal/routedwork"
	"github.com/gastownhall/gascity/internal/workselect"
)

type v2RoutedToNamespaceCheck struct {
	cfg      *config.City
	cityPath string
	newStore func(string) (beads.Store, error)
}

func newV2RoutedToNamespaceCheck(cfg *config.City, cityPath string, newStore func(string) (beads.Store, error)) *v2RoutedToNamespaceCheck {
	return &v2RoutedToNamespaceCheck{cfg: cfg, cityPath: cityPath, newStore: newStore}
}

type routedWorkDemandContractCheck struct {
	cfg      *config.City
	cityPath string
	newStore func(string) (beads.Store, error)
}

type routedWorkDemandScope struct {
	label string
	path  string
}

type routedWorkClaimCacheEntry struct {
	ids map[string]bool
	err error
}

func newRoutedWorkDemandContractCheck(cfg *config.City, cityPath string, newStore func(string) (beads.Store, error)) *routedWorkDemandContractCheck {
	return &routedWorkDemandContractCheck{cfg: cfg, cityPath: cityPath, newStore: newStore}
}

func (c *routedWorkDemandContractCheck) Name() string { return "routed-work-demand-contract" }

func (c *routedWorkDemandContractCheck) CanFix() bool { return false }

func (c *routedWorkDemandContractCheck) Fix(_ *doctor.CheckContext) error { return nil }

func (c *routedWorkDemandContractCheck) Run(_ *doctor.CheckContext) *doctor.CheckResult {
	if c.cfg == nil {
		return okCheck(c.Name(), "no city config loaded")
	}
	if c.newStore == nil || strings.TrimSpace(c.cityPath) == "" {
		return warnCheck(c.Name(),
			"routed work demand contract check skipped: bead store unavailable",
			"fix bead store access, then rerun gc doctor",
			nil)
	}

	var findings []string
	var skipped []string
	claimCache := map[string]routedWorkClaimCacheEntry{}
	for _, scope := range c.scopes() {
		store, err := c.newStore(scope.path)
		if err != nil {
			skipped = append(skipped, fmt.Sprintf("%s skipped: opening bead store: %v", scope.label, err))
			continue
		}
		items, err := store.List(beads.ListQuery{
			Status:    "open",
			AllowScan: true,
			TierMode:  beads.TierBoth,
			Sort:      beads.SortCreatedAsc,
		})
		if err != nil {
			skipped = append(skipped, fmt.Sprintf("%s skipped: listing open routed beads: %v", scope.label, err))
			continue
		}
		for _, bead := range items {
			if strings.TrimSpace(bead.Metadata[routedwork.RoutedToMetadataKey]) == "" {
				continue
			}
			c.scanDemandBead(scope, store, bead, claimCache, &findings, &skipped)
		}
	}

	details := append([]string{}, findings...)
	details = append(details, skipped...)
	sort.Strings(details)
	if len(findings) == 0 && len(skipped) == 0 {
		return okCheck(c.Name(), "routed work demand contract is satisfied")
	}
	if len(findings) == 0 {
		return warnCheck(c.Name(),
			fmt.Sprintf("routed work demand contract check skipped %d scope(s)", len(skipped)),
			"fix bead store access, then rerun gc doctor",
			details)
	}
	return warnCheck(c.Name(),
		fmt.Sprintf("%d routed work demand contract issue(s) found", len(findings)),
		"create routed work through gc route create or formula-order dispatch, then remove or repair stale incompatible beads",
		details)
}

func (c *routedWorkDemandContractCheck) scopes() []routedWorkDemandScope {
	scopes := []routedWorkDemandScope{{
		label: "city",
		path:  filepath.Clean(c.cityPath),
	}}
	for _, rig := range c.cfg.Rigs {
		if rig.Suspended || strings.TrimSpace(rig.Path) == "" {
			continue
		}
		scopes = append(scopes, routedWorkDemandScope{
			label: "rig " + rig.Name,
			path:  resolveStoreScopeRoot(c.cityPath, rig.Path),
		})
	}
	return scopes
}

func (c *routedWorkDemandContractCheck) scanDemandBead(scope routedWorkDemandScope, store beads.Store, bead beads.Bead, claimCache map[string]routedWorkClaimCacheEntry, findings, skipped *[]string) {
	route := strings.TrimSpace(bead.Metadata[routedwork.RoutedToMetadataKey])
	if route == "" {
		return
	}
	if isFormulaOrderRootMissingPoolDemand(bead) {
		*findings = append(*findings, fmt.Sprintf("missing order demand sentinel: %s bead %s has gc.routed_to=%q and an order-run label but missing %s=%q",
			scope.label, bead.ID, route, routedwork.PoolDemandMetadataKey, routedwork.PoolDemandOrderValue))
	}

	plan, err := routedwork.PlanRoute(c.cfg, route, routedwork.DemandGeneric)
	if err != nil {
		if strings.TrimSpace(bead.Assignee) == "" {
			*findings = append(*findings, fmt.Sprintf("unclaimable routed work: %s bead %s has gc.routed_to=%q but no configured target can claim it: %v",
				scope.label, bead.ID, route, err))
		}
		return
	}
	wantStore := routeCreateStoreRoot(c.cfg, c.cityPath, plan)
	if strings.TrimSpace(wantStore) != "" && !samePath(wantStore, scope.path) {
		*findings = append(*findings, fmt.Sprintf("wrong claim store: %s bead %s has gc.routed_to=%q but target %q claims from %s",
			scope.label, bead.ID, route, plan.Target, routedWorkScopeLabelForPath(c.cfg, c.cityPath, wantStore)))
		return
	}
	if strings.TrimSpace(bead.Assignee) != "" {
		return
	}
	agentCfg, ok := findAgentByQualified(c.cfg, plan.Target)
	if !ok {
		*findings = append(*findings, fmt.Sprintf("unclaimable routed work: %s bead %s has gc.routed_to=%q but resolved target %q is not configured",
			scope.label, bead.ID, route, plan.Target))
		return
	}
	if agentCfg.Suspended {
		*findings = append(*findings, fmt.Sprintf("unclaimable routed work: %s bead %s has gc.routed_to=%q but target %q is suspended",
			scope.label, bead.ID, route, plan.Target))
		return
	}
	if !agentCfg.SupportsGenericEphemeralSessions() {
		*findings = append(*findings, fmt.Sprintf("unclaimable routed work: %s bead %s has gc.routed_to=%q but target %q cannot start generic ephemeral sessions",
			scope.label, bead.ID, route, plan.Target))
		return
	}
	if agentCfg.WorkSelector.IsZero() {
		*findings = append(*findings, fmt.Sprintf("unclaimable routed work: %s bead %s has gc.routed_to=%q but target %q has no work_selector",
			scope.label, bead.ID, route, plan.Target))
		return
	}
	ids, err := c.claimableIDs(scope, store, agentCfg, claimCache)
	if err != nil {
		*skipped = append(*skipped, fmt.Sprintf("%s skipped: evaluating work_selector for %s: %v", scope.label, plan.Target, err))
		return
	}
	if !ids[bead.ID] {
		*findings = append(*findings, fmt.Sprintf("unclaimable routed work: %s bead %s has gc.routed_to=%q but target %q does not match work_selector",
			scope.label, bead.ID, route, plan.Target))
	}
}

func (c *routedWorkDemandContractCheck) claimableIDs(scope routedWorkDemandScope, store beads.Store, agentCfg config.Agent, claimCache map[string]routedWorkClaimCacheEntry) (map[string]bool, error) {
	key := scope.path + "\x00" + agentCfg.QualifiedName()
	if cached, ok := claimCache[key]; ok {
		return cached.ids, cached.err
	}
	selector := expandWorkSelectorTemplates(c.cityPath, loadedCityName(c.cfg, c.cityPath), &agentCfg, c.cfg.Rigs, "work_selector", agentCfg.WorkSelector, io.Discard)
	items, err := workselect.List(store, selector, 0)
	entry := routedWorkClaimCacheEntry{ids: map[string]bool{}, err: err}
	if err == nil {
		for _, item := range items {
			entry.ids[item.ID] = true
		}
	}
	claimCache[key] = entry
	return entry.ids, entry.err
}

func isFormulaOrderRootMissingPoolDemand(bead beads.Bead) bool {
	if strings.TrimSpace(bead.Metadata[routedwork.PoolDemandMetadataKey]) == routedwork.PoolDemandOrderValue {
		return false
	}
	for _, label := range bead.Labels {
		if strings.HasPrefix(label, "order-run:") {
			return true
		}
	}
	return false
}

func routedWorkScopeLabelForPath(cfg *config.City, cityPath, path string) string {
	if samePath(path, cityPath) {
		return "city"
	}
	if cfg != nil {
		for _, rig := range cfg.Rigs {
			if strings.TrimSpace(rig.Path) == "" {
				continue
			}
			if samePath(resolveStoreScopeRoot(cityPath, rig.Path), path) {
				return "rig " + rig.Name
			}
		}
	}
	return path
}

func (c *v2RoutedToNamespaceCheck) Name() string { return "v2-routed-to-namespace" }

func (c *v2RoutedToNamespaceCheck) CanFix() bool { return false }

func (c *v2RoutedToNamespaceCheck) Fix(_ *doctor.CheckContext) error { return nil }

func (c *v2RoutedToNamespaceCheck) Run(_ *doctor.CheckContext) *doctor.CheckResult {
	aliases := boundRoutedToAliases(c.cfg)
	if len(aliases) == 0 {
		return okCheck(c.Name(), "no binding-qualified route targets configured")
	}

	var findings []string
	var skipped []string
	c.scanScope(&findings, &skipped, aliases, "city", c.cityPath)
	if c.cfg != nil {
		for _, rig := range c.cfg.Rigs {
			if rig.Suspended || strings.TrimSpace(rig.Path) == "" {
				continue
			}
			c.scanScope(&findings, &skipped, aliases, "rig "+rig.Name, rig.Path)
		}
	}

	if len(findings) == 0 && len(skipped) == 0 {
		return okCheck(c.Name(), "no short-form gc.routed_to values targeting bound agents found")
	}
	details := append([]string{}, findings...)
	details = append(details, skipped...)
	sort.Strings(details)
	if len(findings) == 0 {
		return warnCheck(c.Name(),
			fmt.Sprintf("v2 routed_to namespace check skipped %d scope(s)", len(skipped)),
			"fix bead store access, then rerun gc doctor",
			details)
	}
	if len(skipped) > 0 {
		return warnCheck(c.Name(),
			fmt.Sprintf("%d short-form gc.routed_to value(s) target bound PackV2 agents; %d scope(s) skipped", len(findings), len(skipped)),
			"rewrite gc.routed_to to the binding-qualified agent name, fix skipped store access, then rerun gc doctor",
			details)
	}
	return warnCheck(c.Name(),
		fmt.Sprintf("%d short-form gc.routed_to value(s) target bound PackV2 agents", len(findings)),
		"rewrite gc.routed_to to the binding-qualified agent name, then rerun gc doctor",
		details)
}

func (c *v2RoutedToNamespaceCheck) scanScope(findings, skipped *[]string, aliases map[string][]string, label, path string) {
	if c.newStore == nil || strings.TrimSpace(path) == "" {
		return
	}
	store, err := c.newStore(path)
	if err != nil {
		*skipped = append(*skipped, fmt.Sprintf("%s skipped: opening bead store: %v", label, err))
		return
	}
	seen := make(map[string]bool)
	routes := make([]string, 0, len(aliases))
	for route := range aliases {
		routes = append(routes, route)
	}
	sort.Strings(routes)
	for _, route := range routes {
		items, err := store.List(beads.ListQuery{
			Metadata: map[string]string{"gc.routed_to": route},
		})
		if err != nil {
			*skipped = append(*skipped, fmt.Sprintf("%s skipped: listing beads: %v", label, err))
			return
		}
		for _, bead := range items {
			if seen[bead.ID] {
				continue
			}
			seen[bead.ID] = true
			c.scanRoutedToBead(findings, aliases, label, bead)
		}
	}
}

func (c *v2RoutedToNamespaceCheck) scanRoutedToBead(findings *[]string, aliases map[string][]string, label string, bead beads.Bead) {
	route := strings.TrimSpace(bead.Metadata["gc.routed_to"])
	if route == "" {
		return
	}
	canonicals, ok := aliases[route]
	if !ok {
		return
	}
	switch len(canonicals) {
	case 1:
		*findings = append(*findings, fmt.Sprintf("%s bead %s has gc.routed_to=%q; use %q", label, bead.ID, route, canonicals[0]))
	default:
		*findings = append(*findings, fmt.Sprintf("%s bead %s has gc.routed_to=%q; use one of %s", label, bead.ID, route, strings.Join(canonicals, ", ")))
	}
}

func boundRoutedToAliases(cfg *config.City) map[string][]string {
	aliases := map[string][]string{}
	if cfg == nil {
		return aliases
	}
	unbound := unboundRoutedToIdentities(cfg)
	addAlias := func(short, canonical string) {
		short = strings.TrimSpace(short)
		canonical = strings.TrimSpace(canonical)
		if short == "" || canonical == "" || short == canonical || unbound[short] {
			return
		}
		aliases[short] = appendUniqueString(aliases[short], canonical)
	}
	for i := range cfg.Agents {
		agent := cfg.Agents[i]
		if strings.TrimSpace(agent.BindingName) == "" {
			continue
		}
		addAlias(unboundRouteIdentity(agent), agent.QualifiedName())
	}
	for i := range cfg.NamedSessions {
		session := cfg.NamedSessions[i]
		if strings.TrimSpace(session.BindingName) == "" {
			continue
		}
		addAlias(unboundNamedSessionRouteIdentity(session), session.QualifiedName())
	}
	for key := range aliases {
		sort.Strings(aliases[key])
	}
	return aliases
}

func unboundRouteIdentity(agent config.Agent) string {
	name := strings.TrimSpace(agent.Name)
	if name == "" {
		return ""
	}
	dir := strings.TrimSpace(agent.Dir)
	if dir == "" {
		return name
	}
	return dir + "/" + name
}

func unboundRoutedToIdentities(cfg *config.City) map[string]bool {
	identities := map[string]bool{}
	for i := range cfg.Agents {
		agent := cfg.Agents[i]
		if strings.TrimSpace(agent.BindingName) != "" {
			continue
		}
		if identity := unboundRouteIdentity(agent); identity != "" {
			identities[identity] = true
		}
	}
	for i := range cfg.NamedSessions {
		session := cfg.NamedSessions[i]
		if strings.TrimSpace(session.BindingName) != "" {
			continue
		}
		if identity := unboundNamedSessionRouteIdentity(session); identity != "" {
			identities[identity] = true
		}
	}
	return identities
}

func unboundNamedSessionRouteIdentity(session config.NamedSession) string {
	name := strings.TrimSpace(session.Name)
	if name == "" {
		name = strings.TrimSpace(session.Template)
	}
	if name == "" {
		return ""
	}
	dir := strings.TrimSpace(session.Dir)
	if dir == "" {
		return name
	}
	return dir + "/" + name
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
