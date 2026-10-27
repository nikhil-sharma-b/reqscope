package tui

import (
	"os"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/nikhil-sharma-b/reqscope/internal/project"
)

// Every action is one key press. Actions belong to scopes, and the screen in
// front of the person decides which scopes are live, so the same letter can
// mean "delete" in a list and nothing at all in a text field. Esc (back or
// cancel) and Ctrl+C (cancel or quit) are fixed so there is always a way out.
type action string

type scope int

const (
	scopeGlobal scope = iota
	scopeNav
	scopeBrowse
	scopeEndpoints
	scopeSaved
	scopeScenarios
	scopeHistory
	scopeEnvs
	scopeRequest
	scopeForm
	scopeResponse
	scopeCommand
)

var scopeNames = map[scope]string{
	scopeGlobal: "Everywhere", scopeNav: "Moving around", scopeBrowse: "Lists",
	scopeEndpoints: "Endpoints", scopeSaved: "Saved requests", scopeScenarios: "Scenarios",
	scopeHistory: "History", scopeEnvs: "Environments", scopeRequest: "Request",
	scopeForm: "Request fields", scopeResponse: "Response", scopeCommand: "Commands",
}

const (
	actQuit, actHelp, actPalette, actEnvironment                     action = "quit", "help", "commands", "environment"
	actTab1, actTab2, actTab3, actTab4, actTab5                      action = "endpoints", "saved", "scenarios", "history", "environments"
	actUp, actDown, actTop, actBottom, actPageDown, actPageUp        action = "up", "down", "top", "bottom", "page-down", "page-up"
	actNextTab, actPrevTab, actFilter, actOpen                       action = "next-tab", "previous-tab", "filter", "open"
	actNewRequest, actImport, actMethod, actGroup, actSchemas        action = "new-request", "import", "method-filter", "group-by-tag", "schemas"
	actContract, actRefreshSource                                    action = "view-contract", "refresh-source"
	actRunCase, actDuplicateCase, actDeleteCase, actEditCase         action = "run-case", "duplicate-case", "delete-case", "edit-case"
	actRunScenario, actNewScenario, actDeleteScenario                action = "run-scenario", "new-scenario", "delete-scenario"
	actCheckScenario, actApproveLogins                               action = "check-scenario", "approve-logins"
	actPin, actDeleteRun, actCompare                                 action = "pin", "delete-run", "compare"
	actUseEnv, actNewEnv, actDeleteEnv, actAuthorize, actCredentials action = "use-environment", "new-environment", "delete-environment", "allow-cli", "credentials"
	actSend, actSave, actEditRaw                                     action = "send", "save", "edit-json"
	actEdit, actAdd, actRemove, actMethodCycle, actBody, actBodyType action = "edit", "add", "remove", "method", "body", "body-type"
	actAttach, actFind, actCopy, actHeaders                          action = "attach", "find", "copy", "headers"
	actRenameProject, actValidateProject, actReloadProject           action = "rename-project", "check-project", "reload-project"
	actEditContract, actExportContract, actCompareIDs                action = "edit-contract", "export-contract", "compare-ids"
	actRetention, actPrune, actAttachRun, actReloadCase              action = "history-retention", "prune-history", "attach-run", "reload-case"
	actClearRequest, actCancel, actSaveAsNew                         action = "clear-request", "cancel", "save-as-new"
	actZoom, actPaneLeft, actPaneRight                               action = "zoom", "pane-left", "pane-right"
)

type actionDef struct {
	id     action
	label  string // footer wording, two words at most
	desc   string // help and command wording
	keys   []string
	scopes []scope
	footer int // position in the footer; 0 keeps it out
}

