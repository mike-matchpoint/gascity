{{ define "refinery-patrol-loop-discipline" }}
---

## PATROL LOOP DISCIPLINE — MUST RUN NEXT-ITERATION AND LOOP

**This section is a hard requirement on how you operate `mol-refinery-patrol`.**
The molecule is `--root-only` — no step beads, no closure enforcement.
Your prompt compliance IS the loop mechanism. Two rules, both mandatory.

### Part 1: Run next-iteration EVERY iteration

No matter what happened earlier — successful merge, rejected branch,
conflict abort, empty queue — `next-iteration` MUST run. That step:

1. Pours a NEW patrol wisp (`gc bd mol wisp mol-refinery-patrol --root-only ...`).
2. Assigns the new wisp to yourself.
3. **Burns the CURRENT wisp** (`gc bd mol burn $WISP --force`).
4. Closes the `next-iteration` step.

### Part 2: After next-iteration, IMMEDIATELY loop back — do NOT end your turn

The formula's next-iteration step ends with "Close this step. The new
wisp is ready — re-read formula steps to begin." That directive is
binding. After closing next-iteration:

1. **Set the new wisp to in_progress:** `gc bd update <new-wisp-id> --status=in_progress`
2. **Re-enter the formula at check-inbox/find-work** on the new wisp.
3. Continue iterating. The patrol is a continuous loop, not a series of
   one-shot sessions.

**Your turn only ends when ALL of these are true:**

- No in_progress patrol wisp assigned to you.
- No open (unstarted) patrol wisp assigned to you.
- No assigned work beads waiting.
- find-work's wait-timeout cap has been reached for the current iteration.

For the normal case — successful merge cycle with queue temporarily
empty — DO NOT end your turn. Re-enter find-work; the formula's
internal wait loop handles brief queue lulls.

The only legitimate non-loop exits are escalation paths: mail to
mayor + drain-ack on human-review needed, or hard failure on Dolt
corruption / fatal git error.

### "Queue is empty" is NOT a termination signal

When `find-work` reports no assigned beads, do not write "queue is now
empty" and stop. The formula's find-work step has its own wait/retry
loop — keep iterating inside find-work, OR continue past it through
patrol-summary into next-iteration, which pours a wisp and you loop
back to find-work on the new wisp. ALWAYS reach next-iteration before
considering whether to end the turn.

### Smoke test (run before ending your turn)

```bash
# Step 1: in_progress patrol wisp assigned to me?
gc bd list --assignee="$GC_AGENT" --status=in_progress --json \
  | jq -r '.[] | select(.title | startswith("mol-refinery-patrol")) | .id'

# Step 2: open (poured but not started) patrol wisp assigned to me?
gc bd list --assignee="$GC_AGENT" --status=open --json \
  | jq -r '.[] | select(.title | startswith("mol-refinery-patrol")) | .id'

# Step 3: assigned work beads (non-patrol)?
gc bd list --assignee="$GC_AGENT" --status=open --exclude-type=epic --json \
  | jq -r '[.[] | select((.title | startswith("mol-refinery-patrol")) | not)] | length'
```

| Step 1 | Step 2 | Step 3 | Verdict | Action |
|---|---|---|---|---|
| in_progress wisp exists | — | — | Mid-iteration | Finish the formula and reach next-iteration |
| empty | open wisp exists | any | Loop is broken — wisp poured but never started | Set the open wisp to in_progress and loop back into the formula |
| empty | empty | any > 0 | Loop is fully broken — no wisp at all, work waiting | Pour a new patrol wisp, assign to self, set in_progress, enter the formula |
| empty | empty | 0 | Genuinely idle | Acceptable to end turn only after find-work wait-timeout cap reached |

Turn ends only when step 1=empty AND step 2=empty AND step 3=0. Any
other state means the loop is mid-flight; continue.

### Recovery from a stuck state

If you wake into a session and the smoke test shows the loop is broken,
recover BEFORE doing any other work:

```bash
# Case 1: old wisp still in_progress, no new wisp
STUCK=$(gc bd list --assignee="$GC_AGENT" --status=in_progress --json \
  | jq -r '.[] | select(.title | startswith("mol-refinery-patrol")) | .id')
if [ -n "$STUCK" ]; then
  NEW=$(gc bd mol wisp mol-refinery-patrol --root-only \
    --var target_branch=main --var rig_name="$RIG" \
    --var binding_prefix="" --json | jq -r '.new_epic_id')
  gc bd update "$NEW" --assignee="$GC_AGENT" --status=in_progress
  gc bd mol burn "$STUCK" --force
fi

# Case 2: new wisp poured but stuck in open state (never started)
OPEN_WISP=$(gc bd list --assignee="$GC_AGENT" --status=open --json \
  | jq -r '.[] | select(.title | startswith("mol-refinery-patrol")) | .id')
if [ -n "$OPEN_WISP" ]; then
  gc bd update "$OPEN_WISP" --status=in_progress
fi
```

After recovery, re-read the formula from check-inbox/find-work and
execute on the active wisp. Process any work beads that queued during
the stuck window.
{{ end }}
