> specs-version: 1 | updated: 2026-07-14
# TELOS — GasCity-Dev

## §1 Purpose (≤80 words)
The estate's agentic-factory engine: a Go orchestration-builder SDK for composing multi-agent coding workflows — the machinery that builds and operates the Matchpoint estate. Not a bounded context; its telos is operational. Engine law: ZERO hardcoded roles; Zero Framework Cognition (Go is transport, never reasoning); all role behavior is pack configuration. Priming duty: `gc prime <agent>` emits each agent's behavioral prompt — prompt templates are the behavioral specification. Domain-agnostic by REJECT-level gate: estate doctrine rides city packs, never engine code.

## §2 Role in the system (≤120 words) <!-- sync-stamp: SYSTEM-ATLAS.md@c9b58281beed — no chapter-1 row (rule-6 non-bounded-context); mirror = the seam-register row + the repo's charter docs -->
Factory-engine role (TD §D): one canonical Go object model, two typed projections — the `gc` CLI and the OpenAPI 3.1 HTTP+SSE supervisor API. Work substrate: Dolt-backed beads → molecules → formulas (MEOW). Packs (PackV2) are the import unit; city type = pack composition — code-gen: gastown + codegen-support; execution-monitoring: execution-city-operations + domain-handoff. City = business domain, rig = process/repository; cities run AWS-hosted only (EKS, aws-GasCity). Roles — mayor, deacon, boot, witness, polecat, refinery, evaluator, judge — are pack content. Flow: work-order beads → typed-selector dispatch → polecat generates on rig branches → evaluator/judge verify (C9) → refinery serializes merges → PRs land on estate repos. Today: EVAL-001 built; CSC band authored, unbuilt; cities paused; D6 telos packs ruled, unlanded.

