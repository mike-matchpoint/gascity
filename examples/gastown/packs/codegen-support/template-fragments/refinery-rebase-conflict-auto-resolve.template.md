{{ define "refinery-rebase-conflict-auto-resolve" }}
---

## REBASE CONFLICT — AGENT-DRIVEN AUTO-RESOLVE

**This section defines how to attempt trivial resolution of a rebase conflict before falling through to the formula's conflict-rejection recipe.** It runs as a pre-step inside the formula's `rebase` step, after `git rebase origin/$TARGET` reports conflicts and before the recipe's `git rebase --abort` and rejection writes execute. Nothing here changes how you handle any situation other than a rebase conflict.

Scope is narrow and absolute:

- **Activation:** only when `git rebase origin/$TARGET` fails with conflicts. Not on test failures. Not on push failures. Not on missing-branch errors. Not on `git merge --ff-only` failures. Only rebase conflicts.
- **One attempt per bead.** Tracked via `metadata.auto_resolve_attempted` (see below). If that field is already `"true"` on the bead when you enter this section, skip the entire section and run the formula's conflict-rejection recipe unchanged.
- **Tests are the gate.** Whatever you produce here flows through the formula's existing `run-tests` step. If tests fail on your resolution, the formula's rejection recipe runs as if you had never tried — and the loop guard above prevents a second pass on the next polecat's resubmission.

### What you MAY do

You have judgment. Both branches passed their producer agents' self-review and intend to land on `$TARGET`. Your job is to combine their intent so the combined result remains functional. You MAY:

- **Take both** when two branches add to a shared registration surface — workspace member lists, dependency arrays, build-tool recipe sections, configuration manifests, route tables, infrastructure stack entries, changelog or release-notes blocks, etc. Order deterministically (alphabetic for sortable lists, source-order for free-form text).
- **Reconcile naming drift** when two branches reference the same logical entity under different names — e.g. one branch defines a function spelled in snake_case and the other writes a call site spelled in camelCase. Pick the canonical name and align references in the resolving merge. Prefer the name used at definition over the name used at the call site; prefer the name that already exists on `$TARGET` when one branch's name is already present there.
- **Regenerate derived files** (lockfiles, generated manifests) after resolving their source. See the table below.
- **De-duplicate or normalize** entries to keep the combined result valid — e.g. two branches added the exact same dependency line, or two branches added entries in inconsistent quoting / indentation styles.
- **Restore functionality that is obviously broken by the combination**, where "obviously broken" means a symbol resolves in neither branch alone but is needed by both, or two branches both registered a thing under inconsistent identifiers and one must be normalized for the combined result to even parse / compile.

The bar for an edit is: **does the edit restore behavior the branches independently intended, without inventing new behavior?** If yes, you may make it. If you cannot answer yes confidently, abort.

### What you MUST NOT do

The latitude above is for *reconciling what two branches intend.* It is not for *deciding what the result should be.* You MUST NOT:

- **Add new functionality** that neither branch contained.
- **Change application logic** in either branch beyond renames and alignments needed to make the combined result resolve. If a function body differs between branches, that is a behavioral conflict — reject. Do not pick one body, do not synthesize a third.
- **Resolve any conflict where one side deleted a line that the other side kept or modified.** Deletion-vs-keep is a genuine incompatibility the polecats must settle.
- **Resolve any conflict where both sides modified the same line with substantively different content.** Pure whitespace, ordering, or formatting differences are reconcilable; semantic differences are not.
- **Touch contract surfaces where the two branches disagree on shape.** Renaming a symbol to align references is allowed; changing an argument count, a return type, a schema field type, or a public interface signature is not.
- **Attempt a second pass on the same bead.** If `metadata.auto_resolve_attempted=true` already, skip this section.
- **Skip the test gate.** Whatever you produce flows through `run-tests`. Do not bypass.

### Procedure

After `git rebase origin/$TARGET` reports conflicts and you have confirmed `metadata.auto_resolve_attempted` is not already `"true"`, while the rebase is mid-flight (HEAD on the conflict commit):

**1. Inventory the conflict.** Inventory all conflicted files BEFORE doing anything else — the capture in step 1a depends on the working tree still being mid-rebase. For each conflicted file, classify every `<<<<<<<` / `>>>>>>>` block by hunk shape:

