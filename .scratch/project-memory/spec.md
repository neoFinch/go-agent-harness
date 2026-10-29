# Explicit project memory

Status: ready-for-agent

## Problem Statement

Session history does not provide deliberate project-wide preferences across conversations. Automatically promoting conversation into permanent memory would surprise users, while storing every preference in active context without controls would eventually consume the budget.

## Solution

Save project memory only on explicit user request, include eligible memories across sessions, expose listing and forgetting, handle conflicting updates visibly, and let users exclude memories from a session if compaction cannot free enough space.

## User Stories

1. As a user, I want an explicit request such as “remember this” or “save this as memory” to save project memory, so that my chosen information survives across sessions.
2. As a user, I want ordinary conversation left unsaved as project memory, so that persistence is intentional.
3. As a user, I want the harness to confirm a successful memory save, so that I know what will be remembered.
4. As a user, I want memories scoped to the project, so that unrelated projects do not inherit them.
5. As a user, I want saved eligible memories included in model requests, so that preferences consistently affect conversations.
6. As a user, I want memory tokens included in input accounting, so that context usage is accurate.
7. As a user, I want clearly conflicting new explicit memories to replace old ones, so that obsolete preferences do not remain active.
8. As a user, I want replacement confirmation to identify the old and new information, so that changes are inspectable.
9. As a user, I want unrelated memories preserved during an update, so that one preference change does not erase others.
10. As a user, I want ambiguous conflicts clarified, so that the harness does not guess which information to overwrite.
11. As a user, I want `/memory` to list memories with stable IDs, so that I can inspect what is saved.
12. As a user, I want `/memory forget <id>` to remove saved project memory, so that I can withdraw previously saved information.
13. As a user, I want forgetting explained separately from transcript deletion, so that I understand what remains on disk.
14. As a user, I want compaction to leave saved memories unchanged, so that summarization cannot rewrite project preferences.
15. As a user, I want normal conversation compacted before I am asked to exclude memories, so that memory removal is not the first response to context pressure.
16. As a user, I want to choose session exclusions when memories still prevent fit after compaction, so that the harness does not evict them silently.
17. As a user, I want IDs and estimated token sizes displayed during exclusion, so that I can make an informed choice.
18. As a user, I want session exclusion to keep memory saved for the project, so that a temporary capacity decision is not permanent deletion.
19. As a user, I want exclusions restored when resuming a session, so that its context configuration is retained.
20. As a user, I want new sessions to make all currently saved project memories eligible again, so that exclusions do not leak across sessions.
21. As a user, I want capacity checked again after selecting exclusions, so that an oversized request is not sent anyway.
22. As a user, I want old transcript mentions left intact when a memory is forgotten or excluded, so that the original conversation is not silently rewritten.

## Implementation Decisions

- Project memory is a dedicated persistence capability coordinated by the session controller, distinct from deferred general-purpose file-editing tools.
- Save only when the user explicitly requests it. Quoted text, file content, retrieved transcripts, and model suggestions do not independently authorize persistent writes.
- Give saved memories stable IDs and a project association. Store session exclusion references separately from project memory records.
- Include non-excluded saved memories in requests by default; account for them within input capacity. Memory content is not an unbounded addition outside the context budget.
- Replace clearly conflicting memories only in response to an explicit save request, with visible confirmation. Preserve unrelated entries. Ambiguous replacements require clarification before mutation.
- `/memory` lists records and `/memory forget <id>` removes saved project memory from future automatic inclusion. Never claim successful persistence if the write failed.
- Neither forgetting nor exclusion deletes prior transcript mentions or guarantees that active conversation text no longer contains the same fact. Preserve that distinction in user-facing output.
- Compaction never rewrites or removes saved project memories. If compacted conversation plus required input and memories still does not fit, present a user-selected exclusion flow with stable IDs and estimated token sizes.
- Exclusions remove automatic memory inclusion for the current session, preserve project records, persist with that session, and restore on resume. New sessions begin with all currently saved project memories eligible.
- Recheck capacity after exclusion. Decline or insufficient exclusion must not cause silent memory eviction, transcript deletion, or an oversized model request.
- Exact memory record format and command handling belong behind a small module interface, exercised through the session controller rather than exposed through SDK-specific types.

## Testing Decisions

- Use TDD with session-controller integration tests and real temporary persistence. A scripted model makes request inclusion and streamed behavior observable without paid calls.
- Test explicit saves, ordinary conversation without saves, and untrusted quoted/tool content that must not authorize a memory write.
- Verify persistence across new sessions, project isolation, stable IDs, listing, forgetting, clear replacements, unrelated preservation, and ambiguous-conflict prompts.
- Inspect subsequent model requests to prove inclusion and exclusions; do not merely inspect an internal memory map.
- Force budget pressure with deterministic counts: compaction occurs first, selection is requested only if necessary afterward, selected records remain on disk, and requests are checked again before sending.
- Resume sessions to verify exclusions persist; start new sessions to verify those exclusions do not carry over.
- Inject persistence failures and verify no false success confirmation or partial conflicting replacement is presented as committed.
- Verify forgetting does not rewrite transcripts and compaction does not mutate memory records. There is no existing memory implementation or test suite to reuse as prior art.

## Out of Scope

- Automatically inferred permanent memories, global personal memory across projects, silent eviction, transcript erasure, and semantic/vector memory search.
- General file editing, shell execution, or MCP-based memory storage.
- Claims of complete model forgetting when the information remains in active conversation or archived messages.

## Further Notes

- Related specs: [sessions/context](../sessions-context/spec.md) and [read/search tools](../read-search-tools/spec.md).
- Open details: project identity and storage location; memory size limits; duplicate detection; how natural-language save requests are recognized; conflict detection; unknown IDs; malformed records; and handling changed or deleted records referenced by old session exclusions.
- A bounded memory-exclusion interaction must not accidentally save the eviction discussion as a new project memory.
- Selection presentation and command grammar beyond the agreed list/forget commands are implementation proposals, not previously approved UI contracts.
