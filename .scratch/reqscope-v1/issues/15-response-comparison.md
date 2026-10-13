# 15 — Compare recorded responses

**What to build:** Compare two runs of the same case and suppress known incidental differences without changing retained evidence.

**Blocked by:** 14 — Record history and retain baselines

**Status:** ready-for-agent

**Specification coverage:** User stories 29, 30, 50.

- [ ] Both interfaces compare arbitrary retained run IDs for the same case, including across environments, and show environment/contract/input differences.
- [ ] Comparison covers status, case-insensitive headers and JSON additions/removals/changes by JSON Pointer.
- [ ] Object ordering is ignored, array ordering is retained and null differs from absent fields.
- [ ] Non-JSON text uses a text diff; binary, redacted or truncated content carries explicit comparison limitations.
- [ ] Case-level ignore rules support exact JSON Pointers and header names; rules affect comparison only and leave stored responses intact.
- [ ] Missing or evicted runs produce clear errors, and compared output remains sanitized and bounded.
- [ ] Tests cover cross-environment runs, ignore rules, typed differences, arrays, missing/null fields and unavailable content.
