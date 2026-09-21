package render_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func renderBusinessReceipt(t *testing.T, input types.ObservationLedgerInput, id, lang string) string {
	t.Helper()
	contract := types.BuildRuntimeWorkRelationContract(input, true)
	receipt := &types.AnswerRuntimeWorkRelationReceipt{ObservationID: id, Conclusion: types.RuntimeWorkRelationConclusionRelationUnproven}
	if !types.BindRuntimeWorkRelationReceipt(receipt, contract) {
		t.Fatalf("public observation is not selectable: %q %+v", id, contract)
	}
	for _, stronger := range []types.RuntimeWorkRelationConclusion{types.RuntimeWorkRelationConclusionTargetSelfWorkObserved, types.RuntimeWorkRelationConclusionRelatedCausalityUnproven, types.RuntimeWorkRelationConclusionCausalContributionSupported} {
		if types.BindRuntimeWorkRelationReceipt(&types.AnswerRuntimeWorkRelationReceipt{ObservationID: id, Conclusion: stronger}, contract) {
			t.Fatalf("ordinary measurement acquired %s authority", stronger)
		}
	}
	wire, _ := json.Marshal(receipt)
	wantWire, _ := json.Marshal(map[string]string{"observation_id": id, "conclusion": "relation_unproven"})
	var got, want map[string]any
	_ = json.Unmarshal(wire, &got)
	_ = json.Unmarshal(wantWire, &want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("system measurement leaked to model wire: %s", wire)
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "work", Kind: types.BlockSummary, Text: "Model-owned analysis.", RuntimeWorkRelation: receipt}}}
	return render.RenderAnswerDocument(doc, lang)
}

func TestBusinessReceiptPublicQueryKeepsMeasurementWindows(t *testing.T) {
	fixture, err := os.ReadFile("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	for _, renamed := range []bool{false, true} {
		for _, tc := range []struct {
			name              string
			start, end, ms    float64
			clipped, explicit bool
		}{
			{"wakeup_clipped", 1.001, 1.05, 49, true, false},
			{"full_pair", 1, 1.051, 50, false, false},
			{"explicit_narrow", 1.01, 1.02, 10, true, true},
		} {
			t.Run(fmt.Sprintf("%s/renamed=%v", tc.name, renamed), func(t *testing.T) {
				name, trace := "OpenDocument", string(fixture)
				if renamed {
					name = "ArbitraryWork-Z"
					trace = strings.ReplaceAll(trace, "OpenDocument", name)
				}
				path := filepath.Join(t.TempDir(), "capture.systrace")
				if err := os.WriteFile(path, []byte(trace), 0600); err != nil {
					t.Fatal(err)
				}
				ctx := &types.AgentContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("runtime facts"), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeTargets: []types.RuntimeTarget{{PID: 100, Source: "user_explicit"}}}}}
				if tc.explicit {
					ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &tc.start, TimeEnd: &tc.end, SourceQuote: "explicit window"}
				}
				params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "wakeup_chain", "pid": 100, "time_start": tc.start, "time_end": tc.end, "trace_flavor": "harmony_hitrace"})
				result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
				if err != nil || !result.Success {
					t.Fatalf("public query failed: %v %+v", err, result)
				}
				var id string
				for _, r := range result.Observations {
					if r.Predicate == types.TraceBusinessSpanPredicate && r.Object == name {
						id = r.ID
						if r.Value != fmt.Sprintf("%.3f", tc.ms) {
							t.Fatalf("native in-window measurement changed: %+v", r)
						}
					}
				}
				before, _ := json.Marshal(result.Observations)
				input := types.ObservationLedgerInput{ToolResults: []types.ToolResult{result}, RequestModel: &ctx.AnalysisIR.RequestModel}
				for _, lang := range []string{"zh", "en"} {
					out := renderBusinessReceipt(t, input, id, lang)
					for _, want := range []string{name, fmt.Sprintf("%.3fms", tc.ms), fmt.Sprintf("%.6f–%.6f", tc.start, tc.end)} {
						if !strings.Contains(out, want) {
							t.Errorf("%s receipt lost query measurement %q:\n%s", lang, want, out)
						}
					}
					if tc.clipped && (!strings.Contains(out, "1.000000–1.050000") || !strings.Contains(out, "50.000ms")) {
						t.Errorf("%s receipt lost same-record full pair:\n%s", lang, out)
					}
					if lang == "zh" && (!strings.Contains(out, "查询窗内") || !strings.Contains(out, "因果贡献") || strings.Contains(out, "尚未建立它与目标的关系")) ||
						lang == "en" && (!strings.Contains(out, "in the query window") || !strings.Contains(out, "causal contribution") || strings.Contains(out, "its relation to the target is not established")) {
						t.Errorf("%s ordinary receipt has an unscoped measurement/relation:\n%s", lang, out)
					}
					if !tc.clipped && (strings.Contains(out, "原始配对") || strings.Contains(out, "full paired")) {
						t.Errorf("%s unclipped pair gained a fabricated second ruler:\n%s", lang, out)
					}
				}
				after, _ := json.Marshal(result.Observations)
				if string(before) != string(after) {
					t.Fatal("receipt rendering mutated native observations")
				}
			})
		}
	}
}

