package types

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteFailureTaxonomyToFile serialises a FailureTaxonomy to
// disk via the atomic .tmp + rename pattern. Mirror of
// WritePlanGroupToFile — same crash-safety guarantees: a
// process crash mid-write either leaves the previous version
// intact (rename never started) or the new version atomically
// in place (rename completed). Concurrent readers never see
// half-written bytes.
//
// Failure is non-fatal at the call site; a failed taxonomy
// persist is logged + the in-memory store stays authoritative
// for the rest of the Run.
func WriteFailureTaxonomyToFile(t *FailureTaxonomy, path string) error {
	if t == nil {
		return fmt.Errorf("WriteFailureTaxonomyToFile: nil taxonomy")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("WriteFailureTaxonomyToFile: empty path")
	}
	if t.SchemaVersion == 0 {
		t.SchemaVersion = CurrentSchemaVersion
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("WriteFailureTaxonomyToFile: mkdir %s: %w", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("WriteFailureTaxonomyToFile: marshal: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("WriteFailureTaxonomyToFile: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("WriteFailureTaxonomyToFile: rename %s: %w", tmp, err)
	}
	return nil
}

// LoadFailureTaxonomyFromFile reads + parses one
// FailureTaxonomy from disk. Returns (nil, nil) when the file
// doesn't exist (fresh repo, no recorded pitfalls yet) — the
// orchestrator treats that as an empty taxonomy without
// erroring.
//
// Schema-version gate: a file whose SchemaVersion is unknown
// returns an error rather than silently misinterpreting. New
// versions migrate explicitly.
//
// Pattern-validity gate: invalid entries (e.g. truncated
// emit, schema drift from a buggy old run) are skipped at
// load time + logged; a malformed pattern doesn't poison the
// whole taxonomy. The caller gets a usable subset.
func LoadFailureTaxonomyFromFile(path string) (*FailureTaxonomy, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("LoadFailureTaxonomyFromFile: read %s: %w", path, err)
	}
	var t FailureTaxonomy
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("LoadFailureTaxonomyFromFile: parse %s: %w", path, err)
	}
	if t.SchemaVersion != 0 && t.SchemaVersion != CurrentSchemaVersion {
		return nil, fmt.Errorf("LoadFailureTaxonomyFromFile %s: unsupported schema_version %d (this build expects %d); refusing to load", path, t.SchemaVersion, CurrentSchemaVersion)
	}
	if t.SchemaVersion == 0 {
		t.SchemaVersion = CurrentSchemaVersion
	}
	// Filter invalid entries out; preserve the rest.
	if len(t.Patterns) > 0 {
		filtered := t.Patterns[:0]
		for i := range t.Patterns {
			if t.Patterns[i].IsLoadValid() {
				filtered = append(filtered, t.Patterns[i])
			}
		}
		t.Patterns = filtered
	}
	return &t, nil
}
