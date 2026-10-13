---
title: Reqscope v1
status: ready-for-agent
date: 2026-10-13
scope: specification only
---

# Reqscope v1

## Problem and solution

Developers receive Swagger UI links from backend teams and struggle to find endpoints and repeat debugging requests. Agents need access to the same project through a predictable CLI.

Reqscope is a local API workspace with a Vim-oriented TUI and a noninteractive CLI. Developers import a specification, search operations, configure environments, send requests, save cases, compare responses and run sequential tests. Both interfaces use the same domain model and runner. The initial release targets Linux and macOS.

The name combines request and scope: discover the request, inspect its result and compare executions. The executable is reqscope. This is a naming recommendation adopted for this document; package, repository and trademark availability have not been checked. Do not rename the existing workspace as part of specification delivery.

## Authority and evidence

The user confirmed the shared design after the grilling session and approved subsequent decisions on environment separation and Vim navigation. This document replaces the research note's tentative recommendations where they differ. [Research](../../docs/research/apidog.md) documents Apidog behavior and sources.

The repository currently contains only that research note. There is no existing application, domain glossary, ADR collection, test seam, Git repository or configured issue tracker found. This document is published at the skill's local fallback path and marked ready-for-agent. No tracker label has been applied remotely.

Reference implementation for interaction patterns is /home/nikhil-sharma-b/repos/jira-tui: internal/config/keys.go, internal/config/binding.go, internal/ui/dispatch.go, internal/ui/whichkey.go and internal/ui/help.go. Adapt its interaction principles; inspect dependencies and licensing before copying implementation. Its current stack is Go, Bubble Tea and Lip Gloss.

## Release boundaries

Included: independent product; local files shared through Git; Swagger 2.0 and OpenAPI 3.0/3.1 JSON/YAML import; Swagger UI discovery; HTTP requests; bearer/API key/basic authentication; login-token extraction; saved cases; declarative assertions; response validation; sequential scenarios; response history and comparisons; external editor; TUI and full CLI authoring, with explicit credential and authorization exceptions.

Out of scope:
- Existing Apidog account/project access, native project compatibility or full feature parity.
- Windows support, hosted collaboration, accounts, remote project storage or automatic Git operations.
- Mock servers, generated documentation sites, SDK/code generation.
- WebSocket, SSE, gRPC and dedicated GraphQL/SOAP tooling. Generic HTTP bodies may contain arbitrary content without protocol-specific tooling.
- OAuth browser flows, client certificates, persistent cookie sessions.
- Arbitrary scripts, Postman script compatibility, database steps, nested scenarios, branching, loops, data-row iteration, load tests and scheduling.
- Background automatic specification refresh.
- Full Vim text editing or embedded Neovim.
- Protection against agents using shell commands or other tools under the user's OS account.
- Guaranteed detection of secrets transformed or encoded by a server.
- Installing a credential service, changing desktop configuration or initializing Git as part of specification delivery.

## User stories

Every story belongs to v1 unless an explicit limitation follows in the contracts.

