{{ define "polecat-handoff-override" }}
---

## DONE SEQUENCE — USE THIS, NOT THE ONE ABOVE

**This section supersedes the "FINAL REMINDER: RUN THE DONE SEQUENCE"
section earlier in this prompt.** Use the commands below verbatim.

The handoff to refinery writes branch + target + gc.routed_to + assignee
+ status via `gc bd update` calls. Every write goes directly to Dolt
(the source of truth). After the writes, signal the refinery so it
re-enters its formula immediately instead of waiting for a poll cycle.

Substitute `<work-bead>` with your bead id and `<brief summary>` with a
one-line description of what you implemented.

```bash
git push origin HEAD

BEAD_ID="<work-bead>"
SUMMARY="<brief summary>"
REFINERY_TARGET="${GC_RIG:+$GC_RIG/}gastown.refinery"
BRANCH=$(git branch --show-current)

# Read the merge target from the bead's metadata. Two field names are
# honored, in this order:
#   1. metadata.target_branch — set by the landing-arbiter on repair beads
#      against an integration-branch landing. An explicit per-bead target
#      that MUST NOT be clobbered to "main".
#   2. metadata.target — set by the cartographer on integration-branch
#      task beads (metadata.target=integration/<wo-slug>).
# Ad-hoc slung beads have neither and fall back to "main". Hardcoding "main"
# would clobber both repair-bead and cartographer integration targets and
# break the landing-arbiter / integration-branch flow.
TARGET=$(gc --rig "$GC_RIG" bd show "$BEAD_ID" --json 2>/dev/null \
  | jq -r '.[0].metadata.target_branch // .[0].metadata.target // "main"')
[ -z "$TARGET" ] && TARGET="main"

# Write handoff metadata + notes. --set-metadata KEY= clears the value
# (empty string), which the resume path treats as "no rejection"
# (jq '.metadata.rejection_reason // empty' → empty → [ -n "" ] false).
gc bd update "$BEAD_ID" \
  --set-metadata branch="$BRANCH" \
  --set-metadata target="$TARGET" \
  --set-metadata rejection_reason= \
  --notes "Implemented: $SUMMARY"

# Reassign + reroute to refinery in a single write.
gc bd update "$BEAD_ID" \
  --status=open \
  --assignee="$REFINERY_TARGET" \
  --set-metadata gc.routed_to="$REFINERY_TARGET"

# Signal the refinery to check for work immediately. Wake starts the
# session if it is stopped; nudge delivers a prompt to a running one.
# `gc session nudge` defaults to wait-idle delivery, which de-duplicates
# concurrent calls with the same target through the CLI queue, so
# many polecats handing off in parallel collapse into at most one
# delivered nudge per idle window. Failure is non-fatal: the refinery
# also sees the Dolt write on its next event-watch cycle.
gc session wake "$REFINERY_TARGET" || true
gc session nudge "$REFINERY_TARGET" "Run 'gc prime' to check merge queue and begin processing." || true

gc runtime drain-ack
exit
```

`gc runtime drain-ack` signals the reconciler to kill this session —
it will only restart you if the pool check command finds more work.
Sitting idle after running this sequence is the "Idle Polecat
heresy."
{{ end }}
