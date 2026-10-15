package project

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

type RequestField struct {
	Name  string `json:"name" yaml:"name"`
	Value string `json:"value" yaml:"value"`
}
type FormField struct {
	Name        string `json:"name" yaml:"name"`
	Value       string `json:"value,omitempty" yaml:"value,omitempty"`
	File        string `json:"file,omitempty" yaml:"file,omitempty"`
	ContentType string `json:"contentType,omitempty" yaml:"contentType,omitempty"`
}

type RequestDraft struct {
	Form         []FormField       `json:"form,omitempty" yaml:"form,omitempty"`
	BodyFile     string            `json:"bodyFile,omitempty" yaml:"bodyFile,omitempty"`
	Variables    map[string]string `json:"variables,omitempty" yaml:"variables,omitempty"`
	EndpointID   string            `json:"endpointId,omitempty" yaml:"endpointId,omitempty"`
	Module       string            `json:"module" yaml:"module"`
	Method       string            `json:"method" yaml:"method"`
	URL          string            `json:"url" yaml:"url"`
	Path         map[string]string `json:"path,omitempty" yaml:"path,omitempty"`
	Query        []RequestField    `json:"query,omitempty" yaml:"query,omitempty"`
	Headers      []RequestField    `json:"headers,omitempty" yaml:"headers,omitempty"`
	Body         string            `json:"body" yaml:"body"`
	BodyType     string            `json:"bodyType" yaml:"bodyType"`
	ContentType  string            `json:"contentType,omitempty" yaml:"contentType,omitempty"`
	TimeoutMS    int64             `json:"timeoutMs,omitempty" yaml:"timeoutMs,omitempty"`
	MaxRedirects *int              `json:"maxRedirects,omitempty" yaml:"maxRedirects,omitempty"`
	MaxBodyBytes int64             `json:"maxBodyBytes,omitempty" yaml:"maxBodyBytes,omitempty"`
}

// SendOptions is supplied by adapters, never decoded from request input.
type SendOptions struct {
	confirmRedirect func(context.Context, PreparedRequest) (string, error)
	captureResponse func(*Exchange) error
	Validation      map[string]any
	Variables       map[string]string
	Credentials     *Credentials
	Interactive     bool
	Confirmation    string
}
type PreparedRequest struct {
	Method         string `json:"method"`
	URL            string `json:"url"`
	Classification string `json:"classification"`
	Fingerprint    string `json:"fingerprint"`
	request        *http.Request
	review         AccessReview
	draft          RequestDraft
	form           []preparedFormField
}
type Exchange struct {
	Transport     string        `json:"transport"`
	Checks        []CheckResult `json:"checks"`
	CaseID        string        `json:"caseId,omitempty"`
	Draft         bool          `json:"draft"`
	Method        string        `json:"method"`
	Destination   string        `json:"destination"`
	Status        int           `json:"status"`
	DurationMS    int64         `json:"durationMs"`
	Headers       http.Header   `json:"headers"`
	Body          string        `json:"body"`
	RetainedBytes int           `json:"retainedBytes"`
	Binary        bool          `json:"binary"`
	BodyWithheld  bool          `json:"bodyWithheld,omitempty"`
	Truncated     bool          `json:"truncated"`
}

