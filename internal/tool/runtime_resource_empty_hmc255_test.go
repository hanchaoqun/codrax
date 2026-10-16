package tool

import (
	"encoding/json"
	"html"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// A successful empty query is a query disposition, not a synthetic resource
// event. Exercise the public query and answer binding/rendering paths so the
// model-facing row count and every supported presentation keep that meaning.
func TestHMC255ResourceEmptyQueryHasNoSyntheticMember(t *testing.T) {
	bus, _, view := resourceMeasurementContextHMC249(t, true, 202)
	view.OptionalBlocks = append(view.OptionalBlocks, types.BlockRequirement{Kind: types.BlockDiagram})
	view.DiagramPlan = &types.DiagramFacetGraph{Kind: types.DiagramFlow}
	tables := resourceNativeTablesHMC249(view)
	if len(tables) != 2 {
		t.Fatalf("empty query lost its native summary/frame selectors: %d", len(tables))
	}
	for _, table := range tables {
		if len(table.Rows) != 0 {
			t.Errorf("%s invented %d members for a zero-event query: %+v", table.View, len(table.Rows), table.Rows)
		}
		for _, kind := range []string{"summary", "table", "diagram"} {
			t.Run(string(table.View)+"/"+kind, func(t *testing.T) {
				block := map[string]any{"id": "facts", "kind": kind, "runtime_measurement": map[string]any{"observation_id": table.ObservationID, "view": table.View}}
				blocks := []any{block}
				if kind != "summary" {
					blocks = append([]any{map[string]any{"id": "lead", "kind": "summary", "text": "仅列本次查询记录。"}}, blocks...)
				}
				if out := b1659bExecuteAnswer(t, bus, map[string]any{"blocks": blocks}, false); !out.Success {
					t.Fatalf("empty query could not be rendered: %s", out.Summary)
				}
				doc := bus.Mutable.AnswerDocumentV2()
				visible := html.UnescapeString(render.RenderAnswerDocument(doc, "zh"))
				for _, want := range []string{"原查询匹配资源事件=0", "编号=202", "10.000000000", "10.050000000", "无匹配记录不证明目标存在"} {
					if !strings.Contains(visible, want) {
						t.Errorf("empty query lost %q: %s", want, visible)
					}
				}
				if strings.Contains(visible, "| 无匹配记录 |") || strings.Contains(visible, "线程ID=0") || strings.Contains(visible, "-->") {
					t.Errorf("empty query rendered a fake event, identity or edge: %s", visible)
				}
				raw, _ := json.Marshal(doc)
				var restored types.AnswerDocumentV2
				if json.Unmarshal(raw, &restored) != nil || !types.RebindRuntimeAnswerReceipts(&restored, view) || render.RenderAnswerDocument(&restored, "zh") != render.RenderAnswerDocument(doc, "zh") {
					t.Fatal("current-source rebind changed empty query presentation")
				}
			})
		}
	}
}

func TestHMC255ResourceNativeRowsCountOnlyObservedEventsAndFrames(t *testing.T) {
	events, frames := 0, 0
	for _, spec := range []struct {
		nested bool
		tid    int
		events int
		frames int
	}{{false, 101, 3, 8}, {false, 202, 1, 3}, {true, 101, 1, 3}, {true, 202, 0, 0}} {
		_, _, view := resourceMeasurementContextHMC249(t, spec.nested, spec.tid)
		for _, table := range resourceNativeTablesHMC249(view) {
			want := spec.frames
			if table.View == types.RuntimeMeasurementSummary {
				want = spec.events
				events += len(table.Rows)
			} else {
				frames += len(table.Rows)
			}
			if len(table.Rows) != want {
				t.Errorf("nested=%t tid=%d view=%s rows=%d want=%d", spec.nested, spec.tid, table.View, len(table.Rows), want)
			}
		}
	}
	if events != 5 || frames != 14 {
		t.Fatalf("native row inventory has synthetic/missing members: events=%d frames=%d", events, frames)
	}
}
