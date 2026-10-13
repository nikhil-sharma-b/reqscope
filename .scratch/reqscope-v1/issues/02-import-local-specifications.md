# 02 — Import and inspect local API specifications

**What to build:** Load a backend contract from a local file and inspect its endpoints and schemas without making network requests.

**Blocked by:** 01 — Initialize and reopen a project

**Status:** ready-for-agent

**Specification coverage:** User stories 2, 7, 10, 44, 51.

- [ ] Both interfaces import Swagger 2.0 and OpenAPI 3.0/3.1 in JSON or YAML, preserving original documents and unknown extensions.
- [ ] Imports create source/module metadata, stable endpoint IDs, an import digest and the base needed for later refresh; IDs do not depend solely on operationId.
- [ ] Endpoint/schema list and get operations show method, path, descriptions, tags, request inputs, responses and source context.
- [ ] Local references resolve only within approved import roots with bounded traversal; cycles, missing references and malformed contracts return useful diagnostics.
- [ ] Unsupported schema constraints or operations are identified explicitly; incomplete support never appears as a full validation success.
- [ ] Import preview validates without applying writes or fetching remote references. Invalid imports leave the project unchanged.
- [ ] Public-command and TUI tests cover all supported versions and formats, references, extensions and invalid inputs using realistic fixtures.
