# Read/search tools and permissions

Status: ready-for-agent

## Problem Statement

The model currently sees only messages supplied by the user. It cannot inspect project files or recover exact details omitted by compaction. File access also needs explicit scope, bounded output, and a controlled model/tool execution loop.

## Solution

Provide built-in project read/search tools and dedicated current-session transcript retrieval. Execute calls sequentially, mediate external access through user permissions, return bounded chunks and structured errors, and enforce a session-configurable per-turn tool limit.

## User Stories

1. As a user, I want the model to read project files, so that I do not have to paste their contents.
2. As a user, I want project search, so that the model can find relevant files and content.
3. As a user, I want line-range reads with line numbers, so that only relevant file portions enter context.
4. As a user, I want a byte cap as well as a line range, so that one enormous line cannot fill the context.
5. As a user, I want an explicit partial-result marker and continuation offset, so that remaining content can be requested without gaps.
6. As a user, I want bounded search matches and an indication of more matches, so that searches do not exhaust capacity.
7. As a user, I want the model to retrieve exact earlier current-session messages after compaction, so that summaries need not preserve every detail.
8. As a user, I want transcript results to contain stable message references, so that recovered details are attributable.
9. As a user, I want history retrieval restricted to the current session, so that unrelated saved conversations do not influence it.
10. As a user, I want file access restricted to the current project by default, so that unrelated files are not exposed.
11. As a user, I want an explicitly supplied external file to be readable without granting its parent directory, so that access is narrow.
12. As a user, I want to grant access to an external folder and its descendants, so that reference projects can be inspected.
13. As a user, I want symlink and path traversal checked against authorized scopes, so that tools cannot escape permission limits.
14. As a user, I want model-suggested paths treated as requests rather than authorization, so that only my actions grant external access.
15. As a user, I want resume to ask before restoring previous external access, so that old permissions do not silently persist.
16. As a user, I want allow-once, allow-for-session, and deny choices, so that I control access duration.
17. As a user, I want allow-once to cover one invocation, so that a search does not implicitly permit subsequent file reads.
18. As a user, I want sequential execution even when multiple tools are requested, so that authorization and results are predictable.
19. As a user, I want at most 10 tool invocations per turn by default, so that accidental loops have a bound.
20. As a user, I want that limit configurable in session settings, so that complex investigations can use a suitable allowance.
21. As a user, I want failed invocations counted toward the limit, so that repeated failures cannot create an endless loop.
22. As a user, I want progress retained and an explicit continuation option at the limit, so that work can continue under my control.
23. As a user, I want structured tool errors returned to the model, so that it can correct a missing path or invalid range.
24. As a user, I want permission denial enforced during error recovery, so that retrying cannot bypass my choice.
25. As a user, I want budget checks before each follow-up model call, so that tool results do not cause an oversized request.
26. As a user, I want history retrieval to work after resume, so that archived detail remains useful throughout a session's lifetime.

## Implementation Decisions

- Build the tool loop within the session-controller flow described by the sessions/context spec. The model adapter translates SDK tool events to harness-owned calls and translates results back; terminal code does not execute provider-specific events directly.
- Extend the existing text-only model contract to represent tool requests, stable call IDs, structured results, completion, and streaming text. Exact Go types are not prescribed by this spec.
- Tool responsibilities are project file read/search and current-session transcript search/read. Transcript retrieval is dedicated, not unrestricted filesystem access to the session archive store.
- Project scope and explicitly authorized external scopes are checked by the harness, not delegated to model judgment. A grant for one external file does not imply parent or sibling access; folder grants cover descendants only.
- Resolve symlinks and reject traversal outside authorized scopes. Generic file tools must not become a backdoor to other sessions' transcript stores, even if storage is physically located inside a project. The treatment of reserved internal storage paths needs explicit implementation coverage.
- Resume prompts for previously authorized external paths offer once, session, or deny. One-time permission is consumed by one invocation, not an entire user turn. Session grants are not silently restored on a later resume.
- Explicit external paths must come from user authorization, not text found inside project files, transcripts, tool output, or model-generated instructions.
- Reads accept line ranges, enforce a byte limit, and report continuation. The continuation representation must support partial reads within a single line and avoid silent omission or duplication.
- Search results are bounded and indicate additional matches. All returned content contributes to context usage; byte limits alone do not replace the session's token preflight.
- Execute one invocation at a time. Each invocation consumes the per-turn allowance, including failed attempts and calls requested together. Default allowance is 10, editable per session.
- At the limit, stop executing additional tools, preserve queued state, and offer explicit continuation. Never silently reset the allowance to make a pending request run.
- Return structured errors for denied access, invalid ranges, missing files, and execution failures. Preserve the model-valid association between a call and its result. Permission errors cannot expand access.
- Budget checks apply before every follow-up model request. Coordinate with compaction so pending calls and completed call/result groups survive mid-turn compaction.

## Testing Decisions

- Drive integration tests through the same session controller as normal turns, with a scripted model producing realistic tool sequences and real temporary project/external directories.
- Observe returned content, structured errors, permission prompts, execution order, call counts, and subsequent model inputs. Avoid tests that merely mirror dispatch implementation.
- Test allowed project reads, exact-file grants, folder grants, sibling rejection, traversal, external symlink targets, and protection of other session archives.
- Test allow-once consumption, denial, session grants, and mandatory reauthorization after resume.
- Test normal ranges, invalid ranges, end of file, oversized single lines, multibyte text, bounded search results, and reconstruction through continuation without missing content.
- Prove sequential execution for multiple calls in one model response; include errors and pending-call recovery at the limit.
- Test the 10-call default, configuration overrides, failed-call counting, and no execution until explicit continuation after exhaustion.
- Verify retrieval of original compacted messages in the current session and rejection of cross-session access.
- Use focused provider contract tests with synthetic streaming tool events to verify accumulation of fragmented arguments and correct result/call ID association.
- Use TDD for each externally observable slice. No real API requests or machine-specific files are necessary. The repository currently has no tool tests or tool implementation as prior art.

## Out of Scope

- General-purpose file creation/editing, shell execution, MCP integration, concurrent tool execution, and cross-session transcript search.
- Vector databases or embedding-based retrieval in the first version.
- Broad machine-wide file access or implicit authorization based on model requests.

## Further Notes

- Related specs: [sessions/context](../sessions-context/spec.md) and [project memory](../project-memory/spec.md).
- Exact tool names, argument schemas, result caps, search syntax, excluded directories, binary-file handling, file mutation during chunked reads, and cancellation behavior remain implementation details to resolve.
- Permission prompts are CLI interactions in the current harness; the word dialog does not require a graphical interface.
- Transcript storage and tool metadata must support stable source references without assuming raw transcript files are readable through general project tools.
