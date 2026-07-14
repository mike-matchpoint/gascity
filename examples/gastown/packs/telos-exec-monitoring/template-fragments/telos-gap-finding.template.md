{{ define "telos-gap-finding" }}
---

## TELOS-GAP FINDINGS — CLASSIFY WHERE THE DESIGN CONTEXT LIVED (EMIT ONLY)

FIRST LAW (pack guardrail, verbatim): the monitoring pack emits
telemetry/findings ONLY — conformance verdicts stay in the single
evaluator/judge lane.

When a finding or incident record you are emitting involves an agent decision
made without design context, the record carries a mandatory facet:

```
TELOS-GAP: card | depth | pointer | nowhere | n/a
```

- `card` — the missing statement belongs on the working repo's
  `specs/TELOS.md` card (§1–§3 tier).
- `depth` — it belongs in a specs/ chapter reachable from the card's §5
  pointers; the card gets at most a pointer line.
- `pointer` — the content exists, but the card's §5 pointer tier was missing
  or stale.
- `nowhere` — the governing statement is written nowhere. Flag it for owner
  ratification before anyone codifies it: a `nowhere` finding is a question,
  never a unilateral edit.
- `n/a` — the incident did not involve a design-context gap.

Emission duties:

- Emit the finding as a record (bead note/evidence, plus mail to the owning
  lane) with the facet line and the evidence that supports it.
- Route, never repair: from a monitoring seat you never edit cards, specs, or
  code to close the gap yourself — the owning lane does, under its own gates.
- Never attach a conformance verdict to a finding. Describing what you
  observed is telemetry; judging whether work conforms is the evaluator/judge
  lane's job alone.

{{ end }}
