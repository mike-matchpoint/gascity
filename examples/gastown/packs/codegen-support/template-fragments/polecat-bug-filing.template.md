{{ define "polecat-bug-filing" }}
---

## OUT-OF-SCOPE BUG FILING — SCHEMA-ENFORCED

**Single-purpose fragment.** This section defines how you file a bug
bead for the debugger when you encounter a runtime, lint, or test
failure that is out of scope for your current task. This fragment
owns ONLY bug filing — no fix attempts, no merge logic, no decision
about resolution class.

### Activation gate (MUST hold before anything below runs)

ALL of the following:

- The failure is NOT caused by your own code changes. Verify by:
  - Stashing or reverting in-progress edits, OR
  - Checking out the task's parent SHA, AND
  - Re-running the failure command.
  If the failure persists, it is independent of your work. If it
  disappears, it is your own issue — fix it in the current task.
- The failure is NOT addressable within your current task scope
  (e.g., the failure is in `module_b.py` while your task scope is
  `module_a.py`).
- The failure is not already represented by an open bug bead (see
  dedup below).

If any of the above is false, do NOT file. Either fix the failure
in the current task OR close the current task with
`rejection_reason` explaining why the failure blocks completion.

### Required inputs

| Variable | Source |
|---|---|
| `$GC_RIG`, `$GC_AGENT` | session env |
| `$TASK` | current task bead id |
| `$ENCOUNTERED_BRANCH` | your current branch (typically `polecat/$TASK`) |
| `$COMPONENT` | path of the failing module (NOT your task's component) |
| `$BUG_CLASS` | from the enum: `runtime_failure`, `test_failure`, `regression`, `contract_violation` |
| `$OBSERVED`, `$EXPECTED` | extracted from the failure |
| `$REPRO_CMD` | exact command that reproduces the failure |
| `$BUG_DESCRIPTION` | the long-form description |
| `$FAILURE_SIGNATURE` | top stack frame + error message + test id, stable across re-runs |
| `$OUT_OF_SCOPE_REASON` | one-line explanation of why this failure isn't addressable in your current task |

### Deduplication

Two-stage. Stage 1 is the strict, component-bounded query (catches
obvious dups quickly). Stage 2 drops the component filter and searches
by failure-signature across all open bugs (catches conceptually-
identical failures that producers framed under different components —
e.g., refinery files `component=apps/infrastructure:import-linter`
while polecat would file the same failure as `component=pyproject.toml`).

`$FAILURE_SIGNATURE` SHOULD be the tool's own error message verbatim
(e.g., the lint-imports / pytest assertion / type-checker output),
NOT the framework wrapper output. Wrapper text differs between
`just lint` and a direct invocation; the tool's message is stable.

```bash
COMPONENT_LABEL=$(echo "$COMPONENT" | tr '/' ':')

# Stage 1 — strict: same component + same class.
# (bug.class is intentionally dropped to reduce false negatives; two
# producers framing the same failure with different classes were a
# real-world deadlock — see app-1g8w/app-7nf8 2026-05-21.)
DUPLICATE=$(gc --rig "$GC_RIG" bd list \
  --type=bug --status=open \
  --label "bug" \
  --label "component:$COMPONENT_LABEL" \
  --json | jq -r --arg sig "$FAILURE_SIGNATURE" '
    .[] | select(
      ((.title // "") | contains($sig))
      or ((.metadata.observed // "") | contains($sig))
    ) | .id // empty
  ' | head -1)

# Stage 2 — broad: signature-only across all open bugs. Run when stage
# 1 missed AND the signature is specific enough to use as a search key
# (>= 30 chars filters out generic strings that would false-positive).
if [ -z "$DUPLICATE" ] && [ "${#FAILURE_SIGNATURE}" -ge 30 ]; then
  DUPLICATE=$(gc --rig "$GC_RIG" bd list \
    --type=bug --status=open --label "bug" \
    --json | jq -r --arg sig "$FAILURE_SIGNATURE" '
      .[] | select(
        ((.title // "") | contains($sig))
        or ((.metadata.observed // "") | contains($sig))
      ) | .id // empty
    ' | head -1)
fi

if [ -n "$DUPLICATE" ]; then
  echo "Duplicate of $DUPLICATE; not refiling."
  # Same blocking discipline as the new-bug path: the duplicate is the
  # blocker regardless of who originally filed it.
  gc --rig "$GC_RIG" bd dep add "$TASK" "$DUPLICATE"
  gc --rig "$GC_RIG" bd update "$TASK" \
    --status=blocked \
    --set-metadata blocked_by_bug="$DUPLICATE" \
    --notes "Blocked on bug $DUPLICATE (duplicate of encountered failure): $OUT_OF_SCOPE_REASON"
  return 0
fi
```

### Assemble metadata and labels

```bash
METADATA=$(jq -nc \
  --arg routed_to "codegen-support.debugger" \
  --arg component "$COMPONENT" \
  --arg observed "$OBSERVED" \
  --arg expected "$EXPECTED" \
  --arg bug_class "$BUG_CLASS" \
  --arg filed_by "$GC_AGENT" \
  --arg filed_at "$(date -u +%FT%TZ)" \
  --arg repro "$REPRO_CMD" \
  --arg encountered_bead "$TASK" \
  --arg encountered_branch "$ENCOUNTERED_BRANCH" \
  --arg oos_reason "$OUT_OF_SCOPE_REASON" \
  '{
    "gc.kind": "bug",
    "gc.routed_to": $routed_to,
    "decision_state": "pending",
    "bug.class": $bug_class,
    "component": $component,
    "observed": $observed,
    "expected": $expected,
    "filed_by": $filed_by,
    "filed_at": $filed_at,
    "repro_command": $repro,
    "encountered_during_bead": $encountered_bead,
    "encountered_branch": $encountered_branch,
    "out_of_scope_reason": $oos_reason,
    "evidence_complete": "true"
  }' | jq 'with_entries(select(.value != ""))')

LABELS="bug,source:polecat,component:${COMPONENT_LABEL},bug.class:${BUG_CLASS},bug-routing:debugger"
```

### File the bug

```bash
BUG_ID=$(gc --rig "$GC_RIG" bd create \
  --type=bug \
  --priority=2 \
  --title="$TITLE" \
  --labels="$LABELS" \
  --metadata "$METADATA" \
  --description "$BUG_DESCRIPTION" \
  --json | jq -r '.id')

if [ -z "$BUG_ID" ] || [ "$BUG_ID" = "null" ]; then
  echo "FATAL: failed to file OOS bug; continuing with current task"
  exit 0
fi

# Block the encountering task on the new bug. The activation gate
# already established that the failure is out-of-scope and reproducible
# independent of this task's edits; the task cannot complete until the
# bug is resolved. The formal `bd dep add` edge means the close of the
# bug auto-unblocks this task — no manual reconciliation needed.
gc --rig "$GC_RIG" bd dep add "$TASK" "$BUG_ID"
gc --rig "$GC_RIG" bd update "$TASK" \
  --status=blocked \
  --set-metadata encountered_bug="$BUG_ID" \
  --set-metadata blocked_by_bug="$BUG_ID" \
  --notes "Blocked on bug $BUG_ID: $OUT_OF_SCOPE_REASON"

echo "OOS bug filed: $BUG_ID; task $TASK blocked on it."
```

### Verification (REQUIRED — fail closed if violated)

```bash
gc --rig "$GC_RIG" bd show "$BUG_ID" --json | jq -e --arg p "codegen-support.debugger" '
  .[0].status == "open" and
  .[0].metadata."gc.kind" == "bug" and
  .[0].metadata."gc.routed_to" == $p and
  .[0].metadata.decision_state == "pending" and
  ((.[0].metadata.component // "") != "") and
  ((.[0].metadata.observed // "") != "")
' >/dev/null || {
  echo "BUG $BUG_ID failed schema verification"
  gc --rig "$GC_RIG" bd show "$BUG_ID" --json | jq '.[0] | {id, status, metadata}'
  exit 1
}

# The block contract: the task must carry a formal dep edge to the bug
# AND be in status=blocked. Without the edge, close-of-bug won't
# auto-unblock the task and the work goes orphan.
gc --rig "$GC_RIG" bd show "$TASK" --json | jq -e --arg b "$BUG_ID" '
  .[0].status == "blocked" and
  ([.[0].dependencies[]? | .id] | any(. == $b))
' >/dev/null || {
  echo "TASK $TASK blocking on $BUG_ID failed verification"
  gc --rig "$GC_RIG" bd show "$TASK" --json | jq '.[0] | {id, status, dependencies, metadata}'
  exit 1
}
```

### Forbidden in this fragment

- Attempting to fix the failure. That is the debugger's job to
  scope, not yours.
- Modifying your task scope. The failure is by definition
  out-of-scope (per the activation gate); the task is blocked on the
  bug rather than expanded to cover it.
- Leaving the task in `in_progress` after filing. This fragment owns
  the transition to `status=blocked` + dep edge; skipping either
  half leaves the task in an inconsistent state where the
  reconciler still treats it as active work.
- Filing more than one bug per failure (the dedup gate prevents
  this).
- Any `gc.routed_to=$GC_RIG/codegen-support.landing-arbiter` write — polecat does
  not surface landing failures.
- Setting `decision_class`, `repair_kind`, or `repair_beads` on the
  bug. These are debugger-output fields and MUST be left unset by
  producers. `decision_state` is the one debugger-vocabulary field
  producers DO set, but the only legal value is exactly `"pending"`
  (the intake signal); any other value (`in_progress`, `decided`,
  `decided_investigation`, `rerouted`, `human_review`, `duplicate`)
  is reserved for the debugger to write. The watcher and downstream
  consumers depend on the debugger being the sole writer of
  decision/repair metadata; deviations deadlock the bug at
  unrecognized-state.
{{ end }}
