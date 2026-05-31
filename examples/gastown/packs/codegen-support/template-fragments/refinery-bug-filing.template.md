{{ define "refinery-bug-filing" }}
---

## NON-LANDING BUG FILING — SCHEMA-ENFORCED

**Single-purpose fragment.** This section defines how you file a bug
bead for the debugger when a test failure is confirmed pre-existing
on the target branch, or when you surface any other non-landing-
failure bug. This fragment owns ONLY bug filing — no fix logic, no
merge logic, no convoy-state changes, no decisions about how to fix
the bug.

### Activation gate (MUST hold before anything below runs)

ALL of the following:

- The current failure is NOT an owned-convoy landing failure
  (`WORK.issue_type != "convoy"` OR `metadata.branch` does not
  start with `integration/`). If it IS, the
  `refinery-landing-failure-arbiter` fragment owns this path; exit
  immediately.
- Tests reproduced the failure on the target branch at the
  merge-base SHA, confirming the failure is NOT caused by the
  merging branch. You verified this via:
  ```bash
  git checkout "$TARGET_TIP_SHA"
  <test command> > /tmp/target-test.log 2>&1
  TARGET_FAILED=$? ; [ "$TARGET_FAILED" -ne 0 ]
  ```
- The failure is not already represented by an open bug bead (see
  dedup below).

If any of the above is false, do NOT file. Either route through
`refinery-landing-failure-arbiter` (landing failure) or take the
normal task-rejection path (branch-caused failure).

### Required inputs (set by the surrounding refinery context)

| Variable | Source |
|---|---|
| `$GC_RIG`, `$GC_AGENT` | session env |
| `$WORK` | current work bead id |
| `$TARGET_BRANCH` | typically `main` |
| `$MERGE_BASE_SHA`, `$TARGET_TIP_SHA` | from git |
| `$COMPONENT` | derived from the failing test or affected file path |
| `$OBSERVED`, `$EXPECTED` | extracted from the test output |
| `$REPRO_CMD` | exact command that reproduces the failure |
| `$BUG_DESCRIPTION` | the long-form description (repro / scope / provenance / probable fix) |
| `$FAILURE_SIGNATURE` | top assertion message + test id, stable across re-runs |
| `$RELATED_CONVOY` | the convoy id this failure surfaced during, if any |

### Deduplication

Two-stage. Stage 1 is the strict, component-bounded query. Stage 2
drops the component filter and searches by failure-signature across
all open bugs — this catches the cross-framing duplicate case where
refinery files under `component=apps/<test-path>` and polecat would
file the same underlying failure under `component=<file-being-edited>`
(see app-1g8w/app-7nf8 deadlock 2026-05-21).

`$FAILURE_SIGNATURE` SHOULD be the tool's own error/assertion message
verbatim, NOT the pytest/just wrapper output. Wrapper differs across
invocations; tool output is stable.

```bash
COMPONENT_LABEL=$(echo "$COMPONENT" | tr '/' ':')
BUG_CLASS="pre-existing-test-failure"

# Stage 1 — strict: same component. (bug.class filter intentionally
# dropped; it caused false negatives across producers.)
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

# Stage 2 — broad: signature-only across all open bugs. Guarded on a
# sufficiently-discriminating signature length.
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
  gc --rig "$GC_RIG" bd update "$WORK" \
    --set-metadata duplicate_of="$DUPLICATE"
  return 0   # or exit this fragment's path; do NOT call gc bd create
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
  --arg target_branch "$TARGET_BRANCH" \
  --arg merge_base_sha "$MERGE_BASE_SHA" \
  --arg target_tip_sha "$TARGET_TIP_SHA" \
  --arg pre_existing_at "$(date -u +%FT%TZ)" \
  --arg related_convoy "${RELATED_CONVOY:-}" \
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
    "target_branch": $target_branch,
    "merge_base_sha": $merge_base_sha,
    "target_tip_sha": $target_tip_sha,
    "pre_existing_confirmed_at": $pre_existing_at,
    "related_convoy": $related_convoy,
    "evidence_complete": "true"
  }' | jq 'with_entries(select(.value != ""))')

LABELS="bug,source:refinery,component:${COMPONENT_LABEL},bug.class:${BUG_CLASS},bug-routing:debugger"
[ -n "$BLOCKS_CI" ] && LABELS="${LABELS},blocks:ci"
[ -n "$RELATED_CONVOY" ] && LABELS="${LABELS},blocks:convoy:${RELATED_CONVOY}"
```

