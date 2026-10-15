package project

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/google/uuid"
	"go.yaml.in/yaml/v3"
)

// RefreshPreview describes a three-way merge without writing project resources.
type RefreshPreview struct {
	SourceID       string         `json:"sourceId"`
	Token          string         `json:"previewToken"`
	CurrentDigest  string         `json:"currentDigest"`
	IncomingDigest string         `json:"incomingDigest"`
	Added          []Endpoint     `json:"added"`
	Changed        []Endpoint     `json:"changed"`
	Removed        []Endpoint     `json:"removed"`
	Cases          []CaseResource `json:"affectedCases"`
	Conflicts      []string       `json:"conflicts"`
	Diagnostics    []string       `json:"diagnostics"`
}

// RefreshSpecification re-fetches the source on both preview and apply. The token
// binds the incoming bytes and every resource used to compute the impact.
func RefreshSpecification(root, sourceID, token string, preview bool, opts SourceImportOptions) (result RefreshPreview, err error) {
	specs, err := Specifications(root)
	if err != nil {
		return result, err
	}
	var source Specification
	for _, s := range specs {
		if s.ID == sourceID {
			source = s
			break
		}
	}
	if source.ID == "" {
		return result, Fail("not_found", "source not found")
	}
	var incoming Specification
	if source.Mode == "url" {
		candidate := SourceCandidate{Location: source.Location, Name: source.EmbeddedName}
		if source.EmbeddedName != "" {
			candidate.Spec = map[string]any{}
		}
		incoming, err = ImportSource(root, candidate, true, "", opts)
	} else {
		incoming, err = ImportSpecification(root, source.Location, true)
	}
	if err != nil {
		return result, err
	}
	c := contractCredentials([]*Credentials{opts.Credentials})
	if err = c.Refresh(root); err != nil {
		return result, err
	}
	err = locked(root, func() error {
		current, e := specifications(root)
		if e != nil {
			return e
		}
		found := false
		for _, s := range current {
			if s.ID != sourceID {
				continue
			}
			found = true
			if s.Revision != source.Revision {
				return Fail("revision_conflict", "source changed while fetching; preview again")
			}
		}
		if !found {
			return Fail("revision_conflict", "source removed while fetching; inspect sources again")
		}
		snapshot, e := load(root)
		if e != nil {
			return e
		}
		items, e := cases(root)
		if e != nil {
			return e
		}
		flows, e := scenarios(root)
		if e != nil {
			return e
		}
		result = RefreshPreview{SourceID: sourceID, CurrentDigest: source.Revision, IncomingDigest: incoming.Digest, Added: []Endpoint{}, Changed: []Endpoint{}, Removed: []Endpoint{}, Cases: []CaseResource{}, Conflicts: []string{}, Diagnostics: []string{}}
		state, e := json.Marshal(struct {
			Snapshot Snapshot
			Specs    []Specification
			Cases    []CaseResource
			Flows    []ScenarioResource
			Incoming string
		}{snapshot, current, items, flows, incoming.Digest})
		if e != nil {
			return e
		}
		result.Token = Digest(state)
		documents, e := mergeRefreshDocuments(source, incoming, &result.Conflicts)
		if e != nil {
			return e
		}
		mergedDocuments := documents
		if len(result.Conflicts) > 0 {
			mergedDocuments = incoming.Documents
		}

		candidate, e := resolveRefreshDocuments(root, source, mergedDocuments)
		if e != nil {
			return e
		}
		for _, raw := range appendDocumentBytes(documents, incoming.Documents) {
			var parsed any
			if err := Decode(raw, &parsed); err != nil {
				return err
			}
			if c.Sanitize(string(raw)) != string(raw) || contractHasCredential(parsed, c) {
				return Fail("access_denied", "contract contains protected credentials")
			}
		}
		next := map[string]Endpoint{}
		old := map[string]Endpoint{}
		for _, ep := range source.Endpoints {
			old[ep.ID] = ep
		}
		protectionChanged := false
		for _, ep := range candidate.Endpoints {
			next[ep.ID] = ep
			before, exists := old[ep.ID]
			if !exists {
				result.Added = append(result.Added, ep)
			} else if !sameEndpoint(before, ep) {
				result.Changed = append(result.Changed, ep)
			}
			if !exists || !reflect.DeepEqual(contractProtection(before), contractProtection(ep)) {
				protectionChanged = true
			}
		}
		for _, ep := range source.Endpoints {
			if _, ok := next[ep.ID]; !ok {
				result.Removed = append(result.Removed, ep)
			}
		}
		caseChanges := []Change{}
		affected := map[string]bool{}
		for _, ep := range result.Changed {
			affected[ep.ID] = true
		}
		for _, ep := range result.Removed {
			affected[ep.ID] = true
		}
		for _, item := range items {
			if !affected[item.Request.EndpointID] {
				continue
			}
			result.Cases = append(result.Cases, item)
			if ep, ok := next[item.Request.EndpointID]; ok {
				query, headers := url.Values{}, http.Header{}
				for _, f := range item.Request.Query {
					query.Add(f.Name, f.Value)
				}
				for _, f := range item.Request.Headers {
					headers.Add(f.Name, f.Value)
				}
				if err := serializeContract(ep, item.Request.Path, query, headers); err != nil {
					result.Diagnostics = append(result.Diagnostics, fmt.Sprintf("Case %s: correct preserved inputs before sending: %s", item.ID, err))
				}
				d := item.Request
				if d.BodyFile == "" && len(d.Form) == 0 && !strings.Contains(d.Body, "{{") {
					contentType := d.ContentType
					if contentType == "" && d.BodyType == "json" {
						contentType = "application/json"
					}
					if err := validateContractBody(ep, d.Body, contentType); err != nil {
						result.Diagnostics = append(result.Diagnostics, fmt.Sprintf("Case %s: correct preserved body before sending: %s", item.ID, err))
					}
				}
			} else {
				item.Request.Disconnected = true
				raw, err := yaml.Marshal(item.Case)
				if err != nil {
					return err
				}
				caseChanges = append(caseChanges, Change{Path: "cases/" + item.ID + ".yaml", ExpectedRevision: item.Revision, Content: raw})
				result.Diagnostics = append(result.Diagnostics, "Case "+item.ID+": operation removed; explicitly relink endpointId or clear it and supply module, method and url for an independent request")
			}
		}
		if preview {
			return nil
		}
		if len(result.Conflicts) > 0 {
			return Fail("revision_conflict", "unresolved contract conflicts: %s; reconcile local edits and preview again", strings.Join(result.Conflicts, ", "))
		}
		if token == "" || token != result.Token {
			return Fail("revision_conflict", "refresh preview is missing or stale; preview again")
		}
		candidate.Documents = incoming.Documents
		candidate.Base = incoming.Base
		candidate.Digest = incoming.Digest
		candidate.LocalDocuments = documents
		candidate.DeletedEndpoints = append(source.DeletedEndpoints, result.Removed...)
		b, e := yaml.Marshal(candidate)
		if e != nil {
			return e
		}
		changes := append(caseChanges, Change{Path: "specs/" + sourceID + ".yaml", ExpectedRevision: source.Revision, Content: b})
		if e = commit(root, changes, true); e != nil {
			return e
		}
		if protectionChanged {
			// Revoke before committing. A crash can revoke too much, but can never
			// transfer an existing approval to changed contract destinations/bindings.
			a, path, e := readAccess(root, snapshot)
			if e != nil {
				return e
			}
			for id, grant := range a.Grants {
				if _, ok := grant.BaseURLs[source.ModuleID]; ok {
					delete(a.Grants, id)
				}
			}
			if e = writeAccess(path, a); e != nil {
				return e
			}
		}
		return commit(root, changes, false)
	})
	return result, err
}