func actionTable() []actionDef {
	g, n, b := []scope{scopeGlobal}, []scope{scopeNav}, []scope{scopeBrowse}
	return []actionDef{
		{actQuit, "quit", "Quit", []string{"q"}, g, 0},
		{actHelp, "keys", "Show the keys for this screen", []string{"?"}, g, 99},
		{actPalette, "commands", "Search every command", []string{":", "ctrl+p"}, g, 98},
		{actEnvironment, "env", "Switch environment", []string{"e"}, g, 90},
		{actTab1, "", "Go to endpoints", []string{"1"}, g, 0},
		{actTab2, "", "Go to saved requests", []string{"2"}, g, 0},
		{actTab3, "", "Go to scenarios", []string{"3"}, g, 0},
		{actTab4, "", "Go to history", []string{"4"}, g, 0},
		{actTab5, "", "Go to environments", []string{"5"}, g, 0},

		{actUp, "", "Move up", []string{"k", "up"}, n, 0},
		{actDown, "", "Move down", []string{"j", "down"}, n, 0},
		{actTop, "", "Jump to the top", []string{"g", "home"}, n, 0},
		{actBottom, "", "Jump to the bottom", []string{"G", "end"}, n, 0},
		{actPageDown, "", "Page down", []string{"ctrl+d", "pgdown"}, n, 0},
		{actPageUp, "", "Page up", []string{"ctrl+u", "pgup"}, n, 0},

		{actNextTab, "", "Next tab", []string{"tab"}, g, 0},
		{actPrevTab, "", "Previous tab", []string{"shift+tab"}, g, 0},
		{actOpen, "open", "Open the selection", []string{"enter"}, b, 1},
		{actFilter, "filter", "Filter the list", []string{"/"}, b, 2},
		{actPaneLeft, "", "Move to the left pane: the list, or the request", []string{"h", "left"}, []scope{scopeBrowse, scopeRequest}, 0},
		{actPaneRight, "", "Move to the right pane: the details, or the response", []string{"l", "right"}, []scope{scopeBrowse, scopeRequest}, 0},
		{actZoom, "zoom", "Show the focused pane full screen, or the split again", []string{"z"}, []scope{scopeBrowse, scopeRequest}, 0},

		{actImport, "import", "Import an OpenAPI file or Swagger URL", []string{"i"}, []scope{scopeEndpoints}, 3},
		{actContract, "contract", "View the full contract of the selection", []string{"v"}, []scope{scopeEndpoints}, 4},
		{actNewRequest, "new", "Start a blank request", []string{"n"}, []scope{scopeEndpoints, scopeSaved}, 5},
		{actMethod, "method", "Show one HTTP method at a time", []string{"m"}, []scope{scopeEndpoints}, 6},
		{actGroup, "group", "Group endpoints by tag", []string{"t"}, []scope{scopeEndpoints}, 7},
		{actSchemas, "schemas", "Switch between endpoints and schemas", []string{"s"}, []scope{scopeEndpoints}, 8},
		{actRefreshSource, "refresh", "Re-read the API source and review the changes", []string{"R"}, []scope{scopeEndpoints}, 0},

		{actRunCase, "run", "Send the saved request", []string{"r"}, []scope{scopeSaved}, 2},
		{actEditCase, "details", "Edit the name, checks and comparison rules", []string{"E"}, []scope{scopeSaved}, 3},
		{actDuplicateCase, "copy", "Duplicate the saved request", []string{"c"}, []scope{scopeSaved}, 4},
		{actDeleteCase, "delete", "Delete the saved request", []string{"d"}, []scope{scopeSaved}, 6},

		{actRunScenario, "run", "Run the scenario", []string{"r"}, []scope{scopeScenarios}, 2},
		{actNewScenario, "new", "Create a scenario", []string{"n"}, []scope{scopeScenarios}, 3},
		{actCheckScenario, "check", "Check the scenario without sending anything", []string{"v"}, []scope{scopeScenarios}, 4},
		{actApproveLogins, "approve logins", "Review protected login values the CLI may use", []string{"a"}, []scope{scopeScenarios}, 5},
		{actDeleteScenario, "delete", "Delete the scenario", []string{"d"}, []scope{scopeScenarios}, 6},

		{actPin, "pin", "Pin or unpin the run", []string{"p"}, []scope{scopeHistory}, 2},
		{actCompare, "compare", "Compare runs: press on one, then on another", []string{"c"}, []scope{scopeHistory}, 3},
		{actDeleteRun, "delete", "Delete the run", []string{"d"}, []scope{scopeHistory}, 4},

		{actUseEnv, "use", "Send requests to this environment", []string{"u"}, []scope{scopeEnvs}, 2},
		{actNewEnv, "new", "Create an environment", []string{"n"}, []scope{scopeEnvs}, 3},
		{actCredentials, "credentials", "Manage stored credentials", []string{"c"}, []scope{scopeEnvs}, 4},
		{actAuthorize, "allow CLI", "Review and allow CLI access", []string{"a"}, []scope{scopeEnvs}, 5},
		{actDeleteEnv, "delete", "Delete the environment", []string{"d"}, []scope{scopeEnvs}, 6},

		{actSend, "send", "Send the request", []string{"s"}, []scope{scopeRequest}, 1},
		{actSave, "save", "Save the request", []string{"ctrl+s"}, []scope{scopeRequest}, 7},
		{actEditRaw, "edit JSON", "Edit the whole request as JSON in your editor", []string{"E"}, []scope{scopeRequest}, 0},

		{actEdit, "edit", "Edit the selected field", []string{"enter"}, []scope{scopeForm}, 2},
		{actAdd, "add", "Add a row to this section", []string{"a"}, []scope{scopeForm}, 3},
		{actRemove, "remove", "Remove the selected row", []string{"d"}, []scope{scopeForm}, 4},
		{actBody, "body", "Edit the body in your editor", []string{"b"}, []scope{scopeForm}, 5},
		{actMethodCycle, "method", "Change the HTTP method", []string{"m"}, []scope{scopeForm}, 6},
		{actBodyType, "body type", "Change the body type", []string{"t"}, []scope{scopeForm}, 0},
		{actAttach, "attach", "Attach a file to the body", []string{"f"}, []scope{scopeForm}, 0},

		{actFind, "find", "Find text in the response", []string{"/"}, []scope{scopeResponse}, 2},
		{actCopy, "copy", "Copy the response body", []string{"y"}, []scope{scopeResponse}, 3},
		{actHeaders, "headers", "Show or hide response headers", []string{"H"}, []scope{scopeResponse}, 4},

		{actRenameProject, "", "Rename the project", nil, []scope{scopeCommand}, 0},
		{actValidateProject, "", "Check the project files", nil, []scope{scopeCommand}, 0},
		{actReloadProject, "", "Reload the project files", nil, []scope{scopeCommand}, 0},
		{actEditContract, "", "Add or change an endpoint or schema in the contract", nil, []scope{scopeCommand}, 0},
		{actExportContract, "", "Copy the contract documents to the clipboard", nil, []scope{scopeCommand}, 0},
		{actCompareIDs, "", "Compare two runs by ID", nil, []scope{scopeCommand}, 0},
		{actRetention, "", "Choose how much history to keep", nil, []scope{scopeCommand}, 0},
		{actPrune, "", "Remove unpinned runs beyond the history limits", nil, []scope{scopeCommand}, 0},
		{actAttachRun, "", "Attach the selected run to the open saved request", nil, []scope{scopeCommand}, 0},
		{actReloadCase, "", "Discard edits and reload the saved request", nil, []scope{scopeCommand}, 0},
		{actSaveAsNew, "", "Save the request as a new saved request", nil, []scope{scopeCommand}, 0},
		{actClearRequest, "", "Clear the request", nil, []scope{scopeCommand}, 0},
		{actCancel, "", "Cancel the running request or scenario", nil, []scope{scopeCommand}, 0},
	}
}

