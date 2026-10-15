package project

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/google/uuid"
	"go.yaml.in/yaml/v3"
)

// ContractEdit targets one authoritative operation or named schema, never a resolved copy.
type ContractEdit struct {
	SourceID     string `json:"sourceId" yaml:"sourceId"`
	Revision     string `json:"revision" yaml:"revision"`
	ImportedEdit bool   `json:"importedEdit" yaml:"importedEdit"`
	Document     string `json:"document,omitempty" yaml:"document,omitempty"`
	Path         string `json:"path,omitempty" yaml:"path,omitempty"`
	Method       string `json:"method,omitempty" yaml:"method,omitempty"`
	Name         string `json:"name,omitempty" yaml:"name,omitempty"`
	Value        any    `json:"value,omitempty" yaml:"value,omitempty"`
	ReassignTo   string `json:"reassignTo,omitempty" yaml:"reassignTo,omitempty"`
}
type ContractImpact struct {
	Specification Specification      `json:"specification"`
	Cases         []CaseResource     `json:"affectedCases"`
	Scenarios     []ScenarioResource `json:"affectedScenarios"`
	Preview       bool               `json:"preview"`
}

func contractDocuments(s Specification) map[string][]byte {
	if len(s.LocalDocuments) > 0 {
		return s.LocalDocuments
	}
	return s.Documents
}
func contractRoot(s Specification) string {
	if s.Mode == "url" {
		return s.Location
	}
	return filepath.ToSlash(filepath.Base(s.Location))
}

// ExportContract exports only authoritative OpenAPI documents. Cases, scenarios,
// credentials, approvals and execution history belong to separate stores.
func ExportContract(root, sourceID string, credentials ...*Credentials) (map[string]any, error) {
	c := contractCredentials(credentials)
	if err := c.Refresh(root); err != nil {
		return nil, err
	}
	specs, err := Specifications(root)
	if err != nil {
		return nil, err
	}
	for _, s := range specs {
		if s.ID == sourceID {
			docs := map[string]string{}
			for k, v := range contractDocuments(s) {
				var parsed any
				if err := Decode(v, &parsed); err != nil {
					return nil, err
				}
				if c.Sanitize(string(v)) != string(v) || contractHasCredential(parsed, c) {
					return nil, Fail("access_denied", "contract contains protected credentials; export refused")
				}
				docs[k] = string(v)
			}
			return map[string]any{"rootDocument": contractRoot(s), "documents": docs}, nil
		}
	}
	return nil, Fail("not_found", "source not found")
}

