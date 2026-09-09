package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func b1630PolicyWitnesses() []types.TraceFrequencyLimitAuthority {
	return []types.TraceFrequencyLimitAuthority{
		{CPU: 0, MinFrequencyKHz: 418000, MaxFrequencyKHz: 1530000, LimitRowCount: 16,
			WitnessLine: 8048, WitnessTs: 13762.861720, WindowStartTs: 13762.791708, WindowEndTs: 13763.024898,
			Authority: "direct_in_window_policy_limit"},
		{CPU: 4, MinFrequencyKHz: 558000, MaxFrequencyKHz: 2100000, LimitRowCount: 28,
			WitnessLine: 17113, WitnessTs: 13762.940114, WindowStartTs: 13762.791708, WindowEndTs: 13763.024898,
			Authority: "direct_in_window_policy_limit"},
	}
}

func TestB1630PolicySystemCaveatPublishedTotalAndSelectedSampleAreSeparate(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse=%t", lang, reverse), func(t *testing.T) {
				witnesses := b1630PolicyWitnesses()
				if reverse {
					witnesses[0], witnesses[1] = witnesses[1], witnesses[0]
				}
				// Repeated publications are display duplicates, not additive counts.
				witnesses = append(witnesses, witnesses[0])
				ctx := newBusForMutationTest()
				ctx.AnalysisIR = &types.AnalysisIR{
					RequestModel:   types.RequestModel{Intent: types.IntentRootCause, Language: lang},
					AnswerContract: types.AnswerContract{Language: lang},
				}
				ctx.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true,
					TraceEvidenceAuthority: &types.TraceEvidenceAuthority{FrequencyLimitWitnesses: witnesses}}}
				before, _ := json.Marshal(ctx.ToolResults)
				modelText := "Model-owned conclusion: policy tuple and its count still need interpretation. 模型原文不改。"
				doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, Text: modelText}}}
				result, err := ApplyAndPersistMutation(ctx, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Unix(1, 0))
				if err != nil || !result.Success {
					t.Fatalf("actual publish failed: %v %+v", err, result)
				}
				persisted := ctx.Mutable.AnswerDocumentV2()
				if persisted == nil || len(persisted.Blocks) != 2 || persisted.Blocks[0].Text != modelText || persisted.Blocks[1].ID != runtimeTraceFrequencyAuthorityBlockID || !RuntimeTraceSystemBlock(persisted.Blocks[1]) {
					t.Fatalf("publication changed model prose or system ownership: %+v", persisted)
				}
				text := types.AnswerBlockVisibleSurface(persisted.Blocks[1])
				wants := []string{"lowest", "not", "16", "28", "418000", "1530000", "558000", "2100000", "8048", "17113", "13762.861720", "13762.940114", "13762.791708", "13763.024898"}
				if lang == "zh" {
					wants[0], wants[1] = "上限最低", "总条数不是"
				}
				for _, want := range wants {
					if !strings.Contains(text, want) {
						t.Errorf("published system scope missing %q: %s", want, text)
					}
				}
				for _, witness := range b1630PolicyWitnesses() {
					observation := fmt.Sprintf("CPU%d %s", witness.CPU, types.FormatTraceFrequencyLimitRecordObservation(witness, lang))
					if !strings.Contains(text, observation) {
						t.Errorf("CPU identity lost its own count and selected bounds: missing %q in %s", observation, text)
					}
				}
				for _, wrong := range []string{"418000–1530000kHz（16条", "558000–2100000kHz（28条", "418000–1530000kHz (16 rows", "558000–2100000kHz (28 rows"} {
					if strings.Contains(text, wrong) {
						t.Errorf("all-window count was attached to one policy tuple: %s", text)
					}
				}
				compact := strings.ReplaceAll(text, " ", "")
				if strings.Count(compact, "CPU0") != 1 || strings.Count(compact, "CPU4") != 1 {
					t.Errorf("duplicate publications multiplied CPU policy rows: %s", text)
				}
				after, _ := json.Marshal(ctx.ToolResults)
				if string(before) != string(after) {
					t.Fatal("display changed original witness values, coordinates, or counts")
				}
				frozen, _ := json.Marshal(persisted)
				changed := materializeRuntimeTraceFrequencyAuthorityCaveat(persisted, ctx)
				again, _ := json.Marshal(persisted)
				if changed || string(frozen) != string(again) {
					t.Fatal("repeated system materialization changed the published document")
				}
			})
		}
	}
}

func TestB1630PolicySystemCaveatDoesNotInventMissingWitness(t *testing.T) {
	ctx := newBusForMutationTest()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentRootCause}}
	ctx.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true, TraceEvidenceAuthority: &types.TraceEvidenceAuthority{}}}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, Text: "original"}}}
	before, _ := json.Marshal(doc)
	if materializeRuntimeTraceFrequencyAuthorityCaveat(doc, ctx) {
		t.Fatal("empty frequency authority acquired a policy witness")
	}
	after, _ := json.Marshal(doc)
	if string(before) != string(after) {
		t.Fatal("no-witness materialization changed model content")
	}
}
