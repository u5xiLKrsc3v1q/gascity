# Active Workstream Coordination

Last updated: 2026-05-18 12:10 PT by Mabel

This is a temporary cross-agent coordination channel, not product documentation.
Do not merge this file into public docs unless we explicitly promote it.

Use this file for concise handoffs between active agents. Prefer factual state,
links, branch names, and explicit interface constraints over narrative.

Severity labels:

- `red`: blocks another workstream.
- `yellow`: coordinate before touching the affected area.
- `green`: informational.

## Communication Mechanism

Chosen mechanism: repo-backed coordination branch.

- Repository: `gastownhall/gascity`
- Branch: `codex/workstream-coordination`
- File: `engdocs/coordination/active-workstreams.md`

Agents should fetch this branch when they need the latest shared coordination
state. Agents may propose updates on their own branches or directly update this
coordination branch when asked, but this branch is not a product PR.

## Workstream Handoff

### Workstream

JSON

### Current Branch / PR

Branch: `codex/json-rollup` planned by Jasmine

PR: not opened yet

Base: `origin/main`

Owner: Jasmine

### Latest State

Jasmine is taking over JSON rollout integration. The previous many-small-PR
strategy is being replaced by a single JSON rollup / review-train PR.

Current JSON source of truth is Jasmine's planned rollup branch, not any single
existing PR. Until that branch exists, use these PRs as provenance:

- #2317: schema-platform plumbing plus native management action JSON.
- #2222: session detail JSON plus oddball/root command JSON.
- Jasmine's clean downstream JSON command PRs selected for the rollup.

### Interface Contracts Other Agents Must Honor

- Human-readable output remains default.
- `--json` emits deterministic machine-readable output.
- stdout must be JSON-only when `--json` is used.
- Human diagnostics and warnings go to stderr unless intentionally represented
  in JSON.
- `--json-schema` exposes command schema metadata.
- Result schemas live under `schemas/<command-path>/result.schema.json`.
- Shared failure schema lives at `schemas/failure.schema.json`.
- Do not introduce `--format json`.

Open clarification for Jasmine:

- Confirm the current source of truth for `--json-schema=result`.
- Confirm whether structured failure JSON is required for every new command now
  or is staged command-by-command.
- Confirm schema extension conventions, including whether `x-gc-jsonl` remains
  accepted for JSONL outputs.
- Tell Cleo when the JSON rollup branch is stable enough to rebase against.

### Blockers / Cross-Workstream Risks

- `yellow`: Registry/gc pack command schemas should not lock until Jasmine
  confirms the final schema conventions for the rollup.
- `yellow`: Pack-defined commands may eventually need schema discovery rules;
  flag pack-facing schema changes to Jasmine rather than patching JSON rollout
  branches directly.

### Needed From Other Agents

- Jasmine: post the rollup branch once created and list included/excluded PRs.
- Cleo: flag any registry/gc pack command schema needs before freezing command
  output shapes.

### Last Updated

2026-05-18 12:10 PT by Mabel

## Workstream Handoff

### Workstream

Pack Deprecation

### Current Branch / PR

Branch: `codex/packv2-wave2-goodbye-packv1`

PR: #2126, <https://github.com/gastownhall/gascity/pull/2126>

Base: `main`

Owner: Mabel / relevant implementation agents

### Latest State

#2126 is the source of truth for PackV1/PackV2 deprecation enforcement. It is
green and mergeable as of this update. It should remain conceptually separate
from registry/gc pack implementation.

Related docs/source reconciliation:

- #2318, <https://github.com/gastownhall/gascity/pull/2318>

### Interface Contracts Other Agents Must Honor

- Do not remove or change `gc import migrate` semantics until doctor /
  `doctor --fix` parity exists for the migrate corpus.
- No new `gc pack` replacement command for `gc import migrate`.
- Remediation messaging must remain actionable for hard-failed legacy
  constructs.
- Coordinate before changing legacy `gc pack fetch` or `gc pack list`
  compatibility.

### Blockers / Cross-Workstream Risks

- `red`: Removing `gc import migrate` before doctor parity would break the
  migration contract.
- `yellow`: Registry/gc pack work may touch compatibility messaging around
  `gc import` and legacy `gc pack` commands; coordinate before changing those
  behaviors.
