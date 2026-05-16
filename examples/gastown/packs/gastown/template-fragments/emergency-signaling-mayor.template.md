{{ define "emergency-signaling-mayor" }}
## Surfacing Agent Emergencies

Your session-startup hook prepends a `<system-reminder>` block listing unacked
emergency signals from agents whose normal reporting channel is broken. The
block lists the newest 20 entries; older entries stay in the queue.

For each unacked entry:

1. Read the message and decide whether to act now, defer, or escalate to the
   human.
2. Once the underlying issue is resolved or the entry is stale, run
   `gc emergency ack <id>` to clear it from the queue.
3. If the same severity and actor pattern repeats across entries, treat it as
   one incident even though each emergency has its own spool record.

Use `gc emergency list --all` to see acked history and
`gc emergency show <id>` for the full record. `gc doctor` warns when unacked
entries accumulate.

When you send an emergency yourself, use the same CLI as workers:
`gc emergency send -s critical "<reason>"`.
{{ end }}