1. As a developer, I want to initialize and reopen a local project, so that my API work persists between sessions.
2. As a developer, I want to import a JSON or YAML specification file, so that I can explore my backend's API.
3. As a developer, I want to import a raw specification URL, so that I can use a backend-published contract.
4. As a developer, I want to paste a Swagger UI URL, so that I can start with the link my backend team provides.
5. As a developer, I want a clear fallback when discovery fails, so that I can supply the raw specification location.
6. As a developer, I want to choose among specifications exposed by a Swagger page, so that I use the intended service.
7. As a developer, I want actionable import errors, so that I can fix invalid files and unavailable references.
8. As a developer, I want to search paths, summaries, descriptions and tags while typing, so that I find endpoints quickly.
9. As a developer, I want method filters and tag grouping, so that I narrow similar results.
10. As a developer, I want endpoint descriptions and schemas beside request fields, so that I understand the expected inputs.
11. As a developer, I want separate local, staging and production environments, so that each destination uses the correct settings.
12. As a developer, I want to rename environments and add others, so that the project matches my deployment setup.
13. As a developer, I want to see the active environment and effective destination, so that I know where a request will go.
14. As a developer, I want to configure credentials in the TUI, so that authenticated requests work without exposing values through the CLI.
15. As a developer, I want guided editing of parameters, headers and bodies, so that I can construct a valid request.
16. As a developer, I want to open large request bodies in my editor, so that I can use familiar editing tools.
17. As a developer, I want form bodies and multipart uploads, so that I can test endpoints beyond JSON.
18. As a developer, I want request validation errors before sending, so that missing inputs do not waste calls.
19. As a developer, I want to cancel a running request, so that an unresponsive endpoint does not block my work.
20. As a developer, I want status, timing, headers and a searchable response body, so that I can diagnose behavior.
21. As a developer, I want to copy response values, so that I can reuse useful results.
22. As a developer, I want schema validation and assertions, so that I can check a response against its contract.
23. As a developer, I want to save, edit, duplicate and delete cases, so that repeated requests are easy to maintain.
24. As a developer, I want to assemble sequential cases into a scenario, so that I can test an authenticated workflow.
25. As a developer, I want to extract values between scenario steps, so that later requests use earlier results.
26. As a developer, I want failed dependencies to stop dependent requests, so that invalid state is not reused.
27. As a developer, I want explicit continuation for independent steps, so that one failure need not hide unrelated results.
28. As a developer, I want recorded runs and pinned baselines, so that useful debugging evidence survives cleanup.
29. As a developer, I want to compare any two runs of a case, so that changes in status, headers and body are visible.
30. As a developer, I want saved ignore rules for changing fields, so that incidental differences do not obscure regressions.
31. As a developer, I want a preview before refreshing the specification, so that I understand endpoint changes.
32. As a developer, I want saved inputs preserved through refresh, so that backend changes do not erase my work.
33. As a developer, I want disconnected cases retained when endpoints disappear, so that I can repair them deliberately.
34. As a developer, I want explicit local contract edits, so that I can explore a proposed API change.
35. As a developer, I want stale edits and external changes detected, so that human and agent work does not overwrite silently.
36. As a developer, I want Vim motions and pane navigation by default, so that I can work from the keyboard.
37. As a developer, I want a configurable leader and visible continuation menu, so that commands are discoverable.
38. As a developer, I want help generated from my actual bindings, so that remapped shortcuts remain understandable.
39. As a developer, I want normal and insert modes, so that navigation and typing have distinct behavior.
40. As a developer, I want production identified persistently and confirmed before each send, so that accidental calls are less likely.
41. As a developer, I want to authorize CLI environments and origins through the TUI, so that agents only call approved destinations.
42. As a developer, I want destination changes to invalidate authorization, so that approval cannot transfer to a different service.
43. As a developer, I want credentials redacted before output or history storage, so that ordinary inspection does not disclose them.
44. As an agent, I want to inspect and author contracts, cases, scenarios and ordinary environment settings, so that I can maintain a project without driving the TUI.
45. As an agent, I want input schemas and validation previews, so that I can prepare correct edits.
46. As an agent, I want stable IDs and conflict checks, so that my edits target the intended resources.
47. As an agent, I want authenticated execution without reading secrets, so that I can test approved systems.
48. As an agent, I want explicit environment selection and rejection of unauthorized destinations, so that runs cannot silently target production.
49. As an agent, I want structured results and distinct exit codes, so that I can distinguish transport failures from failed assertions.
50. As an agent, I want to inspect history and compare runs with bounded output, so that debugging fits automated workflows.
51. As an agent, I want missing inputs and unsupported features reported without prompts, so that unattended commands finish predictably.
52. As a developer, I want local state isolated from project exports, so that sharing project files does not share credentials or private responses.
53. As a developer, I want configurable history retention and explicit cleanup, so that local responses do not accumulate without bounds.
54. As a developer, I want readable behavior in narrow terminals and on Linux and macOS, so that navigation remains usable in my setup.

