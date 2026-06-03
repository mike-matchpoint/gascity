package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/workselect"
)

var errWorkNotFound = errors.New("no matching work")

type workCommandOptions struct {
	Agent       string
	Assignee    string
	JSON        bool
	Status      string
	SetMetadata []string
}

func newWorkCmd(stdout, stderr io.Writer) *cobra.Command {
	base := &workCommandOptions{}
	cmd := &cobra.Command{
		Use:   "work",
		Short: "Inspect and claim typed work selectors",
	}
	cmd.PersistentFlags().StringVar(&base.Agent, "agent", "", "agent identity (defaults to $GC_TEMPLATE, $GC_ALIAS, or $GC_AGENT)")

	countOpts := &workCommandOptions{}
	countCmd := &cobra.Command{
		Use:   "count",
		Short: "Count work matching an agent's typed selector",
		RunE: func(_ *cobra.Command, _ []string) error {
			countOpts.Agent = base.Agent
			if code := cmdWorkCount(*countOpts, stdout, stderr); code != 0 {
				return errExit
			}
			return nil
		},
	}
	countCmd.Flags().BoolVar(&countOpts.JSON, "json", false, "print JSON")

	nextOpts := &workCommandOptions{}
	nextCmd := &cobra.Command{
		Use:   "next",
		Short: "Print the next work item matching an agent's typed selector",
		RunE: func(_ *cobra.Command, _ []string) error {
			nextOpts.Agent = base.Agent
			if code := cmdWorkNext(*nextOpts, stdout, stderr); code != 0 {
				return errExit
			}
			return nil
		},
	}
	nextCmd.Flags().BoolVar(&nextOpts.JSON, "json", false, "print JSON")

	claimOpts := &workCommandOptions{}
	claimCmd := &cobra.Command{
		Use:   "claim",
		Short: "Atomically claim the next work item matching an agent's typed selector",
		RunE: func(_ *cobra.Command, _ []string) error {
			claimOpts.Agent = base.Agent
			if code := cmdWorkClaim(*claimOpts, stdout, stderr); code != 0 {
				return errExit
			}
			return nil
		},
	}
	claimCmd.Flags().StringVar(&claimOpts.Assignee, "assignee", "", "claim assignee (defaults to session identity)")
	claimCmd.Flags().BoolVar(&claimOpts.JSON, "json", false, "print JSON")
	claimCmd.Flags().StringVar(&claimOpts.Status, "status", "", "claim status (default and only supported value: in_progress)")
	claimCmd.Flags().StringArrayVar(&claimOpts.SetMetadata, "set-metadata", nil, "metadata key=value to set atomically with the claim")

	cmd.AddCommand(countCmd, nextCmd, claimCmd)
	return cmd
}

func cmdWorkCount(opts workCommandOptions, stdout, stderr io.Writer) int {
	ctx, err := resolveWorkCommandContext(opts, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "gc work count: %v\n", err) //nolint:errcheck
		return 1
	}
	n, err := workSelectorCountForCommand(ctx.store, ctx.selector, ctx.assignmentIDs)
	if err != nil {
		fmt.Fprintf(stderr, "gc work count: %v\n", err) //nolint:errcheck
		return 1
	}
	if opts.JSON {
		return writeCLIJSONLineOrExit(stdout, stderr, "gc work count", map[string]int{"count": n})
	}
	fmt.Fprintln(stdout, n) //nolint:errcheck
	return 0
}

func cmdWorkNext(opts workCommandOptions, stdout, stderr io.Writer) int {
	ctx, err := resolveWorkCommandContext(opts, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "gc work next: %v\n", err) //nolint:errcheck
		return 1
	}
	next, ok, err := nextWorkForCommand(ctx.store, ctx.selector, ctx.assignmentIDs)
	if err != nil {
		fmt.Fprintf(stderr, "gc work next: %v\n", err) //nolint:errcheck
		return 1
	}
	if !ok {
		fmt.Fprintln(stderr, "gc work next: no matching work") //nolint:errcheck
		return 1
	}
	if opts.JSON {
		return writeCLIJSONLineOrExit(stdout, stderr, "gc work next", next)
	}
	writeBeadJSON(next, stdout)
	return 0
}

