package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/tracewire"
	"github.com/hanchaoqun/codrax/internal/types"
)

func statisticsReadinessFixture(t *testing.T, name string) (*types.BusContext, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "measure.trace")
	integer := func(v string) tracewire.ProcessMeasureScalar {
		return tracewire.ProcessMeasureScalar{Status: "known", StorageClass: "integer", Value: v}
	}
	pid := 100
	r := tracewire.ProcessMeasureInterval{RowID: 1, FilterID: integer("10"), IPID: integer("1"), StartNS: integer("1000000000"), DurationNS: integer("500000000"), Value: integer("60"), Name: name, NameKnown: true, PID: &pid, ProcessName: "test", OwnerStatus: "known"}
	line, err := tracewire.FormatProcessMeasureInterval(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(line+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("measurements")}, path
}

func statisticsReadinessQuery(t *testing.T, bus *types.BusContext, path, view string, start, end float64, pid int) types.ToolResult {
	t.Helper()
	args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": view, "time_start": start, "time_end": end, "pid": pid})
	out, err := (&tool.TraceQuery{}).Execute(bus, args)
	if err != nil || !out.Success {
		t.Fatalf("public query: %v %s", err, out.Summary)
	}
	return out
}

func TestTraceStatisticsReadinessPublicRawDoesNotRecommendResolved(t *testing.T) {
	bus, path := statisticsReadinessFixture(t, "H:PreferredFrameRate")
	raw := statisticsReadinessQuery(t, bus, path, "process_measurements", 1, 2, 0)
	if !strings.Contains(raw.Summary, "preferred_frame_rate") {
		t.Error("native exact protocol has no optional derived-view navigation")
	}
	e := &explorerEvaluator{mutable: bus.Mutable, turnRouteHint: types.TurnRouteHint{Route: "repo", Source: "external_tool", Confidence: 1, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional}}
	if !e.externalObservationSufficiency([]types.ToolResult{raw}, nil).Status.Sufficient() {
		t.Fatal("setup: raw observations should retain source-lane sufficiency")
	}
	sig := e.postExternalObservationSufficiencySignal(LoopObservation{AllToolResults: []types.ToolResult{raw}})
	if strings.Contains(sig.Hint, "Prefer closing") || sig.HintKey == "explorer.mid-loop.external-observation-sufficient" {
		t.Fatalf("raw source rows incorrectly recommend full resolution: %+v", sig)
	}
	if !strings.Contains(sig.Hint, "preferred_frame_rate") {
		t.Fatalf("missing optional statistics guidance: %+v", sig)
	}
	if e.midLoopCompletionReadySent || e.postCompletionReadyEscalationSignal(LoopObservation{Iteration: 4}).HintRequested {
		t.Fatal("statistics navigation armed forced closure escalation")
	}
	if e.postExternalObservationSufficiencySignal(LoopObservation{AllToolResults: []types.ToolResult{raw}}).HintRequested {
		t.Fatal("unchanged statistics navigation repeated")
	}
	computed := statisticsReadinessQuery(t, bus, path, "preferred_frame_rate", 1, 2, 0)
	if next := e.postExternalObservationSufficiencySignal(LoopObservation{AllToolResults: []types.ToolResult{raw, computed}}); !next.HintRequested || next.HintKey != "explorer.mid-loop.external-observation-sufficient" {
		t.Fatalf("matching computation did not leave navigation state: %+v", next)
	}
}

func TestTraceStatisticsFrequencyFamilyReaderLabelIsDomainNeutral(t *testing.T) {
	for _, zh := range []bool{false, true} {
		label := answerDocBoundedRuntimeFactFamilyReaderLabel(types.RuntimeQuestionFactFrequencyResidency, zh)
		if strings.Contains(label, "CPU") || strings.Contains(label, "策略") || strings.Contains(label, "policy") {
			t.Fatalf("generic family invented CPU domain: %s", label)
		}
	}
	p := &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactFrequencyResidency}}
	if p.RequestsTraceSchedulerTeaching() {
		t.Fatal("source-independent frequency family became scheduler intent")
	}
}

func TestTraceStatisticsReadinessPublicSourceScopeAndCompletion(t *testing.T) {
	for _, tc := range []struct {
		name       string
		view       string
		start, end float64
		pid        int
		pending    bool
	}{
		{"same", "preferred_frame_rate", 1, 2, 0, false},
		{"broader", "preferred_frame_rate", 1, 3, 0, true},
		{"narrower", "preferred_frame_rate", 1, 1.5, 0, true},
		{"different process", "preferred_frame_rate", 1, 2, 100, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus, path := statisticsReadinessFixture(t, "H:PreferredFrameRate")
			raw := statisticsReadinessQuery(t, bus, path, "process_measurements", 1, 2, 0)
			computed := statisticsReadinessQuery(t, bus, path, tc.view, tc.start, tc.end, tc.pid)
			if got := bus.Mutable.TraceStatisticsAvailability([]types.ToolResult{raw, computed}); got.HasPending() != tc.pending {
				t.Fatalf("wrong exact statistics match: %+v", got)
			}
		})
	}
	for _, mode := range []string{"raw only", "unknown protocol", "similar name", "empty result", "explicit raw completion", "explicit insufficient completion"} {
		t.Run(mode, func(t *testing.T) {
			name := "H:PreferredFrameRate"
			if mode == "unknown protocol" {
				name = "private.counter"
			}
			if mode == "similar name" {
				name += "Extra"
			}
			bus, path := statisticsReadinessFixture(t, name)
			pid := 0
			if mode == "empty result" {
				pid = 999
			}
			raw := statisticsReadinessQuery(t, bus, path, "process_measurements", 1, 2, pid)
			ctx := parseOutputCtx("", "")
			ctx.Mutable = bus.Mutable
			ctx.TurnRouteHint = types.TurnRouteHint{Route: "repo", Source: "external_tool", Confidence: 1, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional}
			e := &explorerEvaluator{mutable: bus.Mutable, analysisIR: ctx.AnalysisIR, turnRouteHint: ctx.TurnRouteHint}
			if strings.HasPrefix(mode, "explicit") {
				bus.Mutable.SetInvestigationComplete(mode)
			}
			out, err := e.ParseOutput(ctx, nil, []types.ToolResult{raw}, nil)
			if err != nil {
				t.Fatal(err)
			}
			want := mode != "raw only"
			if out.SignalUpdates == nil || out.SignalUpdates.HasEnoughFacts != want {
				t.Fatalf("mode %s automatic/explicit completion got %+v want %t", mode, out.SignalUpdates, want)
			}
		})
	}
}

