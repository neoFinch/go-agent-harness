# T02: Read oversized lines and search with bounded continuation

Status: ready-for-agent
Type: task
Blocked by: [T01](01-project-read-loop.md)
Spec: [feature specification](../spec.md), user stories 2, 3, 4, 5, 6

## ADK adoption guidance

Follow [ADR-0001](../../../docs/adr/0001-adopt-adk-go-runtime.md). Reuse ADK's runner, model adapter, function tools, and session store. Implement only the application behavior assigned to this ticket; passing prototype checks do not complete production acceptance criteria.

## Outcome

Make file inspection useful on large files without allowing result size to exhaust context.

## Scope

- Add a byte cap alongside line-range reads, supporting partial content within one oversized line.
- Return explicit continuation state and more-content indicators.
- Add bounded project file/content search with matching source locations and continuation.
- Keep all content within the same permission and input-accounting flow used by reads.

## Acceptance criteria

- [ ] Requested ranges show accurate line numbers and a clear final/partial state.
- [ ] Repeated continuation reconstructs oversized single-line and multibyte content without gaps or accidental duplicates.
- [ ] Large searches return bounded matches, locations, and an explicit way to obtain more.
- [ ] Invalid continuation state, binary content, and read/search errors have bounded structured responses.
- [ ] Search respects authorized scope and reserved internal-store exclusions.
- [ ] Byte caps do not replace token preflight before the follow-up model call.

## TDD and validation

TDD integration fixtures include long single lines, Unicode, empty files, end-of-file, many matches, and invalid ranges/cursors. Validate reconstructed output rather than implementation cursor fields.

Exercise observable session behavior through the controller. Use the official ADK provider adapter and keep provider SDK details out of terminal rendering; do not add implementation-mirroring tests or use paid API calls for automated validation.

## Decisions to resolve in this slice

- Choose byte and match caps, continuation representation, deterministic search ordering, supported search syntax, excluded directories, and binary behavior.
- Define what a continuation does if the source file changes between chunks; never silently combine incompatible snapshots.

Record routine implementation choices with the change. If a choice changes user-visible agreed behavior, identify it explicitly instead of silently treating it as approved.

## Out of scope

Features assigned to other tickets, general file edits, shell execution, MCP, session forking, and a Rust rewrite. Read the linked spec for the authoritative scope.

## Comments

No implementation started.
