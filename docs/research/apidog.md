# Apidog research for api-tui
Research date: 2026-10-13. Purpose: understand Apidog before agreeing on a terminal product. Repository was empty when inspected.

## Evidence and limits
This is research of official documentation and the official agent skills repository, not a hands-on evaluation of the desktop app or reverse engineering of its internals. Product behavior below is documented behavior. Proposed architecture and scope are recommendations, not agreed requirements.

## How Apidog works
A project contains modules, environments and test scenarios. A module groups endpoints, schemas, response components and security schemes, represents a standalone OpenAPI specification, and can have a base URL for each environment. An endpoint describes an operation such as GET /users/{id}. Saved cases retain concrete inputs for that endpoint. These are different objects with different editing responsibilities. [Basic concepts](https://docs.apidog.com/basic-concepts-in-apidog-644342m0)

A typical workflow is to design or import an endpoint, choose an environment, generate or enter inputs, send a request, inspect the response, validate it against its specification and assertions, and save reusable cases. Pre/post processors support scripts and variable extraction. Apidog supports both design-first and request-first work, including generating specifications from requests. [Develop and debug](https://docs.apidog.com/develop-and-debug-apis-in-apidog-541758m0)

Test scenarios sequence requests, pass values between steps, use test data and flow control, and produce reports. Endpoints imported into scenarios can synchronize manually or automatically. Cases can be copied into independent steps or referenced as shared objects; editing a referenced case can affect other consumers. Scenarios can reference other scenarios. The clone needs an explicit policy for references, overrides and synchronization. [Create a test scenario](https://share.apidog.com/apidoc/docs-site/544525/create-a-test-scenario-599311m0)

Variables use {{name}} substitution. Documented scope precedence, low to high, is Global < Module < Environment < Data < Local. Global variables can be project or team scoped. This matters for token extraction, request chaining and reproducible executions. [Using variables](https://docs.apidog.com/en/using-variables-577908m0)

Smart Mock produces responses using explicit mock fields first, property-name rules second and schema-based defaults third. Mock fields may contain fixed values or Faker expressions. The documentation says generated values respect schema constraints. [Smart Mock](https://docs.apidog.com/smart-mock-618190m0)

Matching mock expectations take precedence. The default fallback is Smart Mock; an alternative configuration tries response examples before Smart Mock. A terminal clone must define matching, fallback and reproducibility rather than treating mocks as a single hardcoded response. [Mock precedence](https://docs.apidog.com/en/mock-priority-sequence-618208m0)

Integration workflows include manual execution, scheduled runs and CLI execution in CI. Processors can include assertions, extraction and database operations. [Integration testing](https://docs.apidog.com/en/integration-testing-610064m0)

The documented request protocols include HTTP, GraphQL, gRPC, WebSocket, SOAP and SSE. Supporting streaming and RPC requires additional interaction and execution models. Protocol coverage is a scope decision. [Learning center](https://docs.apidog.com/)

## Interchange and ownership
Apidog imports JSON/YAML in OpenAPI 3.0, 3.1 and Swagger 2.0 formats. Its current documentation describes a Spec-first project mode for keeping specification files authoritative and synchronizing with Git. Choosing local files as the source of truth is therefore a design choice, not sufficient by itself to distinguish this project. [OpenAPI import](https://docs.apidog.com/import-openapi-spec-635046m0)

The CLI documents OpenAPI and native exports and broader import/export workflows. We should evaluate conversion fidelity separately for endpoint contracts, saved requests, scripts, environments and test scenarios. OpenAPI exchange must not be assumed to preserve every project object. The fidelity concern is our inference from the separate resource types. [CLI reference](https://docs.apidog.com/609656m0)

## Existing agent CLI
Apidog already has an agent-oriented CLI, not just a test runner. Its documentation describes querying and editing project assets, schema validation before writes, structured output with agentHints, test reports and workflows that read, validate, write and verify. Remote project access requires an account, project permissions and a token. [Agent guide](https://docs.apidog.com/en/use-apidog-cli-with-an-ai-agent-2264718m0)

The CLI exposes endpoint, schema, environment, variables, test-case, test-scenario, test-suite, mock, import/export and branch commands. cli-schema list/get/validate lets a caller discover accepted input schemas. apidog run supports online project data and local exported files, environment selection, iterations and reporters. Exact version-specific behavior should be checked against an installed CLI before claiming compatibility. [CLI reference](https://docs.apidog.com/609656m0)

Apidog documents AI branches for isolating externally initiated writes, with user confirmation before merging and configurable direct-edit permissions. This is Apidog behavior, not a requirement to copy its permission model. [AI branches](https://docs.apidog.com/en/ai-branch)

The official repository packages skills for general CLI usage, diagnostics, branches, import/export, test cases, scenarios, suites and documentation. Agent usability includes discoverable schemas and verifiable workflows as well as command names. [Official skills](https://github.com/apidog/apidog-cli-skills)

## Proposed terminal design
These are recommendations for discussion.
- Build a shared domain model and execution engine used by both TUI and CLI.
- The TUI should browse project resources, edit request fields, select environments, send/cancel requests, inspect responses and review assertion failures.
- The CLI should discover, inspect, create/edit, validate, execute and report on the same resources without a terminal or interactive prompts.
- Use explicit JSON output, stable resource identifiers, documented error and exit-code contracts, stdin/file inputs and bounded response output.
- Separate saved contracts and inputs from resolved secrets, transient variables and run artifacts.
- Consider schema discovery, dry-run change previews, atomic writes and detection of concurrent edits for agent/human use.
- Pick file ownership deliberately: canonical OpenAPI plus supplementary test files, or a native format with OpenAPI interchange.
- Evaluate a first usable workflow: import a real specification, configure a local environment, send an authenticated request, save a case, chain two requests and execute the same test through the CLI.
- Full Postman script compatibility, advanced authentication, streaming protocols, database steps, load testing, hosted docs and live collaboration each expand scope substantially.

## Initial decision frontier
Already fixed by the user: terminal UI plus a CLI that AI agents can use; research and alignment precede implementation.
Open root decisions:
1. Primary user and concrete pain in the current API workflow.
2. Meaning of clone: selected workflows, eventual broad parity, or interoperability with existing Apidog projects.
3. Ownership and collaboration: local files/Git, shared service, or both.
4. Agent authority: execution only, or full authoring and editing through CLI.
5. Delivery constraint: practical first release or longer product effort, including any real deadline.
Later decisions depend on these answers: milestone features, protocol coverage, canonical storage, import fidelity, secret handling, scripting model, CLI contracts, TUI interaction, concurrent edits and implementation stack.

## Additional model details
Current Apidog separates debug cases used for troubleshooting from test cases that combine inputs, processors and validation settings. Test cases have explicit synchronization with endpoint definitions. We should preserve that distinction when choosing our own model. [Test cases](https://docs.apidog.com/en/test-cases-1605608m0)

Variable scope and device-local values are different concepts. Apidog documents shared values and local device values, with empty device values falling back to shared ones. Its client and CLI can select different values by default. Our shared runner should make value selection explicit so a TUI run and an agent run are reproducible. [Using variables](https://docs.apidog.com/using-variables-577908m0)

Protocol listings do not demonstrate equal testing and mocking support for every protocol. Any parity claim needs a capability matrix and hands-on verification. [Supported API technologies](https://docs.apidog.com/api-technologies-supported-895780m0)
