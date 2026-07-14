# Work Order: GCD-WO-CSC-006 — city-pack binding PILOT (vehicle-graph-city): evaluator/judge/router/doctrine binding + the reusable binding template

NOTE: this WO is long — read the FULL file at
`specs/agent-work-orders/GCD-WO-CSC-006-city-pack-binding-pilot.md` in your worktree before
implementing; the **Binding template** section at the bottom is NORMATIVE for the fan-out WO
(`GCD-WO-CSC-007`) and must be kept mechanically applicable.

Execution classification: Dev-only city-source configuration (TOML patches + template
fragments + repo-native tests in a deployed-city SOURCE repo; no AWS mutation, no deploy
surface, no city runs). `boundary=dev`, **wave 24** (CSC program band 23/24/25), `blocked_by`
`GasCity-Dev::GCD-WO-CSC-003-evaluator-judge-primitives`,
`GasCity-Dev::GCD-WO-CSC-004-wo-router-formula`, and
`GasCity-Dev::GCD-WO-CSC-005-polecat-diligence-refinery-merge-only` (all wave 23 —
cross-wave edges, parser-safe; pinned in harness DEPS regardless).
Multi-repo unit — co_repos (object-form, for the wiring entry):
`{"repo": "vehicle-graph-city", "role": "edit", "test": true}`. Home repo (this file, spec +
GasCity-Dev-side nothing else) = `GasCity-Dev`; ALL code/config edits land in the
`vehicle-graph-city` co_repo worktree.

> **Provenance (binding):** CSC program contract authority
> `master/city-scaling-improvements/wo-authoring-kit.md` — **C11** (this WO IS the C11
> authority: "City binding template … proven on vehicle-graph-city"), **C9** (verdict
> metadata + models — authority `GCD-WO-CSC-003`, imported here), **C10** (router output +
> watch-var — authority `GCD-WO-CSC-004`, imported here), **ADDENDUM A1 §2** (overseer-marker
> fragment + `overseer_issue_id`), **A1 §11** (credential pre-stage: "claude-evaluator/
> claude-judge k8s secret projections — pattern per codex-cartographer; GCD-WO-CSC-006
> documents, operator applies (owner punch list)"). Backlog + sequencing:
> `master/city-scaling-improvements/gap-analysis-and-build-plan.md` §5 row 14, §6
> ("City binding patch template | GCD-WO-CSC-006 | GCD-WO-CSC-007"). Design record:
> `aws-GasCity/docs/investigations/2026-07-city-scaling-improvements.md` WS2 (owner ruling
> **D10**, 2026-07-08; landing map "Each `*-code-generation-city` repo"). Process:
> root `SKILL-work-order-audit-and-authoring.md` §1.1–1.5.
> Verified at authoring (2026-07-08): `GasCity-Dev` `origin/main` @
> `c85d92cf0cfd1215be1467628d6fd2e06db46aae`; `vehicle-graph-city` `origin/main` @
> `71ee67eca00c7d1f8a2891896a5dc6eccda279af`. Re-verify both at execution time — by then
> wave 23 (GCD-WO-CSC-001..005) is merged and this WO binds against the MERGED content,
> never against this file's illustrative expectations.
> Ledger stem: `GasCity-Dev::GCD-WO-CSC-006-city-pack-binding-pilot`.

## Goal

`vehicle-graph-city` (the deployed vehicle-graph code-generation city SOURCE repo) is bound
to the WS2 agent pipeline delivered upstream in wave 23, entirely via the city's existing
patch-based injection surface (`city.toml` / `pack.toml` / `template-fragments/` — never a
pack fork). Clean end state, all in the `vehicle-graph-city` co_repo:

1. **Evaluator + judge bound per rig**: `[[rigs.patches]]` entries for the new
   codegen-support `evaluator` and `judge` agents — `provider = "claude-evaluator"` /
   `"claude-judge"`, `model = "opus"`, `effort = "high"` via `option_defaults`, pool sizes
   `max_active_sessions = 4` (evaluator) / `2` (judge) (kit C9/C11 pins).
2. **Dedicated claude credential profiles**: `[providers.claude-evaluator]` and
   `[providers.claude-judge]` with k8s credential projections mirroring the deployed
   `codex-cartographer` pattern (quoted verbatim below). Provisioning is DOCUMENTED (runbook
   + owner punch list); the operator applies the secrets via aws-GasCity — no secret value
   ever appears in this repo (A1 §11).
3. **Polecat diligence + overseer-marker fragments appended**: the polecat patch's
   `append_fragments` gains the GCD-WO-CSC-005 R2 six: `polecat-code-hygiene`,
   `polecat-evidence-contract`, `polecat-final-rebase-revalidate`,
   `polecat-autonomy-and-blockers`, `polecat-submit-to-evaluator`,
   `polecat-overseer-issue-marker` (6 total: 5 diligence incl. the submit-to-evaluator
   override, + the overseer marker; A1 §2), preserving the 6 existing fragment names.
4. **Refinery gate flipped + router selected**: `[rigs.formula_vars]` gains
   `evaluator_gated = "true"` (C9; refinery becomes serialized merge fan-in — convoy
   autoland full battery retained upstream) and the GCD-WO-CSC-004 watch formula-name var
   selecting the `wo-router` formula for this rig (C10).
5. **City doctrine fragments authored** in the city pack's own `template-fragments/`:
   `city-architecture-standards`, `city-evidence-doctrine`, `city-invariants` (C11 names,
   fixed). This is the ONLY home for MatchPoint-specific prompt content — fixture-realism
   doctrine text, estate architecture standards, VG domain invariants live HERE, never in
   upstream packs (D10 placement principle; upstream generic-ness is GCD-WO-CSC-003's
   REJECT-level grep gate).
6. **City tests** under `tests/` following the existing `tests/cartographer-*.sh` pattern
   (standalone bash, `set -euo pipefail`, assertions against the materialized
   `.gc/system/packs/` + the city TOML), proving every binding above structurally.
7. **The Binding template section of this WO is the reusable, parameterized procedure**
   `GCD-WO-CSC-007` applies mechanically to the 5 fan-out cities.

Business reason: WS2/D10 ports the loop harness's generator→evaluator→judge diligence into
GasCity code-generation cities. Wave 23 delivered the generic primitives (upstream,
domain-agnostic); nothing runs them until a city binds providers, models, pools, doctrine,
and routing. Vehicle-graph is the ratified pilot city (standing pilot-city policy).

## Dependencies

- **Blocked by (all wave 23, merged before this WO dispatches):**
  - `GasCity-Dev::GCD-WO-CSC-003-evaluator-judge-primitives` — authors the
    `codegen-support` `evaluator`/`judge` agents, the C9 verdict-metadata contract
    (`eval_verdict`, `eval_evidence`, `verdict_patch_id`, `eval_reject_count`,
    `judge_verdict`, `rejection_reason`, `decision_state`, `residue`, + A1 §2
    `overseer_issue_id`), the
    `gc.kind=eval_request|judge_request` routing selectors, the three city injection-seam
    fragment names, and the documented city-binding mechanism. THIS WO IMPORTS ALL OF THAT —
    it re-declares nothing.
  - `GasCity-Dev::GCD-WO-CSC-004-wo-router-formula` — authors the 4-step `wo-router`
    formula + the `spec-cartographer-watch` formula-name var (C10). This WO only SETS the
    var per rig.
  - `GasCity-Dev::GCD-WO-CSC-005-polecat-diligence-refinery-merge-only` — authors the R2
    six fragments `polecat-code-hygiene`, `polecat-evidence-contract`,
    `polecat-final-rebase-revalidate`, `polecat-autonomy-and-blockers`,
    `polecat-submit-to-evaluator`, `polecat-overseer-issue-marker` (6 total: 5 diligence
    incl. the submit-to-evaluator override, + the overseer marker; A1 §2) and the
    `evaluator_gated` refinery branches (default `"false"` upstream). This WO only
    APPENDS the fragment names and sets the var.
- **Consumed by:** `GasCity-Dev::GCD-WO-CSC-007-city-pack-binding-fanout` (wave 25) —
  applies the Binding template to the 5 remaining code-generation cities. Keep the template
  section mechanical: every step parameterized, no vehicle-graph-only reasoning buried in
  prose.
- **Discovery-first rule (REJECT-level):** every upstream NAME this WO binds (agent names,
  fragment names, formula name, watch var name, seam mechanism) MUST be read from the
  MERGED wave-23 content in this repo's worktree at execution time (Required Inputs gives
  the exact discovery commands). The expectations written in this file are for
  verification; on any mismatch, the merged content wins. If an expected deliverable is
  ABSENT from the merged tree (e.g. no `agents/evaluator/`), STOP and raise a structured
  blocker — do not improvise a binding.
