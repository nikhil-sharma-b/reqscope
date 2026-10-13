# 08 — Save and maintain request cases

**What to build:** Save a useful request, reopen or duplicate it, and execute it from either interface without losing its relationship to the endpoint.

**Blocked by:** 05 — Send and inspect an HTTP request

**Status:** ready-for-agent

**Specification coverage:** User stories 15, 18, 23, 34, 35, 44, 45, 46.

- [ ] Cases retain stable IDs, endpoint references, concrete inputs and optional validation settings; create/get/list/update/duplicate/delete work through both interfaces.
- [ ] Inputs include ordinary variables and opaque credential references without persisted expanded values.
- [ ] Resolution order follows project defaults, environment values, case inputs and explicit run overrides; missing values fail before sending.
- [ ] Saved cases and explicit drafts execute with identical runner behavior; unsaved TUI sends are labelled as drafts.
- [ ] Save requires expected revision; stale writes return conflicts instead of overwriting. Dirty resources offer save/discard/cancel when leaving or quitting.
- [ ] CLI schemas, stdin/file input and no-write previews cover case operations, and names must resolve uniquely.
- [ ] Deletion explains references and rejects dangling references; downstream scenario support extends this check when introduced.
- [ ] Tests cover reopen/duplicate/edit/run, source identity, undefined variables, revision races and preservation of user inputs.
