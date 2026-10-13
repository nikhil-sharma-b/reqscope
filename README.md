# Reqscope

Reqscope is a local API workspace with a keyboard-driven TUI and a noninteractive CLI. It initializes and reopens projects, imports local Swagger/OpenAPI specifications, and inspects endpoints and schemas. Sending HTTP requests, credentials and history are covered by later tickets.

Requires Go 1.26 or newer. Linux and macOS are the supported targets.

```sh
make build
bin/reqscope project init "My API" --project ./my-api
bin/reqscope project inspect --project ./my-api --output json
bin/reqscope tui --project ./my-api
```

With no arguments, Reqscope opens the TUI only when both stdin and stdout are terminals. Otherwise it prints usage and exits. Starting the TUI without a discovered project opens a path/name initialization form. Tab switches fields, Enter initializes, and Esc quits. Explicit `tui` refuses nonterminal streams and JSON output.

`--project` accepts a directory or its `reqscope.yaml`. Without it, CLI commands look for `reqscope.yaml` in the current directory and its ancestors. They never reopen a previously used project automatically.

## Project files

`reqscope.yaml` contains format version 1, an immutable UUID, name, module/source metadata and resource locations. The project creates `specs/`, `environments/`, `cases/` and `scenarios/`. Environment templates are individual YAML files named by UUID. Local, staging and production start with empty base URLs and variables. Production has the production classification. No environment is authorized for CLI network use.

IDs are independent of names. Names must resolve uniquely; use a UUID when duplicate names exist. Inspection returns each resource's SHA-256 content revision. Unsupported versions, malformed files and unknown fields fail with diagnostics.

Add these patterns to a project's `.gitignore`:

```gitignore
.reqscope.lock
.reqscope-transaction.json
.reqscope-write-*
```

All application readers and writers take the same OS project lock. Writes validate the complete batch and check expected revisions before publishing a durable journal. Each resource is written through a synced temporary file and atomic rename. A reader first replays an interrupted journal under the lock, then loads the project. External edits conflicting with recovery block reopening and report the file to reconcile. External tools reading files directly do not participate in the application lock.

The project files contain no credentials, grants or responses. Their local storage arrives in later tickets; this implementation does not create those stores.

## Local specification imports

```sh
bin/reqscope spec import --input ./api/openapi.yaml --project ./my-api --preview --output json
bin/reqscope spec import --input ./api/openapi.yaml --project ./my-api --output json
bin/reqscope spec inspect --project ./my-api --output json
bin/reqscope endpoint list --project ./my-api --output json
bin/reqscope endpoint get ENDPOINT_UUID --project ./my-api --output json
bin/reqscope schema list --project ./my-api --output json
bin/reqscope schema get SCHEMA_UUID --project ./my-api --output json
```

Imports accept Swagger 2.0 and OpenAPI 3.0/3.1 in JSON or YAML. Select a file with `--input`; stdin imports are unavailable because local references need a directory root. References stay within the selected file's directory, reject symlinks, and never fetch remote URLs. Traversal is capped at 64 files, 64 levels, 100,000 visited values and 10 MiB total input. Missing references and cycles reject the import. Recursive schema contracts therefore require simplification before import.

Preview resolves references and checks the supported structure without persisting resources. Applying an import atomically records source/module UUIDs in the manifest and stores the original document bytes, resolved import base, digest, operations and schemas in `specs/SOURCE_UUID.yaml`. Unknown extensions remain intact. Endpoint IDs derive from source UUID, method and path, independently of operationId. Each import creates a new source; refresh is a later ticket.

Inspection includes inherited path parameters, operation inputs/responses, descriptions, tags and server/security context. `diagnostics` reports unsupported constructs; `complete` remains false because response schema validation is not implemented. A successful import means the contract can be inspected, not that its response constraints were fully validated.

## CLI authoring

```sh
bin/reqscope input-schema list --output json
bin/reqscope input-schema get project.update --output json
printf 'name: Proposed\n' | bin/reqscope project init --input - --project ./new-api --preview --output json
bin/reqscope project validate --project ./my-api --output json
bin/reqscope environment list --project ./my-api --output json
bin/reqscope environment get staging --project ./my-api --output json
```

