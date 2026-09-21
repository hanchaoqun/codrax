package agent

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// This tests the downstream handoff after a coherent analyzer tuple has been
// accepted. It does NOT test whether a model classifies the natural-language
// eval question correctly. In particular, collected root rows, completion
// prose, and a relation_path presentation dimension do not grant causal scope.
func TestCausalFiniteNativeTraceFinalizerHandoff(t *testing.T) {
	caseBytes, err := os.ReadFile("../../eval/cases/trace_query_wakeup_causal_io_chain.case")
	if err != nil {
		t.Fatal(err)
	}
	_, fixture, ok := strings.Cut(string(caseBytes), "HTRACE='")
	if !ok {
		t.Fatal("native trace fixture absent")
	}
	fixture, _, ok = strings.Cut(fixture, "\n'\n")
	if !ok {
		t.Fatal("native trace fixture not closed")
	}
	_, causalQuestion, ok := strings.Cut(string(caseBytes), "QUESTION=\"")
	if !ok {
		t.Fatal("native causal question absent")
	}
	causalQuestion, _, ok = strings.Cut(causalQuestion, "\"\n")
	if !ok {
		t.Fatal("native causal question not closed")
	}
	for _, renamed := range []bool{false, true} {
		t.Run(fmt.Sprintf("renamed=%t", renamed), func(t *testing.T) {
			trace := fixture
			question := causalQuestion
			names := []string{"app-100", "cookie-200", "network-300", "threadpool-400"}
			if renamed {
				replacer := strings.NewReplacer("app", "client", "cookie", "broker", "network", "transport", "threadpool", "worker")
				trace, question = replacer.Replace(trace), replacer.Replace(question)
				names = []string{"client-100", "broker-200", "transport-300", "worker-400"}
			}
			dir := t.TempDir()
			path := filepath.Join(dir, "chain.ftrace")
			if err := os.WriteFile(path, []byte(trace), 0600); err != nil {
				t.Fatal(err)
			}
			start, end := 2.0, 2.020
			queryBus := &types.BusContext{RepoRoot: dir, WorkDir: dir}
			var results []types.ToolResult
			for _, view := range []string{"thread_timeline", "wakeup_chain", "root_cause_rank"} {
				params, err := json.Marshal(map[string]any{
					"source": "path", "path": path, "view": view, "pid": 100,
					"time_start": start, "time_end": end, "trace_flavor": "harmony_hitrace",
				})
				if err != nil {
					t.Fatal(err)
				}
				result, err := (&tool.TraceQuery{}).Execute(queryBus, params)
				if err != nil || !result.Success {
					t.Fatalf("public %s query failed: %v; %s", view, err, result.Summary)
				}
				results = append(results, result)
			}
			// Every lane receives the identical producer results. No rewritten
			// observation, model aggregate, or free-form completion reason is
			// supplied to make the positive handoff assertion pass.
			var baselineLedger string
			for _, scope := range []types.RuntimeQuestionScope{
				types.RuntimeQuestionScopeCausalDiagnosis,
				types.RuntimeQuestionScopeBoundedFactSet,
				types.RuntimeQuestionScopeBoundedEffectVerdict,
			} {
				t.Run(string(scope), func(t *testing.T) {
					rawRequest := question
					scopeQuote := "主要阻塞原因"
					if scope == types.RuntimeQuestionScopeBoundedFactSet {
						rawRequest = "请仅查看 2.000s 到 2.020s 的窗口，目标线程是 " + names[0] + "，给出调度状态分布。"
						scopeQuote = "调度状态"
					} else if scope == types.RuntimeQuestionScopeBoundedEffectVerdict {
						rawRequest = "请查看 2.000s 到 2.020s 的窗口，目标线程是 " + names[0] + "，判断该线程的调度优先级是否限制其执行，并给出调度状态分布。"
						scopeQuote = "调度优先级是否限制其执行"
					}
					rm := types.RequestModel{
						RawRequest: rawRequest, Language: "en", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric,
						RuntimeTargetProfile: &types.RuntimeTargetProfile{
							Declaration: types.RuntimeTargetDeclarationNamedTarget,
							SourceQuote: "目标线程是 " + names[0], Confidence: 0.95,
						},
						RuntimeTargets: []types.RuntimeTarget{{
							Kind: types.RuntimeTargetKindThread, PID: 100, Thread: names[0], Source: "user_explicit", Confidence: 0.95,
						}},
						RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{
							RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
							TimeStart:      &start, TimeEnd: &end, SourceQuote: "2.000s 到 2.020s", Confidence: 0.95,
						},
						RuntimeQuestionProfile: &types.RuntimeQuestionProfile{
							Scope: scope, RuntimeWorkRelationRequested: false, FrameCausalityRequested: false,
							SourceQuote: scopeQuote, Confidence: 0.95,
						},
						RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
							IsDimensionedAnswer: true, Confidence: 0.95,
							Dimensions: []types.RequestedAnswerDimension{{
								Index: 1, Label: "Target scheduler state", Role: types.RequestedAnswerDimensionObservedValue,
								Required: true, SourceQuote: "调度状态",
							}},
						},
					}
					if scope == types.RuntimeQuestionScopeCausalDiagnosis {
						rm.RequestedAnswerDimensions.Dimensions = append(rm.RequestedAnswerDimensions.Dimensions,
							types.RequestedAnswerDimension{Index: 2, Label: "Blocking cause", Role: types.RequestedAnswerDimensionCausalAttribution, Required: true, SourceQuote: "主要阻塞原因"},
							types.RequestedAnswerDimension{Index: 3, Label: "Related path", Role: types.RequestedAnswerDimensionRelationPath, Required: true, SourceQuote: "相关链路"})
					} else {
						rm.RuntimeQuestionProfile.FactFamilies = []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState}
						if scope == types.RuntimeQuestionScopeBoundedEffectVerdict {
							rm.RequestedAnswerDimensions.Dimensions = append(rm.RequestedAnswerDimensions.Dimensions,
								types.RequestedAnswerDimension{Index: 2, Label: "Target effect", Role: types.RequestedAnswerDimensionTargetEffectVerdict, Required: true, SourceQuote: "调度优先级是否限制其执行"})
						}
					}
					// Explicit focus and window are independently required.
					// The minimal causal tuple must not need root intent or
					// duplicate diagnostic classifier flags to unlock its data.
					hmcAssertHandoffTupleAdmission(t, rm, names[0])
					mu := types.NewMutableState(rawRequest)
					mu.SetRequestModel(rm)
					mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
					ir := &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "en"}}
					bus := &types.BusContext{
						RepoRoot: dir, WorkDir: dir, Language: "en", Mutable: mu, AnalysisIR: ir,
						RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{Artifacts: []types.RuntimeArtifactPreflightArtifact{{
							Kind: "trace", Source: path, Carrier: "path",
						}}},
					}
					ctx := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
					ledger := answerDocObservationLedger(ctx)
					set := types.CompileTraceCausalProjectionSet(ledger)
					hmcAssertNativeCausalHandoffInputs(t, set, names, start, end)
					// Scope controls publication, not deletion or re-authoring
					// of the lossless measurements retained for audit.
					ledgerBytes := hmcCausalHandoffSnapshot(t, ledger)
					if baselineLedger == "" {
						baselineLedger = ledgerBytes
					} else if baselineLedger != ledgerBytes {
						t.Fatal("question breadth changed the lossless input ledger")
					}
					before := hmcCausalHandoffSnapshot(t, []any{results, ledger, set, rm, mu.AnswerDocumentV2()})
					instruction := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
					after := hmcCausalHandoffSnapshot(t, []any{results, answerDocObservationLedger(ctx), types.CompileTraceCausalProjectionSet(answerDocObservationLedger(ctx)), ctx.AnalysisIR.RequestModel, mu.AnswerDocumentV2()})
					if before != after {
						t.Fatal("building the finalizer instruction mutated measurements, scope, projection, or model-owned answer")
					}
					if scope != types.RuntimeQuestionScopeCausalDiagnosis {
						for _, forbidden := range []string{
							"## Trace Decision Inputs (Model Owns The Conclusion)",
							"## Final Trace Decision Boundary", "axis_B_existing_rule_eliminable",
							"fscache_page_wait_on_page_bit", "11.000ms",
						} {
							if strings.Contains(instruction, forbidden) {
								t.Errorf("finite question gained unrequested root-cause handoff %q", forbidden)
							}
						}
						hmcAssertHandoffLine(t, instruction, "- principal_state:",
							"target=\x60"+names[0]+"\x60", "window=\x602.000000..2.020000\x60",
							"running=0.000ms", "runnable=0.000ms", "sleep=20.000ms",
							"d_state=0.000ms", "io_wait=0.000ms", "accounted_total=20.000ms")
						return
					}
					for _, want := range []string{
						"## Trace Decision Inputs (Model Owns The Conclusion)",
						"## Final Trace Decision Boundary",
						"selected_window=\x602.000000..2.020000\x60; window_ms=20.000",
						"Confirmed wakeup dependency path: \x60" + strings.Join([]string{names[3], names[2], names[1], names[0]}, " -> ") + "\x60",
					} {
						if !strings.Contains(instruction, want) {
							t.Errorf("authorized causal handoff lost %q", want)
						}
					}
					hmcAssertHandoffLine(t, instruction, "  - rank=#",
						"subject=\x60"+names[3]+"\x60", "kind=\x60io_wait\x60", "effective_attribution=11.000ms",
						"blocked_reason_caller=\x60fscache_page_wait_on_page_bit\x60")
					// These are three separate one-millisecond seats, not a
					// three-millisecond sum or a full explanation of 20ms.
					for _, subject := range names[1:] {
						hmcAssertHandoffLine(t, instruction, "  - rank=#",
							"subject=\x60"+subject+"\x60", "kind=\x60priority_inversion_candidate\x60", "effective_attribution=1.000ms")
					}
				})
			}
		})
	}
}