func TestBusinessReceiptMissingActualDoesNotBorrowOtherCapture(t *testing.T) {
	for _, notes := range [][]string{
		nil,
		{types.TraceNoteKeyActualWindow + "=1.000000..1.050000"},
		{types.TraceNoteKeyActualImpactMS + "=50.000"},
		{types.TraceNoteKeyActualWindow + "=1.001000..1.050000", types.TraceNoteKeyActualImpactMS + "=49.000"},
		{types.TraceNoteKeyActualWindow + "=1.000000..1.050000", types.TraceNoteKeyActualImpactMS + "=NaN"},
		{types.TraceNoteKeyActualWindow + "=1.002000..1.050000", types.TraceNoteKeyActualImpactMS + "=48.000"},
	} {
		selected := types.ObservationRecord{
			ID: "capture-a#span", Producer: "trace_query", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
			Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingHard,
			ProvenanceLane: types.ObservationProvenanceArtifactSpan,
			SourceRef:      types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: "/a.systrace", ArtifactID: "a"},
			Span:           types.ObservationSpan{LineStart: 4, LineEnd: 19, StartTs: 1.001, EndTs: 1.05},
			Predicate:      types.TraceBusinessSpanPredicate, Subject: "owner-100", Object: "SameName", Value: "49.000", Unit: "ms",
			RichNotes: append([]string{types.TraceNoteKeySelectedWindow + "=1.001000..1.050000"}, notes...),
		}
		other := selected
		other.ID, other.SourceRef.Path, other.SourceRef.ArtifactID = "capture-b#span", "/b.systrace", "b"
		other.RichNotes = []string{types.TraceNoteKeySelectedWindow + "=1.001000..1.050000", types.TraceNoteKeyActualWindow + "=1.000000..1.050000", types.TraceNoteKeyActualImpactMS + "=50.000"}
		input := types.ObservationLedgerInput{ToolResults: []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: []types.ObservationRecord{other, selected}}}}
		for _, lang := range []string{"zh", "en"} {
			out := renderBusinessReceipt(t, input, selected.ID, lang)
			if !strings.Contains(out, "49.000ms") || !strings.Contains(out, "1.001000–1.050000") || strings.Contains(out, "50.000ms") || strings.Contains(out, "原始配对") || strings.Contains(out, "full paired") {
				t.Errorf("optional metadata borrowed another record/capture: notes=%v\n%s", notes, out)
			}
		}
		contract := types.BuildRuntimeWorkRelationContract(input, true)
		var schema map[string]any
		if err := json.Unmarshal(tool.BuildAnswerDocumentParametersFor(&types.AnswerSemanticView{RuntimeWorkRelationContract: contract}), &schema); err != nil {
			t.Fatal(err)
		}
		props := schema["properties"].(map[string]any)["blocks"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
		choices := props["runtime_work_relation"].(map[string]any)["oneOf"].([]any)
		if len(choices) != 2 {
			t.Fatalf("measurement changed choice identity: %+v", choices)
		}
		for _, choice := range choices {
			fields := choice.(map[string]any)["properties"].(map[string]any)
			if len(fields) != 2 || fields["observation_id"] == nil || fields["conclusion"].(map[string]any)["const"] != "relation_unproven" {
				t.Fatalf("measurement expanded model schema/authority: %+v", fields)
			}
		}
	}
}
