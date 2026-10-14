# 09 — Send form bodies and file uploads

**What to build:** Test endpoints that accept form values or files using guided request editing and the shared HTTP runner.

**Blocked by:** 05 — Send and inspect an HTTP request

**Status:** complete

**Specification coverage:** User stories 15, 17, 18.

- [x] URL-encoded forms and multipart bodies support multiple fields, repeated values, explicit content types and local upload references.
- [x] TUI selects file references and CLI accepts explicit paths; project data stores references rather than uploaded bytes.
- [x] File existence/readability and required input validation occur before network access.
- [x] Encoding follows supported contract serialization and sends correct boundaries and binary contents.
- [x] Failures, previews and reports remain sanitized and follow existing environment/auth policies as those features are added.
- [x] Local-server tests verify actual form/multipart bytes, duplicate values, binary uploads and missing/unreadable files.

Implementation and validation are recorded in [the forms and uploads review](../../../docs/forms-and-uploads-review.md). Upload references resolve from the project root; uploads have a 32 MiB combined limit. Unsupported nested form objects and encoding rules fail before sending.
