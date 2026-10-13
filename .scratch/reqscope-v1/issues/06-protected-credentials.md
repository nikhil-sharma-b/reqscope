# 06 — Use credentials without exposing them

**What to build:** Configure authentication interactively and allow approved CLI requests to use it while keeping credential values out of public outputs.

**Blocked by:** 05 — Send and inspect an HTTP request

**Status:** complete

**Specification coverage:** User stories 14, 41, 43, 47, 52.

- [x] Masked TUI entry supports adding, replacing and deleting bearer, API key and basic credentials, with protected destination/placement bindings.
- [x] OS credential storage persists values when available; unavailable services produce an explanation and session-only option, never silent plaintext fallback.
- [x] CLI secret/access inspection exposes names, configured state and grants only; it cannot retrieve or modify values or protected bindings.
- [x] Opaque references expand only inside the runner at authorized request positions; destination host/path secret use is rejected.
- [x] Fresh binding and environment checks govern attachment, including changed headers, absolute URLs, overrides and redirects. Cross-origin redirects strip sensitive headers.
- [x] Known secret values and authentication positions are sanitized before rendering, errors, previews, copying, exports or any public output; unsafe binary output exposes metadata only.
- [x] Project export and local state separation prevent sharing credentials or approvals. Document same-account shell access and transformed-secret redaction limitations.
- [x] Tests exercise Linux/macOS credential boundaries with a controlled OS-store seam and prove secrets absent from stdout, stderr, preview and response output.

Implementation: `reqscope-v1/protected-credentials`. Validation and review are recorded in [the implementation review](../../../docs/protected-credentials-review.md). Live OS credential-service acceptance remains separate from the controlled-store tests and macOS cross-build.
