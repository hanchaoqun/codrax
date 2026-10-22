package tool

import (
	"database/sql"
	"fmt"
	"html"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tracewire"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC253ResourceSymbolAmbiguityIsIndependentOfDepth(t *testing.T) {
	bus, result, view := resourceMeasurementContextHMC249(t, false, 101, func(db *sql.DB) {
		// Keep the duplicate dictionary identity, but make this event's depths
		// contiguous. Symbol resolution and depth shape are independent facts.
		if _, err := db.Exec("UPDATE native_hook_frame SET depth=rowid-6 WHERE callchain_id=9"); err != nil {
			t.Fatal(err)
		}
	})
	var members types.RuntimeMeasurementTable
	for _, table := range resourceNativeTablesHMC249(view) {
		if table.View == types.RuntimeMeasurementSummary {
			for _, row := range table.Rows {
				if row[0] == "3" && (row[5] != "已记录深度连续" || !strings.Contains(row[4], "重复深度=0") || !strings.Contains(row[4], "未知符号=1")) {
					t.Fatalf("depth and symbol census changed: %v", row)
				}
			}
		} else {
			members = table
		}
	}
	found := false
	for _, row := range members.Rows {
		if row[0] == "3" && row[4] == "2" {
			found = true
			if row[5] != "未知（有歧义）" {
				t.Errorf("symbol ambiguity attributed to depth: %q", row[5])
			}
		}
	}
	if !found {
		t.Fatal("exact source frame absent")
	}
	args := map[string]any{"blocks": []any{
		map[string]any{"id": "lead", "kind": "summary", "text": "以下仅列原件记录。"},
		map[string]any{"id": "facts", "kind": "table", "runtime_measurement": map[string]any{"observation_id": members.ObservationID, "view": members.View}},
	}}
	if out := b1659bExecuteAnswer(t, bus, args, false); !out.Success {
		t.Fatalf("public emit failed: %s", out.Summary)
	}
	visible := html.UnescapeString(render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), "zh"))
	for label, text := range map[string]string{"tool text": result.Summary, "bound answer": visible} {
		if !strings.Contains(text, "未知（有歧义）") {
			t.Errorf("%s does not retain text ambiguity: %s", label, text)
		}
		if strings.Contains(text, "未知（存在重复") || strings.Contains(text, "-->") {
			t.Errorf("%s invented depth defect or ordered edge", label)
		}
	}
}

func TestHMC253ResourceStatusDomainsAndEscaping(t *testing.T) {
	for _, tc := range []struct{ status, display string }{
		{"null", "空值"}, {"unavailable", "未提供"}, {"invalid", "无效"},
	} {
		scalar := tracewire.ResourceScalar{Status: tc.status}
		text := tracewire.ResourceText{Status: tc.status}
		want := "未知（" + tc.display + "）"
		if !scalar.Valid() || !text.Valid() || resourceFactScalar(scalar) != want || resourceScalarText(scalar) != want || resourceFactText(text) != want || resourceTextText(text) != want {
			t.Fatalf("typed value status %q lost or inferred", tc.status)
		}
	}
	ambiguous := tracewire.ResourceText{Status: "ambiguous"}
	if !ambiguous.Valid() || resourceFactText(ambiguous) != "未知（有歧义）" || resourceDepthState("ambiguous") != "存在重复深度" {
		t.Fatal("text ambiguity and duplicate depth conflated")
	}
	if resourceDepthState("incomplete") != "存在缺失或无效深度" || resourceDepthState("contiguous_observed") != "已记录深度连续" || resourceDepthState("unavailable") != "未提供" {
		t.Fatal("depth shape lost")
	}
	value := "<symbol|branch>\nframe"
	text := tracewire.ResourceText{Status: "known", Value: value}
	if resourceFactText(text) != value || resourceTextText(text) != "&lt;symbol&#124;branch&gt;<br/>frame" {
		t.Fatal("source text changed or double escaped")
	}
	if resourceFactState("future_state") != "future_state" || resourceDepthState("future_state") != "future_state" {
		t.Fatal("unknown schema state given invented semantics")
	}
}

func TestHMC253ResourceQuantityDisplaySharedWithoutUnitInference(t *testing.T) {
	for _, tc := range []struct {
		name, operation, sqlValue, display, unit string
	}{
		{"allocation", "AllocEvent", "512", "512", "单位=bytes"},
		{"descriptor", "FD_Open_Event", "512", "512", "单位未在该载体声明"},
		{"thread_zero", "THREAD_Create_Event", "0", "0", "单位未在该载体声明"},
		{"unknown", "AllocEvent", "NULL", "未知（空值）", "单位=bytes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, result, view := resourceMeasurementContextHMC249(t, false, 101, func(db *sql.DB) {
				if _, err := db.Exec("UPDATE native_hook SET event_type=?, heap_size="+tc.sqlValue+" WHERE rowid=1", tc.operation); err != nil {
					t.Fatal(err)
				}
			})
			quantity := "源数量/大小=" + tc.display + "（" + tc.unit + "）"
			if !strings.Contains(result.Summary, quantity) {
				t.Errorf("tool text quantity differs from native fact: want %q", quantity)
			}
			found := false
			for _, table := range resourceNativeTablesHMC249(view) {
				if table.MemberSet != nil || table.HasCompleteNativeDisplayPopulation() || table.DefaultPresentation != (table.View == types.RuntimeMeasurementSummary) {
					t.Fatal("display-only correction changed publication authority or defaults")
				}
				if table.View != types.RuntimeMeasurementSummary {
					continue
				}
				for _, row := range table.Rows {
					if row[0] == "1" {
						found = true
						if !strings.Contains(row[3], quantity) || !strings.Contains(row[3], tc.operation) {
							t.Errorf("native operation lost quantity/unit contract: %v", row)
						}
					}
				}
			}
			if !found {
				t.Fatal("source event absent")
			}
			for _, want := range []string{"原始id=1", "栈键=7", "地址signed=-1", "地址位型=0xffffffffffffffff", "UIFrame::render", "buffer_cleanup", "物理rowid/原始id"} {
				if !strings.Contains(result.Summary, want) {
					t.Errorf("old rich display lost %q", want)
				}
			}
			if strings.Contains(result.Summary, "资源大小=") || strings.Contains(result.Summary, "512 字节") {
				t.Fatal("legacy quantity label or inferred unit retained")
			}
			for _, r := range result.Observations {
				p, ok := DecodeTraceResourceStack(r)
				if !ok {
					continue
				}
				for _, e := range p.Events {
					for _, f := range e.Frames {
						if !strings.Contains(result.Summary, fmt.Sprintf("|%d / %s|%d|", f.RowID, f.SourceID.Value, f.Line)) {
							t.Errorf("source frame identity omitted: event=%d frame=%d", e.RowID, f.RowID)
						}
					}
				}
			}
		})
	}
}