func TestTraceStatisticsReadinessPublicNoBorrowedOrReplayedComputation(t *testing.T) {
	for _, mode := range []string{"other source", "JSON", "model aggregate", "new run", "replaced source", "limit variation"} {
		t.Run(mode, func(t *testing.T) {
			bus, path := statisticsReadinessFixture(t, "H:PreferredFrameRate")
			raw := statisticsReadinessQuery(t, bus, path, "process_measurements", 1, 2, 0)
			computed := statisticsReadinessQuery(t, bus, path, "preferred_frame_rate", 1, 2, 0)
			switch mode {
			case "other source":
				_, other := statisticsReadinessFixture(t, "H:PreferredFrameRate")
				computed = statisticsReadinessQuery(t, bus, other, "preferred_frame_rate", 1, 2, 0)
			case "JSON":
				data, _ := json.Marshal(computed)
				computed = types.ToolResult{}
				if err := json.Unmarshal(data, &computed); err != nil {
					t.Fatal(err)
				}
			case "model aggregate":
				computed = types.ToolResult{ToolName: "emit_investigation_complete", Success: true, Observations: computed.Observations, Summary: computed.Summary}
			case "new run":
				old := computed
				bus.Mutable = types.NewMutableState("new")
				raw = statisticsReadinessQuery(t, bus, path, "process_measurements", 1, 2, 0)
				computed = old
			case "replaced source":
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
					t.Fatal(err)
				}
				raw = statisticsReadinessQuery(t, bus, path, "process_measurements", 1, 2, 0)
			case "limit variation":
				args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "preferred_frame_rate", "time_start": 1, "time_end": 2, "limit": 32})
				var err error
				computed, err = (&tool.TraceQuery{}).Execute(bus, args)
				if err != nil || !computed.Success {
					t.Fatalf("%v %s", err, computed.Summary)
				}
			}
			got := bus.Mutable.TraceStatisticsAvailability([]types.ToolResult{raw, computed})
			if got.HasPending() != (mode != "limit variation") {
				t.Fatalf("%s: %+v", mode, got)
			}
		})
	}
}

func TestTraceStatisticsReadinessPublicCPUProducerSharesAvailability(t *testing.T) {
	bus, path := statisticsReadinessFixture(t, "H:PreferredFrameRate")
	text := "cpu-1 (1) [000] .... 1.000000: cpu_frequency: state=1000000 cpu_id=0\ncpu-1 (1) [000] .... 1.000000: cpu_idle: state=4294967295 cpu_id=0\ncpu-1 (1) [000] .... 2.000000: cpu_frequency: state=1200000 cpu_id=0\n"
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	computed := statisticsReadinessQuery(t, bus, path, "cpu_state_frequency", 1, 2, 0)
	got := bus.Mutable.TraceStatisticsAvailability([]types.ToolResult{computed})
	if len(got.ComputedViews) != 1 || got.ComputedViews[0] != "cpu_state_frequency" || got.HasPending() {
		t.Fatalf("non-PFR producer missing native computation receipt: %+v\n%s", got, computed.Summary)
	}
}

func TestTraceStatisticsReadinessPublicDefaultSQLiteAndFinalizerNavigation(t *testing.T) {
	bus, _ := statisticsReadinessFixture(t, "unused")
	bus.TraceInputPreparer = traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(bus.WorkDir, ".codrax")})
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_preferred_frame_rate/capture.data")
	if err != nil {
		t.Fatal(err)
	}
	raw := statisticsReadinessQuery(t, bus, path, "process_measurements", 1, 2, 0)
	if !bus.Mutable.TraceStatisticsAvailability([]types.ToolResult{raw}).HasPending() {
		t.Fatalf("default SQLite preparation lost native navigation receipt: %s", raw.Summary)
	}
	navigation := false
	for _, record := range raw.Observations {
		if p, ok := types.DecodeRuntimeMeasurementPublication(record); ok {
			for _, table := range p.Tables {
				for _, note := range table.Notes {
					navigation = navigation || strings.Contains(note, "view=preferred_frame_rate")
				}
			}
		}
	}
	if !navigation {
		t.Fatal("native table handoff lost derived-view navigation")
	}
	computed := statisticsReadinessQuery(t, bus, path, "preferred_frame_rate", 1, 2, 0)
	if got := bus.Mutable.TraceStatisticsAvailability([]types.ToolResult{raw, computed}); got.HasPending() || len(got.ComputedViews) != 1 {
		t.Fatalf("default SQLite exact computed handoff: %+v", got)
	}
}
