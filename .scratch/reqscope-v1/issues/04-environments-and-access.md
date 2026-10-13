# 04 — Configure environments and authorize destinations

**What to build:** Keep deployment settings separate and let a developer explicitly authorize which destinations CLI callers may use.

**Blocked by:** 01 — Initialize and reopen a project

**Status:** ready-for-agent

**Specification coverage:** User stories 11, 12, 13, 40, 41, 42, 48, 52.

- [ ] Initialization supplies unconfigured local, staging and production templates; TUI and CLI support ordinary environment/variable authoring with stable IDs.
- [ ] CLI network access defaults to denied. Interactive TUI authorization records exact origins, effective base URLs, production classification and protected binding fingerprints.
- [ ] Ordinary CLI edits cannot create grants, lower protected classification or alter protected credential bindings; relevant changes invalidate prior approval.
- [ ] Authorization stays local and is scoped to project identity and location; exported/copied projects receive no grants.
- [ ] Environment names are editable independently of production classification. Lowering protected production classification requires TUI confirmation.
- [ ] TUI selection remembers nonproduction environments; restart never restores production as active. Active environment/destination remain visible.
- [ ] Every CLI network run requires explicit --env; authorization is checked against current resolved settings rather than last-used UI state.
- [ ] Tests demonstrate denied defaults, allowed staging, denied production, renamed environments and approval invalidation after destination or classification changes.
