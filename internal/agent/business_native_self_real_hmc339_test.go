package agent

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Model-free adapter regression on the original whole user source. The
// independently frozen wall oracle is operator material, never a live prompt.
func TestHMC339FullDouyinBusinessSelfHandoff(t *testing.T) {
	path, root, oraclePath := os.Getenv("CODRAX_HMC339_REAL_TRACE"), os.Getenv("CODRAX_HMC339_REAL_ARTIFACTS"), os.Getenv("CODRAX_HMC339_WALL_ORACLE")
	if path == "" {
		t.Skip("complete real source opt-in not supplied")
	}
	if !filepath.IsAbs(path) || !filepath.IsAbs(root) || !filepath.IsAbs(oraclePath) {
		t.Fatal("absolute whole source, fresh artifacts and independent oracle required")
	}
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	before, size := hmc333FullSourceHash(t, path)
	if before != "93b45bcca5c9e9b415478d81e176b633d70886328c49731b5258c806695de1ef" || size != 235422121 {
		t.Fatal("not the original complete Douyin capture")
	}
	data, err := os.ReadFile(oraclePath)
	if err != nil {
		t.Fatal(err)
	}
	var oracle struct {
		SourceSHA string `json:"source_sha256"`
		Accounts  []struct {
			Label, Name string
			StartLine   int     `json:"start_line"`
			EndLine     int     `json:"end_line"`
			Window      []int64 `json:"query_window_us"`
			Inclusive   int64   `json:"inclusive_wall_us"`
			Self        int64   `json:"self_wall_us"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal(data, &oracle); err != nil || oracle.SourceSHA != before {
		t.Fatal("wrong independent whole-source oracle", err)
	}
	ctx := &types.AgentContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize, Mutable: types.NewMutableState("Explain observed business wall and state accounts"), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Language: "zh", Intent: types.IntentExplain, PerfTrace: &types.PerfBundle{}}}}
	var selected []types.AnswerRuntimeMeasurementReceipt
	var refs []map[string]any
	var completeNotes []string
	write := func(name string, value any) {
		t.Helper()
		b, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), append(b, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for i, w := range [][2]float64{{236627.820748, 236628.490099}, {236628.031203, 236628.184158}} {
		params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "pid": 57874, "time_start": w[0], "time_end": w[1], "trace_flavor": "harmony_hitrace"})
		r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
		if err != nil || !r.Success {
			t.Fatalf("whole-source query: %v %s", err, r.Summary)
		}
		ctx.Mutable.AppendDispatchToolResult(r)
		ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
		var parent types.ObservationRecord
		var selfNote string
		matched := 0
		for _, row := range r.Observations {
			f, ok := tool.DecodeTraceBusinessTreeFact(row)
			if !ok {
				continue
			}
			for _, o := range oracle.Accounts {
				if len(o.Window) != 2 || float64(o.Window[0])/1e6 != w[0] || float64(o.Window[1])/1e6 != w[1] || f.Node.StartLine != o.StartLine || f.Node.EndLine != o.EndLine {
					continue
				}
				if f.Node.Inclusive == nil || f.Node.Self == nil || math.Abs(f.Node.Inclusive.DurationMs-float64(o.Inclusive)/1000) > 1e-6 || math.Abs(f.Node.Self.DurationMs-float64(o.Self)/1000) > 1e-6 {
					t.Fatal("query changed an independently measured wall account")
				}
				matched++
			}
		}
		if matched != 2 {
			t.Fatalf("query %d lost original parent/makeApplication accounts: %d", i, matched)
		}
		for _, row := range r.Observations {
			if row.Predicate != types.TraceBusinessSpanPredicate || row.Object != "bindApplication" {
				continue
			}
			p, ok := types.DecodeRuntimeMeasurementPublication(row)
			if !ok || len(p.Tables) != 1 || len(p.Tables[0].Rows) != 13 {
				t.Fatal("original native row coordinates changed")
			}
			for _, note := range p.Tables[0].Notes {
				if strings.HasPrefix(note, "自身账户（窗口内墙钟，不是运行时间）：") {
					selfNote = note
				}
			}
			parent = row
			break
		}
		want := "自身账户（窗口内墙钟，不是运行时间）：227.101 毫秒；"
		if i == 1 {
			want = "自身账户（窗口内墙钟，不是运行时间）：0 毫秒；"
		}
		if parent.ID == "" || !strings.HasPrefix(selfNote, want) {
			t.Fatalf("own query self account absent from native table notes: %s", selfNote)
		}
		selected = append(selected, types.AnswerRuntimeMeasurementReceipt{ObservationID: parent.ID, View: types.RuntimeMeasurementSummary})
		refs = append(refs, map[string]any{"observation_id": parent.ID, "view": "summary", "row_indices": []int{0, 2, 3, 4, 6, 7, 8}})
		completeNotes = append(completeNotes, selfNote)
		write([]string{"query-full.json", "query-clipped.json"}[i], r)
	}
	raw, _ := json.Marshal(map[string]any{"reason": "The current native accounts are retained with their own windows.", "confidence": "high", "result_kind": "resolved", "native_fact_handoff": map[string]any{"runtime_measurements": selected, "interpretation_role": "independent_analysis", "interpretation_units": []any{map[string]any{"text": "These are distinct query-owned accounts; preserve source windows and do not infer a response root cause from wall time.", "native_refs": refs}}}})
	completion, err := (&tool.EmitInvestigationComplete{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
	if err != nil || !completion.Success {
		t.Fatalf("public completion: %v %s", err, completion.Summary)
	}
	write("accepted-handoff.json", ctx.Mutable.AcceptedInvestigationNativeFactHandoff())
	for _, name := range []string{"extract.txt", "finalize.txt"} {
		prompt := (&extractorEvaluator{}).BuildInitialInstruction(ctx.ShallowClone(), nil)
		if name == "finalize.txt" {
			prompt = dependencyObservationMessages(t, ctx.ShallowClone())
		}
		// Retain the actual failed message too, before any assertion. Extraction
		// owns current references, not native publication; finalization owns
		// the complete selected producer tables and their source account notes.
		if err := os.WriteFile(filepath.Join(root, name), []byte(prompt), 0600); err != nil {
			t.Fatal(err)
		}
		if name == "extract.txt" {
			if !strings.Contains(prompt, "Native measurement references (rebind current observations; these do not certify publication, completeness or causality)") {
				t.Fatal("extraction lost its reference-only boundary")
			}
			for _, ref := range selected {
				if !strings.Contains(prompt, ref.ObservationID) {
					t.Fatal("extraction lost an exact current query reference")
				}
			}
			continue
		}
		for _, note := range completeNotes {
			if !strings.Contains(prompt, note) {
				t.Fatalf("actual %s lost the whole source-owned self account", name)
			}
		}
	}
	after, afterSize := hmc333FullSourceHash(t, path)
	if after != before || afterSize != size {
		t.Fatal("read-only real regression changed source")
	}
	write("receipt.json", map[string]any{"source": path, "source_bytes": size, "source_sha256": before, "source_unchanged": true, "production_queries": 2, "external_model_requests": 0, "independent_wall_accounts_matched": 4, "native_original_rows": 13, "finalizer_full_and_clipped_self_notes_published": true, "extractor_keeps_exact_reference_only_boundary": true, "causal_or_interpretation_acceptance": false})
}
