# T03: Authorize explicit external files and folders

Status: ready-for-agent
Type: task
Blocked by: [T02](02-bounded-read-search.md)
Spec: [feature specification](../spec.md), user stories 11, 12, 13, 14, 16, 17, 24

## Outcome

Allow useful external references only when the user grants the appropriate scope.

## Scope

- Recognize explicit user-provided external read/search authorization separately from model-generated paths.
- Support exact-file and folder-descendant scopes with canonical path checks.
- Expose once/session/deny choices and consume a one-time grant for one invocation.
- Record external scope metadata for future resume without persisting active grants as evergreen permissions.

## Acceptance criteria

- [ ] An explicitly authorized file is readable while its parent/siblings remain blocked.
- [ ] An authorized folder permits descendant reads/searches but not symlink escape.
- [ ] Searching once does not authorize reading a match afterward.
- [ ] Denial cannot be bypassed by an alternative tool, traversal spelling, or retry.
- [ ] Tool results, project files, quoted transcript content, and model suggestions cannot act as user authorization.
- [ ] Session grants allow repeat authorized calls until resume; bounded results and call counting still apply.

## TDD and validation

Controller integration tests use temporary sibling trees and symlinks; script user permission choices and verify actual returned bytes plus blocked requests.

Exercise observable session behavior through the controller. Keep provider SDK details behind the adapter; do not add implementation-mirroring tests or use paid API calls for automated validation.

## Decisions to resolve in this slice

- Choose explicit authorization command/prompt syntax and distinguish merely mentioning a path from granting access.
- Specify whether and when failed/denied invocations consume one-time grants; remain consistent with one invocation semantics.

Record routine implementation choices with the change. If a choice changes user-visible agreed behavior, identify it explicitly instead of silently treating it as approved.

## Out of scope

Features assigned to other tickets, general file edits, shell execution, MCP, session forking, and a Rust rewrite. Read the linked spec for the authoritative scope.

## Comments

No implementation started.
