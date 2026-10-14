# 11 — Validate responses and assert results

**What to build:** Turn a saved request into a repeatable test with visible contract and assertion outcomes.

**Blocked by:** 08 — Save and maintain request cases

**Status:** complete

**Specification coverage:** User stories 22, 44, 45, 49.

- [x] Both interfaces author and validate status equality/range, header presence/equality and JSON Pointer presence/typed-equality assertions.
- [x] Response schema validation uses the endpoint response definition and correctly reports unsupported constraints as incomplete.
- [x] JSON null, missing fields and differing value types remain distinct; header names are matched case-insensitively.
- [x] Mandatory body checks cannot pass on truncated or unavailable bodies.
- [x] Results separate transport outcome from assertions/schema outcomes; failed checks produce exit code 6 and structured failure details.
- [x] Assertion failures never expose known secrets when authentication is available.
- [x] Tests cover successful and failed assertions, HTTP error responses, schema mismatches, unsupported constraints and truncated responses through public execution.
