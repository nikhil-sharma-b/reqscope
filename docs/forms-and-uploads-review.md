# Forms and uploads review

Reviewed ticket `.scratch/reqscope-v1/issues/09-forms-and-uploads.md` and the
request execution requirements in `.scratch/reqscope-v1/spec.md` against branch
base `5e0c73590aa6b781f8cf0101eda3abfcce391514`. The repository has no
`docs/agents/issue-tracker.md`; the supplied local ticket is the spec source.
Tracker integration can be configured with `/setup-matt-pocock-skills` if needed.

## Standards

The independent review found one stale README statement claiming that forms
and uploads were unsupported. The existing request-support paragraph now
includes them and describes the encodings that are rejected. The follow-up
review confirmed the correction and found no outstanding standards findings.

## Spec

The independent review found two issues. Swagger arrays without an explicit
`collectionFormat` used repeated values instead of the required CSV default.
Form contracts could also accept raw bodies without checking required fields.
Swagger arrays now default to CSV while OpenAPI retains its form/explode
behavior. Contract forms require structured form input so their field validation
runs before sending. Local-server regression tests cover both fixes. The
follow-up review confirmed both corrections and found no remaining spec gaps.

## Validation

`make check` runs Go vet and builds all packages. `make test` runs the complete
suite with the race detector. Local-server tests verify URL encoding, duplicate
values, multipart boundaries, explicit file content types, exact binary bytes,
missing/unreadable/nonregular uploads, OpenAPI required fields and scalar types,
Swagger CSV and repeated serialization, protected form credentials, and response
sanitization. Other tests verify stable confirmation fingerprints, invalidation
when upload bytes change, saved references without bytes, raw file bodies, and
TUI file selection with CLI wire parity.

Relative upload references resolve from the project root. Files are read into
memory before transport, with a 32 MiB combined upload limit. Unsupported nested
form objects and encoding rules fail before sending. Uploaded bytes are opaque;
only form values expand variables and protected credential references.

Final review: zero unresolved standards findings and zero unresolved spec findings.
