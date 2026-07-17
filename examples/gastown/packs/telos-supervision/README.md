# telos-supervision

## Guardrails (BINDING)

"(A) packs carry primitives + sha-pinned POINTERS to the SYSTEM-TELOS snapshot, never a second copy of the law; (B) the monitoring pack emits telemetry/findings ONLY — conformance verdicts stay in the single evaluator/judge lane (GCD-WO-CSC-003 / GCD-WO-EVAL-001, shaped to blueprint ROL-5/6 pre-merge; no telos-specific judge role)."

The supervisor/overseer lane of the telos layer. A distinct contained pack —
never an extension of `codegen-support`, `gastown`, or any foreign pack. In
Gas City the pack is the IMPORT UNIT: cities import only the telos packs their
role mix uses, so the telos layer ships as separate packs for separate
processes (`telos-core` / `telos-codegen` / `telos-exec-monitoring` /
`telos-supervision`). Supervision is a distinct lane from codegen-priming and
monitoring-emission — hence the fourth pack (pack-topology ruling v3,
owner-ratified 2026-07-17, verbatim "Recommended Architecture A, approved.";
spec of record: dossier-city-telos-channel-20260717 §5.0; supersedes the
three-pack phrasing of the 2026-07-14 ruling).

## The fragment-not-agent law (BINDING)

A supervisor primitive in this pack is an **injection FRAGMENT for the city's
overseer role, never an agent**. This pack defines NO agents, NO formulas, and
NO orders — the zero-agents shape of the telos pack family is unchanged by the
v3 ruling. The city's supervisor primitive stays the mayor template of the
pack that supplies it (every mayor variant); this pack is the ONE authored
home of that role's telos duties, injected into every variant so a law change
lands once and reaches all of them.

## What this pack ships

- `template-fragments/telos-overseer-law.template.md` — the overseer telos
  law: telos-first adjudication + the option-space law + the design-space
  scope extension; the capability-wall BUILD-branch rider (parity with the
  loop's standing ruling SR-25; declared obligation
  `sr25-gascity-parity-rider` folded at authoring); the `telos.incident`
  recording duty (opened/closed pair on the city's telemetry partition, same
  row/gate grammar as the estate catalog's row, read through this substrate's
  derived binding — the city's declared telemetry contract / C9 record
  contract, C9 authority `specs/agent-work-orders/GCD-WO-CSC-003`; source of
  record: the estate register CONTRACT-TELOS-TELEMETRY §4);
  knowledge-strengthens-the-town; the directive net-benefit bar; and the telos
  feeders of the obligations view.

No agents, no formulas, no orders. Cities attach the fragment to their
overseer role via the pack-patch lever
(`[[patches.agent]] name = "mayor" append_fragments = ["telos-overseer-law"]`);
behavior tuning stays in the sanctioned override layers — copying a pack file
into a city repo to edit it is a REJECT, exactly like any other imported pack.

## Activation gate (dormant-honest)

Import ≠ inject (the city-topology bootstrap rule R10): this pack has
behavioral effect only when a city BOTH imports it AND wires
`telos-overseer-law` into its overseer role's injection list. That wiring
rides the staged-dormant city config branches behind the gc image gate; the
fragment's city-runtime telemetry duties additionally gate at city resume.
Until then this pack is registered, bundled, and dormant — deliberately so.
Absence of the fragment from a primed overseer prompt in a telos-wired city
is a LOUD defect (`gc prime --strict` enforces rendering; the doctor advisory
lane reports unwired telos fragments).

## What this pack must never carry

- Law text. The system telos law lives in the city-side snapshot (a per-city
  delivery artifact) and the per-repo `specs/TELOS.md` cards. This pack carries
  pointers and read-at-runtime duties only — a copied law text is a stale fork
  by construction.
- An agent, formula, or order definition of any kind (the fragment-not-agent
  law above).
- Verdict authority. The overseer adjudicates and routes; conformance verdicts
  stay in the city's single evaluator/judge lane.
- Business/domain content of any kind. Domain specificity arrives at runtime
  through the city snapshot and repo cards.

In-repo law: the "Telos pack topology" tail sections of
`specs/agent-work-orders/GCD-WO-CSC-003/006/007` bind where telos-layer content
may live. Any telos primitive lands ONLY in the telos packs.
