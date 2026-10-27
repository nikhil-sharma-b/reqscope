package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEndpointLookupFollowsContractChanges(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(root, "Cache", false); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "api.yaml")
	os.WriteFile(file, []byte("openapi: 3.0.3\ninfo: {title: Cache, version: '1'}\npaths: {'/pets': {get: {summary: Before, responses: {'200': {description: OK}}}}}\n"), 0600)
	s, err := ImportSpecification(root, file, false)
	if err != nil {
		t.Fatal(err)
	}
	id := s.Endpoints[0].ID
	first, err := lookupEndpoint(root, id)
	if err != nil || first == nil || first.Operation["summary"] != "Before" || first.Revision != s.Revision {
		t.Fatalf("%+v %v", first, err)
	}
	if _, err := EditContract(root, "endpoint", "update", ContractEdit{SourceID: s.ID, Revision: s.Revision, ImportedEdit: true, Path: "/pets", Method: "GET", Value: map[string]any{"summary": "After", "responses": map[string]any{"200": map[string]any{"description": "OK"}}}}, false); err != nil {
		t.Fatal(err)
	}
	second, err := lookupEndpoint(root, id)
	if err != nil || second == nil || second.Operation["summary"] != "After" || second.Revision == first.Revision {
		t.Fatalf("a changed contract served the cached endpoint: %+v %v", second, err)
	}
	if missing, err := lookupEndpoint(root, "not-an-endpoint"); err != nil || missing != nil {
		t.Fatalf("%+v %v", missing, err)
	}
}