| Hunk shape | Classification |
|---|---|
| Both sides add lines at the same insertion point; neither side deletes or substantively edits a shared line | **Reconcilable** (take-both / merge) |
| Both sides modify the same line(s) but the differences are pure whitespace, formatting, or ordering | **Reconcilable** (normalize) |
| One branch's symbol name does not match a call/reference site introduced by the other branch, and the difference is consistent enough to align | **Reconcilable** (rename to canonical) |
| One side deletes a line the other side keeps or edits | **Not reconcilable** — reject |
| Both sides substantively edit the same line(s) | **Not reconcilable** — reject |
| Two branches disagree on the shape of a contract surface (signature, schema type, public interface) | **Not reconcilable** — reject |

If ANY hunk in ANY conflicted file falls in the bottom three rows, ABORT this section. Do not partial-resolve. Fall through to the formula's conflict-rejection recipe — but FIRST run step 1a below to capture the evidence the conflict-rejection path needs.

**1a. Capture conflict evidence BEFORE `git rebase --abort`.** This is the only step where the conflict text is naturally in hand; `git rebase --abort` wipes the working tree and loses the hunks. If you are aborting (any "Not reconcilable" classification, regen failure, or `--continue` failure), AND the work bead is an owned-convoy integration-branch landing (`.issue_type == "convoy"` AND `metadata.branch` starts with `integration/`), capture these shell variables so the `refinery-landing-failure-arbiter` fragment can file the landing-failure bug:

