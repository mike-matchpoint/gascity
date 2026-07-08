# Work Order: GCD-WO-CSC-004 — `wo-router` formula: 4-step routing-only planner (cartographer replacement lane) + per-rig watch-order formula selection

NOTE: this WO is long — read the FULL file at
`specs/agent-work-orders/GCD-WO-CSC-004-wo-router-formula.md` in your worktree before
implementing. This WO is the **kit C10 contract authority** ("Router output"): the names
and shapes pinned here (`wo-router`, `wo_planning_formula`, the bead/convoy output shape)
are imported by `GCD-WO-CSC-006`/`GCD-WO-CSC-007` — never re-declared downstream. Changing
any pinned literal at execution time requires a structured blocker back to the Mayor, not a
silent rename.

Execution classification: Dev-only pack content in the GasCity platform fork (formula TOML +
order/watch script edits + deterministic asset scripts + packlint/Go structural tests; no
Go role logic, no AWS, no deploy surface, no city runs). `boundary=dev`, **wave 23** (CSC
program band 23/24/25), `blocked_by`
`GasCity-Dev::GCD-WO-EVAL-001-generic-eval-execution-primitives` (wave 18 — cross-wave,
parser-safe; pinned in harness DEPS regardless).

> **Provenance (binding):** CSC program contract authority
> `master/city-scaling-improvements/wo-authoring-kit.md` — **K4-C10** (this WO IS the C10
> authority: "One work bead per WO (coarse-split 2–5 only for enumerated sub-WO families);
> bead body = WO path + verbatim acceptance criteria + `done_when` + integration target;
> WO-level `depends_on` from explicit annotations only; convoy bead + `integration/<wo-id>`
> branch + HOLDING placeholders preserved verbatim from spec-cartographer; `wave:N` labels
> optional. Watch order gains formula-name var (router vs legacy per rig)."), K2
> (GasCity-Dev bounded context), K6 (test discipline). Backlog + sequencing:
> `master/city-scaling-improvements/gap-analysis-and-build-plan.md` §5 row 4, §6 ("Router
> bead/convoy output shape | GCD-WO-CSC-004 | GCD-WO-CSC-006/007"). Design record:
> `aws-GasCity/docs/investigations/2026-07-city-scaling-improvements.md` WS2 "Proposed
> design" item 6 (owner ruling **D10**, 2026-07-08: "cartographer → 4-step `wo-router`",
> spec-cartographer retained during migration) + WS2 risks K3/K8. Process: root
> `SKILL-work-order-audit-and-authoring.md` §1.1–1.5.
> Verified at authoring (2026-07-08): `GasCity-Dev` `origin/main` @
> `c85d92cf0cfd1215be1467628d6fd2e06db46aae`; all file/line references below verified at
> that SHA. Re-verify at execution — GCD-WO-EVAL-001 (wave 18) merges before this WO
> dispatches; its conventions (ZFC for pack work, no domain literals upstream) bind here.
> Ledger stem: `GasCity-Dev::GCD-WO-CSC-004-wo-router-formula`.

## Goal

Code-generation cities gain a **fast, routing-only planning formula** — `wo-router` — that
replaces the spec-cartographer's ~30-minute, 10-step granular decomposition with a 4-step
pass (minutes-class): map one work order to one (rarely 2–5) whole-WO work bead(s), wire
WO-level dependencies from explicit annotations only, and preserve verbatim the
battle-tested emit machinery (convoy bead with `owned` semantics, `integration/<wo-id>`
branch creation, HOLDING placeholder stubs, pre-mutation emit-plan preflight). Clean end
state, all inside `examples/gastown/packs/codegen-support/` plus its tests:

1. **`formulas/wo-router.formula.toml`** — 4 steps: `init-run` (kept verbatim from
   spec-cartographer) → `route-and-order` (the ONLY LLM-reasoning step) → `emit`
   (script-assisted, deterministic mutation) → `validate-store` (deterministic script).
2. **Per-rig formula selection**: `orders/spec-cartographer-watch.toml` +
   `assets/scripts/spec-cartographer-watch.sh` read a rig-scoped var —
   **`wo_planning_formula`** (pinned name; value = formula name, e.g. `"wo-router"`;
   default when unset = `"spec-cartographer"`) — from `[rigs.formula_vars]`, and sling the
   selected formula. Busy-serialization covers BOTH planner formulas.
3. **Deterministic asset scripts**: `assets/scripts/wo-router-emit.sh` (emit-plan assembly
   + integration-branch push + shared preflight + `bd create --graph` + HOLDING release
   batch) and `assets/scripts/wo-router-validate-store.sh` (post-mutation store checks) —
   applying the proven `cartographer-inventory.sh` LLM-prose→script conversion precedent
   (spec-cartographer.formula.toml:866–878: ~80s of LLM shell-prose → 1–2s deterministic
   script).
4. **`spec-cartographer` RETAINED, byte-identical** (C10/D10: additive migration; each rig
   chooses via the watch var; the legacy formula and its packlint pins stay untouched).
5. **Packlint + pack-script tests** proving the formula structure, the watch selection, the
   deterministic scripts, and the no-regression bar on the legacy lane.

Why: WS2/D10. The spec-cartographer is slow because of one-fresh-session-per-step ×10 with
three heavy LLM stages — full transitive spec fan-out with verbatim `key_excerpts` staging
(`read_order`), 200–1500-word self-contained task authoring (`decompose`), and a
per-candidate one-at-a-time bidirectional dependency audit over up to 250 beads (`graph`).
The loop harness proved whole-WO units with explicit-edge-only dependency wiring work
better AND faster: Codex 5.5 implements a full work order from the WO file itself
(generator reads specs in its own worktree), so bead-body excerpt inlining and granular
decomposition are wasted serialized-planner minutes. The router keeps everything that
protects the bead store and the merge machinery, and deletes only LLM reasoning.

**Consumers (import citations — they NEVER re-declare):**
- `GasCity-Dev::GCD-WO-CSC-006-city-pack-binding-pilot` (wave 24) — sets
  `wo_planning_formula = "wo-router"` in the vehicle-graph rig's `[rigs.formula_vars]`
  (its Step-0 discovery R1d verifies the names THIS WO pins).
- `GasCity-Dev::GCD-WO-CSC-007-city-pack-binding-fanout` (wave 25) — same, per fan-out rig.

## Dependencies

- **Blocked by:** `GasCity-Dev::GCD-WO-EVAL-001-generic-eval-execution-primitives`
  (wave 18, merged before this WO dispatches). Binding conventions imported from it:
  **Zero Framework Cognition for pack work** (no new Go role logic — the only Go files
  this WO adds are tests under `test/packlint/`), zero business-domain literals in
  upstream packs, and the repo-gate battery (`make build && make check`,
  `make test-packs`). The `<domain>.<formula>.v1` naming rule is for CITY binding formulas
  in exec-mon city packs; codegen-support formulas follow the repo's bare-name precedent
  (`spec-cartographer`, `mol-debugger-plan`) — `wo-router` conforms (WS2 risk K8 checked).
- **Sibling wave-23 WOs (no edges):** `GCD-WO-CSC-003` (evaluator/judge) and
  `GCD-WO-CSC-005` (polecat/refinery fragments) edit DIFFERENT files in this pack
  (`agents/evaluator|judge/`, `template-fragments/`, `formulas/mol-refinery-patrol` /
  `mol-evaluate-task` / `mol-judge-task`). This WO touches none of those. The single
  file-adjacency risk is `orders/`+`assets/scripts/spec-cartographer-watch.*` — owned
  exclusively by THIS WO.
- **Cities PAUSED (standing policy + kit K1):** this WO verifies all GasCity-in-AWS
  remains paused (zero-replica / suspended) before declaring success — concretely: no
  hosted interaction of any kind (no kubectl, no AWS API, no `gc` daemon/city/session
  start, locally or hosted); all validation is `make`-class + fake-CLI harnesses. Live
  drills are only ever the vehicle-graph pilot, explicitly named, re-suspend after —
  **this WO names NO live drill**; the first live router run arrives via GCD-WO-CSC-006's
  binding at un-pause, under its own gates.
- **Fixture-realism doctrine** (owner-ratified, REJECT-level):
  `master/DOCTRINE-fixture-realism-and-lifecycle-seam-acceptance.md` — test fixtures below
  replicate real shapes (real WO markdown with real annotation lines, real
  `gc ... --json` output shapes); zero-item runs never green (every suite asserts its
  expected match count first; planted-RED self-checks included).

## Non-Goals

Bounded-context REJECT rules (kit K2, GasCity-Dev row) restated, plus WO-specific ones:

- **NO Go role logic, NO engine changes.** ZFC: everything is pack content (TOML, markdown
  step prose, bash). New files inside the already-registered codegen-support pack ship via
  the existing `//go:embed pack.toml formulas orders all:agents all:assets
  template-fragments` directive (`examples/gastown/packs/codegen-support/embed.go:8`) —
  **no embed.go edit needed** (verify: all new files land under `formulas/`,
  `assets/scripts/`, `orders/`, `agents/cartographer/`). No `internal/` edits.
- **NO business/domain literals** in this pack (K2: city packs only). No `matchpoint`,
  no estate repo names, no AWS resource names in any file this WO adds.
- **NO edits to `formulas/spec-cartographer.formula.toml`** — retained byte-identical.
  The existing packlint pins (`test/packlint/spec_cartographer_formula_test.go`) must
  still pass untouched.
- **NO `.gc/system/packs` mirror files created or edited.** Verified mechanism: mirrors
  are MATERIALIZED by the `gc` binary from the embedded pack FS on every `gc start` /
  `gc init` (`cmd/gc/embed_builtin_packs.go:26,53` `MaterializeBuiltinPacks`, idempotent,
  with stale-file pruning). There are NO committed mirror copies in this repo — "keeping
  mirrors in sync" is exactly: ship the files in `examples/gastown/packs/codegen-support/`
  and let the embed+materialize path do the rest.
- **NO city repo edits** (city bindings = GCD-WO-CSC-006/007 via `[rigs.formula_vars]`);
  **NO exec-monitoring-city / execution-city-operations pack changes** (D5; the sole
  exec-city change in the program is GCD-WO-CSC-008).
- **NO agent pool/provider changes.** The router is slung against the SAME
  `codegen-support.cartographer` agent: codex / `gpt-5.5` / `effort=high`,
  `max_active_sessions = 1` all stay exactly as
  `agents/cartographer/agent.toml:60-99` pins them. The duplicate-emit race documented
  there (2026-05-13 WO-008 double-convoy incident) is unresolved upstream — the singleton
  cap is the protection and MUST NOT be raised. The only agent.toml change is the `nudge`
  string (Step 5).
- **NO changes to polecat/refinery/debugger/landing-arbiter formulas or fragments**
  (GCD-WO-CSC-005 territory). The router's emitted beads reuse the EXISTING downstream
  contract fields (`gc.routed_to`, `metadata.target`, convoy `owned`) — consumers are
  untouched.
- **NO new dependency-inference heuristics.** Edges come from explicit `Blocked by:` /
  `Blocks:` / `Land-together rule:` / family annotations ONLY (C10). Re-adding
  per-candidate LLM dependency audits, inline-prose blocker mining, or "looks related"
  edges is a REJECT.
- **NO removal of the self-containment doctrine from spec-cartographer.** The router
  deliberately INVERTS bead-body self-containment (body = pointer to the WO file + verbatim
  acceptance criteria; the polecat reads the WO/specs itself in its own worktree — the
  harness generator model, C10). That inversion applies to `wo-router` output ONLY; the
  legacy formula's forbidden-phrase machinery stays as-is for its own lane.

## Architecture Links

- `master/city-scaling-improvements/wo-authoring-kit.md` — K2, **K4-C10** (this WO's
  contract), C9 (sibling verdict contract — context only), K6, ADDENDUM A1 §4 (estate
  authority; not in this repo — load-bearing pins are quoted in this file).
- `master/city-scaling-improvements/gap-analysis-and-build-plan.md` §5 rows 4/14/15, §6.
- `aws-GasCity/docs/investigations/2026-07-city-scaling-improvements.md` — WS2 "Findings —
  current GasCity primitives", "Proposed design" item 6, risks K3 (whole-WO sizing) and
  K8 (naming) — ratified D10.
- This repo:
  - `examples/gastown/packs/codegen-support/formulas/spec-cartographer.formula.toml`
    (3,510 lines) — THE seam source; the KEEP/DROP map below carries exact line refs.
  - `examples/gastown/packs/codegen-support/agents/cartographer/agent.toml` — launcher
    worktree + pre_start script sync + singleton rationale.
  - `internal/sling/sling.go:975-1077` — `BuildSlingFormulaVars` (precedence: explicit
    `--var` > `rig.formula_vars` > routing-injected) + `mergeRigFormulaVars`; and
    `internal/config/config.go:655-660` (`Rig.FormulaVars`,
    `toml:"formula_vars,omitempty"`, **no json tag** — Go field names on the JSON wire).
  - `cmd/gc/cmd_config.go:60-95,196-230` — `gc config show --json` shape
    (`{"config": <config.City>, ...}`; rigs at `.config.Rigs[]`, fields `Name` /
    `FormulaVars` — Go-marshaled field names).
  - `test/packlint/spec_cartographer_watch_test.go` — the fake-`gc` bash-harness precedent
    the new watch tests extend; `test/packlint/spec_cartographer_formula_test.go` — the
    normalized-contains pin style the new formula test mirrors.
  - `TESTING.md`, `Makefile` (`check` = `fmt-check lint vet check-routed-test-rows test`;
    `test-packs`:124).
- `matchpoint-loop-harness/mlh/` (estate code root; READ-ONLY prior art, cited not
  imported): the whole-WO unit + explicit-`blocked_by`-edges + "NO LLM computes
  eligibility" model this formula ports.

## Packages To Inspect

All READ-first, repo-relative:

- `examples/gastown/packs/codegen-support/formulas/spec-cartographer.formula.toml` — read
  IN FULL. The KEEP/DROP map (Required Inputs R2) is the extraction contract.
- `examples/gastown/packs/codegen-support/assets/scripts/`:
  `cartographer-load-state.sh` (sourced state loader — REUSED verbatim by wo-router
  steps; it has no formula-name assumptions, only `WO_PATH_INPUT`/run-dir layout),
  `cartographer-inventory.sh` (deterministic bead inventory — REUSED verbatim; consumes
  `$RUN_DIR/inventory.json` keys `.blocked_by_annotations[]`, `.inline_wo_blockers[]`,
  `.blocks_annotations[]`, `.family_members[]`, `.cohort_label`, `.land_together_set[]`;
  emits `beads_inventory.json` + `cross_wo_blockers.json` + `downstream_blocks.json` +
  `prior_holding_stubs.json` + `cohort_convoy.json` + `candidate_dependents.json`),
  `cartographer-emit-plan-preflight.sh` (shared pre-mutation gate — REUSED verbatim;
  args `<emit_plan> <graph> <tasks>`, env `WORK_ORDER_ID`; enforces convoy `owned`
  label+metadata, `metadata.target=integration/${WORK_ORDER_ID}`, `gc.routed_to`,
  parent_key/parent_id, HOLDING label/edge coherence),
  `cartographer-validate-emitted-routing.sh` (REUSED with
  `CARTOGRAPHER_ROUTING_MODE=emission`), `spec-cartographer-watch.sh` (EDITED),
  `sync-worktree-scripts.sh` (pre_start mirror of city-level scripts — context).
- `examples/gastown/packs/codegen-support/orders/spec-cartographer-watch.toml` (EDITED —
  comment/description text only; `exec`/`trigger`/`interval`/`timeout` unchanged).
- `internal/formula/` — formula compile/var contract (READ; the new formula must compile
  under the same loader; `gc formula show wo-router` is the smoke).
- `test/packlint/` — all four existing suites; the two new test files join this package.

## Required Inputs

**R1 — pinned names (THE C10 literals; consumers import these):**

| Literal | Value |
|---|---|
| Formula name / file | `wo-router` / `formulas/wo-router.formula.toml` |
| Formula vars | `work_order_path` (required), `rig_name` (required) — identical to spec-cartographer:154-161 so the watch script slings both formulas with the same `--var` pair |
| Watch selection var (rig-scoped, `[rigs.formula_vars]`) | `wo_planning_formula` — value is the formula NAME to sling; unset/empty ⇒ `spec-cartographer` (legacy default; non-CSC cities unchanged) |
| Run-dir layout | `.cartographer-runs/<work-order-id>/<epoch>/` — UNCHANGED (shared with the legacy formula so `cartographer-load-state.sh` works verbatim; the two formulas never run concurrently in a rig — singleton agent + watch busy-gate) |
| Work-bead labels | `source:work-order:${WORK_ORDER_ID}`, `epoch:${EPOCH}` (+ `hold:<blocker-wo-id>` per HOLDING dep; + optional `wave:N` label iff the WO preamble declares a wave — C10 "optional") |
| Bead body shape | see Step 2.5 template (WO path + verbatim `## Acceptance Criteria` + done_when + integration target) |
| Convoy/task metadata | `metadata.owned="true"` + label `owned` + `metadata.target="integration/${WORK_ORDER_ID}"` on the convoy; `metadata."gc.routed_to"=$SLING_TARGET` + `metadata.target="integration/${WORK_ORDER_ID}"` on every task node — byte-compatible with spec-cartographer emit 3a/3c (lines 2362-2510) |
| Route-plan artifact | `$RUN_DIR/route_plan.json` (schema in Step 2.6) |

**R2 — KEEP / DROP seam map** (against `spec-cartographer.formula.toml` @ `c85d92cf`;
quote-verified line refs — the generator re-verifies before extraction):

KEEP (verbatim or near-verbatim carry-over into `wo-router`):

| Seam | Source lines | Disposition |
|---|---|---|
| Formula-description doctrine: completion-first rule, mayor-action notification + disposition (`[CARTOGRAPHER ACTION REQUIRED]`, `decision_state=mayor_action`, `gc.routed_to=gastown.mayor`, never leave failed steps claimable) | 91–149 | carry verbatim (s/ten step beads/four step beads/) |
| HOLDING doctrine: emit-with-placeholders instead of deadlocking on unplanned blockers; scaffold/control-bead enumeration | 52–68, 1414–1462 | carry verbatim (create-side rules; the "identify which subset of new tasks" paragraph collapses to the one-work-bead case) |
| `init-run` step: per-run detached worktree from fetched `origin/main`, helper-script copy, `.beads/redirect`, git exclude patterns, `state.env` + locator copy, work-order-id/epoch derivation, all failure branches, close + `gc runtime drain-ack` discipline | 163–376 | **keep verbatim** as wo-router step 1 (only prose count "ten"→"four" and formula-name strings in mail subjects change) |
| Fresh-session state loading: `WO_PATH_INPUT="{{work_order_path}}" . .gc/scripts/cartographer-load-state.sh` preamble on every post-init step | 348–354 (and each step's step-1) | keep verbatim |
| Deterministic bead inventory via `cartographer-inventory.sh` (script-not-prose rationale) | 739–878 | keep — invoked INSIDE `route-and-order` (Step 2.3) instead of as a separate session |
| Cross-WO blocker edge semantics: convoy bead preferred target, else still-open task beads; false-positives-sequence/false-negatives-break rule | 1154–1171 | keep (applied to the router's whole-WO bead) |
| Cohort convoys: existing-cohort join (`use_existing_convoy_id`, never rename), new-convoy cohort labels applied bare, `convoy_title_override`, stranded members flagged never adopted, create-only rule (`gc convoy add` forbidden; parentage via `parent_key`/`parent_id` in the creation transaction) | 1173–1254 | keep verbatim |
| Family/land-together cohort detection on the TARGET WO (filename regex, `## Sub-work-orders`, label grammar `family:<parent>` / `land-together:<nnn-...>`, precedence family>land-together) | 434–492, 578–605 | keep — folded into `route-and-order` (it reads only the WO file + sibling filenames, no spec-tree walk) |
| Emit architecture: two atomic transactions (`bd create --graph` then `bd batch` HOLDING closes), rollback semantics | 2208–2223 | keep |
| Emit-carrier schema (`nodes[]`/`edges[]`, `parent_key`/`parent_id`, `from_key/to_key/from_id/to_id`, direction "from depends on to") | 2326–2358, 2512–2546 | keep verbatim |
| Convoy node shape + `owned` label AND `metadata.owned` rationale (autoclose hook at `cmd_convoy.go:879` / `hooks.go:51`) + `metadata.target` rationale (refinery auto-land + N²-conflict argument, `runbooks/convoy-close-lifecycle-gap.md`) | 2362–2404 | keep verbatim |
| HOLDING node shape (labels incl. `placeholder:cross-wo-blocker`, `placeholder-blocker:<wo>`, `placeholder-for-wo:<wo>`; `gc.routed_to` OMISSION firewall) | 2406–2434 | keep verbatim |
| Task-node routing + target metadata; `$SLING_TARGET` resolution from `gc config show` awk | 2263–2278, 2479–2510 | keep (moves into `wo-router-emit.sh`) |
| Pre-mutation gate: `cartographer-emit-plan-preflight.sh` invocation, "helper is authoritative" | 2553–2566 (and validate-plan check z, 2135–2158) | keep — run inside `wo-router-emit.sh` immediately before branch push + `bd create` |
| Existing-target re-check (referenced existing beads still open; drop-and-log closed targets; partial-HOLDING downgrade) | 2281–2324 | keep (deterministic — moves into `wo-router-emit.sh`) |
| Integration-branch creation on origin BEFORE `bd create --graph` (idempotent `git push origin origin/main:refs/heads/integration/<wo-id>`; polecat/refinery fail without it) | 2695–2749 | **keep verbatim** |
| `bd create --graph` execution + `emit_mapping.json:ids` resolution + `bd batch` HOLDING close + soft-failure handling + `emitted.json` summary + mayor mail | 2755 to end of emit step | keep (deterministic parts move into `wo-router-emit.sh`; mayor-mail composition stays in step prose) |
| Post-mutation store validation: parent-linkage materialization check, resolvable deps, no-improper-mutation envelope, routing check via `cartographer-validate-emitted-routing.sh` | 3170 to end (validate-store step) | keep — converted to the deterministic `wo-router-validate-store.sh` (inventory_beads precedent) |
| Step close + drain-ack discipline (engine does not auto-close step beads; `--continue` advances) | 357–374 (repeated per step) | keep verbatim on every step |

DROP (deleted from the router lane BY DESIGN — do not partially reintroduce):

| Dropped mechanism | Source lines | Why |
|---|---|---|
| `inventory` step: full spec-tree walk + five-bucket classification + foundation marking + transitive reference recording | 378–635 | router reads ONLY the WO file (+ sibling WO filenames for family detection + the rig roadmap spec iff the WO's annotations name it); polecats read specs themselves |
| `read_order` step: foundation/transitive reads + verbatim `key_excerpts` staging | 637–737 | harness generator model — the implementer reads in its own worktree; no excerpt inlining |
| `decompose` step: atomic-task breakout, 200–1500-word bodies, sizing heuristics | 881–972 | one work bead per WO (C10); coarse-split rule replaces it |
| `graph` step's per-candidate O(N) bidirectional audit: `new_to_existing_decisions` / `existing_to_new_decisions` rows, one-at-a-time iteration, verifier heightened scrutiny | 1000–1153 (fwd), 1256–1413 (rev), 1560–1620 | "NO LLM computes eligibility" — explicit annotations only; deterministic convoy-granularity edges replace the audit |
| Inline-prose blocker mining (`inline_wo_blockers`) + its validate-plan check `m` | 540–576, 2012–2060 | not an explicit annotation; C10 excludes it (the field still exists in `inventory.json` — the router writes it as `[]`) |
| Separate `reconcile` session (prior-epoch classification, `obsolete_review`, edge remapping) | 1653–1784 | collapsed to the deterministic duplicate guard in Step 2.4 (open prior work bead ⇒ mayor-action; else emit fresh) + prior-epoch listing in the mayor mail |
| Separate `validate-plan` session (checks a–n as LLM-run prose) | 1786–2196 | the deterministic preflight helper IS the pre-mutation gate (check z survives; a/l/m/n are moot: no LLM-authored bodies to scan, no candidate audits to cover) |
| Self-containment forbidden-phrase scan on bead bodies | 1814–1831, 936–943 | deliberately inverted: the router bead body POINTS at the WO file (C10) |

**R3 — watch-script config read (pinned mechanism + STOP-gate):** the watch script runs
city-scope via order `exec` (no formula-var substitution available), so it reads the
per-rig var from the resolved config JSON:

```bash
CONFIG_JSON=$(gc config show --json 2>/dev/null || true)
# Per rig, inside the rig loop:
PLANNING_FORMULA=$(printf '%s\n' "$CONFIG_JSON" \
  | jq -r --arg rig "$RIG_NAME" '
      .config.Rigs[]? | select(.Name == $rig) | .FormulaVars.wo_planning_formula // empty
    ' 2>/dev/null || true)
[ -n "$PLANNING_FORMULA" ] || PLANNING_FORMULA="spec-cartographer"
```

`config.City`/`config.Rig` carry TOML tags only (`internal/config/config.go:603-661`), so
`encoding/json` emits Go field names (`Rigs`, `Name`, `FormulaVars`) inside
`gc config show --json`'s `config` key (`cmd/gc/cmd_config.go:196-204`). **STOP-gate:**
before wiring, run `gc config show --json` against a scratch city with a
`[rigs.formula_vars]` entry and confirm the exact key path; if the marshaled names differ,
fix the jq path AND the test fixtures together — never ship a path you did not observe.
Failure posture: any read failure (empty JSON, jq error) falls back to
`spec-cartographer` and logs one line — the watch must never crash a tick over config
plumbing (`set -euo pipefail` is active: keep the `|| true` guards exactly as above).

**R4 — existing watch-script anchors (quoted @ c85d92cf; the edits graft onto these):**
`spec-cartographer-watch.sh:82` `CARTOGRAPHER_AGENT="$RIG_NAME/codegen-support.cartographer"`;
`:89-95` in-progress check `--metadata-field formula=spec-cartographer`; `:96-102`
open-molecule check `--metadata-field formula=spec-cartographer`; `:155-158`
`gc sling "$RIG_NAME/codegen-support.cartographer" spec-cartographer --formula --json
--var work_order_path=... --var rig_name=...`. The engine stamps `metadata.formula` from
the formula name on molecule/step beads — the same queries work for `wo-router` by
substituting the name.

## Implementation Steps

**Step 0 — Extraction fidelity gate.** Diff-read `spec-cartographer.formula.toml` against
the R2 map. For every KEEP row, locate the current text (line numbers may have drifted —
re-anchor by the quoted phrases) and carry it; for every DROP row, confirm nothing in the
new formula reintroduces it. Record the re-anchored line map in the PR description.

**Step 1 — `formulas/wo-router.formula.toml`, header + description.**

```toml
description = """
Route one rig work order into a whole-WO bead DAG: one work bead per
work order (coarse-split 2-5 only for enumerated sub-WO families), a
per-WO owned convoy with an integration/<wo-id> branch, HOLDING
placeholders for unplanned cross-WO blockers, and depends_on edges from
the work order's EXPLICIT annotations only (Blocked by / Blocks /
Land-together rule / index-family structure). The router does not read
the spec tree and does not decompose: the polecat that claims the work
bead reads the work order and specs itself, in its own worktree.
...
"""
formula = "wo-router"
version = 1

[vars]
[vars.work_order_path]
description = "Path (relative to the rig root) to the target work order file"
required = true

[vars.rig_name]
description = "The rig this planning run belongs to (matches city.toml rig name)"
required = true
```

The description carries, verbatim from the R2 KEEP rows: the structural-convoy-parentage
contract paragraphs (spec-cartographer:31–50 — the packlint-pinned language, restated for
this formula), the HOLDING doctrine (52–68), the completion-first rule + mayor-action
notification/disposition blocks (91–149, with "four step beads"), and TWO new pinned
paragraphs: (a) **authoring-side sizing note** (WS2 risk K3): "Work orders routed through
wo-router are implemented whole by one generator session; WO authors must size accordingly
(a polecat may `gc runtime request-restart` to continue a long build). Rig WOs written for
granular decomposition belong on the legacy spec-cartographer lane — per-rig selection via
the `wo_planning_formula` rig var exists for exactly this."; (b) **explicit-annotations
rule**: "Dependency edges derive ONLY from the work order's `Blocked by:` / `Blocks:` /
`Land-together rule:` lines and index-family structure (`## Sub-work-orders`). The router
never infers dependencies from prose, file overlap, or thematic similarity. Missing formal
annotations are an authoring defect to surface in the mayor mail, not to compensate for."

**Step 2 — the four steps.**

*2.1 `id = "init-run"`* — carry spec-cartographer:163–376 verbatim, with exactly these
substitutions: prose "ten step beads" → "four step beads"; mail-subject prefix strings
keep `[CARTOGRAPHER ACTION REQUIRED]` (the mayor's automation signal is shared — do NOT
mint a new prefix); the run-dir layout, helper copy, redirect, exclude list, `state.env`
fields all byte-identical (shared `.cartographer-runs/` layout, R1).

*2.2 `id = "route-and-order"`, `needs = ["init-run"]`* — the only LLM step. Ordered
content:

1. Load state (`cartographer-load-state.sh` preamble, verbatim).
2. **Read the target work order IN FULL** (`$WO_PATH` in the per-run worktree). Extract:
   the `Blocked by:` / `Blocks:` / `Land-together rule:` annotation lines (WO-ref → file
   basename resolution exactly as spec-cartographer:534–539: match against
   `specs/agent-work-orders/` filenames); the verbatim `## Acceptance Criteria` section;
   an optional declared wave (`wave NN` / `wave: NN` in the preamble) for the optional
   `wave:N` label; family detection per spec-cartographer 5b (filename regex + sibling
   check + `## Sub-work-orders` — sibling FILENAMES only, no sibling reads). If the
   annotations name the rig roadmap spec explicitly, read THAT one file for ordering
   context; read nothing else.
3. **Write the minimal `$RUN_DIR/inventory.json`** feeding the deterministic inventory —
   exactly the keys `cartographer-inventory.sh` consumes: `blocked_by_annotations`,
   `blocks_annotations`, `land_together_set`, `family_members`, `cohort_label` (family >
   land-together precedence, labels per R2), `inline_wo_blockers: []` (dropped by design),
   plus `target_work_order`. Then run `bash .gc/scripts/cartographer-inventory.sh` with
   the same exit-status handling as spec-cartographer:759–856 (success → artifacts exist;
   failure → mayor-action disposition).
4. **Duplicate guard (deterministic, replaces reconcile):** from
   `beads_inventory.json:prior_epoch_beads` — if ANY prior bead for this WO with
   `type=="task"`, status `open` or `in_progress`, and no `placeholder:cross-wo-blocker`
   label exists, this is a re-plan over live work: apply the mayor-action disposition with
   reason "open prior work bead(s) exist for ${WORK_ORDER_ID}; re-plan is a mayor call"
   and exit (the watch never re-slings planned WOs — reaching this means a manual
   re-sling). Closed prior beads: list them in the mayor mail, emit fresh.
5. **Compose the work bead(s).** Default exactly ONE, `draft_id: "work-1"`. Coarse-split
   (2–5 beads, `work-1..work-N`) is permitted ONLY when the target WO itself contains a
   `## Sub-work-orders` section enumerating sub-deliverables that do NOT exist as their
   own sibling `.md` files (an index parent whose children are files gets ONE bead —
   the children are separately watched/routed). When splitting, add `new_to_new` ordering
   edges only if the enumeration states an order. Bead body TEMPLATE (pinned):

   ```
   Implement work order ${WORK_ORDER_ID} END-TO-END.

   Work order file — READ IT IN FULL in your worktree before any code:
   ${WO_PATH}
   Also honor the rig's specs/README.md Operating Rule (foundation
   specs) and AGENTS.md build/validation commands.

   Integration target: integration/${WORK_ORDER_ID}

   Acceptance criteria (verbatim from the work order — the binding bar):
   <the WO's ## Acceptance Criteria section, quoted verbatim>

   done_when: every acceptance criterion above is demonstrably
   satisfied with real command evidence; the repo's declared validation
   battery (AGENTS.md) is green; work is committed on your polecat
   branch and handed off per your done sequence.
   ```

   Title: `Implement ${WORK_ORDER_ID}` (≤70 chars; truncate the stem tail, never the
   prefix). This body deliberately references the WO file (self-containment inversion,
   Non-Goals).
6. **Plan convoy + edges + HOLDINGs** (all rules carried from R2 KEEP rows):
   - Convoy: cohort join if `beads_inventory.json:existing_cohort_convoy_id` non-null
     (Case A — `use_existing_convoy_id`, no new node, no rename); else one new convoy
     (Case B) titled from `convoy_title_override` else `${WORK_ORDER_ID}: <WO H1 title>`,
     with cohort label applied bare when set.
   - `new_to_cross_wo` edges: for each `cross_wo_blockers` entry with open beads — every
     work bead → the blocker's `convoy_bead_id` if open, else each still-open
     implementation task bead (spec-cartographer:1154–1171 semantics).
   - HOLDING create: for each blocker entry with `no_beads_known: true` or scaffold-only
     beads — one HOLDING stub (title/body/labels per the spec-cartographer 3b template),
     ALL work beads as `dependent_draft_ids` (whole-WO granularity: the WO literally
     cannot land before its formal blocker), `hold:<blocker-wo-id>` labels on those beads.
   - `existing_to_new` edges (deterministic Blocks handling): for each
     `downstream_blocks.json` entry with an OPEN `convoy_bead_id` — one edge
     `{from_existing_bead_id: <that convoy id>, to_new: <this run's convoy draft, or the
     single work bead when Case A>}`. Downstream WOs without beads get nothing (their own
     routing run will carry the mirror `Blocked by:`).
   - HOLDING release: for each `prior_holding_stubs` entry — classification is
     deterministically `fully_releasable` with retarget edges from EVERY listed dependent
     to this run's convoy draft (Case B) or single work bead (Case A); the emit script's
     still-open re-check may downgrade to `partial` exactly as spec-cartographer:2321–2324.
7. **Write `$RUN_DIR/route_plan.json`** — single artifact, field names REUSING the
   spec-cartographer `graph.json`/`tasks.json` vocabulary so the shared preflight applies
   unchanged:

   ```json
   {
     "tasks":   [ {"draft_id","title","body","done_when","wave_label": null} ],
     "convoys": [ {"draft_convoy_id","title","intent","member_draft_ids",
                    "use_existing_convoy_id","cohort_label"} ],
     "edges":   { "new_to_new": [], "new_to_cross_wo": [], "existing_to_new": [] },
     "holding_stubs_to_create":  [ <spec-cartographer 5.6 entry shape> ],
     "holding_stubs_to_release": [ <spec-cartographer 5.7 entry shape> ],
     "prior_epoch_note": "<closed prior beads summary or null>"
   }
   ```

   (No `new_to_existing` category and no decision arrays — dropped lanes. Every task is a
   convoy member; the no-orphans rule is structural.)
8. Close + drain-ack (verbatim discipline block).

*2.3 `id = "emit"`, `needs = ["route-and-order"]`* — script-assisted. Step prose = load
state; define `cartographer_block_for_mayor_action` (verbatim helper); run:

```bash
bash .gc/scripts/wo-router-emit.sh 2> "$RUN_DIR/wo_router_emit.stderr"
```

then the exact exit-status handling shape of the `inventory_beads` step
(spec-cartographer:815–856): exit 0 → verify `$RUN_DIR/emitted.json` non-empty → compose
and send the mayor summary mail (compact: WO id, bead ids from `emitted.json`, convoy
id/joined, HOLDING created/released/partial, edges added, prior-epoch note) → close +
drain-ack; exit ≠0 → failure.json + mayor mail + mayor-action disposition, never close.

*2.4 `id = "validate-store"`, `needs = ["emit"]`* — deterministic. Same shape:

```bash
bash .gc/scripts/wo-router-validate-store.sh 2> "$RUN_DIR/wo_router_validate.stderr"
```

exit 0 → close + drain-ack (molecule complete); exit ≠0 → mayor-action disposition with
`$RUN_DIR/validation_store.json` attached.

**Step 3 — `assets/scripts/wo-router-emit.sh`** (new; `#!/usr/bin/env bash`,
`set -euo pipefail`; requires sourced state env — assert `RUN_DIR`/`RIG`/`WORK_ORDER_ID`/
`REPO_ROOT` set, exit 2 otherwise, mirroring `cartographer-emit-plan-preflight.sh:19`).
Deterministic sequence — no judgment, jq/git/gc transport only:

1. Derive compatibility views for the shared helper:
   `jq '{convoys, edges, holding_stubs_to_create, holding_stubs_to_release}'
   route_plan.json > graph.json` and `jq '.tasks' route_plan.json > tasks.json`.
2. Resolve `$SLING_TARGET` via the `gc config show` awk (spec-cartographer:2271–2278,
   verbatim); resolve rig path/beads JSONL (2263–2267).
3. Existing-target still-open re-check + drop-log + partial-HOLDING downgrade
   (2281–2324, verbatim semantics; write `reverse_edge_failures.jsonl` /
   `holding_release_effective.jsonl`).
4. Assemble `$RUN_DIR/emit_plan.json` per the 3a–3d rules (convoy node with
   `owned` label + `metadata.owned="true"` + `metadata.target`; HOLDING nodes with the
   five labels and NO routing metadata; task nodes with body from `tasks.json`, labels
   `source:work-order`/`epoch`/`hold:*`/optional `wave:N`, `metadata."gc.routed_to"` +
   `metadata.target`, `parent_key`/`parent_id`; edges per 3d including HOLDING dependent
   edges and release retargets).
5. `bash .gc/scripts/cartographer-emit-plan-preflight.sh "$RUN_DIR/emit_plan.json"
   "$RUN_DIR/graph.json" "$RUN_DIR/tasks.json"` — any failure: exit non-zero (the step
   prose converts that to mayor-action).
6. Integration-branch creation on origin (2716–2749 verbatim: fetch, `ls-remote` skip,
   `git push origin origin/main:refs/heads/integration/${WORK_ORDER_ID}`).
7. `gc --rig "$RIG" bd create --graph "$RUN_DIR/emit_plan.json" --json >
   "$RUN_DIR/emit_mapping.json"` — non-zero: exit non-zero (transaction rolled back;
   nothing created).
8. HOLDING release `bd batch` (2782–2825 semantics: fully-releasable-only closes; batch
   failure is SOFT — record `emit_batch_failure.json`, continue).
9. Write `$RUN_DIR/emitted.json` (the spec-cartographer step-8 shape, minus the dropped
   categories; include `key_to_id_map`, `routing_target`, convoy created/joined, HOLDING
   arrays, `pre_flight_drops`).

All `gc bd show --json` reads use the `.[0].field` form (packlint
`TestBdShowJqScalarExpect` scans `examples/`).

**Step 4 — `assets/scripts/wo-router-validate-store.sh`** (new; same env preamble).
Deterministic post-mutation checks, writing `$RUN_DIR/validation_store.json`
(`{"status":"ok"|"failed","checks":[...]}`), exit 0 iff all pass:

1. Re-query: `gc --rig "$RIG" bd list --all --label="source:work-order:${WORK_ORDER_ID}"
   --label="epoch:${EPOCH}" --json > "$RUN_DIR/validation_query.json"`; FAIL if the bead
   count ≠ `emit_plan.json` node count (zero-item never green).
2. Parent-linkage materialization: the spec-cartographer validate-store check `a` loop
   (3216–3240) carried verbatim (accept `parent`, `parent_id`, or `parent-child`
   dependency row).
3. Routing: `CARTOGRAPHER_ROUTING_MODE=emission bash
   .gc/scripts/cartographer-validate-emitted-routing.sh "$RUN_DIR/emitted.json"
   "$RUN_DIR/validation_query.json" "$RUN_DIR/validation_store_routing.json"`.
4. Dependencies resolvable: every `emit_plan.json` edge target exists in the store (via
   the mapping for keys; `bd show` for ids), open or convoy (check `b` semantics).
5. HOLDING closes recorded: every closed HOLDING id ∈
   `emitted.json:holding_stubs_released` (check `i` exception rule).
6. HOLDING routing firewall: no bead carrying `placeholder:cross-wo-blocker` has
   `gc.routed_to` metadata.

**Step 5 — watch-order formula selection.** Edit
`assets/scripts/spec-cartographer-watch.sh` (anchors in R4):

1. After `RIGS_JSON=$(gc rig list --json)` add the one-time `CONFIG_JSON` read (R3).
2. Inside the rig loop, before the busy check: resolve `PLANNING_FORMULA` per R3
   (default `spec-cartographer`).
3. Busy check: keep the ready-count probe unchanged; run the in-progress-step AND
   open-molecule queries TWICE each — `--metadata-field formula=spec-cartographer` and
   `--metadata-field formula=wo-router` — and defer if ANY of the four counts (or the
   ready count) is non-zero. Rationale comment to add verbatim: "Both planner formulas
   share the singleton cartographer pool; a busy rig defers regardless of which formula
   is running (duplicate-emit race protection is formula-agnostic)."
4. Sling: `gc sling "$RIG_NAME/codegen-support.cartographer" "$PLANNING_FORMULA"
   --formula --json --var work_order_path="$REL_PATH" --var rig_name="$RIG_NAME"` and log
   the selected formula in the existing `[spec-cartographer-watch] slinging` line.
5. Everything else (planned-WO idempotency guards, exact per-WO label/metadata lookups,
   recency sort, one-sling-per-tick break) stays byte-identical.

Edit `orders/spec-cartographer-watch.toml`: comment + `description` text only — mention
"slings the rig's selected planning formula (`[rigs.formula_vars]
wo_planning_formula`, default spec-cartographer)". `exec`, `trigger = "cooldown"`,
`scope = "city"`, `interval = "60s"`, `timeout = "10m"` unchanged.

Edit `agents/cartographer/agent.toml` `nudge` (line 10) to:
`"Check your hook for a spec-cartographer or wo-router molecule, then execute it."`
No other agent.toml field changes (Non-Goals).

**Step 6 — tests.**

- `test/packlint/wo_router_formula_test.go` — mirrors the
  `spec_cartographer_formula_test.go` normalized-contains style, pinning against
  `formulas/wo-router.formula.toml`: exactly four `[[steps]]` (parse the TOML or count
  `id = "` lines for `init-run|route-and-order|emit|validate-store`); the structural
  convoy-parentage contract phrases; the HOLDING doctrine phrases; the explicit-annotations
  rule sentence ("never infers dependencies from prose"); the sizing note; the
  `.gc/scripts/cartographer-load-state.sh` preamble in every post-init step; the
  `cartographer-emit-plan-preflight.sh` and `wo-router-emit.sh` /
  `wo-router-validate-store.sh` invocations; `gc runtime drain-ack` in every step; the
  ABSENCE of dropped-lane markers (`new_to_existing_decisions`,
  `existing_to_new_decisions`, `key_excerpts`, `inline_wo_blockers` as a mined input —
  assert the formula text does not contain them).
- `test/packlint/wo_router_watch_test.go` — extends the existing bash harness pattern
  (`spec_cartographer_watch_test.go`): fake `gc` gains a `config show --json` arm
  returning `{"config":{"Rigs":[{"Name":"Rig","FormulaVars":{"wo_planning_formula":"wo-router"}}]}}`
  (real observed shape per the R3 STOP-gate — regenerate the fixture from a real
  `gc config show --json` run before pinning); the `sling` arm records its argv. Cases:
  (a) var set → sling called with `wo-router`; (b) var absent → `spec-cartographer`;
  (c) `config show` returning nothing → `spec-cartographer` (fallback, no crash);
  (d) busy via an open `wo-router` molecule (`--metadata-field formula=wo-router` arm
  returns one row) → no sling (formula-agnostic serialization).
- Existing `spec_cartographer_watch_test.go` cases stay green: their fake `gc` gets the
  minimal `config show` no-op arm added (returns empty ⇒ legacy default path — behavior
  identical to today). Existing `spec_cartographer_formula_test.go` untouched and green.
- Script self-checks: `wo-router-emit.sh` and `wo-router-validate-store.sh` refuse to run
  without state env (exit 2 + message) — covered by one negative packlint bash case each
  (planted RED: run with env unset, assert non-zero + message).
- Formula loads: extend the packlint package with a compile smoke — `gc formula show
  wo-router` class validation if a harness hook exists; otherwise assert the TOML parses
  via `github.com/BurntSushi/toml` decode in the Go test (both vars declared `required`).

**Step 7 — full battery** (Validation) + PR.

## Git Workflow

Loop execution: branch `wo/GCD-WO-CSC-004-wo-router-formula` (or `polecat/$BEAD_ID` under
city execution) in GasCity-Dev; PR to `origin/main` on **`mike-matchpoint/gascity`** (the
fork is the estate's home — never PR upstream to `gastownhall/gascity`); never commit
directly to `main`; per `CONTRIBUTING.md` run `make setup` once (installs
`.githooks/pre-commit`). One PR for the whole WO. No force-push, no history rewrites.

## Test Coverage

Every acceptance criterion below names its backing test; kit K6 discipline verbatim
(fixture-realism REJECT-level; zero-item runs never green — every grep/count assertion
pins its expected non-zero count; planted-RED cases included).

- **Packlint tier** (`test/packlint/wo_router_formula_test.go`,
  `wo_router_watch_test.go`): formula structure pins, watch selection matrix (4 cases),
  busy-serialization case, fallback case, script env-guard negatives.
- **Go structural tier:** `go test ./internal/builtinpacks` green (embedded tree
  round-trips with the new files — no Go edits, content hash is computed);
  `GC_FAST_UNIT=1 go test ./test/packlint` green including both existing cartographer
  suites (no-regression bar).
- **Repo battery:** `make build && make check` (= `fmt-check lint vet
  check-routed-test-rows test`); `make test-packs` (unchanged suites still green — this
  WO adds no pack-script suite; its scripts are exercised via packlint bash harnesses).

## Validation

- `make build && make check` green; `GC_FAST_UNIT=1 go test ./test/packlint` green
  (old + new); `go test ./internal/builtinpacks` green.
- `git diff --stat` shows `formulas/spec-cartographer.formula.toml` UNCHANGED (the
  retention bar is literal).
- R3 STOP-gate evidence recorded: the observed `gc config show --json` rig/var key path
  (command transcript) matches the jq path shipped in the watch script and the test
  fixture.
- Grep gates (run + record): `grep -rn "matchpoint\|Matchpoint" <all files added/edited
  by this WO>` → empty; `grep -c 'id = "' formulas/wo-router.formula.toml` → 4;
  `grep -n "wo_planning_formula" assets/scripts/spec-cartographer-watch.sh` → present
  (GCD-WO-CSC-006's R1d discovery greps exactly this).
- **Cities-PAUSED clause:** all GasCity-in-AWS remains paused (zero-replica / suspended);
  this WO performed no cluster/AWS/city interaction (offline build + tests only); no live
  drill claimed. Live router behavior on the vehicle-graph pilot is a NAMED FOLLOW-UP
  gated on un-pause via GCD-WO-CSC-006's binding — never an acceptance criterion here.
- **Prod-gate defer:** nothing prod-shaped is created (pack content only) — recorded for
  completeness per the standing policy.
- Evidence discipline: PR carries `{command, output_excerpt}` pairs for every battery run.

## Acceptance Criteria

1. `formulas/wo-router.formula.toml` exists with exactly the 4 pinned steps, both required
   vars, and every R2 KEEP seam present (convoy parentage, HOLDING doctrine,
   mayor-action disposition, load-state preamble, close+drain-ack per step) —
   `test/packlint/wo_router_formula_test.go`.
2. No dropped-lane text in the router (decision arrays, key_excerpts, inline-blocker
   mining, forbidden-phrase scan) — same test, absence assertions.
3. `wo-router-emit.sh` assembles the emit carrier, runs the SHARED preflight, creates
   `integration/<wo-id>` on origin before `bd create --graph`, executes both transactions,
   and writes `emitted.json`; `wo-router-validate-store.sh` proves parent linkage,
   routing, dep resolvability, and HOLDING-close bookkeeping — script env-guard packlint
   cases + formula-test invocation pins (full behavioral proof of the mutation path is
   the un-pause pilot follow-up; the deterministic scripts are line-covered by the
   harness cases).
4. Watch script slings the per-rig selected formula with legacy default and
   formula-agnostic busy serialization; order TOML text updated; cartographer nudge names
   both formulas — `test/packlint/wo_router_watch_test.go` (4-case matrix + busy case)
   + existing watch suites green.
5. `spec-cartographer.formula.toml` byte-identical; both pre-existing cartographer
   packlint suites pass unmodified — CI diff check + `go test ./test/packlint`.
6. Full repo battery green (`make build && make check`, `test-packs`,
   `internal/builtinpacks`) — Validation transcripts.
7. No city started, no AWS/cluster call, no live drill claimed (cities PAUSED) — PR
   observation record.

## Risks

- **Config-JSON shape drift** (R3): the jq path rests on Go-field-name marshaling.
  Mitigated by the STOP-gate observation run + fixture generated from real output; the
  fallback default means a drift regression degrades to legacy behavior, never a crash.
- **Two planners, one run-dir layout:** shared `.cartographer-runs/` is deliberate
  (helper reuse) and safe because the watch busy-gate + singleton agent serialize ALL
  planning per rig across both formulas (Step 5.3). Do not weaken either serialization.
- **Whole-WO bead sizing** (WS2 K3): mitigated by the description's authoring note, the
  coarse-split rule, `gc runtime request-restart`, and per-rig opt-in (legacy lane keeps
  granular decomposition where a rig needs it).
- **Emit determinism gap:** if `route_plan.json` under-specifies a case the emit script
  hits (e.g. a release retarget against a Case-A run with a split), the script exits
  non-zero into mayor-action — fail-loud is the designed posture; do not add silent
  fallbacks.
- **Duplicate-emit race** (documented upstream): unchanged protections —
  `max_active_sessions=1` + busy-gate + planned-WO guards. The router's single
  `bd create --graph` transaction narrows the window vs the legacy 10-step run but the
  caps stay.
- **Packlint bash harness portability** (stat -c vs -f already handled in the watch
  script): keep new harness fixtures using the same portable forms.

## Done Means

- [ ] `wo-router.formula.toml` + `wo-router-emit.sh` + `wo-router-validate-store.sh`
      landed in codegen-support; spec-cartographer untouched.
- [ ] Watch script + order TOML + cartographer nudge updated; `wo_planning_formula`
      read with legacy default and dual-formula busy gate.
- [ ] Both new packlint suites green; all pre-existing suites green unmodified;
      R3 STOP-gate evidence in the PR.
- [ ] `make build && make check` + `make test-packs` + `go test ./internal/builtinpacks`
      green.
- [ ] Merged to `origin/main` on `mike-matchpoint/gascity` via PR from a `wo/`-class
      branch; nothing committed directly to `main`.
- [ ] No city started; live pilot run recorded as the named un-pause follow-up (via
      GCD-WO-CSC-006).

## Master cutover contribution

None — platform-fork pack content; no AWS resources created, renamed, or deleted; no CDK
identity surface (kit K1 prod-gate language not triggered). Runtime exposure reaches
hosted cities only via city-repo binding (GCD-WO-CSC-006/007 set the rig var) plus the
AGC-WO-CSC-006A/B image/deploy lane at un-pause, each under its own gates.
