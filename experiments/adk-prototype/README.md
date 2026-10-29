# ADK Go evaluation — prototype only

Question: can ADK supply the runtime for our Go harness while preserving our agreed session, tool, context, and memory policies?

Evaluated on 2026-10-29 using **google.golang.org/adk/v2 v2.6.0**, the latest tagged release returned by the Go module registry during this run. This module requires Go 1.26.6 or later. The installed system Go was 1.26.3; Go downloaded the compatible toolchain automatically. At evaluation time the root harness module and CLI were unchanged. S01 has since migrated the real CLI; see the [root README](../../README.md).

This is an isolated nested module, saved on branch `prototype/adk-evaluation`. It is evidence for an adoption decision, not the implementation of S01 or a replacement production harness. SQLite is an experiment choice, not an approved permanent storage format.

## Run

From this worktree's repository root:

```sh
go test -C experiments/adk-prototype -v -count=1 -timeout=90s ./...
```

First execution may download a compatible Go toolchain and dependencies. Tests use a scripted model, temporary project files, temporary SQLite databases, and a localhost HTTP stub. No API key or paid model call is needed. Tests clean up their temporary data.

## Interactive offline demo

From the evaluation worktree root:

```sh
go run -C experiments/adk-prototype ./cmd/demo
```

Try these commands in order:

1. `/success` — watch the text arrive in chunks, then see the saved turn marked `completed`.
2. `/fail` — watch a partial reply followed by an intentional connection error; the saved turn is marked `failed` and retains its text.
3. `/transcript` — reopen the journal from disk and display the saved turns.
4. `/exit` — exit without deleting the journal.

The demo prints the absolute journal path. To inspect that file after restarting:

```sh
go run -C experiments/adk-prototype ./cmd/demo -transcript /absolute/path/to/transcript.jsonl
```

Then enter `/transcript`. The journal normally lives in a newly created temporary directory and remains after exit, though the OS may eventually clean temporary files. Use `-transcript` with a path in an existing durable directory if you want to retain it longer. Do not run concurrent instances against the same journal.

This is a scripted model, not a live assistant: it demonstrates capture mechanics, not answer quality or provider-network behavior. ADK conversation state is in memory in this demo. Reopening a journal lets you inspect previous captures; it does **not** resume the old ADK session. SQLite session reload is covered separately by the integration probe.

Ctrl+C during streaming cancels the turn and records the visible text with `cancelled` status before exiting. A fast noninteractive smoke run is:

```sh
printf '/success\n/fail\n/transcript\n/exit\n' | go run -C experiments/adk-prototype ./cmd/demo -delay 0
```

## Observed results

All six probes passed:

| Probe | What it established | What it does not establish |
|---|---|---|
| Streaming and SQLite resume | ADK forwards two partial text events, stores user input and final text with stable IDs, reloads history/settings after closing and reopening the database, and continues the same session | Process-crash recovery, durable partial-stream capture, full permission/session state |
| Bounded read tool and sequential execution | ADK calls our Go function and sends results back to the model; our wrapper enforces a line range, byte bound, and symlink scope; an explicit task-runner override executes three requested calls in order | ADK automatically supplies our file permissions, production continuation offsets, or the 10-call policy |
| Before-model callback | A callback can stop a request before the model is called | Token counting or our 90%/60% policy is implemented |
| OpenAI adapter against local HTTP stub | Official adapter streams Responses SSE, sends `max_output_tokens`, and maps input/output usage | Live OpenAI compatibility, model quality, or streaming tool-argument translation against a real provider |
| Compaction and SQLite history | A fixed summarizer changes the next model context while original messages and compaction records survive database reopen | Semantic summary quality, token-based recent retention, bounded summarizer requests, or our retry/rollback policies |
| Partial-stream failure | Consumer sees partial text, error propagates, but default session history retains only user input for this failed turn | Successful partial-answer durability; this probe intentionally characterizes a gap |

The last probe passes by verifying a known limitation. It is not a claim that ADK satisfies our interrupted-stream requirement.

## Capability assessment

### Reuse candidates

