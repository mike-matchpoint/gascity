{{ define "refinery-merge-close-contract" }}
---

## MERGE / CLOSE CONTRACT — INVARIANT POST-STATES (ENFORCED)

**After the `merge-push` and `rebase` formula steps above complete — or whatever path you took to compute equivalent state — these two contracts MUST hold for every work bead before you move to the next bead or end your turn. The recipes above show WAYS to reach the post-states. This section defines WHAT post-states are required and HOW to verify them. The recipes are not invalidated; these contracts are layered on top as enforced invariants.**

Recipes are negotiable. Contracts are not. If you paraphrase, batch, or script the recipes for speed, run the verification commands below after every iteration anyway.

### Contract S — SUCCESS (after `git push` of the merge to `$TARGET`)

When a work bead's branch has been merged onto `$TARGET` and pushed to origin, the bead MUST end in this state:

| Field | Required value |
|---|---|
| `.status` | `"closed"` |
| `.close_reason` | non-empty string |
| `.metadata.merge_result` | `"merged"` |
| `.metadata.merged_sha` | full 40-char hex SHA of the merge commit |
| `.metadata.merged_target` | `$TARGET` (same value the bead carried before the merge) |
| `.metadata.rejection_reason` | empty or absent |

Verification (run after every successful close, before moving to the next bead):

```bash
gc bd show "$WORK" --json | jq -e '
  .[0].status == "closed" and
  ((.[0].close_reason // "") != "") and
  .[0].metadata.merge_result == "merged" and
  ((.[0].metadata.merged_sha // "") | test("^[0-9a-f]{40}$")) and
  ((.[0].metadata.merged_target // "") != "") and
  ((.[0].metadata.rejection_reason // "") == "")
' >/dev/null || {
  echo "CONTRACT S VIOLATION on $WORK"
  gc bd show "$WORK" --json | jq '.[0] | {status, close_reason, metadata}'
  exit 1
}
```

If verification fails: STOP the iteration. Inspect which field is wrong, write it, re-verify. Do not move on to the next bead until the contract holds. Escalate to mayor with the bead id and the failing field if you cannot resolve it.

### Contract R — REJECTION (after a rebase conflict or non-recoverable merge failure)

Rejection routing splits by bead type. Read `.issue_type` from the bead before writing rejection metadata.

**Task beads (`issue_type == "task"`)** — return to the polecat pool. The bead MUST end in this state:

| Field | Required value |
|---|---|
| `.status` | `"open"` |
| `.assignee` | `""` (empty string) |
| `.metadata."gc.routed_to"` | `"${GC_RIG:+$GC_RIG/}gastown.polecat"` |
| `.metadata.rejection_reason` | non-empty string describing the failure |
| origin branch `metadata.branch` | unchanged (a new polecat needs it to resume) |

**Convoy beads (`issue_type == "convoy"`) with an integration branch** — file a landing-failure bug routed to the landing-arbiter; do NOT escalate the convoy itself. Polecats cannot resume convoy work and a human route on the convoy short-circuits the arbiter's decision loop. The bead MUST end in this state:

| Field | Required value |
|---|---|
| `.status` | `"open"` |
| `.assignee` | `""` (empty string) |
| `.metadata.landing_state` | `"blocked"` |
| `.metadata.merge_result` | `"blocked"` |
| `.metadata.landing_failure_id` | id of the child landing-failure bug (non-empty) |
| `.metadata.blocked_reason` | non-empty string referencing the bug id |
| `.metadata.rejection_reason` | non-empty string referencing the bug id |
| `.metadata."gc.routed_to"` | absent (unset) |
| origin branch `metadata.branch` | unchanged (the arbiter and any repair beads need it) |

The landing-failure bug is filed via the `refinery-landing-failure-arbiter` fragment, which captures all git evidence (SHAs, conflict-region excerpts, auto-resolver outcome) and writes the convoy's blocked metadata. Run that fragment's procedure end-to-end; do NOT manually `--set-metadata gc.routed_to=human` on the convoy.

**Convoy beads without an integration branch** (legacy / non-integration-target convoys) — fall back to the previous escalation behavior: leave the convoy with `gc.routed_to=human` and mail mayor. This path is the residual safety net for convoys that don't fit the owned-integration-branch model.

Verification (run immediately after the reject write, branches on `issue_type`):

