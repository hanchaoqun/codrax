package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Real preparation and Execute: an attached capture without requested bounds
// must not inherit the other capture's sole explicit window.
func TestHMC225ComparisonRequestWindowPublic(t *testing.T) {
	for _, mode := range []string{"unbounded_side", "both_bounded", "unknown_source", "explicit_query"} {
		t.Run(mode, func(t *testing.T) {
			bus, prep, _ := hmc17NamedPathContext(t)
			path, _ := filepath.Abs("../../eval/fixtures/hmosperf_native_resource_stack/capture.data")
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			other := filepath.Join(t.TempDir(), "capture.data")
			if err := os.WriteFile(other, body, 0600); err != nil {
				t.Fatal(err)
			}
			material, err := prep.Prepare(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			bus.AttachedTraceMaterial, bus.AttachedHitrace = material, material.Preview()
			profile := hmc224SourceOwnedScope(other, path, false)
			profile.Comparison.Baseline.RuntimeArtifactTimeWindow = types.RuntimeArtifactTimeWindow{SourceQuote: "baseline", TimeStart: requestWindowTestProfile(10, 10.05).TimeStart, TimeEnd: requestWindowTestProfile(10, 10.05).TimeEnd}
			profile.Comparison.Current.RuntimeArtifactTimeWindow = types.RuntimeArtifactTimeWindow{SourceQuote: "current"}
			profile.TimeWindows = []types.RuntimeArtifactTimeWindow{profile.Comparison.Baseline.RuntimeArtifactTimeWindow}
			if mode == "both_bounded" {
				profile.Comparison.Current.RuntimeArtifactTimeWindow = types.RuntimeArtifactTimeWindow{SourceQuote: "current", TimeStart: requestWindowTestProfile(10, 10.05).TimeStart, TimeEnd: requestWindowTestProfile(10, 10.05).TimeEnd}
				profile.TimeWindows = append(profile.TimeWindows, profile.Comparison.Current.RuntimeArtifactTimeWindow)
			}
			if mode == "unknown_source" {
				profile.Comparison.Current.Path = filepath.Join(t.TempDir(), "unread.data")
			}
			bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeArtifactScopeProfile: profile}}
			args := map[string]any{"source": "attached_trace", "view": "resource_stack", "pid": 101}
			if mode == "explicit_query" {
				args["time_start"], args["time_end"] = 9, 11
			}
			result := hmc17NamedQuery(t, bus, args)
			p := hmc17NamedPayload(t, result).ResourceStack
			wantRows := 4
			if mode == "both_bounded" {
				wantRows = 3
			}
			if p == nil || p.MatchedEvents != wantRows {
				t.Fatalf("source-owned default lost facts: want %d, got %+v", wantRows, p)
			}
			if got := strings.Contains(result.Summary, "trace_query_request_window_inherited="); got != (mode == "both_bounded") {
				t.Fatalf("wrong inherited scope=%v: %s", got, result.Summary)
			}
			if mode == "explicit_query" && (p.Window.StartTs != 9 || p.Window.EndTs != 11) {
				t.Fatal("explicit exploration was overwritten")
			}
		})
	}
}

func TestHMC225ComparisonPrimarySupplementKeepsSourceWindow(t *testing.T) {
	bus, prep, _ := hmc17NamedPathContext(t)
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_rendering_candidates/events.systrace")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(t.TempDir(), "events.systrace")
	if err := os.WriteFile(other, body, 0600); err != nil {
		t.Fatal(err)
	}
	material, err := prep.Prepare(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	bus.AttachedTraceMaterial, bus.AttachedHitrace = material, material.Preview()
	profile := hmc224SourceOwnedScope(path, other, false)
	for i, bounds := range [][2]float64{{1, 1.05}, {2, 2.1}} {
		window := types.RuntimeArtifactTimeWindow{TimeStart: &bounds[0], TimeEnd: &bounds[1], SourceQuote: "own source window"}
		if i == 0 {
			profile.Comparison.Baseline.RuntimeArtifactTimeWindow = window
		} else {
			profile.Comparison.Current.RuntimeArtifactTimeWindow = window
		}
	}
	profile.TimeWindows = []types.RuntimeArtifactTimeWindow{profile.Comparison.Baseline.RuntimeArtifactTimeWindow, profile.Comparison.Current.RuntimeArtifactTimeWindow}
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeArtifactScopeProfile: profile,
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis, FrameCausalityRequested: true}}}
	primaryWindowQuery(t, bus, 1.051)
	foreign := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": other, "view": "rendering_candidates", "time_start": 1, "time_end": 1.05})
	bus.ToolResults = append(bus.ToolResults, foreign)
	before, _ := json.Marshal([]any{bus.AnalysisIR, bus.ToolResults})
	out := RunTraceQuerySystemSupplement(bus)
	if len(out.Executed) != 1 || out.Executed[0] != "rendering_candidates" {
		t.Fatalf("same-source supplement lost or borrowed other side: %+v", out)
	}
	results := bus.Mutable.SystemTraceSupplementResults()
	if len(results) != 1 {
		t.Fatalf("supplement results=%d", len(results))
	}
	found := false
	for _, record := range results[0].Observations {
		payload, ok := DecodeTraceRenderingCandidates(record)
		if !ok {
			continue
		}
		found = true
		if payload.Window.StartTs != 1 || payload.Window.EndTs != 1.05 || payload.Window.EndInclusive {
			t.Fatalf("borrowed other ruler: %+v", payload.Window)
		}
	}
	if !found {
		t.Fatal("no native supplement publication")
	}
	after, _ := json.Marshal([]any{bus.AnalysisIR, bus.ToolResults})
	if !bytes.Equal(before, after) {
		t.Fatal("source projection changed original IR or observations")
	}
}
