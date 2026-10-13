# 09 — Send form bodies and file uploads

**What to build:** Test endpoints that accept form values or files using guided request editing and the shared HTTP runner.

**Blocked by:** 05 — Send and inspect an HTTP request

**Status:** ready-for-agent

**Specification coverage:** User stories 15, 17, 18.

- [ ] URL-encoded forms and multipart bodies support multiple fields, repeated values, explicit content types and local upload references.
- [ ] TUI selects file references and CLI accepts explicit paths; project data stores references rather than uploaded bytes.
- [ ] File existence/readability and required input validation occur before network access.
- [ ] Encoding follows supported contract serialization and sends correct boundaries and binary contents.
- [ ] Failures, previews and reports remain sanitized and follow existing environment/auth policies as those features are added.
- [ ] Local-server tests verify actual form/multipart bytes, duplicate values, binary uploads and missing/unreadable files.
