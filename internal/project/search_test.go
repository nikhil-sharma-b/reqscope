package project_test

import (
	"github.com/nikhil-sharma-b/reqscope/internal/project"
	"reflect"
	"testing"
)

func TestEndpointSearchRankingAndFilters(t *testing.T) {
	specs := []project.Specification{{ID: "source", Location: "pets.yaml", Endpoints: []project.Endpoint{
		{ID: "description", Method: "GET", Path: "/animals", Operation: map[string]any{"description": "Manage PETS", "operationId": "findAnimal", "tags": []any{"store", "store"}}},
		{ID: "path-post", Method: "POST", Path: "/pets", Operation: map[string]any{"summary": "Create pet", "tags": []any{"store"}}},
		{ID: "path-get", Method: "GET", Path: "/pets", Operation: map[string]any{"tags": []any{"store"}}},
	}}}
	ids := func(q project.EndpointQuery) []string {
		got := []string{}
		for _, m := range project.SearchEndpoints(specs, q) {
			got = append(got, m.ID)
		}
		return got
	}
	for _, tt := range []struct {
		query project.EndpointQuery
		want  []string
	}{
		{project.EndpointQuery{Query: "PETS"}, []string{"path-get", "path-post", "description"}},
		{project.EndpointQuery{Query: "pts", Method: "get"}, []string{"path-get", "description"}},
		{project.EndpointQuery{Query: "manage"}, []string{"description"}},
		{project.EndpointQuery{Query: "findanim"}, []string{"description"}},
		{project.EndpointQuery{Query: "store", Method: "POST", Tag: "STORE", GroupByTag: true}, []string{"path-post"}},
		{project.EndpointQuery{Query: "absent"}, []string{}},
		{project.EndpointQuery{Tag: "missing"}, []string{}},
	} {
		if got := ids(tt.query); !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("%+v: %v, want %v", tt.query, got, tt.want)
		}
	}
	rows := project.SearchEndpoints(specs, project.EndpointQuery{GroupByTag: true})
	if len(rows) != 3 || rows[0].Source != "pets.yaml" {
		t.Fatal(rows)
	}
	specs[0].Endpoints[0], specs[0].Endpoints[2] = specs[0].Endpoints[2], specs[0].Endpoints[0]
	if got := ids(project.EndpointQuery{Query: "pets"}); !reflect.DeepEqual(got, []string{"path-get", "path-post", "description"}) {
		t.Fatal("unstable ranking", got)
	}
}
