# Implementation tickets

One file per ticket. Each ticket links its source spec, covered user stories, prerequisites, acceptance criteria, and TDD validation.

All tickets are initially `ready-for-agent`; a ticket is actionable only after every `Blocked by` prerequisite is resolved. Claim before work, then mark resolved after acceptance checks. This index does not authorize starting implementation.

## IDs and dependencies

S = sessions/context, T = read/search tools, M = project memory. Numeric IDs are local to each feature; linked prefixed IDs disambiguate cross-feature dependencies.

## Accepted runtime

[ADR-0001](../docs/adr/0001-adopt-adk-go-runtime.md) adopts ADK Go. The 18-test experiment is evidence, not production completion. Reuse ADK's runtime, official OpenAI adapter, function tools, session service, and compatible summarization machinery. Keep custom budget, permission, memory, interrupted-output capture, and recovery policies in the harness.

The dependency graph and user-story coverage below remain unchanged. S01 now migrates the actual CLI onto ADK and its SQLite backend; do not rebuild the framework features it supplies.

## Suggested order

Start with S01. S01–S03 establish a persisted, budgeted conversation. Then compaction, read/search, and explicit memory can advance as their prerequisites allow; execution remains incremental and no concurrent agents are implied.

| Ticket | Outcome | Blocked by |
|---|---|---|
| [S01](sessions-context/issues/01-persist-streamed-session.md) | Integrate ADK into the CLI and persist streamed sessions | none |
| [S02](sessions-context/issues/02-new-list-resume.md) | Start, discover, and resume saved sessions | S01 |
| [S03](sessions-context/issues/03-budget-settings-preflight.md) | Enforce session context budgets and output reserves | S02 |
| [S04](sessions-context/issues/04-manual-compaction.md) | Compact active context while retaining the transcript | S03 |
| [S05](sessions-context/issues/05-auto-compaction-retries.md) | Automatically compact near capacity with bounded retries | S04 |
| [S06](sessions-context/issues/06-transactional-budget-reduction.md) | Apply smaller budgets only after successful compaction | S05 |
| [T01](read-search-tools/issues/01-project-read-loop.md) | Read a project file through a sequential model/tool loop | S03 |
| [T02](read-search-tools/issues/02-bounded-read-search.md) | Read oversized lines and search with bounded continuation | T01 |
| [T03](read-search-tools/issues/03-external-path-permissions.md) | Authorize explicit external files and folders | T02 |
| [T04](read-search-tools/issues/04-resume-permission-dialog.md) | Reconfirm external access whenever a session resumes | T03, S02 |
| [T05](read-search-tools/issues/05-current-session-history.md) | Retrieve original messages from the current session | T02, S04 |
| [T06](read-search-tools/issues/06-tool-limit-compaction-recovery.md) | Continue bounded tool turns through limits and compaction | T04, T05, S05 |
| [M01](project-memory/issues/01-explicit-save-list-include.md) | Save explicit project memory and include it across sessions | S03 |
| [M02](project-memory/issues/02-memory-conflicts.md) | Replace clear memory conflicts and clarify ambiguous ones | M01 |
| [M03](project-memory/issues/03-forget-memory.md) | Forget saved project memories without rewriting transcripts | M02 |
| [M04](project-memory/issues/04-session-memory-exclusions.md) | Ask users to exclude memory only after compaction cannot fit | M03, S06 |

## Source specs

- [sessions-context](sessions-context/spec.md)
- [read-search-tools](read-search-tools/spec.md)
- [project-memory](project-memory/spec.md)

## User-story coverage

Story numbers below refer to the user-story list in each feature spec. Shared numbers indicate incremental implementation or cross-cutting verification, not competing ownership.

### sessions-context

- [S01](sessions-context/issues/01-persist-streamed-session.md): 1, 2, 30.
- [S02](sessions-context/issues/02-new-list-resume.md): 3, 4, 5, 6, 7, 8.
- [S03](sessions-context/issues/03-budget-settings-preflight.md): 9, 10, 11, 12, 13, 14, 29.
- [S04](sessions-context/issues/04-manual-compaction.md): 5, 17, 18, 19, 20, 21, 22, 23, 29.
- [S05](sessions-context/issues/05-auto-compaction-retries.md): 14, 15, 16, 24, 25, 26, 29.
- [S06](sessions-context/issues/06-transactional-budget-reduction.md): 27, 28.

### read-search-tools

- [T01](read-search-tools/issues/01-project-read-loop.md): 1, 3, 10, 13, 14, 18, 23, 24, 25.
- [T02](read-search-tools/issues/02-bounded-read-search.md): 2, 3, 4, 5, 6.
- [T03](read-search-tools/issues/03-external-path-permissions.md): 11, 12, 13, 14, 16, 17, 24.
- [T04](read-search-tools/issues/04-resume-permission-dialog.md): 15, 16, 17.
- [T05](read-search-tools/issues/05-current-session-history.md): 7, 8, 9, 26.
- [T06](read-search-tools/issues/06-tool-limit-compaction-recovery.md): 18, 19, 20, 21, 22, 23, 24, 25.

### project-memory

- [M01](project-memory/issues/01-explicit-save-list-include.md): 1, 2, 3, 4, 5, 6, 11, 14.
- [M02](project-memory/issues/02-memory-conflicts.md): 7, 8, 9, 10.
- [M03](project-memory/issues/03-forget-memory.md): 12, 13, 22.
- [M04](project-memory/issues/04-session-memory-exclusions.md): 14, 15, 16, 17, 18, 19, 20, 21, 22.

## Open choices carried forward

- S01–S02: storage, crash/partial-stream recovery, session commands. `/clear` as an alias for a new session remains unconfirmed.
- S03: default total context budget, K units, percentage validation/rounding, counting strategy and unknown model capabilities.
- S04–S06: bounded summarization requests, no-progress recovery, and transactional setting changes.
- T01–T06: tool contracts, result caps, continuation, permissions and interrupted tool-turn behavior.
- M01–M04: explicit-save recognition, conflict classification, persistence, and stale memory references.

Resolve each choice in its owning slice. Do not block all implementation on a new design interview; ask only when the choice changes agreed product behavior.
