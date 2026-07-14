# Work Order: GCD-WO-CSC-003 — evaluator + judge primitives (codegen-support): per-task adversarial evaluation, verdict metadata contract (C9 authority), empty-default city seams

NOTE: this WO is long — read the FULL file at
`specs/agent-work-orders/GCD-WO-CSC-003-evaluator-judge-primitives.md` in your worktree
before implementing; tail amendments are BINDING. This WO is the **C9 contract authority**
for the whole CSC program — the names and key grammars pinned here are what
`GCD-WO-CSC-005/006/007` discover and bind at execution time. Deviating from a pinned
literal here breaks three downstream WOs.

Execution classification: Dev-only (pack content — agents, formulas, template fragments,
pack README — plus repo-native Go structural tests; no AWS, no deploy surface, no city
runs). `boundary=dev`, **wave 23** (CSC program band 23/24/25), `blocked_by`
`GasCity-Dev::GCD-WO-EVAL-001-generic-eval-execution-primitives` (wave 18 — cross-wave,
parser-safe; its zero-domain-literals grep gate, ZFC discipline, and agents-never-grade-
themselves layering rules BIND this WO).
Consumed by (same-wave + downstream): `GasCity-Dev::GCD-WO-CSC-005-polecat-diligence-refinery-merge-only`
(wave 23 — same-wave apply_deps DIRECT-WRITE edge `GCD-005←GCD-003` per kit A1 §4),
`GasCity-Dev::GCD-WO-CSC-006-city-pack-binding-pilot` (wave 24),
`GasCity-Dev::GCD-WO-CSC-007-city-pack-binding-fanout` (wave 25).

