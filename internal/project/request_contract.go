package project

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type GuidedField struct {
	Name        string `json:"name"`
	In          string `json:"in"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
	Suggestion  any    `json:"suggestion,omitempty"`
}

func contractParameters(ep Endpoint) []map[string]any {
	result := []map[string]any{}
	positions := map[string]int{}
	for _, raw := range []any{ep.PathInputs, ep.Operation["parameters"]} {
		items, _ := raw.([]any)
		for _, item := range items {
			p := object(item)
			key := fmt.Sprint(p["in"]) + ":" + fmt.Sprint(p["name"])
			if i, ok := positions[key]; ok {
				result[i] = p
			} else {
				positions[key] = len(result)
				result = append(result, p)
			}
		}
	}
	return result
}

// ContractDraft exposes suggestions separately so callers never overwrite edits.
func ContractDraft(root, id string) (RequestDraft, []GuidedField, error) {
	specs, err := Specifications(root)
	if err != nil {
		return RequestDraft{}, nil, err
	}
	for _, s := range specs {
		for _, ep := range s.Endpoints {
			if ep.ID != id {
				continue
			}
			d := RequestDraft{EndpointID: id, Module: ep.ModuleID, Method: ep.Method, URL: ep.Path, Path: map[string]string{}, BodyType: "raw"}
			fields := []GuidedField{}
			for _, p := range contractParameters(ep) {
				name, _ := p["name"].(string)
				in, _ := p["in"].(string)
				required, _ := p["required"].(bool)
				description, _ := p["description"].(string)
				example := p["example"]
				if example == nil {
					example = object(p["schema"])["example"]
				}
				fields = append(fields, GuidedField{name, in, required || in == "path", description, example})
			}
			rb := object(ep.Operation["requestBody"])
			if rb != nil {
				required, _ := rb["required"].(bool)
				description, _ := rb["description"].(string)
				content := object(rb["content"])
				media := keys(content)
				if len(media) > 0 {
					d.ContentType = media[0]
					if _, ok := content["application/json"]; ok {
						d.ContentType = "application/json"
						d.BodyType = "json"
					}
					fields = append(fields, GuidedField{"body", "body", required, description, object(content[d.ContentType])["example"]})
				}
			}
			return d, fields, nil
		}
	}
	return RequestDraft{}, nil, Fail("not_found", "endpoint not found")
}
func serializeContract(ep Endpoint, path map[string]string, query url.Values, headers http.Header) error {
	for _, p := range contractParameters(ep) {
		name, _ := p["name"].(string)
		in, _ := p["in"].(string)
		required, _ := p["required"].(bool)
		var values []string
		switch in {
		case "path":
			if v, ok := path[name]; ok {
				values = []string{v}
			}
			required = true
		case "query":
			values = query[name]
		case "header":
			values = headers.Values(name)
		case "body":
			continue
		default:
			return Fail("invalid_input", "unsupported parameter location %q", in)
		}
		if len(values) == 0 {
			if required {
				return Fail("invalid_input", "missing required %s input %q", in, name)
			}
			continue
		}
		schema := object(p["schema"])
		if schema == nil {
			schema = p
		}
		kind, _ := schema["type"].(string)
		style, _ := p["style"].(string)
		if style == "" {
			style = "simple"
			if in == "query" {
				style = "form"
			}
		}
		explode := in == "query"
		if v, ok := p["explode"].(bool); ok {
			explode = v
		}
		if in == "query" && style != "form" && style != "spaceDelimited" && style != "pipeDelimited" || in != "query" && style != "simple" {
			return Fail("invalid_input", "unsupported parameter serialization for %q", name)
		}
		if kind == "object" {
			return Fail("invalid_input", "object parameter serialization is unsupported")
		}
		if kind == "array" && in == "path" && len(values) == 1 {
			separator := ","
			switch p["collectionFormat"] {
			case "ssv":
				separator = " "
			case "pipes":
				separator = "|"
			case "tsv":
				separator = "\t"
			}
			values = strings.Split(values[0], separator)
		}
		itemSchema := schema
		if kind == "array" {
			itemSchema = object(schema["items"])
		}
		for _, value := range values {
			if err := validateScalar(value, itemSchema); err != nil {
				return Fail("invalid_input", "invalid parameter %q: %s", name, err)
			}
		}
		if kind != "array" {
			if len(values) > 1 && in == "path" {
				return Fail("invalid_input", "path parameter must be scalar")
			}
			continue
		}
		sep := ","
		if style == "spaceDelimited" {
			sep = " "
		}
		if style == "pipeDelimited" {
			sep = "|"
		}
		if format, ok := p["collectionFormat"].(string); ok {
			switch format {
			case "multi":
				explode = true
			case "csv":
				explode = false
			case "ssv":
				explode = false
				sep = " "
			case "pipes":
				explode = false
				sep = "|"
			case "tsv":
				explode = false
				sep = "\t"
			default:
				return Fail("invalid_input", "unsupported collection format")
			}
		}
		switch in {
		case "path":
			path[name] = strings.Join(values, sep)
		case "query":
			if !explode || style != "form" {
				query[name] = []string{strings.Join(values, sep)}
			}
		case "header":
			headers.Set(name, strings.Join(values, sep))
		}
	}
	return nil
}
func validateScalar(v string, schema map[string]any) error {
	switch schema["type"] {
	case "integer":
		if _, err := strconv.ParseInt(v, 10, 64); err != nil {
			return fmt.Errorf("expected integer")
		}
	case "number":
		if n, err := strconv.ParseFloat(v, 64); err != nil || math.IsInf(n, 0) || math.IsNaN(n) {
			return fmt.Errorf("expected number")
		}
	case "boolean":
		if v != "true" && v != "false" {
			return fmt.Errorf("expected boolean")
		}
	}
	if enum, ok := schema["enum"].([]any); ok {
		for _, item := range enum {
			if fmt.Sprint(item) == v {
				return nil
			}
		}
		return fmt.Errorf("value outside enum")
	}
	return nil
}
func validateContractBody(ep Endpoint, body, contentType string) error {
	rb := object(ep.Operation["requestBody"])
	if rb == nil {
		for _, p := range contractParameters(ep) {
			if p["in"] == "body" {
				rb = map[string]any{"required": p["required"], "content": map[string]any{"application/json": map[string]any{"schema": p["schema"]}}}
				break
			}
		}
	}
	if rb == nil {
		return nil
	}
	required, _ := rb["required"].(bool)
	if body == "" {
		if required {
			return Fail("invalid_input", "missing required body")
		}
		return nil
	}
	content := object(rb["content"])
	media := strings.Split(contentType, ";")[0]
	entry, ok := content[media]
	if !ok {
		return Fail("invalid_input", "body content type is not supported by contract")
	}
	if media != "application/json" && !strings.HasSuffix(media, "+json") {
		return nil
	}
	var v any
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		return Fail("invalid_input", "invalid JSON body")
	}
	return validateBodyValue(v, object(object(entry)["schema"]), "body")
}
func validateBodyValue(v any, s map[string]any, path string) error {
	valid := true
	switch s["type"] {
	case "object":
		_, valid = v.(map[string]any)
	case "array":
		_, valid = v.([]any)
	case "string":
		_, valid = v.(string)
	case "number":
		_, valid = v.(float64)
	case "integer":
		n, ok := v.(float64)
		valid = ok && n == float64(int64(n))
	case "boolean":
		_, valid = v.(bool)
	}
	if !valid {
		return Fail("invalid_input", "incompatible type at %s", path)
	}
	if obj, ok := v.(map[string]any); ok {
		required, _ := s["required"].([]any)
		for _, name := range required {
			if _, ok := obj[fmt.Sprint(name)]; !ok {
				return Fail("invalid_input", "missing required %s.%s", path, name)
			}
		}
		names := make([]string, 0, len(obj))
		for name := range obj {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if err := validateBodyValue(obj[name], object(object(s["properties"])[name]), path+"."+name); err != nil {
				return err
			}
		}
	}
	if a, ok := v.([]any); ok {
		for _, item := range a {
			if err := validateBodyValue(item, object(s["items"]), path+"[]"); err != nil {
				return err
			}
		}
	}
	return nil
}