- Runner and agent/model orchestration.
- Function-tool schema and invocation plumbing.
- Official OpenAI Responses adapter, subject to further provider/tool tests.
- Session service and SQLite backend for an adoption experiment.
- Compaction event representation preserving raw history.

### Configure explicitly

- ADK defaults to concurrent tool execution. Install `platform.WithTaskRunner` with a sequential executor to satisfy our sequential-only policy. Never assume the default is sequential.
- Use before-model hooks to enforce application preflight, not a prompt instruction asking the model to stay within budget.
- Configure output limits per request through the model configuration.

### Keep as harness policy/custom implementation

- Total context budget; editable 25% output reserve capped by model limits.
- Complete request token counting including instructions, tools, memory, and results.
- Compaction at 90% of input capacity, 60% post-compaction target, recent complete exchanges up to 40% by tokens. ADK's stock retention configuration uses an event count and is not this policy.
- Three total automatic compaction attempts, bounded no-progress recovery, and transactional budget changes.
- Explicit memory saves/conflicts/forgetting and persistent per-session exclusions.
- Exact-file/folder grants, one-invocation and session permission lifetimes, resume prompts, and reserved archive protection.
- Bounded reads/search/history retrieval with continuation and a configurable 10-invocation limit.
- Recording interrupted partial output separately from successful active-history events.

## Recommendation

ADK is a promising runtime dependency, but these probes do not justify replacing all 16 tickets or adopting its defaults unchanged. Keep our policies in a small harness-owned orchestration layer and validate ADK integration underneath it.

Fragmented tool arguments through the OpenAI adapter are now covered by the follow-up below. Remaining adoption probes include interrupted/resumed pending calls, custom token-driven compaction preserving call/result groups, and external permission resumption. A live provider check can follow explicit setup; none was performed here.

No production specification or ticket was marked implemented. S01 has a pointer to this evidence on the prototype branch.

## Sources inspected

