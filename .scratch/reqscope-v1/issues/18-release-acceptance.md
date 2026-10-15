# 18 — Complete the release workflow on Linux and macOS

**What to build:** Deliver the complete documented Reqscope workflow on both supported systems and verify all slices compose into the agreed release.

**Blocked by:** 03 — Find endpoints quickly; 09 — Send form bodies and file uploads; 10 — Edit request bodies externally; 13 — Chain authentication securely; 15 — Compare recorded responses; 17 — Refresh contracts and reconcile concurrent edits

**Status:** awaiting-external-acceptance

**Specification coverage:** All 54 user stories and the release acceptance workflow.

- [x] Scenario execution integrates with history, retaining a sanitized aggregate report and individual step records, including secret login responses.
- [x] All required commands expose correct help, schemas, previews, paging, revision handling, structured output and exit codes; unattended execution never prompts.
- [x] The TUI action registry includes every delivered operation with configurable Vim/leader keys, correct help and readable narrow layouts.
- [ ] Linux/macOS builds and platform credential integration are verified; unavailable credential services preserve the documented session-only behavior.
- [x] Document installation, project layout, Swagger fallback, environment authorization, secret limitations, CLI examples, comparisons and refresh recovery.
- [x] End-to-end local fixtures demonstrate import/search, staging authentication, saved cases, response comparison, secure scenarios, CLI parity, denied production and invalidated grants.
- [x] Run required adversarial checks across raw requests, edited destinations/classifications/bindings, redirects, reports, SQLite and editor artifacts.
- [x] Verify transaction/revision races and retention/truncation behavior against real local storage, with no internal mock proliferation.
- [x] Complete the real-backend acceptance workflow after the developer provides/configures access, or explicitly record that this external gate remains outstanding. Never claim a real-backend pass from fixture tests.
- [x] Release readiness accounts for every specification story and acceptance gate; failures are fixed in their responsible slices rather than hidden by final integration.

Implementation and evidence:

- Scenario history retains sanitized aggregate reports and linked individual
  exchange, preflight-failure and skipped-step records. CLI previews are bounded;
  TUI history reopens reports. Pins and byte retention apply to each record.
- Release audit completed the environment slice's missing environment deletion
  and ordinary variable CLI operations, with schemas, previews, expected revisions
  and TUI deletion. Deletion revokes approval without lowering protected production
  classification or deleting credential values.
- A reusable local Swagger/account backend acceptance test covers the composed
  CLI workflow. Existing TUI parity and adversarial/storage suites remain at the
  public/shared boundaries. No production backend is used by automated tests.
- `make check`, full race-enabled `make test`, native Linux build, real Linux
  Secret Service integration and macOS arm64/amd64 cross-builds passed.
- Linux/macOS native CI is configured. Native macOS tests, keychain integration,
  manual terminal checks and real-backend acceptance are still outstanding.
  No access to a configured real backend was supplied. Cross-builds and fixtures
  do not close those gates; this ticket is not marked complete.
- All 54 stories, automated evidence, known limits and manual acceptance steps
  are recorded in `docs/release-acceptance.md`. README covers installation and
  the delivered operations. Review results are in `docs/release-acceptance-review.md`.

- Independent standards and specification reviews completed. The initial
  unbounded scenario execution output finding was fixed with a failing-then-passing
  regression test; both follow-up reviews report zero remaining implementation
  findings. Full race tests and builds passed again after the fixes.