## Domain and architecture

Use Go, Bubble Tea and Lip Gloss. Choose supported library versions during implementation and record them in go.mod; the reference project's versions are facts, not a requirement to copy them.

One executable provides explicit tui and CLI subcommands. No arguments starts the TUI only when stdin/stdout are terminals; otherwise show usage without waiting for input. Invoking the TUI is human interaction, not a CLI bypass for unattended execution. All execution commands outside the interactive TUI use the restricted caller policy.

Module responsibilities:
- Project: versioned resource loading, schema validation, IDs, references, revisions and transactions.
- Specification: import, Swagger discovery, operation normalization, reference resolution, refresh comparisons and contract edits.
- Execution: variable resolution, request construction, HTTP transport, assertions, extraction and ordered scenarios.
- Access: local credential storage, protected bindings, environment authorization and caller policy.
- History: sanitized results, retention, pinned baselines and comparison.
- TUI and CLI: translate input into shared application operations and render outcomes.

All requests pass through the same execution and access modules. Rendering must never receive raw credential values. Access is evaluated on freshly loaded current settings immediately before execution. No UI-only permission check.

An endpoint is a contract operation. A case is concrete saved inputs plus optional validation rules against an endpoint. A scenario references cases with per-step input overrides; overrides never mutate the source case. A run is immutable execution evidence. An environment is deployment settings. A protected binding connects a named credential to permitted request positions and destinations. A CLI grant is local approval for an environment's effective destination configuration.

Separate debug-case and test-case objects are unnecessary for this scoped release: a single case supports optional assertions. Do not reproduce Apidog's entire object taxonomy.

## Project and local storage contracts

Implementation defaults below fill mechanical details left unspecified in discussion; they are not additional user-confirmed preferences.

Tracked layout: reqscope.yaml contains format version, immutable project ID, project name, module/source metadata and resource locations. specs/ holds original OpenAPI documents; environments/, cases/ and scenarios/ contain individual YAML resource files. Preserve unknown OpenAPI extensions. No generated normalized contract replaces the authoritative source.

IDs are immutable opaque UUIDs, independent of filenames and display names. Case/scenario references use IDs. Imported endpoint IDs persist across refresh using source identity plus method and normalized path; operationId is searchable metadata, not the sole identity. Removed endpoints become tombstones sufficient to explain disconnected cases. Explicit reassignment is required for renamed paths.

Each source stores import mode, source location, module ID, last imported digest and the last imported base needed for a three-way refresh. Do not retain authentication embedded in URLs. Local edits are compared against the base and incoming contract; unresolved conflicts block applying a refresh.

Store run history and metadata in local SQLite. Store authorization and credential bindings outside the project under the OS user data directory, keyed by project ID and local project location. Git clones on another machine receive no approvals or secrets. OS credential storage is preferred for values; never silently fall back to plaintext files. If unavailable, explain the issue in the TUI and allow session-only credentials. No commands export or reveal stored values.

Settings include selected TUI environment, keybindings, history limits and response limits. Local authorization is managed only through interactive TUI operations. Project files may contain ordinary variables and names of credential references; they may not contain credential values or grant access.

Atomic writes include validation and expected-revision checks under a project lock. Revisions are content digests, not timestamps. Multi-resource edits and refreshes are recoverable transactions; readers must not observe partial logical changes. External invalid files produce diagnostics rather than crashes. The TUI reloads clean resources, preserves dirty buffers and offers reload/discard or manual reconciliation on conflict.

## Import, search and refresh

Support Swagger 2.0 and OpenAPI 3.0/3.1 in JSON/YAML. Validate version and structure, preserve source files and local references, and expose the supported contract subset. Unsupported schema constraints produce visible diagnostics; never label an incomplete validation as a complete pass.

