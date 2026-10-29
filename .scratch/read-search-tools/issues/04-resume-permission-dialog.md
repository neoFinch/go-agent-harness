# T04: Reconfirm external access whenever a session resumes

Status: ready-for-agent
Type: task
Blocked by: [T03](03-external-path-permissions.md), [S02](../../sessions-context/issues/02-new-list-resume.md)
Spec: [feature specification](../spec.md), user stories 15, 16, 17

## Outcome

Resuming saved work must not silently restore machine-file access.

## Scope

- List previously authorized external paths during resume.
- Ask for once/session/deny access; keep past grants inactive until the choice is made.
- Resume the conversation even when external access is denied, with later tool requests constrained accordingly.

## Acceptance criteria

- [ ] Every resume with prior external scopes presents a confirmation and carries no unconfirmed grant.
- [ ] Once covers only one invocation; session covers subsequent authorized calls until the next resume.
- [ ] Deny leaves the conversation available but the relevant external scope inaccessible.
- [ ] Missing or moved external paths fail clearly rather than broadening access to a parent.
- [ ] Resuming restores settings/history independently of the permission choice.

## TDD and validation

Create, authorize, stop, and resume using distinct controller instances with persisted session metadata. Assert choices and access on the next reads and searches.

Exercise observable session behavior through the controller. Keep provider SDK details behind the adapter; do not add implementation-mirroring tests or use paid API calls for automated validation.

## Decisions to resolve in this slice

- Choose how a resume-time once grant is assigned when several saved scopes or future calls exist; it must not silently become a session grant.

Record routine implementation choices with the change. If a choice changes user-visible agreed behavior, identify it explicitly instead of silently treating it as approved.

## Out of scope

Features assigned to other tickets, general file edits, shell execution, MCP, session forking, and a Rust rewrite. Read the linked spec for the authoritative scope.

## Comments

No implementation started.
