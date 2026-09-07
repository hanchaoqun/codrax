package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1605BlockingScopeUsesReaderLabelsWithoutChangingFacts(t *testing.T) {
	for _, tc := range []struct{ token, zh, en string }{
		{"binder_wait", "binder等待", "binder wait"},
		{"io_latency", "IO延迟", "IO latency"},
		{"block_io_completion_closed_issuer_wait", "IO完成唤醒提交线程的等待", "issuer wait ended by IO completion"},
		{"future_wait_kind", "future_wait_kind", "future_wait_kind"},
	} {
		for _, lang := range []string{"zh", "en"} {
			t.Run(tc.token+"/"+lang, func(t *testing.T) {
				bus := runtimeWaitCoverageTestBus()
				bus.AnalysisIR.RequestModel.Language = lang
				bus.AnalysisIR.AnswerContract.Language = lang
				record := &bus.ToolResults[0].Observations[0]
				record.Object = tc.token
				record.RichNotes[1] = types.TraceNoteKeyType + "=" + tc.token
				if tc.token == "block_io_completion_closed_issuer_wait" {
					record.Predicate, record.ClaimKey = "io_latency", "io_latency"
					record.RichNotes = append(record.RichNotes,
						types.TraceNoteKeyIOCompletionWokeIssuer+"=true",
						types.TraceNoteKeyIOCausalWaitCaliber+"=completion_closed_issuer_blocked",
						types.TraceNoteKeyIOIssuerBlocked+"=1.409",
						types.TraceNoteKeyIOIssuerBlockedStart+"=13762.835861",
						types.TraceNoteKeyIOIssuerBlockedEnd+"=13762.837270",
						types.TraceNoteKeyIOIssuerBlockedState+"=S",
					)
				}
				before, _ := json.Marshal(bus.ToolResults)
				modelText := "model-authored binder_wait and its conclusion stay untouched"
				doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, Text: modelText}}}
				if !materializeRuntimeTraceBlockingCoverageAuthorityCaveat(doc, bus) {
					t.Fatal("blocking lower bound must reach the actual reader surface")
				}
				var got string
				for _, block := range doc.Blocks {
					if block.ID == runtimeTraceBlockingCoverageAuthorityBlockID {
						got = block.Text
					}
				}
				want := "类型=" + tc.zh + "；当前至少观测到 1 段、合计 1.409ms"
				if lang == "en" {
					want = "type=" + tc.en + "; at least 1 interval totaling 1.409ms"
				}
				if !strings.Contains(got, want) {
					t.Fatalf("missing reader label with unchanged lower bound %q: %s", want, got)
				}
				if tc.token != "future_wait_kind" && strings.Contains(got, tc.token) {
					t.Fatalf("system-owned scope still exposes the internal type instead of its label: %s", got)
				}
				after, _ := json.Marshal(bus.ToolResults)
				if string(before) != string(after) || doc.Blocks[0].Text != modelText {
					t.Fatal("display changed the typed facts or model-authored text")
				}
				if materializeRuntimeTraceBlockingCoverageAuthorityCaveat(doc, bus) {
					t.Fatal("reader label must retain idempotent materialization")
				}
			})
		}
	}
}
