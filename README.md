# Reqscope

Reqscope is a local API workspace with a keyboard-driven TUI and a noninteractive CLI. It initializes and reopens projects, imports local and remote Swagger/OpenAPI specifications, discovers Swagger UI sources, inspects endpoints and schemas, and sends HTTP requests with protected credentials. It records sanitized runs, executes secure scenarios, compares responses and refreshes imported contracts.

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

Project exports contain no credentials, grants or responses. OS credential storage holds persistent secret values; the OS user data directory holds local access metadata and SQLite history, scoped to project ID and canonical path. Session-only credentials remain in memory.

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

Preview resolves references and checks the supported structure without persisting resources. Applying an import atomically records source/module UUIDs in the manifest and stores the original document bytes, resolved import base, digest, operations and schemas in `specs/SOURCE_UUID.yaml`. Unknown extensions remain intact. Endpoint IDs derive from source UUID, method and path, independently of operationId. Each import creates a new source. Use refresh to update an existing source while retaining its IDs and saved inputs.

Inspection includes inherited path parameters, operation inputs/responses, descriptions, tags and server/security context. `diagnostics` reports unsupported constructs; `complete` remains false because contract support is deliberately partial. Response validation supports the documented subset and marks unsupported checks incomplete. A successful import means the contract can be inspected, not that its response constraints were fully validated.

## Swagger UI and raw URL imports

Open `Space s i`, paste a Swagger UI or raw JSON/YAML URL, and press Enter.
Review the source origin, then press Enter to approve fetching. If discovery
finds several specifications, use j/k and Enter to choose one. Review the
contract preview and press Enter to apply it. Esc cancels without importing.
Approval stays in the local user data directory for this project and origin.
It is not copied with project files.

```sh
bin/reqscope spec import --url https://api.example.com/docs/ --select Pets --project ./my-api --preview --output json
bin/reqscope spec import --url https://api.example.com/openapi.yaml --project ./my-api --output json
```

CLI URL imports, including previews, require an existing source-origin approval
or an applicable approved environment/module destination supplied through
`--env` and `--module`. Project validation and inspection never fetch URLs.
The TUI approves only the entered origin. If an initializer, contract or remote
reference uses another origin, approve that origin through the import form and
retry the original URL. Discovery errors include visited resources and any
candidates found so far. If configuration depends on executing JavaScript,
supply the raw specification URL instead.

Discovery reads static `url`, `urls`, `spec` and `configUrl` declarations in
pages, referenced scripts and configuration resources. Relative URLs resolve
against their containing resource. JavaScript is never executed. The standard
Swagger UI runtime bundles are skipped. Discovery is limited to eight resource
levels, 64 fetched resources and 10 MiB per operation. Contract references
retain the existing 64-level and 100,000-value limits. Redirects report the
sanitized final destination and require its raw URL explicitly. Missing
resources, denied origins, cycles and oversized documents reject the import.

Source URLs must omit user info, fragments and authentication query parameters.
Ordinary query parameters such as `version=v1` remain in source locations and
refresh metadata. Authentication uses protected bindings.
Configure protected credentials for each exact resource URL, using the existing
environment credential form. Select that environment in the TUI before import,
or provide `--env ENV --module MODULE` in the CLI. Matching header/query bindings
are used automatically. Authenticated CLI fetches also require current
environment access approval. An optional `--input FILE` with `--url` supplies
explicit `headers` and `query` lists containing opaque credential references:

```yaml
headers:
  - name: Authorization
    value: '{{secret:swagger-token}}'
```

Import responses containing registered credential values are rejected before
storage. Successful imports preserve the original documents, source location,
resolved base and digest through the same atomic contract import path as files.

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
| 4 | Access or credential denial |
| 5 | Transport failure |
| 6 | Failed assertion/validation or unsuccessful scenario |
| 7 | Storage or internal failure |
| 130 | Cancellation |

Execution precedence follows the specification: unsuccessful scenarios use 6 with per-step causes, ad hoc transport failures use 5, and pre-execution access denial uses 4. Project validation does not authorize execution.

## TUI keys

The project pane supports editing and saving the project name. Environment templates are inspectable. Endpoint and schema panes list imported resources. Enter opens their full contract details, which scroll with j/k. Request and response panes support composing and sending requests. Case and scenario panes support saved workflows. The history pane lists retained runs and scenario reports. Enter reopens the selected evidence.

