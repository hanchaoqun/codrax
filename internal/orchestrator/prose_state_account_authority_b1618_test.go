package orchestrator

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Starts at the public aggregate/producer ledger inputs and exercises the
// normal appendix publication entry. The unrelated query is an actual local
// TraceQuery; it must not lend its authority to model-authored state values.
func TestB1618ModelAggregateCannotBorrowUnrelatedQueryStateAuthority(t *testing.T) {
	root := t.TempDir()
	path := b1618StateCapture(t, root, "real-events", 3)
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "event_search", "event_types": []string{"sched_switch"}})
	result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: root, WorkDir: root}, params)
	if err != nil || !result.Success {
		t.Fatalf("real unrelated event query failed: %v / %s", err, result.Summary)
	}
	queryOnly := types.CompileObservationLedger(types.ObservationLedgerInput{ToolResults: []types.ToolResult{result}})
	if !queryOnly.HasDeterministicRuntimeQueryObservation() {
		t.Fatal("real query must independently satisfy query presence")
	}
	for _, record := range queryOnly.Records {
		if record.Predicate == "target_window_states" {
			t.Fatal("unrelated query unexpectedly produced a state account")
		}
	}
	for _, provenance := range []string{"aggregate_facts", "trace_query", "trace_query:run2"} {
		for _, lang := range []string{"zh", "en"} {
			t.Run(provenance+"/"+lang, func(t *testing.T) {
				fact := types.AnswerAggregateFact{
					Kind: types.AnswerAggregateScalar, Label: "worker-41 state claim", Value: "1000.000", Unit: "ms",
					Role: types.AnswerAggregateRoleSupportingCoverage, Provenance: provenance,
					Dimensions: []types.AnswerAggregateDimension{
						{Name: "origin", Value: string(types.AnswerEvidenceOriginRuntimeArtifact)},
						{Name: "target", Value: "worker-41"}, {Name: "predicate", Value: "target_window_states"},
					},
					Members: []string{"running=777.777", "runnable=0.000", "sleep=222.223", "d_state=0.000", "io_wait=0.000", "total=1000.000"},
				}
				mut := types.NewMutableState("Compare worker-41 state accounts.")
				mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{fact})
				mut.RetainInvestigationAggregateFacts()
				bus := &types.BusContext{Mutable: mut, Language: lang, ToolResults: []types.ToolResult{result}}
				ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
				found := false
				for _, record := range ledger.Records {
					if record.Predicate != "target_window_states" {
						continue
					}
					found = true
					if record.ClaimAuthority != types.ObservationClaimAuthorityModelInference || record.GroundingPolicy == types.ClaimGroundingHard {
						t.Fatalf("fixture must remain a non-hard model claim: %+v", record)
					}
				}
				if !found || len(types.BuildTraceTargetStateScopeAuthoritiesFromLedger(ledger)) != 0 {
					t.Fatal("fixture must retain the model claim without electing state authority")
				}
				body := b1618AuthorityPublishedAppendix(t, bus)
				if strings.Contains(body, "777.777") || strings.Contains(body, "222.223") || strings.Contains(body, "1000.000") {
					t.Fatalf("model state values became system-authored measurement facts through producer %q:\n%s", provenance, body)
				}
			})
		}
	}
}

// Legacy producer-owned hard observations may lack selected-window or result
// coordinates. Qualification must not turn missing provenance into data loss.
func TestB1618HardLegacyStateAccountWithoutWindowRemainsPublishable(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			record := b1618IdentityRecord()
			record.RichNotes = append([]string(nil), record.RichNotes[:len(record.RichNotes)-1]...)
			record.SourceRef = types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact}
			record.Span = types.ObservationSpan{}
			record.SupportRefs = nil
			mut := types.NewMutableState("Compare worker-41 state accounts.")
			bus := &types.BusContext{Mutable: mut, Language: lang, ToolResults: []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: []types.ObservationRecord{record}}}}
			body := b1618AuthorityPublishedAppendix(t, bus)
			if !strings.Contains(body, "running 2.000") || !strings.Contains(body, "sleep 8.000") || (!strings.Contains(body, "来源未明确") && !strings.Contains(body, "source not stated")) {
				t.Fatalf("hard legacy state values or honest unknown provenance lost:\n%s", body)
			}
		})
	}
}

func b1618AuthorityPublishedAppendix(t *testing.T, bus *types.BusContext) string {
	t.Helper()
	doc := psgProseDoc("worker-41: running 2.000ms and sleep 8.000ms remain model-owned observations.")
	bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	before, _ := json.Marshal(doc)
	model := render.RenderAnswerDocument(doc, bus.Language)
	out := &agent.StageOutput{FinalAnswer: model}
	(&Orchestrator{busCtx: bus}).attachSystemCrossCheckAppendix(out, "", nil)
	after, _ := json.Marshal(bus.Mutable.AnswerDocumentV2())
	if string(before) != string(after) || !strings.Contains(out.FinalAnswer, model) {
		t.Fatal("publication changed the model-owned answer")
	}
	var body strings.Builder
	for _, attachment := range bus.Mutable.AnswerDisplayAttachments() {
		if attachment.Source == types.AnswerDisplayAttachmentSourceSystemCrossCheck {
			body.WriteString(attachment.Body)
		}
	}
	return body.String()
}
