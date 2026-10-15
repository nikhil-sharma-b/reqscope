# 17 — Refresh contracts and reconcile concurrent edits

**What to build:** Adopt backend updates deliberately, preserve saved inputs and reconcile concurrent human/agent edits without silent data loss.

**Blocked by:** 07 — Import from Swagger UI and specification URLs; 08 — Save and maintain request cases; 16 — Author and export API contracts

**Status:** in-progress

**Specification coverage:** User stories 31, 32, 33, 35, 42, 46.

- [x] Explicit refresh preview shows added/changed/removed endpoints and affected cases, comparing current edits against imported base and incoming source.
- [x] TUI confirms application; CLI uses a preview token bound to current/incoming digests. Stale previews are rejected.
- [x] Unresolved three-way conflicts block apply; user inputs remain preserved and incompatible inputs receive actionable diagnostics.
- [x] Removed operations leave disconnected cases and explanatory tombstones; those cases cannot send until explicitly relinked or converted to independent requests.
- [x] Destination or binding changes never gain access from refresh; relevant approvals are invalidated.
- [x] TUI reloads clean external changes while preserving dirty buffers and offering reload/discard/manual reconciliation.
- [x] Interrupted multi-resource refreshes recover consistently; readers see coherent state.
- [ ] Tests exercise backend add/change/remove, local conflicts, preserved cases, relinking, stale previews, external invalid edits and interrupted apply.


Implementation notes:

- Branch: `reqscope-v1/refresh-and-conflicts`.
- Shared refresh computes a document-level three-way merge, retains source and
  endpoint identity, and binds preview tokens to current resources and incoming
  document digests. Conflicts block apply.
- Removed-operation cases retain inputs, receive a persisted disconnected flag,
  and commit with endpoint tombstones through the recovery journal. Explicit case
  editing repairs links or converts to independent requests.
- CLI preview/apply and apply input schema are available. TUI confirms refresh and
  offers explicit external-case reload or manual reconciliation.
- Independent standards/spec review completed. Spec review caught dirty project
  draft loss and embedded Swagger identity loss; both were fixed and re-reviewed.
- Completion is pending new regression tests. The invoked TDD skill requires
  user confirmation of test seams; the proposed boundaries are project refresh/
  case APIs, CLI commands and TUI update/view. Confirmation was requested in-session.

Validation: `make check` and `make test` passed, including the full race-enabled suite.
