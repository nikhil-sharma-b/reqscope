# 14 — Record history and retain baselines

**What to build:** Reopen sanitized request results, keep useful baselines and control local history growth.

**Blocked by:** 06 — Use credentials without exposing them; 08 — Save and maintain request cases

**Status:** ready-for-agent

**Specification coverage:** User stories 28, 43, 50, 52, 53.

- [ ] Saved-case sends automatically record immutable run IDs, input/contract digests, environment, timing, sanitized request/response and outcome in local SQLite.
- [ ] Failed runs are recorded with available evidence; ad hoc requests receive standalone IDs and can later be associated with saved cases.
- [ ] History list/get/pin/unpin/delete/prune operate through both interfaces with bounded CLI previews and paging.
- [ ] Defaults retain 100 unpinned runs per case and cap unpinned project history at 100 MiB; settings are configurable.
- [ ] Cleanup evicts oldest unpinned results first; pins survive and oversized pinned history is visible. Deleting a pin requires unpinning.
- [ ] Redaction precedes storage; large, truncated or unsafe binary bodies expose appropriate metadata. Project exports exclude history and secrets.
- [ ] Tests inspect real SQLite artifacts for secret absence, reopen results, enforce retention and prove cleanup cannot delete protected credentials.
