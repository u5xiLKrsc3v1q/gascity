{{ define "emergency-signaling" }}
## When the Reporting Channel Itself Is Broken

If a `bd update`, `bd close`, `gc mail send`, or other write fails because
the substrate is wedged (dolt connection refused, read-only mode, query
timeout over 5 seconds), **do not silently give up** and do not keep retrying
the same command in a loop. The channel you would normally use to report is
itself down.

Escalate through the dolt-independent emergency CLI:

```bash
gc emergency send -s critical \
    --ref <bead-id-if-relevant> \
    "<one-line description of what failed and why it matters>"
```

Use `--severity critical` only for substrate failures where an operator must
intervene, such as dolt being wedged, the mail provider being down, or the
controller socket being gone. Use `--severity error` when the condition is
recoverable but still needs to be flagged. Lower severities are for rare
situational awareness.

After sending the emergency, add the details to your current bead once normal
bead writes recover. Do not block waiting for the substrate to recover; keep
making useful progress or move to the next assignment.

Run `gc emergency send --help` for the full flag list.
{{ end }}
