package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Synthetic producer-shaped rows, not a customer capture. Preview and full
// leaves deliberately have separate contracts and share one exact result.
func b1618P2WaitReaderRows(prefix, path, result string, start, end float64, count int) []types.ObservationRecord {
	emitted, status := count, "complete"
	if emitted > 8 {
		emitted, status = 8, "incomplete"
	}
	scope := fmt.Sprintf("selected_window=%.6f..%.6f", start, end)
	ref := types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: path, PayloadRef: result, RawRef: result}
	set := types.ObservationRecord{
		ID: prefix + "#target_window_wait_occurrences", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
		Role: types.AnswerAggregateRoleSupportingCoverage, Subject: "app-100",
		Predicate: "target_window_wait_occurrences", Object: "complete", Value: fmt.Sprint(count), Unit: "occurrences", ResultCount: &count,
		Span:      types.ObservationSpan{StartTs: start, EndTs: end},
		RichNotes: []string{scope, fmt.Sprintf("target_wait_occurrence_prompt=status=%s,emitted=%d,total=%d", status, emitted, count), fmt.Sprintf("target_wait_occurrence_prompt_sum_ms=%.3f", float64(emitted))},
	}
	rows := []types.ObservationRecord{set}
	for i := 1; i <= count; i++ {
		rowStart := start + float64(i-1)*.001
		rowEnd := start + float64(i)*.001
		rows = append(rows, types.ObservationRecord{
			ID: fmt.Sprintf("%s#target_window_wait_occurrence:%d", prefix, i), Origin: types.AnswerEvidenceOriginRuntimeArtifact,
			Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
			Role: types.AnswerAggregateRoleSupportingCoverage, Subject: "app-100",
			Predicate: "target_window_wait_occurrence", Object: "state=d_sleep;iowait=0;caller=kernel_wait", Value: "1.000", Unit: "ms",
			Span: types.ObservationSpan{StartTs: rowStart, EndTs: rowEnd, LineStart: i * 2, LineEnd: i*2 + 1}, RichNotes: []string{scope},
		})
		if i <= emitted {
			rows[0].RichNotes = append(rows[0].RichNotes, fmt.Sprintf("target_wait_occurrence=#%d state=d_sleep %.6f..%.6f duration=1.000ms iowait=0 caller=kernel_wait lines=%d-%d reason_line=%d", i, rowStart, rowEnd, i*2, i*2+1, i*2))
		}
	}
	return rows
}

func b1618P2WaitReaderContext(records []types.ObservationRecord, lang string) *types.AgentContext {
	mut := types.NewMutableState("typed wait reader scope")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: records}}})
	return &types.AgentContext{Mutable: mut, Language: lang, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Language: lang, Intent: types.IntentTrace,
		RuntimeTargets:         []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 100, Thread: "app-100", Source: "user_explicit"}},
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetWaitOccurrences}},
	}}}
}

func TestB1618P2WaitReaderShowsEachCaptureAndQuery(t *testing.T) {
	rows := append(b1618P2WaitReaderRows("trace_query:a", "/captures/A/same.trace", "result-a", 10, 10.020, 0), b1618P2WaitReaderRows("trace_query:b", "/captures/B/same.trace", "result-b", 20, 20.020, 0)...)
	for _, lang := range []string{"zh", "en"} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse=%t", lang, reverse), func(t *testing.T) {
				input := append([]types.ObservationRecord(nil), rows...)
				if reverse {
					input[0], input[1] = input[1], input[0]
				}
				before, _ := json.Marshal(input)
				ctx := b1618P2WaitReaderContext(input, lang)
				for _, got := range []string{renderAnswerDocTargetWaitOccurrenceAuthority(ctx), renderAnswerDocBoundedRuntimeFinalReaderHandoff(ctx)} {
					for _, want := range []string{"/captures/A/same.trace", "/captures/B/same.trace", "10.000000", "10.020000", "20.000000", "20.020000"} {
						if !strings.Contains(got, want) {
							t.Errorf("reader lost own capture/query %q:\n%s", want, got)
						}
					}
				}
				after, _ := json.Marshal(input)
				if string(before) != string(after) {
					t.Fatal("reader changed original records")
				}
			})
		}
	}
}

