package project

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"go.yaml.in/yaml/v3"
)

// Specification retains the original bytes and normalized import base for refresh.
type Specification struct {
	LocalDocuments   map[string][]byte `json:"localDocuments,omitempty" yaml:"localDocuments,omitempty"`
	DeletedEndpoints []Endpoint        `json:"deletedEndpoints,omitempty" yaml:"deletedEndpoints,omitempty"`
	ID               string            `json:"id" yaml:"id"`
	ModuleID         string            `json:"moduleId" yaml:"moduleId"`
	Mode             string            `json:"mode" yaml:"mode"`
	Location         string            `json:"location" yaml:"location"`
	Version          string            `json:"version" yaml:"version"`
	Digest           string            `json:"digest" yaml:"digest"`
	Documents        map[string][]byte `json:"documents" yaml:"documents"`
	Base             map[string]any    `json:"base" yaml:"base"`
	Endpoints        []Endpoint        `json:"endpoints" yaml:"endpoints"`
	Schemas          []Schema          `json:"schemas" yaml:"schemas"`
	Diagnostics      []string          `json:"diagnostics" yaml:"diagnostics"`
	Complete         bool              `json:"complete" yaml:"complete"`
	Revision         string            `json:"revision" yaml:"-"`
}
type Endpoint struct {
	Revision   string         `json:"revision" yaml:"-"`
	ID         string         `json:"id" yaml:"id"`
	SourceID   string         `json:"sourceId" yaml:"sourceId"`
	ModuleID   string         `json:"moduleId" yaml:"moduleId"`
	Method     string         `json:"method" yaml:"method"`
	Path       string         `json:"path" yaml:"path"`
	Operation  map[string]any `json:"operation" yaml:"operation"`
	PathInputs any            `json:"pathInputs,omitempty" yaml:"pathInputs,omitempty"`
	Context    map[string]any `json:"context" yaml:"context"`
}
type Schema struct {
	Revision   string `json:"revision" yaml:"-"`
	ID         string `json:"id" yaml:"id"`
	SourceID   string `json:"sourceId" yaml:"sourceId"`
	Name       string `json:"name" yaml:"name"`
	Definition any    `json:"definition" yaml:"definition"`
}
type resolver struct {
	retainedOnly bool
	root         string
	documents    map[string][]byte
	parsed       map[string]any
	bytes        int
	nodes        int
	fetch        func(string) ([]byte, error)
}

