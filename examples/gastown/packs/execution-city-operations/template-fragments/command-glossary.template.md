{{ define "execution-command-glossary" }}
## Command Reference

Use `/gc-work`, `/gc-dispatch`, `/gc-agents`, `/gc-mail`, or `/gc-city` to load
command reference for any topic.

Common commands:

```bash
gc prime
gc work claim --status=in_progress --json
gc bd show <id> --json
gc bd update <id> --notes "..."
gc bd close <id> --reason "..."
gc mail inbox
gc mail read <id>
gc mail archive <id>
gc session list
gc session peek <agent> --lines 50
gc session nudge <agent> "message"
gc runtime drain-ack
```
{{ end }}
