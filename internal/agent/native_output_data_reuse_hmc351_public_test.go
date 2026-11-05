package agent

import (
	"bytes"
	"encoding/json"
	"html"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Two real query paths retain different index-coverage notes for the same
// complete partition. The final output may reuse rows, never either query's
// independent scope, notes, receipt or evidence rights.
func TestHMC351PublicNativeOutputReusesDataKeepsBothQueryNotes(t *testing.T) {
	ctx, results := hmc346QueryContext(t, hmc346LargeTrace(t), 100, 2, 2.02)
	tables := hmc346TimelineTables(t, ctx)
	before, _ := json.Marshal(answerDocObservationLedger(ctx))
	causal := hmc218CausalProjection(t, ctx)
	for _, name := range []string{"automatic", "explicit_owner", "both_model_owned"} {
		t.Run(name, func(t *testing.T) {
			blocks := []any{map[string]any{"id": "lead", "kind": "summary", "text": "以下保留各次查询的原生量测和范围说明。"}}
			if name != "automatic" {
				blocks = append(blocks, map[string]any{"id": "chosen", "kind": "table", "runtime_measurement": map[string]any{"observation_id": tables[0].ObservationID, "view": tables[0].View}})
			}
			wantTables := 1
			if name == "both_model_owned" {
				blocks = append(blocks, map[string]any{"id": "also-chosen", "kind": "table", "runtime_measurement": map[string]any{"observation_id": tables[1].ObservationID, "view": tables[1].View}})
				wantTables = 2 // The system must not suppress a model-owned block.
			}
			raw, _ := json.Marshal(map[string]any{"blocks": blocks})
			result, err := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), raw)
			if err != nil || !result.Success {
				t.Fatalf("public answer: %v %s", err, result.Summary)
			}
			doc := ctx.Mutable.AnswerDocumentV2()
			original, _ := json.Marshal(doc)
			text := render.RenderAnswerDocument(doc, "zh")
			if got := strings.Count(text, "| 线程 | 状态 | 开始"); got != wantTables {
				t.Errorf("native rows rendered %d times, want %d while retaining ownership and both query notes", got, wantTables)
			}
			seen := map[string]bool{}
			for _, b := range doc.Blocks {
				if r := b.RuntimeMeasurement; r != nil && r.IsBound() && strings.HasSuffix(r.ObservationID, "#thread_timeline_observation") {
					seen[r.ObservationID] = true
				}
			}
			if len(seen) != 2 {
				t.Fatal("render reuse removed an independent source/query binding")
			}
			for _, table := range tables {
				for _, note := range table.Notes {
					if !strings.Contains(html.UnescapeString(text), note) {
						t.Fatalf("query-specific complete note disappeared: %q", note)
					}
				}
			}
			after, _ := json.Marshal(doc)
			if !bytes.Equal(original, after) {
				t.Fatal("renderer mutated accepted document")
			}
		})
	}
	after, _ := json.Marshal(answerDocObservationLedger(ctx))
	if !bytes.Equal(before, after) || !bytes.Equal(causal, hmc218CausalProjection(t, ctx)) || len(results) != 2 {
		t.Fatal("display reuse changed original facts, queries or causal projection")
	}
}
