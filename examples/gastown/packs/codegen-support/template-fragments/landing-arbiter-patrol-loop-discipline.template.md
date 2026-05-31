{{ define "landing-arbiter-patrol-loop-discipline" }}
---

## PATROL LOOP DISCIPLINE — BOUNDED QUEUE-DRAIN, THEN QUIESCE

**This section is a hard requirement on how you operate
`mol-landing-arbiter-patrol`.** The molecule is `--root-only` (no step
beads, no closure enforcement); your prompt compliance IS the loop
mechanism. Three rules, all mandatory.

### Part 1: Drain the queue inside this wisp — do NOT pour-per-bug

Each cooldown tick wakes a fresh session. Inside one session, you may
decide up to `iteration_cap` bugs (default 8) before quiescing. After
each `emit`, run `next-iteration` and check the queue:

- If another pending bug exists AND iteration cap not exceeded:
  re-enter `claim-next-failure` IN THE SAME WISP. Do NOT pour a new
  wisp per bug — the cooldown tick is the throttle, not the formula
  iteration.
- Otherwise: pour the next wisp, burn this one, drain-ack, exit.

### Part 2: Quiesce on empty queue — do NOT spin

The cooldown timer fires every 60s wall-clock regardless of session
liveness. When `claim-next-failure` reports an empty queue, you MUST
quiesce immediately:

```bash
NEW_WISP=$(gc bd mol wisp mol-landing-arbiter-patrol --root-only \
  --var rig_name="$GC_RIG" \
  --var binding_prefix=gastown. \
  --var iteration_cap=8 \
  --json | jq -r '.new_epic_id')
gc bd update "$NEW_WISP" --assignee=""

CURRENT_WISP="${GC_BEAD_ID:-}"
[ -n "$CURRENT_WISP" ] && gc bd mol burn "$CURRENT_WISP" --force

gc runtime drain-ack
exit
```

**Idle quiesce is the steady state.** The next cooldown tick will
spawn a fresh session that scans the queue and quiesces again if
still empty. Running open-ended would burn cache for no work.

### Part 3: Iteration cap prevents runaway

`iteration_cap` (default 8) bounds how many bugs you decide per wake.
Track decisions as you make them; when the cap is hit, finish the
current bug's `emit` step, then quiesce per Part 2 regardless of
queue state.

The cap exists for two reasons:

1. Pathological self-re-queuing: if a downstream consumer (refinery,
   polecat) creates a new landing-failure bug each time it acts, the
   arbiter could spin indefinitely. The cap caps damage at 8 decisions
   per minute; an operator has time to notice.
2. Session-cost bounding: a fresh-wake session's cost grows with
   per-bug context loads. 8 bugs is a comfortable upper bound; the
   next tick (60s later) takes the next 8 if there are that many.

### Smoke check (before ending the turn)

```bash
# Step 1: in_progress patrol wisp assigned to me?
gc bd list --assignee="$GC_AGENT" --status=in_progress --json \
  | jq -r '.[] | select(.title | startswith("mol-landing-arbiter-patrol")) | .id'

# Step 2: bugs still claimed by me with decision_state=in_progress?
gc --rig "$GC_RIG" bd list --type=bug --status=open \
  --metadata-field gc.kind=owned_convoy_landing_failure \
  --metadata-field decision_state=in_progress \
  --json | jq -r --arg me "$GC_AGENT" '.[] | select(.assignee == $me) | .id'
```

| Step 1 | Step 2 | Verdict | Action |
|---|---|---|---|
| in_progress wisp | any | Mid-iteration | Finish through emit + next-iteration |
| empty | non-empty | Stranded claims | Release via `decision_state=pending`; pour next wisp; quiesce |
| empty | empty | Genuinely idle | Pour next wisp; drain-ack; exit |

### Recovery from a wedged state

If you wake into a session and the smoke test shows a wedged state
(in_progress wisp + claimed bugs but no progress visible from the
last hour's mail/log), recover BEFORE doing other work:

```bash
# Case 1: stranded claims from a dead prior session — release.
STRANDED=$(gc --rig "$GC_RIG" bd list --type=bug --status=open \
  --metadata-field gc.kind=owned_convoy_landing_failure \
  --metadata-field decision_state=in_progress \
  --json | jq -r --arg me "$GC_AGENT" '.[] | select(.assignee == $me) | .id')
for BUG in $STRANDED; do
  gc --rig "$GC_RIG" bd update "$BUG" \
    --assignee="" \
    --set-metadata decision_state=pending
done

# Case 2: stale in_progress wisp with no progress — burn it.
STUCK=$(gc bd list --assignee="$GC_AGENT" --status=in_progress --json \
  | jq -r '.[] | select(.title | startswith("mol-landing-arbiter-patrol")) | .id')
if [ -n "$STUCK" ]; then
  gc bd mol burn "$STUCK" --force
fi
```

After recovery, re-enter the formula at `recover-stranded-claims` (the
formula's step 0) and let the normal loop pick up.
{{ end }}
