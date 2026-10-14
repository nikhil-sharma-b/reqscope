package project

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"
	"go.yaml.in/yaml/v3"
)

// Case retains templates and credential references, never resolved request values.
type Case struct {
	ID         string         `json:"id" yaml:"id"`
	Name       string         `json:"name" yaml:"name"`
	Request    RequestDraft   `json:"request" yaml:"request"`
	Validation map[string]any `json:"validation,omitempty" yaml:"validation,omitempty"`
}
type CaseResource struct {
	Case       `yaml:",inline"`
	Revision   string   `json:"revision" yaml:"-"`
	References []string `json:"references" yaml:"-"`
}
type CaseDeletion struct {
	ID         string   `json:"id"`
	References []string `json:"references"`
	Preview    bool     `json:"preview"`
}

func ValidateCase(c Case) error {
	if !validID(c.ID) || strings.TrimSpace(c.Name) == "" {
		return Fail("invalid_input", "case requires a UUID id and a nonempty name")
	}
	d := c.Request
	if d.EndpointID != "" && !validID(d.EndpointID) {
		return Fail("invalid_input", "endpointId must be a UUID")
	}
	if d.EndpointID == "" && (d.Module == "" || d.Method == "" || d.URL == "") {
		return Fail("invalid_input", "case request requires endpointId or module, method and url")
	}
	if !SupportedBodyType(d.BodyType) {
		return Fail("invalid_input", "unsupported body type")
	}
	if d.TimeoutMS < 0 || d.TimeoutMS > 86400000 || d.MaxBodyBytes < 0 || d.MaxBodyBytes > 1<<30 || d.MaxRedirects != nil && *d.MaxRedirects < 0 {
		return Fail("invalid_input", "invalid request limits")
	}
	// Catch literal authentication fields without resolving any credential values.
	for _, field := range d.Headers {
		if protectedHeader(field.Name) && !strings.Contains(field.Value, "{{") {
			return Fail("access_denied", "authentication headers require opaque credential references")
		}
	}
	_, err := decodeValidation(c.Validation)
	return err
}
func cases(root string) ([]CaseResource, error) {
	if _, err := load(root); err != nil {
		return nil, err
	}
	if err := safePath(root, "cases"); err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(root, "cases", "*.yaml"))
	if err != nil {
		return nil, err
	}
	result := []CaseResource{}
	for _, path := range paths {
		if err := safePath(root, filepath.Join("cases", filepath.Base(path))); err != nil {
			return nil, err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var c Case
		if err := Decode(b, &c); err != nil {
			return nil, err
		}
		if err := ValidateCase(c); err != nil {
			return nil, err
		}
		if filepath.Base(path) != c.ID+".yaml" {
			return nil, Fail("invalid_input", "case filename must match its immutable ID")
		}
		result = append(result, CaseResource{Case: c, Revision: Digest(b)})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		return result[i].ID < result[j].ID
	})
	return result, nil
}
func Cases(root string) ([]CaseResource, error) {
	var result []CaseResource
	err := locked(root, func() error { var err error; result, err = cases(root); return err })
	return result, err
}
func resolveCase(items []CaseResource, selector string) (CaseResource, error) {
	for _, c := range items {
		if c.ID == selector {
			return c, nil
		}
	}
	matches := []CaseResource{}
	for _, c := range items {
		if c.Name == selector {
			matches = append(matches, c)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return CaseResource{}, Fail("ambiguous_resource", "case %q is ambiguous; select its UUID", selector)
	}
	return CaseResource{}, Fail("not_found", "case %q not found", selector)
}
func GetCase(root, selector string) (CaseResource, error) {
	items, err := Cases(root)
	if err != nil {
		return CaseResource{}, err
	}
	result, err := resolveCase(items, selector)
	if err != nil {
		return result, err
	}
	result.References, err = CaseReferences(root, result.ID)
	return result, err
}
func saveCase(root string, c Case, revision string, preview bool) (CaseResource, error) {
	if err := ValidateCase(c); err != nil {
		return CaseResource{}, err
	}
	if c.Request.EndpointID != "" {
		specs, err := specifications(root)
		if err != nil {
			return CaseResource{}, err
		}
		found := false
		for _, s := range specs {
			for _, ep := range s.Endpoints {
				if ep.ID == c.Request.EndpointID {
					found = true
				}
			}
		}
		// Existing disconnected cases remain editable; a newly chosen reference must exist.
		if !found {
			items, err := cases(root)
			if err != nil {
				return CaseResource{}, err
			}
			old, err := resolveCase(items, c.ID)
			if err != nil || old.Request.EndpointID != c.Request.EndpointID {
				return CaseResource{}, Fail("not_found", "endpoint reference not found")
			}
		}
	}
	return writeCase(root, c, revision, preview)
}
func writeCase(root string, c Case, revision string, preview bool) (CaseResource, error) {
	if err := ValidateCase(c); err != nil {
		return CaseResource{}, err
	}
	b, err := yaml.Marshal(c)
	if err != nil {
		return CaseResource{}, err
	}
	refs, err := caseReferences(root, c.ID)
	if err != nil {
		return CaseResource{}, err
	}
	err = commit(root, []Change{{Path: filepath.Join("cases", c.ID+".yaml"), ExpectedRevision: revision, Content: b}}, preview)
	return CaseResource{Case: c, Revision: Digest(b), References: refs}, err
}
func SaveCase(root string, c Case, revision string, preview bool) (CaseResource, error) {
	var result CaseResource
	err := locked(root, func() error {
		if _, err := load(root); err != nil {
			return err
		}
		var err error
		result, err = saveCase(root, c, revision, preview)
		return err
	})
	return result, err
}
func DuplicateCase(root, selector, name string, preview bool) (CaseResource, error) {
	var result CaseResource
	err := locked(root, func() error {
		items, err := cases(root)
		if err != nil {
			return err
		}
		source, err := resolveCase(items, selector)
		if err != nil {
			return err
		}
		source.ID = uuid.NewString()
		source.Name = name
		result, err = writeCase(root, source.Case, "", preview)
		return err
	})
	return result, err
}
func DeleteCase(root, selector, revision string, preview bool) (CaseDeletion, error) {
	result := CaseDeletion{References: []string{}, Preview: preview}
	err := locked(root, func() error {
		items, err := cases(root)
		if err != nil {
			return err
		}
		c, err := resolveCase(items, selector)
		if err != nil {
			return err
		}
		result.ID = c.ID
		if revision == "" {
			return Fail("invalid_input", "delete requires expected revision")
		}
		result.References, err = caseReferences(root, c.ID)
		if err != nil {
			return err
		}
		if len(result.References) > 0 {
			return Fail("invalid_input", "case deletion blocked: %s", strings.Join(result.References, ", "))
		}
		return commit(root, []Change{{Path: filepath.Join("cases", c.ID+".yaml"), ExpectedRevision: revision, Delete: true}}, preview)
	})
	return result, err
}
func SendCase(ctx context.Context, root, env, selector string, overrides map[string]string, opts SendOptions) (Exchange, error) {
	c, err := GetCase(root, selector)
	if err != nil {
		return Exchange{}, err
	}
	opts.Validation = c.Validation
	opts.Variables = overrides
	result, err := Send(ctx, root, env, c.Request, opts)
	result.CaseID = c.ID
	result.Draft = false
	return result, err
}

// CloneDraft preserves user-owned inputs while allowing run-only variable overrides.
func CloneDraft(d RequestDraft) RequestDraft {
	b, _ := json.Marshal(d)
	var copy RequestDraft
	_ = json.Unmarshal(b, &copy)
	return copy
}

// Scenario steps use caseId references. Later scenario operations extend this
// check with their schema and repair operations.
func caseReferences(root, id string) ([]string, error) {
	result := []string{}
	if err := safePath(root, "scenarios"); err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(root, "scenarios", "*.yaml"))
	if err != nil {
		return nil, err
	}
	for _, path := range paths {
		if err := safePath(root, filepath.Join("scenarios", filepath.Base(path))); err != nil {
			return nil, err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var doc yaml.Node
		if err := Decode(b, &doc); err != nil {
			return nil, err
		}
		if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
			return nil, Fail("invalid_input", "cannot inspect scenario references in %s", filepath.Base(path))
		}
		seen := map[*yaml.Node]bool{}
		var walk func(*yaml.Node) bool
		walk = func(n *yaml.Node) bool {
			if n == nil || seen[n] {
				return false
			}
			seen[n] = true
			if n.Kind == yaml.AliasNode {
				return walk(n.Alias)
			}
			if n.Kind == yaml.MappingNode {
				for i := 0; i < len(n.Content); i += 2 {
					if n.Content[i].Value == "caseId" && n.Content[i+1].Value == id {
						return true
					}
				}
			}
			for _, child := range n.Content {
				if walk(child) {
					return true
				}
			}
			return false
		}
		if walk(doc.Content[0]) {
			result = append(result, "scenarios/"+filepath.Base(path))
		}
	}
	return result, nil
}