- [Official ADK Go repository](https://github.com/google/adk-go)
- [ADK model interface at v2.6.0](https://github.com/google/adk-go/blob/v2.6.0/model/llm.go)
- [OpenAI adapter at v2.6.0](https://github.com/google/adk-go/blob/v2.6.0/model/openaimodel/openaimodel.go)
- [Session database at v2.6.0](https://github.com/google/adk-go/blob/v2.6.0/session/database/service.go)
- [Compaction configuration at v2.6.0](https://github.com/google/adk-go/blob/v2.6.0/session/compaction/compaction.go)
- [Task runner override at v2.6.0](https://github.com/google/adk-go/blob/v2.6.0/platform/exec.go)


## Follow-up: interrupted-stream capture

A harness-owned `RunCaptured` integration was added after demonstrating a failing test: partial text was visible but no capture transcript existed. The first test now passes, along with four further capture checks and all six baseline probes (11 tests total).

The append-only JSONL capture journal stores a stable turn ID, session ID, original prompt, visible text chunks, and a terminal status. Chunks are written, synced, and closed before being delivered to the display callback. Readers reopen the journal through `LoadTranscript`; a begin record without a finish record is represented as `incomplete`, not as a successful reply.

Verified behaviors:

- Injected stream disconnect preserves the original prompt and both visible text chunks with `failed` status and error information.
- That unfinished reply is not inserted into successful ADK conversation history.
- Completed streamed replies are recorded once, without repeating the aggregated final text; non-streamed final-only text is also captured.
- Cancellation records the already-visible text and `cancelled` status.
- Failure to create the journal prevents the model request from starting.
- A write failure between chunks prevents the unsaved chunk from being displayed and leaves previous records recoverable as incomplete.

Remaining limitations:

- This is a single-writer prototype wrapper, not yet wired into the real CLI or the planned session controller. S01 remains unimplemented.
- The capture journal and ADK's event store are separate writes, not an atomic transaction. A failure after ADK persists a final event can leave the two stores with different completion information.
- There is no automatic replay/resumption of interrupted generation, no tool-event journal, and no guarantee of durable writes to a filesystem that is itself failing.
- Torn/corrupt JSON records are reported as errors, not automatically repaired. Crash/power-loss recovery has not been simulated.
- Syncing each chunk favors evidence of durability over throughput; batching policy requires a later decision.
- The interruption tests use a scripted ADK model. Live-provider disconnections, token-limit truncation, and incomplete terminal responses still need provider-specific tests.


## Follow-up: OpenAI streamed tool round-trip

The suite now contains 13 top-level tests, including two failure subcases. All pass without paid model calls.

The new round-trip probe drives the official ADK OpenAI Responses adapter through a local HTTP server, rather than bypassing it with a scripted ADK model:

1. The first request advertises the file-read tool schema.
2. The server streams a function-call item and JSON arguments split across three deltas (including the file name split across chunks).
3. The argument-done event omits the full argument payload and the completion event carries no replacement call, so correct execution depends on assembling the deltas rather than copying a complete call from the final event.
4. ADK invokes one real, bounded project-file read with the reconstructed path and line range.
5. The second HTTP request includes exactly one call and one result with the original provider call ID; its decoded result contains the actual second file line.
6. The final answer is delivered as two visible chunks and the capture journal records a single completed answer without duplication.

Failure subcases prove:

- Malformed argument JSON at argument completion produces an error without tool execution.
- A response body interrupted before argument completion produces a real HTTP unexpected EOF, without tool execution.
- Neither failure starts a follow-up model request, displays tool arguments as answer text, or stores a completed tool call. The capture journal retains a failed turn and error information.

These are synthetic protocol-contract checks, not a claim about live-provider model behavior, every valid Responses event ordering, or recovery after a tool has already executed. The file reader remains a bounded test fixture, not the production tool implementation. No change to the real CLI or production feature status is implied.

Run just these checks:

```sh
go test -C experiments/adk-prototype -run '^TestOpenAI(StreamedToolRoundTrip|ToolStreamFailuresDoNotExecuteTools)$' -v -count=1
```


## Follow-up: custom request-budget hook

A harness-owned `BudgetGate` runs through ADK's before-model callback, with ADK's stock compaction disabled for these probes. The first reserve test failed before the hook enforced an output limit; threshold tests then failed before the compaction path was added. All 18 top-level prototype tests now pass, including their parameterized subcases.

Verified with deterministic count fixtures:

- Default output reserve is 25%, capped by the supplied model output limit, and written to the model request configuration before generation.
- A 50% override changes the allowance; invalid percentages are rejected before the model is called.
- For a 1000-unit context with 250 reserved output, input capacity is 750: 674 proceeds, 675 triggers compaction, and even an initial input of 800 must compact before generation.
- The injected compactor receives a 450-unit post-compaction target (60% of input capacity) and a 300-unit recent-history allowance (40%). The candidate is recounted before it is used.
- Summarization errors and candidates that still exceed the prototype target get three total attempts, then generation is blocked. Original transcript and pending prompt remain available; capture records a failed turn.
- The callback sees instructions and tool schemas and runs again after a tool result. In the tool-loop test, the injected compactor removes an older exchange but retains the active prompt and its complete call/result group before the second model call.
- Failed compaction candidates are deep-copied request data rather than mutations of original messages. Summary calls do not go through this same hook recursively: the injected compactor owns its own bounded implementation.

This establishes the integration point, **not a production token-budget implementation**:

- Counts are explicit deterministic fixture values, not measured tokenizer output or provider counts. Real request serialization, tokenizer choice, estimation margin, and model capability discovery remain unimplemented.
- Summary generation is injected. The 40% recent-history allowance is passed to it, but a general token-based selection algorithm is not implemented here.
- This conservative prototype enforces the 60% target strictly. The agreed soft-target exception for large mandatory instructions/current prompts still needs implementation; the experiment must not silently replace that product policy.
- Compacted request state is not yet stored as the session's durable checkpoint, so future turns can require compaction again. There is no settings UI, transactional budget reduction, memory exclusion flow, or new-session offer after failure in this prototype.
- The gate is configured with caller-supplied capability values and rounding choices for the experiment. These are not approved production defaults.

Run these probes:

```sh
go test -C experiments/adk-prototype -run '^TestBudget' -v -count=1
```

Conclusion: ADK supplies a usable pre-request enforcement hook, including within tool loops. Our budget and recovery behavior should remain harness-owned; enabling ADK's default compaction would not implement it automatically.
