package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/google/uuid"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"go.yaml.in/yaml/v3"
)

func (m *Model) caseAction(id action) (bool, tea.Cmd) {
	switch id {
	case actRunCase, actDuplicateCase, actDeleteCase, actEditCase:
	default:
		return false, nil
	}
	c, ok := m.selectedCase()
	if !ok {
		m.say("No saved request selected")
		return true, nil
	}
	switch id {
	case actRunCase:
		m.openCase(c)
		return true, m.send()
	case actDuplicateCase:
		copy, err := project.DuplicateCase(m.root, c.ID, c.Name+" copy", false)
		if err != nil {
			m.fail(err)
			return true, nil
		}
		m.sync()
		for i, item := range m.filteredCases() {
			if item.ID == copy.ID {
				m.lists[savedTab].cursor = i
			}
		}
		m.ok("Duplicated as “" + copy.Name + "”")
	case actDeleteCase:
		m.modal = &confirmModal{title: "Delete “" + c.Name + "”?", yes: "delete", danger: true,
			lines: []string{"The saved request is removed from the project.", quiet.Render("Scenarios that use it block the delete.")},
			onYes: func(m *Model) tea.Cmd {
				if _, err := project.DeleteCase(m.root, c.ID, c.Revision, false); err != nil {
					m.fail(err)
					return nil
				}
				if d := m.drafts["case:"+c.ID]; d != nil {
					delete(m.drafts, d.key)
					if m.req == d {
						m.req = nil
					}
				}
				m.sync()
				m.ok("Deleted “" + c.Name + "”")
				return nil
			}}
	case actEditCase:
		return true, m.editCaseDetails(c)
	}
	return true, nil
}

// editCaseDetails edits everything about a saved request as YAML, including
// checks and comparison rules that have no form of their own.
func (m *Model) editCaseDetails(c project.CaseResource) tea.Cmd {
	current := c.Case
	d := m.drafts["case:"+c.ID]
	if d != nil {
		current.Request = d.request
	}
	b := toYAML(current)
	text := "# The id cannot change.\n" +
		"# validation runs on every send, for example:\n" +
		"#   validation:\n#     assertions:\n#       - {kind: status-equals, value: 200}\n#       - {kind: header-present, name: content-type}\n" +
		"# comparisonIgnore lists jsonPointers and headers to skip when comparing runs.\n" + string(b)
	return m.editText(text, "yaml", func(m *Model, text string) (tea.Cmd, error) {
		var next project.Case
		if err := project.Decode([]byte(text), &next); err != nil {
			return nil, err
		}
		if next.ID != c.ID {
			return nil, project.Fail("invalid_input", "the id cannot change")
		}
		if err := project.ValidateCase(next); err != nil {
			return nil, err
		}
		if d == nil {
			d = newDraft("case:"+c.ID, c.Request)
			m.loadCase(d, c)
			m.drafts[d.key] = d
		}
		d.request = next.Request
		return m.saveCase(d, next, false), nil
	})
}

type scenarioPromptMsg struct{ request project.PreparedRequest }
type scenarioFinishedMsg struct {
	report project.ScenarioReport
	err    error
}
type scenarioApproval struct {
	fingerprint string
	err         error
}

func (m *Model) scenarioAction(id action) (bool, tea.Cmd) {
	switch id {
	case actNewScenario:
		if m.busy != nil {
			m.say("Wait for the running request, or press esc to cancel it")
			return true, nil
		}
		s := project.Scenario{ID: uuid.NewString(), Name: "New scenario", Steps: []project.ScenarioStep{}}
		return true, m.writeScenario(s, "")
	case actRunScenario, actDeleteScenario, actCheckScenario, actApproveLogins:
	default:
		return false, nil
	}
	s, ok := m.selectedScenario()
	if !ok {
		m.say("No scenario selected")
		return true, nil
	}
	switch id {
	case actRunScenario:
		return true, m.runScenario(s)
	case actDeleteScenario:
		m.modal = &confirmModal{title: "Delete “" + s.Name + "”?", yes: "delete", danger: true, lines: []string{"The scenario is removed. Its saved requests stay."},
			onYes: func(m *Model) tea.Cmd {
				if _, err := project.DeleteScenario(m.root, s.ID, s.Revision, false); err != nil {
					m.fail(err)
					return nil
				}
				m.sync()
				m.ok("Deleted “" + s.Name + "”")
				return nil
			}}
	case actCheckScenario:
		if _, err := project.CheckScenario(m.root, s.ID); err != nil {
			m.fail(err)
		} else {
			m.ok("“" + s.Name + "” is valid. Access is checked when it runs.")
		}
	case actApproveLogins:
		m.approveLogins(s)
	}
	return true, nil
}