- `green`: Pack deprecation can proceed independently from registry/gc pack as
  long as compatibility invariants are preserved.

### Needed From Other Agents

- Cleo: keep deprecation/remediation changes out of the registry workstream
  unless a compatibility invariant directly affects canonical `gc pack`
  behavior.
- Jasmine: flag if JSON diagnostics or stderr behavior affects deprecation
  warning/error tests.

### Last Updated

2026-05-18 12:10 PT by Mabel

## Workstream Handoff

### Workstream

Registry-gc-pack

### Current Branch / PR

Branch: `codex/pack-registry-workstream` planned by Cleo

PR: not opened yet

Base: latest `origin/main`

Owner: Cleo

Current local implementation worktree noted by Cleo:

- Worktree: `/Users/dbox/repos/gc-pr2119`
- Current branch: `codex/pack-registry-latest-main`
- State: dirty/unpushed; should be treated as Cleo-owned implementation state.

Older local branches are not current:

- `codex/pack-registry-1a-core`
- `codex/pack-registry-mainline`
- `codex/pack-registry-latest-main`

### Latest State

Cleo will maintain one long-lived registry/gc pack workstream branch for
several days rather than preparing small immediate review PRs. Registry
operations still come first inside that workstream.

The registry/gc pack source of truth is Cleo's planned
`codex/pack-registry-workstream` branch once created.

### Interface Contracts Other Agents Must Honor

- Registry operations land first.
- Dependency mutation must not race ahead of registry config/catalog
  correctness.
- Preserve current PackV2 import fields: `source`, `version`, `export`,
  `transitive`, `shadow`.
- Do not implement #2129 `[[exports]]` in this workstream; treat it as design
  input/future direction.
- Registry handles such as `main:lighthouse` are command-time selectors only.
- Durable `pack.toml` imports must store concrete `source` plus optional
  `version`, not `registry:<registry>:<pack>`.
- Lock/cache internals may preserve registry/ref/commit/hash metadata.
- Preserve `gc import` compatibility and legacy `gc pack fetch/list`
  compatibility.
- `gc import migrate` has no `gc pack` replacement; doctor / `doctor --fix`
  must reach parity before removal.
- Compose with Jasmine's JSON rollup conventions once stable.

### Blockers / Cross-Workstream Risks

- `red`: Do not base registry command JSON/schema tests on an unstable or
  superseded JSON branch without Jasmine confirmation.
- `red`: Do not change `gc import migrate` removal semantics in registry work.
- `yellow`: Coordinate with Pack Deprecation before changing legacy `gc pack`
  `fetch/list` behavior.
- `yellow`: Coordinate with Jasmine before freezing registry command JSON
  schemas or failure behavior.
- `green`: Registry/gc pack overlap with Pack Deprecation is small and should
  be managed through compatibility checkpoints, not branch merging.

### Needed From Other Agents

- Jasmine: confirm JSON rollup branch and schema/failure conventions.
- Mabel: keep Pack Deprecation source-of-truth visible and flag compatibility
  drift.
- Cleo: publish the long-lived branch name once created and summarize changed
  file ownership boundaries.

### Last Updated

2026-05-18 12:10 PT by Mabel

## Workstream Handoff

### Workstream

Pack Reuse / Customization Design

### Current Branch / PR

Branch: managed by Penelope on another machine

PR: feeds into #2119 / #2129 as appropriate

Base: not tracked in this coordination file

Owner: Penelope

### Latest State

Penelope is continuing the user-facing pack reuse/customization guide and
design exploration on a separate machine. Do not migrate or interrupt that
context from this coordination branch.

### Interface Contracts Other Agents Must Honor

- Treat #2129 `[[exports]]` as future design input, not as implemented registry
  behavior.
- Keep user-facing guide language aligned with actual implementation state.

### Blockers / Cross-Workstream Risks

- `yellow`: Reuse/customization guide may update terminology or examples used
  by #2119 and future registry docs.

### Needed From Other Agents

- Penelope: surface guide decisions that change registry/gc pack CLI wording or
  import/export semantics.

### Last Updated

2026-05-18 12:10 PT by Mabel
