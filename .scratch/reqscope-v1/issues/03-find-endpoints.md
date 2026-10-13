# 03 — Find endpoints quickly

**What to build:** Find the relevant operation by partial path or description and inspect enough context to distinguish similar endpoints.

**Blocked by:** 02 — Import and inspect local API specifications

**Status:** completed

**Specification coverage:** User stories 8, 9, 10.

- [x] Search updates with each query edit and covers paths, operation IDs, summaries, descriptions and tags.
- [x] Matching is case-insensitive and fuzzy; path matches rank before descriptive matches with deterministic ties.
- [x] Method filters combine with search and tag grouping; duplicate tags do not create ambiguous identities.
- [x] Results expose method, path, summary and source/module context; selecting a result opens its contract details.
- [x] Global endpoint search and focused-pane search are distinct, reachable through Vim bindings and documented in help.
- [x] CLI queries produce the same selection/ranking and provide bounded results with continuation support; empty results are explicit.
- [x] Tests cover partial paths, descriptions, filtering, stable ranking, empty results and narrow-terminal navigation.

Completed in commit `84404d6` on `reqscope-v1/find-endpoints`. Validation: `make check` and the full race-enabled `make test` suite passed; review findings resolved.