- `j`/`k`, arrows, `gg`/`G`, counts such as `2j`, and `Ctrl-d`/`Ctrl-u` navigate.
- `Ctrl-w h/j/k/l` switches panes; Tab advances; `Ctrl-w o` zooms.
- `ge`, `gq`, `gr`, `gh` jump to endpoint, request, response and history panes.
- `/` searches the focused pane; `n`/`N` advances filtered matches.
- `i` or Enter edits the project name. Esc leaves insert mode and retains edits.
- `Space p i/v/n/s/r` inspects, validates, renames, saves or reloads the project.
- `Space s i` opens file or URL import. Enter previews, a second Enter applies, and Esc cancels. `Space s s` opens schemas.
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

Endpoint search uses case-insensitive substring and fuzzy subsequence matching across paths, operation IDs, summaries, descriptions and tags. Path matches rank first; ties use path, method and immutable source/module/endpoint IDs. `endpoint list` accepts the same query and filters.

```sh
bin/reqscope endpoint search --project ./my-api --query pets --method GET --limit 20 --output json
bin/reqscope endpoint search --project ./my-api --tag pets --group-by tag --offset 20 --limit 20 --output json
```

Search returns `items`, `total`, `empty` and `nextOffset`. Pass a non-null `nextOffset` as the next `--offset`, keeping the query and filters unchanged. The default page size is 100, with a maximum of 1000. Pagination describes the current workspace; imports between pages may change results. Grouping includes an operation in each distinct tag group while retaining its endpoint ID.

In the TUI, `Space /` searches all endpoints as you type. `/` filters the focused pane and retains the global endpoint query. `Space s m` cycles the method filter; `Space s t` toggles tag grouping. `n` and `N` navigate matches, and Enter opens the selected contract with source and module context. These actions also appear in `?` help and the command palette. Narrow terminals show one focused pane; pane switches and selection work as in the full layout.

Environment settings and CLI access
-----------------------------------

Environment IDs stay fixed when names change. Use `environment create` or
`environment update` with the JSON/YAML input schemas to author base URLs and
ordinary variables. Base URLs support `{{variable}}` references from the
selected environment. Undefined or cyclic references fail before authorization.

In the TUI, Space e l opens environments, Space e e edits the selected environment
as JSON, and Space e c creates one. Ctrl+u clears the editor; Enter saves and Esc
cancels. Space e s selects the active environment. Space e a shows the resolved
base URLs, exact origins, production classification and binding fingerprint;
press y to authorize CLI access. Changing these settings invalidates approval.
Lowering production classification requires a separate TUI confirmation.

`environment check-access --env staging --module api [--url URL]` checks the
shared access policy without sending a request. Every future CLI network runner
must use this same check with an explicit environment and recheck overrides and
redirects before contacting them. The request runner uses this policy.
A grant permits exact origins, with normalized hosts and effective ports.
Production access is denied until deliberately approved through the TUI.

Approvals, protected classification and TUI selection live under the OS user
data directory, outside project files, scoped to project ID and canonical local
path. Copying a project does not copy its approval. Renaming an environment keeps
its approval if the effective settings are unchanged. Restart restores only a
nonproduction selection, otherwise a configured nonproduction environment or
the environment chooser. Protected credential bindings participate in the
fingerprint. Credential values and bindings are managed through the TUI.


Sending HTTP requests
---------------------

Send an explicit JSON/YAML draft with an approved environment:

```sh
bin/reqscope request send --project ./my-api --env staging --input request.json --output json
```

```json
{
  "module": "api",
  "method": "POST",
  "url": "/items",
  "query": [{"name": "tag", "value": "one"}, {"name": "tag", "value": "two"}],
  "headers": [{"name": "X-Trace", "value": "demo"}],
  "bodyType": "json",
  "body": "{\"name\":\"edited\"}"
}
```

For a contract request, supply `endpointId` from `endpoint list`. Its module and
method come from the contract, and an omitted URL uses the contract path. `path`
is an object of parameter names and string values. Query/header entries are
ordered lists that preserve repeated values. Relative request paths append to
the configured module base path. An absolute `url`, including a chosen contract
server URL, must have the configured module origin. CLI `--module`, `--url` and
`--method` override draft fields, with contract method/module taking precedence.

Supported contract parameter rules are scalar and array form query parameters,
space/pipe-delimited query arrays, simple path/header parameters, and Swagger 2
collection formats. Supply array elements as repeated query/header entries;
path arrays use a serialized string. Unsupported object parameters, cookie
parameters, label/matrix styles and unsupported form encodings fail before connecting.
Required parameters, scalar types/enums, required body properties and basic JSON
body types are checked. This is not full JSON Schema validation. Examples remain
separate suggestions and never regenerate body content. JSON, raw string/file, URL-encoded form and multipart bodies are supported;
optional `contentType` sets the media type.

