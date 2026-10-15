package project

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Scenario bindings identify both the producer step and its named extraction.
type ScenarioBinding struct {
	StepID  string `json:"stepId" yaml:"stepId"`
	Extract string `json:"extract" yaml:"extract"`
}
type ScenarioStep struct {
	ID             string                      `json:"id" yaml:"id"`
	CaseID         string                      `json:"caseId" yaml:"caseId"`
	Overrides      map[string]string           `json:"overrides,omitempty" yaml:"overrides,omitempty"`
	SecretExtracts map[string]SecretExtraction `json:"secretExtracts,omitempty" yaml:"secretExtracts,omitempty"`
	Extracts       map[string]string           `json:"extracts,omitempty" yaml:"extracts,omitempty"`
	Bindings       map[string]ScenarioBinding  `json:"bindings,omitempty" yaml:"bindings,omitempty"`
}
type Scenario struct {
	ID                string         `json:"id" yaml:"id"`
	Name              string         `json:"name" yaml:"name"`
	ContinueOnFailure bool           `json:"continueOnFailure,omitempty" yaml:"continueOnFailure,omitempty"`
	Steps             []ScenarioStep `json:"steps" yaml:"steps"`
}
type ScenarioResource struct {
	Scenario `yaml:",inline"`
	Revision string `json:"revision" yaml:"-"`
}

