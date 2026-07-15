# Work Order: GCD-WO-CSC-007 — city-pack binding FAN-OUT: apply the GCD-WO-CSC-006 template to the 5 remaining code-generation cities

NOTE: read the FULL file before implementing, and read the merged
`specs/agent-work-orders/GCD-WO-CSC-006-city-pack-binding-pilot.md` **Binding template**
section FIRST — that section is the NORMATIVE procedure this WO executes; this file adds
only the per-city parameters, deltas, and acceptance. Where this file and the merged 006
template conflict, the merged 006 template wins (it was reconciled against the as-built
pilot).

Execution classification: Dev-only city-source configuration across five deployed-city
SOURCE repos (TOML patches + template fragments + repo-native tests; no AWS mutation, no
deploy surface, no city runs). `boundary: dev` (QST-6 fail-closed) · `live-tier: none` (no
hosted interaction anywhere; runtime exposure only via source-sync at un-pause under the
AGC WOs' gates) · `blast radius:` five city source repos (the closed A1 §8 edit set) —
config bindings, doctrine fragments, runbooks, tests; the pilot and exec-mon cities
receive ZERO commits · `additive vs mutating: additive` (template-applied in-place
extensions only; byte-preservation rule pinned). **Wave 25** (CSC program band 23/24/25;
harness-ledger mega-wave 35 as of 2026-07-14),
`blocked_by` `GasCity-Dev::GCD-WO-CSC-006-city-pack-binding-pilot` (wave 24 — cross-wave
edge, parser-safe; pinned in harness DEPS regardless).
Multi-repo unit — co_repos (object-form, EXACTLY per kit A1 §8, for the wiring entry):

```json
[
  {"repo": "demo-sandbox-code-generation-city", "role": "edit", "test": true},
  {"repo": "product-enrichment-code-generation-city", "role": "edit", "test": true},
  {"repo": "compatibility-orchestration-code-generation-city", "role": "edit", "test": true},
  {"repo": "client-platform-code-generation-city", "role": "edit", "test": true},
  {"repo": "analytics-code-generation-city", "role": "edit", "test": true}
]
```

Home repo (this spec; no other GasCity-Dev content) = `GasCity-Dev`; ALL edits land in the
five co_repo worktrees.

> **Provenance (binding):** CSC program contract authority
> `master/city-scaling-improvements/wo-authoring-kit.md` — **C11** ("Fan-out
> (`GCD-WO-CSC-007`) applies the template via co_repos object-form"), **A1 §8 (F8)**
> ("Fan-out pinned BY NAME: GCD-WO-CSC-007 patches exactly `demo-sandbox-`,
> `product-enrichment-`, `compatibility-orchestration-`, `client-platform-`,
> `analytics-code-generation-city`. The 4 superseded stubs (`billing-`, `client-identity-`,
> `client-portal-`, `compatibility-view-code-generation-city`) are **DEAD — never
> patch**."), **A1 §11** (credential pre-stage pattern). Backlog:
> `master/city-scaling-improvements/gap-analysis-and-build-plan.md` §5 row 15, §6 ("City
> binding patch template | GCD-WO-CSC-006 | GCD-WO-CSC-007"). Design record:
> `aws-GasCity/docs/investigations/2026-07-city-scaling-improvements.md` WS2 (owner ruling
> **D10**; landing map). Process: root `SKILL-work-order-audit-and-authoring.md` §1.1–1.5.
> Verified at authoring (2026-07-08), all five co_repos clean at `origin/main`:
> `demo-sandbox-code-generation-city` @ `76c816e2470d5eecb59bd416534d013b43a62e49`;
> `product-enrichment-code-generation-city` @ `ffc9db0327daa6bdfab8e523dc590c59620a0b25`;
> `compatibility-orchestration-code-generation-city` @
> `2111231901edfca9f9c605a167bb3736305aeaf7`;
> `client-platform-code-generation-city` @ `f6ec0b824dbfd75e08899f4937dbbf0f0cd087c9`;
> `analytics-code-generation-city` @ `0ae2fa174488400d4ab10e565c210fd8b2a2adfa`;
> `GasCity-Dev` `origin/main` @ `c85d92cf0cfd1215be1467628d6fd2e06db46aae`. Re-verify all
> at execution time — by then waves 23–24 are merged (upstream primitives + the pilot
> binding + the reconciled template).
> Ledger stem: `GasCity-Dev::GCD-WO-CSC-007-city-pack-binding-fanout`.

## Goal

Every ADR-023 code-generation city other than the pilot is bound to the WS2 evaluator/
judge/router pipeline by mechanically applying the merged GCD-WO-CSC-006 Binding template
(T0–T8, parameters P1–P6) to each of the five named city repos. Clean end state per city:
claude-evaluator/claude-judge provider profiles (one pair per city.toml); evaluator+judge
`[[rigs.patches]]` (opus/high, pools 4/2) on EVERY rig; polecat `append_fragments`
extended with the wave-23 additions on EVERY rig; `evaluator_gated = "true"` + the router
watch var on EVERY rig; the three C11 city doctrine fragments in the city's own
`template-fragments/`; the credentials runbook; the five `csc-*.sh` structural tests
green. After this WO, all 6 code-generation cities carry the binding; the 2
exec-monitoring cities carry zero changes (D5).

Business reason: D10 ratified the pipeline for code-generation cities as a class; the
pilot (wave 24) proved the binding on vehicle-graph; this WO completes estate coverage so
un-pause brings every city up on the evaluator-gated flow, not a mixed fleet.

## Dependencies

- **Blocked by:** `GasCity-Dev::GCD-WO-CSC-006-city-pack-binding-pilot` (wave 24) — the
  merged Binding template section is the procedure; the pilot's doctrine fragments are the
  clone sources; the pilot's tests are the test sources. Also transitively behind
  GCD-WO-CSC-003/004/005 (wave 23) via 006 — all upstream names are discovered from the
  merged GasCity-Dev tree exactly as 006's Step 0 prescribes.
- **Contract imports (never re-declared):** C9 verdict metadata + models
  (GCD-WO-CSC-003); C10 router/watch var (GCD-WO-CSC-004); C11 binding shape + fragment
  names (GCD-WO-CSC-006). The ONE discovery source for names is the merged home worktree;
  the pilot's committed `tests/csc-resolved-names.env` in `vehicle-graph-city` is the
  cross-check (they must agree; if they differ, STOP — upstream changed between waves,
  raise a blocker).
- **Cities PAUSED (standing policy + kit K1):** this WO verifies all GasCity-in-AWS
  remains paused (zero-replica / suspended) before declaring success — concretely: no
  kubectl, no AWS API, no `gc` daemon/city/session/supervisor start anywhere; live drills
  are only ever the vehicle-graph pilot (NOT one of this WO's five repos), and **this WO
  names NO live drill**. Runtime exposure arrives via source-sync at un-pause under the
  AGC WOs' gates.
- **Fixture-realism doctrine** (owner-ratified, REJECT-level):
  `master/DOCTRINE-fixture-realism-and-lifecycle-seam-acceptance.md` — binds the test
  discipline (structural tests fail on absence; zero-item never green) and the
  `city-evidence-doctrine` fragment content.

## Non-Goals

Bounded-context REJECT rules (kit K2, `*-code-generation-city` row) restated, plus fan-out
pins:

- **The 4 superseded stubs are DEAD — never patch (A1 §8):**
  `billing-code-generation-city`, `client-identity-code-generation-city`,
  `client-portal-code-generation-city`, `compatibility-view-code-generation-city`. They
  exist on disk but are superseded by `client-platform-code-generation-city` (ADR-023: one
  client-platform city supersedes the 4 stubs). Zero edits, zero tests, zero mentions in
  deliverables — their names may appear ONLY inside this declaration.
- **NO exec-monitoring-city changes** (`vehicle-graph-execution-monitoring-city`,
  `product-enrichment-execution-monitoring-city` — D5: zero changes).
- **NO vehicle-graph-city edits** — the pilot is done (wave 24); if applying the template
  reveals a pilot defect, raise a blocker referencing 006, do not "fix" the pilot here.
- **NO forking imported packs; NO hand-editing `.gc/system/packs` mirrors; NO upstream
  GasCity-Dev pack edits; NO MatchPoint literals upstream** (identical to 006 — the city
  `template-fragments/` is the sole sanctioned home for MatchPoint content).
- **NO secret values committed; NO cluster interaction** — 10 k8s Secrets (2 per city) are
  operator-applied punch-list items (A1 §11); acceptance never requires them to exist.
- **NO city-wide `max_active_sessions` cap changes** (template rule P6): the
  compatibility-orchestration and client-platform caps stay `24`; only their COMMENTS gain
  the evaluator/judge tenancy + un-pause re-derivation note. Raising platform capacity is
  aws-GasCity/NodePool scope, owner-flagged, not this WO.
- **NO upstream-default tuning** (`max_eval_rejects`, retry semantics,
  `regenerate_on_reject`, convoy-autoland battery — all keep upstream defaults; only the
  vars the template enumerates are set).
- **NO changes to existing bindings beyond the enumerated in-place extensions** —
  cartographer patches, existing polecat/refinery fragment lists, mayor/dog/debugger
  `pack.toml` patches, `[daemon]`, `integration_branch_auto_land = "true"`, and every
  comment stay byte-identical outside the template's additions. (`pack.toml` files change
  ONLY if 006's as-built binding resolved to city scope — R1a deviation path; expected: no
  pack.toml change at all.)
- No telos pack forking; no telos law copies in any city (guardrail A) — see the "Telos
  pack topology" tail section.

## Architecture Links

- Merged `GasCity-Dev/specs/agent-work-orders/GCD-WO-CSC-006-city-pack-binding-pilot.md` —
  the Binding template (NORMATIVE) + the R0 verbatim anchors (the five cities' city.toml
  patch blocks are structurally identical to the quoted vehicle-graph shapes, per-rig).
- `master/city-scaling-improvements/wo-authoring-kit.md` K2/K4-C11/K6, A1 §8/§11.
- `aws-GasCity/docs/investigations/2026-07-city-scaling-improvements.md` WS2 landing map.
- ADR-023 city topology (6 code-gen + 2 exec-mon cities; the 4 stubs superseded) —
  `aws-GasCity` spec/adr set; the rig rosters below are the on-disk reality of that
  topology.
- Per-city `AGENTS.md`/`CLAUDE.md` (hosted-city source-edit process; all five reference
  `vehicle-graph-city/runbooks/hosted-city-agent-editing-process.md`).

## Packages To Inspect

- HOME worktree (`GasCity-Dev`): identical inspection set to 006 (codegen-support
  agents/fragments/formulas/watch script; `Makefile build`) — used for T0 discovery and
  the test binary.
- PILOT (read-only clone source; NOT a co_repo): `vehicle-graph-city` at `origin/main`
  post-wave-24 — `template-fragments/city-*.template.md`, `tests/csc-*.sh`,
  `tests/csc-resolved-names.env`, `runbooks/claude-evaluator-judge-credentials.md`.
  Locate the `vehicle-graph-city` checkout at the estate code root (a sibling of
  `GasCity-Dev` — NOT a fixed relative path; from a rig worktree cwd the estate root is
  several levels up, `../../../vehicle-graph-city`-class); verify you have the right
  directory by its `pack.toml` name before reading; pull first. The as-built pilot
  artifacts (fragments, tests, runbook, `csc-resolved-names.env`) live ONLY there.
- The five co_repo worktrees — full `city.toml` + `pack.toml` per repo; the per-city
  delta table below is the authoring-time survey (re-verify each at execution).

## Required Inputs — per-city delta table (surveyed 2026-07-08 at the SHAs above)

The five cities are NOT copies of vehicle-graph; enumerate deltas, never assume the
pilot's shape. Shared baseline (verified in all 5): same 3 codex provider profiles
(`codex-polecat`/`codex-cartographer`/`codex-debugger`) with the same secret projections
incl. the cartographer `claude-credentials` CLAUDE_CODE_OAUTH_TOKEN `env_from_secret`;
same `[defaults.rig.imports]` (gastown + codegen-support via `.gc/system/packs/...`);
per-rig patch trio identical in shape to the pilot's R0 quote (cartographer →
codex-cartographer; polecat → codex-polecat + the same 6 fragments + 20 sessions +
gpt-5.5/high; refinery → claude + the same 10 fragments + opus/high);
`integration_branch_auto_land = "true"` per rig; same `[daemon]` block; `pack.toml`
identical across all 5 except `[pack] name` (all patch mayor/dog/debugger exactly like the
pilot).

| City repo (P1) | P2 `[pack] name` | P3 rigs (name / prefix) | P6 city cap | Dirs present at survey | Delta actions |
|---|---|---|---|---|---|
| `demo-sandbox-code-generation-city` | `demo-sandbox-code-generation-city-dev` | `Matchpoint-Demo-Sandbox` / `ds` | none | `template-fragments/` (EMPTY), `formulas/` (empty), `orders/` (empty), `assets/`, `commands/`, `doctor/`, `overlays/`; **no `tests/`** | create `tests/`; populate `template-fragments/` |
| `product-enrichment-code-generation-city` | `product-enrichment-code-generation-city-dev` | `Matchpoint-Product-Enrichment` / `pe` | none | `template-fragments/` (EMPTY), `formulas/`, `orders/`, `assets/`, `commands/`, `doctor/`, `overlays/`; **no `tests/`** | create `tests/`; populate `template-fragments/`. NOTE: the PE **domain** also has `product-enrichment-execution-monitoring-city` — out of scope (D5) |
| `compatibility-orchestration-code-generation-city` | `compatibility-orchestration-code-generation-city-dev` | 6 rigs: `Matchpoint-Product-Compatibility`/`pc`, `Matchpoint-Sync-Orchestration`/`so`, `Matchpoint-Vehicle-Projection`/`vp`, `Matchpoint-Estate-Foundation`/`ef`, `Matchpoint-Platform`/`plt`, `Matchpoint-Estate-Ops`/`eo` | `[workspace] max_active_sessions = 24` (city.toml:10, owner-decision comment 2026-06-12) | `template-fragments/` (EMPTY), `scripts/`, `commands/`, `assets/`, `doctor/`, `overlays/`; **no `tests/`** | T2/T3/T4 × 6 rigs; create `tests/`; populate `template-fragments/`; P6 comment update ONLY (cap value unchanged) |
| `client-platform-code-generation-city` | `client-platform-code-generation-city-dev` | 5 rigs: `Matchpoint-Client-Identity`/`cid`, `Matchpoint-Billing`/`bil`, `Matchpoint-Compatibility-View`/`cvw`, `Matchpoint-Client-Portal`/`cpl`, `Matchpoint-Client-Platform-Ops`/`cpo` | `[workspace] max_active_sessions = 24` (city.toml:11, mirrors compat-orch precedent) | **minimal repo**: only `AGENTS.md`, `CLAUDE.md`, `README.md`, `city.toml`, `pack.toml` — **no `template-fragments/`, no `tests/`** | T2/T3/T4 × 5 rigs; CREATE `template-fragments/` AND `tests/`; P6 comment update ONLY |
| `analytics-code-generation-city` | `analytics-code-generation-city-dev` | `Matchpoint-Analytics` / `anl` | none | **minimal repo**: only `AGENTS.md`, `CLAUDE.md`, `README.md`, `city.toml`, `pack.toml` — **no `template-fragments/`, no `tests/`** | CREATE `template-fragments/` AND `tests/` |

P5 `INVARIANTS_SOURCES` per city (for the `city-invariants` fragment's
`<!-- city-specific -->` block — derive from the rig repos' `specs/` + `AGENTS.md` at the
estate code root):

- demo-sandbox: `Matchpoint-Demo-Sandbox` (Dev=demo env doctrine; house/owner-operator
  tenant realities; never contact the real client store from Dev — Dev has NO Shopify
  webhooks, fixtures are the canonical Dev ingress).
- product-enrichment: `Matchpoint-Product-Enrichment` (tenant-scoped PIM;
  ProductMetadataExtensions materialization contract; Neptune is
  CloudFormation-managed — generated work never assumes CDK ownership of it).
- compatibility-orchestration: all six rig repos — the invariants fragment must cover the
  MULTI-RIG reality: bounded contexts per rig; `Matchpoint-Platform` is NON-DEPLOYABLE
  (Core Invariant 8 — generated work in the `plt` rig must never add deployable stacks);
  Estate-Foundation constructs stay domain-agnostic.
- client-platform: all five rig repos (Client-Identity/Billing/Compatibility-View/
  Client-Portal/Client-Platform-Ops bounded contexts; Stripe test-clock/UAT personas are
  dev-only surfaces).
- analytics: `Matchpoint-Analytics` (fact-lake read-model discipline; billing/dashboard
  consumers never write domain stores).

Keep each city-specific block SHORT (≤ ~40 lines) — invariants the evaluator/judge can
actually enforce on a diff, not repo documentation dumps.

## Implementation Steps

**Step 0 — Preflight (all repos).** `git pull` the estate checkout of `vehicle-graph-city`
(pilot artifacts must be post-wave-24 `origin/main`). In the HOME worktree, run 006's
R1a–R1d discovery; diff the resolved names against the pilot's committed
`tests/csc-resolved-names.env` — MUST match exactly (STOP on mismatch). Re-verify each
co_repo against the delta table (rig rosters, caps, dir presence); fold any drift into the
PR description with evidence.

**Step 1 — demo-sandbox-code-generation-city.** Apply template T0–T8 with row-1
parameters: T1 provider pair; T2/T3/T4 on the single `ds` rig; T5 clone
`city-architecture-standards` + `city-evidence-doctrine` from the pilot (adapt the
repo-conventions block to `Matchpoint-Demo-Sandbox`), author `city-invariants` from P5;
T6 runbook clone (namespace = the demo-sandbox city's k8s namespace as named by
aws-GasCity's generated manifest — the `render_platform` PROCESS_SPECS city names are the
authority; AGENTS.md files carry no namespace info); T7 clone the five `csc-*.sh` tests +
commit this city's `tests/csc-resolved-names.env`; T8 validation battery.

**Step 2 — product-enrichment-code-generation-city.** Same as Step 1 with row-2
parameters (rig `pe`; invariants from P5 incl. the exec-mon-city-out-of-scope note in the
fragment is NOT needed — that is WO scope, not agent doctrine; keep the fragment to
domain invariants).

**Step 3 — compatibility-orchestration-code-generation-city.** Row-3 parameters: T1 once;
T2/T3/T4 repeated for ALL SIX rig patch lists (pc, so, vp, ef, plt, eo — pools 4/2 are
per rig; the city cap bounds actual concurrency); T5 with the multi-rig invariants block
(P5); P6: extend the `max_active_sessions = 24` comment (city.toml:4-10) with: evaluator/
judge pooled sessions now share this cap; re-derive capacity from the polecat allocation +
NodePool sizing at un-pause (aws-GasCity action, owner-flagged) — cap VALUE unchanged;
T6/T7/T8.

**Step 4 — client-platform-code-generation-city.** Row-4 parameters: create
`template-fragments/` and `tests/` (minimal repo); T2/T3/T4 × 5 rigs (cid, bil, cvw, cpl,
cpo); P6 comment update as in Step 3 (city.toml:4-11); T5 invariants from the five rig
repos; T6/T7/T8.

**Step 5 — analytics-code-generation-city.** Row-5 parameters: create both dirs; single
`anl` rig; T1–T8.

**Step 6 — Cross-city consistency sweep (set-level seam check).** After all five: run a
consistency script (may live in evidence, not necessarily committed) asserting across the
5 repos + pilot: identical provider-profile TOML (modulo nothing — the pair is
city-invariant); identical evaluator/judge patch fields (provider/pools/model/effort);
identical fragment NAME sets in `append_fragments`; identical `evaluator_gated`/watch-var
settings; `city-evidence-doctrine` + `city-architecture-standards` content-identical to
the pilot except the marked repo-conventions block; `city-invariants` unique per city with
the `<!-- city-specific -->` marker present. Record the sweep transcript in evidence. Any
divergence is a defect — fix the fan-out copy, never the pilot.

**Step 7 — Punch list + PR.** One PR body section listing the 10 operator/owner
punch-list items (per city: k8s Secrets `claude-evaluator` + `claude-judge`, key
`CLAUDE_CODE_OAUTH_TOKEN`, owner supplies values, operator applies via the aws-GasCity
city-secret process) + the two P6 capacity re-derivation flags (compat-orch,
client-platform) + the FIVE **evidence-vars un-pause items (A2.10)** — one per fan-out
city: at un-pause, wire `evidence_publish_cmd` / `evidence_fetch_cmd` into every rig's
`[rigs.formula_vars]` from AGC-WO-CSC-003's spec-18 grammar (the durable-URI S3 evidence
lane). Without them, no-PVC cities dead-end every evaluated bead to `mayor_action`:
local evidence dies with the pod, the judge fails CLOSED on unreachable evidence, and
the shared `eval_reject_count` budget burns straight to escalation. Each city's cloned
credentials runbook (T6) carries the same line, mirroring the pilot's runbook. These are
un-pause actions, never a paused-state acceptance criterion.

## Git Workflow

Loop execution: home-repo branch `wo/GCD-WO-CSC-007-city-pack-binding-fanout` (or
`polecat/$BEAD_ID`); one branch per co_repo carrying that city's edits; the harness
CoordinatedMerge saga owns the 6-repo merge (home + 5 co_repos). Never commit directly to
any `main`; never push secrets. The pilot repo (`vehicle-graph-city`) receives NO commits
from this WO.

## Test Coverage

Per city (5×): the five cloned `csc-*.sh` structural suites (providers / patches /
fragments-resolve / formula-vars / doctrine-content) — written rig-iterating so the
multi-rig cities are fully covered (every rig's patch list asserted, not just the first);
each test FAILS on absence (fixture-realism: zero-item never green); the
fragments-resolve suite keeps the pilot's planted-RED self-check per city. Doctrine-content
markers per city: the shared markers (band-aid, `packages/naming`, `zero-item`, `gid://`,
fabricated-evidence ban, paused-cities rule) plus ONE city-specific invariant marker each
(demo-sandbox: real-client-store prohibition; PE: `ProductMetadataExtensions`;
compat-orch: `Matchpoint-Platform` non-deployable; client-platform: bounded-context rule;
analytics: read-model discipline) — proving the invariants fragment was actually
re-derived, not blind-cloned.

Config-resolution tier per city: `gc config show --validate` exit 0 + `gc lint .` clean
(post-wave-24 `gc` built once from the home worktree via `make build`; `$GC_BIN` passed to
the test scripts; loud SKIP-fail if unavailable).

Set-level: the Step-6 consistency sweep transcript.

## Validation

- All five cities: full `tests/csc-*.sh` green; `gc config show --validate` + `gc lint .`
  clean; transcripts (`{command, output_excerpt}`) in evidence per city.
- Step-0 name diff vs the pilot's `csc-resolved-names.env`: exact match recorded.
- Step-6 cross-city consistency sweep: zero divergence findings (or each finding fixed and
  re-run clean).
- Grep battery: `grep -rl "billing-code-generation-city\|client-identity-code-generation-city\|client-portal-code-generation-city\|compatibility-view-code-generation-city"`
  over the five co_repo diffs returns NOTHING (dead names live only in this WO's
  declaration); upstream-cleanliness grep per city (materialized
  `.gc/system/packs/codegen-support/` free of MatchPoint literals) green.
- **Cities-PAUSED clause:** all GasCity-in-AWS remains paused (zero-replica / suspended);
  this WO performed no cluster/AWS interaction, started no city/daemon/supervisor anywhere
  (offline `gc` config/lint commands only); no live drill claimed. Live validation of the
  fan-out bindings is a NAMED FOLLOW-UP at un-pause (vehicle-graph pilot first per
  standing policy, then fleet) — never an acceptance criterion here.
- No secret values in any diff; caps unchanged (comment-only edits verified in the diff).

## Acceptance Criteria

1. Five cities bound per the merged 006 template: provider pair + per-rig evaluator/judge
   patches (opus/high, 4/2) + polecat fragment additions + `evaluator_gated="true"` +
   router watch var — per-city `tests/csc-binding-providers.sh`,
   `tests/csc-binding-patches.sh`, `tests/csc-binding-formula-vars.sh`; multi-rig cities
   covered on EVERY rig (compat-orch ×6, client-platform ×5) — rig-iterating assertions.
2. Three C11 doctrine fragments per city; shared fragments content-matched to the pilot;
   `city-invariants` re-derived per city with its city-specific marker — per-city
   `tests/csc-doctrine-content.sh` + the Step-6 sweep.
3. Every `append_fragments` name resolves; planted-RED self-check demonstrated per city —
   `tests/csc-binding-fragments-resolve.sh`.
4. Credentials runbook per city; 10 punch-list items + 2 capacity flags + 5
   evidence-vars un-pause items (A2.10) in the PR; acceptance independent of secret
   existence and of the evidence vars being wired (both are un-pause actions).
5. `gc config show --validate` + `gc lint .` clean × 5 (post-wave-24 binary).
6. Dead-stub grep clean; exec-mon cities and `vehicle-graph-city` have zero commits from
   this WO.
7. Existing content byte-preserved outside the enumerated extensions (caps unchanged,
   comments extended only where P6 says so).
8. No city started; no AWS/cluster call; cities-PAUSED verification recorded.

## Risks

- **Blind cloning across differing repos** — the delta table + rig-iterating tests +
  Step-6 sweep exist precisely because the five repos differ (1–6 rigs, caps, missing
  dirs); a generator that assumes the pilot's single-rig shape fails AC-1 on compat-orch/
  client-platform.
- **Pilot drift between waves** (upstream names changed after 006 merged) — Step-0 exact
  name-diff STOP-gate.
- **Cap contention at un-pause** (24-cap cities gain up to 6 rigs × 6 pooled sessions of
  demand) — deliberately NOT resolved here (P6 rule): documented in the cap comments +
  owner-flagged; capacity is platform (NodePool) scope. The risk of doing more (raising
  caps ad hoc) is worse: silent NodePool oversubscription.
- **Repo-set errors** — the DEAD-stub grep + A1 §8 by-name pin prevent patching a
  superseded city; the co_repos list in the preamble is the complete, closed edit set.
- **WO size** — 5 repos × ~10 files is mechanical clone-work off a reconciled template;
  if one city's invariants derivation balloons, cap the fragment (≤ ~40-line
  city-specific block) rather than shrinking tests. If a single generation run cannot
  hold all five cities coherently, complete cities SEQUENTIALLY (Steps 1→5 are
  independent) and keep the Step-6 sweep as the final gate — do not interleave partial
  cities.

## Done Means

- [ ] Step-0 preflight recorded (pilot pulled; names exact-match; deltas re-verified).
- [ ] Five cities: TOML bindings + fragments + runbook + tests, per the delta table.
- [ ] All per-city test suites + config validate + lint green; planted-RED shown per city.
- [ ] Step-6 cross-city consistency sweep clean; transcript in evidence.
- [ ] Dead-stub grep clean; pilot + exec-mon repos untouched.
- [ ] 10 punch-list items + 2 capacity flags + 5 evidence-vars un-pause items surfaced
      in the PR.
- [ ] 6-repo CoordinatedMerge via the harness; no direct-to-main commits; no secrets.
- [ ] No city started; no AWS interaction; live fleet validation left as the named
      un-pause follow-up.
- [ ] Telos topology rows applied unchanged in all five cities: zero telos pack forks,
      zero telos law copies, no verdict delegation to a telos role; guardrails A/B
      honored (D6 v2).

## Master cutover contribution

None — city source-config repos, no AWS resources created or renamed. Runtime exposure
arrives only via source-sync at un-pause under the AGC-WO-CSC deploy WOs' gates. The 10
credential punch-list items + 2 capacity re-derivation flags + 5 evidence-vars
un-pause items (A2.10) ride the CSC program punch list, not the cutover.

## Telos pack topology (amended 2026-07-14 — D6 v2)

Tail amendment — BINDING. Owner ruling D6 v2 (telos-layer program pack-topology
ruling, 2026-07-14) fixes where telos-layer content may live; the merged
`GCD-WO-CSC-006` carries the matching tail section binding its template. These are
ADDITIVE constraints: nothing above is weakened or contradicted. The full constraint
is stated here — an executor reading only this WO needs nothing else:

1. **Template rows apply UNCHANGED — including the telos rows.** The fan-out applies
   `GCD-WO-CSC-006`'s template rows INCLUDING its telos-topology rows unchanged to the
   five cities: city imports of applicable telos packs (per each city's role mix) ride
   the same `.gc/system/packs/<name>` import surface as the binding itself — never a
   pack fork; telos content in a city is limited to the P1.5 delivery artifacts (the
   sha-pinned SYSTEM-TELOS snapshot + the telos-binding fragment) — never Matchpoint
   law inside pack files; no evaluator/judge fragment may delegate verdicts to a telos
   role — conformance verdicts stay in the single evaluator/judge lane.
2. **Guardrails (verbatim, BINDING):**
   "(A) packs carry primitives + sha-pinned POINTERS to the SYSTEM-TELOS snapshot, never a second copy of the law; (B) the monitoring pack emits telemetry/findings ONLY — conformance verdicts stay in the single evaluator/judge lane (GCD-WO-CSC-003 / GCD-WO-EVAL-001, shaped to blueprint ROL-5/6 pre-merge; no telos-specific judge role)."

## WO-CS v1 conformance (audited 2026-07-14 — Track C)

Track C audit-wave C-W1 amendment. Authorities: `master/generation-architecture/
IMPLEMENTATION-CHECKPOINT.md` §5 (C-2);
`Matchpoint-Platform/specs/patterns/SKILL-work-order-audit-and-authoring.md` v3.0.0 §1B.
ADDITIVE layer: nothing above is weakened — the merged GCD-WO-CSC-006 Binding template
stays NORMATIVE and the 2026-07-14 "Telos pack topology" tail section above remains
BINDING and untouched. Amended under the loop's build-phase PAUSE (ruling R3) with this
unit verified PENDING at 0 runs first-hand.

### WOC map (component → disposition)

| WOC | disposition |
|---|---|
| WOC-1 execution classification | UPGRADED in place (R-C2 live-tier terms) |
| WOC-2 deliverables + AC-named tests | in-body, verified |
| WOC-3 negative scope fence | in-body, verified complete: owners named (pilot defects → blocker referencing 006; capacity → aws-GasCity/NodePool, owner-flagged; secrets → operator, A1 §11; the 4 DEAD stubs + 2 exec-mon cities + the pilot = zero-commit fences) |
| WOC-4 static premises | ADDED — `## Premises (drift gate)` + `## Specs impact` below |
| WOC-5 runtime premises | ADDED below (`library-id: UNWRITTEN (Track B)`, ruling R2) |
| WOC-6 coordination declaration | in-body (co_repos preamble, A1 §8 closed set) — COMPLETED below (traversal set + CRD-7 lane + deploy surfaces) |
| WOC-7 policy defaults | in-body (template rows; P6 rule) + declaration below |
| WOC-8 seam probe / anchor record | ADDED — anchor record below (SURVEY DRIFT found and recorded; the delta table is stale in enumerable ways the WO's own re-verify duty absorbs) |
| WOC-9 pattern + telos pins | ADDED below |
| WOC-10 same-motion doc/index obligations | in-body (per-city runbooks, names files, Step-6 sweep transcript, punch-list PR section) + `## Specs impact` below; index motion N/A (no SPECS-INDEX anywhere in the set; Track B) |
| WOC-11 TCS declaration + schema law | ADDED below |
| Residue manifest (GEN-6) | ADDED below + acceptance fold (AC-T1) |

### Anchor re-verification record (WOC-8 — 2026-07-14; GasCity-Dev @ `e3a3a1673600`; co_repo heads recorded)

Current origin/main heads (all fetched first-hand): `demo-sandbox-…` @ `cb0c81b92fae`,
`product-enrichment-…` @ `318eec0be7ac`, `compatibility-orchestration-…` @ `c36445b3cf3a`,
`client-platform-…` @ `75fc23329ae7`, `analytics-…` @ `8e39f4d18c5c`; pilot
`vehicle-graph-city` @ `0ccf4007f2b8`. **SURVEY DRIFT RECORDED (premise re-pin, R-C3 —
the Step-0 "re-verify each co_repo against the delta table" duty absorbs all of it; the
template's parameterized form already tolerates it):**

1. **compat-orch rig roster CHANGED**: now SEVEN rigs — `Matchpoint-Compatibility-Engine`,
   `Matchpoint-Compatibility-Materialization`, `Matchpoint-Vehicle-Projection`,
   `Matchpoint-Estate-Foundation`, `Matchpoint-Platform`, `Matchpoint-Estate-Ops`,
   `Matchpoint-Product-Projection` (the surveyed `Matchpoint-Product-Compatibility`/`pc`
   and `Matchpoint-Sync-Orchestration`/`so` rigs are GONE — estate repo reshape). T2/T3/T4
   iterate EVERY rig found on disk (template rule), so the correct count at execution is
   whatever the worktree shows — the delta-table "×6" and the Step-3 rig-letter list are
   AUTHORING-TIME SURVEY VALUES, not law. P5 invariants derivation for this city covers
   the ON-DISK roster (CE/CM/MPP replace pc/so — derive from those repos' `specs/` +
   `AGENTS.md`).
2. **No `[workspace] max_active_sessions` cap exists in ANY of the five cities today**
   (the surveyed 24-caps on compat-orch/client-platform are gone). The P6 rule already
   conditions on presence ("if present") — with no cap present, the P6 comment-update
   actions are NO-OPS and the two "capacity flags" in Step 7 reduce to an owner-visible
   note that the caps were removed upstream; re-derive at execution.
3. **`template-fragments/` NOW EXISTS in all five cities**, each carrying
   `telos-binding.template.md` (P1.5 delivery). T5's "create if absent" executes as
   create-if-absent; the telos artifact is PRESERVED byte-identical in every city
   (guardrail A) — same rule as the pilot's audit (GCD-WO-CSC-006 AC-T3).
4. The pilot's as-built artifacts (fragments/tests/runbook/names file) are NOT yet on
   `vehicle-graph-city` main (wave 24 unmerged — correct at audit time); Step-0's
   pull-and-match gate remains the execution-time probe.

### Runtime premises (WOC-5)

`library-id: UNWRITTEN (Track B)` (ruling R2). Park-vs-repair per THIS WO's own text.

| # | premise (re-verify at Step 0) | runnable check | on failure |
|---|---|---|---|
| RP-1 | pilot artifacts post-wave-24 on vehicle-graph-city `origin/main` | `git -C <vehicle-graph-city> pull` + `ls tests/csc-resolved-names.env template-fragments/` | PARK (pilot is the clone source; absent = ordering premise broken) |
| RP-2 | resolved names == pilot's committed names file | 006's R1a–R1d re-run + diff vs `tests/csc-resolved-names.env` | PARK (WO text: "MUST match exactly (STOP on mismatch)") |
| RP-3 | co_repo survey re-verified (rig rosters, caps, dirs) | per-repo `grep -c '^\[\[rigs\]\]' city.toml` + dir listing vs the delta table + the drift notes above | REPAIR (fold drift into the PR with evidence — the WO's own duty) |
| RP-4 | post-wave-24 `gc` binary buildable | `make build` in the home worktree | REPAIR (loud SKIP-fail otherwise) |
| RP-5 | cities remain PAUSED | per the Cities-PAUSED Validation clause | PARK (standing policy + K1) |
| RP-6 | dead stubs untouched | the dead-stub grep battery over the five diffs | PARK (A1 §8 REJECT — closed edit set) |

### Coordination declaration (WOC-6)

co_repos (object-form, matches the harness ledger): the five cities in the preamble —
each `role: edit, test: true` because each city's config is edited AND carries its own
acceptance suite (validation traversal set = the five co_repo worktrees + the home
worktree for the `gc` build and discovery). `vehicle-graph-city` is READ-ONLY (clone
source — receives NO commits): per R-C1 that is a runtime-premise row (RP-1) + the
blocked_by ordering edge through GCD-WO-CSC-006, NOT a co-edit leg. CRD-7 lane:
**pre-declared co-edit legs (lane 2)** — grant provenance: owner ruling D10 + kit A1 §8
(F8) BY-NAME pin (the closed five-repo set); the harness CoordinatedMerge saga owns the
6-repo merge. Deploy surfaces: NONE touched (`live-tier: none`).
`register: UNWRITTEN (Track B)` (ruling R2).

### Policy defaults (WOC-7)

Pinned in-body as binding law: the merged-006-template-wins conflict rule, the P6
cap-preservation rule (comments only, values never), upstream defaults untouched,
sequential-per-city completion when a single run cannot hold all five (Step 1→5
independent; Step-6 sweep as the final gate — never interleave partial cities). No open
posture choice remains. A generator asking to confirm a stated default is a template
defect.

### Pattern + telos pins (WOC-9)

Telos pins: `GasCity-Dev/specs/TELOS.md` v3 @ `16026788515b` +
`Matchpoint-Platform/specs/patterns/SYSTEM-TELOS.md` v2 @ `08994e13e751` (each city
carries the P1.5 sha-pinned snapshot pointer — guardrail A; preserved byte-identical per
the anchor record). Catalog patterns: NONE pinned (no-stretch — mechanical template
application; consumed contracts are program contracts, see T3). No consumer stubs in any
repo of the set (pre-adoption; Track B).

### Test-contract declaration (WOC-11 — every row marked; unmarked = authoring-audit RED)

| tier | path class | proving test (path::name) or N/A + justification |
|---|---|---|
| T1 logical | decision logic | N/A + justification: mechanical template application — no decision logic authored; per-city divergence is caught by the Step-6 sweep, not computed |
| T1 logical | parity oracle (refactor-sensitive) | the Step-6 cross-city consistency sweep (5 repos + pilot: identical provider TOML / patch fields / name sets / vars; shared fragments content-identical to the pilot except the marked block; divergence = defect, fix the copy never the pilot) + the byte-preservation rule per city |
| T2 behavioral | happy | five cloned `csc-*.sh` suites × 5 cities (rig-iterating — EVERY rig asserted) + `gc config show --validate` + `gc lint .` × 5 |
| T2 behavioral | failure (full-spectrum via T4 negatives) | planted-RED self-check per city (bogus fragment name → FAIL) + dead-stub grep (returns NOTHING) + loud SKIP-fail on missing `$GC_BIN` |
| T2 behavioral | destructive | N/A + justification: source config only — nothing to destroy |
| T2 behavioral | partial-failure (forced single-leg) | N/A + justification: no multi-leg runtime in-WO; the 6-repo merge legs belong to the harness CoordinatedMerge saga; sequential-per-city completion bounds partial states (WOC-7) |
| T2 behavioral | zero-item (never a GREEN path) | rig-iterating assertions must cover EVERY rig found (multi-rig cities: every patch list asserted, not just the first — the compat-orch ×7 reality makes this row load-bearing); planted-RED per city; one city-specific invariant marker each (proves re-derivation, not blind cloning) |
| T3 contract | schema consumed/published | CONSUMES C9/C10/C11 names via the pilot's committed names file + home-worktree discovery — `$id`: the C9 seam is UNREGISTERED (gap recorded; register-first obligation FOLDED into the C9 authority WO GCD-WO-CSC-003 per supervisor ruling R1). PUBLISHES no data seam (config + doctrine prose) |
| T4 fixtures | pack import | N/A + justification: no fixture-pack substrate; tests assert against real materialized packs + real TOML per city (fixture-realism) |
| T5 integration | estate-E2E registration | N/A + justification: city-source repos; no estate same-diff suite rows (ecosystem debt) |
| T5 integration | requires-siblings | `GasCity-Dev` (home — binary + discovery), the five co_repos, `vehicle-graph-city` (READ-ONLY pilot clone source) (H3) |
| T6 live | live proof | N/A: live-tier `none` — live fleet validation is a NAMED un-pause follow-up (pilot first per standing policy, then fleet); gated on the 10 secret punch items + 5 evidence-vars items (A2.10), never an acceptance criterion here |

### Residue manifest (GEN-6 — silent residue = REJECT)

The implementer fills this table at close-out; ABSENCE of the table is the REJECT
condition (adopted verbatim via skill §1B):

| class | item | detail | vehicle / consumer |
|---|---|---|---|
| delivered | <deliverable> | <evidence pointer> | — |
| not-delivered | <item> | <reason> | <EXISTING vehicle — pending-WO amendment / owning lane> |
| known-gap | <gap> | <blast radius> | <owning-context lane per rule 7> |
| re-sweep | <obligation> | <verify-at-dispatch command> | <dispatcher premise check> |

`none` rows are stated explicitly. Vehicle mapping is mandatory — no "future WO" value
exists. Standing row candidates named in-body: 10 secret punch items, 5 evidence-vars
un-pause items (A2.10), the capacity note (per the anchor record: the surveyed caps no
longer exist — re-derive and surface honestly).

### Acceptance criteria — Track C additions (binding, additive)

- **AC-T1 (residue manifest):** the structured result carries the GEN-6 residue manifest
  above; every non-delivered/known-gap row maps to an EXISTING vehicle; silent residue =
  REJECT.
- **AC-T2 (same-motion specs impact):** the `## Specs impact` declaration below holds at
  merge (a false `none` is a reject — CONTRACT §5.5).
- **AC-T3 (telos artifact preservation):** every city's
  `template-fragments/telos-binding.template.md` is byte-identical before/after this WO's
  diff (guardrail A — the anchor record found the file present in ALL five cities).

## Premises (drift gate)

> premises-watermark: GasCity-Dev@0 + Matchpoint-Platform@79 (authored 2026-07-14)

| spec doc | version | sha256-12 | assumed fact |
|---|---|---|---|
| specs/TELOS.md | 3 | 16026788515b | home-repo telos card: business-agnostic upstream; MatchPoint doctrine lands ONLY city-side (D10 — the fan-out's whole premise) |
| Matchpoint-Platform::specs/patterns/SYSTEM-TELOS.md | 2 | 08994e13e751 | estate telos head; guardrail A (sha-pinned pointers, never law copies) binds every templated city |
| master/city-scaling-improvements/wo-authoring-kit.md | 1 | 68a95bd19427 | C11 fan-out row + A1 §8 (F8) BY-NAME closed edit set (5 cities; 4 stubs DEAD); A1 §11 credential pattern (sha-only lane — ungoverned master/ doc) |
| master/DOCTRINE-fixture-realism-and-lifecycle-seam-acceptance.md | 2 | 8ee5795d2e6d | binds the per-city test discipline (planted-RED, zero-item never green) + the `city-evidence-doctrine` clone content |

## Specs impact

none — deliverables are city-source config + doctrine fragments + runbooks + tests across
the five co_repos; no governed `specs/` doc anywhere in the set is invalidated (none of
the five cities has a `specs/` tree; GasCity-Dev content is untouched beyond this spec
file). No `specs/SPECS-INDEX.md` exists in any repo of the set (pre-SVA `@0` sentinel) —
no index motion. The per-city doc obligations (runbooks, names files, sweep transcript,
punch-list PR section) are named in-body and ride the diffs (WOC-10).
