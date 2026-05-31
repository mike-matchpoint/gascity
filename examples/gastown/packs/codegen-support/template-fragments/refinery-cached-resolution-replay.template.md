{{ define "refinery-cached-resolution-replay" }}
---

## CACHED RESOLUTION REPLAY — APPLY AN ARBITER'S PRIOR DECISION TO A RECURRING REBASE CONFLICT

When you rebase an owned-convoy integration branch onto its target and hit conflicts, **before** capturing evidence and filing a landing-failure bug, check whether the convoy carries a cached resolution from a prior arbiter decision. If it does and the cache applies to the current conflict, replay it mechanically and continue the rebase. This avoids re-spawning the arbiter for a conflict shape it has already resolved.

This fragment runs at the START of the rebase-conflict handling path, before any of the existing auto-resolve heuristics in `refinery-rebase-conflict-auto-resolve` and before the landing-failure-bug filing in `refinery-landing-failure-arbiter`.

### Activation

Run this check ONLY when ALL of the following hold:

- You hit a conflict during either `git rebase` (rebase strategy) OR `git merge --no-ff` (merge_commit strategy) — both produce files in `UU`/`AA`/`AU`/`UA` state. The fragment is strategy-agnostic; the conflict-resolution primitives (`checkout --theirs/--ours`, `git rm`, take-both via auto-resolver fallthrough) are identical in either context.
- The work bead is a convoy (`.issue_type == "convoy"`) with `metadata.branch` starting with `integration/`.
- The convoy has non-empty `metadata.cached_resolution`.

If any condition is false, skip this fragment and proceed with the existing flow.

The cache fragment is the *only* place where structural patterns the auto-resolver can't classify (file renames, content-moved-elsewhere) get resolved without escalating to a fresh arbiter cycle. Without it, the auto-resolver hits "Not reconcilable" on the rename's source path and aborts the merge.

### Procedure

