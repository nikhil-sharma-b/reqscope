# Protected credentials implementation review

Ticket: `.scratch/reqscope-v1/issues/06-protected-credentials.md`.
Branch: `reqscope-v1/protected-credentials`.
Baseline: `e848051951e19f4d9895b8ae66d104a0ab8402b9`.

The implementation adds masked TUI credential entry, OS storage with explicit
session-only recovery, protected destination/placement bindings, read-only CLI
inspection, runner-only reference expansion and sanitization before public output.
Values and approvals stay outside project files. See the README for usage and
same-account shell access and transformed-secret limitations.

## Standards

An independent standards reviewer checked `AGENTS.md`, existing project patterns
and the code-review smell baseline. Branch and commit conventions pass.

Two initial judgment calls were resolved with named credential form indices and
a `credentialPosition` type for destination/placement checks. The final review
reported zero hard violations and zero outstanding judgment calls.

## Spec

An independent spec reviewer checked the ticket and credential/access portions
of the full specification. Three findings were fixed and re-reviewed:

- Unavailable stored credentials blocked TUI startup. Startup now reaches a
  masked recovery screen that allows environment selection and credential
  replacement, including explicit session-only storage.
- Truncated JSON could expose unregistered authentication values. Incomplete
  structured bodies are withheld when they cannot be safely sanitized. Form
  authentication fields are sanitized by position.
- XML authentication values lacked position redaction. XML elements and
  attributes are sanitized; malformed or truncated XML is withheld.

Regression tests reproduced the failures before the fixes. The final targeted
review reported no unresolved findings.

Final totals: Standards 0 unresolved; Spec 0 unresolved.

## Validation

Passed after the final code changes:

- `go test -race ./...`
- `go vet ./...`
- `go build ./...`
- `GOOS=darwin GOARCH=arm64 go build ./...`
- Focused runner, CLI and TUI credential tests during implementation.

Tests cover bearer/basic/API-key attachment, query/body/header placement,
JSON escaping, replacement/deletion, grant revocation, path/origin and redirect
rejection, persisted and session-only state, unavailable-store recovery,
masked entry, public output/errors/previews and response position redaction.
The OS credential-store seam uses a controlled store; no live Linux Secret
Service or macOS Keychain acceptance was performed. No external backend was used.
