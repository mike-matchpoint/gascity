# telos-exec-monitoring

## Guardrails (BINDING — the first law of this pack)

"(A) packs carry primitives + sha-pinned POINTERS to the SYSTEM-TELOS snapshot, never a second copy of the law; (B) the monitoring pack emits telemetry/findings ONLY — conformance verdicts stay in the single evaluator/judge lane (GCD-WO-CSC-003 / GCD-WO-EVAL-001, shaped to blueprint ROL-5/6 pre-merge; no telos-specific judge role)."

Guardrail B is THIS pack's first law: every fragment here emits
telemetry/findings ONLY. No conformance verdicts, no judge role, no grading —
verdicts stay in the city's single evaluator/judge lane. In-repo law: the
"Telos pack topology" tail sections of
`specs/agent-work-orders/GCD-WO-CSC-003/006/007` bind where telos-layer content
may live; any telos primitive lands ONLY in the three telos packs.

## What this pack is

The telos layer's effectiveness/TELOS-GAP telemetry lane. A distinct contained
pack — never an extension of `codegen-support`, `gastown`, or any foreign pack.
In Gas City the pack is the IMPORT UNIT: cities import only the telos packs
their role mix uses, so the telos layer ships as separate packs for separate
processes (`telos-core` / `telos-codegen` / `telos-exec-monitoring` /
`telos-supervision`). This pack is the monitoring-process lane; it requires
`telos-core` imported alongside it.

## What this pack ships

- `template-fragments/telos-effectiveness-telemetry.template.md` — the
  countable effectiveness-telemetry emission duty: one record per observed
  telos-influenced event (STOP-pre-code, telos-cited rejection observed,
  post-hoc incident, unprompted same-diff telos update, card amendment
  observed, declared data mutation), with `verdict=n/a` at emission time —
  classifying a catch as true/false is review-lane work, never the emitter's.
- `template-fragments/telos-gap-finding.template.md` — the TELOS-GAP finding
  facet (grammar: `card | depth | pointer | nowhere | n/a`) attached to any
  finding that involves an agent decision made without design context; the
  finding routes to the owning lane, and the emitter never fixes the gap
  itself.

No agents, no formulas, no orders. Cities attach the fragments to
monitoring-capable roles via `append_fragments`; behavior tuning stays in the
sanctioned override layers — copying a pack file into a city repo to edit it is
a REJECT, exactly like any other imported pack.

## What this pack must never carry

- Verdict or judge duties of any kind (guardrail B — the first law above).
- Law text (pointers and read-at-runtime duties only — guardrail A).
- The city-side `telos-binding` fragment (a per-city delivery artifact).
- Business/domain content of any kind.
