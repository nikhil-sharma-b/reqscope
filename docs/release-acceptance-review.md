# Release acceptance review

Reviewed against fixed point `22ba2e5df3ffd6873759b441e0f71e4960a9c796`.
Implementation commits:

- `ee351be feat(release): integrate acceptance workflow and scenario history`
- `b92fbe4 fix(release): bound scenario output and revoke grants for batch deletion`

Source ticket: `.scratch/reqscope-v1/issues/18-release-acceptance.md`.
Specification: `.scratch/reqscope-v1/spec.md`.
Two independent review agents examined standards and specification separately.

## Standards

No documented standards violations found. The branch and commits follow
AGENTS.md; CLAUDE.md references those instructions.

No actionable design smells or evident correctness defects found. The history
persistence defer adds complexity; extracting it is an optional readability
improvement, not a release blocker.

Follow-up review confirmed that check-slice copies prevent preview clipping
from changing retained evidence. Shared commit handling revokes environment
access after revision validation and before deletion journaling, covering batch
Apply as well as the dedicated delete operation.

## Spec

The initial review found one P2 issue: `scenario run` returned full step bodies,
bypassing history preview limits. Commit `b92fbe4` fixes execution output to at
most 50 bounded step summaries, retaining status/check metadata and linked run
IDs. The regression test failed before the fix and passes afterward, while
verifying that all 51 test steps and full sanitized bodies remain in SQLite.

Follow-up review found no remaining specification findings. Environment access
revocation covers shared batch deletion, preventing recreated environments from
inheriting approval.

Native macOS/keychain verification, manual terminal checks and real-backend
acceptance remain explicitly outstanding. Those external gates prevent marking
the release ticket complete.

## Verification

After the follow-up changes, `make check`, `make build`, full race-enabled
`make test`, macOS arm64/amd64 cross-builds and `git diff --check` passed.
The real Linux OS credential integration test passed during this implementation.
Remote CI, native macOS/keychain and real-backend passes are not claimed.

Final findings: standards 0; specification 0 implementation findings, with the
external acceptance gates listed above still outstanding.
