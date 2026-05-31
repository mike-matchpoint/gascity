{{ define "refinery-landing-failure-arbiter" }}
---

## OWNED-CONVOY LANDING FAILURE — FILE A BUG ROUTED TO THE LANDING ARBITER

**This section defines the convoy-bead rejection path that
Contract R points to for owned-convoy integration landings.** When a
rebase or merge fails on an owned-convoy integration branch in a way
the auto-resolver could not handle, you MUST file a child
landing-failure bug under the convoy and leave the convoy open,
unassigned, and blocked. NEVER write `gc.routed_to=human` on the convoy
itself — the landing-arbiter is the convoy's decision owner.

Activation: only when ALL three hold:

- The work bead's `.issue_type == "convoy"` (task beads still follow
  the normal Contract R pool-rejection path).
- The convoy's `metadata.branch` starts with `integration/` (this is
  an owned-convoy integration-branch landing, not a feature-branch
  convoy).
- The failure is irreconcilable from refinery's side: the auto-resolve
  fragment aborted, OR a merge_commit / continue_rebase retry also
  conflicted, OR a regen tool rejected the resolved source.

If any of those is false, the normal Contract R / task-bead rejection
runs unchanged.

### Required inputs (from the auto-resolve fragment's exports)

The `refinery-rebase-conflict-auto-resolve` fragment captures these
shell variables before `git rebase --abort` wipes the working tree.
You consume them here:

| Variable | Source |
|---|---|
| `$WORK` | the convoy bead id |
| `$BRANCH` | integration branch (from convoy `metadata.branch`) |
| `$TARGET` | landing target (from convoy `metadata.target`, typically `main`) |
| `$MERGE_BASE_SHA` | `git merge-base origin/$BRANCH origin/$TARGET` |
| `$TARGET_TIP_SHA` | `git rev-parse origin/$TARGET` |
| `$INTEGRATION_TIP_SHA` | `git rev-parse origin/$BRANCH` |
| `$AHEAD` | `git rev-list --count origin/$TARGET..origin/$BRANCH` |
| `$BEHIND` | `git rev-list --count origin/$BRANCH..origin/$TARGET` |
| `$ATTEMPTED_STRATEGY` | `rebase`, `merge_commit`, or `continue_rebase` |
| `$FAILURE_CLASS` | refinery's first-pass hint (see below) |
| `$CONFLICT_KIND` | `add/add`, `modify/modify`, etc. |
| `$CONFLICT_PATHS_CSV` | comma-separated conflicted file paths |
| `$CONFLICT_EXCERPTS` | the captured `<<<<<<<`/`=======`/`>>>>>>>` hunks with ~10 lines context per side |
| `$AUTO_RESOLVER_ABORTED_REASON` | which hunk classification fired |
| `$AUTO_RESOLVE_SUMMARY` | the `auto_resolve_summary` value |
| `$COMMITS_REBASED` (optional) | for partial-rebase stale-branch cases |
| `$COMMITS_REMAINING` (optional) | for partial-rebase stale-branch cases |
| `$CONFLICT_PATTERN` (optional) | e.g. `registration_surface` for stale-branch lockfile/manifest churn |

If any of `$MERGE_BASE_SHA`, `$TARGET_TIP_SHA`, `$INTEGRATION_TIP_SHA`,
or `$CONFLICT_EXCERPTS` is missing, set `$EVIDENCE_COMPLETE=false` and
`$EVIDENCE_GAPS=<csv naming what was missed>`. Do NOT skip filing —
the arbiter can route to human if evidence is too thin.

### Failure-class hint (refinery's first pass)

This is informational for the arbiter. Pick the best match from:

- `semantic_add_add` — both sides added the same path; auto-resolver
  aborted on `contract-surface-disagreement` or `both-sides-modify`.
- `semantic_modify_modify` — both sides edited overlapping bodies.
- `deletion_vs_keep` — one side deleted a line the other kept.
- `contract_surface_conflict` — shared signature / schema disagreement.
- `stale_integration_branch` — `$BEHIND` is large (≥10) AND auto-resolver
  partially succeeded then aborted on registration-surface churn.
- `mixed` — `$BEHIND` large AND at least one substantive semantic
  conflict.
- `regenerate_failure` — auto-resolver merged a manifest source but
  the regen tool (`uv lock`, etc.) rejected the result.
- `evidence_incomplete` — if `$EVIDENCE_COMPLETE=false`.

The arbiter re-classifies after gathering architectural context; this
hint just gives it a starting point.

### Idempotency pre-check

