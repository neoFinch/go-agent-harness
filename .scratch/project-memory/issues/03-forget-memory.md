# M03: Forget saved project memories without rewriting transcripts

Status: ready-for-agent
Type: task
Blocked by: [M02](02-memory-conflicts.md)
Spec: [feature specification](../spec.md), user stories 12, 13, 22

## ADK adoption guidance

Follow [ADR-0001](../../../docs/adr/0001-adopt-adk-go-runtime.md). Reuse ADK's runner, model adapter, function tools, and session store. Implement only the application behavior assigned to this ticket; passing prototype checks do not complete production acceptance criteria.

## Outcome

Give the user explicit control over removing durable memory.

## Scope

- Implement /memory forget <id> with visible success/failure.
- Remove forgotten records from subsequent automatic memory inclusion and listing.
- Explain that original mentions can remain in active context and transcripts; forgetting is not transcript erasure.

## Acceptance criteria

- [ ] Forgetting an existing ID survives restart and the record no longer appears in automatic memory input.
- [ ] The original conversation transcript remains unchanged.
- [ ] Unknown IDs and failed deletion do not remove unrelated records or claim success.
- [ ] Existing session exclusions referring to removed records do not break resume or accidentally refer to another record.

## TDD and validation

TDD forget/list/restart/request-inclusion behavior using temporary persisted sessions. Verify transcript content and rollback on failed persistence.

Exercise observable session behavior through the controller. Use the official ADK provider adapter and keep provider SDK details out of terminal rendering; do not add implementation-mirroring tests or use paid API calls for automated validation.

## Decisions to resolve in this slice

- Choose handling for unknown/deleted IDs and ensure IDs are not reused in ways that change old exclusion meaning.

Record routine implementation choices with the change. If a choice changes user-visible agreed behavior, identify it explicitly instead of silently treating it as approved.

## Out of scope

Features assigned to other tickets, general file edits, shell execution, MCP, session forking, and a Rust rewrite. Read the linked spec for the authoritative scope.

## Comments

No implementation started.
