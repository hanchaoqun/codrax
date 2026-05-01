package types

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFailurePattern_IsValid(t *testing.T) {
	good := &FailurePattern{
		ID:          "fp-1-2",
		Name:        "lock-scope drift on new public method",
		Description: "When a new exported method is added to a struct that several goroutines reach into, the existing mutex coverage tends to leave the new method unguarded.",
		Trigger:     "adding a method to a goroutine-shared struct",
		Confidence:  0.8,
	}
	if !good.IsValid() {
		t.Errorf("good pattern should be valid; got %+v", good)
	}
	for _, bad := range []struct {
		name string
		p    *FailurePattern
	}{
		{"nil", nil},
		{"empty name", &FailurePattern{ID: "fp-1", Description: strings.Repeat("a", 50), Trigger: strings.Repeat("b", 30), Confidence: 0.5}},
		{"name too long", &FailurePattern{ID: "fp-1", Name: strings.Repeat("x", 200), Description: strings.Repeat("a", 50), Trigger: strings.Repeat("b", 30), Confidence: 0.5}},
		{"description too short", &FailurePattern{ID: "fp-1", Name: "x", Description: "tiny", Trigger: strings.Repeat("b", 30), Confidence: 0.5}},
		{"trigger too short", &FailurePattern{ID: "fp-1", Name: "x", Description: strings.Repeat("a", 50), Trigger: "x", Confidence: 0.5}},
		{"confidence < 0", &FailurePattern{ID: "fp-1", Name: "x", Description: strings.Repeat("a", 50), Trigger: strings.Repeat("b", 30), Confidence: -0.1}},
		{"confidence > 1", &FailurePattern{ID: "fp-1", Name: "x", Description: strings.Repeat("a", 50), Trigger: strings.Repeat("b", 30), Confidence: 1.5}},
	} {
		if bad.p.IsValid() {
			t.Errorf("%s should be invalid; got valid", bad.name)
		}
	}

	// IsLoadValid additionally requires ID — pre-Append
	// validity (no ID assigned yet) passes IsValid but should
	// fail IsLoadValid.
	noID := &FailurePattern{
		Name: "x", Description: strings.Repeat("a", 50),
		Trigger: strings.Repeat("b", 30), Confidence: 0.5,
	}
	if !noID.IsValid() {
		t.Errorf("emit-time-valid pattern (no ID) should be IsValid; got false")
	}
	if noID.IsLoadValid() {
		t.Errorf("no-ID pattern must NOT be IsLoadValid; got true")
	}
}

func TestFailurePattern_SimilarTo(t *testing.T) {
	a := &FailurePattern{Name: "lock scope drift on public method"}
	b := &FailurePattern{Name: "scope drift on public method add"}
	if !a.SimilarTo(b) {
		t.Errorf("expected similar (3+ shared significant tokens); got not similar")
	}
	c := &FailurePattern{Name: "test fixture imports rot when production widens"}
	if a.SimilarTo(c) {
		t.Errorf("unrelated names should NOT be similar; got similar")
	}
	// Stopwords don't drive similarity.
	d := &FailurePattern{Name: "the and or for"}
	e := &FailurePattern{Name: "and or for the"}
	if d.SimilarTo(e) {
		t.Errorf("stopword-only names should NOT match; got similar")
	}
}

func TestWriteAndLoadFailureTaxonomy_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ft.json")
	now := time.Now()
	tax := &FailureTaxonomy{
		RepoSlug: "demo-12345678",
		Patterns: []FailurePattern{{
			ID:          "fp-1",
			Name:        "lock-scope drift on new public method",
			Description: "When a new exported method is added to a goroutine-shared struct, mutex coverage often misses it.",
			Trigger:     "adding a method to a shared struct",
			Confidence:  0.7,
			FirstSeen:   now,
			LastSeen:    now,
			HitCount:    2,
		}},
	}
	if err := WriteFailureTaxonomyToFile(tax, path); err != nil {
		t.Fatalf("Write: %v", err)
	}
	loaded, err := LoadFailureTaxonomyFromFile(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.RepoSlug != "demo-12345678" {
		t.Errorf("RepoSlug drift; got %q", loaded.RepoSlug)
	}
	if len(loaded.Patterns) != 1 || loaded.Patterns[0].ID != "fp-1" {
		t.Errorf("Pattern round-trip drift; got %+v", loaded.Patterns)
	}
	if loaded.SchemaVersion != CurrentSchemaVersion {
		t.Errorf("SchemaVersion should auto-fill; got %d", loaded.SchemaVersion)
	}
}

func TestLoadFailureTaxonomy_MissingFileReturnsNil(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.json")
	tax, err := LoadFailureTaxonomyFromFile(path)
	if err != nil {
		t.Errorf("missing file should return (nil, nil); got err=%v", err)
	}
	if tax != nil {
		t.Errorf("missing file should return nil taxonomy; got %+v", tax)
	}
}

func TestLoadFailureTaxonomy_RefusesUnknownSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ft.json")
	// Forge a future-version file.
	bad := &FailureTaxonomy{SchemaVersion: 999}
	if err := WriteFailureTaxonomyToFile(bad, path); err != nil {
		t.Fatalf("Write: %v", err)
	}
	_, err := LoadFailureTaxonomyFromFile(path)
	if err == nil {
		t.Fatal("future-schema file should error on load")
	}
	if !strings.Contains(err.Error(), "schema_version") {
		t.Errorf("expected 'schema_version' in err; got %v", err)
	}
}

func TestLoadFailureTaxonomy_FiltersInvalidEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ft.json")
	now := time.Now()
	tax := &FailureTaxonomy{
		RepoSlug:      "demo",
		SchemaVersion: CurrentSchemaVersion,
		Patterns: []FailurePattern{
			{
				ID: "fp-good", Name: "good pattern with proper length",
				Description: strings.Repeat("a", 50), Trigger: strings.Repeat("b", 20),
				Confidence: 0.7, LastSeen: now,
			},
			// Invalid: empty trigger.
			{ID: "fp-bad", Name: "bad pattern", Description: strings.Repeat("a", 50), Confidence: 0.5},
		},
	}
	if err := WriteFailureTaxonomyToFile(tax, path); err != nil {
		t.Fatalf("Write: %v", err)
	}
	loaded, err := LoadFailureTaxonomyFromFile(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Patterns) != 1 {
		t.Errorf("expected only 1 valid pattern after load filter; got %d", len(loaded.Patterns))
	}
	if loaded.Patterns[0].ID != "fp-good" {
		t.Errorf("kept the wrong pattern; got %s", loaded.Patterns[0].ID)
	}
}
