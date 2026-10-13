package project

import (
	"encoding/json"
	"errors"
	"go.yaml.in/yaml/v3"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// AccessReview is a concrete configuration reviewed by the interactive adapter.
// Bindings contains opaque fingerprints, never credential values.
type AccessReview struct {
	EnvironmentID  string            `json:"environmentID"`
	Classification string            `json:"classification"`
	BaseURLs       map[string]string `json:"baseURLs"`
	Origins        map[string]string `json:"origins"`
	Bindings       map[string]string `json:"bindings"`
	Fingerprint    string            `json:"fingerprint"`
}
type localAccess struct {
	Credentials map[string]map[string]localCredential `json:"credentials,omitempty"`
	Production  map[string]bool                       `json:"production"`
	Grants      map[string]AccessReview               `json:"grants"`
	Bindings    map[string]map[string]string          `json:"bindings"`
	Selected    string                                `json:"selected"`
}

func localPath(root, id string) (string, error) {
	p, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	p, err = filepath.Abs(p)
	if err != nil {
		return "", err
	}
	var dir string
	if runtime.GOOS == "darwin" {
		home, e := os.UserHomeDir()
		if e != nil {
			return "", e
		}
		dir = filepath.Join(home, "Library", "Application Support")
	} else {
		dir = os.Getenv("XDG_DATA_HOME")
		if dir == "" {
			home, e := os.UserHomeDir()
			if e != nil {
				return "", e
			}
			dir = filepath.Join(home, ".local", "share")
		}
	}
	if !filepath.IsAbs(dir) {
		return "", Fail("storage_error", "user data directory must be absolute")
	}
	return filepath.Join(dir, "reqscope", "access", Digest([]byte(id+"\n"+p))+".json"), nil
}
func readAccess(root string, s Snapshot) (localAccess, string, error) {
	a := localAccess{Production: map[string]bool{}, Grants: map[string]AccessReview{}, Bindings: map[string]map[string]string{}}
	p, err := localPath(root, s.Manifest.ID)
	if err != nil {
		return a, p, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return a, p, nil
	}
	if err != nil {
		return a, p, err
	}
	if err = json.Unmarshal(b, &a); err != nil {
		return a, p, Fail("storage_error", "invalid local access state")
	}
	if a.Production == nil || a.Grants == nil || a.Bindings == nil {
		return a, p, Fail("storage_error", "invalid local access state")
	}
	return a, p, nil
}
func writeAccess(p string, a localAccess) error {
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	b, err := json.Marshal(a)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(p), ".access-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), p); err != nil {
		return err
	}
	return syncDir(filepath.Dir(p))
}

// reconcileAccess preserves production protection even after untrusted file edits.
func reconcileAccess(root string, s *Snapshot) error {
	a, p, err := readAccess(root, *s)
	if err != nil {
		return err
	}
	changed := false
	for i := range s.Environments {
		e := &s.Environments[i]
		if e.Classification == "production" && !a.Production[e.ID] {
			a.Production[e.ID] = true
			changed = true
		}
		if a.Production[e.ID] {
			e.Classification = "production"
		}
		if grant, ok := a.Grants[e.ID]; ok {
			review, err := resolvedReview(e.Environment, a.Bindings[e.ID])
			if err != nil || review.Fingerprint != grant.Fingerprint {
				delete(a.Grants, e.ID)
				changed = true
			}
		}
	}
	if changed {
		return writeAccess(p, a)
	}
	return nil
}
func Origin(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u == nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", Fail("invalid_input", "destination must be an absolute HTTP(S) URL without user info or fragment")
	}
	port := u.Port()
	if port == "" {
		port = "80"
		if u.Scheme == "https" {
			port = "443"
		}
	}
	return u.Scheme + "://" + net.JoinHostPort(strings.ToLower(u.Hostname()), port), nil
}
func resolveURL(raw string, vars map[string]string) (string, error) {
	resolved, err := interpolate(raw, vars)
	if err != nil {
		return "", err
	}
	_, err = Origin(resolved)
	return resolved, err
}
func resolvedReview(e Environment, bindings map[string]string) (AccessReview, error) {
	r := AccessReview{EnvironmentID: e.ID, Classification: e.Classification, BaseURLs: map[string]string{}, Origins: map[string]string{}, Bindings: bindings}
	for module, raw := range e.BaseURLs {
		resolved, err := resolveURL(raw, e.Variables)
		if err != nil {
			return r, err
		}
		origin, err := Origin(resolved)
		if err != nil {
			return r, err
		}
		r.BaseURLs[module] = resolved
		r.Origins[module] = origin
	}
	b, _ := json.Marshal(r)
	r.Fingerprint = Digest(b)
	return r, nil
}
func ReviewAccess(root, env string) (AccessReview, error) {
	var r AccessReview
	err := locked(root, func() error {
		s, err := load(root)
		if err != nil {
			return err
		}
		e, err := s.Resolve(env)
		if err != nil {
			return err
		}
		a, _, err := readAccess(root, s)
		if err != nil {
			return err
		}
		r, err = resolvedReview(e.Environment, a.Bindings[e.ID])
		return err
	})
	return r, err
}

