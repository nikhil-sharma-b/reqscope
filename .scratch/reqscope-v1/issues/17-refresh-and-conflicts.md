# 17 — Refresh contracts and reconcile concurrent edits

**What to build:** Adopt backend updates deliberately, preserve saved inputs and reconcile concurrent human/agent edits without silent data loss.

**Blocked by:** 07 — Import from Swagger UI and specification URLs; 08 — Save and maintain request cases; 16 — Author and export API contracts

**Status:** ready-for-agent

**Specification coverage:** User stories 31, 32, 33, 35, 42, 46.

- [ ] Explicit refresh preview shows added/changed/removed endpoints and affected cases, comparing current edits against imported base and incoming source.
- [ ] TUI confirms application; CLI uses a preview token bound to current/incoming digests. Stale previews are rejected.
- [ ] Unresolved three-way conflicts block apply; user inputs remain preserved and incompatible inputs receive actionable diagnostics.
- [ ] Removed operations leave disconnected cases and explanatory tombstones; those cases cannot send until explicitly relinked or converted to independent requests.
- [ ] Destination or binding changes never gain access from refresh; relevant approvals are invalidated.
- [ ] TUI reloads clean external changes while preserving dirty buffers and offering reload/discard/manual reconciliation.
- [ ] Interrupted multi-resource refreshes recover consistently; readers see coherent state.
- [ ] Tests exercise backend add/change/remove, local conflicts, preserved cases, relinking, stale previews, external invalid edits and interrupted apply.
