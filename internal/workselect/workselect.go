// Package workselect compiles declarative work selectors into bead queries.
package workselect

import (
	"fmt"
	"strings"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
)

// Compiled is a WorkSelector lowered to the beads.ListQuery surface plus
// post-filters that ListQuery cannot express directly.
type Compiled struct {
	Query        beads.ListQuery
	Unassigned   bool
	ExplicitType bool
	Limit        int
}

// Compile lowers selector into a normalized ListQuery. Empty status defaults
// to open; empty tier defaults to issues; empty sort defaults to created_asc.
func Compile(selector config.WorkSelector, limit int) (Compiled, error) {
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
	compiled, err := Compile(selector, limit)
	if err != nil {
		return nil, err
	}
	items, err := store.List(compiled.Query)
	if err != nil {
		return nil, err
	}
	items = ApplyPostFilters(items, compiled)
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
	compiled, err := Compile(selector, 0)
	if err != nil {
		return false
	}
	if !compiled.Query.Matches(b) {
		return false
	}
	return len(ApplyPostFilters([]beads.Bead{b}, compiled)) == 1
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
	return compiled.Unassigned || !compiled.ExplicitType
}
