{{ define "execution-prompt-eval-contract" }}
## Prompt Eval Evidence And Decision Contract

Prompt eval work is about improving an agentic process without confusing
prompt behavior, eval harness behavior, domain policy, and deterministic code.

Evidence packet fields:

- eval suite, case ID, prompt name, prompt version, model/provider, and run ID
- expected outcome, actual outcome, score, threshold, and failure label
- prompt input, redacted output excerpt, trace excerpt, and scorer rationale
- fixture, corpus, or review-packet artifacts used by the eval
- related production execution, incident, or user-visible failure if any
- prior passing run, regression window, and changed dependency when known
- evaluator limitations, nondeterministic variance, and missing artifacts

Judge decision categories:

- `no_change_eval_noise`: acceptable variance or bad failure signal.
- `change_prompt`: prompt instructions, examples, tool-use policy, or output contract should change.
- `change_eval_fixture`: fixture, expected answer, grading rubric, or threshold is wrong.
- `change_eval_harness`: scorer, parser, runner, redaction, or artifact collection is broken.
- `add_corpus_or_case`: coverage is missing and a new fixture/corpus slice is needed.
- `domain_policy_review`: business/domain policy is ambiguous and needs a domain reviewer.
- `deterministic_runtime_bug`: code, adapter, storage, event, or command behavior is wrong.
- `model_or_provider_issue`: provider behavior changed or model choice is unsuitable.
- `needs_more_evidence`: evidence is too thin for a change decision.

Every judge decision must name the exact target surface to change, the
acceptance criteria, validation command or eval suite to run, rollback/cleanup
expectations, and the blocked execution or release gate it unblocks.
{{ end }}
