package tool

import (
	"bytes"
	"encoding/json"
	"html"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Actual native parsing, typed publication, both public answer mutation paths,
// immutable accepted snapshots, and the user-visible renderer. No fake receipt
// rows or replacement provider is installed by this fixture.
func TestRuntimeMeasurementPublicQueryEmitPatchRender(t *testing.T) {
	const body = "first-11 (11) [000] .... 1.000000: block_rq_issue: 8,0 R 4096 () 8 + 8 [first]\n" +
		"second-22 (22) [001] .... 1.002000: block_rq_issue: 8,0 R 4096 () 16 + 8 [second]\n" +
		"irq-2 (2) [000] .... 1.004000: block_rq_complete: 8,0 R () 8 + 8 [0]\n" +
		"irq-2 (2) [001] .... 1.005000: block_rq_complete: 8,0 R () 16 + 8 [0]\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "io.systrace")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	start, end := 1.001, 1.006
	rm := types.RequestModel{Language: "en", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric,
		PerfTrace:                   &types.PerfBundle{},
		RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactResourcePressure}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1.001–1.006 seconds", Confidence: 1},
	}
	mu := types.NewMutableState("Describe observed IO measurements, members and timeline within 1.001–1.006 seconds")
	mu.SetRequestModel(rm)
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Language: "en", Mutable: mu, AnalysisIR: &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "en"}}}
	query, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "time_start": start, "time_end": end})
	result, err := (&TraceQuery{}).Execute(ctx, query)
	if err != nil || !result.Success {
		t.Fatalf("native query: %v / %s", err, result.Summary)
	}
	mu.AppendDispatchToolResult(result)
	mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	var payload tracequery.Result
	for _, row := range result.Observations {
		if row.Predicate == "io_inflight" {
			data, readErr := os.ReadFile(row.SourceRef.PayloadRef)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if err := json.Unmarshal(data, &payload); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if payload.WindowStats == nil || payload.WindowStats.IOInFlight == nil || len(payload.WindowStats.IOInFlight.Groups) != 1 {
		t.Fatal("native IO prerequisites absent")
	}
	g := payload.WindowStats.IOInFlight.Groups[0]
	if g.Values == nil || g.Values.PeakRequests != 2 || math.Abs(g.Values.MeanRequests-1.2) > 1e-9 || math.Abs(g.Values.BusyMs-4) > 1e-9 || math.Abs(g.Values.RequestMs-6) > 1e-9 || len(g.Members) != 2 {
		t.Fatalf("native arithmetic/member prerequisite: %+v", g)
	}
	view := types.BuildAnswerSemanticViewForBusContext(ctx)
	if view == nil || !view.RuntimeMeasurementContract.Active() {
		for _, row := range result.Observations {
			if row.Predicate != "io_inflight" {
				continue
			}
			_, admitted := types.DecodeRuntimeMeasurementPublication(row)
			t.Logf("measurement publication admitted=%t row=%s producer=%s origin=%s policy=%s lane=%s source=%+v", admitted, row.ID, row.Producer, row.Origin, row.GroundingPolicy, row.ProvenanceLane, row.SourceRef)
			for _, note := range row.RichNotes {
				if body, found := strings.CutPrefix(note, types.TraceNoteKeyRuntimeMeasurement+"="); found {
					var publication types.RuntimeMeasurementPublication
					decodeErr := json.Unmarshal([]byte(body), &publication)
					t.Logf("publication bytes=%d decode=%v source=%+v tables=%d", len(body), decodeErr, publication.Source, len(publication.Tables))
				}
			}
		}
		t.Fatal("public producer did not publish selectable measurement tables")
	}
	choices := view.RuntimeMeasurementContract.Choices()
	byView := map[types.RuntimeMeasurementView]types.RuntimeMeasurementTable{}
	for _, choice := range choices {
		byView[choice.View] = choice
	}
	if len(byView) != 3 {
		t.Fatalf("summary/member/timeline choices missing: %+v", choices)
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
	projectionBefore, _ := json.Marshal(types.CompileTraceCausalProjectionSet(ledger))
	const prose = "Model-owned interpretation is separate from the measurements."
	selected := func(v types.RuntimeMeasurementView) map[string]any {
		return map[string]any{"id": "measurement", "kind": "table", "runtime_measurement": map[string]any{"observation_id": byView[v].ObservationID, "view": string(v)}}
	}
	execute := func(v types.RuntimeMeasurementView, patch bool) {
		t.Helper()
		input := map[string]any{"blocks": []any{map[string]any{"id": "lead", "kind": "summary", "text": prose}, selected(v)}}
		schema := BuildAnswerDocumentParametersFor(types.BuildAnswerSemanticViewForBusContext(ctx))
		if patch {
			input = map[string]any{"replace_blocks": []any{selected(v)}, "unchanged_block_ids": []string{"lead"}}
			schema = BuildAnswerDocumentPatchParametersFor(types.BuildAnswerSemanticViewForBusContext(ctx))
		}
		raw, _ := json.Marshal(input)
		if err := toolparam.Validate(raw, schema); err != nil {
			t.Fatal("published selector schema rejected its own choice", err)
		}
		out := b1659bExecuteAnswer(t, ctx, input, patch)
		if !out.Success {
			t.Fatalf("public answer mutation patch=%t: %s", patch, out.Summary)
		}
		doc := mu.AnswerDocumentV2()
		block := blockByID(t, doc, "measurement")
		if block.RuntimeMeasurement == nil || !block.RuntimeMeasurement.IsBound() || !reflect.DeepEqual(*block.RuntimeMeasurement.BoundTable, byView[v]) {
			t.Fatalf("accepted table diverged from current provider: %+v", block)
		}
		if blockByID(t, doc, "lead").Text != prose {
			t.Fatal("measurement selection rewrote model interpretation")
		}
		visible := render.RenderAnswerDocument(doc, "en")
		literal := html.UnescapeString(visible)
		for _, row := range byView[v].Rows {
			for _, cell := range row {
				if cell != "" && !strings.Contains(literal, cell) {
					t.Fatalf("trusted selected cell %q absent from rendered answer: %s", cell, visible)
				}
			}
		}
		if !strings.Contains(visible, prose) {
			t.Fatal("model prose disappeared")
		}
	}
	execute(types.RuntimeMeasurementSummary, false)
	execute(types.RuntimeMeasurementMembers, true)
	execute(types.RuntimeMeasurementTimeline, true)
	// Reject a stale identity and a competing authored table through BOTH
	// entrypoints without changing the last accepted document or its bound data.
	for _, patch := range []bool{false, true} {
		for _, badKind := range []string{"stale", "competing"} {
			bad := selected(types.RuntimeMeasurementSummary)
			if badKind == "stale" {
				bad["runtime_measurement"] = map[string]any{"observation_id": "different-source-or-window", "view": "summary"}
			} else {
				bad["text"] = "| peak |\n|---|\n|999|"
			}
			input := map[string]any{"blocks": []any{map[string]any{"id": "lead", "kind": "summary", "text": prose}, bad}}
			if patch {
				input = map[string]any{"replace_blocks": []any{bad}, "unchanged_block_ids": []string{"lead"}}
			}
			before := mu.AnswerDocumentV2()
			out := b1659bExecuteAnswer(t, ctx, input, patch)
			if out.Success || !strings.Contains(out.Summary, "runtime_measurement") {
				t.Fatalf("invalid measured table admitted patch=%t/%s: %s", patch, badKind, out.Summary)
			}
			if !reflect.DeepEqual(before, mu.AnswerDocumentV2()) {
				t.Fatal("rejected attempt changed accepted document")
			}
		}
	}
	freshLedger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
	projectionAfter, _ := json.Marshal(types.CompileTraceCausalProjectionSet(freshLedger))
	if !bytes.Equal(projectionBefore, projectionAfter) || mu.TraceRootCauseReport() != nil {
		t.Fatal("pure measurement selection acquired causal authority")
	}
	unchanged, _ := os.ReadFile(path)
	if string(unchanged) != body {
		t.Fatal("read-mode display changed source trace")
	}
}
