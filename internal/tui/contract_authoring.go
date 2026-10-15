package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"go.yaml.in/yaml/v3"
)

type contractDraft struct {
	Kind                 string `json:"kind" yaml:"kind"`
	Action               string `json:"action" yaml:"action"`
	project.ContractEdit `yaml:",inline"`
}

func (m *Model) contractAction(action string) bool {
	if action != "contract-author" && action != "contract-export" {
		return false
	}
	if len(m.specs) == 0 {
		m.status = "Import a contract first"
		return true
	}
	source := m.specs[0]

	if m.pane == endpointsPane || m.pane == schemasPane {
		_, _, sources := m.contractRows()
		if len(sources) > 0 {
			selected := sources[min(m.index, len(sources)-1)]
			for _, s := range m.specs {
				if s.ID == selected {
					source = s
				}
			}
		}
	}

	if action == "contract-export" {
		data, err := project.ExportContract(m.root, source.ID, m.credentials)
		if err != nil {
			m.status = err.Error()
			return true
		}
		b, _ := yaml.Marshal(data)
		m.detail = string(b)
		m.paletteIndex = 0
		m.overlay = "detail"
		m.status = "Contract documents copied to clipboard. Cases and scenarios are not serialized into OpenAPI."
		return true
	}
	draft := contractDraft{Kind: "endpoint", Action: "create", ContractEdit: project.ContractEdit{SourceID: source.ID, Revision: source.Revision, ImportedEdit: false, Path: "/new", Method: "GET", Value: map[string]any{"responses": map[string]any{"200": map[string]any{"description": "OK"}}}}}
	if m.pane == schemasPane {
		draft.Kind = "schema"
		draft.Path = ""
		draft.Method = ""
		draft.Name = "NewSchema"
		draft.Value = map[string]any{"type": "object"}
	}
	b, _ := json.Marshal(draft)
	m.contractText = string(b)
	m.overlay = "contract-edit"
	m.mode = "INSERT"
	m.status = "Set importedEdit to true to edit this imported contract. Choose create, update or delete."
	return true
}
func (m *Model) contractKey(msg tea.KeyPressMsg) bool {
	switch m.overlay {
	case "contract-edit":
		switch msg.String() {
		case "esc":
			m.overlay = ""
			m.mode = "NORMAL"
		case "enter":
			var d contractDraft
			if err := project.Decode([]byte(m.contractText), &d); err != nil {
				m.status = err.Error()
				return true
			}
			impact, err := project.EditContract(m.root, d.Kind, d.Action, d.ContractEdit, true, m.credentials)
			if err != nil {
				m.status = err.Error()
				return true
			}
			m.contractPreview = impact
			m.overlay = "contract-confirm"
			m.mode = "NORMAL"
		default:
			m.contractText = edit(m.contractText, msg)
		}
		return true
	case "contract-confirm":
		switch msg.String() {
		case "esc":
			m.overlay = "contract-edit"
			m.mode = "INSERT"
		case "enter":
			var d contractDraft
			if err := project.Decode([]byte(m.contractText), &d); err != nil {
				m.status = err.Error()
				return true
			}
			_, err := project.EditContract(m.root, d.Kind, d.Action, d.ContractEdit, false, m.credentials)
			if err != nil {
				m.status = err.Error()
				return true
			}
			m.specs, _ = project.Specifications(m.root)
			m.savedCases, _ = project.Cases(m.root)
			m.overlay = ""
			m.status = "Local contract edit saved; imported base retained"
		}
		return true
	}
	return false
}
func (m *Model) contractOverlay() string {
	if m.overlay == "contract-edit" {
		return "Local contract edit JSON | Enter preview | Esc cancel\n" + m.contractText + "\n" + m.status
	}
	if m.overlay == "contract-confirm" {
		lines := []string{"Local contract edit preview | Enter apply | Esc edit", fmt.Sprintf("%d affected cases, %d available scenarios", len(m.contractPreview.Cases), len(m.contractPreview.Scenarios))}
		for _, c := range m.contractPreview.Cases {
			lines = append(lines, "Case: "+c.Name+" | "+c.ID)
		}
		for _, s := range m.contractPreview.Scenarios {
			lines = append(lines, "Scenario: "+s.Name+" | "+s.ID)
		}
		lines = append(lines, "Referenced endpoint deletion requires reassignTo. Schema references must be repaired first.", m.status)
		return strings.Join(lines, "\n")
	}
	return ""
}
