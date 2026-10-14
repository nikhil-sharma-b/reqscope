package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"encoding/json"
	"fmt"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"strings"
)

type exchangeMsg struct {
	result project.Exchange
	err    error
}

func (m *Model) requestAction(action string) (bool, tea.Cmd) {
	switch action {
	case "request-new", "request-compose":
		if m.requestCancel != nil {
			m.status = "Cancel the running request first"
			return true, nil
		}
		if m.requestText != "" {
			m.overlay = "request-edit"
			m.mode = "INSERT"
			m.pane = requestPane
			return true, nil
		}
		d := project.RequestDraft{Method: "GET", BodyType: "raw", Path: map[string]string{}}
		if action == "request-compose" {
			matches := project.SearchEndpoints(m.specs, project.EndpointQuery{Query: m.endpointQuery, Method: m.methodFilter, GroupByTag: m.groupTags})
			if len(matches) == 0 {
				m.status = "Select an endpoint"
				return true, nil
			}
			var err error
			d, m.requestFields, err = project.ContractDraft(m.root, matches[min(m.index, len(matches)-1)].ID)
			if err != nil {
				m.status = err.Error()
				return true, nil
			}
		}
		b, _ := json.Marshal(d)
		m.requestText = string(b)
		m.pane = requestPane
		m.overlay = "request-edit"
		m.mode = "INSERT"
	case "request-edit":
		m.overlay = "request-edit"
		m.mode = "INSERT"
		m.pane = requestPane
	case "request-reset":
		if m.requestCancel == nil {
			m.clearCaseDraft()
			m.status = "Request draft cleared"
		}
	case "request-send":
		return true, m.prepareSend()
	case "request-cancel":
		if m.requestCancel != nil {
			m.requestCancel()
			m.status = "Cancelling request"
		}
	case "response-copy":
		if m.exchange.Binary || m.exchange.BodyWithheld {
			m.status = "Withheld or binary content cannot be copied"
		} else {
			m.status = "Copied retained response body"
			return true, tea.SetClipboard(m.exchange.Body)
		}
	default:
		return false, nil
	}
	return true, nil
}
func (m *Model) prepareSend() tea.Cmd {
	if m.requestCancel != nil {
		m.status = "A request is already running"
		return nil
	}
	var d project.RequestDraft
	if err := project.Decode([]byte(m.requestText), &d); err != nil {
		m.status = err.Error()
		return nil
	}
	if len(m.caseDraft.Validation) > 0 {
		m.status = "Case validation is not supported yet"
		return nil
	}
	p, err := project.PrepareRequestWithCredentials(m.root, m.activeEnvironment, d, true, m.credentials)
	if err != nil {
		m.status = err.Error()
		return nil
	}
	m.prepared = p
	if p.Classification == "production" {
		m.overlay = "request-confirm"
		m.mode = "NORMAL"
		return nil
	}
	return m.startSend(d, "")
}
func (m *Model) startSend(d project.RequestDraft, confirmation string) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	m.requestCancel = cancel
	m.status = "Sending request | Esc or request cancel stops transport"
	m.overlay = ""
	root, env := m.root, m.activeEnvironment
	caseID, draft := m.caseDraft.ID, m.requestDirty() || m.caseDraft.ID == ""
	return func() tea.Msg {
		result, err := project.Send(ctx, root, env, d, project.SendOptions{Interactive: true, Confirmation: confirmation, Credentials: m.credentials})
		result.CaseID = caseID
		result.Draft = draft
		return exchangeMsg{result, err}
	}
}
func (m *Model) requestKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if m.overlay == "request-edit" {
		switch msg.String() {
		case "ctrl+u":
			m.requestText = ""
		case "esc", "enter":
			m.overlay = ""
			m.mode = "NORMAL"
		default:
			m.requestText = edit(m.requestText, msg)
		}
		return true, nil
	}
	if m.overlay == "request-confirm" {
		switch msg.String() {
		case "esc", "ctrl+c":
			m.overlay = ""
			m.status = "Send cancelled"
		case "y":
			var d project.RequestDraft
			if err := project.Decode([]byte(m.requestText), &d); err != nil {
				m.status = err.Error()
				m.overlay = ""
				return true, nil
			}
			return true, m.startSend(d, m.prepared.Fingerprint)
		}
		return true, nil
	}
	if m.requestCancel != nil && (msg.String() == "esc" || msg.String() == "ctrl+c" || msg.String() == "q") {
		m.requestCancel()
		m.status = "Cancelling request"
		return true, nil
	}
	return false, nil
}
func (m *Model) requestOverlay() string {
	if m.overlay == "request-confirm" {
		return fmt.Sprintf("Confirm production send?\n%s %s\ny send | Esc cancel", m.prepared.Method, m.prepared.URL)
	}
	if m.overlay != "request-edit" {
		return ""
	}
	lines := []string{"Edit request JSON | Ctrl+u clear | Enter keep | Esc keep", m.requestText, "Guided contract inputs, examples are suggestions:"}
	for _, f := range m.requestFields {
		required := "optional"
		if f.Required {
			required = "required"
		}
		suggestion := ""
		if f.Suggestion != nil {
			b, _ := json.Marshal(f.Suggestion)
			suggestion = " | suggestion: " + string(b)
		}
		lines = append(lines, fmt.Sprintf("%s %s | %s | %s%s", f.In, f.Name, required, f.Description, suggestion))
	}
	return strings.Join(lines, "\n")
}
func (m *Model) responseLines() []string {
	if m.exchange.Status == 0 {
		return []string{"No response recorded"}
	}
	b, _ := json.Marshal(m.exchange.Headers)
	identity := "Draft"
	if m.exchange.CaseID != "" {
		identity = "Case " + m.exchange.CaseID
	}
	if m.exchange.Draft {
		identity += " | draft inputs"
	}
	lines := []string{identity, fmt.Sprintf("HTTP %d | %d ms | %d retained bytes | binary=%t truncated=%t", m.exchange.Status, m.exchange.DurationMS, m.exchange.RetainedBytes, m.exchange.Binary, m.exchange.Truncated), string(b)}
	if m.exchange.BodyWithheld {
		lines = append(lines, "Body withheld: structured authentication data could not be safely sanitized")
	} else if m.exchange.Binary {
		lines = append(lines, "Binary body: text search and copy unavailable")
	} else {
		lines = append(lines, strings.Split(m.exchange.Body, "\n")...)
	}
	if m.exchange.Truncated {
		lines = append(lines, "Truncated: full-body validation incomplete")
	}
	result := []string{}
	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), strings.ToLower(m.query)) {
			result = append(result, line)
		}
	}
	return result
}
