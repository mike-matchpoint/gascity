{{ define "refinery-arbiter-decision-consumer" }}
---

## ARBITER-DECISION CONSUMER — READ landing_strategy BEFORE REBASING

**This section runs BEFORE the formula's normal `rebase` step.** When
you pick up a convoy bead with `landing_state=queued`, you MUST inspect
`metadata.landing_strategy` and dispatch to the correct landing recipe
before falling into the default rebase path. This is the inverse of
the landing-failure-arbiter fragment: that fragment wrote the strategy;
this fragment reads it.

Activation: only when ALL three hold:

- The work bead is a convoy (`.issue_type == "convoy"`).
- `.metadata.landing_state == "queued"`.
- `.metadata.branch` starts with `integration/`.

If any is false, fall through to the formula's normal rebase step.

### Read the strategy

```bash
LANDING_STRATEGY=$(gc --rig "$GC_RIG" bd show "$WORK" --json \
  | jq -r '.[0].metadata.landing_strategy // ""')
LANDING_ATTEMPTS=$(gc --rig "$GC_RIG" bd show "$WORK" --json \
  | jq -r '.[0].metadata.landing_attempts // "1"')
LANDING_FAILURE_ID=$(gc --rig "$GC_RIG" bd show "$WORK" --json \
  | jq -r '.[0].metadata.landing_failure_id // ""')
```

`$LANDING_STRATEGY` is one of:

- empty / unset: first attempt or post-repair retry — run the
  normal rebase path. (The handoff scan stamps `landing_state=queued`
  on every reassignment; the strategy is only present after the
  arbiter has decided.)
- `continue_rebase`: re-run the normal rebase path with
  `auto_resolve_attempted` reset so the auto-resolver gets another
  shot.
- `merge_commit`: switch to the merge-commit landing recipe below.

### Strategy = continue_rebase

The arbiter judged the prior failure mechanical / racy / stale. Reset
the auto-resolve flag and re-fetch:

```bash
gc --rig "$GC_RIG" bd update "$WORK" \
  --unset-metadata auto_resolve_attempted \
  --unset-metadata auto_resolved \
  --unset-metadata auto_resolve_summary \
  --unset-metadata landing_strategy \
  --notes "Refinery re-attempting per arbiter decision: continue_rebase"
git fetch --prune origin
```

Then run the formula's normal `rebase` step unchanged. If the rebase
fails again, the auto-resolve and landing-failure-arbiter fragments
will fire as if from scratch — except the new bug will record
`attempted_strategy=continue_rebase` and the arbiter will treat the
recurring failure differently than the first one.

### Strategy = merge_commit

The arbiter judged the divergence whole-branch-mechanical with no
unresolved semantic conflicts. Refinery performs an explicit
no-fast-forward merge commit instead of rebase.

**Preconditions (verify before any git operation):**

```bash
BRANCH=$(gc --rig "$GC_RIG" bd show "$WORK" --json | jq -r '.[0].metadata.branch')
TARGET=$(gc --rig "$GC_RIG" bd show "$WORK" --json | jq -r '.[0].metadata.target')

# 1. All children must be closed (owned convoy invariant).
OPEN_CHILDREN=$(gc --rig "$GC_RIG" bd list --all --parent "$WORK" --json --limit 0 \
  | jq '[.[] | select(.status != "closed")] | length')
if [ "$OPEN_CHILDREN" -ne 0 ]; then
  echo "REFUSE merge_commit: $WORK has $OPEN_CHILDREN open child(ren); re-queue refused"
  exit 1
fi

# 2. Convoy must be owned (label must include "owned").
OWNED=$(gc --rig "$GC_RIG" bd show "$WORK" --json | jq -r '.[0].labels[]' | grep -c '^owned$' || true)
if [ "$OWNED" -eq 0 ]; then
  echo "REFUSE merge_commit: $WORK is not an owned convoy"
  exit 1
fi
```

**Fetch and check out an isolated landing surface at the target tip:**

```bash
git fetch --prune origin
git checkout "$TARGET"
git reset --hard "origin/$TARGET"
```

