# Work Order: GCD-WO-CSC-002 — ephemeral workspace mode (runtime-dir split, rig-from-mirror provisioning, WIP-push cadence)

NOTE: this WO is long — read the FULL file at
`specs/agent-work-orders/GCD-WO-CSC-002-ephemeral-workspace-mode.md` in your worktree before
implementing; tail amendments are BINDING.

Execution classification: **Dev-only** (`boundary=dev`, **wave 23**,
`blocked_by` = `GasCity-Dev::GCD-WO-CSC-001-runtime-event-nudge-transport` — same-wave
edge, wired as an apply_deps DIRECT-WRITE per kit ADDENDUM A1 §4). Engine Go + gastown pack
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
   `config.PackSource`-style fields) plus an idempotent `gc rig ensure` command, and the
   k8s no-PVC pod staging provisions rig checkouts by CLONING from the rig source instead
   of streaming whole trees from the controller.
3. **WIP-push cadence support (D8)** as a **gastown pack fragment** (generic prompt
   content: push the worktree branch at every commit boundary) plus the
   `worktree-setup.sh` recovery half: when an evicted agent's branch already exists on
   origin, the fresh worktree resumes FROM `origin/<branch>` — pushed branches are the
   durability layer, loss window = since last WIP-push.

Event/nudge transport is **imported, not re-specified**: this WO consumes contract **C6**
(`GasCity-Dev::GCD-WO-CSC-001-runtime-event-nudge-transport` — `api:` events provider,
`GC_NUDGES` seam, `GC_K8S_CONTROLLER_*` projection). Nothing in this WO may re-declare or
alter any C6 surface.

## Dependencies

- **Blocked by:** `GasCity-Dev::GCD-WO-CSC-001-runtime-event-nudge-transport` (same wave
  23; the ledger edge is an apply_deps direct-write per kit A1 §4). Reason: ephemeral pods
  are only safe once events/nudges have a network transport; the k8s staging changes here
  build on WO-001's pod-env projection and land in the same files
  (`internal/runtime/k8s/{provider.go,pod.go}`) — serialized to avoid co-edit conflicts.
- **Consumed by:**
  - `aws-GasCity::AGC-WO-CSC-002-mirror-and-ephemeral-workspaces` (wave 24) — C7-infra:
    renders the per-city `gc-mirror` sidecar (bare repos, fetch-on-webhook), flips hosted
    cities to no-PVC mode, sets rig `source` values to the in-cluster mirror URLs, owns the
    **mirror URL + provisioning contract** (build plan §6) and the **dispatch-time
    verify-fetch backstop**, and runs the ADR-024 git-reconstruction proof drills
    (polecat resume/rejection + `metadata.work_dir` + eviction drill, vehicle-graph pilot,
    re-suspend after). It cites this WO's `Rig.Source`/`gc rig ensure` seams verbatim.
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
  allowlist, controller Service are **C6 / GCD-WO-CSC-001**; import-cite only. Editing
  `internal/events/*`, `internal/nudgequeue/*`, or `internal/api/*` transport surfaces
  here is a REJECT.
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
// Source is an optional remote git URL for this rig's repository (any URL
// git clone accepts; hosted cities point it at the in-cluster mirror —
// provisioning contract: AGC-WO-CSC-002). When set, `gc rig ensure` (and
// k8s no-PVC pod staging) can materialize/refresh the checkout at Path.
// Empty = current behavior everywhere (Path must already exist).
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
// EnsureRig materializes or refreshes a rig checkout from its remote Source.
//   - missing Path (or Path exists but is not a git repo): clone.
//     Clone shape (pinned): `git clone --filter=blob:none [--branch <ref>]
//     <source> <path>` — full history, on-demand blobs (cheap against an
//     in-cluster mirror; correct for worktree/merge-base operations, unlike
//     the packs' depth-1 shallow clone).
//   - existing repo: `git fetch --prune origin` then fast-forward the
//     current branch ONLY when clean and behind (`merge --ff-only`);
//     a dirty tree or diverged branch is LEFT UNTOUCHED (exit
//     ErrRigDirty-wrapped) — provisioning must never destroy work.
//   - rig.Source == "": ErrRigNoSource (callers decide whether that is an
//     error or a skip).
// Idempotent; safe to run at every pod start.
func EnsureRig(rig Rig, cityRoot string) error
```

Path resolution: `rig.Path` exactly as the rest of the config layer resolves it (relative →
against the city directory; verify the existing resolution helper via
`grep -rn "rig.Path" internal/config cmd/gc | grep -v test` and reuse it). Ref precedence:
`SourceRef` → `DefaultBranch` → remote HEAD. Error paths: wrap with rig name
(`fmt.Errorf("ensuring rig %q: %w", …)`); no partial clones left behind (clone into a
`.tmp-<name>` sibling then `os.Rename`, matching the repo's atomic-write convention).

**2c. CLI.** New file `cmd/gc/cmd_rig_ensure.go`: `gc rig ensure [<name>...|--all]`
(register beside the existing rig subcommands — locate the `rig` cobra parent via
`grep -n "\"rig\"" cmd/gc/*.go`). Behavior: resolve city + config; for each named rig (or
all with `--all`): `config.EnsureRig`; skip-with-notice on `ErrRigNoSource` (`rig %q: no
source configured — skipped`); any other error → non-zero exit after attempting the rest
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
   fixtures: fresh clone (blob-filter arg asserted, ref precedence SourceRef →
   DefaultBranch → HEAD), idempotent re-run (fetch+ff), behind-clean → fast-forwards,
   dirty tree → `ErrRigDirty` and tree untouched (byte-compare), diverged → untouched +
   error, no Source → `ErrRigNoSource`, missing-parent path → created, failed clone →
   no partial dir left (atomic rename pin).
7. **CLI:** `cmd/gc/cmd_rig_ensure_test.go` — `--all` mixed outcomes aggregate correctly
   (one no-source skip + one success + one failure → exit non-zero, all three reported);
   `--json` schema-stable line.
8. **k8s staging:** `internal/runtime/k8s/provider_rig_staging_test.go` (fake `k8sOps`) —
   no-PVC staging: copy excludes in-city rig dirs (exec/copy invocations asserted against
   the exclusion set), `gc rig ensure <rig>` exec'd with the session's rig (and `--all`
   fallback), ensure failure fails `Start`; persistent-PVC mode: invocation sequence
   byte-identical to pre-change golden (file-mode/persistent parity pin).
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
5. `EnsureRig` provisions from a remote source idempotently, never destroys local work,
   leaves no partial state ← Test 6.
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
- **`--filter=blob:none` against exotic git servers**: partial clone requires server
  support; the mirror (git-http-backend class) supports it, GitHub supports it. `EnsureRig`
  must surface the git error verbatim on failure — no silent fallback to full clone
  (predictability over magic); operators can unset the filter expectation only by code
  change (deliberate).
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
- The engine seams AGC-WO-CSC-002 imports are now merged authority: `Rig.Source`/
  `SourceRef` + `EnsureRig` + `gc rig ensure`, `EphemeralPath` + the classification doc,
  the no-PVC staging contract, the WIP-push fragment + recovery semantics. Nothing
  downstream re-declares them.
- Non-k8s cities and persistent-PVC mode provably unchanged.
- `engdocs/architecture/runtime-dir-split.md` shipped and audit-linked.
- Working tree clean, branch pushed.

## Master cutover contribution

**None (platform repo, no AWS).** Hosted materialization (mirror sidecars, no-PVC flip,
EFS retirement, drills) is tracked by aws-GasCity CSC WOs (AGC-WO-CSC-002/003/007) in
their own cutover entries.
