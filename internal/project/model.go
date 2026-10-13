// Package project implements the shared, versioned local workspace operations.
package project

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/google/uuid"
	"go.yaml.in/yaml/v3"
	"io"
	"strings"
)

const FormatVersion = 1

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string                    { return e.Message }
func Fail(code, format string, args ...any) error { return &Error{code, fmt.Sprintf(format, args...)} }

type Locations struct {
	Specs        string `json:"specs" yaml:"specs"`
	Environments string `json:"environments" yaml:"environments"`
	Cases        string `json:"cases" yaml:"cases"`
	Scenarios    string `json:"scenarios" yaml:"scenarios"`
}
type Manifest struct {
	FormatVersion int       `json:"formatVersion" yaml:"formatVersion"`
	ID            string    `json:"id" yaml:"id"`
	Name          string    `json:"name" yaml:"name"`
	Locations     Locations `json:"resources" yaml:"resources"`
	Modules       []string  `json:"modules" yaml:"modules"`
	Sources       []string  `json:"sources" yaml:"sources"`
}
type Environment struct {
	ID             string            `json:"id" yaml:"id"`
	Name           string            `json:"name" yaml:"name"`
	Classification string            `json:"classification" yaml:"classification"`
	BaseURLs       map[string]string `json:"baseURLs" yaml:"baseURLs"`
	Variables      map[string]string `json:"variables" yaml:"variables"`
}
type Resource struct {
	Environment
	Revision string `json:"revision" yaml:"-"`
}
type Snapshot struct {
	Manifest     Manifest   `json:"project"`
	Revision     string     `json:"revision"`
	Environments []Resource `json:"environments"`
}

func Digest(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func Decode(b []byte, v any) error {
	d := yaml.NewDecoder(bytes.NewReader(b))
	d.KnownFields(true)
	if err := d.Decode(v); err != nil {
		return Fail("invalid_input", "invalid JSON/YAML: %v", err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return Fail("invalid_input", "expected exactly one JSON/YAML document")
	}
	return nil
}
func validID(id string) bool { _, err := uuid.Parse(id); return err == nil }
func validateManifest(m Manifest) error {
	if m.FormatVersion != FormatVersion {
		return Fail("unsupported_format", "project format %d is unsupported; use format %d", m.FormatVersion, FormatVersion)
	}
	if !validID(m.ID) || strings.TrimSpace(m.Name) == "" {
		return Fail("invalid_input", "project requires a UUID id and a nonempty name")
	}
	if m.Locations != (Locations{"specs", "environments", "cases", "scenarios"}) {
		return Fail("invalid_input", "resource locations must be specs, environments, cases and scenarios")
	}
	return nil
}
func ValidateEnvironment(e Environment) error {
	if !validID(e.ID) || strings.TrimSpace(e.Name) == "" {
		return Fail("invalid_input", "environment requires a UUID id and a nonempty name")
	}
	if e.Classification != "nonproduction" && e.Classification != "production" {
		return Fail("invalid_input", "classification must be nonproduction or production")
	}
	if e.BaseURLs == nil || e.Variables == nil {
		return Fail("invalid_input", "baseURLs and variables must be objects")
	}
	return nil
}
func (s Snapshot) Resolve(name string) (Resource, error) {
	for _, e := range s.Environments {
		if e.ID == name {
			return e, nil
		}
	}
	var matches []Resource
	for _, e := range s.Environments {
		if e.Name == name {
			matches = append(matches, e)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return Resource{}, Fail("ambiguous_resource", "environment %q is ambiguous; select its UUID", name)
	}
	return Resource{}, Fail("not_found", "environment %q not found; list environments to select a UUID", name)
}
