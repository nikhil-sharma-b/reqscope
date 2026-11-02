package project

// maxSampleDepth bounds nesting so deep or self-similar schemas stay readable.
const maxSampleDepth = 8

// SchemaSample builds a JSON-ready value that shows the fields a schema
// expects. References are already resolved when specifications load, so the
// schema is a plain tree.
func SchemaSample(schema map[string]any) any {
	return sampleValue(schema, 0)
}

// BodySample returns the body a request draft for ep would start from, so
// previews show exactly what the editor will open with. Form bodies return nil
// because they are edited as fields.
func BodySample(ep Endpoint) any {
	_, fields := EndpointDraft(ep)
	for _, f := range fields {
		if f.In == "body" && f.Name == "body" {
			return f.Suggestion
		}
	}
	return nil
}

// ResponseSamples returns a sample body for each status code that declares
// one, keyed by code. JSON is preferred when a response offers several media
// types; codes without a body are left out.
func ResponseSamples(ep Endpoint) map[string]any {
	result := map[string]any{}
	responses := object(ep.Operation["responses"])
	for _, code := range keys(responses) {
		r := object(responses[code])
		var sample any
		if content := object(r["content"]); len(content) > 0 {
			media := keys(content)[0]
			if _, ok := content["application/json"]; ok {
				media = "application/json"
			}
			sample = mediaSample(object(content[media]))
		} else {
			// Swagger 2 puts the schema on the response and examples by MIME type.
			examples := object(r["examples"])
			if ex, ok := examples["application/json"]; ok {
				sample = ex
			} else if names := keys(examples); len(names) > 0 {
				sample = examples[names[0]]
			} else if schema := object(r["schema"]); schema != nil {
				sample = SchemaSample(schema)
			}
		}
		if sample != nil {
			result[code] = sample
		}
	}
	return result
}

// mediaSample returns a body suggestion for one media type entry: the
// contract's own example first, then the first named example, then a sample
// built from the schema.
func mediaSample(entry map[string]any) any {
	if ex, ok := entry["example"]; ok && ex != nil {
		return ex
	}
	examples := object(entry["examples"])
	for _, name := range keys(examples) {
		if value, ok := object(examples[name])["value"]; ok && value != nil {
			return value
		}
	}
	if schema := object(entry["schema"]); schema != nil {
		return SchemaSample(schema)
	}
	return nil
}

func sampleValue(s map[string]any, depth int) any {
	if s == nil {
		return nil
	}
	for _, k := range []string{"example", "default", "const"} {
		if v, ok := s[k]; ok && v != nil {
			return v
		}
	}
	if values := anyList(s["enum"]); len(values) > 0 {
		return values[0]
	}
	if branches := anyList(s["oneOf"]); len(branches) > 0 {
		return sampleValue(object(branches[0]), depth)
	}
	if branches := anyList(s["anyOf"]); len(branches) > 0 {
		return sampleValue(object(branches[0]), depth)
	}
	if parts := anyList(s["allOf"]); len(parts) > 0 {
		return sampleValue(mergeAllOf(s, parts), depth)
	}
	kind, _ := s["type"].(string)
	if list := anyList(s["type"]); kind == "" && len(list) > 0 {
		// OpenAPI 3.1 allows a list of types; pick the first non-null one.
		for _, t := range list {
			if t != "null" {
				kind, _ = t.(string)
				break
			}
		}
	}
	if kind == "" {
		if s["properties"] != nil {
			kind = "object"
		} else if s["items"] != nil {
			kind = "array"
		}
	}
	switch kind {
	case "object":
		if depth >= maxSampleDepth {
			return map[string]any{}
		}
		result := map[string]any{}
		props := object(s["properties"])
		for _, name := range keys(props) {
			p := object(props[name])
			if p["readOnly"] == true {
				continue
			}
			result[name] = sampleValue(p, depth+1)
		}
		return result
	case "array":
		if depth >= maxSampleDepth {
			return []any{}
		}
		return []any{sampleValue(object(s["items"]), depth+1)}
	case "integer":
		return 0
	case "number":
		return 0.0
	case "boolean":
		return false
	case "string":
		return stringSample(s["format"])
	}
	return nil
}

func stringSample(format any) string {
	switch format {
	case "date":
		return "2026-01-01"
	case "date-time":
		return "2026-01-01T00:00:00Z"
	case "email":
		return "user@example.com"
	case "uuid":
		return "00000000-0000-0000-0000-000000000000"
	case "uri", "url":
		return "https://example.com"
	}
	return "string"
}

// mergeAllOf folds allOf parts into one schema so their properties combine.
func mergeAllOf(s map[string]any, parts []any) map[string]any {
	merged := map[string]any{}
	props := map[string]any{}
	for k, v := range s {
		if k != "allOf" && k != "properties" {
			merged[k] = v
		}
	}
	for name, p := range object(s["properties"]) {
		props[name] = p
	}
	for _, part := range parts {
		p := object(part)
		if nested := anyList(p["allOf"]); len(nested) > 0 {
			p = mergeAllOf(p, nested)
		}
		for k, v := range p {
			if k == "properties" {
				for name, x := range object(v) {
					props[name] = x
				}
			} else if _, ok := merged[k]; !ok {
				merged[k] = v
			}
		}
	}
	if len(props) > 0 {
		merged["properties"] = props
		if merged["type"] == nil {
			merged["type"] = "object"
		}
	}
	return merged
}
