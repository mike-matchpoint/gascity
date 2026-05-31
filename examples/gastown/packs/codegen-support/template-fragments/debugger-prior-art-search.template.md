{{ define "debugger-prior-art-search" }}
---

## PRIOR-ART SEARCH — DISCIPLINE

Before deciding, search for similar past bugs in this component.
Precedent matters: a prior fix for the same class of bug is the
default shape unless evidence justifies diverging.

### Search

```bash
# Open bugs in same component (siblings — flag dependency or dedup
# concerns).
PRIOR_OPEN=$(gc --rig "$GC_RIG" bd list --type=bug --status=open \
  --metadata-field component="$COMPONENT" --json \
  | jq -r --arg me "$BUG" '[.[] | select(.id != $me)] | .[].id')

# Closed bugs in same component (prior precedent).
PRIOR_CLOSED=$(gc --rig "$GC_RIG" bd list --type=bug --status=closed \
  --metadata-field component="$COMPONENT" --json \
  | jq -r --arg me "$BUG" '[.[] | select(.id != $me)] | .[].id')

# Also search by bug.class across components — same class of failure
# elsewhere may indicate a systemic fix.
PRIOR_BY_CLASS=$(gc --rig "$GC_RIG" bd list --type=bug --status=closed \
  --metadata-field bug.class="$BUG_CLASS" --json \
  | jq -r --arg me "$BUG" '[.[] | select(.id != $me)] | .[].id' | head -5)
```

### Read the closed precedents

For each closed prior bug:

```bash
gc --rig "$GC_RIG" bd show "$PRIOR" --json \
  | jq '.[0] | {
    title, description,
    decision_class: .metadata.decision_class,
    resolution_class: .metadata.resolution_class,
    repair_beads: .metadata.repair_beads,
    classification_reasoning: .metadata.classification_reasoning
  }'
```

Read the description (architectural synthesis) and follow
`repair_beads` to see what actually shipped.

### Decide alignment

For each candidate decision shape:

- If a prior precedent fixed a similar bug with shape X, default to
  shape X unless your evidence makes a case for diverging.
- If you diverge from precedent, your `classification_reasoning`
  MUST explicitly cite the precedent and the reason for divergence.
  "Bug $PRIOR was fixed with shape X; I'm choosing shape Y because
  <specific architectural difference>."
- If no precedent exists, your decision sets the precedent for
  future cycles. Be deliberate — and if your decision establishes a
  novel shape that isn't documented in specs/ADRs, emit a
  spec-update repair bead per `landing-arbiter-adr-emission`.

### Record on the bug

```bash
gc --rig "$GC_RIG" bd update "$BUG" \
  --set-metadata prior_art="${PRIOR_OPEN},${PRIOR_CLOSED}"
```

If both are empty, set `prior_art=none`.

### Open-siblings handling

If `PRIOR_OPEN` is non-empty, evaluate whether the bugs are:

- **Duplicates** of yours → the producer's dedup query should have
  caught this; if not, mark your bug
  `decision_state=closed-duplicate` with `duplicate_of=<id>` and
  exit.
- **Related but distinct** → proceed normally; cite the relation in
  `classification_reasoning` and consider whether the open sibling's
  resolution would obviate yours.
- **Dependencies of yours** → add a `gc bd dep add` edge from yours
  to theirs, then proceed.
{{ end }}