// contexts lists the scopes live on each screen, most specific first.
var contexts = map[string][]scope{
	"endpoints":    {scopeEndpoints, scopeBrowse, scopeNav, scopeGlobal},
	"saved":        {scopeSaved, scopeBrowse, scopeNav, scopeGlobal},
	"scenarios":    {scopeScenarios, scopeBrowse, scopeNav, scopeGlobal},
	"history":      {scopeHistory, scopeBrowse, scopeNav, scopeGlobal},
	"environments": {scopeEnvs, scopeBrowse, scopeNav, scopeGlobal},
	"form":         {scopeForm, scopeRequest, scopeNav, scopeGlobal},
	"response":     {scopeResponse, scopeRequest, scopeNav, scopeGlobal},
}

// KeyConfig is the optional keys.yaml: the leader key that opens the key menu
// and action names mapped to key lists. An empty list unbinds an action; it
// stays available from the key menu and the command search.
type KeyConfig struct {
	Leader   string              `yaml:"leader"`
	Bindings map[string][]string `yaml:"bindings"`
}

// Binding is one action with its effective keys.
type Binding struct {
	Action      string
	Description string
	Keys        []string
}

type keymap struct {
	// leader opens the key menu; space unless keys.yaml says otherwise.
	leader string
	defs   []actionDef
	byID   map[action]int
	// lookup resolves a key within one scope.
	lookup map[scope]map[string]action
}

// Bindings validates a key configuration and returns the effective bindings.
func Bindings(c KeyConfig) ([]Binding, error) {
	k, err := newKeymap(c)
	if err != nil {
		return nil, err
	}
	out := make([]Binding, 0, len(k.defs))
	for _, d := range k.defs {
		out = append(out, Binding{string(d.id), d.desc, append([]string(nil), d.keys...)})
	}
	return out, nil
}

