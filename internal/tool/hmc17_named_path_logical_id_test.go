package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The model consumes the AgentContext selection, not the tool's reconstruction
// of it. The original binary and its completed query material must therefore
// have the same logical ID on both sides of the public tool boundary.
func TestHMC17NamedPreparedModelFacingLogicalIDRemainsExecutable(t *testing.T) {
	path, _ := hmc17NamedBinaryFile(t)
	bus, _, conversions := hmc17NamedPathContext(t)
	bus.RuntimeArtifactPreflight = types.RuntimeArtifactPreflightProfile{Artifacts: []types.RuntimeArtifactPreflightArtifact{
		{Kind: "trace", Source: path, Carrier: "request_path"},
	}}
	first := hmc17NamedQuery(t, bus, map[string]any{
		"source": "path", "path": path, "view": "event_search", "pattern": "tail-target",
		"time_start": 1.0105, "time_end": 1.012,
	})
	if !first.Success {
		t.Fatalf("initial named preparation failed: %+v", first)
	}
	view := types.RuntimeArtifactSelectionViewFromAgentContext(&types.AgentContext{
		RuntimeArtifactPreflight: bus.RuntimeArtifactPreflight,
		TraceInputPreparer:       bus.TraceInputPreparer,
	})
	// Round-trip only the actual model-facing contract. A preparation handle
	// is never serialized and cannot be reconstructed from the item itself.
	wire, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	var modelView types.RuntimeArtifactSelectionView
	if err := json.Unmarshal(wire, &modelView); err != nil {
		t.Fatal(err)
	}
	item, ok := modelView.SingleTraceArtifact()
	if !ok || item.Source == path {
		t.Fatalf("prepared model context did not expose the single complete query material: %+v", modelView)
	}
	result := hmc17NamedQuery(t, bus, map[string]any{
		"source": "path", "path": item.ID, "view": "event_search", "pattern": "tail-target",
		"time_start": 1.0105, "time_end": 1.012,
	})
	if !result.Success || !strings.Contains(result.Summary, "tail-target") || conversions.Load() != 1 {
		t.Fatalf("model-facing prepared logical ID was rejected or changed capture: item=%+v conversions=%d result=%+v", item, conversions.Load(), result)
	}
	if toolView := traceQueryRuntimeArtifactSelectionView(bus); !reflect.DeepEqual(view, toolView) {
		t.Fatalf("model/tool source views diverged: model=%+v tool=%+v", view, toolView)
	}
	if payload := hmc17NamedPayload(t, result); payload.TimeStart != 1.0105 || payload.TimeEnd != 1.012 {
		t.Fatalf("logical ID changed explicit query window: %g..%g", payload.TimeStart, payload.TimeEnd)
	}
}

func TestHMC17NamedPreparedSourceViewsAgreeOnCaptureIdentity(t *testing.T) {
	for _, distinctCapture := range []bool{false, true} {
		name, wantCount := "source_and_query_are_one", 1
		if distinctCapture {
			name, wantCount = "different_captures_stay_two", 2
		}
		t.Run(name, func(t *testing.T) {
			bus, preparer, _ := hmc17NamedPathContext(t)
			bus.AnalysisIR = &types.AnalysisIR{}
			for i := 0; i < wantCount; i++ {
				path, _ := hmc17NamedBinaryFile(t)
				material, err := preparer.Prepare(context.Background(), path)
				if err != nil {
					t.Fatal(err)
				}
				bus.RuntimeArtifactPreflight.Artifacts = append(bus.RuntimeArtifactPreflight.Artifacts,
					types.RuntimeArtifactPreflightArtifact{Kind: "trace", Source: path, Carrier: "request_path"})
				bus.AnalysisIR.RequestModel.AnalyzerHints.RequiredFileHints = append(bus.AnalysisIR.RequestModel.AnalyzerHints.RequiredFileHints,
					types.RequiredFileHint{Path: material.QueryPath(), Confidence: 1})
			}
			modelView := types.RuntimeArtifactSelectionViewFromAgentContext(&types.AgentContext{
				RuntimeArtifactPreflight: bus.RuntimeArtifactPreflight,
				TraceInputPreparer:       bus.TraceInputPreparer,
				AnalysisIR:               bus.AnalysisIR,
			})
			if modelView.TraceCount != wantCount {
				t.Fatalf("model source view changed physical capture count: want=%d got=%+v", wantCount, modelView)
			}
			if toolView := traceQueryRuntimeArtifactSelectionView(bus); !reflect.DeepEqual(modelView, toolView) {
				t.Fatalf("model/tool source views diverged: model=%+v tool=%+v", modelView, toolView)
			}
		})
	}
}

func TestHMC17NamedModelFacingSelectionCannotRetargetSeenAlias(t *testing.T) {
	bus := hmc17RetargetedNamedAlias(t)
	alias := bus.AnalysisIR.RequestModel.AnalyzerHints.ExactTargets[0]
	bus.RuntimeArtifactPreflight = types.RuntimeArtifactPreflightProfile{Artifacts: []types.RuntimeArtifactPreflightArtifact{
		{Kind: "trace", Source: alias, Carrier: "request_path"},
	}}
	view := types.RuntimeArtifactSelectionViewFromAgentContext(&types.AgentContext{
		RuntimeArtifactPreflight: bus.RuntimeArtifactPreflight,
		TraceInputPreparer:       bus.TraceInputPreparer,
	})
	item, ok := view.SingleTraceArtifact()
	if !ok || item.Source != alias {
		t.Fatalf("model selection replaced a seen alias with its new capture: %+v", view)
	}
	result := hmc17NamedQuery(t, bus, map[string]any{
		"source": "path", "path": item.ID, "view": "event_search", "pattern": "evil-target",
		"time_start": 1.0105, "time_end": 1.012,
	})
	if result.Success || len(result.Observations) != 0 {
		t.Fatalf("model-facing logical selection bypassed the seen alias binding: %+v", result)
	}
}

func TestHMC17NamedCaseAliasCannotReprepareChangedCapture(t *testing.T) {
	path, original := hmc17NamedBinaryFile(t)
	alias := filepath.Join(filepath.Dir(path), strings.ToUpper(filepath.Base(path)))
	sourceInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	aliasInfo, err := os.Stat(alias)
	if err != nil || !os.SameFile(sourceInfo, aliasInfo) {
		t.Skip("fixture volume does not alias filename case")
	}
	bus, _, conversions := hmc17NamedPathContext(t)
	first := hmc17NamedQuery(t, bus, map[string]any{
		"source": "path", "path": path, "view": "event_search", "pattern": "tail-target",
		"time_start": 1.0105, "time_end": 1.012,
	})
	if !first.Success {
		t.Fatalf("initial named preparation failed: %+v", first)
	}
	if err := os.WriteFile(path, bytes.ReplaceAll(original, []byte("tail-target"), []byte("evil-target")), 0o600); err != nil {
		t.Fatal(err)
	}
	result := hmc17NamedQuery(t, bus, map[string]any{
		"source": "path", "path": alias, "view": "event_search", "pattern": "evil-target",
		"time_start": 1.0105, "time_end": 1.012,
	})
	if result.Success || len(result.Observations) != 0 || conversions.Load() != 1 {
		t.Fatalf("same directory entry with another case silently prepared a changed capture: conversions=%d result=%+v", conversions.Load(), result)
	}
}