Structured inputs accept one JSON or YAML document from `--input FILE` or `--input -`, capped at 1 MiB. `input-schema validate NAME --input FILE|-` validates without writing. `--preview` on writes validates and checks revisions without committing the proposed resource changes. Initialization preview does not create a directory.

`project.update` accepts `name` and the `revision` from project inspection:

```json
{"name":"Renamed API","revision":"<64-character digest from project inspect>"}
```

`environment.create` accepts `id`, `name`, `classification`, `baseURLs` and `variables`. Generate a UUID for `id`. Classification is `nonproduction` or `production`; the two objects contain string keys and string values. `environment.update` also requires the resource's `revision` from `environment get`. IDs cannot change. These commands author ordinary project settings and confer no authorization.

JSON stdout contains exactly one envelope with `schemaVersion`, `success`, `data`, `warnings` and `error`. Errors have stable `code` and `message` fields. Diagnostics go to stderr. Text output uses readable JSON data. Neither output mode prompts.

| Exit | Meaning |
| --- | --- |
| 0 | Success |
| 2 | Usage, input or schema validation |
| 3 | Missing or ambiguous resource, or revision conflict |
| 4 | Access or credential denial, reserved for access operations |
| 5 | Transport failure, reserved for execution |
| 6 | Failed assertion/validation or unsuccessful scenario, reserved for execution |
| 7 | Storage or internal failure |
| 130 | Cancellation |

Future execution precedence follows the specification: unsuccessful scenarios use 6 with per-step causes, ad hoc transport failures use 5, and pre-execution access denial uses 4. Project validation does not authorize execution.

## TUI keys

The project pane supports editing and saving the project name. Environment templates are inspectable. Endpoint and schema panes list imported resources. Enter opens their full contract details, which scroll with j/k. Request, response, case, scenario and history panes show empty states until their features arrive.

- `j`/`k`, arrows, `gg`/`G`, counts such as `2j`, and `Ctrl-d`/`Ctrl-u` navigate.
- `Ctrl-w h/j/k/l` switches panes; Tab advances; `Ctrl-w o` zooms.
- `ge`, `gq`, `gr`, `gh` jump to endpoint, request, response and history panes.
- `/` searches the focused pane; `n`/`N` advances filtered matches.
- `i` or Enter edits the project name. Esc leaves insert mode and retains edits.
- `Space p i/v/n/s/r` inspects, validates, renames, saves or reloads the project.
- `Space s i` opens local import. Enter previews, a second Enter applies, and Esc cancels. `Space s s` opens schemas.
- `Space e l` opens environment templates. `Space r i/r` opens request/response panes; `Space c l`, `Space s l` and `Space h l` open cases, scenarios and history. `Space /` opens endpoint search.
- `?` shows help; `:` or `Space :` opens the command palette. Arrow keys select palette commands; Enter runs one.
- `q` quits. Leaving a dirty project or quitting asks for save, discard or cancel.

Pending prefixes display a floating continuation menu; Esc cancels them. Help and the palette use the effective action registry, including remappings and unbound actions. Help scrolls with `j`/`k`. Narrow terminals show the focused pane. Resizing preserves focus, pending keys, selection and draft buffers. Clean project resources reload once per second; a changed project revision retains a dirty name and reports a conflict.

Key configuration is read from the OS config directory's `reqscope/keys.yaml`, or `--keys FILE` on `tui`. On Linux the usual path is `$XDG_CONFIG_HOME/reqscope/keys.yaml` or `~/.config/reqscope/keys.yaml`; on macOS it is `~/Library/Application Support/reqscope/keys.yaml`.

```yaml
leader: ","
bindings:
  help: ["H"]
  quit: ["q", "ctrl+c"]
  half-down: []
```

Bindings use space-separated sequences, such as `ctrl+w h`. `<leader>` expands to the configured leader. Empty lists unbind actions. Duplicate bindings, action/prefix conflicts, unknown actions and reserved Esc/count keys are rejected at startup. Navigation bindings are suspended during text entry.

## Development

```sh
make check
make test
```

Tests cover real temporary projects, revision races, interrupted transactions, public CLI processes, structured input, nonterminal invocation, and TUI events/rendering. See [dependency notes](docs/dependencies.md) for version choices.
