package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func b1618StateCapture(t *testing.T, root, name string, secondRunningMS int) string {
	t.Helper()
	path := filepath.Join(root, name, "same.systrace")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`idle-0 (0) [001] .... 10.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=41 next_prio=120
worker-41 (41) [001] .... 10.002000: sched_switch: prev_comm=worker prev_pid=41 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
idle-0 (0) [001] .... 10.010000: sched_wakeup: comm=worker pid=41 prio=120 target_cpu=1
idle-0 (0) [001] .... 10.011000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=41 next_prio=120
worker-41 (41) [001] .... 10.%06d: sched_switch: prev_comm=worker prev_pid=41 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
idle-0 (0) [001] .... 10.020000: sched_wakeup: comm=worker pid=41 prio=120 target_cpu=1
`, 11000+secondRunningMS*1000)
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}

func b1618StateQuery(t *testing.T, root, path string, start, end float64, filtered bool, subjects ...string) types.ToolResult {
	t.Helper()
	params := map[string]any{"source": "path", "path": path, "view": "window_stats", "pid": 41, "time_start": start, "time_end": end}
	if filtered {
		params["line_start"], params["line_end"] = 1, 6
	}
	raw, _ := json.Marshal(params)
	result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: root, WorkDir: root}, raw)
	if err != nil || !result.Success {
		t.Fatalf("actual TraceQuery failed: %v / %s", err, result.Summary)
	}
	found := false
	subject := "worker-41"
	if len(subjects) > 0 {
		subject = subjects[0]
	}
	for _, record := range result.Observations {
		if record.Predicate == "target_window_states" {
			if record.Subject != subject || record.SourceRef.Path != path || record.SourceRef.QueryScopeID == "" || record.SourceRef.PayloadRef == "" {
				t.Fatalf("invalid producer premise: %+v", record)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("actual query did not produce target account: %+v", result.Observations)
	}
	return result
}

func TestB1618ActualSameTIDDifferentNamesKeepEachAccountSubject(t *testing.T) {
	root := t.TempDir()
	a := b1618StateCapture(t, root, "first", 3)
	b := b1618StateCapture(t, root, "second", 5)
	raw, err := os.ReadFile(b)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(b, []byte(strings.ReplaceAll(string(raw), "worker", "service-7")), 0600); err != nil {
		t.Fatal(err)
	}
	results := []types.ToolResult{b1618StateQuery(t, root, a, 10.01, 10.02, false), b1618StateQuery(t, root, b, 10.01, 10.02, false, "service-7-41")}
	for _, lang := range []string{"zh", "en"} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse=%t", lang, reverse), func(t *testing.T) {
				ordered := append([]types.ToolResult(nil), results...)
				if reverse {
					ordered[0], ordered[1] = ordered[1], ordered[0]
				}
				// The loose thread name, deliberately without numeric TID 41,
				// exercises the existing soft name-index selection on the second
				// capture's alias. It does not bind either account to model prose.
				body := b1618PublishStateAppendix(t, ordered, nil, lang, "service-7: compare the available observations.")
				for _, want := range []struct{ path, subject, value string }{{a, "worker-41", "running 3.000"}, {b, "service-7-41", "running 5.000"}} {
					found := false
					for _, line := range strings.Split(body, "\n") {
						if strings.Contains(line, want.path) && strings.Contains(line, want.subject) && strings.Contains(line, want.value) {
							found = true
						}
					}
					if !found {
						t.Errorf("second-capture alias lost or borrowed another subject: %+v\n%s", want, body)
					}
				}
			})
		}
	}
}

