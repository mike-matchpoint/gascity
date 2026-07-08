# Work Order: GCD-WO-CSC-008 — exec-city filing re-point: codegen-work-filer + publish emitter emit `ComponentErrorRaised.v1` to the error-intake bus (RepoBugReported.v1 becomes overseer-only)

NOTE: this WO is long — read the FULL file at
`specs/agent-work-orders/GCD-WO-CSC-008-exec-city-filing-repoint.md` in your worktree
before implementing. This WO is **the SOLE exec-city change in the CSC program** (kit
ADDENDUM A1 §3; D5 otherwise stands — exec-monitoring CITY REPOS are untouched; the change
lands in the upstream `execution-city-operations` pack only).

Execution classification: Dev-only pack content in the GasCity platform fork (prompt
templates + emitter/builder shell scripts + a mirrored contract schema + pack-script
tests; no Go changes beyond none, no AWS mutation — the `aws` CLI path runs only under the
fake-CLI test harness). `boundary=dev`, **wave 23** (CSC program band 23/24/25),
`blocked_by` `Matchpoint-Platform::PAR-WO-CSC-001-error-emission-package-and-envelope`
(**same-wave, cross-repo — an apply_deps DIRECT-WRITE edge per kit A1 §4**; PAR-WO-CSC-001
merges before this WO dispatches and its merged contract files are this WO's copy source).

> **Provenance (binding):** CSC program contract authority
> `master/city-scaling-improvements/wo-authoring-kit.md` — **ADDENDUM A1 §3** ("NEW
> GCD-WO-CSC-008-exec-city-filing-repoint (wave 23, blocked_by PAR-WO-CSC-001):
> execution-city-operations pack — codegen-work-filer + `publish-cross-city-event.sh` emit
> `ComponentErrorRaised.v1` (investigation context attached as evidence refs) to the error
> bus instead of filing `RepoBugReported.v1` directly; incident-classifier text updated to
> match. This is the SOLE exec-city change in the program"), **A1 §2** (Overseer-Issue
> correlation threading), **K4-C1** (envelope contract — authority PAR-WO-CSC-001,
> imported here, never re-declared), **K3** (bus naming). Backlog:
> `master/city-scaling-improvements/gap-analysis-and-build-plan.md` §5 (A1-amended) + §6.
> Design record: `aws-GasCity/docs/investigations/2026-07-city-scaling-improvements.md`
> WS3 + Decision log **D11** ("only the overseer files `RepoBugReported.v1`; exec-city
> filings route through the overseer — their investigations attach as fast-path triage
> context; single closure path"), **D16/D17** (direct emission; only canonical envelopes
> reach the bus). C1 contract authority ON DISK:
> `Matchpoint-Platform/specs/agent-work-orders/PAR-WO-CSC-001-error-emission-package-and-envelope.md`
> (Matchpoint-Platform `origin/main` @ `28fd3d6fe23057506c0274d0af313494cc9b14f8` at its
> authoring) — its field literals are restated below as IMPORT CITATIONS. Process: root
> `SKILL-work-order-audit-and-authoring.md` §1.1–1.5.
> Verified at authoring (2026-07-08): `GasCity-Dev` `origin/main` @
> `c85d92cf0cfd1215be1467628d6fd2e06db46aae`; all file/line refs below verified at that
> SHA. Re-verify at execution — by then PAR-WO-CSC-001 (same wave) AND GCD-WO-EVAL-001
> (wave 18) are merged; copy contract artifacts from the MERGED trees, never from this
> file's restatements.
> Ledger stem: `GasCity-Dev::GCD-WO-CSC-008-exec-city-filing-repoint`.

## Goal

Execution-monitoring cities stop filing `RepoBugReported.v1` directly at code-generation
cities (the naive-routing gap D11/WS3 closes: the filer chooses `payload.repo`, but the
root-cause fix may live upstream in another domain). Instead they emit the estate error
envelope **`ComponentErrorRaised.v1`** — with their investigation attached as evidence
refs — onto the global error-intake bus, where the OVERSEER (AGC-WO-CSC-004/005, wave 24)
performs root-cause triage and is the only filer of `RepoBugReported.v1`. Clean end state,
all inside `examples/gastown/packs/execution-city-operations/`:

1. **`assets/scripts/publish-cross-city-event.sh`**: registers `ComponentErrorRaised.v1`
   (flat class, error-intake bus + producer event source from injected env);
   **hard-rejects `RepoBugReported.v1`** with a D11-citing error; `RepoChangeRequested.v1`
   lane byte-identical.
2. **`assets/scripts/build-component-error-envelope.sh`** (NEW): deterministic C1
   envelope builder — fingerprint composition, message truncation + secret-key redaction,
   typed evidence refs, schema validation — so no LLM ever hand-assembles wire fields
   (D17: only canonical envelopes reach the bus).
3. **`schemas/events/component-error-raised.v1.schema.json`** (+ example): a byte-verbatim
   MIRROR of the merged Platform contract source (provenance-pinned; parity-tested), per
   the pack's own mirror doctrine (`schemas/README.md`: "Any other copy … is a downstream
   mirror and must track these files").
4. **`agents/codegen-work-filer/prompt.template.md`**: the bug-filing lane re-pointed to
   the builder+emitter pair; Overseer-Issue correlation duty (A1 §2) added.
5. **`agents/incident-classifier/prompt.template.md`** +
   **`template-fragments/incident-taxonomy.template.md`** +
   **`template-fragments/codegen-handoff-contract.template.md`**: text updated to match
   (bug dispositions say "emit a component error (overseer routes it)"; the handoff
   contract's RepoBugReported section becomes the C1 emission contract with a pinned
   incident-class → `error_class` mapping).
6. **Pack tests** (extending the wave-18 `tests/run-tests.sh` suite from GCD-WO-EVAL-001,
   or creating it on the domain-handoff harness pattern if absent) + the ONE cross-pack
   test edit in `domain-handoff/tests/run-tests.sh` that tracked the old lane.

Why: D11 ratified a single closure path — exec-city agent filings route through the
overseer so cross-city validation and fingerprint-based closure happen in ONE ledger.
The exec city keeps its strongest asset (a real investigation: observed vs expected,
reproduction steps, evidence URIs) — that context now travels as C1 evidence refs and
message text, giving the overseer's Fargate triage a fast path (K5 mounts the envelope +
prefetched evidence).