```bash
# Identify whether this is an owned-convoy integration landing.
WORK_TYPE=$(gc --rig "$GC_RIG" bd show "$WORK" --json | jq -r '.[0].issue_type')
WORK_BRANCH=$(gc --rig "$GC_RIG" bd show "$WORK" --json | jq -r '.[0].metadata.branch // ""')
case "$WORK_TYPE/$WORK_BRANCH" in
  convoy/integration/*)
    # Capture SHAs (refs were fetched at the top of the rebase step).
    MERGE_BASE_SHA=$(git merge-base "origin/$BRANCH" "origin/$TARGET" 2>/dev/null || echo "")
    TARGET_TIP_SHA=$(git rev-parse "origin/$TARGET" 2>/dev/null || echo "")
    INTEGRATION_TIP_SHA=$(git rev-parse "origin/$BRANCH" 2>/dev/null || echo "")
    AHEAD=$(git rev-list --count "origin/$TARGET..origin/$BRANCH" 2>/dev/null || echo "")
    BEHIND=$(git rev-list --count "origin/$BRANCH..origin/$TARGET" 2>/dev/null || echo "")

    # Strategy: this fragment runs in the rebase path. If a later
    # decision-consumer recipe (continue_rebase / merge_commit) is
    # active, the consumer overrides ATTEMPTED_STRATEGY before
    # invoking the auto-resolve. Default is "rebase".
    ATTEMPTED_STRATEGY="${ATTEMPTED_STRATEGY:-rebase}"

    # Conflict paths from `git diff --name-only --diff-filter=U`.
    CONFLICT_PATHS_CSV=$(git diff --name-only --diff-filter=U | paste -sd, -)

    # Best-effort kind detection.
    CONFLICT_KIND=""
    for p in $(echo "$CONFLICT_PATHS_CSV" | tr ',' '\n'); do
      [ -z "$p" ] && continue
      # add/add: file exists on both sides but not at merge-base.
      if [ -n "$MERGE_BASE_SHA" ] && ! git show "$MERGE_BASE_SHA:$p" >/dev/null 2>&1; then
        CONFLICT_KIND="add/add"
      elif [ -z "$CONFLICT_KIND" ]; then
        CONFLICT_KIND="modify/modify"
      fi
    done

    # Excerpts: each conflicted file's `<<<<<<<`/`=======`/`>>>>>>>`
    # blocks with ~10 lines context per side. Bound total payload —
    # truncate per-file to 200 lines and record truncation.
    EXCERPT_FILE=$(mktemp)
    EXCERPT_TRUNCATED=false
    for p in $(echo "$CONFLICT_PATHS_CSV" | tr ',' '\n'); do
      [ -z "$p" ] && continue
      [ -f "$p" ] || continue
      echo "### $p" >> "$EXCERPT_FILE"
      # awk: print conflict markers + ~10 lines context per side.
      awk '
        /^<<<<<<</ {
          in_conflict=1
          start_line=NR
          # rewind to capture preceding context
          for (i=length(buf); i>0 && i>length(buf)-10; i--) print buf[i]
          print
          next
        }
        /^>>>>>>>/ {
          in_conflict=0
          print
          # trailing context: read 10 more lines
          for (i=0; i<10 && (getline line) > 0; i++) print line
          next
        }
        {
          if (in_conflict) { print }
          else { buf[NR % 50] = $0 }
        }
      ' "$p" 2>/dev/null | head -200 >> "$EXCERPT_FILE"
      LINECOUNT=$(awk '/^<<<<<<</,/^>>>>>>>/' "$p" 2>/dev/null | wc -l)
      if [ "$LINECOUNT" -gt 200 ]; then
        EXCERPT_TRUNCATED=true
        echo "... [truncated; full conflict region is $LINECOUNT lines]" >> "$EXCERPT_FILE"
      fi
    done
    CONFLICT_EXCERPTS=$(cat "$EXCERPT_FILE")
    rm -f "$EXCERPT_FILE"

    # Which classification fired (set by the inventory step above).
    # Examples: "contract-surface-disagreement",
    # "both-sides-modify", "deletion-vs-keep", "regenerate-failure".
    AUTO_RESOLVER_ABORTED_REASON="${AUTO_RESOLVER_ABORTED_REASON:-unspecified}"

    # First-pass failure-class hint for the arbiter (it re-classifies).
    if [ "$CONFLICT_KIND" = "add/add" ]; then
      FAILURE_CLASS="semantic_add_add"
    elif [ -n "$BEHIND" ] && [ "$BEHIND" -ge 10 ]; then
      FAILURE_CLASS="stale_integration_branch"
    elif [ "$AUTO_RESOLVER_ABORTED_REASON" = "contract-surface-disagreement" ]; then
      FAILURE_CLASS="contract_surface_conflict"
    elif [ "$AUTO_RESOLVER_ABORTED_REASON" = "regenerate-failure" ]; then
      FAILURE_CLASS="regenerate_failure"
    else
      FAILURE_CLASS="semantic_modify_modify"
    fi

    # Evidence completeness: every required field is set.
    EVIDENCE_COMPLETE=true
    EVIDENCE_GAPS=""
    for v in MERGE_BASE_SHA TARGET_TIP_SHA INTEGRATION_TIP_SHA CONFLICT_EXCERPTS; do
      eval "val=\$$v"
      if [ -z "$val" ]; then
        EVIDENCE_COMPLETE=false
        EVIDENCE_GAPS="$EVIDENCE_GAPS$v,"
      fi
    done
    EVIDENCE_GAPS=${EVIDENCE_GAPS%,}

    export MERGE_BASE_SHA TARGET_TIP_SHA INTEGRATION_TIP_SHA AHEAD BEHIND
    export ATTEMPTED_STRATEGY FAILURE_CLASS CONFLICT_KIND CONFLICT_PATHS_CSV
    export CONFLICT_EXCERPTS AUTO_RESOLVER_ABORTED_REASON
    export EVIDENCE_COMPLETE EVIDENCE_GAPS
    ;;
  *)
    # Not an owned-convoy integration landing — the formula's normal
    # task-bead rejection runs; no evidence capture needed.
    ;;
esac
```

After capturing (or skipping capture for task beads), proceed to `git rebase --abort` and fall through to the formula's conflict-rejection recipe. The `refinery-landing-failure-arbiter` fragment will consume these exports if the bead is an owned-convoy integration landing.

**2. Resolve.** For each reconcilable hunk, write the merged content directly to the working tree. Stage:

```bash
git add <resolved-file> ...
```

If the resolution required renaming a symbol across other files in the branch, edit those files too and stage them. Symbol renames that touch files outside the conflict set are allowed when the rename is mechanical (the same identifier substituted consistently) — they are not allowed when the rename would change semantics.

**3. Regenerate derived files** when their source was among the resolved (or rename-touched) files. Run the regenerate command only when the tool exists in PATH; skip silently otherwise. Stage the regenerated derived file.