func cmdWorkClaim(opts workCommandOptions, stdout, stderr io.Writer) int {
	ctx, err := resolveWorkCommandContext(opts, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "gc work claim: %v\n", err) //nolint:errcheck
		return 1
	}
	explicitAssignee := strings.TrimSpace(opts.Assignee)
	claimAssignee := explicitAssignee
	if claimAssignee == "" {
		claimAssignee = defaultWorkClaimAssignee(ctx.agent)
	}
	metadata, err := parseWorkMetadata(opts.SetMetadata)
	if err != nil {
		fmt.Fprintf(stderr, "gc work claim: %v\n", err) //nolint:errcheck
		return 1
	}
	if _, err := normalizeWorkClaimStatus(opts.Status); err != nil {
		fmt.Fprintf(stderr, "gc work claim: %v\n", err) //nolint:errcheck
		return 1
	}
	claimed, err := claimNextWorkForCommand(ctx.store, ctx.selector, ctx.assignmentIDs, explicitAssignee, claimAssignee, metadata)
	if err != nil {
		if errors.Is(err, errWorkNotFound) {
			fmt.Fprintln(stderr, "gc work claim: no matching work") //nolint:errcheck
			return 1
		}
		if errors.Is(err, beads.ErrClaimLost) {
			fmt.Fprintf(stderr, "gc work claim: %v\n", err) //nolint:errcheck
			return 2
		}
		fmt.Fprintf(stderr, "gc work claim: %v\n", err) //nolint:errcheck
		return 1
	}
	if opts.JSON {
		return writeCLIJSONLineOrExit(stdout, stderr, "gc work claim", claimed)
	}
	writeBeadJSON(claimed, stdout)
	return 0
}

type workCommandContext struct {
	cityPath      string
	cfg           *config.City
	agent         config.Agent
	selector      config.WorkSelector
	store         beads.Store
	assignmentIDs []string
}

func resolveWorkCommandContext(opts workCommandOptions, stderr io.Writer) (workCommandContext, error) {
	agentName := strings.TrimSpace(opts.Agent)
	if agentName == "" {
		agentName = firstNonEmptyWork(os.Getenv("GC_TEMPLATE"), os.Getenv("GC_ALIAS"), os.Getenv("GC_AGENT"))
	}
	if agentName == "" {
		return workCommandContext{}, fmt.Errorf("agent not specified (pass --agent or set $GC_AGENT)")
	}
	cityPath, err := resolveCity()
	if err != nil {
		return workCommandContext{}, err
	}
	cfg, err := loadCityConfig(cityPath, stderr)
	if err != nil {
		return workCommandContext{}, err
	}
	resolveRigPaths(cityPath, cfg.Rigs)
	agentCfg, ok := resolveAgentIdentity(cfg, agentName, currentRigContext(cfg))
	if !ok {
		return workCommandContext{}, fmt.Errorf("agent %q not found in config", agentName)
	}
	selector := agentCfg.WorkSelector
	selectorField := "work_selector"
	if selector.IsZero() {
		selector = defaultRoutedWorkSelector(agentCfg)
		selectorField = "default_gc_routed_to"
	}
	cityName := loadedCityName(cfg, cityPath)
	selector = expandWorkSelectorTemplates(cityPath, cityName, &agentCfg, cfg.Rigs, selectorField, selector, stderr)
	storeRoot := agentStoreRoot(cityPath, cfg, &agentCfg)
	store, err := openStoreAtForCity(storeRoot, cityPath)
	if err != nil {
		return workCommandContext{}, err
	}
	return workCommandContext{
		cityPath:      cityPath,
		cfg:           cfg,
		agent:         agentCfg,
		selector:      selector,
		store:         store,
		assignmentIDs: workAssignmentIdentifiers(),
	}, nil
}