func (m *Model) editScenario(s project.ScenarioResource) tea.Cmd {
	current, err := project.GetScenario(m.root, s.ID)
	if err != nil {
		m.fail(err)
		return nil
	}
	return m.writeScenario(current.Scenario, current.Revision)
}

// writeScenario edits a scenario as YAML. Steps name saved requests by ID;
// the header lists them so they can be copied in.
func (m *Model) writeScenario(s project.Scenario, revision string) tea.Cmd {
	b := toYAML(s)
	var header strings.Builder
	header.WriteString("# A scenario runs saved requests in order. The id cannot change.\n")
	header.WriteString("# Each step: id, caseId, and optionally overrides (variable values),\n")
	header.WriteString("# extracts (name: /json/pointer) and bindings (variable: {stepId, extract}).\n")
	header.WriteString("#\n# Saved requests:\n")
	for _, c := range m.data.cases {
		method, path := m.caseRoute(c.Request)
		fmt.Fprintf(&header, "#   %s  %s %s %s\n", c.ID, c.Name, method, path)
	}
	if len(m.data.cases) == 0 {
		header.WriteString("#   none yet; save a request first\n")
	}
	header.WriteString("#\n# Example step:\n#   - id: login\n#     caseId: <one of the IDs above>\n#     extracts: {token: /token}\n")
	return m.editText(header.String()+string(b), "yaml", func(m *Model, text string) (tea.Cmd, error) {
		var next project.Scenario
		if err := project.Decode([]byte(text), &next); err != nil {
			return nil, err
		}
		if next.ID != s.ID {
			return nil, project.Fail("invalid_input", "the id cannot change")
		}
		saved, err := project.SaveScenario(m.root, next, revision, false)
		if err != nil {
			return nil, err
		}
		m.sync()
		for i, item := range m.filteredScenarios() {
			if item.ID == saved.ID {
				m.lists[scenariosTab].cursor = i
			}
		}
		m.ok("Saved “" + saved.Name + "”")
		return nil, nil
	})
}

func (m *Model) approveLogins(s project.ScenarioResource) {
	if m.data.activeEnv == "" {
		m.say("Choose an environment first (" + m.keyName(actEnvironment) + ")")
		return
	}
	review, err := project.ReviewScenarioExtractions(m.root, m.data.activeEnv, s.ID)
	if err != nil {
		m.fail(err)
		return
	}
	lines := []string{"The CLI may reuse these protected values from login responses,", "and only send them to these destinations:", ""}
	for _, step := range review.Scenario.Steps {
		if len(step.SecretExtracts) == 0 {
			continue
		}
		lines = append(lines, bold.Render("Step "+step.ID))
		for _, name := range sortedKeys(step.SecretExtracts) {
			x := step.SecretExtracts[name]
			lines = append(lines, "  "+name+quiet.Render(" from ")+x.Pointer)
			for _, dest := range x.Destinations {
				field := dest.Field
				if field == "" {
					field = "body"
				}
				lines = append(lines, quiet.Render("    → ")+dest.Kind+" in "+dest.In+" "+field+quiet.Render(" at ")+dest.Destination)
			}
		}
	}
	lines = append(lines, "", quiet.Render("Environment  ")+m.envName(review.Access.EnvironmentID)+quiet.Render("  "+review.Access.Classification))
	m.modal = &confirmModal{title: "Approve protected logins?", lines: lines, yes: "approve", onYes: func(m *Model) tea.Cmd {
		if err := project.ApproveScenarioExtractions(m.root, m.data.activeEnv, s.ID, review.Fingerprint); err != nil {
			m.fail(err)
			return nil
		}
		m.ok("Approved for “" + s.Name + "” in " + m.envName(m.data.activeEnv))
		return nil
	}}
}

