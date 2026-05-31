{{ define "landing-arbiter-cached-resolution" }}
---

## CACHED RESOLUTION — TELL REFINERY HOW THE NEXT REBASE OF THE SAME CONFLICT SHAPE WILL RESOLVE

After a `repair_beads` decision, the integration branch carries the architectural fix. But when refinery rebases that branch onto the target (e.g., `main`), the *same conflict shape* often reappears because the target was never updated — the substrate file collision, the take-both reconciliation, the rename, all reappear on the rebase.

Without intervention this triggers a fresh arbiter cycle on a decision you already made. The cached-resolution metadata on the convoy lets refinery replay your decision mechanically on the next rebase, skipping the arbiter cycle for the same shape.

### When to write a cached resolution

Write it on the convoy ONLY when ALL of the following hold:

1. The decision was `repair_beads` (not `continue_rebase`, `merge_commit`, or `human`).
2. The repair you specified, once landed on the integration branch, makes the conflict resolvable by **a mechanically-replayable rule per path** — no semantic judgment needed at the next rebase.
3. You can identify each conflict-path's per-path strategy from the vocabulary below.

If ANY conflict path needs a non-mechanical resolution at the next rebase (custom code reconciliation, structural decision-of-the-moment), do NOT write a cached resolution for that landing. The next rebase failure goes through the arbiter loop again, by design.

### What to include vs leave out: scope the cache to STRUCTURAL paths

The cache is for **structural-decision paths only** — paths where YOUR structural analysis is needed to choose a strategy because the resolution depends on understanding the architecture (substrate vs workstream split, rename of a module, file moved between packages, one side's content is now canonical because the other side's content migrated elsewhere).

Do NOT include **routine reconciliation paths** in the cache. Refinery's existing `refinery-rebase-conflict-auto-resolve` fragment already handles these via take-both / mechanical heuristics:

| Path category | Belongs in cache? | Reason |
|---|---|---|
| Module-level rename, split, extraction (substrate moved to new file, etc.) | **Yes** — `take_integration` or `take_target` | Structural decision; both branches added/modified at same path but the resolution requires knowing one side's content moved elsewhere. |
| **File-content rename pattern** — main has `foo.py` (substrate-only, say); integration has the same substance under a new path `bar.py` (renamed) PLUS its own divergent content in `foo.py` (workstream-only). Conflict on `foo.py` during merge. | **Yes** — `take_integration` on `foo.py` | The auto-resolver classifies this as "Not reconcilable" because it sees substantively-different content on both sides of the same path. Only structural awareness (the substrate substance is preserved at `bar.py` on integration) makes `take_integration` correct. Without this cache entry, every merge attempt aborts and re-spawns the arbiter cycle. |
| File created by one branch that the other will pick up on rebase | **Yes** — `take_integration` | Avoids guessing; explicit. |
| Workspace member lists in `pyproject.toml`, `Cargo.toml`, etc. | **No** | Auto-resolver's take-both for add-only sections is correct; cache's `take_integration` would silently lose target's additions. |
| Dependency arrays / requirements lists | **No** | Same: take-both is right; cache would lose entries. |
| Recipe additions in `justfile`, `Makefile` targets | **No** | Take-both preserves both sides' additions. |
| Lockfiles (`uv.lock`, `poetry.lock`, `package-lock.json`) | **No** | Auto-resolver regenerates from the source manifest. Cache's pin would freeze stale resolution. |
| `__all__` lists in `__init__.py` when both sides only added entries | **No** | Take-both alphabetized union is correct. |
| `__init__.py` when sides have structurally different exports (one removes, one renames) | **Yes** if structural | Use judgment. |
| Source code files (CDK stacks, app.py, modules, classes) | **Usually yes** | Structural integration of work; arbiter's decision. |

**The cache complements the auto-resolver; it does NOT replace it.** Refinery applies cache strategies for covered paths, then runs auto-resolver for uncovered paths, then continues the rebase. Both can contribute to a single rebase iteration. Putting routine paths in the cache is a regression — `take_integration` on `pyproject.toml` would lose main's recently-added workspace members; the auto-resolver's take-both preserves both.

When in doubt: leave the path out of the cache. The auto-resolver is the default; the cache is the override for cases where your structural judgment is required.

### Strategy vocabulary (per path)

| Strategy | Mechanically replayable? | git command refinery uses |
|---|---|---|
| `take_integration` | yes | `git checkout --theirs <path> && git add <path>` |
| `take_target` | yes | `git checkout --ours <path> && git add <path>` |
| `delete_from_target` | yes | `git rm <path> && git add <path>` (when the path was renamed/moved by the repair) |
| `manual_reconcile` | no | refinery will NOT apply; falls through to arbiter |

