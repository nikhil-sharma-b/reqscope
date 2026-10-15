# 16 — Author and export API contracts

**What to build:** Explore explicit local API changes while preserving imported contract ownership and identifying affected saved requests.

**Blocked by:** 08 — Save and maintain request cases

**Status:** complete

**Specification coverage:** User stories 34, 44, 45, 46, 52.

- [x] Endpoint/schema create/update/delete, inspection and export work through both interfaces with explicit imported-contract edit intent.
- [x] Edits preserve original OpenAPI structure, unknown extensions, references and source identity; normalized internal views never replace authoritative documents.
- [x] Schemas, no-write previews and expected revisions cover structured contract edits; invalid edits leave project data unchanged.
- [x] Before edits/deletion, show affected cases and available scenarios; prevent dangling references unless an explicit repair is selected.
- [x] Export excludes local credentials, approvals and history and does not claim to serialize cases/scenarios into ordinary OpenAPI.
- [x] Local edits retain the imported base needed for future three-way refresh and cannot silently change protected destinations or authentication bindings.
- [x] Tests cover edited contracts, stable IDs, reference-aware deletion, extension preservation, sanitized export and stale revisions.


Implementation notes:

- Shared `EditContract` validates authoritative document edits and computes impact
  under the project lock. `ExportContract` returns only current contract documents.
- CLI endpoint/schema create, update and delete commands use discoverable input
  schemas, explicit imported edit intent and source revisions. `--preview` does not
  write. Referenced endpoint deletion requires an explicit same-source reassignment.
- TUI `Space d a` previews and applies structured edits; `Space d o` copies the
  selected source's contract document bundle to the clipboard for export.
- Original documents, imported digest and base remain available for refresh. Local
  documents preserve extensions and references, stable IDs and removal tombstones.
- Registered credential values are rejected before persistence and export, including
  values inside YAML comments and maps with numeric keys. Local destination and
  authentication configuration remains protected by the shared operation.
- `make check` and the full race-enabled `make test` suite pass. Independent
  standards and specification reviews have no unresolved findings. See
  `docs/contract-authoring-review.md`.
