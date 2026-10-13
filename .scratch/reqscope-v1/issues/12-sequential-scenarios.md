# 12 — Run sequential scenarios

**What to build:** Build and execute a short multi-request workflow with value passing and meaningful dependency-aware failures.

**Blocked by:** 11 — Validate responses and assert results

**Status:** ready-for-agent

**Specification coverage:** User stories 24, 25, 26, 27, 44, 45, 49, 51.

- [ ] Scenario create/get/list/update/delete and validation work through both interfaces with uniquely identified ordered steps referencing cases.
- [ ] Per-step overrides do not mutate source cases; all case/contract inputs are snapshotted at scenario start.
- [ ] Ordinary JSON Pointer extraction provides later step inputs; dependencies refer explicitly to producer steps.
- [ ] Default fail-fast stops on transport/assertion/extraction errors or incomplete mandatory checks; explicit continuation runs independent steps.
- [ ] Failed producer steps cause dependent steps to skip with a reason; any mandatory failure keeps the scenario unsuccessful.
- [ ] Cancellation stops active transport and further steps; production confirmation is required for each actual TUI send.
- [ ] Nested scenarios, loops and scripts are rejected explicitly. Shared-case edits/deletion identify affected scenarios.
- [ ] Aggregate and step reports are sanitized and use documented exit precedence. Tests verify chaining, skips, continued failure, cancellation and snapshot stability.