| Source file resolved | Derived file | Regenerate command |
|---|---|---|
| `pyproject.toml` (uv project) | `uv.lock` | `uv lock` |
| `pyproject.toml` (poetry project) | `poetry.lock` | `poetry lock --no-update` |
| `requirements.in` | `requirements.txt` | `pip-compile requirements.in` |
| `Pipfile` | `Pipfile.lock` | `pipenv lock` |
| `package.json` (npm) | `package-lock.json` | `npm install --package-lock-only` |
| `package.json` (yarn berry) | `yarn.lock` | `yarn install --mode=update-lockfile` |
| `package.json` (yarn classic) | `yarn.lock` | `yarn install` |
| `package.json` (pnpm) | `pnpm-lock.yaml` | `pnpm install --lockfile-only` |
| `package.json` (bun) | `bun.lockb` | `bun install` |
| `Cargo.toml` | `Cargo.lock` | `cargo update --workspace` |
| `go.mod` | `go.sum` | `go mod tidy` |
| `Gemfile` | `Gemfile.lock` | `bundle lock` |
| `composer.json` | `composer.lock` | `composer update --lock` |

If a regenerate command fails (non-zero exit), treat the resolution as a genuine incompatibility — `git rebase --abort` and fall through to formula rejection. A failed regenerate means the resolved source produces something the tool rejects.

**4. Continue the rebase:**

```bash
git rebase --continue
```

If `--continue` surfaces conflicts on a later commit in the branch, restart this procedure from step 1 for the new conflicts. The loop guard on `metadata.auto_resolve_attempted` does NOT re-fire mid-rebase — that field is set per-bead-attempt, not per-commit.

If `--continue` fails for any reason other than a fresh conflict (a hook rejects, the working tree is dirty in an unexpected way, etc.), run `git rebase --abort` and fall through to the formula's conflict-rejection recipe.

**5. Record the attempt before proceeding to `run-tests`.** Set the metadata REGARDLESS of outcome — the `auto_resolve_attempted` flag is what prevents a second pass on this bead.

On successful resolve through to a clean `git rebase --continue`:

```bash
gc bd update "$WORK" \
  --set-metadata auto_resolve_attempted=true \
  --set-metadata auto_resolved=true \
  --set-metadata auto_resolve_summary="<file>: take-both; <file>: align-names; <lockfile>: regenerated"
```

On any abort within this section (before reaching the rebase --continue, or after a regenerate failure):

```bash
gc bd update "$WORK" \
  --set-metadata auto_resolve_attempted=true \
  --set-metadata auto_resolve_summary="aborted: <which hunk classification fired, which file>"
```

The `auto_resolve_attempted=true` field prevents a second pass on the same bead. The `auto_resolved=true` field marks successful attempts so operators can audit them. The `auto_resolve_summary` is the forensic breadcrumb naming what you did.

**6. Proceed to `run-tests`.** Tests are the truth. If they pass, the formula's normal `merge-push` step runs and the bead closes per Contract S. If they fail, the formula's normal failure-handling and rejection recipe runs — and the `auto_resolve_attempted=true` metadata you wrote in step 5 ensures the next polecat's resubmission will not loop through this section again.

### Interaction with the formula's existing rejection recipe

When you fall through to the formula's conflict-rejection recipe (any abort path above, or after test failure on an auto-resolved attempt), run that recipe exactly as written. Do NOT skip the `--set-metadata rejection_reason=...` write. The `rejection_reason` describes the conflict the polecat needs to resolve; `auto_resolve_summary` is a separate field that operators read when auditing what this section did. The two fields coexist on the same bead.

### Audit smoke check

Before pouring the next patrol wisp, surface recent auto-resolved closes for spot-check visibility:

```bash
gc bd list --assignee="$GC_AGENT" --status=closed --exclude-type=epic --limit=20 --json \
  | jq -r '.[] | select(.metadata.auto_resolved == "true") | "\(.id)\t\(.metadata.auto_resolve_summary // "")"'
```

This output is informational, not a contract violation. It exists so an operator scanning the patrol log can see at a glance which closes involved auto-resolution and spot-check the summaries.
{{ end }}