// EditContract validates and computes impact under the same lock as the write.
func EditContract(root, kind, action string, edit ContractEdit, preview bool, credentials ...*Credentials) (result ContractImpact, err error) {
	result.Preview = preview
	c := contractCredentials(credentials)
	if err = c.Refresh(root); err != nil {
		return result, err
	}
	if contractHasCredential(edit.Value, c) {
		return result, Fail("access_denied", "contract edit contains protected credentials")
	}
	err = locked(root, func() error {
		if kind != "endpoint" && kind != "schema" || action != "create" && action != "update" && action != "delete" {
			return Fail("invalid_input", "unknown contract edit")
		}
		if !edit.ImportedEdit {
			return Fail("access_denied", "set importedEdit to true to explicitly edit the imported contract")
		}
		specs, e := specifications(root)
		if e != nil {
			return e
		}
		var source Specification
		for _, s := range specs {
			if s.ID == edit.SourceID {
				source = s
				break
			}
		}
		if source.ID == "" {
			return Fail("not_found", "source not found")
		}
		if edit.Revision == "" || edit.Revision != source.Revision {
			return Fail("revision_conflict", "contract changed; inspect and preview again")
		}
		documents := map[string][]byte{}
		for k, v := range contractDocuments(source) {
			documents[k] = v
		}
		document := edit.Document
		if document == "" {
			document = contractRoot(source)
		}
		raw, ok := documents[document]
		if !ok {
			return Fail("not_found", "document is not part of this source")
		}
		var node yaml.Node
		if e = Decode(raw, &node); e != nil {
			return e
		}
		var target []string
		if kind == "endpoint" {
			method := strings.ToLower(edit.Method)
			if !strings.HasPrefix(edit.Path, "/") || !strings.Contains("|get|put|post|delete|options|head|patch|trace|", "|"+method+"|") {
				return Fail("invalid_input", "endpoint requires an absolute path and supported method")
			}
			target = []string{"paths", edit.Path, method}
		} else {
			if strings.TrimSpace(edit.Name) == "" {
				return Fail("invalid_input", "schema name is required")
			}
			target = []string{"components", "schemas", edit.Name}
			if source.Version == "2.0" {
				target = []string{"definitions", edit.Name}
			}
		}
		if e = editContractNode(node.Content[0], target, action, edit.Value); e != nil {
			return e
		}
		documents[document], e = yaml.Marshal(&node)
		if e != nil {
			return e
		}
		// Resolve only retained documents. Editing must never fetch new files or URLs.
		totalBytes := 0
		r := resolver{root: filepath.Dir(source.Location), retainedOnly: true, documents: documents, parsed: map[string]any{}}
		for name, b := range documents {
			totalBytes += len(b)
			if totalBytes > 10*1024*1024 {
				return Fail("invalid_input", "contract exceeds 10 MiB")
			}
			var v any
			if e = Decode(b, &v); e != nil {
				return e
			}
			if c.Sanitize(string(b)) != string(b) || contractHasCredential(v, c) {
				return Fail("access_denied", "contract document contains protected credentials")
			}
			v, e = normalizeYAML(v)
			if e != nil {
				return e
			}
			key := name
			if source.Mode != "url" {
				key = filepath.Join(filepath.Dir(source.Location), filepath.FromSlash(name))
			}
			r.parsed[key] = v
		}
		if source.Mode == "url" {
			r.fetch = func(string) ([]byte, error) {
				return nil, Fail("invalid_input", "reference must target a retained source document")
			}
		}
		candidate, e := importResolvedSpecification(root, source.Location, source.Mode, &r, r.parsed[source.Location], true, "")
		if e != nil {
			return e
		}
		for i := range candidate.Endpoints {
			ep := &candidate.Endpoints[i]
			ep.SourceID = source.ID
			ep.ModuleID = source.ModuleID
			ep.ID = uuid.NewSHA1(uuid.MustParse(source.ID), []byte(ep.Method+" "+ep.Path)).String()
		}
		for i := range candidate.Schemas {
			sc := &candidate.Schemas[i]
			sc.SourceID = source.ID
			sc.ID = uuid.NewSHA1(uuid.MustParse(source.ID), []byte("schema "+sc.Name)).String()
		}
		oldByID := map[string]Endpoint{}
		newByID := map[string]Endpoint{}
		for _, ep := range source.Endpoints {
			oldByID[ep.ID] = ep
		}
		for _, ep := range candidate.Endpoints {
			newByID[ep.ID] = ep
		}
		// Local contract proposals cannot change effective request destinations or auth.
		for id, ep := range newByID {
			before, exists := oldByID[id]
			if exists && !reflect.DeepEqual(contractProtection(before), contractProtection(ep)) {
				return Fail("access_denied", "contract edit changes destination or authentication; use protected settings workflows")
			}
			if !exists && (ep.Operation["servers"] != nil || ep.Operation["security"] != nil) {
				return Fail("access_denied", "new operations cannot override destination or authentication")
			}
		}
		items, e := cases(root)
		if e != nil {
			return e
		}
		flows, e := scenarios(root)
		if e != nil {
			return e
		}
		affected := map[string]bool{}
		changes := []Change{}
		for _, c := range items {
			old, exists := oldByID[c.Request.EndpointID]
			if !exists {
				continue
			}
			next, present := newByID[old.ID]
			if present && reflect.DeepEqual(old.Operation, next.Operation) && reflect.DeepEqual(old.PathInputs, next.PathInputs) && reflect.DeepEqual(old.Context, next.Context) {
				continue
			}
			result.Cases = append(result.Cases, c)
			affected[c.ID] = true
			if !present {
				replacement, ok := newByID[edit.ReassignTo]
				if !ok {
					if !preview {
						return Fail("invalid_input", "endpoint is referenced by case %s; preview impact and select reassignTo", c.ID)
					}
					continue
				}
				if replacement.ModuleID != old.ModuleID {
					return Fail("access_denied", "reassignment must remain in the source module")
				}
				c.Request.EndpointID = replacement.ID
				b, e := yaml.Marshal(c.Case)
				if e != nil {
					return e
				}
				changes = append(changes, Change{Path: "cases/" + c.ID + ".yaml", ExpectedRevision: c.Revision, Content: b})
			}
		}
		if edit.ReassignTo != "" && (kind != "endpoint" || action != "delete") {
			return Fail("invalid_input", "reassignTo is only available for endpoint deletion")
		}
		for _, flow := range flows {
			for _, step := range flow.Steps {
				if affected[step.CaseID] {
					result.Scenarios = append(result.Scenarios, flow)
					break
				}
			}
		}
		for id, ep := range oldByID {
			if _, ok := newByID[id]; !ok {
				source.DeletedEndpoints = append(source.DeletedEndpoints, ep)
			}
		}
		source.LocalDocuments = documents
		source.Endpoints = candidate.Endpoints
		source.Schemas = candidate.Schemas
		source.Diagnostics = candidate.Diagnostics
		b, e := yaml.Marshal(source)
		if e != nil {
			return e
		}
		source.Revision = Digest(b)
		changes = append(changes, Change{Path: "specs/" + source.ID + ".yaml", ExpectedRevision: edit.Revision, Content: b})
		result.Specification = source
		return commit(root, changes, preview)
	})
	return result, err
}

