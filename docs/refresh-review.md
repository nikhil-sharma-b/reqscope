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

Regression tests cover the confirmed project API, CLI command and TUI update/view
boundaries. Coverage includes add/change/remove, independent three-way merges,
unresolved conflicts, saved input and body diagnostics, stable source/endpoint
identity, disconnected cases, explicit relinking and independent conversion,
stale previews after incoming/contract/case/environment changes, invalid external
files, embedded Swagger refresh, protected credential rejection and approval
revocation without changing environment destinations or credential bindings.

A process interruption test kills a refresh writer after its journal is published.
Public readers must recover the contract, tombstone and 800 disconnected cases
before returning. It uses the same journal-observation approach as existing
recovery coverage; interruption timing depends on process scheduling.

TUI coverage verifies selected-source preview, cancellation, confirmation, stale
and conflicting previews, clean external reload, dirty-buffer retention, explicit
reconciliation and discard/reload. The regression suite exposed a clean case
editor retaining stale JSON after polling; reloading now updates both the case
and its open editor. CLI coverage exposed credential redaction of the generic
`token` output key. Preview and apply now use `previewToken`, which survives
sanitization and supports a complete CLI preview/apply cycle.

Final independent review reported zero documented standards violations and zero
material spec findings.
