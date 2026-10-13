package tui

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"strings"
)

func (m *Model) selectedEnvironment() (project.Resource, bool) {
	items := m.environmentItems()
	if len(items) == 0 {
		return project.Resource{}, false
	}
	return items[min(m.index, len(items)-1)], true
}
func (m *Model) environmentAction(action string) bool {
	switch action {
	case "environment-create", "environment-edit":
		var e project.Resource
		if action == "environment-edit" {
			var ok bool
			e, ok = m.selectedEnvironment()
			if !ok {
				return true
			}
		} else {
			e.Environment = project.Environment{ID: uuid.NewString(), Name: "New environment", Classification: "nonproduction", BaseURLs: map[string]string{}, Variables: map[string]string{}}
		}
		m.environmentDraft = e
		b, _ := json.Marshal(e.Environment)
		m.environmentText = string(b)
		m.overlay = "environment-edit"
		m.mode = "INSERT"
	case "environment-select":
		e, ok := m.selectedEnvironment()
		if !ok {
			return true
		}
		if err := project.SelectEnvironment(m.root, e.ID); err != nil {
			m.status = err.Error()
			return true
		}
		m.activeEnvironment = e.ID
		m.status = "Selected " + e.Name
	case "environment-authorize":
		e, ok := m.selectedEnvironment()
		if !ok {
			return true
		}
		r, err := project.ReviewAccess(m.root, e.ID)
		if err != nil {
			m.status = err.Error()
			return true
		}
		m.accessReview = r
		m.overlay = "environment-authorize"
		m.mode = "NORMAL"
	default:
		return false
	}
	return true
}
func (m *Model) environmentUpdate(msg tea.KeyPressMsg) bool {
	switch m.overlay {
	case "environment-edit":
		switch msg.String() {
		case "ctrl+u":
			m.environmentText = ""
		case "esc":
			m.overlay = ""
			m.mode = "NORMAL"
		case "enter":
			var e project.Environment
			if err := project.Decode([]byte(m.environmentText), &e); err != nil {
				m.status = err.Error()
				return true
			}
			if e.ID != m.environmentDraft.ID {
				m.status = "Environment ID is immutable"
				return true
			}
			if err := project.ValidateEnvironment(e); err != nil {
				m.status = err.Error()
				return true
			}
			m.environmentDraft.Environment = e
			if err := project.SaveEnvironment(m.root, e, m.environmentDraft.Revision, true, false); err != nil {
				if pe, ok := err.(*project.Error); ok && pe.Code == "access_denied" {
					m.overlay = "environment-lower"
					m.mode = "NORMAL"
				} else {
					m.status = err.Error()
				}
				return true
			}
			m.saveEnvironment(false)
		default:
			m.environmentText = edit(m.environmentText, msg)
		}
		return true
	case "environment-lower":
		if msg.String() == "esc" {
			m.overlay = "environment-edit"
			m.mode = "INSERT"
		}
		if msg.String() == "y" {
			m.saveEnvironment(true)
		}
		return true
	case "environment-authorize":
		if msg.String() == "esc" {
			m.overlay = ""
		}
		if msg.String() == "y" {
			if err := project.ApproveAccess(m.root, m.accessReview); err != nil {
				m.status = err.Error()
				m.overlay = ""
			} else {
				m.status = "CLI access authorized for reviewed destinations"
				m.overlay = ""
			}
		}
		return true
	}
	return false
}
func (m *Model) saveEnvironment(confirm bool) {
	e := m.environmentDraft
	if err := project.SaveEnvironment(m.root, e.Environment, e.Revision, false, confirm); err != nil {
		m.status = err.Error()
		return
	}
	m.reload()
	m.overlay = ""
	m.mode = "NORMAL"
	m.status = "Environment saved"
}
func (m *Model) activeDestination() string {
	if m.activeEnvironment == "" {
		return "Env: choose"
	}
	e, err := m.snapshot.Resolve(m.activeEnvironment)
	if err != nil {
		return "Env: choose"
	}
	r, err := project.ReviewAccess(m.root, e.ID)
	if err != nil {
		return "Env: " + e.Name + " | invalid destination"
	}
	b, _ := json.Marshal(r.BaseURLs)
	return fmt.Sprintf("Env: %s | %s | %s", e.Name, e.Classification, b)
}
func (m *Model) environmentOverlay() string {
	switch m.overlay {
	case "environment-edit":
		return "Edit environment JSON | ID is fixed\n" + m.environmentText + "\nCtrl+u clear | Enter save | Esc cancel\n" + m.status
	case "environment-lower":
		return "Lower protected production classification?\n" + m.environmentDraft.Name + "\nExisting CLI approval will be removed.\ny confirm | Esc return to editing\n" + m.status
	case "environment-authorize":
		r := m.accessReview
		b, _ := json.Marshal(r.BaseURLs)
		origins, _ := json.Marshal(r.Origins)
		bindings, _ := json.Marshal(r.Bindings)
		return strings.Join([]string{"Authorize CLI destinations?", "Environment: " + r.EnvironmentID, "Classification: " + r.Classification, "Base URLs: " + string(b), "Exact origins: " + string(origins), "Protected bindings: " + string(bindings), "Fingerprint: " + r.Fingerprint, "y approve | Esc cancel"}, "\n")
	}
	return ""
}
