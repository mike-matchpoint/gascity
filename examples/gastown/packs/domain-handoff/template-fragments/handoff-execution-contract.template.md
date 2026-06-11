{{ define "domain-handoff-execution-contract" }}
## Domain Handoff Execution Contract

**Contract version: 1.1.** This pack is the single source of truth. The
machine-readable schemas live beside this pack at
`schemas/events/gascity-work-requested.v1.schema.json`,
`schemas/events/gascity-execution-terminal.v1.schema.json`,
`schemas/events/gascity-domain-command-requested.v1.schema.json`, and
`schemas/events/gascity-domain-command-terminal.v1.schema.json`. If this prose
and a schema ever diverge, the schema wins.

A deterministic domain runtime (for example an AWS EventBridge pipeline)
requests agentic work with `GasCityWorkRequested.v1`. The ingress adapter
turns each accepted request into ONE durable bead with
`gc.kind=event_handoff`, `gc.routed_to=payload.route`, and every
`payload.metadata.*` key projected onto the bead as `payload.<key>` metadata.
Four deterministic pack orders own the machine ends of the lifecycle — agents
never publish to the event bus themselves:

- `handoff-work-dispatch` cooks `payload.formula_name` as a molecule ATTACHED
  to the handoff bead and stamps `handoff.molecule_root`. The molecule's step
  beads are the execution spine.
- `domain-command-publish` publishes open `domain_command_request` waiter
  beads to the domain bridge.
- `domain-command-reconcile` applies returned command terminals to waiters
  and unblocks the steps that depend on them.
- `handoff-terminal-sweep` publishes the final
  `GasCityExecutionTerminal.v1` when the molecule terminates and closes the
  handoff bead.

### Molecule mode

A claimed bead that carries `gc.root_bead_id` metadata is a step of a handoff
molecule. In molecule mode:

- **Do NOT emit next-stage intake beads.** The molecule already materialized
  every pipeline stage as a dependency-ordered step bead; emitting another
  would duplicate the stage. Freeform (non-molecule) work keeps the usual
  emit-next-kind chaining.
- The workflow root is the handoff bead named by `gc.root_bead_id`. Read the
  execution context from its `payload.*` metadata: `payload.execution_id`,
  `payload.target_env`, `payload.correlation` (JSON), `payload.variables`
  (JSON), `payload.artifact_prefix`, `payload.formula_bundle_hash`.
- Write artifacts under the execution's artifact prefix
  (`<artifact_prefix>/beads/<bead-id>/...`); record every artifact URI on the
  bead before closing.
- **Failure recording:** a step that cannot complete sets
  `handoff.step_status=failed` plus a `--notes` blocker on itself BEFORE
  closing; the terminal sweep folds failed steps into a FAILED execution
  terminal. Never close a failed step silently.
- **Runtime fan-out:** a planning step that must expand per-item work creates
  one intake bead per item (each carrying `gc.kind` and the execution's
  `payload.execution_id`/`payload.artifact_prefix` metadata) and wires every
  created bead with `gc bd dep <item> --blocks <fan-in-step>` so the fan-in
  step stays invisible to work queries until all items close. Locate sibling
  steps by querying beads with the same `gc.root_bead_id` and the target
  `gc.kind`.
- **Semantic terminal outcomes:** when an execution ends in a judged state
  (for example an approver rejection rather than a runtime failure), the
  finalizing step stamps `handoff.terminal_status` (`SUCCEEDED`, `FAILED`,
  `REJECTED`, `CANCELLED`, `TIMED_OUT`) and `handoff.terminal_reason` on the
  WORKFLOW ROOT handoff bead; the sweep publishes them verbatim. A finalizing
  step that produced the execution manifest stamps
  `handoff.artifact_manifest_uri` on the workflow root — required for a
  SUCCEEDED terminal whenever the request set
  `payload.artifact_manifest_required=true`.

### Requesting a deterministic domain action

Agents never mutate protected domain state and never call domain services
directly. To run an allowlisted domain command (survey, eval, build,
validation, decision write, publication request):

1. Write the command input artifact to the execution's artifact prefix and
   build the FLAT `GasCityDomainCommandRequested.v1` payload (see the schema;
   `command_id`, `execution_id`, `target_env`, `command`, `idempotency_key`,
   `payload_s3_uri`, `requested_at` are required). Derive `command_id` and
   `idempotency_key` deterministically from the execution, step, and item so
   retries dedupe.
2. Create the waiter bead and block your step (or the step the result gates):

```bash
CMD_PAYLOAD_FILE="${TMPDIR:-/tmp}/domain-command-payload.$$.json"
# ... write the flat command payload JSON to "$CMD_PAYLOAD_FILE" ...
jq -e . "$CMD_PAYLOAD_FILE" >/dev/null || { echo "command payload is not valid JSON" >&2; exit 1; }

META=$(jq -n --arg origin "$GC_BEAD_ID" --slurpfile payload "$CMD_PAYLOAD_FILE" \
  '{ "gc.kind": "domain_command_request", origin_bead: $origin,
     payload: ($payload[0] | tojson) }')
WAITER=$(gc bd create "domain command $(jq -r .command "$CMD_PAYLOAD_FILE") for $GC_BEAD_ID" \
  --type task --metadata "$META" --json | jq -r '.id // empty')
rm -f "$CMD_PAYLOAD_FILE"
[ -n "$WAITER" ] && [ "$WAITER" != "null" ] || { echo "FAILED to create waiter bead" >&2; exit 1; }
gc bd dep "$WAITER" --blocks "$GC_BEAD_ID"
gc bd update "$GC_BEAD_ID" --notes "Waiting on domain command waiter $WAITER"
```

3. Exit WITHOUT closing your step (`gc runtime drain-ack`). The step is now
   blocked and invisible to work queries. When the command terminal arrives,
   the reconcile order closes the waiter, your step becomes ready again, and
   the pool re-claims it: read `handoff.command_status` and
   `handoff.command_terminal` from the closed waiter (named in your step's
   notes) and continue. A FAILED or REJECTED command is YOUR judgment call:
   retry with a new waiter, record a step failure, or escalate — never ignore
   it.

Do not put reply addressing in the payload; `domain-command-publish` injects
the standard reply block. Do not hand-roll `aws events put-events`; the only
approved publish paths are the deterministic pack orders above.
{{ end }}
