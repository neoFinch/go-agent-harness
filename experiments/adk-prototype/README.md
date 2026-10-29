# ADK Go evaluation — prototype only

Question: can ADK supply the runtime for our Go harness while preserving our agreed session, tool, context, and memory policies?

Evaluated on 2026-10-29 using **google.golang.org/adk/v2 v2.6.0**, the latest tagged release returned by the Go module registry during this run. This module requires Go 1.26.6 or later. The installed system Go was 1.26.3; Go downloaded the compatible toolchain automatically. The root harness module and CLI are unchanged.

This is an isolated nested module, saved on branch `prototype/adk-evaluation`. It is evidence for an adoption decision, not the implementation of S01 or a replacement production harness. SQLite is an experiment choice, not an approved permanent storage format.

## Run

From this worktree's repository root:

```sh
go test -C experiments/adk-prototype -v -count=1 -timeout=90s ./...
```

First execution may download a compatible Go toolchain and dependencies. Tests use a scripted model, temporary project files, temporary SQLite databases, and a localhost HTTP stub. No API key or paid model call is needed. Tests clean up their temporary data.

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

Before adoption, next useful probes are streaming tool arguments through the OpenAI adapter, interrupted/resumed pending calls, custom token-driven compaction preserving call/result groups, and external permission resumption. A live provider check can follow explicit setup; none was performed here.

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
