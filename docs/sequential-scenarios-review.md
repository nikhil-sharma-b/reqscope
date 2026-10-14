# Sequential scenarios review

Reviewed ticket `.scratch/reqscope-v1/issues/12-sequential-scenarios.md` against
branch base `98e102ae98db7595216fd09cdef3e81e8d3d7400`. The local ticket and
`.scratch/reqscope-v1/spec.md` provide the requirements. This repository has no
`docs/agents/issue-tracker.md`; tracker integration can be configured with
`/setup-matt-pocock-skills` when needed.

## Standards

The independent review found that scenario creation could open an editor
during an active run, then lose the editor when a confirmation or completion
arrived. Creation now uses the existing active-run guard. Follow-up review
found no remaining documented violations or meaningful code smells.

## Spec

The independent review found that production redirects sent requests without
individual confirmation. The scenario confirmation callback now reaches the
redirect handler. Each production redirect shows its method and sanitized URL,
requires approval before contact, and checks authorization again after approval.
Production redirects without a confirmation callback are rejected. Confirmation
errors and mismatched fingerprints stop even a continued scenario. Follow-up
review found no remaining requirements gaps or scope creep.

## Validation

`make check` passes vet and builds all packages. `make test` passes the full
suite with the race detector. Its first run caught a test-fixture race when a
hijacked connection returned before the test read recorded paths. A mutex now
protects that fixture's writes and reads.

Project API tests cover CRUD, revision conflicts, previews, references, rejected
features, escaped JSON Pointers, exact numbers and null, run-only overrides,
dependent skips, continued failures, transport/assertion/extraction errors,
incomplete checks, active cancellation, sanitized extraction failures, and case
and contract snapshot stability. A regression prevents batch writes from
creating dangling scenario references.

CLI tests cover authoring, schemas, input types, references, execution and exit
precedence. TUI tests cover authoring, validation, editing, deletion, CLI request
parity, shared-case impact confirmation, cancellation, per-step production
confirmation, and production redirects approved or cancelled before contact.

Secret login chaining and durable history remain assigned to tickets 13 and 14.
Scenario reports currently return through the shared API and CLI, with a compact
TUI summary. No extracted values are persisted to environment settings.

Final review: zero unresolved standards findings and zero unresolved spec findings.
