{{ define "execution-eval-run-contract" }}
## Eval Run Contract

Public formula contracts are `eval.run_cohort.v1` and
`eval.replay_step.v1`. Finalizers write
`schemas/eval/eval-run-manifest.v1.schema.json`; they never rewrite fixtures,
traces, grader output, or scores.

### City binding

An importing city names its binding formulas `<domain>.eval_run_cohort.v1`
and `<domain>.eval_replay_step.v1`. The binding invokes the generic formula
with formula variables: `surface_kind`, the complete `grader_cmd`,
`cohort_ref` or `fixture_ref` plus `fixture_id`, threshold inputs where
applicable, run identity, and `binding_prefix`. The prefix includes a trailing
dot and qualifies `eval-runner` and `prompt-eval-classifier`. A binding owns
domain prompts, cohorts, rubrics, and grader code; this pack owns orchestration
only.

### Recorded-fixture field alignment

The fixture reference resolves to one complete `EvalStepFixture@v1`-shaped
object. Its fields map as follows:

- `fixture_id` maps to the case row `fixture_id`; its canonical shape is
  `STEP#<gc_kind>#<case>` and its middle segment equals `gc_kind`.
- `gc_kind` maps to the injected `surface_kind` routing value.
- `input.payload` and `input.scope` are the exact execution input.
- `workspace_fixture_refs[]` entries retain `path` and `content_sha256` and are materialized without mutation.
- `expected_terminal_status`, `expected_domain_commands`, and
  `rubric_expectations` are grader inputs, never agent judgments.
- `dimension_registry_stamp` and `provenance` remain fixture provenance.

The actual trace retains terminal status, a non-empty command sequence,
exactly one `manifest.json` and one `output.json` artifact ref with URI and
SHA-256, and an optional events excerpt. Grading results retain status,
terminal-status match, expected and actual status, matched/missing/unexpected
command sets, weighted rubric scores, and citation findings.

The run manifest aligns by convention with shared prompt-eval run records:
`run_id`, start/completion timestamps, case count, aggregate score axes, and
artifact refs preserve their meanings. The standalone manifest adds the exact
formula contract, suite/cohort reference, inline case rows, grader identity,
threshold, and gate outcome. Thresholds align with a separate promotion-gate
interface; they are not represented as run-row fields. This is a documented
field convention only. The pack imports no external contract code.

### Evidence and routing

A failed gate or replay regression carries `triage_packets`. Each packet
contains every field in `execution-prompt-eval-contract`: eval suite, case ID,
prompt name/version, model/provider, run ID, expected and actual outcome,
score, threshold, failure label, prompt input, redacted output excerpt, trace
excerpt, scorer rationale, fixture/corpus/review artifacts, related execution
or incident, prior passing run, regression window, changed dependency,
evaluator limitations, nondeterministic variance, and missing artifacts.

The deterministic filer validates these fields, groups by `failure_label`,
and creates one bead per distinct class with
`gc.routed_to=<binding_prefix>prompt-eval-classifier`. The classifier routes
to the judge or evidence gatherer, and proven deterministic defects can route
to the work filer. Agents do not hand-roll alternate packet or routing paths.
{{ end }}
