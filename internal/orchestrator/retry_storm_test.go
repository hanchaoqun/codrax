package orchestrator

import (
	"strings"
	"testing"
)

// TestRetryStormDetector_Threshold — ceil(max/2) ≥ 2.
func TestRetryStormDetector_Threshold(t *testing.T) {
	cases := []struct {
		max, want int
	}{
		{1, 2}, // floor
		{2, 2},
		{3, 2},
		{4, 2},
		{5, 3},
		{6, 3},
		{7, 4},
		{10, 5},
	}
	for _, c := range cases {
		d := newAnalyzeRetryStormDetector(c.max)
		if d.threshold != c.want {
			t.Errorf("max=%d: want threshold %d, got %d", c.max, c.want, d.threshold)
		}
	}
}

// TestRetryStormDetector_DifferentFingerprints_NeverExhausted
func TestRetryStormDetector_DifferentFingerprints_NeverExhausted(t *testing.T) {
	d := newAnalyzeRetryStormDetector(3)
	d.observe("a")
	d.observe("b")
	d.observe("c")
	if d.exhausted() {
		t.Errorf("different fingerprints must not exhaust; repeats=%d", d.repeats)
	}
}

// TestRetryStormDetector_SameFingerprint_TripsAtThreshold
func TestRetryStormDetector_SameFingerprint_TripsAtThreshold(t *testing.T) {
	d := newAnalyzeRetryStormDetector(3) // threshold 2
	d.observe("a")
	if d.exhausted() {
		t.Errorf("first observe must not exhaust")
	}
	d.observe("a")
	if !d.exhausted() {
		t.Errorf("second observe of same fp must exhaust at threshold=2")
	}
}

// TestRetryStormDetector_EmptyFingerprintResets
func TestRetryStormDetector_EmptyFingerprintResets(t *testing.T) {
	d := newAnalyzeRetryStormDetector(3)
	d.observe("a")
	d.observe("a")
	if !d.exhausted() {
		t.Errorf("setup: should be exhausted")
	}
	// Empty fingerprint = the LLM's emit had no rejection; reset.
	d.observe("")
	if d.exhausted() {
		t.Errorf("empty fingerprint should reset; repeats=%d", d.repeats)
	}
}

// TestRetryStormDetector_FingerprintChanges_ResetsCounter
func TestRetryStormDetector_FingerprintChanges_ResetsCounter(t *testing.T) {
	d := newAnalyzeRetryStormDetector(5) // threshold 3
	d.observe("a")
	d.observe("a")
	if d.exhausted() {
		t.Errorf("threshold=3 with 2 same-fp observes should not yet exhaust")
	}
	d.observe("b") // different fingerprint resets to 1
	d.observe("b")
	if d.exhausted() {
		t.Errorf("after fingerprint change, 2 of new fp should not exhaust threshold=3")
	}
	d.observe("b")
	if !d.exhausted() {
		t.Errorf("3 same-fp observes should exhaust threshold=3")
	}
}

// TestRetryStormDetector_NilSafe — panics-free under nil receiver.
func TestRetryStormDetector_NilSafe(t *testing.T) {
	var d *analyzeRetryStormDetector
	d.observe("x")
	if d.exhausted() {
		t.Errorf("nil detector should never exhaust")
	}
	if d.repeatCount() != 0 {
		t.Errorf("nil detector repeatCount must be 0")
	}
}

// TestRetryStormUserCaveat_RedlineAudit asserts the user-facing
// caveat string contains no internal vocab and references the user-
// actionable REPL commands. Pinned per the B6 audit table.
func TestRetryStormUserCaveat_RedlineAudit(t *testing.T) {
	c := retryStormUserCaveat()
	// R6: must not leak internal pipeline vocab
	for _, banned := range []string{
		"subtopic_coherence", "shape_subject_coherence", "R1.", "R2.",
		"AnalyzerHints", "TermGraph", "QualityGate", "predicates.",
	} {
		if strings.Contains(c, banned) {
			t.Errorf("caveat must not leak internal token %q; got %q", banned, c)
		}
	}
	// R7: must reference user-actionable controls
	if !strings.Contains(c, "/repos focus") || !strings.Contains(c, "/repos cap") {
		t.Errorf("caveat must reference REPL commands; got %q", c)
	}
	// Cross-language: Chinese present, English commands present, no
	// third natural language. (Spot-check absence of common
	// French/Spanish/German tokens.)
	for _, tok := range []string{"diese", "todos", "tutti", "cette", "nuestro", "nuestra"} {
		if strings.Contains(c, tok) {
			t.Errorf("caveat must not include third-language tokens (%s); got %q", tok, c)
		}
	}
}
