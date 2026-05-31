{{ define "polecat-validate-before-commit" }}
---

## VALIDATE BEFORE COMMIT — TESTS AND LINT MUST PASS

You do not commit, push, or hand off until the bead's validation surface passes against your changes. The refinery is a second line of defense, not the first; reaching refinery with broken code burns a full handoff cycle and produces a rejection-reason rebound. Catch it here.

### Step 1 — Identify the validation surface

In order of precedence:

1. **The bead body's `## Validation commands` (or equivalent) section.** If the bead author wrote concrete commands, those are authoritative. Run them verbatim.
2. **The bead body's `## Acceptance criteria` section** when criteria are mechanically checkable (grep guards, import-isolation checks, `python -c` import probes). Translate them into commands.
3. **The repo's `AGENTS.md § Commands § Validation` (or equivalent)** when the bead doesn't supply commands. Use the canonical lint + test invocations the repo declares.
4. **Heuristic fallback** when no doc surface defines validation: scan the changed paths and pick the obvious matching commands (`pytest` against changed test dirs, the language's standard lint + format check, project-level `just test` or `make test` if present). Note in your commit message that you fell back because the bead didn't specify.

The bead's "Validation commands" section overrides AGENTS.md, which overrides heuristics. Never skip a level higher than necessary.

### Step 2 — Run the surface, capture output

Run every command from Step 1. Capture stdout + stderr + exit code. A non-zero exit on ANY command is a failure.

```bash
{ <command> ; } > /tmp/polecat-validate-<step>.log 2>&1
echo "exit=$?"
```

Run lint before tests — lint errors are usually faster to fix and catching them first avoids polluting test output with import-style problems. Run the most-targeted suite first (e.g., the package containing your changes) before running the full suite.

### Step 3 — Classify failures

For each non-passing command, classify:

- **Caused by your changes** — fix it. Iterate: edit → re-run the failing command(s) → confirm pass. Don't proceed until the surface is clean.
- **Pre-existing, unrelated to your changes** — verify by running the same command against the merge-base (`git stash && git checkout <merge-base> -- <relevant paths>`, or a temporary worktree). If it fails on the base too, the failure is pre-existing.
  - Document the pre-existing failure in your commit message AND in a note on the bead (`gc bd update <issue> --notes "Pre-existing failure: <command> — see /tmp/polecat-validate-<step>.log"`).
  - Do NOT silently push through pre-existing failures. The refinery's merge gate will fail on them; you have to escalate so a human can fix or waive them.
  - Mail witness/mayor with the evidence; pause your work until the gate clears or you receive explicit "proceed despite pre-existing failure" authorization.
- **Caused by missing infrastructure (e.g., `.venv` absent, dependencies not installed)** — bootstrap with the canonical commands from AGENTS.md (`just install`, `uv sync`, etc.) and re-run. If bootstrap itself fails, escalate; do not push.

### Step 4 — Record evidence in the commit

Your commit message body must include the validation evidence block:

```
Validation:
- <command 1>: pass (N tests)
- <command 2>: pass
- <command 3>: pass
```

If you encountered any pre-existing failure or fell back to a heuristic surface, name it explicitly:

```
Validation:
- just lint: pass
- just test: pass (47 tests)
- pre-existing: <name>::<test_id> fails on base; not addressed (see bead note)
```

A commit without a validation evidence block is incomplete — the witness, refinery, and any future polecat resuming this work need to be able to read what was checked.

### Step 5 — Acceptance criteria checks

If the bead has an `## Acceptance criteria` section, translate the criteria into commands (or grep guards, import probes, etc.) and run them as part of Step 2. The bead body is the contract; passing only the tests but missing an acceptance criterion is a regression by definition. Examples of mechanical criteria:

```bash
# "Module X must not import workstream libraries"
! grep -nE "from aws_cdk.(aws_stepfunctions|aws_lambda) import" apps/.../substrate_stack.py

# "Class Y is referenced exactly in files A and B"
git grep -nE "ClassName" -- <expected-paths> | wc -l == <expected-count>

# "Module Z imports successfully"
python -c "from <module> import <symbol>"
```

These belong alongside the test commands in Step 2.

### Anti-patterns

- **Skipping validation when the change "looks small."** Even a one-line change can break import resolution or a downstream test. Run the surface.
- **Running only the tests you wrote** while skipping the existing suite. Regressions in untouched code are exactly what the existing suite catches.
- **Treating refinery as the validator.** The refinery's merge-and-test step is the second line of defense, not the first. A polecat that pushes broken code consumes a refinery rejection cycle and a fresh polecat respawn — both expensive.
- **"Tests are flaky, I'll just retry"** without investigating. If a test passes on retry, document why (race condition? network?) on the bead. If you can't, escalate.
- **Pushing despite a single failing test** by rationalizing it as "unrelated." If you didn't verify it fails on the merge-base too, you don't know it's unrelated. Verify, then escalate, then push only with authorization.
- **Suppressing output to make the commit summary "clean."** If a command emitted warnings, the commit-message evidence block records that they were warnings, not silenced.

### Escalation

If you cannot get the validation surface to pass after 2–3 fix iterations, OR if you discover a pre-existing failure that blocks your work, escalate per the rejection/escalation discipline in the base prompt — mail witness with the failing output and the bead ID. Do NOT push the failing work; leave the worktree intact so the next polecat (or witness) can resume.

{{ end }}
