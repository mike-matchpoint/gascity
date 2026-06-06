package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
)

var errExpiredBeadPreserved = errors.New("expired bead preserved")

// wispGC performs mechanical garbage collection of closed molecules that
// have exceeded their TTL. Follows the nil-guard tracker pattern used by
// crashTracker and idleTracker: nil means disabled.
type wispGC interface {
	// shouldRun returns true if enough time has elapsed since the last run.
	shouldRun(now time.Time) bool

	// runGC lists closed molecules, deletes those older than TTL, and returns
	// the count of purged entries. Errors from individual deletes are
	// best-effort and surfaced without stopping the purge; the returned error
	// also covers list failures.
	runGC(store beads.Store, now time.Time) (int, error)
}

// memoryWispGC is the production implementation of wispGC.
type memoryWispGC struct {
	interval time.Duration
	ttl      time.Duration
	lastRun  time.Time
}

// newWispGC creates a wisp GC tracker. Returns nil if disabled (interval or
// TTL is zero). Callers nil-guard before use.
func newWispGC(interval, ttl time.Duration) wispGC {
	if interval <= 0 || ttl <= 0 {
		return nil
	}
	return &memoryWispGC{
		interval: interval,
		ttl:      ttl,
	}
}

func (m *memoryWispGC) shouldRun(now time.Time) bool {
	return now.Sub(m.lastRun) >= m.interval
}

func (m *memoryWispGC) runGC(store beads.Store, now time.Time) (int, error) {
	m.lastRun = now
	if store == nil {
		return 0, fmt.Errorf("listing closed molecules: bead store unavailable")
	}

	entries, err := closedWispGCEntries(store)
	if err != nil {
		return 0, err
	}

	cutoff := now.Add(-m.ttl)
	purged, deleteErr := purgeExpiredBeadClosures(store, entries, cutoff)

	trackEntries, trackErr := closedOperationalChurnGCEntries(store)
	if trackErr == nil {
		trackPurged, trackDeleteErr := purgeExpiredOperationalChurnRoots(store, trackEntries, cutoff)
		purged += trackPurged
		deleteErr = errors.Join(deleteErr, trackDeleteErr)
	} else {
		deleteErr = errors.Join(deleteErr, fmt.Errorf("listing closed operational churn beads: %w", trackErr))
	}

	return purged, deleteErr
}

func closedWispGCEntries(store beads.Store) ([]beads.Bead, error) {
	entries := make([]beads.Bead, 0)
	seen := make(map[string]struct{})
	appendUnique := func(items []beads.Bead) {
		for _, item := range items {
			if item.ID == "" {
				continue
			}
			if _, ok := seen[item.ID]; ok {
				continue
			}
			seen[item.ID] = struct{}{}
			entries = append(entries, item)
		}
	}
	molecules, err := store.List(beads.ListQuery{Status: "closed", Type: "molecule"})
	if err != nil {
		return nil, fmt.Errorf("listing closed molecule roots: %w", err)
	}
	appendUnique(molecules)
	wisps, err := store.List(beads.ListQuery{Status: "closed", Metadata: map[string]string{"gc.kind": "wisp"}})
	if err != nil {
		return nil, fmt.Errorf("listing closed wisp roots: %w", err)
	}
	appendUnique(wisps)
	return entries, nil
}

func closedOperationalChurnGCEntries(store beads.Store) ([]beads.Bead, error) {
	entries := make([]beads.Bead, 0)
	seen := make(map[string]struct{})
	appendUnique := func(items []beads.Bead) {
		for _, item := range items {
			if item.ID == "" || !isOperationalChurnBead(item) {
				continue
			}
			if _, ok := seen[item.ID]; ok {
				continue
			}
			seen[item.ID] = struct{}{}
			entries = append(entries, item)
		}
	}
	labeled, err := store.List(beads.ListQuery{Status: "closed", Label: labelOrderTracking, TierMode: beads.TierBoth})
	if err != nil {
		return nil, fmt.Errorf("listing closed order-tracking beads: %w", err)
	}
	appendUnique(labeled)
	retained, err := store.List(beads.ListQuery{Status: "closed", Metadata: map[string]string{retentionClassMetadataKey: operationalChurnRetentionClass}, TierMode: beads.TierBoth})
	if err != nil {
		return nil, fmt.Errorf("listing closed operational-retention beads: %w", err)
	}
	appendUnique(retained)
	return entries, nil
}

func isOperationalChurnBead(b beads.Bead) bool {
	if b.Metadata[retentionClassMetadataKey] == operationalChurnRetentionClass {
		return true
	}
	for _, label := range b.Labels {
		if label == labelOrderTracking {
			return true
		}
	}
	return false
}

func expiredBeadReferenceTime(b beads.Bead) time.Time {
	for _, key := range []string{retentionClosedAtMetadataKey, "closed_at", "gc.hqstore.closed_at"} {
		if t, ok := parseBeadMetadataTime(b.Metadata[key]); ok {
			return t
		}
	}
	return b.CreatedAt
}

