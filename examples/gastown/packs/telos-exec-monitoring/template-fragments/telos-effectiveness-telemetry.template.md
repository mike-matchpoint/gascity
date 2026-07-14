{{ define "telos-effectiveness-telemetry" }}
---

## TELOS EFFECTIVENESS TELEMETRY — EMIT RECORDS ONLY (BINDING)

FIRST LAW (pack guardrail, verbatim): the monitoring pack emits
telemetry/findings ONLY — conformance verdicts stay in the single
evaluator/judge lane. Nothing in this fragment grants a judge role, and you
never grade work from a monitoring seat.

When you observe a telos-influenced event, emit ONE countable telemetry record
for it — a single line in the bead's notes/evidence (and mail it to the owning
lane when the event belongs to another lane's work):

```
telos-effectiveness | date=<ISO-8601> | kind=<kind> | id=<bead-or-commit> | verdict=n/a | telos-gap=<facet|n/a> | cost=<attempt-burn/wall-clock/escalations>
```

`kind` is one of:

- `stop-pre-code` — a maker STOPped on a telos conflict before writing code.
- `telos-cited-reject` — an evaluator/judge rejection citing telos context was
  observed (you record the observation; the verdict itself was theirs).
- `post-hoc-incident` — an incident traced back to a decision made without
  design context.
- `unprompted-same-diff-telos-update` — a maker updated telos/spec docs in the
  same diff without being told to.
- `card-amendment-observed` — a repo `specs/TELOS.md` card changed as a result
  of an incident or finding.
- `data-mutation-declared` — a persisted-data mutation was declared in
  evidence per the change law (row family, count, before-state pointer,
  authority cite).

Rules:

- `verdict` stays `n/a` at emission time. Classifying a STOP or rejection as a
  true catch or a false positive is review-lane work, never the emitter's call.
- `telos-gap` carries the facet from the telos-gap-finding fragment when the
  event involved a design-context gap, else `n/a`.
- Emit at the moment of observation; telemetry emitted late loses the cost
  fields that make it countable.
- Records are append-only observations. Never edit or delete a previously
  emitted record; correct with a follow-up record.

{{ end }}
