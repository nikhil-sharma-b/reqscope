# Swagger URL import implementation review

Ticket: `.scratch/reqscope-v1/issues/07-swagger-url-import.md`.
Branch: `reqscope-v1/swagger-url-import`.
Baseline: `6333fb40c118fec1de0c49cc3682dd69d466cc07`.

The implementation adds static Swagger discovery, raw JSON/YAML URL imports,
local source-origin approval, protected credential attachment and bounded remote
reference fetching. Both adapters reuse contract inspection and atomic storage.
The README documents selection, approval, authentication and raw-URL fallback.

## Standards

The independent standards review found no documented violations. Its initial
judgment-call finding about an unreachable redirect loop was fixed. The final
standards review found no outstanding maintainability findings.

## Spec

Independent specification reviews identified and verified fixes for these cases:

- Ordinary query parameters were rejected. They now remain in source metadata;
  authentication parameters and registered credential values are rejected.
- Optional script failures blocked valid candidates. Discovery now retains
  candidates and records resource warnings.
- TUI failures omitted candidate and raw-URL fallback guidance. The import form
  now shows that guidance and wraps long source details.
- Source URLs could expose registered secrets or rejected resource credentials.
  Checks now precede fetching, discovery output and persistence.
- Multiple embedded declarations collapsed into one candidate. Numbered names
  distinguish declarations and identify them when rereading their resource.

Regression tests reproduced the query, optional-script, TUI fallback, URL-secret
and multiple-embedded failures before their fixes. A changed embedded declaration
also fails the reviewed digest check before applying an import.

Final totals: Standards 0 unresolved; Spec 0 unresolved.

## Validation

Passed after the final code changes:

- `go test -race ./...`
- `go vet ./...`
- `go build ./...`
- `GOOS=darwin GOARCH=arm64 go build ./...`
- Focused public project API, CLI and TUI import tests during implementation.

Local HTTP fixtures cover relative initializer/config resources, multiple
candidates, raw JSON/YAML, embedded specifications, discovery fallback,
authenticated imports, denied origins, redirects, missing and cyclic references,
size bounds, failed-import preservation and inspection without network access.
Credential tests use controlled or session-only storage. No external backend
or live macOS credential service was needed.

Redirects produce a sanitized error requiring the final raw URL. JavaScript is
never executed. Local-file imports retain their existing filesystem reference
root and do not fetch remote URLs.
