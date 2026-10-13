package project

import (
	"sort"
	"strings"
)

// EndpointQuery selects operations without changing their source-based identities.
type EndpointQuery struct {
	Query      string
	Method     string
	Tag        string
	GroupByTag bool
}

type EndpointMatch struct {
	Endpoint
	Source  string   `json:"source"`
	Summary string   `json:"summary"`
	Tags    []string `json:"tags"`
	Group   string   `json:"group"`
}

// SearchEndpoints ranks path matches first, then metadata matches. Ties use
// path, method, source, module and endpoint IDs, independent of import order.
func SearchEndpoints(specs []Specification, q EndpointQuery) []EndpointMatch {
	type ranked struct {
		match EndpointMatch
		rank  int
	}
	rows := []ranked{}
	query := strings.ToLower(strings.TrimSpace(q.Query))
	for _, s := range specs {
		for _, e := range s.Endpoints {
			if q.Method != "" && !strings.EqualFold(q.Method, e.Method) {
				continue
			}
			tags := []string{}
			seen := map[string]bool{}
			add := func(tag string) {
				if !seen[tag] {
					seen[tag] = true
					tags = append(tags, tag)
				}
			}
			switch values := e.Operation["tags"].(type) {
			case []any:
				for _, v := range values {
					if tag, ok := v.(string); ok {
						add(tag)
					}
				}
			case []string:
				for _, tag := range values {
					add(tag)
				}
			}
			sort.Strings(tags)
			if q.Tag != "" {
				found := false
				for _, tag := range tags {
					if strings.EqualFold(tag, q.Tag) {
						found = true
					}
				}
				if !found {
					continue
				}
			}
			rank := 0
			if query != "" {
				rank = matchRank(e.Path, query)
				if rank < 0 {
					rank = 1000000
					for _, field := range []string{"operationId", "summary", "description"} {
						value, _ := e.Operation[field].(string)
						if n := matchRank(value, query); n >= 0 && n+10000 < rank {
							rank = n + 10000
						}
					}
					for _, tag := range tags {
						if n := matchRank(tag, query); n >= 0 && n+10000 < rank {
							rank = n + 10000
						}
					}
					if rank == 1000000 {
						continue
					}
				}
			}
			summary, _ := e.Operation["summary"].(string)
			groups := []string{""}
			if q.GroupByTag {
				groups = tags
				if len(groups) == 0 {
					groups = []string{"Untagged"}
				}
			}
			for _, group := range groups {
				rows = append(rows, ranked{EndpointMatch{e, s.Location, summary, tags, group}, rank})
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if q.GroupByTag && a.match.Group != b.match.Group {
			return a.match.Group < b.match.Group
		}
		if a.rank != b.rank {
			return a.rank < b.rank
		}
		x, y := a.match, b.match
		for _, pair := range [][2]string{{x.Path, y.Path}, {x.Method, y.Method}, {x.SourceID, y.SourceID}, {x.ModuleID, y.ModuleID}, {x.ID, y.ID}} {
			if pair[0] != pair[1] {
				return pair[0] < pair[1]
			}
		}
		return false
	})
	result := make([]EndpointMatch, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.match)
	}
	return result
}

// A contiguous match outranks a subsequence match; fuzzy gaps break ties.
func matchRank(value, query string) int {
	value = strings.ToLower(value)
	if strings.Contains(value, query) {
		return 0
	}
	needle := []rune(query)
	pos, start, end := 0, -1, 0
	for i, r := range []rune(value) {
		if pos < len(needle) && r == needle[pos] {
			if start < 0 {
				start = i
			}
			end = i
			pos++
		}
	}
	if pos != len(needle) {
		return -1
	}
	return min(9999, end-start-len(needle)+2)
}
