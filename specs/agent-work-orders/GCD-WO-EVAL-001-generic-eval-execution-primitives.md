# Work Order: GCD-WO-EVAL-001 — generic eval-execution primitives in the `execution-city-operations` pack lineage

Execution classification: Dev-only (pack content + embed wiring + repo-native tests; no AWS,
no deploy surface). Master cutover contribution: **None (platform repo, no AWS)** — see the
section below.

> **Repo status note (binding):** this repository (`mike-matchpoint/gascity`, the GasCity
> platform fork; upstream `gastownhall/gascity`) is **newly loop-homed** into the MatchPoint
> estate loop. This is its **first estate work order**; this file creates
> `specs/agent-work-orders/` (the repo's own `specs/` previously held only `architecture.md`).
>
> **Provenance (binding):** ratified plan
> `master/source-schema-foundation/enrichment-eval-and-agent-efficacy-plan.md` §4.2 **G1**
> (NEW wave 17 "Enrichment eval & agent-efficacy band", `boundary=dev`), under owner ruling
> **OR-1** (2026-07-05): *GasCity is the agentic-execution orchestration harness — generic
> primitives that both execution-monitoring cities can reuse land in GasCity-Dev system packs;
> business-domain-specific agent prompts land in city custom packs.* Companion doctrine: plan
> §2 ("GasCity-native runners") and §0 rule 2 (cities PAUSED — authoring + structural
> validation only).
>
> Verified against this repo's `origin/main` 2026-07-05 @ `24d3f7b4` ("Use runtime inventory
> for API status hot paths"). Re-verify at execution time.

## Goal

The `execution-city-operations` pack (`examples/gastown/packs/execution-city-operations/`)
gains **generic, domain-agnostic eval-execution primitives**, reusable by BOTH MatchPoint
execution-monitoring cities (product-enrichment + vehicle-graph) and any future importing
city, with **ZERO MatchPoint/business-domain literals** (the load-bearing constraint — domain
bindings, rubrics, and prompt content live in city custom packs per OR-1):

1. **`eval.run_cohort.v1` molecule formula** — plan → per-case fan-out (one bead per cohort
   case, fixture payload injected) → execute-under-test (routes to the importing city's bound
   surface-under-test agent pool via `gc.kind` metadata) → deterministic-grade (invokes a
   city-provided grading TOOL command — agents never grade themselves) → aggregate →
   threshold gate → finalize with an eval-run manifest artifact.
2. **`eval.replay_step.v1` molecule formula** — replay ONE recorded step-bead fixture (input
   payload + workspace fixture ref) against the routed agent pool and grade the trace
   (terminal status, domain-command set, artifact rubric) through the same
   deterministic-tool seam.
3. **Generic eval-runner agent prompt(s)** (one-shot, city scope) for the plan / aggregate /
   finalize steps, written to the pack's existing agent + prompt-fragment conventions.
4. **An eval-run artifact contract template-fragment** (+ JSON schema for the eval-run
   manifest), aligned to the MatchPoint `EvalStepFixture@v1` / eval-run row shapes **by field
   convention only** — the alignment is documented; the pack imports no MatchPoint code and
   stays standalone.
5. **Producer→triage wiring:** failed/regressed eval runs emit the evidence-packet shape the
   existing `prompt-eval-classifier` / `prompt-eval-judge` / `prompt-eval-evidence-gatherer`
   agents already consume — these triage roles finally get a producer. The routing is
   documented in the pack.
6. **Embed/ship wiring** per the repo's own bundled-pack pattern + structural tests per the
   repo's test conventions.

## Dependencies

- **Blocked by:** `Matchpoint-Platform::PAR-WO-SSF-P2A-003-agent-execution-eval-kit` (plan
  §4.1 B1, wave 14 — the `EvalStepFixture@v1` bundle family + trace-grading interfaces this
  pack aligns to by field convention). At authoring time that WO file is part of the same
  authoring batch and may not yet be on Matchpoint-Platform `origin/main` — re-verify the
  merged contract field set at execution time and align the manifest/fixture field names to
  the MERGED family, never to this WO's illustrative lists.
- **Consumed by (same wave 17):** `Matchpoint-Product-Enrichment::PE-WO-EVAL-CITY-001-product-city-eval-pack`
  and `Matchpoint-Vehicle-Graph::VG-WO-EVAL-CITY-001-vehicle-city-agent-efficacy-pack` —
  the city custom packs bind these formulas to their domain agent pools, rubrics, and
  grading tools. Nothing in this WO may presuppose either consumer's domain.
- **The MatchPoint cities are PAUSED** (plan §0 rule 2; standing Gas City pause policy):
  this WO is authoring + repo-native tests only — `make check` / `make test-packs` class
  validation against fake CLIs. **No city runs, no live eval drills**; live validation is a
  named follow-up gated on un-pause, never an acceptance criterion here.
- Repo gates: this repo's own `CONTRIBUTING.md` (fork/branch workflow, `make setup`,
  `make build && make check`), `TESTING.md` (three-tier test philosophy), and the two design
  principles (Zero Framework Cognition; Bitter Lesson Alignment) bind every change.

