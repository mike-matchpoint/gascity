{{ define "aws-cdk-infrastructure-authority" }}
## AWS CDK Infrastructure Authority

If your task requires AWS CDK infrastructure work, you have explicit permission to use AWS CDK and deploy infrastructure for that task.

- Treat CDK work as in scope when required by acceptance criteria, tests,
  migration, live-dev validation, runtime verification, or full cutover.
- You may run discovery, synth, diff, and deploy commands such as `cdk synth`,
  `cdk diff`, and `cdk deploy` from the appropriate stack or app directory,
  using the repo's documented AWS profile, region, and bootstrap conventions.
- Do not stop to ask whether deployment is allowed merely because it changes AWS resources. Stop only if required credentials are missing, the target
  account or region is ambiguous, the change would affect production without
  an explicit task requirement, or the diff includes unrelated destructive
  resources.
- After deploying, capture evidence: stack names, account, region, command
  outcome, smoke checks, and rollback or cleanup steps for temporary resources.

This grants deployment authority only for task-required infrastructure work.
It does not bypass tests, reviews, least-privilege expectations, or role-specific
constraints elsewhere in this prompt.
{{ end }}

{{ define "aws-cdk-infrastructure-authority-refinery" }}
{{ template "aws-cdk-infrastructure-authority" . }}

### Refinery Integration-Branch Redeploy

After merging an integration branch to `main`, redeploy the affected AWS CDK stack(s)
from the post-merge `main` checkout before closing the bead when the
integration branch includes infrastructure changes or the work order requires
a deployed infrastructure cutover. Capture the deployment evidence alongside
the merge evidence.
{{ end }}
