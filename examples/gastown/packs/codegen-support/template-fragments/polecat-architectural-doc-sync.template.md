{{ define "polecat-architectural-doc-sync" }}
---

## ARCHITECTURAL DECISIONS UPDATE DOCS IN THE SAME COMMIT

If the bead you are implementing asserts a **structural shape** — a rename, a split, an extraction, a move-between-modules, a new stack/lambda module, or any change that establishes how future code in this area will be organized — your commit MUST include a diff to the matching invariant doc or ADR alongside the code change.

### Detecting the situation

There are two cases:

**Case 1: A sibling spec-update bead exists.** The arbiter that filed your bead may have already filed a sibling `spec_update` bead under the same convoy. Detect it:

```bash
gc --rig "$GC_RIG" bd list --parent "$source_convoy" --json \
  | jq -r '.[] | select(.metadata."gc.kind" == "owned_convoy_architectural_doc_update") | .id'
```

If a sibling exists:

- The sibling bead body specifies the file path and the exact prose to add. Copy-paste that prose; do not improvise.
- Land the spec change in the same commit as the code change.
- Close both beads with cross-references in the close reasons.

**Case 2: No sibling spec-update bead, but you observe yourself creating a structural shape.** Triggers (any of):

- Creating a new file under `apps/infrastructure/cdk/stacks/`, `apps/lambdas/`, `apps/fargate/`, or `packages/`.
- Renaming a class that other modules import.
- Splitting a module into multiple files.
- Moving a construct or class from one module to another.
- Changing a public constructor signature in a way that propagates to call sites in `app.py` or other stacks.
- Introducing a new cross-stack reference shape (construct handle vs naming-helper string).

If you see any of these and no sibling spec-update bead exists:

1. **Stop implementing.**
2. Reread the bead body — does it cite an existing invariant, ADR, or rule that already covers this shape? If yes, proceed and quote the citation in your commit message (Case 3 below).
3. If no citation exists, the arbiter missed a doc update. File a question bead routed back to the arbiter:

   ```bash
   gc --rig "$GC_RIG" bd create \
     --type=question \
     --parent="$source_convoy" \
     --metadata "gc.routed_to=$GC_RIG/codegen-support.landing-arbiter" \
     --metadata "blocks=$YOUR_BEAD" \
     --title "Structural shape detected without spec update: <shape>" \
     --description "..."
   ```

   Pause the repair and wait for the arbiter's sibling bead before resuming.

**Case 3: Existing citation covers the shape.** If you confirmed (via the bead body or your own read) that the structural shape replicates an existing pattern with a binding citation, proceed without a new doc update. Quote the citation in your commit message per the cross-reference discipline below.

### What does NOT count as a structural shape

Proceed without doc updates for any of:

- Adding methods, attributes, or helpers to an existing class without changing its public signature.
- Adding a new resource to an existing stack consistent with the stack's existing concern (e.g., adding a new IAM policy to a stack that already manages IAM).
- Tests, fixtures, behavioral cases.
- Imports, formatting, lint fixes.
- Configuration-only changes (SSM paths, tag values, naming-helper outputs).

### The doc target

When the sibling spec-update bead names the file, use it. When you must locate the target yourself (Case 2 escalation post-arbiter-reply, or rare Case 3 verification):

1. The numbered invariant docs at `specs/01-...` through `specs/14-...` are the first lookups for general structural patterns.
2. `specs/adr/` is for new decisions that reverse or refine prior ADRs.
3. Module docstrings (e.g., a stack's opening triple-quoted string) are for stack-local conventions.

Never write a structural-shape decision into a bead body or commit message as the sole record. The bead store is ephemeral; specs are persistent.

### Cross-reference discipline in the commit message

```
<commit subject>

<body explaining the code change>

Architectural decision: <one sentence of the structural shape>
Recorded in: specs/<path>:<lineno>  (or specs/adr/<NNN>-<slug>.md)
Per landing-failure bug $BUG; sibling spec-update bead $SPEC_BEAD_ID.
```

If `$SPEC_BEAD_ID` is absent (Case 3), replace the last line with:

```
Replicates existing pattern: <spec line / ADR / module docstring path:line>
```

### Anti-patterns

- Implementing a structural shape without checking for a sibling spec-update bead. The check is mandatory whenever your bead title or body mentions rename / split / extract / new stack / new module / cross-stack reference.
- Writing your own spec wording when a sibling bead exists. The arbiter wrote the wording precisely so two arbiters facing the same conflict reach the same precedent; your wording divergence defeats the purpose.
- Proceeding past Case 2 without filing the question bead. A missed doc update by the arbiter is a system gap; surface it.

{{ end }}
