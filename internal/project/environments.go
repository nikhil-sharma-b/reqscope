package project

import "path/filepath"

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
		a, path, e := readAccess(root, s)
		if e != nil {
			return e
		}
		delete(a.Grants, env.ID)
		delete(a.Extractions, env.ID)
		if a.Selected == env.ID {
			a.Selected = ""
		}
		// Revoke before publishing deletion so a crash fails closed.
		if e = writeAccess(path, a); e != nil {
			return e
		}
		return commit(root, changes, false)
	})
	return
}