> **Provenance (binding):** CSC program contract authority
> `master/city-scaling-improvements/wo-authoring-kit.md` — **C9** (this WO IS the C9
> authority: "Evaluator/judge verdicts — authority `GCD-WO-CSC-003`"), **ADDENDUM A1 §2**
> (`overseer_issue_id` bead-metadata key — defined in the C9 table here, marker-emission
> duty is GCD-WO-CSC-005's), K2 (bounded context: GasCity-Dev row), K6 (test discipline).
> Backlog + sequencing: `master/city-scaling-improvements/gap-analysis-and-build-plan.md`
> §5 row 3, §6 ("Evaluator/judge verdict metadata keys + evidence artifact shape |
> GCD-WO-CSC-003 | GCD-WO-CSC-005/006/007"). Design record:
> `aws-GasCity/docs/investigations/2026-07-city-scaling-improvements.md` WS2 (owner ruling
> **D10**, 2026-07-08: evaluator+judge pack agents on Claude Opus; resume-and-fix retry
> semantics; `regenerate_on_reject` var reserved; harness diligence ported generic).
> Layering conventions: `specs/agent-work-orders/GCD-WO-EVAL-001-generic-eval-execution-primitives.md`
> (zero domain literals upstream; Zero Framework Cognition — pack content only; agents
> never grade themselves). Process: root `SKILL-work-order-audit-and-authoring.md` §1.1–1.5.
> Verified at authoring (2026-07-08): `GasCity-Dev` `origin/main` @
> `a47df8f5adbc7b8e4243ae344360c2dbbf2c864f` (read-only `git log -1 --format=%H
> origin/main`; the commits past `c85d92cf` are CSC spec-file-only — every pack-content
> file/line reference in this WO is byte-identical at both SHAs). Re-verify at
> execution time.
> Ledger stem: `GasCity-Dev::GCD-WO-CSC-003-evaluator-judge-primitives`.

## Goal

The `codegen-support` pack (`examples/gastown/packs/codegen-support/`) gains **generic,
domain-agnostic per-task evaluator and judge primitives** that port the
matchpoint-loop-harness's generator→evaluator→judge diligence
(`matchpoint-loop-harness/mlh/prompts/{evaluator,stop_judge}.md` — estate code root,
siblings of this repo) into GasCity code-generation cities, with **ZERO
MatchPoint/refactor-specific content** (the load-bearing constraint — MatchPoint doctrine
text lands ONLY in city packs via the injection seams, per D10 + GCD-WO-EVAL-001's OR-1
layering). Clean end state:

1. **NEW `evaluator` agent** (`agents/evaluator/{agent.toml,prompt.template.md}`):
   **rig-scope** one-shot pool agent (structural precedent: `agents/landing-arbiter/`),
   typed `work_selector` on `metadata "gc.kind" = "eval_request"`, default pool
   `max_active_sessions = 4`, provider `claude`, `model = "opus"` / `effort = "high"` via
   `[option_defaults]`. Adversarial acting stance: assume-broken, act-don't-read, ordered
   checks stopping at the first substantiated failure, deliverable-in-diff,
   anti-fabrication hunt, band-aid detection, conflict-only merge-readiness, run-to-
   completion. Evaluates the polecat's PUSHED branch in its own worktree (what will
   actually merge). **Acting is proven, not assumed (blueprint ROL-5):** the evaluator
   runs a capability self-test — one trivial real command whose captured output becomes
   the FIRST evidence line — before any check; a session that cannot execute commands
   has NO verdict authority: it reports the infrastructure failure (mayor mail) and
   writes NO verdict (an unsubstantiated verdict is an infrastructure failure, never a
   judgment).
2. **NEW `judge` agent** (`agents/judge/{agent.toml,prompt.template.md}`): rig-scope
   one-shot pool agent, selector on `"gc.kind" = "judge_request"`, default pool
   `max_active_sessions = 2`, claude/opus/high. **CONDITIONAL (blueprint ROL-6): the
   judge runs ONLY on risk-marker PASSes** — the evaluator routes a PASS to
   `judge_request` only when a deterministic risk marker fires (R2 risk-marker table);
   a no-marker, evidence-backed PASS records `judge_verdict=NOT_REQUIRED` (with the
   marker checklist outcome in evidence) and hands off to the refinery directly. The
   judge never runs unconditionally on every PASS and never re-verifies properties
   owned by deterministic gates. Port of the harness stop-judge:
   maker-checker on the status transition ONLY — verifies the evaluator PASS is
   evidence-backed (spot-reproduces ≥1 executed command), re-confirms deliverable-in-diff,
   re-runs the conflict-only merge check, never resolves anything. Judge-PASS hands the
   bead to the refinery (existing handoff shape); judge-REJECT returns it to the polecat
   pool with a one-line reason.
3. **Formulas `formulas/mol-evaluate-task.formula.toml` + `formulas/mol-judge-task.formula.toml`**
   (`contract = "graph.v2"`, wisp-style steps read in order — refinery-patrol precedent):
   the canonical evaluation/judging procedures, poured per claimed bead by the agent
   prompts.
4. **Verdict metadata contract (C9 — defined ONCE here, everyone else imports):** bead
   metadata keys `eval_verdict`, `eval_evidence` (path/URI to a JSONL artifact of
   `{"command": …, "output_excerpt": …}` objects), **`verdict_patch_id` (the
   content-state key: `git patch-id --stable` of the evaluated diff — every verdict is
   keyed to the CONTENT it proved, blueprint LAW-4/STM-5)**, `eval_reject_count`,
   `judge_verdict`, `rejection_reason`, `decision_state`, `overseer_issue_id` (A1 §2),
   **`residue` (structured close-out rows — delivered / not-delivered / known-gap,
   GEN-6)** — with a full writer/clearer ownership table in the pack README. **A PASS
   without the evidence artifact is invalid (the judge rejects it), and any verdict
   without `verdict_patch_id` is invalid — verdicts attach to content-states, never to
   sessions: re-wakes, crashes, and agent swaps never invalidate a verdict; only
   content change does.**
5. **Escalation rule:** rejections share ONE budget (`eval_reject_count`, incremented by
   evaluator AND judge rejections). At `max_eval_rejects` (formula var, default `"2"`)
   the rejecting agent sets `decision_state=mayor_action`, clears routing, and sends
   mayor mail `[EVALUATOR ACTION REQUIRED]` — the harness Q_REJECT analog. Retry
   semantics are **resume-and-fix** (D10): a rejected bead returns to the polecat pool
   with `rejection_reason` and its branch intact; `regenerate_on_reject` is a RESERVED
   var name, documented but NOT implemented.
6. **Empty-default city injection seams:** three fragment files
   `template-fragments/{city-architecture-standards,city-evidence-doctrine,city-invariants}.template.md`,
   each an EMPTY `{{ define }}` (name reservation). Cities deliver content by defining
   the same names in their own city-pack `template-fragments/` and appending them via
   `append_fragments` on the evaluator/judge rig patches (the mechanism GCD-WO-CSC-006
   pins city-side). A compose-level test proves the city define wins the name collision.
7. **Pack README (`examples/gastown/packs/codegen-support/README.md`) = the C9 binding
   doc** GCD-WO-CSC-006 R1b discovers: pipeline overview, verdict-key table, routing
   selectors, evidence grammar, escalation + mayor re-arm recipe, retry semantics +
   reserved `regenerate_on_reject`, the city-binding recipe (**rig scope stated
   explicitly**), and an explicit "no per-profile `CLAUDE_CONFIG_DIR` required" statement.
8. **Repo-native tests:** packlint string-contract tests
   (`test/packlint/spec_cartographer_formula_test.go` pattern), a compose-level test
   (`cmd/gc/embed_builtin_packs_test.go` pattern) proving the new agents compose and the
   seam-fragment precedence works, and the generic-ness grep gate.

Business reason: WS2/D10 — the harness's per-unit adversarial evaluation is the proven
diligence model; GasCity's single serialized refinery is today both the sole quality gate
and the merge bottleneck. These primitives split diligence (parallel per-task
evaluator/judge) from merging (refinery fan-in, slimmed by GCD-WO-CSC-005). Nothing runs
until a city binds providers/models/pools (GCD-WO-CSC-006) — everything here is inert
pack content with safe defaults.

## Dependencies

- **Blocked by `GasCity-Dev::GCD-WO-EVAL-001-generic-eval-execution-primitives`** (wave
  18 — merges before this WO dispatches; NOT yet merged at the authoring SHA `c85d92cf`.
  Verify its merged content at execution — Step 0(b) is the gate). This WO
  does NOT touch its pack (`execution-city-operations`); it imports its BINDING
  conventions: (a) zero business-domain literals in any pack runtime surface (grep gate
  restated in Validation); (b) Zero Framework Cognition — no new Go role logic, pack
  content only (the ONLY Go edits here are test files); (c) agents never grade
  themselves — here realized as maker/checker/approver separation: polecat generates,
  evaluator evaluates, judge approves the transition, refinery merges; no role ever
  self-passes. The `<domain>.<formula>.v1` naming convention it authored applies to
  CITY-side domain binding formulas in exec-mon cities — NOT to these upstream
  codegen-support formulas, whose names follow this pack's `mol-*` convention
  (`mol-debugger-plan` precedent) and are pinned by kit C9. State this distinction in the
  pack README so no city "helpfully" renames them.
- **Consumed by `GCD-WO-CSC-005` (same wave, direct-write edge):** imports the C9 keys,
  the `gc.kind=eval_request` routing selector value, and the effective-var lookup block
  (Required Inputs R4) verbatim. Its submit-to-evaluator polecat fragment is the
  PRODUCER of `eval_request` beads; this WO's evaluator is the consumer. The bead-field
  handshake is pinned in R3 — both WOs quote the same table.
- **Consumed by `GCD-WO-CSC-006`/`007` (waves 24/25):** their Step-0 discovery gates read
  THE MERGED agent/fragment/formula names and the pack README from this repo. Every name
  in this WO's "Pinned literals" table (R2) is load-bearing for them.
- **Cities PAUSED (standing policy + kit K1):** this WO verifies all GasCity-in-AWS
  remains paused (zero-replica / suspended) before declaring success — for this WO that
  means: authoring + repo-native structural tests ONLY (`make check`, `go test`, packlint,
  scratch-city compose tests via the repo's own test harnesses); **no city started, no
  daemon, no supervisor, no kubectl/AWS call, no live eval run**. Live behavior
  (evaluator actually claiming beads) is validated at the vehicle-graph pilot after
  un-pause under GCD-WO-CSC-006's named follow-up — never an acceptance criterion here.
- **Fixture-realism doctrine (owner-ratified, REJECT-level):**
  `master/DOCTRINE-fixture-realism-and-lifecycle-seam-acceptance.md` binds the test
  discipline: structural tests must FAIL when the asserted content is absent (planted-RED
  self-checks below); zero-item runs never green. The doctrine's TEXT stays out of this
  pack (MatchPoint-specific — it is city-fragment content per D10); its evidence-contract
  PATTERN (PASS requires reproduced real output; fabricated evidence is a defect class)
  is exactly what the evaluator prompt enforces generically.
- Repo gates: `CONTRIBUTING.md` (fork/branch workflow, `make setup`, `make build && make
  check`), `TESTING.md` (tier discipline, sharded targets), AGENTS.md (ZERO hardcoded
  roles — role names live in pack config only, never Go).

## Non-Goals

Bounded-context REJECT rules (kit K2, GasCity-Dev row) restated:

- **NO new Go role logic (ZFC).** The evaluator/judge are pure pack content. Go edits are
  LIMITED to test files (`test/packlint/*.go`, `cmd/gc/embed_builtin_packs_test.go`-class
  additions). No `internal/` changes, no CLI changes, no scheduler/routing Go — routing is
  existing `work_selector` metadata machinery.
- **NO embed.go change.** `codegen-support/embed.go` already embeds
  `pack.toml formulas orders all:agents all:assets template-fragments` — every new file
  in this WO lands under an existing glob. Adding/altering embed directives is a finding.
- **NO `.gc/system/packs` mirror files committed.** VERIFIED at authoring: mirrors are NOT
  tracked files — they are materialized at runtime from the embedded `PackFS` by
  `MaterializeBuiltinPacks` (`cmd/gc/embed_builtin_packs.go:26,49` — "materialized to
  .gc/system/packs/ on every gc start and gc init"; `gc config show` materializes too,
  `cmd/gc/cmd_config.go:30`). "Keeping mirrors in sync" = new files fall under the embed
  globs + the materialization test proves they appear. Hand-writing mirror files anywhere
  is a REJECT.
- **NO MatchPoint / business-domain literals** in `pack.toml`, `agents/`, `formulas/`,
  `template-fragments/`, README (grep gate: `matchpoint|enrichment|vehicle`, plus no
  `master/` spine paths, no D-numbers, no AWS resource names/ARNs/S3 URIs). Tests may
  carry such strings ONLY as negative-assertion registration data.
- **NO AWS-specific types or lanes.** The evidence durability seam is a generic
  city-supplied command pair (`evidence_publish_cmd` / `evidence_fetch_cmd` formula vars,
  default empty). Naming S3/buckets/regions upstream is a REJECT — the AWS lane is
  aws-GasCity's spec-18 contract (`AGC-WO-CSC-003`), adopted city-side.
- **NO polecat/refinery/gastown-pack edits** — the polecat diligence fragments, the
  submit-to-evaluator done-override, and the `mol-refinery-patrol` `evaluator_gated`
  branches are `GCD-WO-CSC-005` (same wave, AFTER this WO). This WO's README may DESCRIBE
  the pipeline but edits nothing outside `examples/gastown/packs/codegen-support/` + the
  named test files.
- **NO wo-router / cartographer / watch-order changes** (`GCD-WO-CSC-004`). No
  `spec-cartographer*` file is touched.
- **NO city bindings** — no city repo edits, no providers, no pools tuned per city, no
  `[[rigs.patches]]` anywhere (GCD-WO-CSC-006/007). Upstream defaults must leave every
  existing importing city behaviorally unchanged: with no `eval_request` beads and no
  city binding, both new agents are inert (`min_active_sessions = 0`, demand-driven).
- **NO grading/verdict logic in Go, no LLM-computed eligibility** — eligibility is typed
  selectors + explicit metadata (harness law "NO LLM computes eligibility" ported as: the
  routing state machine below is deterministic `gc bd update` writes).
- **NO pack.toml `[[named_session]]` entries** — one-shot pool agents are
  directory-discovered (GCD-WO-EVAL-001 precedent). `pack.toml` is untouched.
- **NO upstream (gastownhall/gascity) PR** — fork `main` only.
- **NO implementation of `regenerate_on_reject`** — the name is RESERVED in the README
  (D10); wiring regenerate semantics now is scope creep.

## Architecture Links

- `master/city-scaling-improvements/wo-authoring-kit.md` — K2, **C9** (this WO's output
  contract, quoted in R2/R3), A1 §2, K6 (estate authority; not in this repo — load-bearing
  pins are quoted in this file).
- `master/city-scaling-improvements/gap-analysis-and-build-plan.md` §5 rows 3/5/14, §6.
- `aws-GasCity/docs/investigations/2026-07-city-scaling-improvements.md` — WS2 "Findings —
  harness diligence inventory (portable)" + "Proposed design" items 1/2/4 (ratified D10)
  + "NOT portable" list (what must NOT be cloned upstream).
- `matchpoint-loop-harness/mlh/prompts/evaluator.md` + `stop_judge.md` (estate code root)
  — the diligence SOURCE prompts. Port the MECHANISMS (stance, ordered checks,
  deliverable-in-diff, anti-fabrication taxonomy, band-aid test, merge-tree
  conflict-only, run-to-completion, evidence contract); STRIP every MatchPoint residue
  (owner-decision sections, master/ doctrine references, prod/AWS mandates, test-fast
  program names, D1–D7 doctrine text, tenant/demo rules — those are city-fragment
  content, GCD-WO-CSC-006's job).
- This repo:
  - `examples/gastown/packs/codegen-support/agents/landing-arbiter/agent.toml` — THE
    structural precedent (rig-scope one-shot claude agent, typed
    `work_selector`/`scale_check_query` on `gc.kind` metadata, `[option_defaults]
    model = "opus"`).
  - `examples/gastown/packs/codegen-support/agents/landing-arbiter/prompt.template.md` —
    prompt-shape precedent (role header, propulsion/GUPP section, S1 typed claim with
    `gc work claim --status=in_progress --json` + no-matching-work drain, S2 claim
    verification).
  - `examples/gastown/packs/gastown/formulas/mol-refinery-patrol.toml` — wisp-formula
    precedent (root-only pour, steps read in order NOT materialized, pour-before-burn,
    `gc runtime drain-ack` discipline, `gc.routed_to` polecat rejection route recipe).
  - `examples/gastown/packs/codegen-support/formulas/mol-debugger-plan.formula.toml` —
    graph.v2 authoring oracle (`[vars]` blocks with description/required/default,
    `[[steps]]` id/title/needs, fail-fast identity step).
  - `examples/gastown/packs/codegen-support/template-fragments/polecat-handoff-override.template.md`
    — the refinery handoff recipe the judge-PASS route reuses (wake+nudge included) and
    the fragment file format (`{{ define "<name>" }} … {{ end }}`).
  - `docs/guides/shareable-packs.md:234-235` — "When multiple packs provide the same
    formula name, the importing pack wins over its imports." NOTE: this documented
    precedence is about FORMULA names; fragment-name precedence is UNCONFIRMED (WS2 risk
    K1) — Step 8's compose test resolves it for fragments; the README records the result.
  - `cmd/gc/cmd_lint.go:50-56` (`gc lint <pack>`), `:314-320` (appended fragment names
    are lint-validated via `tmpl.Lookup` — why the empty-default seam files must exist
    upstream).
  - `cmd/gc/embed_builtin_packs.go:26,49` + `cmd/gc/cmd_config.go:30` — mirror
    materialization mechanism (Non-Goals).
  - `internal/config/config.go:655-660` — `[rigs.formula_vars]` ("rig-scoped defaults for
    formula vars… loses to --var flags") + `engdocs/architecture/formulas.md:214-231`
    (precedence table; NOTE it applies to the SLING path — see R4 gotcha).

## Packages To Inspect

All paths repo-relative; READ-first, then extend:

- `examples/gastown/packs/codegen-support/` — the pack under extension: `pack.toml`
  (schema 2, untouched), `embed.go` (globs — verify, don't edit), all THREE existing
  agents (cartographer, debugger, landing-arbiter — structure), all 25 existing
  template-fragments (naming + define conventions), `formulas/` (the three existing
  `.formula.toml` files — note codegen-support formulas use the `.formula.toml` suffix;
  gastown's use bare `.toml`; FOLLOW THE codegen-support CONVENTION for the two new
  formulas).
- `examples/gastown/packs/gastown/formulas/mol-refinery-patrol.toml` — READ ONLY here
  (edited by GCD-WO-CSC-005): the rejection-route recipe
  (`--set-metadata gc.routed_to="${GC_RIG:+$GC_RIG/}{{binding_prefix}}polecat"`,
  `--status=open --assignee=""`), the `gc workflow delete-source/reopen-source` cleanup
  idiom, `gc mail send mayor/` escalation shape, wisp pour/burn discipline.
- `examples/gastown/packs/codegen-support/template-fragments/refinery-wisp-pour-vars-override.template.md`
  — READ: documents the load-bearing gotcha "`bd mol wisp` does not consume
  `[rigs.formula_vars]` at pour time" (R4 rationale).
- `test/packlint/` — `spec_cartographer_formula_test.go` (the string-contract test
  pattern: `repoRoot()` from `bd_show_jq_test.go:22`, read file, normalize whitespace,
  assert required literals), `spec_cartographer_watch_test.go`.
- `cmd/gc/embed_builtin_packs_test.go` — the compose-test pattern. PRIMARY precedent:
  `TestCodegenSupportBuiltinPackComposesWithGastown` (~line 182 — composes THIS pack
  with gastown at city + rig scope and asserts qualified names incl. the rig-scoped
  `app/codegen-support.cartographer` / `app/codegen-support.landing-arbiter` forms the
  new rig-scoped agents will mirror); secondary:
  `TestExecutionCityOperationsBuiltinPackComposesWithMaintenance` (~line 401).
- `cmd/gc/prompt.go:26-51` (`PromptContext` — what template data prompts may use; NOTE:
  no formula-vars accessor exists → R4), `cmd/gc/template_resolve.go:304-310` (fragment
  append order: per-agent, imported-pack defaults, city defaults).
- `internal/session/lifecycle.go` (`GC_AGENT` guarantee — cited by the fail-fast steps),
  `examples/gastown/packs/gastown/assets/scripts/worktree-setup.sh` (pre_start worktree
  provisioning the new agents reuse).
- `Makefile` — `build` (line ~27), `check`, `test` targets.

## Required Inputs

**R1 — verbatim structural anchor** (the agent.toml shape to mirror). From
`agents/landing-arbiter/agent.toml` @ `c85d92cf` (re-verify in worktree): `scope = "rig"`,
`provider = "claude"`, `lifecycle = "one_shot"`, `wake_mode = "fresh"`,
`idle_timeout = "1h"`, `work_dir` under `.gc/worktrees/{{.Rig}}/…`, `pre_start` calling
`{{.ConfigDir}}/../gastown/assets/scripts/worktree-setup.sh {{.RigRoot}} {{.WorkDir}}
{{.AgentBase}} --sync`, paired `[work_selector]`/`[scale_check_query]` with identical
predicates (status/type/unassigned/ready/sort + `[.metadata]` table), `[option_defaults]
model = "opus"`.

**R2 — PINNED LITERALS (C9; the downstream discovery targets — copy EXACTLY):**

| Literal | Value (identity — never rename) |
|---|---|
| Agent names | `evaluator`, `judge` (dirs `agents/evaluator/`, `agents/judge/`) |
| Agent scope | `rig` — **binding statement for cities: bind via `[[rigs.patches]]`, like polecat** (GCD-WO-CSC-006 R1a expects exactly this) |
| Routing selector values | `gc.kind` = `"eval_request"` (evaluator), `"judge_request"` (judge) |
| Default pools | evaluator `max_active_sessions = 4`, judge `max_active_sessions = 2`, both `min_active_sessions = 0` |
| Models | `provider = "claude"`; `[option_defaults] model = "opus"`, `effort = "high"` (both agents; cities re-point provider — and may restate model/effort/pools per C11) |
| Formula names | `mol-evaluate-task`, `mol-judge-task` (files `formulas/mol-evaluate-task.formula.toml`, `formulas/mol-judge-task.formula.toml`; `formula = "<name>"`, `version = 1`, `contract = "graph.v2"`) |
| Seam fragment names | `city-architecture-standards`, `city-evidence-doctrine`, `city-invariants` (files `template-fragments/<name>.template.md`) |
| Verdict keys | `eval_verdict`, `eval_evidence`, `verdict_patch_id`, `eval_reject_count`, `judge_verdict`, `rejection_reason`, `decision_state`, `overseer_issue_id`, `residue` |
| Verdict values | `eval_verdict` ∈ {`PASS`, `REJECT`}; `judge_verdict` ∈ {`PASS`, `REJECT`, `NOT_REQUIRED`}; `decision_state` escalation value = `mayor_action` |
| Content-state key grammar | `verdict_patch_id` = first field of `git diff "origin/$TARGET...origin/$BRANCH" \| git patch-id --stable`, computed at verdict time in the evaluator's worktree; written with EVERY `eval_verdict` write (the evaluator is the sole writer; the judge re-computes and matches — never rewrites) |
| Risk markers (judge triggers — deterministic, evaluated by the evaluator at PASS time, checklist + outcomes recorded as an evidence line) | (i) the diff contains zero source-code changes (docs/spec/config-only or empty — the no-diff/empty-leg/docs-band classes); (ii) `eval_reject_count > 0` (first PASS after any rejection); (iii) corrective-class bead: non-empty `overseer_issue_id` OR non-empty `metadata.target_branch` (repair bead). ANY marker ⇒ `gc.kind=judge_request`; NO marker ⇒ `judge_verdict=NOT_REQUIRED` + direct refinery handoff |
| Residue value shape | JSON array string: `[{"item": "<acceptance criterion / scope item>", "status": "delivered"\|"not-delivered"\|"known-gap", "mapped_to": "<existing bead or WO id — REQUIRED unless delivered>"}]`; writer = the polecat submit sequence (GCD-WO-CSC-005); silent residue = evaluator REJECT |
| Escalation var | `max_eval_rejects`, default `"2"` |
| Reserved var | `regenerate_on_reject` (documented, NOT implemented) |
| Evidence local grammar | `<city-root>/.gc/evidence/<rig>/<work-bead-id>/eval-attempt-<N>.jsonl`, N = `eval_reject_count`+1 at claim time |
| Evidence line shape | one JSON object per line: `{"command": "<exact command run>", "output_excerpt": "<real captured output, ≤2000 chars>"}` |
| Mayor mail subject | `[EVALUATOR ACTION REQUIRED] <bead-id>: max eval rejects reached [HIGH]` |

**R3 — routing state machine (deterministic `gc bd update` writes; producer↔consumer
handshake with GCD-WO-CSC-005; both WOs pin this table):**

| Transition | Writer | Writes |
|---|---|---|
| Submit for evaluation | polecat done-override (GCD-WO-CSC-005) | `branch`, `target`, notes, `residue` (structured rows — R2 shape; written fresh on EVERY submission); CLEARS `rejection_reason`, `eval_verdict`, `judge_verdict`, `verdict_patch_id`; sets `gc.kind=eval_request`, `gc.routed_to=` (clear), `--status=open --assignee=""`. KEEPS `eval_reject_count`, `overseer_issue_id` |
| Evaluator PASS — risk marker present (R2 risk-marker table) | evaluator | `eval_verdict=PASS`, `eval_evidence=<path/URI>`, `verdict_patch_id=<patch-id>`, `gc.kind=judge_request`, `gc.routed_to=` (clear), `--status=open --assignee=""` |
| Evaluator PASS — NO risk marker (conditional judge, ROL-6) | evaluator | `eval_verdict=PASS`, `eval_evidence=<path/URI>`, `verdict_patch_id=<patch-id>`, `judge_verdict=NOT_REQUIRED`, `gc.kind=` (clear), `--status=open --assignee="$REFINERY_TARGET"`, `gc.routed_to="$REFINERY_TARGET"` + wake/nudge (handoff-override recipe — the same block the judge-PASS route uses); marker checklist + outcomes appended to the evidence JSONL |
| Evaluator REJECT (budget left) | evaluator | `eval_verdict=REJECT`, `rejection_reason=<failure + required fix>`, `eval_reject_count=<n+1>`, `eval_evidence=<path/URI>`, `verdict_patch_id=<patch-id>`, `gc.kind=` (clear), `gc.routed_to="${GC_RIG:+$GC_RIG/}{{binding_prefix}}polecat"`, `--status=open --assignee=""` |
| Judge PASS | judge | `judge_verdict=PASS`, `gc.kind=` (clear), `--status=open --assignee="$REFINERY_TARGET"`, `gc.routed_to="$REFINERY_TARGET"` + wake/nudge (handoff-override recipe); `branch`/`target`/`verdict_patch_id` NOT rewritten — the judge PASS is valid ONLY at the patch-id it re-computed and matched (step 2 of `mol-judge-task`) |
| Judge stale-content re-route (re-computed patch-id ≠ `verdict_patch_id`) | judge | `eval_verdict=` (clear), `judge_verdict=` (clear), `verdict_patch_id=` (clear), `rejection_reason="stale verdict: branch content changed since evaluation"`, `gc.kind=eval_request`, `gc.routed_to=` (clear), `--status=open --assignee=""`. **`eval_reject_count` NOT incremented** — staleness is a race, not a content failure (blueprint ACC-1/ROL-5); the work re-enters evaluation, never construction |
| Judge infra re-route (missing/empty/unreproducible evidence, missing `verdict_patch_id` — an unsubstantiated PASS) | judge | same writes as the stale-content row with `rejection_reason="unsubstantiated evaluator PASS: <what was missing>"` — treated as an INFRASTRUCTURE failure of the evaluation lane (LAW-7): never accepted, never charged to the shared budget, never routed to the polecat (the maker did nothing wrong) |
| Judge REJECT (budget left) | judge | `judge_verdict=REJECT`, `rejection_reason=<one line>`, `eval_reject_count=<n+1>`, `gc.kind=` (clear), route to polecat pool as above |
| Escalation (either agent, `n+1 ≥ max_eval_rejects`) | rejecting agent | verdict + reason + count as above, PLUS `decision_state=mayor_action`, `gc.kind=` (clear), `gc.routed_to=` (clear), `--status=open --assignee=""`, then mayor mail (R2 subject) |
| Mayor re-arm (documented recipe, no automation) | mayor/human | clear `decision_state`, optionally reset `eval_reject_count`, re-set `gc.kind=eval_request` (re-evaluate) or `gc.routed_to=<pool>polecat` (re-fix) |

`eval_reject_count` is a SINGLE shared budget: evaluator and judge CONTENT rejections
both increment it; the stale-content and infra re-route rows never do (attempt-neutral,
blueprint ACC-2). `overseer_issue_id` is written only by external correlation machinery
(estate-side; A1 §2) and NEVER cleared by any agent in this table. `residue` is written
by the polecat submit sequence (GCD-WO-CSC-005) and never cleared by evaluator/judge —
it is the close-out record downstream planning consumes. **Content-state law (LAW-4):**
every evaluator verdict write carries the `verdict_patch_id` it proved (the evaluator is
the sole writer; the judge re-computes and matches — never rewrites); verdicts survive
re-wakes, crashes, and agent swaps — only a changed patch-id invalidates them.

**R4 — effective-var lookup (load-bearing gotcha + the canonical block).** VERIFIED facts:
(a) `bd mol wisp` does NOT consume `[rigs.formula_vars]` at pour time (in-repo authority:
`refinery-wisp-pour-vars-override.template.md`; the documented precedence table in
`engdocs/architecture/formulas.md:214-231` applies to the SLING path via
`BuildSlingFormulaVars`, not to `bd mol wisp`); (b) `PromptContext` (`cmd/gc/prompt.go:26-51`)
has NO formula-vars accessor, so prompts cannot render them either. Therefore city-set
rig vars (battery commands, `max_eval_rejects`, evidence seams) must be resolved AT RUN
TIME inside formula steps. Canonical block (embed verbatim at the top of every consuming
step; GCD-WO-CSC-005 copies the SAME block into `mol-refinery-patrol`):

```bash
# City-root + effective rig-var resolution. `bd mol wisp` does not consume
# [rigs.formula_vars] at pour time, so read them from the composed config here.
# Precedence: explicit wisp --var (non-default) > [rigs.formula_vars] > formula default.
CITY_ROOT=${CITY_ROOT:-$(d="$PWD"; while [ "$d" != "/" ] && [ ! -f "$d/city.toml" ]; do d=$(dirname "$d"); done; printf '%s' "$d")}
[ -f "$CITY_ROOT/city.toml" ] || { echo "cannot resolve city root from $PWD"; gc runtime drain-ack; exit 1; }
effective_rig_var() { # $1=var name  $2=wisp-rendered value  $3=formula default
  name="$1"; wisp="$2"; def="$3"
  if [ -n "$wisp" ] && [ "$wisp" != "$def" ]; then printf '%s' "$wisp"; return; fi
  cfg=$(cd "$CITY_ROOT" && gc config show --json 2>/dev/null \
    | jq -r --arg rig "${GC_RIG:-}" --arg key "$name" \
        '<RIG-FORMULA-VARS-PATH>' 2>/dev/null || true)
  if [ -n "$cfg" ] && [ "$cfg" != "null" ]; then printf '%s' "$cfg"; else printf '%s' "$def"; fi
}
```

`<RIG-FORMULA-VARS-PATH>` is the jq path selecting the named rig's `formula_vars[$key]`
from the REAL `gc config show --json` output. **EXECUTION-TIME VERIFICATION REQUIRED
(STOP-gate):** build `gc`, `gc init` a scratch city with one rig carrying
`[rigs.formula_vars] test_command = "sentinel-cmd"`, run `gc config show --json`, inspect
the ACTUAL key spelling/nesting (candidates: `.config.rigs[]?`, `.rigs[]?`, key
`formula_vars` vs `FormulaVars`), pin the working path into BOTH formulas, and record the
transcript in evidence. If `gc config show --json` does not expose rig formula_vars at
all, STOP — raise a structured blocker (the fallback would be a Go change, out of ZFC
scope) rather than inventing a different channel.

**R5 — harness diligence inventory to port** (from `mlh/prompts/evaluator.md` +
`stop_judge.md`; the WS2 "portable" list): assume-broken default-doubt stance; different
model on purpose; "judge BEHAVIOR, not intent — ACT, do not merely read"; ordered checks
stop-at-first-substantiated-failure; deliverable-exists anti-empty-work ("evidence
DESCRIBES, the diff must DELIVER"; ≈0-source-lines diff toward the named deliverable =
REJECT); reproduce declared outputs; anti-fabrication hunt (trace every evidence value to
its source; re-run and compare; the fabrication taxonomy of R6); allow
additive/restorative repairs, REJECT band-aids (anything that disables, loosens, narrows,
skips, special-cases, or masks a gate/test/check to force green — additive gate changes
need a repo-general ADR); merge-readiness = genuine CONFLICT only via
`git merge-tree --write-tree` (never fail merely-behind branches; verification only —
never resolve, that is the maker's job); run-to-completion (never end the session without
a verdict; never emit a placeholder verdict while a check still runs; wait for real exit
codes); stop-judge = maker-checker on the transition only (spot-reproduce ≥1 evidence
command, deliverable-in-diff, conflict-only re-check, one-line verdicts, never resolves,
does not over-claim). **NOT portable (strip):** owner-decision machinery, master/
spine/doctrine references, prod/AWS/CDK rules, canonical status taxonomy, test-fast
program names, multi-repo co-edit machinery, tenant/demo rules.

**R6 — fabricated-evidence taxonomy (generic; verbatim list both prompts carry):**
hard-coded or faked PASS values / proof output; empty proof files, or proof files that
CONTRADICT what the real command produces when re-run; self-validating CIRCULAR
manifests (a step writes the very "PASS" a later step copies and "validates"); string
obfuscation, renaming, or encoding used to dodge a grep/detector; evidence values
recalled or pattern-assembled instead of captured from the executed command.

## Implementation Steps

**Step 0 — verification gate (record all outputs in evidence).** (a) `git log -1
origin/main` — confirm base ≥ `c85d92cf`; (b) confirm GCD-WO-EVAL-001 content merged
(`ls examples/gastown/packs/execution-city-operations/formulas/` non-empty); (c) confirm
R1 anchor unchanged (`agents/landing-arbiter/agent.toml`); (d) confirm embed globs cover
the new paths (`cat examples/gastown/packs/codegen-support/embed.go`); (e) run the R4
scratch-city verification and pin `<RIG-FORMULA-VARS-PATH>`; (f) confirm no file this WO
creates already exists (else STOP: overlap with a sibling WO).

**Step 1 — `agents/evaluator/agent.toml`** (verbatim; comments required — they carry the
binding rationale):

```toml
# Rig-scoped one-shot evaluator pool (CSC / GCD-WO-CSC-003).
#
# The canonical work item is an open, unassigned work bead carrying
# metadata gc.kind=eval_request — written by the polecat done-sequence
# override (submit-to-evaluator) when a city enables the evaluated flow.
# Cities bind this agent PER RIG via [[rigs.patches]] (like polecat):
# provider re-point, pool size, and append_fragments for the three city
# doctrine seams. With no eval_request beads and no binding, this agent
# is inert (min_active_sessions = 0, demand-driven).
#
# The evaluator is a DIFFERENT model than the generator on purpose.
# It never edits code, never rebases, never merges. Verdicts are bead
# metadata writes; evidence is a JSONL artifact (see the pack README,
# "Verdict metadata contract").
scope = "rig"
provider = "claude"
lifecycle = "one_shot"
wake_mode = "fresh"
idle_timeout = "1h"
nudge = "Check for eval_request work beads and evaluate them."

# Dedicated worktree per evaluator session: the evaluation checks out the
# polecat's PUSHED branch (what will actually merge), detached.
work_dir = ".gc/worktrees/{{.Rig}}/evaluators/{{.AgentBase}}"
pre_start = [
  "{{.ConfigDir}}/../gastown/assets/scripts/worktree-setup.sh {{.RigRoot}} {{.WorkDir}} {{.AgentBase}} --sync",
]

# Parallel per-task evaluation; the serialization point stays the refinery.
min_active_sessions = 0
max_active_sessions = 4

[work_selector]
status = "open"
unassigned = true
ready = true
sort = "created_asc"

[work_selector.metadata]
"gc.kind" = "eval_request"

[scale_check_query]
status = "open"
unassigned = true
ready = true
sort = "created_asc"

[scale_check_query.metadata]
"gc.kind" = "eval_request"

# Pin the model. Permission_mode omitted (provider default) — the formula
# runs gc bd update / gc mail send, blocked under permission_mode = "plan".
[option_defaults]
model = "opus"
effort = "high"
```

**Step 2 — `agents/evaluator/prompt.template.md`.** Sections, in order (follow the
landing-arbiter prompt's voice and shape; every command block below is load-bearing):

1. **Header + recovery line** (`> **Recovery**: Run {{ cmd }} prime after compaction…`).
2. **Role** — "ADVERSARIAL EVALUATOR ({{ .RigName }})": you evaluate ONE submitted work
   bead's pushed branch; ASSUME THE WORK IS BROKEN until proven otherwise; default
   stance is DOUBT; you carry none of the generator's reasoning and you are a different
   model on purpose; do not praise; judge BEHAVIOR, not intent — you ACT, you do not
   merely read the diff. You never edit code, never rebase or resolve, never merge or
   push, never weaken any gate. Allow genuinely additive/restorative improvements
   (with a repo-general ADR when a gate changed); hold the line HARD against band-aids
   (R5 wording).
3. **Propulsion/GUPP block** (pack convention): work on the hook is the assignment;
   claim → evaluate → verdict → drain; no idling.
4. **Startup S0 — stranded-claim recovery** (NDI): `STRANDED=$(gc bd list
   --assignee="$GC_AGENT" --status=in_progress --exclude-type=epic --limit=1 --json |
   jq -r '.[0].id // empty')` — if non-empty, adopt it as `$WORK` (skip S1) and re-enter
   the formula from context.
5. **Startup S1 — typed claim** (landing-arbiter S1 verbatim shape): `gc --rig "$GC_RIG"
   work claim --status=in_progress --json` with the `no matching work` → `gc runtime
   drain-ack; exit 0` branch and the error branch.
6. **Startup S2 — verify the claim** (assignee = `$GC_AGENT`, status = `in_progress`);
   read `metadata.branch` / `metadata.target` — see the formula's fail-fast step for the
   missing-branch path.
6b. **S2b — capability self-test (acting evaluator, ROL-5):** before any check, run one
   trivial real command in your worktree (`git rev-parse HEAD`) and capture its output —
   this becomes the FIRST line of the evidence JSONL. If you cannot execute commands,
   you have NO verdict authority: mail the mayor the infrastructure failure, `gc runtime
   drain-ack`, exit — write NO verdict (a verdict produced without proven command
   execution is an infrastructure failure, never a judgment).
7. **S3 — pour the formula wisp and work its steps in order** (steps are NOT
   materialized as child beads — refinery-patrol convention):

```bash
WISP=$(gc bd mol wisp mol-evaluate-task --root-only \
  --var work_bead="$WORK" \
  --var rig_name={{ .RigName }} \
  --var binding_prefix={{ .BindingPrefix }} \
  --var target_branch={{ .DefaultBranch }} \
  --json | jq -r '.new_epic_id // empty')
if [ -z "$WISP" ]; then echo "could not pour mol-evaluate-task"; gc runtime drain-ack; exit 1; fi
gc bd update "$WISP" --assignee="$GC_AGENT"
```

   Plus the pour-time gotcha note: battery/evidence/budget vars deliberately NOT passed
   here — the formula resolves them from `[rigs.formula_vars]` at run time (R4).
8. **Verdict contract summary** — the R2 keys + "a PASS without the evidence artifact is
   invalid; the judge will reject it" + "every verdict you write carries
   `verdict_patch_id` — the patch-id of the diff you evaluated; verdicts are keyed to
   content, not to your session" + the risk-marker rule (on PASS, evaluate the R2
   risk-marker checklist deterministically and record it in evidence: any marker →
   route `judge_request`; no marker → `judge_verdict=NOT_REQUIRED` + direct refinery
   handoff — the judge lane exists for risk, not for ceremony) + run-to-completion
   paragraph (never end the session without writing a verdict to the bead; a
   long-running check still executing is not a finding — wait for its real exit code;
   if a tool genuinely cannot complete, write REJECT with the infrastructure reason,
   never a placeholder).
9. **City doctrine seams** — one short paragraph: "City-specific architecture standards,
   evidence doctrine, and invariants are appended to this prompt by the city binding
   (fragments `city-architecture-standards`, `city-evidence-doctrine`,
   `city-invariants`). Enforce everything they state as REJECT-level acceptance
   criteria." (Content arrives via `append_fragments` — see Step 5.)

**Step 3 — `agents/judge/agent.toml`**: identical structure to Step 1 with:
`nudge = "Check for judge_request work beads and judge the transition."`,
`work_dir = ".gc/worktrees/{{.Rig}}/judges/{{.AgentBase}}"`, `max_active_sessions = 2`,
both metadata selectors `"gc.kind" = "judge_request"`, and the header comment describing
the maker-checker role ("the person who enters a transfer and the person who approves it
must differ — you are the approver").

**Step 4 — `agents/judge/prompt.template.md`.** Same skeleton as Step 2 (recovery, role,
GUPP, S0/S1/S2, pour `mol-judge-task` with the same four vars, verdict summary, city
seams paragraph). Role text (stop_judge port, generic): you did not write the code and
you did not run the adversarial evaluation; your ONLY job is deciding whether the
proposed transition (evaluator PASS → refinery merge queue) is genuinely earned from the
evidence on disk; you only ever receive RISK-MARKER transitions (the C9 conditional-judge
rule — no-marker PASSes are handed to the refinery without you; do not re-verify
properties the deterministic gates own); the PASS you approve is keyed to content — you
re-compute the patch-id and approve ONLY at a match (a mismatch is a stale verdict, not
a defect: re-route to evaluation, budget-neutral); spot-reproduce at least one executed
command from the evidence
artifact; confirm the deliverable exists in the branch diff; re-run the conflict-only
merge check; you never resolve, fix, rebase, or edit anything; you do not over-claim —
PASS approves exactly one bead's handoff to the refinery, nothing more.

**REQUIRED section — "Code review via the code-review skill" (Mayor ruling D18, owner
directive 18, 2026-07-09; folded pre-pickup):** when the transition under judgment
includes CODE, the judge REVIEWS IT USING THE CODE-REVIEW SKILL as its procedure: review
the diff against the repo's pattern docs, §6 usage contracts, and §4 failure / §5
structured-logging standards via the skill's checklist; resolve WHICH patterns apply
through the repo's own `specs/patterns/` consumer stubs; review at the CONSUMER'S
`bound_version` (stub front matter), NEVER at hosting HEAD — sanctioned version lag is
legal and is not a finding (the design-audit rule); and use the standardization ethos
the skill links for the judgment calls it marks non-mechanical. The prompt section NAMES
the skill and states its purpose (owner directive 19.2: a skill unnamed in its
consumer's prompt does not exist operationally — this judge binding is that rule's first
instance). **Generic-ness constraint (D10 placement principle; the pack's REJECT-level
generic-ness gate stands):** the UPSTREAM prompt text refers to "the city-bound
code-review skill and pattern-law surfaces named by the city seam fragments" —
generically; the CONCRETE estate identity (the PS-WO-006 artifact
`SKILL-pattern-code-review`, Platform-hosted, per-repo pointer-stub linked) is injected
by the city SOURCE repos' fragment content via the GCD-WO-CSC-006/007 binding lane,
never hardcoded in `codegen-support`.

**Validation-criteria addition — END-TO-END PROCESS COHESION (Mayor ruling D21, owner
directive 19, 2026-07-09; folded pre-pickup):** the judge's transition checks include
end-to-end process cohesion, judged AWARE of how the estate uses `specs/`, skills, and
GasCity as a whole: the diff updates the docs its change invalidates (specs/ADRs ride
the diff, before acceptance); where the change adds or reshapes a skill, its
skill-to-prompt bindings are present or explicitly punch-listed; the indexes the change
touches (pattern-usage shard, README/overall index) are current. **A change that ships
working code while ORPHANING its docs, bindings, or indexes is a REJECT, not a pass.**
(Upstream text states the criterion generically — "the doc/binding/index surfaces the
city's law fragments name"; concrete estate surfaces arrive via the same city seam
fragments.)

**Step 5 — the three empty-default seam fragments.** Three files under
`template-fragments/`, each EXACTLY this shape (name substituted):

```
{{ define "city-architecture-standards" }}{{/*
Empty upstream default (CSC city injection seam — GCD-WO-CSC-003).
City packs deliver content by defining this SAME name in their own
template-fragments/ and appending it via append_fragments on the
evaluator/judge rig patches. Upstream stays business-domain-agnostic:
never put concrete standards here. See the pack README, "City binding".
*/}}{{ end }}
```

Same for `city-evidence-doctrine` and `city-invariants`. The `{{/* */}}` template-comment
form is REQUIRED (a markdown comment inside the define would render into prompts).

**Step 6 — `formulas/mol-evaluate-task.formula.toml`.** Header: description (one
evaluation = one wisp; poured root-only by the evaluator prompt; steps read in order, not
materialized; on crash re-read steps and recover from bead/git state),
`formula = "mol-evaluate-task"`, `version = 1`, `contract = "graph.v2"`.

`[vars]` (every one with a description): `work_bead` (required), `rig_name` (required),
`binding_prefix` (default `"gastown."`), `target_branch` (default `"main"`),
`max_eval_rejects` (default `"2"`), `setup_command`/`typecheck_command`/`lint_command`/
`build_command`/`test_command` (default `""` — empty = skip; SAME names as
`mol-refinery-patrol` so one `[rigs.formula_vars]` entry feeds both formulas),
`evidence_publish_cmd` (default `""` — "city-supplied command; invoked as
`$evidence_publish_cmd <local-file>`; MUST print exactly one line on stdout: the durable
URI recorded as `eval_evidence`. Empty = record the city-root-relative local path; only
correct where all agents share one filesystem — see README"), and NO
`regenerate_on_reject` (reserved name — description of the reservation lives in the
README, not as a dead var).

Steps (`[[steps]]`, chained `needs`):

1. `validate-identity` — fail-fast `$GC_AGENT` (refinery precedent verbatim, mail mayor +
   drain on empty); recover `WORK={{work_bead}}` (fall back to reading the wisp bead's
   vars via `$GC_BEAD_ID` like mol-debugger-plan's validate-identity); read the work
   bead: `BRANCH`/`TARGET` from metadata (`.metadata.branch`,
   `.metadata.target_branch // .metadata.target // "{{target_branch}}"`),
   `EVAL_REJECT_COUNT=$(… '.[0].metadata.eval_reject_count // "0"')`. **Missing
   `metadata.branch` = malformed submission → REJECT path immediately** (write
   `eval_verdict=REJECT`, `rejection_reason="submitted without metadata.branch"`,
   increment count, route per R3, burn wisp, drain) — never evaluate air.
2. `setup-worktree` — in the evaluator's own worktree: `git fetch --prune origin`, then
   `git checkout --detach "origin/$BRANCH"` (evaluate the PUSHED branch — what will
   merge; if the ref does not exist, that IS the finding: REJECT "declared branch not on
   origin"). Then the **capability self-test** (ROL-5): `git rev-parse HEAD` — its
   `{command, output_excerpt}` pair is the FIRST evidence line; a failure here is an
   infrastructure failure (mayor mail + drain, NO verdict written, no budget consumed).
   Then compute the content-state key:
   `PATCH_ID=$(git diff "origin/$TARGET...origin/$BRANCH" | git patch-id --stable | awk '{print $1}')`
   — recorded in evidence and written as `verdict_patch_id` with the verdict (empty
   PATCH_ID on a non-empty diff = infrastructure failure, same posture).
3. `ordered-checks` — embed the R4 effective-var block, resolve the five battery vars +
   `MAX_EVAL_REJECTS`; then the ordered checks, STOPPING at the first substantiated
   failure (each check's real command + output appended to the evidence file AS IT RUNS
   — step 4's file is created here first):
   1. **Deliverable-in-diff:** `git diff --stat "origin/$TARGET...origin/$BRANCH"` —
      substantive source changes toward the deliverable the bead body names; a diff that
      is only notes/spec/config toward it = REJECT ("evidence DESCRIBES, the diff must
      DELIVER").
   2. **Declared commands:** run the acceptance/validation commands the bead body and its
      referenced work-order file declare, in this worktree, reading real output. A
      declared failure signal that fires = REJECT.
   3. **Configured battery:** run each non-empty command of
      setup/typecheck/lint/build/test in order; any failure = REJECT (diagnose
      branch-caused vs pre-existing on `origin/$TARGET`; pre-existing failures are noted
      in evidence, not charged to this bead — mirror the refinery's diagnose idiom).
   4. **Anti-fabrication hunt:** verify declared evidence artifacts exist at their
      declared paths and contain REAL captured output — re-run and compare; REJECT on
      sight for anything in the R6 taxonomy (quote the taxonomy verbatim in the step).
   5. **Band-aid detection:** R5 rule verbatim (additive/restorative allowed — gate
      changes need a repo-general ADR in the diff; weakening/disabling/narrowing/
      special-casing to force green = REJECT).
   6. **City invariants:** enforce every REJECT-level rule the appended city doctrine
      fragments state (inert when a city appended none).
   7. **Residue declaration (GEN-6):** the submission carries the C9 `residue` rows
      (bead metadata `residue`, R2 value shape): every acceptance criterion / scope
      item of the bead's named work maps to a `delivered` row or an explicit
      `not-delivered`/`known-gap` row whose `mapped_to` names an EXISTING bead or WO.
      Missing/empty declaration, an unmapped gap, or a claimed-delivered item the diff
      demonstrably does not deliver = REJECT ("silent residue"). The rows themselves
      are the polecat's (GCD-WO-CSC-005 submit sequence) — this check verifies
      presence and honesty, never authors them.
   8. **Merge-readiness (conflict-only):** `git merge-tree --write-tree
      "origin/$TARGET" "origin/$BRANCH"` — conflict markers reported = failed check;
      a merely-BEHIND but conflict-free branch is merge-ready and MUST NOT be failed for
      staleness; verification only, never resolve (maker's job).
4. `write-evidence` — create
   `EVID_DIR="$CITY_ROOT/.gc/evidence/${GC_RIG:-city}/$WORK"`;
   `ATTEMPT=$((EVAL_REJECT_COUNT + 1))`;
   `EVID_FILE="$EVID_DIR/eval-attempt-$ATTEMPT.jsonl"` (this step formalizes the file
   step 3 has been appending to — the formula text must order file-creation BEFORE the
   first check so evidence is written at execution time, never reconstructed). Then the
   publish seam: `EVIDENCE_PUBLISH_CMD=$(effective_rig_var evidence_publish_cmd
   "{{evidence_publish_cmd}}" "")`; if non-empty run it, capture the single-line stdout
   as `EVID_REF` (publish failure = do NOT pass — treat as an infrastructure REJECT with
   the reason recorded); else `EVID_REF="${EVID_FILE#$CITY_ROOT/}"`.
5. `verdict-and-route` — exactly the R3 writes, as FOUR explicit command blocks
   (PASS-with-risk-marker → `judge_request` / PASS-no-marker → `judge_verdict=
   NOT_REQUIRED` + direct refinery handoff / REJECT-with-budget / ESCALATE), each a
   single chained `gc bd update … && …` so the metadata write cannot be skipped
   (refinery merge-push chaining idiom). Every verdict write includes
   `verdict_patch_id="$PATCH_ID"` (computed in `setup-worktree`). Before the PASS
   fork, the step runs the **deterministic risk-marker checklist** (R2 table: (i)
   zero-source-line/docs-only diff via `git diff --numstat` classification; (ii)
   `EVAL_REJECT_COUNT > 0`; (iii) non-empty `overseer_issue_id` or
   `metadata.target_branch`) and appends the checklist + outcomes as an evidence line
   — the fork between the two PASS blocks is those three tests, never model whim. The
   PASS-no-marker block uses the SAME refinery handoff recipe as `mol-judge-task`
   (assignee + `gc.routed_to` + wake/nudge). The escalate
   block ends with `gc mail send mayor/ -s "[EVALUATOR ACTION REQUIRED] $WORK: max eval
   rejects reached [HIGH]" -m "<bead, branch, attempt count, last rejection_reason,
   evidence ref>"`. PASS (either block) is FORBIDDEN unless `$EVID_FILE` exists and is
   non-empty (`[ -s "$EVID_FILE" ]` guard inline — a PASS without evidence is invalid
   by construction, not just by judge review) AND `$PATCH_ID` is non-empty.
6. `finalize` — restore the worktree to a neutral detached state
   (`git checkout --detach "origin/$TARGET" || true`), burn the wisp
   (`gc bd mol burn` pour-less variant — one-shot, no next iteration), `gc runtime
   drain-ack`, exit 0.

**Step 7 — `formulas/mol-judge-task.formula.toml`.** Same header conventions,
`formula = "mol-judge-task"`. `[vars]`: `work_bead` (required), `rig_name` (required),
`binding_prefix` (default `"gastown."`), `target_branch` (default `"main"`),
`max_eval_rejects` (default `"2"`), `evidence_fetch_cmd` (default `""` — "city-supplied
command; invoked as `$evidence_fetch_cmd <uri> <dest-file>`; materializes a URI-form
`eval_evidence` artifact for verification. Empty + URI-form evidence that is not directly
readable = the PASS is unverifiable → JUDGE REJECT (fail closed)").

Steps:

1. `validate-identity` — as Step 6.1 (work bead must carry `eval_verdict=PASS` AND
   non-empty `eval_evidence` AND non-empty `verdict_patch_id` AND `metadata.branch`;
   anything else is an UNSUBSTANTIATED PASS = the R3 **judge infra re-route** row:
   clear the verdict keys, `rejection_reason="unsubstantiated evaluator PASS: <what
   was missing>"`, `gc.kind=eval_request`, NO `eval_reject_count` increment — an
   infrastructure failure of the evaluation lane is never accepted and never charged
   to the maker).
2. `verify-evidence` — resolve the artifact: if `eval_evidence` contains `://`, fetch via
   the effective `evidence_fetch_cmd` (R4 block; fail closed per the var description);
   else read `$CITY_ROOT/<eval_evidence>`. Assert: non-empty; every line parses as JSON
   with BOTH `command` and `output_excerpt` keys (`jq -e 'has("command") and
   has("output_excerpt")'` per line). Fetch + detach onto `origin/$BRANCH` (as Step
   6.2), then **re-compute the content-state key** —
   `PATCH_ID=$(git diff "origin/$TARGET...origin/$BRANCH" | git patch-id --stable | awk '{print $1}')`
   — and compare to `metadata.verdict_patch_id`: **mismatch = the R3 stale-content
   re-route row** (the branch changed since evaluation — clear the verdicts, route
   `gc.kind=eval_request`, NO budget increment; staleness is a race, never a content
   REJECT). On match, **spot-reproduce at least one executed
   command** from the artifact in the judge's own worktree and confirm the real output
   is consistent with the recorded excerpt. Contradiction or unreadable/malformed
   artifact = REJECT (a fabricated-evidence content failure — this one DOES consume
   budget).
3. `verify-deliverable` — `git diff --stat "origin/$TARGET...origin/$BRANCH"` non-trivial
   toward the bead's named deliverable (anti-empty-work re-check — the judge repeats it
   because the transition, not the evaluation, is being approved).
4. `verify-merge-readiness` — `git merge-tree --write-tree "origin/$TARGET"
   "origin/$BRANCH"`; conflict = transition not earned (REJECT, one line); behind-only =
   fine (verbatim conflict-only rule from R5).
5. `verdict-and-route` — R3 writes: judge-PASS block (valid only at the matched
   patch-id from step 2; verbatim, including the wake/nudge
   lines — quote the `polecat-handoff-override` recipe shape):

```bash
REFINERY_TARGET="${GC_RIG:+$GC_RIG/}{{binding_prefix}}refinery"
gc bd update "$WORK" \
  --set-metadata judge_verdict=PASS \
  --set-metadata gc.kind= \
  --status=open \
  --assignee="$REFINERY_TARGET" \
  --set-metadata gc.routed_to="$REFINERY_TARGET"
gc session wake "$REFINERY_TARGET" || true
gc session nudge "$REFINERY_TARGET" "Run 'gc prime' to check merge queue and begin processing." || true
```

   judge-REJECT and ESCALATE blocks mirror the evaluator's (increment the SHARED
   `eval_reject_count`; same escalation rule and mail subject).
6. `finalize` — as Step 6.6.

**Step 8 — pack README (`examples/gastown/packs/codegen-support/README.md`, NEW).** The
C9 binding doc (GCD-WO-CSC-006 R1b reads it). Required content, all generic:
(1) pack overview incl. the evaluated code-gen pipeline diagram
(polecat → evaluator → judge → refinery merge fan-in; refinery gating var is the gastown
`mol-refinery-patrol` `evaluator_gated` var — cross-reference only, authored by the
polecat/refinery WO); (2) the R2 pinned-literals table; (3) the R3 routing/ownership
table verbatim; (4) evidence grammar + publish/fetch seam + the fail-closed rule + the
single-filesystem caveat ("the local default is only correct where evaluator and judge
share one filesystem; distributed cities must supply `evidence_publish_cmd` /
`evidence_fetch_cmd` returning/consuming durable URIs"); (5) escalation + the mayor
re-arm recipe (R3 last row); (6) retry semantics: resume-and-fix (rejected beads return
with `rejection_reason` and an intact branch; pool agents resume, not regenerate) +
"**`regenerate_on_reject` is a RESERVED formula-var name** for a future
regenerate-from-zero mode; it is not implemented; do not repurpose the name"; (7) the
city-binding recipe: **"evaluator and judge are RIG-scope agents — bind per rig via
`[[rigs.patches]]` (exactly like polecat)"**, provider re-point + `option_defaults` +
pool sizing + `append_fragments` with the three seam names + `[rigs.formula_vars]` for
battery/budget/evidence vars; (8) **"No per-profile `CLAUDE_CONFIG_DIR` is required or
expected — the runtime's default handling suffices"** (GCD-WO-CSC-006 Step-1 gates on
this statement); (9) the seam-fragment mechanism note recording Step 9c's compose-test
outcome (which define wins the name collision on append); (10) a note that these formula
names are upstream identities pinned by the consuming program — the `<domain>.<name>.v1`
convention applies to city-side DOMAIN binding formulas (execution-city-operations
README), not here; (11) **the content-state law**: every verdict carries
`verdict_patch_id` (grammar in the R2 table); verdicts attach to content-states, never
sessions — re-wakes/crashes/agent swaps never invalidate one, only content change does;
downstream degrades (the gastown `evaluator_gated` flow) are valid ONLY at a matching
patch-id; (12) **the conditional-judge rule + risk-marker table** (R2): the judge runs
only on risk-marker PASSes; `judge_verdict=NOT_REQUIRED` semantics and the evaluator's
recorded marker checklist; (13) **the residue contract** (R2 `residue` shape): writer =
the polecat submit sequence, silent residue = evaluator REJECT, rows are never cleared
by evaluator/judge and are consumed as premises by downstream planning (the wo-router
lane, GCD-WO-CSC-004).

**Step 9 — tests.**

- **(a) `test/packlint/csc_evaluator_judge_agents_test.go`** (pattern:
  `spec_cartographer_formula_test.go`; `repoRoot()` helper already exists in the
  package): for each agent.toml assert the R2 literals (`scope = "rig"`,
  `provider = "claude"`, `lifecycle = "one_shot"`, `wake_mode = "fresh"`, the
  `"gc.kind" = "eval_request"`/`"judge_request"` selector lines in BOTH `work_selector`
  and `scale_check_query`, `max_active_sessions = 4`/`2`, `min_active_sessions = 0`,
  `model = "opus"`, `effort = "high"`); for each prompt assert the load-bearing stance
  strings (normalized-whitespace contains): "ASSUME THE WORK IS BROKEN" / "judge
  BEHAVIOR, not intent" (evaluator), "spot-reproduce" + "did not write the code"
  (judge), "git merge-tree --write-tree" + "MUST NOT be failed for staleness" (both),
  the capability self-test line (`git rev-parse HEAD` + "NO verdict authority",
  evaluator), the content-state line (`verdict_patch_id`, both prompts), the
  risk-marker rule (`NOT_REQUIRED`, evaluator),
  the run-to-completion sentence, the three seam names in the doctrine paragraph, and
  the pour commands naming `mol-evaluate-task`/`mol-judge-task`.
- **(b) `test/packlint/csc_eval_formula_test.go`**: both formulas assert
  `contract = "graph.v2"`, the R2 verdict keys and values, the escalation strings
  (`decision_state=mayor_action`, the mail subject literal), the routing writes
  (`gc.kind=judge_request` in evaluate; the refinery handoff block in judge; the polecat
  pool route `{{binding_prefix}}polecat`), the evidence grammar
  (`.gc/evidence/` + `eval-attempt-`), the `[ -s "$EVID_FILE" ]` PASS guard, the R4
  lookup function name `effective_rig_var` present in BOTH formulas, the content-state
  machinery (`git patch-id --stable` present in BOTH formulas; `verdict_patch_id`
  written in every **mol-evaluate-task** verdict block (the evaluator is the sole
  writer); the stale-content re-route string "stale verdict"
  + `gc.kind=eval_request` in the judge formula; the infra re-route string
  "unsubstantiated evaluator PASS"), the conditional-judge fork (`NOT_REQUIRED` +
  the risk-marker checklist strings in mol-evaluate-task's verdict step), the residue
  check strings (`residue` + "silent residue" in mol-evaluate-task's ordered checks),
  and vars
  `max_eval_rejects`/`evidence_publish_cmd`/`evidence_fetch_cmd` declared with the
  pinned defaults. Include ONE planted-RED style negative: assert the string
  `regenerate_on_reject` does NOT appear in either formula (reserved ≠ declared).
- **(c) compose + seam-precedence test** (extend `cmd/gc/embed_builtin_packs_test.go` or
  a sibling `csc_` test file in `cmd/gc`, following
  `TestCodegenSupportBuiltinPackComposesWithGastown` — extend its qualified-name
  assertion list rather than duplicating its scaffolding where practical): compose a
  scratch city importing `gastown` + `codegen-support`; assert the composed config
  exposes qualified `codegen-support.evaluator` and `codegen-support.judge` agents
  (rig-scoped: expect the rig-qualified `app/codegen-support.evaluator` /
  `app/codegen-support.judge` forms alongside the existing asserted names, no
  regression on those); assert the
  materialized `.gc/system/packs/codegen-support/` tree contains the two agent dirs, the
  two formulas, and the three seam fragments (this is the mirror-mechanism proof —
  Non-Goals). Then the **K1 shadowing/precedence probe**: give the scratch city's own
  pack a `template-fragments/city-invariants.template.md` defining `city-invariants`
  with a sentinel string, patch a test agent with
  `append_fragments = ["city-invariants"]`, render its prompt through the repo's render
  path, and assert the sentinel appears EXACTLY once (city define beat the upstream
  empty define). If the upstream empty define wins instead, DO NOT work around it in
  pack content — STOP and raise a structured blocker (template-precedence fix would be a
  Go change outside this WO; GCD-WO-CSC-006's whole binding depends on the answer).
  Record the outcome in the README (Step 8 item 9).
- **(d) generic-ness grep gate test** (`test/packlint/csc_genericness_test.go` or fold
  into (a)): case-insensitive `matchpoint|enrichment|vehicle` over all NINE new pack
  files (4 agent files + 2 formulas + 3 seam fragments) + README returns ZERO hits; also
  assert no `master/` path string and no `sk-ant-` / token-looking string.

**Step 10 — full validation battery** (see Validation) + PR.

## Git Workflow

Loop execution: branch `wo/GCD-WO-CSC-003-evaluator-judge-primitives` (or
`polecat/$BEAD_ID` under city execution) in this repo; PR to `origin/main` on
`mike-matchpoint/gascity` (the fork is the estate home — never PR upstream to
`gastownhall/gascity`). Never commit directly to `main`; run `make setup` once
(pre-commit hooks); commit early and often; the harness merges on green
evaluator+judge — never self-merge.

## Test Coverage

- **Packlint string-contract tier** (Step 9a/9b/9d): every pinned literal from R2 and
  every load-bearing routing write is pinned by a failing-when-absent assertion —
  these tests are the drift alarm for GCD-WO-CSC-005/006/007's discovery gates.
- **Go compose tier** (Step 9c): agents compose into a scratch city; materialization
  proves the embed globs ship the new files; the seam-precedence probe resolves WS2 risk
  K1 with a real rendered prompt (planted-RED by construction: the sentinel-absent case
  fails the assertion).
- **Repo baseline:** `make build && make check` (fmt/lint/vet/test), `go test
  ./internal/builtinpacks` (content hash recomputes over the new embedded files —
  exercises the embed without any Go change), `GC_FAST_UNIT=1 go test ./cmd/gc -run
  '<the compose test name>'`.
- Every Acceptance Criterion below names its backing test.

## Validation

- `make build && make check` green; `go test ./test/packlint` green; the Step 9c compose
  test green with the seam-precedence outcome recorded in the README.
- **Generic-ness grep gate (load-bearing, GCD-WO-EVAL-001 discipline):**
  `grep -riE "matchpoint|enrichment|vehicle" examples/gastown/packs/codegen-support/`
  returns no hits in the pack runtime surface (agents/, formulas/, template-fragments/,
  pack.toml, README.md); test-file hits only as negative-assertion data, each justified
  in the PR.
- `gc lint` over a scratch city composing the pack: clean (the three seam names resolve
  via the empty upstream defines — `cmd/gc/cmd_lint.go:314` class diagnostics).
- R4 STOP-gate transcript in evidence: the scratch-city `gc config show --json` output
  with the pinned `<RIG-FORMULA-VARS-PATH>` shown selecting the sentinel value.
- Diff audit: NO changes outside `examples/gastown/packs/codegen-support/` + the named
  test files; `git diff --stat origin/main...HEAD` shows pack.toml and embed.go
  UNCHANGED.
- **Cities-PAUSED clause:** all GasCity-in-AWS remains paused (zero-replica /
  suspended); this WO performed no cluster/AWS interaction, started no city, no daemon,
  no supervisor (scratch-city compose tests run inside the Go test harness / temp dirs);
  no live drill is claimed. Live evaluator/judge behavior on the vehicle-graph pilot is
  GCD-WO-CSC-006's named un-pause follow-up.
- Evidence artifacts: command transcripts as `{command, output_excerpt}` pairs — this
  WO's own proof meets the C9 evidence bar it defines.

## Acceptance Criteria

Each criterion names its backing test:

1. Evaluator + judge agents present with EXACTLY the R2 structural literals (scope,
   selectors, pools, models) — `test/packlint/csc_evaluator_judge_agents_test.go`.
2. Both prompts carry the ported diligence stance (assume-broken, act-don't-read,
   deliverable-in-diff, anti-fabrication taxonomy, band-aid rule, conflict-only
   merge-readiness, run-to-completion; judge = transition-only maker-checker with
   spot-reproduction) — same test. The judge prompt ADDITIONALLY carries the two
   D18/D21 sections (Step 4): the REQUIRED code-review-skill review procedure
   (skill named generically + purpose + review-at-`bound_version` rule) and the
   END-TO-END PROCESS COHESION criterion (orphaned docs/bindings/indexes = REJECT) —
   packlint literal assertions added to the same test; both sections generic
   (no estate literals — the concrete skill identity rides the city seam fragments,
   AC 7 unweakened).
3. Both formulas present, graph.v2, with the R3 routing state machine as explicit
   chained `gc bd update` blocks — including `verdict_patch_id` on every
   mol-evaluate-task verdict write (the evaluator is the sole writer; the judge
   re-computes and matches — `git patch-id --stable` present in both formulas), the
   deterministic risk-marker fork with
   the `NOT_REQUIRED` direct-handoff block, the judge's stale-content and infra
   re-route rows (budget-neutral, back to `eval_request`), the residue
   presence/honesty check — plus the shared `eval_reject_count` budget, the
   `max_eval_rejects` escalation to `decision_state=mayor_action` + mayor mail, and the
   evidence-guarded PASS — `test/packlint/csc_eval_formula_test.go`.
4. Evidence artifact grammar + publish/fetch seam pinned; PASS impossible without a
   non-empty artifact — same test (grammar strings + `[ -s` guard).
5. Three seam fragments exist as EMPTY defines with the C11 names; city-define-wins
   precedence proven (or a structured blocker raised); mechanism recorded in the README
   — Step 9c compose test.
6. Pack README carries all thirteen Step-8 items, including the explicit RIG-scope
   binding statement, the no-`CLAUDE_CONFIG_DIR` statement, the reserved
   `regenerate_on_reject` note, the content-state law, the conditional-judge
   risk-marker table, and the residue contract — packlint assertion on the README
   literals (add to 9a or 9d).
7. Zero domain literals in the pack runtime surface; no embed.go/pack.toml change; no
   Go changes outside test files — grep gate + diff audit (Validation).
8. Agents compose into a scratch city as `codegen-support.evaluator`/`.judge`; new files
   materialize to `.gc/system/packs/codegen-support/` — Step 9c.
9. No city started; no AWS/cluster call; no live eval run (cities PAUSED) — Validation
   clause.

## Risks

- **Fragment-name precedence differs from the documented formula-name rule (WS2 K1)** —
  resolved empirically by the Step 9c probe with a STOP-gate; GCD-WO-CSC-006 binds
  against the RECORDED outcome, not an assumption.
- **`gc config show --json` shape drift (R4)** — the jq path is pinned at execution
  against real output with a transcript; the packlint test pins the formula text so
  later refactors that break the path fail loudly in packlint review, and the lookup
  fails SAFE (formula default) at runtime.
- **Selector/claim semantics:** `gc work claim` honors the agent's typed selector
  (landing-arbiter precedent). If claim flags behave differently for pool agents at
  execution time, mirror the landing-arbiter's exact claim invocation — it is the proven
  in-repo shape; do not invent new flags.
- **Evidence durability in distributed cities:** the local default dies with a pod in
  no-shared-FS mode — DELIBERATE fail-closed design (judge rejects unreachable
  evidence), forcing the durable-URI seam at un-pause instead of silently passing.
  Documented in the README; the AWS lane is `AGC-WO-CSC-005`-adjacent city wiring, not
  upstream scope.
- **Stale verdicts skating through:** prevented structurally on TWO axes — the submit
  sequence (GCD-WO-CSC-005) clears `eval_verdict`/`judge_verdict`/`verdict_patch_id` on
  every resubmission, and the content-state key makes staleness detectable even when a
  clear is missed: the judge re-computes the patch-id before approving, and the
  refinery's gated branch (same WO) degrades ONLY when the verdict's `verdict_patch_id`
  equals the patch-id of the content it is about to merge. The
  R3 table is the single shared authority; both WOs quote it.
- **WO size:** 2 agents + 2 formulas + 3 one-line fragments + README + 4 test files is
  one coherent generation run. If the prompts balloon, cut prose, never checks — the
  ordered-check list and the R3 writes are the load-bearing core.

## Done Means

- [ ] Step-0 verification transcript recorded (base SHA, EVAL-001 present, anchors,
      embed globs, R4 pin).
- [ ] `agents/evaluator/` + `agents/judge/` shipped per R1/R2; prompts carry the ported
      diligence; seam paragraph present.
- [ ] `mol-evaluate-task` + `mol-judge-task` shipped with the R3 state machine,
      shared budget, escalation, evidence-guarded PASS, R4 lookup block.
- [ ] Three empty-default seam fragments shipped (template-comment form).
- [ ] README binding doc shipped with all ten items (rig scope + no-CLAUDE_CONFIG_DIR +
      reserved var + precedence outcome).
- [ ] Packlint + compose + grep-gate tests green; `make build && make check` green;
      `go test ./internal/builtinpacks` green.
- [ ] No edits outside the pack + test files; pack.toml/embed.go byte-identical.
- [ ] PR merged to `origin/main` via the loop (evaluator+judge green); no direct-to-main
      commit.
- [ ] No city started; live pilot validation left to GCD-WO-CSC-006's named follow-up.

## Master cutover contribution

None — platform repo (GasCity-Dev fork), no AWS resources created, renamed, or deleted
(kit K1 prod-gate language not triggered). Runtime exposure reaches hosted cities only
via the wave-24/25 city bindings + aws-GasCity image/deploy WOs at un-pause, under their
own gates. The C9 contract this WO authors is referenced by the estate's error/overseer
program (A1 §2 `overseer_issue_id`) — that linkage lives in bead metadata and city
fragments, not in this repo's runtime surface.

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
