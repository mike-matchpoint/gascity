# Runtime Read Contract

Status: active for the runtime read isolation cutover started 2026-05-27.

GasCity runtime loops must keep controller progress bounded even when Dolt or
the Beads CLI is slow. Hot runtime reads therefore declare a read class and may
not silently fall back from cache or indexed SQL to hydrated `bd list`, `bd
query`, `bd ready`, `bd show`, or `bd dep list`.

## Read Classes

| Class | Budget | Routes | Fallback | Failure behavior |
| --- | ---: | --- | --- | --- |
| `hot-authoritative` | 3.5s caller, 2s target store | complete cache or indexed SQL | none | defer the affected action and emit/return degradation |
| `hot-degraded-ok` | 2s caller, 1s target store | cache, indexed SQL, bounded partial cache | none | return partial/degraded state and keep the loop alive |
| `foreground-authoritative` | command timeout | indexed acceleration, Beads CLI | allowed | return command error after the operator command budget |
| `maintenance` | explicit maintenance deadline | serialized maintenance worker or operator command | allowed only outside hot ticks | retry/report through maintenance state |

`internal/beads.RuntimeList` and `internal/beads.RuntimeReady` are the canonical
entry points for hot runtime code. `BdStore.List`, `BdStore.Ready`,
`BdStore.Get`, and `BdStore.DepList` remain foreground-authoritative APIs.

## Current Call-Site Classification

Runtime hot paths must use the runtime API:

- `cmd/gc/build_desired_state.go`: controller demand list/ready probes use
  `hot-degraded-ok`.
- `cmd/gc/session_reconciler.go`: lifecycle assigned-work gates use
  `hot-authoritative`; diagnostic/work-dir lookups use `hot-degraded-ok`.
- `cmd/gc/order_dispatch.go` and `internal/orders/runtime_helpers.go`: order
  dispatch reads are being migrated in the dispatch snapshot phase. Until that
  phase lands, these call sites are classified as hot and incomplete for full
  cutover.
- `internal/beads/caching_store*.go`: cache prime/reconcile may use indexed SQL
  and must degrade instead of broad hydrated fallback when invoked by hot
  runtime code.

Foreground/operator paths may keep Beads CLI fallback:

- `gc bd ...` commands and manual diagnostics.
- explicit order sweep/cleanup commands.
- migration/import/export tools and tests that exercise foreground behavior.

Maintenance paths must not run implicitly inside controller ticks. Backup,
managed Dolt GC, broad cleanup, and export commands are covered by the
maintenance-isolation phase.

## Regression Rules

- Hot runtime callers pass a `beads.ReadPolicy` at the call site.
- A `hot-authoritative` or `hot-degraded-ok` read returning unsupported,
  timeout, incomplete dependency coverage, incomplete label coverage, or
  connection errors returns `*beads.DegradedReadError`.
- Hot runtime code must not call `bd list`, `bd query`, `bd ready`, `bd show`,
  or `bd dep list` after that degradation.
- New controller/order/session/cache hot reads must be added to this inventory
  and covered by a runner-spy or static guard test.

