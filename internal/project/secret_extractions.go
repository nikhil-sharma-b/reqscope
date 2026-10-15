package project

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
)

// SecretExtraction is public metadata. Its value exists only for one run.
type SecretExtraction struct {
	Pointer      string              `json:"pointer" yaml:"pointer"`
	Destinations []CredentialBinding `json:"destinations" yaml:"destinations"`
}

func validateExtractionDestination(b CredentialBinding) error {
	u, err := url.Parse(b.Destination)
	if err != nil {
		return Fail("invalid_input", "invalid extraction destination")
	}
	if _, err := Origin(b.Destination); err != nil {
		return err
	}
	if u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.User != nil || strings.ContainsAny(b.Destination, "{}") || b.Module == "" {
		return Fail("invalid_input", "extraction destination requires an exact URL without query, fragment or user information")
	}
	if b.Kind != "bearer" && b.Kind != "api-key" {
		return Fail("invalid_input", "secret extraction kind must be bearer or api-key")
	}
	if b.Kind == "bearer" && (b.In != "header" || b.Field != "Authorization") {
		return Fail("invalid_input", "bearer extraction requires Authorization header")
	}
	switch b.In {
	case "header":
		if !validHeaderName(b.Field) || b.Field != http.CanonicalHeaderKey(b.Field) || strings.EqualFold(b.Field, "Host") || strings.EqualFold(b.Field, "Content-Length") || strings.EqualFold(b.Field, "Content-Type") {
			return Fail("invalid_input", "invalid extraction header")
		}
	case "query":
		if !secretName.MatchString(b.Field) {
			return Fail("invalid_input", "invalid extraction query field")
		}
	case "body":
		if b.Field != "" {
			return Fail("invalid_input", "body extraction placement requires empty field")
		}
	default:
		return Fail("invalid_input", "invalid extraction placement")
	}
	return nil
}

func (run *ScenarioRun) extractionFingerprint(review AccessReview) string {
	// Include source revisions and contracts, so case edits also need fresh approval.
	cases := []CaseResource{}
	contracts := []*Endpoint{}
	for _, input := range run.inputs {
		cases = append(cases, input.source)
		contracts = append(contracts, input.endpoint)
	}
	b, _ := json.Marshal(struct {
		Scenario  ScenarioResource
		Cases     []CaseResource
		Contracts []*Endpoint
		Review    AccessReview
	}{run.resource, cases, contracts, review})
	return Digest(b)
}

// ApproveScenarioExtractions is used by the TUI after displaying the full review.
// CLI adapters deliberately do not expose this operation.
func ApproveScenarioExtractions(root, env, selector, fingerprint string) error {
	run, err := SnapshotScenario(root, selector)
	if err != nil {
		return err
	}
	return locked(root, func() error {
		s, err := load(root)
		if err != nil {
			return err
		}
		e, err := s.Resolve(env)
		if err != nil {
			return err
		}
		a, p, err := readAccess(root, s)
		if err != nil {
			return err
		}
		review, err := resolvedReview(e.Environment, a.Bindings[e.ID])
		if err != nil {
			return err
		}
		if fingerprint != run.extractionFingerprint(review) {
			return Fail("conflict", "extraction review changed; review again")
		}
		for _, input := range run.inputs {
			for _, extract := range input.step.SecretExtracts {
				for _, b := range extract.Destinations {
					origin, _ := Origin(b.Destination)
					if review.Origins[b.Module] != origin {
						return Fail("access_denied", "extraction destination must match configured module origin")
					}
				}
			}
		}
		if a.Extractions == nil {
			a.Extractions = map[string]map[string]string{}
		}
		if a.Extractions[e.ID] == nil {
			a.Extractions[e.ID] = map[string]string{}
		}
		if a.ProtectedPointers == nil {
			a.ProtectedPointers = map[string]map[string][]string{}
		}
		if a.ProtectedPointers[e.ID] == nil {
			a.ProtectedPointers[e.ID] = map[string][]string{}
		}
		pointers := []string{}
		for _, input := range run.inputs {
			for _, extract := range input.step.SecretExtracts {
				pointers = append(pointers, extract.Pointer)
			}
		}
		a.ProtectedPointers[e.ID][run.resource.ID] = pointers
		a.Extractions[e.ID][run.resource.ID] = fingerprint
		return writeAccess(p, a)
	})
}

type ExtractionReview struct {
	Scenario    ScenarioResource `json:"scenario"`
	Cases       []CaseResource   `json:"cases"`
	Access      AccessReview     `json:"access"`
	Fingerprint string           `json:"fingerprint"`
}