func defaultRoutedWorkSelector(agent config.Agent) config.WorkSelector {
	target := strings.TrimSpace(agent.PoolName)
	if target == "" {
		target = agent.QualifiedName()
	}
	return config.WorkSelector{
		Status:      "open",
		ExcludeType: "epic",
		Unassigned:  true,
		Ready:       true,
		Metadata:    map[string]string{"gc.routed_to": target},
		Sort:        "created_asc",
	}
}

func workSelectorCountForCommand(store beads.Store, selector config.WorkSelector, assignmentIDs []string) (int, error) {
	assigned, err := listAssignedWorkForIdentities(store, assignmentIDs, 0)
	if err != nil {
		return 0, err
	}
	selected, err := workselect.List(store, selector, 0)
	if err != nil {
		return 0, err
	}
	seen := make(map[string]struct{}, len(assigned)+len(selected))
	for _, item := range assigned {
		seen[item.ID] = struct{}{}
	}
	for _, item := range selected {
		seen[item.ID] = struct{}{}
	}
	return len(seen), nil
}

func nextWorkForCommand(store beads.Store, selector config.WorkSelector, assignmentIDs []string) (beads.Bead, bool, error) {
	assigned, err := listAssignedWorkForIdentities(store, assignmentIDs, 1)
	if err != nil {
		return beads.Bead{}, false, err
	}
	if len(assigned) > 0 {
		return assigned[0], true, nil
	}
	return workselect.Next(store, selector)
}

func claimNextWorkForCommand(store beads.Store, selector config.WorkSelector, assignmentIDs []string, explicitAssignee string, fallbackAssignee string, metadata map[string]string) (beads.Bead, error) {
	claimIDs := appendUniqueWorkIdentifier(assignmentIDs, explicitAssignee)
	claimIDs = appendUniqueWorkIdentifier(claimIDs, fallbackAssignee)
	assigned, err := listAssignedWorkForIdentities(store, claimIDs, 1)
	if err != nil {
		return beads.Bead{}, err
	}
	if len(assigned) > 0 {
		target := assigned[0]
		claimAssignee := strings.TrimSpace(explicitAssignee)
		if claimAssignee == "" {
			claimAssignee = strings.TrimSpace(target.Assignee)
		}
		if claimAssignee == "" {
			claimAssignee = strings.TrimSpace(fallbackAssignee)
		}
		return claimWorkByID(store, target.ID, claimAssignee, metadata)
	}
	return claimNextWork(store, selector, fallbackAssignee, metadata)
}

func claimNextWork(store beads.Store, selector config.WorkSelector, assignee string, metadata map[string]string) (beads.Bead, error) {
	assignee = strings.TrimSpace(assignee)
	if assignee == "" {
		return beads.Bead{}, fmt.Errorf("assignee is required")
	}
	next, ok, err := workselect.Next(store, selector)
	if err != nil {
		return beads.Bead{}, err
	}
	if !ok {
		return beads.Bead{}, errWorkNotFound
	}
	return claimWorkByID(store, next.ID, assignee, metadata)
}

func claimWorkByID(store beads.Store, id string, assignee string, metadata map[string]string) (beads.Bead, error) {
	claimer, ok := store.(beads.ClaimStore)
	if !ok {
		return beads.Bead{}, fmt.Errorf("claiming bead %q: %w", id, beads.ErrClaimUnsupported)
	}
	return claimer.Claim(id, beads.ClaimOpts{Assignee: assignee, Metadata: metadata})
}

func workSelectorCountForController(store beads.Store, selector config.WorkSelector) (int, error) {
	items, err := workSelectorListForController(store, selector)
	return len(items), err
}

