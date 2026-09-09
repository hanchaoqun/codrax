package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

const b1635aWaitScopeZH = "等待统计范围：仅限本句纳入统计的自身等待记录；若列出百分比及剩余时长，也仅针对这些记录，不能据此认定全窗等待已完整覆盖。"
const b1635aWaitScopeEN = "Wait accounting scope: only the focused thread's own wait records included in this statement; any percentages and residual time apply only to those records and do not establish complete coverage of the window's waits."

// These are producer-shaped records, not a persisted projection: the public
// mutation entry compiles the ledger and constructs its own tree and account.
// The 60ms published symptom is deliberately not the account's 80ms wait.
func b1635aWaitScopeBus(lang, accountKind string, reverse bool) *types.BusContext {
	ref := types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact,
		Path: "/captures/a/trace.ftrace", ArtifactID: "trace.ftrace",
		PayloadRef: "/results/a/query.json", RawRef: "/results/a/query.json", QueryScopeID: "scope-a"}
	base := func(id, predicate, subject, object, value string) types.ObservationRecord {
		return types.ObservationRecord{
			ID: id, Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			Role: types.AnswerAggregateRolePrincipalAnswer, GroundingPolicy: types.ClaimGroundingHard,
			Predicate: predicate, ClaimKey: predicate + ":" + id, Subject: subject, Object: object,
			Value: value, Unit: "ms", SourceRef: ref, ObservedAt: "2026-09-09T00:00:00Z",
			Span:        types.ObservationSpan{LineStart: 10, LineEnd: 20, StartTs: 10.01, EndTs: 10.07},
			SupportRefs: []string{"/captures/a/trace.ftrace:10-20"}, Confidence: 0.9,
			RichNotes: []string{"selected_window=10.000000..10.100000"},
		}
	}
	self := base("self", "root_cause_target_self_state", "app-100", "sleep_wait", "60.000")
	self.RichNotes = append(self.RichNotes, "tier=target_self_state", "dominant_state=s_sleep", "chain_relevance=on_chain", "impact_ms=60.000", "cumulative_impact_ms=60.000")
	chain := base("chain", "root_cause_primary", "worker-200", "runnable_wait", "6.000")
	chain.RichNotes = append(chain.RichNotes, "rank=1", "tier=primary", "dominant_state=runnable", "chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1", "impact_ms=6.000", "cumulative_impact_ms=6.000", "effective_impact_ms=6.000")
	path := base("path", "wakeup_chain", "app-100", "worker-200 -> app-100", "")
	observations := []types.ObservationRecord{self, chain, path}
	if accountKind != "missing" {
		account := base("account", "target_window_states", "app-100", "state_partition", "90.000")
		account.Role = types.AnswerAggregateRoleSupportingCoverage
		running, total := "10.000", "90.000"
		if accountKind == "complete" {
			running, total = "20.000", "100.000"
		}
		if accountKind == "foreign_capture" {
			account.SourceRef.Path = "/captures/b/trace.ftrace"
			account.SourceRef.PayloadRef, account.SourceRef.RawRef = "/results/b/query.json", "/results/b/query.json"
		}
		if accountKind == "foreign_target" {
			account.Subject = "other-300"
		}
		if accountKind == "foreign_window" {
			account.RichNotes = []string{"selected_window=20.000000..20.100000"}
		}
		if accountKind == "zero" {
			running, total = "0.000", "0.000"
			account.RichNotes = append(account.RichNotes, "running=0", "runnable=0", "sleep=0", "d_state=0", "io_wait=0", "total=0")
		} else {
			account.RichNotes = append(account.RichNotes, "running="+running, "runnable=5.000", "sleep=75.000", "d_state=0.000", "io_wait=0.000", "total="+total)
		}
		account.Value = total
		observations = append(observations, account)
	}
	if reverse {
		for i, j := 0, len(observations)-1; i < j; i, j = i+1, j-1 {
			observations[i], observations[j] = observations[j], observations[i]
		}
	}
	return &types.BusContext{
		Mutable: types.NewMutableState("B1635a wait scope"), Language: lang,
		AnalysisIR: &types.AnalysisIR{
			RequestModel:   types.RequestModel{Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck},
			AnswerContract: types.AnswerContract{Language: lang},
		},
		ToolResults: []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: observations}},
	}
}

