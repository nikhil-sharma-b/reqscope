package project

import (
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"go.yaml.in/yaml/v3"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const journalName = ".reqscope-transaction.json"

type Change struct {
	Path             string `json:"path"`
	ExpectedRevision string `json:"expectedRevision"`
	Content          []byte `json:"content"`
}
type transaction struct {
	Version int      `json:"version"`
	Changes []Change `json:"changes"`
}

func Discover(start string) (string, error) {
	p, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(p, "reqscope.yaml")); err == nil {
			return p, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
		p = parent
	}
	return "", Fail("not_found", "no reqscope.yaml found; run reqscope project init or supply --project")
}
func safePath(root, relative string) error {
	path := root
	parts := strings.Split(relative, string(filepath.Separator))
	for _, part := range parts {
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return Fail("invalid_input", "%s is a symlink; project resources must stay within the project", relative)
		}
	}
	return nil
}
func locked(root string, fn func() error) error {
	if err := safePath(root, ".reqscope.lock"); err != nil {
		return err
	}
	if err := safePath(root, journalName); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(root, ".reqscope.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err = unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		return err
	}
	defer unix.Flock(int(f.Fd()), unix.LOCK_UN)
	if err = recoverTransaction(root); err != nil {
		return err
	}
	return fn()
}
func syncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
func atomicWrite(path string, b []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".reqscope-write-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0644); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	return syncDir(dir)
}
func allowedPath(path string) bool {
	if path == "reqscope.yaml" {
		return true
	}
	return filepath.Dir(path) == "environments" && filepath.Ext(path) == ".yaml" && validID(path[len("environments/"):len(path)-len(".yaml")])
}
func validateChanges(changes []Change) error {
	seen := map[string]bool{}
	for _, c := range changes {
		if !allowedPath(c.Path) || seen[c.Path] {
			return Fail("invalid_input", "invalid or duplicate resource path %q", c.Path)
		}
		seen[c.Path] = true
		if c.Path == "reqscope.yaml" {
			var m Manifest
			if err := Decode(c.Content, &m); err != nil {
				return err
			}
			if err := validateManifest(m); err != nil {
				return err
			}
		} else {
			var e Environment
			if err := Decode(c.Content, &e); err != nil {
				return err
			}
			if err := ValidateEnvironment(e); err != nil {
				return err
			}
			if filepath.Base(c.Path) != e.ID+".yaml" {
				return Fail("invalid_input", "environment filename must match its immutable ID")
			}
		}
	}
	return nil
}
func recoverTransaction(root string) error {
	b, err := os.ReadFile(filepath.Join(root, journalName))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var tx transaction
	if err = json.Unmarshal(b, &tx); err != nil {
		return Fail("storage_error", "transaction journal is corrupt; restore it before reopening: %v", err)
	}
	if tx.Version != 1 {
		return Fail("storage_error", "unsupported transaction journal version")
	}
	if err = validateChanges(tx.Changes); err != nil {
		return err
	}
	for _, c := range tx.Changes {
		if err = safePath(root, c.Path); err != nil {
			return err
		}
		current, readErr := os.ReadFile(filepath.Join(root, c.Path))
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return readErr
		}
		revision := ""
		if readErr == nil {
			revision = Digest(current)
		}
		if revision != c.ExpectedRevision && revision != Digest(c.Content) {
			return Fail("revision_conflict", "%s changed during interrupted transaction; reconcile it with %s before retrying", c.Path, journalName)
		}
	}
	for _, c := range tx.Changes {
		if err = atomicWrite(filepath.Join(root, c.Path), c.Content); err != nil {
			return err
		}
	}
	if err = os.Remove(filepath.Join(root, journalName)); err != nil {
		return err
	}
	return syncDir(root)
}
func commit(root string, changes []Change, preview bool) error {
	if err := validateChanges(changes); err != nil {
		return err
	}
	for _, c := range changes {
		if err := safePath(root, c.Path); err != nil {
			return err
		}
		b, err := os.ReadFile(filepath.Join(root, c.Path))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if (err == nil && c.ExpectedRevision != Digest(b)) || (errors.Is(err, os.ErrNotExist) && c.ExpectedRevision != "") {
			return Fail("revision_conflict", "%s changed or already exists; inspect and retry with its current revision", c.Path)
		}
		if err == nil {
			if c.Path == "reqscope.yaml" {
				var old, next Manifest
				if err := Decode(b, &old); err != nil {
					return err
				}
				if err := Decode(c.Content, &next); err != nil {
					return err
				}
				if old.ID != next.ID {
					return Fail("invalid_input", "project ID is immutable")
				}
			} else {
				var old, next Environment
				if err := Decode(b, &old); err != nil {
					return err
				}
				if err := Decode(c.Content, &next); err != nil {
					return err
				}
				if old.ID != next.ID {
					return Fail("invalid_input", "environment ID is immutable")
				}
			}
		}
	}
	if preview {
		return nil
	}
	b, err := json.Marshal(transaction{1, changes})
	if err != nil {
		return err
	}
	if err = atomicWrite(filepath.Join(root, journalName), b); err != nil {
		return err
	}
	return recoverTransaction(root)
}

