# 18 — Complete the release workflow on Linux and macOS

**What to build:** Deliver the complete documented Reqscope workflow on both supported systems and verify all slices compose into the agreed release.

**Blocked by:** 03 — Find endpoints quickly; 09 — Send form bodies and file uploads; 10 — Edit request bodies externally; 13 — Chain authentication securely; 15 — Compare recorded responses; 17 — Refresh contracts and reconcile concurrent edits

**Status:** ready-for-agent

**Specification coverage:** All 54 user stories and the release acceptance workflow.

- [ ] Scenario execution integrates with history, retaining a sanitized aggregate report and individual step records, including secret login responses.
- [ ] All required commands expose correct help, schemas, previews, paging, revision handling, structured output and exit codes; unattended execution never prompts.
- [ ] The TUI action registry includes every delivered operation with configurable Vim/leader keys, correct help and readable narrow layouts.
- [ ] Linux/macOS builds and platform credential integration are verified; unavailable credential services preserve the documented session-only behavior.
- [ ] Document installation, project layout, Swagger fallback, environment authorization, secret limitations, CLI examples, comparisons and refresh recovery.
- [ ] End-to-end local fixtures demonstrate import/search, staging authentication, saved cases, response comparison, secure scenarios, CLI parity, denied production and invalidated grants.
- [ ] Run required adversarial checks across raw requests, edited destinations/classifications/bindings, redirects, reports, SQLite and editor artifacts.
- [ ] Verify transaction/revision races and retention/truncation behavior against real local storage, with no internal mock proliferation.
- [ ] Complete the real-backend acceptance workflow after the developer provides/configures access, or explicitly record that this external gate remains outstanding. Never claim a real-backend pass from fixture tests.
- [ ] Release readiness accounts for every specification story and acceptance gate; failures are fixed in their responsible slices rather than hidden by final integration.
