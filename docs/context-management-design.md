# Context management design

## Agreed direction

- Adopt ADK Go as the runtime per ADR-0001: reuse runner/model/tool/session infrastructure; retain harness-specific policies. Production CLI migration starts in S01; the nested experiment is not itself the migrated product.

- Support starting a new session and resuming an existing saved session in the first version; defer forking.
- Resuming continues the same session and appends new messages to its transcript.
- Persist compacted context alongside the transcript so resuming does not require generating a new summary each time.
- On resume, restore the saved active context, checking that it fits the current budget before the next model request. Compact if necessary.
- Make the resumed session's transcript available for history retrieval when that capability is implemented.
- Model-driven history retrieval is restricted to the current session's full transcript, including compacted messages. Other sessions are not automatically searchable; explicitly resuming one makes it the current session.
- The user-selected context budget covers instructions, conversation history, the current prompt, and reserved output together.
- Default the output reserve to 25% of the selected context budget. Users can change the percentage in settings.
- New sessions inherit project defaults. Settings changes apply to the current session; resuming a session restores its saved settings.
- When reducing the context budget, check whether current input plus the newly calculated output reserve fits. If it does not, explain that compaction is required before applying the lower budget.
- For a reduction requiring compaction, ask before compacting and apply the new budget only after compaction succeeds and the resulting context fits. If declined, failed, or still too large, retain the old budget and active history.
- Cap the calculated output reserve at the selected model's maximum output limit; the remaining context budget is available for input.
- Reserve output space before sending a request and enforce the corresponding output limit.
- Keep context usage and compaction mode prominent in settings.
- Trigger the compaction policy before exhausting the available input allowance.
- Initially trigger automatic compaction before a request when estimated input usage, including the current prompt, reaches 90% of input capacity (context budget minus effective output reserve).
- Target at most 60% of input capacity after compaction, counting instructions, summary, retained recent exchanges, and the current prompt. This is a soft target when instructions and the current prompt already exceed it; the actual input limit remains mandatory.
- Retain the newest complete exchanges verbatim up to 40% of input capacity during compaction, subject to space for instructions, the current prompt, and the summary within the overall target. Summarize older exchanges while preserving their originals in the transcript.
- Summaries preserve the current goal, unfinished work, user constraints and preferences, decisions and their rationale, rejected approaches, relevant file paths, identifiers, errors, and transcript message references. Preserve uncertainty explicitly rather than turning guesses into facts; large code blocks and raw tool output remain retrievable from the transcript.
- Automatically compact by default and display a notice when it starts.
- First-version compaction controls are automatic compaction and manual `/compact`; defer optional ask/off modes.
- Preserve the existing history if compaction fails and explain the failure.
- On automatic compaction failure, retry up to three total attempts (the initial attempt plus two retries). If all fail, do not send the pending model request or discard the session; offer the user the option to start a new session. Starting a new session requires user choice.

## Still to decide

- Confirm `/clear` semantics: proposed as starting a new session while retaining the old transcript, not deleting it.
- Transcript storage and the resume command interface.
- Token counting strategy.
- Settings command interface, storage format, and validation of incompatible budgets and output limits.

The earlier 32K total, 4K output, and 25K compaction example was illustrative; its numeric defaults are not agreed.

## Related scope

- Defer MCP integration. Discuss built-in tools and memory separately before deciding their initial scope.
- The first built-in tool milestone supports reading/searching project files and the current session's transcript. Defer file creation/editing and shell execution.
- File reads use explicit line ranges and return bounded chunks, with line numbers and a clear indication when more content is available. The model can request subsequent ranges. Bound search results as well, indicating when additional matches exist.
- Enforce a maximum returned byte size in addition to line ranges, including for a single oversized line. On reaching the byte limit, explicitly mark the partial result and provide a continuation offset so subsequent reads can recover the remaining content without silently dropping it.
- Default to at most 10 tool invocations per user turn, editable in session settings. Count each invocation, including multiple calls requested together. When the limit is reached, preserve progress, explain that the limit was reached, and offer explicit continuation rather than silently resetting the counter.
- Execute tool invocations sequentially only; do not run calls concurrently, including when a model response requests multiple calls.
- Check the context budget before every model call, including calls inside a tool loop. Compaction may occur mid-turn, but must preserve pending tool-call state and retain tool calls with their corresponding results as complete groups rather than splitting them.
- Return tool failures as structured errors to the model so it can correct paths or ranges. Failed invocations count toward the per-turn tool limit. Permission denial remains enforced; error recovery must not bypass it.
- File tools read/search only within the current project by default. A user may explicitly provide an external path to read; this exception does not automatically authorize access to its parent directory or unrelated external paths. Model-generated paths alone do not grant external access.
- When the user explicitly grants read/search access to an external folder, allow that folder and its descendants for the current session. Resolve symlinks and prevent traversal outside authorized scopes.
- On session resume, always show a confirmation dialog for previously authorized external paths rather than silently restoring permission. Offer allow once, allow for the resumed session, or deny. One-time permission authorizes one tool invocation: searching a folder and reading a result are separate invocations. Session permission lasts only until the next resume.
- Support project memory across sessions only when the user explicitly requests it, such as "save this as memory" or "remember this". Ordinary conversation is not silently promoted to project memory. This is a narrowly scoped persistence operation, separate from deferred general-purpose file editing tools.
- An explicitly saved memory replaces a clearly conflicting existing project memory, with a visible confirmation identifying the replacement. Preserve unrelated memories and ask for clarification when the conflict is ambiguous.
- `/memory` lists saved project memories with stable IDs; `/memory forget <id>` removes a memory from future project-memory inclusion. Removing project memory does not erase its mentions from saved conversation transcripts; transcript deletion is separate.
- Include saved project memories in each model request by default and count their tokens against input capacity. Compaction does not rewrite or remove saved project memories.
- If compaction succeeds but the request still cannot fit with project memories included, ask the user which memories to exclude from the current session, displaying stable IDs and estimated token sizes. Do not silently evict memories or send an oversized request.
- Session exclusion keeps project memories saved on disk and is distinct from `/memory forget <id>`, which removes saved project memory. Neither operation erases previous transcript mentions.
- Persist memory exclusions with the session and restore them on resume. New sessions begin with all saved project memories eligible for inclusion.
