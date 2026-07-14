# telos-codegen

The telos layer's priming/conscience lane for codegen roles. A distinct
contained pack — never an extension of `codegen-support`, `gastown`, or any
foreign pack. In Gas City the pack is the IMPORT UNIT: cities import only the
telos packs their role mix uses, so the telos layer ships as separate packs for
separate processes (`telos-core` / `telos-codegen` / `telos-exec-monitoring`).
This pack is the codegen-process lane; it requires `telos-core` imported
alongside it.

## Guardrails (BINDING)

"(A) packs carry primitives + sha-pinned POINTERS to the SYSTEM-TELOS snapshot, never a second copy of the law; (B) the monitoring pack emits telemetry/findings ONLY — conformance verdicts stay in the single evaluator/judge lane (GCD-WO-CSC-003 / GCD-WO-EVAL-001, shaped to blueprint ROL-5/6 pre-merge; no telos-specific judge role)."

In-repo law: the "Telos pack topology" tail sections of
`specs/agent-work-orders/GCD-WO-CSC-003/006/007` bind where telos-layer content
may live. Any telos primitive lands ONLY in the three telos packs.

## What this pack ships

- `template-fragments/telos-codegen-priming.template.md` — primes a codegen
  role with the read order (city snapshot → repo `specs/TELOS.md` card if
  present → the change law binds every commit) and the STOP-on-conflict duty
  (bead conflicts with purpose/boundaries → STOP, file a question bead, never
  code around a telos).

This pack emits NOTHING — no telemetry, no findings, no verdicts. It primes
design conscience at the maker seat only; conformance verdicts stay in the
city's single evaluator/judge lane, and no fragment here grants review or
judge authority.

No agents, no formulas, no orders. Cities attach the fragment to codegen roles
via `append_fragments` (or `global_fragments` where every role generates code);
behavior tuning stays in the sanctioned override layers — copying a pack file
into a city repo to edit it is a REJECT, exactly like any other imported pack.

## What this pack must never carry

- Law text (pointers and read-at-runtime duties only — guardrail A).
- The city-side `telos-binding` fragment (a per-city delivery artifact).
- Any emission or verdict duty (emission is the monitoring pack's lane;
  verdicts are the evaluator/judge lane's).
- Business/domain content of any kind.
