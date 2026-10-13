package project

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

type staticToken struct {
	text    string
	literal bool
}

// Tokenize literal configuration only. Comments and string contents cannot become declarations.
func staticTokens(s string) []staticToken {
	tokens := []staticToken{}
	for i := 0; i < len(s); {
		ch := s[i]
		if unicode.IsSpace(rune(ch)) {
			i++
			continue
		}
		if strings.HasPrefix(s[i:], "//") {
			end := strings.IndexByte(s[i:], '\n')
			if end < 0 {
				break
			}
			i += end
			continue
		}
		if strings.HasPrefix(s[i:], "/*") {
			end := strings.Index(s[i+2:], "*/")
			if end < 0 {
				break
			}
			i += end + 4
			continue
		}
		if ch == '\'' || ch == '"' {
			quote := ch
			i++
			var value strings.Builder
			valid := true
			closed := false
			for i < len(s) {
				ch = s[i]
				i++
				if ch == quote {
					closed = true
					break
				}
				if ch == '\\' {
					if i >= len(s) {
						break
					}
					ch = s[i]
					i++
					switch ch {
					case '\\', '\'', '"', '/':
						value.WriteByte(ch)
					case 'n':
						value.WriteByte('\n')
					case 'r':
						value.WriteByte('\r')
					case 't':
						value.WriteByte('\t')
					case 'u':
						if i+4 > len(s) {
							valid = false
							break
						}
						n, err := strconv.ParseUint(s[i:i+4], 16, 16)
						if err != nil {
							valid = false
						}
						value.WriteRune(rune(n))
						i += 4
					default:
						valid = false
					}
				} else {
					value.WriteByte(ch)
				}
			}
			tokens = append(tokens, staticToken{value.String(), valid && closed})
			continue
		}
		if ch == '`' {
			i++
			for i < len(s) && s[i] != '`' {
				if s[i] == '\\' {
					i++
				}
				i++
			}
			if i < len(s) {
				i++
			}
			tokens = append(tokens, staticToken{"dynamic", false})
			continue
		}
		if unicode.IsLetter(rune(ch)) || ch == '_' || ch == '$' || ch >= '0' && ch <= '9' || ch == '-' {
			start := i
			i++
			for i < len(s) && (unicode.IsLetter(rune(s[i])) || unicode.IsDigit(rune(s[i])) || s[i] == '_' || s[i] == '$' || s[i] == '.') {
				i++
			}
			tokens = append(tokens, staticToken{s[start:i], false})
			continue
		}
		tokens = append(tokens, staticToken{string(ch), false})
		i++
	}
	return tokens
}
func staticValue(tokens []staticToken, index *int, depth int) (any, bool) {
	if depth > 64 || *index >= len(tokens) {
		return nil, false
	}
	token := tokens[*index]
	*index++
	if token.literal {
		return token.text, true
	}
	switch token.text {
	case "true":
		return true, true
	case "false":
		return false, true
	case "null":
		return nil, true
	case "{":
		m := map[string]any{}
		for *index < len(tokens) && tokens[*index].text != "}" {
			key := tokens[*index].text
			*index++
			if *index >= len(tokens) || tokens[*index].text != ":" {
				return nil, false
			}
			*index++
			v, ok := staticValue(tokens, index, depth+1)
			if !ok {
				return nil, false
			}
			m[key] = v
			if *index < len(tokens) && tokens[*index].text == "," {
				*index++
			} else {
				break
			}
		}
		if *index >= len(tokens) || tokens[*index].text != "}" {
			return nil, false
		}
		*index++
		return m, true
	case "[":
		a := []any{}
		for *index < len(tokens) && tokens[*index].text != "]" {
			v, ok := staticValue(tokens, index, depth+1)
			if !ok {
				return nil, false
			}
			a = append(a, v)
			if *index < len(tokens) && tokens[*index].text == "," {
				*index++
			} else {
				break
			}
		}
		if *index >= len(tokens) || tokens[*index].text != "]" {
			return nil, false
		}
		*index++
		return a, true
	}
	var number json.Number = json.Number(token.text)
	if _, err := number.Float64(); err == nil {
		return number, true
	}
	return nil, false
}

var scriptTag = regexp.MustCompile(`(?is)<script\b([^>]*)>(.*?)</script\s*>`)
var scriptSource = regexp.MustCompile(`(?is)\bsrc\s*=\s*["']([^"']+)["']`)

func staticSourceConfiguration(s string) ([]SourceCandidate, []string) {
	candidates := []SourceCandidate{}
	resources := []string{}
	embedded := 0
	scripts := []string{s}
	if strings.Contains(strings.ToLower(s), "<script") {
		scripts = nil
		for _, tag := range scriptTag.FindAllStringSubmatch(s, -1) {
			if src := scriptSource.FindStringSubmatch(tag[1]); src != nil {
				location := html.UnescapeString(src[1])
				if !strings.Contains(location, "swagger-ui-bundle") && !strings.Contains(location, "swagger-ui-standalone-preset") {
					resources = append(resources, location)
				}
			}
			scripts = append(scripts, tag[2])
		}
	}
	for _, script := range scripts {
		tokens := staticTokens(script)
		for i := 0; i+2 < len(tokens); i++ {
			name := tokens[i].text
			if tokens[i+1].text != ":" || (name != "url" && name != "urls" && name != "spec" && name != "configUrl") {
				continue
			}
			index := i + 2
			v, ok := staticValue(tokens, &index, 0)
			// Reject expressions such as url: '/api' + suffix.
			if !ok || index < len(tokens) && tokens[index].text != "," && tokens[index].text != "}" {
				continue
			}
			switch name {
			case "url":
				if location, ok := v.(string); ok {
					candidates = append(candidates, SourceCandidate{Location: location})
				}
			case "configUrl":
				if location, ok := v.(string); ok {
					resources = append(resources, location)
				}
			case "urls":
				if list, ok := v.([]any); ok {
					for _, item := range list {
						m := object(item)
						location, _ := m["url"].(string)
						name, _ := m["name"].(string)
						if location != "" {
							candidates = append(candidates, SourceCandidate{Name: name, Location: location})
						}
					}
				}
			case "spec":
				if m := object(v); m != nil {
					embedded++
					title, _ := object(m["info"])["title"].(string)
					candidates = append(candidates, SourceCandidate{Name: fmt.Sprintf("Embedded specification %d: %s", embedded, title), Spec: m})
				}
			}
			i = index - 1
		}
	}
	return candidates, resources
}