func interpolate(raw string, vars map[string]string) (string, error) {
	return expandVariables(raw, vars, map[string]bool{}, 0)
}
func expandVariables(raw string, vars map[string]string, active map[string]bool, depth int) (string, error) {
	if depth > 32 {
		return "", Fail("invalid_input", "variable nesting limit exceeded")
	}
	var result strings.Builder
	for {
		start := strings.Index(raw, "{{")
		if start < 0 {
			if strings.Contains(raw, "}}") {
				return "", Fail("invalid_input", "invalid variable syntax")
			}
			result.WriteString(raw)
			return result.String(), nil
		}
		if strings.Contains(raw[:start], "}}") {
			return "", Fail("invalid_input", "invalid variable syntax")
		}
		result.WriteString(raw[:start])
		end := strings.Index(raw[start+2:], "}}")
		if end < 0 {
			return "", Fail("invalid_input", "invalid variable syntax")
		}
		end += start + 2
		name := strings.TrimSpace(raw[start+2 : end])
		if strings.HasPrefix(name, "secret:") {
			result.WriteString(raw[start : end+2])
			raw = raw[end+2:]
			continue
		}
		v, ok := vars[name]
		if !ok {
			return "", Fail("invalid_input", "undefined variable %q", name)
		}
		if active[name] {
			return "", Fail("invalid_input", "cyclic variables")
		}
		active[name] = true
		expanded, err := expandVariables(v, vars, active, depth+1)
		delete(active, name)
		if err != nil {
			return "", err
		}
		result.WriteString(expanded)
		if result.Len() > 10*1024*1024 {
			return "", Fail("invalid_input", "expanded input exceeds 10 MiB")
		}
		raw = raw[end+2:]
	}
}
func requestAccess(root, env, module, target string, interactive bool) (AccessReview, error) {
	if !interactive {
		return CheckCLIAccess(root, env, module, target)
	}
	r, err := ReviewAccess(root, env)
	if err != nil {
		return r, err
	}
	origin, err := Origin(target)
	if err != nil {
		return r, err
	}
	if r.Origins[module] == "" || origin != r.Origins[module] {
		return r, Fail("access_denied", "destination is outside the configured environment origin")
	}
	return r, nil
}
func PrepareRequest(root, env string, d RequestDraft, interactive bool) (PreparedRequest, error) {
	return PrepareRequestWithCredentials(root, env, d, interactive, DefaultCredentials)
}
func PrepareRequestWithCredentials(root, env string, d RequestDraft, interactive bool, c *Credentials) (PreparedRequest, error) {
	if err := c.Refresh(root); err != nil {
		return PreparedRequest{}, err
	}
	p, err := prepareRequestSnapshot(root, env, d, interactive, nil, false, c)
	p.URL = c.Sanitize(p.URL)
	return p, c.SanitizeError(err)
}
func prepareRequest(root, env string, d RequestDraft, interactive bool) (PreparedRequest, error) {
	return prepareRequestSnapshot(root, env, d, interactive, nil, false)
}
func prepareRequestSnapshot(root, env string, d RequestDraft, interactive bool, endpoint *Endpoint, snapshot bool, credentials ...*Credentials) (PreparedRequest, error) {
	var p PreparedRequest
	if !interactive && strings.TrimSpace(env) == "" {
		return p, Fail("access_denied", "every CLI network run requires explicit --env")
	}
	s, err := Load(root)
	if err != nil {
		return p, err
	}
	e, err := s.Resolve(env)
	if err != nil {
		return p, err
	}
	vars := map[string]string{}
	for name, value := range s.Manifest.Variables {
		vars[name] = value
	}
	for name, value := range e.Variables {
		vars[name] = value
	}
	for name, value := range d.Variables {
		vars[name] = value
	}
	if d.EndpointID != "" {
		if !snapshot {
			specs, err := Specifications(root)
			if err != nil {
				return p, err
			}
			for _, spec := range specs {
				for _, ep := range spec.Endpoints {
					if ep.ID == d.EndpointID {
						copy := ep
						endpoint = &copy
					}
				}
			}
		}
		if endpoint == nil {
			return p, Fail("not_found", "endpoint not found")
		}
		d.Module = endpoint.ModuleID
		d.Method = endpoint.Method
		if d.URL == "" {
			d.URL = endpoint.Path
		}
	}
	r, err := ReviewAccess(root, env)
	if err != nil {
		return p, err
	}
	raw, err := interpolate(d.URL, vars)
	if err != nil {
		return p, err
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return p, Fail("invalid_input", "invalid destination")
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return p, Fail("invalid_input", "invalid query encoding")
	}

	headers := http.Header{}
	for _, f := range d.Query {
		v, err := interpolate(f.Value, vars)
		if err != nil {
			return p, err
		}
		query.Add(f.Name, v)
	}
	for _, f := range d.Headers {
		v, err := interpolate(f.Value, vars)
		if err != nil {
			return p, err
		}

		headers.Add(f.Name, v)
	}
	paths := map[string]string{}
	for name, v := range d.Path {
		v, err = interpolate(v, vars)
		if err != nil {
			return p, err
		}
		paths[name] = v
	}
	if endpoint != nil {
		if err = serializeContract(*endpoint, paths, query, headers); err != nil {
			return p, err
		}
	}
	for name, v := range paths {
		raw = strings.ReplaceAll(raw, "{"+name+"}", url.PathEscape(v))
	}
	if strings.Contains(raw, "{{secret:") {
		return p, Fail("access_denied", "secrets are forbidden in destination host/path")
	}
	if strings.ContainsAny(raw, "{}") {
		return p, Fail("invalid_input", "missing path input")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return p, Fail("invalid_input", "invalid destination")
	}
	if !u.IsAbs() {
		base := r.BaseURLs[d.Module]
		if base == "" {
			return p, Fail("access_denied", "module destination is unconfigured")
		}
		u, err = url.Parse(strings.TrimRight(base, "/") + "/" + strings.TrimLeft(raw, "/"))
		if err != nil {
			return p, Fail("invalid_input", "invalid destination")
		}
	}
	u.RawQuery = query.Encode()
	r, err = requestAccess(root, env, d.Module, u.String(), interactive)
	if err != nil {
		return p, err
	}
	body, err := interpolate(d.Body, vars)
	if err != nil {
		return p, err
	}
	if !SupportedBodyType(d.BodyType) {
		return p, Fail("invalid_input", "unsupported body type")
	}
	if d.BodyType == "json" && body != "" {
		if !json.Valid([]byte(body)) {
			return p, Fail("invalid_input", "invalid JSON body")
		}
		if d.ContentType == "" {
			d.ContentType = "application/json"
		}
	}
	form, err := prepareForm(root, d, vars, endpoint)
	if err != nil {
		return p, err
	}
	if d.BodyFile != "" {
		if d.BodyType != "raw" || body != "" || len(d.Form) > 0 {
			return p, Fail("invalid_input", "bodyFile requires an exclusive raw body")
		}
		b, err := readUpload(root, d.BodyFile)
		if err != nil {
			return p, err
		}
		body = string(b)
	}
	if d.BodyType == "form" || d.BodyType == "multipart" {
		if d.ContentType == "" {
			d.ContentType = headers.Get("Content-Type")
		}
		if body != "" || d.BodyFile != "" {
			return p, Fail("invalid_input", "form bodies cannot include raw body input")
		}
		body, d.ContentType, err = encodeForm(form, d.BodyType, d.ContentType)
		if err != nil {
			return p, err
		}
	}
	if endpoint != nil {
		media := strings.TrimSpace(strings.Split(d.ContentType, ";")[0])
		if (media == "application/x-www-form-urlencoded" || media == "multipart/form-data") && d.BodyType != "form" && d.BodyType != "multipart" {
			return p, Fail("invalid_input", "contract forms require structured form input")
		}
		if err = validateContractBody(*endpoint, body, d.ContentType); err != nil {
			return p, err
		}
	}
	if d.TimeoutMS < 0 || d.TimeoutMS > 86400000 || d.MaxBodyBytes < 0 || d.MaxBodyBytes > 1<<30 || d.MaxRedirects != nil && *d.MaxRedirects < 0 {
		return p, Fail("invalid_input", "invalid request limits")
	}
	method := strings.ToUpper(d.Method)
	switch method {
	case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "TRACE":
	default:
		return p, Fail("invalid_input", "unsupported HTTP method")
	}
	req, err := http.NewRequest(method, u.String(), strings.NewReader(body))
	if err != nil {
		return p, Fail("invalid_input", "invalid request")
	}
	req.Header = headers
	if d.ContentType != "" {
		req.Header.Set("Content-Type", d.ContentType)
	}
	// Validate headers before opening a connection, including repeated fields.
	for name, values := range req.Header {
		if !validHeaderName(name) {
			return p, Fail("invalid_input", "invalid header name")
		}
		for _, v := range values {
			for i := 0; i < len(v); i++ {
				if v[i] < 32 && v[i] != '\t' || v[i] == 127 {
					return p, Fail("invalid_input", "invalid header value")
				}
			}
		}
	}
	credentialDraft := d
	if d.BodyType == "form" || d.BodyType == "multipart" {
		credentialDraft.Form = make([]FormField, len(form))
		for i, f := range form {
			credentialDraft.Form[i] = f.FormField
		}
	}
	c := DefaultCredentials
	if len(credentials) > 0 {
		c = credentials[0]
	}
	if err := c.validateCredentials(root, env, r, req, credentialDraft); err != nil {
		return p, err
	}
	data, _ := json.Marshal(struct {
		Draft            RequestDraft
		URL, Body, Grant string
		Header           http.Header
	}{d, u.String(), body, r.Fingerprint, headers})
	p = PreparedRequest{Method: method, URL: SanitizedDestination(u.String()), Classification: r.Classification, Fingerprint: Digest(data), request: req, review: r, draft: d, form: form}
	return p, nil
}
func SanitizedDestination(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "invalid destination"
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}
func Send(ctx context.Context, root, env string, d RequestDraft, opts SendOptions) (result Exchange, err error) {
	return send(ctx, root, env, d, opts, nil, false)
}
func send(ctx context.Context, root, env string, d RequestDraft, opts SendOptions, endpoint *Endpoint, snapshot bool) (result Exchange, err error) {
	v, err := decodeValidation(opts.Validation)
	if err != nil {
		return result, err
	}
	if !snapshot && v.ResponseSchema && d.EndpointID != "" {
		specs, e := Specifications(root)
		if e != nil {
			return result, e
		}
		for _, spec := range specs {
			for _, ep := range spec.Endpoints {
				if ep.ID == d.EndpointID {
					copy := ep
					endpoint = &copy
				}
			}
		}
	}
	result.Transport = "not-started"
	d = CloneDraft(d)
	if d.Variables == nil {
		d.Variables = map[string]string{}
	}
	for name, value := range opts.Variables {
		d.Variables[name] = value
	}
	c := opts.Credentials
	if c == nil {
		c = DefaultCredentials
	}
	defer func() { err = c.SanitizeError(err) }()
	var p PreparedRequest
	if snapshot {
		if err = c.Refresh(root); err == nil {
			p, err = prepareRequestSnapshot(root, env, d, opts.Interactive, endpoint, true, c)
			p.URL = c.Sanitize(p.URL)
		}
	} else {
		p, err = PrepareRequestWithCredentials(root, env, d, opts.Interactive, c)
	}
	if err != nil {
		return Exchange{}, err
	}
	if opts.Interactive && p.Classification == "production" && opts.Confirmation != p.Fingerprint {
		return Exchange{}, Fail("access_denied", "production send requires confirmation of the current request")
	}
	used, err := c.attach(root, env, p)
	if err != nil {
		return Exchange{}, err
	}
	timeout := 30 * time.Second
	if d.TimeoutMS != 0 {
		timeout = time.Duration(d.TimeoutMS) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	redirects := 5
	if d.MaxRedirects != nil {
		redirects = *d.MaxRedirects
	}
	capBytes := int64(10 * 1024 * 1024)
	if d.MaxBodyBytes != 0 {
		capBytes = d.MaxBodyBytes
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DisableKeepAlives = true
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > redirects {
			return Fail("transport_error", "redirect limit exceeded")
		}
		from, _ := Origin(via[0].URL.String())
		to, _ := Origin(req.URL.String())
		if from != to {
			for name := range req.Header {
				sensitive := protectedHeader(name)
				for _, item := range used {
					if item.In == "header" && strings.EqualFold(name, item.Field) {
						sensitive = true
					}
				}
				if sensitive {
					req.Header.Del(name)
				}
			}
		}
		current, err := requestAccess(root, env, p.draft.Module, req.URL.String(), opts.Interactive)
		if err != nil {
			return err
		}
		if current.Fingerprint != p.review.Fingerprint {
			return Fail("access_denied", "environment changed during execution")
		}
		if opts.Interactive && current.Classification == "production" {
			if opts.confirmRedirect == nil {
				return Fail("access_denied", "production redirects require individual confirmation")
			}
			data, _ := json.Marshal(struct {
				Method, URL, Access, Initial string
				Headers                      http.Header
			}{req.Method, req.URL.String(), current.Fingerprint, p.Fingerprint, req.Header})
			redirect := PreparedRequest{Method: req.Method, URL: c.Sanitize(SanitizedDestination(req.URL.String())), Classification: current.Classification, Fingerprint: Digest(data)}
			confirmation, err := opts.confirmRedirect(ctx, redirect)
			if err != nil {
				return err
			}
			if confirmation != redirect.Fingerprint {
				return Fail("access_denied", "production redirect requires confirmation of the current request")
			}
			// Authorization may have changed while waiting for the user's response.
			current, err = requestAccess(root, env, p.draft.Module, req.URL.String(), opts.Interactive)
			if err != nil {
				return err
			}
			if current.Fingerprint != p.review.Fingerprint {
				return Fail("access_denied", "environment changed during redirect confirmation")
			}
		}
		items, err := c.items(root, env)
		if err != nil {
			return err
		}
		for _, item := range used {
			if err := checkSecretPosition(items, c.bindingReview(current), credentialPosition{Destination: req.URL.String(), Module: p.draft.Module, In: item.In, Field: item.Field}, "{{secret:"+item.Name+"}}"); err != nil {
				return err
			}
		}
		return nil
	}}
	fresh, err := requestAccess(root, env, p.draft.Module, p.request.URL.String(), opts.Interactive)
	if err != nil {
		return Exchange{}, err
	}
	if fresh.Fingerprint != p.review.Fingerprint {
		return Exchange{}, Fail("access_denied", "environment or binding changed during execution")
	}
	start := time.Now()
	response, err := client.Do(p.request.WithContext(ctx))
	if err != nil {
		return Exchange{Transport: "failed"}, transportError(ctx, err)
	}
	defer response.Body.Close()
	b, err := io.ReadAll(io.LimitReader(response.Body, capBytes))
	if err != nil {
		return Exchange{Transport: "failed"}, transportError(ctx, err)
	}
	result = Exchange{Transport: "completed", Draft: true, Method: p.Method, Destination: p.URL, Status: response.StatusCode, DurationMS: time.Since(start).Milliseconds(), Headers: response.Header, Truncated: int64(len(b)) == capBytes && (response.ContentLength < 0 || response.ContentLength > capBytes)}
	if int64(len(b)) == capBytes {
		cancel()
	}
	result.RetainedBytes = len(b)
	result.Binary = !utf8.Valid(b) || strings.ContainsRune(string(b), 0) || binaryContentType(response.Header.Get("Content-Type"))
	if !result.Binary {
		result.Body = string(b)
	}
	var captureErr error
	if protectErr := c.protectResponse(root, env, &result); protectErr != nil {
		result.Body = ""
		result.Headers = http.Header{}
		result.BodyWithheld = true
		return result, protectErr
	}
	if opts.captureResponse != nil {
		captureErr = opts.captureResponse(&result)
	}
	result.Checks = responseChecks(result, v, endpoint)
	c.sanitizeExchange(&result, used)
	if captureErr != nil {
		return result, captureErr
	}
	for _, check := range result.Checks {
		if check.Outcome != "passed" {
			return result, Fail("check_failed", "Response checks failed or are incomplete")
		}
	}
	return result, nil
}
func transportError(ctx context.Context, err error) error {
	var pe *Error
	if errors.As(err, &pe) {
		return pe
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return Fail("cancelled", "request cancelled")
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return Fail("transport_error", "request timed out")
	}
	return Fail("transport_error", "HTTP transport failed")
}

func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", rune(c)) {
			continue
		}
		return false
	}
	return true
}
func binaryContentType(contentType string) bool {
	media := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	return media != "" && !strings.HasPrefix(media, "text/") && media != "application/json" && !strings.HasSuffix(media, "+json") && media != "application/xml" && !strings.HasSuffix(media, "+xml") && media != "application/javascript" && media != "application/x-www-form-urlencoded"
}

func ioRequestBody(req *http.Request) (string, error) {
	if req.GetBody == nil {
		return "", nil
	}
	body, err := req.GetBody()
	if err != nil {
		return "", err
	}
	defer body.Close()
	b, err := io.ReadAll(body)
	return string(b), err
}
func setRequestBody(req *http.Request, body string) {
	req.Body = io.NopCloser(strings.NewReader(body))
	req.ContentLength = int64(len(body))
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(body)), nil }
}
