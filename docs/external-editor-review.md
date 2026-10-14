# External editor review

Reviewed `.scratch/reqscope-v1/issues/10-external-editor.md` and the external
editor requirements in `.scratch/reqscope-v1/spec.md` against branch base
`70f7d955f5d5152d994e0ab6754f07dab68bee85`. Two independent review agents inspected
the implementation commit `207267a`. The repository has no
`docs/agents/issue-tracker.md`; the supplied local ticket is the spec source.
Tracker integration can be configured with `/setup-matt-pocock-skills` if needed.

## Standards

No hard documented violations or actionable defects found. The branch and
commit follow AGENTS.md. No judgement-call smell warrants a change in this
scoped implementation.

## Spec

No findings. The implementation covers unexpanded export, case/draft
preservation, recoverable invalid JSON, failure and cancellation preservation,
private scratch files and cleanup, and normal/insert and unsaved-change behavior.
Bubble Tea suspends and restores the terminal around the editor process.

## Validation

`make check` runs Go vet and builds all packages. `make test` runs the full suite
with the race detector. Controlled shell editors run through a real Bubble Tea
program to exercise valid edits, invalid JSON, unchanged exit, failure after
writing, signal cancellation and a missing editor. Tests verify unexpanded
secret references, registered credential rejection before export, scratch
cleanup, case identity, saved bodies, unsaved-change prompts, mode transitions,
and the send-time invalid JSON diagnostic.

The final test refinement selects an active environment and checks the specific
invalid JSON error, so an unrelated preparation failure cannot satisfy the test.

Scratch files contain only inline raw or JSON bodies. Form and file-backed
bodies retain their existing editing flows. Editors must wait until editing
finishes. Cleanup covers normal completion and failures; terminating Reqscope
forcibly can leave a private temporary directory behind.

Final review: zero standards findings and zero spec findings.