func ValidateScenario(s Scenario) error {
	if !validID(s.ID) || strings.TrimSpace(s.Name) == "" || len(s.Steps) == 0 {
		return Fail("invalid_input", "scenario requires a UUID id, nonempty name and ordered steps; nested scenarios, loops and scripts are unsupported")
	}
	seen := map[string]ScenarioStep{}
	for _, step := range s.Steps {
		if strings.TrimSpace(step.ID) == "" || !validID(step.CaseID) {
			return Fail("invalid_input", "steps require a unique id and caseId; nested scenarios, loops and scripts are unsupported")
		}
		if _, ok := seen[step.ID]; ok {
			return Fail("invalid_input", "duplicate scenario step id")
		}
		for name, extraction := range step.SecretExtracts {
			if !secretName.MatchString(name) || !validPointer(extraction.Pointer) || len(extraction.Destinations) == 0 {
				return Fail("invalid_input", "secret extraction requires a name, JSON Pointer and destinations")
			}
			if _, ok := step.Extracts[name]; ok {
				return Fail("invalid_input", "extraction names must be unique")
			}
			for _, b := range extraction.Destinations {
				if b.Kind != extraction.Destinations[0].Kind {
					return Fail("invalid_input", "all extraction destinations must use the same authentication kind")
				}
				if err := validateExtractionDestination(b); err != nil {
					return err
				}
			}
		}
		for name, pointer := range step.Extracts {
			if strings.TrimSpace(name) == "" || !validPointer(pointer) {
				return Fail("invalid_input", "invalid ordinary JSON Pointer extraction")
			}
		}
		for name, b := range step.Bindings {
			producer, ok := seen[b.StepID]
			if strings.TrimSpace(name) == "" || !ok {
				return Fail("invalid_input", "bindings must reference an earlier producer step")
			}
			_, ordinary := producer.Extracts[b.Extract]
			if _, secret := producer.SecretExtracts[b.Extract]; !ordinary && !secret {
				return Fail("invalid_input", "binding references an undeclared producer extract")
			}
		}
		seen[step.ID] = step
	}
	return nil
}
func scenarios(root string) ([]ScenarioResource, error) {
	if _, err := load(root); err != nil {
		return nil, err
	}
	if err := safePath(root, "scenarios"); err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(root, "scenarios", "*.yaml"))
	if err != nil {
		return nil, err
	}
	result := []ScenarioResource{}
	for _, path := range paths {
		if err := safePath(root, filepath.Join("scenarios", filepath.Base(path))); err != nil {
			return nil, err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var s Scenario
		if err := Decode(b, &s); err != nil {
			return nil, err
		}
		if err := ValidateScenario(s); err != nil {
			return nil, err
		}
		if filepath.Base(path) != s.ID+".yaml" {
			return nil, Fail("invalid_input", "scenario filename must match its immutable ID")
		}
		result = append(result, ScenarioResource{s, Digest(b)})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		return result[i].ID < result[j].ID
	})
	return result, nil
}
func Scenarios(root string) (result []ScenarioResource, err error) {
	err = locked(root, func() error { var e error; result, e = scenarios(root); return e })
	return
}
func resolveScenario(items []ScenarioResource, selector string) (ScenarioResource, error) {
	for _, s := range items {
		if s.ID == selector {
			return s, nil
		}
	}
	var matches []ScenarioResource
	for _, s := range items {
		if s.Name == selector {
			matches = append(matches, s)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return ScenarioResource{}, Fail("ambiguous_resource", "scenario name is ambiguous; select its UUID")
	}
	return ScenarioResource{}, Fail("not_found", "scenario not found")
}
func GetScenario(root, selector string) (ScenarioResource, error) {
	items, err := Scenarios(root)
	if err != nil {
		return ScenarioResource{}, err
	}
	return resolveScenario(items, selector)
}
func validateScenarioCases(s Scenario, items []CaseResource) error {
	for _, step := range s.Steps {
		if _, err := resolveCase(items, step.CaseID); err != nil {
			return Fail("not_found", "scenario step references a missing case")
		}
	}
	return nil
}
func SaveScenario(root string, s Scenario, revision string, preview bool) (result ScenarioResource, err error) {
	err = locked(root, func() error {
		if err := ValidateScenario(s); err != nil {
			return err
		}
		items, err := cases(root)
		if err != nil {
			return err
		}
		if err := validateScenarioCases(s, items); err != nil {
			return err
		}
		b, err := yaml.Marshal(s)
		if err != nil {
			return err
		}
		result = ScenarioResource{s, Digest(b)}
		return commit(root, []Change{{Path: filepath.Join("scenarios", s.ID+".yaml"), ExpectedRevision: revision, Content: b}}, preview)
	})
	return
}
func DeleteScenario(root, selector, revision string, preview bool) (result map[string]any, err error) {
	err = locked(root, func() error {
		items, err := scenarios(root)
		if err != nil {
			return err
		}
		s, err := resolveScenario(items, selector)
		if err != nil {
			return err
		}
		if revision == "" {
			return Fail("invalid_input", "delete requires expected revision")
		}
		result = map[string]any{"id": s.ID, "preview": preview}
		return commit(root, []Change{{Path: filepath.Join("scenarios", s.ID+".yaml"), ExpectedRevision: revision, Delete: true}}, preview)
	})
	return
}

// CaseReferences lets adapters show affected scenarios before a shared case edit.
func CaseReferences(root, id string) (result []string, err error) {
	err = locked(root, func() error { var e error; result, e = caseReferences(root, id); return e })
	return
}

type scenarioInput struct {
	step     ScenarioStep
	source   CaseResource
	endpoint *Endpoint
}
type ScenarioRun struct {
	resource ScenarioResource
	inputs   []scenarioInput
}

// SnapshotScenario captures all source cases and contracts under the workspace lock.
func SnapshotScenario(root, selector string) (run *ScenarioRun, err error) {
	err = locked(root, func() error {
		items, err := scenarios(root)
		if err != nil {
			return err
		}
		s, err := resolveScenario(items, selector)
		if err != nil {
			return err
		}
		cs, err := cases(root)
		if err != nil {
			return err
		}
		if err := validateScenarioCases(s.Scenario, cs); err != nil {
			return err
		}
		specs, err := specifications(root)
		if err != nil {
			return err
		}
		run = &ScenarioRun{resource: s}
		for _, step := range s.Steps {
			c, _ := resolveCase(cs, step.CaseID)
			input := scenarioInput{step: step, source: c}
			if c.Request.EndpointID != "" {
				for _, spec := range specs {
					for _, ep := range spec.Endpoints {
						if ep.ID == c.Request.EndpointID {
							copy := ep
							input.endpoint = &copy
						}
					}
				}
				if input.endpoint == nil {
					return Fail("not_found", "scenario case references a disconnected endpoint")
				}
			}
			run.inputs = append(run.inputs, input)
		}
		return nil
	})
	return
}
func CheckScenario(root, selector string) (ScenarioResource, error) {
	run, err := SnapshotScenario(root, selector)
	if err != nil {
		return ScenarioResource{}, err
	}
	return run.resource, nil
}

type ScenarioStepReport struct {
	ID             string   `json:"id"`
	CaseID         string   `json:"caseId"`
	CaseRevision   string   `json:"caseRevision"`
	ContractDigest string   `json:"contractDigest,omitempty"`
	Outcome        string   `json:"outcome"`
	Reason         string   `json:"reason,omitempty"`
	Error          *Error   `json:"error,omitempty"`
	Exchange       Exchange `json:"exchange"`
}
type ScenarioReport struct {
	ScenarioID string               `json:"scenarioId"`
	Revision   string               `json:"revision"`
	Successful bool                 `json:"successful"`
	Steps      []ScenarioStepReport `json:"steps"`
}

// Confirm is called before each actual production send. Returning an error ends the run.
type ScenarioOptions struct {
	SendOptions
	Confirm func(context.Context, PreparedRequest) (string, error)
}

func RunScenario(ctx context.Context, root, env, selector string, opts ScenarioOptions) (ScenarioReport, error) {
	run, err := SnapshotScenario(root, selector)
	if err != nil {
		return ScenarioReport{}, err
	}
	return run.Run(ctx, root, env, opts)
}
func (run *ScenarioRun) Run(ctx context.Context, root, env string, opts ScenarioOptions) (report ScenarioReport, err error) {
	report = ScenarioReport{ScenarioID: run.resource.ID, Revision: run.resource.Revision, Successful: true, Steps: []ScenarioStepReport{}}
	c := opts.Credentials
	if c == nil {
		c = DefaultCredentials
	}
	parent := c
	c = c.forRun()
	defer c.releaseRun(parent)
	defer func() {
		if err != nil {
			report.Successful = false
		}
		b, e := json.Marshal(report)
		if e == nil {
			_ = json.Unmarshal(c.SanitizeJSON(b), &report)
		}
		err = c.SanitizeError(err)
	}()
	if err := c.Refresh(root); err != nil {
		return report, err
	}
	// Denial before execution keeps exit code 4; later failures belong to the scenario.
	if !opts.Interactive && strings.TrimSpace(env) == "" {
		return report, Fail("access_denied", "every CLI network run requires explicit --env")
	}
	review, err := ReviewAccess(root, env)
	if err != nil {
		return report, err
	}
	if !opts.Interactive {
		for module, target := range review.BaseURLs {
			if _, err := CheckCLIAccess(root, env, module, target); err != nil {
				return report, err
			}
		}
	}
	if err := run.checkExtractionApproval(root, env); err != nil {
		return report, err
	}
	values := map[string]map[string]string{}
	outcomes := map[string]string{}
	stopped := false
	cancelled := false
	executionStarted := false
	var preExecutionDenial *Error
	for _, input := range run.inputs {
		step := input.step
		item := ScenarioStepReport{ID: step.ID, CaseID: input.source.ID, CaseRevision: input.source.Revision, Outcome: "skipped"}
		if input.endpoint != nil {
			b, _ := json.Marshal(input.endpoint)
			item.ContractDigest = Digest(b)
		}
		if ctx.Err() != nil {
			cancelled = true
			stopped = true
		}
		if stopped {
			item.Reason = "scenario stopped after failure"
			if cancelled {
				item.Reason = "scenario cancelled"
			}
		} else {
			for _, binding := range step.Bindings {
				if outcomes[binding.StepID] != "passed" {
					item.Reason = "producer step " + binding.StepID + " did not pass"
					break
				}
			}
		}
		if item.Reason != "" {
			outcomes[step.ID] = "skipped"
			report.Steps = append(report.Steps, item)
			report.Successful = false
			continue
		}
		variables := map[string]string{}
		for k, v := range opts.Variables {
			variables[k] = v
		}
		for k, v := range step.Overrides {
			variables[k] = v
		}
		for k, b := range step.Bindings {
			variables[k] = values[b.StepID][b.Extract]
		}
		sendOpts := opts.SendOptions
		sendOpts.Variables = variables
		sendOpts.Validation = input.source.Validation
		sendOpts.Credentials = c
		sendOpts.Confirmation = ""
		secretValues := map[string]string{}
		if len(step.SecretExtracts) > 0 {
			sendOpts.captureResponse = func(ex *Exchange) error { return c.captureSecrets(step, ex, secretValues) }
		}
		d := CloneDraft(input.source.Request)
		// A contract snapshot is used for construction and checks; access stays live.
		var stepErr error
		confirmationFailed := false
		if opts.Confirm != nil {
			sendOpts.confirmRedirect = func(ctx context.Context, p PreparedRequest) (string, error) {
				fingerprint, err := opts.Confirm(ctx, p)
				confirmationFailed = err != nil || fingerprint != p.Fingerprint
				return fingerprint, err
			}
		}
		if e := run.checkExtractionApproval(root, env); e != nil {
			return report, e
		}
		p, e := prepareScenarioRequest(root, env, d, sendOpts, input.endpoint)
		if e != nil {
			stepErr = e
		} else if p.Classification == "production" && opts.Interactive {
			if opts.Confirm == nil {
				stepErr = Fail("access_denied", "production scenario send requires per-step confirmation")
			} else {
				sendOpts.Confirmation, stepErr = opts.Confirm(ctx, p)
				confirmationFailed = stepErr != nil || sendOpts.Confirmation != p.Fingerprint
			}
		}
		if stepErr == nil {
			sendOpts.CaseID = input.source.ID
			item.Exchange, stepErr = send(ctx, root, env, d, sendOpts, input.endpoint, true)
		}
		if item.Exchange.Transport == "completed" || item.Exchange.Transport == "failed" {
			executionStarted = true
		}
		item.Exchange.CaseID = input.source.ID
		item.Exchange.Draft = false
		extracted := secretValues
		if stepErr == nil && len(step.Extracts) > 0 {
			body, e := jsonValue(item.Exchange.Body)
			for name, pointer := range step.Extracts {
				value, ok := pointerValue(body, pointer)
				if e != nil || !ok || item.Exchange.Truncated || item.Exchange.Binary || item.Exchange.BodyWithheld {
					stepErr = Fail("extraction_failed", "ordinary JSON Pointer extraction unavailable")
					break
				}
				var text string
				if s, ok := value.(string); ok {
					text = s
				} else {
					b, _ := json.Marshal(value)
					text = string(b)
				}
				// Sanitized authentication fields are not ordinary scenario inputs.
				if strings.Contains(text, "[REDACTED]") || strings.Contains(text, "{{secret:") {
					stepErr = Fail("extraction_failed", "protected response values require secure extraction")
					break
				}
				extracted[name] = text
			}
		}
		item.Outcome = "passed"
		if stepErr != nil {
			item.Outcome = "failed"
			report.Successful = false
			var pe *Error
			if errors.As(c.SanitizeError(stepErr), &pe) {
				item.Error = pe
			} else {
				item.Error = &Error{Code: "storage_error", Message: "scenario execution failed"}
			}
			if item.Error.Code == "cancelled" || ctx.Err() != nil {
				cancelled = true
				if item.Exchange.Transport == "" || item.Exchange.Transport == "not-started" {
					item.Outcome = "skipped"
					item.Reason = "scenario cancelled before send"
				}
			}
			if !executionStarted && (item.Error.Code == "access_denied" || item.Error.Code == "credential_unavailable") {
				preExecutionDenial = item.Error
			}
			stopped = cancelled || confirmationFailed || preExecutionDenial != nil || !run.resource.ContinueOnFailure
		} else {
			values[step.ID] = extracted
		}
		outcomes[step.ID] = item.Outcome
		report.Steps = append(report.Steps, item)
	}
	if cancelled {
		return report, Fail("cancelled", "scenario cancelled")
	}
	if preExecutionDenial != nil {
		return report, preExecutionDenial
	}
	if !report.Successful {
		return report, Fail("scenario_failed", "scenario has failed or skipped mandatory steps")
	}
	return report, nil
}
func prepareScenarioRequest(root, env string, d RequestDraft, opts SendOptions, endpoint *Endpoint) (PreparedRequest, error) {
	d = CloneDraft(d)
	if d.Variables == nil {
		d.Variables = map[string]string{}
	}
	for k, v := range opts.Variables {
		d.Variables[k] = v
	}
	p, err := prepareRequestSnapshot(root, env, d, opts.Interactive, endpoint, true, opts.Credentials)
	p.URL = opts.Credentials.Sanitize(p.URL)
	return p, opts.Credentials.SanitizeError(err)
}
