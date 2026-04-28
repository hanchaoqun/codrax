package types

import (
	"testing"
)

// TestCollectLogSourceAnchors_Tier1ByteIdenticalToPreFix locks the
// regression: when a frame's tail matches an evidence item in the
// SAME file with sufficient line drift (> minGap), the existing
// Tier 1 logic still produces an anchor with Reason=line_drift.
// Identical to pre-T1.2 behaviour modulo the new Reason field.
func TestCollectLogSourceAnchors_Tier1ByteIdenticalToPreFix(t *testing.T) {
	rm := RequestModel{Scenario: ScenarioRootCause}
	bundle := &LogBundle{
		Errors: []LogError{{
			Frames: []LogFrame{{File: "auth.go", Line: 234, Func: "(*Authenticator).Login"}},
		}},
	}
	items := []EvidenceItem{{
		Source:       "auth.go",
		LineStart:    241, // 7 lines drift
		AnchorKind:   AnchorDefinition,
		AnchorSymbol: "(*Authenticator).Login",
	}}
	out := collectLogSourceAnchors(rm, bundle, items, 5)
	if len(out) != 1 {
		t.Fatalf("Tier 1 should emit exactly 1 anchor; got %d", len(out))
	}
	if out[0].Reason != DriftReasonLineDrift {
		t.Errorf("Reason=%s, want %s", out[0].Reason, DriftReasonLineDrift)
	}
	if out[0].File != "auth.go" || out[0].AnchoredLine != 241 {
		t.Errorf("anchor wrong: %+v", out[0])
	}
}

// TestCollectLogSourceAnchors_Tier2TailRename pins the new fuzzy-
// tail Tier 2 path: same file, function name actually changed
// (different tail token, but similar enough by substring or
// Levenshtein). The fuzzy matcher detects the rename; OriginalFunc
// carries the log's name and Func carries the current name.
func TestCollectLogSourceAnchors_Tier2TailRename(t *testing.T) {
	rm := RequestModel{Scenario: ScenarioRootCause}
	bundle := &LogBundle{
		Errors: []LogError{{
			// Log frame's tail is "processRequest"
			Frames: []LogFrame{{File: "auth.go", Line: 234, Func: "Handler.processRequest"}},
		}},
	}
	// Evidence's tail is "processRequestV2" — substring match
	// (processRequest ⊂ processRequestV2) → fuzzy hit.
	items := []EvidenceItem{{
		Source:       "auth.go",
		LineStart:    241,
		AnchorKind:   AnchorDefinition,
		AnchorSymbol: "Handler.processRequestV2",
	}}
	out := collectLogSourceAnchors(rm, bundle, items, 0)
	if len(out) != 1 {
		t.Fatalf("Tier 2 should emit 1 anchor for rename; got %d", len(out))
	}
	if out[0].Reason != DriftReasonTailRename {
		t.Errorf("Reason=%s, want %s", out[0].Reason, DriftReasonTailRename)
	}
	if out[0].OriginalFunc != "Handler.processRequest" {
		t.Errorf("OriginalFunc should carry log-side name; got %q", out[0].OriginalFunc)
	}
	// Func carries the lowercase normalized tail (matching the existing
	// normalizedSurfaceSymbolTail convention shared with Tier 1).
	if out[0].Func != "processrequestv2" {
		t.Errorf("Func should carry current renamed tail (lowercased); got %q", out[0].Func)
	}
}

// TestCollectLogSourceAnchors_Tier3FileMoved pins the cross-file
// path: log frame's file is absent from the evidence pool, but
// the symbol tail matches EXACTLY ONE evidence item elsewhere.
// OriginalFile carries the log's path; File carries the new path.
func TestCollectLogSourceAnchors_Tier3FileMoved(t *testing.T) {
	rm := RequestModel{Scenario: ScenarioRootCause}
	bundle := &LogBundle{
		Errors: []LogError{{
			Frames: []LogFrame{{File: "src/handlers/foo.go", Line: 50, Func: "Login"}},
		}},
	}
	// Evidence pool only has the new path; same tail "Login".
	items := []EvidenceItem{{
		Source:       "internal/handlers/foo.go",
		LineStart:    45,
		AnchorKind:   AnchorDefinition,
		AnchorSymbol: "Login",
	}}
	out := collectLogSourceAnchors(rm, bundle, items, 0)
	if len(out) != 1 {
		t.Fatalf("Tier 3 should emit 1 anchor for file move; got %d", len(out))
	}
	if out[0].Reason != DriftReasonFileMoved {
		t.Errorf("Reason=%s, want %s", out[0].Reason, DriftReasonFileMoved)
	}
	if out[0].OriginalFile != "src/handlers/foo.go" {
		t.Errorf("OriginalFile should carry log-side path; got %q", out[0].OriginalFile)
	}
	if out[0].File != "internal/handlers/foo.go" {
		t.Errorf("File should carry new path; got %q", out[0].File)
	}
}

// TestCollectLogSourceAnchors_Tier3RejectsAmbiguousMove guards
// against false positives: when the log's frame.File is absent
// AND the tail matches multiple evidence items, we cannot
// disambiguate which one represents the move, so emit nothing.
func TestCollectLogSourceAnchors_Tier3RejectsAmbiguousMove(t *testing.T) {
	rm := RequestModel{Scenario: ScenarioRootCause}
	bundle := &LogBundle{
		Errors: []LogError{{
			Frames: []LogFrame{{File: "old.go", Line: 50, Func: "handle"}},
		}},
	}
	items := []EvidenceItem{
		{Source: "a.go", LineStart: 10, AnchorKind: AnchorDefinition, AnchorSymbol: "handle"},
		{Source: "b.go", LineStart: 20, AnchorKind: AnchorDefinition, AnchorSymbol: "handle"},
	}
	out := collectLogSourceAnchors(rm, bundle, items, 0)
	if len(out) != 0 {
		t.Errorf("ambiguous tail (>1 match) should NOT emit Tier 3 anchor; got %d", len(out))
	}
}

// TestLogSourceDriftFuzzyTailMatch_Boundaries pins the fuzzy
// matcher's length floors so future tweaks don't accidentally
// open false-positive holes.
func TestLogSourceDriftFuzzyTailMatch_Boundaries(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
		why  string
	}{
		// Substring containment, both >= 4 chars.
		{"FooHandler", "FooHandlerV2", true, "prefix rename via substring"},
		{"handleAuth", "handle", true, "suffix rename via substring"},
		// Levenshtein ≤ 2 for length >= 5.
		{"handler", "handlers", true, "single-char addition (distance=1)"},
		{"parsing", "parsing2", true, "suffix bump (distance=1)"},
		// Below floor — must NOT match.
		{"do", "doX", false, "3-char vs 3-char too short"},
		{"go", "goB", false, "2-char base too short"},
		// Identical — Tier 1 path, fuzzy returns false.
		{"Foo", "Foo", false, "exact match would be Tier 1"},
		// Empty — degenerate.
		{"", "Foo", false, "empty a"},
		{"Foo", "", false, "empty b"},
		// Long but non-similar.
		{"completely-different", "totally-other", false, "no overlap, distance > 2"},
	}
	for _, tc := range cases {
		got := logSourceDriftFuzzyTailMatch(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("logSourceDriftFuzzyTailMatch(%q, %q) = %v, want %v (%s)",
				tc.a, tc.b, got, tc.want, tc.why)
		}
	}
}
