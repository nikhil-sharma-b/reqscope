package tui

import (
	"path/filepath"
	"sort"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

// pollInterval is how often the workspace looks for changes made outside the
// app. A look costs a handful of stat calls; files are read only when their
// size or modification time moved.
const pollInterval = time.Second

// workspace is everything loaded from disk. Nothing on screen reads files
// while drawing: rendering works from this copy alone.
type workspace struct {
	fp project.Fingerprint
	// gen increases on every load the app starts itself. A background load
	// that began before one of those is dropped, so it cannot put older data
	// back on screen.
	gen int
	// specGen increases whenever specifications change, retiring caches built
	// from them.
	specGen   int
	snapshot  project.Snapshot
	specs     []project.Specification
	cases     []project.CaseResource
	scenarios []project.ScenarioResource
	// modules names each API by its title, for people rather than UUIDs.
	modules    map[string]string
	moduleList []string
	endpoints  map[string]*project.Endpoint
	activeEnv  string
	review     project.AccessReview
	reviewEnv  string
	reviewErr  error
}

type loadedMsg struct {
	gen       int
	env       string
	fp        project.Fingerprint
	snapshot  *project.Snapshot
	specs     *[]project.Specification
	cases     *[]project.CaseResource
	scenarios *[]project.ScenarioResource
	review    *project.AccessReview
	reviewErr error
	credDone  bool
	credErr   error
	errs      []error
}

// loadWorkspace reads only the areas whose fingerprint moved since prev. It
// runs off the UI goroutine for polls, so it must not touch the model.
func loadWorkspace(root, env string, prev project.Fingerprint, prevReviewEnv string, creds *project.Credentials, gen int) loadedMsg {
	msg := loadedMsg{gen: gen, env: env}
	fp, err := project.Fingerprints(root)
	if err != nil {
		msg.errs = append(msg.errs, err)
		msg.fp = prev
		return msg
	}
	msg.fp = fp
	projectChanged := fp.Manifest != prev.Manifest || fp.Environments != prev.Environments || fp.Local != prev.Local
	if projectChanged {
		if s, err := project.Load(root); err == nil {
			msg.snapshot = &s
		} else {
			msg.errs = append(msg.errs, err)
		}
		msg.credDone = true
		msg.credErr = creds.Refresh(root)
	}
	if fp.Specs != prev.Specs || fp.Manifest != prev.Manifest {
		if specs, err := project.Specifications(root); err == nil {
			msg.specs = &specs
		} else {
			msg.errs = append(msg.errs, err)
		}
	}
	if fp.Cases != prev.Cases {
		if cases, err := project.Cases(root); err == nil {
			msg.cases = &cases
		} else {
			msg.errs = append(msg.errs, err)
		}
	}
	if fp.Scenarios != prev.Scenarios {
		if flows, err := project.Scenarios(root); err == nil {
			msg.scenarios = &flows
		} else {
			msg.errs = append(msg.errs, err)
		}
	}
	if env != "" && (projectChanged || env != prevReviewEnv) {
		r, err := project.ReviewAccess(root, env)
		msg.review, msg.reviewErr = &r, err
	}
	return msg
}

// poll schedules the next background look for outside changes.
func (m *Model) poll(delay time.Duration) tea.Cmd {
	if m.root == "" {
		return nil
	}
	root, env, fp, reviewEnv, creds, gen := m.root, m.data.activeEnv, m.data.fp, m.data.reviewEnv, m.credentials, m.data.gen
	load := func() tea.Msg { return loadWorkspace(root, env, fp, reviewEnv, creds, gen) }
	if delay == 0 {
		return load
	}
	return tea.Tick(delay, func(time.Time) tea.Msg { return load() })
}

// sync reloads, on the UI goroutine, whatever the app just changed on disk.
// Only areas whose files moved are read again.
func (m *Model) sync() {
	if m.root == "" {
		return
	}
	m.data.gen++
	m.applyLoad(loadWorkspace(m.root, m.data.activeEnv, m.data.fp, m.data.reviewEnv, m.credentials, m.data.gen))
}

// reloadAll forgets every fingerprint so each area is read again.
func (m *Model) reloadAll() {
	m.data.fp = project.Fingerprint{}
	m.data.reviewEnv = ""
	m.sync()
}

func (m *Model) applyLoad(msg loadedMsg) {
	if msg.gen != m.data.gen {
		return
	}
	m.data.fp = msg.fp
	if msg.snapshot != nil {
		m.data.snapshot = *msg.snapshot
		if m.data.activeEnv != "" {
			if _, err := m.data.snapshot.Resolve(m.data.activeEnv); err != nil {
				m.data.activeEnv = ""
			}
		}
	}
	if msg.specs != nil {
		m.data.specs = *msg.specs
		m.data.specGen++
		m.indexModules()
	}
	if msg.cases != nil {
		m.data.cases = *msg.cases
		m.reconcileDrafts()
	}
	if msg.scenarios != nil {
		m.data.scenarios = *msg.scenarios
	}
	if msg.review != nil && msg.env == m.data.activeEnv {
		m.data.review, m.data.reviewErr, m.data.reviewEnv = *msg.review, msg.reviewErr, msg.env
	}
	if msg.credDone {
		m.credErr = msg.credErr
	}
	if len(msg.errs) > 0 {
		m.fail(msg.errs[0])
	}
	m.clampLists()
}

func (m *Model) indexModules() {
	m.data.endpoints = map[string]*project.Endpoint{}
	for i := range m.data.specs {
		for j := range m.data.specs[i].Endpoints {
			e := &m.data.specs[i].Endpoints[j]
			m.data.endpoints[e.ID] = e
		}
	}
	m.data.modules = map[string]string{}
	m.data.moduleList = nil
	for _, s := range m.data.specs {
		name := filepath.Base(s.Location)
		if info, ok := s.Base["info"].(map[string]any); ok {
			if title, ok := info["title"].(string); ok && title != "" {
				name = title
			}
		}
		if _, ok := m.data.modules[s.ModuleID]; !ok {
			m.data.moduleList = append(m.data.moduleList, s.ModuleID)
		}
		m.data.modules[s.ModuleID] = name
	}
	for _, id := range m.data.snapshot.Manifest.Modules {
		if _, ok := m.data.modules[id]; !ok {
			m.data.modules[id] = id
			m.data.moduleList = append(m.data.moduleList, id)
		}
	}
}

// moduleChoices lists every API a blank request can target: imported ones
// and any the active environment has a base URL for.
func (m *Model) moduleChoices() []string {
	out := append([]string(nil), m.data.moduleList...)
	seen := map[string]bool{}
	for _, id := range out {
		seen[id] = true
	}
	if e, ok := m.envByID(m.data.activeEnv); ok {
		for _, id := range sortedKeys(e.BaseURLs) {
			if !seen[id] {
				out = append(out, id)
			}
		}
	}
	return out
}

// moduleName shows an API by its title when one is known.
func (m *Model) moduleName(id string) string {
	if name, ok := m.data.modules[id]; ok {
		return name
	}
	return id
}

func (m *Model) envByID(id string) (project.Resource, bool) {
	for _, e := range m.data.snapshot.Environments {
		if e.ID == id {
			return e, true
		}
	}
	return project.Resource{}, false
}

// environments are listed with production last, the order people reach for them.
func (m *Model) environments() []project.Resource {
	envs := append([]project.Resource(nil), m.data.snapshot.Environments...)
	sort.SliceStable(envs, func(i, j int) bool {
		return envs[i].Classification != "production" && envs[j].Classification == "production"
	})
	return envs
}