- **Cities PAUSED (standing policy + kit K1):** this WO verifies all GasCity-in-AWS remains
  paused (zero-replica / suspended) before declaring success — for this WO that means: no
  hosted interaction of any kind is performed (no kubectl, no AWS API, no `gc` daemon
  start, no city/session/supervisor launch, locally or hosted); live drills are only ever
  the vehicle-graph pilot, explicitly named, re-suspend after — **this WO names NO live
  drill**; runtime exposure of this binding arrives via source-sync + the AGC deploy WOs
  at un-pause, under their own gates.
- **Fixture-realism doctrine** (owner-ratified, REJECT-level):
  `master/DOCTRINE-fixture-realism-and-lifecycle-seam-acceptance.md` binds the test
  discipline here (structural tests must FAIL when the asserted binding is absent —
  zero-item runs never green) AND is a content SOURCE for the `city-evidence-doctrine`
  fragment (step 5).

## Non-Goals

Bounded-context REJECT rules (kit K2, `*-code-generation-city` row) restated:

- **NO forking imported pack files.** The gastown/codegen-support packs are imported via
  `.gc/system/packs/<name>`; behavior is tuned ONLY through `[[rigs.patches]]` /
  `[[patches.agent]]` / `append_fragments` / `[rigs.formula_vars]` / `[providers.*]` and
  city-pack fragments. Copying an upstream agent/formula/fragment into this repo to edit it
  is a REJECT.
- **NO hand-editing `.gc/system/packs` mirrors.** `.gc/` is gitignored runtime state here
  (`.gitignore:2`); the mirrors are materialized by the `gc` binary. Tests READ them; nothing
  writes them.
- **NO upstream (GasCity-Dev pack) edits in this WO.** If a wave-23 deliverable is wrong or
  missing, that is a structured blocker against the upstream WO — not a city-side patch-over
  and not a quick fix in `examples/gastown/packs/`.
- **NO MatchPoint literals upstream / NO generic content city-side duplication.** MatchPoint
  doctrine text goes ONLY in this city's `template-fragments/`; conversely, do not re-state
  the generic evaluator/judge/polecat prompt bodies here — the city fragments carry ONLY the
  city-specific doctrine content.
- **NO exec-monitoring-city changes** (`vehicle-graph-execution-monitoring-city` untouched —
  D5: zero changes; the sole exec-city change in the program is GCD-WO-CSC-008, not this WO).
- **NO other city repos** — the 5 fan-out cities are `GCD-WO-CSC-007`; the 4 superseded
  stubs (`billing-`, `client-identity-`, `client-portal-`,
  `compatibility-view-code-generation-city`) are DEAD — never patch (A1 §8).
- **NO secret values, tokens, or credentials committed anywhere** — the WO ships the
  PROJECTION (TOML) + the provisioning RUNBOOK; the k8s Secrets themselves are operator
  actions via aws-GasCity (A1 §11 owner punch list). Acceptance must NOT require the
  secrets to exist.
- **NO aws-GasCity edits** (render/deploy is AGC-WO-CSC-006A/B scope; the binding travels
  to the hosted city via source-sync, not the runtime image — A1 §1).
- **NO tuning of upstream defaults not named here** — `max_eval_rejects` (default 2, C9),
  retry semantics (resume-and-fix, D10/K6), refinery convoy-autoland full battery, and the
  `regenerate_on_reject` reserved var all keep their upstream defaults; the city sets ONLY
  the vars this WO enumerates.
- **NO changes to existing bindings beyond the enumerated additions** — the cartographer
  patch, the existing 6 polecat fragments, the 10 refinery fragments, the refinery patch,
  `integration_branch_auto_land = "true"`, the mayor/dog/debugger `pack.toml` patches, and
  `[daemon]` all remain byte-identical (spec-cartographer is RETAINED during migration per
  C10 — the router is selected via the watch var, the legacy formula stays available).
- **NO telos pack forks / NO telos law copies in the city** — telos content in the city is
  limited to the P1.5 delivery artifacts (the sha-pinned SYSTEM-TELOS snapshot + the
  telos-binding fragment); no evaluator/judge fragment delegates verdicts to a telos role
  (D6 v2 — see the "Telos pack topology" tail section, which also binds the Binding
  template below).

## Architecture Links

- `master/city-scaling-improvements/wo-authoring-kit.md` — K2 (bounded context), K4
  C9/C10/C11, K6 (test discipline), A1 §2/§8/§11 (estate authority; not in this repo — the
  load-bearing pins are quoted in this file).