// Assert fixture admission and exercise the shared public authority consumers.
// This is not a replacement for emit_analysis's parser, which is not invoked
// in this handoff-only test. In particular, the target quote belongs to
// RuntimeTargetProfile; RuntimeTarget has no SourceQuote field.
func hmcAssertHandoffTupleAdmission(t *testing.T, rm types.RequestModel, target string) {
	t.Helper()
	normalized, warnings := types.NormalizeRequestedAnswerDimensionProfile(rm.RawRequest, rm.RequestedAnswerDimensions)
	if len(warnings) != 0 || !reflect.DeepEqual(normalized, rm.RequestedAnswerDimensions) {
		t.Fatalf("fixture dimensions did not survive the production normalization unchanged: %v", warnings)
	}
	anchors, decided := types.RuntimeUserTargetAnchorEntities(&rm)
	wantAnchors := []types.AnchorUserEntity{{Value: "100", TypedLane: true}, {Value: target, TypedLane: true}}
	if !decided || !rm.RuntimeTargetProfile.NamedTarget() || !reflect.DeepEqual(anchors, wantAnchors) {
		t.Fatal("fixture must use explicit named-target authority, never legacy nil-profile or query-cursor authority")
	}
	if !rm.RuntimeArtifactScopeProfile.HasExplicitTimeWindows() {
		t.Fatal("fixture must carry the independently explicit current-request window")
	}
	for _, quote := range []string{
		rm.RuntimeTargetProfile.SourceQuote,
		rm.RuntimeArtifactScopeProfile.SourceQuote,
		rm.RuntimeQuestionProfile.SourceQuote,
	} {
		if quote == "" || !strings.Contains(rm.RawRequest, quote) {
			t.Fatalf("fixture profile quote is not verbatim in the current request: %q", quote)
		}
	}
	var roles []types.RequestedAnswerDimensionRole
	for i, dimension := range rm.RequestedAnswerDimensions.Dimensions {
		if !dimension.Required || dimension.Index != i+1 || dimension.SourceQuote == "" || !strings.Contains(rm.RawRequest, dimension.SourceQuote) {
			t.Fatalf("fixture dimension lost current-request admission: %+v", dimension)
		}
		roles = append(roles, dimension.Role)
	}
	wantRoles := []types.RequestedAnswerDimensionRole{types.RequestedAnswerDimensionObservedValue}
	wantFamilies := []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState}
	causal := rm.RuntimeQuestionProfile.Scope == types.RuntimeQuestionScopeCausalDiagnosis
	if causal {
		wantRoles = append(wantRoles, types.RequestedAnswerDimensionCausalAttribution, types.RequestedAnswerDimensionRelationPath)
		wantFamilies = nil
	} else if rm.RuntimeQuestionProfile.Scope == types.RuntimeQuestionScopeBoundedEffectVerdict {
		wantRoles = append(wantRoles, types.RequestedAnswerDimensionTargetEffectVerdict)
	}
	if !reflect.DeepEqual(roles, wantRoles) || !reflect.DeepEqual(rm.RuntimeQuestionProfile.FactFamilies, wantFamilies) ||
		rm.RuntimeQuestionProfile.RuntimeWorkRelationRequested || rm.RuntimeQuestionProfile.FrameCausalityRequested ||
		rm.Intent != types.IntentExplain || rm.Scenario != types.ScenarioGeneric ||
		rm.Predicates.IsDiagnosticQuestion || rm.DiagnosticProfile.RequiresDiagnosticRootCause() {
		t.Fatal("fixture must preserve the coherent causal or finite post-validation tuple")
	}
	if decided, allowed := types.RuntimeTraceReportShapeAuthority(&rm); !decided || allowed != causal {
		t.Fatal("public trace report authority disagrees with the admitted fixture tuple")
	}
}