Swagger UI discovery reads static page configuration and referenced initializer/config resources for url, urls and embedded spec declarations. Do not execute JavaScript or use a browser runtime. Resolve relative locations against their resource URL. When multiple candidates exist, the TUI prompts; the CLI requires an explicit selection. If discovery cannot identify a valid source, return candidates and an instruction to supply the raw URL.

Network imports in the CLI require previously authorized source origins. A new project can be authored/imported from a local file without network approval; the TUI can approve a Swagger source origin before fetching it. This extends the agreed destination control to network import/reference fetches so those commands cannot bypass production restrictions.

Remote references require approval for their origins and bounded depth/size; project imports must not read arbitrary local files outside approved project/import roots. Diagnose cycles and missing references. No implicit fetching of an arbitrary URL during preview or schema inspection.

Search is case-insensitive and updates on every query edit. Rank path matches before summary/description matches, with stable tie-breaking; support fuzzy matching. Search encompasses paths, operation IDs, summaries, descriptions and tags. Method filters combine with the query. Empty results are explicit. Keep source/module context visible.

Refresh is explicit and preview-first in both interfaces. Preview shows added, changed and removed endpoints and affected cases. CLI applies a reviewed preview token tied to current and incoming digests. TUI confirms the preview. Do not silently adopt upstream credential or destination changes. Retain case inputs; incompatible inputs require correction. Disconnected cases cannot send until explicitly relinked or converted to an independent request.

## Environments and access policy

Initialize local, staging and production templates, all unconfigured and unauthorized. Names are editable; production is a protected classification independent of name. Environment fields include ID, name, module base URLs, ordinary variables and a requested classification. Protected local classification and credential bindings override untrusted edits: lowering production classification requires TUI confirmation and renewal of CLI authorization.

TUI remembers local/staging selection. Reopening a project never restores production as the active environment; select a configured nonproduction default or show the environment chooser. Show environment, production state and resolved destination persistently.

Every CLI network run requires --env with an explicit ID or unique name. No implicit fallback, environment-variable selection or last-used TUI selection. All environments default denied. TUI grants CLI use for a concrete environment, effective base URLs and exact origins. An origin is scheme, normalized host and effective port; do not use substring or wildcard matching.

Store a fingerprint of effective destinations, production classification and protected bindings with approval. Recheck against current files and resolved variables; changes invalidate approval. CLI cannot create, extend or renew grants, change protected bindings, reduce production protection or disable access checks. It can author ordinary settings, which may invalidate approval.

CLI URL/auth overrides cannot expand the grant. Absolute URLs, server alternatives, interpolated hosts, redirects and secret use in query/body/header positions must pass the same checks. Secrets are disallowed in destination host/path fields. For credential attachment, require the locally approved credential/destination binding even if an endpoint file requests a different header or authentication placement. Strip sensitive headers on cross-origin redirects and reject redirect destinations outside the grant before contacting them. Bound redirects and report rejection without secret-bearing URLs.

TUI may deliberately authorize production CLI access, but the default remains denied. Every production TUI send requires confirmation showing method and sanitized resolved URL. A scenario requires confirmation for each actual production HTTP send; cancellation ends the scenario. No unattended approval flag. Production confirmation is an accident-prevention measure, not a guarantee about endpoint side effects.

The threat boundary is our tool's behavior. Same-account shell access and direct local state changes are outside scope. Do not claim TUI interaction proves a human identity.

## Secret handling and variables

Secret names and configured/missing state are inspectable through CLI. Values, credential bindings and secret lifecycle are TUI-managed. CLI cannot read back, replace or delete values. Ordinary CLI variable values must not be repurposed as stored protected credentials.

At run time resolve variables from low to high: project defaults, environment variables, case inputs, explicit run/step input overrides, prior step extracts. Secret references are typed opaque values usable only at approved request positions. Undefined variables fail before sending. Keep run extracts local to the scenario; do not persist automatically into environment settings.