func TestB1618P2WaitReaderDoesNotShadowOtherWindowFullRoster(t *testing.T) {
	preview := b1618P2WaitReaderRows("trace_query:preview", "/captures/A.trace", "result-eight", 10, 10.020, 8)
	full := b1618P2WaitReaderRows("trace_query:full", "/captures/A.trace", "result-eleven", 20, 20.020, 11)
	for _, reverse := range []bool{false, true} {
		records := append(append([]types.ObservationRecord(nil), preview...), full...)
		if reverse {
			records = append(append([]types.ObservationRecord(nil), full...), preview...)
		}
		ctx := b1618P2WaitReaderContext(records, "en")
		ledger := types.ObservationLedger{Records: records}
		shadowed := answerDocReaderAuthorityShadowedObservationIDs(ctx, ledger)
		for _, record := range preview {
			if !shadowed[record.ID] {
				t.Errorf("exactly restated preview source should be shadowed: %s", record.ID)
			}
		}
		for _, record := range full {
			if shadowed[record.ID] {
				t.Errorf("an eight-item preview cannot shadow another query's eleven-item full roster: %s", record.ID)
			}
		}
		if got := renderTraceFinalTargetWaitEnumerationAuthority(ledger, &ctx.AnalysisIR.RequestModel); !strings.Contains(got, "occurrence_count=11") {
			t.Errorf("full-list authority disappeared:\n%s", got)
		}
	}
}

func TestB1618P2WaitReaderDoesNotShadowAmbiguousSourceID(t *testing.T) {
	for _, otherResult := range []bool{false, true} {
		for _, reverse := range []bool{false, true} {
			shown := b1618P2WaitReaderRows("trace_query:collision", "/captures/A/same.trace", "result-a", 10, 10.020, 0)[0]
			other := shown
			other.RichNotes = []string{"selected_window=10.000000..10.020000", "target_wait_occurrence_prompt=status=incomplete,emitted=0,total=11", "target_wait_occurrence_prompt_sum_ms=0.000"}
			if otherResult {
				other.SourceRef.PayloadRef, other.SourceRef.RawRef = "result-b", "result-b"
			} else {
				other.SourceRef.Path = "/captures/B/same.trace"
			}
			records := []types.ObservationRecord{shown, other}
			if reverse {
				records[0], records[1] = records[1], records[0]
			}
			ctx := b1618P2WaitReaderContext(records, "en")
			if shadowed := answerDocReaderAuthorityShadowedObservationIDs(ctx, types.ObservationLedger{Records: records}); shadowed[shown.ID] {
				t.Errorf("same source ID with unrepresented capture/result must remain visible: otherResult=%t reverse=%t", otherResult, reverse)
			}
		}
	}
}

func TestB1618P2WaitPrincipalDoesNotReplaceAnotherCapture(t *testing.T) {
	for _, sameCapture := range []bool{false, true} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("sameCapture=%t/reverse=%t", sameCapture, reverse), func(t *testing.T) {
				principal := b1618P2WaitReaderRows("trace_query:principal", "/captures/A/same.trace", "full-result", 10, 30, 1)
				for i := range principal {
					principal[i].SystemSupplement = true
				}
				path := "/captures/B/same.trace"
				if sameCapture {
					path = "/captures/A/same.trace"
				}
				supporting := b1618P2WaitReaderRows("trace_query:supporting", path, "finite-result", 20, 20.020, 2)
				records := append(append([]types.ObservationRecord(nil), principal...), supporting...)
				if reverse {
					records = append(append([]types.ObservationRecord(nil), supporting...), principal...)
				}
				ctx := b1618P2WaitReaderContext(records, "en")
				ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeFullArtifact, SourceQuote: "the entire capture"}
				rm := &ctx.AnalysisIR.RequestModel
				ledger := types.ObservationLedger{Records: records}
				waits := types.BuildTraceTargetWaitSummaryAuthorities(ledger, rm)
				if len(waits) != 2 || !waits[0].IsRequestedScopePrincipal() || waits[1].IsRequestedScopePrincipal() {
					t.Fatalf("fixture must retain principal and supporting accounts: %+v", waits)
				}
				enumeration := renderTraceFinalTargetWaitEnumerationAuthority(ledger, rm)
				recap := renderAnswerDocTracePrincipalValueAuthority(ctx)
				wantCount, wantRows := 2, 3
				if sameCapture {
					wantCount, wantRows = 1, 1
				}
				if got := strings.Count(enumeration, "target_wait_enumeration_authority artifact="); got != wantCount {
					t.Errorf("principal must replace only its own capture/target domain: got=%d want=%d\n%s", got, wantCount, enumeration)
				}
				if got := strings.Count(recap, "principal_occurrence=`"); got != wantRows {
					t.Errorf("recap borrowed another capture's principal scope: got=%d want=%d\n%s", got, wantRows, recap)
				}
			})
		}
	}
}

