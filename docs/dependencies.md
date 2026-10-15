# Dependency choices

Verified on 2026-10-13 using upstream documentation and tagged Go module metadata. Versions are pinned in `go.mod` and `go.sum`.

| Dependency | Version | Purpose and primary source |
| --- | --- | --- |
| Bubble Tea | v2.1.0 | Terminal event loop. [Upstream v2 guide](https://github.com/charmbracelet/bubbletea/blob/main/UPGRADE_GUIDE_V2.md) documents `charm.land/bubbletea/v2`, key events and `tea.View`. |
| Lip Gloss | v2.0.6 | Layout and floating layers. [Upstream repository](https://github.com/charmbracelet/lipgloss) and [API](https://pkg.go.dev/charm.land/lipgloss/v2) document the v2 module and compositor. |
| go-yaml | v3.0.5 | JSON/YAML input and project files. [Maintained API](https://pkg.go.dev/go.yaml.in/yaml/v3) provides strict known-field decoding and node types. |
| google/uuid | v1.6.0 | Opaque UUID identities. [Upstream repository](https://github.com/google/uuid) documents UUID generation and parsing. |
| x/sys | v0.49.0 | OS file locking on Linux/macOS. [Unix API](https://pkg.go.dev/golang.org/x/sys/unix) provides `Flock`. |
| x/term | v0.47.0 | Terminal detection. [API](https://pkg.go.dev/golang.org/x/term) documents `IsTerminal`. |
| go-keyring | v0.2.6 | OS credential storage through Linux Secret Service and macOS Keychain. [Upstream repository](https://github.com/zalando/go-keyring) documents the APIs, MIT license and platform dependencies. |
| x/ansi | v0.11.9 | Width-aware truncation. [Upstream API](https://pkg.go.dev/github.com/charmbracelet/x/ansi) documents `Truncate`. |

Bubble Tea v2.1.0 and the selected x/sys and x/term versions require Go 1.26.0, so that is the application's minimum. No implementation code was copied from jira-tui; its interaction patterns informed the specification. Dependency licenses are retained in upstream modules.

Local history uses modernc.org/sqlite v1.46.1, a pure Go SQLite driver through database/sql. It builds without a C toolchain. Full OpenAPI validation remains deferred. Credential tests substitute only the OS store boundary; production uses go-keyring with no file fallback.
