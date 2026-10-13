# 11 — Validate responses and assert results

**What to build:** Turn a saved request into a repeatable test with visible contract and assertion outcomes.

**Blocked by:** 08 — Save and maintain request cases

**Status:** ready-for-agent

**Specification coverage:** User stories 22, 44, 45, 49.

- [ ] Both interfaces author and validate status equality/range, header presence/equality and JSON Pointer presence/typed-equality assertions.
- [ ] Response schema validation uses the endpoint response definition and correctly reports unsupported constraints as incomplete.
- [ ] JSON null, missing fields and differing value types remain distinct; header names are matched case-insensitively.
- [ ] Mandatory body checks cannot pass on truncated or unavailable bodies.
- [ ] Results separate transport outcome from assertions/schema outcomes; failed checks produce exit code 6 and structured failure details.
- [ ] Assertion failures never expose known secrets when authentication is available.
- [ ] Tests cover successful and failed assertions, HTTP error responses, schema mismatches, unsupported constraints and truncated responses through public execution.
