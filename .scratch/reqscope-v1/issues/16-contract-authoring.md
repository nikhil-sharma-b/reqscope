# 16 — Author and export API contracts

**What to build:** Explore explicit local API changes while preserving imported contract ownership and identifying affected saved requests.

**Blocked by:** 08 — Save and maintain request cases

**Status:** ready-for-agent

**Specification coverage:** User stories 34, 44, 45, 46, 52.

- [ ] Endpoint/schema create/update/delete, inspection and export work through both interfaces with explicit imported-contract edit intent.
- [ ] Edits preserve original OpenAPI structure, unknown extensions, references and source identity; normalized internal views never replace authoritative documents.
- [ ] Schemas, no-write previews and expected revisions cover structured contract edits; invalid edits leave project data unchanged.
- [ ] Before edits/deletion, show affected cases and available scenarios; prevent dangling references unless an explicit repair is selected.
- [ ] Export excludes local credentials, approvals and history and does not claim to serialize cases/scenarios into ordinary OpenAPI.
- [ ] Local edits retain the imported base needed for future three-way refresh and cannot silently change protected destinations or authentication bindings.
- [ ] Tests cover edited contracts, stable IDs, reference-aware deletion, extension preservation, sanitized export and stale revisions.
