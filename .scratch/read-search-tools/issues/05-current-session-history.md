# T05: Retrieve original messages from the current session

Status: ready-for-agent
Type: task
Blocked by: [T02](02-bounded-read-search.md), [S04](../../sessions-context/issues/04-manual-compaction.md)
Spec: [feature specification](../spec.md), user stories 7, 8, 9, 26

## Outcome

After compaction the model can recover exact earlier detail without loading the full transcript.

## Scope

- Provide dedicated bounded transcript search and message-range reads.
- Use stable transcript references and current-session scope, including after resume.
- Reuse bounded result and preflight conventions without exposing unrestricted archive paths.

## Acceptance criteria

- [ ] A compacted original message can be found and read exactly through a tool request.
- [ ] Results identify source message IDs and indicate continuation for partial results.
- [ ] Attempts to select another session or read its raw archive through project search/read fail.
- [ ] A resumed session retrieves from its own transcript and appends new records to that identity.
- [ ] Retrieval results are input-budgeted and never automatically load the entire transcript.

## TDD and validation

Integration test creates two sessions with distinct facts, compacts one, and scripts retrieval before and after resume. Prove no cross-session content enters model input.

Exercise observable session behavior through the controller. Keep provider SDK details behind the adapter; do not add implementation-mirroring tests or use paid API calls for automated validation.

## Decisions to resolve in this slice

- Choose transcript search/read schemas and cap behavior; use transcript references established by S01 rather than line-number assumptions about storage files.

Record routine implementation choices with the change. If a choice changes user-visible agreed behavior, identify it explicitly instead of silently treating it as approved.

## Out of scope

Features assigned to other tickets, general file edits, shell execution, MCP, session forking, and a Rust rewrite. Read the linked spec for the authoritative scope.

## Comments

No implementation started.
