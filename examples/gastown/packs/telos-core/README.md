# telos-core

Shared core primitives of the telos layer. A distinct contained pack — never an
extension of `codegen-support`, `gastown`, or any foreign pack. In Gas City the
pack is the IMPORT UNIT: cities import only the telos packs their role mix
uses, so the telos layer ships as separate packs for separate processes
(`telos-core` / `telos-codegen` / `telos-exec-monitoring`), with this pack
carrying what the other two share.

## Guardrails (BINDING)

"(A) packs carry primitives + sha-pinned POINTERS to the SYSTEM-TELOS snapshot, never a second copy of the law; (B) the monitoring pack emits telemetry/findings ONLY — conformance verdicts stay in the single evaluator/judge lane (GCD-WO-CSC-003 / GCD-WO-EVAL-001, shaped to blueprint ROL-5/6 pre-merge; no telos-specific judge role)."

In-repo law: the "Telos pack topology" tail sections of
`specs/agent-work-orders/GCD-WO-CSC-003/006/007` bind where telos-layer content
may live. Any telos primitive lands ONLY in the three telos packs.

## What this pack ships

- `template-fragments/telos-snapshot-pin.template.md` — the shared snapshot
  consumption primitive: read the city-root `specs/SYSTEM-TELOS.md` snapshot,
  verify its provenance header (`snapshot-of … @ <sha256-12> | specs-version: N
  | synced: …`), surface `telos: system vN` for evidence, and fail VISIBLY
  (`TELOS SNAPSHOT: MISSING`) when the snapshot is absent — loud, never silent.
- `template-fragments/telos-evidence-line.template.md` — the common evidence
  template: how every role records its priming versions
  (`telos: system vN / repo vM`) in bead close evidence so stale priming is
  visible in evidence review.

No agents, no formulas, no orders. Cities attach the fragments to roles via
`global_fragments` / `append_fragments`; behavior tuning stays in the
sanctioned override layers — copying a pack file into a city repo to edit it is
a REJECT, exactly like any other imported pack.

## What this pack must never carry

- Law text. The system telos law lives in the city-side snapshot (a per-city
  delivery artifact) and the per-repo `specs/TELOS.md` cards. This pack carries
  pointers and read-at-runtime duties only — a copied law text is a stale fork
  by construction.
- The city-side `telos-binding` fragment. That fragment is a per-city delivery
  artifact shipped with each city's source repo, not pack law.
- Business/domain content of any kind. Domain specificity arrives at runtime
  through the city snapshot and repo cards.
