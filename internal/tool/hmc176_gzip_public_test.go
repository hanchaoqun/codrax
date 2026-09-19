package tool

import (
	"bytes"
	"compress/gzip"
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Gzip is a byte transport, not a new source clock or causal relation. This
// exercises the same business/IO fixture through the ordinary public query
// and automatic supplementation boundaries, without a converter substitute.
func TestHMC176GzipPublicQueryAndSupplementPreserveWindowIOBusiness(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "eval", "fixtures", "hmosperf_business_io_chain", "events.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	var compressed bytes.Buffer
	w := gzip.NewWriter(&compressed)
	if _, err := w.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	original := compressed.Bytes()
	path := filepath.Join(t.TempDir(), "业务 capture.sys.gz")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	bus, preparer, preparations := hmc17NamedPathContext(t)
	start, end := 1.0, 1.05
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentRootCause, Scenario: types.ScenarioRootCause,
		RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 100, Thread: "app-main", Source: "user_explicit", Confidence: 1}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1.000..1.050"},
		AnalyzerHints:               types.AnalyzerHints{ExactTargets: []string{path}},
	}}
	result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": "event_search", "pid": 100, "time_start": start, "time_end": end})
	if !result.Success {
		t.Fatalf("gzip public query failed: %+v", result)
	}
	bus.ToolResults = append(bus.ToolResults, result)
	material, err := preparer.Prepare(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if material.SourcePath() != path || material.QueryPath() == path || strings.Contains(material.Preview(), "block_rq_complete") {
		t.Fatalf("expected bounded preview with full separate query: %+v", material)
	}
	decoded, err := os.ReadFile(material.QueryPath())
	if err != nil || !bytes.Equal(body, decoded) {
		t.Fatalf("transport changed source text: %v", err)
	}
	out := RunTraceQuerySystemSupplement(bus)
	if !out.Attempted || len(out.Executed) == 0 || out.SkipReason != "" {
		t.Fatalf("gzip supplement skipped: %+v", out)
	}
	foundIO, foundBusiness := false, false
	for _, supplemented := range bus.Mutable.SystemTraceSupplementResults() {
		if !supplemented.Success {
			continue
		}
		payload := hmc17NamedPayload(t, supplemented)
		if payload.TimeStart != start || payload.TimeEnd != end {
			t.Fatalf("explicit window changed: %g..%g", payload.TimeStart, payload.TimeEnd)
		}
		if payload.RootCauseRank != nil {
			for _, item := range payload.RootCauseRank.Items {
				if item.Thread.PID == 900 && item.ChainRelevance == "on_chain" {
					t.Fatalf("background IO crowned: %+v", item)
				}
				foundIO = foundIO || item.Type == "io_latency" && item.Thread.PID == 200 && item.ChainRelevance == "on_chain" && item.ResourceCompletionClosure && math.Abs(item.EffectiveImpactMs-31) < 1e-6
			}
		}
		for _, observation := range supplemented.Observations {
			if observation.SourceRef.Path == path {
				t.Fatal("text line references attributed to compressed bytes")
			}
			foundBusiness = foundBusiness || strings.Contains(observation.Subject, "LoadDocumentIndex") || strings.Contains(observation.Object, "LoadDocumentIndex") || strings.Contains(strings.Join(observation.RichNotes, "\n"), "LoadDocumentIndex")
		}
	}
	if !foundIO || !foundBusiness || preparations.Load() != 1 {
		t.Fatalf("IO=%v business=%v preparations=%d", foundIO, foundBusiness, preparations.Load())
	}
	if bus.AttachedHitrace != "" || bus.AttachedTraceMaterial != nil {
		t.Fatal("named gzip replaced sticky attachment")
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, original) {
		t.Fatal("original changed", err)
	}
}