func newKeymap(c KeyConfig) (*keymap, error) {
	leader := strings.TrimSpace(c.Leader)
	if leader == "" {
		leader = "space"
	}
	if strings.Contains(leader, " ") || leader == "esc" || leader == "ctrl+c" || !validKey(leader) {
		return nil, project.Fail("invalid_input", "keys.yaml: leader %q must be a single key other than esc and ctrl+c", c.Leader)
	}
	k := &keymap{leader: leader, defs: actionTable(), byID: map[action]int{}, lookup: map[scope]map[string]action{}}
	for i, d := range k.defs {
		k.byID[d.id] = i
	}
	names := make([]string, 0, len(c.Bindings))
	for name := range c.Bindings {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		i, ok := k.byID[action(name)]
		if !ok {
			return nil, project.Fail("invalid_input", "keys.yaml: unknown action %q", name)
		}
		keys := []string{}
		for _, key := range c.Bindings[name] {
			key = strings.TrimSpace(key)
			if key == " " || key == "" {
				key = "space"
			}
			if strings.Contains(key, " ") {
				return nil, project.Fail("invalid_input", "keys.yaml: %q for %s is a sequence; bind a single key", key, name)
			}
			if key == "esc" || key == "ctrl+c" {
				return nil, project.Fail("invalid_input", "keys.yaml: %s is reserved for going back and cancelling", key)
			}
			if !validKey(key) {
				return nil, project.Fail("invalid_input", "keys.yaml: unsupported key %q for %s", key, name)
			}
			keys = append(keys, key)
		}
		k.defs[i].keys = keys
	}
	for _, d := range k.defs {
		for _, s := range d.scopes {
			if k.lookup[s] == nil {
				k.lookup[s] = map[string]action{}
			}
			for _, key := range d.keys {
				if other, ok := k.lookup[s][key]; ok {
					return nil, project.Fail("invalid_input", "keys.yaml: %s is bound to both %s and %s", key, other, d.id)
				}
				k.lookup[s][key] = d.id
			}
		}
	}
	for s, keys := range k.lookup {
		if id, ok := keys[leader]; ok {
			return nil, project.Fail("invalid_input", "keys.yaml: %s is the leader but is also bound to %s in %s", leader, id, scopeNames[s])
		}
	}
	// A key must mean one thing on every screen where it is live.
	for name, chain := range contexts {
		seen := map[string]action{}
		for _, s := range chain {
			for key, id := range k.lookup[s] {
				if other, ok := seen[key]; ok && other != id {
					return nil, project.Fail("invalid_input", "keys.yaml: %s means both %s and %s on the %s screen", key, other, id, name)
				}
				seen[key] = id
			}
		}
	}
	return k, nil
}

func validKey(s string) bool {
	if utf8.RuneCountInString(s) == 1 {
		r, _ := utf8.DecodeRuneInString(s)
		return unicode.IsPrint(r) && !unicode.IsSpace(r)
	}
	switch s {
	case "space", "enter", "tab", "shift+tab", "backspace", "delete", "up", "down", "left", "right", "home", "end", "pgup", "pgdown":
		return true
	}
	for _, prefix := range []string{"ctrl+", "alt+"} {
		if rest, ok := strings.CutPrefix(s, prefix); ok && utf8.RuneCountInString(rest) == 1 {
			r, _ := utf8.DecodeRuneInString(rest)
			return unicode.IsPrint(r) && !unicode.IsSpace(r)
		}
	}
	if rest, ok := strings.CutPrefix(s, "f"); ok && len(rest) <= 2 && rest != "" && strings.Trim(rest, "0123456789") == "" {
		return true
	}
	return false
}

func (k *keymap) resolve(chain []scope, key string) (action, bool) {
	for _, s := range chain {
		if id, ok := k.lookup[s][key]; ok {
			return id, true
		}
	}
	return "", false
}

func (k *keymap) def(id action) actionDef { return k.defs[k.byID[id]] }

// key is the first key for an action, as the footer shows it.
func (k *keymap) key(id action) string {
	if d := k.def(id); len(d.keys) > 0 {
		return d.keys[0]
	}
	return ""
}

func loadKeys(path string) (KeyConfig, error) {
	var c KeyConfig
	if path == "" {
		return c, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	err = project.Decode(b, &c)
	return c, err
}