- `master/city-scaling-improvements/gap-analysis-and-build-plan.md` §5 rows 14–15, §6.
- `aws-GasCity/docs/investigations/2026-07-city-scaling-improvements.md` — WS2 "Pack model —
  confirmed understanding" + "Proposed design" (ratified D10) + landing map.
- `vehicle-graph-city/runbooks/hosted-city-agent-editing-process.md` — the hosted-city
  source-edit process this repo's CLAUDE.md/AGENTS.md binds all edits to.
- GasCity-Dev (this repo): `docs/guides/shareable-packs.md` ("the importing pack wins over
  its imports", :234-235); `internal/config/revision.go:387` (`template-fragments` is a
  convention-discovery dir — no registration key needed); `cmd/gc/cmd_lint.go:53`
  (`gc lint <pack>` — "Validate a pack before merge", incl. fragment-reference checks);
  `cmd/gc/cmd_config.go:30` + `cmd/gc/embed_builtin_packs.go:48-54` (`gc config show
  --validate` materializes builtin packs to `.gc/system/packs/` then validates the composed
  config — the mechanism the city tests use to obtain the mirrors).
- `master/DOCTRINE-fixture-realism-and-lifecycle-seam-acceptance.md` — content source for
  `city-evidence-doctrine` + the test-discipline bar.
- `matchpoint-loop-harness/mlh/prompts/{implementer,evaluator,stop_judge}.md` (estate code
  root) — the diligence-source prompts WS2 ported; content DERIVATION source for the city
  doctrine fragments (the generic parts already went upstream via wave 23; only
  MatchPoint-specific residue is drawn from here).

## Packages To Inspect

All READ-first; paths are worktree-relative unless prefixed.

In the HOME worktree (`GasCity-Dev`, post-wave-23 `origin/main`):

- `examples/gastown/packs/codegen-support/agents/evaluator/` + `agents/judge/`
  (`agent.toml` + `prompt.template.md`) — delivered by GCD-WO-CSC-003: agent names, scope,
  `work_selector` metadata, default pool sizes, and the prompt's city injection seams.
- `examples/gastown/packs/codegen-support/template-fragments/` — the full fragment
  namespace: the pre-existing set (`polecat-decide-and-act`, `polecat-handoff-override`,
  `polecat-done-target-override`, `polecat-architectural-doc-sync`,
  `polecat-validate-before-commit`, `polecat-bug-filing`, `mayor-cartographer-protocol`,
  10 × `refinery-*`, 4 × `debugger-*`, 4 × `landing-arbiter-*`) PLUS the wave-23 additions
  from GCD-WO-CSC-005 R2 — `polecat-code-hygiene`, `polecat-evidence-contract`,
  `polecat-final-rebase-revalidate`, `polecat-autonomy-and-blockers`,
  `polecat-submit-to-evaluator`, `polecat-overseer-issue-marker` (6 total: 5 diligence
  incl. the submit-to-evaluator override, + the overseer marker) — and
  any empty-default city-seam defines from GCD-WO-CSC-003. Fragment file format:
  `<name>.template.md` opening with `{{ define "<name>" }}` (precedent:
  `polecat-validate-before-commit.template.md:1`).
- `examples/gastown/packs/codegen-support/formulas/` — `wo-router` formula (GCD-WO-CSC-004)
  + `mol-evaluate-task` / `mol-judge-task` class formulas (GCD-WO-CSC-003).
- `examples/gastown/packs/gastown/formulas/mol-refinery-patrol.toml` — the
  `evaluator_gated` branches (GCD-WO-CSC-005). NOTE the home: this formula lives in the
  GASTOWN pack, not codegen-support — the `evaluator_gated` var this WO sets in
  `[rigs.formula_vars]` (Step 4) is consumed there.
- `examples/gastown/packs/codegen-support/assets/scripts/spec-cartographer-watch.sh` +
  `orders/spec-cartographer-watch.toml` — the watch order (exec-based, cooldown trigger);
  GCD-WO-CSC-004 adds the formula-name var this WO sets.
- `Makefile` `build` target (line ~27) — builds the `gc` binary the city tests use.
- `internal/runtime/k8s/runtime_identity.go:231-260` + `internal/runtime/k8s/pod.go:33`
  (`claudeSecretName = "claude-credentials"`), `pod.go:483,731` (`CLAUDE_CONFIG_DIR`
  handling) — how provider credential profiles project into hosted pods (context for the
  runbook; NOT edited).

In the CO_REPO worktree (`vehicle-graph-city`):

- `city.toml` — full file; the verbatim anchors this WO patches around are quoted in
  Required Inputs.
- `pack.toml` — schema-2 city pack (`name = "vehicle-graph-city"`), mayor/dog/debugger
  patches. UNCHANGED by this WO (evaluator/judge are rig-scope bindings → `city.toml`; see
  Required Inputs R1 scope check).
- `template-fragments/` — DOES NOT EXIST yet in this repo; step 5 creates it (the city repo
  is itself a schema-2 pack; `template-fragments/` is convention-discovered).
- `tests/cartographer-load-state.sh`, `tests/cartographer-validate-emitted-routing.sh`,
  `tests/spec-cartographer-watch-busy-predicate.sh` — the city test pattern to follow:
  standalone `#!/usr/bin/env bash` + `set -euo pipefail`, `ROOT=$(cd "$(dirname
  "${BASH_SOURCE[0]}")/.." && pwd)`, `TMP=$(mktemp -d)` + trap cleanup, direct `grep -q`
  assertions against `$ROOT/.gc/system/packs/...` and city files, fake state under `$TMP`.
- `runbooks/` — home for the credentials runbook (step 6).
- `AGENTS.md` / `CLAUDE.md` — the hosted-city editing process gate.

## Required Inputs

**R0 — verbatim anchors (the exact existing patch shapes this WO extends).** Quoted from
`vehicle-graph-city/city.toml` @ `71ee67ec` so the generator can locate and preserve them;
re-verify against the worktree:

`city.toml:18-38` — the codex-cartographer provider profile (THE pattern the new claude
profiles mirror):

```toml
[providers.codex-cartographer]
base = "builtin:codex"

[providers.codex-cartographer.k8s_credentials]
name = "codex-cartographer"
secret_name = "codex-cartographer"
target_dir = ".codex-cartographer"
optional = false

[providers.codex-cartographer.k8s_credentials.env]
CODEX_HOME = "{{.TargetDir}}"
GASCITY_PROVIDER_PROFILE = "codex-cartographer"

# Cartographer also carries Claude credentials — the same account the mayor
# uses — projected from the city claude-credentials secret as
# CLAUDE_CODE_OAUTH_TOKEN (owner directive, 2026-07-02).
[[providers.codex-cartographer.k8s_credentials.env_from_secret]]
name = "CLAUDE_CODE_OAUTH_TOKEN"
secret_name = "claude-credentials"
key = "CLAUDE_CODE_OAUTH_TOKEN"
```

