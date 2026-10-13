# Send HTTP request review

Reviewed ticket `.scratch/reqscope-v1/issues/05-send-http-request.md` against
branch base `df64c87d1e9a86ba3c74d4e601f1a47f9464ad5a`. The local ticket is the spec
source; this repository does not yet have `docs/agents/issue-tracker.md`.
Tracker integration can be configured with `/setup-matt-pocock-skills` if needed.

## Standards

The independent standards review found no documented-standard violations.
It found two correctness issues: serialized integer path arrays were validated
as a single element, and repeated independent variables were mistaken for
cycles. Both are fixed and covered by regression tests. The review also identified
duplicated interpolation logic. Environment URLs and request fields now share
variable expansion, with URL validation retained at the environment boundary.

## Spec

The independent spec review found two ticket 05 gaps: query parameters embedded
in URLs bypassed contract validation, and optional JSON bodies could not be
omitted. Both are fixed and covered by local-server regression tests. External
editor round trips belong to ticket 10 and are outside this ticket's scope.

A further limit check found that reading one extra byte to detect truncation
could wait until timeout when a server stalled exactly at the cap. Body reads
now stop at the cap. Unknown-length bodies reaching the cap are conservatively
marked truncated; complete bodies with an exact known content length are not.

## Validation

`make check` runs Go vet and builds all packages. `make test` runs the full suite
with the race detector. Local-server tests exercise actual request bytes,
TUI/CLI parity, production confirmation, invalid input, destination overrides,
redirect authorization and limits, cancellation, timeouts, TLS verification,
response retention, binary metadata, and contract serialization.

Protected credentials remain unavailable. Supported serialization and deferred
forms/uploads, external editing, full schema validation and run history are
documented in README.md.

Final review: zero unresolved standards findings, zero unresolved ticket 05
spec findings.
