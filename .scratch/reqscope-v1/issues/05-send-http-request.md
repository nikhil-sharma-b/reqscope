# 05 — Send and inspect an HTTP request

**What to build:** Construct and send an HTTP request from a contract or explicit draft, then inspect its result using the same execution behavior in the TUI and CLI.

**Blocked by:** 02 — Import and inspect local API specifications; 04 — Configure environments and authorize destinations

**Status:** complete

**Specification coverage:** User stories 13, 15, 18, 19, 20, 21, 39, 40, 48, 49, 51.

- [x] Guided fields expose required inputs and descriptions; initial contract examples are suggestions and never overwrite edited content.
- [x] Path/query/header serialization follows supported contract rules; JSON and raw bodies work and valid repeated fields are preserved.
- [x] Missing required inputs, invalid JSON and undefined variables fail before contacting the server. Ad hoc drafts use the same policy as saved requests.
- [x] All destinations, server alternatives, absolute URLs, interpolated hosts and redirects pass the environment policy; rejected redirects are not contacted.
- [x] Each production TUI send requires confirmation with method and sanitized destination; cancelling prevents the send.
- [x] TLS verification defaults on; timeout defaults to 30 seconds, redirects to five and retained body size to 10 MiB. Limits are configurable; no automatic retries occur.
- [x] Requests are cancellable. Status, timing, headers and searchable/copyable response content are shown; binary and truncated bodies expose limitations.
- [x] HTTP error statuses remain completed exchanges; transport failures, input errors and access denials use distinct documented exit codes.
- [x] Local-server tests record actual bytes, verify TUI/CLI parity, redirects, cancellation and limits. Protected credentials remain unavailable until ticket 06 provides safe attachment/redaction.

Implementation: `reqscope-v1/send-http-request`. Validation and review are recorded in [the implementation review](../../../docs/send-http-request-review.md).
