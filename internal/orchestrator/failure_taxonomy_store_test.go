package orchestrator

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func newValidPattern(name string) *types.FailurePattern {
	return &types.FailurePattern{
		Name:        name,
		Description: "When a new exported method is added to a goroutine-shared struct, the mutex coverage tends to leave it unguarded under concurrent calls.",
		Trigger:     "adding a method to a goroutine-shared struct",
		Consequence: "data race detected by go test -race",
		Confidence:  0.7,
	}
}

func TestStore_DisabledStoreIsNoOp(t *testing.T) {
	s := NewFailureTaxonomyStore("", "", 0, 0)
	if s.Enabled() {
		t.Errorf("empty cacheDir+repoSlug should disable store")
	}
	saved, err := s.Append(newValidPattern("noop test"))
	if err != nil || saved != nil {
		t.Errorf("disabled Append should be no-op; got %v %v", saved, err)
	}
	if got := s.Load(); got != nil {
		t.Errorf("disabled Load should return nil; got %+v", got)
	}
	if got := s.RelevantTo("feature", []string{"x.go"}, 5); got != nil {
		t.Errorf("disabled RelevantTo should return nil; got %+v", got)
	}
}

func TestStore_AppendNewPatternPersists(t *testing.T) {
	dir := t.TempDir()
	s := NewFailureTaxonomyStore(dir, "demo-1234", 50, 90)
	p := newValidPattern("lock-scope drift on new public method")
	saved, err := s.Append(p)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if saved == nil {
		t.Fatal("expected saved pattern; got nil")
	}
	if saved.ID == "" {
		t.Errorf("Append should auto-assign ID; got empty")
	}
	if saved.HitCount != 1 {
		t.Errorf("first Append should set HitCount=1; got %d", saved.HitCount)
	}

	// Reload via fresh store + verify persisted.
	s2 := NewFailureTaxonomyStore(dir, "demo-1234", 50, 90)
	loaded := s2.Load()
	if loaded == nil || len(loaded.Patterns) != 1 {
		t.Fatalf("expected 1 persisted pattern; got %+v", loaded)
	}
	if loaded.Patterns[0].ID != saved.ID {
		t.Errorf("persisted ID drift; got %q want %q", loaded.Patterns[0].ID, saved.ID)
	}
}

func TestStore_AppendDedupsBumpsHitCount(t *testing.T) {
	dir := t.TempDir()
	s := NewFailureTaxonomyStore(dir, "demo-dedup", 50, 90)
	first, err := s.Append(newValidPattern("lock-scope drift on new public method"))
	if err != nil {
		t.Fatalf("Append #1: %v", err)
	}
	second, err := s.Append(newValidPattern("lock scope drift public method add"))
	if err != nil {
		t.Fatalf("Append #2: %v", err)
	}
	// SimilarTo should treat them as the same; second
	// returns the bumped first.
	if first.ID != second.ID {
		t.Errorf("dedup expected; got distinct IDs %s vs %s", first.ID, second.ID)
	}
	if second.HitCount != 2 {
		t.Errorf("expected HitCount=2 after second Append; got %d", second.HitCount)
	}
	all := s.All()
	if len(all) != 1 {
		t.Errorf("expected only 1 pattern after dedup; got %d", len(all))
	}
}

func TestStore_RelevantTo_KindFilter(t *testing.T) {
	dir := t.TempDir()
	s := NewFailureTaxonomyStore(dir, "demo-relevance", 50, 90)
	p1 := newValidPattern("auth migration test fixture rot")
	p1.AppliesToKinds = []string{"feature"}
	if _, err := s.Append(p1); err != nil {
		t.Fatalf("Append p1: %v", err)
	}
	p2 := newValidPattern("docs build break on stale links")
	p2.AppliesToKinds = []string{"refactor"}
	if _, err := s.Append(p2); err != nil {
		t.Fatalf("Append p2: %v", err)
	}
	got := s.RelevantTo("feature", []string{"x.go"}, 5)
	if len(got) != 1 {
		t.Fatalf("expected only the feature-tagged pattern; got %d", len(got))
	}
	if !strings.Contains(got[0].Name, "auth migration") {
		t.Errorf("got wrong pattern; %s", got[0].Name)
	}
}

func TestStore_RelevantTo_PathOverlap(t *testing.T) {
	dir := t.TempDir()
	s := NewFailureTaxonomyStore(dir, "demo-paths", 50, 90)
	p1 := newValidPattern("auth pkg refactor fixture stale fixture broken")
	p1.AppliesToPaths = []string{"internal/auth/"}
	if _, err := s.Append(p1); err != nil {
		t.Fatalf("Append p1: %v", err)
	}
	p2 := newValidPattern("payment subsystem rounding drift assertion")
	p2.AppliesToPaths = []string{"internal/payment/"}
	if _, err := s.Append(p2); err != nil {
		t.Fatalf("Append p2: %v", err)
	}
	// Task touches internal/auth/ — auth pattern should rank
	// higher.
	got := s.RelevantTo("feature", []string{"internal/auth/session.go"}, 5)
	if len(got) == 0 {
		t.Fatal("expected at least one relevant pattern")
	}
	if !strings.Contains(got[0].Name, "auth") {
		t.Errorf("auth pattern should rank first; got %s", got[0].Name)
	}
}

