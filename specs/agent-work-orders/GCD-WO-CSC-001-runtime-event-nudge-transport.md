# Work Order: GCD-WO-CSC-001 — runtime event & nudge controller-API transport (`api:` events provider + typed nudge endpoints)

NOTE: this WO is long — read the FULL file at
`specs/agent-work-orders/GCD-WO-CSC-001-runtime-event-nudge-transport.md` in your worktree
before implementing; tail amendments are BINDING.

Execution classification: **Dev-only** (`boundary=dev`, **wave 23**, `blocked_by` = none).
Engine Go + generated-client + OpenAPI + k8s-provider projection changes inside this repo only;
no AWS resources, no deploy surface, no city runs. Master cutover contribution: **None
(platform repo, no AWS)** — see the section below.

> **Provenance (binding):** authored under the City-Scaling Improvements (CSC) program:
> `master/city-scaling-improvements/wo-authoring-kit.md` — **contract C6 (§A-TRANSPORT)
> names THIS WO as its authority** — plus kit K1–K7 and ADDENDUM A1 (A1 §4: the w23 chain
> `GCD-002←GCD-001` is an apply_deps direct-write);
> `master/city-scaling-improvements/gap-analysis-and-build-plan.md` §5 row 1 + §7 item 1
> (E1 transport pin, RESOLVED 2026-07-08) + §6 contract-authority table ("Event/nudge
> transport contract (Go seams, env switch, file fallback)" → GCD-WO-CSC-001); living
> record `aws-GasCity/docs/investigations/2026-07-city-scaling-improvements.md` (owner
> rulings D7 git-native B+A end-state; E1/E2 findings: `events.jsonl` is the one genuine
> EFS blocker, agent pods DO write the nudge queue).
>
> **Ledger stem (verbatim):** `GasCity-Dev::GCD-WO-CSC-001-runtime-event-nudge-transport`
>
> Verified against this repo's `origin/main` 2026-07-08 @
> `c85d92cf0cfd1215be1467628d6fd2e06db46aae` (read-only `git log -1 --format=%H`; matches
> the remote main pinned in the living doc). Re-verify at execution time and rebase onto
> current `origin/main` before assessing prerequisites.

## Goal

Give hosted (Kubernetes) Gas City agents a **network transport for the runtime event bus and
the deferred-nudge queue**, so that neither `.gc/events.jsonl` nor `.gc/nudges/state.json`
ever needs a shared (RWX/EFS) filesystem again — the single remaining architectural blocker
to git-native per-pod ephemeral workspaces (living doc WS1 D3 table: "`.gc/events.jsonl`
(runtime event bus) … **The one genuine blocker.**").

End state (all four are C6-pinned):

1. **New `api:<base-url>` events provider** implementing `events.Provider`
   (`internal/events/events.go:194-210`) over the controller's **EXISTING** typed endpoints
   — `POST /v0/city/{cityName}/events`, `GET /v0/city/{cityName}/events`, SSE
   `GET /v0/city/{cityName}/events/stream?after_seq=` — registered in
   `newEventsProviderForNameWithConfig` (`cmd/gc/providers.go:771-783`) beside the existing
   `exec:`/`fake`/`fail`/file schemes. `Record` is **bounded-timeout (250 ms-class)
   drop-with-stderr**, mirroring today's flock semantics (`recordFlockTimeout`,
   `internal/events/recorder.go:26-32`). `events.jsonl` becomes a **single-writer** file
   owned by the controller process on mayor-local disk (the small-EBS-PVC placement is
   AGC-WO-CSC-002 infra scope — see Non-Goals).
2. **`EventEmitRequest` gains a typed payload field** (+ optional explicit timestamp) so the
   API emit path carries what `gc event emit --payload` and bead hooks already produce
   (`cmd/gc/cmd_event_emit.go:119-125`) — documented typed-wire exception + registered-payload
   validation (both halves, pinned in Implementation Step 2).
3. **NEW typed controller endpoints for nudge-queue operations** — enqueue / claim / ack /
   withdraw (+ read) — and a `GC_NUDGES` provider seam mirroring `GC_EVENTS`, so agent pods
   stop writing `.gc/nudges/state.json` via NFS flock. `state.json` + `wake.sock` stay
   **mayor-local**; **bead shadows in Dolt remain the nudge durability layer** (unchanged
   machinery in `cmd/gc/nudge_beads.go`, relocated — not rewritten — by this WO).
4. **k8s provider projection + security pin:** agent pods get
   `GC_EVENTS=api:http://<controller-svc>:<port>` and `GC_NUDGES=api:http://<controller-svc>:<port>`
   projected at the adapter edge (stop stripping `GC_EVENTS` at
   `internal/runtime/k8s/pod.go:462`; follow the `projectedPodDoltEnv` pattern,
   `internal/runtime/k8s/pod.go:146-193`), plus an optional provider-managed Service in
   front of the controller TCP listener. Mutation exposure is opened via an
   **endpoint-class allowlist (event emit + nudge ops ONLY)** — **NEVER blanket
   `api.allow_mutations`** — with a NetworkPolicy posture pinned for the infra consumer.

**File mode stays the default and byte-identical when `GC_EVENTS`/`GC_NUDGES` are unset** —
non-k8s cities and the upstream (`gastownhall/gascity`) behavior are untouched.

## Dependencies

- **Blocked by: none** (wave-23 head unit for this repo).
- **Consumed by (import this WO's contracts, never re-declare):**
  - `GasCity-Dev::GCD-WO-CSC-002-ephemeral-workspace-mode` (same wave 23; direct-write DEPS
    edge per kit A1 §4) — consumes the transport so per-pod ephemeral workspaces have no
    shared-file event/nudge dependency.
  - `aws-GasCity::AGC-WO-CSC-002-mirror-and-ephemeral-workspaces` (wave 24) — renders the
    hosted wiring: controller Service (if not provider-ensured), NetworkPolicy manifests
    per the posture pinned here, mayor small-EBS PVC for `.gc` single-writer state, the
    agent-pod env contract, AND the site-level `[api]` values (`bind` non-localhost +
    `allow_mutation_classes = ["events_emit", "nudges"]`) into each city's `.gc/site.toml`
    derivation (kit A2.6) — this WO guarantees that config surface exists (Step 5b);
    AGC-WO-CSC-002 renders the values.
- **Repo gates:** `CONTRIBUTING.md` (fork/branch workflow, `make setup`, `make build &&
  make check`), `TESTING.md` (tier boundaries + sharded runners — read before writing any
  test), `engdocs/architecture/api-control-plane.md` + `engdocs/contributors/huma-usage.md`
  (MANDATORY before touching `internal/api/`, `cmd/gc/` event construction, or
  `internal/events/`), AGENTS.md invariants (typed wire, typed events, worker boundary,
  ZFC, no hardcoded roles).
- **Cities PAUSED** (standing policy + kit K1): this WO is engine code + repo-native tests
  only. No AWS-hosted city is started; live k8s validation is a named follow-up on the
  vehicle-graph pilot (AGC-WO-CSC-002's drill), never an acceptance criterion here.

## Non-Goals

Bounded-context REJECT rules (kit K2, restated verbatim for this repo): GasCity-Dev may
build **engine Go (transport, workspace mode), `examples/gastown/packs/*` content +
`.gc/system/packs` + `cmd/gc/.gc/system/packs` mirrors (keep in sync), packlint tests**; it
must NEVER contain **AWS-specific types in engine or upstream packs; business/domain
literals in codegen-support/gastown; breaking file-mode defaults (non-k8s cities untouched);
new Go role logic for pack behavior**.

Specifically out of scope here:

- **NO pack-content changes.** This WO is pure engine/transport. (Pack fragments — WIP-push
  cadence etc. — are GCD-WO-CSC-002.)
- **NO mirror/workspace changes**: `GC_CITY_RUNTIME_DIR` split, rig-from-mirror
  provisioning, no-PVC staging changes are GCD-WO-CSC-002 (C7-engine). Do not touch
  `internal/config/pack_fetch.go` or `initCityInPod` beyond what Step 6 names.
- **NO k8s manifests, CDK, or per-city rendering** — the mayor PVC, per-city NetworkPolicy
  objects, and any statically-rendered Service are `aws-GasCity::AGC-WO-CSC-002` (this WO
  pins their contract; the infra WO materializes it). No file under this repo may name an
  AWS account, region, ECR image, or MatchPoint city.
- **NO Dolt-backed event transport** — E1 resolved to controller-API; do not build the
  alternative.
- **NO new event types** and NO changes to event payload registry semantics
  (`internal/events/payload.go` `RegisterPayload`/`DecodePayload` stay as-is).
- **NO blanket mutation enablement**: do not set or default `api.allow_mutations = true`
  anywhere; do not widen the CSRF middleware; do not add mutation classes beyond the two
  pinned literals (`events_emit`, `nudges`). REJECT-level.
- **NO change to nudge delivery** (poller/dispatcher/`worker.Handle.Nudge()` injection
  paths): delivery already runs controller-side. Only the queue **state operations** gain a
  network seam.
- **NO removal of the file provider, flock, or rotation machinery** — the controller keeps
  using `events.FileRecorder` (now effectively single-writer); rotation, archives, and the
  `exec:` provider are untouched.
- Vehicles/business scope: not applicable — this repo is business-agnostic and must stay so.

## Architecture Links

- `engdocs/architecture/api-control-plane.md` — typed-wire law, Huma registration,
  documented `json.RawMessage` exceptions (§ the Event.Payload exception this WO extends to
  `EventEmitRequest`), genclient consumer registry (§2 — currently three consumers;
  this WO adds a fourth and MUST update that list).
- `engdocs/contributors/huma-usage.md` — operation registration, SSE registration,
  middleware (`api.UseMiddleware`) patterns.
- `internal/events/events.go:1-10` — events package charter (best-effort recording).
- `AGENTS.md` — "Typed wire", "Typed events", worker-boundary migration, ZFC.
- Kit C6 (§A-TRANSPORT) — the pinned design this WO implements; where this file and the kit
  conflict, the kit wins.

## Packages To Inspect (read before writing code)

| Path | Why (file:line evidence) |
|---|---|
| `internal/events/events.go` | `Event` struct (174-182), `Recorder` (186-188), **`Provider` (194-210)**, `TailProvider` (214-216), `Watcher` (220-230), `Discard` |
| `internal/events/recorder.go` | `FileRecorder` flock semantics (26-37, 214-254), `Watch` polling + `GC_EVENTS_POLL_INTERVAL` (484-519), rotation (341-458) |
| `internal/events/eventstest/conformance.go` | `RunProviderTests` — the conformance suite the new provider must pass (26) |
| `internal/events/exec/exec.go` | the existing alternate-transport provider — structural model for the new package |
| `internal/events/payload.go` | `RegisterPayload`, `DecodePayload`, `NoPayload` — emit-path payload validation |
| `internal/events/reader.go:17-25` | `Filter` (Type/Actor/Subject/Since/Until/AfterSeq/Limit) — client-side mapping table in Step 1 |
| `cmd/gc/providers.go:721-910` | provider-name resolution (`eventsProviderName`, `fastEventsProviderName` 747-757, `newEventsProviderForNameWithConfig` 771-783, `openCityEventsProviderWithConfig` 885-910) |
| `cmd/gc/cmd_event_emit.go` | `gc event emit` (payload flag 83, `doEventEmit` 108-129) — the agent-side producer |
| `cmd/gc/cmd_events.go` | existing genclient consumer for event read/stream — reuse its transport idioms |
| `internal/api/huma_handlers_events.go` | list (18-126), **emit (160-176)**, rotate, SSE stream (267-339) |
| `internal/api/huma_types_events.go` | `EventEmitRequest` (25-30), `EventStreamInput.resolveAfterSeq` (102-114) |
| `internal/api/convoy_event_stream.go:40-107` | `WireEvent`, `toWireEvent` — custom types pass through; corrupt registered payloads skip |
| `internal/api/supervisor_city_routes.go` | city-scoped registration (events at 207-221; nudge endpoints land beside them) |
| `internal/api/city_scope.go` | `cityGet/cityPost/cityRegister` helpers (127-179), `cityScopePrefix` (43) |
| `internal/api/huma_handlers_supervisor.go:157-199` | `newSupervisorHumaAPI`, CSRF middleware, **`humaReadOnlyMiddleware` (191-199) — the blanket gate this WO refines** |
| `internal/api/middleware.go` | mux-level gates, `isMutationMethod` (121-127) |
| `internal/api/client.go` + `internal/api/genclient/` | CLI client adapter + generated client (`doc.go` consumer registry; regen via `go generate ./internal/api/genclient`) |
| `cmd/gc/apiroute.go` | `apiClient` non-localhost read-only routing (54, 91) — context for the allowlist |
| `cmd/gc/controller.go:1325-1359` | standalone controller API server construction (`readOnly` at 1333, `NewSupervisorMux` at 1341) |
| `internal/config/config.go` | `APIConfig` (1713-1722), `EventsConfig` (1490-1498), mail-provider env precedent (`GC_MAIL`, providers.go:649-651) |
| `internal/supervisor/config.go:38` | supervisor-mode `AllowMutations` — thread the allowlist here too |
| `internal/nudgequeue/state.go` | `Item` (31-49), `State` (52-56), `WithState` (90-122), `StatePath`/`LockPath`/`WakeSocketPath` (145-175) |
| `internal/nudgequeue/waits.go` | `WithdrawWaitNudges` (17-24) — already importable; the withdraw endpoint calls it |
| `cmd/gc/cmd_nudge.go` | queue ops to RELOCATE: `claimDueQueuedNudgesMatching` (1238-1271), `enqueueQueuedNudgeWithStore` (1347-1430), `ackQueuedNudgesWithOutcome` (1436-1482), `releaseQueuedNudgeClaims` (1484-1520), `recordQueuedNudgeFailureDetailed` (1527+), list fns (1273-1341), recover/prune fns (1610-1691), `queuedNudge = nudgequeue.Item` alias (54-56), TTL consts (31-42) |
| `cmd/gc/nudge_beads.go` | bead-shadow durability: `ensureQueuedNudgeBead` (127), `markQueuedNudgeTerminal` (201), `openNudgeBeadStore` (36) |
| `cmd/gc/nudge_dispatcher.go:53` | `pingNudgeWakeSocket` — server-side wake after enqueue |
| `internal/api/wait_nudges.go` | existing thin API→nudgequeue bridge (8-10) — extend, don't fork |
| `internal/runtime/k8s/provider.go` | provider fields (44-73), env docs (88-115), `usesPersistentWorkspace` (309-311) |
| `internal/runtime/k8s/pod.go` | **`GC_EVENTS` strip (462)**, `buildPodEnvForRoot` (452-570), `projectedPodDoltEnv` (146-193), managed-host constants (21-22) |

## Required Inputs

- Repo at current `origin/main` (provenance SHA above; rebase + re-verify).
- `make setup` toolchain (Go, oapi-codegen via the repo's generate script, dashboard npm
  toolchain for `make dashboard-check`).
- NO cloud credentials, NO cluster, NO city runtime. k8s behavior is tested against the
  existing `k8sOps` fake seam (`internal/runtime/k8s/provider.go:292-307` `newProviderWithOps`)
  and `httptest` servers.

## Implementation Steps

Every step names its exact files. Where a signature is given, copy it VERBATIM.

### Step 1 — `api:` events provider package: `internal/events/apitransport/`

New package `internal/events/apitransport` (name pinned; do NOT name it `api` — avoids
confusion with `internal/api`). Files: `apitransport.go`, `watcher.go`,
`apitransport_test.go`, `conformance_test.go`, `testenv_import_test.go` (copy the
testenv-import guard idiom used by sibling packages, e.g. `internal/events/exec/`).

Scheme grammar (pinned): provider name `api:<base-url>` where `<base-url>` is an absolute
`http://` or `https://` URL **without** trailing slash and **without** path
(e.g. `api:http://gc-controller.vg-city.svc.cluster.local:9443`). Parse = everything after
the first `api:` prefix; reject (constructor error) on empty/relative/parse-failure or
non-empty `URL.Path`.

```go
// Package apitransport implements events.Provider over the controller's
// typed HTTP+SSE API. It is the hosted-mode replacement for multi-writer
// flock access to .gc/events.jsonl: agent-side processes emit and watch
// through the controller, which remains the single writer of the file.
package apitransport

// NewProvider returns an api-transport events provider.
// baseURL is the parsed remainder of the "api:<base-url>" provider name.
// cityName scopes every call to /v0/city/{cityName}/...; it is REQUIRED
// (constructor error when empty). stderr receives best-effort Record
// diagnostics, mirroring FileRecorder's contract.
func NewProvider(baseURL, cityName string, stderr io.Writer) (*Provider, error)
```

`Provider` implements `events.Provider` AND `events.TailProvider` (compile-time asserts:
`var _ events.Provider = (*Provider)(nil)` / `var _ events.TailProvider = (*Provider)(nil)`).
Transport: use `internal/api/genclient` (`genclient.NewClientWithResponses` — same
construction idiom as `internal/api/client.go:349-365`). This adds a **fourth legitimate
genclient consumer**: update `internal/api/genclient/doc.go` AND
`engdocs/architecture/api-control-plane.md` §2 in the same commit. Import direction is
clean (genclient imports only `oapi-codegen/runtime`; no cycle with `internal/events`).

Method contracts (pinned):

- `Record(e events.Event)` — POST `/v0/city/{c}/events` with the Step-2 body (type, actor,
  subject, message, payload, ts — payload/ts only when non-zero). Per-call budget: a pinned
  const `recordHTTPTimeout = 250 * time.Millisecond` (context deadline), rationale comment
  citing `recordFlockTimeout` parity. On ANY error (timeout, non-2xx, transport): ONE line
  to stderr (`events: api: record %s: %v` shape), then drop — never return, never retry,
  never block beyond the deadline. Set header `X-GC-Request: gc-events-apitransport`
  (CSRF middleware, `huma_handlers_supervisor.go:179-187`, requires a non-empty value on
  mutations).
- `List(filter events.Filter) ([]events.Event, error)` — GET `/v0/city/{c}/events` mapping
  server-side params `type=Filter.Type`, `actor=Filter.Actor`, `since` (from `Filter.Since`
  as a duration relative to `time.Now()` when non-zero — the wire takes Go durations, see
  `huma_types_events.go:21`), `limit` (see below). Fields the wire cannot express —
  `Subject`, `Until`, `AfterSeq` — are applied **client-side** after fetch using the exact
  predicate semantics of `internal/events/reader.go:29-47`; when any of the three is set,
  the request omits `limit` server-side and applies `Filter.Limit` after local filtering.
  Wire→domain mapping: rebuild `events.Event` from each wire item; `Payload` =
  re-`json.Marshal` of the wire payload value (registered types are field-stable; custom
  types pass through — `convoy_event_stream.go:82-84`), with empty/`NoPayload` mapping to
  nil.
- `ListTail(filter events.Filter, limit int) ([]events.Event, error)` — same GET without
  cursor (the handler's fast tail path, `huma_handlers_events.go:60-83`), `limit` param set;
  same client-side residual filtering rule.
- `LatestSeq() (uint64, error)` — GET `/v0/city/{c}/events?limit=1` with NO filters; return
  `uint64(body.Total)` (authoritative when the filter is empty — handler comment at
  `huma_handlers_events.go:69-77`).
- `Watch(ctx context.Context, afterSeq uint64) (events.Watcher, error)` — SSE GET
  `/v0/city/{c}/events/stream?after_seq=<afterSeq>` (`watcher.go`). The watcher:
  * yields events with `Seq > afterSeq`, in order, skipping `heartbeat` frames;
  * **auto-reconnects** on stream error/EOF with `Last-Event-ID: <last received seq>`
    (falling back to `after_seq` query when no event received yet), retry backoff pinned:
    250 ms initial, ×2, cap 5 s, reset on successful event — this is the
    **controller-restart resume** mechanism;
  * satisfies the `events.Watcher` contract verbatim (`events.go:220-230`): `Next()` blocks
    until event/ctx-cancel/`Close`; `Close()` unblocks pending `Next`, safe concurrently.
  * dedupes across reconnects via the seq cursor (monotonic guard: drop `Seq <= cursor`).
- `Close() error` — cancel all watcher goroutines, `CloseIdleConnections()` on the HTTP
  client, idempotent.

Error paths (pinned): `List`/`ListTail`/`LatestSeq` return typed errors wrapping status +
endpoint (`fmt.Errorf("events api: GET %s: %w", ...)`) — callers already handle provider
errors. NO panics. NO retries on read paths (callers poll). Bounds: request bodies ≤ the
server's existing Huma limits; the provider imposes no additional caps.

Registration — `cmd/gc/providers.go`: extend `newEventsProviderForNameWithConfig` with an
`api:` branch BEFORE the `switch` (mirror the `exec:` prefix check at 772-774):

```go
func newEventsProviderForNameWithConfig(v, eventsPath string, stderr io.Writer, eventsCfg config.EventsConfig) (events.Provider, error)
```

…gains a sibling with city identity (the `api:` scheme needs the city NAME, which the
current signature lacks):

```go
// newEventsProviderForNameWithCity is newEventsProviderForNameWithConfig plus
// the city identity required by the api: scheme. cityName may be empty for
// every other scheme; an api: provider with an empty cityName is a
// construction error (the transport cannot address /v0/city/{cityName}/...).
func newEventsProviderForNameWithCity(v, eventsPath, cityName string, stderr io.Writer, eventsCfg config.EventsConfig) (events.Provider, error)
```

Keep `newEventsProviderForNameWithConfig` as a delegating wrapper (empty cityName) so
existing call sites compile; update `openCityEventsProviderWithConfig`
(`providers.go:885-910`) to resolve the city name via the same helper the controller uses
(`loadedCityName(cfg, cityPath)` — `controller.go:1284`; fall back to
`filepath.Base(cityPath)` when config load fails, matching that helper's own fallback) and
call the WithCity variant. The `api:` scheme must NOT take the "no city needed" shortcut at
889-896 — it needs city resolution but NOT a local events path.

Controller/supervisor self-pointing guard (pinned): `gc controller` and `gc supervisor`
startup MUST hard-fail with a clear error when their own resolved events provider has the
`api:` scheme (`cmd/gc/controller.go` where `rec` is constructed, and the supervisor's
equivalent construction site — locate via `grep -n "newEventsProvider" cmd/gc/*.go`):
message pinned: `events provider "api:" is agent-side only: the controller is the
transport's server (unset GC_EVENTS or [events].provider for the controller process)`.
Don't-swallow-errors: exit non-zero, no silent fallback.

`fastEventsProviderName` (`providers.go:747-757`) needs no change — env wins, and hook
emissions in pods will carry `GC_EVENTS=api:…` from Step 5.

### Step 2 — `EventEmitRequest` payload + timestamp (typed-wire exception, validated)

File `internal/api/huma_types_events.go` — replace the request struct (current form at
25-30) with:

```go
// EventEmitRequest is the request body for POST /v0/city/{cityName}/events.
//
// Payload is a DOCUMENTED typed-wire exception (api-control-plane.md §
// "documented exceptions"): it carries the same opaque JSON that
// events.Event.Payload stores. For event types with a registered payload
// (events.RegisterPayload) the server decode-validates it against the
// registered Go type and rejects undecodable bodies; unregistered (custom)
// types pass through opaque, exactly matching `gc event emit --payload`.
type EventEmitRequest struct {
	Type    string          `json:"type" doc:"Event type." minLength:"1"`
	Actor   string          `json:"actor" doc:"Actor that produced the event." minLength:"1"`
	Subject string          `json:"subject,omitempty" doc:"Event subject."`
	Message string          `json:"message,omitempty" doc:"Event message."`
	Payload json.RawMessage `json:"payload,omitempty" doc:"Opaque JSON payload; decode-validated against the registered payload type when one exists."`
	Ts      *time.Time      `json:"ts,omitempty" doc:"Explicit event timestamp; server time when omitted."`
}
```

Handler `humaHandleEventEmit` (`internal/api/huma_handlers_events.go:160-176`) — extend:

- `Payload` non-empty → must be `json.Valid` (Huma/JSON parse already guarantees this for
  `json.RawMessage`; keep a defensive check) AND, when `events.DecodePayload(input.Body.Type,
  input.Body.Payload)` reports a registered type with a decode error → return
  `huma.Error422UnprocessableEntity` naming the type and the decode error. Registered-and-
  decodes → accept; unregistered → accept opaque (custom-event branch parity with
  `toWireEvent`, `convoy_event_stream.go:82-84`).
- `Ts` set → `e.Ts = input.Body.Ts.UTC()`; unset → zero (recorder auto-fills,
  `recorder.go:280-282`). This preserves the conformance suite's
  `RecordPreservesExplicitTimestamp` through the wire.
- Record the payload: `ep.Record(events.Event{Type, Actor, Subject, Message, Payload, Ts})`.

Wire-contract ripple (MANDATORY, CI-enforced): regenerate the OpenAPI artifacts + generated
client + dashboard types — `go generate ./internal/api/genclient`, `make dashboard-check`,
`TestOpenAPISpecInSync` green. Update the documented-exceptions list in
`engdocs/architecture/api-control-plane.md` (add `EventEmitRequest.Payload` beside the
existing Event.Payload exception) — the typed-wire law requires the exception to be
documented, not just implemented.

### Step 3 — relocate nudge-queue operations into `internal/nudgequeue` (single authority)

The queue state machine currently lives in `package main` and cannot be called by API
handlers. Move it — do NOT duplicate it (SRP: `internal/nudgequeue` owns queue semantics;
`cmd/gc` keeps CLI, delivery, and target resolution).

New file `internal/nudgequeue/queue.go` — relocate VERBATIM (rename to exported, receivers
unchanged) from `cmd/gc/cmd_nudge.go`:

| From (cmd/gc/cmd_nudge.go) | To (internal/nudgequeue) |
|---|---|
| `defaultQueuedNudge*` consts (31-42) | `DefaultTTL`, `DefaultClaimTTL`, `DefaultRetryDelay`, `DefaultMaxAttempts`, `DefaultDeadRetention` |
| `enqueueQueuedNudgeWithStore` (1347-1430) | `Enqueue(cityPath string, store beads.Store, item Item) error` |
| `claimDueQueuedNudgesMatching` (1238-1271) | `ClaimDueMatching(cityPath string, store beads.Store, now time.Time, match func(Item) bool) ([]Item, error)` |
| `ackQueuedNudgesWithOutcome` (1436-1482) | `AckWithOutcome(cityPath string, store beads.Store, ids []string, outcome, reason, commitBoundary string) error` |
| `releaseQueuedNudgeClaims` (1484-1520) | `ReleaseClaims(cityPath string, store beads.Store, ids []string) error` |
| `recordQueuedNudgeFailureDetailed` (1527-1590) + `failedQueuedNudge` (1592) | `RecordFailureDetailed(...) ([]Item, error)` |
| `listQueuedNudges` / `listQueuedNudgesForTarget` core loops (1273-1341) | `ListForAgent(cityPath string, store beads.Store, agentMatch func(string) bool, now time.Time) (pending, inFlight, dead []Item, err error)` |
| prune/recover/exists/sort helpers (1610-1714) | unexported siblings (`pruneExpired`, `recoverExpiredInFlight`, `pruneDead`, …) |

**Signature note (deliberate extension — evaluators must NOT flag this as drift):** the
real `claimDueQueuedNudgesMatching(cityPath string, now time.Time, match func(queuedNudge)
bool)` (`cmd_nudge.go:1238`) has NO store parameter — it opens its own store internally
via `openNudgeBeadStore(cityPath)`. The relocated `ClaimDueMatching` signature above ADDS
the explicit `store beads.Store` parameter by design (same for the other ops the table
threads `store` into: ack/1436, release/1484, failure/1527, list/1273 all open their own
store today), so the API handlers (Step 4c) can pass the server's store instead of opening
a second one. "Relocate VERBATIM" binds the queue SEMANTICS and function bodies; the
store-threading parameter is this WO's pinned interface change, and the `cmd/gc`
delegating wrappers keep their original signatures, opening the store exactly where the
old code did.

New file `internal/nudgequeue/beads.go` — relocate the bead-shadow layer from
`cmd/gc/nudge_beads.go`: `ensureQueuedNudgeBead` (127) → `EnsureShadowBead`,
`markQueuedNudgeTerminal` (201) → `MarkShadowTerminal`, plus their runtime helpers and the
rollback path referenced at `cmd_nudge.go:1416-1421`. `openNudgeBeadStore`
(`nudge_beads.go:36`) STAYS in cmd/gc (CLI store opening is a cmd concern; the API server
passes its own store).

New file `internal/nudgequeue/wake.go` — relocate `pingNudgeWakeSocket`
(`cmd/gc/nudge_dispatcher.go:53`) → `PingWakeSocket(cityPath string)`. `Enqueue` calls it on
success exactly as the old code did (`cmd_nudge.go:1423-1428`).

`cmd/gc` call sites: replace bodies with thin delegations (e.g.
`func enqueueQueuedNudgeWithStore(cityPath string, store beads.Store, item queuedNudge) error {
return nudgequeue.Enqueue(cityPath, store, item) }`) — zero behavior change, existing
`cmd/gc` tests keep passing unmodified except import-level details. The
`queuedNudge = nudgequeue.Item` alias (`cmd_nudge.go:54`) already makes types identical.
`internal/session/submit.go:529` (direct `nudgequeue.WithState`) is controller-process-only;
leave it, but add the comment `// controller-process direct write; agent-side ops go through
the nudges provider (GCD-WO-CSC-001)`.

### Step 4 — typed nudge endpoints + `GC_NUDGES` provider seam

**4a. Config seam.** `internal/config/config.go`: new `NudgesConfig` + City field, mirroring
`EventsConfig` (1490-1498):

```go
// NudgesConfig holds nudge-queue transport settings.
type NudgesConfig struct {
	// Provider selects the queue-op backend: "" (default: city-local
	// .gc/nudges/state.json via flock) or "api:<base-url>" (typed controller
	// endpoints; hosted agent pods). There is deliberately no exec/fake
	// provider: bead shadows + the file queue are the only durability layers.
	Provider string `toml:"provider,omitempty"`
}
```

City struct gains `Nudges NudgesConfig \`toml:"nudges,omitempty"\`` (place beside `Events`,
config.go:199). Resolution precedence (pinned, mirrors `eventsProviderConfig`,
providers.go:727-742): env `GC_NUDGES` → `city.toml [nudges].provider` → `""`. New
`cmd/gc/providers.go` funcs: `nudgesProviderName() string` + `nudgesProviderConfig()`
following the events pattern verbatim.

**4b. Wire types.** New file `internal/api/huma_types_nudges.go`:

```go
// WireNudge is the wire shape of one queued nudge. Field-for-field mirror of
// nudgequeue.Item (internal/nudgequeue/state.go:31-49) — same JSON names.
type WireNudge struct {
	ID                string         `json:"id"`
	BeadID            string         `json:"bead_id,omitempty"`
	Agent             string         `json:"agent"`
	SessionID         string         `json:"session_id,omitempty"`
	ContinuationEpoch string         `json:"continuation_epoch,omitempty"`
	Source            string         `json:"source"`
	Message           string         `json:"message"`
	Reference         *WireNudgeRef  `json:"reference,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	DeliverAfter      time.Time      `json:"deliver_after"`
	ExpiresAt         time.Time      `json:"expires_at"`
	Attempts          int            `json:"attempts,omitempty"`
	LastAttemptAt     time.Time      `json:"last_attempt_at,omitempty"`
	LastError         string         `json:"last_error,omitempty"`
	ClaimedAt         time.Time      `json:"claimed_at,omitempty"`
	LeaseUntil        time.Time      `json:"lease_until,omitempty"`
	DeadAt            time.Time      `json:"dead_at,omitempty"`
}

// WireNudgeRef mirrors nudgequeue.Reference.
type WireNudgeRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// NudgeEnqueueRequest is the body for POST /v0/city/{cityName}/nudges.
type NudgeEnqueueRequest struct {
	ID                string        `json:"id,omitempty" doc:"Client-supplied idempotency id; server-generated when empty."`
	Agent             string        `json:"agent" minLength:"1" doc:"Qualified agent name the nudge targets."`
	Message           string        `json:"message" minLength:"1"`
	Source            string        `json:"source" minLength:"1" doc:"Producer identity (e.g. formula step, hook)."`
	SessionID         string        `json:"session_id,omitempty"`
	ContinuationEpoch string        `json:"continuation_epoch,omitempty"`
	Reference         *WireNudgeRef `json:"reference,omitempty"`
	DeliverAfter      *time.Time    `json:"deliver_after,omitempty"`
	ExpiresAt         *time.Time    `json:"expires_at,omitempty" doc:"Defaults to now+24h (nudgequeue.DefaultTTL)."`
}

// NudgeClaimRequest is the body for POST /v0/city/{cityName}/nudges/claim.
// Claim semantics are exactly queuedNudgeClaimableForTarget
// (cmd/gc/cmd_nudge.go:1210-1224): agent match, session fence, epoch fence.
type NudgeClaimRequest struct {
	Agent             string `json:"agent" minLength:"1"`
	SessionID         string `json:"session_id,omitempty"`
	ContinuationEpoch string `json:"continuation_epoch,omitempty"`
}

// NudgeAckRequest is the body for POST /v0/city/{cityName}/nudges/ack.
// outcome routes to the queue op: "injected" → AckWithOutcome, "failed" →
// RecordFailureDetailed (retry/dead-letter per queue policy), "released" →
// ReleaseClaims (lease returned, item back to pending).
type NudgeAckRequest struct {
	IDs            []string `json:"ids" minItems:"1"`
	Outcome        string   `json:"outcome" enum:"injected,failed,released"`
	Reason         string   `json:"reason,omitempty"`
	CommitBoundary string   `json:"commit_boundary,omitempty"`
}

// NudgeWithdrawRequest is the body for POST /v0/city/{cityName}/nudges/withdraw.
type NudgeWithdrawRequest struct {
	IDs []string `json:"ids" minItems:"1"`
}
```

List input: `NudgeListInput{ CityScope; Agent string \`query:"agent" required:"false"\`;
SessionID string \`query:"session_id" required:"false"\` }`; output body
`{ Pending, InFlight, Dead []WireNudge; Counts {Pending, InFlight, Dead int} }` (shape
mirror of `nudgeStatusJSON`, `cmd_nudge.go:73-90`).

**4c. Handlers.** New file `internal/api/huma_handlers_nudges.go` — five handlers on
`(*Server)` calling the Step-3 relocated ops with `s.state.CityPath()` and the same bead
store the bead handlers use (inspect `internal/api/handler_beads.go` / `state.go` for the
store accessor on `State` and reuse it — do not open a second store). Conversions
`WireNudge ⇄ nudgequeue.Item` are mechanical field copies (write both directions in
`huma_types_nudges.go`, with a field-set equality test — Step 7). Enqueue response: 201 +
`{id, bead_id}`. Claim response: `{items []WireNudge}` (claimed = moved to in-flight with
lease, exactly `ClaimDueMatching` semantics). Ack/withdraw: `{status:"ok", acked int}`.
Error paths: unknown outcome → 422 (Huma enum handles); empty city → existing 404 city
resolution; queue-op error → 500 with wrapped message.

**4d. Registration.** `internal/api/supervisor_city_routes.go`, immediately after the
events block (221):

```go
// Nudge queue (typed transport for hosted agent pods — GCD-WO-CSC-001).
cityGet(sm, "/nudges", (*Server).humaHandleNudgeList)
cityRegister(sm, huma.Operation{OperationID: "enqueue-nudge", Method: http.MethodPost, Path: "/nudges", Summary: "Enqueue a deferred nudge", DefaultStatus: http.StatusCreated, Metadata: map[string]any{mutationClassMetadataKey: mutationClassNudges}}, (*Server).humaHandleNudgeEnqueue)
cityRegister(sm, huma.Operation{OperationID: "claim-nudges", Method: http.MethodPost, Path: "/nudges/claim", Summary: "Claim due nudges for a target", Metadata: map[string]any{mutationClassMetadataKey: mutationClassNudges}}, (*Server).humaHandleNudgeClaim)
cityRegister(sm, huma.Operation{OperationID: "ack-nudges", Method: http.MethodPost, Path: "/nudges/ack", Summary: "Ack, fail, or release claimed nudges", Metadata: map[string]any{mutationClassMetadataKey: mutationClassNudges}}, (*Server).humaHandleNudgeAck)
cityRegister(sm, huma.Operation{OperationID: "withdraw-nudges", Method: http.MethodPost, Path: "/nudges/withdraw", Summary: "Withdraw queued wait nudges", Metadata: map[string]any{mutationClassMetadataKey: mutationClassNudges}}, (*Server).humaHandleNudgeWithdraw)
```

(also tag the existing `emit-event` registration at 209-215 with
`Metadata: map[string]any{mutationClassMetadataKey: mutationClassEventsEmit}` — see Step 5a.)
Then regenerate OpenAPI/genclient/dashboard as in Step 2.

**4e. Agent-side routing.** New file `cmd/gc/nudge_queue_client.go`: a small resolver

```go
// nudgeQueueOps is the seam between nudge CLI commands and the queue state.
// File mode (default) hits .gc/nudges/state.json via internal/nudgequeue;
// api mode calls the typed controller endpoints. Delivery, target
// resolution, and pollers are NOT behind this seam — they already run
// where the sessions live.
type nudgeQueueOps interface {
	Enqueue(item queuedNudge) error
	ClaimDueForTarget(target nudgeTarget, now time.Time) ([]queuedNudge, error)
	Ack(ids []string, outcome, reason, commitBoundary string) error
	Withdraw(ids []string) error
	ListForTarget(target nudgeTarget, now time.Time) (pending, inFlight, dead []queuedNudge, err error)
}

// nudgeQueueOpsFor resolves the ops implementation from nudgesProviderName().
// "" → fileNudgeQueueOps{cityPath, store}; "api:<base>" → apiNudgeQueueOps
// backed by genclient with the same X-GC-Request header and 250ms-class
// Record-path budget EXCEPT claim/ack/withdraw, which are correctness ops:
// they use a 5s timeout and RETURN errors (a failed ack must surface — the
// lease recovery path depends on it).
func nudgeQueueOpsFor(cityPath string) (nudgeQueueOps, error)
```

Rewire ONLY the queue-op call sites in `cmd_nudge.go` (drain/status/poll paths and
`queueSessionNudgeWithWorker`, 650-672) and `internal/api/wait_nudges.go` withdraw callers'
CLI analogs through this seam. Delivery internals (`deliverSessionNudge*`,
`tryDeliverQueuedNudgesByPoller`) keep calling the file-backed functions directly ONLY via
the seam's file implementation when provider is file — i.e. in api mode a pod never touches
`state.json`. Unknown scheme → error naming `GC_NUDGES`. In api mode `pingNudgeWakeSocket`
is server-side (inside `Enqueue` handler); the client must NOT dial the socket.

### Step 5 — mutation-class allowlist (the security pin) + supervisor threading

**5a. Class vocabulary (pinned literals, closed set):** new file
`internal/api/mutation_class.go`:

```go
// Mutation classes gate which mutating operation groups stay enabled when
// the API binds non-localhost without blanket AllowMutations. Closed set —
// adding a class is a security decision, not a convenience (CSC kit C6:
// event emit + nudge ops ONLY).
const (
	mutationClassMetadataKey = "gc_mutation_class"
	mutationClassEventsEmit  = "events_emit"
	mutationClassNudges      = "nudges"
)

// allowedMutationClassSet validates and normalizes the configured list.
// Unknown literals are a config error (fail closed, never ignore).
func allowedMutationClassSet(classes []string) (map[string]bool, error)
```

**5b. Config.** `internal/config/config.go` `APIConfig` (1713-1722) gains:

```go
// AllowMutationClasses enables SPECIFIC mutating endpoint classes when Bind
// is non-localhost, without enabling all mutations. Valid literals:
// "events_emit" (POST /v0/city/{c}/events), "nudges" (POST /v0/city/{c}/nudges*).
// Hosted agent-pod transport (GCD-WO-CSC-001) uses exactly these two.
// AllowMutations=true still means everything (superset); never required for
// the event/nudge transport and never set by the k8s wiring.
AllowMutationClasses []string `toml:"allow_mutation_classes,omitempty"`
```

Mirror field on `internal/supervisor/config.go` next to `AllowMutations` (38). Config
validation (wherever `APIConfig` is validated at load — locate via existing validation of
`[api]`; if none exists, validate at server construction) rejects unknown literals with the
exact message `api.allow_mutation_classes: unknown class %q (valid: events_emit, nudges)`.

**Site-level `[api]` override surface (kit A2.6 erratum — REQUIRED, this WO).** The k8s
render must be able to set `[api] bind` to a non-localhost address AND
`allow_mutation_classes = ["events_emit", "nudges"]` **per city via the `.gc/site.toml`
derivation**, without editing `city.toml` (AGC-WO-CSC-002 renders those values at
deploy time — see Dependencies). Today `SiteBinding`
(`internal/config/site_binding.go:154-158`) carries only workspace identity + rig
bindings; extend it with an optional `[api]` block limited to exactly these two fields
(`bind`, `allow_mutation_classes` — NOT `port`, NOT `allow_mutations`; the narrow surface
is deliberate) and apply it onto `cfg.API` in `applySiteBindings`
(`site_binding.go:187-199`), site value winning over `city.toml`. Note the defaults this
overlay overrides: `Bind` defaults to `"127.0.0.1"` (`internal/config/config.go:1716-1717`,
`BindOrDefault`), so hosted cities MUST get a site-rendered non-localhost bind for agent
pods to reach the controller Service at all. Named test:
`internal/config/site_binding_api_test.go` — site `[api]` block sets `Bind` +
`AllowMutationClasses` on the effective config (precedence over `city.toml` values);
absent block leaves both untouched; unknown class literal in the site file fails with the
same pinned validation message.

**5c. Enforcement.** `internal/api/huma_handlers_supervisor.go`: change signatures —

```go
func newSupervisorHumaAPI(mux *http.ServeMux, readOnly bool, allowedMutationClasses map[string]bool) huma.API
func humaReadOnlyMiddleware(api huma.API, allowedMutationClasses map[string]bool) func(ctx huma.Context, next func(huma.Context))
```

Middleware rule (replaces the blanket body at 191-199): mutation method AND server readOnly
→ allowed iff `ctx.Operation().Metadata[mutationClassMetadataKey]` is a string present in
`allowedMutationClasses`; otherwise the existing 403 `read_only:` Problem Details, now with
detail `read_only: mutations disabled: server bound to non-localhost address (class-allowlisted
endpoints excepted)`. `NewSupervisorMux` (`supervisor.go:140-150`) gains the
`allowedMutationClasses map[string]bool` parameter and threads it; update ALL constructors:
`cmd/gc/controller.go:1341` (compute the set from `cfg.API.AllowMutationClasses` beside the
`readOnly` computation at 1330-1336, logging the enabled classes on the same stderr line
pattern as 1335), `cmd/gc/cmd_supervisor.go`, `internal/api/server.go:287`, and test
constructors. The `/svc/*` proxy read-only gate (`server.go:54`) is untouched — services
are not in the allowlist vocabulary.

**5d. `apiClient` routing note.** `cmd/gc/apiroute.go:54,91`: the non-loopback fallback
check stays keyed on `AllowMutations` (general CLI mutation routing must NOT ride the
narrow class allowlist) — add a comment stating that class-allowlisted transports
(events/nudges providers) construct their own clients and bypass `apiClient` deliberately.

### Step 6 — k8s provider projection + controller Service

`internal/runtime/k8s/provider.go` + `pod.go`:

- New provider fields + env (document in the `NewProvider` doc block, 88-115):
  * `GC_K8S_CONTROLLER_HOST` — in-cluster DNS name of the controller API Service (e.g.
    `gc-controller.<namespace>.svc.cluster.local`). Empty (default) = transport projection
    disabled (exact current behavior).
  * `GC_K8S_CONTROLLER_PORT` — controller API port; default `"9443"`
    (`config.DefaultAPIPort`, config.go:1709).
  * `GC_K8S_CONTROLLER_SERVICE` — Service NAME to ensure; default = first DNS label of
    `GC_K8S_CONTROLLER_HOST`.
  * `GC_K8S_CONTROLLER_SERVICE_SELECTOR_JSON` — JSON label map selecting the controller
    pod. Empty = the provider does NOT manage the Service (platform renders it —
    AGC-WO-CSC-002's choice).
- **Projection** (follow `projectedPodDoltEnv`, pod.go:146-193 — projection at the adapter
  edge; controller-local values never leak): new `pod.go` func

  ```go
  // projectedPodTransportEnv returns the agent-pod event/nudge transport env.
  // Empty controllerHost → empty map (file/strip behavior preserved).
  // Non-empty → GC_EVENTS and GC_NUDGES both set to "api:http://" +
  // net.JoinHostPort(controllerHost, controllerPort). The controller's own
  // GC_EVENTS/GC_NUDGES cfg values are never forwarded (they are
  // controller-local paths/modes; the strip list keeps them out).
  func projectedPodTransportEnv(controllerHost, controllerPort string) map[string]string
  ```

  Wire into `buildPodEnvForRoot` (pod.go:452-570) beside the projectedDolt merge (518-529),
  threading the two provider fields through the same call chain as
  `managedServiceHost/managedServicePort`. **Keep `GC_EVENTS` in the skip map (462) and add
  `GC_NUDGES`** — stripping the controller's value and projecting the pod value are two
  halves of the same adapter move ("stop-strip" = stop losing the transport, not remove the
  strip of controller-local values). http (not https) is pinned for the in-namespace hop;
  TLS termination is out of scope (NetworkPolicy bounds the exposure).
- **Service ensure**: new `provider.go` method

  ```go
  // EnsureControllerService idempotently creates/updates the namespace-local
  // Service fronting the controller API listener. No-op unless
  // GC_K8S_CONTROLLER_SERVICE_SELECTOR_JSON is set. Called once from
  // NewProvider (after client construction), errors are returned to the
  // caller — a missing Service breaks every agent-side emit.
  func (p *Provider) EnsureControllerService(ctx context.Context) error
  ```

  Spec: `corev1.Service{ metadata.name = GC_K8S_CONTROLLER_SERVICE, spec.selector = <parsed
  JSON>, ports = [{name:"api", port:<GC_K8S_CONTROLLER_PORT as int>, targetPort:same}] }`,
  create-or-update through the existing `k8sOps` seam (extend the interface + fake — see
  how pod create/exec are seamed; keep the fake asserting the manifest).
- **NetworkPolicy posture (pinned contract — rendered by AGC-WO-CSC-002, NOT here):**
  ingress to the controller pod's API port permitted ONLY from pods in the same city
  namespace (agent pods); all other ingress to that port denied; agent pods need no new
  egress rules beyond namespace-local. This WO records the posture in
  `engdocs/architecture/api-control-plane.md` (new short "hosted exposure" note) so the
  engine documents WHY non-localhost bind + class allowlist is safe. The engine never
  renders NetworkPolicy objects.

### Step 7 — tests (see Test Coverage for the named-path battery)

Plus regeneration hygiene: `go generate ./internal/api/genclient`, `make dashboard-check`,
`go vet ./...`, `make test` (or `make test-fast-parallel`), targeted shards per TESTING.md
for `cmd/gc` process suites touched by the nudge relocation.

## Git Workflow

Work on the loop-provided worktree/branch; commit early and often with conventional messages
(`feat(events): api transport provider`, `feat(api): typed nudge endpoints`, …). NEVER
stash; NEVER weaken/skip a failing gate to pass; the harness merges — never self-merge.
Before declaring done: fetch + rebase onto current `origin/main`, re-run the full acceptance
battery on the rebased tree. Follow `.githooks/pre-commit` (active via
`git config core.hooksPath` → `.githooks`) and the repo's session-completion push rules as
they apply to the loop workflow.

## Test Coverage

Fixture-realism doctrine (owner-ratified, REJECT-level) applies verbatim: fixtures replicate
real producer behavior — real event types from `events.KnownEventTypes` with their
registered payload shapes, real `nudgequeue.Item` envelopes with fences and TTLs, full HTTP
round-trips through the REAL Huma server (`httptest` over `NewSupervisorMux` with a real
`FileRecorder` on `t.TempDir()`), never hand-mocked handler stubs. **Zero-item runs never
pass green**: every list/stream assertion requires the expected non-zero count first; a
harness that finds no events/nudges must FAIL.

1. **Conformance (provider):** `internal/events/apitransport/conformance_test.go` — run
   `eventstest.RunProviderTests` (conformance.go:26) with a factory that spins a real
   city-scoped API server backed by `events.NewFileRecorder` in `t.TempDir()` and returns
   `apitransport.NewProvider(server.URL, cityName, io.Discard)`. This transitively proves
   Record/List/LatestSeq/Watch semantics INCLUDING explicit-timestamp preservation (the
   Step-2 `ts` field is load-bearing here).
2. **Payload round-trip (seam/shape):** `internal/api/huma_validation_nudges_payload_test.go`
   (or extend `internal/api/huma_validation_test.go` per its conventions) — POST
   `/v0/city/{c}/events` with (a) a registered type + valid registered payload → 201 and the
   payload survives byte-equivalent through GET; (b) registered type + structurally invalid
   payload → 422 REJECT (regression-pins the seam); (c) custom type + arbitrary JSON → 201
   passthrough. Field-SET equality on the wire item, not subset.
3. **Multi-writer gap-free seq:** `internal/events/apitransport/multiwriter_test.go` — 8
   concurrent `apitransport.Provider`s × 200 `Record`s each against ONE server/file →
   `LatestSeq == 1600`, seqs exactly `1..1600` gap-free, single `events.jsonl`, zero
   `events: api: record` stderr lines.
4. **Controller-restart SSE resume + reconciler convergence:**
   `internal/events/apitransport/resume_test.go` — start watcher, deliver N events, STOP
   the httptest server, record M more into the same file via a fresh server on the same
   recorder path, restart listener on the same address (use a manual `net.Listener` to
   keep the URL stable), assert the watcher yields all N+M in order exactly once
   (reconnect via Last-Event-ID). Plus `internal/api/huma_sse_test.go`-style server-side
   check: `after_seq` resume returns no duplicates/no gaps after a server restart against
   the same file (recorder re-open path, `NewFileRecorder` seq recovery,
   recorder.go:180-183). **Convergence half** (same test file or
   `internal/nudgequeue/restart_convergence_test.go`): across the simulated restart,
   `LatestSeq` continues monotonically (no seq reset — the re-opened recorder resumes from
   the file tail), and nudge queue state persists: items pending/in-flight before the
   restart are listed, claimable, and ackable after it against the fresh server (state.json
   is mayor-local durable state; a controller bounce must converge, not amnesia).
5. **Nudge queue op parity (behavioral invariants):**
   `internal/nudgequeue/queue_test.go` — relocated-function unit coverage: enqueue
   supersession by (agent, source, reference) incl. in-flight supersession
   (cmd_nudge.go:1372-1411 semantics), claim fences (session/epoch —
   `queuedNudgeClaimableForTarget` truth table), lease expiry recovery → re-claimable,
   ack outcomes `injected|failed|released` each land the item in the right bucket with the
   right shadow-bead terminal metadata (`MarkShadowTerminal` fields exactly as
   nudge_beads.go writes today), dead-letter after `DefaultMaxAttempts`.
6. **Nudge endpoints (integration over real grammars):**
   `internal/api/huma_handlers_nudges_test.go` — full HTTP: enqueue→list→claim→ack lifecycle;
   withdraw of a pending wait nudge marks the shadow bead `wait-canceled`
   (waits.go:237-253 metadata verbatim); claim honors fences over the wire; enqueue is
   idempotent on client-supplied `id` (re-POST same id → no duplicate). REJECT test:
   malformed enqueue (empty agent) → 422.
7. **Pod-eviction survival (nudges):** `internal/nudgequeue/eviction_test.go` — enqueue via
   API ops, claim from "pod A", simulate eviction (no ack; advance clock past
   `DefaultClaimTTL` via the recover function's `now` parameter), assert
   `recoverExpiredInFlight` returns the item to pending and a second target claims and acks
   it; shadow bead reflects final terminal state; nothing depended on any pod-local file.
8. **Wire⇄domain mirror pin:** `internal/api/huma_types_nudges_test.go` — reflective
   field-SET equality between `WireNudge` and `nudgequeue.Item` JSON tags (both directions
   of the converter round-trip a fully-populated Item unchanged) — regression-pins silent
   renames.
9. **Allowlist enforcement:** `internal/api/mutation_class_test.go` — with
   readOnly=true + classes `{events_emit,nudges}`: POST events → 2xx, POST nudges/claim →
   2xx, POST `/v0/city/{c}/beads` → 403 `read_only:`, POST `/v0/city/{c}/sling` → 403,
   PATCH city → 403; with empty class set: event emit → 403 (current behavior preserved);
   unknown literal in config → construction error with the pinned message. CSRF still
   required on allowlisted classes (missing `X-GC-Request` → 403 csrf).
10. **k8s projection:** `internal/runtime/k8s/pod_transport_env_test.go` —
    `projectedPodTransportEnv` table test (empty host → empty map; host+port →
    both `GC_EVENTS`/`GC_NUDGES` = `api:http://host:port`); `buildPodEnvForRoot` with
    controller host set: pod env contains projected values, controller's own
    `GC_EVENTS=/some/path` and `GC_NUDGES` values are NOT forwarded; with host unset:
    resulting env list is **byte-identical to pre-change golden** (file-mode parity at the
    pod-env seam). `internal/runtime/k8s/provider_service_test.go` — `EnsureControllerService`
    via the `k8sOps` fake: manifest shape (name/selector/port), idempotency (second call
    updates, not duplicates), no-op without selector env.
11. **File-mode byte-parity:** `cmd/gc/providers_events_api_test.go` — provider-name
    resolution table: `GC_EVENTS` unset + no city config → `*events.FileRecorder`;
    `GC_EVENTS=api:http://…` → `*apitransport.Provider`; `api:` with unresolvable city →
    error; malformed `api:` URL → error. Same table for `GC_NUDGES` via
    `nudgeQueueOpsFor`. Plus an end-to-end file-mode test: with providers unset, `gc event
    emit` writes the identical JSONL line (modulo seq/ts) it writes today — assert against
    a golden envelope produced by `doEventEmit` on a plain `FileRecorder`.
12. **Dispatch-latency parity:** `internal/events/apitransport/latency_test.go` — watcher
    waiting on `Next()`; `Record` an event; assert delivery `< 1s` wall-clock (p95 over 20
    iterations; generous CI bound), documented against the hosted comparator (2s file
    polling per `GC_EVENTS_POLL_INTERVAL` hosted setting, recorder.go:508-519). Not a
    micro-benchmark — a regression tripwire.
13. **Cascade-close burst:** `internal/events/apitransport/burst_test.go` — replay a
    realistic convoy cascade-close shape: 1 `convoy.closed` + 300 `bead.closed` + 300
    `bead.updated` events (real registered payload shapes) recorded through 4 concurrent
    api providers as fast as possible; assert ZERO drops (all 601 present, gap-free seq)
    and zero stderr drop lines — the 250 ms budget must hold under burst against a local
    server. If the server cannot sustain this, that is a FINDING to fix (e.g. handler
    contention), not a bound to relax.
14. **Self-pointing guard:** `cmd/gc/controller_events_guard_test.go` — controller startup
    with `GC_EVENTS=api:http://127.0.0.1:1` exits non-zero with the pinned message.
15. **OpenAPI/regeneration:** existing CI gates (`TestOpenAPISpecInSync`,
    `TestGeneratedClientInSync`, `openapi_sync_test.go`, `make dashboard-check`) green —
    treated as named acceptance tests, not incidental.

## Validation

- `make build && make check` clean; `go vet ./...` clean; sharded suites per TESTING.md for
  touched tiers (`make test-fast-parallel`, `make test-cmd-gc-process-parallel` for the
  cmd/gc nudge relocation, `make test-integration-shards-parallel` if integration tiers are
  touched).
- `make dashboard-check` (API surface changed) + dashboard serves locally per AGENTS.md
  quality gates.
- **Cities-PAUSED clause:** this WO verifies that all GasCity-in-AWS remains paused
  (zero-replica / suspended) before declaring success; it starts NO hosted city, NO live
  cluster, NO AWS resource; live drills happen only where explicitly named by later WOs
  (vehicle-graph pilot under AGC-WO-CSC-002, re-suspend after). All validation here is
  local (unit/integration/httptest/fake-k8sOps).
- **Prod-gate defer language:** no prod-shaped resource is touched here; if any follow-on
  reveals an additive-CREATE prod surface, it AUTO-defers to the end-game cutover per the
  standing prod-gate defer policy (2026-06-29) — never actioned mid-loop from this WO.
- Functional-parity doctrine: file mode must hold behavioral parity (same envelopes, same
  best-effort semantics); artifact-level diffs (e.g. new OpenAPI operations) are expected
  and fine.

## Acceptance Criteria (each ← named test)

1. `api:<base-url>` provider passes the FULL events conformance suite ← Test 1.
2. `EventEmitRequest` carries payload+ts; registered-type payloads are decode-validated
   (invalid → 422), custom types pass through; documented exception recorded ← Test 2 +
   the api-control-plane.md diff (checked in Test 15's spec-sync).
3. Concurrent api-transport writers produce gap-free monotonic seq in ONE
   single-writer events.jsonl ← Test 3.
4. SSE watcher survives controller restart via Last-Event-ID/after_seq resume — no gaps,
   no duplicates; server-side resume against a re-opened recorder equally clean; restart
   convergence: seq continuity + nudge queue state fully servable post-bounce ← Test 4.
5. Nudge queue ops (enqueue/claim/ack/withdraw + list) exist as typed city-scoped
   endpoints with wire shapes mirroring `nudgequeue.Item` field-for-field ← Tests 6, 8.
6. Relocated queue semantics are behavior-identical (supersession, fences, lease recovery,
   outcomes, dead-letter, shadow-bead metadata) ← Test 5.
7. Nudges survive pod eviction via bead shadows + lease expiry re-claim ← Test 7.
8. Read-only non-localhost servers allow ONLY the two pinned mutation classes when
   configured; everything else still 403s; unknown class literals fail construction;
   blanket `allow_mutations` never required nor set ← Test 9.
9. Agent pods receive `GC_EVENTS`/`GC_NUDGES` `api:` projection iff controller host env is
   set; controller-local values never leak; provider-managed Service correct + idempotent;
   pod env byte-identical when disabled ← Test 10.
10. File mode (env unset) is default and byte-parity preserved; malformed/underspecified
    `api:` configs fail loudly ← Tests 11, 14.
11. Event dispatch latency via SSE beats the 2s hosted polling comparator ← Test 12.
12. Cascade-close burst is drop-free through the api transport ← Test 13.
13. OpenAPI spec, generated client, and dashboard types regenerated and in sync ← Test 15.
14. `make check` + vet + touched shards green on the REBASED tree ← Validation.

## Risks

- **Genclient consumer creep:** doc.go pins legitimate consumers; adding apitransport
  without updating the registry doc will read as drift — update doc.go +
  api-control-plane.md §2 in the same commit (Step 1).
- **Conformance `Ts` preservation:** easy to miss that the wire needs an explicit `ts` —
  Test 1 fails without Step 2's `Ts` field; do not "fix" by relaxing the suite (REJECT).
- **Nudge relocation churn:** `cmd/gc` process tests are sensitive; keep delegating
  wrappers so diffs stay mechanical. Do not fork logic between cmd/gc and nudgequeue —
  single authority (kit §1.5).
- **`internal/session/submit.go` direct `WithState`:** controller-process path — documented,
  not routed. If a future agent-side path reaches it, that is a NEW seam bug; the comment
  pins the boundary.
- **Middleware/operation coupling:** `ctx.Operation()` metadata lookup must handle nil
  metadata (operations without the key are simply not allowlisted). A typo'd metadata key
  silently disables the allowlist → the literal is a shared const (Step 5a) used by BOTH
  registration and middleware; Test 9 catches drift.
- **Self-DoS via Record timeout too tight in CI:** the 250 ms budget against localhost
  httptest is generous; if CI flakes, investigate server contention — the budget is
  C6-pinned, not tunable per-test.
- **K8s Service ensure vs platform-rendered Service:** double management if AGC-WO-CSC-002
  also renders one — the env gate (selector JSON unset → provider no-op) is the seam;
  AGC-WO-CSC-002 picks exactly one lane (flagged in its blocked_by context).
- **Scope-size:** this WO is large but cohesive (one transport seam, two payloads). If a
  generation run cannot complete coherently, split commits by step order — steps 1-2,
  3-4, 5, 6, 7 are each independently green-able except 4e (needs 3+4d).

## Done Means

- All Acceptance Criteria green on the rebased tree; full battery run exactly once at final
  state (fast recipes during iteration don't count as evidence).
- No file-mode behavior change for non-k8s cities (byte-parity tests + golden pod-env test
  prove it).
- Contracts this WO authors are now importable authority: `api:` scheme grammar,
  `EventEmitRequest` v2, nudge wire types + endpoints, `GC_NUDGES`/`NudgesConfig`,
  mutation-class vocabulary, the site-level `[api]` override surface
  (`bind` + `allow_mutation_classes` via `.gc/site.toml` — A2.6),
  `GC_K8S_CONTROLLER_*` env contract, NetworkPolicy posture —
  GCD-WO-CSC-002 and AGC-WO-CSC-002 cite these; nothing re-declares them.
- Docs updated: `engdocs/architecture/api-control-plane.md` (payload exception, genclient
  consumer, hosted-exposure note), `internal/api/genclient/doc.go`.
- Working tree clean, branch pushed, evidence artifacts (test transcripts) available to the
  evaluator.

## Master cutover contribution

**None (platform repo, no AWS).** This repo hosts no MatchPoint stacks; nothing here enters
`master/` prod-cutover tracking. The hosted rollout of this transport (image build/deploy,
Service/NetworkPolicy render, mayor PVC) is tracked by the aws-GasCity CSC WOs
(AGC-WO-CSC-002/006A/006B) in their own cutover entries.
