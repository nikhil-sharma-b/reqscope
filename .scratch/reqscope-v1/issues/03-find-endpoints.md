# 03 — Find endpoints quickly

**What to build:** Find the relevant operation by partial path or description and inspect enough context to distinguish similar endpoints.

**Blocked by:** 02 — Import and inspect local API specifications

**Status:** ready-for-agent

**Specification coverage:** User stories 8, 9, 10.

- [ ] Search updates with each query edit and covers paths, operation IDs, summaries, descriptions and tags.
- [ ] Matching is case-insensitive and fuzzy; path matches rank before descriptive matches with deterministic ties.
- [ ] Method filters combine with search and tag grouping; duplicate tags do not create ambiguous identities.
- [ ] Results expose method, path, summary and source/module context; selecting a result opens its contract details.
- [ ] Global endpoint search and focused-pane search are distinct, reachable through Vim bindings and documented in help.
- [ ] CLI queries produce the same selection/ranking and provide bounded results with continuation support; empty results are explicit.
- [ ] Tests cover partial paths, descriptions, filtering, stable ranking, empty results and narrow-terminal navigation.
