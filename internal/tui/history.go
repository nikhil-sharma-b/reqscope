package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

func (m *Model) refreshHistory() {
	p, err := project.History(m.root, "", 50, m.historyOffset)
	if err != nil {
		m.status = err.Error()
		return
	}
	m.historyPage = p
	if p.PinnedOverLimit {
		m.status = fmt.Sprintf("Pinned history exceeds limit: %d bytes", p.PinnedBytes)
	}
}
func (m *Model) historyLines() []string {
	lines := []string{}
	for i, r := range m.historyPage.Items {
		line := fmt.Sprintf("%s | %s | %s %s | %s | pinned=%t", r.ID, r.StartedAt.Local().Format("2006-01-02 15:04:05"), r.Response.Method, r.Response.Destination, r.Outcome, r.Pinned)
		if r.ScenarioID != "" {
			line = fmt.Sprintf("%s | %s | scenario | %s | pinned=%t", r.ID, r.StartedAt.Local().Format("2006-01-02 15:04:05"), r.Outcome, r.Pinned)
		}
		mark := "  "
		if i == m.index {
			mark = "> "
		}
		lines = append(lines, mark+line)
	}
	if len(lines) == 0 {
		return []string{"No runs recorded"}
	}
	return lines
}
func (m *Model) historyAction(action string) bool {
	if action == "compare" {
		m.comparisonText = ""
		if len(m.historyPage.Items) > 0 {
			m.comparisonText = m.historyPage.Items[min(m.index, len(m.historyPage.Items)-1)].ID + " "
		}
		m.overlay = "comparison-input"
		m.mode = "INSERT"
		return true
	}
	if action == "history" {
		m.historyOffset = 0
		m.refreshHistory()
		return false
	}
	if !strings.HasPrefix(action, "history-") && !(action == "open" && m.pane == historyPane) {
		return false
	}
	if m.root == "" {
		return true
	}
	switch action {
	case "history-settings":
		m.refreshHistory()
		b, _ := json.Marshal(m.historyPage.Limits)
		m.historyText = string(b)
		m.overlay = "history-settings"
		m.mode = "INSERT"
		return true
	case "history-prune":
		if err := project.PruneHistory(m.root); err != nil {
			m.status = err.Error()
		}
	case "history-next":
		if m.historyPage.NextOffset != nil {
			m.historyOffset = *m.historyPage.NextOffset
		}
		m.index = 0
	case "history-previous":
		m.historyOffset = max(0, m.historyOffset-50)
		m.index = 0
	default:
		if len(m.historyPage.Items) == 0 {
			return true
		}
		r := m.historyPage.Items[min(m.index, len(m.historyPage.Items)-1)]
		if action == "open" {
			full, err := project.GetRun(m.root, r.ID)
			if err != nil {
				m.status = err.Error()
				return true
			}
			if full.Scenario != nil {
				m.scenarioReport = *full.Scenario
				m.overlay = "scenario-report"
				m.status = full.Outcome + " " + full.Error
				return true
			}
			m.exchange = full.Response
			m.pane = responsePane
			m.index = 0
			m.query = ""
			m.status = full.Outcome + " " + full.Error
			return true
		}
		err := project.ChangeRun(m.root, r.ID, strings.TrimPrefix(action, "history-"), m.caseDraft.ID)
		if err != nil {
			m.status = err.Error()
		}
	}
	m.refreshHistory()
	return true
}
func (m *Model) historyKey(msg tea.KeyPressMsg) bool {
	if m.overlay != "history-settings" {
		return false
	}
	switch msg.String() {
	case "esc":
		m.overlay = ""
		m.mode = "NORMAL"
	case "ctrl+u":
		m.historyText = ""
	case "enter":
		var limits project.HistoryLimits
		err := project.Decode([]byte(m.historyText), &limits)
		if err == nil {
			err = project.SetHistoryLimits(m.root, limits)
		}
		if err != nil {
			m.status = err.Error()
		} else {
			m.overlay = ""
			m.mode = "NORMAL"
			m.refreshHistory()
		}
	default:
		m.historyText = edit(m.historyText, msg)
	}
	return true
}
