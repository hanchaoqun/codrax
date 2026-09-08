package tool

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The two queries observe the same complete capture. The requested 20 ms
// interval has no D/marked-IO wait; a 0.5 ms wider probe includes a 0.3 ms D
// interval. Similar endpoints do not authorize joining those accounts.
func TestB1618TargetWaitQueryJoinActualExecuteKeepsZeroAndProbeSeparate(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "same.systrace")
	trace := strings.Join([]string{
		`idle-0 (0) [000] .... 9.999000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=120`,
		`app-20 (20) [000] .... 10.020100: sched_switch: prev_comm=app prev_pid=20 prev_prio=120 prev_state=D ==> next_comm=worker next_pid=21 next_prio=120`,
		`worker-21 (21) [000] .... 10.020400: sched_wakeup: comm=app pid=20 prio=120 target_cpu=000`,
		`worker-21 (21) [000] .... 10.020450: sched_switch: prev_comm=worker prev_pid=21 prev_prio=120 prev_state=S ==> next_comm=app next_pid=20 next_prio=120`,
		`app-20 (20) [000] .... 10.021000: sched_switch: prev_comm=app prev_pid=20 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o600); err != nil {
		t.Fatal(err)
	}
	var results []types.ToolResult
	for _, end := range []float64{10.020, 10.0205} {
		params, err := json.Marshal(map[string]any{
			"source": "path", "path": tracePath, "view": "window_stats",
			"pid": 20, "time_start": 10.0, "time_end": end,
		})
		if err != nil {
			t.Fatal(err)
		}
		result, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
		if err != nil || !result.Success {
			t.Fatalf("actual query failed: err=%v result=%+v", err, result)
		}
		results = append(results, result)
	}
	for _, reverse := range []bool{false, true} {
		for _, lang := range []string{"zh", "en"} {
			t.Run(lang+"/reverse="+map[bool]string{false: "false", true: "true"}[reverse], func(t *testing.T) {
				ordered := append([]types.ToolResult(nil), results...)
				if reverse {
					ordered[0], ordered[1] = ordered[1], ordered[0]
				}
				start, end := 10.0, 10.020
				bus := &types.BusContext{
					Mutable: types.NewMutableState("typed wait account test"),
					AnalysisIR: &types.AnalysisIR{
						RequestModel: types.RequestModel{
							Intent: types.IntentTrace,
							RuntimeTargets: []types.RuntimeTarget{{
								Kind: types.RuntimeTargetKindThread, PID: 20, Thread: "app-20", Source: "user_explicit",
							}},
							RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{
								RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
								TimeStart:      &start, TimeEnd: &end, SourceQuote: "typed request interval",
							},
						},
						AnswerContract: types.AnswerContract{Language: lang},
					},
					ToolResults: ordered,
				}
				ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
				waits := types.BuildTraceTargetWaitSummaryAuthorities(ledger, runtimeTraceAuthorityRequestModel(bus))
				if len(waits) != 2 {
					t.Fatalf("measured-zero requested account and wider probe must both survive: %+v", waits)
				}
				if waits[0].Count != 0 || waits[0].WallClockMS != 0 || !waits[0].IsRequestedScopePrincipal() ||
					waits[1].Count != 1 || math.Abs(waits[1].WallClockMS-.3) > .000001 || waits[1].IsRequestedScopePrincipal() {
					t.Fatalf("separate measured accounts changed: %+v", waits)
				}
				doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary, Text: "model-authored conclusion"}}}
				if !materializeRuntimeTraceTargetStateAuthorityBlock(doc, bus) {
					t.Fatal("actual typed accounts must materialize")
				}
				if doc.Blocks[0].Text != "model-authored conclusion" {
					t.Fatal("account publication must not rewrite the model answer")
				}
				block := answerDocumentTestBlockByID(t, doc, runtimeTraceTargetStateAuthorityBlockID)
				surface := types.AnswerBlockVisibleSurface(block)
				var statePart, probePart string
				for _, paragraph := range strings.Split(surface, "\n\n") {
					if strings.Contains(paragraph, "10.000000..10.020000") {
						statePart = paragraph
					}
					if strings.Contains(paragraph, "10.000000..10.020500") {
						probePart = paragraph
					}
				}
				zeroText, positiveText := "等待明细完整，共 0 段", "等待明细完整，共 1 段"
				if lang == "en" {
					zeroText, positiveText = "wait roster is complete: 0 intervals", "wait roster is complete: 1 intervals"
				}
				if !strings.Contains(statePart, zeroText) || strings.Contains(statePart, "0.300ms") ||
					!strings.Contains(probePart, positiveText) || !strings.Contains(probePart, "0.300ms") {
					t.Fatalf("requested zero borrowed the wider query's waits or the probe disappeared:\n%s", surface)
				}
			})
		}
	}
}

func TestB1618TargetWaitQueryJoinRequiresExactScopedSource(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*types.TraceTargetStateScopeAuthority, *types.TraceTargetWaitSummaryAuthority, *[]types.ObservationRecord)
		want bool
	}{
		{name: "same_result", want: true},
		{name: "one_microsecond_representation", want: true, edit: func(_ *types.TraceTargetStateScopeAuthority, w *types.TraceTargetWaitSummaryAuthority, rows *[]types.ObservationRecord) {
			w.WindowEndTs += .000001
			(*rows)[1].RichNotes = []string{types.TraceNoteKeySelectedWindow + "=10.000000..10.020001"}
		}},
		{name: "neighbor_window_500_microseconds", edit: func(_ *types.TraceTargetStateScopeAuthority, w *types.TraceTargetWaitSummaryAuthority, rows *[]types.ObservationRecord) {
			w.WindowEndTs += .0005
			(*rows)[1].RichNotes = []string{types.TraceNoteKeySelectedWindow + "=10.000000..10.020500"}
		}},
		{name: "same_basename_different_capture", edit: func(_ *types.TraceTargetStateScopeAuthority, w *types.TraceTargetWaitSummaryAuthority, rows *[]types.ObservationRecord) {
			(*rows)[1].SourceRef.Path = "/other/same.systrace"
			w.ArtifactKey = types.TraceCausalProjectionRecordArtifactIdentity((*rows)[1])
		}},
		{name: "target_case_is_not_identity", edit: func(_ *types.TraceTargetStateScopeAuthority, w *types.TraceTargetWaitSummaryAuthority, rows *[]types.ObservationRecord) {
			w.Subject, (*rows)[1].Subject = "APP-20", "APP-20"
		}},
		{name: "no_result_receipt", edit: func(_ *types.TraceTargetStateScopeAuthority, _ *types.TraceTargetWaitSummaryAuthority, rows *[]types.ObservationRecord) {
			(*rows)[0].SourceRef.PayloadRef, (*rows)[1].SourceRef.PayloadRef = "", ""
		}},
		{name: "same_query_independent_result", edit: func(_ *types.TraceTargetStateScopeAuthority, _ *types.TraceTargetWaitSummaryAuthority, rows *[]types.ObservationRecord) {
			(*rows)[1].SourceRef.PayloadRef = "/payload/second.json"
		}},
		{name: "unknown_capture", edit: func(s *types.TraceTargetStateScopeAuthority, w *types.TraceTargetWaitSummaryAuthority, _ *[]types.ObservationRecord) {
			s.ArtifactKey, w.ArtifactKey = "", ""
		}},
		{name: "missing_selected_window_no_span_fallback", edit: func(_ *types.TraceTargetStateScopeAuthority, _ *types.TraceTargetWaitSummaryAuthority, rows *[]types.ObservationRecord) {
			(*rows)[1].RichNotes = nil
		}},
		{name: "colliding_state_id_different_result", edit: func(_ *types.TraceTargetStateScopeAuthority, _ *types.TraceTargetWaitSummaryAuthority, rows *[]types.ObservationRecord) {
			copy := (*rows)[0]
			copy.SourceRef.PayloadRef = "/payload/second.json"
			*rows = append(*rows, copy)
		}},
		{name: "colliding_state_id_different_facts", edit: func(_ *types.TraceTargetStateScopeAuthority, _ *types.TraceTargetWaitSummaryAuthority, rows *[]types.ObservationRecord) {
			copy := (*rows)[0]
			copy.Value = "19.000"
			*rows = append(*rows, copy)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ref := types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: "/capture/same.systrace", PayloadRef: "/payload/first.json"}
			rows := []types.ObservationRecord{{
				ID: "result#target_window_states", Producer: "trace_query", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
				GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref, Subject: "app-20", Predicate: "target_window_states",
				Span: types.ObservationSpan{StartTs: 10, EndTs: 10.02}, Value: "20.000", Unit: "ms",
				RichNotes: []string{types.TraceNoteKeySelectedWindow + "=10.000000..10.020000"},
			}}
			set := rows[0]
			set.ID, set.Predicate, set.Value, set.Unit = "result#target_window_wait_occurrences", "target_window_wait_occurrences", "1", "occurrences"
			rows = append(rows, set)
			key := types.TraceCausalProjectionRecordArtifactIdentity(rows[0])
			state := types.TraceTargetStateScopeAuthority{
				ArtifactKey: key, ArtifactLabel: "same.systrace", Subject: "app-20", WindowStartTs: 10, WindowEndTs: 10.02,
				EvidenceID: rows[0].ID, SourceRecordIDs: []string{rows[0].ID},
			}
			wait := types.TraceTargetWaitSummaryAuthority{
				ArtifactKey: key, ArtifactLabel: "same.systrace", Subject: "app-20", WindowStartTs: 10, WindowEndTs: 10.02,
				RecordID: set.ID, SourceRecordIDs: []string{set.ID}, Count: 1,
			}
			if tc.edit != nil {
				tc.edit(&state, &wait, &rows)
			}
			for _, reverse := range []bool{false, true} {
				ordered := append([]types.ObservationRecord(nil), rows...)
				if reverse {
					for i, j := 0, len(ordered)-1; i < j; i, j = i+1, j-1 {
						ordered[i], ordered[j] = ordered[j], ordered[i]
					}
				}
				index, ok := matchingTraceTargetWaitSummary(state, []types.TraceTargetWaitSummaryAuthority{wait}, types.ObservationLedger{Records: ordered})
				if ok != tc.want || (ok && index != 0) {
					t.Fatalf("reverse=%s source match=%t index=%d want=%t", strconv.FormatBool(reverse), ok, index, tc.want)
				}
			}
		})
	}
}

func TestB1618TargetWaitQueryJoinUnknownQueryKeepsLocalRosterWithoutZeroWindow(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			count := 1
			ref := types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: "/capture/legacy.systrace", PayloadRef: "/payload/legacy.json"}
			// A legacy export retains its complete local occurrence list, but
			// has no producer-owned selected_window. Span is not a query proof.
			rows := []types.ObservationRecord{{
				ID: "legacy#target_window_wait_occurrences", Producer: "trace_query", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
				GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref, Subject: "app-20", Predicate: "target_window_wait_occurrences",
				Span: types.ObservationSpan{StartTs: 10, EndTs: 10.02}, Object: "complete", Value: "1", Unit: "occurrences", ResultCount: &count,
			}, {
				ID: "legacy#target_window_wait_occurrence:1", Producer: "trace_query", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
				GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref, Subject: "app-20", Predicate: "target_window_wait_occurrence",
				Span: types.ObservationSpan{StartTs: 10.01, EndTs: 10.0103}, Object: "state=d_sleep;iowait=unknown;caller=unknown", Value: "0.300", Unit: "ms",
			}}
			start, end := 10.0, 10.02
			bus := &types.BusContext{
				Mutable: types.NewMutableState("legacy scoped wait"),
				AnalysisIR: &types.AnalysisIR{
					RequestModel: types.RequestModel{
						Intent:                      types.IntentTrace,
						RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 20, Thread: "app-20", Source: "user_explicit"}},
						RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetWaitOccurrences}},
						RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "typed request interval"},
					},
					AnswerContract: types.AnswerContract{Language: lang},
				},
				ToolResults: []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: rows}},
			}
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary, Text: "model conclusion"}}}
			if !materializeRuntimeTraceTargetStateAuthorityBlock(doc, bus) {
				t.Fatal("legacy complete positive occurrence list must remain independently readable")
			}
			text := types.AnswerBlockVisibleSurface(answerDocumentTestBlockByID(t, doc, runtimeTraceTargetStateAuthorityBlockID))
			unknown := "查询范围未明确"
			if lang == "en" {
				unknown = "query window not stated"
			}
			if !strings.Contains(text, unknown) || !strings.Contains(text, "0.300ms") || strings.Contains(text, "0.000000..0.000000") ||
				strings.Contains(text, "窗口=10.000000..10.020000") || strings.Contains(text, "window=10.000000..10.020000") {
				t.Fatalf("legacy list was lost or its missing query was fabricated:\n%s", text)
			}
		})
	}
}
