package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nikhil-sharma-b/reqscope/internal/cli"
	"go.yaml.in/yaml/v3"
)

func specCommand(t *testing.T, root string, args ...string) (int, map[string]any) {
	t.Helper()
	var out, diag bytes.Buffer
	code := cli.Run(append([]string{"--project", root, "--output", "json"}, args...), strings.NewReader(""), &out, &diag)
	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	return code, envelope
}
func contract(version string) map[string]any {
	doc := map[string]any{"info": map[string]any{"title": "Pet store", "version": "1.0"}, "x-owner": map[string]any{"team": "pets"}, "paths": map[string]any{"/pets/{id}": map[string]any{"parameters": []any{map[string]any{"name": "id", "in": "path", "required": true, "type": "string"}}, "get": map[string]any{"operationId": "duplicate", "description": "Find a pet", "tags": []string{"pets"}, "responses": map[string]any{"200": map[string]any{"description": "A pet"}}}, "delete": map[string]any{"operationId": "duplicate", "responses": map[string]any{"204": map[string]any{"description": "Deleted"}}}}}}
	schemas := map[string]any{"Pet": map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}, "x-sensitive": true}}
	if version == "2.0" {
		doc["swagger"] = version
		doc["definitions"] = schemas
	} else {
		doc["openapi"] = version
		doc["components"] = map[string]any{"schemas": schemas}
		item := doc["paths"].(map[string]any)["/pets/{id}"].(map[string]any)
		item["parameters"] = []any{map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}}}
	}
	return doc
}
func TestImportVersionsAndFormats(t *testing.T) {
	for _, version := range []string{"2.0", "3.0.3", "3.1.0"} {
		for _, format := range []string{"json", "yaml"} {
			t.Run(version+format, func(t *testing.T) {
				root := t.TempDir()
				if code, e := specCommand(t, root, "project", "init", "Pets"); code != 0 {
					t.Fatal(e)
				}
				doc := contract(version)
				var b []byte
				if format == "json" {
					b, _ = json.Marshal(doc)
				} else {
					b, _ = yaml.Marshal(doc)
				}
				file := filepath.Join(t.TempDir(), "pets."+format)
				os.WriteFile(file, b, 0600)
				code, e := specCommand(t, root, "spec", "import", "--input", file, "--preview")
				if code != 0 {
					t.Fatal(e)
				}
				preview := e["data"].(map[string]any)
				if preview["complete"] != false || len(preview["diagnostics"].([]any)) == 0 {
					t.Fatal("incomplete validation hidden")
				}
				_, e = specCommand(t, root, "endpoint", "list")
				if len(e["data"].([]any)) != 0 {
					t.Fatal("preview persisted endpoints")
				}
				code, e = specCommand(t, root, "spec", "import", "--input", file)
				if code != 0 {
					t.Fatal(e)
				}
				source := e["data"].(map[string]any)
				if source["digest"] == "" || source["moduleId"] == "" {
					t.Fatal(source)
				}
				code, e = specCommand(t, root, "endpoint", "list")
				if code != 0 {
					t.Fatal(e)
				}
				endpoints := e["data"].([]any)
				if len(endpoints) != 2 {
					t.Fatal(endpoints)
				}
				a := endpoints[0].(map[string]any)
				other := endpoints[1].(map[string]any)
				if a["id"] == other["id"] {
					t.Fatal("operationId used as sole identity")
				}
				_, e = specCommand(t, root, "endpoint", "get", a["id"].(string))
				if e["data"].(map[string]any)["revision"] == "" {
					t.Fatal(e)
				}
				_, e = specCommand(t, root, "schema", "list")
				schemas := e["data"].([]any)
				if len(schemas) != 1 {
					t.Fatal(e)
				}
				_, e = specCommand(t, root, "schema", "get", schemas[0].(map[string]any)["id"].(string))
				if e["data"].(map[string]any)["schema"].(map[string]any)["definition"].(map[string]any)["x-sensitive"] != true {
					t.Fatal("extension lost")
				}
				_, e = specCommand(t, root, "spec", "inspect", source["id"].(string))
				stored := e["data"].(map[string]any)
				raw := stored["documents"].(map[string]any)[filepath.Base(file)].(string)
				var restored []byte
				if err := json.Unmarshal([]byte(`"`+raw+`"`), &restored); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(b, restored) {
					t.Fatal("original document changed")
				}
				if stored["base"].(map[string]any)["x-owner"] == nil {
					t.Fatal("base lost extension")
				}
			})
		}
	}
}
func TestLocalReferencesAndInvalidImports(t *testing.T) {
	root := t.TempDir()
	specCommand(t, root, "project", "init", "Pets")
	dir := t.TempDir()
	file := filepath.Join(dir, "api.yaml")
	base := "openapi: 3.1.0\ninfo: {title: Pets, version: '1'}\npaths: {}\ncomponents:\n  schemas:\n    Pet: {$ref: 'models.yaml#/Pet'}\n"
	os.WriteFile(filepath.Join(dir, "models.yaml"), []byte("Pet: {type: object, oneOf: [{type: string}, {type: integer}], x-team: pets}\n"), 0600)
	os.WriteFile(file, []byte(base), 0600)
	code, e := specCommand(t, root, "spec", "import", "--input", file)
	if code != 0 {
		t.Fatal(e)
	}
	if len(e["data"].(map[string]any)["documents"].(map[string]any)) != 2 {
		t.Fatal("reference bytes not retained")
	}
	_, before := specCommand(t, root, "project", "inspect")
	for _, ref := range []string{"missing.yaml#/Pet", "models.yaml#/Missing", "https://example.invalid/spec.yaml#/Pet", "../outside.yaml#/Pet", "api.yaml#/components/schemas/Pet"} {
		t.Run(ref, func(t *testing.T) {
			os.WriteFile(file, []byte(strings.Replace(base, "models.yaml#/Pet", ref, 1)), 0600)
			code, e := specCommand(t, root, "spec", "import", "--input", file)
			if code != 2 || e["error"] == nil {
				t.Fatal(e)
			}
			_, after := specCommand(t, root, "project", "inspect")
			if before["data"].(map[string]any)["revision"] != after["data"].(map[string]any)["revision"] {
				t.Fatal("failed import modified project")
			}
		})
	}
}

