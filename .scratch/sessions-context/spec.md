# Sessions and context management

Status: ready-for-agent

## Problem Statement

The harness streams replies and retains conversation history only while the process runs. Long conversations grow without a budget, quitting loses the session, and clearing discards the active history. Users need continuity without sending an ever-growing transcript or silently losing important decisions.

## Solution

Persist each session's transcript separately from its active context. Let users start and resume sessions, configure a total context budget, and continue through automatic or manual compaction. Preserve originals for bounded current-session history retrieval. Show context usage, settings changes, compaction progress, and failures explicitly.

## User Stories

1. As a user, I want a durable session identity, so that I can return to the same conversation.
2. As a user, I want messages saved as the conversation happens, so that persistence does not depend on reaching compaction.
3. As a user, I want to discover and resume saved sessions, so that I can continue previous work.
4. As a user, I want resumed messages appended to the same transcript, so that continuity is preserved.
5. As a user, I want saved compacted context restored, so that every resume does not require another summary request.
6. As a user, I want new sessions to inherit project settings, so that I need not configure every conversation.
7. As a user, I want settings changes to affect only the current session, so that unrelated sessions retain their settings.
8. As a user, I want settings restored on resume, so that the session behaves consistently.
9. As a user, I want context budget choices such as 32K and 64K, so that I can control the amount sent to a model.
10. As a user, I want that budget to include input and reserved output together, so that its meaning is unambiguous.
11. As a user, I want a default output reserve of 25% and an editable percentage, so that I can balance answer length against input capacity.
12. As a user, I want the reserve capped by model capabilities, so that unusable output capacity is not withheld from input.
13. As a user, I want visible input usage, output reserve, and compaction behavior, so that I understand capacity.
14. As a user, I want preflight checks before every model call, so that large prompts and tool results cannot bypass budgeting.
15. As a user, I want automatic compaction at 90% of input capacity, so that room is made before exhaustion.
16. As a user, I want a visible compaction notice, so that pauses and context changes are understandable.
17. As a user, I want a manual `/compact` command, so that I can compact at a useful stopping point.
18. As a user, I want compaction to target 60% of input capacity, so that it creates meaningful room for further work.
19. As a user, I want recent complete exchanges retained within a token allowance of up to 40% of input capacity, so that recent detail survives verbatim.
20. As a user, I want summaries to preserve goals, constraints, decisions, rationale, rejected approaches, unfinished work, relevant identifiers, errors, and transcript references, so that work can continue accurately.
21. As a user, I want uncertain information to remain uncertain in summaries, so that guesses do not become facts.
22. As a user, I want full original messages retained after compaction, so that omitted details remain recoverable.
23. As a user, I want tool-call/result groups and pending execution state preserved during mid-turn compaction, so that the agent can continue legally and correctly.
24. As a user, I want failed automatic compaction retried for at most three total attempts, so that transient failures have a bounded recovery path.
25. As a user, I want a new-session option after those attempts fail, so that I decide whether to leave the current conversation.
26. As a user, I want failed compaction to preserve history and my pending prompt, so that recovery does not lose work.
27. As a user, I want a budget reduction that needs compaction explained and confirmed, so that its consequence is explicit.
28. As a user, I want a smaller budget applied only after successful compaction and a fit check, so that failed changes do not leave unusable settings.
29. As a user, I want oversized current input explained rather than silently truncated, so that I can adjust it knowingly.
30. As a user, I want streaming to continue working through these features, so that persistence and budgeting do not require waiting for full answers.

## Implementation Decisions

