{{ define "refinery-rebase-integration-bootstrap" }}
---

## REBASE STEP — INTEGRATION BRANCH SELF-HEAL

**This section adds a guard BEFORE the formula's `rebase` step runs
`git rebase origin/$TARGET`.** It activates ONLY when `$TARGET` starts
with `integration/`. For non-integration targets the guard is a no-op
and the formula's rebase step runs unchanged.

### Why this exists

The cartographer formula (`formulas/spec-cartographer.formula.toml`
step `emit`) pre-creates `origin/integration/<wo-slug>` from
`origin/main` before emitting any task beads, so the canonical flow
never trips on a missing branch. This guard is a defensive backstop
for paths the cartographer does NOT own:

- Operator runs `gc convoy create --owned --target integration/foo`
  manually (the documented upstream gascity pattern — that command sets
  metadata but does NOT push the branch).
- A future workspace formula targets an integration branch without
  going through cartographer.
- Cartographer regressed or its emit step ran partially.

Without this guard, those paths produce work beads whose
`metadata.target` references a branch that doesn't exist on origin,
and your `git rebase origin/$TARGET` fails with
`fatal: invalid upstream 'origin/<target>'` — not a normal rebase
conflict, so the formula's conflict-rejection path does not handle it
cleanly. See `runbooks/cartographer-wo014-test-findings.md` Issue D
for the originating incident.

### What to do

**Before running `git rebase origin/$TARGET`** in the `rebase` step,
run this check verbatim:

```bash
case "$TARGET" in
  integration/*)
    if ! git ls-remote --exit-code origin "refs/heads/$TARGET" >/dev/null 2>&1; then
      echo "integration branch $TARGET missing on origin; creating from origin/main"
      git fetch --prune origin main
      if ! git push origin "origin/main:refs/heads/$TARGET"; then
        echo "FATAL: could not create $TARGET from origin/main; escalating to mayor"
        gc mail send mayor/ \
          -s "ESCALATION: integration branch $TARGET missing and unrecoverable" \
          -m "Work bead: $WORK
Branch: $BRANCH
Target: $TARGET

The rebase step needs origin/$TARGET to exist, and creating it from
origin/main failed (likely auth, network, or branch-protection).

Bead is left assigned to refinery; investigate the push failure and
either create the branch manually or reassign the bead away from
refinery."
        gc runtime drain-ack
        exit 1
      fi
      # Refresh the remote-tracking ref so the subsequent rebase sees it.
      git fetch --prune origin "$TARGET"
    fi
    ;;
esac
```

Then proceed with `git rebase origin/$TARGET` as the formula
instructs.

### Idempotency notes

- The `git ls-remote --exit-code` check makes the create a no-op on
  re-entry (resume after crash, repeat polecat rebase).
- Multiple refineries (or a refinery + cartographer) racing to create
  the same branch: whichever loses the push gets a `! [rejected]
  (fetch first)` error and the `git push` step exits non-zero —
  triggering escalation. In practice this is exceptionally unlikely
  (cartographer pre-creates BEFORE any task bead exists, so refinery
  has no work to race against until that's done), but if you see it,
  re-running the check is safe: the second iteration's `ls-remote`
  finds the branch and skips the push.

### Scope

This guard applies ONLY when `$TARGET` starts with `integration/`.
Targets like `main`, `develop`, or any feature branch fall through
unchanged. The formula's existing failure-handling for normal rebase
conflicts is untouched.
{{ end }}
