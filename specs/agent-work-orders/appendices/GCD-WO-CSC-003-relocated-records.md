# GCD-WO-CSC-003 — relocated conformance/audit records (companion)

> Companion to `specs/agent-work-orders/GCD-WO-CSC-003-evaluator-judge-primitives.md`.
> BINDING with the WO (the WO's header tail-amendment note covers these sections; each
> relocated section's in-WO heading carries the pointer here). Relocated VERBATIM
> 2026-07-15 as harness 100k dispatch-bundle file-cap remediation (axis-6
> WO-FILE-OVER-CAP, validate_dag_completeness.py — over-cap WO bytes are silently
> sliced from generator context; sizing law: SKILL-work-order-audit-and-authoring §1B).
> ZERO content change: pre-relocation in-file bytes at this repo's
> GCD-WO-CSC-003-evaluator-judge-primitives.md @ `fdec7f36`. This appendices/ subdir is
> outside every harness WO glob (non-recursive `*.md` over specs/agent-work-orders/).

## Blueprint conformance (amended 2026-07-14 — LAW-4/ROL-5/ROL-6/QST/GEN-6)

Tail amendment — BINDING (see the header note). This WO was reshaped pre-dispatch to
the ratified generation-system blueprint
(`master/generation-architecture/BLUEPRINT.md` v1.4); the C9 contract it authors is
the substrate the whole CSC lane inherits, so conformance lands HERE, once. The edits
are integrated above (Goal 1/2/4, R2, R3, Step 2, Step 6, Step 7, Step 8 items 11–13,
Step 9, AC 2/3/6, Risks); this section is the summary and the citation map:

1. **Content-state verdict keying (LAW-4/STM-5).** New C9 key `verdict_patch_id` =
   `git patch-id --stable` of the evaluated diff (`origin/$TARGET...origin/$BRANCH`),
   written with EVERY `eval_verdict` write — the evaluator is the sole writer; the
   judge re-computes and matches, never rewrites (R2 grammar row; Step 6.2 compute;
   Step 6.5 writes; Step 7.2 re-compute + match). Verdicts attach to content-states, never worker
   lifetimes: re-wakes, crashes, and agent swaps never invalidate a verdict — only a
   changed patch-id does. The judge re-computes and compares before approving; a
   mismatch is the R3 stale-content re-route (back to `gc.kind=eval_request`,
   budget-neutral — staleness is a race, not a defect). Downstream: the
   `evaluator_gated` refinery degrade (GCD-WO-CSC-005) is valid ONLY at a matching
   patch-id.
2. **Acting evaluator (ROL-5/LAW-7).** Capability self-test before any verdict is
   trusted (Goal 1; Step 2 §6b; Step 6.2): one real command whose captured output is
   the first evidence line; no command execution ⇒ no verdict authority ⇒
   infrastructure failure, never a judgment. Verdicts carry executed-commands evidence
   with real output excerpts (the existing `.gc/evidence/` JSONL — unchanged path). An
   unsubstantiated PASS (missing/empty/unreproducible evidence or missing
   `verdict_patch_id`) is an infrastructure failure of the evaluation lane: the judge
   re-routes it to re-evaluation, budget-neutral, never accepts it and never charges
   the maker (R3 infra re-route row; Step 7.1).
3. **Conditional judge (ROL-6).** The judge runs ONLY on risk markers, evaluated
   deterministically by the evaluator at PASS time and recorded in evidence (R2
   risk-marker table): zero-source/docs-only diff (no-diff/empty-leg/docs-band
   classes), first PASS after any rejection (`eval_reject_count > 0`),
   corrective-class bead (`overseer_issue_id` / repair `target_branch`). A no-marker
   PASS writes `judge_verdict=NOT_REQUIRED` + direct refinery handoff (R3 row; Step
   6.5). Deterministic properties stay with the deterministic gates — the judge never
   re-verifies them and never runs as ceremony.
4. **Question package (QST-1/2/5) — scoped note.** This WO's mayor escalation
   (`decision_state=mayor_action` + mail) is an ESCALATION lane and keeps its pinned
   shape; the structured decision-package/batching/ACK contract for worker questions
   lands with the producers: the polecat blocker fragment (GCD-WO-CSC-005) and the
   router's mayor-action package (GCD-WO-CSC-004). No re-declaration here (C9
   discipline: defined once, imported).