A landing-failure bug for THIS attempt may already exist (resume after
crash, formula re-entry). Compute the attempt id and check before
creating:

```bash
LANDING_ATTEMPT_ID="${WORK}:${INTEGRATION_TIP_SHA}:${TARGET_TIP_SHA}:${ATTEMPTED_STRATEGY}"

EXISTING_BUG=$(gc --rig "$GC_RIG" bd list \
  --parent "$WORK" \
  --type=bug \
  --status=open \
  --metadata-field gc.kind=owned_convoy_landing_failure \
  --metadata-field landing_attempt_id="$LANDING_ATTEMPT_ID" \
  --json | jq -r '.[0].id // empty')

if [ -n "$EXISTING_BUG" ]; then
  echo "Landing-failure bug already filed for this attempt: $EXISTING_BUG"
  BUG_ID="$EXISTING_BUG"
else
  # ... proceed to create (below)
  :
fi
```

If `$EXISTING_BUG` is set, skip the create and go straight to the
convoy update (which is also idempotent — `--set-metadata` is
no-op when the value already matches).

### Assemble the evidence metadata

```bash
METADATA=$(jq -nc \
  --arg routed_to "$GC_RIG/codegen-support.landing-arbiter" \
  --arg source_convoy "$WORK" \
  --arg source_branch "$BRANCH" \
  --arg target_branch "$TARGET" \
  --arg merge_base "$MERGE_BASE_SHA" \
  --arg target_sha "$TARGET_TIP_SHA" \
  --arg integration_sha "$INTEGRATION_TIP_SHA" \
  --arg ahead_count "$AHEAD" \
  --arg behind_count "$BEHIND" \
  --arg attempted_strategy "$ATTEMPTED_STRATEGY" \
  --arg failure_class "$FAILURE_CLASS" \
  --arg conflict_kind "$CONFLICT_KIND" \
  --arg conflict_paths "$CONFLICT_PATHS_CSV" \
  --arg conflict_excerpts "$CONFLICT_EXCERPTS" \
  --arg aborted_reason "${AUTO_RESOLVER_ABORTED_REASON:-}" \
  --arg auto_resolve_summary "${AUTO_RESOLVE_SUMMARY:-}" \
  --arg commits_rebased "${COMMITS_REBASED:-}" \
  --arg commits_remaining "${COMMITS_REMAINING:-}" \
  --arg conflict_pattern "${CONFLICT_PATTERN:-}" \
  --arg evidence_collected_at "$(date -u +%FT%TZ)" \
  --arg evidence_complete "${EVIDENCE_COMPLETE:-true}" \
  --arg evidence_gaps "${EVIDENCE_GAPS:-}" \
  --arg landing_attempt_id "$LANDING_ATTEMPT_ID" \
  '{
    "gc.kind": "owned_convoy_landing_failure",
    "gc.routed_to": $routed_to,
    "source_convoy": $source_convoy,
    "source_branch": $source_branch,
    "target_branch": $target_branch,
    "merge_base": $merge_base,
    "target_sha": $target_sha,
    "integration_sha": $integration_sha,
    "ahead_count": $ahead_count,
    "behind_count": $behind_count,
    "attempted_strategy": $attempted_strategy,
    "failure_class": $failure_class,
    "conflict_kind": $conflict_kind,
    "conflict_paths": $conflict_paths,
    "conflict_excerpts": $conflict_excerpts,
    "auto_resolver_attempted": "true",
    "auto_resolver_aborted_reason": $aborted_reason,
    "auto_resolve_summary": $auto_resolve_summary,
    "commits_rebased": $commits_rebased,
    "commits_remaining": $commits_remaining,
    "conflict_pattern": $conflict_pattern,
    "evidence_collected_at": $evidence_collected_at,
    "evidence_complete": $evidence_complete,
    "evidence_gaps": $evidence_gaps,
    "landing_attempt_id": $landing_attempt_id,
    "decision_state": "pending"
  }' | jq 'with_entries(select(.value != ""))')
```

The trailing `with_entries(select(.value != ""))` strips empty
optional fields so the bug's metadata stays tidy.

### Create the bug (if not already created)

