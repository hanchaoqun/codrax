package orchestrator

import (
	"testing"
	"time"
)

// TestRelevantTo_NoValidatorAcceptsStalePatterns pins the back-
// compat: pre-commit-60 callers (no validator wired) keep the
// same behaviour — patterns are returned regardless of
// ExampleFile validity.
func TestRelevantTo_NoValidatorAcceptsStalePatterns(t *testing.T) {
	dir := t.TempDir()
	s := NewAnswerTaxonomyStore(dir, "slug", 50, 90)
	p := validPattern("stale pattern alpha")
	p.ExampleFile = "removed/file/that/no/longer/exists.go"
	if _, err := s.Append(p); err != nil {
		t.Fatal(err)
	}
	got := s.RelevantTo("", "", 5)
	if len(got) != 1 {
		t.Errorf("no validator: stale pattern should still be returned; got %d", len(got))
	}
}

// TestRelevantTo_ValidatorEvictsStalePatterns pins the commit-60
// Batch E.2 audit-MEDIUM-#11 fix: when validator is wired AND
// reports the ExampleFile gone, RelevantTo skips the pattern.
func TestRelevantTo_ValidatorEvictsStalePatterns(t *testing.T) {
	dir := t.TempDir()
	s := NewAnswerTaxonomyStore(dir, "slug", 50, 90)
	stale := validPattern("stale pattern beta gamma")
	stale.ExampleFile = "deleted_module/handler.go"
	if _, err := s.Append(stale); err != nil {
		t.Fatal(err)
	}
	live := validPattern("live pattern delta epsilon")
	live.ExampleFile = "current_module/active.go"
	if _, err := s.Append(live); err != nil {
		t.Fatal(err)
	}

	// Validator: only the live file survives.
	s.SetExampleFileValidator(func(rel string) bool {
		return rel == "current_module/active.go"
	})

	got := s.RelevantTo("", "", 5)
	if len(got) != 1 {
		t.Fatalf("expected 1 surviving pattern after stale-evict; got %d", len(got))
	}
	if got[0].Name != "live pattern delta epsilon" {
		t.Errorf("expected live pattern; got %q", got[0].Name)
	}
}

// TestRelevantTo_EmptyExampleFileBypassesValidator ensures
// patterns without an ExampleFile (the common case) aren't
// affected by the validator.
func TestRelevantTo_EmptyExampleFileBypassesValidator(t *testing.T) {
	dir := t.TempDir()
	s := NewAnswerTaxonomyStore(dir, "slug", 50, 90)
	p := validPattern("no-example pattern alpha")
	// p.ExampleFile intentionally empty
	if _, err := s.Append(p); err != nil {
		t.Fatal(err)
	}
	calls := 0
	s.SetExampleFileValidator(func(rel string) bool {
		calls++
		return false
	})
	got := s.RelevantTo("", "", 5)
	if len(got) != 1 {
		t.Errorf("no-ExampleFile pattern should bypass validator; got %d", len(got))
	}
	if calls != 0 {
		t.Errorf("validator should NOT be called when ExampleFile is empty; called %d times", calls)
	}
}

// TestRelevantTo_StalePatternStillOnDisk ensures the eviction is
// at READ time only — the pattern stays on disk so /answer-
// pitfalls show <id> still shows historic context.
func TestRelevantTo_StalePatternStillOnDisk(t *testing.T) {
	dir := t.TempDir()
	s := NewAnswerTaxonomyStore(dir, "slug", 50, 90)
	stale := validPattern("stale survives storage alpha")
	stale.ExampleFile = "deleted.go"
	stale.LastSeen = time.Now()
	stale.HitCount = 1
	if _, err := s.Append(stale); err != nil {
		t.Fatal(err)
	}
	s.SetExampleFileValidator(func(rel string) bool { return false })

	// RelevantTo should evict.
	if got := s.RelevantTo("", "", 5); len(got) != 0 {
		t.Errorf("stale should be evicted from RelevantTo; got %d", len(got))
	}
	// All() should still show it (operator-facing browse).
	if got := s.All(); len(got) != 1 {
		t.Errorf("stale should remain in All() listing; got %d", len(got))
	}
}