func TestB1635AWaitScopePublicMutation(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, accountKind := range []string{"partial", "complete", "missing", "foreign_capture", "foreign_target", "foreign_window", "zero"} {
			for _, reverse := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/reverse=%t", lang, accountKind, reverse), func(t *testing.T) {
					bus := b1635aWaitScopeBus(lang, accountKind, reverse)
					before, err := json.Marshal(bus.ToolResults)
					if err != nil {
						t.Fatal(err)
					}
					ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
					projection := types.CompileTraceCausalProjection(ledger)
					model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), lang == "zh")
					coverage := runtimeTraceProjCoverageVerdictFor(projection, model)
					if coverage.SymptomMS != 60 || coverage.AttributedMS != 6 || !coverage.Comparable {
						t.Fatalf("fixture must reach the actual symptom coverage branch: %+v; self=%+v", coverage, model.SelfRows)
					}
					if accountKind == "partial" && runtimeTraceProjFourStateAccountProvable(projection, model) != nil {
						t.Fatal("partial account must not gain the full-window identity")
					}
					if accountKind == "complete" && runtimeTraceProjFourStateAccountProvable(projection, model) == nil {
						t.Fatal("complete same-target account must retain the original identity")
					}
					doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
						ID: "model-summary", Kind: types.BlockSummary, Text: "Model diagnosis stays unchanged. 模型保留自身结论。",
					}}}
					modelWire, err := modelOwnedAnswerBlockWire(doc)
					if err != nil {
						t.Fatal(err)
					}
					result, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
					if err != nil || !result.Success {
						t.Fatalf("public mutation: %+v %v", result, err)
					}
					stored := bus.Mutable.AnswerDocumentV2()
					if err := requireModelOwnedAnswerBlockWirePreserved(modelWire, stored); err != nil {
						t.Fatal(err)
					}
					var lead string
					for _, block := range stored.Blocks {
						if RuntimeTraceSystemBlock(block) && runtimeTraceCausalProjectionStandaloneLeadBlockID(block.ID) && strings.Contains(block.Text, "60.000ms") {
							lead = block.Text
							break
						}
					}
					if lead == "" {
						t.Fatal("public path did not publish the target wait projection")
					}
					want := b1635aWaitScopeEN
					if lang == "zh" {
						want = b1635aWaitScopeZH
					}
					if strings.Count(lead, want) != 1 {
						t.Fatalf("wait scope must appear once even without a balanced account; want %q:\n%s", want, lead)
					}
					waitPrefix := "- Of the focused thread's 60.000ms wait time"
					if lang == "zh" {
						waitPrefix = "- 关注线程等待(sleep/D-state/runnable) 60.000ms"
					}
					if !strings.Contains(lead, want+"\n"+waitPrefix) {
						t.Fatal("scope disclosure must be adjacent to its unchanged wait statement")
					}
					// Preserve the original ratio, denominator, residual and rank data.
					for _, number := range []string{"60.000ms", "6.000ms", "54.000ms", "10%", "90%"} {
						if !strings.Contains(lead, number) {
							t.Errorf("original %s missing", number)
						}
					}
					after, err := json.Marshal(bus.ToolResults)
					if err != nil {
						t.Fatal(err)
					}
					if string(after) != string(before) {
						t.Fatal("publication mutated source observations")
					}
					if got := runtimeTraceProjCoverageVerdictFor(projection, model); !reflect.DeepEqual(got, coverage) {
						t.Fatal("publication changed the coverage verdict")
					}
					storedWire, err := json.Marshal(stored)
					if err != nil {
						t.Fatal(err)
					}
					if materializeRuntimeTraceCausalProjectionBlock(stored, bus) {
						t.Fatal("an already-published projection must not add another scope disclosure")
					}
					repeatedWire, err := json.Marshal(stored)
					if err != nil || string(repeatedWire) != string(storedWire) {
						t.Fatal("repeated materialization changed the published document")
					}
				})
			}
		}
	}
}

func TestB1635AWaitScopeDoesNotLabelWholeWindowFallback(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		projection := types.TraceCausalProjection{WindowStartTs: 100, WindowEndTs: 100.101}
		model := custom1gWindowModel(false)
		if got, _, _, _ := runtimeTraceProjTargetSymptomAdmission(model); got != 0 {
			t.Fatalf("fixture denominator=%v", got)
		}
		line := runtimeTraceProjWindowLine(projection, model, lang == "zh")
		if strings.Contains(line, b1635aWaitScopeZH) || strings.Contains(line, b1635aWaitScopeEN) {
			t.Fatalf("whole-window fallback must not claim a wait-record denominator: %s", line)
		}
	}
}