```bash
TYPE=$(gc bd show "$WORK" --json | jq -r '.[0].issue_type // "task"')
case "$TYPE" in
  task)
    POOL="${GC_RIG:+$GC_RIG/}gastown.polecat"
    gc bd show "$WORK" --json | jq -e --arg pool "$POOL" '
      .[0].status == "open" and
      ((.[0].assignee // "") == "") and
      (.[0].metadata."gc.routed_to" == $pool) and
      ((.[0].metadata.rejection_reason // "") != "")
    ' >/dev/null || {
      echo "CONTRACT R VIOLATION on task bead $WORK"
      gc bd show "$WORK" --json | jq '.[0] | {issue_type, status, assignee, metadata}'
      exit 1
    }
    ;;
  convoy)
    BRANCH=$(gc bd show "$WORK" --json | jq -r '.[0].metadata.branch // ""')
    case "$BRANCH" in
      integration/*)
        # Integration-branch landing: landing-failure bug must exist and the convoy
        # must be in blocked-passive state. gc.routed_to MUST be absent.
        gc bd show "$WORK" --json | jq -e '
          .[0].status == "open" and
          ((.[0].assignee // "") == "") and
          (.[0].metadata.landing_state == "blocked") and
          ((.[0].metadata.landing_failure_id // "") != "") and
          ((.[0].metadata.blocked_reason // "") != "") and
          ((.[0].metadata."gc.routed_to" // "" | length) == 0)
        ' >/dev/null || {
          echo "CONTRACT R VIOLATION on integration-convoy bead $WORK"
          gc bd show "$WORK" --json | jq '.[0] | {issue_type, status, assignee, metadata}'
          exit 1
        }
        # Spot-check the landing-failure bug exists, is parented, and is routed
        # to the arbiter.
        BUG_ID=$(gc bd show "$WORK" --json | jq -r '.[0].metadata.landing_failure_id')
        gc bd show "$BUG_ID" --json | jq -e --arg arb "${GC_RIG:+$GC_RIG/}landing-arbiter" '
          .[0].status == "open" and
          .[0].metadata."gc.kind" == "owned_convoy_landing_failure" and
          .[0].metadata."gc.routed_to" == $arb
        ' >/dev/null || {
          echo "CONTRACT R VIOLATION: landing-failure bug $BUG_ID for $WORK not in expected state"
          gc bd show "$BUG_ID" --json | jq '.[0] | {id, status, metadata}'
          exit 1
        }
        ;;
      *)
        # Non-integration convoy: fall back to the legacy human-escalation
        # invariant.
        gc bd show "$WORK" --json | jq -e '
          .[0].status == "open" and
          ((.[0].assignee // "") == "") and
          (.[0].metadata."gc.routed_to" == "human") and
          ((.[0].metadata.rejection_reason // "") != "") and
          ((.[0].metadata.blocked_reason // "") != "")
        ' >/dev/null || {
          echo "CONTRACT R VIOLATION on non-integration convoy bead $WORK"
          gc bd show "$WORK" --json | jq '.[0] | {issue_type, status, assignee, metadata}'
          exit 1
        }
        ;;
    esac
    ;;
  *)
    echo "CONTRACT R: unexpected issue_type $TYPE on $WORK; treat as task by default but verify the rejection path is correct"
    ;;
esac
```

`gc.routed_to` discipline: for task beads, the reconciler routes pool work by this field, not by `assignee` — a rejected task bead whose `gc.routed_to` still points at refinery is stranded. For integration-branch convoys, `gc.routed_to` MUST be absent — the landing-failure bug carries the arbiter route, not the convoy. For non-integration convoys, `gc.routed_to=human` plus an escalation mail remains the recoverable path.

### Helper scripts must verify

You MAY write a helper script to batch the merge cycle when the queue is large. If you do, the script MUST:

1. End every successful-merge iteration with the Contract S verification command.
2. End every rejection iteration with the Contract R verification command.
3. Exit non-zero on verification failure. Do not print `PASS` for a bead whose contract failed.
4. Chain the metadata write and the `gc bd close` with `&&` (or equivalent guard) so a partial failure surfaces as a script-level error.

A helper script that runs the git operations correctly but skips contract verification is wrong. `PASS` without verification is not `PASS`.

### Smoke check before ending the turn

Before pouring the next patrol wisp, run this audit against beads you closed in the current session:

```bash
gc bd list --assignee="$GC_AGENT" --status=closed --exclude-type=epic --limit=50 --json \
  | jq -r '.[] | select(
      (.metadata.merge_result // "") != "merged" or
      (.metadata.merged_target // "") == "" or
      ((.metadata.merged_sha // "") | test("^[0-9a-f]{40}$") | not)
    ) | .id'
```

Any output from this command is a list of Contract S violations. For each violating bead, repair inline before pouring the next iteration:

```bash
# Read the bead's pre-close target and merged_sha, then write the missing audit fields.
# Writes are idempotent.
TARGET=$(gc bd show "$BEAD" --json | jq -r '.[0].metadata.target // "main"')
SHA=$(gc bd show "$BEAD" --json | jq -r '.[0].metadata.merged_sha')
gc bd update "$BEAD" \
  --set-metadata merge_result=merged \
  --set-metadata merged_target="$TARGET" \
  --set-metadata merged_sha="$SHA"
```

If the smoke check finds violations: do not pour the next wisp until the audit returns empty.
{{ end }}