func parseBeadMetadataTime(raw string) (time.Time, bool) {
	if raw == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		t, err := time.Parse(layout, raw)
		if err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func purgeExpiredBeadClosures(store beads.Store, entries []beads.Bead, cutoff time.Time) (int, error) {
	return purgeExpiredBeads(store, entries, cutoff, deleteExpiredBeadClosure)
}

func purgeExpiredOperationalChurnRoots(store beads.Store, entries []beads.Bead, cutoff time.Time) (int, error) {
	return purgeExpiredBeads(store, entries, cutoff, deleteExpiredOperationalChurnRoot)
}

func purgeExpiredBeads(store beads.Store, entries []beads.Bead, cutoff time.Time, deleteFn func(beads.Store, string) error) (int, error) {
	purged := 0
	var deleteErr error
	for _, entry := range entries {
		ref := expiredBeadReferenceTime(entry)
		if ref.IsZero() || !ref.Before(cutoff) {
			continue
		}
		if err := deleteFn(store, entry.ID); err != nil {
			if errors.Is(err, errExpiredBeadPreserved) {
				continue
			}
			deleteErr = errors.Join(deleteErr, fmt.Errorf("deleting expired bead %q: %w", entry.ID, err))
			continue
		}
		purged++
	}
	return purged, deleteErr
}

func deleteExpiredBeadClosure(store beads.Store, rootID string) error {
	// deleteWorkflowBead removes every dependency attached to each closure
	// member before deleting the bead. Only use the closure deleter for roots
	// whose full ownership tree is safe to collect.
	ids, err := collectExpiredBeadClosure(store, rootID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := deleteWorkflowBead(store, id); err != nil {
			return err
		}
	}
	return nil
}

func deleteExpiredOperationalChurnRoot(store beads.Store, rootID string) error {
	active, err := beadHasOpenOwnedDescendant(store, rootID)
	if err != nil {
		return err
	}
	if active {
		return errExpiredBeadPreserved
	}
	return deleteWorkflowBead(store, rootID)
}

func beadHasOpenOwnedDescendant(store beads.Store, rootID string) (bool, error) {
	seen := map[string]struct{}{}
	var visit func(string) (bool, error)
	visit = func(id string) (bool, error) {
		if id == "" {
			return false, nil
		}
		if _, ok := seen[id]; ok {
			return false, nil
		}
		seen[id] = struct{}{}

		children, err := store.List(beads.ListQuery{
			ParentID:      id,
			IncludeClosed: true,
			TierMode:      beads.TierBoth,
		})
		if err != nil {
			return false, fmt.Errorf("list children for %s: %w", id, err)
		}
		for _, child := range children {
			if child.Status != "closed" {
				return true, nil
			}
			active, err := visit(child.ID)
			if active || err != nil {
				return active, err
			}
		}

		upDeps, err := store.DepList(id, "up")
		if err != nil {
			return false, fmt.Errorf("list dependents for %s: %w", id, err)
		}
		for _, dep := range upDeps {
			if dep.Type != "parent-child" || dep.IssueID == "" {
				continue
			}
			dependent, err := store.Get(dep.IssueID)
			if err != nil {
				return false, fmt.Errorf("get dependent %s: %w", dep.IssueID, err)
			}
			if dependent.Status != "closed" {
				return true, nil
			}
			active, err := visit(dep.IssueID)
			if active || err != nil {
				return active, err
			}
		}
		return false, nil
	}
	return visit(rootID)
}

func collectExpiredBeadClosure(store beads.Store, rootID string) ([]string, error) {
	if store == nil {
		return nil, fmt.Errorf("bead store unavailable")
	}
	rootOwned := make([]string, 0, 4)
	related, err := store.List(beads.ListQuery{
		Metadata:      map[string]string{"gc.root_bead_id": rootID},
		IncludeClosed: true,
	})
	if err != nil {
		return nil, fmt.Errorf("list workflow-owned beads for %s: %w", rootID, err)
	}
	for _, bead := range related {
		if bead.ID != "" && bead.ID != rootID {
			rootOwned = append(rootOwned, bead.ID)
		}
	}

	seen := make(map[string]struct{}, len(rootOwned)+1)
	ids := make([]string, 0, len(rootOwned)+1)
	var visit func(string) error
	visit = func(id string) error {
		if id == "" {
			return nil
		}
		if _, ok := seen[id]; ok {
			return nil
		}
		seen[id] = struct{}{}

		if id == rootID {
			for _, relatedID := range rootOwned {
				if err := visit(relatedID); err != nil {
					return err
				}
			}
		}

		// Treat structural parentage as workflow ownership. Some molecule step
		// beads are linked only by ParentID / parent-child deps and do not carry
		// gc.root_bead_id metadata, so GC must follow those ownership edges while
		// still ignoring non-ownership deps such as blocks or waits-for.
		children, err := store.Children(id, beads.IncludeClosed)
		if err != nil {
			return fmt.Errorf("list children for %s: %w", id, err)
		}
		for _, child := range children {
			if err := visit(child.ID); err != nil {
				return err
			}
		}

		upDeps, err := store.DepList(id, "up")
		if err != nil {
			return fmt.Errorf("list dependents for %s: %w", id, err)
		}
		for _, dep := range upDeps {
			if dep.Type != "parent-child" || dep.IssueID == "" {
				continue
			}
			if err := visit(dep.IssueID); err != nil {
				return err
			}
		}

		ids = append(ids, id)
		return nil
	}
	if err := visit(rootID); err != nil {
		return nil, err
	}
	return ids, nil
}