Login extraction targets a JSON Pointer and stores its result as a secret when used as an authentication token. Define extraction type as ordinary or secret. Persisted stored credentials are available before output processing; newly extracted secret values must be registered before logging, reporting, saving or rendering the originating response. Secret extracts remain in process memory and require permitted destination bindings for later requests. An agent may choose existing protected extraction/binding definitions but cannot change them to redirect credentials. Creating a new protected auth extraction requires TUI approval; ordinary extraction remains fully authorable.

Redact registered values in request previews, URLs, errors, headers, bodies, assertions, comparisons, history and exports. Redact known authentication positions even if value matching is insufficient. Redaction occurs before persistence and before any public adapter sees data. Secret getters do not exist in the CLI contract. Sensitive copies are refused or return redacted text. Binary content with secret-bearing data must not be exposed; retain sanitized metadata only when safe sanitization cannot be performed.

Arbitrary encoding/transformation of a secret by an API may evade matching; document that limitation. Default output is sanitized even in the TUI. Credential entry is masked and can be replaced without revealing the stored value.

## Request execution and validation

Case inputs cover path/query parameters, headers, body content, content type, authentication reference and upload file references. Body types: JSON, raw text/binary file, URL-encoded forms and multipart. Uploads require explicit local file paths and are checked before network execution. Store references rather than file contents.

Use contract serialization rules for parameters; preserve duplicate query/header entries where valid. Required input, invalid JSON, missing files, undefined variables and incompatible parameter types fail before sending. No implicit body regeneration overwrites edited content. Contract examples supply initial values where available, visibly marked as suggestions.

Provide raw body editing and external editor round trips. Cancellation/nonzero editor exit preserves the previous buffer. Revalidate edited input before sending. Never write expanded credentials to editor scratch files.

Defaults: TLS verification enabled, 30-second request timeout, at most 5 redirects and 10 MiB retained response body. Limits are configurable locally; expose truncation explicitly and cancel body reads at the cap. Large/binary bodies show metadata rather than assume UTF-8. A truncated body cannot pass a full-body assertion/schema check; mark validation incomplete. No automatic retries, especially for requests that may change data.

A run records ID, source/case IDs, input and contract digests, environment ID, timestamps, duration, destination metadata, sanitized request/response, transport outcome and validation outcomes. HTTP 4xx/5xx responses are completed HTTP exchanges, not transport failures; assertions decide pass/fail. Ad hoc send reports the status without automatically converting it to a transport error.

Declarative assertions cover status equality/range, header presence/equality, JSON Pointer presence and typed equality, and response-schema validation. Preserve JSON types and distinguish absent fields from null. Unsupported checks are errors or explicit incomplete outcomes, never silent successes.

## Scenarios

Scenarios contain ordered uniquely identified steps referencing cases, per-step ordinary input overrides, extraction declarations and failure policy. References retain current case behavior at run start; snapshot all cases/contracts at scenario start so a concurrent edit cannot change mid-run semantics. Show impacted scenarios before editing a shared case. No nested scenarios or copying hidden mutable objects.

Default fail-fast stops on transport errors, failed assertions, extraction failures or incomplete mandatory validation. Optional continue-on-failure allows subsequent independent steps. Dependencies are explicit producer-step references derived from extraction bindings, not guessed from variable names. Dependent steps are skipped if a producer fails; record the reason. A continued scenario with any failed mandatory step still exits unsuccessfully.

Cancellation stops further steps and cancels active transport. Store an aggregate report plus each sanitized step result. The same settings and inputs yield the same constructed requests and validation in TUI and CLI, apart from deliberate caller authorization/production prompts.

## History and comparisons

Record sanitized completed and failed runs for saved cases. Ad hoc requests receive standalone run IDs and may later be saved as cases. Retain at most 100 unpinned runs per case by default and cap unpinned history at 100 MiB per project. These byte defaults are implementation choices. Evict oldest unpinned runs first. Pinned baselines survive cleanup and may exceed the cap; warn about pinned size. Explicit deletion of a pin requires unpinning first. Purge operations never delete protected credentials.