**Use `take_integration` when** the integration branch's version is the post-split / post-rename / post-refactor canonical content, AND the target's version's substance has been preserved elsewhere (a new file, a different module). The current case archetype: substrate moved from `<concern>_stack.py` to `<concern>_substrate_stack.py` on integration; `<concern>_stack.py` is now workstream-only.

**Use `take_target` when** the repair is on the integration side but the target file is the canonical one going forward (rare — typically only when the repair removes integration content rather than splitting/moving it).

**Use `delete_from_target` when** the integration branch renamed a file and the old name still exists in target with overlapping content. Refinery deletes the old path during rebase.

**Use `manual_reconcile` (or omit the cache entirely) when** the resolution requires per-iteration judgment — e.g., `app.py` where each rebase adds new stack registrations that interleave with the integration's, or `__init__.py` where the `__all__` list needs sorted-union merging beyond a simple file-level checkout.

### Schema

Write to `convoy.metadata.cached_resolution` as a JSON string:

```json
{
  "schema_version": 1,
  "issued_by_bug": "<bug-id, e.g., app-q9ip.9>",
  "issued_by_arbiter_session": "<your session id, e.g., city-hl51d5>",
  "issued_at": "<UTC timestamp>",
  "valid_against_target_sha": "<sha of target_branch tip when you decided>",
  "strategy_by_path": {
    "<path1>": "take_integration",
    "<path2>": "take_integration",
    "<path3>": "manual_reconcile"
  },
  "rationale_one_line": "<one sentence — recorded for the next arbiter's audit if cache invalidates>"
}
```

If `strategy_by_path` contains ANY `manual_reconcile`, refinery falls through to the arbiter for the entire rebase — the cache is binary at the rebase level (apply all, or apply none). This is intentional: a mixed resolution is too brittle to apply mechanically.

### Writing the cache

After emitting your repair beads and updating the convoy's `landing_state`, write the cached resolution:

```bash
CACHED_RES=$(jq -nc \
  --arg bug "$BUG" \
  --arg arb "$GC_AGENT" \
  --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --arg target_sha "$TARGET_SHA" \
  --arg rationale "<your one-liner>" \
  '{
    schema_version: 1,
    issued_by_bug: $bug,
    issued_by_arbiter_session: $arb,
    issued_at: $ts,
    valid_against_target_sha: $target_sha,
    strategy_by_path: {
      "<path1>": "take_integration",
      "<path2>": "take_integration"
    },
    rationale_one_line: $rationale
  }')

gc --rig "$GC_RIG" bd update "$source_convoy" \
  --set-metadata cached_resolution="$CACHED_RES"
```

### Cache invalidation by refinery

Refinery automatically discards the cache (and falls through to filing a new bug) when:

- The current rebase conflict touches a path NOT in `strategy_by_path` — the cache is incomplete.
- The current `git rev-parse origin/$TARGET` differs from `valid_against_target_sha` by more than a configurable distance — main has moved enough that the decision may be stale.
- Any cached strategy is `manual_reconcile` — refinery never applies that.
- Applying the cached strategies leaves any unresolved conflict (refinery checks `git status` post-apply).
- Tests fail on the resolved tree — even with the cache applied correctly, a regression invalidates the assumption.

If refinery invalidates, it strips `cached_resolution` from the convoy with a note recording why, then files a fresh landing-failure bug. You, the next arbiter, see the prior cache in the bug's history and decide whether to refine it or escalate.

### Anti-patterns

- **Writing a cache for `manual_reconcile`-heavy resolutions.** If most paths need judgment, the cache delivers no value; just let the next arbiter cycle run.
- **Writing `take_integration` when integration's content is incomplete.** Verify in your reasoning: integration's version of the path must be the canonical end-state, with the target's substance preserved elsewhere. If the polecat's repair-bead body left a TODO or partial implementation, the cache will produce a worse merge than the arbiter cycle would.
- **Putting routine reconciliation paths in the cache** (workspace lists, dependency manifests, lockfiles, take-both-friendly registration surfaces). The auto-resolver handles these correctly via take-both; the cache's `take_integration` would silently lose the target's additions. Cache covers structural decisions only — see the "What to include vs leave out" table above.
- **Enumerating every path in `git diff --name-only` as covered.** The arbiter should classify each path as structural-or-routine and include only the structural ones. Over-covering produces silent data loss when auto-resolver's take-both would have been correct.
- **Omitting `valid_against_target_sha`.** Without it, refinery has no way to know the cache was valid against a particular target state. If main moves significantly, the cache becomes silently stale.
- **Writing a cache after a `continue_rebase` or `merge_commit` decision.** Those decisions don't produce repair-able conflict shapes — no cache to write.

{{ end }}