func contractProtection(ep Endpoint) map[string]any {
	result := map[string]any{}
	for _, k := range []string{"servers", "host", "basePath", "schemes", "security", "securityDefinitions", "securitySchemes"} {
		result["context/"+k] = ep.Context[k]
		result["operation/"+k] = ep.Operation[k]
	}
	return result
}

func editContractNode(node *yaml.Node, path []string, action string, value any) error {
	if node.Kind != yaml.MappingNode {
		return Fail("invalid_input", "edit target must be an object; edit the referenced document explicitly")
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == "$ref" {
			return Fail("invalid_input", "edit the referenced document explicitly")
		}
	}
	key := path[0]
	index := -1
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			index = i
			break
		}
	}
	if len(path) > 1 {
		if index < 0 {
			if action != "create" {
				return Fail("not_found", "contract target does not exist")
			}
			node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"})
			index = len(node.Content) - 2
		}
		return editContractNode(node.Content[index+1], path[1:], action, value)
	}
	if action == "create" && index >= 0 {
		return Fail("revision_conflict", "contract target already exists")
	}
	if action != "create" && index < 0 {
		return Fail("not_found", "contract target does not exist")
	}
	if action == "delete" {
		node.Content = append(node.Content[:index], node.Content[index+2:]...)
		return nil
	}
	if value == nil {
		return Fail("invalid_input", "contract value is required")
	}
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var replacement yaml.Node
	if err = Decode(b, &replacement); err != nil {
		return err
	}
	if index >= 0 {
		node.Content[index+1] = replacement.Content[0]
	} else {
		node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, replacement.Content[0])
	}
	return nil
}

func contractCredentials(credentials []*Credentials) *Credentials {
	if len(credentials) > 0 && credentials[0] != nil {
		return credentials[0]
	}
	return DefaultCredentials
}
func contractHasCredential(value any, c *Credentials) bool {
	switch v := value.(type) {
	case string:
		return c.Sanitize(v) != v
	case map[any]any:
		for k, x := range v {
			if contractHasCredential(k, c) || contractHasCredential(x, c) {
				return true
			}
		}
	case map[string]any:
		for k, x := range v {
			if c.Sanitize(k) != k || contractHasCredential(x, c) {
				return true
			}
		}
	case []any:
		for _, x := range v {
			if contractHasCredential(x, c) {
				return true
			}
		}
	}
	return false
}