Compare any two recorded runs of the same case, including across environments. Label environment and contract/input changes alongside status and headers. Header names compare case-insensitively; JSON object-key ordering does not matter; arrays compare in order. For valid JSON report added/removed/changed values with JSON Pointer paths. Otherwise provide a text diff or binary metadata comparison.

Saved ignore rules belong to a case, identify exact JSON Pointer fields and optional header names, and affect comparison only. Raw retained responses remain unchanged except initial mandatory sanitization/truncation. Missing fields are distinct from null. Comparison is between retained sanitized bodies; it cannot recover redacted, truncated or evicted content.

## TUI interaction

Default layout: searchable endpoint list on the left, request and response views on the right. Narrow terminals collapse to one focused pane with explicit switches; avoid clipped controls. Layout changes preserve selection, unsaved buffers and pending request state.

Default motions: j/k, gg/G, counts, Ctrl-d/Ctrl-u; Ctrl-w h/j/k/l changes pane, Ctrl-w o zooms; / searches the focused pane, n/N advances matches; g-prefixed commands jump to endpoints, requests, responses and history. Global endpoint search is also reachable through the leader menu. Enter opens a selection; i or Enter edits an editable field; Esc leaves input mode or cancels the pending sequence without losing edits.

Space is the configurable default leader. Group actions under request, environment, case, scenario, history and project prefixes. Sending, saving, refresh, credential management and editor launch use explicit actions. Define exact action bindings centrally during implementation; do not scatter literals across widgets.

A which-key overlay lists valid next keys whenever a prefix is pending. Escape or sequence completion dismisses it. The overlay floats without reflowing panes. Help on ? and the command palette derive from the same effective binding/action registry, respecting overrides and unbound actions. Validate ambiguous/conflicting bindings at startup. Text entry suspends navigation bindings; secret fields never become copyable through generic yank actions.

Unsaved edits are visible. Leaving a dirty resource or quitting offers save/discard/cancel. Request send uses the visible draft; reports label its unsaved state. Explicit save updates the case after revision validation.

## CLI contract

Use explicit subcommands with --project for project selection. Selection defaults only to discovery of a reqscope.yaml in the current directory or ancestors, never to an arbitrary previously used project. Resource names must resolve uniquely; IDs are always accepted.

Required command groups:
- project: init, inspect and validate.
- spec: import, inspect, refresh-preview, refresh-apply and export.
- endpoint and schema: list, get, create, update and delete, with explicit contract-edit intent for imported source edits.
- environment and variables: list/get and ordinary create/update/delete operations.
- case and scenario: list/get/create/update/delete; scenario validate/run.
- request: send a saved case or explicit draft with --env.
- history: list/get/pin/unpin/delete/prune.
- compare: two run IDs and saved or supplied ignore rules.
- input-schema: list/get/validate for structured command inputs.
- access and secret: inspect names, configured state and grants only. Write operations explain that interactive TUI management is required.

Structured writes accept JSON/YAML files or stdin, use schema validation and support a no-write preview. Existing-resource updates require the revision from get. Deletion reports references; reject dangling references unless the caller explicitly selects a documented repair operation. No execution happens during validation/preview.

--output json returns one versioned envelope containing success state, data, warnings and structured error when present. Fields include schemaVersion and a stable error code; resource output includes ID/revision. Progress and human diagnostics go to stderr, keeping stdout parseable. No ANSI styling or interactive prompts in JSON mode. Bounded body previews carry truncation/body-availability metadata; paging has explicit limits and continuation cursors. Reports and file outputs are sanitized.

Exit codes: 0 success; 2 usage/input/schema validation; 3 missing resource or revision conflict; 4 authorization/credential denial; 5 transport/network failure; 6 failed assertion/schema check or unsuccessful scenario; 7 storage/internal failure; 130 cancellation. Document precedence: unsuccessful scenarios use 6 and carry per-step causes; ad hoc transport failures use 5; pre-execution access denial uses 4. Validation success does not imply authorization to execute.

