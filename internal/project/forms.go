package project

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func SupportedBodyType(kind string) bool {
	return kind == "" || kind == "raw" || kind == "json" || kind == "form" || kind == "multipart"
}

type preparedFormField struct {
	FormField
	data []byte
}

// Upload references are relative to the project root unless absolute.
func readUpload(root, path string) ([]byte, error) {
	if strings.Contains(path, "{{") {
		return nil, Fail("invalid_input", "upload paths must be explicit local references")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0444 == 0 {
		return nil, Fail("invalid_input", "upload must be a readable regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, Fail("invalid_input", "upload file is missing or unreadable")
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0444 == 0 {
		return nil, Fail("invalid_input", "upload must be a readable regular file")
	}
	const limit = 32 << 20
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, Fail("invalid_input", "upload file is unreadable")
	}
	if len(b) > limit {
		return nil, Fail("invalid_input", "upload exceeds 32 MiB")
	}
	return b, nil
}

func prepareForm(root string, d RequestDraft, vars map[string]string, ep *Endpoint) ([]preparedFormField, error) {
	if d.BodyType != "form" && d.BodyType != "multipart" {
		if len(d.Form) > 0 {
			return nil, Fail("invalid_input", "form fields require form or multipart body type")
		}
		return nil, nil
	}
	fields := make([]preparedFormField, 0, len(d.Form))
	totalBytes := 0
	for _, f := range d.Form {
		if f.Name == "" || strings.ContainsAny(f.Name, "\r\n") || strings.Contains(f.Name, "{{") {
			return nil, Fail("invalid_input", "invalid form field name")
		}
		if f.File != "" && (d.BodyType != "multipart" || f.Value != "") {
			return nil, Fail("invalid_input", "files require multipart and cannot also contain a value")
		}
		if f.ContentType != "" {
			if _, _, err := mime.ParseMediaType(f.ContentType); err != nil || strings.ContainsAny(f.ContentType, "\r\n") {
				return nil, Fail("invalid_input", "invalid form content type")
			}
		}
		value, err := interpolate(f.Value, vars)
		if err != nil {
			return nil, err
		}
		f.Value = value
		item := preparedFormField{FormField: f}
		if f.File != "" {
			item.data, err = readUpload(root, f.File)
			if err != nil {
				return nil, err
			}
			totalBytes += len(item.data)
			if totalBytes > 32<<20 {
				return nil, Fail("invalid_input", "combined uploads exceed 32 MiB")
			}
			if strings.ContainsAny(filepath.Base(f.File), "\r\n") {
				return nil, Fail("invalid_input", "invalid upload filename")
			}
		}
		fields = append(fields, item)
	}
	if ep != nil {
		return serializeFormContract(*ep, d, fields)
	}
	return fields, nil
}

func encodeForm(fields []preparedFormField, kind, contentType string) (string, string, error) {
	media := "application/x-www-form-urlencoded"
	if kind == "multipart" {
		media = "multipart/form-data"
	}
	params := map[string]string{}
	if contentType != "" {
		actual, p, err := mime.ParseMediaType(contentType)
		if err != nil || actual != media {
			return "", "", Fail("invalid_input", "content type does not match form body type")
		}
		params = p
	}
	if kind == "form" {
		values := url.Values{}
		for _, f := range fields {
			values.Add(f.Name, f.Value)
		}
		if contentType == "" {
			contentType = media
		}
		return values.Encode(), contentType, nil
	}
	var b bytes.Buffer
	writer := multipart.NewWriter(&b)
	// Stable boundaries keep production confirmation valid for unchanged inputs.
	if params["boundary"] == "" {
		fingerprint := []byte{}
		for _, f := range fields {
			metadata, _ := json.Marshal(f.FormField)
			fingerprint = append(fingerprint, metadata...)
			fingerprint = append(fingerprint, []byte(Digest(f.data))...)
		}
		params["boundary"] = "reqscope-" + Digest(fingerprint)[:48]
	}
	if boundary := params["boundary"]; boundary != "" {
		if err := writer.SetBoundary(boundary); err != nil {
			return "", "", Fail("invalid_input", "invalid multipart boundary")
		}
	}
	for _, f := range fields {
		disposition := map[string]string{"name": f.Name}
		data := []byte(f.Value)
		ct := f.ContentType
		if f.File != "" {
			disposition["filename"] = filepath.Base(f.File)
			data = f.data
			if ct == "" {
				ct = "application/octet-stream"
			}
		}
		header := textproto.MIMEHeader{"Content-Disposition": {mime.FormatMediaType("form-data", disposition)}}
		if ct != "" {
			header.Set("Content-Type", ct)
		}
		if bytes.Contains(data, []byte("\r\n--"+writer.Boundary())) || bytes.HasPrefix(data, []byte("--"+writer.Boundary())) {
			return "", "", Fail("invalid_input", "multipart boundary conflicts with field contents")
		}
		part, err := writer.CreatePart(header)
		if err != nil {
			return "", "", Fail("invalid_input", "cannot encode multipart field")
		}
		if _, err = part.Write(data); err != nil {
			return "", "", Fail("invalid_input", "cannot encode multipart field")
		}
	}
	if err := writer.Close(); err != nil {
		return "", "", Fail("invalid_input", "cannot finish multipart body")
	}
	params["boundary"] = writer.Boundary()
	return b.String(), mime.FormatMediaType(media, params), nil
}

func formContract(ep Endpoint, media string) (map[string]any, map[string]any, bool, error) {
	rb := object(ep.Operation["requestBody"])
	if rb != nil {
		entry, ok := object(rb["content"])[media]
		if !ok {
			return nil, nil, false, Fail("invalid_input", "body content type is not supported by contract")
		}
		required, _ := rb["required"].(bool)
		return object(object(entry)["schema"]), object(object(entry)["encoding"]), required, nil
	}
	consumes := ep.Operation["consumes"]
	if consumes == nil {
		consumes = ep.Context["consumes"]
	}
	if len(anyList(consumes)) > 0 {
		supported := false
		for _, value := range anyList(consumes) {
			if value == media {
				supported = true
			}
		}
		if !supported {
			return nil, nil, false, Fail("invalid_input", "body content type is not supported by contract")
		}
	}
	properties := map[string]any{}
	required := []any{}
	encoding := map[string]any{}
	for _, p := range contractParameters(ep) {
		if p["in"] != "formData" {
			continue
		}
		name, _ := p["name"].(string)
		properties[name] = p
		if p["required"] == true {
			required = append(required, name)
		}
		if p["type"] == "array" {
			format := p["collectionFormat"]
			if format == nil {
				format = "csv"
			}
			encoding[name] = map[string]any{"collectionFormat": format}
		}
	}
	return map[string]any{"type": "object", "properties": properties, "required": required}, encoding, len(required) > 0, nil
}

func serializeFormContract(ep Endpoint, d RequestDraft, fields []preparedFormField) ([]preparedFormField, error) {
	media := "application/x-www-form-urlencoded"
	if d.BodyType == "multipart" {
		media = "multipart/form-data"
	}
	schema, encoding, required, err := formContract(ep, media)
	if err != nil {
		return nil, err
	}
	if required && len(fields) == 0 {
		return nil, Fail("invalid_input", "missing required form body")
	}
	values := map[string][]preparedFormField{}
	for _, f := range fields {
		values[f.Name] = append(values[f.Name], f)
	}
	for _, name := range anyList(schema["required"]) {
		if len(values[fmt.Sprint(name)]) == 0 {
			return nil, Fail("invalid_input", "missing required form input")
		}
	}
	result := []preparedFormField{}
	seen := map[string]bool{}
	for _, f := range fields {
		if seen[f.Name] {
			continue
		}
		seen[f.Name] = true
		items := values[f.Name]
		property := object(object(schema["properties"])[f.Name])
		enc := object(encoding[f.Name])
		if style, ok := enc["style"].(string); ok && style != "form" {
			return nil, Fail("invalid_input", "unsupported form serialization")
		}
		if enc["allowReserved"] == true || enc["headers"] != nil || property["type"] == "object" {
			return nil, Fail("invalid_input", "unsupported form serialization")
		}
		itemSchema := property
		if property["type"] == "array" {
			itemSchema = object(property["items"])
		}
		if itemSchema["type"] == "object" || itemSchema["type"] == "array" {
			return nil, Fail("invalid_input", "unsupported nested form serialization")
		}
		binary := itemSchema["format"] == "binary" || itemSchema["type"] == "file"
		for i := range items {
			if items[i].File != "" && itemSchema["type"] != nil && !binary {
				return nil, Fail("invalid_input", "file input is incompatible with contract type")
			}
			if binary && items[i].File == "" {
				return nil, Fail("invalid_input", "file input requires a local reference")
			}
			if items[i].File == "" {
				if err := validateScalar(items[i].Value, itemSchema); err != nil {
					return nil, Fail("invalid_input", "incompatible form input type")
				}
			}
			if items[i].ContentType == "" {
				items[i].ContentType, _ = enc["contentType"].(string)
			}
			if items[i].ContentType != "" {
				if _, _, err := mime.ParseMediaType(items[i].ContentType); err != nil || strings.ContainsAny(items[i].ContentType, "\r\n") {
					return nil, Fail("invalid_input", "invalid form content type")
				}
			}
		}
		explode := true
		if v, ok := enc["explode"].(bool); ok {
			explode = v
		}
		sep := ","
		switch enc["collectionFormat"] {
		case "multi", nil:
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
			return nil, Fail("invalid_input", "unsupported form collection format")
		}
		if property["type"] == "array" && !explode {
			parts := []string{}
			for _, item := range items {
				if item.File != "" {
					return nil, Fail("invalid_input", "file arrays require repeated parts")
				}
				parts = append(parts, item.Value)
			}
			items[0].Value = strings.Join(parts, sep)
			items = items[:1]
		}
		result = append(result, items...)
	}
	return result, nil
}

func anyList(v any) []any { a, _ := v.([]any); return a }