```bash
# 1. Read the cache and current conflict set
CACHED=$(gc --rig "$GC_RIG" bd show "$WORK" --json \
  | jq -r '.[0].metadata.cached_resolution // empty')

[ -z "$CACHED" ] && { echo "cache: absent — skipping replay"; exit_normal_flow; }

# 2. Verify schema_version (only schema_version=1 is supported by this fragment)
SCHEMA_VERSION=$(echo "$CACHED" | jq -r '.schema_version // 0')
if [ "$SCHEMA_VERSION" != "1" ]; then
  echo "cache: unknown schema_version=$SCHEMA_VERSION — invalidating and skipping replay"
  invalidate_cache "unknown-schema-version"
  exit_normal_flow
fi

# 3. Get the conflicting paths from the current rebase
CONFLICT_PATHS=$(git diff --name-only --diff-filter=U)

# 4. Partition conflict paths into cache-covered vs uncovered.
#
# Partial coverage is EXPECTED, not a failure. The cache is for
# structural-decision paths only — paths where the arbiter's structural
# analysis is needed. Routine reconciliation paths (workspace member
# lists in pyproject.toml, recipe additions in justfile, dependency
# arrays, alphabetically-merged __all__ lists, lockfile regeneration)
# are handled by `refinery-rebase-conflict-auto-resolve` per its
# existing heuristics. The cache and the auto-resolver compose:
#
#   cache         → applies its strategies for covered paths
#   auto-resolver → handles uncovered paths via take-both / regen / etc.
#   rebase --continue → both have written their resolutions; conflict cleared
#
# Only INVALIDATE the cache when a covered path's strategy CAN'T be
# applied (stale, git failure, tests fail) — uncovered paths are NOT
# a cache failure.
COVERED_PATHS=""
UNCOVERED_PATHS=""
for path in $CONFLICT_PATHS; do
  STRATEGY=$(echo "$CACHED" | jq -r --arg p "$path" '.strategy_by_path[$p] // empty')
  if [ -z "$STRATEGY" ]; then
    UNCOVERED_PATHS="$UNCOVERED_PATHS $path"
  elif [ "$STRATEGY" = "manual_reconcile" ]; then
    # manual_reconcile means the arbiter explicitly deferred to a fresh
    # arbiter cycle for this path. Cache stays valid for other paths,
    # but we cannot continue the rebase mechanically — escalate now.
    echo "cache: path $path has strategy=manual_reconcile — fall through to arbiter (cache stays valid)"
    exit_normal_flow
  else
    COVERED_PATHS="$COVERED_PATHS $path"
  fi
done

# If nothing in the current conflict batch is covered by the cache,
# there's no value in continuing with cache logic — exit cleanly and
# let the auto-resolver handle the entire batch. Cache stays around
# for future rebase attempts where covered paths may conflict.
if [ -z "$COVERED_PATHS" ]; then
  echo "cache: no covered paths in current conflict batch (uncovered:$UNCOVERED_PATHS) — exiting to auto-resolver"
  exit_normal_flow
fi

if [ -n "$UNCOVERED_PATHS" ]; then
  echo "cache: partial coverage — applying for$COVERED_PATHS; leaving for auto-resolver:$UNCOVERED_PATHS"
fi

# 5. Per-path freshness check (CRITICAL)
#
# A cached `take_integration` is only safe when the target branch has NOT
# modified that path since the cache was written. If main added/changed the
# path, taking integration's version would silently lose main's edits.
#
# We check each cached path individually. If ANY path was touched on the
# target branch since the cache's valid_against_target_sha, the cache is
# stale — invalidate and fall through. The arbiter on the next cycle will
# see the per-path diff and either refine the strategy or escalate.
CACHED_TARGET_SHA=$(echo "$CACHED" | jq -r '.valid_against_target_sha // empty')
CURRENT_TARGET_SHA=$(git rev-parse origin/"$TARGET")

if [ -n "$CACHED_TARGET_SHA" ] && [ "$CACHED_TARGET_SHA" != "$CURRENT_TARGET_SHA" ]; then
  # First: coarse drift check. If the target has moved an extreme amount,
  # even unchanged paths may have transitive concerns (e.g., import-graph
  # changes that affect linting). Default ceiling 500 commits.
  TOTAL_DRIFT=$(git rev-list --count "$CACHED_TARGET_SHA".."$CURRENT_TARGET_SHA" 2>/dev/null || echo 999)
  if [ "$TOTAL_DRIFT" -gt 500 ]; then
    echo "cache: target moved $TOTAL_DRIFT commits since cache was written (>500 ceiling) — invalidating"
    invalidate_cache "target-drift:$TOTAL_DRIFT-commits"
    exit_normal_flow
  fi

  # Second: per-path freshness. This is the load-bearing check.
  STALE_PATHS=""
  for path in $(echo "$CACHED" | jq -r '.strategy_by_path | keys[]'); do
    STRATEGY=$(echo "$CACHED" | jq -r --arg p "$path" '.strategy_by_path[$p]')
    case "$STRATEGY" in
      take_integration|take_target|delete_from_target)
        # Only mechanically-replayable strategies need freshness checking.
        # Did the target branch modify this path since the cache was written?
        path_drift=$(git log --oneline "$CACHED_TARGET_SHA"..origin/"$TARGET" -- "$path" 2>/dev/null | wc -l | tr -d ' ')
        if [ "$path_drift" -gt 0 ]; then
          STALE_PATHS="$STALE_PATHS $path"
          echo "cache: path $path has $path_drift commits on target since cache write — STALE"
        fi
        ;;
      manual_reconcile)
        # manual_reconcile is never applied; freshness check doesn't apply.
        ;;
    esac
  done

  if [ -n "$STALE_PATHS" ]; then
    echo "cache: stale paths detected:$STALE_PATHS — invalidating cache, falling through to arbiter"
    invalidate_cache "stale-paths:$STALE_PATHS"
    exit_normal_flow
  fi

  echo "cache: target moved $TOTAL_DRIFT commits but no cached paths touched on target — replay safe"
fi

# 6. Apply strategies — refinery loops over CACHE-COVERED conflict paths
#    only and runs the matching git command. Uncovered paths are left
#    in their conflicted state for the auto-resolver to handle next.
#    Any cache-strategy failure on a covered path invalidates the cache.
for path in $COVERED_PATHS; do
  STRATEGY=$(echo "$CACHED" | jq -r --arg p "$path" '.strategy_by_path[$p]')
  case "$STRATEGY" in
    take_integration)
      git checkout --theirs -- "$path" || { abort_replay "checkout-theirs-failed:$path"; invalidate_cache "checkout-theirs-failed:$path"; exit_normal_flow; }
      git add -- "$path"
      ;;
    take_target)
      git checkout --ours -- "$path" || { abort_replay "checkout-ours-failed:$path"; invalidate_cache "checkout-ours-failed:$path"; exit_normal_flow; }
      git add -- "$path"
      ;;
    delete_from_target)
      git rm -- "$path" || { abort_replay "rm-failed:$path"; invalidate_cache "rm-failed:$path"; exit_normal_flow; }
      ;;
    *)
      echo "cache: unknown strategy $STRATEGY for $path — invalidating"
      invalidate_cache "unknown-strategy:$STRATEGY"
      exit_normal_flow
      ;;
  esac
done

# 7. After applying cache strategies, exit to the outer rebase flow.
#
#    - If UNCOVERED_PATHS is non-empty, the rebase still has unresolved
#      conflicts on those paths. Refinery's main rebase-conflict handler
#      (which calls `refinery-rebase-conflict-auto-resolve` next) will
#      try the take-both / regen / mechanical heuristics on them. If the
#      auto-resolver succeeds, the rebase continues; if it fails, refinery
#      files a landing-failure bug — and the cache stays valid for future
#      rebase attempts where covered paths may conflict again.
#    - If UNCOVERED_PATHS is empty, all conflicts in this batch were
#      cache-covered. The covered paths are now staged; the outer flow
#      runs `git rebase --continue` next, which may surface a new
#      conflict batch (calling this fragment again) or finish cleanly
#      (proceeding to post-rebase tests + push).
#
#    Either way: exit cleanly. Do NOT call `rebase --continue` here —
#    that's the outer flow's responsibility, so it can sequence tests
#    and the next conflict batch correctly.
echo "cache: applied for $COVERED_PATHS; exiting to outer flow (uncovered paths:$UNCOVERED_PATHS)"
exit_normal_flow
```

