# Contract authoring review

Reviewed `.scratch/reqscope-v1/issues/16-contract-authoring.md` and the storage
contracts in `.scratch/reqscope-v1/spec.md` against branch base
`fd923d75af945b7697e7ca29d142fe6ba8fd60af`.

The repository has no `docs/agents/issue-tracker.md`; the local ticket supplies
the requirements. Tracker integration can be configured with
`/setup-matt-pocock-skills` when needed.

## Standards

The independent review found no documented standards violations. It identified
incorrect source selection in projects with multiple imports, caused by decoding
display text into an incomplete resource type. Contract rows now carry source IDs
alongside labels and details. A TUI regression test exports the selected second
source. The follow-up review found no remaining findings.

## Spec

The independent review identified that contract authoring could persist or export
registered credential values. The shared operations now refresh the credential
registry and reject protected values before persistence and export. Both adapters
pass their credential context. Checks cover parsed strings, YAML maps with numeric
keys, and raw comments. Regression tests verify rejected edits leave revisions
unchanged and exports reject protected documents. The follow-up review confirmed
no remaining findings.

## Validation

`make check` runs vet and builds every package. `make test` runs the complete
race-enabled suite. Focused public API, CLI and TUI tests cover create/update/delete,
input schemas, no-write previews, explicit edit intent, stale revisions, stable IDs,
base and extension preservation, retained external references without reading the
original files, dangling-reference rejection, affected cases/scenarios, atomic
case reassignment, tombstones, protected destination/authentication rejection,
contract-only exports, credential rejection and selected-source export.

Exports are document bundles keyed by retained filenames or remote URLs. They do
not serialize cases or scenarios into OpenAPI. The TUI copies the bundle to the
terminal clipboard and displays it for inspection. Reference repairs are explicit
edits in retained documents; the authoring operation never fetches new references.
