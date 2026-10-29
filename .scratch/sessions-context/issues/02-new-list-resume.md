# S02: Start, discover, and resume saved sessions

Status: resolved
Type: task
Blocked by: [S01](01-persist-streamed-session.md)
Spec: [feature specification](../spec.md), user stories 3, 4, 5, 6, 7, 8

## ADK adoption guidance

Follow [ADR-0001](../../../docs/adr/0001-adopt-adk-go-runtime.md). Reuse ADK's runner, model adapter, function tools, and session store. Implement only the application behavior assigned to this ticket; passing prototype checks do not complete production acceptance criteria.

## Outcome

Let a user leave a conversation and continue it later with the same identity and settings.

## Scope

- Expose new-session, listing, and resume operations through the controller and CLI using ADK session-service Create/List/Get operations; do not implement duplicate session CRUD.
- Restore settings and active context and append new messages to the same transcript.
- Persist the active-context checkpoint independently of original records so later compaction can update it.
- New sessions inherit project defaults; session changes do not rewrite defaults or sibling sessions.

## Acceptance criteria

- [x] A fresh process resumes an existing conversation and its next model request includes the restored active history.
- [x] A resumed reply appends to the original session rather than creating a duplicate.
- [x] Settings survive resume; a new session inherits defaults independently.
- [x] Unknown sessions, invalid checkpoints, and inaccessible storage produce actionable errors without overwriting valid data.
- [x] Resume does not generate an unnecessary summary; budget revalidation is added by S03.
- [x] Starting a distinct new session preserves the previous transcript.

## TDD and validation

Use temporary persisted sessions across two controller instances and inspect subsequent scripted-model requests. Test invalid IDs and corrupt/incomplete state without relying on private serialization layout.

Exercise observable session behavior through the controller. Use the official ADK provider adapter and keep provider SDK details out of terminal rendering; do not add implementation-mirroring tests or use paid API calls for automated validation.

## Decisions to resolve in this slice

- Choose exact new/list/resume command syntax and record it in usage help.
- Do not resolve the outstanding /clear semantics by assumption; implement a distinct new-session operation unless the user confirms that alias.

Record routine implementation choices with the change. If a choice changes user-visible agreed behavior, identify it explicitly instead of silently treating it as approved.

## Out of scope

Features assigned to other tickets, general file edits, shell execution, MCP, session forking, and a Rust rewrite. Read the linked spec for the authoritative scope.

## Comments

Implemented in the ADK evaluation worktree; see the answer below.


## Answer

Added controller `NewSession`, `Sessions`, and `Resume`, backed by ADK Create/List/Get. CLI commands are `/new`, `/sessions`, and `/resume <id>`; startup supports `-resume <id>`. `/help` and `-h` document them. A failed resume leaves the active session unchanged and does not create another session or call the model. `/clear` retains its existing same-session meaning.

Versioned session-scoped ADK state stores the active clear-boundary checkpoint and model setting separately from original events/capture records. Clear saves its marker and checkpoint atomically. Resume validates checkpoint version and references, restores the saved model and successful active history, and appends to the original transcript without summarization. Incomplete/failed captured turns remain inspectable and excluded from successful context.

The currently implemented setting is the model name. New sessions use startup defaults (`-model`, `OPENAI_MODEL`, or the built-in default) independently of resumed session settings; a project settings file is not introduced here. Budget/output settings and their revalidation belong to S03. S01 sessions without a state envelope retain their original clear boundary and adopt the current model on first resume. Credentials/endpoints remain environment configuration rather than persisted secrets. Concurrent writers to one session are unsupported.

Validation: root normal and race test suites pass. Integration tests cover reopened storage, stable identity and append behavior, clear restoration, model defaults, invalid IDs/checkpoints/storage, failed-turn exclusion, and legacy migration. CLI tests cover commands across runs and the official OpenAI adapter through a localhost HTTP stub, including a changed startup default on resume. No paid calls were made. See the root [README](../../../README.md) for usage and recovery limits.
