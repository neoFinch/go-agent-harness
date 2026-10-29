# Agent Harness

A Go terminal harness using ADK Go v2.6.0 and its official OpenAI Responses adapter.

## Run

Use Go 1.26.6 or newer. Go can download the compatible toolchain automatically when toolchain auto-selection is enabled.

With `OPENAI_API_KEY` exported in the same terminal:

```sh
go run ./cmd/harness
```

The default model is `gpt-4o-mini`. Override it with `OPENAI_MODEL` or a flag:

```sh
go run ./cmd/harness -model gpt-4o-mini
```

`OPENAI_BASE_URL` is supported by ADK's adapter for a compatible endpoint. No environment values or credentials are printed.

Enter prompts normally. Replies stream as they arrive and successful exchanges remain available within the current session.

- `exit` or `/exit`: exit.
- `/clear`: reset active model context, retaining the same session identity and all saved records.
- Ctrl+C: cancel generation and exit. Already-saved visible text remains available, marked cancelled when the status write succeeds.

The startup message shows the session ID and database path. Every launch currently starts a new session; session listing and resume are planned in S02. `/settings`, `/compact`, file tools, and project-memory commands are not wired into this CLI yet.

## Local storage and failure behavior

By default, project-local data lives in `.harness/sessions.sqlite`, excluded from Git. Choose another directory with:

```sh
go run ./cmd/harness -data-dir /absolute/path/to/session-data
```

ADK's built-in SQLite session service owns structured model events. Supplemental `harness_turns` records in the same database capture original input, visible output, stable turn/invocation IDs, and status. A row starts as `incomplete`; each visible chunk is committed before display. A turn becomes `completed` only after ADK finishes successfully and its final status is saved. Stream errors, truncation, and cancellation are distinguished from completion.

Only invocations with a completed capture record after the latest clear marker are included in later model history. Failed prompts and partial replies remain saved for inspection but are not silently replayed as successful context. Clear markers change this active-history view without deleting ADK events or captured turns.

SQLite uses WAL, full synchronous writes, and a single connection per controller. A newly created data directory uses owner-only permissions and the database is created with mode 0600. Stored conversations are local data, not source files.

ADK events and capture status are separate database writes, not one transaction across a network call. If the process stops or status persistence fails after ADK saved a final event, the capture remains incomplete and is conservatively excluded from future successful history. We do not automatically promote ambiguous records to completed or replay generation. If storage fails, errors are shown; unsaved text is not displayed. Automatic repair of interrupted writes and full process/power-loss testing are not claimed by this slice.

## Verify without API calls

```sh
go test ./... -count=1
go test -race ./... -count=1
```

A small event-copy plugin avoids an ADK v2.6.0 race between Runner and its event producer during early stream termination, using the supported event replacement hook.

Tests exercise the real controller and SQLite with scripted ADK models, plus the real CLI and official provider adapter against a localhost HTTP stub. They cover streaming, saved transcripts, follow-up history, clear behavior, failed-turn exclusion, cancellation, truncation, and storage failures. No paid API requests are needed.

The isolated framework experiment remains separate:

```sh
go test -C experiments/adk-prototype ./... -count=1
go run -C experiments/adk-prototype ./cmd/demo
```

The demo's `/success`, `/fail`, and `/transcript` commands are mock controls, not production CLI features.

## Plan

See [.scratch/README.md](.scratch/README.md) for implementation tickets and [ADR-0001](docs/adr/0001-adopt-adk-go-runtime.md) for the runtime decision.