// ApproveAccess is for interactive confirmation only; the CLI never exposes it.
func ApproveAccess(root string, review AccessReview) error {
	return locked(root, func() error {
		s, err := load(root)
		if err != nil {
			return err
		}
		e, err := s.Resolve(review.EnvironmentID)
		if err != nil {
			return err
		}
		a, p, err := readAccess(root, s)
		if err != nil {
			return err
		}
		current, err := resolvedReview(e.Environment, a.Bindings[e.ID])
		if err != nil {
			return err
		}
		if current.Fingerprint != review.Fingerprint {
			return Fail("revision_conflict", "destination settings changed; review again")
		}
		if len(current.BaseURLs) == 0 {
			return Fail("invalid_input", "configure a destination before approving CLI access")
		}
		a.Grants[e.ID] = current
		return writeAccess(p, a)
	})
}

// CheckCLIAccess freshly resolves settings. target may be an override or redirect.
func CheckCLIAccess(root, env, module, target string) (AccessReview, error) {
	if strings.TrimSpace(env) == "" {
		return AccessReview{}, Fail("access_denied", "every CLI network run requires explicit --env")
	}
	var r AccessReview
	err := locked(root, func() error {
		s, err := load(root)
		if err != nil {
			return err
		}
		e, err := s.Resolve(env)
		if err != nil {
			return err
		}
		a, _, err := readAccess(root, s)
		if err != nil {
			return err
		}
		r, err = resolvedReview(e.Environment, a.Bindings[e.ID])
		if err != nil {
			return err
		}
		g, ok := a.Grants[e.ID]
		if !ok || g.Fingerprint != r.Fingerprint {
			return Fail("access_denied", "CLI destination access denied; review and authorize in the TUI")
		}
		base, ok := r.BaseURLs[module]
		if !ok {
			return Fail("access_denied", "module destination is unconfigured")
		}
		if target == "" {
			target = base
		}
		origin, err := Origin(target)
		if err != nil {
			return err
		}
		if origin != r.Origins[module] {
			return Fail("access_denied", "destination origin is outside the grant")
		}
		return nil
	})
	return r, err
}
func SelectEnvironment(root, env string) error {
	return locked(root, func() error {
		s, err := load(root)
		if err != nil {
			return err
		}
		e, err := s.Resolve(env)
		if err != nil {
			return err
		}
		a, p, err := readAccess(root, s)
		if err != nil {
			return err
		}
		a.Selected = ""
		if e.Classification != "production" {
			a.Selected = e.ID
		}
		return writeAccess(p, a)
	})
}
func RememberedEnvironment(root string) (Resource, error) {
	var e Resource
	err := locked(root, func() error {
		s, err := load(root)
		if err != nil {
			return err
		}
		a, _, err := readAccess(root, s)
		if err != nil {
			return err
		}
		if a.Selected != "" {
			e, err = s.Resolve(a.Selected)
			if err == nil && e.Classification != "production" {
				return nil
			}
		}
		for _, item := range s.Environments {
			if item.Classification != "production" && len(item.BaseURLs) > 0 {
				e = item
				return nil
			}
		}
		return Fail("not_found", "select an environment")
	})
	return e, err
}

// SaveEnvironment is the ordinary authoring boundary. confirmLower is TUI-only.
func SaveEnvironment(root string, e Environment, revision string, preview, confirmLower bool) error {
	return locked(root, func() error {
		s, err := load(root)
		if err != nil {
			return err
		}
		a, p, err := readAccess(root, s)
		if err != nil {
			return err
		}
		if a.Production[e.ID] && e.Classification != "production" && !confirmLower {
			return Fail("access_denied", "lowering production classification requires TUI confirmation")
		}
		b, err := yaml.Marshal(e)
		if err != nil {
			return err
		}
		if err = commitProtected(root, []Change{{Path: filepath.Join("environments", e.ID+".yaml"), ExpectedRevision: revision, Content: b}}, preview, confirmLower); err != nil {
			return err
		}
		if preview {
			return nil
		}
		if confirmLower && e.Classification != "production" {
			delete(a.Production, e.ID)
			delete(a.Grants, e.ID)
			return writeAccess(p, a)
		}
		return nil
	})
}
