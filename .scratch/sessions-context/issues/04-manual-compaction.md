# S04: Compact active context while retaining the transcript

Status: ready-for-agent
Type: task
Blocked by: [S03](03-budget-settings-preflight.md)
Spec: [feature specification](../spec.md), user stories 5, 17, 18, 19, 20, 21, 22, 23, 29

## ADK adoption guidance

Follow [ADR-0001](../../../docs/adr/0001-adopt-adk-go-runtime.md). Reuse ADK's runner, model adapter, function tools, and session store. Implement only the application behavior assigned to this ticket; passing prototype checks do not complete production acceptance criteria.

## Outcome

Provide manual /compact that shortens active context without deleting original conversation.

## Scope

- Reuse ADK's summarizer and history-preserving compaction records where compatible. Supply our summary requirements and token-based selection/checkpoint policy rather than reimplementing provider calls or accepting event-count retention as equivalent.
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

Exercise observable session behavior through the controller. Use the official ADK provider adapter and keep provider SDK details out of terminal rendering; do not add implementation-mirroring tests or use paid API calls for automated validation.

## Decisions to resolve in this slice

- Choose summary output allowance, bounded summarization strategy, and handling when no progress is possible.
- Clarify manual-compaction retry/cancellation behavior without claiming the automatic three-attempt rule was agreed for every manual operation.

Record routine implementation choices with the change. If a choice changes user-visible agreed behavior, identify it explicitly instead of silently treating it as approved.

## Out of scope

Features assigned to other tickets, general file edits, shell execution, MCP, session forking, and a Rust rewrite. Read the linked spec for the authoritative scope.

## Comments

No implementation started.