No --force, --agent=false or environment variable bypass changes credential/access policy. Agent guidance may describe valid next operations but must never suggest exposing a credential.

## Testing decisions

No existing test seam exists. Prefer the public CLI process, shared application entry points and TUI event/render boundaries. Use temporary real projects, real filesystem/SQLite and in-process local HTTP servers. Introduce a credential-store interface only at the actual OS integration boundary, with a controlled test store; do not add internal mocks for every module.

Required behavior coverage:
- Import supported formats, local references, Swagger discovery with relative initializer resources and multiple candidates, malformed specs and discovery fallback.
- Search partial paths/descriptions, method filters, stable ordering and no results.
- JSON/form/multipart serialization and upload failures against an HTTP server that records received bytes.
- TUI/CLI request parity for approved environments and scenario snapshots during concurrent edits.
- Authenticated login/extraction/chaining; failed producer skips dependents; continuation preserves overall failure.
- Default-denied CLI execution; approved staging; denied production; edited base URL, classification, auth placement, host interpolation, cross-origin redirect and raw-request bypass attempts.
- Registered secrets absent from stdout, stderr, editor files, SQLite, previews, reports and diffs; login response sanitization happens before first output.
- Contract refresh add/change/remove, local-edit conflicts, preserved case inputs and disconnected-case rejection.
- Revision races and interrupted multi-file transactions cannot silently lose changes.
- JSON comparison, arrays, null vs missing, ignored fields, pinning, retention and truncation/incomplete validation.
- TUI motions, modes, prefix cancellation, configured leader, help correctness, production confirmations, narrow layout, editor cancellation and unsaved edits.
- CLI exit codes, schemas, stdin input, paging and no prompts without a terminal.
- Linux/macOS build and OS credential integration, including missing-service session-only behavior.

Use a small reusable realistic OpenAPI fixture and local HTTP backend for end-to-end coverage. Do not require a production service or real credentials for automated tests. Human acceptance uses the user's actual backend Swagger URL only after access and credentials are configured; this specification does not claim that acceptance has already run.

## Release acceptance

1. Import a backend Swagger UI URL, discovering the correct raw specification or completing the explicit fallback.
2. Find an operation by partial path and description.
3. Configure staging and credentials in the TUI; verify a request at the backend.
4. Save a case, rerun it, compare responses and save an ignore rule.
5. Assemble login, secret extraction and an asserted API request into a scenario.
6. Author/read/validate project resources via CLI and execute the scenario using explicit approved staging.
7. Demonstrate matching execution behavior, sanitized output and useful failure reports.
8. Demonstrate denied production CLI calls, denied unapproved destinations and renewed approval after destination changes.
9. Refresh the specification with preserved inputs and visible conflicts.
10. Demonstrate Vim/leader/help behavior and retained local history on Linux and macOS.

Completion requires these acceptance paths and required automated checks. A runnable request editor alone does not satisfy v1.

## Implementation order

This is an implementation sequence, not tracker tickets or authorization to begin implementation.

1. Versioned project model, schemas, IDs, revisions and recoverable writes.
2. Specification import/discovery/search and environment configuration.
3. Credential/access module and sanitized HTTP runner, verified before credential-bearing output exists.
4. Public CLI authoring/execution contracts and high-level integration fixtures.
5. TUI navigation, request editing, credential management and authorization.
6. Assertions, secret extraction and sequential scenarios.
7. History, comparisons, retention, refresh/conflicts and concurrency handling.
8. Platform verification, documentation and real-project acceptance.

Before implementation, dependency versions and supported OpenAPI validation/serialization libraries need primary-source verification. They are implementation research, not unresolved product questions. If tracker publication is desired, run /setup-engineering-workflows; until then this local document is the ready-for-agent specification.
