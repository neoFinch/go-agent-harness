# Issue tracker: Local Markdown

Issues and specs live in `.scratch/` within this project.

## Conventions

- One feature per directory: `.scratch/<feature-slug>/`.
- Specs live in `spec.md` within that directory.
- Implementation tickets live in `issues/<NN>-<slug>.md`, numbered from `01`, with one file per ticket.
- Record ticket state with a `Status:` line near the top.
- Record dependencies with `Blocked by:` references to ticket numbers.
- Append discussion under a `## Comments` heading.

## Tracker operations

- Publish: create or update the relevant Markdown file, creating directories as needed.
- Fetch: read the file at the supplied path or locate the referenced ticket number within its feature.
- Work on tickets whose blocking dependencies are resolved.

## Wayfinding

- Store the decision map in `.scratch/<effort>/map.md`, with Notes, Decisions-so-far, and Fog sections.
- Store child tickets in `issues/NN-<slug>.md`.
- Record ticket type with `Type:` (`research`, `prototype`, `grilling`, or `task`).
- Select open, unblocked, unclaimed tickets in numeric order.
- Set `Status: claimed` before work.
- On resolution, append an `## Answer`, set `Status: resolved`, and link the decision from the map's Decisions-so-far section.
