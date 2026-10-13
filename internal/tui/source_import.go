package tui

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"strings"

	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

func (m *Model) sourceOptions() project.SourceImportOptions {
	opts := project.SourceImportOptions{Credentials: m.credentials, Interactive: true}
	if m.activeEnvironment != "" {
		if review, err := project.ReviewAccess(m.root, m.activeEnvironment); err == nil {
			origin, _ := project.Origin(m.importPath)
			for module, target := range review.Origins {
				if target == origin {
					opts.Environment = m.activeEnvironment
					opts.Module = module
					break
				}
			}
		}
	}
	return opts
}
func (m *Model) discoverImport() {
	discovery, err := project.DiscoverSource(m.root, m.importPath, m.sourceOptions())
	m.sourceDiscovery = discovery
	if err != nil {
		m.status = err.Error() + "\nSupply the raw JSON/YAML URL or approve the denied origin and retry."
		for _, c := range discovery.Candidates {
			m.status += "\nCandidate: " + c.Name + " | " + c.Location
		}
		for _, resource := range discovery.Resources {
			m.status += "\nResource: " + resource
		}
		m.overlay = "import"
		return
	}
	if len(discovery.Candidates) == 0 {
		m.status = discovery.Hint
		m.overlay = "import"
		return
	}
	m.paletteIndex = 0
	if len(discovery.Candidates) > 1 {
		m.overlay = "source-select"
		return
	}
	m.sourceCandidate = discovery.Candidates[0]
	m.previewSource()
}
func (m *Model) previewSource() {
	preview, err := project.ImportSource(m.root, m.sourceCandidate, true, "", m.sourceOptions())
	if err != nil {
		m.status = "Supply the raw JSON/YAML URL.\nCandidate: " + m.sourceCandidate.Location + "\n" + err.Error()
		m.overlay = "import"
		return
	}
	m.importPreview = preview
	m.overlay = "import-confirm"
}
func (m *Model) sourceImportKey(msg tea.KeyPressMsg) bool {
	key := msg.String()
	switch m.overlay {
	case "source-approve":
		if key == "esc" {
			m.overlay = "import"
			return true
		}
		if key == "enter" {
			if err := project.ApproveSourceOrigin(m.root, m.importPath); err != nil {
				m.status = err.Error()
				return true
			}
			m.discoverImport()
		}
		return true
	case "source-select":
		switch key {
		case "esc":
			m.overlay = "import"
		case "j", "down":
			m.paletteIndex = min(len(m.sourceDiscovery.Candidates)-1, m.paletteIndex+1)
		case "k", "up":
			m.paletteIndex = max(0, m.paletteIndex-1)
		case "enter":
			m.sourceCandidate = m.sourceDiscovery.Candidates[m.paletteIndex]
			m.previewSource()
		}
		return true
	}
	return false
}
func (m *Model) sourceImportOverlay() string {
	switch m.overlay {
	case "source-approve":
		origin, err := project.Origin(m.importPath)
		if err != nil {
			return err.Error()
		}
		return "Approve specification source origin\n" + origin + "\nThis permits future CLI imports from this origin for this project.\nMatching protected credentials in the active environment may be used.\nEnter approve and discover | Esc cancel\n" + m.status
	case "source-select":
		lines := []string{"Choose a specification | j/k move | Enter preview | Esc cancel"}
		for i, c := range m.sourceDiscovery.Candidates {
			mark := "  "
			if i == m.paletteIndex {
				mark = "> "
			}
			lines = append(lines, fmt.Sprintf("%s%s | %s", mark, c.Name, c.Location))
		}
		return strings.Join(lines, "\n")
	}
	return ""
}