## §3 Boundaries — what this repo must NEVER do
- Never hardcodes role names or puts decision/grading logic in Go — eligibility is typed selectors plus explicit metadata; agents never grade themselves (maker/checker/approver stay separate: polecat generates, evaluator evaluates, judge approves, refinery merges) (AGENTS.md:7-9, 194-197; GCD-WO-CSC-003 Non-Goals; GCD-WO-EVAL-001 Non-Goals).
- Never carries business-domain content in pack runtime surfaces — REJECT-level grep gates (`matchpoint|enrichment|vehicle`; no `master/` paths, no D-numbers, no AWS resource names); domain doctrine arrives only via city-pack fragments through the injection seams (GCD-WO-CSC-003 Non-Goals/Validation; GCD-WO-EVAL-001 Validation).
- Never owns the AWS platform layer — EKS, queues, buses, IAM, deploy belong to aws-GasCity; packs never name AWS resources (bus names are injected env; evidence durability is the generic `evidence_publish_cmd`/`evidence_fetch_cmd` pair) (aws-GasCity spec 00; GCD-WO-CSC-003 Non-Goals; GCD-WO-CSC-008).
- Never owns city bindings — providers, models, pools, doctrine fragments, `[rigs.formula_vars]` live in the city SOURCE repos; the 4 superseded city stubs are DEAD, never patch (GCD-WO-CSC-006 Goal; GCD-WO-CSC-007 provenance A1 §8).
- Never PRs upstream — estate work lands on fork `main` only (`mike-matchpoint/gascity`; upstream contribution is a separate, unrecorded decision) (GCD-WO-CSC-003 + GCD-WO-EVAL-001 Git Workflow).
- Never starts cities, daemons, or AWS surfaces while the standing pause holds — authoring plus repo-native structural tests only; live drills are only ever the vehicle-graph pilot, explicitly named, re-suspended after (every CSC WO "Cities PAUSED" clause).
- Never emits cross-city events except through the single approved emitter (`execution-city-operations/assets/scripts/publish-cross-city-event.sh`); exec-city direct `RepoBugReported.v1` emission is CLOSED — the overseer is the only filer (domain-handoff pack.toml; GCD-WO-CSC-008 Goal 1).
- Never tracks runtime state in status files — the process table is the source of truth; never bare `tmux kill-server`; agent work here is complete only after `git pull --rebase; bd dolt push; git push` succeeds (AGENTS.md:208-211, 242-245, 335-358).
### Adjudication-reserved surfaces (machine rows — compiled into RESERVED-SURFACES)
| surface / data family | reservation class | authority pointer |
|---|---|---|
| C9 verdict metadata on beads (`eval_verdict`, `eval_evidence`, `verdict_patch_id`, `eval_reject_count`, `judge_verdict`, `rejection_reason`, `decision_state`, `overseer_issue_id`, `residue`) — verdicts key to content-states via `git patch-id --stable`, never to sessions (re-wakes/crashes/swaps never invalidate); ONE shared reject budget (content rejections only — judge stale-content and infra re-routes NEVER increment); evaluator = sole `verdict_patch_id` writer (judge re-computes, never rewrites); `overseer_issue_id` never cleared by any agent; `residue` written fresh by polecat submit, never cleared by evaluator/judge — silent residue = REJECT (fold: dossier reserved rows 1+2) | contract shape — LAW as amended @f24adef56 + @c41ecd0a, not residue | GCD-WO-CSC-003 R2/R3 |
| role-name strings — in pack TOML/templates they are CORRECT content; the same string in Go source is a bug; the ZFC/zero-roles gate applies to Go only (dual misread runs both directions) | design-sealed | AGENTS.md:7-9, 229; GCD-WO-CSC-003 Non-Goals |
| `.gc/system/packs/` mirrors — runtime-materialized from the embedded PackFS on every `gc start`/`gc init`; never committed, never hand-written | rule-6 persisted-data (generated) | GCD-WO-CSC-003 Non-Goals; cmd/gc/embed_builtin_packs.go |
| `[rigs.formula_vars]` — NOT consumed at `bd mol wisp` pour time; `PromptContext` has no formula-vars accessor; city rig vars resolve at RUN TIME inside formula steps via the canonical `effective_rig_var` block — the "redundant" runtime lookup is load-bearing | design-sealed | GCD-WO-CSC-003 R4; refinery-wisp-pour-vars-override.template.md |
| empty `{{ define }}` seam fragments (`city-architecture-standards` / `city-evidence-doctrine` / `city-invariants`) = name reservations cities fill via `append_fragments`, template-comment form REQUIRED; `regenerate_on_reject` = RESERVED var name — documented, NOT implemented (packlint asserts its absence) (fold: dossier reserved rows 6+7) | design-sealed | GCD-WO-CSC-003 Goal 6 + Step 5; Non-Goals + R2 |
| pinned literals are IDENTITY — agent names (`evaluator`, `judge`), formula names (`mol-evaluate-task`, `mol-judge-task`; codegen-support uses `.formula.toml`, gastown bare `.toml` — follow each pack's convention), seam fragment names, `gc.kind` values (`eval_request`/`judge_request`), the six GCD-WO-CSC-005 fragment names, city-side `<domain>.eval_run_cohort.v1` naming; downstream Step-0 discovery gates bind BY NAME, renames break waves 24/25 | contract shape | GCD-WO-CSC-003 header + R2; GCD-WO-CSC-005 header; GCD-WO-EVAL-001 Goal 7 |
| merge-readiness = conflict-only (`git merge-tree --write-tree`); a behind-but-conflict-free branch MUST NOT be failed for staleness; verification only — resolving is the maker's job | contract shape | GCD-WO-CSC-003 R5 + Step 6.3 check 8 |
| typed-wire exceptions are enumerated design — `SessionRawMessageFrame` + `EventPayloadUnion` are the ONLY custom-marshal wire types; `/svc/*` is the single untyped path; SSE framing carved out in exactly two files; `internal/api/openapi.json` + `docs/schema/openapi.json` are GENERATED (pre-commit regenerates; `TestOpenAPISpecInSync`) | design-sealed / generated | specs/architecture.md §3.2, §3.4–3.9 |
| worker-boundary migration bypass list — the named remaining direct `session.Manager` sites are documented state, not violations to fix wholesale; the removed Agent/Handle interfaces must not be reconstructed | design-sealed (migration in progress since 12a0a848) | AGENTS.md:141-164 |
| doc/code conflict direction — DX wins, tutorials win, the glossary wins: update the DOC, never regress code to a stale doc; counterweight: specs/architecture.md is NORMATIVE with a same-commit maintenance rule for cited symbols | doc-adjudication | AGENTS.md:26-27, 174; engdocs/architecture/glossary.md:5-7; specs/architecture.md §8 |
| dolt ≥ 1.86.2 and bd 1.0.6 (estate fork `Matchpoint-Intelligence/Beads-Dev`) floors are load-bearing — older dolt can hang `dolt_backup sync` under heavy write load; bead shadows in Dolt remain the nudge durability layer | runtime premise | README.md:58-71; GCD-WO-CSC-001 Goal 3 |
### Credential-scope pointers (H1 adjacency)
| credential | scope | expiry class | owning refresh mechanism |
|---|---|---|---|
| — | none recorded (P1.3 dossier) | — | capability register UNWRITTEN |

## §4 Change law (verbatim single-writer copy of SYSTEM-TELOS head §4 — DO NOT EDIT HERE)
1. A change altering behavior, contracts, or structure described by this repo's `specs/` or ADRs
   updates those documents **in the same diff**; working code that orphans its docs is a REJECT.
2. Purpose/role/boundary motion (card §1–§3) requires an ADR + TELOS bump + SYSTEM-ATLAS row
   update **in the same motion**.
3. Pattern misfits are never resolved locally: STOP → LQ → judge (ADR-028).
4. Every governed-doc edit bumps its `specs-version` header and SPECS-INDEX row (gate RED
   otherwise).
5. If your task conflicts with card §1–§3, STOP and raise it — do not code around a telos.
6. **Persisted-data adjudication.** Declaring existing persisted/live data defective and
   rewriting, re-keying, or deleting it (any framing — repair, backfill, normalization,
   migration, cleanup of rows this work order did not create), or landing a contract/validation
   change that invalidates an existing persisted shape, is domain-design adjudication: STOP and
   raise it unless the work order explicitly scopes that exact operation (named row family +
   transform + before-state capture). While waiting: readers adapt to persisted reality.
   Authorized mutations are declared in evidence: row family, count, before-state pointer,
   authority cite.
7. **Cross-context changes ride versioned seams** (expand-contract): producer publishes v(n+1)
   with its fixture pack; consumers adopt in sequenced work orders; the old version retires —
   never an atomic multi-context landing (a task that appears to require one is a
   seam-versioning adjudication: STOP → judge). Foreign-context defects route to the owning
   lane, never repaired in place without a license. Exception: owner-declared
   wholesale-reshaping phases.


## §5 Depth pointers
| pointer | target |
|---|---|
| specs chapters | specs/00-overview.md · specs/01-core-invariants.md · specs/02-system-boundaries.md — UNWRITTEN (@0 lane; estate specs-anatomy adoption is an open owner question, dossier (g)1) |
| system head + atlas row | Matchpoint-Platform/specs/patterns/SYSTEM-TELOS.md (v1) · SYSTEM-ATLAS: NO chapter-1 row (rule-6 non-bounded-context); nearest atlas record = the `GasCity-{role}-{Env}` seam-register row (legacy CFN export family); operative charter sentences = TD §D final table row + owner ruling OR-1 (GCD-WO-EVAL-001:18-21) |
| DOMAIN-MODEL | DOMAIN-MODEL: WRITTEN — specs/architecture.md (object model, typed wire, event registry) + engdocs/architecture/glossary.md (authoritative terms — "this glossary wins") + engdocs/architecture/nine-concepts.md (primitive/mechanism taxonomy) + docs/specs/pack-spec.md (PackV2 pack/city/rig model); grain: beads are the universal persistence substrate — everything is a bead |
| SPECS-INDEX | specs/SPECS-INDEX.md — UNWRITTEN (@0 lane; index birth has NO assigned vehicle yet) |
| seam manifests | specs/registers/SEAM-MANIFEST.md — UNWRITTEN |
| mechanism inventory | specs/registers/MECHANISM-INVENTORY.md — UNWRITTEN |
| runtime premises | specs/registers/RUNTIME-PREMISES.md — UNWRITTEN |
| pattern stubs | specs/patterns/ — UNWRITTEN |
| load-bearing ADRs | specs/adr/ — UNWRITTEN; normative in-repo law = specs/architecture.md (header-less but normative: CI-guard table §5, same-commit maintenance rule §8) + AGENTS.md (settled decisions, design principles, active migrations; CLAUDE.md = `@AGENTS.md`) |
| estate work orders | specs/agent-work-orders/ (9 WOs) — GCD-WO-CSC-003 is the C9 verdict-contract authority (R2/R3, amended @f24adef56 + @c41ecd0a); its codegen-support pack README becomes the C9 binding doc when wave 23 executes |
| mandatory pre-reading (API/CLI/events work) | engdocs/architecture/api-control-plane.md + engdocs/contributors/huma-usage.md (AGENTS.md:100-113) |
| engine deep docs | engdocs/architecture/ (beads, formulas incl. SLING-path precedence, life-of-a-bead, life-of-a-molecule, controller, session) · engdocs/contributors/ (codebase-map, primitive-test, reconciler-debugging) · TESTING.md · CONTRIBUTING.md · TRACK3_CONTRACT.md |
| pack law | docs/specs/pack-spec.md (PackV2, authoritative; pack schema 2) · docs/guides/shareable-packs.md (formula-name precedence) |
| estate-side law | aws-GasCity/specs/00 + 17 (+ ADR-023) — hosting + city-topology law (city = business domain, rig = process/repo; topology from `PROCESS_SPECS`) · master/city-scaling-improvements/wo-authoring-kit.md (C9/C10/C11 program contracts) |
| telos delivery (ruled, NOT landed) | master/telos/agent-execution-plan.md rows P1.5/P1.5b/P1.5c/P1.5d + delta D6 — the ruled three-pack delivery design (telos-core / telos-codegen / telos-exec-monitoring + sha-pinned SYSTEM-TELOS snapshot + city doctrine fragments); zero `telos` strings in-repo today — never treat the packs as existing |
