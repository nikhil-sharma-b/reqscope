package project

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// IgnoreRules apply only to comparisons. A pointer suppresses the value at that
// exact path, including its children; header names are case-insensitive.
type IgnoreRules struct {
	JSONPointers []string `json:"jsonPointers,omitempty" yaml:"jsonPointers,omitempty"`
	Headers      []string `json:"headers,omitempty" yaml:"headers,omitempty"`
}

func (r IgnoreRules) Validate() error {
	if len(r.JSONPointers)+len(r.Headers) > 100 {
		return Fail("invalid_input", "at most 100 comparison ignore rules are supported")
	}
	for _, p := range r.JSONPointers {
		if !validPointer(p) || len(p) > 1024 {
			return Fail("invalid_input", "invalid comparison JSON Pointer")
		}
	}
	for _, h := range r.Headers {
		if !validHeaderName(h) || len(h) > 128 {
			return Fail("invalid_input", "invalid comparison header name")
		}
	}
	return nil
}

type RunIdentity struct {
	ID             string `json:"id"`
	Environment    string `json:"environment"`
	ContractDigest string `json:"contractDigest"`
	InputDigest    string `json:"inputDigest"`
	RetainedBytes  int    `json:"retainedBytes"`
	Binary         bool   `json:"binary"`
	Truncated      bool   `json:"truncated"`
	BodyWithheld   bool   `json:"bodyWithheld"`
}
type Difference struct {
	Area   string `json:"area"`
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}
type Comparison struct {
	CaseID      string       `json:"caseId"`
	Before      RunIdentity  `json:"before"`
	After       RunIdentity  `json:"after"`
	BodyFormat  string       `json:"bodyFormat"`
	Ignore      IgnoreRules  `json:"ignore"`
	Differences []Difference `json:"differences"`
	Limitations []string     `json:"limitations"`
}

func identity(r Run) RunIdentity {
	return RunIdentity{r.ID, comparisonClip(r.Environment, 512), r.ContractDigest, r.InputDigest, r.Response.RetainedBytes, r.Response.Binary, r.Response.Truncated, r.Response.BodyWithheld}
}

