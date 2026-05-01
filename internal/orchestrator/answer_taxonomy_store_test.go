package orchestrator

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func validPattern(name string) *types.AnswerPattern {
	return &types.AnswerPattern{
		Name:        name,
		Description: "abstract description that exceeds the thirty-char floor for content validation.",
		Trigger:     "fires when X meets Y precondition",
		Confidence:  0.7,
	}
}

func TestAnswerTaxonomyStore_DisabledIsNoOp(t *testing.T) {
	s := NewAnswerTaxonomyStore("", "", 50, 90)
	if s.Enabled() {
		t.Fatal("empty cacheDir+repoSlug must yield disabled store")
	}
	got, err := s.Append(validPattern("x"))
	if err != nil {
		t.Fatalf("Append on disabled store should not error: %v", err)
	}
	if got != nil {
		t.Error("disabled Append should return nil")
	}
	if s.All() != nil {
		t.Error("disabled All should return nil")
	}
	if err := s.Clear(); err != nil {
		t.Errorf("disabled Clear should be no-op: %v", err)
	}
}

func TestAnswerTaxonomyStore_AppendThenLoad(t *testing.T) {
	dir := t.TempDir()
	s := NewAnswerTaxonomyStore(dir, "slug", 50, 90)
	if !s.Enabled() {
		t.Fatal("expected enabled store")
	}
	if _, err := s.Append(validPattern("first pattern alpha")); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if _, err := s.Append(validPattern("second different gamma delta")); err != nil {
		t.Fatalf("Append: %v", err)
	}
	all := s.All()
	if len(all) != 2 {
		t.Fatalf("want 2 patterns, got %d", len(all))
	}

	// Independent fresh store reading the same file should see both.
	s2 := NewAnswerTaxonomyStore(dir, "slug", 50, 90)
	if got := len(s2.All()); got != 2 {
		t.Errorf("fresh-store load expected 2; got %d", got)
	}
	// Path sanity.
	expectedPath := filepath.Join(dir, "slug", "answer_taxonomy.json")
	if s.taxonomyPath() != expectedPath {
		t.Errorf("path drift: got %q want %q", s.taxonomyPath(), expectedPath)
	}
}

func TestAnswerTaxonomyStore_AppendDedupBumpsHitCount(t *testing.T) {
	dir := t.TempDir()
	s := NewAnswerTaxonomyStore(dir, "slug", 50, 90)
	if _, err := s.Append(validPattern("enumeration counts interfaces")); err != nil {
		t.Fatalf("Append: %v", err)
	}
	// SimilarTo should match — high token overlap with first.
	if _, err := s.Append(validPattern("enumeration counts implementations")); err != nil {
		t.Fatalf("Append: %v", err)
	}
	all := s.All()
	if len(all) != 1 {
		t.Fatalf("dedup failed; want 1 pattern, got %d", len(all))
	}
	if all[0].HitCount < 2 {
		t.Errorf("HitCount should be bumped on dedup; got %d", all[0].HitCount)
	}
}

func TestAnswerTaxonomyStore_RelevantTo_FilterByScenario(t *testing.T) {
	dir := t.TempDir()
	s := NewAnswerTaxonomyStore(dir, "slug", 50, 90)
	scenarioOnly := validPattern("scoped explanation alpha")
	scenarioOnly.AppliesToScenarios = []string{"explain"}
	if _, err := s.Append(scenarioOnly); err != nil {
		t.Fatal(err)
	}
	universal := validPattern("broadly applicable beta gamma")
	if _, err := s.Append(universal); err != nil {
		t.Fatal(err)
	}

	// scenario=explain should yield BOTH.
	got := s.RelevantTo("explain", "", 5)
	if len(got) != 2 {
		t.Errorf("scenario=explain: want 2 patterns, got %d", len(got))
	}

	// scenario=count should yield ONLY universal (scenarioOnly filtered).
	got = s.RelevantTo("count", "", 5)
	if len(got) != 1 {
		t.Errorf("scenario=count: want 1 pattern, got %d", len(got))
	}
	if len(got) > 0 && got[0].Name != "broadly applicable beta gamma" {
		t.Errorf("scenario=count: expected broadly applicable; got %q", got[0].Name)
	}
}

func TestAnswerTaxonomyStore_Clear_WipesDisk(t *testing.T) {
	dir := t.TempDir()
	s := NewAnswerTaxonomyStore(dir, "slug", 50, 90)
	if _, err := s.Append(validPattern("temp pattern alpha")); err != nil {
		t.Fatal(err)
	}
	if len(s.All()) != 1 {
		t.Fatal("setup failed")
	}
	if err := s.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if len(s.All()) != 0 {
		t.Errorf("Clear should empty in-memory list; got %d", len(s.All()))
	}
	// File should be gone too.
	s2 := NewAnswerTaxonomyStore(dir, "slug", 50, 90)
	if got := len(s2.All()); got != 0 {
		t.Errorf("fresh-store after Clear should see 0 patterns; got %d", got)
	}
}

func TestAnswerTaxonomyStore_RelevantTo_EmptyScenarioReturnsUnconstrained(t *testing.T) {
	dir := t.TempDir()
	s := NewAnswerTaxonomyStore(dir, "slug", 50, 90)
	scoped := validPattern("scoped only explain alpha")
	scoped.AppliesToScenarios = []string{"explain"}
	if _, err := s.Append(scoped); err != nil {
		t.Fatal(err)
	}
	universal := validPattern("broadly applicable gamma delta")
	if _, err := s.Append(universal); err != nil {
		t.Fatal(err)
	}

	// Empty scenario — only unconstrained survives. Mirrors the
	// analyze-stage entry path where AnalysisIR is not yet set.
	got := s.RelevantTo("", "", 5)
	if len(got) != 1 {
		t.Fatalf("empty scenario: want 1 unconstrained pattern, got %d", len(got))
	}
	if got[0].Name != "broadly applicable gamma delta" {
		t.Errorf("expected broadly applicable; got %q", got[0].Name)
	}
}

func TestAnswerTaxonomyStore_Append_AssignsTimestampsAndID(t *testing.T) {
	dir := t.TempDir()
	s := NewAnswerTaxonomyStore(dir, "slug", 50, 90)
	before := time.Now()
	saved, err := s.Append(validPattern("brand new pattern beta"))
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if saved == nil {
		t.Fatal("saved pattern is nil")
	}
	if saved.ID == "" {
		t.Error("Append should assign an ID")
	}
	if saved.FirstSeen.IsZero() || saved.LastSeen.IsZero() {
		t.Error("Append should stamp FirstSeen/LastSeen")
	}
	if saved.LastSeen.Before(before) {
		t.Error("LastSeen should be >= time before Append call")
	}
	if saved.HitCount < 1 {
		t.Errorf("HitCount should default to 1, got %d", saved.HitCount)
	}
}
