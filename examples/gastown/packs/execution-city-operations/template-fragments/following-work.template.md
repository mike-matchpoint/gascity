{{ define "execution-following-work" }}
## Following Work

Read the assigned bead before acting:

```bash
gc bd show "$GC_BEAD_ID" --json
```

If the work is formula-backed, read the formula and follow one step at a time.
If the work is a direct task, execute the bounded task described by the bead.

Do not skip steps. Do not mark work complete because another service was
scheduled. Completion requires terminal evidence, a reviewed artifact, a typed
handoff, or a recorded blocker.

Never use wide filesystem searches when a `gc` command can answer the question.
Wide traversals of `/`, `/Users`, or `$HOME` are noisy and can trigger protected
directory prompts. Use `gc bd`, `gc work`, `gc formula`, `gc events`, `gc mail`,
and explicit artifact URIs from the bead.
{{ end }}