func workSelectorListForController(store beads.Store, selector config.WorkSelector) ([]beads.Bead, error) {
	if len(selector.Any) > 0 {
		return workSelectorAnyListForController(store, selector)
	}
	compiled, err := workselect.Compile(selector, 0)
	if err != nil {
		return nil, err
	}
	if compiled.Ready && !compiled.ExplicitType {
		ready, readyErr := readyForControllerDemand(store)
		if readyErr != nil {
			return nil, readyErr
		}
		matching := make([]beads.Bead, 0, len(ready))
		for _, item := range ready {
			if compiled.Query.Matches(item) {
				matching = append(matching, item)
			}
		}
		return workselect.ApplyPostFilters(matching, compiled), nil
	}
	if compiled.Ready {
		items, readyErr := beads.RuntimeReadyList(context.Background(), store, compiled.Query,
			beads.RuntimeReadPolicy(beads.ReadClassHotDegradedOK, "controller.demand.ready-selector"))
		if readyErr != nil {
			return nil, readyErr
		}
		return workselect.ApplyPostFilters(items, compiled), nil
	}
	items, err := listForControllerDemand(store, compiled.Query)
	if err != nil {
		if !beads.IsPartialResult(err) || len(items) == 0 {
			return nil, err
		}
	}
	filtered, filterErr := workselect.ApplyStorePostFilters(store, items, compiled)
	if filterErr != nil {
		return nil, filterErr
	}
	return filtered, err
}

func workSelectorAnyListForController(store beads.Store, selector config.WorkSelector) ([]beads.Bead, error) {
	seen := make(map[string]struct{})
	items := make([]beads.Bead, 0)
	var firstErr error
	for _, clause := range selector.Any {
		clauseItems, err := workSelectorListForController(store, clause)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		for _, item := range clauseItems {
			if _, ok := seen[item.ID]; ok {
				continue
			}
			seen[item.ID] = struct{}{}
			items = append(items, item)
		}
	}
	workselect.SortSelectorResults(selector, items)
	return items, firstErr
}

func parseWorkMetadata(values []string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(values))
	for _, raw := range values {
		key, val, ok := strings.Cut(raw, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid --set-metadata %q (want key=value)", raw)
		}
		out[key] = val
	}
	return out, nil
}

func normalizeWorkClaimStatus(raw string) (string, error) {
	status := strings.TrimSpace(raw)
	if status == "" {
		return "in_progress", nil
	}
	if status != "in_progress" {
		return "", fmt.Errorf("unsupported --status %q (only in_progress is supported by the atomic claim primitive)", raw)
	}
	return status, nil
}

func defaultWorkClaimAssignee(agentCfg config.Agent) string {
	return firstNonEmptyWork(
		os.Getenv("GC_ALIAS"),
		os.Getenv("GC_SESSION_NAME"),
		os.Getenv("GC_AGENT"),
		agentCfg.QualifiedName(),
	)
}

func firstNonEmptyWork(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func workAssignmentIdentifiers() []string {
	return uniqueNonEmptyWorkIdentifiers(
		os.Getenv("GC_SESSION_ID"),
		os.Getenv("GC_SESSION_NAME"),
		os.Getenv("GC_ALIAS"),
	)
}

func appendUniqueWorkIdentifier(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if strings.TrimSpace(existing) == value {
			return values
		}
	}
	out := make([]string, 0, len(values)+1)
	out = append(out, value)
	return out
}

