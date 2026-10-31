package project

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/zalando/go-keyring"
)

// CredentialStore is the OS integration boundary. Keys are random local identities.
// Adapter errors never become public output, since they can contain secret values.
type CredentialStore interface {
	Set(key, value string) error
	Get(key string) (string, error)
	Delete(key string) error
}
type osCredentialStore struct{}

func (osCredentialStore) Set(k, v string) error        { return keyring.Set("reqscope", k, v) }
func (osCredentialStore) Get(k string) (string, error) { return keyring.Get("reqscope", k) }
func (osCredentialStore) Delete(k string) error {
	err := keyring.Delete("reqscope", k)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

type CredentialBinding struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Module      string `json:"module"`
	Destination string `json:"destination"`
	In          string `json:"in"`
	Field       string `json:"field"`
}
type localCredential struct {
	CredentialBinding
	Destinations []CredentialBinding `json:"destinations,omitempty"`
	Key          string              `json:"key"`
	Session      bool                `json:"session"`
}
type SecretState struct {
	Name       string `json:"name"`
	Configured bool   `json:"configured"`
}

// Credentials keeps session values and a process-lifetime redaction registry.
// There is deliberately no public value getter.
type Credentials struct {
	mu        sync.Mutex
	store     CredentialStore
	session   map[string]string
	known     map[string]bool
	transient map[string]localCredential
}

func NewCredentials(store CredentialStore) *Credentials {
	return &Credentials{store: store, session: map[string]string{}, known: map[string]bool{}}
}

var DefaultCredentials = NewCredentials(osCredentialStore{})
var secretReference = regexp.MustCompile(`\{\{secret:([a-zA-Z0-9_-]+)\}\}`)
var secretName = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func (c *Credentials) remember(v string) {
	if v != "" {
		c.known[v] = true
		b, _ := json.Marshal(v)
		c.known[string(b[1:len(b)-1])] = true
		c.known[url.QueryEscape(v)] = true
		c.known[url.PathEscape(v)] = true
	}
}
func (c *Credentials) value(item localCredential) (string, error) {
	var v string
	var err error
	if item.Session {
		v = c.session[item.Key]
	} else {
		v, err = c.store.Get(item.Key)
	}
	if err != nil || v == "" {
		return "", Fail("access_denied", "credential is missing or OS credential service is unavailable; replace it in the TUI")
	}
	c.rememberValue(item, v)
	return v, nil
}

// rememberValue registers every form a credential value can take in output.
func (c *Credentials) rememberValue(item localCredential, v string) {
	c.remember(v)
	if item.Kind == "basic" {
		c.remember(base64.StdEncoding.EncodeToString([]byte(v)))
		if _, password, ok := strings.Cut(v, ":"); ok {
			c.remember(password)
		}
	}
}