// Apply validates and commits a batch under one project lock. Empty expected revisions create resources.
func Apply(root string, changes []Change, preview bool) error {
	return locked(root, func() error { return commit(root, changes, preview) })
}
func Init(root, name string, preview bool) (Snapshot, error) {
	if name == "" {
		return Snapshot{}, Fail("invalid_input", "project name is required")
	}
	m := Manifest{FormatVersion: 1, ID: uuid.NewString(), Name: name, Locations: Locations{"specs", "environments", "cases", "scenarios"}, Modules: []string{}, Sources: []string{}}
	s := Snapshot{Manifest: m, Environments: []Resource{}}
	b, _ := yaml.Marshal(m)
	changes := []Change{{Path: "reqscope.yaml", Content: b}}
	s.Revision = Digest(b)
	for _, name := range []string{"local", "staging", "production"} {
		class := "nonproduction"
		if name == "production" {
			class = "production"
		}
		e := Environment{uuid.NewString(), name, class, map[string]string{}, map[string]string{}}
		b, _ := yaml.Marshal(e)
		changes = append(changes, Change{Path: filepath.Join("environments", e.ID+".yaml"), Content: b})
		s.Environments = append(s.Environments, Resource{e, Digest(b)})
	}
	if err := validateChanges(changes); err != nil {
		return Snapshot{}, err
	}
	if preview {
		if _, err := os.Stat(filepath.Join(root, "reqscope.yaml")); err == nil {
			return Snapshot{}, Fail("revision_conflict", "project already exists")
		} else if !errors.Is(err, os.ErrNotExist) {
			return Snapshot{}, err
		}
		return s, nil
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return Snapshot{}, err
	}
	err := locked(root, func() error {
		for _, dir := range []string{"specs", "environments", "cases", "scenarios"} {
			if err := safePath(root, dir); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Join(root, dir), 0755); err != nil {
				return err
			}
		}
		return commit(root, changes, false)
	})
	return s, err
}
func Load(root string) (Snapshot, error) {
	var s Snapshot
	err := locked(root, func() error { var err error; s, err = load(root); return err })
	return s, err
}
func load(root string) (Snapshot, error) {
	var s Snapshot
	if err := safePath(root, "reqscope.yaml"); err != nil {
		return s, err
	}
	if err := safePath(root, "environments"); err != nil {
		return s, err
	}
	b, err := os.ReadFile(filepath.Join(root, "reqscope.yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return s, Fail("not_found", "reqscope.yaml missing; initialize a project")
	}
	if err != nil {
		return s, err
	}
	if err = Decode(b, &s.Manifest); err != nil {
		return s, err
	}
	if err = validateManifest(s.Manifest); err != nil {
		return s, err
	}
	s.Revision = Digest(b)
	s.Environments = []Resource{}
	paths, err := filepath.Glob(filepath.Join(root, "environments", "*.yaml"))
	if err != nil {
		return s, err
	}
	sort.Strings(paths)
	for _, path := range paths {
		if err := safePath(root, filepath.Join("environments", filepath.Base(path))); err != nil {
			return s, err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return s, err
		}
		var e Environment
		if err = Decode(b, &e); err != nil {
			return s, Fail("invalid_input", "%s: %v", path, err)
		}
		if err = ValidateEnvironment(e); err != nil {
			return s, err
		}
		if filepath.Base(path) != e.ID+".yaml" {
			return s, Fail("invalid_input", "%s: filename must match environment UUID", path)
		}
		s.Environments = append(s.Environments, Resource{e, Digest(b)})
	}
	sort.Slice(s.Environments, func(i, j int) bool {
		a, b := s.Environments[i], s.Environments[j]
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.ID < b.ID
	})
	return s, nil
}