// runScenario runs on its own goroutine. Before each production send it
// pauses and hands the decision back to the person through a message.
func (m *Model) runScenario(s project.ScenarioResource) tea.Cmd {
	if m.busy != nil {
		m.say("Wait for the running request, or press esc to cancel it")
		return nil
	}
	if m.data.activeEnv == "" {
		m.modal = m.envPicker()
		if p, ok := m.modal.(*pickerModal); ok {
			p.note = "Choose where the scenario sends requests, then press " + m.keyName(actRunScenario) + " again."
		}
		return nil
	}
	run, err := project.SnapshotScenario(m.root, s.ID)
	if err != nil {
		m.fail(err)
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan tea.Msg, 1)
	approvals := make(chan scenarioApproval, 1)
	m.scenarioEvents, m.scenarioApprovals = events, approvals
	m.busy = &busy{label: "Running " + s.Name + "…", cancel: cancel}
	root, env, creds := m.root, m.data.activeEnv, m.credentials
	return func() tea.Msg {
		go func() {
			report, err := run.Run(ctx, root, env, project.ScenarioOptions{SendOptions: project.SendOptions{Interactive: true, Credentials: creds}, Confirm: func(ctx context.Context, p project.PreparedRequest) (string, error) {
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

func (m *Model) awaitScenario() tea.Cmd {
	events := m.scenarioEvents
	return func() tea.Msg { return <-events }
}

func (m *Model) confirmScenarioSend(p project.PreparedRequest) tea.Cmd {
	approvals := m.scenarioApprovals
	m.scenarioPaused = true
	m.modal = &confirmModal{title: "Send to production?", yes: "send", danger: true, private: true,
		lines: []string{methodStyle(p.Method).Render(p.Method) + "  " + p.URL, "", quiet.Render("The scenario is paused before this step. Cancelling stops the remaining steps.")},
		onYes: func(m *Model) tea.Cmd {
			m.scenarioPaused = false
			approvals <- scenarioApproval{fingerprint: p.Fingerprint}
			return m.awaitScenario()
		},
		onNo: func(m *Model) tea.Cmd { return m.cancelBusy() }}
	return nil
}

func (m *Model) finishScenario(msg scenarioFinishedMsg) {
	if m.busy != nil && m.busy.cancel != nil {
		m.busy.cancel()
	}
	m.busy = nil
	m.scenarioEvents, m.scenarioApprovals, m.scenarioPaused = nil, nil, false
	m.modal = m.reportViewer(m.scenarioName(msg.report.ScenarioID), msg.report)
	switch {
	case msg.err != nil:
		m.fail(msg.err)
	case msg.report.Successful:
		m.ok("Scenario passed")
	default:
		m.fail(fmt.Errorf("scenario failed"))
	}
}

func (m *Model) reportViewer(name string, r project.ScenarioReport) modal {
	verdict := okStyle.Bold(true).Render("Passed")
	if !r.Successful {
		verdict = badStyle.Bold(true).Render("Failed")
	}
	lines := []string{verdict + quiet.Render(fmt.Sprintf("   %d step%s", r.TotalSteps, plural(r.TotalSteps, "", "s"))), ""}
	for _, step := range r.Steps {
		spans := []span{sp(pad(step.ID, 14), bold), plain(" "), sp(pad(step.Outcome, 9), outcomeStyle(step.Outcome))}
		if step.Exchange.Status != 0 {
			spans = append(spans, sp(statusText(step.Exchange.Status), statusStyle(step.Exchange.Status)), sp(fmt.Sprintf("  %d ms", step.Exchange.DurationMS), quiet))
		}
		lines = append(lines, row(spans, 90, false))
		if step.Reason != "" {
			lines = append(lines, quiet.Render("    "+step.Reason))
		}
		if step.Error != nil {
			lines = append(lines, badStyle.Render("    "+sentence(step.Error.Message)))
		}
		for _, c := range step.Exchange.Checks {
			mark, style := "✓", okStyle
			if c.Outcome == "failed" {
				mark, style = "✗", badStyle
			}
			lines = append(lines, "    "+style.Render(mark)+" "+c.Kind+" "+quiet.Render(c.Outcome+" "+c.Message))
		}
	}
	if r.StepsTruncated {
		lines = append(lines, quiet.Render("Later steps were not kept."))
	}
	if name == "" {
		name = "Scenario"
	}
	return &viewerModal{title: name, lines: lines, width: 90}
}

func (m *Model) loadHistory() {
	if m.root == "" {
		return
	}
	page, err := project.History(m.root, "", 100, 0)
	if err != nil {
		m.fail(err)
		return
	}
	m.history, m.historyNext, m.historyMeta = page.Items, page.NextOffset, page
	if page.PinnedOverLimit {
		m.say(fmt.Sprintf("Pinned runs use %s, more than the history limit. Unpin some to free space.", humanBytes(int(page.PinnedBytes))))
	}
	m.clampLists()
}

// loadMoreHistory fetches the next page when the cursor reaches the end.
func (m *Model) loadMoreHistory() {
	if m.historyNext == nil || m.lists[historyTab].query() != "" {
		return
	}
	page, err := project.History(m.root, "", 100, *m.historyNext)
	if err != nil {
		m.fail(err)
		return
	}
	m.history = append(m.history, page.Items...)
	m.historyNext = page.NextOffset
}

func (m *Model) historyAction(id action) (bool, tea.Cmd) {
	switch id {
	case actCompareIDs:
		m.compareByID()
		return true, nil
	case actRetention:
		m.editRetention()
		return true, nil
	case actPrune:
		m.modal = &confirmModal{title: "Remove unpinned runs?", yes: "remove", danger: true,
			lines: []string{"Unpinned runs beyond the history limits are deleted.", quiet.Render("Pinned runs stay.")},
			onYes: func(m *Model) tea.Cmd {
				if err := project.PruneHistory(m.root); err != nil {
					m.fail(err)
					return nil
				}
				m.loadHistory()
				m.ok("History trimmed")
				return nil
			}}
		return true, nil
	case actPin, actDeleteRun, actCompare, actAttachRun:
	default:
		return false, nil
	}
	r, ok := m.selectedRun()
	if !ok {
		m.say("No run selected")
		return true, nil
	}
	switch id {
	case actPin:
		change := "pin"
		if r.Pinned {
			change = "unpin"
		}
		if err := project.ChangeRun(m.root, r.ID, change, ""); err != nil {
			m.fail(err)
			return true, nil
		}
		m.loadHistory()
		m.ok(map[string]string{"pin": "Pinned; it stays until you unpin it", "unpin": "Unpinned"}[change])
	case actDeleteRun:
		m.modal = &confirmModal{title: "Delete this run?", yes: "delete", danger: true, lines: []string{quiet.Render(runTime(r.StartedAt) + "  " + r.Response.Method + " " + r.Response.Destination)},
			onYes: func(m *Model) tea.Cmd {
				if err := project.ChangeRun(m.root, r.ID, "delete", ""); err != nil {
					m.fail(err)
					return nil
				}
				m.loadHistory()
				m.ok("Run deleted")
				return nil
			}}
	case actCompare:
		switch m.compareFrom {
		case "":
			m.compareFrom = r.ID
			m.say("Now move to another run and press " + m.keyName(actCompare) + " again")
		case r.ID:
			m.say("Pick a different run to compare with")
		default:
			from := m.compareFrom
			m.compareFrom = ""
			m.showComparison(from, r.ID)
		}
	case actAttachRun:
		if m.req == nil || m.req.caseRes == nil {
			m.say("Open the saved request to attach this run to first")
			return true, nil
		}
		if err := project.ChangeRun(m.root, r.ID, "associate", m.req.caseRes.ID); err != nil {
			m.fail(err)
			return true, nil
		}
		m.loadHistory()
		m.ok("Run attached to “" + m.req.title() + "”")
	}
	return true, nil
}

func (m *Model) compareByID() {
	start := ""
	if r, ok := m.selectedRun(); ok && m.tab == historyTab {
		start = r.ID + " "
	}
	m.modal = &promptModal{title: "Compare runs", label: "compare", input: newInput(start, "first-run-id second-run-id"), note: []string{"Two run IDs separated by a space."},
		submit: func(m *Model, value string) (tea.Cmd, error) {
			ids := strings.Fields(value)
			if len(ids) != 2 {
				return nil, project.Fail("invalid_input", "enter two run IDs separated by a space")
			}
			result, err := project.CompareRuns(m.root, ids[0], ids[1], nil)
			if err != nil {
				return nil, err
			}
			m.modal = m.comparisonViewer(result)
			return nil, nil
		}}
}

func (m *Model) showComparison(before, after string) {
	result, err := project.CompareRuns(m.root, before, after, nil)
	if err != nil {
		m.fail(err)
		return
	}
	m.modal = m.comparisonViewer(result)
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func valueText(v any) string {
	if v == nil {
		return "—"
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func (m *Model) comparisonViewer(c project.Comparison) modal {
	lines := []string{
		quiet.Render("Before  ") + shortID(c.Before.ID) + quiet.Render("  "+m.envName(c.Before.Environment)),
		quiet.Render("After   ") + shortID(c.After.ID) + quiet.Render("  "+m.envName(c.After.Environment)),
		"",
	}
	switch len(c.Differences) {
	case 0:
		lines = append(lines, okStyle.Render("No differences"))
	default:
		lines = append(lines, bold.Render(fmt.Sprintf("%d difference%s", len(c.Differences), plural(len(c.Differences), "", "s"))))
	}
	for _, d := range c.Differences {
		lines = append(lines, "  "+accent.Render(d.Area)+" "+d.Path+quiet.Render("  "+d.Kind))
		lines = append(lines, quiet.Render("    ")+badStyle.Render("− "+clip(valueText(d.Before), 80)))
		lines = append(lines, quiet.Render("    ")+okStyle.Render("+ "+clip(valueText(d.After), 80)))
	}
	if len(c.Ignore.JSONPointers)+len(c.Ignore.Headers) > 0 {
		lines = append(lines, "", quiet.Render("Ignored  "+strings.Join(append(append([]string(nil), c.Ignore.JSONPointers...), c.Ignore.Headers...), ", ")))
	}
	if len(c.Limitations) > 0 {
		lines = append(lines, "", bold.Render("Not compared"))
		for _, l := range c.Limitations {
			lines = append(lines, quiet.Render("  "+l))
		}
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return &viewerModal{title: "Comparison", lines: lines, copyText: m.credentials.Sanitize(string(b))}
}

func (m *Model) editRetention() {
	page, err := project.History(m.root, "", 1, 0)
	if err != nil {
		m.fail(err)
		return
	}
	limits := page.Limits
	m.modal = &formModal{title: "History limits", label: "save",
		note: []string{"Older unpinned runs are removed once either limit is passed."},
		fields: []*formField{
			{label: "Runs per saved request", input: newInput(strconv.Itoa(limits.PerCase), "")},
			{label: "Total size in bytes", input: newInput(strconv.FormatInt(limits.MaxBytes, 10), ""), note: humanBytes(int(limits.MaxBytes))},
		},
		submit: func(m *Model, f *formModal) (tea.Cmd, error) {
			perCase, err := strconv.Atoi(strings.TrimSpace(f.fields[0].value()))
			if err != nil {
				return nil, project.Fail("invalid_input", "runs per saved request must be a whole number")
			}
			maxBytes, err := strconv.ParseInt(strings.TrimSpace(f.fields[1].value()), 10, 64)
			if err != nil {
				return nil, project.Fail("invalid_input", "total size must be a whole number of bytes")
			}
			if err := project.SetHistoryLimits(m.root, project.HistoryLimits{PerCase: perCase, MaxBytes: maxBytes}); err != nil {
				return nil, err
			}
			m.loadHistory()
			m.ok("History limits saved")
			return nil, nil
		}}
}

// viewRun shows a retained run in full: what was sent and what came back.
func (m *Model) viewRun(r project.Run) {
	full, err := project.GetRun(m.root, r.ID)
	if err != nil {
		m.fail(err)
		return
	}
	if full.Scenario != nil {
		m.modal = m.reportViewer(m.scenarioName(full.ScenarioID), *full.Scenario)
		return
	}
	ex := full.Response
	var lines []string
	if ex.Status != 0 {
		lines = append(lines, statusBadge(ex.Status, statusText(ex.Status))+quiet.Render(fmt.Sprintf("  %d ms  %s", ex.DurationMS, humanBytes(ex.RetainedBytes))))
	}
	lines = append(lines, outcomeStyle(full.Outcome).Render(full.Outcome)+quiet.Render("  "+full.StartedAt.Local().Format("Jan 02 15:04:05")+"  "+m.envName(full.Environment)+"  run "+shortID(full.ID)))
	if full.Error != "" {
		lines = append(lines, badStyle.Render(sentence(full.Error)))
	}
	lines = append(lines, "", bold.Render("Request"), methodStyle(full.Request.Method).Render(full.Request.Method)+"  "+full.Request.Destination)
	for _, name := range sortedKeys(full.Request.Headers) {
		for _, v := range full.Request.Headers[name] {
			lines = append(lines, quiet.Render(name+": ")+v)
		}
	}
	if full.Request.Body != "" {
		for _, line := range bodyLines(full.Request) {
			lines = append(lines, jsonLine(line))
		}
	}
	lines = append(lines, "", bold.Render("Response"))
	for _, c := range ex.Checks {
		mark := okStyle.Render("✓")
		if c.Outcome == "failed" {
			mark = badStyle.Render("✗")
		}
		lines = append(lines, mark+" "+c.Kind+" "+quiet.Render(c.Outcome+" "+c.Message))
	}
	for _, name := range sortedKeys(ex.Headers) {
		for _, v := range ex.Headers[name] {
			lines = append(lines, accent.Render(name)+quiet.Render(": ")+v)
		}
	}
	lines = append(lines, "")
	switch {
	case ex.BodyWithheld:
		lines = append(lines, warnStyle.Render("Body withheld: it held authentication data that could not be safely redacted."))
	case ex.Binary:
		lines = append(lines, quiet.Render("Binary body"))
	default:
		for _, line := range bodyLines(ex) {
			lines = append(lines, jsonLine(line))
		}
	}
	copyText := ""
	if !ex.Binary && !ex.BodyWithheld {
		copyText = ex.Body
	}
	m.modal = &viewerModal{title: "Run " + shortID(full.ID), lines: lines, copyText: copyText}
}

func (m *Model) renameProject() {
	m.modal = &promptModal{title: "Rename the project", label: "rename", input: newInput(m.data.snapshot.Manifest.Name, "Name"),
		submit: func(m *Model, name string) (tea.Cmd, error) {
			name = strings.TrimSpace(name)
			s, err := project.Load(m.root)
			if err != nil {
				return nil, err
			}
			manifest := s.Manifest
			manifest.Name = name
			b, err := yaml.Marshal(manifest)
			if err != nil {
				return nil, err
			}
			if err := project.Apply(m.root, []project.Change{{Path: "reqscope.yaml", ExpectedRevision: s.Revision, Content: b}}, false); err != nil {
				return nil, err
			}
			m.sync()
			m.ok("Renamed to “" + name + "”")
			return nil, nil
		}}
}
