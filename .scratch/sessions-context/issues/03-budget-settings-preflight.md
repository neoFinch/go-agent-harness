# S03: Enforce session context budgets and output reserves

Status: ready-for-agent
Type: task
Blocked by: [S02](02-new-list-resume.md)
Spec: [feature specification](../spec.md), user stories 9, 10, 11, 12, 13, 14, 29

## ADK adoption guidance

Follow [ADR-0001](../../../docs/adr/0001-adopt-adk-go-runtime.md). Reuse ADK's runner, model adapter, function tools, and session store. Implement only the application behavior assigned to this ticket; passing prototype checks do not complete production acceptance criteria.

## Outcome

Let users see and configure a real request budget before the model is called.

## Scope

- Add settings display and updates for total context budget and output percentage, with a 25% output default.
- Compute effective output reserve using the model output cap and enforce it in provider requests.
- Count the actual assembled ADK request input, including instructions, schemas, and any future tool or memory content, through one before-model preflight path. Replace deterministic prototype counts with the selected real counting strategy.
- Restore settings on resume and validate model capabilities and input capacity before generation.
- At this stage, explicitly block a required budget reduction/compaction rather than sending an oversized request; S06 supplies the full change workflow.

## Acceptance criteria

- [ ] For controlled model capabilities, reserve and input capacity match the documented formula, including model-output capping.
- [ ] Changing the percentage affects both provider output allowance and displayed input capacity.
- [ ] The CLI shows input usage, output allowance, and compaction availability clearly; do not suggest auto-compaction is active until S05.
- [ ] No ordinary request is sent above the selected capacity or model window; current prompts are never silently truncated.
- [ ] Invalid or incompatible values preserve the previous settings; resumed settings are revalidated.
- [ ] Provider completion usage is available without breaking streamed text.

## TDD and validation

TDD through the controller with deterministic counts at exact capacity edges; use a local HTTP stub to verify provider output-limit encoding, usage extraction, and streaming failure handling.

Exercise observable session behavior through the controller. Use the official ADK provider adapter and keep provider SDK details out of terminal rendering; do not add implementation-mirroring tests or use paid API calls for automated validation.

## Decisions to resolve in this slice

- Select the initial total budget, K unit, valid percentage range, and rounding rules; include examples in help.
- Choose a token-counting method with a stated estimation margin and treatment of all serialized input; distinguish estimates from provider-reported usage.
- Define conservative behavior for unavailable model capability metadata; never silently infer a larger window.

Record routine implementation choices with the change. If a choice changes user-visible agreed behavior, identify it explicitly instead of silently treating it as approved.

## Out of scope

Features assigned to other tickets, general file edits, shell execution, MCP, session forking, and a Rust rewrite. Read the linked spec for the authoritative scope.

## Comments

No implementation started.

### ADK budget-hook evidence

Branch `prototype/adk-evaluation` has [custom budget integration probes](../../../experiments/adk-prototype/README.md) verifying reserve capping/overrides, exact 90% triggering, candidate recounting, three-attempt blocking, and before-model checks after tool results. These use deterministic token counts and an injected compactor. Real tokenization, general retention selection, soft-target relaxation, persisted compacted context, and user-facing recovery remain outstanding; this ticket is not completed by the experiment.
