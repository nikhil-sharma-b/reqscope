package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEndpointSearchPagination(t *testing.T) {
	root := t.TempDir()
	specCommand(t, root, "project", "init", "Search")
	b, _ := json.Marshal(contract("3.1.0"))
	file := filepath.Join(t.TempDir(), "api.json")
	os.WriteFile(file, b, 0600)
	if code, e := specCommand(t, root, "spec", "import", "--input", file); code != 0 {
		t.Fatal(e)
	}
	code, e := specCommand(t, root, "endpoint", "search", "--query", "pts", "--limit", "1")
	if code != 0 {
		t.Fatal(e)
	}
	page := e["data"].(map[string]any)
	if page["total"] != float64(2) || page["nextOffset"] != float64(1) || len(page["items"].([]any)) != 1 {
		t.Fatal(page)
	}
	first := page["items"].([]any)[0].(map[string]any)
	if first["method"] != "DELETE" || first["source"] != file || first["moduleId"] == "" {
		t.Fatal(first)
	}
	_, e = specCommand(t, root, "endpoint", "search", "--query", "pts", "--limit", "1", "--offset", "1")
	page = e["data"].(map[string]any)
	if page["nextOffset"] != nil || page["items"].([]any)[0].(map[string]any)["method"] != "GET" {
		t.Fatal(page)
	}
	_, e = specCommand(t, root, "endpoint", "search", "--query", "find", "--method", "get", "--tag", "pets", "--group-by", "tag")
	page = e["data"].(map[string]any)
	if page["total"] != float64(1) || page["items"].([]any)[0].(map[string]any)["group"] != "pets" {
		t.Fatal(page)
	}
	_, e = specCommand(t, root, "endpoint", "search", "--query", "zzzz")
	page = e["data"].(map[string]any)
	if page["empty"] != true || len(page["items"].([]any)) != 0 {
		t.Fatal(page)
	}
	for _, args := range [][]string{{"--limit", "0"}, {"--limit", "1001"}, {"--offset", "-1"}, {"--method", "GTE"}, {"--group-by", "source"}} {
		if code, e := specCommand(t, root, append([]string{"endpoint", "search"}, args...)...); code != 2 {
			t.Fatal(e)
		}
	}
}
