package cli

import (
	"encoding/json"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"go.yaml.in/yaml/v3"
	"regexp"
	"strconv"
	"strings"
)

type InitInput struct {
	Name string `json:"name" yaml:"name"`
}
type UpdateInput struct {
	Name     string `json:"name" yaml:"name"`
	Revision string `json:"revision" yaml:"revision"`
}
type EnvironmentInput struct {
	project.Environment `yaml:",inline"`
	Revision            string `json:"revision,omitempty" yaml:"revision,omitempty"`
}

var schemaNames = []string{"project.init", "project.update", "environment.create", "environment.update", "request.send", "case.create", "case.update", "case.duplicate", "case.delete", "case.run"}

func schema(name string) (map[string]any, error) {
	str := map[string]any{"type": "string", "minLength": 1, "pattern": `\S`}
	props := map[string]any{"name": str}
	required := []string{"name"}
	switch name {
	case "request.send":
		props = map[string]any{}
		for _, name := range []string{"endpointId", "module", "method", "url", "body", "bodyType", "contentType"} {
			props[name] = map[string]any{"type": "string"}
		}
		props["variables"] = map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}}
		props["path"] = map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}}
		field := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"name": map[string]any{"type": "string"}, "value": map[string]any{"type": "string"}}, "required": []string{"name", "value"}}
		for _, name := range []string{"query", "headers"} {
			props[name] = map[string]any{"type": "array", "items": field}
		}
		for _, name := range []string{"timeoutMs", "maxRedirects", "maxBodyBytes"} {
			props[name] = map[string]any{"type": "integer", "minimum": 0}
		}
		required = []string{}
		props["timeoutMs"].(map[string]any)["maximum"] = 86400000
		props["maxBodyBytes"].(map[string]any)["maximum"] = 1073741824
		props["bodyType"].(map[string]any)["enum"] = []string{"", "json", "raw"}
	case "case.create", "case.update":
		draft, _ := schema("request.send")
		props = map[string]any{"id": map[string]any{"type": "string", "format": "uuid"}, "name": str, "request": draft, "validation": map[string]any{"type": "object"}}
		required = []string{"id", "name", "request"}
		if name == "case.update" {
			props["revision"] = map[string]any{"type": "string", "pattern": "^[a-f0-9]{64}$"}
			required = append(required, "revision")
		}
	case "case.duplicate":
	case "case.delete":
		props = map[string]any{"revision": map[string]any{"type": "string", "pattern": "^[a-f0-9]{64}$"}}
		required = []string{"revision"}
	case "case.run":
		props = map[string]any{"variables": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}}}
		required = []string{}
	case "project.init":
	case "project.update":
		props["revision"] = map[string]any{"type": "string", "pattern": "^[a-f0-9]{64}$"}
		required = append(required, "revision")
	case "environment.create", "environment.update":
		props["id"] = map[string]any{"type": "string", "format": "uuid"}
		props["classification"] = map[string]any{"enum": []string{"nonproduction", "production"}}
		props["baseURLs"] = map[string]any{"type": "object", "additionalProperties": map[string]string{"type": "string"}}
		props["variables"] = props["baseURLs"]
		required = append(required, "id", "classification", "baseURLs", "variables")
		if name == "environment.update" {
			props["revision"] = map[string]any{"type": "string", "pattern": "^[a-f0-9]{64}$"}
			required = append(required, "revision")
		}
	default:
		return nil, project.Fail("not_found", "unknown input schema %q; use input-schema list", name)
	}
	result := map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "title": name, "type": "object", "additionalProperties": false, "properties": props, "required": required}
	if name == "request.send" {
		result["anyOf"] = []any{map[string]any{"required": []string{"endpointId"}}, map[string]any{"required": []string{"module", "method", "url"}}}
	}
	return result, nil
}
func validateInput(name string, b []byte) (any, error) {
	shape, err := schema(name)
	if err != nil {
		return nil, err
	}
	if name == "request.send" {
		if err := validateDocument(b, shape); err != nil {
			return nil, err
		}
		var d project.RequestDraft
		if err := project.Decode(b, &d); err != nil {
			return nil, err
		}
		if d.EndpointID == "" && (d.Module == "" || d.Method == "" || d.URL == "") {
			return nil, project.Fail("invalid_input", "provide endpointId or module, method and url")
		}
		if d.TimeoutMS < 0 || d.TimeoutMS > 86400000 || d.MaxBodyBytes < 0 || d.MaxBodyBytes > 1073741824 || d.MaxRedirects != nil && *d.MaxRedirects < 0 {
			return nil, project.Fail("invalid_input", "invalid request limits")
		}
		if d.BodyType != "" && d.BodyType != "json" && d.BodyType != "raw" {
			return nil, project.Fail("invalid_input", "unsupported body type")
		}
		return d, nil
	}
	if err := validateDocument(b, shape); err != nil {
		return nil, err
	}
	switch name {
	case "case.create", "case.update":
		var v CaseInput
		if err := project.Decode(b, &v); err != nil {
			return nil, err
		}
		if err := project.ValidateCase(v.Case); err != nil {
			return nil, err
		}
		if name == "case.update" && !validRevision(v.Revision) {
			return nil, project.Fail("invalid_input", "update requires revision from case get")
		}
		if name == "case.create" && v.Revision != "" {
			return nil, project.Fail("invalid_input", "create does not accept revision")
		}
		return v, nil
	case "case.duplicate":
		var v CaseDuplicateInput
		if err := project.Decode(b, &v); err != nil {
			return nil, err
		}
		return v, validName(v.Name)
	case "case.delete":
		var v CaseDeleteInput
		if err := project.Decode(b, &v); err != nil {
			return nil, err
		}
		if !validRevision(v.Revision) {
			return nil, project.Fail("invalid_input", "delete requires revision from case get")
		}
		return v, nil
	case "case.run":
		var v struct {
			Variables map[string]string `json:"variables" yaml:"variables"`
		}
		if err := project.Decode(b, &v); err != nil {
			return nil, err
		}
		return v, nil
	case "project.init":
		var v InitInput
		if err := project.Decode(b, &v); err != nil {
			return nil, err
		}
		if err := validName(v.Name); err != nil {
			return nil, err
		}
		return v, nil
	case "project.update":
		var v UpdateInput
		if err := project.Decode(b, &v); err != nil {
			return nil, err
		}
		if err := validName(v.Name); err != nil {
			return nil, err
		}
		if !validRevision(v.Revision) {
			return nil, project.Fail("invalid_input", "revision must be a SHA-256 digest from project inspect")
		}
		return v, nil
	default:
		var v EnvironmentInput
		if err := project.Decode(b, &v); err != nil {
			return nil, err
		}
		if err := project.ValidateEnvironment(v.Environment); err != nil {
			return nil, err
		}
		if name == "environment.update" && !validRevision(v.Revision) {
			return nil, project.Fail("invalid_input", "revision must be a SHA-256 digest from environment get")
		}
		if name == "environment.create" && v.Revision != "" {
			return nil, project.Fail("invalid_input", "create does not accept revision")
		}
		return v, nil
	}
}
func validName(name string) error {
	if strings.TrimSpace(name) == "" {
		return project.Fail("invalid_input", "name is required")
	}
	return nil
}
func validRevision(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}
func formatted(v any) string { b, _ := json.MarshalIndent(v, "", "  "); return string(b) }

