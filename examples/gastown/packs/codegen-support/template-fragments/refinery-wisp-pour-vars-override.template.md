{{ define "refinery-wisp-pour-vars-override" }}
---

## WISP POUR — CANONICAL COMMAND (SUPERSEDES)

**This section supersedes every `gc bd mol wisp mol-refinery-patrol` invocation shown earlier in this prompt** (in the Startup section and inside the formula's `next-iteration`, `rebase`, and `handle-failures` rejection paths). Use the canonical command below instead, unchanged.

The earlier recipes pass three vars — `target_branch`, `rig_name`, `binding_prefix` — and rely on every other formula variable falling through to its `[vars.<name>].default` declared in the formula TOML. That is incorrect for any formula variable whose effective value differs from its formula-level default. In particular, `integration_branch_auto_land` defaults to `"false"` in the formula, which disables the integration-branch auto-land check inside `next-iteration`. With the earlier recipe, no convoy can ever be auto-landed regardless of what `[rigs.formula_vars]` says in city config, because `bd mol wisp` does not consume `[rigs.formula_vars]` at pour time.

### Canonical wisp-pour command

Use this verbatim everywhere you would otherwise type `gc bd mol wisp mol-refinery-patrol ...`:

```bash
gc bd mol wisp mol-refinery-patrol --root-only \
  --var target_branch={{ .DefaultBranch }} \
  --var rig_name={{ .RigName }} \
  --var binding_prefix={{ .BindingPrefix }} \
  --var integration_branch_auto_land=true
```

`integration_branch_auto_land=true` is required for the integration-branch landing path to run inside `next-iteration`. Omitting it leaves owned convoys orphaned with their integration branches stuck N commits ahead of `${target_branch}`.

If additional rig-level overrides are added to `city.toml`'s `[rigs.formula_vars]` in the future, append matching `--var key=value` lines here. The set of variables passed at pour time IS the set the wisp will use; no city-config fallback applies.

### Verification

After pouring a new wisp, verify the variable took effect by re-reading the wisp bead:

```bash
gc bd show "$NEW_WISP_ID" --json | jq '.[0].metadata.formula_vars // .[0].metadata.vars // empty'
```

The output should include `integration_branch_auto_land: true`. If absent, the pour did not honor the flag — re-pour with the explicit `--var` and burn the prior wisp.

### Sanity check before ending the turn

Before any patrol turn ends (find-work timeout, drain-ack, or any path that pours the next iteration's wisp), confirm the pending open wisp carries the integration-branch auto-land flag:

```bash
PENDING=$(gc bd list --assignee="$GC_AGENT" --status=open --type=epic --json \
  | jq -r '.[] | select(.title | startswith("mol-refinery-patrol")) | .id' | head -1)
[ -n "$PENDING" ] && gc bd show "$PENDING" --json \
  | jq -e '(.[0].metadata.formula_vars.integration_branch_auto_land // .[0].metadata.vars.integration_branch_auto_land // "false") == "true"' >/dev/null \
  || { echo "WARN: pending patrol wisp $PENDING missing integration_branch_auto_land=true"; }
```

The warning is informational; the canonical pour above is the authoritative writer of this state.
{{ end }}
