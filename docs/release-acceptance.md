# Release acceptance

Reqscope v1 is not yet certified for release. The local automated workflow is
implemented. Native macOS execution, the macOS credential service and the developer's real
backend remain external gates. No backend
URL or credentials were provided for ticket 18.

## Reproduce automated acceptance

```sh
make check
make test
make build
```

`TestReleaseAcceptanceLocalWorkflow` uses one temporary project and a local HTTP
backend through the public CLI entry point. It imports a Swagger UI contract,
searches by path and description, creates asserted cases, sends through approved
staging, compares real recorded responses with saved ignore rules, executes a
login/secret-extraction scenario, inspects/pins/pages its SQLite history, denies
production and foreign destinations, invalidates a changed grant and refreshes
the source without losing saved request inputs. Interactive provisioning uses
the same shared approval and credential operations as the TUI. It does not add
a CLI authorization bypass. Separate TUI event tests verify parity and approval.

`TestProtectedScenarioLoginAndNoSecretOutputs` uses a fresh credential registry
and a newly issued token. It verifies the consumer's actual Authorization bytes,
linked individual history and the aggregate report, then scans project and local
data files, including SQLite, for secret bytes. The full suite also covers raw
request bypasses, edited destinations/classifications/bindings, host expansion,
redirects, form uploads, assertion diagnostics, contract exports, external editor
scratch files, revision races, transaction recovery and truncation/retention.
The credential-store substitute exists only at the OS integration boundary.

## Verification recorded on 2026-10-15

- Linux x86_64: `make check`, `make test` and `make build` passed. The full
  race-enabled suite passed in CLI, project and TUI packages.
- Linux Secret Service: the opt-in `TestOSCredentialIntegration` passed using
  the real OS adapter, including entry cleanup.
- macOS arm64 and amd64: `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64|amd64 go build`
  passed. These binaries were cross-compiled on Linux and were not executed.
- Native macOS suite, keychain and manual terminal checks: outstanding.
- Real backend acceptance: outstanding, access not configured.
- The remote CI matrix is configured; no remote CI run is claimed here.

## Story coverage

Numbers refer to the [approved specification](../.scratch/reqscope-v1/spec.md).
Paths below are test files, each containing named behavior checks. Automated
coverage establishes local behavior; story 54 also needs native macOS evidence.

| Story | Behavior | Evidence in `internal/` |
| --- | --- | --- |
| 1 | Initialize/reopen | `project/project_test.go`, `cli/process_test.go` |
| 2 | JSON/YAML imports | `cli/specification_test.go` |
| 3 | Raw URL import | `cli/remote_specification_test.go` |
| 4 | Swagger discovery | `tui/source_import_test.go`, `project/remote_specification_test.go` |
| 5 | Discovery fallback | `tui/source_import_test.go`, `cli/remote_specification_test.go` |
| 6 | Choose specification | `project/remote_specification_test.go`, `tui/source_import_test.go` |
| 7 | Import diagnostics | `cli/specification_test.go`, `cli/remote_specification_test.go` |
| 8 | Search paths/descriptions/tags | `project/search_test.go`, `tui/search_test.go` |
| 9 | Method filters/tag grouping | `project/search_test.go`, `cli/search_test.go` |
| 10 | Input descriptions/schemas | `project/request_test.go`, `tui/specification_test.go` |
| 11 | Separate environments | `project/project_test.go`, `project/access_test.go` |
| 12 | Rename/add environments | `tui/environment_test.go`, `cli/environment_lifecycle_test.go` |
| 13 | Effective destination | `tui/environment_test.go` |
| 14 | Masked credential management | `tui/credentials_test.go` |
| 15 | Construct request inputs | `project/request_test.go`, `tui/request_test.go` |
| 16 | External editor | `tui/editor_test.go` |
| 17 | Forms/uploads | `project/forms_test.go`, `tui/request_test.go` |
| 18 | Preflight validation | `project/request_test.go`, `project/forms_test.go` |
| 19 | Cancel active request | `cli/request_test.go`, `tui/request_test.go` |
| 20 | Response inspection/search | `tui/request_test.go`, `project/request_test.go` |
| 21 | Copy response | `tui/request_test.go` |
| 22 | Assertions/schema checks | `project/assertions_test.go`, `tui/cases_test.go` |
| 23 | Case lifecycle | `project/cases_test.go`, `tui/cases_test.go`, `cli/cases_test.go` |
| 24 | Sequential scenario | `tui/scenarios_test.go`, `cli/scenarios_test.go` |
| 25 | Extract and bind values | `project/scenarios_test.go`, `project/secret_extractions_test.go` |
| 26 | Skip failed dependencies | `project/scenarios_test.go` |
| 27 | Continue independent steps | `project/scenarios_test.go` |
| 28 | Record runs/pin baselines | `project/history_test.go`, `project/scenarios_test.go`, `tui/history_test.go` |
| 29 | Compare recorded responses | `project/comparison_test.go`, `tui/comparison_test.go` |
| 30 | Saved ignores | `cli/comparison_test.go`, `tui/comparison_test.go` |
| 31 | Refresh preview | `cli/refresh_test.go`, `tui/refresh_test.go` |
| 32 | Preserve saved inputs | `project/refresh_test.go`, `cli/release_acceptance_test.go` |
| 33 | Disconnected case recovery | `project/refresh_test.go` |
| 34 | Local contract edits | `project/contract_authoring_test.go`, `tui/contract_authoring_test.go` |
| 35 | Concurrent/stale edits | `project/validation_test.go`, `project/recovery_test.go`, `tui/refresh_test.go` |
| 36 | Vim/pane navigation | `tui/navigation_test.go`, `tui/model_test.go` |
| 37 | Leader/continuation overlay | `tui/navigation_test.go`, `tui/model_test.go` |
| 38 | Effective binding help | `tui/navigation_test.go` |
| 39 | Normal/insert modes | `tui/model_test.go`, `tui/editor_test.go` |
| 40 | Production confirmation | `tui/request_test.go`, `tui/scenarios_test.go` |
| 41 | TUI CLI authorization | `tui/environment_test.go`, `tui/scenarios_test.go` |
| 42 | Invalidate changed destinations | `project/access_test.go`, `project/secret_extractions_test.go` |
| 43 | Sanitize before output/storage | `project/credentials_test.go`, `project/secret_extractions_test.go`, `tui/editor_test.go` |
| 44 | CLI resource authoring | `cli/contract_authoring_test.go`, `cli/cases_test.go`, `cli/scenarios_test.go`, `cli/environment_lifecycle_test.go` |
| 45 | Schemas/no-write preview | `cli/process_test.go`, `cli/contract_authoring_test.go`, `cli/environment_lifecycle_test.go` |
| 46 | IDs/revisions | `project/revision_test.go`, `project/cases_test.go` |
| 47 | Opaque authenticated execution | `cli/credentials_test.go`, `cli/release_acceptance_test.go` |
| 48 | Explicit environment/access | `cli/access_test.go`, `project/request_test.go` |
| 49 | JSON/exit codes | `cli/process_test.go`, `cli/scenarios_test.go` |
| 50 | Bounded history/comparison | `cli/history_test.go`, `project/comparison_test.go`, `project/scenarios_test.go` |
| 51 | No unattended prompts | `cli/process_test.go`, `cli/scenarios_test.go`, `cli/remote_specification_test.go` |
| 52 | Local-state/export isolation | `project/contract_authoring_test.go`, `project/access_test.go` |
| 53 | Retention/cleanup | `project/history_test.go`, `project/scenarios_test.go` |
| 54 | Narrow/platform behavior | `tui/environment_test.go`, `tui/model_test.go`; macOS runtime gate outstanding |