Environment `{{variables}}` expand in URL, path values, query/header values and
body. Undefined variables fail before connecting. `timeoutMs`, `maxRedirects`
and `maxBodyBytes` configure limits per draft. Defaults are 30 seconds, five
redirects and 10 MiB of retained response. TLS certificates are verified.
Timeouts are bounded at one day and retained bodies at 1 GiB. There are no
automatic retries. Authorization, Proxy-Authorization and Cookie headers require
protected credential references. Literal authentication headers are rejected.

In the TUI, select an environment with Space e s. Space r c composes the selected
endpoint and shows required inputs, descriptions and example suggestions.
Space r n creates an explicit draft. Space r e edits the retained draft as JSON;
Ctrl+u clears the editor, Enter or Esc retains the buffer. Space r x clears the
draft to allow composing another endpoint. Space r s sends; every production
send requires y confirmation with method and resolved URL without query values.
Esc cancels confirmation or an active request. Draft edits and destination
changes invalidate production confirmation. Space r k also cancels transport.

Responses show status, duration, headers and body. Use / to filter response
lines, normal motions to scroll, and Space r y to copy the retained body through
the terminal clipboard. Binary bodies show metadata and cannot be searched or
copied as text. Truncated bodies are labelled incomplete for full-body
validation. Clipboard support depends on the terminal's OSC52 support.

HTTP 4xx/5xx statuses are completed exchanges and return exit 0. Input errors
return 2, missing resources 3, access denials 4, transport/TLS/timeout/redirect
limit failures 5, storage failures 7, and cancellation 130. Ctrl+C cancels a CLI
request. JSON output uses the existing versioned envelope, with status, timing,
headers, retained byte count and binary/truncation flags in `data`.


Protected credentials
---------------------

Select the active environment, then use `Space e k` to add or replace a named
credential. Tab moves between fields and Enter advances or saves. Value entry
is masked, and stored values cannot be revealed. Basic credentials use
`username:password`; bearer credentials contain the token alone. Bearer and
basic credentials attach to Authorization. API keys may bind to an exact header,
query field or the body. Bindings include the module, exact origin and exact
URL path. A redirect to a different path requires a different binding and is
rejected before sending credentials. Fields and paths do not use wildcards.

Use `Space e i` for names and configured state, and `Space e d` to delete a
credential by name with confirmation. Adding, replacing or deleting a credential
removes the environment's CLI approval. Use `Space e a` to review and approve
access again. These actions also appear in help and the command palette.

Credential values use Linux Secret Service over D-Bus or the macOS Keychain.
If the OS store is unavailable or locked, the TUI explains the failure and offers
an explicit session-only choice. There is no plaintext fallback and no automatic
installation or service configuration. Session values disappear when the process
exits and cannot be used by a separate CLI process. Missing values require TUI
replacement; ordinary project variables do not become protected credentials.

Request drafts contain opaque references, for example:

```json
{"module":"api","method":"GET","url":"/items","headers":[{"name":"Authorization","value":"{{secret:login}}"}]}
```

Header and query values must consist of the reference alone. Body references
can appear inside a raw or JSON body; JSON values retain correct escaping.
Use the explicit query list for query credentials. References may pass through
ordinary environment variables, but the runner still checks the effective
placement and destination. References cannot be used in destination hosts or
paths, field names, or control fields. The runner expands them only after
checking the current grant and protected binding. Absolute URLs, CLI overrides
and redirects receive the same checks. Cross-origin redirects strip sensitive
headers and are rejected before contacting an origin outside the grant.

```sh
bin/reqscope secret list --project ./my-api --env staging --output json
bin/reqscope secret inspect --project ./my-api --env staging --output json
bin/reqscope access inspect --project ./my-api --env staging --output json
```

These commands expose names, configured state and grant status. They cannot
retrieve or change values, bindings or approvals. An unavailable OS store can
still be inspected for missing state; other project output fails closed until
the store is available, so unknown stored values cannot leak through inspection.

