{{ define "refinery-owned-convoy-autoland-handoff" }}
---

## OWNED CONVOY AUTO-LAND HANDOFF — REQUIRED

This section supersedes the prose-only integration-branch check in
`mol-refinery-patrol`'s `next-iteration` step. The required behavior is a bead
handoff, not a convoy close.

This section also supersedes any earlier instruction to prove
`integration_branch_auto_land=true` by reading wisp metadata. Root-only wisp
runtime vars are not persisted on the wisp bead; the required proof is that the
handoff scan below is run.

When an owned convoy's children are all closed, assign the convoy bead to
yourself with merge metadata. Then process that convoy through the normal
refinery merge path. Do not run raw `git merge`/`git push` for the handoff, and
do not close the convoy before the integration branch has been merged to
`main`.

### Handoff Scan

Run this scan before you declare the refinery queue idle, and run it during
every `next-iteration` before pouring the next patrol wisp:

```bash
REFINERY="$GC_AGENT"
LAND_TARGET="main"
QUEUED=0

gc bd list --type=convoy --status=open --label owned --json --limit 0 \
  | jq -r '.[] | [
      .id,
      (.metadata.target // ""),
      (.metadata.branch // ""),
      (.assignee // ""),
      (.metadata.landing_state // "")
    ] | @tsv' \
  | while IFS=$'\t' read -r CID RAW_TARGET RAW_BRANCH CURRENT_ASSIGNEE LANDING_STATE; do
      # An owned convoy's integration branch can live in either metadata
      # field depending on the lifecycle position:
      #   - Cartographer-initial: target=integration/*, branch unset.
      #   - Post-handoff (queued/repair_pending/blocked-then-cleared):
      #     branch=integration/*, target=main (rewritten by the
      #     assignment step below when first queued).
      # Accept both shapes so re-queue after a blocked / repair-pending
      # cycle works without forcing the arbiter or polecat to mutate
      # target back to the integration path.
      INTEGRATION_BRANCH=""
      case "$RAW_TARGET" in
        integration/*) INTEGRATION_BRANCH="$RAW_TARGET" ;;
      esac
      if [ -z "$INTEGRATION_BRANCH" ]; then
        case "$RAW_BRANCH" in
          integration/*) INTEGRATION_BRANCH="$RAW_BRANCH" ;;
        esac
      fi
      if [ -z "$INTEGRATION_BRANCH" ]; then
        continue
      fi

      if [ -n "$RAW_BRANCH" ] && [ "$RAW_BRANCH" != "$INTEGRATION_BRANCH" ]; then
        echo "WARN: owned convoy $CID has unexpected branch=$RAW_BRANCH integration=$INTEGRATION_BRANCH; skipping"
        continue
      fi

      # landing_state=blocked means a landing-failure bug is in flight
      # and the arbiter owns the convoy's next move. Do NOT re-queue
      # underneath it. (repair_pending, queued, and unset are eligible:
      # repair_pending means polecats just finished and we should land
      # the integration branch again; queued falls through to the
      # already-queued check below.)
      if [ "$LANDING_STATE" = "blocked" ]; then
        continue
      fi

      CHILDREN_JSON=$(gc bd list --all --parent "$CID" --json --limit 0)
      TOTAL=$(printf '%s' "$CHILDREN_JSON" | jq 'length')
      if [ "$TOTAL" -eq 0 ]; then
        echo "WARN: owned convoy $CID has integration target but no parent-linked children; skipping"
        continue
      fi

      OPEN_CHILDREN=$(printf '%s' "$CHILDREN_JSON" | jq '[.[] | select(.status != "closed")] | length')
      if [ "$OPEN_CHILDREN" -ne 0 ]; then
        continue
      fi

      if [ "$CURRENT_ASSIGNEE" = "$REFINERY" ] && [ "$RAW_BRANCH" = "$INTEGRATION_BRANCH" ]; then
        echo "Owned convoy $CID already queued for landing: $INTEGRATION_BRANCH -> $LAND_TARGET"
        continue
      fi

      # Increment landing_attempts (first attempt = 1, post-repair retry = 2+).
      # Read the current value before the assignment write so attempt counters
      # survive across the blocked/repair-pending intermediate states.
      PRIOR_ATTEMPTS=$(gc bd show "$CID" --json | jq -r '.[0].metadata.landing_attempts // "0"')
      NEXT_ATTEMPTS=$((PRIOR_ATTEMPTS + 1))

      gc bd update "$CID" \
        --assignee="$REFINERY" \
        --set-metadata branch="$INTEGRATION_BRANCH" \
        --set-metadata target="$LAND_TARGET" \
        --set-metadata gc.routed_to="$REFINERY" \
        --set-metadata landing_state=queued \
        --set-metadata landing_attempts="$NEXT_ATTEMPTS" \
        --notes "Queued owned convoy for refinery integration landing (attempt $NEXT_ATTEMPTS): $INTEGRATION_BRANCH -> $LAND_TARGET"

      gc bd show "$CID" --json | jq -e \
        --arg refinery "$REFINERY" \
        --arg branch "$INTEGRATION_BRANCH" \
        --arg target "$LAND_TARGET" \
        --arg attempts "$NEXT_ATTEMPTS" '
          .[0].status == "open" and
          .[0].assignee == $refinery and
          .[0].metadata.branch == $branch and
          .[0].metadata.target == $target and
          .[0].metadata."gc.routed_to" == $refinery and
          .[0].metadata.landing_state == "queued" and
          .[0].metadata.landing_attempts == $attempts
        ' >/dev/null || {
          echo "OWNED CONVOY HANDOFF FAILED for $CID"
          gc bd show "$CID" --json | jq '.[0] | {id, status, assignee, metadata}'
          exit 1
        }

      echo "Queued owned convoy $CID for landing (attempt $NEXT_ATTEMPTS): $INTEGRATION_BRANCH -> $LAND_TARGET"
      QUEUED=$((QUEUED + 1))
    done
```

After the scan, immediately re-run the normal assigned-work query. If the scan
queued a convoy, that convoy is now work assigned to you. Pick it up as `WORK`
and merge `metadata.branch` to `metadata.target` through the normal
merge/close contract.

### Important Boundaries

- `gc convoy check` is only for unowned convoys. It is not the owned integration
  branch landing mechanism.
- `gc convoy land` closes an owned convoy but does not merge git branches. Do
  not use it before refinery has merged the integration branch and recorded
  merge metadata.
- A zero-child owned integration convoy is not ready; it is a data-linkage
  problem. Skip it and warn instead of assigning it.
- Wisp metadata is not proof that `integration_branch_auto_land=true` was
  applied. Root-only wisp runtime vars are not persisted to
  `.metadata.formula_vars` or `.metadata.vars`. The actual required behavior is
  the handoff scan above.
{{ end }}