func TestB1618P2WaitReaderKeepsUnknownQueryWithoutInventingZeroWindow(t *testing.T) {
	records := b1618P2WaitReaderRows("trace_query:unknown", "/captures/unknown.trace", "unknown-query-result", 10, 10.020, 1)
	for i := range records {
		records[i].RichNotes = records[i].RichNotes[1:] // no producer query identity; member spans remain real
	}
	for _, lang := range []string{"zh", "en"} {
		ctx := b1618P2WaitReaderContext(records, lang)
		ledger := types.ObservationLedger{Records: records}
		if got := types.BuildTraceTargetWaitSummaryAuthorities(ledger, &ctx.AnalysisIR.RequestModel); len(got) != 1 || got[0].WallClockMS != 1 {
			t.Fatalf("unknown query still has independently measured wait: %+v", got)
		}
		for _, got := range []string{
			renderAnswerDocTargetWaitOccurrenceAuthority(ctx),
			renderTraceFinalTargetWaitEnumerationAuthority(ledger, &ctx.AnalysisIR.RequestModel),
			renderAnswerDocTracePrincipalValueAuthority(ctx),
		} {
			if strings.Contains(got, "0.000000..0.000000") || strings.Contains(got, "0.000000–0.000000") {
				t.Errorf("missing query became an actual zero window:\n%s", got)
			}
			if !strings.Contains(got, "unknown") && !strings.Contains(got, "未明确") {
				t.Errorf("missing query boundary was not disclosed:\n%s", got)
			}
		}
		if got := answerDocReaderAuthorityShadowedObservationIDs(ctx, ledger); len(got) != 0 {
			t.Errorf("unknown query must not shadow by borrowed scope: %v", got)
		}
	}
}

func TestB1618P2WaitZeroEnumerationDoesNotInventOrdinalOne(t *testing.T) {
	records := b1618P2WaitReaderRows("trace_query:zero", "/captures/zero.trace", "zero-result", 10, 10.020, 0)
	ctx := b1618P2WaitReaderContext(records, "en")
	got := renderTraceFinalTargetWaitEnumerationAuthority(types.ObservationLedger{Records: records}, &ctx.AnalysisIR.RequestModel)
	if !strings.Contains(got, "occurrence_count=0") || !strings.Contains(got, "complete_occurrence_ordinals=`none`") || strings.Contains(got, "1..0") {
		t.Fatalf("measured zero should publish an empty ordinal set:\n%s", got)
	}
}

func TestB1618P2WaitReaderShadowUsesSameSourcesInLedgerAndToolHandoff(t *testing.T) {
	preview := b1618P2WaitReaderRows("trace_query:preview", "/captures/A.trace", "result-eight", 10, 10.020, 8)
	full := b1618P2WaitReaderRows("trace_query:full", "/captures/A.trace", "result-eleven", 20, 20.020, 11)
	records := append(append([]types.ObservationRecord(nil), preview...), full...)
	ctx := b1618P2WaitReaderContext(records, "en")
	var refs []types.ToolObservationRef
	for _, record := range records {
		ref, ok := types.ToolObservationRefFromObservationRecord(record)
		if !ok {
			t.Fatalf("producer-shaped row must have a handoff ref: %s", record.ID)
		}
		refs = append(refs, ref)
	}
	artifacts := *ctx.Mutable.TurnAArtifacts()
	artifacts.HandoffCarriers = []types.ToolHandoffCarrier{{Version: types.ToolHandoffCarrierVersion, ToolName: "trace_query", ReasonCode: "tool_observation_handoff", ObservationRefs: refs}}
	ctx.Mutable.SetTurnAArtifacts(artifacts)
	ledger := answerDocObservationLedger(ctx)
	projected := answerDocObservationPromptRecords(ctx, ledger.Records, len(records))
	projected = answerDocObservationRecordsWithoutReaderAuthorityDuplicates(ctx, ledger, projected)
	visibleLedger := map[string]bool{}
	for _, record := range projected {
		visibleLedger[record.ID] = true
	}
	visibleHandoff := map[string]bool{}
	for _, carrier := range answerDocToolHandoffCarriersForFinalizer(ctx, artifacts.HandoffCarriers) {
		for _, ref := range carrier.ObservationRefs {
			visibleHandoff[ref.ID] = true
		}
	}
	for _, record := range full {
		if !visibleLedger[record.ID] || !visibleHandoff[record.ID] {
			t.Errorf("another query's full-list source must survive both actual consumers: %s ledger=%t handoff=%t", record.ID, visibleLedger[record.ID], visibleHandoff[record.ID])
		}
	}
	for _, record := range preview {
		if visibleLedger[record.ID] || visibleHandoff[record.ID] {
			t.Errorf("exactly restated preview source should be removed from both duplicate surfaces: %s", record.ID)
		}
	}
	if len(ledger.Records) != len(records) {
		t.Fatalf("lossless observation ledger was altered: got %d want %d", len(ledger.Records), len(records))
	}
}

