# Domain docs

This repo uses a single-context layout: root `CONTEXT.md` and `docs/adr/`.

## Before exploring

Read root `CONTEXT.md` and relevant decisions in `docs/adr/`.
If either is absent, proceed without requesting its creation.
Create domain documents lazily as terms and decisions are resolved.

## Vocabulary

Use terms as defined in `CONTEXT.md`.
Keep that file a glossary, without implementation details.
If a concept is missing, reconsider whether it belongs to the domain or note the gap for domain modeling.

## Decisions

Surface contradictions with existing ADRs explicitly rather than silently overriding them.
Record ADRs only for decisions with meaningful reversal costs, non-obvious reasoning, and genuine trade-offs.