func (r *resolver) read(name string) (any, error) {
	if v, ok := r.parsed[name]; ok {
		return v, nil
	}
	if r.retainedOnly {
		return nil, Fail("invalid_input", "reference must target a retained source document")
	}
	if len(r.documents) >= 64 {
		return nil, Fail("invalid_input", "reference file limit of 64 exceeded")
	}
	if r.fetch != nil {
		b, err := r.fetch(name)
		if err != nil {
			return nil, err
		}
		r.bytes += len(b)
		if r.bytes > 10*1024*1024 {
			return nil, Fail("invalid_input", "import exceeds 10 MiB")
		}
		var v any
		if err = Decode(b, &v); err != nil {
			return nil, Fail("invalid_input", "invalid remote contract document")
		}
		v, err = normalizeYAML(v)
		if err != nil {
			return nil, Fail("invalid_input", "invalid remote contract document")
		}
		r.documents[name] = b
		r.parsed[name] = v
		return v, nil
	}
	rel, err := filepath.Rel(r.root, name)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, Fail("invalid_input", "reference outside approved import root: %s", name)
	}
	if err = safePath(r.root, rel); err != nil {
		return nil, err
	}
	f, err := os.Open(name)
	if err != nil {
		return nil, Fail("invalid_input", "missing reference %s: %v", rel, err)
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 10*1024*1024+1))
	if err != nil {
		return nil, err
	}
	r.bytes += len(b)
	if r.bytes > 10*1024*1024 {
		return nil, Fail("invalid_input", "import exceeds 10 MiB")
	}
	var v any
	if err = Decode(b, &v); err != nil {
		return nil, Fail("invalid_input", "%s: %v", rel, err)
	}
	v, err = normalizeYAML(v)
	if err != nil {
		return nil, err
	}
	r.documents[filepath.ToSlash(rel)] = b
	r.parsed[name] = v
	return v, nil
}
func (r *resolver) resolve(v any, file string, stack map[string]bool, depth int) (any, error) {
	r.nodes++
	if depth > 64 || r.nodes > 100000 {
		return nil, Fail("invalid_input", "reference traversal limit exceeded")
	}
	switch v := v.(type) {
	case map[string]any:
		result := map[string]any{}
		if raw, ok := v["$ref"]; ok {
			ref, ok := raw.(string)
			if !ok {
				return nil, Fail("invalid_input", "$ref must be a string in %s", file)
			}
			u, err := url.Parse(ref)
			if err != nil || (r.fetch == nil && (u.Scheme != "" || u.Host != "" || u.RawQuery != "")) {
				return nil, Fail("invalid_input", "remote or invalid reference %q is not fetched; supply a local reference", ref)
			}
			target := file
			if r.fetch != nil {
				base, _ := url.Parse(file)
				location := base.ResolveReference(u)
				location.Fragment = ""
				target = location.String()
			} else if u.Path != "" {
				target = filepath.Clean(filepath.Join(filepath.Dir(file), filepath.FromSlash(u.Path)))
			}
			identity := target + "#" + u.Fragment
			if stack[identity] {
				return nil, Fail("invalid_input", "reference cycle at %s", ref)
			}
			value, err := r.read(target)
			if err != nil {
				return nil, err
			}
			if u.Fragment != "" {
				if !strings.HasPrefix(u.Fragment, "/") {
					return nil, Fail("invalid_input", "unsupported reference fragment %q", ref)
				}
				for _, part := range strings.Split(u.Fragment[1:], "/") {
					part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
					var ok bool
					if array, yes := value.([]any); yes {
						index, err := strconv.Atoi(part)
						if err != nil || index < 0 || index >= len(array) {
							return nil, Fail("invalid_input", "missing reference pointer %q", ref)
						}
						value = array[index]
						continue
					}
					obj, yes := value.(map[string]any)
					if !yes {
						return nil, Fail("invalid_input", "missing reference pointer %q", ref)
					}
					value, ok = obj[part]
					if !ok {
						return nil, Fail("invalid_input", "missing reference pointer %q", ref)
					}
				}
			}
			stack[identity] = true
			resolved, err := r.resolve(value, target, stack, depth+1)
			delete(stack, identity)
			if err != nil {
				return nil, err
			}
			object, ok := resolved.(map[string]any)
			if boolean, yes := resolved.(bool); yes && len(v) == 1 {
				return boolean, nil
			}
			if !ok {
				return nil, Fail("invalid_input", "reference %q must target an object", ref)
			}
			for k, x := range object {
				result[k] = x
			}
		}
		for k, x := range v {
			if k == "$ref" {
				continue
			}
			if dictionary := object(x); dictionary != nil && (k == "properties" || k == "schemas" || k == "definitions" || k == "parameters" || k == "securitySchemes" || k == "securityDefinitions" || k == "responses" || k == "content" || k == "headers") {
				entries := map[string]any{}
				for _, name := range keys(dictionary) {
					resolved, err := r.resolve(dictionary[name], file, stack, depth+1)
					if err != nil {
						return nil, err
					}
					entries[name] = resolved
				}
				result[k] = entries
				continue
			}
			if strings.HasPrefix(k, "x-") || k == "example" || k == "examples" || k == "default" || k == "enum" || k == "const" {
				result[k] = x
				continue
			}
			resolved, err := r.resolve(x, file, stack, depth+1)
			if err != nil {
				return nil, err
			}
			result[k] = resolved
		}
		return result, nil
	case []any:
		result := make([]any, len(v))
		for i, x := range v {
			resolved, err := r.resolve(x, file, stack, depth+1)
			if err != nil {
				return nil, err
			}
			result[i] = resolved
		}
		return result, nil
	default:
		return v, nil
	}
}
func object(v any) map[string]any { m, _ := v.(map[string]any); return m }
func keys(m map[string]any) []string {
	a := make([]string, 0, len(m))
	for k := range m {
		a = append(a, k)
	}
	sort.Strings(a)
	return a
}
func ImportSpecification(root, filename string, preview bool) (Specification, error) {
	return importSpecification(root, filename, preview, "")
}

