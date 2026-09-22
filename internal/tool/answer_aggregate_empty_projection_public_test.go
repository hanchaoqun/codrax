package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// An existing empty projection is an authority decision, not an absent plan.
// Exercise both reads and the real emit/patch advisory lane after a native
// query; checking only the types-level projection cannot detect resurrection.
func TestAnswerAggregateEmptyProjectionPublicQueryEmitRenderPatch(t *testing.T) {
	for _, tc := range []struct {
		name, state    string
		start          float64
		native, source bool
	}{
		{"empty_zero_origin_D_IO", "D", 0, true, false},
		{"empty_positive_S_IO", "S", 1, true, false},
		{"mixed_retains_independent_source", "D|K", 2, true, true},
		{"no_native_preserves_model_handoff", "D", 3, false, false},
	} {
		for _, lang := range []string{"zh", "en"} {
			t.Run(tc.name+"/"+lang, func(t *testing.T) {
				dir := t.TempDir()
				path := filepath.Join(dir, "native.systrace")
				if err := os.WriteFile(path, []byte(traceWaitBucketNativeCycle(tc.start, tc.state, 1)), 0600); err != nil {
					t.Fatal(err)
				}
				end := tc.start + .004
				query, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "thread_timeline", "pid": 77, "trace_flavor": "harmony_hitrace", "time_start": tc.start, "time_end": end})
				result, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, query)
				if err != nil || !result.Success {
					t.Fatalf("native query failed: %v; %s", err, result.Summary)
				}
				rm := types.RequestModel{
					RawRequest: "Report the exact scheduler wait measurements for reader-77 in the selected window", Language: lang,
					Intent: types.IntentExplain, Scenario: types.ScenarioGeneric,
					RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 77, Thread: "reader-77", Source: "user_explicit", Confidence: 1}},
					RuntimeTargetProfile:        &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: "reader-77", Confidence: 1},
					RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &tc.start, TimeEnd: &end, SourceQuote: fmt.Sprintf("%.6f..%.6f", tc.start, end), Confidence: 1},
					RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState, types.RuntimeQuestionFactTargetWaitOccurrences}, SourceQuote: "scheduler wait measurements", Confidence: 1},
				}
				// Exact-output is typed, not inferred from the model's value text.
				rm.Predicates.IsScalarAnswer = true
				facts := []types.AnswerAggregateFact{
					{Kind: types.AnswerAggregateScalar, Label: "provisional runtime scalar", Value: "987654.321", Unit: "ms", Role: types.AnswerAggregateRolePrincipalAnswer, Dimensions: []types.AnswerAggregateDimension{{Name: "origin", Value: "runtime_artifact"}}},
					{Kind: types.AnswerAggregateScalar, Label: "provisional inferred scalar", Value: "876543.210", Unit: "ms", Role: types.AnswerAggregateRolePrincipalAnswer, Dimensions: []types.AnswerAggregateDimension{{Name: "origin", Value: "system_inference"}}},
				}
				const sourceValue = "RetainedSourceMarker"
				if tc.source {
					facts = append(facts, types.AnswerAggregateFact{Kind: types.AnswerAggregateScalar, Label: "independent source constant", Value: sourceValue, Role: types.AnswerAggregateRolePrincipalAnswer, SupportRefs: []string{"reader.go:2"}, Dimensions: []types.AnswerAggregateDimension{{Name: "origin", Value: "current_source"}}})
					if err := os.WriteFile(filepath.Join(dir, "reader.go"), []byte("package reader\nconst Marker = \"RetainedSourceMarker\"\n"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				mu := types.NewMutableState(rm.RawRequest)
				mu.SetRequestModel(rm)
				mu.SetInvestigationAggregateFacts(facts)
				mu.RetainInvestigationAggregateFacts()
				// The negative control has no delivered native observation. A
				// request/profile alone cannot exclude its only model handoff.
				if tc.native {
					mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
				}
				bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, Language: lang, Mutable: mu,
					AnalysisIR:               &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: lang}},
					RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: path, Carrier: "path"}}},
				}
				if tc.source {
					bus.EvidenceItems = []types.EvidenceItem{{ID: "source-marker", Kind: types.EvidenceDirect, Source: "reader.go", LineStart: 2, LineEnd: 2, AnchorKind: types.AnchorDefinition, Snippet: "const Marker = \"RetainedSourceMarker\"", GroundingStatus: types.GroundingGrounded}}
				}
				ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
				if ledger.HasDeterministicRuntimeQueryObservation() != tc.native {
					t.Fatal("control did not establish the requested native-evidence boundary")
				}
				if tc.native {
					waits := types.BuildTraceTargetWaitSummaryAuthorities(ledger, &rm)
					if len(waits) != 1 || waits[0].Count != 1 || fmt.Sprintf("%.3f", waits[0].WallClockMS) != "1.000" || waits[0].WindowStartTs != tc.start || waits[0].WindowEndTs != end {
						t.Fatalf("real native account not established: %+v", waits)
					}
				}
				snapshot := func() string {
					currentLedger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
					data, err := json.Marshal([]any{mu.StableInvestigationAggregateFacts(), currentLedger, bus.AnalysisIR.RequestModel, bus.EvidenceItems, result})
					if err != nil {
						t.Fatal(err)
					}
					return string(data)
				}
				before := snapshot()
				plan := answerSurfacePlan(bus)
				if plan == nil {
					t.Fatal("typed bus must produce a surface plan")
				}
				wantCount := len(facts)
				if tc.native {
					wantCount = 0
					if tc.source {
						wantCount = 1
					}
				}
				if len(plan.StableAggregateFacts) != wantCount || (tc.native && tc.source && plan.StableAggregateFacts[0].Value != sourceValue) {
					t.Fatalf("types projection itself is wrong: %+v", plan.StableAggregateFacts)
				}
				checkReads := func() {
					t.Helper()
					if got := preEmitStableAggregateFacts(bus); !reflect.DeepEqual(got, plan.StableAggregateFacts) {
						t.Errorf("direct read resurrected facts outside the existing projection: got=%+v want=%+v", got, plan.StableAggregateFacts)
					}
					cache := newPreEmitCheckContext(bus)
					for i := 0; i < 3; i++ {
						if got := cache.stableAggregateFactsForCheck(); !reflect.DeepEqual(got, plan.StableAggregateFacts) {
							t.Errorf("cached read %d resurrected facts outside the existing projection: got=%+v want=%+v", i, got, plan.StableAggregateFacts)
						}
					}
					if !cache.stableFactsExcluded || cache.derivedBuilds.surfacePlan != 1 || cache.derivedBuilds.stableAggregateFacts != 1 {
						t.Errorf("existing projected plan must own the cached result, including empty: %+v excluded=%t", cache.derivedBuilds, cache.stableFactsExcluded)
					}
				}
				checkReads()
				model := "模型自己的解释原样保留。 Model-owned explanation remains unchanged."
				if tc.source {
					model += " " + sourceValue
				}
				payload, _ := json.Marshal(map[string]any{"blocks": []map[string]any{{"id": "model_summary", "kind": "summary", "text": model}}})
				logDir := filepath.Join(dir, "public-advisories")
				logger, err := logging.NewFromFlags(logDir, "debug", false)
				if err != nil {
					t.Fatal(err)
				}
				previousLogger := logging.Default
				logging.SetDefault(logger)
				defer func() { logging.SetDefault(previousLogger); _ = logger.Close() }()
				emitted, err := (&EmitAnswerDocument{}).Execute(bus, payload)
				if err != nil || !emitted.Success {
					t.Fatalf("public emit failed: %v; %s", err, emitted.Summary)
				}
				checkDoc := func() string {
					t.Helper()
					doc := mu.AnswerDocumentV2()
					modelCount := 0
					for _, block := range doc.Blocks {
						if block.ID == "model_summary" {
							modelCount++
							if block.Text != model {
								t.Error("system rewrote model-owned explanation")
							}
						}
					}
					visible := render.RenderAnswerDocument(doc, lang)
					if modelCount != 1 || !strings.Contains(visible, model) {
						t.Fatal("model-owned block lost or duplicated")
					}
					for _, fact := range facts[:2] {
						if strings.Contains(visible, fact.Value) {
							t.Errorf("system inserted provisional aggregate %q", fact.Value)
						}
					}
					return visible
				}
				visible := checkDoc()
				docBeforePatch := mu.AnswerDocumentV2()
				patched, err := (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{"unchanged_block_ids":["model_summary"]}`))
				if err != nil || !patched.Success {
					t.Fatalf("public no-op patch failed: %v; %s", err, patched.Summary)
				}
				if checkDoc() != visible || !reflect.DeepEqual(docBeforePatch, mu.AnswerDocumentV2()) {
					t.Error("no-op patch changed publication")
				}
				checkReads()
				var advisories strings.Builder
				entries, err := os.ReadDir(logDir)
				if err != nil {
					t.Fatal(err)
				}
				for _, entry := range entries {
					data, err := os.ReadFile(filepath.Join(logDir, entry.Name()))
					if err != nil {
						t.Fatal(err)
					}
					for _, line := range strings.Split(string(data), "\n") {
						if strings.Contains(line, "accepted as soft advisory") {
							advisories.WriteString(line + "\n")
						}
					}
				}
				for _, fact := range facts[:2] {
					mentioned := strings.Contains(advisories.String(), fact.Value)
					if mentioned == tc.native {
						t.Errorf("public emit/patch advisory resurrected an excluded value or lost the no-native handoff: value=%q native=%t advisories=%s", fact.Value, tc.native, advisories.String())
					}
				}
				if snapshot() != before {
					t.Error("projection/emit/patch changed original Mutable facts, ledger, window, query result or evidence")
				}
			})
		}
	}
}
