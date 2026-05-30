package beads

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// List returns beads matching the query. Active-bead queries are served from
// cache when available. IncludeClosed queries merge cached active results with
// backing-store history when possible, preserving partial backing rows when bd
// reports corrupt entries and retaining cache-only fallback for transient
// non-partial bd failures.
func (c *CachingStore) List(query ListQuery) ([]Bead, error) {
	if !query.HasFilter() && !query.AllowScan {
		return nil, fmt.Errorf("listing beads: %w", ErrQueryRequiresScan)
	}
	// The cache only holds the issues tier (PrimeActive/Prime call the
	// backing store without a TierMode). Wisps and union queries must
	// reach the backing store directly so we do not return a stale or
	// incomplete snapshot of the wisps table.
	if query.TierMode != TierIssues {
		return c.backing.List(query)
	}
	if query.Live || query.ParentID != "" || query.Label != "" {
		c.mu.RLock()
		startSeq := c.mutationSeq
		c.mu.RUnlock()
		items, err := c.backing.List(query)
		if err == nil {
			items = c.refreshCachedBeads(query, startSeq, items)
		}
		return items, err
	}

	c.mu.RLock()
	state := c.state
	if state == cacheLive || state == cachePartial {
		primePartialErr := c.primePartialErr
		if len(c.dirty) > 0 {
			c.mu.RUnlock()
			return c.backing.List(liveListQuery(query))
		}
		if primePartialErr != nil {
			c.mu.RUnlock()
			return c.backing.List(liveListQuery(query))
		}
		// PrimeActive loads the full active set (open + in_progress), so
		// active-only queries are complete even before the history prime finishes.
		cached := make([]Bead, 0, len(c.beads))
		for _, b := range c.beads {
			if !query.Matches(b) {
				continue
			}
			cached = append(cached, cloneBead(b))
		}
		c.mu.RUnlock()

		finish := func(items []Bead, err error) ([]Bead, error) {
			sortBeadsForQuery(items, query.Sort)
			if query.Limit > 0 && len(items) > query.Limit {
				items = items[:query.Limit]
			}
			return items, err
		}

		if !query.IncludesClosed() {
			return finish(cached, nil)
		}

		// The cache never has a complete closed-only or parent-history view, so
		// preserve the old backing-store behavior for those query shapes.
		if query.Status == "closed" || query.ParentID != "" {
			return c.backing.List(liveListQuery(query))
		}

		all, err := c.backing.List(liveListQuery(query))
		if err != nil {
			if !IsPartialResult(err) {
				return finish(cached, nil)
			}
		}

		seen := make(map[string]bool, len(cached))
		for _, b := range cached {
			seen[b.ID] = true
		}
		for _, b := range all {
			if seen[b.ID] {
				continue
			}
			cached = append(cached, b)
			seen[b.ID] = true
		}
		return finish(cached, err)
	}
	c.mu.RUnlock()
	return c.backing.List(liveListQuery(query))
}

// RuntimeList serves hot runtime callers from cache or indexed SQL only. It
// never delegates to the backing Store.List fallback path.
func (c *CachingStore) RuntimeList(ctx context.Context, query ListQuery, policy ReadPolicy) ([]Bead, error) {
	policy = normalizeReadPolicy(policy)
	if policy.AllowFallback {
		return c.List(query)
	}
	if !query.HasFilter() && !query.AllowScan {
		return nil, fmt.Errorf("runtime cache list: %w", ErrQueryRequiresScan)
	}
	if cached, ok := c.runtimeCachedList(query); ok {
		return enforceRuntimeRowCap(cached, policy, "list", "cache")
	}
	indexed, ok := c.backing.(IndexedLister)
	if !ok {
		return nil, degradedRead(policy, "list", "cache", "unavailable", ErrIndexedListUnsupported)
	}
	indexedQuery := liveListQuery(query)
	if indexedQuery.Label != "" {
		indexedQuery.SkipLabels = false
	}
	if policy.MaxRows > 0 && indexedQuery.Limit <= 0 {
		indexedQuery.Limit = policy.MaxRows
	}
	readCtx, cancel := contextWithReadPolicy(ctx, policy)
	defer cancel()
	result, err := indexed.ListIndexed(readCtx, indexedQuery)
	if err != nil {
		return result.Beads, degradedRead(policy, "list", "indexed", "", err)
	}
	if !result.DependencyCoverage {
		return result.Beads, degradedRead(policy, "list", "indexed", "dependency-incomplete", ErrIndexedListUnsupported)
	}
	if !result.LabelsCoverage {
		return result.Beads, degradedRead(policy, "list", "indexed", "labels-incomplete", ErrIndexedListUnsupported)
	}
	items := ApplyListQuery(result.Beads, query)
	return enforceRuntimeRowCap(items, policy, "list", "indexed")
}