## Non-Goals

- **NO domain bindings, rubrics, cohort data, grading tools, or prompt content** for any
  business domain — those are PEC1/VGC1 city-pack scope. No `matchpoint`, `enrichment`, or
  `vehicle` literal anywhere in the pack runtime surface (tests excepted as registration
  data; see Risks).
- **NO MatchPoint code imports** — alignment to `EvalStepFixture@v1` / eval-run rows is by
  documented field convention; the pack must remain consumable by a city with no MatchPoint
  estate at all.
- **NO new Go role logic.** Zero Framework Cognition: Go handles transport, not reasoning.
  The only Go edits are the `embed.go` `go:embed` directive and test files. No new packages
  under `internal/`, no grader implementation in Go — the grader is a city-provided command
  invoked through a formula var.
- **NO grading by agents.** Agents plan, aggregate, and file; grading is exclusively the
  deterministic city-provided tool (plan §2: "cities invoke graders as deterministic tools;
  agents never grade themselves").
- **NO changes to the `domain-handoff` pack**, the `internal/builtinpacks` registry Go code
  (the pack is already registered; its content hash is computed, not pinned), the gc CLI, or
  any other pack's content.
- **NO upstream (gastownhall/gascity) PR** — this lands on the fork's `main` only; upstream
  contribution is a separate decision.
- **NO city bootstrap, no `.gc/system/packs` mirror** — `execution-city-operations` has no
  mirror under `examples/gastown/.gc/system/packs/` (only bd/codegen-support/core/dolt/
  gastown/maintenance do); do not create one.

## Architecture Links

- `master/source-schema-foundation/enrichment-eval-and-agent-efficacy-plan.md` §0–§2, §4.2 G1
  (MatchPoint estate — the ratifying authority; not in this repo).
- This repo: `CLAUDE.md` → `AGENTS.md` (MEOW stack: beads → molecules → formulas; ZERO
  hardcoded roles), `specs/architecture.md`, `CONTRIBUTING.md` (Design Philosophy +
  Primitive Test at `engdocs/contributors/primitive-test.md`), `TESTING.md`.
- Pack doctrine precedents in-tree:
  - `examples/gastown/packs/execution-city-operations/pack.toml` — "Generic operations
    primitives for execution and monitoring cities."
  - `examples/gastown/packs/domain-handoff/pack.toml` — the business-process-agnostic
    execution machinery pack; its dispatcher cooks inbound work into molecules whose
    "`gc.kind`-stamped step beads flow through the city's own routing" — the exact routing
    seam the execute-under-test step reuses.
  - `examples/gastown/packs/codegen-support/formulas/mol-debugger-plan.formula.toml` — the
    graph.v2 formula authoring oracle (`[vars]`, `[[steps]]` with `id`/`title`/`needs`,
    fail-fast identity steps, `gc runtime drain-ack` discipline).
  - `internal/bootstrap/packs/core/formulas/mol-scoped-work.toml` — step `metadata =
    { "gc.kind" = ... }` stamping precedent.

## Packages To Inspect

All paths repo-relative; inspect READ-first, then extend:

- `examples/gastown/packs/execution-city-operations/` — the pack under extension:
  - `pack.toml` (schema 2; `[[named_session]]` only for the four always-mode roles —
    one-shot pool agents are directory-discovered, NOT listed here),
  - `embed.go` (`//go:embed pack.toml all:agents template-fragments all:schemas all:assets`
    — **currently embeds no `formulas/`**; this WO adds the directive term),
  - `agents/prompt-eval-classifier|prompt-eval-judge|prompt-eval-evidence-gatherer/`
    (`agent.toml`: `scope = "city"`, `lifecycle = "one_shot"`, `wake_mode = "fresh"`,
    `work_dir = ".gc/agents/execution-city-operations/<pool>/{{.AgentBase}}"`,
    `[work_selector.metadata] "gc.routed_to" = "{{.Agent}}"`; `prompt.template.md`:
    propulsion include + boundaries + evidence-contract + prompt-eval-contract +
    incident-taxonomy + command-glossary, `gc work claim --status=in_progress --json`
    startup, `gc runtime drain-ack` completion),
  - `template-fragments/prompt-eval-contract.template.md` (define
    `execution-prompt-eval-contract`: the evidence-packet field list + the nine judge
    decision categories — the CONSUMER contract the producer wiring must satisfy),
  - `template-fragments/propulsion.template.md` (per-agent propulsion defines — new agents
    add defines here),
  - `schemas/` (`schemas/README.md` + `schemas/events/*.schema.json` + `examples/` —
    the schema-file conventions for the new eval-run manifest schema),
  - `assets/scripts/publish-cross-city-event.sh` (the single approved event-bus emitter,
    should any cross-city eval signal ever be needed — cite, don't duplicate).
- `examples/gastown/packs/codegen-support/formulas/*.formula.toml` + `embed.go`
  (`//go:embed pack.toml formulas orders all:agents all:assets template-fragments`) — the
  formula-shipping precedent, including the `formulas` embed term.
- `internal/builtinpacks/registry.go` (pack already registered at line 57;
  `SyntheticContentHash()` derives from embedded content — no golden to update) +
  `registry_test.go` (pack identity list — unchanged by this WO).
- `internal/config/config.go` `FormulasDir()` (~line 2228) — packs use the **well-known
  `formulas/` directory**; legacy `[formulas].dir` is rejected
  (`internal/config/compose_test.go` "formulas_dir" case). No pack.toml formula registration
  key exists or is needed.
- `cmd/gc/embed_builtin_packs_test.go` —
  `TestExecutionCityOperationsBuiltinPackComposesWithMaintenance` (~line 401): composes the
  pack into a scratch city and asserts qualified agent names
  (`execution-city-operations.mayor`); the structural test to extend for the new agent(s).
- `examples/gastown/packs/domain-handoff/tests/run-tests.sh` + `tests/fakes/{gc,aws}` — the
  pack-script test harness precedent (REAL pack scripts against a fake `gc` JSON-bead-state
  CLI; no network, no live city, no LLM) and `Makefile` `test-packs` (line ~122, currently
  runs only the domain-handoff suite).
- `Makefile` — `check` (= `fmt-check lint vet check-routed-test-rows test`), `test-packs`,
  `check-docs` (= `go test ./test/docsync`), `build`.

## Required Inputs

- The evidence-packet field list and judge decision categories in
  `template-fragments/prompt-eval-contract.template.md` — the producer emission MUST cover
  every packet field the fragment names (eval suite, case ID, prompt name, prompt version,
  model/provider, run ID; expected/actual outcome, score, threshold, failure label; prompt
  input, redacted output excerpt, trace excerpt, scorer rationale; fixture/corpus artifacts;
  related execution/incident; prior passing run/regression window; evaluator limitations).
- The MatchPoint field conventions to align to (documented, not imported), from the merged
  `PAR-WO-SSF-P2A-003` kit: `EvalStepFixture@v1` carries input payload + workspace fixture
  ref + expected terminal status + expected domain-command set + rubric expectations; the
  eval-run row family carries run id, suite/cohort id, case results, scores, thresholds,
  gate outcome, artifact refs.
- The routing seam facts: molecule step beads carry `gc.kind` metadata and city agent pools
  claim them through their own `work_selector` metadata (`domain-handoff` pack.toml comment +
  `assets/scripts/handoff-work-dispatch.sh` lines 8–14, 48–50); triage items route via
  `gc.routed_to`.
- Formula authoring contract: `contract = "graph.v2"`, `[vars]` blocks with
  `description`/`required`/`default`, `[[steps]]` with `id`/`title`/`needs`/`description`,
  optional step `metadata` tables (core `mol-scoped-work.toml` precedent).

## Implementation Steps

1. **`formulas/eval-run-cohort.formula.toml`** (realizes the plan's `eval.run_cohort.v1`
   primitive; the `v1` contract identity is recorded in the formula description and stamped
   into the eval-run manifest `contract` field — formula name/version follow repo convention:
   `formula = "eval-run-cohort"`, `version = 1`, `contract = "graph.v2"`). Vars (all
   city-supplied; every one generic):
   - `cohort_ref` (required) — path/URI of the cohort case-set the plan step enumerates;
   - `surface_kind` (required) — the `gc.kind` value stamped on execute-under-test step
     beads, routing them to the city's bound surface-under-test agent pool;
   - `grader_cmd` (required) — the city-provided deterministic grading tool command; the
     grade step is a deterministic exec of this command over the case output artifact
     (agents never grade);
   - `threshold` (required) + `gate_metric` (default per formula doc) — the threshold-gate
     inputs;
   - `run_id` / `eval_suite` / `binding_prefix` — identity + routing prefix vars following
     the `mol-debugger-plan` var conventions.
   Steps: `plan` (eval-runner agent enumerates cohort cases, validates identity fail-fast)
   → fan-out: one per-case bead each carrying the injected fixture payload and
   `metadata = { "gc.kind" = <surface_kind> }` → `execute-under-test` (claimed by the
   city's pool through its own routing — this formula never names any domain pool) →
   `grade` (deterministic `grader_cmd` exec per case; output = per-case grading-result
   JSON) → `aggregate` (eval-runner agent folds per-case results) → `threshold-gate`
   (deterministic compare; fail path emits the triage evidence packet, step 5) →
   `finalize` (write the eval-run manifest artifact, close the molecule).
2. **`formulas/eval-replay-step.formula.toml`** (realizes `eval.replay_step.v1`): vars
   `fixture_ref` (required — ONE recorded step-bead fixture: input payload + workspace
   fixture ref), `surface_kind`, `grader_cmd`, plus the same identity vars. Steps: validate →
   materialize the fixture workspace → dispatch one `gc.kind`-stamped replay bead →
   `grade-trace` (deterministic `grader_cmd` over the recorded trace: terminal status,
   requested domain-command set, artifact rubric) → finalize with a single-case eval-run
   manifest. Same triage emission on failure.
3. **`agents/eval-runner/`** (`agent.toml` + `prompt.template.md`): one-shot, city-scope
   pool agent per the `prompt-eval-classifier` conventions (`work_dir =
   ".gc/agents/execution-city-operations/eval-runners/{{.AgentBase}}"`, `work_selector` on
   `"gc.routed_to" = "{{.Agent}}"`, `min_active_sessions = 0`). The prompt covers the
   plan/aggregate/finalize responsibilities, includes a NEW
   `execution-propulsion-eval-runner` define added to
   `template-fragments/propulsion.template.md`, includes the existing boundary /
   evidence-contract / prompt-eval-contract / command-glossary fragments, and states the
   hard rules: never grade, never edit prompts or pins, never touch grading tools — route
   judgment to the triage roles. If drafting shows plan vs aggregate/finalize warrant
   separate roles, split into two agents under the same conventions (judgment call;
   document in the PR).
4. **Eval-run artifact contract:** NEW
   `template-fragments/eval-run-contract.template.md` (define
   `execution-eval-run-contract`) + NEW `schemas/eval/eval-run-manifest.v1.schema.json`
   + example under `schemas/eval/examples/`, following `schemas/README.md` conventions.
   Fields: run id, eval suite/cohort ref, formula contract id (`eval.run_cohort.v1` /
   `eval.replay_step.v1`), per-case rows (case id, fixture ref, terminal status,
   domain-command set, artifact refs, score, grading-result ref), aggregate scores,
   threshold + gate outcome, grader command identity + version/hash, timestamps. The
   fragment documents — in prose — the field-convention alignment to MatchPoint
   `EvalStepFixture@v1` / eval-run rows (which field maps to which), and states the pack
   is standalone: alignment is convention, not import.
5. **Producer→triage wiring:** on threshold-gate failure or replay regression, the
   finalize path files ONE triage bead per failure class routed
   (`gc.routed_to`) to the city-qualified `prompt-eval-classifier`, whose body/metadata
   carries every evidence-packet field from `execution-prompt-eval-contract` (Required
   Inputs list) — sourced from the eval-run manifest. Document the routing (producer →
   classifier → judge/evidence-gatherer → codegen-work-filer, per the classifier's Route
   section) in the pack: a `pack.toml` comment block + the eval-run contract fragment.
6. **Embed/ship + tests:**
   - `embed.go`: add `formulas` to the `go:embed` directive (codegen-support precedent);
     `internal/builtinpacks` needs no Go change — content hash is computed.
   - Extend `cmd/gc/embed_builtin_packs_test.go`
     `TestExecutionCityOperationsBuiltinPackComposesWithMaintenance` to assert the composed
     `execution-city-operations.eval-runner` agent (and any split sibling).
   - NEW `examples/gastown/packs/execution-city-operations/tests/run-tests.sh` following the
     domain-handoff harness (fake `gc`; reuse/adapt `tests/fakes/`): structural assertions
     that both formulas parse and stamp `gc.kind` from `surface_kind`, that the grade steps
     exec `grader_cmd` (fake grader script) rather than any agent path, that a failing gate
     files a classifier-routed bead whose fields cover the evidence-packet list, and that a
     sample manifest validates against `eval-run-manifest.v1.schema.json`.
   - `Makefile`: append the new suite to `test-packs`.
   - Run the generic-ness grep gate (Validation) before requesting review.

## Git Workflow

All authoring work on branch `wo/ssf-eval-band`; at loop execution, work orders in this repo
use `wo/<stem>` / `polecat/$BEAD_ID` branches. PR to `origin/main` on
**`mike-matchpoint/gascity`** (the fork is the estate's home; never PR to
`gastownhall/gascity` upstream from this WO). Never commit directly to `main`. Per this
repo's `CONTRIBUTING.md`: run `make setup` once (installs `.githooks/pre-commit`), and never
open a PR from a fork's `main` branch.

## Test Coverage

- **Pack-script tier (`tests/run-tests.sh`, wired into `make test-packs`):** the structural
  suite of step 6 — formula parse, `gc.kind` routing stamp, deterministic-grader exec seam,
  triage emission field coverage, manifest schema validation. Planted RED: a manifest missing
  the gate outcome fails schema validation; a triage packet missing `failure label` fails the
  field-coverage assertion.
- **Go structural tier:** extended
  `TestExecutionCityOperationsBuiltinPackComposesWithMaintenance` green (new agent composes;
  no `dog`/`operations-dog` regression); `go test ./internal/builtinpacks` green (embedded
  tree still materializes + round-trips the content hash — exercises the new `formulas/`
  embed term).
- **Docs tier:** `make check-docs` green (docsync) if any docs-tree file is touched.
- Unit tests follow `TESTING.md` tier 1 discipline (tests next to code, `t.TempDir()`,
  `require`/`assert`).

## Validation

- `make build && make check` green (`fmt-check lint vet check-routed-test-rows test`).
- `make test-packs` green including the NEW execution-city-operations suite.
- `go test ./internal/builtinpacks` green;
  `GC_FAST_UNIT=1 go test ./cmd/gc -run TestExecutionCityOperations` green.
- **Generic-ness grep gate (load-bearing):**
  `grep -riE "matchpoint|enrichment|vehicle" examples/gastown/packs/execution-city-operations/`
  returns NO hits in the pack runtime surface (`pack.toml`, `agents/`, `formulas/`,
  `template-fragments/`, `schemas/`, `assets/`); hits under `tests/` are permitted only as
  registration-data fixtures and each must be justified in the PR description.
- The eval-run manifest example validates against its schema; the schema follows the pack's
  existing `schemas/` conventions.

## Master cutover contribution

None — platform repo, no AWS. Nothing here deploys; runtime exposure arrives only through
the importing city packs (PEC1/VGC1) at city un-pause, under their own gates.

## Acceptance Criteria

- Both formulas, the eval-runner agent(s) + propulsion define, the eval-run contract
  fragment + `eval-run-manifest.v1` schema/example, and the producer→triage routing
  documentation landed inside `examples/gastown/packs/execution-city-operations/`.
- The pack ships: `formulas` embedded via `embed.go`; bundled-pack materialization tests
  green; the composed city exposes the new qualified agent name(s).
- Producer→triage evidence packets cover every field named by
  `execution-prompt-eval-contract`; the classifier/judge/evidence-gatherer prompts are
  UNCHANGED (they are consumers — this WO gives them a producer, not an edit).
- Zero domain literals in the pack runtime surface (grep gate); agents never grade
  (deterministic `grader_cmd` seam proven by the fake-grader test).
- All repo gates green: `make build && make check`, `make test-packs`, targeted `go test`
  runs above.
- No city was started; no live eval ran (cities PAUSED).

## Risks

- **Upstream-fork divergence** (`gastownhall/gascity` upstream moves independently; the
  builtinpacks `Repository` const and bundled-pack import sources point at the upstream
  URL) → keep the addition fully self-contained inside the existing pack directory + one
  `go:embed` term + additive test/Makefile lines, so rebases onto upstream stay clean;
  no registry Go edits, no cross-pack edits.
- **Generic-ness erosion** (domain assumptions leaking into "generic" formulas/prompts) →
  the grep gate (`matchpoint|enrichment|vehicle`) is a hard acceptance criterion on the
  runtime surface, tests excepted as registration data; every var a city must supply
  (`surface_kind`, `grader_cmd`, `cohort_ref`, thresholds) is an injected formula var,
  never a default naming any domain.
- **Contract drift vs `PAR-WO-SSF-P2A-003`** (field-convention alignment authored before/
  while the kit merges) → re-verify the merged `EvalStepFixture@v1` field set at execution
  time; the alignment lives in ONE prose section of the contract fragment, so reconciliation
  is a fragment edit, not a schema break.
- **Formula-contract mismatch** (graph.v2 semantics — fan-out/aggregate shapes this pack has
  not used before; codegen-support formulas are the nearest precedent, not an exact one) →
  structural tests parse and exercise the formulas against the fake `gc` before any city
  ever cooks them; if graph.v2 cannot express the fan-out cleanly, ORCHESTRATOR-RESOLVE
  (report, do not invent new Go orchestration — Zero Framework Cognition).

## Done Means

- [ ] All work on `wo/ssf-eval-band` (authoring) or `wo/<stem>` / `polecat/$BEAD_ID` (loop
      execution), merged to `origin/main` on `mike-matchpoint/gascity` via PR; nothing
      committed directly to `main`.
- [ ] Deliverables 1–6 landed inside the pack; embed + structural tests shipped.
- [ ] Pack-script + Go structural + docs tiers green; none skipped without an N/A reason.
- [ ] Generic-ness grep gate green on the runtime surface.
- [ ] Triage consumers untouched; producer wiring documented in-pack.
- [ ] No city started; live eval drill recorded as a named follow-up gated on un-pause.
- [ ] No prior work-order file edited to change behavior.
