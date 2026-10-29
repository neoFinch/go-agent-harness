# T06: Continue bounded tool turns through limits and compaction

Status: ready-for-agent
Type: task
Blocked by: [T04](04-resume-permission-dialog.md), [T05](05-current-session-history.md), [S05](../../sessions-context/issues/05-auto-compaction-retries.md)
Spec: [feature specification](../spec.md), user stories 18, 19, 20, 21, 22, 23, 24, 25

## Outcome

Keep long tool turns predictable when limits or context pressure interrupt execution.

## Scope

- Expose the per-turn tool limit in session settings with a default of 10.
- Preserve queued invocations, errors, completed results, and pending prompt on limit exhaustion; require explicit continuation.
- Integrate mid-turn automatic compaction while preserving model-valid call/result groups and unexecuted calls.
- Keep all invocations sequential and count failed invocations, including multiple requests from one model response.

## Acceptance criteria

- [ ] The tenth invocation may execute; the eleventh cannot execute without an explicit continuation choice.
- [ ] A changed limit survives resume and applies consistently.
- [ ] Failed calls count; repeated permission errors cannot evade the limit.
- [ ] Continuation does not duplicate completed calls or fabricate results for queued calls.
- [ ] Context is checked before each model call and compaction never orphans a result or splits an indivisible call group.
- [ ] Three compaction failures preserve tool progress and block further ordinary requests until the user chooses recovery.

## TDD and validation

TDD scripted multi-call turns crossing both limits and compaction thresholds, including denial, failure, resume state, and explicit continuation. Verify order and exact-once execution through observable results.

Exercise observable session behavior through the controller. Keep provider SDK details behind the adapter; do not add implementation-mirroring tests or use paid API calls for automated validation.

## Decisions to resolve in this slice

- Define the continuation allowance/reset rule and make it explicit to the user; never silently reset.
- Define resume behavior for interrupted tool batches without claiming all external grants survive resume.

Record routine implementation choices with the change. If a choice changes user-visible agreed behavior, identify it explicitly instead of silently treating it as approved.

## Out of scope

Features assigned to other tickets, general file edits, shell execution, MCP, session forking, and a Rust rewrite. Read the linked spec for the authoritative scope.

## Comments

No implementation started.
