package tool

import (
	"encoding/json"
	"html"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Keep every pre-existing private credential in legacy invariance assertions.
// Only the newly derived presentation is compared separately by its public
// title/source contract; unlike JSON equality, this still checks private scope.
func differingMeasurementFieldExceptPresentationHMC269(a, b types.RuntimeMeasurementTable) string {
	left, right := reflect.ValueOf(a), reflect.ValueOf(b)
	for i := 0; i < left.NumField(); i++ {
		name := left.Type().Field(i).Name
		if name == "nativeSourcePresentation" {
			continue
		}
		l, r := left.Field(i), right.Field(i)
		if l.CanInterface() {
			if !reflect.DeepEqual(l.Interface(), r.Interface()) {
				return name
			}
			continue
		}
		if l.Kind() == reflect.Pointer {
			if l.IsNil() || r.IsNil() {
				if l.IsNil() != r.IsNil() {
					return name
				}
				continue
			}
			l, r = l.Elem(), r.Elem()
		}
		if !l.Comparable() || !l.Equal(r) {
			return name
		}
	}
	return ""
}

// Real SQLite original -> Prepare -> TraceQuery -> current contract -> public
// emit/render. The wrong title is deliberately unrelated to native values:
// source identity must not depend on recognizing a word in that title.
func TestHMC269NativeSourceTitlePublicOwnership(t *testing.T) {
	for _, kind := range []string{"table", "summary", "diagram"} {
		t.Run(kind, func(t *testing.T) {
			bus, _, original := rawReviewContextHMC268(t, "current")
			table := rawReviewMembersHMC268(t, rawReviewChoicesHMC268(t, bus))
			block := rawReviewSelectorHMC268(table, kind)
			blocks := []any{block}
			if kind != "summary" {
				lead := rawReviewSelectorHMC268(table, "summary")
				lead["id"] = "lead"
				blocks = append([]any{lead}, blocks...)
			}
			control := b1659bExecuteAnswer(t, bus, map[string]any{"blocks": blocks}, false)
			if !control.Success {
				t.Fatal(control.Summary)
			}
			before, _ := json.Marshal(bus.Mutable.AnswerDocumentV2())
			block["title"] = "另一份采集：baseline.data"
			rejected := b1659bExecuteAnswer(t, bus, map[string]any{"blocks": blocks}, false)
			if rejected.Success {
				t.Fatalf("native source %s accepted a competing model title; %s", original, rejected.Summary)
			}
			after, _ := json.Marshal(bus.Mutable.AnswerDocumentV2())
			if string(before) != string(after) {
				t.Fatal("a rejected source-title collision replaced the accepted document")
			}
			if !strings.Contains(rejected.Summary, "title") {
				t.Fatalf("repair did not identify the competing field: %s", rejected.Summary)
			}
			visible := html.UnescapeString(render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), "zh"))
			if !strings.Contains(visible, original) || strings.Contains(visible, "另一份采集") {
				t.Fatal("source-owned presentation lost its original or rendered the rejected title")
			}
		})
	}
}

func TestHMC269SourceTitleExecutableSchemaParity(t *testing.T) {
	bus, _, _ := rawReviewContextHMC268(t, "current")
	native := types.BuildAnswerSemanticViewForBusContext(bus)
	table := rawReviewMembersHMC268(t, native.RuntimeMeasurementContract.Choices())
	for _, patch := range []bool{false, true} {
		schema, field := BuildAnswerDocumentParametersFor(native), "blocks"
		if patch {
			schema, field = BuildAnswerDocumentPatchParametersFor(native), "replace_blocks"
		}
		for _, title := range []string{"", "arbitrary source heading"} {
			block := rawReviewSelectorHMC268(table, "table")
			block["title"] = title
			raw, _ := json.Marshal(map[string]any{field: []any{block}})
			err := toolparam.Validate(raw, schema)
			if (err == nil) != (title == "") {
				t.Fatalf("schema mismatch patch=%t title=%q: %v", patch, title, err)
			}
		}
		ordinary, _ := json.Marshal(map[string]any{field: []any{
			map[string]any{"id": "ordinary", "kind": "summary", "title": "Unrelated explanation", "text": "Unchanged."},
		}})
		if err := toolparam.Validate(ordinary, schema); err != nil {
			t.Fatalf("native conditional schema blocked ordinary title: %v", err)
		}
	}
	legacy := runtimeMeasurementTestView()
	block := rawReviewSelectorHMC268(legacy.RuntimeMeasurementContract.Choices()[0], "table")
	block["title"] = "Legacy table without current source ownership"
	raw, _ := json.Marshal(map[string]any{"blocks": []any{block}})
	if err := toolparam.Validate(raw, BuildAnswerDocumentParametersFor(legacy)); err != nil {
		t.Fatal("legacy unqualified title was globally forbidden", err)
	}
}

func TestHMC269OrdinaryTitleRemainsModelOwned(t *testing.T) {
	bus, _, _ := rawReviewContextHMC268(t, "current")
	title := "普通说明与来源无绑定"
	out := b1659bExecuteAnswer(t, bus, map[string]any{"blocks": []any{
		map[string]any{"id": "ordinary", "kind": "summary", "title": title, "text": "说明保留。"},
	}}, false)
	if !out.Success {
		t.Fatal(out.Summary)
	}
	if got := blockByID(t, bus.Mutable.AnswerDocumentV2(), "ordinary"); got.Title != title || got.RuntimeMeasurement != nil {
		t.Fatal("ordinary title was rewritten")
	}
}
