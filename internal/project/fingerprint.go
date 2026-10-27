package project

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Fingerprint holds one change token per resource area, built only from file
// metadata. Adapters compare tokens to decide which areas to read again, so an
// unchanged multi-megabyte specification is never decoded twice.
type Fingerprint struct {
	Manifest     string
	Environments string
	Specs        string
	Cases        string
	Scenarios    string
	// Local covers the per-user access file: credentials, grants and approvals.
	Local string
}

// Fingerprints stats the project files without taking the project lock. A
// token may change spuriously, which only costs a reload; it never hides a
// change, because every write replaces the file and moves its mtime or size.
func Fingerprints(root string) (Fingerprint, error) {
	var f Fingerprint
	manifest := filepath.Join(root, "reqscope.yaml")
	info, err := os.Stat(manifest)
	if err != nil {
		return f, err
	}
	f.Manifest = stamp(info)
	f.Environments = dirStamp(filepath.Join(root, "environments"))
	f.Specs = dirStamp(filepath.Join(root, "specs"))
	f.Cases = dirStamp(filepath.Join(root, "cases"))
	f.Scenarios = dirStamp(filepath.Join(root, "scenarios"))
	b, err := os.ReadFile(manifest)
	if err != nil {
		return f, err
	}
	var m Manifest
	if Decode(b, &m) == nil && m.ID != "" {
		if p, err := localPath(root, m.ID); err == nil {
			if info, err := os.Stat(p); err == nil {
				f.Local = stamp(info)
			}
		}
	}
	return f, nil
}

func stamp(info os.FileInfo) string {
	return fmt.Sprintf("%s:%d:%d", info.Name(), info.Size(), info.ModTime().UnixNano())
}

func dirStamp(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "missing"
	}
	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			// A file replaced mid-listing is reported as changed.
			parts = append(parts, e.Name()+":gone")
			continue
		}
		parts = append(parts, stamp(info))
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}
