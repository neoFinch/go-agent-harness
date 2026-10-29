# M02: Replace clear memory conflicts and clarify ambiguous ones

Status: ready-for-agent
Type: task
Blocked by: [M01](01-explicit-save-list-include.md)
Spec: [feature specification](../spec.md), user stories 7, 8, 9, 10

## Outcome

An explicit updated preference should not leave contradictory saved memories active.

## Scope

- Detect duplicate/clearly conflicting candidates during explicit saves.
- Replace a clear conflict visibly while preserving unrelated entries.
- Request clarification before mutating an ambiguous conflict.
- Make conflict updates durable as one observable successful operation.

## Acceptance criteria

- [ ] Saving use spaces after an explicit use tabs memory replaces the relevant record and reports old and new values.
- [ ] Unrelated memory remains unchanged and appears in later requests.
- [ ] Ambiguous candidates produce a clarification without changing stored records.
- [ ] A failed write does not announce a successful replacement or lose the old memory.
- [ ] Untrusted/model-originated suggestions cannot initiate replacement without explicit user authorization.

## TDD and validation

Integration tests inspect listing and subsequent requests, including ambiguous classification and failure injection. Avoid assertions tied to private conflict-detection algorithms.

Exercise observable session behavior through the controller. Keep provider SDK details behind the adapter; do not add implementation-mirroring tests or use paid API calls for automated validation.

## Decisions to resolve in this slice

- Choose and test conservative conflict/duplicate detection; document when clarification is mandatory.
- Define stable-ID behavior on replacement and how session references to changed records are resolved.

Record routine implementation choices with the change. If a choice changes user-visible agreed behavior, identify it explicitly instead of silently treating it as approved.

## Out of scope

Features assigned to other tickets, general file edits, shell execution, MCP, session forking, and a Rust rewrite. Read the linked spec for the authoritative scope.

## Comments

No implementation started.