5. **Residue rows (GEN-6/GOV-4).** New C9 key `residue` (R2 shape row): structured
   delivered / not-delivered / known-gap rows, every gap mapped to an EXISTING bead or
   WO. Writer = the polecat submit sequence (GCD-WO-CSC-005); this WO's evaluator
   enforces presence + honesty — silent residue = REJECT (Step 6.3 check 7); rows are
   never cleared by evaluator/judge and are consumed as premises by downstream
   planning (GCD-WO-CSC-004's router).

Estate-authority note (FLAG, no action here): the kit C9 table in
`master/city-scaling-improvements/wo-authoring-kit.md` predates this amendment; this
WO remains the C9 authority per its own header ("defined ONCE here, everyone else
imports") — the kit table gains `verdict_patch_id`/`NOT_REQUIRED`/`residue` via the
Mayor's governed-doc lane, not via this WO.

## Telos injection coverage — Finding (code-cited; re-anchor by content, not line number)

> Relocated VERBATIM 2026-07-15 (same file-cap remediation lane as the header note;
> pre-relocation in-file bytes at this repo's
> GCD-WO-CSC-003-evaluator-judge-primitives.md @ `bb4a7083`). Subsection of the WO's
> `## Telos injection coverage (amended 2026-07-15 — Track-D investigation)` tail; the
> executable Scope additions remain in the WO body and stand alone.

- Plain-`.md` prompts never receive fragments: `renderPromptWithMeta`
  (`cmd/gc/prompt.go:100–106` @ origin/main `76591be5`; content anchor: the
  `!isPromptTemplatePath(templatePath)` early return) exits BEFORE shared-template
  loading and the `injectFragments` append loop (`cmd/gc/prompt.go:173–187`); the
  `AgentDefaults.AppendFragments` doc comment (`internal/config/config.go`): "plain .md
  remains inert."
- The implicit per-provider utility agents ride exactly that path: `InjectImplicitAgents`
  (`internal/config/config.go:3205` area; content anchor: `promptTemplate :=
  citylayout.SystemPacksRoot + "/core/assets/prompts/pool-worker.md"`) gives every
  implicit provider agent — city scope AND per-rig, one per configured provider — that
  plain-md template and `default_sling_formula` fallback `"mol-do-work"`; estate count at
  investigation: 8 implicit utility agents.
- Consequence: those rendered prompts carry ZERO city fragments (no
  `workspace.global_fragments`, no `[agent_defaults]` or per-agent `append_fragments`) —
  zero telos fragments; real work slung at an implicit provider agent gets an un-primed
  session outside every doctrine seam this WO authors.

### Anchor re-verification record (WOC-8 — 2026-07-14, origin/main @ `e3a3a1673600`)

The in-body @`a47df8f5` claim ("commits past `c85d92cf` are CSC spec-file-only") was TRUE
at `a47df8f5`; supplement: `a47df8f5..e3a3a167` is NOT spec-only, BUT
`git diff --name-only c85d92cf..e3a3a167 -- examples/gastown/packs/codegen-support
examples/gastown/packs/gastown` is EMPTY and `agents/landing-arbiter/agent.toml` is
byte-identical. Drift notes: (a)
`cmd/gc/embed_builtin_packs_test.go` (Step 9c's precedent) HAS changed — re-anchor
`TestCodegenSupportBuiltinPackComposesWithGastown` by name, not "~line 182"; (b) the
Step-0(b) gate passes TODAY (3 formulas; EVAL-001 = MERGED_DEV). Every R1/R2 pack-content
anchor holds at `e3a3a167`.

## WOC map (WO-CS v1 conformance audit, 2026-07-14 — Track C)

> Relocated VERBATIM 2026-07-17 (same file-cap remediation lane as the header note;
> pre-relocation in-file bytes at this repo's
> GCD-WO-CSC-003-evaluator-judge-primitives.md @ `d44c5c04`). Subsection of the WO's
> `## WO-CS v1 conformance (audited 2026-07-14 — Track C)` tail; "below" in the
> verbatim text means below THAT tail in the WO file, where the referenced sections
> remain.

WOC-1 UPGRADED in place (R-C2 live-tier terms — header block). WOC-2 in-body, verified
(each AC names its backing test). WOC-3 in-body (§ Non-Goals), verified complete — every
deferred seam names its owner there; `regenerate_on_reject` = RESERVED name, not a
deferred seam. WOC-4/-5/-6/-9/-11 + GEN-6 residue ADDED below (WOC-4 = `## Premises
(drift gate)` + `## Specs impact`; WOC-11's T3 row records the C9 registration GAP →
AC-T2; residue rides AC-T1). WOC-7 in-body + declaration below. WOC-8 in-body EXEMPLAR —
RE-VERIFIED below. WOC-10 in-body (pack README = the C9 binding doc; Done Means
checklist) + `## Specs impact` below (incl. the index-motion N/A).

