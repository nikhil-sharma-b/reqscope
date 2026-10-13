# Environments and access review

Ticket: `.scratch/reqscope-v1/issues/04-environments-and-access.md`.
Baseline: `0a2702ba9a85eef15c24dc47aeca9856da979fa3`.

## Standards

The independent review found one production protection gap. Saving a newly
created or promoted production environment initially deferred local protection
until the next load. The implementation now reconciles committed settings under
the project lock before returning. A regression test covers an immediate external
classification downgrade after promotion.

## Spec

The independent review found that a single header could clip the active destination.
The TUI now reserves wrapped rows for environment, classification and effective
base URLs. Tests cover 80 and 32 columns with a long project name.

Both reviewers confirmed their fixes. No remaining findings on either axis.

## Validation

`make check` passes Go vet and build. `make test` passes the full race-enabled
suite. Shared operations cover default denial, approved staging, deliberate
production approval, exact origins, renaming, variable changes, copied projects,
classification protection and selection. CLI tests cover explicit environment
requirements and denied classification lowering. TUI event tests cover authoring,
cancelled approval, confirmed approval, remembered selection and destination
visibility.

HTTP sending and credential lifecycle are separate tickets. This implementation
provides the shared fresh-settings authorization boundary for those operations;
the check-access command performs no network request.
