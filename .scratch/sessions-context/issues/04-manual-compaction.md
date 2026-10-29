# S04: Compact active context while retaining the transcript

Status: ready-for-agent
Type: task
Blocked by: [S03](03-budget-settings-preflight.md)
Spec: [feature specification](../spec.md), user stories 5, 17, 18, 19, 20, 21, 22, 23, 29

## Outcome

Provide manual /compact that shortens active context without deleting original conversation.

## Scope

- Generate a summary preserving goals, constraints, rationale, rejected approaches, unfinished work, uncertainty, and source references.
- Target at most 60% of input capacity and retain newest complete exchanges up to 40%, subject to summary and required-input space.
- Persist a candidate checkpoint only after successful summarization and validation; preserve the transcript.
- Treat complete model-valid call/result groups as indivisible when selecting retained context; exercise generated tool turns after T06 integrates the runtime loop.
- Fit compaction requests within model limits using an explicitly bounded strategy, without recursively invoking ordinary compaction.

## Acceptance criteria

- [ ] /compact produces a visible status and resumes normal streaming with the compacted context.
- [ ] Original message references and content remain retrievable from storage.
- [ ] Recent retention uses tokens and complete exchanges, not a fixed message count.
- [ ] Required instructions/current prompt are not silently truncated; a 60% soft-target exception never permits the hard limit to be exceeded.
- [ ] Failure leaves previous active context usable, including on resume.
- [ ] Oversized summarization input is bounded or rejected clearly rather than creating an oversized request or recursion.

## TDD and validation

Write integration tests for manual compaction, checkpoint reload, retention accounting, and rollback using scripted summaries. Add long coding-conversation fixtures for manual semantic review; fake outputs alone cannot prove summary quality.

Exercise observable session behavior through the controller. Keep provider SDK details behind the adapter; do not add implementation-mirroring tests or use paid API calls for automated validation.

## Decisions to resolve in this slice

- Choose summary output allowance, bounded summarization strategy, and handling when no progress is possible.
- Clarify manual-compaction retry/cancellation behavior without claiming the automatic three-attempt rule was agreed for every manual operation.

Record routine implementation choices with the change. If a choice changes user-visible agreed behavior, identify it explicitly instead of silently treating it as approved.

## Out of scope

Features assigned to other tickets, general file edits, shell execution, MCP, session forking, and a Rust rewrite. Read the linked spec for the authoritative scope.

## Comments

No implementation started.
