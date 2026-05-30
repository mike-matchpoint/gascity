package main

import (
	"context"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/formula"
	"github.com/gastownhall/gascity/internal/molecule"
	"github.com/gastownhall/gascity/internal/routedwork"
	"github.com/gastownhall/gascity/internal/sling"
)

const (
	routeCreateRequestedTargetMetadataKey = "gc.route_requested_target"
	routeCreateResolvedTargetMetadataKey  = "gc.route_target"
	routeCreateClaimStoreRefMetadataKey   = "gc.route_claim_store"
)

type routeCreateOptions struct {
	Target      string
	On          string
	Type        string
	Labels      []string
	Title       string
	Description string
	Metadata    []string
	Vars        []string
	JSON        bool
	DryRun      bool
}

type routeCreateDeps struct {
	Sling slingDeps
}

type routeCreateJSONResult struct {
	SchemaVersion string            `json:"schema_version"`
	OK            bool              `json:"ok"`
	ID            string            `json:"id,omitempty"`
	Target        string            `json:"target,omitempty"`
	Formula       string            `json:"formula,omitempty"`
	Method        string            `json:"method,omitempty"`
	WispRootID    string            `json:"wisp_root_id,omitempty"`
	WorkflowID    string            `json:"workflow_id,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	DryRun        bool              `json:"dry_run,omitempty"`
}

func newRouteCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "route",
		Short: "Create typed routed work",
	}
	cmd.AddCommand(newRouteCreateCmd(stdout, stderr))
	return cmd
}

func newRouteCreateCmd(stdout, stderr io.Writer) *cobra.Command {
	var opts routeCreateOptions
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create formula-backed routed work",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if code := cmdRouteCreate(opts, stdout, stderr); code != 0 {
				return errExit
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.Target, "target", "", "pool or agent target to route work to")
	cmd.Flags().StringVar(&opts.On, "on", "", "formula to attach to the created source bead")
	cmd.Flags().StringVar(&opts.Type, "type", "task", "created bead type")
	cmd.Flags().StringArrayVar(&opts.Labels, "label", nil, "created bead label (repeatable)")
	cmd.Flags().StringVar(&opts.Title, "title", "", "created bead title")
	cmd.Flags().StringVar(&opts.Description, "description", "", "created bead description")
	cmd.Flags().StringArrayVar(&opts.Metadata, "metadata", nil, "created bead metadata key=value (repeatable)")
	cmd.Flags().StringArrayVar(&opts.Vars, "var", nil, "formula variable key=value (repeatable)")
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "print JSON result")
	cmd.Flags().BoolVarP(&opts.DryRun, "dry-run", "n", false, "show what would be created without mutating")
	_ = cmd.MarkFlagRequired("target")
	_ = cmd.MarkFlagRequired("on")
	_ = cmd.MarkFlagRequired("title")
	return cmd
}

func cmdRouteCreate(opts routeCreateOptions, stdout, stderr io.Writer) int {
	fail := func(code, message string) int {
		if opts.JSON {
			return writeJSONError(stdout, stderr, code, message, 1)
		}
		fmt.Fprintln(stderr, message) //nolint:errcheck
		return 1
	}

	cityPath, err := resolveCity()
	if err != nil {
		return fail("city_resolve_failed", fmt.Sprintf("gc route create: %v", err))
	}
	cfg, prov, err := loadSlingCityConfig(cityPath)
	if err != nil {
		return fail("config_load_failed", fmt.Sprintf("gc route create: %v", err))
	}
	emitLoadCityConfigWarnings(stderr, prov)
	applyFeatureFlags(cfg)
	resolveRigPaths(cityPath, cfg.Rigs)

	plan, err := routedwork.PlanRoute(cfg, opts.Target, routedwork.DemandGeneric)
	if err != nil {
		return fail("target_resolve_failed", fmt.Sprintf("gc route create: %v", err))
	}
	storeDir := routeCreateStoreRoot(cfg, cityPath, plan)
	store, err := openStoreAtForCity(storeDir, cityPath)
	if err != nil {
		return fail("store_open_failed", fmt.Sprintf("gc route create: opening store %s: %v", storeDir, err))
	}
	cityName := loadedCityName(cfg, cityPath)
	storeRef := workflowStoreRefForDir(storeDir, cityPath, cityName, cfg)
	storeEnv, err := slingStoreEnvWithError(cfg, cityPath, storeDir)
	if err != nil {
		return fail("store_env_failed", fmt.Sprintf("gc route create: building store env: %v", err))
	}
	runner := SlingRunner(shellSlingRunner)
	if len(storeEnv) > 0 {
		pinnedEnv := maps.Clone(storeEnv)
		runner = func(dir, command string, env map[string]string) (string, error) {
			merged := maps.Clone(pinnedEnv)
			for k, v := range env {
				merged[k] = v
			}
			return shellSlingRunner(dir, command, merged)
		}
	}
	deps := routeCreateDeps{Sling: slingDeps{
		CityName: cityName,
		CityPath: cityPath,
		Cfg:      cfg,
		SP:       newSessionProvider(),
		Runner:   runner,
		Store:    store,
		StoreRef: storeRef,
		SourceWorkflowStores: func() ([]sling.SourceWorkflowStore, error) {
			stores, skips, err := openSourceWorkflowStores(cfg, cityPath, "")
			if err != nil {
				return nil, err
			}
			if len(skips) > 0 {
				fmt.Fprintln(stderr, "warning:", formatSourceWorkflowStoreSkips(skips)) //nolint:errcheck
			}
			out := make([]sling.SourceWorkflowStore, 0, len(stores))
			for _, storeView := range stores {
				out = append(out, sling.SourceWorkflowStore{
					Store:    storeView.store,
					StoreRef: workflowStoreRefForDir(storeView.path, cityPath, cityName, cfg),
				})
			}
			return out, nil
		},
	}}
	return doRouteCreate(opts, deps, stdout, stderr)
}

func doRouteCreate(opts routeCreateOptions, deps routeCreateDeps, stdout, stderr io.Writer) int {
	if err := validateRouteCreateOptions(opts); err != nil {
		return routeCreateError(opts, stdout, stderr, "invalid_arguments", err)
	}
	if deps.Sling.Cfg == nil {
		return routeCreateError(opts, stdout, stderr, "config_missing", fmt.Errorf("city config is required"))
	}
	if deps.Sling.Store == nil {
		return routeCreateError(opts, stdout, stderr, "store_missing", fmt.Errorf("bead store is required"))
	}
	populateSlingDepsCallbacks(&deps.Sling)

	plan, err := routedwork.PlanRoute(deps.Sling.Cfg, opts.Target, routedwork.DemandGeneric)
	if err != nil {
		return routeCreateError(opts, stdout, stderr, "target_resolve_failed", err)
	}
	a, ok := resolveAgentIdentity(deps.Sling.Cfg, plan.Target, "")
	if !ok {
		return routeCreateError(opts, stdout, stderr, "target_resolve_failed", fmt.Errorf("route target %q not found", opts.Target))
	}

	sourceMetadata, err := parseMetadataArgs(opts.Metadata)
	if err != nil {
		return routeCreateError(opts, stdout, stderr, "invalid_metadata", err)
	}
	if sourceMetadata == nil {
		sourceMetadata = map[string]string{}
	}
	storeRef := strings.TrimSpace(deps.Sling.StoreRef)
	if storeRef == "" {
		storeRef = plan.ClaimStoreRef
	}
	sourceMetadata[routeCreateRequestedTargetMetadataKey] = plan.RequestedTarget
	sourceMetadata[routeCreateResolvedTargetMetadataKey] = plan.Target
	sourceMetadata[routeCreateClaimStoreRefMetadataKey] = storeRef

	if err := prevalidateRouteCreateFormula(opts, deps.Sling, a, sourceMetadata); err != nil {
		return routeCreateError(opts, stdout, stderr, "formula_validation_failed", err)
	}
	if opts.DryRun {
		result := routeCreateJSONResult{
			SchemaVersion: "1",
			OK:            true,
			Target:        plan.Target,
			Formula:       opts.On,
			Metadata:      sourceMetadata,
			DryRun:        true,
		}
		if opts.JSON {
			return writeCLIJSONLineOrExit(stdout, stderr, "gc route create", result)
		}
		fmt.Fprintf(stdout, "Would create %s routed to %s with formula %s\n", strings.TrimSpace(opts.Type), plan.Target, opts.On) //nolint:errcheck
		return 0
	}

	created, err := deps.Sling.Store.Create(beads.Bead{
		Title:       strings.TrimSpace(opts.Title),
		Description: opts.Description,
		Type:        strings.TrimSpace(opts.Type),
		Labels:      nonEmptyStrings(opts.Labels),
		Metadata:    sourceMetadata,
	})
	if err != nil {
		return routeCreateError(opts, stdout, stderr, "bead_create_failed", fmt.Errorf("creating source bead: %w", err))
	}
	slingOpts := slingOpts{
		Target:        a,
		BeadOrFormula: created.ID,
		OnFormula:     opts.On,
		Vars:          opts.Vars,
	}
	result, err := sling.DoSling(slingOpts, deps.Sling, deps.Sling.Store)
	printSlingWarnings(result, stderr)
	if err != nil {
		return routeCreateError(opts, stdout, stderr, "route_attach_failed", err)
	}
	if opts.JSON {
		fresh, _ := deps.Sling.Store.Get(created.ID)
		return writeCLIJSONLineOrExit(stdout, stderr, "gc route create", routeCreateJSONResult{
			SchemaVersion: "1",
			OK:            true,
			ID:            created.ID,
			Target:        result.Target,
			Formula:       result.FormulaName,
			Method:        result.Method,
			WispRootID:    result.WispRootID,
			WorkflowID:    result.WorkflowID,
			Metadata:      fresh.Metadata,
		})
	}
	fmt.Fprintf(stdout, "Created %s -- %q\n", created.ID, created.Title) //nolint:errcheck
	printSlingResult(result, stdout, stderr)
	return 0
}

func validateRouteCreateOptions(opts routeCreateOptions) error {
	if strings.TrimSpace(opts.Target) == "" {
		return fmt.Errorf("--target is required")
	}
	if strings.TrimSpace(opts.On) == "" {
		return fmt.Errorf("--on is required")
	}
	if strings.TrimSpace(opts.Title) == "" {
		return fmt.Errorf("--title is required")
	}
	if strings.TrimSpace(opts.Type) == "" {
		return fmt.Errorf("--type is required")
	}
	return nil
}

func routeCreateError(opts routeCreateOptions, stdout, stderr io.Writer, code string, err error) int {
	if opts.JSON {
		return writeJSONError(stdout, stderr, code, fmt.Sprintf("gc route create: %v", err), 1)
	}
	fmt.Fprintf(stderr, "gc route create: %v\n", err) //nolint:errcheck
	return 1
}

func prevalidateRouteCreateFormula(opts routeCreateOptions, deps slingDeps, a config.Agent, sourceMetadata map[string]string) error {
	userVars, err := parseRouteKeyValues("--var", opts.Vars)
	if err != nil {
		return err
	}
	searchPaths := sling.SlingFormulaSearchPaths(deps, a)
	compileVars := make(map[string]string, len(userVars)+len(sourceMetadata)+1)
	for key, value := range sourceMetadata {
		compileVars[key] = value
	}
	for key, value := range userVars {
		compileVars[key] = value
	}
	recipe, err := formula.CompileWithoutRuntimeVarValidation(context.Background(), opts.On, searchPaths, compileVars)
	if err != nil {
		return err
	}
	validationVars := make(map[string]string, len(compileVars)+1)
	for key, value := range compileVars {
		validationVars[key] = value
	}
	if _, ok := recipe.Vars["warrant_id"]; ok {
		if strings.TrimSpace(validationVars["warrant_id"]) == "" {
			validationVars["warrant_id"] = "route-create-pending"
		}
	}
	return molecule.ValidateRecipeRuntimeVars(recipe, molecule.Options{Vars: validationVars})
}

func parseRouteKeyValues(flag string, items []string) (map[string]string, error) {
	if len(items) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(items))
	for _, item := range items {
		key, value, ok := strings.Cut(item, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid %s %q (want key=value)", flag, item)
		}
		out[key] = value
	}
	return out, nil
}

func nonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func routeCreateStoreRoot(cfg *config.City, cityPath string, plan routedwork.RoutePlan) string {
	if plan.Scope == routedwork.ScopeCity || strings.TrimSpace(plan.Rig) == "" {
		return filepath.Clean(cityPath)
	}
	for _, rig := range cfg.Rigs {
		if rig.Name == plan.Rig || rig.Path == plan.Rig {
			return resolveStoreScopeRoot(cityPath, rig.Path)
		}
	}
	return resolveStoreScopeRoot(cityPath, plan.Rig)
}
