# 10 — Edit request bodies externally

**What to build:** Edit a large request body with the developer's existing editor and safely return to the current case.

**Blocked by:** 08 — Save and maintain request cases

**Status:** complete

**Specification coverage:** User stories 16, 18, 35, 39, 43.

- [x] TUI editor action exports the unexpanded body and returns to the same case/draft after editing.
- [x] Successful edits are revalidated before send; invalid edited JSON produces recoverable diagnostics.
- [x] Cancellation or a nonzero editor exit preserves the previous buffer and reports the outcome.
- [x] No expanded credential enters editor scratch content; scratch cleanup runs on normal completion and failure.
- [x] Normal/insert modes and unsaved-change handling continue to work around the editor round trip.
- [x] Tests use a controlled external editor command to exercise successful edits, invalid output, cancellation, failure and secret exclusion.

Implementation and validation are recorded in [the external editor review](../../../docs/external-editor-review.md). Use `Space r o` to edit inline raw or JSON bodies with `$VISUAL`, `$EDITOR`, or `vi`.
