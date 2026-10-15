package project

import (
	"path/filepath"
	"strings"
)

// DeleteEnvironment removes ordinary settings. Local credential values and
// production protection survive; recreating the ID cannot restore CLI approval.
func DeleteEnvironment(root, selector, revision string, preview bool) (result map[string]any, err error) {
	err = locked(root, func() error {
		s, e := load(root)
		if e != nil {
			return e
		}
		env, e := s.Resolve(selector)
		if e != nil {
			return e
		}
		if revision == "" {
			return Fail("invalid_input", "delete requires expected revision")
		}
		changes := []Change{{Path: filepath.Join("environments", env.ID+".yaml"), ExpectedRevision: revision, Delete: true}}
		if e = commit(root, changes, true); e != nil {
			return e
		}
		result = map[string]any{"id": env.ID, "preview": preview, "references": []string{}}
		if preview {
			return nil
		}
		return commit(root, changes, false)
	})
	return
}

// Called under the project lock, after revision validation and before publishing
// any deletion journal. All writers, including batch Apply, revoke first.
func revokeDeletedEnvironmentAccess(root string, changes []Change) error {
	var ids []string
	for _, change := range changes {
		if change.Delete && filepath.Dir(change.Path) == "environments" {
			ids = append(ids, strings.TrimSuffix(filepath.Base(change.Path), ".yaml"))
		}
	}
	if len(ids) == 0 {
		return nil
	}
	s, err := load(root)
	if err != nil {
		return err
	}
	a, path, err := readAccess(root, s)
	if err != nil {
		return err
	}
	for _, id := range ids {
		delete(a.Grants, id)
		delete(a.Extractions, id)
		if a.Selected == id {
			a.Selected = ""
		}
	}
	return writeAccess(path, a)
}
