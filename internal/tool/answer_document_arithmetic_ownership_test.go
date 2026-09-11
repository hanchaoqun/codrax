package tool

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// A duration token and a percentage in prose do not identify the numerator,
// denominator, source or subject of a ratio. Even a numerically wrong sentence
// is model-owned. Evidence-only reconciliation has its own cited public lane.
func TestB193PublicPersistDoesNotJudgeProseArithmetic(t *testing.T) {
	for _, tc := range []struct {
		name, lang, text, status string
		windows                  []string
	}{
		{"cross_sentence_zh", "zh", "五段等待合计 3.094ms。在 233.190ms 窗口中约占 1.3%。", "complete", []string{"selected_window=13762.791708..13763.024898"}},
		{"cross_sentence_en", "en", "The waits total 3.094ms. Over the 233.190ms window they account for about 1.3%.", "complete", []string{"selected_window=13762.791708..13763.024898"}},
		{"different_values", "en", "The work takes 7.500ms. Within 300.000ms it occupies 2.5%.", "complete", []string{"selected_window=1..1.3"}},
		{"wrong_ratio_zh", "zh", "八段合计 0.817ms，占窗口 0.44%。", "complete", []string{"selected_window=69326.832743749..69327.060110624"}},
		{"wrong_ratio_en", "en", "Eight slices total 0.817ms, 0.44% of the selected window.", "complete", []string{"selected_window=69326.832743749..69327.060110624"}},
		{"incomplete", "zh", "累计 50.000ms，占比 50%。", "incomplete", []string{"selected_window=1..1.1"}},
		{"ambiguous", "en", "1.000ms and 2.000ms, 50%.", "complete", []string{"selected_window=1..1.002", "selected_window=2..2.004"}},
		{"postpositive", "zh", "窗口 114.940ms，占 73.4%（84.358ms）。", "incomplete", []string{"selected_window=34579.472865..34579.587805"}},
		{"no_window", "en", "About 1.000ms, 50%.", "unknown", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := types.ToolResult{ToolName: "trace_query", Success: true,
				EnumerationAuthority: &types.ToolEnumerationAuthority{Status: tc.status}}
			for _, window := range tc.windows {
				result.Observations = append(result.Observations, types.ObservationRecord{
					ID:     "trace_query:arithmetic#root_cause_rank:" + window,
					Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
					GroundingPolicy: types.ClaimGroundingHard, Predicate: "root_cause_primary",
					Subject: "worker", Object: "runnable", Value: "1.000", Unit: "ms",
					RichNotes: []string{"tier=primary", window}, Confidence: 0.8,
				})
			}
			bus := &types.BusContext{Language: tc.lang, ToolResults: []types.ToolResult{result}, Mutable: types.NewMutableState("arithmetic ownership")}
			// Freeze bytes independently: result and bus intentionally share the
			// original observation slices, so comparing them cannot detect mutation.
			evidenceBefore, err := json.Marshal(bus.ToolResults)
			if err != nil {
				t.Fatal(err)
			}
			doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, Text: tc.text}}, Caveats: []string{"Model caveat: arithmetic remains my interpretation."}}
			before, _ := json.Marshal(doc.Blocks)
			for round := 0; round < 2; round++ {
				mutation := types.NewReplaceAllMutation(doc)
				var prev *types.AnswerDocumentV2
				if round == 1 {
					prev = bus.Mutable.AnswerDocumentV2()
					var ids []string
					for _, block := range prev.Blocks {
						ids = append(ids, block.ID)
					}
					mutation = types.NewPartialMutation(&types.AnswerDocumentV2Patch{UnchangedBlockIDs: ids})
				}
				persisted, err := ApplyAndPersistMutation(bus, "b193_public", mutation, prev, time.Now())
				if err != nil || !persisted.Success {
					t.Fatalf("public mutation failed: %v %+v", err, persisted)
				}
				got := bus.Mutable.AnswerDocumentV2()
				var model []types.AnswerBlock
				for _, block := range got.Blocks {
					if block.ID == "summary" {
						model = append(model, block)
					}
				}
				after, _ := json.Marshal(model)
				if string(before) != string(after) {
					t.Fatalf("model blocks changed: %s -> %s", before, after)
				}
				if !containsB193String(got.Caveats, doc.Caveats[0]) {
					t.Fatal("model caveat was removed")
				}
				for _, caveat := range got.Caveats {
					if strings.HasPrefix(caveat, "数值关系复算：") || strings.HasPrefix(caveat, "Arithmetic relation check:") {
						t.Errorf("prose-derived system verdict shipped: %s", caveat)
					}
				}
				evidenceAfter, err := json.Marshal(bus.ToolResults)
				if err != nil {
					t.Fatal(err)
				}
				if string(evidenceBefore) != string(evidenceAfter) {
					t.Fatal("producer evidence was changed")
				}
			}
		})
	}
}

func containsB193String(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