Registered values, their JSON/URL representations and basic encoding are
redacted before runner results, errors, CLI previews, TUI rendering and response
copying. Response authentication headers and common JSON/XML authentication fields
are redacted even for unregistered server-generated values. Binary response
bodies expose metadata only. Invalid or incomplete structured JSON bodies are
withheld when they cannot be safely sanitized. XML elements and attributes,
plain authentication assignments and form-encoded authentication fields
are redacted as well. Arbitrary transformations or encodings performed by
a server can evade redaction; this is not guaranteed secret detection.

Only ordinary project files and credential reference names belong in Git or
project exports. Values reside in the OS store, and protected binding metadata
and approvals reside in the user data directory, keyed by project identity and
canonical local path. Copying project files does not copy credentials or grants.
Reqscope's CLI restrictions are an application boundary: processes with shell
access under the same OS account may access the OS store, edit local approvals
or inspect process memory. Keep that account and the OS credential service
protected. Automated tests use a controlled store at the OS boundary; macOS
cross-builds do not substitute for live Keychain acceptance on a Mac.

Saved request cases
-------------------

A case stores a UUID, display name, request templates, ordinary `variables`, and
optional `validation` settings. Use an `endpointId` to retain the imported
endpoint relationship, or supply an independent module/method/URL request.
Credential inputs stay as opaque `{{secret:NAME}}` references. Saving never
expands variables or credentials.

```sh
bin/reqscope case create --project ./my-api --input case.json --preview
bin/reqscope case create --project ./my-api --input case.json
bin/reqscope case list --project ./my-api
bin/reqscope case get CASE_ID --project ./my-api
bin/reqscope case update --project ./my-api --input edited-case.json
bin/reqscope case duplicate CASE_ID --project ./my-api --input duplicate.json
bin/reqscope request send --project ./my-api --env staging --case CASE_ID
bin/reqscope request send --project ./my-api --env staging --case CASE_ID --input overrides.json
bin/reqscope case delete CASE_ID --project ./my-api --input deletion.json --preview
```

`case.json` contains `id`, `name`, and `request`. Updates also require the
`revision` returned by `case get`. Duplicate input is `{"name":"Copy name"}`;
delete input is `{"revision":"CURRENT_DIGEST"}`. Overrides input is
`{"variables":{"item":"run-only value"}}`. All structured inputs accept
JSON/YAML from files or `--input -`. Names resolve only when unique; UUIDs always
select their resource. `input-schema list|get|validate` covers case write and
run inputs. Write previews perform validation without saving or sending.
Deletion lists referencing scenario files and refuses to leave dangling
`caseId` references. Scenario authoring validates case references when resources are saved.

Request variables resolve from project manifest `variables`, environment
variables, case/draft variables, then explicit run overrides. Undefined or cyclic
variables fail before transport. Run overrides never update saved inputs.
Saved runs include `caseId`; explicit drafts and dirty TUI sends report
`draft: true`. Optional validation settings are retained, but running a case
with them currently fails before sending until validation execution is added.

In the TUI, `<leader> c l` lists cases, `c o` opens the selection, `c r` runs it,
`c d` duplicates it, and `c x` requests deletion. `<leader> c s` saves the visible
request, opening the case JSON editor for a new case; `c e` edits case metadata
and inputs. Enter saves the case editor. Leaving or quitting with dirty inputs
prompts to save, discard, or cancel. Saves check the original revision; a
conflict preserves the buffer for reconciliation. Unsaved sends use the visible
draft and keep the saved case unchanged.

### Form bodies and local uploads

Use `bodyType: "form"` for URL-encoded values or `bodyType: "multipart"` for
values and files. The `form` array preserves repeated field names. Each entry
has `name` and either `value` or `file`, with an optional multipart `contentType`.
For example, save this input as `upload.json`:

```json
{
  "module": "api",
  "method": "POST",
  "url": "/upload",
  "bodyType": "multipart",
  "form": [
    {"name": "tag", "value": "one"},
    {"name": "tag", "value": "two"},
    {"name": "attachment", "file": "fixtures/sample.bin", "contentType": "application/octet-stream"}
  ]
}
```

Send it with `reqscope request send --project ./my-api --env staging --input upload.json`.
Relative upload paths resolve from the project root. Absolute paths are also
accepted. Paths must be explicit, without variable or credential references.
Saved cases retain paths and never store uploaded bytes. Raw binary bodies use
`bodyType: "raw"` and `bodyFile` instead of `body`.

In the TUI, compose a contract request or edit the JSON draft, then press
`Space r f` to choose a file reference. Enter the field name, use `j/k` to move,
`Enter` to open a directory or select a file, and `h` to visit the parent directory.
Raw bodies use the selected file as `bodyFile`. Multipart bodies add the reference
to `form`. JSON draft editing also supports repeated file entries.