**Perform the merge commit:**

```bash
MERGE_MSG="Merge integration branch '$BRANCH' (convoy $WORK; arbiter decision: merge_commit)

Landing strategy: merge_commit
Convoy: $WORK
Source: $BRANCH
Landing-failure bug: $LANDING_FAILURE_ID"

if ! git merge --no-ff "origin/$BRANCH" -m "$MERGE_MSG"; then
  echo "merge_commit FAILED — conflicts surfaced during merge"
  git merge --abort

  # File a new landing-failure bug with attempted_strategy=merge_commit
  # and increment landing_attempts. The auto-resolve fragment and
  # landing-failure-arbiter fragment handle the bug-filing flow; set
  # the strategy variable so they record it correctly.
  ATTEMPTED_STRATEGY="merge_commit"
  FAILURE_CLASS="mixed"  # merge_commit conflicting implies semantic divergence remains
  # Then fall through to the auto-resolve + landing-failure-arbiter
  # path (it captures conflict excerpts and files the bug).
  return 1 2>/dev/null || exit 1
fi
```

**Run the configured refinery verification on the merged tree.** This
follows the formula's normal `run-tests` and `handle-failures` steps —
the merge commit must pass tests before push.

**Push:**

```bash
git push origin "$TARGET"
git fetch origin
LOCAL=$(git rev-parse "$TARGET")
REMOTE=$(git rev-parse "origin/$TARGET")
[ "$LOCAL" = "$REMOTE" ] || { echo "PUSH FAILED — STOP"; exit 1; }
```

**Close the convoy through Contract S:**

```bash
MERGED_SHA=$(git rev-parse HEAD)
MERGED_SHORT=$(git rev-parse --short HEAD)
gc --rig "$GC_RIG" bd update "$WORK" \
  --set-metadata merge_result=merged \
  --set-metadata merged_sha="$MERGED_SHA" \
  --set-metadata merged_target="$TARGET" \
  --set-metadata landing_strategy_executed=merge_commit \
  --unset-metadata rejection_reason \
  --unset-metadata blocked_reason \
  --unset-metadata blocked_by_bug \
  --unset-metadata landing_state
gc --rig "$GC_RIG" bd close "$WORK" --reason "Merged integration branch to $TARGET at $MERGED_SHORT (strategy: merge_commit)"
```

**Cleanup:**

```bash
git push origin --delete "$BRANCH" || true
```

The convoy's landing-failure bug is already closed by the arbiter when
the strategy was decided; nothing further to do.

### Failure during merge_commit

If the `git merge --no-ff` step conflicts, refinery files a NEW
landing-failure bug via the `refinery-landing-failure-arbiter`
fragment with `attempted_strategy=merge_commit` and a `mixed`
failure-class hint, leaves the convoy open with
`landing_state=blocked`, and clears `gc.routed_to`. The arbiter then
re-decides: a second `merge_commit` after both rebase and merge_commit
failed almost always means `repair_beads` or `human`.

If verification (tests/lint/typecheck) fails after a clean merge,
follow the formula's normal `handle-failures` step: diagnose whether
the failure is branch-regression or pre-existing on target. Since the
divergence already passed the arbiter's `merge_commit` criteria, a
test failure here is almost always pre-existing target breakage — in
which case the formula's pre-existing path applies (find an existing
tracking bug for the symptom OR file one, then PROCEED with the merge
push), NOT abort. The merge can land while the pre-existing bug is
tracked separately for remediation; the convoy's content is not the
cause and shouldn't be blocked by it.

Only abort the merge if the failure is verified branch-introduced
(reproduces on the merged tree but NOT on `origin/$TARGET` alone) —
in which case fall through to the rebase-conflict-rejection flow
exactly as the formula prescribes.

### Verification (REQUIRED — fail closed if violated)

After a successful merge_commit close, Contract S still applies — run
the merge-close-contract verification command unchanged. The post-state
is no different from a successful rebase landing except that
`merged_sha` references a merge commit (two-parent), not a linear
rebase.
{{ end }}
