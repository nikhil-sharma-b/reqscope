# Saved cases review

Reviewed ticket `.scratch/reqscope-v1/issues/08-saved-cases.md` against branch
base `415481e4dd4267ea3878d6b0b456c7ed7735ae99`. The local ticket and
`.scratch/reqscope-v1/spec.md` provide the requirements. This repository does
not have `docs/agents/issue-tracker.md`; tracker integration can be configured
with `/setup-matt-pocock-skills` when needed.

## Standards

The independent standards review found no documented-standard violations.
It identified duplicated case-loading and run-input handling, plus a correctness
bug: saved-case run input bypassed schema validation and accepted YAML numbers
as strings. The shared case-loading helper and named run-input type remove the
duplication. Actual saved runs now use the published input validator. A
regression test verifies that validation and execution both reject numeric
variable values. The follow-up review confirmed these fixes and reported no
remaining actionable findings.

## Spec

The independent spec review found the same run-input validation mismatch and
an endpoint identity bug: composing a different endpoint after opening a saved
case reused the old request and case identity. New/compose actions now create
a draft after the dirty-buffer decision. A regression test composes the second
endpoint, saves a separate case, and checks that the original endpoint reference
and revision remain unchanged.

## Validation

`make check` passes Go vet and builds all packages. `make test` passes the full
suite with the race detector. Tests cover saved-case lifecycle operations,
stdin/file input, schemas and no-write previews, ambiguous names, variable
precedence, undefined variables before sending, non-mutating overrides,
endpoint/source identity, concurrent revision conflicts, deletion reference
checks, TUI dirty navigation/quit choices, and saved-run TUI/CLI parity against
a local HTTP server.

Optional validation settings are retained. Cases containing validation settings
fail before transport until the validation ticket implements execution.
Scenario authoring will extend the existing `caseId` deletion reference checks.

The follow-up spec review confirmed both fixes and reported no additional
findings. Final review: zero unresolved standards findings and zero unresolved
ticket 08 spec findings.