func TestStore_Clear_WipesCache(t *testing.T) {
	dir := t.TempDir()
	s := NewFailureTaxonomyStore(dir, "demo-clear", 50, 90)
	if _, err := s.Append(newValidPattern("any pattern enough chars to validate")); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := s.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	all := s.All()
	if len(all) != 0 {
		t.Errorf("expected empty after Clear; got %d", len(all))
	}
	// Reload via fresh store — file should be missing.
	s2 := NewFailureTaxonomyStore(dir, "demo-clear", 50, 90)
	if got := s2.Load(); got == nil || len(got.Patterns) != 0 {
		t.Errorf("Clear should wipe disk; got %+v", got)
	}
}

func TestStore_DecaySweep_DropsOldOneOffs(t *testing.T) {
	dir := t.TempDir()
	// 30-day decay window.
	s := NewFailureTaxonomyStore(dir, "demo-decay", 50, 30)
	// Force-load + manually inject an aged pattern with
	// HitCount=1 (fresh insertion-time API doesn't let us
	// backdate). Then trigger decay by appending a new
	// pattern.
	t1 := s.Load()
	old := time.Now().AddDate(0, 0, -90)
	t1.Patterns = append(t1.Patterns, types.FailurePattern{
		ID: "fp-old", Name: "old one-off pattern", Description: strings.Repeat("a", 50),
		Trigger: strings.Repeat("b", 20), Confidence: 0.6,
		FirstSeen: old, LastSeen: old, HitCount: 1,
	})
	if _, err := s.Append(newValidPattern("trigger decay sweep here recent")); err != nil {
		t.Fatalf("trigger Append: %v", err)
	}
	all := s.All()
	for _, p := range all {
		if p.ID == "fp-old" {
			t.Errorf("decay should have dropped old one-off; still present")
		}
	}
}

// TestRelevantToWithKeywords_BoostShiftsRanking pins commit
// 40 P2: when two patterns score identically on kind +
// paths + recency, the one whose text matches the supplied
// keywords ranks higher. Used by the multi-phase
// orchestrator to bias toward the active phase's pitfalls.
func TestRelevantToWithKeywords_BoostShiftsRanking(t *testing.T) {
	dir := t.TempDir()
	s := NewFailureTaxonomyStore(dir, "demo-keywords", 50, 90)
	// Two patterns with same kind/path scope, distinct text.
	a := newValidPattern("schema migration backfill misorder")
	a.Description = "When applying a schema migration before the ORM update, the new column has no default and existing rows fail validation."
	a.Trigger = "running schema migration ahead of ORM update"
	a.AppliesToKinds = []string{"feature"}
	if _, err := s.Append(a); err != nil {
		t.Fatalf("Append a: %v", err)
	}
	b := newValidPattern("orm refactor stale fixture mock")
	b.Description = "ORM refactor that adds a method downstream tests stub will leave the new method unstubbed; tests pass with zero values."
	b.Trigger = "ORM refactor that widens an interface"
	b.AppliesToKinds = []string{"feature"}
	if _, err := s.Append(b); err != nil {
		t.Fatalf("Append b: %v", err)
	}

	// Without keyword boost, both score equally — recency
	// tie-break determines order.
	noBoost := s.RelevantTo("feature", nil, 5)
	if len(noBoost) != 2 {
		t.Fatalf("expected 2 patterns; got %d", len(noBoost))
	}

	// With keyword "ORM refactor", pattern b should rank
	// first.
	withBoost := s.RelevantToWithKeywords("feature", nil, []string{"ORM refactor"}, 5)
	if len(withBoost) != 2 {
		t.Fatalf("expected 2 patterns; got %d", len(withBoost))
	}
	if !strings.Contains(withBoost[0].Name, "orm") {
		t.Errorf("ORM-keyword query should rank ORM pattern first; got %s", withBoost[0].Name)
	}

	// Inverse: keyword "schema migration" should rank a first.
	withSchemaBoost := s.RelevantToWithKeywords("feature", nil, []string{"schema migration"}, 5)
	if !strings.Contains(withSchemaBoost[0].Name, "schema") {
		t.Errorf("schema-keyword query should rank schema pattern first; got %s", withSchemaBoost[0].Name)
	}
}

// TestRelevantToWithKeywords_EmptyKeywordsMatchesRelevantTo
// pins back-compat: nil keywords reduces RelevantToWithKeywords
// to RelevantTo (zero boost contribution).
func TestRelevantToWithKeywords_EmptyKeywordsMatchesRelevantTo(t *testing.T) {
	dir := t.TempDir()
	s := NewFailureTaxonomyStore(dir, "demo-empty-kw", 50, 90)
	if _, err := s.Append(newValidPattern("any pattern long enough to validate")); err != nil {
		t.Fatalf("Append: %v", err)
	}
	a := s.RelevantTo("feature", nil, 5)
	b := s.RelevantToWithKeywords("feature", nil, nil, 5)
	if len(a) != len(b) {
		t.Errorf("nil-keyword RelevantToWithKeywords should match RelevantTo; got %d vs %d", len(a), len(b))
	}
}

func TestScorePatternRelevance_KindMismatchZeroes(t *testing.T) {
	now := time.Now()
	p := &types.FailurePattern{
		Name: "x", Description: strings.Repeat("a", 50), Trigger: strings.Repeat("b", 20),
		Confidence: 0.9, AppliesToKinds: []string{"refactor"}, LastSeen: now,
	}
	if got := scorePatternRelevance(p, "feature", nil, 90, now); got != 0 {
		t.Errorf("kind mismatch should return 0; got %f", got)
	}
}