func (c *CachingStore) runtimeCachedList(query ListQuery) ([]Bead, bool) {
	if query.TierMode != TierIssues || query.Live {
		return nil, false
	}
	if query.Label != "" {
		// Active cache primes/reconciles intentionally skip label hydration.
		// Label-filtered hot reads need indexed label coverage instead of
		// treating missing cached labels as negative evidence.
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.state != cacheLive && c.state != cachePartial {
		return nil, false
	}
	if c.primePartialErr != nil {
		return nil, false
	}
	if len(c.dirty) > 0 {
		return nil, false
	}
	if query.IncludesClosed() {
		return nil, false
	}
	cached := make([]Bead, 0, len(c.beads))
	for _, b := range c.beads {
		if !query.Matches(b) {
			continue
		}
		cached = append(cached, cloneBead(b))
	}
	sortBeadsForQuery(cached, query.Sort)
	if query.Limit > 0 && len(cached) > query.Limit {
		cached = cached[:query.Limit]
	}
	return cached, true
}

func liveListQuery(query ListQuery) ListQuery {
	query.Live = true
	return query
}

// CountIndexed delegates cheap aggregate counts to the backing indexed store.
// The in-memory cache intentionally holds only active rows, so all-status row
// counts must be answered by the authoritative indexed reader.
func (c *CachingStore) CountIndexed(ctx context.Context, query ListQuery) (int, error) {
	if c == nil || c.backing == nil {
		return 0, ErrIndexedListUnsupported
	}
	counter, ok := c.backing.(IndexedCounter)
	if !ok {
		return 0, ErrIndexedListUnsupported
	}
	return counter.CountIndexed(ctx, liveListQuery(query))
}

// CachedList returns query results from the in-memory cache only. The boolean
// reports whether the cache was initialized enough to answer without touching
// the backing store. Dirty entries are returned from the last observed
// snapshot; callers must treat this as a read model that may lag writes or
// reconciliation by one tick.
func (c *CachingStore) CachedList(query ListQuery) ([]Bead, bool) {
	if query.TierMode != TierIssues {
		return nil, false
	}
	if query.Label != "" {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.state != cacheLive && c.state != cachePartial {
		return nil, false
	}
	if c.primePartialErr != nil {
		return nil, false
	}
	cached := make([]Bead, 0, len(c.beads))
	for _, b := range c.beads {
		if !query.Matches(b) {
			continue
		}
		cached = append(cached, cloneBead(b))
	}
	sortBeadsForQuery(cached, query.Sort)
	if query.Limit > 0 && len(cached) > query.Limit {
		cached = cached[:query.Limit]
	}
	return cached, true
}

func (c *CachingStore) refreshCachedBeads(query ListQuery, startSeq uint64, items []Bead) []Bead {
	refreshedParents := make(map[string]Bead)
	removedParents := make(map[string]struct{})
	for _, id := range c.staleParentCacheIDs(query.ParentID, items) {
		fresh, err := c.backing.Get(id)
		switch {
		case err == nil:
			refreshedParents[id] = cloneBead(fresh)
		case errors.Is(err, ErrNotFound):
			removedParents[id] = struct{}{}
		default:
			c.recordProblem("refresh parent cache during list", fmt.Errorf("%s: %w", id, err))
		}
	}
	if len(items) == 0 && len(refreshedParents) == 0 && len(removedParents) == 0 {
		return items
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state != cacheLive && c.state != cachePartial {
		return items
	}
	now := time.Now()
	refreshed := make([]Bead, 0, len(items))
	for _, item := range items {
		if c.deletedSeq[item.ID] > startSeq {
			continue
		}
		if c.beadSeq[item.ID] > startSeq {
			current, ok := c.beads[item.ID]
			if ok && query.Matches(current) {
				refreshed = append(refreshed, cloneBead(current))
			}
			continue
		}
		if current, keep := c.recentLocalBeadConflictLocked(item.ID, item, now, false); keep {
			if query.Matches(current) {
				refreshed = append(refreshed, current)
			}
			continue
		}
		if c.beadSeq[item.ID] == startSeq {
			current, ok := c.beads[item.ID]
			if ok && current.Status == "closed" && item.Status != "closed" {
				continue
			}
		}
		c.beads[item.ID] = cloneBead(item)
		c.deps[item.ID] = depsFromBeadFields(item)
		delete(c.dirty, item.ID)
		delete(c.deletedSeq, item.ID)
		if !recentLocalMutation(c.localBeadAt[item.ID], now) {
			delete(c.beadSeq, item.ID)
			delete(c.localBeadAt, item.ID)
		}
		if query.Matches(item) {
			refreshed = append(refreshed, cloneBead(item))
		}
	}
	for id, bead := range refreshedParents {
		if c.deletedSeq[id] > startSeq || c.beadSeq[id] > startSeq {
			continue
		}
		if _, keep := c.recentLocalBeadConflictLocked(id, bead, now, false); keep {
			continue
		}
		c.beads[id] = bead
		c.deps[id] = depsFromBeadFields(bead)
		delete(c.dirty, id)
		delete(c.deletedSeq, id)
		if !recentLocalMutation(c.localBeadAt[id], now) {
			delete(c.beadSeq, id)
			delete(c.localBeadAt, id)
		}
	}
	for id := range removedParents {
		if c.deletedSeq[id] > startSeq || c.beadSeq[id] > startSeq {
			continue
		}
		if current, ok := c.beads[id]; ok && current.Status != "closed" && recentLocalMutation(c.localBeadAt[id], now) {
			continue
		}
		delete(c.beads, id)
		delete(c.deps, id)
		delete(c.dirty, id)
		delete(c.deletedSeq, id)
		delete(c.beadSeq, id)
		delete(c.localBeadAt, id)
	}
	c.markFreshLocked(time.Now())
	c.updateStatsLocked()
	return refreshed
}

func (c *CachingStore) staleParentCacheIDs(parentID string, fresh []Bead) []string {
	if parentID == "" {
		return nil
	}

	freshIDs := make(map[string]struct{}, len(fresh))
	for _, item := range fresh {
		freshIDs[item.ID] = struct{}{}
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.state != cacheLive && c.state != cachePartial {
		return nil
	}

	var stale []string
	for id, bead := range c.beads {
		if bead.ParentID != parentID {
			continue
		}
		if _, ok := freshIDs[id]; ok {
			continue
		}
		stale = append(stale, id)
	}
	return stale
}

// ListOpen returns all cached beads, optionally filtered by status.
func (c *CachingStore) ListOpen(status ...string) ([]Bead, error) {
	query := ListQuery{AllowScan: true}
	if len(status) > 0 {
		query.Status = status[0]
	}
	return c.List(query)
}

// Get returns a single bead by ID from the cache or backing store.
func (c *CachingStore) Get(id string) (Bead, error) {
	c.mu.RLock()
	if _, deleted := c.deletedSeq[id]; deleted {
		c.mu.RUnlock()
		return Bead{}, ErrNotFound
	}
	if _, mutated := c.beadSeq[id]; mutated {
		if _, dirty := c.dirty[id]; !dirty {
			if b, ok := c.beads[id]; ok {
				c.mu.RUnlock()
				return cloneBead(b), nil
			}
		}
	}
	if c.state == cacheLive || c.state == cachePartial {
		if _, ok := c.dirty[id]; ok {
			startSeq := c.mutationSeq
			c.mu.RUnlock()
			fresh, err := c.backing.Get(id)
			if err != nil {
				return Bead{}, err
			}
			c.mu.Lock()
			if c.state != cacheLive && c.state != cachePartial {
				c.mu.Unlock()
				return fresh, nil
			}
			switch {
			case c.deletedSeq[id] > startSeq:
				c.mu.Unlock()
				return Bead{}, ErrNotFound
			case c.beadSeq[id] > startSeq:
				if _, stillDirty := c.dirty[id]; stillDirty {
					c.mu.Unlock()
					return c.backing.Get(id)
				}
				if current, ok := c.beads[id]; ok {
					c.mu.Unlock()
					return cloneBead(current), nil
				}
				c.mu.Unlock()
				return Bead{}, ErrNotFound
			}
			c.beads[id] = cloneBead(fresh)
			c.deps[id] = depsFromBeadFields(fresh)
			delete(c.dirty, id)
			delete(c.deletedSeq, id)
			delete(c.beadSeq, id)
			c.markFreshLocked(time.Now())
			c.updateStatsLocked()
			c.mu.Unlock()
			return fresh, nil
		}
		if b, ok := c.beads[id]; ok {
			c.mu.RUnlock()
			return cloneBead(b), nil
		}
		c.mu.RUnlock()
		return c.backing.Get(id)
	}
	c.mu.RUnlock()
	return c.backing.Get(id)
}

// RuntimeGet returns a bead by ID only when the runtime cache can answer
// without falling through to a foreground backing Get.
func (c *CachingStore) RuntimeGet(_ context.Context, id string, policy ReadPolicy) (Bead, error) {
	policy = normalizeReadPolicy(policy)
	if policy.AllowFallback {
		return c.Get(id)
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if _, deleted := c.deletedSeq[id]; deleted {
		return Bead{}, ErrNotFound
	}
	if b, ok := c.beads[id]; ok {
		if _, dirty := c.dirty[id]; dirty {
			return Bead{}, degradedRead(policy, "get", "cache", "dirty", ErrIndexedListUnsupported)
		}
		return cloneBead(b), nil
	}
	if c.state == cacheLive {
		return Bead{}, ErrNotFound
	}
	return Bead{}, degradedRead(policy, "get", "cache", "partial", ErrIndexedListUnsupported)
}

// Ready returns open beads whose blocking deps are all closed.
func (c *CachingStore) Ready(query ...ReadyQuery) ([]Bead, error) {
	if readyQueryFromArgs(query) != (ReadyQuery{}) {
		return c.backing.Ready(query...)
	}
	c.mu.RLock()
	if c.state == cacheLive && c.depsComplete {
		if len(c.dirty) > 0 {
			c.mu.RUnlock()
			return c.backing.Ready(query...)
		}
		if c.primePartialErr != nil {
			c.mu.RUnlock()
			return c.backing.Ready(query...)
		}
		statusByID := make(map[string]string, len(c.beads))
		depsByID := make(map[string][]Dep, len(c.deps))
		openBeads := make([]Bead, 0, len(c.beads))
		for _, b := range c.beads {
			statusByID[b.ID] = b.Status
			if b.Status == "open" && !b.Ephemeral && !IsReadyExcludedType(b.Type) {
				openBeads = append(openBeads, cloneBead(b))
			}
		}
		for _, b := range openBeads {
			deps := cloneDeps(c.deps[b.ID])
			depsByID[b.ID] = deps
		}
		c.mu.RUnlock()

		var result []Bead
		for _, b := range openBeads {
			blocked := false
			for _, dep := range depsByID[b.ID] {
				switch dep.Type {
				case "blocks", "waits-for", "conditional-blocks":
				default:
					continue
				}
				if status, ok := statusByID[dep.DependsOnID]; ok && status != "closed" {
					blocked = true
					break
				}
			}
			if !blocked {
				result = append(result, cloneBead(b))
			}
		}
		return result, nil
	}
	c.mu.RUnlock()
	return c.backing.Ready(query...)
}

// CachedReady returns ready beads from the in-memory active read model.
// The boolean reports whether the cache was initialized enough to answer
// without touching the backing store. Unlike Ready, this can answer from a
// partial active cache only when each open bead has known dependency coverage.
func (c *CachingStore) CachedReady() ([]Bead, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.state != cacheLive && c.state != cachePartial {
		return nil, false
	}
	if c.primePartialErr != nil || len(c.dirty) > 0 {
		return nil, false
	}

	statusByID := make(map[string]string, len(c.beads))
	openBeads := make([]Bead, 0, len(c.beads))
	for _, b := range c.beads {
		statusByID[b.ID] = b.Status
		if b.Status == "open" && !b.Ephemeral && !IsReadyExcludedType(b.Type) {
			openBeads = append(openBeads, cloneBead(b))
		}
	}

	result := make([]Bead, 0, len(openBeads))
	for _, b := range openBeads {
		deps, ok := c.deps[b.ID]
		switch {
		case ok:
		case c.depsComplete:
			deps = nil
		default:
			return nil, false
		}
		if cachedBeadReady(statusByID, deps) {
			result = append(result, cloneBead(b))
		}
	}
	return result, true
}

// RuntimeReady serves controller/session hot ready checks from cache or the
// bounded indexed active read path. It never invokes bd ready.
func (c *CachingStore) RuntimeReady(ctx context.Context, query ReadyQuery, policy ReadPolicy) ([]Bead, error) {
	policy = normalizeReadPolicy(policy)
	if policy.AllowFallback {
		return readyWithQuery(c, query)
	}
	ready, ok := c.CachedReady()
	if !ok {
		return c.runtimeIndexedReady(ctx, query, policy)
	}
	ready = filterReadyQuery(ready, query)
	return enforceRuntimeRowCap(ready, policy, "ready", "cache")
}

// RuntimeReadyList computes selector-ready rows for controller hot reads from
// dependency-complete cache state or from one bounded indexed active snapshot.
// It does not invoke bd ready, bd dep list, bd get, or per-row store fallbacks.
func (c *CachingStore) RuntimeReadyList(ctx context.Context, query ListQuery, policy ReadPolicy) ([]Bead, error) {
	policy = normalizeReadPolicy(policy)
	if policy.AllowFallback {
		return readyListWithQuery(c, query)
	}
	if rows, ok := c.runtimeCachedActiveRowsForReady(query); ok {
		ready := readyFromActiveRowsMatchingListQuery(rows, query)
		return enforceRuntimeRowCap(ready, policy, "ready", "cache")
	}
	return c.runtimeIndexedReadyList(ctx, query, policy)
}

func (c *CachingStore) runtimeIndexedReady(ctx context.Context, query ReadyQuery, policy ReadPolicy) ([]Bead, error) {
	if c == nil || c.backing == nil {
		return nil, degradedRead(policy, "ready", "indexed", "", errors.New("nil backing store"))
	}
	// RuntimeList reports degraded row-cap or partial indexed reads. Do not
	// compute readiness unless the active set is complete enough to prove that
	// missing dependency targets are closed rather than just outside the window.
	rows, err := RuntimeList(ctx, c.backing, ListQuery{
		AllowScan:  true,
		SkipLabels: true,
		TierMode:   TierIssues,
	}, policy)
	if err != nil {
		return nil, degradedRead(policy, "ready", "indexed", "", err)
	}
	ready := readyFromActiveRows(rows, query)
	return enforceRuntimeRowCap(ready, policy, "ready", "indexed")
}

func (c *CachingStore) runtimeCachedActiveRowsForReady(query ListQuery) ([]Bead, bool) {
	if c == nil || query.TierMode != TierIssues || query.Live || query.IncludesClosed() {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.state != cacheLive && c.state != cachePartial {
		return nil, false
	}
	if c.primePartialErr != nil || len(c.dirty) > 0 || !c.depsComplete {
		return nil, false
	}
	rows := make([]Bead, 0, len(c.beads))
	for _, b := range c.beads {
		if b.Status == "closed" || b.Ephemeral {
			continue
		}
		row := cloneBead(b)
		row.Dependencies = cloneDeps(c.deps[b.ID])
		rows = append(rows, row)
	}
	sortBeadsForQuery(rows, query.Sort)
	return rows, true
}

func (c *CachingStore) runtimeIndexedReadyList(ctx context.Context, query ListQuery, policy ReadPolicy) ([]Bead, error) {
	if c == nil || c.backing == nil {
		return nil, degradedRead(policy, "ready", "indexed", "", errors.New("nil backing store"))
	}
	indexed, ok := c.backing.(IndexedLister)
	if !ok {
		return nil, degradedRead(policy, "ready", "indexed", "unavailable", ErrIndexedListUnsupported)
	}
	indexedQuery := liveListQuery(runtimeReadyActiveListQuery(query, policy))
	readCtx, cancel := contextWithReadPolicy(ctx, policy)
	defer cancel()
	result, err := indexed.ListIndexed(readCtx, indexedQuery)
	if err != nil {
		return nil, degradedRead(policy, "ready", "indexed", "", err)
	}
	if !result.DependencyCoverage {
		return nil, degradedRead(policy, "ready", "indexed", "dependency-incomplete", ErrIndexedListUnsupported)
	}
	if query.Label != "" && !result.LabelsCoverage {
		return nil, degradedRead(policy, "ready", "indexed", "labels-incomplete", ErrIndexedListUnsupported)
	}
	rows := rowsWithIndexedDependencies(result)
	if policy.MaxRows > 0 && len(rows) >= policy.MaxRows {
		return nil, degradedRead(policy, "ready", "indexed", "row-cap", fmt.Errorf("runtime ready active set returned %d rows at cap %d", len(rows), policy.MaxRows))
	}
	ready := readyFromActiveRowsMatchingListQuery(rows, query)
	return enforceRuntimeRowCap(ready, policy, "ready", "indexed")
}

func rowsWithIndexedDependencies(result IndexedListResult) []Bead {
	depsByID := indexedResultDepMap(result)
	rows := make([]Bead, 0, len(result.Beads))
	for _, row := range result.Beads {
		next := cloneBead(row)
		next.Dependencies = cloneDeps(depsByID[next.ID])
		rows = append(rows, next)
	}
	return rows
}

func readyFromActiveRows(rows []Bead, query ReadyQuery) []Bead {
	statusByID := make(map[string]string, len(rows))
	for _, row := range rows {
		statusByID[row.ID] = row.Status
	}
	ready := make([]Bead, 0, len(rows))
	for _, row := range rows {
		if row.Status != "open" || row.Ephemeral || IsReadyExcludedType(row.Type) {
			continue
		}
		if !cachedBeadReady(statusByID, row.Dependencies) {
			continue
		}
		ready = append(ready, cloneBead(row))
	}
	return filterReadyQuery(ready, query)
}

func readyFromActiveRowsMatchingListQuery(rows []Bead, query ListQuery) []Bead {
	statusByID := make(map[string]string, len(rows))
	for _, row := range rows {
		statusByID[row.ID] = row.Status
	}
	ready := make([]Bead, 0, len(rows))
	explicitType := query.Type != ""
	for _, row := range rows {
		if row.Status != "open" {
			continue
		}
		if !explicitType && IsReadyExcludedType(row.Type) {
			continue
		}
		if !query.Matches(row) {
			continue
		}
		if !cachedBeadReady(statusByID, row.Dependencies) {
			continue
		}
		ready = append(ready, cloneBead(row))
		if query.Limit > 0 && len(ready) >= query.Limit {
			break
		}
	}
	return ready
}

func filterReadyQuery(items []Bead, query ReadyQuery) []Bead {
	if query == (ReadyQuery{}) {
		return items
	}
	out := make([]Bead, 0, len(items))
	for _, item := range items {
		if query.Assignee != "" && item.Assignee != query.Assignee {
			continue
		}
		out = append(out, item)
		if query.Limit > 0 && len(out) >= query.Limit {
			break
		}
	}
	return out
}

func cachedBeadReady(statusByID map[string]string, deps []Dep) bool {
	for _, dep := range deps {
		switch dep.Type {
		case "blocks", "waits-for", "conditional-blocks":
		default:
			continue
		}
		if status, ok := statusByID[dep.DependsOnID]; ok && status != "closed" {
			return false
		}
	}
	return true
}

// Children returns beads with the given parent ID.
func (c *CachingStore) Children(parentID string, opts ...QueryOpt) ([]Bead, error) {
	return c.List(ListQuery{
		ParentID:      parentID,
		IncludeClosed: HasOpt(opts, IncludeClosed),
		Sort:          SortCreatedAsc,
	})
}

// ListByLabel returns beads matching the given label. The active cache may be
// label-sparse, so label filters are answered by the backing store directly.
// Pass IncludeClosed to include closed beads.
func (c *CachingStore) ListByLabel(label string, limit int, opts ...QueryOpt) ([]Bead, error) {
	return c.List(ListQuery{
		Label:         label,
		Limit:         limit,
		IncludeClosed: HasOpt(opts, IncludeClosed),
		Sort:          SortCreatedDesc,
		TierMode:      TierModeFromOpts(opts),
	})
}

// ListByAssignee returns beads assigned to the given agent with matching status.
func (c *CachingStore) ListByAssignee(assignee, status string, limit int) ([]Bead, error) {
	return c.List(ListQuery{
		Assignee: assignee,
		Status:   status,
		Limit:    limit,
		Sort:     SortCreatedDesc,
	})
}

// ListByMetadata filters beads by metadata key-value pairs. By default, serves
// from cache only (non-closed beads). Pass IncludeClosed to also query the
// backing store for closed beads and merge results.
func (c *CachingStore) ListByMetadata(filters map[string]string, limit int, opts ...QueryOpt) ([]Bead, error) {
	return c.List(ListQuery{
		Metadata:      filters,
		Limit:         limit,
		IncludeClosed: HasOpt(opts, IncludeClosed),
		Sort:          SortCreatedDesc,
		TierMode:      TierModeFromOpts(opts),
	})
}

func matchesMetadata(b Bead, filters map[string]string) bool {
	for k, v := range filters {
		if b.Metadata[k] != v {
			return false
		}
	}
	return true
}

// DepList returns dependencies for a bead in the given direction.
func (c *CachingStore) DepList(id, direction string) ([]Dep, error) {
	c.mu.RLock()
	if c.state == cacheLive {
		if direction == "down" || direction == "" {
			if !c.depsComplete {
				c.mu.RUnlock()
				return c.backing.DepList(id, direction)
			}
			if deps, ok := c.deps[id]; ok {
				c.mu.RUnlock()
				return cloneDeps(deps), nil
			}
			// Dep not cached yet - fetch from backing and cache it.
			c.mu.RUnlock()
			deps, err := c.backing.DepList(id, direction)
			if err != nil {
				return nil, err
			}
			c.mu.Lock()
			c.deps[id] = cloneDeps(deps)
			c.mu.Unlock()
			return deps, nil
		}
		// Reverse lookups are only partially cached; defer to the backing
		// store so callers do not observe incomplete results.
		c.mu.RUnlock()
		return c.backing.DepList(id, direction)
	}
	c.mu.RUnlock()
	return c.backing.DepList(id, direction)
}

// Ping delegates to the backing store.
func (c *CachingStore) Ping() error {
	return c.backing.Ping()
}
