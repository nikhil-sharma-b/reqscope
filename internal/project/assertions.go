package project

import (
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"reflect"
	"strconv"
	"strings"
)

// CheckResult reports validation independently of the HTTP transport.
type CheckResult struct {
	Kind    string `json:"kind"`
	Outcome string `json:"outcome"`
	Message string `json:"message,omitempty"`
}
type assertion struct {
	Kind    string          `json:"kind"`
	Name    string          `json:"name,omitempty"`
	Pointer string          `json:"pointer,omitempty"`
	Value   json.RawMessage `json:"value,omitempty"`
	Min     int             `json:"min,omitempty"`
	Max     int             `json:"max,omitempty"`
}
type validation struct {
	Assertions     []assertion `json:"assertions,omitempty"`
	ResponseSchema bool        `json:"responseSchema,omitempty"`
}

func decodeValidation(raw map[string]any) (validation, error) {
	var v validation
	b, err := json.Marshal(raw)
	if err != nil {
		return v, Fail("invalid_input", "invalid validation settings")
	}
	if len(raw) == 0 {
		return v, nil
	}
	if rawValue, exists := raw["responseSchema"]; exists {
		if _, ok := rawValue.(bool); !ok {
			return v, Fail("invalid_input", "responseSchema must be a boolean")
		}
	}
	if rawValue, exists := raw["assertions"]; exists {
		if _, ok := rawValue.([]any); !ok {
			return v, Fail("invalid_input", "assertions must be an array")
		}
	}
	decoder := json.NewDecoder(strings.NewReader(string(b)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&v); err != nil {
		return v, Fail("invalid_input", "invalid validation settings")
	}
	var fields struct {
		Assertions []map[string]json.RawMessage `json:"assertions"`
	}
	_ = json.Unmarshal(b, &fields)
	for i, a := range v.Assertions {
		allowed := map[string]bool{"kind": true}
		switch a.Kind {
		case "status-equals":
			allowed["value"] = true
		case "status-range":
			allowed["min"] = true
			allowed["max"] = true
		case "header-present":
			allowed["name"] = true
		case "header-equals":
			allowed["name"] = true
			allowed["value"] = true
		case "json-present":
			allowed["pointer"] = true
		case "json-equals":
			allowed["pointer"] = true
			allowed["value"] = true
		}
		for name, value := range fields.Assertions[i] {
			if !allowed[name] || string(value) == "null" && !(name == "value" && a.Kind == "json-equals") {
				return v, Fail("invalid_input", "invalid response assertion fields")
			}
		}

		valid := false
		switch a.Kind {
		case "status-equals":
			var status int
			valid = json.Unmarshal(a.Value, &status) == nil && status >= 100 && status <= 599 && a.Name == "" && a.Pointer == "" && a.Min == 0 && a.Max == 0
		case "status-range":
			valid = a.Min >= 100 && a.Max <= 599 && a.Min <= a.Max && len(a.Value) == 0 && a.Name == "" && a.Pointer == ""
		case "header-present", "header-equals":
			valid = validHeaderName(a.Name) && a.Pointer == "" && a.Min == 0 && a.Max == 0
			if a.Kind == "header-present" {
				valid = valid && len(a.Value) == 0
			} else {
				var s string
				valid = valid && json.Unmarshal(a.Value, &s) == nil && string(a.Value) != "null"
			}
		case "json-present", "json-equals":
			valid = validPointer(a.Pointer) && a.Name == "" && a.Min == 0 && a.Max == 0
			if a.Kind == "json-present" {
				valid = valid && len(a.Value) == 0
			} else {
				valid = valid && json.Valid(a.Value)
			}
		}
		if !valid {
			return v, Fail("invalid_input", "invalid response assertion")
		}
	}
	return v, nil
}
func validPointer(p string) bool {
	if p != "" && !strings.HasPrefix(p, "/") {
		return false
	}
	for i := 0; i < len(p); i++ {
		if p[i] == '~' {
			i++
			if i >= len(p) || p[i] != '0' && p[i] != '1' {
				return false
			}
		}
	}
	return true
}
func pointerValue(v any, p string) (any, bool) {
	if p == "" {
		return v, true
	}
	for _, token := range strings.Split(p[1:], "/") {
		token = strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
		switch x := v.(type) {
		case map[string]any:
			var ok bool
			v, ok = x[token]
			if !ok {
				return nil, false
			}
		case []any:
			i, err := strconv.Atoi(token)
			if err != nil || i < 0 || i >= len(x) || strconv.Itoa(i) != token {
				return nil, false
			}
			v = x[i]
		default:
			return nil, false
		}
	}
	return v, true
}
func jsonValue(raw string) (any, error) {
	var v any
	d := json.NewDecoder(strings.NewReader(raw))
	d.UseNumber()
	if !json.Valid([]byte(raw)) {
		return nil, fmt.Errorf("invalid JSON")
	}
	err := d.Decode(&v)
	return v, err
}
func typedEqual(a, b any) bool {
	switch x := a.(type) {
	case json.Number:
		y, ok := b.(json.Number)
		if !ok {
			return false
		}
		n, nok := new(big.Rat).SetString(string(x))
		m, mok := new(big.Rat).SetString(string(y))
		return nok && mok && n.Cmp(m) == 0
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !typedEqual(x[i], y[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for k, v := range x {
			w, ok := y[k]
			if !ok || !typedEqual(v, w) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(a, b)
	}
}
func responseChecks(r Exchange, v validation, ep *Endpoint) []CheckResult {
	checks := []CheckResult{}
	body, bodyErr := jsonValue(r.Body)
	unavailable := r.Truncated || r.Binary || r.BodyWithheld
	for _, a := range v.Assertions {
		outcome := "passed"
		pass := false
		switch a.Kind {
		case "status-equals":
			var n int
			_ = json.Unmarshal(a.Value, &n)
			pass = r.Status == n
		case "status-range":
			pass = r.Status >= a.Min && r.Status <= a.Max
		case "header-present":
			_, pass = r.Headers[http.CanonicalHeaderKey(a.Name)]
		case "header-equals":
			var s string
			_ = json.Unmarshal(a.Value, &s)
			for _, actual := range r.Headers.Values(a.Name) {
				if actual == s {
					pass = true
				}
			}
		case "json-present", "json-equals":
			if unavailable {
				outcome = "incomplete"
			} else if bodyErr == nil {
				actual, exists := pointerValue(body, a.Pointer)
				pass = exists
				if a.Kind == "json-equals" {
					expected, _ := jsonValue(string(a.Value))
					pass = exists && typedEqual(actual, expected)
				}
			}
		}
		if outcome == "passed" && !pass {
			outcome = "failed"
		}
		checks = append(checks, CheckResult{Kind: a.Kind, Outcome: outcome})
	}
	if v.ResponseSchema {
		check := CheckResult{Kind: "response-schema", Outcome: "passed"}
		schema, found := responseSchema(ep, r)
		switch {
		case unavailable:
			check.Outcome = "incomplete"
			check.Message = "Complete response body unavailable"
		case !found:
			check.Outcome = "incomplete"
			check.Message = "Response schema unavailable for status or media type"
		default:
			check.Outcome, check.Message = checkSchema(body, schema, bodyErr)
		}
		checks = append(checks, check)
	}
	return checks
}

func responseSchema(ep *Endpoint, r Exchange) (any, bool) {
	if ep == nil {
		return nil, false
	}
	responses := object(ep.Operation["responses"])
	raw, ok := responses[strconv.Itoa(r.Status)]
	if !ok {
		raw, ok = responses[fmt.Sprintf("%dXX", r.Status/100)]
	}
	if !ok {
		raw, ok = responses["default"]
	}
	if !ok {
		return nil, false
	}
	response := object(raw)
	if schema, ok := response["schema"]; ok {
		return schema, true
	}
	media := strings.ToLower(strings.TrimSpace(strings.Split(r.Headers.Get("Content-Type"), ";")[0]))
	content := object(response["content"])
	entry, ok := content[media]
	if !ok {
		entry, ok = content[strings.Split(media, "/")[0]+"/*"]
	}
	if !ok {
		entry, ok = content["*/*"]
	}
	if !ok {
		return nil, false
	}
	schema, ok := object(entry)["schema"]
	return schema, ok
}

// Unsupported constraints are detected throughout the schema, including optional
// properties absent from this response. They can never produce a complete pass.
func supportedSchema(raw any, depth int) bool {
	if depth > 64 {
		return false
	}
	if _, ok := raw.(bool); ok {
		return true
	}
	s, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	for k, v := range s {
		switch k {
		case "title", "description", "example", "examples", "default", "deprecated", "readOnly", "writeOnly", "$schema", "$id":
		case "type":
			valid := func(v any) bool {
				switch v {
				case "object", "array", "string", "integer", "number", "boolean", "null":
					return true
				}
				return false
			}
			if a, ok := v.([]any); ok {
				for _, t := range a {
					if !valid(t) {
						return false
					}
				}
			} else if !valid(v) {
				return false
			}
		case "nullable":
			if _, ok := v.(bool); !ok {
				return false
			}
		case "required":
			a, ok := v.([]any)
			if !ok {
				return false
			}
			for _, n := range a {
				if _, ok := n.(string); !ok {
					return false
				}
			}
		case "enum":
			if _, ok := v.([]any); !ok {
				return false
			}
		case "const":
		case "properties":
			p, ok := v.(map[string]any)
			if !ok {
				return false
			}
			for _, child := range p {
				if !supportedSchema(child, depth+1) {
					return false
				}
			}
		case "items", "additionalProperties":
			if !supportedSchema(v, depth+1) {
				return false
			}
		default:
			if !strings.HasPrefix(k, "x-") {
				return false
			}
		}
	}
	return true
}
func checkSchema(body, raw any, bodyErr error) (string, string) {
	if !supportedSchema(raw, 0) {
		return "incomplete", "Response schema contains unsupported constraints"
	}
	if bodyErr != nil {
		return "failed", "Response is not valid JSON"
	}
	// Normalize contract numbers to the same lossless representation as the body.
	b, _ := json.Marshal(raw)
	normalized, _ := jsonValue(string(b))
	if !schemaMatches(body, normalized) {
		return "failed", "Response does not match its schema"
	}
	return "passed", ""
}
func schemaMatches(v, raw any) bool {
	if b, ok := raw.(bool); ok {
		return b
	}
	s := object(raw)
	matchesType := func(t any) bool {
		switch t {
		case "null":
			return v == nil
		case "object":
			_, ok := v.(map[string]any)
			return ok
		case "array":
			_, ok := v.([]any)
			return ok
		case "string":
			_, ok := v.(string)
			return ok
		case "boolean":
			_, ok := v.(bool)
			return ok
		case "number":
			_, ok := v.(json.Number)
			return ok
		case "integer":
			n, ok := v.(json.Number)
			if !ok {
				return false
			}
			r, ok := new(big.Rat).SetString(string(n))
			return ok && r.IsInt()
		}
		return false
	}
	if t, ok := s["type"]; ok && !(v == nil && s["nullable"] == true) {
		valid := false
		if a, ok := t.([]any); ok {
			for _, kind := range a {
				valid = valid || matchesType(kind)
			}
		} else {
			valid = matchesType(t)
		}
		if !valid {
			return false
		}
	}
	if expected, ok := s["const"]; ok && !typedEqual(v, expected) {
		return false
	}
	if enum, ok := s["enum"].([]any); ok {
		found := false
		for _, e := range enum {
			found = found || typedEqual(v, e)
		}
		if !found {
			return false
		}
	}
	if obj, ok := v.(map[string]any); ok {
		for _, name := range anyList(s["required"]) {
			if _, exists := obj[fmt.Sprint(name)]; !exists {
				return false
			}
		}
		properties := object(s["properties"])
		for name, value := range obj {
			if child, exists := properties[name]; exists {
				if !schemaMatches(value, child) {
					return false
				}
			} else if child, exists := s["additionalProperties"]; exists && !schemaMatches(value, child) {
				return false
			}
		}
	}
	if items, ok := v.([]any); ok {
		if child, exists := s["items"]; exists {
			for _, item := range items {
				if !schemaMatches(item, child) {
					return false
				}
			}
		}
	}
	return true
}

// ValidationInputSchema describes the same declarative input used by both adapters.
func ValidationInputSchema() map[string]any {
	variants := []any{}
	for _, kind := range []string{"status-equals", "status-range", "header-present", "header-equals", "json-present", "json-equals"} {
		props := map[string]any{"kind": map[string]any{"const": kind}}
		required := []string{"kind"}
		switch kind {
		case "status-equals":
			props["value"] = map[string]any{"type": "integer", "minimum": 100, "maximum": 599}
			required = append(required, "value")
		case "status-range":
			for _, key := range []string{"min", "max"} {
				props[key] = map[string]any{"type": "integer", "minimum": 100, "maximum": 599}
				required = append(required, key)
			}
		case "header-present", "header-equals":
			props["name"] = map[string]any{"type": "string", "minLength": 1}
			required = append(required, "name")
			if kind == "header-equals" {
				props["value"] = map[string]any{"type": "string"}
				required = append(required, "value")
			}
		case "json-present", "json-equals":
			props["pointer"] = map[string]any{"type": "string"}
			if kind == "json-equals" {
				props["value"] = map[string]any{}
				required = append(required, "value")
			}
		}
		variants = append(variants, map[string]any{"type": "object", "additionalProperties": false, "properties": props, "required": required})
	}
	return map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"assertions": map[string]any{"type": "array", "items": map[string]any{"oneOf": variants}}, "responseSchema": map[string]any{"type": "boolean"}}}
}
