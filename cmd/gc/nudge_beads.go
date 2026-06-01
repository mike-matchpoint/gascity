package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/nudgequeue"
)

const (
	nudgeBeadType    = "chore"
	nudgeBeadLabel   = "gc:nudge"
	nudgeLookupLimit = nudgequeue.NudgeLookupLimit

	// nudgeEnqueueRollbackCloseReason is the close_reason metadata value
	// stamped on partially-created nudge beads when enqueueQueuedNudgeWithStore's
	// withNudgeQueueState transaction returns an error after the backing
	// bead was successfully created. The rollback path closes the bead through
	// RuntimeCloseAll, which forwards close_reason as `bd close --reason`.
	// Without this stamp, cities running with validation.on-close=error reject
	// the rollback close and the bead leaks open with metadata.state="queued".
	// The 42-character form satisfies the >=20 char validator floor.
	nudgeEnqueueRollbackCloseReason = "nudge rollback: enqueue transaction failed"
)

type nudgeReference = nudgequeue.Reference

func openNudgeBeadStore(cityPath string) beads.Store {
	store, err := openCityStoreAt(cityPath)
	if err != nil {
		return nil
	}
	return store
}

//nolint:unparam // Existing tests exercise lookup behavior with one stable nudge ID.
func findQueuedNudgeBead(store beads.Store, nudgeID string) (beads.Bead, bool, error) {
	return findNudgeBead(store, nudgeID, false)
}

func findAnyQueuedNudgeBead(store beads.Store, nudgeID string) (beads.Bead, bool, error) {
	return findNudgeBead(store, nudgeID, true)
}

func findNudgeBead(store beads.Store, nudgeID string, includeClosed bool) (beads.Bead, bool, error) {
	return findNudgeBeadWithList(context.Background(), store, nudgeID, includeClosed, func(_ context.Context, store beads.Store, query beads.ListQuery) ([]beads.Bead, error) {
		return store.List(query)
	})
}

func findQueuedNudgeBeadRuntime(ctx context.Context, store beads.Store, nudgeID, caller string) (beads.Bead, bool, error) {
	return findNudgeBeadRuntime(ctx, store, nudgeID, false, caller)
}

func findAnyQueuedNudgeBeadRuntime(ctx context.Context, store beads.Store, nudgeID, caller string) (beads.Bead, bool, error) {
	return findNudgeBeadRuntime(ctx, store, nudgeID, true, caller)
}

func findNudgeBeadRuntime(ctx context.Context, store beads.Store, nudgeID string, includeClosed bool, caller string) (beads.Bead, bool, error) {
	return findNudgeBeadWithList(ctx, store, nudgeID, includeClosed, func(ctx context.Context, store beads.Store, query beads.ListQuery) ([]beads.Bead, error) {
		policy := beads.RuntimeReadPolicy(beads.ReadClassHotAuthoritative, caller)
		if query.Limit > 0 {
			policy.MaxRows = query.Limit + 1
		}
		return beads.RuntimeList(ctx, store, query, policy)
	})
}

