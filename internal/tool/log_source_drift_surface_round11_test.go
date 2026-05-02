package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestRenderDriftBoundedCurrentRootCauseSummary_NeutralPhrasingNoPanic
// Round-11 user red line: drift mode fires for both IntentRootCause
// AND IntentTrace (R9). The prose tail clause must NOT use panic-
// specific terminology like "dereference" / "解引用" since IntentTrace
// users would find that off-topic.
func TestRenderDriftBoundedCurrentRootCauseSummary_NeutralPhrasingNoPanic(t *testing.T) {
	plan := &types.AnswerSurfacePlan{
		SummarySurfaceMode: types.AnswerSummarySurfaceDriftBoundedRootCause,
		DriftBoundedSurfaceItems: []types.EvidenceItem{
			{
				Kind:         types.EvidenceRelationship,
				Source:       "x.go",
				LineStart:    100,
				AnchorKind:   types.AnchorCall,
				AnchorSymbol: "Run",
				Subject:      "Caller",
				Object:       "Run",
			},
		},
	}
	for _, lang := range []string{"en", "zh"} {
		out := RenderDriftBoundedCurrentRootCauseSummary(plan, lang)
		if strings.Contains(strings.ToLower(out), "dereference") {
			t.Errorf("[%s] panic-specific 'dereference' leaked: %q", lang, out)
		}
		if strings.Contains(out, "解引用") {
			t.Errorf("[%s] panic-specific '解引用' leaked: %q", lang, out)
		}
	}
}

// TestRenderDriftBoundedPerfBundleSurfaceSummary_UsesPerfTraceWording:
// round-11 perf-only scenario must NOT render "attached log" prose.
// The new helper uses "perf trace" wording so the artifact name
// matches reality.
func TestRenderDriftBoundedPerfBundleSurfaceSummary_UsesPerfTraceWording(t *testing.T) {
	bundle := &types.PerfBundle{
		Meta: types.PerfMeta{
			Signals: []string{"jank", "main-thread-stall"},
		},
	}
	for lang, mustContain := range map[string]string{
		"en": "perf trace",
		"zh": "性能轨迹",
	} {
		out := RenderDriftBoundedPerfBundleSurfaceSummary(bundle, lang)
		if !strings.Contains(out, mustContain) {
			t.Errorf("[%s] perf surface prose missing %q: %q", lang, mustContain, out)
		}
		if strings.Contains(out, "log") || strings.Contains(out, "日志") {
			t.Errorf("[%s] perf prose incorrectly mentions log: %q", lang, out)
		}
	}
}

// TestRenderDriftBoundedLogBundleSurfaceSummary_NonPanicSignalGrammar:
// round-11 — the prose grammar should fit non-panic signal sets
// (timeout / db / network / validation). Pre-fix "is a [timeout]"
// read awkwardly; new wording "surfaced these signals: [timeout]"
// fits any subset.
func TestRenderDriftBoundedLogBundleSurfaceSummary_NonPanicSignalGrammar(t *testing.T) {
	cases := []struct {
		name    string
		signals []types.LogSignal
	}{
		{"timeout-only", []types.LogSignal{types.SignalTimeout}},
		{"db+network", []types.LogSignal{types.SignalDB, types.SignalNetwork}},
		{"validation", []types.LogSignal{types.SignalValidation}},
		{"performance-only", []types.LogSignal{types.SignalPerformance}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bundle := &types.LogBundle{Meta: types.LogMeta{Signals: tc.signals}}
			out := RenderDriftBoundedLogBundleSurfaceSummary(bundle, "en")
			// Pre-fix "is a [X]" assumed X was a panic-class noun; the
			// new "surfaced these signals: ..." reads naturally for any
			// subset of signal labels.
			if out == "" {
				t.Errorf("expected non-empty prose for signals %v", tc.signals)
			}
			// Must NOT contain "is a `panic`" assumption when we fed
			// non-panic signals.
			for _, sig := range tc.signals {
				if strings.Contains(out, "is a `"+string(sig)+"`") {
					t.Errorf("pre-fix awkward grammar still present for signal %q: %q", sig, out)
				}
			}
		})
	}
}