func TestB1618AccountUnknownCoordinatesAndScopedMainThreadProof(t *testing.T) {
	base := b1618IdentityRecord()
	for _, tc := range []struct {
		name                              string
		sameCapture, sameSubject, unknown bool
		want                              bool
	}{
		{"same_capture_subject", true, true, false, true},
		{"different_capture", false, true, false, false},
		{"different_subject", true, false, false, false},
		{"unknown_capture", true, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := b1618IdentityAccount(base)
			proof := base
			proof.RichNotes = append(append([]string(nil), base.RichNotes...), types.TraceNoteKeyTGID+"=41")
			if !tc.sameCapture {
				proof.SourceRef.Path = "/captures/other/same.systrace"
				proof.SourceRef.CaptureIdentityPath = proof.SourceRef.Path
			}
			if !tc.sameSubject {
				proof.Subject = "other-41"
			}
			if tc.unknown {
				account.scope.ArtifactKey = ""
			}
			out := []proseWallClockAccount{account}
			proseWallClockMarkMainThreads(out, types.ObservationLedger{Records: []types.ObservationRecord{proof}})
			if out[0].mainThread != tc.want {
				t.Fatalf("main-thread evidence crossed source/subject scope: %+v", out[0])
			}
		})
	}
	account := b1618IdentityAccount(base)
	account.scope.WindowKnown = false
	account.scope.WindowStartTs = 0
	account.scope.WindowEndTs = 0
	account.windowMS = 0
	account.source.SourceRef.QueryTargetPID = 0
	account.source.SourceRef.QueryTargetThread = "name`with\nmarkup" + strings.Repeat("x", 200)
	account.source.SourceRef.PayloadRef = "DO_NOT_SHOW_INTERNAL_PAYLOAD"
	account.source.SourceRef.RawRef = "DO_NOT_SHOW_INTERNAL_RAW"
	zh, en := proseFactPartitionFact(&proseFactThreadFacts{subject: account.subject, account: &account})
	for _, body := range []string{zh, en} {
		for _, bad := range []string{"0.000000..0.000000", "窗长 0.000", "window 0.000", "tid/pid=0", "DO_NOT_SHOW_INTERNAL", "name`with", "name`with\nmarkup"} {
			if strings.Contains(body, bad) {
				t.Errorf("missing coordinate became value or internal text leaked: %q\n%s", bad, body)
			}
		}
		if strings.Contains(body, "\n") {
			t.Errorf("source scalar injected a line: %s", body)
		}
	}
	if !strings.Contains(zh, "窗长未明确") || !strings.Contains(en, "window duration not stated") {
		t.Fatalf("unknown window length must be explicit: %s / %s", zh, en)
	}
}

func b1618PublishStateAppendix(t *testing.T, results []types.ToolResult, profile *types.RuntimeArtifactScopeProfile, lang string, texts ...string) string {
	t.Helper()
	mut := types.NewMutableState("Compare worker-41 state accounts.")
	bus := &types.BusContext{Mutable: mut, Language: lang, ToolResults: results}
	if profile != nil {
		bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
			RuntimeArtifactScopeProfile: profile,
			RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 41, Thread: "worker", Source: "user_explicit", Confidence: 1}},
		}}
	}
	text := "worker-41: running 2.000ms and sleep 8.000ms are model-owned observations, not a claim about another window."
	if len(texts) > 0 {
		text = texts[0]
	}
	doc := psgProseDoc(text)
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	beforeDoc, _ := json.Marshal(mut.AnswerDocumentV2())
	beforeResults, _ := json.Marshal(results)
	model := render.RenderAnswerDocument(doc, lang)
	out := &agent.StageOutput{FinalAnswer: model}
	(&Orchestrator{busCtx: bus}).attachSystemCrossCheckAppendix(out, "", nil)
	afterDoc, _ := json.Marshal(mut.AnswerDocumentV2())
	afterResults, _ := json.Marshal(results)
	if string(beforeDoc) != string(afterDoc) || string(beforeResults) != string(afterResults) || !strings.Contains(out.FinalAnswer, model) {
		t.Fatal("publication changed the model or original producer records")
	}
	var body strings.Builder
	for _, attachment := range mut.AnswerDisplayAttachments() {
		if attachment.Source == types.AnswerDisplayAttachmentSourceSystemCrossCheck {
			body.WriteString(attachment.Body)
		}
	}
	if body.Len() == 0 {
		t.Fatal("public appendix was not attached")
	}
	return body.String()
}

