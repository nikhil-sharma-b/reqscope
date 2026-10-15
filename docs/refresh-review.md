# Refresh review

Reviewed the refresh ticket at
`.scratch/reqscope-v1/issues/17-refresh-and-conflicts.md` against branch base
`b8a8711d5634042ecb679986e1264c93ea07f6e9`.

The repository has no `docs/agents/issue-tracker.md`. The local ticket supplies
requirements; tracker integration can be configured with
`/setup-matt-pocock-skills` if needed.

## Standards

The independent review found no documented-standard violations. It noted a
possible duplication smell between refresh input diagnostics and request
preparation. Both use the existing contract serialization and body validators;
refresh does not expand variables or read body files during inspection.

## Spec

The independent review found two defects: refreshing discarded an unsaved project
name, and embedded Swagger sources lost their declaration identity. Refresh now
retains dirty project drafts, and imported embedded sources retain the name used
for rediscovery. Follow-up review found no remaining material spec issues.

## Validation

`make check` passes. Existing project, CLI and TUI tests pass. The full
race-enabled suite passed with `make test`.

New regression tests remain pending confirmation of the test boundaries required
by the invoked TDD skill. The ticket remains in progress until those tests cover
backend changes, conflicts, disconnected cases and repair, stale previews,
external edits and recovery.