`exit_normal_flow` here means "skip this fragment's logic and proceed with the existing rebase-conflict handling" (the auto-resolve heuristics, then landing-failure bug if those fail).

### Helpers

```bash
invalidate_cache() {
  local reason="$1"
  gc --rig "$GC_RIG" bd update "$WORK" \
    --unset-metadata cached_resolution \
    --notes "Cached resolution invalidated: $reason"
}

abort_replay() {
  local reason="$1"
  echo "cache: replay aborted ($reason) — attempting clean state for arbiter escalation"
  git rebase --abort 2>/dev/null || true
}
```

### What this fragment does NOT do

- It does NOT delete the cache after a successful replay. Subsequent rebase attempts (e.g., after main moves further) may need to apply the same strategies again. Invalidation only happens when refinery detects the cache is no longer correct (stale paths, strategy apply fail, tests fail on a cache-resolved tree).
- It does NOT invalidate the cache when conflict paths fall OUTSIDE its `strategy_by_path`. Uncovered paths are EXPECTED — the cache covers structural-decision paths only; routine paths (workspace lists, dependency manifests, lockfiles, take-both registration surfaces) are the auto-resolver's domain. The two compose: cache applies for its paths, auto-resolver handles the rest, both run within the same rebase iteration.
- It does NOT call `git rebase --continue` itself. The outer rebase loop owns sequencing — running auto-resolver for uncovered paths after the cache applies, then continuing the rebase, then surfacing the next conflict batch (which re-enters this fragment) or finishing cleanly (proceeding to post-rebase tests).
- It does NOT update or extend the cache. Cache writes are the arbiter's responsibility, never refinery's.
- It does NOT replay for non-convoy beads or non-integration-branch landings. Polecat task rejection still goes through Contract R as before.

### Audit signal

Successful cache replays should produce an event-log signal so the operator can monitor cache effectiveness over time:

```bash
gc --rig "$GC_RIG" bd update "$WORK" \
  --notes "Cached resolution replayed: applied $(echo "$CACHED" | jq -r '.strategy_by_path | keys | join(",")') from bug $(echo "$CACHED" | jq -r '.issued_by_bug')"
```

A cache that's invalidated repeatedly across attempts is a signal that the arbiter's strategy was wrong — investigate the cached_resolution write logic, not refinery.

{{ end }}
