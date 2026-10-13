# Initialize-project implementation review

Ticket: [.scratch/reqscope-v1/issues/01-initialize-project.md](../.scratch/reqscope-v1/issues/01-initialize-project.md).

Reviewed changes from `e2b0944` on `reqscope-v1/initialize-project`, initially with `git diff e2b0944...HEAD`, followed by review of the fixes with `git diff HEAD`. Standards and spec reviews ran in separate parallel agents through the code-review skill.

## Standards

No documented standards violations. Branch and commit names satisfy AGENTS.md; CLAUDE.md adds no separate rules.

Two nonblocking judgment calls were raised:

- Numeric pane dispatch repeated across rendering and actions. Named `paneID` constants and one `actionPanes` map now replace it. Follow-up review passed.
- Input-schema names, shapes and validation dispatch require coordinated changes. A small registry could consolidate them. This remains an optional improvement for the four explicit input shapes.

## Spec

Two correctness findings were raised and fixed:

- Explicit missing project paths returned storage failure/exit 7. They now return an actionable `not_found` error/exit 3, with a regression test.
- Focused project search ignored the query. It now filters fields, marks the current match and shows an explicit empty result, with a regression test.

Follow-up review found no additional issues. It also verified the required leader groups, endpoint-search shortcut and separate palette query. Placeholder panes preserve issue 01's scope; later workflow features remain deferred.

## Validation

- `go test -race ./...` passed after the fixes, including real CLI processes, revision races and recovery after killing a writer during a 1,501-resource transaction.
- `go vet ./...` and `go build ./...` passed.
- Linux amd64 built and ran natively; Linux arm64, macOS arm64 and macOS amd64 cross-builds passed. macOS runtime acceptance has not run.
- A terminal smoke test initialized a project, opened effective help, displayed the leader continuation menu and exited cleanly.
- `git diff --check` passed.

Totals: Standards had two nonblocking suggestions, one resolved and one optional schema-registry improvement remaining. Spec had two findings, both fixed. No unresolved correctness findings.
