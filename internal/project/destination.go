package project

import (
	"net/url"
	"strings"
)

// DraftDestination resolves where a draft would go without preparing a send:
// it applies variables, path and query values and the module base URL, but
// skips body and contract checks. Path inputs that are still empty stay as
// their {name} placeholder, so the result can be copied before every value
// is known. It returns the full URL and the path on its own.
func DraftDestination(root, env string, d RequestDraft, c *Credentials) (string, string, error) {
	if err := c.Refresh(root); err != nil {
		return "", "", err
	}
	s, err := Load(root)
	if err != nil {
		return "", "", err
	}
	e, err := s.Resolve(env)
	if err != nil {
		return "", "", err
	}
	vars := map[string]string{}
	for _, layer := range []map[string]string{s.Manifest.Variables, e.Variables, d.Variables} {
		for name, value := range layer {
			vars[name] = value
		}
	}
	if d.EndpointID != "" {
		endpoint, err := lookupEndpoint(root, d.EndpointID)
		if err != nil {
			return "", "", err
		}
		d.Module = endpoint.ModuleID
		if d.URL == "" {
			d.URL = endpoint.Path
		}
	}
	raw, err := interpolate(d.URL, vars)
	if err != nil {
		return "", "", c.SanitizeError(err)
	}
	for name, v := range d.Path {
		if v == "" {
			continue
		}
		if v, err = interpolate(v, vars); err != nil {
			return "", "", c.SanitizeError(err)
		}
		raw = strings.ReplaceAll(raw, "{"+name+"}", url.PathEscape(v))
	}
	if strings.Contains(raw, "{{secret:") {
		return "", "", Fail("access_denied", "secrets are forbidden in destination host/path")
	}
	path, rawQuery, _ := strings.Cut(raw, "?")
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", "", Fail("invalid_input", "invalid query encoding")
	}
	for _, f := range d.Query {
		v, err := interpolate(f.Value, vars)
		if err != nil {
			return "", "", c.SanitizeError(err)
		}
		query.Add(f.Name, v)
	}
	// Keep {name} placeholders readable: url.URL would escape the braces.
	full := path
	if !strings.Contains(path, "://") {
		r, err := ReviewAccess(root, env)
		if err != nil {
			return "", "", err
		}
		base := r.BaseURLs[d.Module]
		if base == "" {
			return "", "", Fail("access_denied", "module destination is unconfigured")
		}
		full = strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")
	}
	pathOnly := "/"
	if i := strings.Index(full, "://"); i >= 0 {
		if j := strings.Index(full[i+3:], "/"); j >= 0 {
			pathOnly = full[i+3+j:]
		}
	} else if full != "" {
		pathOnly = full
	}
	if encoded := query.Encode(); encoded != "" {
		full += "?" + encoded
	}
	return c.Sanitize(full), c.Sanitize(pathOnly), nil
}
