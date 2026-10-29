# M01: Save explicit project memory and include it across sessions

Status: ready-for-agent
Type: task
Blocked by: [S03](../../sessions-context/issues/03-budget-settings-preflight.md)
Spec: [feature specification](../spec.md), user stories 1, 2, 3, 4, 5, 6, 11, 14

## ADK adoption guidance

Follow [ADR-0001](../../../docs/adr/0001-adopt-adk-go-runtime.md). Reuse ADK's runner, model adapter, function tools, and session store. Implement only the application behavior assigned to this ticket; passing prototype checks do not complete production acceptance criteria.

## Outcome

Deliver one complete memory flow: explicit save, inspect, start another session, and use the saved preference.

## Scope

- Add dedicated project-memory persistence behind session orchestration, separate from general file editing.
- Handle explicit user save requests and /memory listing with stable IDs.
- Include eligible project memories in every model request and input accounting.
- Keep saves intentional and project-scoped; existing records must not be silently overwritten in this first slice.

## Acceptance criteria

- [ ] An explicit remember/save request creates a durable memory and visible confirmation only after persistence succeeds.
- [ ] A new session for that project includes saved memory; a different project does not.
- [ ] Ordinary dialogue, quoted text, file content, and model/tool output do not independently authorize writes.
- [ ] /memory shows stable IDs and saved content.
- [ ] Memory tokens affect preflight; no silent eviction or oversized request occurs.
- [ ] Compaction input construction treats saved memory separately and does not mutate its durable record.

## TDD and validation

TDD a save/list/new-session request flow with temporary storage and scripted model requests. Test persistence failure, project isolation, and untrusted text attempting to request a save.

Exercise observable session behavior through the controller. Use the official ADK provider adapter and keep provider SDK details out of terminal rendering; do not add implementation-mirroring tests or use paid API calls for automated validation.

## Decisions to resolve in this slice

- Choose project identity, record format, storage location, and a bounded memory size policy.
- Specify recognition of natural-language explicit saves; if clarification is needed, ask rather than guessing the content to persist.
- Document a safe temporary behavior for duplicates/conflicts until M02.

Record routine implementation choices with the change. If a choice changes user-visible agreed behavior, identify it explicitly instead of silently treating it as approved.

## Out of scope

Features assigned to other tickets, general file edits, shell execution, MCP, session forking, and a Rust rewrite. Read the linked spec for the authoritative scope.

## Comments

No implementation started.
