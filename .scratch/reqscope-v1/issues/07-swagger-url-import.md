# 07 — Import from Swagger UI and specification URLs

**What to build:** Start a project from the backend team's Swagger link or a raw specification URL, with clear fallback when automatic discovery is insufficient.

**Blocked by:** 02 — Import and inspect local API specifications; 04 — Configure environments and authorize destinations; 06 — Use credentials without exposing them

**Status:** complete

**Specification coverage:** User stories 3, 4, 5, 6, 7.

- [x] TUI approves source origins before network fetching; CLI imports and remote-reference fetching require existing applicable origin authorization.
- [x] Raw JSON/YAML URLs import through the existing contract path, preserving source location and refresh metadata without embedded credentials.
- [x] Static Swagger configuration discovery supports url, urls and embedded spec declarations in pages and referenced initializer/config resources.
- [x] Relative locations resolve against their containing resource; JavaScript is never executed and no browser runtime is required.
- [x] Multiple specifications are selectable in the TUI; CLI requires explicit selection. Failed discovery provides useful candidates and a raw-URL fallback.
- [x] Remote references obey origin approval and bounded size/depth; redirects, missing resources and cycles produce sanitized errors.
- [x] Import failures leave the prior project intact. Validation/preview never implicitly contacts an unapproved source.
- [x] Local HTTP fixture tests cover relative discovery, several candidates, raw URLs, fallback, authenticated imports, redirects and denied reference origins.
