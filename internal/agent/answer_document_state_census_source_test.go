package agent

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func b1638StateCensusContext(results []types.ToolResult) *types.AgentContext {
	ctx := answerDocCausalCeilingTestContext(false)
	start, end := 13762.791708, 13763.024898
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.FrameCausalityRequested = false
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end}
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 2955, Thread: "CompThread_0-2955", Source: "user_explicit"}}
	ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
	ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "model-summary", Kind: types.BlockSummary, Text: "Model prose remains unchanged. 模型自己的正文不变。"}}})
	for _, result := range results {
		ctx.Mutable.AppendDispatchToolResult(result)
	}
	return ctx
}

func b1638StateCensusLines(prompt string) []string {
	var out []string
	for _, line := range strings.Split(prompt, "\n") {
		if strings.HasPrefix(line, "- blocked_reason_state_relation ") {
			out = append(out, line)
		}
	}
	return out
}

func TestB1638ActualStateCensusQuerySource(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/real_traces/donghu.ftrace")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	query := func(filtered bool) types.ToolResult {
		params := map[string]any{"source": "path", "path": path, "view": "root_cause_rank", "thread": "CompThread_0-2955", "time_start": 13762.791708, "time_end": 13763.024898}
		if filtered {
			params["line_start"], params["line_end"] = 1, 400
		}
		raw, _ := json.Marshal(params)
		result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, raw)
		if err != nil || !result.Success {
			t.Fatalf("actual query: %v %s", err, result.Summary)
		}
		return result
	}
	main, filtered := query(false), query(true)
	for _, tc := range []struct {
		name    string
		results []types.ToolResult
	}{
		{"main", []types.ToolResult{main}}, {"filtered", []types.ToolResult{filtered}},
		{"repeat", []types.ToolResult{main, main}},
		{"main_then_filtered", []types.ToolResult{main, filtered}},
		{"filtered_then_main", []types.ToolResult{filtered, main}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := b1638StateCensusContext(tc.results)
			ctx.RepoRoot, ctx.WorkDir = dir, dir
			ledger := answerDocObservationLedger(ctx)
			set := types.CompileTraceCausalProjectionSet(ledger)
			if len(set.Projections) != 1 || set.Projections[0].TargetStateAccount == nil {
				t.Fatal("missing actual selected account")
			}
			account := set.Projections[0].TargetStateAccount
			var source *types.ObservationRecord
			for i := range ledger.Records {
				if ledger.Records[i].ID == account.EvidenceID {
					source = &ledger.Records[i]
				}
			}
			if source == nil || source.SourceRef.QueryScopeID == "" {
				t.Fatal("selected account has no producer receipt")
			}
			var count string
			for _, r := range ledger.Records {
				if r.Predicate == "blocked_reason_census" && r.Subject == account.Subject && types.TraceRuntimeAccountRecordsSameResult(*source, r) {
					count = r.Value
				}
			}
			if count == "" {
				t.Fatal("own census missing from actual result")
			}
			before, _ := json.Marshal([]any{tc.results, ledger, set, ctx.Mutable.AnswerDocumentV2()})
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			lines := b1638StateCensusLines(prompt)
			if len(lines) != 1 || !strings.Contains(lines[0], "blocked_reason_records="+count+";") || !strings.Contains(lines[0], fmt.Sprintf("d_state=%.3fms;", account.DStateMS)) {
				t.Errorf("current account must pair only its own census: D=%.3f count=%s got=%q", account.DStateMS, count, lines)
			}
			t.Logf("selected D=%.3f own census=%s card_count=%d", account.DStateMS, count, len(lines))
			after, _ := json.Marshal([]any{tc.results, answerDocObservationLedger(ctx), types.CompileTraceCausalProjectionSet(answerDocObservationLedger(ctx)), ctx.Mutable.AnswerDocumentV2()})
			if string(before) != string(after) {
				t.Fatal("prompt changed source facts, projection selection/values, or model document")
			}
		})
	}
}

// Test fixtures retain the original account facts exactly; this only supplies
// the matching record that the new same-result companion needs to identify.
func b1638StateRecord(account types.TraceCausalProjectionTargetStateAccount, ref types.ObservationSourceRef) types.ObservationRecord {
	return types.ObservationRecord{
		ID: account.EvidenceID, Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard,
		SourceRef: ref, Subject: account.Subject, Predicate: "target_window_states", Object: "state_partition", Unit: "ms", Value: fmt.Sprint(account.TotalMS),
		Span: types.ObservationSpan{LineStart: account.LineStart, LineEnd: account.LineEnd}, SupportRefs: append([]string(nil), account.SupportRefs...),
		RichNotes: []string{
			fmt.Sprintf("selected_window=%.9f..%.9f", account.WindowStartTs, account.WindowEndTs),
			fmt.Sprintf("running=%g", account.RunningMS), fmt.Sprintf("runnable=%g", account.RunnableMS), fmt.Sprintf("sleep=%g", account.SleepMS),
			fmt.Sprintf("d_state=%g", account.DStateMS), fmt.Sprintf("io_wait=%g", account.IOWaitMS), fmt.Sprintf("sleep_io_wait=%g", account.SleepIOWaitMS),
			fmt.Sprintf("total=%g", account.TotalMS), fmt.Sprintf("deterministic_running=%g", account.DeterministicRunningMS),
			fmt.Sprintf("head_carry_ms=%g", account.HeadCarryMS), "head_carry_state=" + account.HeadCarryState,
			fmt.Sprintf("tail_open_ms=%g", account.TailOpenMS), "tail_open_state=" + account.TailOpenState,
		},
	}
}

