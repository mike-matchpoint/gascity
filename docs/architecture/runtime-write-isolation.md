# Runtime Write Isolation

Status: active for the runtime write isolation cutover started 2026-05-30.

GasCity runtime loops must keep controller progress bounded even when the
Beads CLI or Dolt is slow. Beads remains the durable source of truth for work,
session state, and order audit rows; this contract adds a bounded write-side
adapter around existing Beads CLI commands for hot runtime callers.

## Write Classes

| Class | Budget | Route | Fallback | Failure behavior |
| --- | ---: | --- | --- | --- |
| `hot-state` | 2s caller | runtime writer | none | report degraded state, keep controller loop alive |
| `reservation` | 1s caller | runtime writer with deterministic ID/idempotency | none | defer unsafe action on ambiguous or failed reservation |
| `cursor-reservation` | 1s caller | runtime writer with deterministic cursor key | none | defer event action before execution |
| `post-action-critical` | 2s caller | runtime writer and compensator | none | record ambiguous/failed write for bounded repair |
| `audit-repair` | 1s caller | runtime writer queue | none | pause or retry through compensation |
| `foreground-authoritative` | command timeout | existing `Store` methods and Beads CLI | allowed | return command error after operator budget |
| `maintenance` | explicit deadline | serialized maintenance/operator path | allowed outside hot ticks | retry/report through maintenance state |

`internal/beads.RuntimeCreate`, `RuntimeUpdate`, `RuntimeCloseAll`, and
`RuntimePing` are the canonical entry points for hot writes. `BdStore.Create`,
`Update`, `CloseAll`, `Ping`, `Get`, and `List` remain
foreground-authoritative APIs.

## Durable Authority

Runtime write isolation does not introduce a new durable data store and does
not write Beads or Dolt tables directly. The runtime writer still invokes
`bd`, but every hot invocation is run through a caller-owned context,
process-group cleanup, a short wait delay, an explicit write class, and an
idempotency key where duplicate outcomes are possible.

Local order runtime leases are ephemeral single-flight and compensation state
only. They never become authoritative work or session state. A
tracking-required order action may start only after the local lease and a
durable Beads tracking reservation are created with a deterministic ID inside
the reservation budget. If that reservation fails or times out ambiguously, the
order is deferred before execution.

Only orders marked in source as both idempotent and tracking-degraded-allowed
may run while tracking persistence is degraded. The default for existing orders
is tracking-required.

## Runtime Writer Manager

Each canonical store key owns one runtime write manager. The manager provides:

- one active Beads writer per store key.
- a bounded queue of 128 runtime writes.
- idempotency-key coalescing for duplicate queued or active writes.
- explicit `DegradedWriteError` outcomes: `not-started`,
  `ambiguous-timeout`, `failed`, `partial`, and `unsupported`.
- per-store stats for queue depth, drops, collapses, timeouts, failures,
  completions, oldest backlog age, and breaker state.
- a circuit breaker that opens after repeated runtime write timeouts or
  failures. While open, audit and maintenance compensation are refused,
  tracking-required work defers, hot-state writes may still report state, and
  a bounded `RuntimePing` recovery probe can close the breaker after Beads
  write health recovers.

Runtime write methods deliberately unwrap `CachingStore` before writing. Hot
writes must not invoke `CachingStore` post-write `Get` readbacks or silently
fall back to foreground Beads CLI semantics after a runtime timeout.

## Observability

Runtime write errors must carry enough information for order dispatch, session
reconciliation, status, and doctor to explain the degraded path: caller, class,
operation, idempotency key, store key, outcome, and wrapped error. When
`GC_BD_TRACE` is set, runtime writes append a diagnostic line containing the
caller, class, operation, command subcommand, duration, timeout, outcome, and
store key without logging raw metadata payloads.

Doctor and status surfaces should prefer these runtime writer stats and
degradation records over foreground Beads probes so diagnostics do not wedge
behind the same Beads/Dolt stall they are reporting.

## Regression Rules

- Hot runtime write callers pass a `beads.WritePolicy` at the call site.
- Hot runtime write paths never call foreground `Store` write methods after a
  runtime timeout.
- Runtime writes never go through `CachingStore.Create`, `Update`, `Close`, or
  `CloseAll` readback paths.
- Deterministic creates treat a duplicate ID with matching reservation
  metadata or idempotency metadata as success, and a duplicate with mismatched
  metadata as a conflict.
- New controller/order/session hot writes must be added to this contract and
  covered by runner-spy, timeout, process-cleanup, or static guard tests.
