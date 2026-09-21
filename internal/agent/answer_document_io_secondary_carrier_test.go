package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func ioCarrierInitialMessages(t *testing.T, ctx *types.AgentContext) string {
	t.Helper()
	registry := skill.NewRegistry()
	skill.RegisterDefaults(registry)
	cfg, err := registry.Get("answer-document-skill")
	if err != nil {
		t.Fatal(err)
	}
	assembler := DefaultPromptAssembler()
	messages := assembler.RenderMessages(assembler.AssembleContext(ctx, cfg))
	messages = AppendDynamicInstruction(messages, &answerDocumentEvaluator{}, ctx, cfg)
	var out strings.Builder
	for _, message := range messages {
		out.WriteString(message.Content)
		out.WriteByte('\n')
	}
	return out.String()
}

func ioCarrierAuthorityLine(prompt, id string) string {
	for _, line := range strings.Split(prompt, "\n") {
		if strings.Contains(line, "id=`"+id+"`") && strings.Contains(line, "predicate=`io_latency`") {
			return line
		}
	}
	return ""
}

// A public critical-blocking query republishes its closed 31ms interval as an
// io_latency EvidenceFact. That record is not the independent 35ms physical
// request, even when a request with matching names is already in the context.
func TestIOSecondaryCarrierPublicFinalizerKeepsRequestAndObservationDistinct(t *testing.T) {
	fixture, err := os.ReadFile("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"zh", "en"} {
		for _, rename := range []bool{false, true} {
			for _, differentSource := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/rename=%t/different_source=%t", lang, rename, differentSource), func(t *testing.T) {
					body, worker := string(fixture), "document-worker"
					if rename {
						body = strings.NewReplacer("document-worker", "loader", "app-main", "client", "storage-irq", "device-irq", "OpenDocument", "OpenAsset").Replace(body)
						worker = "loader"
					}
					dir := t.TempDir()
					path := filepath.Join(dir, "capture-a.systrace")
					if err := os.WriteFile(path, []byte(body), 0600); err != nil {
						t.Fatal(err)
					}
					secondaryPath := path
					if differentSource {
						secondaryPath = filepath.Join(dir, "capture-b.systrace")
						if err := os.WriteFile(secondaryPath, []byte(body), 0600); err != nil {
							t.Fatal(err)
						}
					}
					start, end := 1.0, 1.051
					ctx := &types.AgentContext{
						RepoRoot: dir, WorkDir: dir, AgentName: types.AgentFinalizer, Stage: types.StageFinalize, Language: lang,
						Mutable: types.NewMutableState("Explain the independently measured IO and response intervals"),
						AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
							Language: lang, Intent: types.IntentRootCause, PerfTrace: &types.PerfBundle{},
							RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis},
							RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end},
						}},
					}
					var request, secondary types.ObservationRecord
					nativeBefore := map[string]string{}
					for _, query := range []struct{ view, path string }{{"window_stats", path}, {"critical_blocking_calls", secondaryPath}} {
						params, _ := json.Marshal(map[string]any{"source": "path", "path": query.path, "view": query.view, "pid": 100, "time_start": start, "time_end": end, "trace_flavor": "harmony_hitrace"})
						result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
						if err != nil || !result.Success {
							t.Fatalf("public %s failed: %v %s", query.view, err, result.Summary)
						}
						data, err := os.ReadFile(result.RawRef)
						if err != nil {
							t.Fatal(err)
						}
						nativeBefore[result.RawRef] = string(data)
						if query.view == "critical_blocking_calls" {
							var native tracequery.Result
							if err := json.Unmarshal(data, &native); err != nil {
								t.Fatal(err)
							}
							closed := false
							for _, item := range native.CriticalBlocking.Items {
								closed = closed || item.Thread.PID == 200 && item.ResourceCompletionClosure && fmt.Sprintf("%.3f", item.DurationMs) == "31.000"
							}
							if !closed {
								t.Fatal("premise: native critical-blocking result must already prove 31ms")
							}
						}
						for _, row := range result.Observations {
							if row.Subject != worker+"-200" || row.Predicate != "io_latency" {
								continue
							}
							if query.view == "window_stats" {
								request = row
							} else {
								secondary = row
							}
						}
						ctx.Mutable.AppendDispatchToolResult(result)
					}
					if request.Value != "35.000" || traceQueryObservationSupplementNoteValue(request, types.TraceNoteKeyIOIssuerBlocked) != "31.000" ||
						secondary.ID == "" || len(secondary.RichNotes) != 0 || secondary.Span.StartTs != 1.00901 || secondary.Span.EndTs != 1.04001 {
						t.Fatalf("premise: need separate request and untyped secondary observation: request=%+v secondary=%+v", request, secondary)
					}
					ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
					ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary, Text: "Model-owned original answer 8.765ms"}}})
					before := b1645Snapshot(t, []any{ctx.Mutable.TurnAArtifacts(), ctx.Mutable.AnswerDocumentV2(), types.CompileTraceCausalProjectionSet(answerDocObservationLedger(ctx))})
					prompt := ioCarrierInitialMessages(t, ctx)
					requestLine, secondaryLine := ioCarrierAuthorityLine(prompt, request.ID), ioCarrierAuthorityLine(prompt, secondary.ID)
					for _, want := range []string{"request_residence=`35.000`", "issuer_blocked=`31.000`", "completion_woke_issuer=`true`", "observed_interval=`1.005000..1.040000`", path} {
						if !strings.Contains(requestLine, want) {
							t.Errorf("original request card lost %q: %s", want, requestLine)
						}
					}
					for _, want := range []string{"observed_interval=`1.009010..1.040010`", secondaryPath, secondary.SourceRef.QueryScopeID, worker + "-200"} {
						if !strings.Contains(secondaryLine, want) {
							t.Errorf("secondary observation/source was dropped: missing %q in %s", want, secondaryLine)
						}
					}
					meaning := "This IO-related observation carries no independent typed request measurement"
					if lang == "zh" {
						meaning = "本条 IO 相关观测未携带独立的请求计量字段"
					}
					if !strings.Contains(secondaryLine, meaning) {
						t.Errorf("secondary observation lacks its own carrier meaning: %s", secondaryLine)
					}
					for _, forbidden := range []string{"该请求的完成唤醒证明未发布", "Completion-to-issuer wakeup proof is unpublished", "completion_woke_issuer=", "request_residence=", "issuer_blocked="} {
						if strings.Contains(secondaryLine, forbidden) {
							t.Errorf("secondary observation borrowed/invented request meaning %q: %s", forbidden, secondaryLine)
						}
					}
					if before != b1645Snapshot(t, []any{ctx.Mutable.TurnAArtifacts(), ctx.Mutable.AnswerDocumentV2(), types.CompileTraceCausalProjectionSet(answerDocObservationLedger(ctx))}) {
						t.Fatal("display mutated native observations, model answer or root authority")
					}
					for file, original := range nativeBefore {
						data, err := os.ReadFile(file)
						if err != nil || string(data) != original {
							t.Fatal("display mutated native result")
						}
					}
				})
			}
		}
	}
}