// These are real tool calls and the normal publication entry, not hand-built
// authorities or injected reconciliation rows. Only the request profile is a
// typed fixture; this test does not claim to invoke the analyzer LLM.
func TestB1618ActualStateQueriesKeepSourceAndIndependentWindows(t *testing.T) {
	root := t.TempDir()
	pathA := b1618StateCapture(t, root, "a", 3)
	pathB := b1618StateCapture(t, root, "b", 5)
	aStart, aEnd, bStart, bEnd := 10.0, 10.01, 10.01, 10.02
	profile := &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeWindows: []types.RuntimeArtifactTimeWindow{
		{TimeStart: &aStart, TimeEnd: &aEnd, SourceQuote: "10..10.01"}, {TimeStart: &bStart, TimeEnd: &bEnd, SourceQuote: "10.01..10.02"},
	}}
	for _, tc := range []struct {
		name                    string
		results                 []types.ToolResult
		profile                 *types.RuntimeArtifactScopeProfile
		paths, windows, running []string
	}{
		{"two_windows", []types.ToolResult{b1618StateQuery(t, root, pathA, aStart, aEnd, false), b1618StateQuery(t, root, pathA, bStart, bEnd, false)}, profile, []string{pathA, pathA}, []string{"10.000000..10.010000", "10.010000..10.020000"}, []string{"running 2.000", "running 3.000"}},
		{"same_name_captures", []types.ToolResult{b1618StateQuery(t, root, pathA, bStart, bEnd, false), b1618StateQuery(t, root, pathB, bStart, bEnd, false)}, nil, []string{pathA, pathB}, []string{"10.010000..10.020000", "10.010000..10.020000"}, []string{"running 3.000", "running 5.000"}},
		{"line_filter", []types.ToolResult{b1618StateQuery(t, root, pathA, bStart, bEnd, true)}, nil, []string{pathA}, []string{"10.010000..10.020000"}, []string{"running 3.000"}},
		{"full_and_filtered", []types.ToolResult{b1618StateQuery(t, root, pathA, bStart, bEnd, false), b1618StateQuery(t, root, pathA, bStart, bEnd, true)}, profile, []string{pathA, pathA}, []string{"10.010000..10.020000", "10.010000..10.020000"}, []string{"running 3.000", "running 3.000"}},
	} {
		for _, lang := range []string{"zh", "en"} {
			for _, reverse := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/reverse=%t", tc.name, lang, reverse), func(t *testing.T) {
					results := append([]types.ToolResult(nil), tc.results...)
					if reverse {
						for i, j := 0, len(results)-1; i < j; i, j = i+1, j-1 {
							results[i], results[j] = results[j], results[i]
						}
					}
					body := b1618PublishStateAppendix(t, results, tc.profile, lang)
					for i, path := range tc.paths {
						found := false
						for _, line := range strings.Split(body, "\n") {
							if strings.Contains(line, path) && strings.Contains(line, tc.windows[i]) && strings.Contains(line, tc.running[i]) && strings.Contains(line, "worker-41") {
								found = true
							}
						}
						if !found {
							t.Errorf("source/window/account missing together: %s / %s / %s\n%s", path, tc.windows[i], tc.running[i], body)
						}
					}
					if tc.name == "line_filter" && !strings.Contains(body, "1..6") {
						t.Errorf("line-filter receipt missing: %s", body)
					}
					if tc.name == "full_and_filtered" {
						plain, filtered := false, false
						for _, line := range strings.Split(body, "\n") {
							if !strings.Contains(line, "running 3.000") || !strings.Contains(line, pathA) {
								continue
							}
							if strings.Contains(line, "1..6") {
								filtered = true
							}
							if strings.Contains(line, "未施加行号过滤") || strings.Contains(line, "no line filter") {
								plain = true
							}
						}
						if !plain || !filtered {
							t.Errorf("equal values from full and filtered results must stay independent: %s", body)
						}
					}
					stateLines := 0
					for _, line := range strings.Split(body, "\n") {
						if strings.Contains(line, "running ") && strings.Contains(line, "runnable ") {
							stateLines++
						}
					}
					if stateLines != len(tc.results) {
						t.Errorf("each account must appear once, not repeated between both faces: got %d want %d\n%s", stateLines, len(tc.results), body)
					}
					for _, result := range results {
						for _, r := range result.Observations {
							if r.SourceRef.QueryScopeID != "" && strings.Contains(body, r.SourceRef.QueryScopeID) {
								t.Error("internal query identity leaked into appendix")
							}
							if r.SourceRef.PayloadRef != "" && strings.Contains(body, r.SourceRef.PayloadRef) {
								t.Error("internal payload path leaked into appendix")
							}
						}
					}
				})
			}
		}
	}
}

func TestB1618ActualStateAccountsWithoutPartitionTriggerRemainScoped(t *testing.T) {
	root := t.TempDir()
	a := b1618StateCapture(t, root, "first", 3)
	b := b1618StateCapture(t, root, "second", 5)
	results := []types.ToolResult{b1618StateQuery(t, root, a, 10.01, 10.02, false), b1618StateQuery(t, root, b, 10.01, 10.02, false)}
	for _, lang := range []string{"zh", "en"} {
		body := b1618PublishStateAppendix(t, results, nil, lang, "worker-41: compare the available observations.")
		for _, path := range []string{a, b} {
			found := false
			for _, line := range strings.Split(body, "\n") {
				if strings.Contains(line, path) && strings.Contains(line, "10.010000..10.020000") && strings.Contains(line, "running ") {
					found = true
				}
			}
			if !found {
				t.Errorf("non-partition account face lost provenance: %s", body)
			}
		}
		if strings.Contains(body, "互斥分区") || strings.Contains(body, "mutually exclusive partition") {
			t.Errorf("fixture must exercise the non-partition face: %s", body)
		}
	}
}
