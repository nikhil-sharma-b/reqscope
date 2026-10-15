package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

func (m *Model) refreshAction(action string) bool {
	if action == "case-reload" || action == "case-reconcile" {
		if m.caseDraft.ID == "" {
			m.status = "Open a saved case first"
			return true
		}
		current, err := project.GetCase(m.root, m.caseDraft.ID)
		if err != nil {
			m.status = err.Error()
			return true
		}
		if action == "case-reload" {
			m.loadCaseDraft(current)
			m.status = "External case loaded; draft discarded"
		} else {
			// Keep local inputs and explicitly adopt the latest expected revision only
			// after presenting the external version for manual reconciliation.
			m.externalCase = current
			m.overlay = "case-reconcile"
			m.status = "Compare the external case with your draft before adopting its revision"
		}
		return true
	}
	if action != "contract-refresh" {
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
			id := sources[min(m.index, len(sources)-1)]
			for _, s := range m.specs {
				if s.ID == id {
					source = s
				}
			}
		}
	}
	m.importPath = source.Location
	preview, err := project.RefreshSpecification(m.root, source.ID, "", true, m.sourceOptions())
	if err != nil {
		m.status = err.Error()
		return true
	}
	m.refreshPreview = preview
	m.overlay = "refresh-confirm"
	m.mode = "NORMAL"
	return true
}

func (m *Model) refreshKey(msg tea.KeyPressMsg) bool {
	if m.overlay == "case-reconcile" {
		switch msg.String() {
		case "esc":
			m.overlay = ""
		case "enter":
			m.caseDraft.Revision = m.externalCase.Revision
			m.overlay = ""
			m.status = "Draft retained against reviewed external revision; edit inputs and save to reconcile"
		}
		return true
	}
	if m.overlay != "refresh-confirm" {
		return false
	}
	switch msg.String() {
	case "esc":
		m.overlay = ""
	case "enter":
		if len(m.refreshPreview.Conflicts) > 0 {
			m.status = "Reconcile contract conflicts before refreshing"
			return true
		}
		_, err := project.RefreshSpecification(m.root, m.refreshPreview.SourceID, m.refreshPreview.Token, false, m.sourceOptions())
		if err != nil {
			m.status = err.Error()
			return true
		}
		previousSnapshot, previousDraft := m.snapshot, m.draft
		m.reload()
		if m.dirty {
			m.snapshot = previousSnapshot
			m.draft = previousDraft
		}
		m.overlay = ""
		m.status = "Contract refreshed; saved inputs retained"
	}
	return true
}

func (m *Model) refreshOverlay() string {
	if m.overlay == "case-reconcile" {
		return fmt.Sprintf("Manual case reconciliation | Enter retain draft against this revision | Esc cancel\nExternal case: %+v\nLocal request: %s\n%s", m.externalCase.Case, m.requestText, m.status)
	}
	if m.overlay != "refresh-confirm" {
		return ""
	}
	p := m.refreshPreview
	lines := []string{"Refresh preview | Enter apply | Esc cancel", fmt.Sprintf("%d added, %d changed, %d removed; %d affected cases", len(p.Added), len(p.Changed), len(p.Removed), len(p.Cases))}
	for _, group := range []struct {
		name      string
		endpoints []project.Endpoint
	}{{"Added", p.Added}, {"Changed", p.Changed}, {"Removed", p.Removed}} {
		for _, ep := range group.endpoints {
			lines = append(lines, group.name+": "+ep.Method+" "+ep.Path)
		}
	}
	for _, c := range p.Cases {
		lines = append(lines, "Case: "+c.Name+" | "+c.ID)
	}
	for _, conflict := range p.Conflicts {
		lines = append(lines, "Conflict: "+conflict)
	}
	lines = append(lines, p.Diagnostics...)
	lines = append(lines, m.status)
	return strings.Join(lines, "\n")
}
