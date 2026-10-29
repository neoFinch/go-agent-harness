# Sessions/context decision map

## Notes

- [Feature specification](spec.md) defines agreed behavior and integration test boundaries.
- [Ticket index](../README.md) defines ordering and cross-feature dependencies.

## Decisions-so-far

- [S01](issues/01-persist-streamed-session.md#answer): ADK Runner and official OpenAI adapter; SQLite events plus visible-output capture; incomplete replies excluded from successful history.
- [S02](issues/02-new-list-resume.md#answer): `/new`, `/sessions`, `/resume <id>`, and `-resume`; versioned ADK state checkpoints and saved model; `/clear` keeps session identity.

## Fog

- [S03](issues/03-budget-settings-preflight.md): actual budget defaults, units, tokenizer/capabilities, validation, and output enforcement.
- [S04](issues/04-manual-compaction.md): extend the checkpoint for compacted context while retaining transcript originals.
- Automatic repair and concurrent writing of a session remain unsupported.