OpenAPI form schemas guide required fields and file references. Supported array
encodings include `form` with `explode` and Swagger `collectionFormat` values
`multi`, `csv`, `ssv`, `pipes`, and `tsv`. Nested objects and unsupported encoding
rules fail before sending. Missing, unreadable, or nonregular files fail before
network access. Upload bytes are read into memory with a 32 MiB combined limit.

The runner generates multipart boundaries and preserves file bytes. Production
confirmation includes a digest of those bytes, so changing a file requires a new
confirmation. Protected body credential references expand in form values before
encoding and follow the existing destination bindings. File bytes are opaque and
never expanded as templates. Response output uses the existing sanitizer.

## External body editor

Use `Space r o` or the `request-editor` command to edit the current inline raw
or JSON body. Reqscope uses `$VISUAL`, then `$EDITOR`, then `vi`. Editor settings
may include arguments, such as `code --wait`. The editor must wait until editing
finishes before it exits.

The scratch file contains only the unexpanded body, including variable and
secret references. Registered credential values are rejected before export.
Reqscope creates a private temporary directory and a file readable only by its
owner, and removes the directory after the editor returns, including failures.
The current case, draft and mode remain selected. A nonzero editor exit,
including cancellation, retains the previous buffer. Exiting without changes
also retains it. Successful edits become unsaved draft changes. Invalid JSON
remains editable with a diagnostic, and sending revalidates all request inputs.
File-backed bodies and structured forms use the existing request and upload
editors instead.

Saved cases can include response validation, authored with `case create` or
`case update` JSON/YAML input, or the TUI's saved case JSON editor:

```json
"validation": {
  "assertions": [
    {"kind": "status-equals", "value": 200},
    {"kind": "status-range", "min": 200, "max": 299},
    {"kind": "header-present", "name": "content-type"},
    {"kind": "header-equals", "name": "x-result", "value": "ok"},
    {"kind": "json-present", "pointer": "/items/0/id"},
    {"kind": "json-equals", "pointer": "/active", "value": true}
  ],
  "responseSchema": true
}
```

JSON Pointers use RFC 6901 escaping and support an empty pointer for the whole
body. Equality preserves JSON types and distinguishes missing fields from null.
Header names ignore case; equality checks each repeated header value.
`responseSchema` selects the endpoint's exact status response, status range,
or default response, then its response media type. It supports JSON types,
nullable values, required properties, nested properties and array items, enum,
const, boolean schemas and additional properties. Other constraints, including
unresolved references, produce an explicit incomplete result.

Run with `request send --case CASE --env ENV --output json`. Results include
`transport` and per-check `checks` with `passed`, `failed` or `incomplete`
outcomes. HTTP 4xx/5xx are completed exchanges. Failed or incomplete checks
exit with code 6, transport failures with 5, and invalid assertions with 2.
Truncated or unavailable bodies cannot pass JSON or response schema checks.
Check diagnostics omit actual and expected values to protect secrets.

Sequential scenarios
--------------------

A scenario contains a UUID `id`, a display `name`, and ordered `steps`. Each
step has a unique `id` and references a saved case by UUID `caseId`.
`overrides` supplies ordinary variables for that step. `extracts` maps names
to JSON Pointers into its response. Later steps use `bindings` to explicitly
identify a producer step and one of its named extracts:

```json
{
  "id": "00000000-0000-4000-8000-000000000012",
  "name": "Create and inspect",
  "steps": [
    {
      "id": "create",
      "caseId": "00000000-0000-4000-8000-000000000001",
      "overrides": {"name": "Example"},
      "extracts": {"itemId": "/id"}
    },
    {
      "id": "inspect",
      "caseId": "00000000-0000-4000-8000-000000000002",
      "bindings": {"item": {"stepId": "create", "extract": "itemId"}}
    }
  ]
}
```

The inspect case can use `{{item}}` in its request. JSON strings become string
inputs; other JSON values retain their JSON representation, including `null`
and exact numbers. Extraction requires a complete, retained JSON response.
Authentication fields are sanitized and cannot be passed as ordinary extracts.
Extracts remain local to the run. Nested scenarios, loops and scripts are
unsupported and rejected as unknown fields.