func hmcAssertNativeCausalHandoffInputs(t *testing.T, set types.TraceCausalProjectionSet, names []string, start, end float64) {
	t.Helper()
	ioFound, pathFound := false, false
	runnableSubjects := map[string]bool{}
	for _, projection := range set.Projections {
		if math.Abs(projection.WindowStartTs-start) > 1e-9 || math.Abs(projection.WindowEndTs-end) > 1e-9 {
			t.Fatalf("public query changed the selected window: %.9f..%.9f", projection.WindowStartTs, projection.WindowEndTs)
		}
		pathFound = pathFound || reflect.DeepEqual(projection.WakeupPath, []string{names[3], names[2], names[1], names[0]})
		for _, node := range projection.RankedSeats {
			if node.Subject == names[3] && node.TypeToken == "io_wait" &&
				math.Abs(node.EffectiveImpactMS-11) < 1e-6 &&
				node.BlockedReasonCaller == "fscache_page_wait_on_page_bit" {
				ioFound = true
			}
			if node.TypeToken == "priority_inversion_candidate" && math.Abs(node.EffectiveImpactMS-1) < 1e-6 {
				runnableSubjects[node.Subject] = true
			}
		}
	}
	if !ioFound || !pathFound {
		t.Fatalf("fixture admission failed before testing handoff: IO=%t path=%t", ioFound, pathFound)
	}
	for _, subject := range names[1:] {
		if !runnableSubjects[subject] {
			t.Fatalf("fixture admission failed: missing producer one-millisecond seat for %s", subject)
		}
	}
}

func hmcAssertHandoffLine(t *testing.T, instruction, prefix string, fields ...string) {
	t.Helper()
	for _, line := range strings.Split(instruction, "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		matches := true
		for _, field := range fields {
			matches = matches && strings.Contains(line, field)
		}
		if matches {
			return
		}
	}
	t.Errorf("actual finalizer instruction has no %q line carrying %q", prefix, fields)
}

func hmcCausalHandoffSnapshot(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
