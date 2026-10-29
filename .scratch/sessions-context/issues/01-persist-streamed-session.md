# S01: Persist a streamed conversation through the session controller

Status: ready-for-agent
Type: task
Blocked by: none
Spec: [feature specification](../spec.md), user stories 1, 2, 30

## Outcome

Run the existing prompt-to-stream flow through a session controller and persist a usable transcript as the conversation occurs.

## Scope

- Move turn orchestration out of terminal rendering while retaining the harness-owned model interface.
- Create durable session and message identities, and persist user input, completion state, and streamed replies without pretending partial replies completed.
- Keep storage and model dependencies replaceable for tests; use real temporary storage and a scripted model.
- Preserve the current behavior that an unsuccessful reply does not become a successful active-history exchange.

## Acceptance criteria

- [ ] A CLI conversation streams before generation completes and produces a durable transcript with stable references.
- [ ] Completed and interrupted/failed turns are distinguishable; a persistence error is visible and never reported as a successful save.
- [ ] The original input remains available after a failed model request; blank input and exit continue to work.
- [ ] No new tool execution, compaction, or project-memory behavior is required in this slice.

## TDD and validation

First write a failing integration test for a streamed exchange and its saved records. Cover partial-stream failure and storage failure. Add a small CLI smoke test for the controller wiring; do not require network calls.

Exercise observable session behavior through the controller. Keep provider SDK details behind the adapter; do not add implementation-mirroring tests or use paid API calls for automated validation.

## Decisions to resolve in this slice

- Choose and document project/session identity, storage location, serialization, and recovery rules for partial writes before relying on them.
- Preserve existing /clear behavior for now; changing it to start a durable new session remains an explicit product question.

Record routine implementation choices with the change. If a choice changes user-visible agreed behavior, identify it explicitly instead of silently treating it as approved.

## Out of scope

Features assigned to other tickets, general file edits, shell execution, MCP, session forking, and a Rust rewrite. Read the linked spec for the authoritative scope.

## Comments

No implementation started.

### ADK evaluation evidence

Branch `prototype/adk-evaluation` contains [six offline ADK Go integration probes](../../../experiments/adk-prototype/README.md) against v2.6.0. Streaming, SQLite reload, tool execution with a sequential override, an OpenAI HTTP-stub contract, and compaction-record persistence worked. Default session storage does not retain partial streamed output on failure; S01 still needs explicit partial/failure capture. This is an evaluation, not completion of this ticket or approval to replace the current architecture.

### Follow-up capture experiment

The same prototype branch now contains a harness-owned streamed-turn capture journal. Five capture tests verify partial-disconnect durability, successful reply deduplication, cancellation, and storage failures; all six baseline probes still pass (11 total). Original failed input and visible output survive journal reopen without inserting an unfinished reply into successful ADK history. This is evidence for S01, not ticket completion: real CLI wiring, full transcript/schema decisions, atomic coordination with ADK storage, and crash recovery remain outstanding.