func TestB1618P2WaitReaderRetainsUnverifiedSourceInActualPromptAndHandoff(t *testing.T) {
	for _, tc := range []struct {
		name           string
		kind           types.ObservationSourceKind
		receipt        bool
		rawShadow      bool
		compiledShadow bool
	}{
		{name: "complete-runtime-source", kind: types.ObservationSourceRuntimeArtifact, receipt: true, rawShadow: true, compiledShadow: true},
		// The ledger's existing origin-owned normalization fills a missing
		// kind, and corrects a current_source badge on a runtime observation.
		// Pin this real positive path rather than pretending it stays unknown.
		{name: "missing-kind-normalized-from-runtime-origin", receipt: true, compiledShadow: true},
		{name: "current-source-kind-normalized-from-runtime-origin", kind: types.ObservationSourceCurrentSource, receipt: true, compiledShadow: true},
		{name: "wrong-source-kind-remains-web", kind: types.ObservationSourceWebPage, receipt: true},
		{name: "missing-result-receipt", kind: types.ObservationSourceRuntimeArtifact},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The bounded note roster stands on its existing soft contract;
			// there are intentionally no full-list leaves to lend provenance.
			record := b1618P2WaitReaderRows("trace_query:local-preview", "/captures/local.trace", "local-result", 10, 10.020, 1)[0]
			record.SourceRef.Kind = tc.kind
			if !tc.receipt {
				record.SourceRef.PayloadRef, record.SourceRef.RawRef = "", ""
			}
			ctx := b1618P2WaitReaderContext([]types.ObservationRecord{record}, "en")
			// Exercise the ordinary ledger rendering lane independently of the
			// separate requested-fact recap, which deliberately retains IDs.
			ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = nil
			rawLedger := types.ObservationLedger{Records: []types.ObservationRecord{record}}
			rawAuthorities := types.BuildTargetWaitOccurrenceAuthorities(rawLedger, &ctx.AnalysisIR.RequestModel)
			if len(rawAuthorities) != 1 || rawAuthorities[0].Count != 1 || len(rawAuthorities[0].Rows) != 1 {
				t.Fatalf("raw preview count/rows disappeared: %+v", rawAuthorities)
			}
			if got := len(rawAuthorities[0].SourceRecordIDs) > 0; got != tc.rawShadow {
				t.Errorf("raw source IDs require runtime provenance: got=%t want=%t", got, tc.rawShadow)
			}
			rawPrompt := answerDocObservationPromptRecords(ctx, rawLedger.Records, 1)
			rawPrompt = answerDocObservationRecordsWithoutReaderAuthorityDuplicates(ctx, rawLedger, rawPrompt)
			if visible := len(rawPrompt) == 1; visible == tc.rawShadow {
				t.Errorf("actual prompt projection must retain an unverified raw source: visible=%t shadow=%t", visible, tc.rawShadow)
			}
			ref, ok := types.ToolObservationRefFromObservationRecord(record)
			if !ok {
				t.Fatal("the original observation still has a handoff reference")
			}
			artifacts := *ctx.Mutable.TurnAArtifacts()
			artifacts.HandoffCarriers = []types.ToolHandoffCarrier{{Version: types.ToolHandoffCarrierVersion, ToolName: "trace_query", ReasonCode: "tool_observation_handoff", ObservationRefs: []types.ToolObservationRef{ref}}}
			ctx.Mutable.SetTurnAArtifacts(artifacts)
			ledger := answerDocObservationLedger(ctx)
			authorities := types.BuildTargetWaitOccurrenceAuthorities(ledger, &ctx.AnalysisIR.RequestModel)
			if len(authorities) != 1 || authorities[0].Count != 1 || authorities[0].SumMS != 1 || len(authorities[0].Rows) != 1 || authorities[0].Rows[0].DurationM != 1 {
				t.Fatalf("local preview measurements must remain available without borrowing source proof: %+v", authorities)
			}
			if got := len(authorities[0].SourceRecordIDs) > 0; got != tc.compiledShadow {
				t.Errorf("source IDs respect the compiled runtime provenance: got=%t want=%t", got, tc.compiledShadow)
			}
			reader := renderAnswerDocTargetWaitOccurrenceAuthority(ctx)
			if !strings.Contains(reader, "1 occurrence(s), totaling 1.000 ms") || !strings.Contains(reader, "Occurrence 1:") {
				t.Errorf("reader removed useful local count/rows:\n%s", reader)
			}
			for name, prompt := range map[string]string{
				"observation ledger": renderAnswerDocObservationLedger(ctx),
				"tool handoff":       renderAnswerDocToolHandoffCarriers(ctx),
			} {
				visible := strings.Contains(prompt, record.ID)
				if visible == tc.compiledShadow {
					t.Errorf("%s must retain unverified source and omit only the verified repeated source: visible=%t shadow=%t\n%s", name, visible, tc.compiledShadow, prompt)
				}
			}
		})
	}
}
