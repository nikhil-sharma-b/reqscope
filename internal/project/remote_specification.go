package project

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// SourceImportOptions contains local credential references, never credential values.
type SourceImportOptions struct {
	Interactive bool
	Environment string
	Module      string
	Headers     []RequestField
	Query       []RequestField
	Credentials *Credentials
}
type SourceCandidate struct {
	Name     string         `json:"name"`
	Location string         `json:"location"`
	Spec     map[string]any `json:"spec,omitempty"`
}
type SourceDiscovery struct {
	Candidates []SourceCandidate `json:"candidates"`
	Resources  []string          `json:"resources"`
	Hint       string            `json:"hint,omitempty"`
	Warnings   []string          `json:"warnings,omitempty"`
}

func sourceURL(raw string) (string, error) {
	origin, err := Origin(raw)
	if err != nil {
		return "", err
	}
	u, _ := url.Parse(raw)
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", Fail("invalid_input", "invalid source query parameters")
	}
	for name, values := range query {
		if sensitivePosition(strings.ToLower(strings.ReplaceAll(name, "-", "_"))) {
			return "", Fail("invalid_input", "source URLs must omit authentication parameters; use protected credential bindings")
		}
		for _, value := range values {
			if strings.Contains(value, "{{") {
				return "", Fail("invalid_input", "source URLs cannot embed credential references")
			}
		}
	}
	return origin, nil
}

// ApproveSourceOrigin is an interactive-only boundary. Grants are local to this project.
func ApproveSourceOrigin(root, raw string) error {
	origin, err := sourceURL(raw)
	if err != nil {
		return err
	}
	return locked(root, func() error {
		s, err := load(root)
		if err != nil {
			return err
		}
		a, p, err := readAccess(root, s)
		if err != nil {
			return err
		}
		if a.SourceOrigins == nil {
			a.SourceOrigins = map[string]bool{}
		}
		a.SourceOrigins[origin] = true
		return writeAccess(p, a)
	})
}
func checkedSourceURL(root, raw string, opts SourceImportOptions) (string, error) {
	c := opts.Credentials
	if c == nil {
		c = DefaultCredentials
	}
	if err := c.Refresh(root); err != nil {
		return "", err
	}
	if c.Sanitize(raw) != raw {
		return "", Fail("invalid_input", "source URL contains protected credentials; use its raw URL with protected bindings")
	}
	return sourceURL(raw)
}
func checkSourceOrigin(root, raw string, opts SourceImportOptions) error {
	origin, err := checkedSourceURL(root, raw, opts)
	if err != nil {
		return err
	}
	if opts.Environment != "" && opts.Module != "" {
		if _, err := CheckCLIAccess(root, opts.Environment, opts.Module, raw); err == nil {
			return nil
		}
	}
	return locked(root, func() error {
		s, err := load(root)
		if err != nil {
			return err
		}
		a, _, err := readAccess(root, s)
		if err != nil {
			return err
		}
		if !a.SourceOrigins[origin] {
			return Fail("access_denied", "source origin %s requires approval in the TUI", origin)
		}
		return nil
	})
}

type sourceFetcher struct {
	root      string
	opts      SourceImportOptions
	bytes     int
	resources int
}

func (f *sourceFetcher) fetch(raw string) ([]byte, error) {
	if err := checkSourceOrigin(f.root, raw, f.opts); err != nil {
		return nil, err
	}
	f.resources++
	if f.resources > 64 {
		return nil, Fail("invalid_input", "source resource limit of 64 exceeded")
	}
	c := f.opts.Credentials
	if c == nil {
		c = DefaultCredentials
	}
	if err := c.Refresh(f.root); err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DisableKeepAlives = true
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	if err := checkSourceOrigin(f.root, raw, f.opts); err != nil {
		return nil, err
	}
	req, _ := http.NewRequestWithContext(context.Background(), "GET", raw, nil)
	headers, query := f.opts.Headers, f.opts.Query
	if len(headers)+len(query) == 0 && f.opts.Environment != "" && f.opts.Module != "" {
		items, err := credentialItems(f.root, f.opts.Environment)
		if err != nil {
			return nil, err
		}
		for _, name := range keysCredentials(items) {
			item := items[name]
			if item.Module != f.opts.Module || item.Destination != raw {
				continue
			}
			field := RequestField{Name: item.Field, Value: "{{secret:" + item.Name + "}}"}
			if item.In == "header" {
				headers = append(headers, field)
			}
			if item.In == "query" {
				query = append(query, field)
			}
		}
	}
	if len(headers)+len(query) > 0 {
		d := RequestDraft{Module: f.opts.Module, Method: "GET", URL: raw, Headers: headers, Query: query}
		p, err := PrepareRequestWithCredentials(f.root, f.opts.Environment, d, f.opts.Interactive, c)
		if err != nil {
			return nil, err
		}
		if _, err = c.attach(f.root, f.opts.Environment, p); err != nil {
			return nil, err
		}
		req = p.request
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, Fail("transport_error", "source fetch failed")
	}
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		location, err := response.Location()
		response.Body.Close()
		if err != nil {
			return nil, Fail("transport_error", "invalid source redirect")
		}
		// A redirect changes the containing resource. Require the caller to use the final raw URL,
		// rather than resolving references against a misleading import location.
		if location.String() != raw {
			return nil, Fail("transport_error", "source redirected; supply the final raw URL %s", SanitizedDestination(location.String()))
		}
		return nil, Fail("transport_error", "source redirect cycle")
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, Fail("transport_error", "source resource returned HTTP %d", response.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(response.Body, int64(10*1024*1024-f.bytes+1)))
	response.Body.Close()
	if err != nil {
		return nil, Fail("transport_error", "source body read failed")
	}
	f.bytes += len(b)
	if f.bytes > 10*1024*1024 {
		return nil, Fail("invalid_input", "source import exceeds 10 MiB")
	}
	// Prevent echoed credentials from reaching authoritative documents or refresh metadata.
	if c.Sanitize(string(b)) != string(b) {
		return nil, Fail("invalid_input", "source document contains protected credentials")
	}
	return b, nil
}