```bash
TITLE="Landing failure: $BRANCH -> $TARGET ($FAILURE_CLASS)"
WO_ID=$(echo "$BRANCH" | sed -E 's#^integration/##')
DESCRIPTION=$(cat <<EOF
Refinery could not land owned convoy $WORK on $TARGET.

- Source branch: $BRANCH
- Target: $TARGET
- Attempted strategy: $ATTEMPTED_STRATEGY
- Refinery failure-class hint: $FAILURE_CLASS
- Auto-resolver outcome: ${AUTO_RESOLVER_ABORTED_REASON:-n/a}
- Auto-resolve summary: ${AUTO_RESOLVE_SUMMARY:-n/a}

Conflict paths:
$(echo "$CONFLICT_PATHS_CSV" | tr ',' '\\n' | sed 's/^/  - /')

Full conflict-region excerpts, SHAs, ahead/behind counts, and any
strategy-specific evidence are on this bug's metadata.

Routed to: $GC_RIG/codegen-support.landing-arbiter for classification, intent
synthesis (reads work order, AGENTS.md, ADRs), and decision
(continue_rebase / merge_commit / repair_beads / human).
EOF
)

if [ -z "$BUG_ID" ]; then
  BUG_ID=$(gc --rig "$GC_RIG" bd create \
    --type=bug \
    --priority=1 \
    --parent="$WORK" \
    --title="$TITLE" \
    --labels="owned-convoy-landing-failure,refinery-escalation,source:work-order:${WO_ID}" \
    --metadata "$METADATA" \
    --description "$DESCRIPTION" \
    --json | jq -r '.id')
fi

if [ -z "$BUG_ID" ] || [ "$BUG_ID" = "null" ]; then
  echo "FATAL: failed to create landing-failure bug for $WORK"
  echo "Leaving $WORK assigned to refinery so the next iteration retries."
  exit 1
fi

echo "Landing-failure bug filed: $BUG_ID"
```

### Update the convoy to blocked-passive state

```bash
gc --rig "$GC_RIG" bd update "$WORK" \
  --status=open \
  --assignee="" \
  --unset-metadata gc.routed_to \
  --set-metadata landing_state=blocked \
  --set-metadata merge_result=blocked \
  --set-metadata landing_decision_owner="$GC_RIG/codegen-support.landing-arbiter" \
  --set-metadata landing_failure_id="$BUG_ID" \
  --set-metadata blocked_by_bug="$BUG_ID" \
  --set-metadata blocked_reason="integration landing failed ($FAILURE_CLASS); landing-failure bug filed: $BUG_ID" \
  --set-metadata rejection_reason="integration landing failed ($FAILURE_CLASS); landing-failure bug filed: $BUG_ID"
```

### Verification (REQUIRED — fail closed if violated)

```bash
gc --rig "$GC_RIG" bd show "$BUG_ID" --json | jq -e --arg arb "$GC_RIG/codegen-support.landing-arbiter" '
  .[0].status == "open" and
  .[0].metadata."gc.kind" == "owned_convoy_landing_failure" and
  .[0].metadata."gc.routed_to" == $arb and
  .[0].metadata.decision_state == "pending" and
  ((.[0].metadata.evidence_complete // "") != "")
' >/dev/null || {
  echo "LANDING-FAILURE BUG $BUG_ID failed verification"
  gc --rig "$GC_RIG" bd show "$BUG_ID" --json | jq '.[0] | {id, status, metadata}'
  exit 1
}

gc --rig "$GC_RIG" bd show "$WORK" --json | jq -e --arg bug "$BUG_ID" '
  .[0].status == "open" and
  ((.[0].assignee // "") == "") and
  .[0].metadata.landing_state == "blocked" and
  .[0].metadata.landing_failure_id == $bug and
  (.[0].metadata."gc.routed_to" // "" | length == 0)
' >/dev/null || {
  echo "CONVOY $WORK failed post-filing verification"
  gc --rig "$GC_RIG" bd show "$WORK" --json | jq '.[0] | {id, status, assignee, metadata}'
  exit 1
}
```

### Forbidden in this path

- `gc bd update <convoy> --set-metadata gc.routed_to=human` — this is
  the dead-end pattern this architecture replaces.
- `gc bd update <convoy> --assignee=<rig>/codegen-support.landing-arbiter` — only the
  bug carries arbiter routing.
- `gc bd close <convoy>` — the convoy stays open.
- `git push origin --delete $BRANCH` — the integration branch must
  survive for the arbiter's decision and any subsequent repair work.

### Fail-closed behavior

If `gc bd create` fails (network error, validation rejection,
permission), do NOT proceed to the convoy update. Leave the convoy
open with refinery still as assignee (so the next patrol iteration
retries), emit a runtime error naming the failure, and continue past
this bead. Do NOT write `gc.routed_to=human` on the convoy as a
fallback — that is the pattern being replaced.

After successful filing, fall through to the formula's normal patrol
wrap-up (pour next wisp, burn this one). The bug is now on the
arbiter's queue; refinery is done with this convoy until the
arbiter writes back a decision.
{{ end }}
