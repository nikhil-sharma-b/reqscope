package tui

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"strings"
)

func (m *Model) caseItems() []project.CaseResource {
	result := []project.CaseResource{}
	for _, c := range m.savedCases {
		if strings.Contains(strings.ToLower(c.Name+" "+c.Request.EndpointID), strings.ToLower(m.query)) {
			result = append(result, c)
		}
	}
	return result
}
func (m *Model) requestDirty() bool {
	return m.requestText != m.requestBaseline || m.caseMetadataDirty || m.overlay == "case-edit" && m.caseText != m.caseTextBaseline
}
func (m *Model) clearCaseDraft() {
	m.caseDraft = project.CaseResource{}
	m.caseMetadataDirty = false
	m.requestText = ""
	m.requestBaseline = ""
	m.requestFields = nil
}
func (m *Model) saveCaseDraft() bool {
	if m.caseDraft.ID != "" {
		refs, err := project.CaseReferences(m.root, m.caseDraft.ID)
		if err != nil {
			m.status = err.Error()
			return false
		}
		if len(refs) > 0 {
			m.overlay = "case-impact"
			m.mode = "NORMAL"
			m.status = "Affected scenarios: " + strings.Join(refs, ", ")
			return false
		}
	}
	return m.saveCaseDraftConfirmed()
}
func (m *Model) saveCaseDraftConfirmed() bool {
	var d project.RequestDraft
	if err := project.Decode([]byte(m.requestText), &d); err != nil {
		m.status = err.Error()
		return false
	}
	c := m.caseDraft.Case
	if c.ID == "" {
		c.ID = uuid.NewString()
		c.Name = "New case"
	}
	c.Request = d
	saved, err := project.SaveCase(m.root, c, m.caseDraft.Revision, false)
	if err != nil {
		m.status = err.Error()
		return false
	}
	m.caseDraft = saved
	m.requestBaseline = m.requestText
	m.caseMetadataDirty = false
	m.savedCases, err = project.Cases(m.root)
	if err != nil {
		m.status = err.Error()
		return false
	}
	m.status = "Case saved"
	return true
}
func (m *Model) caseAction(action string) (bool, tea.Cmd) {
	if action == "open" && m.pane == casesPane {
		action = "case-open"
	}
	switch action {
	case "case-open", "case-run", "case-duplicate", "case-delete":
		items := m.caseItems()
		if len(items) == 0 {
			m.status = "Select a saved case"
			return true, nil
		}
		c := items[min(m.index, len(items)-1)]
		switch action {
		case "case-delete":
			m.caseDelete = c
			m.overlay = "case-delete"
			m.status = "Deleting a case rejects scenario references"
		case "case-duplicate":
			_, err := project.DuplicateCase(m.root, c.ID, c.Name+" copy", false)
			if err != nil {
				m.status = err.Error()
			} else {
				m.savedCases, _ = project.Cases(m.root)
				m.status = "Case duplicated"
			}
		default:
			current, err := project.GetCase(m.root, c.ID)
			if err != nil {
				m.status = err.Error()
				return true, nil
			}
			m.loadCaseDraft(current)
			m.pane = requestPane
			m.query = ""
			m.index = 0
			if action == "case-run" {
				return true, m.prepareSend()
			}
		}
	case "case-save":
		if m.caseDraft.ID == "" {
			var d project.RequestDraft
			if err := project.Decode([]byte(m.requestText), &d); err != nil {
				m.status = err.Error()
				return true, nil
			}
			m.caseDraft = project.CaseResource{Case: project.Case{ID: uuid.NewString(), Name: "New case", Request: d}}
			m.caseMetadataDirty = true
			b, _ := json.Marshal(m.caseDraft.Case)
			m.caseText = string(b)
			m.caseTextBaseline = m.caseText
			m.overlay = "case-edit"
			m.mode = "INSERT"
		} else {
			m.saveCaseDraft()
		}
	case "case-edit":
		if m.caseDraft.ID == "" {
			return m.caseAction("case-save")
		}
		var d project.RequestDraft
		if err := project.Decode([]byte(m.requestText), &d); err != nil {
			m.status = err.Error()
			return true, nil
		}
		c := m.caseDraft.Case
		c.Request = d
		b, _ := json.Marshal(c)
		m.caseText = string(b)
		m.caseTextBaseline = m.caseText
		m.overlay = "case-edit"
		m.mode = "INSERT"
	default:
		return false, nil
	}
	return true, nil
}
func (m *Model) caseKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	switch m.overlay {
	case "case-impact":
		if msg.String() == "esc" || msg.String() == "c" {
			m.overlay = ""
			m.nextAction = ""
		}
		if msg.String() == "y" && m.saveCaseDraftConfirmed() {
			m.overlay = ""
			m.mode = "NORMAL"
			if m.nextAction != "" {
				action, n := m.nextAction, m.nextCount
				m.nextAction = ""
				_, cmd := m.action(action, n)
				return true, cmd
			}
		}
		return true, nil
	case "case-edit":
		switch msg.String() {
		case "ctrl+u":
			m.caseText = ""
		case "esc":
			m.overlay = ""
			m.mode = "NORMAL"
		case "enter":
			var c project.Case
			if err := project.Decode([]byte(m.caseText), &c); err != nil {
				m.status = err.Error()
				return true, nil
			}
			if c.ID != m.caseDraft.ID {
				m.status = "Case ID is immutable"
				return true, nil
			}
			if err := project.ValidateCase(c); err != nil {
				m.status = err.Error()
				return true, nil
			}
			m.caseDraft.Case = c
			m.caseMetadataDirty = true
			b, _ := json.Marshal(c.Request)
			m.requestText = string(b)
			if m.saveCaseDraft() {
				m.overlay = ""
				m.mode = "NORMAL"
			}
		default:
			m.caseText = edit(m.caseText, msg)
		}
		return true, nil
	case "case-delete":
		if msg.String() == "esc" || msg.String() == "c" {
			m.overlay = ""
		}
		if msg.String() == "y" {
			result, err := project.DeleteCase(m.root, m.caseDelete.ID, m.caseDelete.Revision, false)
			if err != nil {
				m.status = err.Error()
			} else {
				m.savedCases, _ = project.Cases(m.root)
				if m.caseDraft.ID == result.ID {
					m.clearCaseDraft()
				}
				m.overlay = ""
				m.status = "Case deleted; no scenario references"
			}
		}
		return true, nil
	case "case-leave":
		switch msg.String() {
		case "esc", "c":
			m.overlay = ""
			m.nextAction = ""
			return true, nil
		case "s":
			if !m.saveCaseDraft() {
				return true, nil
			}
		case "d":
			m.clearCaseDraft()
		default:
			return true, nil
		}
		action, n := m.nextAction, m.nextCount
		m.nextAction = ""
		m.overlay = ""
		_, cmd := m.action(action, n)
		return true, cmd
	}
	return false, nil
}
func (m *Model) caseOverlay() string {
	switch m.overlay {
	case "case-impact":
		return "Confirm shared case edit?\n" + m.status + "\ny save | Esc cancel"
	case "case-edit":
		return "Affected scenarios: " + strings.Join(m.caseDraft.References, ", ") + "\nEdit saved case JSON | Ctrl+u clear | Enter save | Esc cancel editing\n" + m.caseText + "\n" + m.status
	case "case-delete":
		return fmt.Sprintf("Delete case %s?\nReferences will be checked before deletion.\ny delete | Esc cancel\n%s", m.caseDelete.Name, m.status)
	case "case-leave":
		return "Unsaved request/case changes\ns save | d discard | c cancel\n" + m.status
	}
	return ""
}

func (m *Model) loadCaseDraft(c project.CaseResource) {
	m.caseDraft = c
	b, _ := json.Marshal(c.Request)
	m.requestText = string(b)
	m.requestBaseline = m.requestText
	m.caseMetadataDirty = false
	m.requestFields = nil
}