func DiscoverSource(root, raw string, opts SourceImportOptions) (SourceDiscovery, error) {
	result := SourceDiscovery{Candidates: []SourceCandidate{}, Resources: []string{}}
	if _, err := checkedSourceURL(root, raw, opts); err != nil {
		return result, err
	}
	f := sourceFetcher{root: root, opts: opts}
	seen := map[string]bool{}
	var visit func(string, int) error
	visit = func(location string, depth int) error {
		if seen[location] {
			return nil
		}
		seen[location] = true
		if depth > 8 {
			return Fail("invalid_input", "discovery depth limit exceeded")
		}
		if _, err := checkedSourceURL(root, location, opts); err != nil {
			return err
		}
		result.Resources = append(result.Resources, location)
		b, err := f.fetch(location)
		if err != nil {
			return err
		}
		var doc any
		if Decode(b, &doc) == nil {
			doc, _ = normalizeYAML(doc)
			m := object(doc)
			if m["openapi"] != nil || m["swagger"] != nil {
				result.Candidates = append(result.Candidates, SourceCandidate{Name: location, Location: location})
				return nil
			}
		}
		candidates, resources := staticSourceConfiguration(string(b))
		base, _ := url.Parse(location)
		for _, candidate := range candidates {
			if candidate.Spec == nil {
				u, err := url.Parse(candidate.Location)
				if err != nil {
					continue
				}
				candidate.Location = base.ResolveReference(u).String()
				if _, err := checkedSourceURL(root, candidate.Location, opts); err != nil {
					continue
				}
			} else {
				candidate.Location = location
			}
			if candidate.Name == "" {
				candidate.Name = candidate.Location
			}
			duplicate := false
			for _, previous := range result.Candidates {
				if previous.Location == candidate.Location && previous.Name == candidate.Name {
					duplicate = true
				}
			}
			if !duplicate {
				result.Candidates = append(result.Candidates, candidate)
			}
		}
		var resourceError error
		for _, resource := range resources {
			u, err := url.Parse(resource)
			if err != nil {
				continue
			}
			if err := visit(base.ResolveReference(u).String(), depth+1); err != nil {
				if resourceError == nil {
					resourceError = err
				}
				result.Warnings = append(result.Warnings, err.Error())
			}
		}
		if len(result.Candidates) == 0 {
			return resourceError
		}
		return nil
	}
	err := visit(raw, 0)
	if len(result.Candidates) == 0 {
		result.Hint = "Static discovery found no specification. Supply the raw JSON/YAML URL with --url."
	}
	return result, err
}

func ImportSource(root string, candidate SourceCandidate, preview bool, expectedDigest string, opts SourceImportOptions) (Specification, error) {
	if _, err := checkedSourceURL(root, candidate.Location, opts); err != nil {
		return Specification{}, err
	}
	f := sourceFetcher{root: root, opts: opts}
	r := resolver{documents: map[string][]byte{}, parsed: map[string]any{}, fetch: f.fetch}
	var v any
	var err error
	if candidate.Spec != nil {
		// Re-read the containing resource so a changed embedded declaration cannot apply a stale preview.
		discovery, err := DiscoverSource(root, candidate.Location, opts)
		if err != nil {
			return Specification{}, err
		}
		var current *SourceCandidate
		for i := range discovery.Candidates {
			c := &discovery.Candidates[i]
			if c.Spec != nil && c.Name == candidate.Name && c.Location == candidate.Location {
				current = c
				break
			}
		}
		if current == nil {
			return Specification{}, Fail("revision_conflict", "embedded specification changed; discover again")
		}
		candidate = *current
		if err := checkSourceOrigin(root, candidate.Location, opts); err != nil {
			return Specification{}, err
		}
		b, marshalErr := json.Marshal(candidate.Spec)
		if marshalErr != nil {
			return Specification{}, Fail("invalid_input", "invalid embedded specification")
		}
		r.documents[candidate.Location] = b
		r.parsed[candidate.Location] = candidate.Spec
		v = candidate.Spec
	} else {
		v, err = r.read(candidate.Location)
	}
	if err != nil {
		return Specification{}, err
	}
	s, err := importResolvedSpecification(root, candidate.Location, "url", &r, v, preview, expectedDigest)
	if err != nil {
		var failure *Error
		if errors.As(err, &failure) && (failure.Code == "invalid_input" || failure.Code == "unsupported_format") {
			return Specification{}, Fail(failure.Code, "remote contract is invalid or has missing/cyclic references; check the raw specification")
		}
	}
	return s, err
}

func keysCredentials(items map[string]localCredential) []string {
	names := make([]string, 0, len(items))
	for name := range items {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