// Validate node types before decoding, which otherwise coerces numbers to strings.
func validateDocument(b []byte, shape map[string]any) error {
	var doc yaml.Node
	if err := project.Decode(b, &doc); err != nil {
		return err
	}
	if len(doc.Content) != 1 {
		return project.Fail("invalid_input", "input must be one object")
	}
	return validateNode(doc.Content[0], shape, "input")
}
func propertyMap(value any) map[string]any {
	if m, ok := value.(map[string]any); ok {
		return m
	}
	if m, ok := value.(map[string]string); ok {
		result := map[string]any{}
		for key, v := range m {
			result[key] = v
		}
		return result
	}
	return nil
}
func validateNode(node *yaml.Node, rule map[string]any, path string) error {
	fail := func() error { return project.Fail("invalid_input", "%s has invalid type or value", path) }
	switch rule["type"] {
	case "object":
		if node.Kind != yaml.MappingNode {
			return fail()
		}
		props := propertyMap(rule["properties"])
		seen := map[string]bool{}
		for i := 0; i < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			if key.Tag != "!!str" || seen[key.Value] {
				return fail()
			}
			seen[key.Value] = true
			child, ok := props[key.Value]
			if !ok {
				if rule["additionalProperties"] == false {
					return project.Fail("invalid_input", "unknown field %s.%s", path, key.Value)
				}
				child = rule["additionalProperties"]
			}
			if next := propertyMap(child); next != nil {
				if err := validateNode(value, next, path+"."+key.Value); err != nil {
					return err
				}
			}
		}
		if required, ok := rule["required"].([]string); ok {
			for _, key := range required {
				if !seen[key] {
					return project.Fail("invalid_input", "%s.%s is required", path, key)
				}
			}
		}
	case "array":
		if node.Kind != yaml.SequenceNode {
			return fail()
		}
		for _, child := range node.Content {
			if err := validateNode(child, propertyMap(rule["items"]), path+"[]"); err != nil {
				return err
			}
		}
	case "integer":
		if node.Kind != yaml.ScalarNode || node.Tag != "!!int" {
			return fail()
		}
		var value int64
		if err := node.Decode(&value); err != nil {
			return fail()
		}
		for key, bound := range rule {
			switch key {
			case "minimum", "maximum":
				number, err := strconv.ParseInt(fmtNumber(bound), 10, 64)
				if err != nil {
					return fail()
				}
				if key == "minimum" && value < number || key == "maximum" && value > number {
					return fail()
				}
			}
		}
	case "string":
		if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
			return fail()
		}
		if pattern, ok := rule["pattern"].(string); ok {
			matched, err := regexp.MatchString(pattern, node.Value)
			if err != nil || !matched {
				return fail()
			}
		}
	default:
		if _, ok := rule["enum"]; ok && (node.Kind != yaml.ScalarNode || node.Tag != "!!str") {
			return fail()
		}
	}
	if choices, ok := rule["enum"].([]string); ok {
		found := false
		for _, choice := range choices {
			if node.Value == choice {
				found = true
			}
		}
		if !found {
			return fail()
		}
	}
	return nil
}
func fmtNumber(value any) string { b, _ := json.Marshal(value); return string(b) }