func sameEndpoint(a, b Endpoint) bool {
	a.Revision, b.Revision = "", ""
	return reflect.DeepEqual(a, b)
}

type absentRefreshValue struct{}

func mergeRefreshValue(base, local, incoming any, path string, conflicts *[]string) any {
	if reflect.DeepEqual(local, base) {
		return incoming
	}
	if reflect.DeepEqual(incoming, base) || reflect.DeepEqual(local, incoming) {
		return local
	}
	b, bok := base.(map[string]any)
	l, lok := local.(map[string]any)
	n, nok := incoming.(map[string]any)
	if bok && lok && nok {
		all := map[string]any{}
		for k := range b {
			all[k] = nil
		}
		for k := range l {
			all[k] = nil
		}
		for k := range n {
			all[k] = nil
		}
		merged := map[string]any{}
		get := func(m map[string]any, k string) any {
			if v, ok := m[k]; ok {
				return v
			}
			return absentRefreshValue{}
		}
		for _, k := range keys(all) {
			v := mergeRefreshValue(get(b, k), get(l, k), get(n, k), path+"/"+strings.ReplaceAll(strings.ReplaceAll(k, "~", "~0"), "/", "~1"), conflicts)
			if _, absent := v.(absentRefreshValue); !absent {
				merged[k] = v
			}
		}
		return merged
	}
	*conflicts = append(*conflicts, path)
	return local
}

