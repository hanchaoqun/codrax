package agent

import (
	"errors"
	"os"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Capture the first real NewFinalizerAgent adapter request: this tests the
// default skill's complete system message together with the dynamic user
// handoff. The adapter stops locally; no model response or answer is fabricated.
func finalizerReaderFieldMessages(t *testing.T, ctx *types.AgentContext) (string, string) {
	t.Helper()
	before := readerFieldScopePublicSnapshot(t, ctx)
	attached := ctx.AttachedHitrace
	reg := toolpkg.NewRegistry()
	reg.Register(&toolpkg.EmitAnswerDocument{})
	reg.Register(&toolpkg.EmitAnswerDocumentPatch{})
	capture := &traceTeachingCaptureLLM{stop: errors.New("captured finalizer field-visibility request")}
	finalizer := NewFinalizerAgent(&Dependencies{LLM: capture, Tools: reg, MaxIterations: 1})
	_, err := finalizer.Execute(ctx, traceTeachingSkill(t, "answer-document-skill"))
	if !errors.Is(err, capture.stop) || capture.calls != 1 {
		t.Fatalf("HARNESS: expected one actual finalizer request, calls=%d err=%v", capture.calls, err)
	}
	if readerFieldScopePublicSnapshot(t, ctx) != before || ctx.AttachedHitrace != attached {
		t.Fatal("prompt construction changed original fields, precision, scope, causal authority, or model-owned answer")
	}
	var system, user strings.Builder
	for _, message := range capture.messages {
		switch message.Role {
		case "system":
			system.WriteString(message.Content)
			system.WriteByte('\n')
		case "user":
			user.WriteString(message.Content)
			user.WriteByte('\n')
		}
	}
	if system.Len() == 0 || user.Len() == 0 {
		t.Fatal("HARNESS: actual finalizer request omitted system or user messages")
	}
	return system.String(), user.String()
}

func assertFinalizerReaderFieldMessagePolicy(t *testing.T, system, user string) {
	t.Helper()
	for _, header := range []string{"READER WORDS OVER FIELD SPELLINGS:", "TARGET WAIT OCCURRENCE AUTHORITY:"} {
		if strings.Count(system, header) != 1 {
			t.Fatalf("HARNESS: default trace system rule %q did not reach the actual adapter exactly once", header)
		}
	}
	for _, old := range []string{
		"A field name may appear only as a quoted key beside its cited evidence row, never as the sentence's own vocabulary",
		"Use reader language rather than field names, predicates, enum literals, or status codes",
		"raw JSON field names, enum literals, authority/status keys, and their snake_case values belong only in structured fields and audit carriers",
	} {
		for name, text := range map[string]string{"system": system, "user": user} {
			if strings.Contains(text, old) {
				t.Errorf("FIELD_VISIBILITY_OVERREACH on %s: %q", name, old)
			}
		}
	}
	for _, want := range []string{
		"internal protocol, validation, routing, or ranking-control",
		"relevant, evidence-supported raw-data field names, units, identifiers, and business statuses",
		"source, scope, precision, or evidence strength",
		"no additional causal or current-source authority",
	} {
		if !strings.Contains(system, want) {
			t.Errorf("FIELD_VISIBILITY_SCOPE: actual system message omitted %q", want)
		}
	}
	// The raw measurement and chain rules must remain present, not disappear
	// while fixing terminology. These assertions do not inspect answer prose.
	for _, want := range []string{
		"preserve its count and every published start/end/duration/state/IO-marker/caller relation exactly",
		"do not merge or discard an item",
		"never proves no sleep, waiting, blocking, or IO activity",
		"The fact fence is unchanged",
		"The projected `emit_answer_document` tool schema is the only authority",
	} {
		if !strings.Contains(system, want) {
			t.Errorf("existing measurement, JSON, or completeness teaching changed: %q", want)
		}
	}
	readerFieldScopePublicRejectOverbroadTeaching(t, user)
}

func TestFinalizerReaderFieldActualMessagesNativeFinite(t *testing.T) {
	result := readerFieldScopePublicResult(t)
	var attached string
	for _, record := range result.Observations {
		if types.IsValidTraceEventSearchInventoryRecord(record) {
			data, err := os.ReadFile(record.SourceRef.Path)
			if err != nil {
				t.Fatal(err)
			}
			attached = string(data)
		}
	}
	if attached == "" {
		t.Fatal("HARNESS: native query did not retain its existing source attachment")
	}
	for _, lang := range []string{"zh", "en"} {
		for _, scope := range []types.RuntimeQuestionScope{types.RuntimeQuestionScopeBoundedFactSet, types.RuntimeQuestionScopeBoundedEffectVerdict} {
			t.Run(lang+"/"+string(scope), func(t *testing.T) {
				ctx := readerFieldScopePublicContext(lang, scope, []types.ToolResult{result})
				// The original fixture tested only the dynamic user handoff. A
				// real attached-source carrier also activates default system
				// Trace guidance, as it does in the reported production run.
				ctx.AttachedHitrace = attached
				system, user := finalizerReaderFieldMessages(t, ctx)
				views := traceEventInventoryPromptViews(t, user)
				if len(views) != 1 || !views[0].Inventory.RowsComplete || views[0].Inventory.Coverage.MatchedTotal != 1 || len(views[0].Inventory.Rows) != 1 {
					t.Fatalf("HARNESS: complete native inventory did not reach actual finalizer: %+v", views)
				}
				row := views[0].Inventory.Rows[0]
				if row.Line != 1 || row.EmitterTID != 101 || row.EmitterTGID != 101 || row.MarkerPID != 201 || row.TraceTimeSeconds != 6 ||
					!strings.Contains(row.Raw, "result=E_BUSY, sample_count=9007199254740993") {
					t.Fatalf("native raw business fields or exact integer identity changed: %+v", row)
				}
				assertFinalizerReaderFieldMessagePolicy(t, system, user)
				if strings.Contains(user, "## Final Trace Decision Boundary (Typed Facts; Model-Owned Conclusion)") {
					t.Fatal("finite native enumeration gained a complete causal conclusion boundary")
				}
			})
		}
	}
}

// This existing typed causal producer fixture covers the sibling causal
// handoff. It is not presented as a second native causal trace measurement.
func TestFinalizerReaderFieldActualMessagesTypedCausal(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			ctx := finalValueComponentContext(t, lang, []types.ObservationRecord{
				finalValueComponentRecord("reader-field-system", "io_wait", "io_wait", "io_dependency", 2, 11, 11),
			})
			system, user := finalizerReaderFieldMessages(t, ctx)
			if !strings.Contains(user, "principal_root_cause_population=`typed_on_chain_only`") ||
				!strings.Contains(user, "measured_state_occupancy=11.000ms") ||
				!strings.Contains(user, "reader_facing_control_metadata_policy=`json_only_never_visible`") {
				t.Fatal("HARNESS: causal evidence, value, or reader-control handoff missing at actual adapter")
			}
			assertFinalizerReaderFieldMessagePolicy(t, system, user)
		})
	}
}

func TestFinalizerReaderFieldActualMessagesNoTraceActivation(t *testing.T) {
	const request = "Explain how the configuration parser preserves a setting name."
	rm := types.RequestModel{RawRequest: request, Language: "en", Intent: types.IntentExplain,
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeNotApplicable}}
	mu := types.NewMutableState(request)
	mu.SetRequestModel(rm)
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary, Text: "Existing model-owned source explanation."}}})
	ctx := ctxbuilder.BuildAgentContext(&types.BusContext{Language: "en", Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "en"}}},
		types.AgentFinalizer, types.StageFinalize)
	system, user := finalizerReaderFieldMessages(t, ctx)
	for _, marker := range []string{
		"READER WORDS OVER FIELD SPELLINGS:", "TARGET WAIT OCCURRENCE AUTHORITY:",
		"## Reader-ready finite-window facts", "## Reader-ready Trace facts",
		"reader_facing_control_metadata_policy=", "## Final Trace Decision Boundary",
	} {
		if strings.Contains(system, marker) || strings.Contains(user, marker) {
			t.Errorf("Trace-specific reader policy activated without a Trace carrier: %q", marker)
		}
	}
}
