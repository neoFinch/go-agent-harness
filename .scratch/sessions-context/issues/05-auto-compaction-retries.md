# S05: Automatically compact near capacity with bounded retries

Status: ready-for-agent
Type: task
Blocked by: [S04](04-manual-compaction.md)
Spec: [feature specification](../spec.md), user stories 14, 15, 16, 24, 25, 26, 29

## Outcome

Make long-running conversations continue automatically while keeping failure recovery explicit.

## Scope

- Use the common preflight to trigger compaction at input usage >= 90% of input capacity, including pending input.
- Display progress and retry failed automatic compaction for three total attempts.
- After exhaustion preserve the pending prompt and session and offer a user-controlled new-session action.
- Prevent immediate compaction loops when a valid soft-target exception cannot reduce below the trigger.

## Acceptance criteria

- [ ] Below-threshold input proceeds; the threshold itself triggers before an ordinary request.
- [ ] Success continues exactly once with the pending prompt and the new context.
- [ ] Success on attempts two or three works; three failures cause no pending ordinary request and no automatic session replacement.
- [ ] Candidate failures do not overwrite active context or transcript.
- [ ] Post-compaction fit is rechecked; no-progress behavior is bounded and visible.
- [ ] New-session choice is explicit and the old session remains saved.

## TDD and validation

TDD deterministic threshold, retry-count, no-progress, rollback, and exact-once continuation tests at the controller. Include input that jumps directly past the hard limit.

Exercise observable session behavior through the controller. Keep provider SDK details behind the adapter; do not add implementation-mirroring tests or use paid API calls for automated validation.

## Decisions to resolve in this slice

- Specify no-progress and retryable-versus-nonretryable failure handling with a maximum of three attempts; never retry forever.
- The pending prompt is preserved; do not silently transfer it into a new session without making that action visible.

Record routine implementation choices with the change. If a choice changes user-visible agreed behavior, identify it explicitly instead of silently treating it as approved.

## Out of scope

Features assigned to other tickets, general file edits, shell execution, MCP, session forking, and a Rust rewrite. Read the linked spec for the authoritative scope.

## Comments

No implementation started.