// ImportReviewedSpecification applies only the exact document digest reviewed by the TUI.
func ImportReviewedSpecification(root, filename, digest string) (Specification, error) {
	return importSpecification(root, filename, false, digest)
}
func importSpecification(root, filename string, preview bool, expectedDigest string) (Specification, error) {
	var s Specification
	abs, err := filepath.Abs(filename)
	if err != nil {
		return s, err
	}
	r := resolver{root: filepath.Dir(abs), documents: map[string][]byte{}, parsed: map[string]any{}}
	v, err := r.read(abs)
	if err != nil {
		return s, err
	}
	return importResolvedSpecification(root, abs, "local", &r, v, preview, expectedDigest)
}

func importResolvedSpecification(root, abs, mode string, r *resolver, v any, preview bool, expectedDigest string) (Specification, error) {
	var s Specification
	doc := object(v)
	if doc == nil {
		return s, Fail("invalid_input", "contract must be an object")
	}
	version, _ := doc["openapi"].(string)
	if swagger, ok := doc["swagger"].(string); ok {
		version = swagger
	}
	if version != "2.0" && !strings.HasPrefix(version, "3.0.") && !strings.HasPrefix(version, "3.1.") {
		return s, Fail("unsupported_format", "supported versions are Swagger 2.0 and OpenAPI 3.0/3.1")
	}
	info := object(doc["info"])
	if title, ok := info["title"].(string); !ok || strings.TrimSpace(title) == "" {
		return s, Fail("invalid_input", "info.title is required")
	}
	if _, ok := info["version"].(string); !ok {
		return s, Fail("invalid_input", "info.version must be a string")
	}
	if object(doc["paths"]) == nil {
		return s, Fail("invalid_input", "paths must be an object")
	}
	resolved, err := r.resolve(doc, abs, map[string]bool{}, 0)
	if err != nil {
		return s, err
	}
	base := object(resolved)
	if err := validateContractFields(base, "contract"); err != nil {
		return s, err
	}
	s = Specification{ID: uuid.NewString(), ModuleID: uuid.NewString(), Mode: mode, Location: abs, Version: version, Documents: r.documents, Base: base, Endpoints: []Endpoint{}, Schemas: []Schema{}, Diagnostics: []string{}, Complete: false}
	// Import inspection does not perform response validation.
	s.Diagnostics = append(s.Diagnostics, "Contract inspection only; response schema validation is not implemented")
	context := map[string]any{}
	if security := object(object(base["components"])["securitySchemes"]); security != nil {
		context["securitySchemes"] = security
	}
	for _, k := range []string{"servers", "host", "basePath", "schemes", "consumes", "produces", "security", "securityDefinitions"} {
		if x, ok := base[k]; ok {
			context[k] = x
		}
	}
	paths := object(base["paths"])
	for _, path := range keys(paths) {
		if strings.HasPrefix(path, "x-") {
			continue
		}
		if !strings.HasPrefix(path, "/") || object(paths[path]) == nil {
			return Specification{}, Fail("invalid_input", "invalid path item %q", path)
		}
		item := object(paths[path])
		for _, method := range keys(item) {
			switch method {
			case "get", "put", "post", "delete", "options", "head", "patch", "trace":
			default:
				if method != "parameters" && method != "summary" && method != "description" && method != "servers" && !strings.HasPrefix(method, "x-") {
					s.Diagnostics = append(s.Diagnostics, "Unsupported path operation: "+path+" "+method)
				}
				continue
			}
			op := object(item[method])
			if op == nil || len(object(op["responses"])) == 0 {
				return Specification{}, Fail("invalid_input", "%s %s requires an operation object and responses", method, path)
			}
			for code, response := range object(op["responses"]) {
				if _, ok := object(response)["description"].(string); !ok {
					return Specification{}, Fail("invalid_input", "%s %s response %s requires description", method, path, code)
				}
			}
			id := uuid.NewSHA1(uuid.MustParse(s.ID), []byte(strings.ToUpper(method)+" "+path)).String()
			endpointContext := map[string]any{}
			for k, v := range context {
				endpointContext[k] = v
			}
			for _, k := range []string{"summary", "description", "servers"} {
				if v, ok := item[k]; ok {
					endpointContext[k] = v
				}
			}
			s.Endpoints = append(s.Endpoints, Endpoint{ID: id, SourceID: s.ID, ModuleID: s.ModuleID, Method: strings.ToUpper(method), Path: path, Operation: op, PathInputs: item["parameters"], Context: endpointContext})
		}
	}
	schemas := object(base["definitions"])
	if version != "2.0" {
		schemas = object(object(base["components"])["schemas"])
	}
	for _, name := range keys(schemas) {
		if object(schemas[name]) == nil {
			if _, ok := schemas[name].(bool); !ok || !strings.HasPrefix(version, "3.1.") {
				return Specification{}, Fail("invalid_input", "schema %s must be an object", name)
			}
		}
		s.Schemas = append(s.Schemas, Schema{ID: uuid.NewSHA1(uuid.MustParse(s.ID), []byte("schema "+name)).String(), SourceID: s.ID, Name: name, Definition: schemas[name]})
	}
	inspectConstraints(base, "", &s.Diagnostics)
	b, _ := json.Marshal(r.documents)
	s.Digest = Digest(b)
	if expectedDigest != "" && s.Digest != expectedDigest {
		return s, Fail("revision_conflict", "import files changed since preview; review again")
	}
	if preview {
		return s, nil
	}
	snapshot, err := Load(root)
	if err != nil {
		return s, err
	}
	snapshot.Manifest.Sources = append(snapshot.Manifest.Sources, s.ID)
	snapshot.Manifest.Modules = append(snapshot.Manifest.Modules, s.ModuleID)
	manifest, _ := yaml.Marshal(snapshot.Manifest)
	b, err = yaml.Marshal(s)
	if err != nil {
		return s, err
	}
	s.Revision = Digest(b)
	err = Apply(root, []Change{{Path: "reqscope.yaml", ExpectedRevision: snapshot.Revision, Content: manifest}, {Path: "specs/" + s.ID + ".yaml", Content: b}}, false)
	return s, err
}
func inspectConstraints(v any, path string, diagnostics *[]string) {
	switch v := v.(type) {
	case map[string]any:
		for _, k := range keys(v) {
			switch k {
			case "allOf", "anyOf", "oneOf", "not", "if", "then", "else", "unevaluatedProperties", "patternProperties", "dependentSchemas", "contains", "prefixItems", "callbacks", "webhooks", "links", "discriminator", "$dynamicRef":
				*diagnostics = append(*diagnostics, fmt.Sprintf("Unsupported constraint or operation %s/%s", path, k))
			}
			inspectConstraints(v[k], path+"/"+k, diagnostics)
		}
	case []any:
		for i, x := range v {
			inspectConstraints(x, fmt.Sprintf("%s/%d", path, i), diagnostics)
		}
	}
}
func Specifications(root string) ([]Specification, error) {
	var result []Specification
	err := locked(root, func() error { var err error; result, err = specifications(root); return err })
	return result, err
}
func specifications(root string) ([]Specification, error) {
	snapshot, err := load(root)
	if err != nil {
		return nil, err
	}
	result := []Specification{}
	for _, id := range snapshot.Manifest.Sources {
		if !validID(id) {
			return nil, Fail("invalid_input", "invalid source UUID")
		}
		path := "specs/" + id + ".yaml"
		if err := safePath(root, path); err != nil {
			return nil, err
		}
		b, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			return nil, err
		}
		var s Specification
		if err = Decode(b, &s); err != nil {
			return nil, err
		}
		if err = validateSpecification(s); err != nil {
			return nil, err
		}
		s.Revision = Digest(b)
		for i := range s.Endpoints {
			s.Endpoints[i].Revision = s.Revision
		}
		for i := range s.Schemas {
			s.Schemas[i].Revision = s.Revision
		}
		result = append(result, s)
	}
	return result, nil
}
func validateSpecification(s Specification) error {
	if !validID(s.ID) || !validID(s.ModuleID) || (s.Mode != "local" && s.Mode != "url") || s.Base == nil || len(s.Documents) == 0 || len(s.Digest) != 64 {
		return Fail("invalid_input", "invalid specification resource")
	}
	return nil
}

