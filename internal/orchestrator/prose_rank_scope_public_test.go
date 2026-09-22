package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func rankScopePublicCapture(t *testing.T, root, name string) string {
	t.Helper()
	data, err := os.ReadFile("../../eval/cases/trace_query_wakeup_background_demotion.case")
	if err != nil {
		t.Fatal(err)
	}
	_, trace, ok := strings.Cut(string(data), "HTRACE='")
	if !ok {
		t.Fatal("HARNESS: native trace fixture missing")
	}
	trace, _, ok = strings.Cut(trace, "\n'\n")
	if !ok {
		t.Fatal("HARNESS: native trace fixture unterminated")
	}
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "same.systrace")
	if err := os.WriteFile(path, []byte(trace), 0600); err != nil {
		t.Fatal(err)
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func rankScopePublicQuery(t *testing.T, root, path string, end float64) types.ToolResult {
	t.Helper()
	reg := tool.NewRegistry()
	reg.Register(&tool.TraceQuery{})
	raw, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "root_cause_rank", "pid": 100,
		"time_start": 2.0, "time_end": end, "trace_flavor": "harmony_hitrace"})
	result, err := reg.Execute(&types.BusContext{RepoRoot: root, WorkDir: root}, "trace_query", raw)
	if err != nil || !result.Success {
		t.Fatalf("HARNESS: native query failed: %v / %s", err, result.Summary)
	}
	positive, app := 0, 0
	for _, record := range result.Observations {
		if !strings.Contains(record.ID, "#root_cause_rank:") {
			continue
		}
		rank, _ := proseFactNoteInt(record.RichNotes, types.TraceNoteKeyRank)
		if rank <= 0 {
			continue
		}
		positive++
		board := types.TraceRankBoardDisplayIdentityFromRecord(record)
		if !board.Complete || board.ArtifactPath != path || board.WindowStartTs != 2 || board.WindowEndTs != end || board.BoardTarget != "app-100" {
			t.Fatalf("HARNESS: query did not publish complete original scope: %+v", board)
		}
		if rank <= 4 {
			wantSubject := map[int]string{1: "threadpool-400", 2: "cookie-200", 3: "threadpool-400", 4: "network-300"}[rank]
			wantValue := "1.000"
			if rank == 1 {
				wantValue = "11.000"
			}
			value, _ := proseFactNoteFloat(record.RichNotes, types.TraceNoteKeyEffectiveImpactMS)
			if record.Subject != wantSubject || fmt.Sprintf("%.3f", value) != wantValue {
				t.Fatalf("HARNESS: native four-seat ordering/value changed or admitted a background thread: %+v", record)
			}
		}
		if record.Subject == "app-100" {
			app++
			value, _ := proseFactNoteFloat(record.RichNotes, types.TraceNoteKeyEffectiveImpactMS)
			if rank != 5 || fmt.Sprintf("%.3f", value) != "0.020" {
				t.Fatalf("HARNESS: expected the broad-window 0.020ms rank #5: %+v", record)
			}
		}
	}
	wantRanks, wantApp := 4, 0
	if end > 2.020 {
		wantRanks, wantApp = 5, 1
	}
	if positive != wantRanks || app != wantApp {
		t.Fatalf("HARNESS: native rank populations=%d/%d, want %d/%d", positive, app, wantRanks, wantApp)
	}
	return result
}

// Exercise real query results through the public registered emitter and the
// shipping attachment/render path. This is not an Orchestrator.Run replay.
func rankScopePublicAppendix(t *testing.T, root, lang string, results []types.ToolResult) string {
	t.Helper()
	start, end := 2.0, 2.020
	rm := types.RequestModel{Intent: types.IntentExplain, Language: lang,
		RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 100, Thread: "app-100", Source: "user_explicit", Confidence: 1}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "2.000..2.020", Confidence: 1},
	}
	mu := types.NewMutableState("Compare observations about app-100 in 2.000..2.020.")
	mu.SetRequestModel(rm)
	mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	bus := &types.BusContext{RepoRoot: root, WorkDir: root, Language: lang, Mutable: mu, AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
	before, _ := json.Marshal([]any{results, ledger, types.CompileTraceCausalProjectionSet(ledger), rm})
	const model = "app-100 and threadpool-400: the model owns this conclusion and its uncertainty."
	reg := tool.NewRegistry()
	reg.Register(&tool.EmitAnswerDocument{})
	raw, _ := json.Marshal(map[string]any{"blocks": []map[string]any{{"id": "model", "kind": "summary", "surface_role": "principal", "trace_causal_claim_caliber": "no_causal_conclusion", "text": model}}})
	result, err := reg.Execute(bus, "emit_answer_document", raw)
	if err != nil || !result.Success {
		t.Fatalf("HARNESS: actual emit failed: %v / %s", err, result.Summary)
	}
	docBefore, _ := json.Marshal(mu.AnswerDocumentV2())
	reportBefore := mu.TraceRootCauseReport()
	out := &agent.StageOutput{FinalAnswer: render.RenderAnswerDocument(mu.AnswerDocumentV2(), lang)}
	(&Orchestrator{busCtx: bus}).attachSystemCrossCheckAppendix(out, "", nil)
	var body string
	for _, attachment := range mu.AnswerDisplayAttachments() {
		if attachment.Source == types.AnswerDisplayAttachmentSourceSystemCrossCheck {
			if body != "" {
				t.Fatal("duplicate system cross-check attachment")
			}
			body = attachment.Body
		}
	}
	if body == "" || !strings.Contains(out.FinalAnswer, body) || !strings.Contains(out.FinalAnswer, model) {
		t.Fatal("HARNESS: shipping output did not preserve the appendix and model prose")
	}
	docAfter, _ := json.Marshal(mu.AnswerDocumentV2())
	ledgerAfter := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
	after, _ := json.Marshal([]any{results, ledgerAfter, types.CompileTraceCausalProjectionSet(ledgerAfter), rm})
	if string(before) != string(after) || string(docBefore) != string(docAfter) || !reflect.DeepEqual(reportBefore, mu.TraceRootCauseReport()) {
		t.Fatal("display changed original query facts, scope, rank population, model document or sidecar")
	}
	return body
}

