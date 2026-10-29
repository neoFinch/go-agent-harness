# S06: Apply smaller budgets only after successful compaction

Status: ready-for-agent
Type: task
Blocked by: [S05](05-auto-compaction-retries.md)
Spec: [feature specification](../spec.md), user stories 27, 28

## Outcome

Changing a budget must not commit settings that the current conversation cannot satisfy.

## Scope

- Evaluate current required input against the proposed budget and its recalculated output reserve.
- If compaction is needed, explain and ask for confirmation before applying the setting.
- Stage compacted context and settings together and commit only after fit succeeds.
- Make this state transition reusable by the later project-memory exclusion workflow.

## Acceptance criteria

- [ ] A fitting budget change applies normally.
- [ ] An overflowing reduction shows the proposed input capacity and explains required compaction.
- [ ] Decline, summarization failure, persistent overflow, or save failure keeps old settings and active history.
- [ ] Successful compaction and fit apply the new settings and checkpoint together and survive resume.
- [ ] Do not mutate old context through slice aliasing or partially persist a failed candidate.

## TDD and validation

Integration tests cover accept/decline, failure injection, unchanged old state after restart, and successful new state after restart. Inspect observable requests and saved settings.

Exercise observable session behavior through the controller. Keep provider SDK details behind the adapter; do not add implementation-mirroring tests or use paid API calls for automated validation.

## Decisions to resolve in this slice

- Define retry and storage-commit mechanics for this operation; retain the user-approved atomic success/failure semantics.
- Coordinate M04 to allow user-selected memory exclusions before a staged reduction is finalized.

Record routine implementation choices with the change. If a choice changes user-visible agreed behavior, identify it explicitly instead of silently treating it as approved.

## Out of scope

Features assigned to other tickets, general file edits, shell execution, MCP, session forking, and a Rust rewrite. Read the linked spec for the authoritative scope.

## Comments

No implementation started.