func ReviewScenarioExtractions(root, env, selector string) (ExtractionReview, error) {
	run, err := SnapshotScenario(root, selector)
	if err != nil {
		return ExtractionReview{}, err
	}
	review, err := ReviewAccess(root, env)
	if err != nil {
		return ExtractionReview{}, err
	}
	result := ExtractionReview{Scenario: run.resource, Access: review, Fingerprint: run.extractionFingerprint(review)}
	for _, input := range run.inputs {
		result.Cases = append(result.Cases, input.source)
	}
	return result, nil
}
func (run *ScenarioRun) checkExtractionApproval(root, env string) error {
	s, err := Load(root)
	if err != nil {
		return err
	}
	e, err := s.Resolve(env)
	if err != nil {
		return err
	}
	a, _, err := readAccess(root, s)
	if err != nil {
		return err
	}
	approved := a.Extractions[e.ID][run.resource.ID]
	required := approved != ""
	for _, input := range run.inputs {
		required = required || len(input.step.SecretExtracts) > 0
	}
	if !required {
		return nil
	}
	review, err := resolvedReview(e.Environment, a.Bindings[e.ID])
	if err != nil {
		return err
	}
	if approved != run.extractionFingerprint(review) {
		return Fail("access_denied", "protected scenario extraction or destination changed; approve in TUI")
	}
	return nil
}

func (c *Credentials) forRun() *Credentials {
	c.mu.Lock()
	defer c.mu.Unlock()
	run := NewCredentials(c.store)
	for k, v := range c.session {
		run.session[k] = v
	}
	for k, v := range c.known {
		run.known[k] = v
	}
	run.transient = map[string]localCredential{}
	return run
}
func (c *Credentials) releaseRun(parent *Credentials) {
	parent.mu.Lock()
	defer parent.mu.Unlock()
	// Retain redaction knowledge for subsequent adapter output, never usable values.
	for v := range c.known {
		parent.known[v] = true
	}
	c.session = nil
	c.transient = nil
}
func (c *Credentials) items(root, env string) (map[string]localCredential, error) {
	items, err := credentialItems(root, env)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = map[string]localCredential{}
	}
	for name, item := range c.transient {
		items[name] = item
	}
	return items, nil
}
func (c *Credentials) bindingReview(review AccessReview) AccessReview {
	bindings := map[string]string{}
	for k, v := range review.Bindings {
		bindings[k] = v
	}
	for name, item := range c.transient {
		b, _ := json.Marshal(item)
		bindings[name] = Digest(b)
	}
	review.Bindings = bindings
	return review
}
func (c *Credentials) captureSecrets(step ScenarioStep, ex *Exchange, values map[string]string) error {
	body, err := jsonValue(ex.Body)
	failed := err != nil || ex.Truncated || ex.Binary || ex.BodyWithheld
	// Register every available secret even when another extraction fails.
	for name, extract := range step.SecretExtracts {
		value, ok := pointerValue(body, extract.Pointer)
		text, isString := value.(string)
		if !ok || !isString || text == "" || strings.ContainsAny(text, "\r\n\x00") {
			failed = true
			continue
		}
		c.mu.Lock()
		c.remember(text)
		c.remember("Bearer " + text)
		c.mu.Unlock()
		id := "run-" + uuid.NewString()
		b := extract.Destinations[0]
		b.Name = id
		item := localCredential{CredentialBinding: b, Key: id, Session: true, Destinations: extract.Destinations}
		c.session[id] = text
		c.transient[id] = item
		values[name] = "{{secret:" + id + "}}"
	}
	if failed {
		// Malformed or truncated JSON may contain an unregistered partial token.
		ex.Body = ""
		ex.Headers = http.Header{}
		ex.BodyWithheld = true
		return Fail("extraction_failed", "protected JSON Pointer extraction unavailable")
	}
	return nil
}

// Approved pointers stay protected even in ordinary scenarios or direct sends.
// Applying them across this environment prevents cloning a producer to read back
// its token under another extraction name.
func (c *Credentials) protectResponse(root, env string, ex *Exchange) error {
	s, err := Load(root)
	if err != nil {
		return err
	}
	e, err := s.Resolve(env)
	if err != nil {
		return err
	}
	a, _, err := readAccess(root, s)
	if err != nil {
		return err
	}
	if len(a.ProtectedPointers[e.ID]) == 0 {
		return nil
	}
	body, parseErr := jsonValue(ex.Body)
	if parseErr != nil || ex.Truncated || ex.Binary {
		ex.Body = ""
		ex.Headers = http.Header{}
		ex.BodyWithheld = true
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, pointers := range a.ProtectedPointers[e.ID] {
		for _, pointer := range pointers {
			value, ok := pointerValue(body, pointer)
			if ok {
				if text, ok := value.(string); ok {
					c.remember(text)
				}
			}
		}
	}
	return nil
}
