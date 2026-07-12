# Execution City Operations Pack

This pack provides generic execution supervision, evidence, and evaluation
operations. It contains no business rubric, surface implementation, cohort,
or grader. Importing cities supply those bindings.

## Eval binding convention

A city publishes binding formulas named `<domain>.eval_run_cohort.v1` and
`<domain>.eval_replay_step.v1`. A binding formula invokes the corresponding
generic formula and supplies:

- `surface_kind`: the exact `gc.kind` selected by the city's surface-under-test pool;
- `grader_cmd`: the complete deterministic grading command;
- `cohort_ref` for cohort runs or `fixture_ref` plus `fixture_id` for replay;
- `threshold` and optional `gate_metric` for cohort promotion gates;
- `run_id`, `eval_suite`, and the qualified `binding_prefix` used to route the pack's eval agents.

`binding_prefix` includes a trailing dot. Its default is
`execution-city-operations.`; a renamed import supplies its own qualified
prefix. Bindings pass values as formula vars. They do not copy or edit these
formulas.

The cohort plan emits at least one case. The controller-owned `fan-out-cases`
step expands each item through the private `eval-run-case` graph fragment and
closes only after every case fragment reaches a terminal result. Aggregate
depends on that control step, so it cannot race case execution. Replay expands
the same private fragment inline, keeping materialization, routing, and grading
behavior identical across both public contracts.

## Deterministic grading and triage

The surface agent writes a trace; it never grades. The controller executes
`assets/scripts/eval-grader-check.sh`, which loads the injected `grader_cmd`,
runs it, validates its structured result, and preserves that result as an
artifact. The cohort threshold is evaluated by
`assets/scripts/eval-threshold-check.sh`.

Failed gates and replay regressions are filed through
`assets/scripts/eval-file-triage.sh`. It validates the evidence packet, creates
one bead per distinct failure class, and routes each bead to the city-qualified
`prompt-eval-classifier`. The established route continues from classifier to
judge or evidence gatherer, then to the work filer when deterministic code
work is justified.

See `template-fragments/eval-run-contract.template.md` for the manifest,
fixture alignment, and complete routing contract.