func TestMalformedContractsAreRejected(t *testing.T) {
	root := t.TempDir()
	specCommand(t, root, "project", "init", "Pets")
	for _, change := range []string{"parameters", "schema", "tags"} {
		t.Run(change, func(t *testing.T) {
			doc := contract("3.1.0")
			item := doc["paths"].(map[string]any)["/pets/{id}"].(map[string]any)
			switch change {
			case "parameters":
				item["parameters"] = "broken"
			case "schema":
				doc["components"].(map[string]any)["schemas"].(map[string]any)["Pet"] = map[string]any{"type": 123}
			case "tags":
				item["get"].(map[string]any)["tags"] = 123
			}
			b, _ := json.Marshal(doc)
			file := filepath.Join(t.TempDir(), "api.json")
			os.WriteFile(file, b, 0600)
			code, e := specCommand(t, root, "spec", "import", "--input", file, "--preview")
			if code != 2 {
				t.Fatalf("malformed %s accepted: %v", change, e)
			}
		})
	}
}

func TestOpaqueExtensionsAndBooleanReferences(t *testing.T) {
	root := t.TempDir()
	specCommand(t, root, "project", "init", "Pets")
	file := filepath.Join(t.TempDir(), "api.yaml")
	os.WriteFile(file, []byte("openapi: 3.1.0\ninfo: {title: Pets, version: '1'}\nx-catalog: {$ref: 'https://example.invalid/metadata'}\npaths: {}\ncomponents:\n  schemas:\n    Anything: true\n    Alias: {$ref: '#/components/schemas/Anything'}\n"), 0600)
	code, e := specCommand(t, root, "spec", "import", "--input", file)
	if code != 0 {
		t.Fatal(e)
	}
	base := e["data"].(map[string]any)["base"].(map[string]any)
	if base["x-catalog"].(map[string]any)["$ref"] != "https://example.invalid/metadata" {
		t.Fatal("extension interpreted")
	}
}
func TestYAMLNumericResponseCodes(t *testing.T) {
	root := t.TempDir()
	specCommand(t, root, "project", "init", "Pets")
	file := filepath.Join(t.TempDir(), "api.yaml")
	os.WriteFile(file, []byte("swagger: '2.0'\ninfo: {title: Pets, version: '1'}\npaths:\n  /pets:\n    get:\n      responses:\n        200: {description: OK}\n        default: {description: Error}\n"), 0600)
	code, e := specCommand(t, root, "spec", "import", "--input", file)
	if code != 0 {
		t.Fatal(e)
	}
}

