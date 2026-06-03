{{ define "execution-incident-taxonomy" }}
## Generic Execution Incident Taxonomy

Use this taxonomy before creating follow-up work:

- `execution_runtime_failure`: deterministic execution failed or timed out.
- `deterministic_service_bug`: a service produced wrong behavior for a valid request.
- `adapter_or_contract_bug`: an integration payload, schema, or artifact contract is broken.
- `event_ingest_or_delivery_failure`: event bus, intake, outbox, DLQ, or replay behavior is suspect.
- `artifact_contract_violation`: required terminal artifacts, citations, or manifests are missing or malformed.
- `prompt_or_eval_failure`: prompt evaluation, agent review packet, or eval scoring needs judgment.
- `domain_data_or_evidence_gap`: source evidence is absent, ambiguous, or out of scope.
- `domain_schema_or_policy_question`: the domain model or policy needs a business-specific decision.
- `code_capability_gap`: deterministic code lacks a needed capability.
- `operator_or_secret_config_blocker`: runtime configuration, credentials, IAM, or operator action is required.
- `duplicate_or_known_issue`: an existing open issue or work order already covers the incident.
- `unknown_needs_evidence`: evidence is too thin to classify safely.

Disposition choices:

- close as healthy/no-op with evidence
- route to evidence bundling or witness review
- route to a domain-specific execution agent
- create bounded utility work for a dog
- file a codegen bug
- file a codegen work order/change request
- escalate to the mayor or operator
{{ end }}
