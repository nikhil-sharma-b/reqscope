package project_test

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

func TestURLFormActualBytes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if string(b) != "tag=a%26b&tag=c+d" || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Errorf("%q %v", b, r.Header)
		}
	}))
	defer server.Close()
	root := requestWorkspace(t, server.URL)
	_, err := project.Send(context.Background(), root, "staging", project.RequestDraft{Module: "api", Method: "POST", URL: "/", BodyType: "form", Form: []project.FormField{{Name: "tag", Value: "a&b"}, {Name: "tag", Value: "c d"}}}, project.SendOptions{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestMultipartBinaryAndRepeatedFields(t *testing.T) {
	data := []byte{0, 255, 13, 10, 1}
	path := filepath.Join(t.TempDir(), "blob.bin")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "multipart/form-data" || params["boundary"] == "" {
			t.Errorf("%v %v", r.Header, err)
			return
		}
		reader := multipart.NewReader(r.Body, params["boundary"])
		names := []string{"tag", "tag", "file"}
		values := []string{"one", "two", string(data)}
		for i, name := range names {
			p, err := reader.NextPart()
			if err != nil {
				t.Error(err)
				return
			}
			b, _ := io.ReadAll(p)
			if p.FormName() != name || string(b) != values[i] {
				t.Errorf("part %d: %s %q", i, p.FormName(), b)
			}
			if i == 2 && (p.FileName() != "blob.bin" || p.Header.Get("Content-Type") != "application/octet-stream") {
				t.Errorf("%v", p.Header)
			}
		}
		if _, err := reader.NextPart(); err != io.EOF {
			t.Errorf("extra part: %v", err)
		}
	}))
	defer server.Close()
	root := requestWorkspace(t, server.URL)
	_, err := project.Send(context.Background(), root, "staging", project.RequestDraft{Module: "api", Method: "POST", URL: "/", BodyType: "multipart", Form: []project.FormField{{Name: "tag", Value: "one"}, {Name: "tag", Value: "two"}, {Name: "file", File: path, ContentType: "application/octet-stream"}}}, project.SendOptions{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestMultipartConfirmationTracksFileContents(t *testing.T) {
	root := requestWorkspace(t, "http://localhost:12345")
	path := filepath.Join(root, "data.bin")
	os.WriteFile(path, []byte("one"), 0600)
	d := project.RequestDraft{Module: "api", Method: "POST", URL: "/", BodyType: "multipart", Form: []project.FormField{{Name: "file", File: "data.bin"}}}
	a, err := project.PrepareRequest(root, "staging", d, true)
	if err != nil {
		t.Fatal(err)
	}
	b, err := project.PrepareRequest(root, "staging", d, true)
	if err != nil {
		t.Fatal(err)
	}
	if a.Fingerprint != b.Fingerprint {
		t.Fatal("unchanged multipart draft invalidated confirmation")
	}
	os.WriteFile(path, []byte("two"), 0600)
	c, err := project.PrepareRequest(root, "staging", d, true)
	if err != nil {
		t.Fatal(err)
	}
	if a.Fingerprint == c.Fingerprint {
		t.Fatal("changed upload did not invalidate confirmation")
	}
}

func TestUploadPreflightNoNetwork(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer server.Close()
	root := requestWorkspace(t, server.URL)
	unreadable := filepath.Join(root, "unreadable.bin")
	os.WriteFile(unreadable, []byte("hidden"), 0000)
	for _, path := range []string{"missing-secret-path.bin", "unreadable.bin", ".", "{{file}}"} {
		_, err := project.Send(context.Background(), root, "staging", project.RequestDraft{Module: "api", Method: "POST", URL: "/", BodyType: "multipart", Form: []project.FormField{{Name: "file", File: path}}}, project.SendOptions{})
		if err == nil {
			t.Errorf("accepted %q", path)
		}
		if err != nil && strings.Contains(err.Error(), path) {
			t.Errorf("path leaked: %v", err)
		}
	}
	if calls != 0 {
		t.Fatalf("preflight made %d requests", calls)
	}
}

func TestFormContractValidationAndSerialization(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		b, _ := io.ReadAll(r.Body)
		if string(b) != "count=2&tags=one%2Ctwo" {
			t.Errorf("%q", b)
		}
	}))
	defer server.Close()
	root := requestWorkspace(t, server.URL)
	path := filepath.Join(t.TempDir(), "form.json")
	spec := `{"openapi":"3.0.3","info":{"title":"Forms","version":"1"},"paths":{"/":{"post":{"responses":{"200":{"description":"ok"}},"requestBody":{"required":true,"content":{"application/x-www-form-urlencoded":{"schema":{"type":"object","required":["count"],"properties":{"count":{"type":"integer"},"tags":{"type":"array","items":{"type":"string"}}}},"encoding":{"tags":{"style":"form","explode":false}}}}}}}}}`
	os.WriteFile(path, []byte(spec), 0600)
	imported, err := project.ImportSpecification(root, path, false)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := project.Load(root)
	e, _ := s.Resolve("staging")
	e.BaseURLs[imported.ModuleID] = server.URL
	if err := project.SaveEnvironment(root, e.Environment, e.Revision, false, false); err != nil {
		t.Fatal(err)
	}
	review, _ := project.ReviewAccess(root, "staging")
	project.ApproveAccess(root, review)
	d, fields, err := project.ContractDraft(root, imported.Endpoints[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if d.BodyType != "form" || len(fields) != 2 {
		t.Fatalf("%+v %+v", d, fields)
	}
	for _, form := range [][]project.FormField{nil, {{Name: "count", Value: "bad"}}} {
		d.Form = form
		if _, err := project.Send(context.Background(), root, "staging", d, project.SendOptions{}); err == nil {
			t.Fatal("invalid required input accepted")
		}
	}
	raw := d
	raw.BodyType = "raw"
	raw.Form = nil
	raw.Body = "other=value"
	if _, err := project.Send(context.Background(), root, "staging", raw, project.SendOptions{}); err == nil {
		t.Fatal("raw form bypassed required contract inputs")
	}
	if calls != 0 {
		t.Fatal("invalid contract contacted server")
	}
	d.Form = []project.FormField{{Name: "count", Value: "2"}, {Name: "tags", Value: "one"}, {Name: "tags", Value: "two"}}
	if _, err := project.Send(context.Background(), root, "staging", d, project.SendOptions{}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal(calls)
	}
}

func TestFormCredentialsEncodeValuesAndPreserveFileBytes(t *testing.T) {
	secret := "token &+value"
	data := []byte("\x00{{secret:key}}\xff")
	for _, kind := range []string{"form", "multipart"} {
		t.Run(kind, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if kind == "form" {
					if err := r.ParseForm(); err != nil {
						t.Error(err)
					}
					if r.PostForm.Get("token") != secret {
						t.Error("secret escaped incorrectly")
					}
				} else {
					if err := r.ParseMultipartForm(1024); err != nil {
						t.Error(err)
						return
					}
					defer r.MultipartForm.RemoveAll()
					if r.FormValue("token") != secret {
						t.Error("multipart secret changed")
					}
					f, _, err := r.FormFile("file")
					if err != nil {
						t.Error(err)
						return
					}
					defer f.Close()
					b, _ := io.ReadAll(f)
					if string(b) != string(data) {
						t.Error("binary references expanded")
					}
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"token":"unknown-value"}`)
			}))
			defer server.Close()
			root := requestWorkspace(t, server.URL)
			os.WriteFile(filepath.Join(root, "file.bin"), data, 0600)
			vault := project.NewCredentials(&credentialStore{values: map[string]string{}})
			if err := vault.Save(root, "staging", project.CredentialBinding{Name: "key", Kind: "api-key", Module: "api", Destination: server.URL + "/", In: "body"}, secret, false); err != nil {
				t.Fatal(err)
			}
			review, _ := project.ReviewAccess(root, "staging")
			project.ApproveAccess(root, review)
			d := project.RequestDraft{Module: "api", Method: "POST", URL: "/", BodyType: kind, Variables: map[string]string{"token": "{{secret:key}}"}, Form: []project.FormField{{Name: "token", Value: "{{token}}"}}}
			if kind == "multipart" {
				d.Form = append(d.Form, project.FormField{Name: "file", File: "file.bin"})
			}
			result, err := project.Send(context.Background(), root, "staging", d, project.SendOptions{Credentials: vault})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(result.Body, "unknown-value") || strings.Contains(result.Body, secret) {
				t.Fatal("response credential leaked")
			}
		})
	}
}

func TestSavedMultipartStoresOnlyReferencesAndRawFile(t *testing.T) {
	payload := "binary-content-not-for-project-data"
	received := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
			r.ParseMultipartForm(1024)
			defer r.MultipartForm.RemoveAll()
			f, _, err := r.FormFile("file")
			if err != nil {
				t.Error(err)
				return
			}
			defer f.Close()
			b, _ := io.ReadAll(f)
			received <- string(b)
		} else {
			b, _ := io.ReadAll(r.Body)
			received <- string(b)
		}
	}))
	defer server.Close()
	root := requestWorkspace(t, server.URL)
	os.WriteFile(filepath.Join(root, "payload.bin"), []byte(payload), 0600)
	c := project.Case{ID: "9b0c819b-2070-467e-a1c2-ab629f8d85e1", Name: "Upload", Request: project.RequestDraft{Module: "api", Method: "POST", URL: "/", BodyType: "multipart", Form: []project.FormField{{Name: "file", File: "payload.bin"}}}}
	if _, err := project.SaveCase(root, c, "", false); err != nil {
		t.Fatal(err)
	}
	paths, _ := filepath.Glob(filepath.Join(root, "cases", "*.yaml"))
	for _, path := range paths {
		b, _ := os.ReadFile(path)
		if strings.Contains(string(b), payload) {
			t.Fatal("stored upload bytes")
		}
	}
	if _, err := project.SendCase(context.Background(), root, "staging", c.ID, nil, project.SendOptions{}); err != nil {
		t.Fatal(err)
	}
	d := project.RequestDraft{Module: "api", Method: "POST", URL: "/", BodyType: "raw", BodyFile: "payload.bin", ContentType: "application/octet-stream"}
	if _, err := project.Send(context.Background(), root, "staging", d, project.SendOptions{}); err != nil {
		t.Fatal(err)
	}
	if a, b := <-received, <-received; a != payload || b != payload {
		t.Fatalf("%q %q", a, b)
	}
}

func TestSwaggerFormAndFileContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1024); err != nil {
			t.Error(err)
			return
		}
		defer r.MultipartForm.RemoveAll()
		if r.FormValue("count") != "3" || len(r.MultipartForm.Value["tag"]) != 2 || r.FormValue("labels") != "x,y" || len(r.MultipartForm.Value["labels"]) != 1 {
			t.Error(r.MultipartForm.Value)
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			t.Error(err)
			return
		}
		defer f.Close()
		b, _ := io.ReadAll(f)
		if string(b) != "blob" {
			t.Error(string(b))
		}
	}))
	defer server.Close()
	root := requestWorkspace(t, server.URL)
	path := filepath.Join(t.TempDir(), "swagger.json")
	spec := `{"swagger":"2.0","info":{"title":"Uploads","version":"1"},"consumes":["multipart/form-data"],"paths":{"/":{"post":{"responses":{"200":{"description":"ok"}},"parameters":[{"in":"formData","name":"count","type":"integer","required":true},{"in":"formData","name":"tag","type":"array","items":{"type":"string"},"collectionFormat":"multi"},{"in":"formData","name":"file","type":"file","required":true},{"in":"formData","name":"labels","type":"array","items":{"type":"string"}}]}}}}`
	os.WriteFile(path, []byte(spec), 0600)
	imported, err := project.ImportSpecification(root, path, false)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := project.Load(root)
	e, _ := s.Resolve("staging")
	e.BaseURLs[imported.ModuleID] = server.URL
	project.SaveEnvironment(root, e.Environment, e.Revision, false, false)
	review, _ := project.ReviewAccess(root, "staging")
	project.ApproveAccess(root, review)
	d, fields, err := project.ContractDraft(root, imported.Endpoints[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if d.BodyType != "multipart" || fields[2].In != "file" {
		t.Fatalf("%+v %+v", d, fields)
	}
	os.WriteFile(filepath.Join(root, "blob.bin"), []byte("blob"), 0600)
	d.Form = []project.FormField{{Name: "count", Value: "3"}, {Name: "tag", Value: "a"}, {Name: "tag", Value: "b"}, {Name: "file", File: "blob.bin"}, {Name: "labels", Value: "x"}, {Name: "labels", Value: "y"}}
	if _, err := project.Send(context.Background(), root, "staging", d, project.SendOptions{}); err != nil {
		t.Fatal(err)
	}
}
