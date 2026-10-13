# 01 — Initialize and reopen a project

**What to build:** Create a local Reqscope project and reopen it through either interface. Establish the shared application operations and a usable keyboard-driven TUI so later features extend an actual workflow.

**Blocked by:** None — can start immediately

**Status:** ready-for-agent

**Specification coverage:** User stories 1, 36, 37, 38, 39, 45, 46, 49, 51, 52, 54.

- [ ] Project initialization creates a versioned project with an immutable ID; inspection and validation work through the TUI and CLI.
- [ ] Explicit project selection and current-directory/ancestor discovery work; ambiguous resource names and malformed or unsupported project formats produce actionable errors.
- [ ] Resource writes validate inputs, use content revisions and a project lock, and are atomic; multi-resource changes recover after interruption without exposing partial logical state.
- [ ] CLI structured writes accept JSON/YAML files or stdin and provide schemas and no-write previews. JSON stdout is a single versioned envelope; diagnostics use stderr and documented exit codes.
- [ ] No arguments starts the TUI only with terminal input/output; nonterminal use prints usage without prompting.
- [ ] The TUI provides normal/insert modes, Vim motions and counts, pane switching, configurable Space leader, a floating continuation menu, a command palette and help derived from effective bindings.
- [ ] Conflicting keybindings are rejected; Esc cancels pending sequences or leaves input mode. Narrow layouts preserve focus and buffers.
- [ ] High-level tests exercise a real temporary project, interrupted writes, noninteractive invocation and TUI key events. Verify supported dependency choices from primary sources before adopting them.
