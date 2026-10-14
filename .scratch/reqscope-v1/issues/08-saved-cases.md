# 08 — Save and maintain request cases

**What to build:** Save a useful request, reopen or duplicate it, and execute it from either interface without losing its relationship to the endpoint.

**Blocked by:** 05 — Send and inspect an HTTP request

**Status:** complete

**Specification coverage:** User stories 15, 18, 23, 34, 35, 44, 45, 46.

- [x] Cases retain stable IDs, endpoint references, concrete inputs and optional validation settings; create/get/list/update/duplicate/delete work through both interfaces.
- [x] Inputs include ordinary variables and opaque credential references without persisted expanded values.
- [x] Resolution order follows project defaults, environment values, case inputs and explicit run overrides; missing values fail before sending.
- [x] Saved cases and explicit drafts execute with identical runner behavior; unsaved TUI sends are labelled as drafts.
- [x] Save requires expected revision; stale writes return conflicts instead of overwriting. Dirty resources offer save/discard/cancel when leaving or quitting.
- [x] CLI schemas, stdin/file input and no-write previews cover case operations, and names must resolve uniquely.
- [x] Deletion explains references and rejects dangling references; downstream scenario support extends this check when introduced.
- [x] Tests cover reopen/duplicate/edit/run, source identity, undefined variables, revision races and preservation of user inputs.

Implementation and validation are recorded in [the saved cases review](../../../docs/saved-cases-review.md). Optional validation settings are retained; execution is covered by ticket 11. Scenario authoring in ticket 12 will extend deletion reference checks.