- Use ADK Go as the runtime. A small harness-owned session controller remains the main behavioral test seam for CLI actions, settings, permissions, persistence coordination, and recovery; delegate the model/tool execution loop to ADK Runner and its LLM agent. Keep terminal rendering separate.
- Reuse ADK's official OpenAI adapter and model interface for output limits, usage, streamed text, and tool events. Retire the original text-only model interface if redundant; do not build a second provider adapter or framework-neutral runtime abstraction without an actual requirement. Terminal rendering remains independent of provider SDK details.
- Reuse ADK's session service and initially its SQLite backend for durable structured events and session state. Retain a transcript view, a compatible active-context checkpoint, and supplemental interrupted-output capture as needed. Define their recovery relationship; do not duplicate ADK's entire session store. Compaction preserves original records and stable references.
- Compute effective output reserve as the smaller of context budget multiplied by the configured percentage and the model's maximum output allowance. Input capacity is total context budget minus effective reserve. Enforce the output allowance in the provider request.
- Count all request input: instructions, included project memories, tool definitions, message content, summaries, tool calls/results, retrieved excerpts, and the current prompt. Check model compatibility as well as the selected budget.
- Trigger at input usage greater than or equal to 90% of input capacity. Target at most 60% after compaction; retain newest complete exchanges up to 40% subject to the total target. These percentages refer to input capacity, not the total context budget.
- The 60% target is soft when instructions and the current prompt already exceed it. The hard input limit is mandatory. An overlarge current prompt is not silently shortened.
- Preserve model-valid tool-call/result groups and unfinished turn state. Never fabricate successful results for unexecuted calls.
- Summaries retain the information listed in user stories; preserve provenance and uncertainty. Large code and raw tool output can remain in the transcript for retrieval.
- Automatic compaction gets three total attempts: initial attempt plus two retries. Exhaustion blocks the pending ordinary request, preserves the original session, and offers a new session without starting one automatically.
- For budget reductions needing compaction, keep the old settings and history until the candidate compacted context fits the requested budget. Decline, failure, or insufficient reduction leaves the old state active.
- If compaction succeeds but included project memories still prevent fit, invoke the explicit memory-exclusion workflow defined by the project-memory spec.
- Compaction requests themselves must fit the actual model window and their own reserved output allowance. Do not recursively apply ordinary auto-compaction to a compaction request. The bounded summarization strategy is an implementation decision still to resolve.

## Testing Decisions

- Use TDD in vertical behavior slices: demonstrate a meaningful failure, implement the behavior, then refactor with tests passing.
- Main coverage consists of integration tests through the session controller using a scripted model, deterministic token accounting, and real temporary storage. Assert emitted text/prompts, requests, persisted state, and resume behavior rather than private helpers or struct layouts.
- Verify exact threshold edges, reserve capping, changed output percentages, compaction target accounting, complete exchange retention, and budget checks after tool results.
- Verify rollback for declined and failed budget reductions, all three retry outcomes, pending prompt preservation, and no request sent above the permitted input capacity.
- Verify compaction leaves transcript originals intact and resume restores settings and context without unnecessary summary calls.
- Test compaction requests that are themselves too large and ensure recovery is bounded rather than recursively looping.
- Scripted summaries can test retention mechanics and provenance, but cannot establish semantic summary quality. Before shipping, manually evaluate representative long coding conversations for preservation of decisions and rejected alternatives.
- Add focused provider contract tests with a local HTTP stub for output-limit encoding, usage extraction, streaming errors, and completion events. No paid calls are required for the automated suite.
- A few CLI smoke tests may verify command wiring. There are no existing tests in the repository to use as prior art; the current model interface and streaming flow are the existing seams to build on.

## Out of Scope

- Forking sessions, cloud synchronization, multi-user sessions, background compaction, and automatic cross-session history search.
- Optional ask/off compaction modes in the first version.
- Implementing read/search tools or project-memory operations here; those have linked specifications.
- Rewriting the harness in Rust.

## Further Notes

- Runtime decision: [ADR-0001 — adopt ADK Go](../../docs/adr/0001-adopt-adk-go-runtime.md). This supersedes earlier build-from-scratch implementation assumptions without changing the agreed user stories.

- Related specs: [read/search tools](../read-search-tools/spec.md) and [project memory](../project-memory/spec.md).
- Agreed direction is captured in the repository's context-management design notes. The glossary remains the authority for domain vocabulary.
- Open product detail: `/clear` was proposed as starting a new session while retaining the old transcript, but was not explicitly confirmed. The existing implementation only clears in-memory history. Do not silently treat the proposed behavior as approved.
- Open implementation details: default total budget; whether K means 1,000 or 1,024 tokens; percentage bounds and rounding; tokenizer/counting method and estimation margin; unknown model capability handling; storage location and serialization; crash/partial-stream recovery; exact settings and resume commands.
- Pending input, incomplete replies, and completed transcript entries must remain distinguishable; exact recovery representation is not yet chosen.
- Progress through these open details in implementation planning without restarting the design interview or inventing agreement.