// CompareRuns reads both records under the history lock. Nil rules selects the
// case's saved rules; supplied rules replace them for this comparison only.
func CompareRuns(root, beforeID, afterID string, rules *IgnoreRules) (result Comparison, err error) {
	result.Differences = []Difference{}
	result.Limitations = []string{}
	err = historyDB(root, func(db *sql.DB) error {
		read := func(id string) (Run, error) {
			r, e := scanRun(db.QueryRow("SELECT id,case_id,pinned,evidence FROM runs WHERE id=?", id))
			if e == sql.ErrNoRows {
				return r, Fail("not_found", "comparison run is missing or evicted")
			}
			return r, e
		}
		a, e := read(beforeID)
		if e != nil {
			return e
		}
		b, e := read(afterID)
		if e != nil {
			return e
		}
		if a.CaseID == "" || a.CaseID != b.CaseID {
			return Fail("invalid_input", "comparison requires two runs of the same saved case")
		}
		if rules == nil {
			items, e := cases(root)
			if e != nil {
				return e
			}
			for _, c := range items {
				if c.ID == a.CaseID {
					result.Ignore = c.ComparisonIgnore
					break
				}
			}
		} else {
			result.Ignore = *rules
		}
		if e := result.Ignore.Validate(); e != nil {
			return e
		}
		result.CaseID = a.CaseID
		result.Before = identity(a)
		result.After = identity(b)
		result.compare(a, b)
		return nil
	})
	return
}
func comparisonClip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "�") + "…"
}
func (c *Comparison) limitation(s string) {
	for _, v := range c.Limitations {
		if s == v {
			return
		}
	}
	c.Limitations = append(c.Limitations, s)
}
func (c *Comparison) add(area, path, kind string, a, b any) {
	if len(c.Differences) >= 100 {
		c.limitation("Output limited to 100 differences")
		return
	}
	bound := func(v any) any {
		raw, _ := json.Marshal(v)
		if len(raw) > 1024 {
			c.limitation("Difference values limited to 1024 bytes; previews may be incomplete")
			return comparisonClip(string(raw), 1024)
		}
		return v
	}
	if len(path) > 1024 {
		c.limitation("Difference paths limited to 1024 bytes")
	}
	c.Differences = append(c.Differences, Difference{area, comparisonClip(path, 1024), kind, bound(a), bound(b)})
}
func (c *Comparison) compare(a, b Run) {
	for _, v := range []struct{ path, a, b string }{{"environment", a.Environment, b.Environment}, {"contractDigest", a.ContractDigest, b.ContractDigest}, {"inputDigest", a.InputDigest, b.InputDigest}} {
		if v.a != v.b {
			c.add("context", v.path, "changed", v.a, v.b)
		}
	}
	if a.Response.Status != b.Response.Status {
		c.add("status", "", "changed", a.Response.Status, b.Response.Status)
	}
	headers := func(h http.Header) map[string]any {
		out := map[string]any{}
		keys := []string{}
		for k := range h {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			key := strings.ToLower(k)
			old, _ := out[key].([]string)
			out[key] = append(old, h[k]...)
		}
		for _, k := range c.Ignore.Headers {
			delete(out, strings.ToLower(k))
		}
		return out
	}
	ah, bh := headers(a.Response.Headers), headers(b.Response.Headers)
	for _, k := range comparisonKeys(ah, bh) {
		x, xok := ah[k]
		y, yok := bh[k]
		if !reflect.DeepEqual(x, y) {
			c.add("header", k, differenceKind(xok, yok), x, y)
		}
	}
	for _, v := range []struct {
		label string
		x     Exchange
	}{{"Before", a.Response}, {"After", b.Response}} {
		if v.x.Status == 0 {
			c.limitation(v.label + " response unavailable")
		}
		if v.x.Binary {
			c.limitation(v.label + " body is binary; only retained metadata can be compared")
		}
		if v.x.BodyWithheld {
			c.limitation(v.label + " body was withheld")
		}
		if v.x.Truncated {
			c.limitation(v.label + " body was truncated")
		}
		raw, _ := json.Marshal(v.x.Headers)
		if strings.Contains(v.x.Body, "[REDACTED]") || strings.Contains(string(raw), "[REDACTED]") {
			c.limitation(v.label + " evidence is redacted; hidden values cannot be compared")
		}
	}
	x, y := a.Response, b.Response
	if x.Binary || y.Binary || x.BodyWithheld || y.BodyWithheld || x.Status == 0 || y.Status == 0 {
		c.BodyFormat = "unavailable"
		for _, v := range []struct {
			p    string
			a, b any
		}{{"retainedBytes", x.RetainedBytes, y.RetainedBytes}, {"binary", x.Binary, y.Binary}, {"truncated", x.Truncated, y.Truncated}, {"bodyWithheld", x.BodyWithheld, y.BodyWithheld}} {
			if !reflect.DeepEqual(v.a, v.b) {
				c.add("body-metadata", v.p, "changed", v.a, v.b)
			}
		}
		return
	}
	av, ae := jsonValue(x.Body)
	bv, be := jsonValue(y.Body)
	if ae == nil && be == nil {
		c.BodyFormat = "json"
		c.jsonDiff("", av, true, bv, true, 0)
		return
	}
	c.BodyFormat = "text"
	// A linear line diff keeps the common prefix and suffix as context. The
	// changed middle is a replacement hunk, avoiding quadratic LCS memory.
	if x.Body == y.Body {
		return
	}
	al, bl := strings.Split(x.Body, "\n"), strings.Split(y.Body, "\n")
	start := 0
	for start < len(al) && start < len(bl) && al[start] == bl[start] {
		start++
	}
	endA, endB := len(al), len(bl)
	for endA > start && endB > start && al[endA-1] == bl[endB-1] {
		endA--
		endB--
	}
	remaining := 100 - len(c.Differences)
	for i := start; i < endA && len(c.Differences) < 100; i++ {
		c.add("text", "/"+strconv.Itoa(i+1), "removed", al[i], nil)
	}
	for i := start; i < endB && len(c.Differences) < 100; i++ {
		c.add("text", "/"+strconv.Itoa(i+1), "added", nil, bl[i])
	}
	if endA+endB-2*start > remaining {
		c.limitation("Output limited to 100 differences")
	}
}
func differenceKind(a, b bool) string {
	if !a {
		return "added"
	}
	if !b {
		return "removed"
	}
	return "changed"
}
func comparisonKeys(a, b map[string]any) []string {
	keys := map[string]bool{}
	for k := range a {
		keys[k] = true
	}
	for k := range b {
		keys[k] = true
	}
	out := []string{}
	for k := range keys {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func (c *Comparison) jsonDiff(path string, a any, aok bool, b any, bok bool, depth int) {
	for _, p := range c.Ignore.JSONPointers {
		if p == path {
			return
		}
	}
	if len(c.Differences) >= 100 {
		c.limitation("Output limited to 100 differences")
		return
	}
	if depth > 64 {
		c.limitation("JSON comparison depth limited to 64")
		return
	}
	am, amok := a.(map[string]any)
	bm, bmok := b.(map[string]any)
	if (amok || !aok) && (bmok || !bok) && (len(am) > 0 || len(bm) > 0) {
		for _, k := range comparisonKeys(am, bm) {
			av, ao := am[k]
			bv, bo := bm[k]
			p := strings.ReplaceAll(strings.ReplaceAll(k, "~", "~0"), "/", "~1")
			c.jsonDiff(path+"/"+p, av, ao, bv, bo, depth+1)
		}
		return
	}
	aa, aaok := a.([]any)
	ba, baok := b.([]any)
	if (aaok || !aok) && (baok || !bok) && (len(aa) > 0 || len(ba) > 0) {
		for i := 0; i < max(len(aa), len(ba)); i++ {
			var av, bv any
			ao, bo := i < len(aa), i < len(ba)
			if ao {
				av = aa[i]
			}
			if bo {
				bv = ba[i]
			}
			c.jsonDiff(path+"/"+strconv.Itoa(i), av, ao, bv, bo, depth+1)
		}
		return
	}
	if aok != bok || !typedEqual(a, b) {
		c.add("json", path, differenceKind(aok, bok), a, b)
	}
}

func ComparisonIgnoreSchema() map[string]any {
	array := func() map[string]any {
		return map[string]any{"type": "array", "maxItems": 100, "items": map[string]any{"type": "string"}}
	}
	return map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"jsonPointers": array(), "headers": array()}}
}
