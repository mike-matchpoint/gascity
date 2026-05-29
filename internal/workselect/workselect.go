// Package workselect compiles declarative work selectors into bead queries.
package workselect

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
)

// Compiled is a WorkSelector lowered to the beads.ListQuery surface plus
// post-filters that ListQuery cannot express directly.
type Compiled struct {
	Query        beads.ListQuery
	Unassigned   bool
	Ready        bool
	ExplicitType bool
	Limit        int
}

// Compile lowers selector into a normalized ListQuery. Empty status defaults
// to open; empty tier defaults to issues; empty sort defaults to created_asc.
func Compile(selector config.WorkSelector, limit int) (Compiled, error) {
	if len(selector.Any) > 0 {
		return Compiled{}, fmt.Errorf("work selector: any must be evaluated with List")
	}
	query := beads.ListQuery{
		Status:      strings.TrimSpace(selector.Status),
		Type:        strings.TrimSpace(selector.Type),
		ExcludeType: strings.TrimSpace(selector.ExcludeType),
		Label:       strings.TrimSpace(selector.Label),
		Assignee:    strings.TrimSpace(selector.Assignee),
		ParentID:    strings.TrimSpace(selector.Parent),
		AllowScan:   true,
		SkipLabels:  true,
		Sort:        beads.SortCreatedAsc,
	}
	if query.Status == "" {
		query.Status = "open"
	}
	if selector.Metadata != nil {
		query.Metadata = make(map[string]string, len(selector.Metadata))
		for k, v := range selector.Metadata {
			k = strings.TrimSpace(k)
			if k == "" {
				return Compiled{}, fmt.Errorf("work selector: metadata key is empty")
			}
			query.Metadata[k] = strings.TrimSpace(v)
		}
	}
	switch strings.TrimSpace(selector.Tier) {
	case "", "issues":
		query.TierMode = beads.TierIssues
	case "wisps":
		query.TierMode = beads.TierWisps
	case "both":
		query.TierMode = beads.TierBoth
	default:
		return Compiled{}, fmt.Errorf("work selector: unsupported tier %q", selector.Tier)
	}
	switch strings.TrimSpace(selector.Sort) {
	case "":
		// created_asc default set above.
	case "created_asc":
		query.Sort = beads.SortCreatedAsc
	case "created_desc":
		query.Sort = beads.SortCreatedDesc
	default:
		return Compiled{}, fmt.Errorf("work selector: unsupported sort %q", selector.Sort)
	}
	if selector.Unassigned && query.Assignee != "" {
		return Compiled{}, fmt.Errorf("work selector: cannot set both assignee and unassigned")
	}
	if query.Type != "" && query.Type == query.ExcludeType {
		return Compiled{}, fmt.Errorf("work selector: type and exclude_type are both %q", query.Type)
	}
	compiled := Compiled{
		Query:        query,
		Unassigned:   selector.Unassigned,
		Ready:        selector.Ready,
		ExplicitType: query.Type != "",
		Limit:        limit,
	}
	if requiresPostFilter(compiled) && limit > 0 {
		compiled.Query.Limit = 0
	} else {
		compiled.Query.Limit = limit
	}
	return compiled, nil
}

