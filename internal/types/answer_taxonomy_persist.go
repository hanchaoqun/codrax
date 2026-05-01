package types

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteAnswerTaxonomyToFile serialises an AnswerTaxonomy to disk
// via the atomic .tmp + rename pattern. Mirror of
// WriteFailureTaxonomyToFile — same crash-safety guarantees.
//
// Failure is non-fatal at the call site; a failed taxonomy
// persist is logged + the in-memory store stays authoritative for
// the rest of the Run.
func WriteAnswerTaxonomyToFile(t *AnswerTaxonomy, path string) error {
	if t == nil {
		return fmt.Errorf("WriteAnswerTaxonomyToFile: nil taxonomy")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("WriteAnswerTaxonomyToFile: empty path")
	}
	if t.SchemaVersion == 0 {
		t.SchemaVersion = CurrentAnswerSchemaVersion
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("WriteAnswerTaxonomyToFile: mkdir %s: %w", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("WriteAnswerTaxonomyToFile: marshal: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("WriteAnswerTaxonomyToFile: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("WriteAnswerTaxonomyToFile: rename %s: %w", tmp, err)
	}
	return nil
}

// LoadAnswerTaxonomyFromFile reads + parses one AnswerTaxonomy
// from disk. Returns (nil, nil) when the file doesn't exist (fresh
// repo, no recorded pitfalls yet) — the orchestrator treats that
// as an empty taxonomy without erroring.
//
// Schema-version gate: a file whose SchemaVersion is unknown
// returns an error rather than silently misinterpreting.
//
// Pattern-validity gate: invalid entries are skipped at load time
// + warned to stderr; a malformed pattern doesn't poison the whole
// taxonomy.
func LoadAnswerTaxonomyFromFile(path string) (*AnswerTaxonomy, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("LoadAnswerTaxonomyFromFile: read %s: %w", path, err)
	}
	var t AnswerTaxonomy
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("LoadAnswerTaxonomyFromFile: parse %s: %w", path, err)
	}
	if t.SchemaVersion != 0 && t.SchemaVersion != CurrentAnswerSchemaVersion {
		return nil, fmt.Errorf("LoadAnswerTaxonomyFromFile %s: unsupported schema_version %d (this build expects %d); refusing to load", path, t.SchemaVersion, CurrentAnswerSchemaVersion)
	}
	if t.SchemaVersion == 0 {
		t.SchemaVersion = CurrentAnswerSchemaVersion
	}
	if len(t.Patterns) > 0 {
		before := len(t.Patterns)
		filtered := t.Patterns[:0]
		for i := range t.Patterns {
			if t.Patterns[i].IsLoadValid() {
				filtered = append(filtered, t.Patterns[i])
			}
		}
		t.Patterns = filtered
		if dropped := before - len(filtered); dropped > 0 {
			fmt.Fprintf(os.Stderr,
				"[answer_taxonomy] WARN: %s dropped %d invalid pattern(s) at load time (schema drift or truncated emit?)\n",
				path, dropped)
		}
	}
	return &t, nil
}
