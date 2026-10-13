package cli

import (
	"encoding/json"
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"go.yaml.in/yaml/v3"
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

var schemaNames = []string{"project.init", "project.update", "environment.create", "environment.update", "request.send"}

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

// Validate the YAML node types before decoding, which otherwise coerces numbers to strings.
func validateDocument(b []byte, shape map[string]any) error {
	var doc yaml.Node
	if err := project.Decode(b, &doc); err != nil {
		return err
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return project.Fail("invalid_input", "input must be an object")
	}
	props := shape["properties"].(map[string]any)
	seen := map[string]bool{}
	nodes := doc.Content[0].Content
	for i := 0; i < len(nodes); i += 2 {
		key, value := nodes[i], nodes[i+1]
		rule, ok := props[key.Value]
		if !ok || seen[key.Value] || key.Tag != "!!str" {
			return project.Fail("invalid_input", "unknown or duplicate field %q", key.Value)
		}
		seen[key.Value] = true
		property := rule.(map[string]any)
		if property["type"] == "object" {
			if value.Kind != yaml.MappingNode {
				return project.Fail("invalid_input", "%s must be an object", key.Value)
			}
			keys := map[string]bool{}
			for j := 0; j < len(value.Content); j += 2 {
				k, v := value.Content[j], value.Content[j+1]
				if k.Tag != "!!str" || v.Tag != "!!str" || v.Kind != yaml.ScalarNode || keys[k.Value] {
					return project.Fail("invalid_input", "%s must contain unique string keys and string values", key.Value)
				}
				keys[k.Value] = true
			}
		} else if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
			return project.Fail("invalid_input", "%s must be a string", key.Value)
		}
	}
	for _, key := range shape["required"].([]string) {
		if !seen[key] {
			return project.Fail("invalid_input", "%s is required", key)
		}
	}
	return nil
}