// Save is called only by interactive credential entry, never by the CLI.
func (c *Credentials) Save(root, env string, b CredentialBinding, value string, sessionOnly bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.remember(value)
	return locked(root, func() error {
		s, err := load(root)
		if err != nil {
			return err
		}
		e, err := s.Resolve(env)
		if err != nil {
			return err
		}
		if !secretName.MatchString(b.Name) || len(b.Name) > 128 || value == "" || strings.ContainsAny(value, "\r\n\x00") {
			return Fail("invalid_input", "invalid credential name or value")
		}
		origin, err := Origin(b.Destination)
		if err != nil {
			return Fail("invalid_input", "invalid credential destination")
		}
		review, err := resolvedReview(e.Environment, nil)
		if err != nil {
			return err
		}
		if origin != review.Origins[b.Module] {
			return Fail("access_denied", "credential destination must match configured module origin")
		}
		u, _ := url.Parse(b.Destination)
		if u.RawQuery != "" || u.ForceQuery || strings.ContainsAny(b.Destination, "{}") {
			return Fail("invalid_input", "credential destination must have an exact path and no query")
		}
		if u.Path == "" {
			u.Path = "/"
		}
		if !validDestinationScope(u.EscapedPath()) {
			return Fail("invalid_input", "credential destination path may only end with /* to cover a path prefix")
		}
		b.Destination = u.String()
		switch b.Kind {
		case "bearer", "basic":
			if b.In != "header" || !strings.EqualFold(b.Field, "Authorization") {
				return Fail("invalid_input", "bearer and basic credentials require the Authorization header")
			}
			if b.Kind == "basic" {
				user, password, ok := strings.Cut(value, ":")
				if !ok || user == "" || password == "" {
					return Fail("invalid_input", "basic entry requires username:password")
				}
				c.remember(password)
				c.remember(base64.StdEncoding.EncodeToString([]byte(value)))
			}
		case "api-key":
		default:
			return Fail("invalid_input", "credential kind must be bearer, api-key or basic")
		}
		if b.In != "header" && b.In != "query" && b.In != "body" {
			return Fail("invalid_input", "credential placement must be header, query or body")
		}
		if b.In == "header" {
			if !validHeaderName(b.Field) || strings.EqualFold(b.Field, "Host") || strings.EqualFold(b.Field, "Content-Length") || strings.EqualFold(b.Field, "Content-Type") {
				return Fail("invalid_input", "invalid credential header")
			}
			b.Field = http.CanonicalHeaderKey(b.Field)
		} else if b.In == "query" {
			if !secretName.MatchString(b.Field) {
				return Fail("invalid_input", "invalid credential query field")
			}
		} else if b.Field != "" {
			return Fail("invalid_input", "body placement uses an empty field")
		}
		a, p, err := readAccess(root, s)
		if err != nil {
			return err
		}
		if a.Credentials == nil {
			a.Credentials = map[string]map[string]localCredential{}
		}
		if a.Credentials[e.ID] == nil {
			a.Credentials[e.ID] = map[string]localCredential{}
		}
		old := a.Credentials[e.ID][b.Name]
		item := localCredential{CredentialBinding: b, Key: uuid.NewString(), Session: sessionOnly}
		if sessionOnly {
			c.session[item.Key] = value
		} else if err = c.store.Set(item.Key, value); err != nil {
			return Fail("credential_unavailable", "OS credential storage is unavailable or locked; choose session-only storage explicitly, or unlock the service and retry")
		}
		a.Credentials[e.ID][b.Name] = item
		if a.Bindings[e.ID] == nil {
			a.Bindings[e.ID] = map[string]string{}
		}
		data, _ := json.Marshal(item)
		a.Bindings[e.ID][b.Name] = Digest(data)
		delete(a.Grants, e.ID)
		if err = writeAccess(p, a); err != nil {
			delete(c.session, item.Key)
			if !sessionOnly {
				_ = c.store.Delete(item.Key)
			}
			return err
		}
		if old.Key != "" {
			delete(c.session, old.Key)
			if !old.Session {
				_ = c.store.Delete(old.Key)
			}
		}
		return nil
	})
}
func (c *Credentials) Delete(root, env, name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return locked(root, func() error {
		s, err := load(root)
		if err != nil {
			return err
		}
		e, err := s.Resolve(env)
		if err != nil {
			return err
		}
		a, p, err := readAccess(root, s)
		if err != nil {
			return err
		}
		item, ok := a.Credentials[e.ID][name]
		if !ok {
			return Fail("not_found", "credential not found")
		}
		if !item.Session {
			if err = c.store.Delete(item.Key); err != nil {
				return Fail("access_denied", "OS credential service could not delete credential; unlock it and retry")
			}
		}
		delete(c.session, item.Key)
		delete(a.Credentials[e.ID], name)
		delete(a.Bindings[e.ID], name)
		delete(a.Grants, e.ID)
		return writeAccess(p, a)
	})
}
func credentialItems(root, env string) (map[string]localCredential, error) {
	s, err := Load(root)
	if err != nil {
		return nil, err
	}
	e, err := s.Resolve(env)
	if err != nil {
		return nil, err
	}
	a, _, err := readAccess(root, s)
	if err != nil {
		return nil, err
	}
	return a.Credentials[e.ID], nil
}
func (c *Credentials) Inspect(root, env string) ([]SecretState, error) {
	items, err := c.items(root, env)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	result := []SecretState{}
	for name, item := range items {
		_, err := c.value(item)
		result = append(result, SecretState{name, err == nil})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

// validDestinationScope accepts an exact clean path, or one whose last
// segment is "*" to cover every path beneath that prefix.
func validDestinationScope(path string) bool {
	if strings.Contains(path, "*") && (!strings.HasSuffix(path, "/*") || strings.Count(path, "*") != 1) {
		return false
	}
	return cleanEscapedPath(strings.TrimSuffix(path, "*"))
}

// cleanEscapedPath rejects dot segments, empty segments and encoded
// separators, so a prefix check cannot be escaped by path normalization.
func cleanEscapedPath(path string) bool {
	lower := strings.ToLower(path)
	for _, encoded := range []string{"%2e", "%2f", "%5c", "\\"} {
		if strings.Contains(lower, encoded) {
			return false
		}
	}
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		last := i == len(segments)-1
		if segment == "." || segment == ".." || (segment == "" && i > 0 && !last) {
			return false
		}
	}
	return true
}

// destinationPathPermits reports whether a bound destination path covers a
// request path: exact match, or segment-aligned prefix for a "/*" binding.
func destinationPathPermits(bound, target string) bool {
	if bound == "" {
		bound = "/"
	}
	if target == "" {
		target = "/"
	}
	if prefix, ok := strings.CutSuffix(bound, "*"); ok && strings.HasSuffix(prefix, "/") {
		return cleanEscapedPath(target) && strings.HasPrefix(target, prefix)
	}
	return bound == target
}

type credentialPosition struct {
	Destination string
	Module      string
	In          string
	Field       string
}

func checkSecretPosition(items map[string]localCredential, review AccessReview, position credentialPosition, text string) error {
	target, module, in, field := position.Destination, position.Module, position.In, position.Field
	matches := secretReference.FindAllStringSubmatch(text, -1)
	if strings.Contains(secretReference.ReplaceAllString(text, ""), "{{secret:") {
		return Fail("invalid_input", "invalid secret reference")
	}
	for _, match := range matches {
		item, ok := items[match[1]]
		if !ok {
			return Fail("access_denied", "credential reference is not configured")
		}
		data, _ := json.Marshal(item)
		u, err := url.Parse(target)
		if err != nil {
			return Fail("access_denied", "invalid credential destination")
		}
		u.RawQuery = ""
		u.ForceQuery = false
		if u.Path == "" {
			u.Path = "/"
		}
		origin, err := Origin(u.String())
		if err != nil {
			return err
		}
		bound, _ := Origin(item.Destination)
		bu, _ := url.Parse(item.Destination)
		permitted := item.Module == module && origin == bound && destinationPathPermits(bu.EscapedPath(), u.EscapedPath()) && item.In == in && item.Field == field
		if len(item.Destinations) > 0 {
			permitted = false
			for _, destination := range item.Destinations {
				targetOrigin, _ := Origin(destination.Destination)
				targetURL, _ := url.Parse(destination.Destination)
				if destination.Module == module && targetOrigin == origin && destinationPathPermits(targetURL.EscapedPath(), u.EscapedPath()) && destination.In == in && destination.Field == field && destination.Kind == item.Kind {
					permitted = true
				}
			}
		}
		if review.Bindings[item.Name] != Digest(data) || !permitted {
			return Fail("access_denied", "credential destination or placement is outside its protected binding")
		}
		if in != "body" && text != match[0] {
			return Fail("access_denied", "credential field must contain only its opaque reference")
		}
	}
	return nil
}
func (c *Credentials) validateCredentials(root, env string, review AccessReview, req *http.Request, d RequestDraft) error {
	items, err := c.items(root, env)
	if err != nil {
		return err
	}
	review = c.bindingReview(review)
	// URL references are forbidden even if they appear in an inline query. Use explicit query fields.
	for _, raw := range []string{d.URL, req.URL.Scheme, req.URL.Host, req.URL.Path, d.ContentType, d.Method, d.Module} {
		if strings.Contains(raw, "{{secret:") {
			return Fail("access_denied", "secrets are forbidden in destination and control fields")
		}
	}
	for name, values := range req.Header {
		if strings.Contains(name, "{{secret:") {
			return Fail("access_denied", "secret header names are forbidden")
		}
		for _, v := range values {
			if protectedHeader(name) && !secretReference.MatchString(v) {
				return Fail("access_denied", "authentication headers require protected credential references")
			}
			if err := checkSecretPosition(items, review, credentialPosition{Destination: req.URL.String(), Module: d.Module, In: "header", Field: name}, v); err != nil {
				return err
			}
		}
	}
	for name, values := range req.URL.Query() {
		if strings.Contains(name, "{{secret:") {
			return Fail("access_denied", "secret query names are forbidden")
		}
		for _, v := range values {
			if err := checkSecretPosition(items, review, credentialPosition{Destination: req.URL.String(), Module: d.Module, In: "query", Field: name}, v); err != nil {
				return err
			}
		}
	}
	if d.BodyType == "form" || d.BodyType == "multipart" {
		for _, f := range d.Form {
			if strings.Contains(f.ContentType, "{{secret:") {
				return Fail("access_denied", "secrets are forbidden in form control fields")
			}
			if err := checkSecretPosition(items, review, credentialPosition{Destination: req.URL.String(), Module: d.Module, In: "body"}, f.Value); err != nil {
				return err
			}
		}
		return nil
	}
	if d.BodyFile != "" {
		return nil
	}
	body, _ := ioRequestBody(req)
	return checkSecretPosition(items, review, credentialPosition{Destination: req.URL.String(), Module: d.Module, In: "body"}, body)
}
func protectedHeader(name string) bool {
	switch strings.ToLower(name) {
	case "authorization", "proxy-authorization", "cookie", "set-cookie":
		return true
	}
	return false
}
func (c *Credentials) attach(root, env string, p PreparedRequest) ([]localCredential, error) {
	items, err := c.items(root, env)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	used := []localCredential{}
	expand := func(text string, jsonEscape bool, in, field string) (string, error) {
		var failure error
		result := secretReference.ReplaceAllStringFunc(text, func(ref string) string {
			item := items[secretReference.FindStringSubmatch(ref)[1]]
			v, err := c.value(item)
			if err != nil {
				failure = err
				return ""
			}
			position := item
			position.In = in
			position.Field = field
			used = append(used, position)
			if item.Kind == "bearer" {
				v = "Bearer " + v
			}
			if item.Kind == "basic" {
				v = "Basic " + base64.StdEncoding.EncodeToString([]byte(v))
			}
			c.remember(v)
			if jsonEscape {
				b, _ := json.Marshal(v)
				v = string(b[1 : len(b)-1])
			}
			return v
		})
		return result, failure
	}
	for name, values := range p.request.Header {
		for i, v := range values {
			values[i], err = expand(v, false, "header", name)
			if err != nil {
				return nil, err
			}
		}
		p.request.Header[name] = values
	}
	q := p.request.URL.Query()
	for name, values := range q {
		for i, v := range values {
			values[i], err = expand(v, false, "query", name)
			if err != nil {
				return nil, err
			}
		}
		q[name] = values
	}
	p.request.URL.RawQuery = q.Encode()
	if p.draft.BodyType == "form" || p.draft.BodyType == "multipart" {
		fields := append([]preparedFormField(nil), p.form...)
		for i := range fields {
			if fields[i].File != "" {
				continue
			}
			fields[i].Value, err = expand(fields[i].Value, false, "body", "")
			if err != nil {
				return nil, err
			}
		}
		body, ct, err := encodeForm(fields, p.draft.BodyType, p.request.Header.Get("Content-Type"))
		if err != nil {
			return nil, err
		}
		setRequestBody(p.request, body)
		p.request.Header.Set("Content-Type", ct)
		return used, nil
	}
	if p.draft.BodyFile != "" {
		return used, nil
	}
	body, err := ioRequestBody(p.request)
	if err != nil {
		return nil, err
	}
	body, err = expand(body, p.draft.BodyType == "json" || strings.HasSuffix(strings.Split(p.draft.ContentType, ";")[0], "json"), "body", "")
	if err != nil {
		return nil, err
	}
	setRequestBody(p.request, body)
	return used, nil
}

// Refresh registers persistent values before any adapter output. Unavailable
// configured stores fail closed rather than rendering possibly secret-bearing data.
func (c *Credentials) Refresh(root string) error {
	if root == "" {
		return nil
	}
	s, err := Load(root)
	if err != nil {
		return err
	}
	a, _, err := readAccess(root, s)
	if err != nil {
		return err
	}
	// OS store lookups can take a process launch each, so they run without the
	// registry lock: an adapter refreshing in the background must not stall
	// Sanitize on the thread that draws the screen.
	var stored []localCredential
	c.mu.Lock()
	for _, items := range a.Credentials {
		for _, item := range items {
			if !item.Session {
				stored = append(stored, item)
				continue
			}
			if c.session[item.Key] == "" {
				continue
			}
			if _, err := c.value(item); err != nil {
				c.mu.Unlock()
				return err
			}
		}
	}
	c.mu.Unlock()
	for _, item := range stored {
		v, err := c.store.Get(item.Key)
		if err != nil || v == "" {
			return Fail("access_denied", "credential is missing or OS credential service is unavailable; replace it in the TUI")
		}
		c.mu.Lock()
		c.rememberValue(item, v)
		c.mu.Unlock()
	}
	return nil
}
func (c *Credentials) Sanitize(text string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	values := make([]string, 0, len(c.known))
	for v := range c.known {
		values = append(values, v)
	}
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	for _, v := range values {
		text = strings.ReplaceAll(text, v, "[REDACTED]")
	}
	return text
}

// SanitizeJSON handles JSON escapes and redacts authentication positions even
// for server-generated values which are not in the registered-value set.
func (c *Credentials) SanitizeJSON(raw []byte) []byte {
	var v any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if !json.Valid(raw) || decoder.Decode(&v) != nil {
		return []byte(c.sanitizeTextPositions(string(raw), nil))
	}
	var walk func(any) any
	walk = func(v any) any {
		switch x := v.(type) {
		case string:
			return c.Sanitize(x)
		case []any:
			for i, item := range x {
				x[i] = walk(item)
			}
			return x
		case map[string]any:
			result := map[string]any{}
			field, _ := x["name"].(string)
			for k, item := range x {
				if k == "value" && sensitivePosition(field) {
					if value, ok := item.(string); ok && !secretReference.MatchString(value) {
						item = "[REDACTED]"
					}
				}
				if sensitivePosition(k) {
					item = "[REDACTED]"
				} else {
					item = walk(item)
				}
				result[c.Sanitize(k)] = item
			}
			return result
		}
		return v
	}
	b, _ := json.Marshal(walk(v))
	return b
}
func sensitivePosition(name string) bool {
	switch strings.ToLower(name) {
	case "authorization", "proxy-authorization", "cookie", "set-cookie", "password", "passwd", "token", "access_token", "refresh_token", "api_key", "apikey", "secret", "client_secret":
		return true
	}
	return false
}
func (c *Credentials) sanitizeExchange(result *Exchange, used []localCredential) {
	h := http.Header{}
	for name, values := range result.Headers {
		sensitive := sensitivePosition(name)
		for _, item := range used {
			if item.In == "header" && strings.EqualFold(item.Field, name) {
				sensitive = true
			}
		}
		if sensitive {
			h[c.Sanitize(name)] = []string{"[REDACTED]"}
			continue
		}
		for _, v := range values {
			h.Add(c.Sanitize(name), c.Sanitize(v))
		}
	}
	result.Headers = h
	if !result.Binary {
		media := strings.ToLower(strings.TrimSpace(strings.Split(result.Headers.Get("Content-Type"), ";")[0]))
		trimmed := strings.TrimSpace(result.Body)
		structured := media == "application/json" || strings.HasSuffix(media, "+json") || strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
		if structured && !json.Valid([]byte(result.Body)) {
			result.Body = ""
			result.BodyWithheld = true
		} else if media == "application/xml" || media == "text/xml" || strings.HasSuffix(media, "+xml") || strings.HasPrefix(trimmed, "<") {
			var safe bool
			result.Body, safe = c.sanitizeXML(result.Body, used)
			result.BodyWithheld = !safe
		} else if media == "application/x-www-form-urlencoded" {
			fields, err := url.ParseQuery(result.Body)
			if err != nil {
				result.Body = ""
				result.BodyWithheld = true
			} else {
				for name, values := range fields {
					sensitive := sensitivePosition(name)
					for _, item := range used {
						if strings.EqualFold(name, item.Field) {
							sensitive = true
						}
					}
					if sensitive {
						fields[name] = []string{"[REDACTED]"}
					} else {
						for i, v := range values {
							values[i] = c.Sanitize(v)
						}
						fields[name] = values
					}
				}
				result.Body = fields.Encode()
			}
		} else {
			result.Body = string(c.SanitizeJSON([]byte(result.Body)))
			if !json.Valid([]byte(result.Body)) {
				result.Body = c.sanitizeTextPositions(result.Body, used)
			}
		}
		// Redact bound body/query field names in structured responses as well.
		for _, item := range used {
			if item.Field != "" {
				result.Body = redactResponseField(result.Body, item.Field)
			}
		}
	}
	result.Destination = c.Sanitize(result.Destination)
}
func redactResponseField(body, field string) string {
	var v any
	decoder := json.NewDecoder(strings.NewReader(body))
	decoder.UseNumber()
	if decoder.Decode(&v) != nil {
		return body
	}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, item := range x {
				if strings.EqualFold(k, field) {
					x[k] = "[REDACTED]"
				} else {
					walk(item)
				}
			}
		case []any:
			for _, item := range x {
				walk(item)
			}
		}
	}
	walk(v)
	b, _ := json.Marshal(v)
	return string(b)
}

// SanitizeError preserves domain exit codes without exposing secret-bearing context.
func (c *Credentials) SanitizeError(err error) error {
	if err == nil {
		return nil
	}
	var pe *Error
	if errors.As(err, &pe) {
		return &Error{Code: pe.Code, Message: c.Sanitize(pe.Message)}
	}
	return errors.New(c.Sanitize(err.Error()))
}

// Plain text header-style and form-style assignments also have known positions.
var authenticationAssignment = regexp.MustCompile(`(?im)((?:authorization|proxy-authorization|cookie|set-cookie|password|passwd|token|access_token|refresh_token|api_key|apikey|secret|client_secret)[ \t]*[:=][ \t]*)([^\r\n&]+)`)

func (c *Credentials) sanitizeTextPositions(text string, used []localCredential) string {
	text = c.Sanitize(text)
	text = authenticationAssignment.ReplaceAllString(text, "${1}[REDACTED]")
	for _, item := range used {
		if item.Field != "" {
			position := regexp.MustCompile(`(?im)(` + regexp.QuoteMeta(item.Field) + `[ \t]*[:=][ \t]*)([^\r\n&]+)`)
			text = position.ReplaceAllString(text, "${1}[REDACTED]")
		}
	}
	return text
}
