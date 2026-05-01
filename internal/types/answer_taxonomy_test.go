package types

import (
	"path/filepath"
	"testing"
	"time"
)

func validAnswerPattern(name string) AnswerPattern {
	return AnswerPattern{
		Name:        name,
		Description: "abstract description that exceeds the thirty-char floor for content validation.",
		Trigger:     "fires when X meets Y precondition",
		Confidence:  0.7,
	}
}

func TestAnswerPattern_IsValid(t *testing.T) {
	p := validAnswerPattern("enumeration counts interfaces not implementations")
	if !p.IsValid() {
		t.Fatalf("baseline valid pattern rejected: %+v", p)
	}
	cases := []struct {
		name string
		mut  func(*AnswerPattern)
	}{
		{"empty name", func(p *AnswerPattern) { p.Name = "" }},
		{"too-long name", func(p *AnswerPattern) {
			p.Name = "x"
			for i := 0; i < 100; i++ {
				p.Name += "x"
			}
		}},
		{"too-short description", func(p *AnswerPattern) { p.Description = "short" }},
		{"too-short trigger", func(p *AnswerPattern) { p.Trigger = "X" }},
		{"confidence below floor", func(p *AnswerPattern) { p.Confidence = -0.1 }},
		{"confidence above ceiling", func(p *AnswerPattern) { p.Confidence = 1.1 }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := validAnswerPattern("baseline")
			c.mut(&p)
			if p.IsValid() {
				t.Errorf("expected IsValid=false for %s; got true", c.name)
			}
		})
	}
}

func TestAnswerPattern_IsLoadValid_RequiresID(t *testing.T) {
	p := validAnswerPattern("baseline pattern")
	if p.IsLoadValid() {
		t.Error("IsLoadValid should require ID; got true on a fresh pattern")
	}
	p.ID = "ap-1"
	if !p.IsLoadValid() {
		t.Error("IsLoadValid should accept a valid + ID-bearing pattern")
	}
}

func TestAnswerPattern_SimilarTo_TokenOverlap(t *testing.T) {
	a := validAnswerPattern("enumeration counts interfaces not implementations")
	b := validAnswerPattern("enumeration counts implementations not interfaces")
	if !a.SimilarTo(&b) {
		t.Error("expected SimilarTo=true for high token overlap")
	}
	c := validAnswerPattern("totally unrelated build cache miss")
	if a.SimilarTo(&c) {
		t.Error("expected SimilarTo=false for disjoint patterns")
	}
}

func TestAnswerTaxonomy_PersistRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "answer_taxonomy.json")
	tax := &AnswerTaxonomy{
		RepoSlug:      "test-slug",
		SchemaVersion: CurrentAnswerSchemaVersion,
		Patterns: []AnswerPattern{
			func() AnswerPattern {
				p := validAnswerPattern("test pattern")
				p.ID = "ap-test-1"
				p.FirstSeen = time.Now()
				p.LastSeen = time.Now()
				p.HitCount = 1
				return p
			}(),
		},
	}
	if err := WriteAnswerTaxonomyToFile(tax, path); err != nil {
		t.Fatalf("write: %v", err)
	}
	loaded, err := LoadAnswerTaxonomyFromFile(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded == nil {
		t.Fatal("loaded taxonomy is nil")
	}
	if len(loaded.Patterns) != 1 {
		t.Fatalf("want 1 pattern, got %d", len(loaded.Patterns))
	}
	if loaded.Patterns[0].Name != "test pattern" {
		t.Errorf("name round-trip drifted: %q", loaded.Patterns[0].Name)
	}
}

func TestAnswerTaxonomy_LoadMissingFile_NotError(t *testing.T) {
	loaded, err := LoadAnswerTaxonomyFromFile("/tmp/this-path-does-not-exist-codrax-test.json")
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if loaded != nil {
		t.Error("missing file should return nil taxonomy")
	}
}

func TestAnswerTaxonomy_LoadDropsInvalidPatterns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tax.json")
	bad := AnswerPattern{Name: "no id no body"} // fails IsLoadValid
	good := func() AnswerPattern {
		p := validAnswerPattern("good pattern")
		p.ID = "ap-good"
		p.FirstSeen = time.Now()
		p.LastSeen = time.Now()
		return p
	}()
	tax := &AnswerTaxonomy{
		RepoSlug:      "x",
		SchemaVersion: CurrentAnswerSchemaVersion,
		Patterns:      []AnswerPattern{bad, good},
	}
	if err := WriteAnswerTaxonomyToFile(tax, path); err != nil {
		t.Fatalf("write: %v", err)
	}
	loaded, err := LoadAnswerTaxonomyFromFile(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.Patterns) != 1 {
		t.Fatalf("expected 1 surviving pattern after invalid drop; got %d", len(loaded.Patterns))
	}
}