### File the bug

```bash
BUG_ID=$(gc --rig "$GC_RIG" bd create \
  --type=bug \
  --priority=1 \
  --title="$TITLE" \
  --labels="$LABELS" \
  --metadata "$METADATA" \
  --description "$BUG_DESCRIPTION" \
  --json | jq -r '.id')

if [ -z "$BUG_ID" ] || [ "$BUG_ID" = "null" ]; then
  echo "FATAL: failed to create pre-existing-failure bug"
  # Do NOT block the merge — the failure is pre-existing, not caused
  # by this branch. Surface a runtime error and continue.
  exit 0
fi

echo "Pre-existing-failure bug filed: $BUG_ID"
```

### Dependency wiring — DO NOT add a dep edge to the convoy

By gate, the failure is pre-existing on the target branch — the convoy
did NOT cause it. Adding `bd dep add "$RELATED_CONVOY" "$BUG_ID"` would
formally block the convoy on a bug the convoy isn't responsible for,
falsely preventing it from landing.

```bash
# CORRECT (no-op shown for clarity): record the surfacing relationship
# in metadata and labels only. The convoy continues to land.
:  # intentionally no `bd dep add` call here

# INCORRECT (do not do this):
# gc --rig "$GC_RIG" bd dep add "$RELATED_CONVOY" "$BUG_ID"   # blocks convoy
```

The `metadata.related_convoy` field and `blocks:convoy:<id>` label are
search hints for humans triaging the bug — they are NOT formal blocking
edges. The convoy's forward motion is governed by the
`refinery-merge-close-contract`, not by this fragment.

(Contrast: `polecat-bug-filing` DOES add `bd dep add "$TASK" "$BUG_ID"`
because the polecat task surfaced a failure that blocks its own
completion. The refinery case is different by design: the surfacing
agent and the blocked work are not the same bead.)

### Verification (REQUIRED — fail closed if violated)

```bash
gc --rig "$GC_RIG" bd show "$BUG_ID" --json | jq -e --arg p "codegen-support.debugger" '
  .[0].status == "open" and
  .[0].metadata."gc.kind" == "bug" and
  .[0].metadata."gc.routed_to" == $p and
  .[0].metadata.decision_state == "pending" and
  ((.[0].metadata.component // "") != "") and
  ((.[0].metadata.observed // "") != "") and
  ((.[0].metadata.expected // "") != "")
' >/dev/null || {
  echo "BUG $BUG_ID failed schema verification"
  gc --rig "$GC_RIG" bd show "$BUG_ID" --json | jq '.[0] | {id, status, metadata}'
  exit 1
}
```

### Forbidden in this fragment

- ANY `gc.routed_to=$GC_RIG/codegen-support.landing-arbiter` write — that is the
  landing-failure path, owned by a different fragment.
- ANY convoy state mutation — the merge continues normally after
  filing.
- ANY decision about how to fix the bug — the debugger decides.
- ANY git operation other than the reproduction check in the gate.
- Setting `decision_class`, `repair_kind`, or `repair_beads` on the
  bug. These are debugger-output fields and MUST be left unset by
  producers. `decision_state` is the one debugger-vocabulary field
  producers DO set, but the only legal value is exactly `"pending"`
  (the intake signal); any other value (`in_progress`, `decided`,
  `decided_investigation`, `rerouted`, `human_review`,
  `closed-duplicate`) is reserved for the debugger to write. The
  watcher and downstream consumers depend on the debugger being the
  sole writer of decision/repair metadata; deviations deadlock the
  bug at unrecognized-state.

### Replaces the inline `gc bd create` pattern

The pre-existing-failure path in the patrol formula historically
contained an inline `gc bd create --type=bug --priority=1 --title=...`
with no metadata, labels, or routing. That pattern is deprecated by
this fragment. You MUST defer to this fragment for any
pre-existing-failure bug filing and MUST NOT emit a bare
`gc bd create --type=bug` call from the patrol formula's
`handle-failures` step. This fragment IS the authoritative
implementation; the formula's mention of "file a bead" resolves to
"run this fragment's recipe."
{{ end }}