// List returns beads matching selector using the same normalized predicate for
// demand counts, next-item discovery, and claim candidate selection.
func List(store beads.Store, selector config.WorkSelector, limit int) ([]beads.Bead, error) {
	if len(selector.Any) > 0 {
		return listAny(store, selector, limit)
	}
	compiled, err := Compile(selector, limit)
	if err != nil {
		return nil, err
	}
	items, err := store.List(compiled.Query)
	if err != nil {
		return nil, err
	}
	items = ApplyPostFilters(items, compiled)
	if compiled.Ready {
		items, err = applyReadyFilter(store, items)
		if err != nil {
			return nil, err
		}
	}
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

// Count returns the number of beads matching selector.
func Count(store beads.Store, selector config.WorkSelector) (int, error) {
	items, err := List(store, selector, 0)
	if err != nil {
		return 0, err
	}
	return len(items), nil
}

// Next returns the first bead matching selector.
func Next(store beads.Store, selector config.WorkSelector) (beads.Bead, bool, error) {
	items, err := List(store, selector, 1)
	if err != nil {
		return beads.Bead{}, false, err
	}
	if len(items) == 0 {
		return beads.Bead{}, false, nil
	}
	return items[0], true, nil
}

// Matches reports whether b satisfies selector after normalization.
func Matches(selector config.WorkSelector, b beads.Bead) bool {
	if len(selector.Any) > 0 {
		for _, clause := range selector.Any {
			if Matches(clause, b) {
				return true
			}
		}
		return false
	}
	compiled, err := Compile(selector, 0)
	if err != nil {
		return false
	}
	if !compiled.Query.Matches(b) {
		return false
	}
	return len(ApplyPostFilters([]beads.Bead{b}, compiled)) == 1
}

func listAny(store beads.Store, selector config.WorkSelector, limit int) ([]beads.Bead, error) {
	seen := make(map[string]struct{})
	items := make([]beads.Bead, 0)
	for _, clause := range selector.Any {
		clauseItems, err := List(store, clause, 0)
		if err != nil {
			return nil, err
		}
		for _, item := range clauseItems {
			if _, ok := seen[item.ID]; ok {
				continue
			}
			seen[item.ID] = struct{}{}
			items = append(items, item)
		}
	}
	SortSelectorResults(selector, items)
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

// SortSelectorResults applies the deterministic selector result order used by
// any-clause unions after de-duplication.
func SortSelectorResults(selector config.WorkSelector, items []beads.Bead) {
	sortBeads(items, selectorSortOrder(selector))
}

func selectorSortOrder(selector config.WorkSelector) beads.SortOrder {
	if len(selector.Any) == 0 {
		switch strings.TrimSpace(selector.Sort) {
		case "created_desc":
			return beads.SortCreatedDesc
		default:
			return beads.SortCreatedAsc
		}
	}
	var order beads.SortOrder
	for _, clause := range selector.Any {
		clauseOrder := selectorSortOrder(clause)
		if order == "" {
			order = clauseOrder
			continue
		}
		if order != clauseOrder {
			return beads.SortCreatedAsc
		}
	}
	if order == "" {
		return beads.SortCreatedAsc
	}
	return order
}

func sortBeads(items []beads.Bead, order beads.SortOrder) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			if order == beads.SortCreatedDesc {
				return items[i].ID > items[j].ID
			}
			return items[i].ID < items[j].ID
		}
		if order == beads.SortCreatedDesc {
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
}

// ApplyPostFilters enforces selector predicates that are not represented by
// beads.ListQuery: unassigned rows and default infrastructure exclusion.
func ApplyPostFilters(items []beads.Bead, compiled Compiled) []beads.Bead {
	if !requiresPostFilter(compiled) {
		return items
	}
	filtered := items[:0]
	for _, item := range items {
		if compiled.Unassigned && strings.TrimSpace(item.Assignee) != "" {
			continue
		}
		if !compiled.ExplicitType && beads.IsReadyExcludedType(item.Type) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func requiresPostFilter(compiled Compiled) bool {
	return compiled.Unassigned || compiled.Ready || !compiled.ExplicitType
}

// ApplyStorePostFilters completes selector filters that need store reads.
// Controller demand uses this after its cache-aware list path; gc work commands
// use List, which calls the same helper after a direct store list.
func ApplyStorePostFilters(store beads.Store, items []beads.Bead, compiled Compiled) ([]beads.Bead, error) {
	items = ApplyPostFilters(items, compiled)
	if compiled.Ready {
		return applyReadyFilter(store, items)
	}
	return items, nil
}

func applyReadyFilter(store beads.Store, items []beads.Bead) ([]beads.Bead, error) {
	if len(items) == 0 {
		return items, nil
	}
	statusByID := make(map[string]string, len(items))
	for _, item := range items {
		statusByID[item.ID] = item.Status
	}
	filtered := items[:0]
	for _, item := range items {
		ready, err := selectorItemReady(store, item, statusByID)
		if err != nil {
			return nil, err
		}
		if ready {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func selectorItemReady(store beads.Store, item beads.Bead, statusByID map[string]string) (bool, error) {
	deps := item.Dependencies
	if len(deps) == 0 {
		var err error
		deps, err = store.DepList(item.ID, "down")
		if err != nil {
			return false, err
		}
	}
	for _, dep := range deps {
		if !selectorBlockingDep(dep.Type) {
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

func selectorBlockingDep(depType string) bool {
	switch strings.TrimSpace(depType) {
	case "blocks", "waits-for", "conditional-blocks":
		return true
	default:
		return false
	}
}
