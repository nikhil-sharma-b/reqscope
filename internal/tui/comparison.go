package tui

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"strings"
)

func (m *Model) comparisonKey(msg tea.KeyPressMsg) bool {
	switch m.overlay {
	case "comparison-report":
		switch msg.String() {
		case "esc":
			m.overlay = ""
		case "j", "down":
			m.paletteIndex = min(m.paletteIndex+1, max(0, len(m.comparisonLines)-1))
		case "k", "up":
			m.paletteIndex = max(0, m.paletteIndex-1)
		}
		return true
	case "comparison-input":
		switch msg.String() {
		case "esc":
			m.overlay = ""
			m.mode = "NORMAL"
		case "ctrl+u":
			m.comparisonText = ""
		case "enter":
			ids := strings.Fields(m.comparisonText)
			if len(ids) != 2 {
				m.status = "Enter two retained run IDs, separated by a space"
				return true
			}
			result, err := project.CompareRuns(m.root, ids[0], ids[1], nil)
			if err != nil {
				m.status = err.Error()
				return true
			}
			b, _ := json.MarshalIndent(result, "", "  ")
			m.comparisonLines = strings.Split(string(b), "\n")
			m.paletteIndex = 0
			m.overlay = "comparison-report"
			m.mode = "NORMAL"
		default:
			m.comparisonText = edit(m.comparisonText, msg)
		}
		return true
	}
	return false
}