`city.toml:70-96` — the existing rig patch block for rig `Matchpoint-Vehicle-Graph`
(prefix `vg`; the region the new patches join, and whose existing entries stay
byte-identical):

```toml
[[rigs.patches]]
agent = "cartographer"
# The upstream codegen-support pack is provider-neutral. This hosted AWS city
# uses a dedicated Codex credential projection for cartographer sessions.
provider = "codex-cartographer"

[[rigs.patches]]
agent = "polecat"
provider = "codex-polecat"
append_fragments = ["polecat-decide-and-act", "polecat-handoff-override", "polecat-done-target-override", "polecat-architectural-doc-sync", "polecat-validate-before-commit", "polecat-bug-filing"]
max_active_sessions = 20
[rigs.patches.option_defaults]
effort = "high"
model = "gpt-5.5"

[[rigs.patches]]
agent = "refinery"
provider = "claude"
append_fragments = ["refinery-rebase-integration-bootstrap", "refinery-arbiter-decision-consumer", "refinery-cached-resolution-replay", "refinery-rebase-conflict-auto-resolve", "refinery-landing-failure-arbiter", "refinery-patrol-loop-discipline", "refinery-merge-close-contract", "refinery-wisp-pour-vars-override", "refinery-owned-convoy-autoland-handoff", "refinery-bug-filing"]

[rigs.patches.option_defaults]
model = "opus"
effort = "high"

[rigs.formula_vars]
integration_branch_auto_land = "true"
```

The `option_defaults model = "opus" / effort = "high"` on the refinery patch is the
in-repo precedent that the claude provider threads `effort` through `option_defaults`
(WS2 risk K5 — resolved by precedent; no new plumbing).

**R1 — discovery commands (run in the HOME worktree at execution; record outputs in
evidence; each has a STOP-gate):**

| # | What | Command (home worktree root) | Expected (verify, don't trust) | STOP if |
|---|---|---|---|---|
| R1a | Evaluator/judge agent names + scope | `cat examples/gastown/packs/codegen-support/agents/evaluator/agent.toml agents/judge/agent.toml` | agents named `evaluator` / `judge`, rig-scope pool agents (patched per rig via `[[rigs.patches]]`, like polecat); selectors on `gc.kind = eval_request` / `judge_request`; default pools 4 / 2 | agents absent, or scope = city (then bind via `[[patches.agent]]` in `pack.toml` instead — mirror the scope the merged agent.toml declares, and say so in the PR) |
| R1b | City-binding mechanism + seam fragment names | GCD-WO-CSC-003's pack README/binding doc + `grep -rn "city-architecture-standards\|city-evidence-doctrine\|city-invariants" examples/gastown/packs/codegen-support/` | append_fragments-based city content over empty-default upstream defines; seam names exactly the C11 three | seam names differ from C11 (bind the MERGED names; flag the kit drift in the PR) |
| R1c | New polecat fragment names | `ls examples/gastown/packs/codegen-support/template-fragments/ \| grep polecat` (plain fixed-string pattern — ALL six new GCD-WO-CSC-005 names start with `polecat-`, including `polecat-overseer-issue-marker`; do NOT copy an escaped alternation into `grep -E` — an escaped pipe in ERE matches literally and finds nothing), then set-difference against the 6 pre-existing `polecat-*` names quoted in R0 | SIX new names total (kit C11: "the 5 new + overseer-marker"): the 5 D10 diligence fragments — code hygiene (incl. fabricated-evidence ban), evidence contract, final rebase+revalidate, autonomy-and-blockers, submit-to-evaluator done-sequence override — plus the overseer-issue-marker fragment (A1 §2); exact names as merged | fewer than the GCD-WO-CSC-005 deliverable set present, or the done-sequence override fails the Step-3 supersession gate |
| R1d | Router formula name + watch var name | `ls examples/gastown/packs/codegen-support/formulas/ \| grep -i router` + `grep -n "formula" examples/gastown/packs/codegen-support/assets/scripts/spec-cartographer-watch.sh` | a `wo-router` formula; the watch script reads the per-rig `[rigs.formula_vars]` var **`wo_planning_formula`** (GCD-WO-CSC-004's pinned name — the discovery grep pattern `formula` matches it as a substring; verify the merged spelling, don't trust this file) with legacy `spec-cartographer` default | no var exists (STOP — the C10 seam is missing upstream) |
| R1e | Verdict metadata keys (context for doctrine fragments + runbook, NOT re-declared) | kit C9 + GCD-WO-CSC-003 merged docs | `eval_verdict, eval_evidence, verdict_patch_id, eval_reject_count, judge_verdict (PASS\|REJECT\|NOT_REQUIRED), rejection_reason, decision_state, overseer_issue_id, residue` | — (import citation only) |

