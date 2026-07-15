# Work Order: GCD-WO-CSC-002 — ephemeral workspace mode (runtime-dir split, rig-from-mirror provisioning, WIP-push cadence)

NOTE: this WO is long — read the FULL file at
`specs/agent-work-orders/GCD-WO-CSC-002-ephemeral-workspace-mode.md` in your worktree before
implementing; tail amendments are BINDING.

Execution classification: **Dev-only** — `boundary: dev` (QST-6 fail-closed) ·
`live-tier: none` (no live surface touched — engine code + pack content + repo-native
tests; the ADR-024 live drills are aws-GasCity::AGC-WO-CSC-002's named pause exception) ·
`blast radius:` GasCity-Dev engine (citylayout/runtime routing, rig provisioning, k8s
no-PVC staging) + gastown pack polecat prompt/worktree script + the DEFAULT PATHS of
ephemeral runtime artifacts (`.gc/X` → `.gc/runtime/X`, process-lifetime artifacts only,
mixed-version note pinned in Step 1c); contracts consumed by aws-GasCity::AGC-WO-CSC-002
and AGC-WO-CSC-007 · `additive vs mutating: additive` (no pre-existing durable data
transformed; non-k8s cities and persistent-PVC mode byte-parity pinned). **Wave 23** (CSC
band; harness-ledger mega-wave 33 as of 2026-07-14),
`blocked_by` = `GasCity-Dev::GCD-WO-CSC-001-runtime-event-nudge-transport` — same-wave
edge, wired as an apply_deps DIRECT-WRITE per kit ADDENDUM A1 §4. Engine Go + gastown pack
content inside this repo only; no AWS resources, no deploy surface, no city runs. Master
cutover contribution: **None (platform repo, no AWS)** — see the section below.

> **Provenance (binding):** authored under the City-Scaling Improvements (CSC) program:
> `master/city-scaling-improvements/wo-authoring-kit.md` — **contract C7 names THIS WO as
> the ENGINE authority** ("Workspace/mirror — engine authority: GCD-WO-CSC-002; infra
> authority: AGC-WO-CSC-002") — plus kit K1–K7 and ADDENDUM A1;
> `master/city-scaling-improvements/gap-analysis-and-build-plan.md` §5 row 2 ("GC_RUNTIME_DIR
> emptyDir split; rig-from-mirror provisioning (extend pack remote-source machinery to
> rigs); WIP-push cadence support (D8)") + §6 (mirror URL contract = AGC-WO-CSC-002 citing
> this WO's engine seams); living record
> `aws-GasCity/docs/investigations/2026-07-city-scaling-improvements.md` — owner rulings
> **D7** (git-native B+A end-state), **D8** ("accept 'lose uncommitted work since last
> WIP-push' on pod eviction, WITH push-at-commit-boundary cadence — loss window ≈ minutes"),
> D3 item table ("Polecat worktrees → per-pod ephemeral; pushed branches = durability
> (content-hashed branch names ⇒ idempotent re-derivation); WIP-push cadence for dirty
> work"; "Controller sockets/locks/runtime heartbeats → mayor-pod emptyDir (`GC_RUNTIME_DIR`
> split — already an earmarked upstream task)").
>
> **Ledger stem (verbatim):** `GasCity-Dev::GCD-WO-CSC-002-ephemeral-workspace-mode`
>
> **Naming note (binding):** the kit's shorthand "`GC_RUNTIME_DIR`" refers to the engine's
> REAL runtime-dir override env var **`GC_CITY_RUNTIME_DIR`**
> (`internal/citylayout/runtime.go` `CityRuntimeEnvForRuntimeDir` /
> `TrustedAmbientCityRuntimeDir`). This WO uses the real name everywhere; no new
> `GC_RUNTIME_DIR` variable is introduced.
>
> Verified against this repo's `origin/main` 2026-07-08 @
> `c85d92cf0cfd1215be1467628d6fd2e06db46aae` (read-only `git log -1 --format=%H`).
> Re-verify at execution time and rebase onto current `origin/main` before assessing
> prerequisites.

## Goal

Make hosted Gas City workspaces **fully ephemeral and git-native**: every filesystem thing
an agent pod or mayor pod needs is either (a) reconstructable from git (rig checkouts,
worktrees — cloned/fetched from an in-cluster mirror, WIP durability via
push-at-commit-boundary), (b) mayor-local durable single-writer state on a small volume
(`.gc` city state: `events.jsonl`, `nudges/state.json`, checkpoints), or (c) ephemeral
runtime residue that can live on an emptyDir (`GC_CITY_RUNTIME_DIR` split: sockets, locks,
PID files, traces). No RWX/shared filesystem remains load-bearing anywhere in the engine.

Three engine deliverables (C7-engine, all pinned):

1. **`GC_CITY_RUNTIME_DIR` split → emptyDir-friendly layout.** Classify every `.gc` write
   path as EPHEMERAL vs DURABLE; route every ephemeral class through the existing
   runtime-dir override so an operator (or the k8s render) can point
   `GC_CITY_RUNTIME_DIR` at an emptyDir and get a quiet, small, durable `.gc`.
2. **Rig-from-mirror provisioning.** Rigs gain a remote-source form (the same machinery
   packs already have: `internal/config/pack_fetch.go` clone/update +
   `config.PackSource`-style fields), an **environment-resolved mirror contract**
   (`GC_RIG_MIRROR_BASE` fetch base + `GC_RIG_MIRROR_PUSH_BASE` push base — site-injected,
   never baked into git-tracked `city.toml`), and an idempotent `gc rig ensure` command;
   the k8s no-PVC pod staging provisions rig checkouts by CLONING from the resolved rig
   source instead of streaming whole trees from the controller. Fetch and push are SPLIT
   on purpose: the in-cluster mirror is read-only (fetch accelerator); push-back stays
   git-native to the canonical remote (living doc WS1 finding 5: "city→GitHub is already
   git-native" — this WO must not break it).
3. **WIP-push cadence support (D8)** as a **gastown pack fragment** (generic prompt
   content: push the worktree branch at every commit boundary) plus the
   `worktree-setup.sh` recovery half: when an evicted agent's branch already exists on
   origin, the fresh worktree resumes FROM `origin/<branch>` — pushed branches are the
   durability layer, loss window = since last WIP-push.

Event/nudge transport is **imported, not re-specified**: this WO consumes contract **C6**
(`GasCity-Dev::GCD-WO-CSC-001-runtime-event-nudge-transport` — `api:` events provider,
`GC_NUDGES` seam, `GC_K8S_CONTROLLER_*` projection). Nothing in this WO may re-declare or
alter any C6 surface.

**Pinned contract surface (what AGC-WO-CSC-002's STOP-gates resolve against — these names
are AUTHORITY once merged):**

| Pin | Value |
|---|---|
| Runtime-dir split env var | `GC_CITY_RUNTIME_DIR` (existing; this WO completes its coverage — the kit's "`GC_RUNTIME_DIR`" is shorthand for it) |
| Rig-mirror fetch base env var | `GC_RIG_MIRROR_BASE` — absolute URL, no trailing slash, no path beyond the serving root (e.g. `git://<mirror-svc-dns>:9418`) |
| Rig push base env var | `GC_RIG_MIRROR_PUSH_BASE` — absolute URL base of the canonical writable remote (e.g. `https://github.com/<org>`) |
| Rig clone-URL grammar | `<GC_RIG_MIRROR_BASE>/<repo-name>.git`; push URL `<GC_RIG_MIRROR_PUSH_BASE>/<repo-name>.git`; `<repo-name>` = basename of `Rig.Source` (minus `.git`) when set, else the RIG NAME (deployed cities name rigs by repo — e.g. `vehicle-graph-city/city.toml:61` `name = "Matchpoint-Vehicle-Graph"`) |
| Per-rig explicit override | `Rig.Source` / `Rig.SourceRef` (`city.toml`, optional — see Step 2a) |
| Rig checkout landing path | `Rig.Path` when set (resolved against the city dir); else **`<cityRoot>/rigs/<rig-name>`** (matches the hosted layout and the `GC_RIG_ROOT` pod remap at `internal/runtime/k8s/pod.go:512`) |
| Pod-side pre-provision verify-fetch hook env | **NONE in v1** — deliberately not exposed; AGC-WO-CSC-002's mayor-side `/heads` verify + hourly reconcile are the freshness backstop (its S6.3 fallback branch) |
| No-PVC staging behavior | `GC_K8S_WORKSPACE_PVC` unset ⇒ per-pod `/workspace` via `gc init --from` (existing mode, `provider.go:59-60,309-311,1326-1343`) + rig-dir copy exclusion + in-pod `gc rig ensure` (Step 2d) |

## Dependencies

- **Blocked by:** `GasCity-Dev::GCD-WO-CSC-001-runtime-event-nudge-transport` (same wave
  23; the ledger edge is an apply_deps direct-write per kit A1 §4). Reason: ephemeral pods
  are only safe once events/nudges have a network transport; the k8s staging changes here
  build on WO-001's pod-env projection and land in the same files
  (`internal/runtime/k8s/{provider.go,pod.go}`) — serialized to avoid co-edit conflicts.
- **Consumed by:**
  - `aws-GasCity::AGC-WO-CSC-002-mirror-and-ephemeral-workspaces` (wave 24) — C7-infra:
    renders the per-city `gc-mirror` sidecar (bare repos, fetch-on-webhook), flips hosted
    cities to no-PVC mode, and **injects this WO's `GC_RIG_MIRROR_BASE` /
    `GC_RIG_MIRROR_PUSH_BASE` env** (its S4.1: mayor `env_vars()` + agent-pod
    `agent_env_defaults()` via `GC_K8S_AGENT_ENV_JSON`) with the URL VALUES it owns —
    per K2 it may NOT edit city repos' `city.toml`, which is exactly why the mirror
    contract is env-resolved, not per-rig config. It owns the **mirror URL + provisioning
    contract values** (build plan §6) and the **dispatch-time verify-fetch backstop**, and
    runs the ADR-024 git-reconstruction proof drills (polecat resume/rejection +
    `metadata.work_dir` + eviction drill, vehicle-graph pilot, re-suspend after). It cites
    this WO's pinned contract surface (table above) verbatim.
  - `aws-GasCity::AGC-WO-CSC-007-efs-retirement-cutover` (wave 24) — deletes the shared FS
    this WO makes unnecessary.
- **Repo gates:** `CONTRIBUTING.md`, `TESTING.md` (read before writing any test; use the
  sharded targets), `AGENTS.md` invariants — especially: **"Adding rig config fields: when
  adding a field to `config.Rig`, also add the corresponding optional field to `RigPatch`
  and wire the merge into `applyRigPatch`"** (no field-sync test exists for Rig; manual
  check REQUIRED), ZFC (no judgment calls in Go), no hardcoded roles, atomic file writes.
- **Cities PAUSED** (standing policy + kit K1): engine code + pack content + repo-native
  tests only. No hosted city, no live drill here; the pilot drills belong to
  AGC-WO-CSC-002 and are gated on that WO's explicit pause-exception steps.

## Non-Goals

Bounded-context REJECT rules (kit K2, restated): GasCity-Dev may build **engine Go
(transport, workspace mode), `examples/gastown/packs/*` content (+ `.gc/system/packs` /
`cmd/gc/.gc/system/packs` mirrors IF present — verified ABSENT at authoring SHA
`c85d92cf`; re-verify and keep in sync only if they exist at execution time), packlint
tests**; it must NEVER contain **AWS-specific types in engine or upstream packs;
business/domain literals in codegen-support/gastown; breaking file-mode defaults (non-k8s
cities untouched); new Go role logic for pack behavior (ZFC)**.

Specifically out of scope here:

- **NO transport work** — events/nudge `api:` provider, `GC_NUDGES`, mutation-class
  allowlist, typed nudge endpoints, controller Service are **C6 / GCD-WO-CSC-001**;
  import-cite only. Editing `internal/events/*` or `internal/api/*`, or any nudge
  queue-op/endpoint/provider surface, is a REJECT. The ONE sanctioned touch in
  `internal/nudgequeue` is Step 1c's **path-derivation** edits (`WakeSocketPath` routing;
  `StatePath`/`LockPath` explicitly NOT moved) — path derivation is this WO's C7 seam,
  queue semantics are C6's.
- **NO pod-side pre-provision verify-fetch hook** — deliberately not exposed in v1 (see
  the pinned-contract table); freshness is AGC-WO-CSC-002's mayor-side + hourly lanes.
  Do not add a mirror-control-API client to the engine (the engine treats rig sources as
  opaque git URLs; the control API is an AGC-WO-CSC-002 contract).
- **NO mirror serving** — the `gc-mirror` sidecar image/Deployment/Service, bare-repo
  layout, webhook-triggered fetch, and the mirror URL naming are AGC-WO-CSC-002 (which
  also owns `docker/gascity-mirror/` per kit A1 §5). The engine treats a rig source as an
  opaque git URL.
- **NO EFS/PVC lifecycle** — PVC/PV/access-point deletion, `WorkspaceFileSystem` removal,
  S3 archival are AGC-WO-CSC-007. The engine keeps `GC_K8S_WORKSPACE_PVC` support intact
  (mixed-mode migration; the FLIP to no-PVC is an infra config change, not an engine
  default change).
- **NO evidence-store work** — S3 artifact store is AGC-WO-CSC-003 (D9).
- **NO per-city policy content** — the WIP-push fragment is generic durability discipline;
  city-specific push policies/cadences (if any city wants different) ride city
  `append_fragments`, never upstream edits. No MatchPoint literal anywhere.
- **NO new Go role logic**: the WIP-push behavior is PROMPT content + a shell-script
  recovery branch; Go gains only config/provisioning plumbing (rig fetch, path routing) —
  transport not reasoning.
- **NO changes to refinery/convoy merge machinery, bead storage, or Dolt** — beads are
  already network-backed (`internal/storehealth/storehealth.go:42` class of evidence;
  D3 GO verdict); nothing here touches them.
- **NO scheduled/sync-loop anything** — webhook lane and CronJob demotion are
  AGC-WO-CSC-001.

## Architecture Links

- Kit C7 (engine half) — the pinned design; kit wins over this file on conflict.
- Living doc WS1 "D3 go/no-go" table — the EFS-item → post-EFS-home mapping this WO
  implements the engine half of.
- `engdocs/archive/backlogs/worktree-roadmap.md` — worktree isolation lessons (read before
  touching `worktree-setup.sh`).
- `AGENTS.md` — rig-field sync rule, atomic writes, ZFC, tmux safety.
- `docs/guides/shareable-packs.md` — pack import/override precedence (fragments seam).

## Packages To Inspect (read before writing code)

| Path | Why (file:line evidence) |
|---|---|
| `internal/citylayout/layout.go` | canonical roots (11-33), `RuntimePath` (58-61) — `.gc` join point |
| `internal/citylayout/runtime.go` | the EXISTING split machinery: `RuntimeDataDir`, `CityRuntimeEnvForRuntimeDir` (env projection incl. `GC_CITY_RUNTIME_DIR`), `TrustedAmbientCityRuntimeDir` (anchor-gated override trust), `normalizeRuntimeDir` (in-city coercion) |
| `cmd/gc/controller.go:73-100` | `controllerSocketPath` — `.gc/controller.sock` + sockaddr_un fallback pattern |
| `internal/nudgequeue/state.go:144-175` | `StatePath` (`.gc/nudges/state.json`), `LockPath` (`.gc/nudges/state.lock`), `WakeSocketPath` (`.gc/nudges/wake.sock` + /tmp fallback) |
| `internal/session/submit.go:600-610` | nudge poller PID/log paths (`.gc/nudges/pollers/…`) |
| `cmd/gc/cmd_nudge.go:1719-1809` | poller PID lease (`nudgePollerPIDPath`, `acquireNudgePollerLease`) |
| `internal/citylayout/runtime.go:86` | `SessionNameLocksDir` (`.gc/session-name-locks`) |
| `internal/session/mcp_state.go:111` | session-mcp state (`.gc/session-mcp/<id>.json`) |
| `internal/supervisor/config.go:208` | supervisor publications (`.gc/supervisor/publications.json`) |
| `internal/config/config.go:602-661` | **`Rig` struct** — `Path`, `DefaultBranch`, `Includes`, `Imports`, `FormulaVars`; the struct this WO extends |
| `internal/config/config.go:663+` | `AgentOverride` / `RigPatch` — the patch types that must gain the new fields (AGENTS.md rule) |
| `internal/config/config.go:798-833` | `PackSource` (Source/Ref/Path) + `Import` — the remote-source shape being extended to rigs |
| `internal/config/pack_fetch.go` | `FetchPacks`/`clonePack`/`updatePack` (34-97), `runGit` clean-env + `fetchGitEnvBlacklist` (225-252) — REUSE, do not fork |
| `internal/runtime/k8s/provider.go` | fields (44-73), `usesPersistentWorkspace` (309-311), `podWorkspaceRoot` (313-326), **`initCityInPod` (1326-1343)**, `initBeadsInPod` (1348-1400), `verifyBeadsInPod` (1407-1424) |
| `internal/runtime/k8s/pod.go` | `buildPodEnvForRoot` path remaps (`GC_RIG_ROOT` at 512), runtime-dir projection (`projectedPodRuntimeDirForRoot`, 494-511) |
| `examples/gastown/packs/gastown/assets/scripts/worktree-setup.sh` | worktree creation: branch naming (52-59), origin/HEAD refresh + start-point (111-146), local-branch reuse (127-133), excludes (160-200) |
| `examples/gastown/packs/gastown/agents/polecat/agent.toml` | `pre_start` seam (line 6), `work_dir = ".gc/worktrees/{{.Rig}}/polecats/{{.AgentBase}}"` |
| `examples/gastown/packs/gastown/agents/polecat/prompt.template.md` | fragment includes (`{{ template "…" . }}` at 5,61,65,69,78,125), commit/branch discipline (30-34), affected-test gate (119-122), done-sequence push (258-276) |
| `examples/gastown/packs/gastown/template-fragments/` | fragment file conventions (e.g. `approval-fallacy.template.md` — `{{ define }}` blocks) |
| `internal/builtinpacks/registry.go:50-60` | packs embedded in the binary — new pack files ship via existing `PackFS`, NO embed wiring changes |
| `cmd/gc/cmd_rig*.go` (locate via `grep -rn "rig add" cmd/gc`) | rig CLI family — `gc rig ensure` joins it |
| `Makefile:120-124` | `check-all` + `test-packs` targets |

## Required Inputs

- Repo at current `origin/main` (provenance SHA above; rebase + re-verify).
- The MERGED GCD-WO-CSC-001 tree (its transport seams are upstream of this WO in the same
  wave; if executing before its merge, STOP and re-queue — the k8s staging edits collide).
- NO cloud credentials, NO cluster, NO city runtime. Everything validates against
  `t.TempDir()` git repos, the `k8sOps` fake seam, `make test-packs`-class script harnesses,
  and local `gc` binaries.

## Implementation Steps

### Step 1 — runtime-dir split: classify and route every `.gc` write path

**1a. Classification table (authoritative, shipped as docs).** New engineering doc
`engdocs/architecture/runtime-dir-split.md` containing EXACTLY this table (verified rows;
extend only with additional verified rows found during the Step-1c audit):

| `.gc` path | Writer | Class | Post-split home |
|---|---|---|---|
| `.gc/events.jsonl` (+ rotation archives) | controller (single writer post-C6) | DURABLE | city `.gc` (mayor volume) |
| `.gc/nudges/state.json` | controller/API (post-C6) | DURABLE | city `.gc` |
| `.gc/nudges/state.lock` | same | DURABLE-ADJACENT (lock guards state.json; MUST stay co-located — split-brain risk if lock and file resolve differently) | city `.gc` |
| `.gc/source-state.json` (hosted sync record; written by external tooling) | external | DURABLE | city `.gc` |
| `.gc/supervisor/publications.json` (`internal/supervisor/config.go:208`) | supervisor | DURABLE | city `.gc` |
| `.gc/cache/packs`, `.gc/cache/includes` (`citylayout/layout.go:30-32`) | config layer | RECONSTRUCTABLE (re-fetchable) | city `.gc` (unchanged; cheap) |
| `.gc/system/packs` | gc render | RECONSTRUCTABLE | city `.gc` (unchanged) |
| `.gc/worktrees/**` | pack pre_start (agent-side) | RECONSTRUCTABLE (git; Step 3) | per-pod workspace |
| `.gc/controller.sock` (`cmd/gc/controller.go:93`) | controller | **EPHEMERAL** | runtime dir |
| `.gc/nudges/wake.sock` (`nudgequeue/state.go:164`) | supervisor listener + enqueue ping | **EPHEMERAL** | runtime dir |
| `.gc/nudges/pollers/*.pid`, `*.log` (`session/submit.go:603-607`, `cmd_nudge.go:1720`) | pollers | **EPHEMERAL** | runtime dir |
| `.gc/session-name-locks/` (`citylayout/runtime.go:86`) | session naming | **EPHEMERAL** | runtime dir |
| `.gc/session-mcp/<id>.json` (`session/mcp_state.go:111`) | session layer | **EPHEMERAL** (session-scoped; dies with the session) | runtime dir |
| `.gc/runtime/**` (traces, pack state, services — `citylayout/runtime.go`) | various | already runtime-dir routed | runtime dir (unchanged) |
| `.gc/evidence/<rig>/<bead>/` (eval-attempt JSONL artifacts — GCD-WO-CSC-003's evidence local grammar) | evaluator agents (SHELL prompt-driven writes, not Go) | DURABLE-ish (must survive the workspace; the `evidence_publish_cmd`/`evidence_fetch_cmd` city vars wire it to S3 at un-pause per AGC-WO-CSC-003 spec-18) — NOT ephemeral | city `.gc` |

Caveat (one line): the Step-1d audit test scans Go sources only — shell writers like the
`.gc/evidence/` row will never be collected by it; that row is doc-authoritative.

**1b. Routing helper.** `internal/citylayout/runtime.go` gains:

```go
// EphemeralPath resolves rel under the effective ephemeral-runtime root:
// the trusted GC_CITY_RUNTIME_DIR override when anchored (see
// TrustedAmbientCityRuntimeDir), else the canonical .gc/runtime data dir.
// EVERY socket, lock (except locks that guard durable files), PID file,
// and trace MUST resolve through this helper — the emptyDir split contract
// (CSC C7, engdocs/architecture/runtime-dir-split.md).
func EphemeralPath(cityRoot string, rel ...string) string
```

Semantics: `runtimeDir := TrustedAmbientCityRuntimeDir(cityRoot)`; empty →
`RuntimeDataDir(cityRoot)`; join `rel` under it. (Callers that already thread an explicit
runtimeDir — controller/supervisor — get a sibling
`EphemeralPathForRuntimeDir(cityRoot, runtimeDir string, rel ...string) string` applying
`normalizeRuntimeDir` exactly as `ControlDispatcherTraceDefaultPathForRuntimeDir` does.)

**1c. Route the ephemeral classes** (each edit relocates the path construction ONLY —
zero behavior change when the override is unset, because `EphemeralPath` then resolves
under `.gc/runtime/…` — note this IS a default-path move for these artifacts, from `.gc/X`
to `.gc/runtime/X`; see the compat note below):

- `controllerSocketPath` (`cmd/gc/controller.go:91`) → `EphemeralPath(cityPath,
  "controller.sock")`, keeping the existing sockaddr_un length fallback verbatim.
- `nudgequeue.WakeSocketPath` (`state.go:163`) → `EphemeralPath(cityPath, "nudges",
  "wake.sock")`, keeping the length fallback.
- Poller PID/log paths (`session/submit.go:603-607`, `cmd_nudge.go:1719-1723`) →
  `EphemeralPath(cityPath, "nudges", "pollers", …)`.
- `SessionNameLocksDir` (`citylayout/runtime.go:86`) → `EphemeralPath(cityRoot,
  "session-name-locks")`.
- Session-mcp state (`session/mcp_state.go:111`) → `EphemeralPath(cityPath, "session-mcp",
  …)`.
- `nudgequeue.LockPath` **stays with state.json** (durable-adjacent — table rationale).

**Compat note (pinned):** moving defaults from `.gc/X` to `.gc/runtime/X` is safe for
sockets/PIDs/locks — they are process-lifetime artifacts, never read across versions; the
"No status files — query live state" principle already forbids anything durable there.
BUT writer/reader pairs must move ATOMICALLY in one commit per artifact class (socket
producer + consumer; PID writer + `existingPollerPID` reader), and each class needs a
mixed-version note in the doc (old binary + new binary sharing a city during a rolling
update will not see each other's sockets → same behavior as a dead peer; all such paths
already have liveness fallbacks — cite `pingNudgeWakeSocket`'s fast-dial-failure fallback,
`cmd_nudge.go:1424-1427` comment, and the poller lease recovery).

**1d. Audit sweep (grep battery, becomes a test).** Every `filepath.Join(...,".gc",...)`
and `citylayout.RuntimePath(` call site in non-test Go must appear in the classification
table. Enforce with `internal/citylayout/runtime_split_audit_test.go`: walk the repo's Go
sources (AST or grep-equivalent), collect `.gc` path constructions outside
`internal/citylayout`, assert each maps to a table-listed class via an in-test allowlist
that MIRRORS the doc table (a new unclassified path fails the test with "classify me in
runtime-dir-split.md"). Zero-item guard: the test asserts the collected set is NON-EMPTY
(≥ the 14 known rows) — an empty scan is RED, never green.

### Step 2 — rig-from-mirror provisioning

**2a. Config fields.** `internal/config/config.go` `Rig` struct (602-661) gains, directly
after `DefaultBranch`:

```go
// Source is an optional CANONICAL remote git URL for this rig's repository
// (any URL git clone accepts). It is the explicit per-rig override for
// provisioning: when GC_RIG_MIRROR_BASE is set the fetch still goes through
// the mirror (repo name = Source's basename) and Source becomes the push
// target; without a mirror base, Source is cloned/pushed directly. Hosted
// cities normally leave it EMPTY — the site injects GC_RIG_MIRROR_BASE /
// GC_RIG_MIRROR_PUSH_BASE and the repo name defaults to the rig name, so
// city.toml stays environment-agnostic (never bake cluster DNS into git
// truth). Empty + no mirror env = current behavior (Path must exist).
Source string `toml:"source,omitempty"`
// SourceRef is the git ref to check out when provisioning from Source
// (branch, tag, or commit). Empty = the remote default branch
// (origin/HEAD), falling back to DefaultBranch when set.
SourceRef string `toml:"source_ref,omitempty"`
```

Per the AGENTS.md rig-field rule: add matching optional pointer fields to `RigPatch`
(`*string` each) and wire both into `applyRigPatch` (manual check — no field-sync test
exists for Rig; ADD one now: `internal/config/rig_field_sync_test.go`, reflective
struct-field comparison Rig↔RigPatch modeled on `TestAgentFieldSync` — locate it via
`grep -rn "TestAgentFieldSync" internal/config`).

**2b. Fetch machinery.** New file `internal/config/rig_fetch.go` — REUSE `pack_fetch.go`'s
`runGit` + env blacklist (exported or file-internal shared helper; do NOT duplicate the
blacklist):

```go
// RigEnsureOptions carries the site-injected mirror contract (CSC C7).
// Callers populate it via RigEnsureOptionsFromEnv (GC_RIG_MIRROR_BASE /
// GC_RIG_MIRROR_PUSH_BASE); tests pass values directly. Both values, when
// set, must be absolute URLs without a trailing slash (validation error
// otherwise — fail loudly, never guess).
type RigEnsureOptions struct {
	MirrorBase string
	PushBase   string
}

func RigEnsureOptionsFromEnv() RigEnsureOptions

// RigRepoName returns the repository basename used in the clone-URL
// grammar: basename of rig.Source minus ".git" when Source is set, else
// the rig name (deployed cities name rigs by repo).
func RigRepoName(rig Rig) string

// resolveRigProvisioningURLs pins the fetch/push resolution:
//   fetch: MirrorBase set → "<MirrorBase>/<RigRepoName>.git";
//          else rig.Source when set; else ok=false (no remote form —
//          callers treat as ErrRigNoSource).
//   push:  rig.Source set → rig.Source (the canonical remote);
//          else PushBase set → "<PushBase>/<RigRepoName>.git";
//          else "" — ONLY legal when fetch==rig.Source (clone default push
//          is already canonical). A mirror-derived fetch with NO resolvable
//          push target is a hard ERROR: the mirror is read-only and a rig
//          that cannot push strands every agent's work (WIP-push cadence,
//          done-sequence push, branch recovery all push to origin).
func resolveRigProvisioningURLs(rig Rig, opts RigEnsureOptions) (fetch, push string, ok bool, err error)

// EnsureRig materializes or refreshes a rig checkout from its resolved
// remote source.
//   - missing target (or exists but not a git repo): FULL clone
//     (`git clone [--branch <ref>] <fetch> <target>`) — full history and
//     blobs (correct for worktree/merge-base/rebase operations; cheap from
//     an in-cluster mirror; deliberately NOT --depth 1 like packs, and NOT
//     --filter=blob:none: git-daemon mirrors don't enable
//     uploadpack.allowFilter, and lazy blobs would couple every later
//     checkout to mirror availability). Then, when push != fetch:
//     `git remote set-url --push origin <push>`.
//   - existing repo: `git fetch --prune origin` then fast-forward the
//     current branch ONLY when clean and behind (`merge --ff-only`);
//     a dirty tree or diverged branch is LEFT UNTOUCHED (ErrRigDirty-
//     wrapped) — provisioning must never destroy work. Re-assert the
//     push URL (idempotent set-url) so drifted checkouts converge.
//   - no resolvable remote form: ErrRigNoSource (callers decide whether
//     that is an error or a skip).
// Idempotent; safe to run at every pod start.
func EnsureRig(rig Rig, cityRoot string, opts RigEnsureOptions) error
```

Target-path resolution (pinned): `rig.Path` when set, resolved exactly as the rest of the
config layer resolves it (relative → against the city directory; verify the existing
helper via `grep -rn "resolveRigPaths" cmd/gc` — `beads_provider_lifecycle.go:632` — and
mirror its semantics; note it SKIPS empty paths); **when `rig.Path` is empty — the
deployed-city shape, e.g. `vehicle-graph-city/city.toml:60-64` has no `path` — the target
is `<cityRoot>/rigs/<rig-name>`**, which is byte-consistent with the hosted layout and the
`GC_RIG_ROOT` controller→pod remap (`pod.go:512`), so `worktree-setup.sh` finds the same
`$RIG_ROOT` it does today. Ref precedence: `SourceRef` → `DefaultBranch` → remote HEAD.
Error paths: wrap with rig name (`fmt.Errorf("ensuring rig %q: %w", …)`); no partial
clones left behind (clone into a `.tmp-<name>` sibling then `os.Rename`, matching the
repo's atomic-write convention).

**2c. CLI.** New file `cmd/gc/cmd_rig_ensure.go`: `gc rig ensure [<name>...|--all]`
(register beside the existing rig subcommands — locate the `rig` cobra parent via
`grep -n "\"rig\"" cmd/gc/*.go`). Behavior: resolve city + config; for each named rig (or
all with `--all`): `config.EnsureRig(rig, cityRoot, config.RigEnsureOptionsFromEnv())`;
skip-with-notice on `ErrRigNoSource` (`rig %q: no source resolvable (no source, no
GC_RIG_MIRROR_BASE) — skipped`); any other error → non-zero exit after attempting the rest
(aggregate, report each). `--json` summary flag following the repo's
`writeCLIJSONLineOrErr` idiom (see `cmd_event_emit.go:63`). No timeout imposed by gc
(no-timeouts-on-long-ops: clones may be big; the operator/pod controls the budget).

**2d. k8s no-PVC staging integration.** `internal/runtime/k8s/provider.go`
`initCityInPod` (1326-1343) currently copies the ENTIRE city dir (which, in hosted
layouts, can contain multi-GB rig checkouts) into the pod. Change (pinned):

- `copyDirToPod` of the city source EXCLUDES configured rig checkout directories that lie
  inside the city tree: compute the exclusion set from the loaded city config's rig paths
  (thread the rig paths through `runtime.Config` — inspect how `cfg.Env`/config reach
  `initCityInPod` and extend the provider's staging inputs minimally; if config is not
  available at that seam, derive exclusions from `GC_RIG_ROOT`-class env plus a new
  provider input — pick the smallest seam and document it in the code).
- After `gc init --from` succeeds and before `initBeadsInPod`, exec in the pod:
  `gc rig ensure <rig>` for the session's rig when the session env identifies one
  (`GC_RIG_ROOT` remap already exists at `pod.go:512`; map the pod-visible rig root back to
  the rig name via the staged city config), else `gc rig ensure --all`. Failure = staging
  failure (pod start error) — a pod without its rig cannot work; don't-swallow-errors.
- **Env plumbing (pinned):** the in-pod `gc rig ensure` inherits the container env, which
  carries `GC_RIG_MIRROR_BASE`/`GC_RIG_MIRROR_PUSH_BASE` from `GC_K8S_AGENT_ENV_JSON`
  (`appendAgentEnvDefaults`, `pod.go:572+`; AGC-WO-CSC-002 S4.1 injects the values). Do
  NOT add either var to the `buildPodEnvForRoot` skip map (`pod.go:459-486`) — they are
  agent-side values, not controller-local ones; Test 8 pins the pass-through. The
  rig-exclusion + ensure behavior activates on the same predicate: non-empty
  `GC_RIG_MIRROR_BASE` in the effective pod env (provider `agentEnv` merged with session
  `cfg.Env`) — env absent ⇒ staging sequence byte-identical to today.
- `GC_K8S_WORKSPACE_PVC` set (persistent mode): NO behavior change (exclusion + ensure are
  no-PVC-mode-only; `usesPersistentWorkspace` at 309-311 is the guard).

### Step 3 — WIP-push cadence (D8): pack fragment + worktree recovery

**3a. Fragment.** New file
`examples/gastown/packs/gastown/template-fragments/wip-push-cadence.template.md`, following
the existing fragment file conventions (`{{ define "wip-push-cadence" }}…{{ end }}`; inspect
`approval-fallacy.template.md` for the exact define/comment style). Content requirements
(generic, zero business literals; prompt-voice consistent with the polecat prompt):

- Commit early and often on your per-bead branch; after **EVERY commit**, push:
  `git push origin HEAD` (first push: `git push -u origin HEAD`).
- After any history rewrite (rebase), `git push --force-with-lease origin HEAD` — never
  bare `--force`.
- Rationale line: the workspace is disposable; **pushed commits are the ONLY durable copy
  of in-progress work** — anything uncommitted or unpushed is lost if the session dies
  (loss window = since your last push).
- Never hold work back to "push when done" — the done-sequence push is the LAST push, not
  the first.

**3b. Prompt wiring.** `examples/gastown/packs/gastown/agents/polecat/prompt.template.md`:
add `{{ template "wip-push-cadence" . }}` immediately after the existing worktree/commit
discipline block (the "Stay in your worktree… Commit and push from there." paragraph at
lines 30-34) so the cadence reads as part of branch discipline. This is default-ON for the
gastown polecat (upstream-safe: pushing WIP branches is already this pack's durability
model — content-hashed branch names, done-sequence `git push origin HEAD` at 276).
Refinery/other agents: NOT wired here (merge processors don't produce WIP); cities may
append the same fragment to other agents via `append_fragments`
(`internal/config/config.go:750`) — that is city scope, not this WO.

**3c. Worktree recovery from pushed WIP.** `examples/gastown/packs/gastown/assets/scripts/
worktree-setup.sh`: today, when the local branch is missing (fresh pod), the worktree is
created from `origin/HEAD` (111-146) — an evicted agent's pushed WIP on `origin/<branch>`
would be ORPHANED. Add, between the local-branch check (127) and the default-ref creation
branch (133+):

```sh
# WIP-push recovery (D8/CSC): a prior ephemeral workspace may have pushed
# this deterministic branch. Resume from origin's copy instead of cutting a
# fresh branch from the default ref — pushed branches are the durability
# layer for ephemeral workspaces.
git -C "$RIG_ROOT" fetch origin "$BRANCH" >/dev/null 2>&1 || true
if git -C "$RIG_ROOT" show-ref --verify --quiet "refs/remotes/origin/$BRANCH"; then
    WORKTREE_ADD="git -C $RIG_ROOT worktree add $WT -b $BRANCH refs/remotes/origin/$BRANCH"
elif [ -n "$DEFAULT_REF" ]; then
    ...existing default-ref branch...
```

(exact splice: keep the existing local-branch-exists fast path FIRST — a persistent
workspace with the branch already local is unaffected; the new remote check runs only when
no local branch exists. Preserve the `GIT_LFS_SKIP_SMUDGE=1` prefix and the error/
restore_stage handling of the surrounding code verbatim. Also set the branch's upstream so
the fragment's `git push origin HEAD` targets the same ref:
`git -C "$WT" branch --set-upstream-to "origin/$BRANCH" "$BRANCH" 2>/dev/null || true`
after a successful recovery add.)

Note on per-bead polecat branches (`polecat/<bead-id>`, prompt.template.md:34): those are
created BY the agent inside the worktree and are already pushed by the done-sequence; the
cadence fragment makes their intermediate states durable too. The recovery hook covers the
worktree base branch (`gc-<agent>-<hash>`); bead branches recover via ordinary
`git fetch`/checkout in the resumed worktree (the rejection-resume flow already relies on
the pushed `polecat/<bead-id>` branch — do not change it).

**3d. Pack tests.** The repo's pack-script test harness runs via `make test-packs`
(Makefile:122-124, currently only `domain-handoff`). Add
`examples/gastown/packs/gastown/tests/run-tests.sh` (same structure as
`domain-handoff/tests/run-tests.sh`: bash, fake CLIs on PATH, temp git repos) covering
Step-3c (tests named in Test Coverage), and ADD it to the `test-packs` Makefile recipe
(one line; keep the existing line).

### Step 4 — regeneration + hygiene

`go vet ./...`; `make test` / `make test-fast-parallel`; `make test-packs`;
`make test-cmd-gc-process-parallel` (rig CLI + controller path moves);
NO OpenAPI surface is touched by this WO (no `internal/api` types) — `make dashboard-check`
only if CI requires it for the touched tree. Update `engdocs/architecture/runtime-dir-split.md`
cross-links from `AGENTS.md`'s architecture-docs list ONLY if that list is the discovery
path CI tests (verify; otherwise leave AGENTS.md untouched).

## Git Workflow

Loop-provided worktree/branch; commit early/often; never stash; never weaken a gate;
harness merges. One commit per atomic writer/reader path move (Step 1c compat note).
Fetch + rebase onto current `origin/main` before final acceptance; re-run the full battery
on the rebased tree. Pre-commit hooks active (`git config core.hooksPath` → `.githooks`).

## Test Coverage

Fixture-realism doctrine (REJECT-level) verbatim: real git repositories in `t.TempDir()`
(real commits, real remotes via `git init --bare` + clone — never stubbed git), real
`city.toml` fixtures parsed by the real config loader, pod staging via the real `k8sOps`
fake seam asserting exact exec command lines. **Zero-item runs never green**: the Step-1d
audit test fails on an empty scan; every list assertion requires expected non-zero counts.

1. **Runtime-split routing:** `internal/citylayout/ephemeral_path_test.go` —
   `EphemeralPath` table: override unset → `.gc/runtime/<rel>`; trusted override (env
   anchored per `TrustedAmbientCityRuntimeDir`) → `<override>/<rel>`; unanchored env →
   canonical; in-city-but-outside-`.gc` override coerced (normalizeRuntimeDir parity).
2. **Per-artifact moves:** `cmd/gc/controller_socket_path_test.go` (socket under runtime
   dir + length-fallback preserved), `internal/nudgequeue/state_paths_test.go`
   (wake.sock ephemeral-routed; `StatePath`/`LockPath` UNMOVED — co-location pin as an
   explicit assertion), `internal/session/submit_poller_paths_test.go` (PID/log routed;
   writer and reader resolve identically under an override).
3. **Emptydir end-to-end:** `test/` integration (build-tagged per TESTING.md) or
   `cmd/gc` process test: start a city with `GC_CITY_RUNTIME_DIR` pointed at a separate
   temp dir; controller comes up, socket + wake.sock + poller artifacts appear ONLY under
   the override; `.gc` receives ONLY durable classes (events.jsonl, nudges/state.json,
   cache) — assert by directory diff against the classification table.
4. **Audit battery:** `internal/citylayout/runtime_split_audit_test.go` (Step 1d) — scan
   non-empty; every `.gc` construction classified; adding an unclassified path fails.
5. **Rig field sync:** `internal/config/rig_field_sync_test.go` — Rig↔RigPatch reflective
   sync (catches the new Source/SourceRef in both + `applyRigPatch` merge behavior for
   set/unset/override cases).
6. **EnsureRig behavior:** `internal/config/rig_fetch_test.go` — against real bare-repo
   fixtures (a `file://` bare "mirror" plus a second bare "canonical" remote — real
   commits, real SHAs): **URL-resolution table** (`resolveRigProvisioningURLs`: every
   Source × MirrorBase × PushBase combination incl. repo-name-from-Source vs from-rig-name,
   trailing-slash/relative-URL validation errors, and the pinned HARD ERROR for
   mirror-derived fetch with no resolvable push target); fresh clone is a FULL clone (no
   `--depth`, no `--filter` — args asserted) with ref precedence SourceRef →
   DefaultBranch → HEAD; **push split proven end-to-end**: after EnsureRig, `git push
   origin HEAD` from the checkout lands the ref on the CANONICAL bare remote, not the
   mirror (zero-item guard: the pushed ref must exist there); empty `rig.Path` lands at
   `<cityRoot>/rigs/<rig-name>`; idempotent re-run (fetch+ff, push-url re-asserted);
   behind-clean → fast-forwards; dirty tree → `ErrRigDirty` and tree untouched
   (byte-compare); diverged → untouched + error; no source resolvable → `ErrRigNoSource`;
   missing-parent path → created; failed clone → no partial dir left (atomic rename pin).
7. **CLI:** `cmd/gc/cmd_rig_ensure_test.go` — `--all` mixed outcomes aggregate correctly
   (one no-source skip + one success + one failure → exit non-zero, all three reported);
   `--json` schema-stable line.
8. **k8s staging:** `internal/runtime/k8s/provider_rig_staging_test.go` (fake `k8sOps`) —
   no-PVC staging with `GC_RIG_MIRROR_BASE` in agentEnv: copy excludes in-city rig dirs
   (exec/copy invocations asserted against the exclusion set), `gc rig ensure <rig>`
   exec'd with the session's rig (and `--all` fallback), ensure failure fails `Start`;
   pod env contains `GC_RIG_MIRROR_BASE`/`GC_RIG_MIRROR_PUSH_BASE` (pass-through pin —
   neither stripped by the skip map); mirror env ABSENT → staging invocation sequence
   byte-identical to pre-change golden; persistent-PVC mode: byte-identical golden
   regardless of env (file-mode/persistent parity pins).
9. **Fragment + prompt render:** extend the pack render/structural tests (locate the
   existing prompt-render test harness via `grep -rn "prompt.template" internal/config
   cmd/gc --include=*_test.go`): gastown polecat rendered prompt CONTAINS the
   wip-push-cadence text exactly once; fragment defines parse (`{{ define
   "wip-push-cadence" }}`); no business literal in the fragment (grep-assert none of:
   `matchpoint|vehicle|enrichment|aws` case-insensitive).
10. **Worktree recovery (pack script suite,**
    `examples/gastown/packs/gastown/tests/run-tests.sh`**):**
    - fresh worktree, no origin branch → created from origin/HEAD (existing behavior
      regression-pinned);
    - origin has `gc-<agent>-<hash>` with 2 WIP commits beyond default → new worktree
      checks out FROM origin's branch, both commits present, upstream set (simulated
      eviction: delete local worktree + local branch, keep bare remote — the REAL eviction
      shape);
    - local branch exists → local fast path unchanged (no fetch-created divergence);
    - `--sync` behavior on recovered worktree unchanged;
    - zero-commit guard: the recovery test asserts commit COUNT > base (a run that finds
      nothing to recover must fail, not pass).
11. **Repo gates as named tests:** `make test-packs` includes the new suite (Makefile diff
    asserted by running it); `make check` green.

## Validation

- `make build && make check`, `go vet ./...`, `make test-packs`, sharded suites per
  TESTING.md for touched tiers — all green on the REBASED tree; full battery exactly once
  at final state.
- **Cities-PAUSED clause:** this WO verifies that all GasCity-in-AWS remains paused
  (zero-replica / suspended) before declaring success; it starts NO hosted city and runs
  NO live drill. The ADR-024 proof drills (polecat resume/rejection, `metadata.work_dir`,
  eviction drill on the vehicle-graph pilot, re-suspend after) are AGC-WO-CSC-002's
  explicitly-named pause exception — NOT exercised here.
- **Prod-gate defer language:** no prod-shaped resource exists in this repo; any
  additive-CREATE prod surface discovered downstream auto-defers to the end-game cutover
  per the standing prod-gate defer policy — never actioned from this WO.
- Functional parity: non-k8s cities and persistent-PVC hosted mode behave identically
  (Tests 2, 3, 8 goldens); default-path moves for ephemeral artifacts are documented with
  the mixed-version note (Step 1c).

## Acceptance Criteria (each ← named test)

1. Every `.gc` write path is classified (doc table) and the classification is
   test-enforced; unclassified additions fail CI ← Test 4.
2. `GC_CITY_RUNTIME_DIR` override captures ALL ephemeral artifacts (sockets, PID/log,
   session-name-locks, session-mcp) while durable state stays in `.gc`; lock/state
   co-location pinned ← Tests 1, 2, 3.
3. A city runs end-to-end with the runtime dir on a separate (emptyDir-equivalent) mount ←
   Test 3.
4. `Rig.Source`/`Rig.SourceRef` exist with RigPatch parity and merge wiring; field sync is
   now test-enforced ← Test 5.
5. `EnsureRig` resolves fetch/push per the pinned grammar (`GC_RIG_MIRROR_BASE` /
   `GC_RIG_MIRROR_PUSH_BASE` / `Rig.Source`, rig-name default, `rigs/<name>` landing,
   fail-loud on unpushable mirror fetch), provisions idempotently with a full clone,
   proves push-back to the canonical remote, never destroys local work, leaves no partial
   state ← Test 6.
6. `gc rig ensure` (names/`--all`/`--json`) behaves as pinned ← Test 7.
7. k8s no-PVC staging clones rigs from source instead of streaming them from the
   controller; persistent mode untouched byte-for-byte ← Test 8.
8. WIP-push cadence fragment ships in the gastown pack, renders exactly once in the
   polecat prompt, and contains zero business/domain literals ← Test 9.
9. An evicted workspace's pushed WIP is recovered by `worktree-setup.sh` from
   `origin/<branch>` with upstream set; all pre-existing worktree behaviors
   regression-pinned ← Test 10.
10. `make test-packs` runs the new gastown suite; full check battery green on rebased
    tree ← Test 11 + Validation.

## Risks

- **Same-wave co-edit with GCD-WO-CSC-001** in `internal/runtime/k8s/`: the blocked_by
  edge serializes execution; on rebase, re-verify the WO-001 projection code paths before
  splicing staging changes (do not re-order the env-projection vs staging hunks).
- **Default-path moves (Step 1c)**: any external tooling that greps `.gc/controller.sock`
  or `.gc/nudges/wake.sock` literally will need the runtime-dir-split doc; in-repo callers
  are covered by the audit test. Hosted-side path consumers are re-rendered by
  AGC-WO-CSC-002 (it imports the doc table).
- **Read-only mirror + push split**: the in-cluster mirror serves fetch only (git daemon,
  no receive-pack) — every push path (WIP cadence, done-sequence, recovery upstream) MUST
  target the canonical remote. The resolveRigProvisioningURLs hard-error (mirror fetch
  without push target) plus Test 6's end-to-end push assertion are the guards; a regression
  here silently strands agent work on pods.
- **Rig-name = repo-name default**: the clone-URL grammar's rig-name fallback matches the
  deployed convention (verified: `vehicle-graph-city/city.toml:61`; the GCD-WO-CSC-007
  fan-out table lists all cities' rigs by repo name). A future city whose rig name diverges
  from its repo name must set `Rig.Source` — documented in the field comment and the
  runtime-dir-split doc's companion section.
- **Mayor-side runtime-dir adoption is infra's move**: this WO makes the split
  engine-complete; RESOLVED by kit A2.8 — AGC-WO-CSC-002 S4.2 delivers the mayor emptyDir
  + `GC_CITY_RUNTIME_DIR` + `GC_CITY_PATH` anchor render (the anchor is required or the
  override trust-check no-ops, kit A3.2); AGC-WO-CSC-006B carries nothing here. Defaults
  are safe either way (everything lands under `.gc/runtime` on the PVC).
- **Worktree-setup splice fragility**: the script has staging/restore trap logic around
  the creation block — the recovery branch must live INSIDE the existing error-handling
  structure (Test 10's regression cases guard the fast paths).
- **Rig-name ↔ pod-env mapping** in Step 2d may reveal that the staging seam lacks config
  access; the WO pins "smallest seam + document" — if that turns into a provider-interface
  change touching WO-001 code, keep it additive and note it in the PR body for the
  evaluator.
- **Fragment shadowing** (WS2 risk K1 analog): `wip-push-cadence` is a NEW name — no
  shadow risk upstream; cities appending it elsewhere use `append_fragments` (documented
  seam).
- **No committed `.gc/system/packs` mirrors** exist at authoring SHA; if they appear by
  execution time (another WO), sync them per kit K2 — check before final commit
  (`ls .gc/system/packs cmd/gc/.gc/system/packs`).

## Done Means

- All Acceptance Criteria green on the rebased tree; evidence transcripts available.
- The engine seams AGC-WO-CSC-002 imports are now merged authority (the pinned-contract
  table in Goal): `GC_RIG_MIRROR_BASE` + `GC_RIG_MIRROR_PUSH_BASE` + the clone-URL grammar
  and `rigs/<name>` landing rule, `Rig.Source`/`SourceRef` + `EnsureRig` + `gc rig ensure`,
  `GC_CITY_RUNTIME_DIR` coverage (`EphemeralPath` + the classification doc), the no-PVC
  staging contract, the WIP-push fragment + recovery semantics, and the explicit
  NO-verify-hook-env ruling. Nothing downstream re-declares them.
- Non-k8s cities and persistent-PVC mode provably unchanged.
- `engdocs/architecture/runtime-dir-split.md` shipped and audit-linked.
- Working tree clean, branch pushed.

## Master cutover contribution

**None (platform repo, no AWS).** Hosted materialization (mirror sidecars, no-PVC flip,
EFS retirement, drills) is tracked by aws-GasCity CSC WOs (AGC-WO-CSC-002/003/007) in
their own cutover entries.

## WO-CS v1 conformance (audited 2026-07-14 — Track C)

Track C audit-wave C-W1 amendment. Authorities: `master/generation-architecture/
IMPLEMENTATION-CHECKPOINT.md` §5 (C-2);
`Matchpoint-Platform/specs/patterns/SKILL-work-order-audit-and-authoring.md` v3.0.0 §1B
(WOC-1..11). ADDITIVE layer: nothing above is weakened. Amended under the loop's
build-phase PAUSE (ruling R3) with this unit verified PENDING at 0 runs first-hand.

### WOC map (component → disposition)

| WOC | disposition |
|---|---|
| WOC-1 execution classification | UPGRADED in place (R-C2 live-tier terms) |
| WOC-2 deliverables + AC-named tests | in-body, verified (every AC names its test) |
| WOC-3 negative scope fence | in-body, verified complete: every deferred seam names its owner (GCD-WO-CSC-001/C6 — transport; AGC-WO-CSC-002 — mirror serving, verify-fetch hook, mayor runtime-dir render; AGC-WO-CSC-007 — EFS/PVC lifecycle; AGC-WO-CSC-003 — evidence store; AGC-WO-CSC-001 — webhook/CronJob lane; city `append_fragments` — per-city push policy) |
| WOC-4 static premises | ADDED — `## Premises (drift gate)` + `## Specs impact` below |
| WOC-5 runtime premises | ADDED below (`library-id: UNWRITTEN (Track B)`, ruling R2) |
| WOC-6 coordination declaration | ADDED below |
| WOC-7 policy defaults | in-body (pinned-contract table) + declaration below |
| WOC-8 seam probe / anchor record | ADDED — anchor record below. Executable probe of the consumed upstream artifact (C6 seams): IMPOSSIBLE at authoring — GCD-WO-CSC-001 is unmerged; the blocked_by serialization + the Required Inputs STOP ("if executing before its merge, STOP and re-queue") are the guard; RP-2 below makes the probe a Step-0 runnable check |
| WOC-9 pattern + telos pins | ADDED below |
| WOC-10 same-motion doc/index obligations | in-body (runtime-dir-split.md + Done Means) + `## Specs impact` below; index motion N/A (pre-SVA @0; Track B) |
| WOC-11 TCS declaration + schema law | ADDED below |
| Residue manifest (GEN-6) | ADDED below + acceptance fold (AC-T1) |

### Anchor re-verification record (WOC-8 — 2026-07-14, GasCity-Dev origin/main @ `e3a3a1673600`)

Verified first-hand (read-only git): the engine dirs this WO anchors
(`internal/citylayout`, `internal/nudgequeue`, `internal/config`, `internal/runtime/k8s`,
`internal/session`, `cmd/gc` non-test) AND the gastown pack
(`examples/gastown/packs/gastown/`) are byte-identical `c85d92cf..e3a3a167` (only
`cmd/gc/embed_builtin_packs_test.go` changed — not anchored here). All `file:line`
references above hold at `e3a3a167`. Note: `internal/builtinpacks/registry.go` HAS changed
since `a47df8f5` (telos packs registered) — this WO cites it only as "no embed wiring
changes needed" context (`registry.go:50-60` may have shifted lines; re-anchor by content).
The committed `.gc/system/packs` mirrors remain ABSENT at `e3a3a167` (Non-Goals check
re-verified).

### Runtime premises (WOC-5)

`library-id: UNWRITTEN (Track B)` (ruling R2 — honest marker, never a dead pointer).
Park-vs-repair per THIS WO's own text (ruling R-C1 Beat-5).

| # | premise (re-verify at Step 0) | runnable check | on failure |
|---|---|---|---|
| RP-1 | repo base is current `origin/main` | `git fetch origin && git log -1 --format=%H origin/main`, then rebase | REPAIR (rebase + re-verify) |
| RP-2 | GCD-WO-CSC-001 MERGED (its transport seams are upstream in the same wave) | `grep -rn "apitransport" cmd/gc/providers.go internal/events/` non-empty on the rebased tree (its Step-1 package + registration) | PARK (WO text: "STOP and re-queue — the k8s staging edits collide") |
| RP-3 | toolchain present | `make setup && make build` | REPAIR |
| RP-4 | cities remain PAUSED — no hosted interaction | per this WO's Cities-PAUSED Validation clause | PARK (standing policy + kit K1) |
| RP-5 | no committed `.gc/system/packs` mirrors appeared | `ls .gc/system/packs cmd/gc/.gc/system/packs` (expect absent) | REPAIR (WO text: sync per kit K2 before final commit) |

### Coordination declaration (WOC-6)

co_repos: `[]` — single-repo unit (matches the harness ledger). Validation traversal set
is entirely in-repo (`t.TempDir()` git repos, fake `k8sOps`, `make test-packs` harnesses).
Ordering: blocked_by `GasCity-Dev::GCD-WO-CSC-001` (same-wave direct-write edge — the
serialization that substitutes for a co-edit conflict in `internal/runtime/k8s/`);
consumed-by `aws-GasCity::AGC-WO-CSC-002` and `AGC-WO-CSC-007` via import citations of the
pinned-contract table (publish-first, adopt-behind-ordering-edges; SYSTEM-TELOS §4 rule 7
lane-1 posture). Deploy surfaces: NONE touched (`live-tier: none`); the hosted mirror
sidecar / no-PVC flip / EFS retirement surfaces belong to the AGC WOs.
`register: UNWRITTEN (Track B)` — no DEPLOY-SURFACES register exists yet (ruling R2).

### Policy defaults (WOC-7)

Pinned in-body as binding law: the pinned-contract table (env var names, clone-URL
grammar, `rigs/<name>` landing, NO verify-fetch hook in v1), full-clone (never `--depth`/
`--filter`), atomic `.tmp-<name>` + rename, fetch/push split with the mirror-fetch-
without-push hard error, `ErrRigDirty` never-destroy posture. Seed default declared by
this audit (WOC-7, binding unless the WO text overrides): `make dashboard-check` runs
ONLY when CI requires it for the touched tree (no `internal/api` surface is touched) — as
Step 4 already states; a generator asking to confirm any pinned default is a template
defect.

### Pattern + telos pins (WOC-9)

Telos pins: `GasCity-Dev/specs/TELOS.md` v3 @ `16026788515b` +
`Matchpoint-Platform/specs/patterns/SYSTEM-TELOS.md` v2 @ `08994e13e751`. Catalog
patterns: NONE pinned (no-stretch rule — engine-fork workspace/provisioning work; no AWS
resource named, PAT-001 does not bind). No `specs/patterns/` consumer stubs / shard in
this repo (pre-adoption; Track B).

### Test-contract declaration (WOC-11 — every row marked; unmarked = authoring-audit RED)

| tier | path class | proving test (path::name) or N/A + justification |
|---|---|---|
| T1 logical | decision logic | Test 6 `internal/config/rig_fetch_test.go` URL-resolution table (every Source × MirrorBase × PushBase combination + the pinned hard error — gold table inline) + Test 1 `internal/citylayout/ephemeral_path_test.go` routing table. Misbuild check: the Test 6 combination table + Test 5 reflective Rig↔RigPatch sync are the misbuild pins; no mutation-testing harness exists (recorded honestly) |
| T1 logical | parity oracle (refactor-sensitive) | Test 8 byte-identical staging goldens (mirror-env absent; persistent-PVC mode) + Test 3 directory-diff against the classification table + Test 2 co-location pin (`StatePath`/`LockPath` UNMOVED) |
| T2 behavioral | happy | Test 6 fresh clone + idempotent re-run + push-back-to-canonical end-to-end; Test 10 origin-branch recovery |
| T2 behavioral | failure (full-spectrum via T4 negatives) | Test 6 dirty/diverged/no-source/URL-validation errors; Test 7 CLI aggregate failure (exit non-zero, all reported). No generated negative pack exists (see T4) |
| T2 behavioral | destructive | Test 10 simulated eviction (delete local worktree + branch, keep bare remote — the REAL eviction shape) |
| T2 behavioral | partial-failure (forced single-leg) | Test 6 failed-clone-leaves-no-partial-dir (atomic rename pin); Test 7 mixed-outcome aggregation (skip + success + failure in one run) |
| T2 behavioral | zero-item (never a GREEN path) | Test 4 audit scan asserts NON-EMPTY (≥ the 14 known rows); Test 10 zero-commit guard (a recovery run that finds nothing to recover FAILS) |
| T3 contract | schema consumed/published | N/A + justification: the exported contract surface is an env/config GRAMMAR (pinned-contract table: env var names, clone-URL grammar, landing rule) + CLI + script behavior — no data-shape payload crosses a seam; downstream consumption is WO-contract import citation + STOP-gates (per premise-pinning CONTRACT: WO files are not pinnable), not schema interchange. No PAT-030 schema class exists here (anti-scope) |
| T4 fixtures | pack import | N/A + justification: no fixture-pack substrate; fixtures are REAL git repositories/bare remotes built in-test per the fixture-realism doctrine (ecosystem note as in GCD-WO-CSC-001's audit) |
| T5 integration | estate-E2E registration | N/A + justification: platform-fork repo; no estate same-diff suite row exists (ecosystem debt) |
| T5 integration | requires-siblings | none (H3) — the MERGED GCD-WO-CSC-001 tree is an ORDERING premise (RP-2), not a sibling test dependency |
| T6 live | live proof | N/A: live-tier `none` — the ADR-024 proof drills (polecat resume/rejection, `metadata.work_dir`, eviction on the vehicle-graph pilot) are aws-GasCity::AGC-WO-CSC-002's explicitly-named pause exception (owner named in-body) |

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
exists. (The Step-2d "smallest seam + document" fallback, if taken, is a `delivered` row
naming the chosen seam — see Risks.)

### Acceptance criteria — Track C additions (binding, additive)

- **AC-T1 (residue manifest):** the structured result carries the GEN-6 residue manifest
  above; every non-delivered/known-gap row maps to an EXISTING vehicle; silent residue =
  REJECT.
- **AC-T2 (same-motion specs impact):** the `## Specs impact` declaration below holds at
  merge (a false `none` is a reject — CONTRACT §5.5).

## Premises (drift gate)

> premises-watermark: GasCity-Dev@0 + Matchpoint-Platform@79 (authored 2026-07-14)

| spec doc | version | sha256-12 | assumed fact |
|---|---|---|---|
| specs/TELOS.md | 3 | 16026788515b | repo telos card: business-agnostic orchestration-SDK fork; §4 change law binds every diff; §3 row 11 pins the dolt/bd floors + bead-shadow durability this WO's D3 posture relies on |
| Matchpoint-Platform::specs/patterns/SYSTEM-TELOS.md | 2 | 08994e13e751 | estate telos head; §4 rule 7 governs the downstream (aws-GasCity) adoption lane of this WO's contracts |
| master/city-scaling-improvements/wo-authoring-kit.md | 1 | 68a95bd19427 | C7 names THIS WO the engine authority (kit wins on conflict); A1 §4 same-wave direct-write edge; A2.8 mayor-side runtime-dir render = AGC-WO-CSC-002 S4.2; K1 cities-paused; K2 bounded context (sha-only lane — ungoverned master/ doc) |
| master/DOCTRINE-fixture-realism-and-lifecycle-seam-acceptance.md | 2 | 8ee5795d2e6d | fixture-realism REJECT-level: real git repos/remotes in tests, zero-item runs never green |

## Specs impact

none — this WO's doc deliverable is `engdocs/architecture/runtime-dir-split.md` (a NEW
engineering doc, named in Done Means and audit-linked by Test 4) plus pack content; no
governed `specs/` doc is invalidated (`specs/TELOS.md` §3 row 11's dolt/bd/durability
facts are untouched by a path-routing change; `specs/architecture.md` describes the
object model / wire surfaces, which this WO does not alter). No `specs/SPECS-INDEX.md`
exists (pre-SVA `@0` sentinel) — no index motion.