func TestIOSecondaryCarrierLegacyAndRequestProofRemainDistinct(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, mode := range []string{"legacy_no_notes", "legacy_source_notes", "request_true", "request_false", "request_unknown"} {
			t.Run(lang+"/"+mode, func(t *testing.T) {
				ctx := b1644IOCompletionContext(t, "block_rq", "proven", lang)
				causalIOInstructionContext(ctx)
				artifacts := *ctx.Mutable.TurnAArtifacts()
				var record types.ObservationRecord
				for _, row := range artifacts.ToolResults[0].Observations {
					if row.Predicate == "io_latency" && row.Value == "0.100" {
						record = row
					}
				}
				if record.ID == "" {
					t.Fatal("public request absent")
				}
				if strings.HasPrefix(mode, "legacy") {
					record.RichNotes = nil
					if mode == "legacy_source_notes" {
						record.RichNotes = []string{"selected_window=5.000000..5.001000"}
					}
					// Neither prose nor an IO-looking object/interval is request
					// identity or permission to reconstruct missing proof notes.
					record.Summary = "block_rq_issue to block_rq_complete proves blocking 999ms"
				} else {
					var notes []string
					for _, note := range record.RichNotes {
						if !strings.HasPrefix(note, types.TraceNoteKeyIOCompletionWokeIssuer+"=") {
							notes = append(notes, note)
						}
					}
					if mode != "request_unknown" {
						notes = append(notes, types.TraceNoteKeyIOCompletionWokeIssuer+"="+strings.TrimPrefix(mode, "request_"))
					}
					record.RichNotes = notes
				}
				artifacts.ToolResults[0].Observations = []types.ObservationRecord{record}
				ctx.Mutable = types.NewMutableState("legacy publication review")
				ctx.Mutable.SetTurnAArtifacts(artifacts)
				before := b1645Snapshot(t, ctx.Mutable.TurnAArtifacts())
				line := ioCarrierAuthorityLine(ioCarrierInitialMessages(t, ctx), record.ID)
				if !strings.Contains(line, "value=`0.100ms`") || !strings.Contains(line, record.SourceRef.Path) {
					t.Fatalf("original value/source lost: %s", line)
				}
				if strings.HasPrefix(mode, "legacy") {
					for _, forbidden := range []string{"该请求的完成唤醒证明未发布", "Completion-to-issuer wakeup proof is unpublished", "completion_woke_issuer=", "999ms"} {
						if strings.Contains(line, forbidden) {
							t.Errorf("legacy observation interpreted/reparsed as request: %s", line)
						}
					}
				} else {
					proof := strings.TrimPrefix(mode, "request_")
					if proof == "unknown" {
						proof = ""
						if strings.Contains(line, "completion_woke_issuer=") {
							t.Fatal("absent proof became a Boolean")
						}
					}
					if !strings.Contains(line, answerDocIOCompletionProofMeaning(proof, lang)) || !strings.Contains(line, "request_residence=`0.100`") {
						t.Errorf("real request's %s proof meaning changed: %s", mode, line)
					}
				}
				if before != b1645Snapshot(t, ctx.Mutable.TurnAArtifacts()) {
					t.Fatal("legacy source publication was rewritten")
				}
			})
		}
	}
}

func TestIOSecondaryCarrierPartialRequestMetadataKeepsUnknownProof(t *testing.T) {
	for _, key := range []string{types.TraceNoteKeyIORequestResidence, types.TraceNoteKeyIORequestResidenceCaliber, types.TraceNoteKeyIORequestResidenceClock} {
		for _, lang := range []string{"zh", "en"} {
			record := types.ObservationRecord{RichNotes: []string{key + "=present"}}
			if got := answerDocIOObservationProofMeaning(record, lang); got != answerDocIOCompletionProofMeaning("", lang) {
				t.Errorf("partial metadata %s lost unknown-proof request explanation: %s", key, got)
			}
		}
	}
}