func TestReferenceContainmentAndBounds(t *testing.T) {
	root := t.TempDir()
	specCommand(t, root, "project", "init", "Pets")
	dir := t.TempDir()
	file := filepath.Join(dir, "api.yaml")
	outside := filepath.Join(t.TempDir(), "secret.yaml")
	os.WriteFile(outside, []byte("Pet: {type: string}\n"), 0600)
	os.Symlink(outside, filepath.Join(dir, "linked.yaml"))
	for _, ref := range []string{"linked.yaml#/Pet", "deep.yaml#/Pet"} {
		t.Run(ref, func(t *testing.T) {
			os.WriteFile(file, []byte("openapi: 3.1.0\ninfo: {title: Pets, version: '1'}\npaths: {}\ncomponents: {schemas: {Pet: {$ref: '"+ref+"'}}}\n"), 0600)
			if ref == "deep.yaml#/Pet" {
				nested := "{type: string}"
				for i := 0; i < 70; i++ {
					nested = "{properties: {child: " + nested + "}}"
				}
				os.WriteFile(filepath.Join(dir, "deep.yaml"), []byte("Pet: "+nested), 0600)
			}
			code, e := specCommand(t, root, "spec", "import", "--input", file, "--preview")
			if code != 2 {
				t.Fatal(e)
			}
		})
	}
}

func TestReusableParameters(t *testing.T) {
	for _, version := range []string{"2.0", "3.0.3", "3.1.0"} {
		t.Run(version, func(t *testing.T) {
			root := t.TempDir()
			specCommand(t, root, "project", "init", "Pets")
			doc := contract(version)
			parameter := map[string]any{"name": "id", "in": "path", "required": true}
			ref := "#/parameters/ID"
			if version == "2.0" {
				parameter["type"] = "string"
				doc["parameters"] = map[string]any{"ID": parameter}
			} else {
				parameter["schema"] = map[string]any{"type": "string"}
				doc["components"].(map[string]any)["parameters"] = map[string]any{"ID": parameter}
				ref = "#/components/parameters/ID"
			}
			doc["paths"].(map[string]any)["/pets/{id}"].(map[string]any)["parameters"] = []any{map[string]any{"$ref": ref}}
			b, _ := json.Marshal(doc)
			file := filepath.Join(t.TempDir(), "api.json")
			os.WriteFile(file, b, 0600)
			code, e := specCommand(t, root, "spec", "import", "--input", file)
			if code != 0 {
				t.Fatal(e)
			}
		})
	}
}

func TestSchemaPropertyNamesDoNotHideReferences(t *testing.T) {
	root := t.TempDir()
	specCommand(t, root, "project", "init", "Pets")
	file := filepath.Join(t.TempDir(), "api.yaml")
	for _, name := range []string{"enum", "default", "x-property"} {
		t.Run(name, func(t *testing.T) {
			os.WriteFile(file, []byte("openapi: 3.1.0\ninfo: {title: Pets, version: '1'}\npaths: {}\ncomponents: {schemas: {Pet: {type: object, properties: {'"+name+"': {$ref: 'missing.yaml#/Pet'}}}}}\n"), 0600)
			code, e := specCommand(t, root, "spec", "import", "--input", file, "--preview")
			if code != 2 {
				t.Fatal(e)
			}
		})
	}
}