func b1638StateCensusFixture() (types.TraceCausalProjectionSet, types.ObservationLedger) {
	ref := types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: "/captures/donghu.ftrace", PayloadRef: "/results/query.json", RawRef: "/results/query.txt", QueryScopeID: "producer-query-a", ToolCallID: "call-a"}
	account := types.TraceCausalProjectionTargetStateAccount{Subject: "CompThread_0-2955", EvidenceID: "state-a", DStateMS: 36.757, TotalMS: 36.757, WindowStartTs: 13762.791708, WindowEndTs: 13763.024898}
	state := b1638StateRecord(account, ref)
	count := 12
	census := types.ObservationRecord{ID: "census-a", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
		Predicate: "blocked_reason_census", Subject: account.Subject, Value: "12", ResultCount: &count,
		RichNotes: []string{"selected_window=13762.791708..13763.024898", "blocked_reason_census=dma_fence_default_w×12(Σ39.157ms)"}}
	return types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{{ArtifactPath: ref.Path, TargetStateAccount: &account}}}, types.ObservationLedger{Records: []types.ObservationRecord{state, census}}
}

func TestB1638StateCensusSourceGuards(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*types.TraceCausalProjectionSet, *types.ObservationLedger)
		want   bool
	}{
		{"known", func(*types.TraceCausalProjectionSet, *types.ObservationLedger) {}, true},
		{"equal_record_duplicate", func(_ *types.TraceCausalProjectionSet, l *types.ObservationLedger) {
			l.Records = append(l.Records, l.Records[0])
		}, true},
		{"unknown_query", func(_ *types.TraceCausalProjectionSet, l *types.ObservationLedger) {
			for i := range l.Records {
				l.Records[i].SourceRef.QueryScopeID = ""
			}
		}, false},
		{"unknown_result", func(_ *types.TraceCausalProjectionSet, l *types.ObservationLedger) {
			for i := range l.Records {
				l.Records[i].SourceRef.PayloadRef = ""
				l.Records[i].SourceRef.RawRef = ""
			}
		}, false},
		{"missing_state", func(_ *types.TraceCausalProjectionSet, l *types.ObservationLedger) { l.Records = l.Records[1:] }, false},
		{"state_nonruntime", func(_ *types.TraceCausalProjectionSet, l *types.ObservationLedger) {
			l.Records[0].SourceRef.Kind = types.ObservationSourceCurrentSource
		}, false},
		{"foreign_query", func(_ *types.TraceCausalProjectionSet, l *types.ObservationLedger) {
			l.Records[1].SourceRef.QueryScopeID = "producer-query-b"
		}, false},
		{"foreign_capture", func(_ *types.TraceCausalProjectionSet, l *types.ObservationLedger) {
			l.Records[1].SourceRef.CaptureIdentityPath = "/captures/other.ftrace"
		}, false},
		{"foreign_path", func(_ *types.TraceCausalProjectionSet, l *types.ObservationLedger) {
			l.Records[1].SourceRef.Path = "/captures/other.ftrace"
		}, false},
		{"foreign_window", func(_ *types.TraceCausalProjectionSet, l *types.ObservationLedger) {
			l.Records[1].RichNotes[0] = "selected_window=13762.700000..13763.000000"
		}, false},
		{"foreign_target_case", func(_ *types.TraceCausalProjectionSet, l *types.ObservationLedger) {
			l.Records[1].Subject = "compthread_0-2955"
		}, false},
		{"same_source_facts_different_display_metadata", func(_ *types.TraceCausalProjectionSet, l *types.ObservationLedger) {
			r := l.Records[0]
			r.Summary = "independent display wording"
			r.Confidence = 0.25
			l.Records = append(l.Records, r)
		}, true},
		{"collision_account_facts", func(_ *types.TraceCausalProjectionSet, l *types.ObservationLedger) {
			r := l.Records[0]
			r.RichNotes = append([]string(nil), r.RichNotes...)
			r.RichNotes[1] = "running=1"
			l.Records = append(l.Records, r)
		}, false},
		{"collision_query", func(_ *types.TraceCausalProjectionSet, l *types.ObservationLedger) {
			r := l.Records[0]
			r.SourceRef.QueryScopeID = "other"
			l.Records = append(l.Records, r)
		}, false},
		{"collision_foreign_target", func(_ *types.TraceCausalProjectionSet, l *types.ObservationLedger) {
			r := l.Records[0]
			r.Subject = "other-41"
			l.Records = append(l.Records, r)
		}, false},
		{"account_fact_changed", func(s *types.TraceCausalProjectionSet, _ *types.ObservationLedger) {
			s.Projections[0].TargetStateAccount.RunnableMS = 1
		}, false},
		{"account_locator_changed", func(s *types.TraceCausalProjectionSet, _ *types.ObservationLedger) {
			s.Projections[0].TargetStateAccount.LineEnd = 7
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set, ledger := b1638StateCensusFixture()
			tc.mutate(&set, &ledger)
			before, _ := json.Marshal([]any{set, ledger})
			got := renderTraceFinalBlockedReasonStateRelation(set, ledger)
			if (got != "") != tc.want {
				t.Errorf("card present=%t want=%t: %s", got != "", tc.want, got)
			}
			after, _ := json.Marshal([]any{set, ledger})
			if !reflect.DeepEqual(before, after) {
				t.Fatal("source facts or projection mutated")
			}
		})
	}
}

