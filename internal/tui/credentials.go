package tui

import (
	"encoding/json"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

var credentialFields = []string{"Name", "Kind: bearer/api-key/basic", "Module", "Exact destination URL", "Placement: header/query/body", "Field: empty for body", "Value: basic uses username:password"}

func (m *Model) clearCredentialEntry() {
	m.credentialEntry = [7]string{}
	m.credentialField = 0
	m.overlay = ""
	m.mode = "NORMAL"
}
func (m *Model) credentialAction(action string) bool {
	switch action {
	case "credential-edit":
		m.credentialEntry = [7]string{"", "bearer", "", "", "header", "Authorization", ""}
		m.credentialField = 0
		m.overlay = "credential-edit"
		m.mode = "INSERT"
	case "credential-delete":
		m.credentialEntry = [7]string{}
		m.credentialField = 0
		m.overlay = "credential-delete"
		m.mode = "INSERT"
	case "credential-inspect":
		states, err := m.credentials.Inspect(m.root, m.activeEnvironment)
		if err != nil {
			m.status = err.Error()
			return true
		}
		b, _ := json.Marshal(states)
		m.detail = string(b)
		m.overlay = "detail"
		m.paletteIndex = 0
	default:
		return false
	}
	return true
}
func (m *Model) saveCredential(session bool) {
	f := m.credentialEntry
	b := project.CredentialBinding{Name: f[0], Kind: f[1], Module: f[2], Destination: f[3], In: f[4], Field: f[5]}
	if err := m.credentials.Save(m.root, m.activeEnvironment, b, f[6], session); err != nil {
		if pe, ok := err.(*project.Error); ok && pe.Code == "credential_unavailable" {
			m.overlay = "credential-session"
			m.mode = "NORMAL"
		}
		m.status = err.Error()
		return
	}
	m.clearCredentialEntry()
	m.status = "Credential saved; review CLI access again"
}
func (m *Model) credentialUpdate(msg tea.KeyPressMsg) bool {
	switch m.overlay {
	case "credential-edit":
		switch msg.String() {
		case "esc", "ctrl+c":
			m.clearCredentialEntry()
		case "tab":
			m.credentialField = (m.credentialField + 1) % len(credentialFields)
		case "shift+tab":
			m.credentialField = (m.credentialField + len(credentialFields) - 1) % len(credentialFields)
		case "ctrl+u":
			m.credentialEntry[m.credentialField] = ""
		case "enter":
			if m.credentialField < len(credentialFields)-1 {
				m.credentialField++
			} else {
				m.saveCredential(false)
			}
		default:
			m.credentialEntry[m.credentialField] = edit(m.credentialEntry[m.credentialField], msg)
		}
		return true
	case "credential-session":
		switch msg.String() {
		case "y":
			m.saveCredential(true)
		case "esc", "ctrl+c":
			m.clearCredentialEntry()
		}
		return true
	case "credential-delete":
		switch msg.String() {
		case "esc", "ctrl+c":
			m.clearCredentialEntry()
		case "enter":
			m.overlay = "credential-delete-confirm"
			m.mode = "NORMAL"
		case "ctrl+u":
			m.credentialEntry[0] = ""
		default:
			m.credentialEntry[0] = edit(m.credentialEntry[0], msg)
		}
		return true
	case "credential-delete-confirm":
		switch msg.String() {
		case "esc", "ctrl+c":
			m.clearCredentialEntry()
		case "y":
			if err := m.credentials.Delete(m.root, m.activeEnvironment, m.credentialEntry[0]); err != nil {
				m.status = err.Error()
			} else {
				m.clearCredentialEntry()
				m.status = "Credential deleted; CLI approval removed"
			}
		}
		return true
	}
	return false
}
func (m *Model) credentialOverlay() string {
	switch m.overlay {
	case "credential-edit":
		lines := []string{"Add or replace credential | active environment: " + m.activeEnvironment, "Existing values cannot be revealed. Tab changes fields; Enter advances/saves; Esc cancels."}
		for i, label := range credentialFields {
			marker := "  "
			if i == m.credentialField {
				marker = "> "
			}
			value := m.credentialEntry[i]
			if i == 6 {
				value = strings.Repeat("*", len([]rune(value)))
			}
			lines = append(lines, marker+label+": "+value)
		}
		return strings.Join(append(lines, m.status), "\n")
	case "credential-session":
		return "OS credential storage unavailable or locked.\n" + m.status + "\nKeep this value only until this process exits?\ny choose session-only | Esc discard"
	case "credential-delete":
		return "Delete credential from active environment\nName: " + m.credentialEntry[0] + "\nEnter review | Esc cancel"
	case "credential-delete-confirm":
		return "Delete credential " + m.credentialEntry[0] + "?\nCLI approval will be removed.\ny delete | Esc cancel\n" + m.status
	}
	return ""
}