`TestScenarioCLIReportBoundsAndRetainsFullHistory` verifies that execution output
returns at most 50 bounded step summaries while SQLite retains all steps and full
sanitized bodies. `TestEnvironmentDeletionRevokesAccessWithoutLoweringProtection`
also covers deletion through the shared batch writer.

The registry covers delivered request, environment, credential, case, scenario,
history, comparison, refresh and contract actions. Variable editing uses the
ordinary environment JSON editor. Environment deletion uses `Space e x`.
Binding overrides, help, leader conflicts and narrow layouts use the existing
TUI event/render seam. Clipboard output depends on terminal OSC52 support.

## Platform gates

The GitHub Actions acceptance matrix builds, vets and runs the race suite natively
on Linux and macOS. Adding the workflow is not evidence that its remote jobs have
run. Cross-compilation checks build compatibility only, not native credential
services or terminal behavior.

With an unlocked Linux Secret Service or macOS login keychain, run:

```sh
REQSCOPE_TEST_KEYRING=1 go test ./internal/project -run '^TestOSCredentialIntegration$' -count=1 -v
```

This opt-in test writes one random entry through the real OS adapter, uses it
against a local HTTP server and deletes it. It never uses existing credentials.
OS dialogs may require human interaction. The default suite skips this test and
checks unavailable-service behavior through the credential-store boundary.
Persistent save failure must ask for an explicit session-only choice. Session
values work in the current process and become unconfigured after restart; a
separate CLI process cannot inherit a TUI session-only secret.

On each OS, open the native binary in a terminal and record:

- Import/search, pane navigation, custom leader/help and a narrow layout.
- Credential entry, persistent execution after restart, session-only behavior
  with the service locked/unavailable, and retained history after restart.
- Production confirmation before each actual send and redirect.

## Real backend gate

Outstanding. The developer must supply/configure the intended Swagger URL,
staging environment and credentials through the TUI. Do not paste values into
CLI input files, reports or this document. Choose harmless backend operations
and obtain any needed permission before sending writes.

1. Import the backend Swagger UI URL. Record the selected raw URL or fallback,
   omitting authentication data.
2. Search by partial path and description. Inspect the contract and configure
   staging's effective module URL and protected credentials in the TUI.
3. Send an approved staging request, save it as a case and rerun it. Compare two
   run IDs and save an ignore rule for an incidental field.
4. Create login and API cases with assertions. Declare secret extraction and
   exact consumer destination/placement; approve that definition in the TUI.
5. Author/read/validate the scenario using CLI schemas and previews. Run it
   with explicit `--env staging`, then inspect its aggregate and step history.
6. Verify matching TUI/CLI requests and sanitized output. Cause a harmless
   assertion failure and verify useful per-step reporting and exit 6.
7. Verify unapproved production and foreign destinations exit 4 without contact.
   Change staging's destination and verify the old grant fails. Review and
   authorize the new settings in the TUI before executing again.
8. Preview a contract refresh, retain saved inputs, and demonstrate a local-edit
   conflict plus deliberate repair or reload using the README recovery guide.
9. Reopen local history and repeat terminal/credential checks on Linux and macOS.

Record date, OS/architecture, commit, sanitized backend identity, run IDs,
commands/actions, expected/observed results and remaining failures. Never attach
raw login responses or credential values. Passing fixtures cannot close this gate.

## Known limits

Response schema validation implements a subset; unsupported constructs and
unavailable full bodies are incomplete, never passing full-schema evidence.
Static Swagger discovery cannot execute JavaScript. Redaction protects registered
values and recognized auth positions; unknown arbitrary business fields cannot
be identified as secrets automatically. External tools bypass the project lock.
Local-data storage is sanitized, not encrypted. A pinned aggregate may exceed
retention limits; linked step records need their own pins if independently needed.
