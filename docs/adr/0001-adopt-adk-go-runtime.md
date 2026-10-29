---
status: accepted
---

# Adopt ADK Go as the harness runtime

On 2026-10-29 we chose ADK Go over implementing another agent runtime because the isolated evaluation demonstrated streaming, the OpenAI Responses adapter, function-tool execution, session persistence/resume, compaction records, and hooks for our budget and sequential-execution policies. Reuse ADK's runner, LLM agent, official OpenAI adapter, function tools, and session service; begin integration with the evaluated v2.6.0 release and its SQLite session backend. Keep harness-owned orchestration small and focused on our CLI and product policies rather than wrapping every ADK type or duplicating its execution loop.

## Consequences

- The initial production integration replaces direct provider orchestration with ADK; the existing text-only model interface may be retired rather than expanded into a second runtime abstraction. Use ADK's model interface for scripted test models and isolate provider SDK specifics from terminal rendering.
- Retain custom total-budget/output-reserve calculations, complete request counting, token-based retention and compaction targets, bounded retries, transactional settings, file permissions, tool limits, explicit project memory, and per-session exclusions. ADK's concurrent default and continue-on-compaction-failure behavior do not satisfy our policies: install sequential execution explicitly and keep application-level blocking preflight authoritative.
- Reuse ADK's summary generation and history-preserving compaction representation where compatible, while owning candidate selection, validation, checkpoints, and recovery. Do not blindly copy the prototype's strict 60% target into production: the agreed mandatory-input soft-target exception still applies.
- Use ADK's durable session/event store as the structured history source. Capture visible interrupted output and turn status as an additional concern; define recovery between that journal and the ADK store rather than creating an unrelated second complete session database.
- Adopting ADK does not opt into automatic long-term memory extraction, cross-session retrieval, edits, shell execution, MCP, or multi-agent execution.
- Production requires a compatible Go toolchain (the evaluated release requires Go 1.26.6 or later). Upgrade the root module and pin dependencies in S01, not implicitly through the nested experiment.

## Evidence and limits

The evaluation is retained on branch `prototype/adk-evaluation` under `experiments/adk-prototype`; its README records 18 passing top-level tests and their limits. These establish runtime integration, not a finished harness: real tokenization, semantic summary quality, durable custom compaction, pending-tool resume, external permission restoration, and live-provider checks remain implementation/validation work. S01 subsequently migrated the root CLI onto ADK with SQLite and interrupted-output capture; see its ticket and the root README for production validation and recovery limits. The remaining tickets are not complete merely because their prototypes pass.
