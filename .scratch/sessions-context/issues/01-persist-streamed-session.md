# S01: Integrate ADK into the CLI and persist streamed sessions

Status: ready-for-agent
Type: task
Blocked by: none
Spec: [feature specification](../spec.md), user stories 1, 2, 30

## ADK adoption guidance

Follow [ADR-0001](../../../docs/adr/0001-adopt-adk-go-runtime.md). Reuse ADK's runner, model adapter, function tools, and session store. Implement only the application behavior assigned to this ticket; passing prototype checks do not complete production acceptance criteria.

## Outcome

Migrate the real CLI from direct OpenAI orchestration to ADK Runner with the official OpenAI adapter, preserving streaming and persisting sessions. Promote only the validated interrupted-output capture behavior from the experiment, not the throwaway module wholesale.

## Scope

- Move turn orchestration out of terminal rendering into a thin controller around ADK. Reuse its model interface/official OpenAI adapter; retire the old text-only interface and direct provider wrapper when unused.
- Pin the evaluated ADK release in the root module and upgrade the Go directive/toolchain as required. Keep the nested experiment separate.
- Use ADK's built-in SQLite session service initially and define coordination with supplemental interrupted-output capture rather than implementing another full session database.
- Create durable session and message identities, and persist user input, completion state, and streamed replies without pretending partial replies completed.
- Keep storage and model dependencies replaceable for tests; use real temporary storage and a scripted model.
- Preserve the current behavior that an unsuccessful reply does not become a successful active-history exchange.

## Acceptance criteria

- [ ] The actual CLI executes through ADK and the official OpenAI adapter, streams before completion, and produces durable history with stable references.
- [ ] The root module has pinned ADK dependencies, unused direct provider orchestration is removed, and there is no second bespoke model/tool loop.
- [ ] Completed and interrupted/failed turns are distinguishable; a persistence error is visible and never reported as a successful save.
- [ ] The original input remains available after a failed model request; blank input and exit continue to work.
- [ ] No new tool execution, compaction, or project-memory behavior is required in this slice.

## TDD and validation

First write a failing integration test for a streamed exchange and its saved records. Cover partial-stream failure and storage failure. Add a small CLI smoke test for the controller wiring; do not require network calls.

Exercise observable session behavior through the controller. Use the official ADK provider adapter and keep provider SDK details out of terminal rendering; do not add implementation-mirroring tests or use paid API calls for automated validation.

## Decisions to resolve in this slice

- Choose project/session identity and local SQLite location. Document supplemental capture serialization and recovery rules for partial writes or divergent stores before relying on them.
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