// Validate the types of fields used by inspection, without interpreting extensions.
func validateContractFields(value any, location string) error {
	switch value := value.(type) {
	case map[string]any:
		for _, key := range keys(value) {
			v := value[key]
			path := location + "/" + key
			if strings.HasPrefix(key, "x-") {
				continue
			}
			if key == "parameters" && (location == "contract" || location == "contract/components") {
				dictionary := object(v)
				if dictionary == nil {
					return Fail("invalid_input", "%s must be an object", path)
				}
				for _, name := range keys(dictionary) {
					if err := validateContractFields(dictionary[name], path+"/"+name); err != nil {
						return err
					}
				}
				continue
			}
			switch key {
			case "parameters", "tags", "required", "enum", "allOf", "anyOf", "oneOf", "servers", "security":
				// required is a boolean on parameters and an array on schemas.
				if key == "required" {
					if _, ok := v.(bool); ok {
						break
					}
				}
				a, ok := v.([]any)
				if !ok {
					return Fail("invalid_input", "%s must be an array", path)
				}
				if key == "tags" && location == "contract" {
					for _, item := range a {
						if _, ok := object(item)["name"].(string); !ok {
							return Fail("invalid_input", "%s tags require name", path)
						}
					}
				} else if key == "tags" || key == "required" {
					for _, item := range a {
						if _, ok := item.(string); !ok {
							return Fail("invalid_input", "%s must contain strings", path)
						}
					}
				}
				if key == "parameters" {
					for _, item := range a {
						p := object(item)
						name, nok := p["name"].(string)
						in, iok := p["in"].(string)
						if !nok || name == "" || !iok || in == "" {
							return Fail("invalid_input", "%s parameters require name and in", path)
						}
					}
				}
			case "summary", "description", "operationId":
				if _, ok := v.(string); !ok {
					return Fail("invalid_input", "%s must be a string", path)
				}
			case "type":
				if strings.Contains(location, "/securitySchemes/") || strings.Contains(location, "/securityDefinitions/") {
					if _, ok := v.(string); !ok {
						return Fail("invalid_input", "%s must be a string", path)
					}
					break
				}
				valid := func(x any) bool {
					t, ok := x.(string)
					return ok && (t == "file" && value["in"] == "formData" || t == "object" || t == "array" || t == "string" || t == "number" || t == "integer" || t == "boolean" || t == "null")
				}
				if a, ok := v.([]any); ok {
					for _, t := range a {
						if !valid(t) {
							return Fail("invalid_input", "invalid schema type at %s", path)
						}
					}
				} else if !valid(v) {
					return Fail("invalid_input", "invalid schema type at %s", path)
				}
			case "properties", "responses", "content", "encoding":
				if object(v) == nil {
					return Fail("invalid_input", "%s must be an object", path)
				}
			}
			// Property names, media types and response codes are user-defined keys.
			if key == "properties" || key == "schemas" || key == "definitions" || key == "responses" || key == "content" || key == "encoding" {
				for _, name := range keys(object(v)) {
					if err := validateContractFields(object(v)[name], path+"/"+name); err != nil {
						return err
					}
				}
				continue
			}
			if key == "example" || key == "examples" || key == "default" || key == "enum" || key == "const" {
				continue
			}
			if err := validateContractFields(v, path); err != nil {
				return err
			}
		}
	case []any:
		for i, item := range value {
			if err := validateContractFields(item, fmt.Sprintf("%s/%d", location, i)); err != nil {
				return err
			}
		}
	}
	return nil
}

func normalizeYAML(v any) (any, error) {
	switch v := v.(type) {
	case map[any]any:
		result := map[string]any{}
		for key, x := range v {
			var name string
			switch key := key.(type) {
			case string:
				name = key
			case int:
				name = strconv.Itoa(key)
			default:
				return nil, Fail("invalid_input", "contract keys must be strings or response status codes")
			}
			if _, exists := result[name]; exists {
				return nil, Fail("invalid_input", "duplicate contract key %s", name)
			}
			value, err := normalizeYAML(x)
			if err != nil {
				return nil, err
			}
			result[name] = value
		}
		return result, nil
	case map[string]any:
		for key, x := range v {
			value, err := normalizeYAML(x)
			if err != nil {
				return nil, err
			}
			v[key] = value
		}
		return v, nil
	case []any:
		for i, x := range v {
			value, err := normalizeYAML(x)
			if err != nil {
				return nil, err
			}
			v[i] = value
		}
		return v, nil
	default:
		return v, nil
	}
}
