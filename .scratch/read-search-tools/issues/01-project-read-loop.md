# T01: Read a project file through a sequential model/tool loop

Status: ready-for-agent
Type: task
Blocked by: [S03](../../sessions-context/issues/03-budget-settings-preflight.md)
Spec: [feature specification](../spec.md), user stories 1, 3, 10, 13, 14, 18, 23, 24, 25

## ADK adoption guidance

Follow [ADR-0001](../../../docs/adr/0001-adopt-adk-go-runtime.md). Reuse ADK's runner, model adapter, function tools, and session store. Implement only the application behavior assigned to this ticket; passing prototype checks do not complete production acceptance criteria.

## Outcome

Complete the first real tool turn: the model requests a project file range, receives it, and streams an answer.

## Scope

- Reuse ADK function tools, official provider translation, stable call IDs, and Runner dispatch. Register the actual bounded file reader; do not write another fragmented-argument parser or tool loop.
- Wire a line-range file-read tool into the session controller with project scope checks and a conservative response cap from the start.
- Install the explicit sequential task-runner override and return structured errors through ADK's call/result contracts.
- Apply preflight to every follow-up request, and enforce the default 10-call stop immediately; configurable continuation arrives in T06.

## Acceptance criteria

- [ ] A scripted read call returns requested numbered lines, then the model streams its answer.
- [ ] Only current-project files are accessible; traversal, external symlink targets, and reserved session-store access are blocked.
- [ ] Missing files, malformed arguments, and invalid ranges produce structured results instead of crashing the harness.
- [ ] Model or file text cannot grant external permission.
- [ ] Multiple calls never execute concurrently, and each has the correct result ID.
- [ ] The tool loop cannot bypass input capacity or exceed ten invocations by repeatedly failing.

## TDD and validation

First integration test drives a tool turn through the controller and temporary project. Add local HTTP streaming fixtures for fragmented arguments and ID/result mapping. No real filesystem outside fixtures or external API is used.

Exercise observable session behavior through the controller. Use the official ADK provider adapter and keep provider SDK details out of terminal rendering; do not add implementation-mirroring tests or use paid API calls for automated validation.

## Decisions to resolve in this slice

- Use ADK contracts directly inside the runtime integration; introduce only the small controller/UI events actually needed by the CLI, not parallel copies of every framework type.
- Define line numbering, range endpoints, invalid-range behavior, and safe initial caps; do not ship unbounded reads while waiting for T02.
- Protect stored transcripts through generic file tools even when physically inside the project.

Record routine implementation choices with the change. If a choice changes user-visible agreed behavior, identify it explicitly instead of silently treating it as approved.

## Out of scope

Features assigned to other tickets, general file edits, shell execution, MCP, session forking, and a Rust rewrite. Read the linked spec for the authoritative scope.

## Comments

No implementation started.

### ADK provider integration evidence

On branch `prototype/adk-evaluation`, the [provider tool-stream probe](../../../experiments/adk-prototype/provider_tool_test.go) verifies fragmented Responses arguments through the official ADK adapter, one bounded real file read, preserved call/result IDs in the second HTTP request, and streamed answer capture. Malformed argument JSON and an interrupted HTTP body both execute no tool and produce no follow-up model call. This uses local HTTP fixtures, not live OpenAI; T01 remains unimplemented for the real harness.