**R2 — doctrine content sources** (for step 5; all at the estate code root, siblings of
this repo): `master/DOCTRINE-fixture-realism-and-lifecycle-seam-acceptance.md` (D1 fixture
realism, D2 live-proofs-traverse-real-lanes, zero-item-never-green);
`matchpoint-loop-harness/mlh/prompts/evaluator.md` + `implementer.md` (MatchPoint-specific
evidence/architecture residue ONLY — the generic mechanisms are already upstream);
`Matchpoint-Vehicle-Graph/specs/` + `AGENTS.md` (VG domain invariants; incl. the standing
guardrail *no tenant_id on Neptune* and the repo's naming/test conventions).

## Implementation Steps

All edits in the `vehicle-graph-city` co_repo worktree. Follow the hosted-city agent
editing process (`runbooks/hosted-city-agent-editing-process.md`) — durable source changes
only, no runtime state committed.

**Step 0 — Discovery gate.** Run R1a–R1d; write the resolved name table (agent names, 3
seam fragment names, N new polecat fragment names, router formula name, watch var name) to
the PR description AND to `tests/csc-resolved-names.env` (a sourceable
`KEY="value"` file the tests read, so tests and TOML can never drift apart). STOP on any
R1 gate failure.

**Step 1 — Claude provider profiles (`city.toml`).** Insert after the
`[providers.codex-cartographer]` block (R0), before `[providers.codex-debugger]`:

```toml
# --- CSC evaluator/judge provider profiles (GCD-WO-CSC-006) ---
# Pattern: the codex-cartographer projection above. Dedicated per-role
# secrets isolate rate limits/accounts exactly like the codex-* trio.
# The k8s Secrets (claude-evaluator / claude-judge, key
# CLAUDE_CODE_OAUTH_TOKEN) are provisioned by the aws-GasCity operator —
# see runbooks/claude-evaluator-judge-credentials.md (owner punch list,
# kit A1 §11). No secret value lives in this repo.

[providers.claude-evaluator]
base = "builtin:claude"

[providers.claude-evaluator.k8s_credentials]
name = "claude-evaluator"
secret_name = "claude-evaluator"
target_dir = ".claude-evaluator"
optional = false

[providers.claude-evaluator.k8s_credentials.env]
GASCITY_PROVIDER_PROFILE = "claude-evaluator"

[[providers.claude-evaluator.k8s_credentials.env_from_secret]]
name = "CLAUDE_CODE_OAUTH_TOKEN"
secret_name = "claude-evaluator"
key = "CLAUDE_CODE_OAUTH_TOKEN"

[providers.claude-judge]
base = "builtin:claude"

[providers.claude-judge.k8s_credentials]
name = "claude-judge"
secret_name = "claude-judge"
target_dir = ".claude-judge"
optional = false

[providers.claude-judge.k8s_credentials.env]
GASCITY_PROVIDER_PROFILE = "claude-judge"

[[providers.claude-judge.k8s_credentials.env_from_secret]]
name = "CLAUDE_CODE_OAUTH_TOKEN"
secret_name = "claude-judge"
key = "CLAUDE_CODE_OAUTH_TOKEN"
```

Notes (verify at execution, adjust ONLY per merged upstream docs): `base =
"builtin:claude"` is a supported custom-provider base
(`internal/config/resolve.go:529,695`). The k8s runtime sets a default
`CLAUDE_CONFIG_DIR` (`pod.go:731`); do NOT add a per-profile `CLAUDE_CONFIG_DIR` unless
GCD-WO-CSC-003's merged binding doc calls for it — if it does, mirror `CODEX_HOME`'s
`"{{.TargetDir}}"` form.

**Step 2 — Evaluator/judge rig patches (`city.toml`).** Append to the
`Matchpoint-Vehicle-Graph` rig's patch list, after the refinery patch block (R0),
substituting the three seam fragment names resolved in Step 0 (expected exactly as
written — C11 pins them):

```toml
# --- CSC per-task evaluator/judge binding (GCD-WO-CSC-006; kit C9/C11) ---
# Claude Opus, effort=high (precedent: the refinery patch above). City
# doctrine fragments append MatchPoint-specific standards to the generic
# upstream prompts — that content lives ONLY in this repo's
# template-fragments/ (D10 placement principle).

[[rigs.patches]]
agent = "evaluator"
provider = "claude-evaluator"
append_fragments = ["city-architecture-standards", "city-evidence-doctrine", "city-invariants"]
max_active_sessions = 4
[rigs.patches.option_defaults]
model = "opus"
effort = "high"

[[rigs.patches]]
agent = "judge"
provider = "claude-judge"
append_fragments = ["city-architecture-standards", "city-evidence-doctrine", "city-invariants"]
max_active_sessions = 2
[rigs.patches.option_defaults]
model = "opus"
effort = "high"
```

(If R1a resolved city scope instead of rig scope, these become `[[patches.agent]]` blocks
in `pack.toml` with identical fields minus the rig context — the mayor patch there is the
shape precedent. State the deviation in the PR.)

**Step 3 — Polecat fragment additions (`city.toml`).** Extend the EXISTING polecat patch's
`append_fragments` array in place: keep the 6 existing names in order (R0), then append
the Step-0-resolved new names (expected: the GCD-WO-CSC-005 R2 six —
`polecat-code-hygiene`, `polecat-evidence-contract`, `polecat-final-rebase-revalidate`,
`polecat-autonomy-and-blockers`, `polecat-submit-to-evaluator`,
`polecat-overseer-issue-marker` (6 total: 5 diligence incl. the submit-to-evaluator
override, + the overseer marker carrying the `Overseer-Issue: <issue-id>` PR/commit
marker duty, A1 §2)). One line, one array — no second polecat patch block. Change NOTHING
else in that patch (provider, `max_active_sessions = 20`, gpt-5.5/high stay
byte-identical).

**Done-routing supersession gate (REJECT-level).** The city's existing list already routes
the polecat done sequence to the refinery: `polecat-handoff-override` renders "DONE
SEQUENCE — USE THIS, NOT THE ONE ABOVE" ending in
`REFINERY_TARGET="${GC_RIG:+$GC_RIG/}gastown.refinery"` (source:
codegen-support `template-fragments/polecat-handoff-override.template.md`), and
`polecat-done-target-override` amends it. The new submit-to-evaluator override changes that
terminal routing. Before appending, READ the merged override fragment: it MUST contain
explicit supersession language over the earlier refinery-handoff done sequence (the
established convention — `polecat-handoff-override` itself supersedes the upstream "FINAL
REMINDER" section the same way). Append the six new names AFTER the existing six so the
evaluator-routing override renders LAST. If the merged fragment does NOT explicitly
supersede the refinery handoff, STOP and raise a structured blocker against GCD-WO-CSC-005
— never ship a rendered polecat prompt carrying two live done-targets, and never patch the
upstream fragment from this repo (Non-Goals).

**Step 4 — Formula vars (`city.toml`).** Extend the rig's existing `[rigs.formula_vars]`
table (R0) in place:

```toml
[rigs.formula_vars]
integration_branch_auto_land = "true"
evaluator_gated = "true"
<WATCH_FORMULA_VAR> = "<ROUTER_FORMULA_NAME>"
```

`<WATCH_FORMULA_VAR>` / `<ROUTER_FORMULA_NAME>` = the Step-0-resolved names (C10;
expected: `wo_planning_formula` = `"wo-router"` per GCD-WO-CSC-004's pinned literals —
R1d verifies the merged spelling). `evaluator_gated = "true"` flips the GCD-WO-CSC-005 refinery
branches for this rig (C9; upstream default `"false"` keeps every non-CSC city unchanged).
`spec-cartographer` remains available (legacy formula retained — C10); routing to the
router is exactly this one var.

**Step 5 — City doctrine fragments.** Create `template-fragments/` with three files, each
in the codegen-support fragment format (`{{ define "<name>" }}` … `{{ end }}`,
kebab-case name = filename minus `.template.md`):

- `template-fragments/city-architecture-standards.template.md` — define
  `city-architecture-standards`. Content (MatchPoint-specific; derive, don't invent):
  SOLID + no-band-aids + never-weaken/skip/special-case-a-gate + additive-repair-with-ADR
  (from `mlh/prompts/implementer.md` residue); Matchpoint-Vehicle-Graph repo conventions —
  `specs/` + ADR discipline, `packages/naming` helpers for every AWS name, uv-workspace +
  pytest tier layout, typed contracts (`pydantic extra="forbid"`) at seams (from
  `Matchpoint-Vehicle-Graph/AGENTS.md` + `specs/`).
- `template-fragments/city-evidence-doctrine.template.md` — define
  `city-evidence-doctrine`. Content: the fixture-realism doctrine core, carried faithfully
  from `master/DOCTRINE-fixture-realism-and-lifecycle-seam-acceptance.md`: fixtures
  replicate real producer behavior (captured/producer-generated; canonical
  `gid://<ns>/...` ids; real numerics; full envelopes); live proofs traverse real lanes
  (EventBridge→SQS, API GW, real IAM — name any bypassed layer + compensating proof);
  **zero-item runs are NEVER green**; evidence values must trace to executed commands
  (fabricated-evidence ban); PASS without the `eval_evidence` artifact is invalid (C9).
- `template-fragments/city-invariants.template.md` — define `city-invariants`. Content: VG
  domain invariants the evaluator/judge must enforce: no `tenant_id` on Neptune (standing
  estate guardrail); dev-boundary rule ("DEV = do it for REAL; PROD = never" — no prod
  mutation from generated work, prod gates ride the estate cutover); all GasCity-in-AWS
  paused — generated work must not start cities or assume live city services; bounded
  context per `Matchpoint-Vehicle-Graph/specs/` (what this repo may never write).

Bar for all three: ≤ ~120 lines each, imperative voice, no duplication of the generic
upstream prompt text, no secrets, no live endpoints. These fragments are the TEMPLATE
content for 007 — `city-architecture-standards`/`city-evidence-doctrine` are written to be
clone-adaptable (estate-generic MatchPoint doctrine + a short repo-conventions block),
`city-invariants` is the per-city part (mark the VG-specific block with a
`<!-- city-specific -->` comment so 007's adaptation point is explicit).

**Step 6 — Credentials runbook (A1 §11 duty).** Create
`runbooks/claude-evaluator-judge-credentials.md`: the projection contract (secret names
`claude-evaluator` / `claude-judge` in the city namespace, key `CLAUDE_CODE_OAUTH_TOKEN`,
consumed via the Step-1 `env_from_secret` blocks); WHO does WHAT — owner supplies token
values (punch-list item), aws-GasCity operator applies the Secrets per the existing
city-secret process (`codex-*` / `claude-credentials` precedent, VG-verbatim secret
handling), this repo only declares projections; verification commands for the operator
(`kubectl -n <city-ns> get secret claude-evaluator claude-judge` — documented, NOT run by
this WO: cities paused, no cluster interaction); explicit note that pods fail credential
materialization if the secrets are absent at un-pause (`optional = false` is deliberate —
fail loud, never silently run evaluator/judge on borrowed credentials).

The runbook ALSO carries the **evidence-vars un-pause punch item (A2.10)**: at un-pause,
wire `evidence_publish_cmd` / `evidence_fetch_cmd` per city into the rig's
`[rigs.formula_vars]` from AGC-WO-CSC-003's spec-18 grammar (the durable-URI S3 evidence
lane) — **without them, no-PVC cities dead-end every evaluated bead to `mayor_action`**:
local evidence dies with the pod, the judge fails CLOSED on unreachable evidence (C9
fail-closed rule), and the repeated rejections burn the shared `eval_reject_count` budget
straight to escalation. State this as an explicit punch-list line beside the secrets
item; it is an un-pause action, not a paused-state acceptance criterion.

**Step 7 — City tests (`tests/`, cartographer-*.sh pattern).** Five standalone scripts +
the Step-0 names file. Common preamble per the existing pattern (`set -euo pipefail`,
`ROOT` resolution, `mktemp -d` + trap). Where a test needs the materialized
`.gc/system/packs/`, it materializes them itself: build `gc` once from the HOME worktree
(`make build` in GasCity-Dev, Makefile:27) and run `gc config show --validate` in the city
root (materializes builtin packs — `cmd/gc/cmd_config.go:30` — and validates the composed
config); tests take the `gc` binary path from `$GC_BIN` (default: `gc` on PATH) and SKIP-fail
loudly (exit 1 with a message) if unavailable — never silently pass.

- `tests/csc-binding-providers.sh` — asserts both `[providers.claude-evaluator]` /
  `[providers.claude-judge]` blocks exist with `base = "builtin:claude"`,
  `optional = false`, `env_from_secret` projecting `CLAUDE_CODE_OAUTH_TOKEN` from the
  matching secret name; asserts NO token-like value is committed
  (`grep -RInE 'sk-ant-|oauth[_-]?token\s*=\s*"[^"{]' city.toml template-fragments/ runbooks/claude-evaluator-judge-credentials.md`
  finds nothing).
- `tests/csc-binding-patches.sh` — asserts the evaluator patch (provider
  `claude-evaluator`, `max_active_sessions = 4`, opus/high) and judge patch
  (`claude-judge`, `2`, opus/high) exist for the rig; asserts the polecat patch still
  carries all 6 original fragments AND every new name from `csc-resolved-names.env`;
  asserts the refinery patch and cartographer patch are unchanged
  (`git diff --quiet origin/main -- city.toml` is expected to FAIL overall, so anchor
  these as content assertions, e.g. `grep -q 'agent = "cartographer"' -A2` shape checks,
  not whole-file diffs).
- `tests/csc-binding-fragments-resolve.sh` — for EVERY name in every `append_fragments`
  array in `city.toml` + `pack.toml`: assert a backing `{{ define "<name>" }}` exists in
  `template-fragments/*.template.md` (city) or
  `.gc/system/packs/*/template-fragments/*.template.md` (materialized upstream).
  **Planted-RED self-check** (fixture-realism: zero-item never green): copy `city.toml`
  to `$TMP`, inject a bogus fragment name, re-run the resolver function against the copy,
  assert it FAILS — proving the test detects absence.
- `tests/csc-binding-formula-vars.sh` — asserts `evaluator_gated = "true"` and
  `<WATCH_FORMULA_VAR> = "<ROUTER_FORMULA_NAME>"` (names sourced from
  `csc-resolved-names.env`) in the rig's `formula_vars`; asserts the router formula file
  exists in the materialized codegen-support pack AND the watch script reads the var
  (`grep -q "$WATCH_FORMULA_VAR" .gc/system/packs/codegen-support/assets/scripts/spec-cartographer-watch.sh`).
- `tests/csc-doctrine-content.sh` — asserts each of the three doctrine fragments exists,
  has the correct `{{ define }}` name, is non-trivial (≥ 30 lines), and carries its
  load-bearing markers: architecture-standards mentions `packages/naming` and band-aid;
  evidence-doctrine contains `zero-item` (never-green rule), `gid://`, and the
  fabricated-evidence ban; invariants contains `tenant_id` (Neptune guardrail) and the
  paused-cities rule. Asserts the MatchPoint-literal placement rule: `grep -ril
  "matchpoint" template-fragments/` MAY hit (this is the sanctioned home) — and the test
  re-asserts the UPSTREAM surface stays clean by grepping the materialized
  `.gc/system/packs/codegen-support/` for `matchpoint|vehicle-graph` and expecting zero
  hits outside tests (mirror of GCD-WO-EVAL-001's generic-ness gate, from the consumer
  side).

**Step 8 — Full validation battery** (see Validation) + PR.

## Git Workflow

Loop execution: home-repo branch `wo/GCD-WO-CSC-006-city-pack-binding-pilot` (or
`polecat/$BEAD_ID` under city execution) in GasCity-Dev — the home repo carries NO content
change beyond this spec file already being on main; the co_repo branch in
`vehicle-graph-city` carries all edits. The harness CoordinatedMerge saga owns the
multi-repo merge; never commit directly to `main` in either repo; never push secrets.
vehicle-graph-city merges to its `origin/main` — hosted pickup happens later via
source-sync at un-pause (no deploy action in this WO).

## Test Coverage

- **City structural tier (`tests/csc-*.sh`, step 7):** the five suites above; each
  assertion FAILS (non-zero) when its binding is absent; the fragment-resolution suite
  carries the planted-RED self-check. These are the acceptance-criteria backing tests —
  every AC below names one.
- **Config-resolution tier:** `gc config show --validate` (composed city config with all
  patches + both new providers resolves clean; exit 0) and `gc lint .` over the city pack
  (fragment references check — `cmd/gc/cmd_lint.go:317` class diagnostics clean). Run from
  the city worktree root with the wave-23 `gc` built from the home worktree.
- **No-regression tier:** ALL pre-existing `tests/*.sh` still pass unmodified — 7 on
  disk at the pinned `71ee67ec`: the 5 `cartographer-*.sh` scripts +
  `tests/spec-cartographer-watch-busy-predicate.sh` + `tests/debugger-plan-lifecycle.sh`
  (enumerate from the worktree at execution rather than trusting this count; run every
  one). The binding must not disturb legacy cartographer machinery — C10 retains it.

## Validation

- All `tests/*.sh` green in the co_repo worktree (old + new), with the planted-RED
  self-check demonstrated in evidence (show the injected-bogus-name run failing).
- `gc config show --validate` exit 0; `gc lint .` clean — both run with the post-wave-23
  binary; command transcripts in evidence (`{command, output_excerpt}` pairs — C9-grade
  evidence discipline applies to this WO's own proof).
- Discovery-gate table (Step 0) recorded; every bound name traced to a merged upstream
  file path.
- Upstream-cleanliness grep (step 7, doctrine test) green: no MatchPoint literals in the
  materialized upstream packs; MatchPoint content present ONLY in city
  `template-fragments/`.
- **Cities-PAUSED clause:** all GasCity-in-AWS remains paused (zero-replica / suspended);
  this WO performed no cluster/AWS interaction, started no city, no daemon, no supervisor
  (`gc config show`/`gc lint` are offline config commands); no live drill is claimed. Live
  behavior validation (evaluator/judge actually claiming beads on the vehicle-graph pilot)
  is a NAMED FOLLOW-UP gated on un-pause under the standing pilot policy — never an
  acceptance criterion here.
- No secret values anywhere in the diff (providers test + manual review).
- Home-repo worktree untouched except loop bookkeeping (this WO adds no GasCity-Dev
  content).

## Acceptance Criteria

Each criterion names its backing test:

1. Claude provider profiles present, secret projections correct, no committed secrets —
   `tests/csc-binding-providers.sh`.
2. Evaluator/judge patches bound per rig with claude-evaluator/claude-judge, opus/high,
   pools 4/2; existing cartographer/polecat/refinery patch fields undisturbed —
   `tests/csc-binding-patches.sh`.
3. Polecat `append_fragments` = original 6 + the GCD-WO-CSC-005 R2 six
   (`polecat-code-hygiene`, `polecat-evidence-contract`,
   `polecat-final-rebase-revalidate`, `polecat-autonomy-and-blockers`,
   `polecat-submit-to-evaluator`, `polecat-overseer-issue-marker` — 6 total: 5 diligence
   incl. the submit-to-evaluator override, + the overseer marker per A1 §2), every name
   resolving to a real fragment — `tests/csc-binding-patches.sh` +
   `tests/csc-binding-fragments-resolve.sh`.
4. `evaluator_gated = "true"` + router selection var set to the merged router formula
   name; router formula exists; watch script consumes the var —
   `tests/csc-binding-formula-vars.sh`.
5. Three city doctrine fragments authored with the C11 names, define-wrapper format,
   load-bearing content markers; MatchPoint content city-side only —
   `tests/csc-doctrine-content.sh`.
6. Composed config + pack lint clean with the post-wave-23 binary — Validation battery
   transcripts.
7. Credentials runbook shipped; owner punch list line stated in the PR description
   ("provision k8s Secrets claude-evaluator/claude-judge in the vehicle-graph city
   namespace — owner supplies CLAUDE_CODE_OAUTH_TOKEN values, operator applies");
   acceptance does NOT depend on the secrets existing.
8. Binding template section below verified still-accurate against what was actually built
   (it is what 007 executes); any execution-time deviation folded back into that section
   in the same PR.
9. No city started; no AWS/cluster call; no live drill claimed (cities PAUSED).

## Risks

- **Fragment shadowing/resolution semantics differ from expectation (WS2 risk K1)** — the
  binding relies on GCD-WO-CSC-003's merged, packlint-validated mechanism; Step 0 R1b
  discovers it instead of assuming; the fragment-resolution test proves every name binds.
  Mitigation is discovery + tests, not assumption.
- **`effort` threading through `option_defaults` on claude providers (WS2 risk K5)** —
  in-repo precedent exists (refinery/mayor patches, R0); if the merged evaluator agent
  documents a different knob, follow the merged doc and note it.
- **Upstream deliverable drift** (003/004/005 merged shapes differ from kit expectations) —
  every name is bound via Step-0 discovery with STOP-gates; this file's TOML is
  verification-expected, not authoritative, for NAMES (it IS authoritative for structure,
  providers, pools, models — kit C9/C11 pins those).
- **Secrets not yet provisioned at un-pause** → evaluator/judge pods fail credential
  materialization loudly (`optional = false`, deliberate). The runbook + punch list are the
  mitigation; nothing in this WO can or should create secrets.
- **Session-capacity interaction**: vehicle-graph-city has NO city-wide
  `max_active_sessions` cap (single rig), so +4/+2 pools are additive headroom here. The
  multi-rig fan-out cities DO cap (24) — that consideration is explicitly handed to 007
  (Binding template, parameter P6), not silently ignored.
- **WO-sizing**: this WO is one coherent run (2 TOML files, 3 fragments, 1 runbook, 5
  tests). If the doctrine fragments balloon, cut content, not tests — the fragments have a
  ≤ ~120-line bar.

## Done Means

- [ ] Step-0 discovery table recorded; all names bound from merged wave-23 content;
      `tests/csc-resolved-names.env` committed.
- [ ] `city.toml`: 2 provider profiles + 2 rig patches + polecat fragment additions +
      2 formula vars, existing content byte-preserved except the enumerated in-place
      extensions.
- [ ] `template-fragments/`: 3 doctrine fragments, define-wrapped, content bars met.
- [ ] `runbooks/claude-evaluator-judge-credentials.md` shipped; punch-list line in PR.
- [ ] 5 new tests + all pre-existing tests green; planted-RED demonstrated;
      `gc config show --validate` + `gc lint .` clean.
- [ ] Binding template section reconciled with as-built reality.
- [ ] Multi-repo merge via the harness saga; no direct-to-main commits; no secrets.
- [ ] No city started, no AWS interaction, live pilot drill left as the named un-pause
      follow-up.
- [ ] Telos pack topology (D6 v2) honored: no telos pack fork, no telos law copies in the
      city, no verdict delegation to a telos role; guardrails A/B honored.

## Master cutover contribution

None — city source-config repo, no AWS resources created or renamed (kit K1 prod-gate
language not triggered: nothing prod-shaped is CREATEd). Runtime exposure arrives only via
source-sync into the hosted city at un-pause, under aws-GasCity's own gates (AGC-WO-CSC
family). The credential punch-list item rides the CSC program punch list, not the cutover.

---

## Binding template (NORMATIVE for GCD-WO-CSC-007 — apply mechanically per city)

Telos rows: this template is additionally bound by the "Telos pack topology (amended
2026-07-14 — D6 v2)" section at this file's tail — its constraints apply unchanged to
every templated city.

Parameters per target city:

| Param | Meaning | vehicle-graph value (pilot) |
|---|---|---|
| P1 `CITY_REPO` | city source repo root | `vehicle-graph-city` |
| P2 `PACK_NAME` | `[pack] name` in the city `pack.toml` | `vehicle-graph-city` |
| P3 `RIG_LIST` | every `[[rigs]]` entry (name/prefix) | `Matchpoint-Vehicle-Graph` (`vg`) |
| P4 `SECRET_PAIR` | k8s secret names (fixed) | `claude-evaluator` / `claude-judge` |
| P5 `INVARIANTS_SOURCES` | rig repos' `specs/` + AGENTS.md for the `city-invariants` fragment | `Matchpoint-Vehicle-Graph/specs/` |
| P6 `CITY_CAP` | city-wide `[workspace] max_active_sessions`, if present | absent (single rig) |

Procedure (numbers = this WO's Implementation Steps; all are per-CITY_REPO):

- **T0 = Step 0** verbatim (discovery from the home worktree; one shared
  `csc-resolved-names.env` per city repo).
- **T1 = Step 1** verbatim (provider profiles are city-scoped: exactly ONE
  claude-evaluator + ONE claude-judge block per city.toml regardless of rig count; same
  secret names in every city — secrets are per-namespace).
- **T2 = Step 2** repeated for EVERY rig in P3 (each rig's patch list gains the two
  blocks; pools 4/2 are PER RIG).
- **T3 = Step 3** repeated for every rig's polecat patch.
- **T4 = Step 4** repeated for every rig's `formula_vars`.
- **T5 = Step 5**: create `template-fragments/` if absent;
  `city-architecture-standards` + `city-evidence-doctrine` = clone the pilot fragments,
  adapting ONLY the repo-conventions block to the city's rigs; `city-invariants` =
  re-derive from P5 (the `<!-- city-specific -->` block is per-city work, not a copy).
- **T6 = Step 6**: clone the runbook, substituting the city namespace; ONE punch-list
  line per city (2 secrets each).
- **T7 = Step 7**: create `tests/` if absent; clone the five `csc-*.sh` tests — they are
  written parameter-free (they iterate ALL rigs/patches found in the TOML, so multi-rig
  cities are covered without edits) except the doctrine markers, which follow the city's
  invariants fragment.
- **T8 = Validation** verbatim (config validate + lint + tests + paused clause).
- **P6 rule (multi-rig capped cities)**: do NOT change `CITY_CAP`. Evaluator/judge pooled
  sessions share the existing cap headroom by design while cities are paused; capacity
  re-derivation (polecat-allocation-derived, NodePool-backed) is an aws-GasCity
  operational action at un-pause. UPDATE the cap's comment to name evaluator/judge as
  pooled-session tenants and add the un-pause re-derivation note; record the same in the
  PR as an owner-visible flag.
- Preservation rule: existing patches, fragments, `[daemon]`, caps, and comments stay
  byte-identical outside the enumerated in-place extensions.

## Telos pack topology (amended 2026-07-14 — D6 v2)

Tail amendment — BINDING. Owner ruling D6 v2 (telos-layer program pack-topology
ruling, 2026-07-14) fixes where telos-layer content may live. These are ADDITIVE
constraints binding BOTH this WO's pilot binding AND the **Binding template** section
above — every city templated by `GCD-WO-CSC-007` inherits them unchanged. The full
constraint is stated here — an executor reading only this WO needs nothing else:

1. **Same import surface, never a fork.** City imports of applicable telos packs (per
   the city's role mix) ride the same `.gc/system/packs/<name>` import surface as this
   WO's binding — never a pack fork. Behavior tuning stays in the sanctioned override
   layers already named in Non-Goals; copying a telos pack file into the city repo to
   edit it is a REJECT, exactly like any other imported pack.
2. **Telos content in a city = the P1.5 delivery artifacts only.** The sha-pinned
   SYSTEM-TELOS snapshot + the telos-binding fragment — never Matchpoint law inside
   pack files.
3. **Verdicts never delegate to a telos role.** No evaluator/judge fragment (pilot or
   templated) may delegate verdicts to a telos role — conformance verdicts stay in the
   single evaluator/judge lane.
4. **Guardrails (verbatim, BINDING):**
   "(A) packs carry primitives + sha-pinned POINTERS to the SYSTEM-TELOS snapshot, never a second copy of the law; (B) the monitoring pack emits telemetry/findings ONLY — conformance verdicts stay in the single evaluator/judge lane (GCD-WO-CSC-003 / GCD-WO-EVAL-001, shaped to blueprint ROL-5/6 pre-merge; no telos-specific judge role)."