func uniqueNonEmptyWorkIdentifiers(values ...string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func listAssignedWorkForIdentities(store beads.Store, identities []string, limit int) ([]beads.Bead, error) {
	if store == nil || len(identities) == 0 {
		return nil, nil
	}
	out := make([]beads.Bead, 0)
	seen := make(map[string]struct{})
	statusByID := make(map[string]string)
	for _, status := range []string{"in_progress", "open"} {
		for _, assignee := range identities {
			query := beads.ListQuery{
				Status:      status,
				Assignee:    assignee,
				ExcludeType: "epic",
				AllowScan:   true,
				SkipLabels:  true,
				Sort:        beads.SortCreatedAsc,
				TierMode:    beads.TierBoth,
			}
			items, err := store.List(query)
			if err != nil {
				return nil, err
			}
			for _, item := range items {
				if _, ok := seen[item.ID]; ok {
					continue
				}
				if !directAssignedWorkCandidate(item) {
					continue
				}
				if status == "open" {
					ready, err := directAssignedWorkReady(store, item, statusByID)
					if err != nil {
						return nil, err
					}
					if !ready {
						continue
					}
				}
				seen[item.ID] = struct{}{}
				out = append(out, item)
				if limit > 0 && len(out) >= limit {
					return out, nil
				}
			}
		}
	}
	return out, nil
}

func directAssignedWorkCandidate(item beads.Bead) bool {
	switch strings.TrimSpace(item.Type) {
	case "epic", "session", "agent", "role", "rig", "gate", "molecule", "message", "merge-request":
		return false
	default:
		return true
	}
}

func directAssignedWorkReady(store beads.Store, item beads.Bead, statusByID map[string]string) (bool, error) {
	deps := item.Dependencies
	if len(deps) == 0 {
		var err error
		deps, err = store.DepList(item.ID, "down")
		if err != nil {
			return false, err
		}
	}
	for _, dep := range deps {
		if !directAssignedBlockingDep(dep.Type) {
			continue
		}
		status, ok := statusByID[dep.DependsOnID]
		if !ok {
			depBead, err := store.Get(dep.DependsOnID)
			if err != nil {
				return false, err
			}
			status = depBead.Status
			statusByID[dep.DependsOnID] = status
		}
		if status != "closed" {
			return false, nil
		}
	}
	return true, nil
}

func directAssignedBlockingDep(depType string) bool {
	switch strings.TrimSpace(depType) {
	case "blocks", "waits-for", "conditional-blocks":
		return true
	default:
		return false
	}
}

func agentStoreRoot(cityPath string, cfg *config.City, agentCfg *config.Agent) string {
	rigName := configuredRigName(cityPath, agentCfg, cfg.Rigs)
	if rigName == "" {
		return cityPath
	}
	return rigRootForName(rigName, cfg.Rigs)
}

func expandWorkSelectorTemplates(
	cityPath, cityName string,
	agentCfg *config.Agent,
	rigs []config.Rig,
	field string,
	selector config.WorkSelector,
	stderr io.Writer,
) config.WorkSelector {
	var expandSelector func(string, config.WorkSelector) config.WorkSelector
	expandSelector = func(field string, selector config.WorkSelector) config.WorkSelector {
		expand := func(name, value string) string {
			if strings.TrimSpace(value) == "" {
				return value
			}
			return expandAgentCommandTemplate(cityPath, cityName, agentCfg, rigs, field+"."+name, value, stderr)
		}
		if len(selector.Any) > 0 {
			clauses := make([]config.WorkSelector, len(selector.Any))
			for i, clause := range selector.Any {
				clauses[i] = expandSelector(fmt.Sprintf("%s.any[%d]", field, i), clause)
			}
			selector.Any = clauses
			return selector
		}
		selector.Status = expand("status", selector.Status)
		selector.Type = expand("type", selector.Type)
		selector.ExcludeType = expand("exclude_type", selector.ExcludeType)
		selector.Label = expand("label", selector.Label)
		selector.Assignee = expand("assignee", selector.Assignee)
		selector.Parent = expand("parent", selector.Parent)
		selector.Tier = expand("tier", selector.Tier)
		selector.Sort = expand("sort", selector.Sort)
		if len(selector.Metadata) > 0 {
			metadata := make(map[string]string, len(selector.Metadata))
			for k, v := range selector.Metadata {
				metadata[k] = expand("metadata."+k, v)
			}
			selector.Metadata = metadata
		}
		return selector
	}
	return expandSelector(field, selector)
}