```sh
bin/reqscope scenario create --project ./my-api --input scenario.json --preview
bin/reqscope scenario create --project ./my-api --input scenario.json
bin/reqscope scenario list --project ./my-api
bin/reqscope scenario get SCENARIO_ID --project ./my-api
bin/reqscope scenario update --project ./my-api --input edited-scenario.json
bin/reqscope scenario validate SCENARIO_ID --project ./my-api
bin/reqscope scenario run SCENARIO_ID --project ./my-api --env staging --output json
bin/reqscope scenario delete SCENARIO_ID --project ./my-api --input deletion.json
```

Updates and deletion require the `revision` from `scenario get`. Create,
update and delete support `--preview`; run does not. An optional run input
uses `{"variables":{"name":"value"}}`. `input-schema list|get|validate`
includes `scenario.create`, `scenario.update`, `scenario.delete` and
`scenario.run`. Validation checks references without sending or authorizing.

All cases and contracts are snapshotted before execution. Overrides never
change saved cases. Variable precedence is project defaults, environment,
case, run input, step overrides, then bound extracts. Access and credential
checks remain live. `case get` and case write previews identify affected
scenario files; deletion refuses referenced cases.

The default policy stops after a transport, assertion or extraction failure,
or an incomplete mandatory check. Set `continueOnFailure: true` to run later
independent steps. Steps depending on a failed or skipped producer are skipped
with a reason. Every step is mandatory: any failure or skip keeps the report
unsuccessful. Reports include sanitized exchanges, per-step causes, case
revisions and contract digests. Unsuccessful scenarios exit 6, including
transport failures inside a scenario; access denial before any send exits 4,
and cancellation exits 130. Ctrl+C cancels active transport and further steps.

In the TUI, `<leader> s l` lists scenarios, `s n` creates one, `s e` edits the
selected scenario JSON, `s v` validates, `s r` runs, `s x` deletes, and `s k`
cancels. Enter saves the editor; Esc cancels editing. Production requires a
confirmation showing method and sanitized URL before each step send. Esc at
that prompt cancels the whole scenario. A shared-case save shows affected
scenarios and asks for confirmation before writing.

Secure login chaining
--------------------

Use `secretExtracts` for login tokens. For example, a login step can declare:

```json
"secretExtracts": {
  "session": {
    "pointer": "/data/session",
    "destinations": [
      {"kind":"bearer", "module":"api", "destination":"https://api.example.com/me", "in":"header", "field":"Authorization"}
    ]
  }
}
```

A later step binds its `auth` variable to `{"stepId":"login","extract":"session"}`
and sets its Authorization header to `{{auth}}`. The runner inserts the Bearer
prefix. API keys use `kind: "api-key"` and a header, query or body placement.
All destinations for one token use the same kind. Body placement authorizes
that exact destination's body and uses an empty `field`.

In the TUI, select the scenario and press `<leader> s a`. Review the JSON
Pointer, source cases, permitted exact destination URLs and placements, and
active environment. Press `y` to approve or Esc to cancel. CLI commands can
prepare candidate metadata and run approved scenarios, but cannot approve it.
Changing a scenario, source case, contract or environment settings requires
fresh TUI approval. CLI environment access still needs its separate approval.

Tokens are registered before response checks or output, passed downstream as
opaque references, and discarded as usable credentials when the run ends.
They are never saved to the OS credential store or environment variables.
Redaction knowledge remains in memory for later output in the same process.
Approved pointers remain protected throughout their environment, including
ordinary scenarios and direct requests. This conservatively hides matching
fields in other responses too. Missing, non-string, empty or incomplete token
extracts fail the producer and prevent dependent sends.

Run history
-----------

Every request send records a sanitized result in local SQLite, including failed
sends. Saved-case runs carry their case ID; ad hoc runs receive standalone IDs.
History is stored beside local access state in a separate `reqscope/history`
directory under the OS user data directory, keyed by project ID and location.
Project files and project inspection output exclude history and credentials.

```
reqscope history list --limit 50 --offset 0
reqscope history list --case CASE_ID_OR_NAME
reqscope history get RUN_ID
reqscope history pin RUN_ID
reqscope history unpin RUN_ID
reqscope history delete RUN_ID
reqscope history associate RUN_ID --case CASE_ID_OR_NAME
reqscope history prune
reqscope history settings
reqscope history settings --input limits.json
```

