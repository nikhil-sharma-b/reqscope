# 14 — Record history and retain baselines

**What to build:** Reopen sanitized request results, keep useful baselines and control local history growth.

**Blocked by:** 06 — Use credentials without exposing them; 08 — Save and maintain request cases

**Status:** complete

**Specification coverage:** User stories 28, 43, 50, 52, 53.

- [x] Saved-case sends automatically record immutable run IDs, input/contract digests, environment, timing, sanitized request/response and outcome in local SQLite.
- [x] Failed runs are recorded with available evidence; ad hoc requests receive standalone IDs and can later be associated with saved cases.
- [x] History list/get/pin/unpin/delete/prune operate through both interfaces with bounded CLI previews and paging.
- [x] Defaults retain 100 unpinned runs per case and cap unpinned project history at 100 MiB; settings are configurable.
- [x] Cleanup evicts oldest unpinned results first; pins survive and oversized pinned history is visible. Deleting a pin requires unpinning.
- [x] Redaction precedes storage; large, truncated or unsafe binary bodies expose appropriate metadata. Project exports exclude history and secrets.
- [x] Tests inspect real SQLite artifacts for secret absence, reopen results, enforce retention and prove cleanup cannot delete protected credentials.

Implementation notes:

- Local SQLite history is keyed by project ID and location outside project files.
- CLI `history` commands and TUI history leader actions expose inspection, paging,
  retention settings, pin protection, cleanup and ad hoc case association.
- Retention byte limits count serialized evidence, excluding SQLite page/index overhead.
- Request evidence retains safe bodies up to 1 MiB; uploads and unsafe bodies retain metadata.
- Verification covers SQLite artifact secret absence, reopened failures and responses,
  default retention, pins, paging, association and protected credentials surviving cleanup.