func findNudgeBeadWithList(ctx context.Context, store beads.Store, nudgeID string, includeClosed bool, list func(context.Context, beads.Store, beads.ListQuery) ([]beads.Bead, error)) (beads.Bead, bool, error) {
	if store == nil || nudgeID == "" {
		return beads.Bead{}, false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	opts := []beads.QueryOpt(nil)
	if includeClosed {
		opts = append(opts, beads.IncludeClosed)
	}
	items, err := list(ctx, store, beads.ListQuery{
		Label:         "nudge:" + nudgeID,
		IncludeClosed: beads.HasOpt(opts, beads.IncludeClosed),
		Limit:         nudgeLookupLimit + 1,
		Sort:          beads.SortCreatedDesc,
	})
	if err != nil {
		return beads.Bead{}, false, err
	}
	capped := len(items) > nudgeLookupLimit
	var fallback beads.Bead
	hasFallback := false
	for _, item := range items {
		if !nudgeBeadMatches(item, nudgeID, includeClosed) {
			continue
		}
		if item.Status != "closed" {
			return item, true, nil
		}
		if !includeClosed {
			continue
		}
		if isTerminalNudgeState(item.Metadata["state"]) {
			return item, true, nil
		}
		if !capped && !hasFallback {
			fallback = item
			hasFallback = true
		}
	}
	if capped {
		return beads.Bead{}, false, beads.LookupLimitError{Kind: "nudge", Label: "nudge:" + nudgeID, Limit: nudgeLookupLimit}
	}
	if includeClosed && hasFallback {
		return fallback, true, nil
	}
	return beads.Bead{}, false, nil
}

func ensureQueuedNudgeBead(store beads.Store, item queuedNudge) (string, bool, error) {
	return ensureQueuedNudgeBeadRuntime(context.Background(), store, item)
}

func ensureQueuedNudgeBeadRuntime(ctx context.Context, store beads.Store, item queuedNudge) (string, bool, error) {
	if store == nil {
		return "", false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	beadID := nudgeAuditBeadIDForStore(store, item.ID)
	if beadID != "" {
		existing, err := beads.RuntimeGet(ctx, store, beadID, beads.RuntimeReadPolicy(beads.ReadClassHotAuthoritative, "nudge.ensure.get"))
		if err == nil {
			if nudgeBeadMatches(existing, item.ID, false) {
				return existing.ID, false, nil
			}
			return "", false, fmt.Errorf("nudge audit bead id %q exists but does not match queued nudge %q", beadID, item.ID)
		}
		if !errors.Is(err, beads.ErrNotFound) {
			if !beads.IsDegradedRead(err) {
				return "", false, err
			}
		}
	}
	if existing, ok, err := findQueuedNudgeBeadRuntime(ctx, store, item.ID, "nudge.ensure.find"); err != nil {
		if beads.IsLookupLimitError(err) || !beads.IsDegradedRead(err) {
			return "", false, err
		}
	} else if ok {
		return existing.ID, false, nil
	}
	meta := map[string]string{
		"nudge_id":           item.ID,
		"gc.idempotency_key": "nudge:" + item.ID,
		"agent":              item.Agent,
		"session_id":         item.SessionID,
		"continuation_epoch": item.ContinuationEpoch,
		"state":              "queued",
		"source":             item.Source,
		"message":            item.Message,
		"deliver_after":      item.DeliverAfter.UTC().Format(time.RFC3339),
		"expires_at":         item.ExpiresAt.UTC().Format(time.RFC3339),
		"reference_json":     marshalNudgeReference(item.Reference),
		"last_attempt_at":    formatOptionalTime(item.LastAttemptAt),
		"last_error":         item.LastError,
		"terminal_reason":    "",
		"commit_boundary":    "",
		"terminal_at":        "",
	}
	bead := beads.Bead{
		ID:    beadID,
		Title: "nudge:" + item.ID,
		Type:  nudgeBeadType,
		Labels: []string{
			nudgeBeadLabel,
			"agent:" + item.Agent,
			"nudge:" + item.ID,
			"source:" + item.Source,
		},
		Metadata: meta,
	}
	policy := nudgeRuntimeWritePolicy("nudge.ensure.create", item.ID)
	created, err := beads.RuntimeCreate(ctx, store, bead, policy)
	if err != nil {
		if existing, ok, findErr := reconcileQueuedNudgeCreate(ctx, store, item.ID); findErr == nil && ok {
			return existing.ID, false, nil
		}
		return "", false, err
	}
	return created.ID, true, nil
}

func markQueuedNudgeTerminal(store beads.Store, item queuedNudge, state, reason, commitBoundary string, now time.Time) error {
	return markQueuedNudgeTerminalRuntime(context.Background(), store, item, state, reason, commitBoundary, now)
}

func markQueuedNudgeTerminalRuntime(ctx context.Context, store beads.Store, item queuedNudge, state, reason, commitBoundary string, now time.Time) error {
	if store == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	update := map[string]string{
		"state":           state,
		"last_attempt_at": formatOptionalTime(item.LastAttemptAt),
		"last_error":      item.LastError,
		"terminal_reason": reason,
		"commit_boundary": commitBoundary,
		"terminal_at":     now.UTC().Format(time.RFC3339),
		"close_reason":    nudgeCanonicalCloseReason(state),
	}

	tryTerminalize := func(existing beads.Bead) error {
		if existing.ID == "" {
			return beads.ErrNotFound
		}
		if !nudgeBeadMatches(existing, item.ID, true) {
			return beads.ErrNotFound
		}
		if existing.Status == "closed" && isTerminalNudgeState(existing.Metadata["state"]) {
			return nil
		}
		if _, err := nudgeRuntimeCloseAll(ctx, store, []string{existing.ID}, update, nudgeRuntimeWritePolicy("nudge.terminal.close", item.ID+":"+existing.ID+":"+state)); err != nil {
			if isMissingQueuedNudgeBeadErr(err, existing.ID) {
				return beads.ErrNotFound
			}
			if terminal, reconcileErr := nudgeRuntimeTerminalized(ctx, store, item.ID, existing.ID, state); reconcileErr == nil && terminal {
				return nil
			}
			return err
		}
		return nil
	}

	if strings.TrimSpace(item.BeadID) != "" {
		existing, err := beads.RuntimeGet(ctx, store, item.BeadID, beads.RuntimeReadPolicy(beads.ReadClassHotAuthoritative, "nudge.terminal.get"))
		if err == nil {
			if err := tryTerminalize(existing); err == nil {
				return nil
			} else if !errors.Is(err, beads.ErrNotFound) {
				return err
			}
		} else if !errors.Is(err, beads.ErrNotFound) && !isMissingQueuedNudgeBeadErr(err, item.BeadID) && !beads.IsDegradedRead(err) {
			return err
		}
	}

	b, ok, err := findAnyQueuedNudgeBeadRuntime(ctx, store, item.ID, "nudge.terminal.find-fallback")
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if err := tryTerminalize(b); err != nil && !errors.Is(err, beads.ErrNotFound) {
		return err
	}
	return nil
}

func rollbackQueuedNudgeBeadRuntime(store beads.Store, beadID string) error {
	if store == nil || strings.TrimSpace(beadID) == "" {
		return nil
	}
	metadata := map[string]string{
		"state":           "failed",
		"terminal_reason": nudgeEnqueueRollbackCloseReason,
		"terminal_at":     time.Now().UTC().Format(time.RFC3339),
		"close_reason":    nudgeEnqueueRollbackCloseReason,
	}
	if _, err := nudgeRuntimeCloseAll(context.Background(), store, []string{beadID}, metadata, nudgeRuntimeWritePolicy("nudge.enqueue.rollback", beadID)); err != nil {
		return fmt.Errorf("closing rollback nudge bead %q: %w", beadID, err)
	}
	return nil
}

func nudgeRuntimeCloseAll(ctx context.Context, store beads.Store, ids []string, metadata map[string]string, policy beads.WritePolicy) (int, error) {
	return beads.RuntimeCloseAll(ctx, store, ids, metadata, policy)
}

func nudgeRuntimeWritePolicy(caller, idempotencyKey string) beads.WritePolicy {
	return beads.RuntimeWritePolicy(beads.WriteClassHotState, caller, idempotencyKey)
}

func reconcileQueuedNudgeCreate(ctx context.Context, store beads.Store, nudgeID string) (beads.Bead, bool, error) {
	deadline := time.Now().Add(2 * time.Second)
	var lastErr error
	for {
		existing, ok, err := findQueuedNudgeBeadRuntime(ctx, store, nudgeID, "nudge.ensure.reconcile-create")
		if err == nil && ok {
			return existing, true, nil
		}
		if err != nil {
			lastErr = err
		}
		if time.Now().After(deadline) {
			return beads.Bead{}, false, lastErr
		}
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return beads.Bead{}, false, lastErr
			}
			return beads.Bead{}, false, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func nudgeRuntimeTerminalized(ctx context.Context, store beads.Store, nudgeID, beadID, wantState string) (bool, error) {
	existing, err := beads.RuntimeGet(ctx, store, beadID, beads.RuntimeReadPolicy(beads.ReadClassHotAuthoritative, "nudge.terminal.reconcile-get"))
	if err == nil {
		return nudgeTerminalStateMatches(existing, nudgeID, wantState), nil
	}
	if !errors.Is(err, beads.ErrNotFound) && !isMissingQueuedNudgeBeadErr(err, beadID) && !beads.IsDegradedRead(err) {
		return false, err
	}
	found, ok, findErr := findAnyQueuedNudgeBeadRuntime(ctx, store, nudgeID, "nudge.terminal.reconcile-find")
	if findErr != nil {
		return false, findErr
	}
	if !ok || found.ID != beadID {
		return false, nil
	}
	return nudgeTerminalStateMatches(found, nudgeID, wantState), nil
}

func nudgeTerminalStateMatches(b beads.Bead, nudgeID, wantState string) bool {
	if !nudgeBeadMatches(b, nudgeID, true) {
		return false
	}
	if b.Status != "closed" {
		return false
	}
	if strings.TrimSpace(wantState) == "" {
		return isTerminalNudgeState(b.Metadata["state"])
	}
	return strings.TrimSpace(b.Metadata["state"]) == strings.TrimSpace(wantState)
}

type nudgeStoreIDPrefixer interface {
	IDPrefix() string
}

func nudgeAuditBeadIDForStore(store beads.Store, nudgeID string) string {
	nudgeID = strings.TrimSpace(nudgeID)
	if nudgeID == "" {
		return ""
	}
	prefix := nudgeStoreIDPrefix(store)
	if prefix == "" {
		return nudgeID
	}
	if strings.HasPrefix(strings.ToLower(nudgeID), prefix+"-") {
		return nudgeID
	}
	sum := sha256.Sum256([]byte(nudgeID))
	return prefix + "-nudge-" + hex.EncodeToString(sum[:8])
}

func nudgeStoreIDPrefix(store beads.Store) string {
	prefixer, ok := store.(nudgeStoreIDPrefixer)
	if !ok || prefixer == nil {
		return ""
	}
	return strings.Trim(strings.ToLower(strings.TrimSpace(prefixer.IDPrefix())), "-")
}

func nudgeBeadMatches(b beads.Bead, nudgeID string, includeClosed bool) bool {
	if strings.TrimSpace(b.ID) == "" || strings.TrimSpace(nudgeID) == "" {
		return false
	}
	if b.Type != "" && b.Type != nudgeBeadType {
		return false
	}
	if !includeClosed && b.Status == "closed" {
		return false
	}
	if !includeClosed && isTerminalNudgeState(b.Metadata["state"]) {
		return false
	}
	if strings.TrimSpace(b.Metadata["nudge_id"]) != strings.TrimSpace(nudgeID) {
		return false
	}
	if len(b.Labels) > 0 {
		if !nudgeHasLabel(b.Labels, nudgeBeadLabel) {
			return false
		}
		if !nudgeHasLabel(b.Labels, "nudge:"+nudgeID) {
			return false
		}
	}
	return true
}

func nudgeHasLabel(labels []string, want string) bool {
	for _, label := range labels {
		if label == want {
			return true
		}
	}
	return false
}

// nudgeCanonicalCloseReason maps a nudge queue terminalization state code
// to a human-readable close_reason of at least 20 characters, suitable for
// use as `bd close --reason` under validation.on-close=error.
//
// markQueuedNudgeTerminal stamps the result in metadata.close_reason
// before invoking RuntimeCloseAll. BdStore's runtime close forwards
// metadata.close_reason as the --reason argument, which allows cities running
// with validation.on-close=error to accept the close.
// Without the canonical reason, the validator rejects close calls with
// reason <20 chars, the close fails, the entire withNudgeQueueState
// transaction rolls back, and the nudge bounces between InFlight and
// Pending forever (one bead.updated event per claim attempt) until
// expires_at cuts in.
//
// Unknown codes fall back to a descriptive phrase that remains >=20
// characters after bd's validator trims whitespace. Codes already 20+
// chars pass through unchanged.
func nudgeCanonicalCloseReason(stateCode string) string {
	switch stateCode {
	case "failed":
		return "nudge failed: queue terminalization rejected delivery"
	case "expired":
		return "nudge expired past deliver-by deadline"
	case "superseded":
		return "nudge superseded by newer queued entry"
	case "injected":
		return "nudge delivered via provider injection"
	case "accepted_for_injection":
		return "nudge accepted for hook-transport injection"
	}
	if len(stateCode) >= 20 {
		return stateCode
	}
	if stateCode == "" {
		return "nudge terminalized: unknown-state"
	}
	return "nudge terminalized: " + stateCode
}

func isMissingQueuedNudgeBeadErr(err error, beadID string) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, beads.ErrNotFound) {
		return true
	}
	beadID = strings.ToLower(strings.TrimSpace(beadID))
	if beadID == "" {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no issue found matching "+strings.ToLower(strconv.Quote(beadID))) ||
		strings.Contains(msg, "error resolving "+beadID+": no issue found") ||
		strings.Contains(msg, "ambiguous id") ||
		strings.Contains(msg, "use more characters to disambiguate")
}

func marshalNudgeReference(ref *nudgeReference) string {
	if ref == nil {
		return ""
	}
	data, err := json.Marshal(ref)
	if err != nil {
		return ""
	}
	return string(data)
}

func formatOptionalTime(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.UTC().Format(time.RFC3339)
}
