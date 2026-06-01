package beads

import "context"

// RuntimeCreate persists a bead under a runtime write policy. HQStore is
// in-process, so it preserves foreground semantics while honoring cancellation.
func (s *HQStore) RuntimeCreate(ctx context.Context, b Bead, policy WritePolicy) (Bead, error) {
	select {
	case <-ctxDone(ctx):
		return Bead{}, degradedWrite(normalizeWritePolicy(policy), "hq", "create", WriteOutcomeNotStarted, ctx.Err())
	default:
	}
	created, err := s.Create(b)
	if existing, ok, duplicateErr := runtimeCreateDuplicateResult(s, b, err, policy, "hq"); ok {
		return existing, duplicateErr
	}
	return created, err
}

// RuntimeGet retrieves a bead under a runtime read policy.
func (s *HQStore) RuntimeGet(ctx context.Context, id string, policy ReadPolicy) (Bead, error) {
	select {
	case <-ctxDone(ctx):
		return Bead{}, degradedRead(normalizeReadPolicy(policy), "get", "hq", "", ctx.Err())
	default:
	}
	return s.Get(id)
}

// RuntimeList lists beads under a runtime read policy.
func (s *HQStore) RuntimeList(ctx context.Context, query ListQuery, policy ReadPolicy) ([]Bead, error) {
	policy = normalizeReadPolicy(policy)
	select {
	case <-ctxDone(ctx):
		return nil, degradedRead(policy, "list", "hq", "", ctx.Err())
	default:
	}
	rows, err := s.List(query)
	if err != nil {
		return nil, degradedRead(policy, "list", "hq", "", err)
	}
	return enforceRuntimeRowCap(rows, policy, "list", "hq")
}

// RuntimeReady returns ready beads under a runtime read policy.
func (s *HQStore) RuntimeReady(ctx context.Context, query ReadyQuery, policy ReadPolicy) ([]Bead, error) {
	policy = normalizeReadPolicy(policy)
	select {
	case <-ctxDone(ctx):
		return nil, degradedRead(policy, "ready", "hq", "", ctx.Err())
	default:
	}
	rows, err := s.Ready(query)
	if err != nil {
		return nil, degradedRead(policy, "ready", "hq", "", err)
	}
	return enforceRuntimeRowCap(rows, policy, "ready", "hq")
}

// RuntimeReadyList computes selector-ready rows from the complete in-process
// active read model under a runtime read policy.
func (s *HQStore) RuntimeReadyList(ctx context.Context, query ListQuery, policy ReadPolicy) ([]Bead, error) {
	policy = normalizeReadPolicy(policy)
	if policy.AllowFallback {
		return readyListWithQuery(s, query)
	}
	select {
	case <-ctxDone(ctx):
		return nil, degradedRead(policy, "ready", "hq", "", ctx.Err())
	default:
	}
	rows, err := s.runtimeReadyActiveRows(query)
	if err != nil {
		return nil, degradedRead(policy, "ready", "hq", "", err)
	}
	ready := readyFromActiveRowsMatchingListQuery(rows, query)
	return enforceRuntimeRowCap(ready, policy, "ready", "hq")
}

func (s *HQStore) runtimeReadyActiveRows(query ListQuery) ([]Bead, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := s.ensureOpenLocked(); err != nil {
		return nil, err
	}
	rows := make([]Bead, 0, len(s.main)+len(s.wisps))
	add := func(values map[string]Bead) {
		for _, b := range values {
			if b.Status == "closed" {
				continue
			}
			row := cloneBead(b)
			for _, dep := range s.deps {
				if dep.IssueID == row.ID {
					row.Dependencies = append(row.Dependencies, dep)
				}
			}
			rows = append(rows, row)
		}
	}
	switch query.TierMode {
	case TierWisps:
		add(s.wisps)
	case TierBoth:
		add(s.main)
		add(s.wisps)
	default:
		add(s.main)
	}
	sortBeadsForQuery(rows, query.Sort)
	return rows, nil
}

// RuntimeUpdate updates a bead under a runtime write policy.
func (s *HQStore) RuntimeUpdate(ctx context.Context, id string, opts UpdateOpts, policy WritePolicy) error {
	select {
	case <-ctxDone(ctx):
		return degradedWrite(normalizeWritePolicy(policy), "hq", "update", WriteOutcomeNotStarted, ctx.Err())
	default:
	}
	return s.Update(id, opts)
}

// RuntimeCloseAll closes beads under a runtime write policy.
func (s *HQStore) RuntimeCloseAll(ctx context.Context, ids []string, metadata map[string]string, policy WritePolicy) (int, error) {
	select {
	case <-ctxDone(ctx):
		return 0, degradedWrite(normalizeWritePolicy(policy), "hq", "close-all", WriteOutcomeNotStarted, ctx.Err())
	default:
	}
	return s.CloseAll(ids, metadata)
}

// RuntimePing reports in-process store health under a runtime write policy.
func (s *HQStore) RuntimePing(ctx context.Context, policy WritePolicy) error {
	select {
	case <-ctxDone(ctx):
		return degradedWrite(normalizeWritePolicy(policy), "hq", "ping", WriteOutcomeNotStarted, ctx.Err())
	default:
	}
	return s.Ping()
}