func TestB1638StateCensusChecksEverySelectedAccountField(t *testing.T) {
	base, _ := b1638StateCensusFixture()
	typ := reflect.TypeOf(*base.Projections[0].TargetStateAccount)
	for index := 0; index < typ.NumField(); index++ {
		t.Run(typ.Field(index).Name, func(t *testing.T) {
			set, ledger := b1638StateCensusFixture()
			field := reflect.ValueOf(set.Projections[0].TargetStateAccount).Elem().Field(index)
			switch field.Kind() {
			case reflect.String:
				field.SetString(field.String() + "changed")
			case reflect.Float64:
				field.SetFloat(field.Float() + 0.001)
			case reflect.Int:
				field.SetInt(field.Int() + 1)
			case reflect.Slice:
				field.Set(reflect.ValueOf([]string{"/different/source:4-8"}))
			default:
				t.Fatalf("new account field needs an explicit disagreement probe: %s", typ.Field(index).Name)
			}
			if got := renderTraceFinalBlockedReasonStateRelation(set, ledger); got != "" {
				t.Fatalf("a changed selected account field must not borrow the original source: %s", typ.Field(index).Name)
			}
		})
	}
}

func TestB1638UnknownSourceKeepsIndependentFactsInPublicPrompt(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, missing := range []string{"query", "result", "state_source"} {
			t.Run(lang+"/"+missing, func(t *testing.T) {
				_, ledger := b1638StateCensusFixture()
				root := ledger.Records[0]
				root.ID, root.Predicate, root.Object, root.Value = "root", "root_cause_primary", "d_state_or_io_wait", "36.757"
				root.RichNotes = []string{"selected_window=13762.791708..13763.024898", "rank=1", "tier=primary", "chain_relevance=on_chain", "effective_impact_ms=36.757"}
				ledger.Records = append(ledger.Records, root)
				for i := range ledger.Records {
					switch missing {
					case "query":
						ledger.Records[i].SourceRef.QueryScopeID = ""
					case "result":
						ledger.Records[i].SourceRef.PayloadRef, ledger.Records[i].SourceRef.RawRef = "", ""
					case "state_source":
						if ledger.Records[i].Predicate == "target_window_states" {
							ledger.Records[i].SourceRef.QueryScopeID = ""
						}
					}
				}
				result := types.ToolResult{ToolName: "trace_query", Success: true, Observations: ledger.Records}
				ctx := b1638StateCensusContext([]types.ToolResult{result})
				ctx.Language, ctx.AnalysisIR.AnswerContract.Language = lang, lang
				before, _ := json.Marshal([]any{answerDocObservationLedger(ctx), ctx.Mutable.AnswerDocumentV2()})
				prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
				if lines := b1638StateCensusLines(prompt); len(lines) != 0 {
					t.Fatalf("unknown source must not pair: %q", lines)
				}
				for _, scalar := range []string{"36.757", "39.157", "dma_fence_default_w"} {
					if !strings.Contains(prompt, scalar) {
						t.Errorf("independent original fact became invisible: %s", scalar)
					}
				}
				after, _ := json.Marshal([]any{answerDocObservationLedger(ctx), ctx.Mutable.AnswerDocumentV2()})
				if string(before) != string(after) {
					t.Fatal("missing join authority must not change original facts/model document")
				}
			})
		}
	}
}

func TestB1638DistinctCaptureCardsDoNotShareDisplayDedup(t *testing.T) {
	a, al := b1638StateCensusFixture()
	b, bl := b1638StateCensusFixture()
	b.Projections[0].ArtifactPath = "/captures/b/donghu.ftrace"
	b.Projections[0].TargetStateAccount.EvidenceID = "state-b"
	for i := range bl.Records {
		bl.Records[i].SourceRef.Path = b.Projections[0].ArtifactPath
		bl.Records[i].ID = strings.ReplaceAll(bl.Records[i].ID, "-a", "-b")
	}
	a.Projections = append(a.Projections, b.Projections...)
	al.Records = append(al.Records, bl.Records...)
	if lines := b1638StateCensusLines(renderTraceFinalBlockedReasonStateRelation(a, al)); len(lines) != 2 {
		t.Fatalf("two independently sourced equal cards must not disappear into one: %q", lines)
	}
}