**Contract relationships (import citations — nothing re-declared here):**
- `ComponentErrorRaised.v1` fields/enums/caps/fingerprint rule — **authority
  PAR-WO-CSC-001** (kit K4-C1). This WO consumes the JSON Schema ("shell producers
  consume the SCHEMA, not the Python package" — PAR-WO-CSC-001 Goal/consumers list) and
  restates literals below strictly as import citations with source paths.
- Bus name `MatchpointErrorIntake-EventBus-{Env}` — kit K3; the RESOURCE is built by
  `AGC-WO-CSC-004` (wave 24). This pack never names it: the hosting harness injects it as
  env (Non-Goals/R3).
- `Overseer-Issue: <issue-id>` marker grammar + `overseer_issue_id` — authority
  PAR-WO-CSC-001 Step 10 (regex `^Overseer-Issue: (?P<issue_id>[A-Za-z0-9_.:-]+)$`) and
  kit A1 §2; cited, not redefined.
- `RepoBugReported.v1` schema — UNCHANGED in this pack (the receiving codegen-city
  adapter still consumes it — from the overseer; D11). Only the EMIT path is closed.

## Dependencies

- **Blocked by:** `Matchpoint-Platform::PAR-WO-CSC-001-error-emission-package-and-envelope`
  (wave 23, same-wave direct-write edge, A1 §4). At this WO's execution its merged tree
  provides the copy source for the schema mirror:
  `Matchpoint-Platform/packages/contracts/schemas/events/component-error-raised/v1.json`
  (+ sibling `pin.json` with `bundledSchemaSha256`). **STOP-gate:** if that file is absent
  from Matchpoint-Platform `origin/main` at execution, STOP and raise a structured blocker
  — never hand-write the schema from this WO's restatements.
- **Blocked by (transitive program gate, no ledger edge needed):**
  `GasCity-Dev::GCD-WO-EVAL-001-generic-eval-execution-primitives` (wave 18) will already
  be merged; it creates this pack's `tests/run-tests.sh` + `Makefile` `test-packs` entry
  and pins the pack's generic-ness grep gate. Step 6 discovers-and-extends rather than
  assumes (fallback: create the suite on the `domain-handoff/tests/run-tests.sh` +
  `tests/fakes/{gc,aws}` precedent).
- **Consumed by:** `aws-GasCity::AGC-WO-CSC-004-error-intake-and-issue-ledger` (wave 24 —
  intake schema-validates C1 events and quarantines malformed ones; its aws-GasCity
  schema mirror parity-tests against the same Platform source this pack mirrors) and
  `AGC-WO-CSC-005` (triage consumes the envelopes). No code import in either direction.
- **Runtime env contract (pinned HERE; provisioning is aws-GasCity-side):** R3 defines
  the `GASCITY_ERROR_*` env this pack reads. Injection into hosted exec-city pods is an
  aws-GasCity render/deploy duty at un-pause (flagged to the program seam round — see
  Risks). Nothing in this WO requires the env to exist: every runtime path fails loud
  when unset, and all tests inject fakes.
- **Cities PAUSED (standing policy + kit K1):** this WO verifies all GasCity-in-AWS
  remains paused (zero-replica / suspended) before declaring success — concretely: no
  hosted interaction of any kind (no kubectl, no AWS API, no `gc` daemon/city/session
  start, locally or hosted); the emitter's live `aws events put-events` arm is exercised
  ONLY against the fake `aws` CLI. Live drills are only ever the vehicle-graph pilot,
  explicitly named, re-suspend after — **this WO names NO live drill**; the first live C1
  emission is part of AGC-WO-CSC-005's T3 full-loop proof (kit A1 §14) under its own
  gates.
- **Fixture-realism doctrine** (owner-ratified, REJECT-level):
  `master/DOCTRINE-fixture-realism-and-lifecycle-seam-acceptance.md` — the example
  envelope and test fixtures are FULL canonical envelopes (all 22 keys, real-shaped ARNs
  of the `317250221986`/`us-west-2` account shape, real 64-hex fingerprints computed by
  the pinned rule, RFC3339 timestamps); zero-item runs never green (planted-RED cases
  below).

## Non-Goals

Bounded-context REJECT rules (kit K2, GasCity-Dev row) restated, plus WO-specific ones:

- **NO exec-monitoring CITY REPO changes** (D5: `vehicle-graph-execution-monitoring-city`
  etc. untouched). This WO is upstream pack content only; cities pick it up via the
  runtime image / source machinery under OTHER WOs' gates.
- **NO code-generation-city or codegen-support/gastown pack changes**; NO city-repo
  `city.toml` edits.
- **NO re-declaration of any C1 field, enum literal, cap, or the fingerprint/marker
  grammar** — PAR-WO-CSC-001 is the single author (SKILL §1.5). Every literal below
  carries its import citation; the schema mirror is a byte-verbatim COPY.
- **NO `RepoBugReported.v1` contract changes**: `schemas/events/repo-bug-reported.v1.schema.json`,
  its example, and the receiving-adapter routing prose stay byte-identical (the overseer
  becomes its only producer — D11; the receiving lane in codegen cities is unchanged).
- **NO `RepoChangeRequested.v1` behavior change** — capability/change requests are not
  errors; that lane (ownership-index resolution included) stays byte-identical, proven by
  test.
- **NO Go changes.** The pack's `//go:embed pack.toml all:agents template-fragments
  all:schemas all:assets` directive (`embed.go:8`) already covers every file this WO adds
  (`agents/`, `assets/scripts/`, `schemas/`, `template-fragments/`). If GCD-WO-EVAL-001's
  merged embed line differs (it adds `formulas`), the covering terms above still hold —
  verify, don't edit.
- **NO hand-rolled `aws events put-events` anywhere** — the pack's single-emitter doctrine
  stands; the builder script builds JSON only, the emitter publishes.
- **NO business-domain VALUES in the pack**: bus names, event sources
  (`matchpoint.<domain-slug>`), domains, ARNs, log groups arrive via injected
  `GASCITY_ERROR_*` env (R3), exactly like `GASCITY_EVENT_BUS` today. The ONE sanctioned
  literal exception: the mirrored contract schema's `$id`
  (`https://contracts.matchpointintelligence.com/...`) — a verbatim mirror per the pack's
  own mirror doctrine; Step 6 registers it as an explicit, justified exemption in the
  generic-ness grep gate (GCD-WO-EVAL-001) rather than weakening the gate.
- **NO new closure/lifecycle event emission** from exec cities (CodeReleaseDeployed /
  RepoChange* lifecycle families are AGC-WO-CSC-001/006B producers; the ledger listener is
  AGC-WO-CSC-004).
- **NO secrets, tokens, account IDs, or live endpoints** committed (fixture ARNs use the
  canonical dev-account SHAPE as test data only, per fixture-realism).

## Architecture Links

- `master/city-scaling-improvements/wo-authoring-kit.md` — A1 §2/§3/§4, K4-C1, K3, K5
  (overseer consumes `/input/envelope.json` + prefetched evidence — why refs matter), K6.
- `master/city-scaling-improvements/gap-analysis-and-build-plan.md` §5/§6.
- `aws-GasCity/docs/investigations/2026-07-city-scaling-improvements.md` — WS3 findings
  ("the **filer chooses `payload.repo`** … **This is the naive-routing gap**"), proposed
  design §§1–5, D11/D16/D17.
- `Matchpoint-Platform/specs/agent-work-orders/PAR-WO-CSC-001-error-emission-package-and-envelope.md`
  — Steps 2–5 (wire constants, model, fingerprint, redaction/truncation), Step 9 (schema),
  Step 10 (marker grammar). THE contract authority.
- This repo (all verified @ `c85d92cf`):
  - `examples/gastown/packs/execution-city-operations/assets/scripts/publish-cross-city-event.sh`
    (309 lines) — the emitter under edit; R1 quotes the exact replaced lines.
  - `agents/codegen-work-filer/prompt.template.md` (129 lines) + `agent.toml` (selector on
    `gc.routed_to`, codex one-shot pool ≤3 — UNCHANGED).
  - `agents/incident-classifier/prompt.template.md` (73 lines).
  - `template-fragments/codegen-handoff-contract.template.md` (define
    `execution-codegen-handoff-contract`) + `template-fragments/incident-taxonomy.template.md`
    (define `execution-incident-taxonomy`).
  - `schemas/README.md` (mirror doctrine + event registry) + `schemas/events/*` +
    `schemas/events/examples/*`.
  - `examples/gastown/packs/domain-handoff/tests/run-tests.sh` (fake `gc`/`aws` harness
    precedent; line 183 exercises the OLD RepoBugReported emit lane — the one cross-pack
    edit) + `tests/fakes/`.
  - `Makefile:122-124` (`test-packs`), `test/packlint/bd_show_jq_test.go` (scan rules the
    new shell must satisfy: `.[0].field` jq forms on `bd show --json`).

## Packages To Inspect

READ-first, in this order: the emitter script IN FULL; the filer prompt IN FULL; the
handoff-contract + incident-taxonomy fragments; `schemas/README.md` + both repo-event
schemas + examples; the classifier prompt; the domain-handoff test harness + fakes; the
MERGED Matchpoint-Platform contract source + pin (copy source); the MERGED wave-18
`tests/run-tests.sh` of this pack (if present) + its generic-ness gate implementation;
`agents/evidence-bundler/prompt.template.md` (where evidence URIs come from — READ ONLY).

## Required Inputs

**R1 — exact emitter lines replaced/extended** (quoted @ `c85d92cf`; re-anchor by content
if drifted):

- `publish-cross-city-event.sh:103` (schema registry case arm):
  `RepoBugReported.v1) SCHEMA_FILE="$SCHEMA_DIR/repo-bug-reported.v1.schema.json" ;;`
  → REPLACED by a hard reject (Step 1.2).
- `:110` usage prose in the filer prompt (`--event-type` is `RepoBugReported.v1` or
  `RepoChangeRequested.v1`) → superseded (Step 3).
- `:149-166` ownership-resolution case arm currently matching
  `RepoBugReported.v1|RepoChangeRequested.v1)` → narrowed to `RepoChangeRequested.v1)`
  only (body unchanged for that type).
- `:271-275` source selection (`EVENT_SOURCE_VALUE=` envelope vs flat) → extended with the
  error-lane override (Step 1.1).
- `:291-292` live-publish env requirements (`GASCITY_EVENT_BUS`, `AWS_REGION`) → bus
  resolution becomes lane-aware (Step 1.1); `AWS_REGION` requirement unchanged.

**R2 — C1 wire facts (import citations from PAR-WO-CSC-001; source step in parens):**
detail-type `ComponentErrorRaised.v1`; `source` = `matchpoint.<domain-slug>` (Step 2
`EVENT_SOURCE_PREFIX`); detail = the 22-key envelope, ALWAYS all keys present, optionals
explicit `null` (Step 3 serialization rule): `event_id, schema_version("1.0"),
occurred_at(RFC3339 UTC "Z"), environment(dev|prod), repo, domain, component,
component_arn, signal_source, severity, error_class, message, fingerprint,
correlation_id, causation_id, run_id, execution_arn, tenant_id, trace_refs{log_group,
log_stream, request_id?, xray_id?}, input_refs[](typed, discriminator "kind":
s3_uri{uri}|sqs{queue_arn,body_sha256}|eventbridge{archive,event_id}|ddb{table,key}),
evidence_prefix?, retry_state{attempt,max_attempts,will_retry,from_dlq}`. Enums
(closed; consumer-side extension is a seam blocker): `signal_source ∈ {lambda_failure,
dlq_arrival, sfn_catch, ecs_task_stopped, alarm_state, log_pattern, app_explicit}` — this
producer uses **`app_explicit` only**; `severity ∈ {low, medium, high, critical}`;
`error_class ∈ {validation, contract, dependency, timeout, throttle, data, logic, infra,
unknown}`. Caps: `message` ≤ 2048 UTF-8 bytes with truncation marker `...[truncated]`
(Step 5 `TRUNCATION_MARKER`); entry ≤ 32,768 bytes; `input_refs` ≤ 25; S3 refs are
`s3://` ONLY (presigned https is a credential leak — Step 3 `S3InputRef`). Fingerprint =
`sha256 hex of "\n"-join(repo, component, error_class, normalized_signature)` (Step 4 —
the JOIN rule is contract; the NORMALIZATION function lives only in the Python library,
so this producer supplies an already-normalization-stable signature, R4). Redaction
denylist key classes (Step 5 `DENYLIST_KEY_RE`): password/passwd/secret/token/
authorization/auth-header/api-key/x-api-key/credential(s)/cookie/set-cookie/session-id/
private-key/access-key-id → value replaced with `<redacted>`.

**R3 — injected env contract (pinned by THIS WO; mirrors the existing
`GASCITY_EVENT_BUS`/`GASCITY_SOURCE_CITY` harness-injection pattern, script lines 40-48):**

| Env | Meaning | Example value (NEVER in pack files) |
|---|---|---|
| `GASCITY_ERROR_INTAKE_BUS` | error-intake EventBridge bus name | `MatchpointErrorIntake-EventBus-Dev` (kit K3; resource by AGC-WO-CSC-004) |
| `GASCITY_ERROR_EVENT_SOURCE` | wire `Source` for C1 entries | `matchpoint.vehicle-graph` (K4-C1 grammar) |
| `GASCITY_ERROR_ENVIRONMENT` | `environment` field | `dev` |
| `GASCITY_ERROR_DOMAIN` | `domain` field (slug, `^[a-z][a-z0-9-]*$`) | `vehicle-graph` |
| `GASCITY_ERROR_COMPONENT_ARN` | fallback `component_arn` when the incident carries none: the exec-city observer's own identity ARN | (harness-injected) |
| `GASCITY_ERROR_LOG_GROUP` / `GASCITY_ERROR_LOG_STREAM` | `trace_refs` for app_explicit filings (stream defaults `$HOSTNAME`) | (harness-injected) |

All REQUIRED at emit time except the two with stated fallbacks; the builder dies loudly
naming the missing var. Provisioning duty: aws-GasCity exec-city pod/env render (surfaced
as a program seam item — Risks).

**R4 — signature-slug rule (normalization-stable producer input).** The builder takes
`--signature <slug>` and ENFORCES grammar `^[a-z][a-z ._-]{0,255}$` **with no digit
runs** (reject otherwise). Rationale (cited): PAR-WO-CSC-001 Step 4's normalization
NFKC-lowercases, collapses whitespace, strips volatile tokens (uuid/ts/arn/url/hex/digit
runs → placeholders) and truncates at 256 — a slug in this grammar is a FIXED POINT of
that function, so shell-side fingerprints equal library-side fingerprints for the same
inputs without reimplementing `normalize_signature` (which remains Python-only authority).
Slug construction (deterministic, pinned): `<incident_class>` + `" "` + `<component>`,
lowercased, characters outside the grammar mapped to `-`, digit runs mapped to `-`,
whitespace collapsed, truncated to 256. Example:
`adapter_or_contract_bug graph-build-validation` →
`adapter_or_contract_bug graph-build-validation` (underscores allowed).

**R5 — incident-class → `error_class` mapping (pinned table; the classifier/filer follow
it verbatim — one primary class each, single documented exception):**

| `execution-incident-taxonomy` class | C1 `error_class` |
|---|---|
| `execution_runtime_failure` | `logic` — EXCEPTION: `timeout` when the evidence shows a timeout/deadline terminal |
| `deterministic_service_bug` | `logic` |
| `adapter_or_contract_bug` | `contract` |
| `event_ingest_or_delivery_failure` | `dependency` |
| `artifact_contract_violation` | `contract` |
| `prompt_or_eval_failure` | `logic` (only when filed as a code error; eval-triage routes stay primary) |
| `domain_data_or_evidence_gap` | `data` |
| `domain_schema_or_policy_question` | — not filed (owner/mayor route) |
| `code_capability_gap` | — not an error: `RepoChangeRequested.v1` lane (unchanged) |
| `operator_or_secret_config_blocker` | `infra` |
| `duplicate_or_known_issue` | — not filed |
| `unknown_needs_evidence` | `unknown` (only after the evidence-bundler route is exhausted) |

## Implementation Steps

**Step 0 — Contract-source gate.** In the Matchpoint-Platform sibling checkout (or its
`origin/main` via the loop's context bundle), locate the MERGED
`packages/contracts/schemas/events/component-error-raised/v1.json` + `pin.json`. Record
`shasum -a 256` of `v1.json` and confirm it equals `pin.json:bundledSchemaSha256`. STOP
if absent/mismatched (Dependencies).

**Step 1 — emitter (`assets/scripts/publish-cross-city-event.sh`).**

*1.1 Error lane (additive, generic).* Register the event type in the schema case (R1
line 103 region):

```bash
ComponentErrorRaised.v1) SCHEMA_FILE="$SCHEMA_DIR/component-error-raised.v1.schema.json" ;;
```

The C1 schema does not require `process_slug`, so the existing class derivation
(`:116-119`) yields `EVENT_CLASS="flat"` — payload file IS the EventBridge detail
(no city envelope; correct for C1). Add lane-aware bus + source resolution: for
`EVENT_TYPE = ComponentErrorRaised.v1`, `PUBLISH_BUS="${GASCITY_ERROR_INTAKE_BUS:-}"`
(die naming the var when unset) and `EVENT_SOURCE_VALUE="${GASCITY_ERROR_EVENT_SOURCE:-}"`
(die when unset) — overriding the flat-lane defaults at `:274` and the `:291`/`:300` bus
uses (introduce `PUBLISH_BUS` defaulting to `$GASCITY_EVENT_BUS` for all other types so
every non-C1 path is byte-equivalent). Skip the ownership-index block for C1 (no
`target_city` concept — intake routes centrally); skip envelope assembly (flat). The
dry-run block prints the same fields plus `bus=$PUBLISH_BUS`.

*1.2 Close the RepoBugReported emit lane.* Replace the `:103` case arm with:

```bash
RepoBugReported.v1) die "RepoBugReported.v1 is filed exclusively by the overseer (CSC ruling D11); execution cities emit ComponentErrorRaised.v1 via build-component-error-envelope.sh + this emitter" ;;
```

and narrow the `:149-166` ownership case arm to `RepoChangeRequested.v1)` only (its body
unchanged). Note: `--schema-file` bypass must NOT resurrect the lane — add an explicit
guard immediately after argument parsing: if `EVENT_TYPE = RepoBugReported.v1`, die with
the same message regardless of flags.

*1.3 Header comment* (lines 1-48): update the event-class prose to name the third lane
("error events (component → estate error-intake bus): flat C1 envelopes, bus/source from
`GASCITY_ERROR_INTAKE_BUS`/`GASCITY_ERROR_EVENT_SOURCE`") and the RepoBugReported
overseer-only rule.

**Step 2 — builder (`assets/scripts/build-component-error-envelope.sh`, NEW).**
Deterministic; `set -euo pipefail`; jq + shasum/sha256sum only; no `aws`, no `gc`
mutations. Interface (pinned):

```
build-component-error-envelope.sh \
  --repo <repo> --component <slug> --error-class <enum> --severity <enum> \
  --signature <slug per R4> --message-file <path> \
  [--correlation-id <id>] [--causation-id <bead-id>] [--run-id <id>] \
  [--execution-arn <arn>] [--tenant-id <id>] [--component-arn <arn>] \
  [--evidence-uri s3://... ]... [--evidence-prefix s3://...] \
  --out <envelope.json>
```

Behavior (each rule cites its R2 authority):
1. Validate enum args against the R2 literal sets (die on any other value — closed
   enums); validate `--signature` per R4 (die on digit runs / bad chars).
2. `event_id` = uuid (reuse the emitter's `gen_uuid` shape); `occurred_at` =
   `date -u +%Y-%m-%dT%H:%M:%SZ` (RFC3339 UTC, seconds precision — schema `format:
   date-time` satisfied); `environment`/`domain` from R3 env (die when unset);
   `component_arn` = `--component-arn` else `$GASCITY_ERROR_COMPONENT_ARN` (die if both
   unset); `signal_source` = `"app_explicit"` const.
3. `message`: read `--message-file`; apply denylist redaction — one `sed -E`/`perl -pe`
   pass implementing `(<R2 key classes>)(\s*[:=]\s*)(\S+)` → `\1\2<redacted>`,
   case-insensitive (behavioral parity with PAR-WO-CSC-001 Step 5; the Python
   `redact_text` remains the authority — cite in a comment); then UTF-8-safe truncate to
   2048 bytes INCLUDING the appended `...[truncated]` marker when over (never split a
   codepoint: truncate with `iconv -c`-safe byte cut then strip a trailing partial
   sequence, or python3 one-liner when available — pick ONE implementation and test the
   multi-byte boundary case).
4. `fingerprint`: `printf '%s\n%s\n%s\n%s' "$REPO" "$COMPONENT" "$ERROR_CLASS" "$SIGNATURE"
   | sha256` (the C1 join rule, K4-C1/PAR-WO-CSC-001 Step 4 — note the join has NO
   trailing newline: `"\n".join` of four parts = three separators; the printf above is
   exact).
5. `correlation_id`: `--correlation-id` when given (the filer passes
   `overseer_issue_id` — A1 §2) else a fresh uuid. `causation_id`/`run_id`/
   `execution_arn`/`tenant_id`: pass-through else `null`.
6. `trace_refs`: `{log_group: $GASCITY_ERROR_LOG_GROUP (die unset), log_stream:
   ${GASCITY_ERROR_LOG_STREAM:-$HOSTNAME}, request_id: null, xray_id: null}`.
7. `input_refs`: each `--evidence-uri` MUST match `^s3://\S+$` (die otherwise — pointers
   only, no https) → `{kind:"s3_uri", uri:...}`; cap at 25 (FIRST 25, warn on drop).
   `evidence_prefix`: `s3://`-validated else `null`.
8. `retry_state`: `{"attempt":1,"max_attempts":1,"will_retry":false,"from_dlq":false}`
   const (one-shot app-explicit filing).
9. Assemble the full 22-key object with jq (`-n` + args; ALL keys always present,
   optionals explicit `null` — R2 serialization rule); validate against
   `$SCHEMA_DIR/component-error-raised.v1.schema.json` using the emitter's exact
   `validate_detail` approach (python3+jsonschema preferred, jq required-field fallback);
   assert byte size ≤ 32,768; write `--out`.

**Step 3 — filer prompt (`agents/codegen-work-filer/prompt.template.md`).** Surgical
replacement of the bug lane; the change surface quoted:

- Role section (lines 23-27): append: "Concrete code DEFECTS are not filed at a
  code-generation city anymore: you emit `ComponentErrorRaised.v1` to the estate
  error-intake bus and the OVERSEER investigates root cause and routes the fix
  (single closure path). Capability/change requests still file
  `RepoChangeRequested.v1` directly."
- Filing Decision (lines 42-50): the `payload.repo`-in-ownership-index bullet becomes
  conditional ("for `RepoChangeRequested.v1` only"); add bullets: "for component errors:
  the incident classification (class, severity, affected surface) is present; evidence
  URIs are `s3://` object URIs where available; `repo` names the repo that EXPERIENCED
  the defect — an observation, NOT a routing verdict (the overseer decides ownership)."
- Event Bus Discipline + recipe (lines 52-116): keep the emitter-only doctrine sentence
  and the pack-resolution loop (lines 80-98) verbatim; replace the extract-and-publish
  recipe with the two-lane version. Bug lane (pinned recipe):

```bash
# Component-error lane (handoff bead metadata: event_type=ComponentErrorRaised.v1
# plus the incident fields written by the classifier).
BEAD_JSON=$(gc bd show "$GC_BEAD_ID" --json)
META() { printf '%s' "$BEAD_JSON" | jq -r "(.[0].metadata // .metadata).$1 // empty"; }
INCIDENT_CLASS=$(META incident_class); SEVERITY=$(META severity)
REPO=$(META repo); COMPONENT=$(META component)
OVERSEER_ISSUE_ID=$(META overseer_issue_id)   # A1 §2 threading; empty when unrouted
MSG_FILE="${TMPDIR:-/tmp}/component-error-msg.$$.txt"
printf '%s' "$BEAD_JSON" | jq -r '(.[0].metadata // .metadata).investigation_summary // (.[0].description // .description) // empty' > "$MSG_FILE"
ENV_FILE="${TMPDIR:-/tmp}/component-error-envelope.$$.json"
ARGS=( --repo "$REPO" --component "$COMPONENT"
       --error-class "<from the pinned mapping table>" --severity "$SEVERITY"
       --signature "<slug per the pinned rule>" --message-file "$MSG_FILE"
       --causation-id "$GC_BEAD_ID" --out "$ENV_FILE" )
[ -n "$OVERSEER_ISSUE_ID" ] && ARGS+=( --correlation-id "$OVERSEER_ISSUE_ID" )
# one --evidence-uri per s3:// URI in the bead's evidence list
"$BUILD" "${ARGS[@]}"
"$PUBLISH" --event-type ComponentErrorRaised.v1 --payload-file "$ENV_FILE" --dry-run
"$PUBLISH" --event-type ComponentErrorRaised.v1 --payload-file "$ENV_FILE"
```

  (`$BUILD` resolved beside `$PUBLISH` in the same pack-dir loop.) The prompt states the
  R5 mapping table and the R4 slug rule inline (they are operating instructions, not
  contract re-declarations — each cites this WO + PAR-WO-CSC-001). The
  `RepoChangeRequested.v1` recipe stays the current lines 68-108 flow verbatim.
- **Overseer-Issue marker duty (A1 §2):** add a Completion bullet: "When the handoff bead
  carries `overseer_issue_id`, record the marker line `Overseer-Issue: <issue-id>`
  (grammar authority PAR-WO-CSC-001 Step 10) in the claimed bead's completion notes and
  in any follow-up work item you file, so downstream git-surface writers carry it into
  PR/commit text." Completion section also swaps "event type" record line to name which
  lane fired.
- Line 110-116 (`--event-type is RepoBugReported.v1 or RepoChangeRequested.v1` + env
  list): rewrite to the two live lanes + the R3 env names.

**Step 4 — classifier + fragments (text-to-match, A1 §3).**

- `agents/incident-classifier/prompt.template.md:53-61` Follow-Up: "If code work is
  needed, route to `codegen-work-filer`." gains: "— a concrete DEFECT becomes a
  `ComponentErrorRaised.v1` emission (the overseer routes the fix); a missing CAPABILITY
  becomes a `RepoChangeRequested.v1` filing. Stamp the routed bead's metadata with
  `incident_class`, `severity`, `repo`, `component`, an `investigation_summary`, and the
  evidence URIs — the filer's envelope is built from exactly those fields."
- `template-fragments/incident-taxonomy.template.md` Disposition choices: "file a codegen
  bug" → "emit a component error (overseer-routed root-cause fix)"; the change-request
  and other lines unchanged.
- `template-fragments/codegen-handoff-contract.template.md`: bump "Contract version:
  1.0" → 1.1 with a dated note; replace the `RepoBugReported.v1` intent bullet +
  payload section with a "Component errors (defects)" section: the C1 emission contract
  (builder+emitter pair, R2 field summary AS IMPORT CITATION naming PAR-WO-CSC-001 and
  the pack's mirrored schema as ground truth, the R5 mapping table, the R4 slug rule,
  the repo-is-observation-not-verdict rule, the A1 §2 marker duty). Keep: the
  schema-wins sentence, the `RepoChangeRequested.v1` payload section, the no-route-fields
  rule and ownership-index prose FOR the change-request lane; state explicitly that
  `RepoBugReported.v1` remains a RECEIVED contract (overseer → codegen city) whose schema
  lives unchanged in this pack.

**Step 5 — schema mirror + example + README.**

- Copy the merged Platform `v1.json` byte-verbatim to
  `schemas/events/component-error-raised.v1.schema.json` (Step 0 source). Do not edit a
  byte — `$id` stays the Platform contract id (the mirror doctrine already governs:
  schemas/README.md "Any other copy … is a downstream mirror and must track these files").
- `schemas/events/examples/component-error-raised.v1.example.json`: one FULL envelope
  (all 22 keys; `signal_source":"app_explicit"`; realistic `s3://` input_refs; a
  fingerprint actually computed by the R4/Step-2.4 rule from the example's own
  repo/component/error_class/signature — recompute in a test, never eyeball).
- `schemas/README.md`: register the event under "## Events" (flat class, error-intake
  bus, producer = execution cities via the builder+emitter; consumer = estate error
  intake); add a "Mirror provenance" line: source path in Matchpoint-Platform + the
  pin sha256 recorded at copy time + "parity: tests recompute this file's sha256 and
  compare to the recorded value; refresh requires re-copying from the Platform source";
  update the RepoBugReported paragraph with the overseer-only producer note.

**Step 6 — tests.** Locate the merged wave-18 suite
(`examples/gastown/packs/execution-city-operations/tests/run-tests.sh`, from
GCD-WO-EVAL-001). If present: extend it and reuse its fakes; if absent (fallback):
create it on the domain-handoff harness pattern (`tests/fakes/gc`, `tests/fakes/aws`
capture-file idiom) and add it to `Makefile` `test-packs`. Cases (each is a named
`CURRENT_TEST` block; suite fails RED if any case body runs zero assertions):

1. *emitter: RepoBugReported.v1 rejected* — publish with the old example payload →
   non-zero exit; stderr contains "overseer" and "D11"; also rejected when `--schema-file`
   is passed (bypass guard).
2. *emitter: RepoChangeRequested.v1 lane unchanged* — dry-run with its example payload +
   a fixture `GASCITY_CODEGEN_OWNERSHIP_JSON` succeeds with byte-identical dry-run output
   fields vs the pre-change contract (ownership resolution, envelope class, idempotency
   key shape).
3. *emitter: C1 flat publish* — a valid builder-produced envelope +
   `GASCITY_ERROR_INTAKE_BUS`/`GASCITY_ERROR_EVENT_SOURCE` set → fake `aws` captures ONE
   entry with `EventBusName` = the error bus, `Source` = the injected source,
   `DetailType = ComponentErrorRaised.v1`, `Detail` parsing back to the input envelope;
   unset bus env → dies naming `GASCITY_ERROR_INTAKE_BUS`.
4. *builder: golden envelope* — pinned inputs (repo/component/class/severity/signature/
   message/evidence URIs + fixture env) → output validates against the mirrored schema;
   all 22 keys present; fingerprint equals the pinned sha256 (precomputed in the fixture
   — regression pin); `correlation_id` echoes a supplied `--correlation-id`.
5. *builder: guards* (planted-RED battery) — digit-run signature → reject; bad
   enum → reject; https evidence URI → reject; 26 evidence URIs → 25 kept + warning;
   >2KB message → truncated, ends with `...[truncated]`, ≤2048 bytes, still valid UTF-8
   (multi-byte boundary fixture); `password=hunter2` in message → `<redacted>`; missing
   `GASCITY_ERROR_DOMAIN` → dies naming it.
6. *schema mirror parity* — recompute sha256 of the mirrored schema file == the value
   recorded in `schemas/README.md`; example validates against it (jsonschema when
   available, jq fallback).
7. *prompts/fragments repointed* — grep assertions: filer prompt contains
   `ComponentErrorRaised.v1` and `build-component-error-envelope.sh` and the
   `Overseer-Issue:` marker duty, and does NOT instruct publishing
   `--event-type RepoBugReported.v1`; handoff-contract fragment carries the mapping table
   and the received-only RepoBugReported note; taxonomy fragment disposition updated.
8. *RepoBugReported artifacts untouched* — `git diff --quiet <base> --
   'schemas/events/repo-bug-reported.v1.schema.json'
   'schemas/events/examples/repo-bug-reported.v1.example.json'` (byte-identical).
- **Generic-ness gate reconciliation:** run the merged gate
  (`grep -riE "matchpoint|enrichment|vehicle"` over the pack runtime surface, per
  GCD-WO-EVAL-001). Expected hits after this WO: ONLY the mirrored schema's `$id`/
  `description` lines (+ pre-existing sanctioned test/example content, e.g. the
  repo-bug-reported example's fixture values). Extend the gate's exclusion to exactly
  `schemas/events/component-error-raised.v1.schema.json` with a one-line justification
  comment ("verbatim downstream mirror of the Platform C1 contract; mirror doctrine,
  GCD-WO-CSC-008") — never a blanket exclusion. Record before/after gate output in the
  PR.
- **Cross-pack edit (the ONE domain-handoff change):**
  `examples/gastown/packs/domain-handoff/tests/run-tests.sh:178-192` ("publish: repo
  events still require the codegen ownership index") currently drives the emitter with
  `RepoBugReported.v1`. Preserve the test's INTENT by switching it to
  `RepoChangeRequested.v1` + its example payload (same missing-index assertion on
  `GASCITY_CODEGEN_OWNERSHIP_JSON`); the RepoBugReported rejection is covered by case 1
  in the ECO suite. No other domain-handoff file changes.
- New/edited shell obeys packlint sweeps (scan roots include `examples/`):
  `.[0].field` jq forms on `bd show --json` output, no deprecated `gc nudge` forms.

**Step 7 — full battery** (Validation) + PR.

## Git Workflow

Loop execution: branch `wo/GCD-WO-CSC-008-exec-city-filing-repoint` (or `polecat/$BEAD_ID`
under city execution) in GasCity-Dev; PR to `origin/main` on **`mike-matchpoint/gascity`**
(never upstream); never commit directly to `main`; `make setup` once for the pre-commit
hook. Single-repo WO (no co_repos). No force-push, no history rewrites.

## Test Coverage

Kit K6 discipline verbatim; every acceptance criterion names its backing test; all suites
run REAL pack scripts against fake CLIs (no mocks of the code under test, no network, no
LLM); zero-item runs never green (each `CURRENT_TEST` block asserts at least one explicit
expectation; the suite's summary asserts the expected case COUNT).

- **Pack-script tier** (`execution-city-operations/tests/run-tests.sh`, wired into
  `make test-packs`): Step-6 cases 1–8 including the planted-RED guard battery and the
  fingerprint regression pin.
- **Cross-pack tier**: `domain-handoff/tests/run-tests.sh` green with the intent-preserving
  swap (its other 100% of cases byte-untouched).
- **Go structural tier**: `go test ./internal/builtinpacks` green (embed round-trip with
  the new files); `GC_FAST_UNIT=1 go test ./test/packlint` green (jq-form + nudge-form
  sweeps over the new shell).
- **Repo battery**: `make build && make check`; `make test-packs`.

## Validation

- `make build && make check`, `make test-packs`, `go test ./internal/builtinpacks`,
  `GC_FAST_UNIT=1 go test ./test/packlint` — all green; transcripts
  (`{command, output_excerpt}`) in the PR.
- Step-0 contract-source evidence recorded: Platform source path, its `pin.json` sha,
  the recomputed sha of the mirrored file — all equal.
- Grep gates (run + record): `grep -rn "aws events put-events" examples/gastown/packs/
  execution-city-operations/ | grep -v publish-cross-city-event.sh` → empty (single
  emitter); `grep -rn "RepoBugReported" examples/gastown/packs/execution-city-operations/
  agents/` → only received-contract/rejection prose, zero publish instructions;
  generic-ness gate output with ONLY the sanctioned mirror exemption.
- **Cities-PAUSED clause:** all GasCity-in-AWS remains paused (zero-replica / suspended);
  this WO performed no cluster/AWS/city interaction (fake-CLI harnesses only; no `gc`
  daemon started); no live drill claimed. The first live C1 emission from an exec city is
  AGC-WO-CSC-005's T3 full-loop proof (A1 §14) at un-pause — never an acceptance
  criterion here.
- **Prod-gate defer:** nothing prod-shaped is created here (pack content; the bus is
  AGC-WO-CSC-004's, dev-first) — recorded per the standing policy.

## Acceptance Criteria

1. Emitter rejects `RepoBugReported.v1` (flag-bypass included) with the D11-citing
   message; RepoChangeRequested lane byte-equivalent — suite cases 1–2.
2. Emitter publishes C1 as a FLAT entry to `$GASCITY_ERROR_INTAKE_BUS` with
   `Source=$GASCITY_ERROR_EVENT_SOURCE`, schema-validated, failing loud on missing env —
   case 3.
3. Builder emits full 22-key canonical envelopes: pinned fingerprint join, R4 slug guard,
   2KB redact+truncate, `s3://`-only typed refs ≤25, const `app_explicit`/retry_state,
   correlation threading via `--correlation-id` — cases 4–5.
4. Schema mirror is byte-verbatim vs the merged Platform source with recorded+recomputed
   sha parity; example validates — case 6 + Step-0 evidence.
5. Filer prompt re-pointed (two lanes, builder+emitter recipe, R5 mapping, marker duty);
   classifier + taxonomy + handoff-contract fragments match; no prompt instructs a direct
   RepoBugReported publish — case 7.
6. RepoBugReported schema/example untouched; receiving-lane prose intact — case 8.
7. Generic-ness gate green with exactly the one justified mirror exemption — Step-6
   gate record.
8. Cross-pack domain-handoff suite green via the intent-preserving swap — cross-pack tier.
9. Full repo battery green; no city started; no AWS call outside fakes (cities PAUSED) —
   Validation record.

## Risks

- **Env injection has no aws-GasCity owner WO yet (SEAM FLAG):** R3 pins the
  `GASCITY_ERROR_*` contract, but injecting it into hosted exec-city pods is an
  aws-GasCity render duty not explicitly listed in AGC-WO-CSC-004's scope. Surfaced to
  the CSC seam round with the recommendation: fold into AGC-WO-CSC-004 (which owns the
  bus + its exports) or the un-pause punch list. Until injected, hosted filings fail
  LOUD at the builder (by design — no silent fallback to the old lane).
- **Marker-duty interpretation (SEAM FLAG):** A1 §2 assigns the `Overseer-Issue:` marker
  duty to this WO; exec-city filers write no PRs, so this WO implements it as
  correlation-id threading + marker lines in bead completion notes / follow-up items
  (Step 3). If the seam round wants a different git-surface carrier, it is a one-section
  prompt edit.
- **Fingerprint parity drift:** the shell join is byte-pinned (`printf` form, no trailing
  newline) and regression-pinned by test; the R4 fixed-point slug rule avoids ever
  needing `normalize_signature` in shell. Any future need for richer signatures routes
  through PAR-WO-CSC-001 (library), never a shell reimplementation.
- **Schema mirror staleness:** sha-parity test + README provenance make drift loud;
  AGC-WO-CSC-004's aws-side mirror parity-tests against the same source, so a Platform
  contract bump flags both mirrors.
- **Wave-18 suite shape unknown at authoring:** Step 6's discover-and-extend with a
  pinned fallback harness bounds the improvisation; if the merged suite's generic-ness
  gate is implemented differently than assumed (e.g. Validation-only prose, no test),
  implement the gate as a suite case with the same regex + exemption and say so in
  the PR.
- **Truncation edge cases (multi-byte):** one implementation chosen and boundary-tested
  (case 5); the marker constant is imported, not invented.

## Done Means

- [ ] Emitter: C1 lane live (flat, env-addressed), RepoBugReported hard-closed,
      RepoChangeRequested byte-equivalent; header prose updated.
- [ ] Builder script landed with the full guard battery; golden + planted-RED tests
      green.
- [ ] Schema mirror + example + README provenance landed; sha parity proven.
- [ ] Filer/classifier prompts + both fragments re-pointed; marker duty implemented.
- [ ] ECO pack suite (new/extended) + domain-handoff swap + packlint + repo battery all
      green; generic-ness gate green with the single justified exemption.
- [ ] Merged to `origin/main` on `mike-matchpoint/gascity` via PR from a `wo/`-class
      branch; nothing committed directly to `main`.
- [ ] No city started; live emission proof deferred to AGC-WO-CSC-005's T3 gate; both
      SEAM FLAGS surfaced in the PR description for the program seam round.

## Master cutover contribution

None — platform-fork pack content; no AWS resources created, renamed, or deleted; no CDK
identity surface (kit K1 prod-gate language not triggered). Runtime exposure reaches
hosted exec cities only via the AGC-WO-CSC-006A/B runtime-image/deploy lane at un-pause,
gated behind AGC-WO-CSC-004's bus existing and the R3 env injection landing aws-side.
The D11 doctrine artifact (only the overseer files `RepoBugReported.v1`) is enforced at
the transport seam by this WO and inherited by the estate cutover as-is.
