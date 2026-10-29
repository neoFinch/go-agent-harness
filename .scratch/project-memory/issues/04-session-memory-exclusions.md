# M04: Ask users to exclude memory only after compaction cannot fit

Status: ready-for-agent
Type: task
Blocked by: [M03](03-forget-memory.md), [S06](../../sessions-context/issues/06-transactional-budget-reduction.md)
Spec: [feature specification](../spec.md), user stories 14, 15, 16, 17, 18, 19, 20, 21, 22

## ADK adoption guidance

Follow [ADR-0001](../../../docs/adr/0001-adopt-adk-go-runtime.md). Reuse ADK's runner, model adapter, function tools, and session store. Implement only the application behavior assigned to this ticket; passing prototype checks do not complete production acceptance criteria.

## Outcome

When memories still prevent fit after compaction, users decide what to exclude without deleting project records.

## Scope

- After successful compaction still leaves insufficient capacity, display eligible memories with stable IDs and estimated token sizes.
- Apply user-selected exclusions only to this session, recheck capacity, and persist selection for resume.
- Preserve original project records and default all currently saved records to eligible in new sessions.
- Integrate the exclusion selection with staged budget reductions without committing a failed candidate.

## Acceptance criteria

- [ ] Ordinary compaction runs before asking for exclusion; successful fit does not ask unnecessarily.
- [ ] Selected IDs stop automatic inclusion for the session while remaining visible as saved project records.
- [ ] Resume restores exclusions; a distinct new session does not inherit them.
- [ ] Decline or insufficient selection sends no oversized ordinary request and performs no silent extra eviction.
- [ ] For a pending budget reduction, new settings commit only after the complete candidate, including selected exclusions, fits; otherwise old state remains.
- [ ] Compaction does not rewrite memories, and the exclusion interaction itself cannot become an unsolicited memory save.
- [ ] UI distinguishes session exclusion from permanent forgetting and does not promise transcript-level erasure.

## TDD and validation

TDD deterministic pressure fixtures across compaction, exclusion, resume, new sessions, and budget reduction rollback. Observe model input and persisted user-facing state.

Exercise observable session behavior through the controller. Use the official ADK provider adapter and keep provider SDK details out of terminal rendering; do not add implementation-mirroring tests or use paid API calls for automated validation.

## Decisions to resolve in this slice

- Choose selection command/CLI interaction and behavior for stale or concurrently changed memory references.
- If no selection can make required input fit, preserve state and explain remaining options rather than repeatedly prompting forever.

Record routine implementation choices with the change. If a choice changes user-visible agreed behavior, identify it explicitly instead of silently treating it as approved.

## Out of scope

Features assigned to other tickets, general file edits, shell execution, MCP, session forking, and a Rust rewrite. Read the linked spec for the authoritative scope.

## Comments

No implementation started.