func rankScopePublicLine(t *testing.T, body, lang, subject string) string {
	t.Helper()
	prefix := "- Evidence reference: " + subject + " — "
	if lang == "zh" {
		prefix = "- 事实对照：" + subject + " — "
	}
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, prefix) && strings.Contains(line, "=#") {
			return line
		}
	}
	t.Fatalf("HARNESS: missing rank fact for %s: %s", subject, body)
	return ""
}

func TestRankScopePublicNativeQueryEmitAppendix(t *testing.T) {
	root := t.TempDir()
	a, b := rankScopePublicCapture(t, root, "a"), rankScopePublicCapture(t, root, "b")
	wide, narrow := rankScopePublicQuery(t, root, a, 2.0205), rankScopePublicQuery(t, root, a, 2.020)
	other := rankScopePublicQuery(t, root, b, 2.0205)
	for _, lang := range []string{"zh", "en"} {
		for _, tc := range []struct {
			name    string
			results []types.ToolResult
			paths   []string
			windows []string
		}{
			{"single_wide", []types.ToolResult{wide}, []string{a}, []string{"2.000000..2.020500"}},
			{"wide_then_narrow", []types.ToolResult{wide, narrow}, []string{a}, []string{"2.000000..2.020500"}},
			{"narrow_then_wide", []types.ToolResult{narrow, wide}, []string{a}, []string{"2.000000..2.020500"}},
			{"same_board_multiple_seats", []types.ToolResult{narrow}, []string{a, a}, []string{"2.000000..2.020000", "2.000000..2.020000"}},
			{"cross_capture_same_basename", []types.ToolResult{wide, other}, []string{a, b}, []string{"2.000000..2.020500", "2.000000..2.020500"}},
		} {
			t.Run(lang+"/"+tc.name, func(t *testing.T) {
				body := rankScopePublicAppendix(t, root, lang, tc.results)
				subject := "app-100"
				if tc.name == "same_board_multiple_seats" {
					subject = "threadpool-400"
				}
				line := rankScopePublicLine(t, body, lang, subject)
				for _, path := range tc.paths {
					if !strings.Contains(line, path) {
						t.Errorf("RANK_SCOPE_LOST: source absent from rank line: %s", line)
					}
				}
				for _, window := range tc.windows {
					if strings.Count(line, window) < len(tc.windows) {
						t.Errorf("RANK_SCOPE_LOST: each rank must retain query %s: %s", window, line)
					}
				}
				if subject == "app-100" && (!strings.Contains(line, "0.020ms") || strings.Contains(line, "2.000000..2.020000")) {
					t.Fatalf("broad rank lost its value or borrowed the main window: %s", line)
				}
				if subject == "threadpool-400" && (!strings.Contains(line, "11.000ms") || !strings.Contains(line, "1.000ms")) {
					t.Fatalf("same-board multiple values changed: %s", line)
				}
			})
		}
	}
}

// Missing metadata is a legacy/consumer control derived from a real row, not
// a claim that a fresh native query omitted its selected-window receipt.
func TestRankScopePublicMissingWindowDoesNotBorrowRequest(t *testing.T) {
	root := t.TempDir()
	path := rankScopePublicCapture(t, root, "capture")
	wide := rankScopePublicQuery(t, root, path, 2.0205)
	var row types.ObservationRecord
	for _, record := range wide.Observations {
		if record.Subject == "app-100" && strings.Contains(record.ID, "#root_cause_rank:") {
			row = record
			break
		}
	}
	row.RichNotes = append([]string(nil), row.RichNotes...)
	for i, note := range row.RichNotes {
		if strings.HasPrefix(note, "selected_window=") {
			row.RichNotes[i] = "selected_window="
		}
	}
	if types.TraceRankBoardDisplayIdentityFromRecord(row).Complete {
		t.Fatal("HARNESS: window removal left a complete board")
	}
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			body := rankScopePublicAppendix(t, root, lang, []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: []types.ObservationRecord{row}}})
			line := rankScopePublicLine(t, body, lang, "app-100")
			if !strings.Contains(line, "0.020ms") || (!strings.Contains(line, "榜域信息不完整") && !strings.Contains(line, "rank domains are incomplete")) || strings.Contains(line, "2.000000..2.020") {
				t.Fatalf("unknown scope borrowed a window or lost its original value/boundary: %s", line)
			}
		})
	}
}