Settings input requires `{"perCase":100,"maxBytes":104857600}`. Defaults keep
100 unpinned runs per saved case and 100 MiB of unpinned evidence per project.
Cleanup runs after sends, settings changes and unpinning. It evicts oldest
unpinned runs first. Pins survive cleanup, cannot be explicitly deleted until
unpinned, and report their total size and whether it exceeds the project limit.
Zero limits remove all applicable unpinned runs. Limits count serialized evidence
bytes; SQLite indexes and page overhead add disk space beyond that amount.

List output omits bodies and response details; get limits each body preview to
4096 bytes and bounds header and check previews. The TUI reopens retained results
with Enter from `g h`. History pages contain 50 entries. Leader actions under
`h` are `n` next page, `b` previous page, `p` pin, `u` unpin, `x` delete,
`c` cleanup, `s` edit retention JSON and `a` associate with the current saved case.
Request body evidence is capped at 1 MiB. Uploads and bodies that cannot be safely
reopened retain metadata. Binary and incomplete unsafe bodies are withheld.

### Compare retained responses

Compare two retained run IDs belonging to the same saved case, including runs
from different environments:

```sh
reqscope compare BEFORE_RUN_ID AFTER_RUN_ID --output json
reqscope compare BEFORE_RUN_ID AFTER_RUN_ID --input ignores.json --output json
```

The result labels environment, input and contract digest changes and compares
status, case-insensitive header names and JSON values. JSON changes use escaped
JSON Pointer paths and `added`, `removed` or `changed` kinds. Objects ignore key
order, arrays retain index order, and null differs from a missing field. Numeric
values compare without losing integer precision. Text changes show removed and
added lines with one-based line paths. The common prefix and suffix are omitted;
the changed middle is shown as a replacement hunk.

Save `comparisonIgnore` on a case through `case create`, revision-checked
`case update`, or the TUI saved-case JSON editor:

```json
{"jsonPointers":["/updatedAt","/items/0/requestId"],"headers":["Date","X-Request-Id"]}
```

This object is also the optional `--input` document. Supplied rules replace saved
rules for one comparison; `{}` disables all ignores for that invocation. JSON
Pointers match exact paths, including an entire value's children when that value
is selected. The empty pointer ignores the whole JSON body. Header ignores are
case-insensitive. Rules never change stored evidence and do not affect execution
or assertions. `input-schema get comparison.ignore` describes the input shape.

In the TUI, `Space h d` opens comparison with the selected history ID prefilled.
Enter two IDs separated by a space, use Ctrl+u to clear, then Enter to compare.
The report scrolls with j/k and closes with Esc. Saved case rules apply.

Comparisons expose up to 100 differences with 1024-byte value and path previews;
output clipping is explicitly reported. JSON traversal stops at depth 64. At most
100 ignore rules are accepted. Redacted or truncated evidence carries comparison
limitations. Binary, withheld and unavailable bodies compare retained metadata
only. These results cannot establish equality of original content. Missing or
evicted runs return `not_found`, CLI exit 3; unrelated or ad hoc runs return
`invalid_input`, exit 2. A successful comparison exits 0 even when differences
exist.

### Local contract edits

Use `endpoint create|update|delete` or `schema create|update|delete` with
`--input FILE|-`. Input schemas are available under the corresponding names,
for example `input-schema get schema.update`. Supply the `sourceId` and current
`revision` from `spec inspect`, and set `importedEdit: true` to explicitly edit
an imported contract. Endpoint edits target `path` and `method`; schema edits
use `name`. Create and update take a complete operation or schema in `value`.
The optional `document` selects a retained reference document instead of the
source's root document. Referenced path items must be edited in their own document.

Run with `--preview` to validate without writing and inspect `affectedCases`
and `affectedScenarios`. Deleting an endpoint used by cases requires an explicit
`reassignTo` endpoint ID in the same source. Schema references must be repaired
before deletion. Updates retain resource IDs; creating an operation at a new path
creates a new ID. Removed operations retain tombstones.

Original documents, imported digest and resolved import base stay unchanged.
Local authoritative documents are retained separately, including unknown
extensions and references. Edits cannot change an existing operation's destination
or authentication settings, and cannot fetch new references. Registered credential
values are rejected before persistence or export.

`spec export SOURCE_ID` returns `rootDocument` and a `documents` map containing
only authoritative OpenAPI document text. Write each document under its retained
key when sharing a local source bundle. Remote document keys retain their source
URLs. Cases and scenarios are separate project resources and are not serialized
into ordinary OpenAPI. Credentials, approvals and history are excluded.

