# 06 — Use credentials without exposing them

**What to build:** Configure authentication interactively and allow approved CLI requests to use it while keeping credential values out of public outputs.

**Blocked by:** 05 — Send and inspect an HTTP request

**Status:** ready-for-agent

**Specification coverage:** User stories 14, 41, 43, 47, 52.

- [ ] Masked TUI entry supports adding, replacing and deleting bearer, API key and basic credentials, with protected destination/placement bindings.
- [ ] OS credential storage persists values when available; unavailable services produce an explanation and session-only option, never silent plaintext fallback.
- [ ] CLI secret/access inspection exposes names, configured state and grants only; it cannot retrieve or modify values or protected bindings.
- [ ] Opaque references expand only inside the runner at authorized request positions; destination host/path secret use is rejected.
- [ ] Fresh binding and environment checks govern attachment, including changed headers, absolute URLs, overrides and redirects. Cross-origin redirects strip sensitive headers.
- [ ] Known secret values and authentication positions are sanitized before rendering, errors, previews, copying, exports or any public output; unsafe binary output exposes metadata only.
- [ ] Project export and local state separation prevent sharing credentials or approvals. Document same-account shell access and transformed-secret redaction limitations.
- [ ] Tests exercise Linux/macOS credential boundaries with a controlled OS-store seam and prove secrets absent from stdout, stderr, preview and response output.
