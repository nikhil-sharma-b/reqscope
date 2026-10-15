# Secure login chaining review

Reviewed `.scratch/reqscope-v1/issues/13-secure-login-chaining.md` and the login
extraction requirements in `.scratch/reqscope-v1/spec.md` against branch base
`deb6ce7b1a7470a1870ae86bb0dd82a883e6139b`.

This repository has no `docs/agents/issue-tracker.md`. The local ticket supplies
the requirements; `/setup-matt-pocock-skills` can configure tracker integration.

## Standards

The independent review found no documented standards violations. It identified
one optional maintainability suggestion: share destination placement validation
between stored credentials and secret extraction definitions. The checks remain
separate because extraction restricts authentication kinds and requires explicit
canonical field names, while stored credential entry normalizes header names.

## Spec

The independent review found that ordinary extraction could read an approved
protected field if its response type changed from a string to a number or
object. Regression tests reproduced this for null, numbers, empty strings,
objects and arrays. Protection now registers values before response checks and
redacts the selected subtree regardless of type before output. Response checks
still inspect the original response, preserving schema and assertion behavior.

Protected definitions require explicit TUI approval bound to the scenario,
source case revisions, contracts and environment settings. The CLI exposes no
approval operation. Generated credentials live only in a private run instance;
only redaction knowledge survives in the adapter's credential registry.
Approved pointers remain protected across ordinary scenarios and direct sends
in the same environment, conservatively hiding matching fields elsewhere.

## Validation

`make check` passes vet and builds every package. `make test` passes the complete
suite with the race detector.

Project boundary tests cover successful bearer login chaining, absence of tokens
from reports and project/local access files, ordinary extraction readback,
modified pointers and bindings, source-case and environment edits, removal of
protection, runtime destination changes, failed extraction dependent skips,
rejected origins and header/query/body placements, blocked redirects, and
successful API-key query/body placement. TUI/CLI coverage checks the complete
approval review, cancellation, denial before approval, authenticated CLI execution
after approval, and sanitized TUI output.

Follow-up independent review found no remaining spec or security gaps and no
new standards issues. Final totals: standards has zero documented violations
and one optional duplication suggestion; spec has zero unresolved findings.