In the TUI, `Space d a` opens the local contract JSON editor. Set the edit intent,
choose `create`, `update` or `delete`, and press Enter to preview affected cases
and scenarios. Enter applies; Esc returns to editing. The selected endpoint or
schema chooses the source, or the first source is used from other panes.
`Space d o` copies the contract document bundle to the terminal clipboard and
opens an inspection view.

### Refresh and concurrent edits

Refresh an existing source explicitly. Preview re-reads its original local file
or approved remote URL and compares the imported base, local contract edits and
incoming documents.

```sh
bin/reqscope spec refresh-preview SOURCE_ID --project ./my-api --output json
bin/reqscope input-schema get spec.refresh-apply --output json
# Save the returned token as {"previewToken":"..."} in refresh.json.
bin/reqscope spec refresh-apply SOURCE_ID --input refresh.json --project ./my-api --output json
```

Preview lists added, changed and removed endpoints, affected cases, conflicting
document paths and input diagnostics. Apply fetches again and rejects a missing
or stale token. Local contract conflicts must be reconciled before a new preview.
Network refresh uses the import origin approvals and optional `--env` and
`--module` credential context. Refresh never changes environment destinations or
credential bindings. Contract destination or security changes revoke affected
CLI approvals before the transaction is applied.

Removed operations retain endpoint tombstones. Their cases keep saved inputs and
receive `request.disconnected: true`. To repair a case, explicitly update its
`endpointId` and clear `disconnected`, or clear `endpointId`, supply the independent
request's module, method and URL, and clear `disconnected`. A returning backend
operation does not reconnect a case automatically. Case updates require the
current revision as usual. Contract and case changes share a recoverable transaction.

In the TUI, `Space d r` previews the selected source, Enter applies and Esc cancels.
Clean external case changes reload during polling. Dirty buffers remain available;
`Space c z` discards the case draft and reloads, while `Space c m` displays the
external case for manual comparison. Enter accepts that revision while retaining
local inputs for further editing and saving. Invalid external files show errors
and retain the last valid view. Unsaved project-name edits survive contract refresh.

## Installation and release acceptance

Build from a checkout with Go 1.26 or newer and a C toolchain for race-enabled
tests. Run `make build`, then install the native binary into a directory on PATH:

```sh
install -m 755 bin/reqscope "$HOME/.local/bin/reqscope"
```

Create that directory first if needed. Build on the target OS for a native
Linux or macOS binary. `make check` builds and vets every package; `make test`
runs the full suite with the race detector. The
[acceptance workflow](.github/workflows/acceptance.yml) runs those checks on
Linux and macOS and retains each native binary as an artifact.

Ordinary variables are scoped to an explicit environment. `variables list` and
`variables get NAME` return the enclosing environment revision. Create/update
inputs contain `name`, string `value` and that `revision`; deletion takes the
variable name and an input containing `revision`. Empty string values are valid.
Use the corresponding `input-schema get variables.create|update|delete` schema.
These commands edit the same settings as the TUI environment JSON editor.

```sh
reqscope variables list --env staging --project ./my-api --output json
reqscope variables create --env staging --input variable.json --project ./my-api --preview --output json
reqscope variables update --env staging --input variable.json --project ./my-api --output json
reqscope variables delete temporary --env staging --input revision.json --project ./my-api --output json
reqscope environment delete obsolete --input revision.json --project ./my-api --preview --output json
```

`Space e x` confirms deletion of the selected environment. Deletion revokes its
CLI grant and extraction approvals, clears its local selection, and retains OS
credentials and production protection. Recreating the UUID requires fresh access
approval and cannot lower production classification through CLI authoring.
Cases and scenarios select environments at execution time and do not hold
references to a deleted environment.

Scenario execution records an aggregate `runId` plus individual records linked
by `scenarioRunId` and `stepId`. Reports retain failures and skipped-step reasons.
Pre-execution access/extraction denial returns exit 4 without starting a scenario
history entry. A run that starts and then fails retains its report. `history get`
returns at most 50 step summaries with `totalSteps` and `stepsTruncated`; inspect
individual step run IDs for bounded exchange previews. The TUI history pane
reopens the full aggregate. Pin a report and any step baselines you need to keep:
step records and aggregates have independent retention, so pinning a report does
not prevent its linked exchanges from being evicted. The report itself retains
sanitized step exchanges. Aggregate reports cannot be associated with a case or
compared as though they were case exchanges.

Local fixtures do not certify a real backend or an OS credential service.
See [release acceptance evidence and outstanding gates](docs/release-acceptance.md)
for all 54 story mappings, the platform check and the manual backend procedure.
