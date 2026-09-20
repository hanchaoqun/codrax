package agent

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Pin the public query -> TurnA -> finalizer/semantic-compact path for each
// independently measured group. The original failure lost a worker result;
// subsequent replay retained numbers but exposed an ambiguous representative
// thread and lost group endpoint scope outside the request-detail Top8. An
// unrelated group, raw request or global note must not satisfy this contract.
func TestPublishedIOGroupDistributionsReachFinalizer(t *testing.T) {
	for _, scope := range []types.RuntimeQuestionScope{types.RuntimeQuestionScopeCausalDiagnosis, types.RuntimeQuestionScopeBoundedFactSet} {
		t.Run(string(scope), func(t *testing.T) {
			path, err := filepath.Abs(filepath.Join("..", "..", "eval", "fixtures", "hmosperf_io_request_latency_distribution", "events.systrace"))
			if err != nil {
				t.Fatal(err)
			}
			start, end := 1.0, 14.0
			ctx := &types.AgentContext{
				RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh",
				AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
				Mutable: types.NewMutableState("Describe the measured IO groups and their limits"),
				AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
					Language: "zh", PerfTrace: &types.PerfBundle{},
					RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: scope, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactIOLatency}},
					RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1..14 seconds"},
				}},
			}
			params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "time_start": start, "time_end": end})
			result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
			if err != nil || !result.Success {
				t.Fatalf("query failed: %v %+v", err, result)
			}
			ctx.Mutable.AppendDispatchToolResult(result)
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
			ledger := answerDocObservationLedger(ctx)
			before, _ := json.Marshal(ledger)
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			semantic := types.ProjectObservationPromptRecords(ledger.Records, &ctx.AnalysisIR.RequestModel, nil,
				types.SemanticReviewObservationPromptProjectionOptions(types.ObservationPromptRecordLimit))
			for _, group := range []struct {
				identity, numbers, startEvent, endEvent string
			}{
				{"event=block_rq dev=12,80 op=R", "samples=11 min=1.000 mean=6.000 max=11.000 p50=6.000 p90=10.000 p95=10.500 p99=10.900", "block_rq_issue", "block_rq_complete"},
				{"event=block_rq dev=12,80 op=W", "samples=3 min=5.000 mean=15.000 max=25.000 p50=15.000 p90=23.000 p95=24.000 p99=24.800", "block_rq_issue", "block_rq_complete"},
				{"event=block_bio dev=12,80 op=R", "samples=3 min=2.000 mean=4.000 max=6.000 p50=4.000 p90=5.600 p95=5.800 p99=5.960", "block_bio_queue", "block_bio_complete"},
			} {
				// Find the real published group, then pin each consumer's same-ID
				// record. A token in another group or a Top8 request is not this
				// distribution's identity, endpoint or measurement authority.
				found, finalizerFound := 0, 0
				for _, record := range ledger.Records {
					if record.Predicate != "storage_latency_by_layer" ||
						!strings.Contains(traceQueryObservationSupplementNoteValue(record, "storage_request_group"), group.identity) {
						continue
					}
					found++
					if record.SourceRef.Path != path || record.SourceRef.QueryScopeID == "" {
						t.Errorf("group %s lost its original physical/query source: %+v", group.identity, record.SourceRef)
					}
					if got := traceQueryObservationSupplementNoteValue(record, types.TraceNoteKeySelectedWindow); got != "1.000000..14.000000" {
						t.Errorf("group %s selected window = %q", group.identity, got)
					}
					if strings.Contains(" "+record.Summary, " thread=") {
						t.Errorf("all-issuer group %s has an unqualified single-thread summary: %s", group.identity, record.Summary)
					}
					// Raw rich notes can be consumed outside the current compact
					// budget. Hiding a misleading identity behind truncation is
					// not sufficient; only an explicitly representative label may
					// retain the block group's original Thread metadata.
					for _, note := range record.RichNotes {
						if strings.HasPrefix(strings.TrimSpace(note), "thread=") {
							t.Errorf("all-issuer group %s retains a bare thread identity in raw notes: %s", group.identity, note)
						}
					}
					var finalizerLine string
					for _, line := range strings.Split(prompt, "\n") {
						if strings.HasPrefix(line, "- `"+record.ID+"`:") {
							if finalizerLine != "" {
								t.Errorf("finalizer repeats the same group record %s", record.ID)
							}
							finalizerLine = line
						}
					}
					if finalizerLine != "" {
						finalizerFound++
					}
					var semanticRecord *types.ObservationPromptRecord
					for i := range semantic {
						if semantic[i].ID == record.ID {
							semanticRecord = &semantic[i]
							break
						}
					}
					semanticLine := ""
					if semanticRecord != nil {
						semanticLine = semanticRecord.Summary + " " + strings.Join(semanticRecord.Notes, " ")
						if len(semanticRecord.Notes) > 6 {
							t.Errorf("semantic group %s expanded its six-note budget: %d", record.ID, len(semanticRecord.Notes))
						}
					}
					surfaces := map[string]string{"semantic compact": semanticLine}
					// The finalizer may suppress a duplicate publication of this
					// same measured group. Require at least one actual same-ID
					// line for each group, not both ordinary and EvidencePack rows.
					if finalizerLine != "" {
						surfaces["finalizer"] = finalizerLine
					}
					for surface, line := range surfaces {
						for _, want := range []string{group.identity, "issuers=all", group.numbers, group.startEvent, group.endEvent,
							"not target blocking time", "storage_source_path=" + path, "selected_window=1.000000..14.000000"} {
							if !strings.Contains(line, want) {
								t.Errorf("%s group record %s (%s) lost %q in its own record:\n%s", surface, record.ID, group.identity, want, line)
							}
						}
						if strings.Contains(" "+line, " thread=") {
							t.Errorf("%s all-issuer group %s still implies a single thread: %s", surface, record.ID, line)
						}
					}
				}
				if found == 0 {
					t.Errorf("published ledger lost distribution group %s", group.identity)
				}
				if finalizerFound == 0 {
					t.Errorf("finalizer lost every publication of distribution group %s", group.identity)
				}
			}
			for _, want := range []string{"issuers=all", "ambiguous_cohorts=1", "pairing_suppressed=2", "unpaired_start=1", "selected_window=1.000000..14.000000"} {
				if !strings.Contains(prompt, want) {
					t.Errorf("finalizer lost scope/pairing disclosure %q", want)
				}
			}
			after, _ := json.Marshal(answerDocObservationLedger(ctx))
			if string(before) != string(after) {
				t.Fatal("display mutated accepted measurements")
			}
			start, end = 2, 3
			if strings.Contains(renderAnswerDocObservationLedger(ctx), "p99=10.900") {
				t.Fatal("wider query population entered narrower requested-window ledger")
			}
		})
	}
}
