package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/google/uuid"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

type scenarioPromptMsg struct{ request project.PreparedRequest }
type scenarioFinishedMsg struct {
	report project.ScenarioReport
	err    error
}
type scenarioApproval struct {
	fingerprint string
	err         error
}

func (m *Model) scenarioItems() []project.ScenarioResource {
	items := []project.ScenarioResource{}
	for _, s := range m.savedScenarios {
		if strings.Contains(strings.ToLower(s.Name), strings.ToLower(m.query)) {
			items = append(items, s)
		}
	}
	return items
}
func (m *Model) scenarioAction(action string) (bool, tea.Cmd) {
	if action == "open" && m.pane == scenariosPane {
		action = "scenario-edit"
	}
	switch action {
	case "scenario-create":
		m.scenarioDraft = project.ScenarioResource{Scenario: project.Scenario{ID: uuid.NewString(), Name: "New scenario", Steps: []project.ScenarioStep{}}}
		b, _ := json.Marshal(m.scenarioDraft.Scenario)
		m.scenarioText = string(b)
		m.overlay = "scenario-edit"
		m.mode = "INSERT"
		m.pane = scenariosPane
	case "scenario-edit", "scenario-delete", "scenario-validate", "scenario-run":
		if m.requestCancel != nil {
			m.status = "Cancel the running request or scenario first"
			return true, nil
		}
		items := m.scenarioItems()
		if len(items) == 0 {
			m.status = "Select a scenario"
			return true, nil
		}
		s, err := project.GetScenario(m.root, items[min(m.index, len(items)-1)].ID)
		if err != nil {
			m.status = err.Error()
			return true, nil
		}
		m.scenarioDraft = s
		switch action {
		case "scenario-edit":
			b, _ := json.Marshal(s.Scenario)
			m.scenarioText = string(b)
			m.overlay = "scenario-edit"
			m.mode = "INSERT"
		case "scenario-delete":
			m.overlay = "scenario-delete"
		case "scenario-validate":
			_, err := project.CheckScenario(m.root, s.ID)
			if err != nil {
				m.status = err.Error()
			} else {
				m.status = "Scenario valid; execution access checked separately"
			}
		case "scenario-run":
			run, err := project.SnapshotScenario(m.root, s.ID)
			if err != nil {
				m.status = err.Error()
				return true, nil
			}
			ctx, cancel := context.WithCancel(context.Background())
			m.requestCancel = cancel
			events := make(chan tea.Msg, 1)
			approvals := make(chan scenarioApproval, 1)
			m.scenarioEvents = events
			m.scenarioApprovals = approvals
			root, env, credentials := m.root, m.activeEnvironment, m.credentials
			m.status = "Running scenario | Esc cancels active transport and further steps"
			return true, func() tea.Msg {
				go func() {
					report, err := run.Run(ctx, root, env, project.ScenarioOptions{SendOptions: project.SendOptions{Interactive: true, Credentials: credentials}, Confirm: func(ctx context.Context, p project.PreparedRequest) (string, error) {
						select {
						case events <- scenarioPromptMsg{p}:
						case <-ctx.Done():
							return "", project.Fail("cancelled", "scenario cancelled")
						}
						select {
						case a := <-approvals:
							return a.fingerprint, a.err
						case <-ctx.Done():
							return "", project.Fail("cancelled", "scenario cancelled")
						}
					}})
					events <- scenarioFinishedMsg{report, err}
				}()
				return <-events
			}
		}
	case "scenario-cancel":
		if m.requestCancel != nil {
			m.requestCancel()
			m.status = "Cancelling scenario"
		}
	default:
		return false, nil
	}
	return true, nil
}
func (m *Model) awaitScenario() tea.Cmd {
	events := m.scenarioEvents
	return func() tea.Msg { return <-events }
}
func (m *Model) scenarioKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	switch m.overlay {
	case "scenario-confirm":
		switch msg.String() {
		case "y":
			m.scenarioApprovals <- scenarioApproval{fingerprint: m.prepared.Fingerprint}
			m.overlay = ""
			return true, m.awaitScenario()
		case "esc", "ctrl+c", "q":
			m.requestCancel()
			m.overlay = ""
			m.status = "Cancelling scenario"
			return true, m.awaitScenario()
		}
		return true, nil
	case "scenario-edit":
		switch msg.String() {
		case "ctrl+u":
			m.scenarioText = ""
		case "esc":
			m.overlay = ""
			m.mode = "NORMAL"
		case "enter":
			var s project.Scenario
			if err := project.Decode([]byte(m.scenarioText), &s); err != nil {
				m.status = err.Error()
				return true, nil
			}
			if s.ID != m.scenarioDraft.ID {
				m.status = "Scenario ID is immutable"
				return true, nil
			}
			saved, err := project.SaveScenario(m.root, s, m.scenarioDraft.Revision, false)
			if err != nil {
				m.status = err.Error()
				return true, nil
			}
			m.scenarioDraft = saved
			m.savedScenarios, _ = project.Scenarios(m.root)
			m.overlay = ""
			m.mode = "NORMAL"
			m.status = "Scenario saved"
		default:
			m.scenarioText = edit(m.scenarioText, msg)
		}
		return true, nil
	case "scenario-delete":
		if msg.String() == "esc" {
			m.overlay = ""
		}
		if msg.String() == "y" {
			_, err := project.DeleteScenario(m.root, m.scenarioDraft.ID, m.scenarioDraft.Revision, false)
			if err != nil {
				m.status = err.Error()
			} else {
				m.savedScenarios, _ = project.Scenarios(m.root)
				m.overlay = ""
				m.status = "Scenario deleted"
			}
		}
		return true, nil
	}
	return false, nil
}
func (m *Model) scenarioOverlay() string {
	switch m.overlay {
	case "scenario-edit":
		return "Edit scenario JSON | Ctrl+u clear | Enter save | Esc cancel\n" + m.scenarioText + "\n" + m.status
	case "scenario-delete":
		return "Delete scenario " + m.scenarioDraft.Name + "?\ny delete | Esc cancel\n" + m.status
	case "scenario-confirm":
		return fmt.Sprintf("Confirm production scenario send?\n%s %s\ny send | Esc cancels scenario", m.prepared.Method, m.prepared.URL)
	case "scenario-report":
		lines := []string{"Scenario report | Esc close", fmt.Sprintf("Successful: %t", m.scenarioReport.Successful), m.status}
		for _, step := range m.scenarioReport.Steps {
			lines = append(lines, step.ID+": "+step.Outcome)
			if step.Reason != "" {
				lines = append(lines, step.Reason)
			}
			if step.Error != nil {
				lines = append(lines, step.Error.Code+": "+step.Error.Message)
			}
			if step.Exchange.Status != 0 {
				lines = append(lines, fmt.Sprintf("HTTP %d | %d ms", step.Exchange.Status, step.Exchange.DurationMS))
			}
			for _, check := range step.Exchange.Checks {
				lines = append(lines, check.Kind+": "+check.Outcome)
			}
		}
		return strings.Join(lines, "\n")
	}
	return ""
}
