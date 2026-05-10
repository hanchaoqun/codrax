package orchestrator

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/multigraph"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/topology"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
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
// caveat strings contain no internal vocab and gate /repos suggestion
// behind multi-repo posture. P7 (2026-05-10) split single / multi /
// CN / EN branches.
func TestRetryStormUserCaveat_RedlineAudit(t *testing.T) {
	// Build small fixture multigraphs for both single + multi.
	zhMulti := retryStormUserCaveat(retryStormFixtureBusCtx(t, "zh", true))
	zhSingle := retryStormUserCaveat(retryStormFixtureBusCtx(t, "zh", false))
	enMulti := retryStormUserCaveat(retryStormFixtureBusCtx(t, "en", true))
	enSingle := retryStormUserCaveat(retryStormFixtureBusCtx(t, "en", false))

	allMessages := []string{zhMulti, zhSingle, enMulti, enSingle}

	// R6: no internal pipeline vocab.
	for _, c := range allMessages {
		for _, banned := range []string{
			"subtopic_coherence", "shape_subject_coherence", "R1.", "R2.",
			"AnalyzerHints", "TermGraph", "QualityGate", "predicates.",
		} {
			if strings.Contains(c, banned) {
				t.Errorf("caveat must not leak internal token %q; got %q", banned, c)
			}
		}
	}

	// Multi-repo branches reference /repos; single-repo branches MUST NOT.
	if !strings.Contains(zhMulti, "/repos") {
		t.Errorf("CN multi-repo should reference /repos; got %q", zhMulti)
	}
	if strings.Contains(zhSingle, "/repos") {
		t.Errorf("CN single-repo MUST NOT reference /repos (misleading); got %q", zhSingle)
	}
	if !strings.Contains(enMulti, "/repos") {
		t.Errorf("EN multi-repo should reference /repos; got %q", enMulti)
	}
	if strings.Contains(enSingle, "/repos") {
		t.Errorf("EN single-repo MUST NOT reference /repos (misleading); got %q", enSingle)
	}

	// CN-purity: no English prose mid-sentence (CLI command names like
	// /repos exempted as inline code identifiers, not English prose).
	englishProseInCN := []string{"REPL", "active 子仓", "active sub-repo"}
	for _, banned := range englishProseInCN {
		for _, msg := range []string{zhMulti, zhSingle} {
			if strings.Contains(msg, banned) {
				t.Errorf("CN branch must not mix English prose %q; got %q", banned, msg)
			}
		}
	}

	// No third natural language tokens.
	for _, msg := range allMessages {
		for _, tok := range []string{"diese", "todos", "tutti", "cette", "esto", "это"} {
			if strings.Contains(msg, tok) {
				t.Errorf("must be CN+EN only; banned %q in %q", tok, msg)
			}
		}
	}
}

// retryStormFixtureBusCtx returns a BusContext with a populated
// MultiGraph (or nil) shaped for the multiRepo / lang branches the
// audit test needs to exercise.
func retryStormFixtureBusCtx(t *testing.T, lang string, multi bool) *types.BusContext {
	t.Helper()
	ctx := &types.BusContext{Language: lang}
	if !multi {
		return ctx
	}
	repos := []topology.SubRepo{
		{Slug: "a-1", RootAbs: "/p/a", RootRel: "a", FileCount: 1},
		{Slug: "b-2", RootAbs: "/p/b", RootRel: "b", FileCount: 1},
	}
	topo := &topology.RepoTopology{
		ParentRoot:   "/p",
		ParentSlug:   "p-deadbeef",
		Repos:        repos,
		DiscoveredAt: time.Now(),
	}
	build := func(string, string) (*rmtypes.Graph, error) {
		return &rmtypes.Graph{
			FileIndex:      map[string]*rmtypes.FileInfo{},
			SymbolDefs:     map[string][]*rmtypes.Symbol{},
			ImportGraph:    map[string][]string{},
			ReverseImports: map[string][]string{},
			Scores:         map[string]float64{},
			QueryScores:    map[string]float64{},
			Metadata:       rmtypes.Metadata{ScanTime: time.Now()},
		}, nil
	}
	mg, err := multigraph.New(multigraph.Config{Topology: topo, Build: build, Cap: 4})
	if err != nil {
		t.Fatalf("multigraph.New: %v", err)
	}
	ctx.MultiGraph = mg
	return ctx
}