func mergeRefreshDocuments(source, incoming Specification, conflicts *[]string) (map[string][]byte, error) {
	decode := func(docs map[string][]byte) (map[string]any, error) {
		result := map[string]any{}
		for name, raw := range docs {
			var v any
			if err := Decode(raw, &v); err != nil {
				return nil, err
			}
			normalized, err := normalizeYAML(v)
			if err != nil {
				return nil, err
			}
			result[name] = normalized
		}
		return result, nil
	}
	base, err := decode(source.Documents)
	if err != nil {
		return nil, err
	}
	local, err := decode(contractDocuments(source))
	if err != nil {
		return nil, err
	}
	next, err := decode(incoming.Documents)
	if err != nil {
		return nil, err
	}
	merged := mergeRefreshValue(base, local, next, "", conflicts).(map[string]any)
	docs := map[string][]byte{}
	for name, v := range merged {
		raw, err := yaml.Marshal(v)
		if err != nil {
			return nil, err
		}
		docs[name] = raw
	}
	return docs, nil
}

func resolveRefreshDocuments(root string, source Specification, documents map[string][]byte) (Specification, error) {
	r := resolver{root: filepath.Dir(source.Location), retainedOnly: true, documents: documents, parsed: map[string]any{}}
	for name, raw := range documents {
		var v any
		if err := Decode(raw, &v); err != nil {
			return Specification{}, err
		}
		v, err := normalizeYAML(v)
		if err != nil {
			return Specification{}, err
		}
		if source.Mode != "url" {
			name = filepath.Join(filepath.Dir(source.Location), filepath.FromSlash(name))
		}
		r.parsed[name] = v
	}
	if source.Mode == "url" {
		r.fetch = func(string) ([]byte, error) {
			return nil, Fail("invalid_input", "reference must target a retained source document")
		}
	}
	s, err := importResolvedSpecification(root, source.Location, source.Mode, &r, r.parsed[source.Location], true, "")
	if err != nil {
		return s, err
	}
	s.ID, s.ModuleID = source.ID, source.ModuleID
	s.EmbeddedName = source.EmbeddedName
	for i := range s.Endpoints {
		ep := &s.Endpoints[i]
		ep.SourceID, ep.ModuleID = s.ID, s.ModuleID
		ep.ID = uuid.NewSHA1(uuid.MustParse(s.ID), []byte(ep.Method+" "+ep.Path)).String()
	}
	for i := range s.Schemas {
		sc := &s.Schemas[i]
		sc.SourceID = s.ID
		sc.ID = uuid.NewSHA1(uuid.MustParse(s.ID), []byte("schema "+sc.Name)).String()
	}
	return s, nil
}

func appendDocumentBytes(groups ...map[string][]byte) [][]byte {
	result := [][]byte{}
	for _, docs := range groups {
		for _, raw := range docs {
			result = append(result, raw)
		}
	}
	return result
}
