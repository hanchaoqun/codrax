package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Language policy must survive the native tool/supplement/publication path;
// translating a synthetic appendix alone cannot catch conflicting carriers.
func TestAnswerDocumentLocalePublicQuerySupplementEmitRenderPatch(t *testing.T) {
	for _, tc := range []struct {
		name, project, contract, request, want string
	}{
		{"project_zh_over_contract_en", "zh", "en", "en", "zh"},
		{"project_en_over_contract_zh", "en", "zh", "zh", "en"},
		{"auto_contract_zh", "auto", "zh", "en", "zh"},
		{"auto_contract_en", "auto", "en", "zh", "en"},
		{"unset_contract_en", "", "en", "zh", "en"},
		{"auto_request_zh", "auto", "", "zh", "zh"},
		{"disabled_contract_zh", "off", "zh", "zh", "en"},
		{"project_chinese_alias", "简体中文", "en", "en", "zh"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, types.AttachedTraceBlobBasename)
			original := traceWaitBucketNativeCycle(1, "D", 1)
			if err := os.WriteFile(path, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			start, end := 1.0, 1.004
			const request = "Report scheduler states and waits of reader-77 in 1.000000..1.004000"
			rm := types.RequestModel{
				RawRequest: request, Language: tc.request, Intent: types.IntentExplain, Scenario: types.ScenarioGeneric,
				RuntimeTargets:       []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 77, Thread: "reader-77", Source: "user_explicit", Confidence: 1}},
				RuntimeTargetProfile: &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: "reader-77", Confidence: 1},
				RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{
					RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1.000000..1.004000", Confidence: 1,
				},
				RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet,
					FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState, types.RuntimeQuestionFactTargetWaitOccurrences}, SourceQuote: "scheduler states and waits", Confidence: 1},
			}
			mu := types.NewMutableState(request)
			mu.SetRequestModel(rm)
			bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, Language: tc.project, Mutable: mu,
				AnalysisIR:               &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: tc.contract}},
				RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: path, Carrier: "path"}}},
			}
			result, err := (&TraceQuery{}).Execute(bus, json.RawMessage(`{"view":"event_search","pattern":"reader","pid":77,"time_start":1,"time_end":1.004}`))
			if err != nil || !result.Success {
				t.Fatalf("native discovery query failed: %v; %s", err, result.Summary)
			}
			bus.ToolResults = append(bus.ToolResults, result)
			out := RunTraceQuerySystemSupplement(bus)
			if !reflect.DeepEqual(out.Executed, []string{"window_stats"}) || mu.SystemTraceSupplementMeta() == nil || len(mu.SystemTraceSupplementResults()) != 1 {
				t.Fatalf("fixture must execute the real finite-state supplement: %+v; meta=%+v", out, mu.SystemTraceSupplementMeta())
			}
			ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
			waits := types.BuildTraceTargetWaitSummaryAuthorities(ledger, &rm)
			if len(waits) != 1 || waits[0].Count != 1 || waits[0].IOWaitOccurrences != 1 || waits[0].DStateOccurrences != 0 || waits[0].SleepIOWaitOccurrences != 0 || fmt.Sprintf("%.3f", waits[0].WallClockMS) != "1.000" || waits[0].WindowStartTs != start || waits[0].WindowEndTs != end {
				t.Fatalf("native wait classification, count, measurement or window changed: %+v", waits)
			}
			snapshot := func() string {
				t.Helper()
				current := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
				data, err := json.Marshal([]any{bus.ToolResults, current, types.CompileTraceCausalProjectionSet(current), bus.AnalysisIR, mu.RequestModel(), mu.SystemTraceSupplementMeta(), mu.SystemTraceSupplementResults()})
				if err != nil {
					t.Fatal(err)
				}
				return string(data)
			}
			before := snapshot()
			const model = "模型自己的业务判断。 Model-owned conclusion."
			const caveat = "模型自己的保留说明。 Model-owned caveat."
			payload, _ := json.Marshal(map[string]any{"blocks": []map[string]any{{"id": "model_summary", "kind": "summary", "text": model}}, "caveats": []string{caveat}})
			emit, err := (&EmitAnswerDocument{}).Execute(bus, payload)
			if err != nil || !emit.Success {
				t.Fatalf("public emit failed: %v; %s", err, emit.Summary)
			}
			check := func() string {
				t.Helper()
				doc := mu.AnswerDocumentV2()
				count, modelCount := 0, 0
				var appendix string
				for _, block := range doc.Blocks {
					if block.ID == runtimeTraceTargetStateAuthorityBlockID {
						count++
						appendix = block.Text
					}
					if block.ID == "model_summary" {
						modelCount++
						if block.Text != model {
							t.Error("language selection rewrote model-owned prose")
						}
					}
				}
				if count != 1 || modelCount != 1 {
					t.Fatalf("system appendix/model duplication: %d/%d", count, modelCount)
				}
				prefix, wrongPrefix := runtimeTraceSupplementDisclosurePrefixEN, runtimeTraceSupplementDisclosurePrefixZH
				labels := []string{"original scheduler state: D", "accounting category:", "non-IO D-state 0, scheduler-marked IO wait 1, interruptible sleep carrying an IO-wait marker 0"}
				if tc.want == "zh" {
					prefix, wrongPrefix = wrongPrefix, prefix
					labels = []string{"原始调度状态：D", "统计类别：", "非 IO D-state 0、调度器标记的 IO 等待 1、带 IO 等待标记的可中断睡眠 0"}
				}
				for _, label := range labels {
					if !strings.Contains(appendix, label) {
						t.Errorf("system wait inventory must use effective language %s; missing %q:\n%s", tc.want, label, appendix)
					}
				}
				if !strings.Contains(appendix, "1.000ms") || strings.Count(appendix, "\n- ") != 1 {
					t.Errorf("system wait inventory lost its native count/measurement: %s", appendix)
				}
				disclosures, modelCaveats := 0, 0
				for _, got := range doc.Caveats {
					if strings.HasPrefix(got, prefix) {
						disclosures++
					}
					if strings.HasPrefix(got, wrongPrefix) {
						t.Errorf("system supplement uses conflicting language: %s", got)
					}
					if got == caveat {
						modelCaveats++
					}
				}
				if disclosures != 1 || modelCaveats != 1 {
					t.Errorf("effective-language supplement/model caveat missing or duplicated: %d/%d", disclosures, modelCaveats)
				}
				published := render.RenderAnswerDocument(doc, tc.want)
				if !strings.Contains(published, model) || !strings.Contains(published, caveat) || !strings.Contains(published, prefix) || !strings.Contains(published, labels[0]) {
					t.Error("rendered output did not retain model prose and effective-language system surfaces")
				}
				if strings.Contains(published, "Trace 因果投影") || strings.Contains(published, "Trace causal projection") {
					t.Error("locale resolution expanded finite wait facts into causal authority")
				}
				return published
			}
			published, docBeforePatch := check(), mu.AnswerDocumentV2()
			patch, err := (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{"unchanged_block_ids":["model_summary"]}`))
			if err != nil || !patch.Success {
				t.Fatalf("public no-op patch failed: %v; %s", err, patch.Summary)
			}
			if check() != published || !reflect.DeepEqual(docBeforePatch, mu.AnswerDocumentV2()) {
				t.Error("no-op patch changed or duplicated the published document")
			}
			if snapshot() != before {
				t.Error("publication mutated raw evidence, observation ledger, projection, language declaration or supplement metadata")
			}
			rawAfter, err := os.ReadFile(path)
			if err != nil || string(rawAfter) != original {
				t.Error("publication changed the native trace bytes")
			}
		})
	}
}
