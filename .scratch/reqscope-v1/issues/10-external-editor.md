# 10 — Edit request bodies externally

**What to build:** Edit a large request body with the developer's existing editor and safely return to the current case.

**Blocked by:** 08 — Save and maintain request cases

**Status:** ready-for-agent

**Specification coverage:** User stories 16, 18, 35, 39, 43.

- [ ] TUI editor action exports the unexpanded body and returns to the same case/draft after editing.
- [ ] Successful edits are revalidated before send; invalid edited JSON produces recoverable diagnostics.
- [ ] Cancellation or a nonzero editor exit preserves the previous buffer and reports the outcome.
- [ ] No expanded credential enters editor scratch content; scratch cleanup runs on normal completion and failure.
- [ ] Normal/insert modes and unsaved-change handling continue to work around the editor round trip.
- [ ] Tests use a controlled external editor command to exercise successful edits, invalid output, cancellation, failure and secret exclusion.
