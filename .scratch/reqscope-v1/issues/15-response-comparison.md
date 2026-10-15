# 15 — Compare recorded responses

**What to build:** Compare two runs of the same case and suppress known incidental differences without changing retained evidence.

**Blocked by:** 14 — Record history and retain baselines

**Status:** complete

**Specification coverage:** User stories 29, 30, 50.

- [x] Both interfaces compare arbitrary retained run IDs for the same case, including across environments, and show environment/contract/input differences.
- [x] Comparison covers status, case-insensitive headers and JSON additions/removals/changes by JSON Pointer.
- [x] Object ordering is ignored, array ordering is retained and null differs from absent fields.
- [x] Non-JSON text uses a text diff; binary, redacted or truncated content carries explicit comparison limitations.
- [x] Case-level ignore rules support exact JSON Pointers and header names; rules affect comparison only and leave stored responses intact.
- [x] Missing or evicted runs produce clear errors, and compared output remains sanitized and bounded.
- [x] Tests cover cross-environment runs, ignore rules, typed differences, arrays, missing/null fields and unavailable content.

Implementation notes:

- Shared `CompareRuns` reads retained evidence under the history lock. CLI
  `compare BEFORE AFTER` and TUI `Space h d` expose arbitrary same-case IDs.
- Saved `comparisonIgnore` case rules are available through both case editors;
  CLI `--input` can replace rules for one comparison without changing evidence.
- Results label context changes, typed JSON Pointer differences, line replacements,
  metadata and explicit redaction, truncation, unavailable-content and output limits.
- `make check` and the full race-enabled `make test` suite pass. Additional
  contract-change integration coverage passes through the public project API.
- Independent standards and specification reviews found no unresolved issues;
  see `docs/response-comparison-review.md`.
