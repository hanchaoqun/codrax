package tool

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1646CoverageMarkerActualPublication(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, marked := range []bool{false, true} {
			for _, completion := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/marked=%t/completion=%t", lang, marked, completion), func(t *testing.T) {
					bus := b1646SleepIOMarkerBus(t, lang, marked, completion)
					// A real capped query activates the existing coverage footer;
					// the full state account still comes from the original query.
					params, _ := json.Marshal(map[string]any{"source": "path", "path": filepath.Join(bus.RepoRoot, "marker-and-completion.ftrace"), "view": "event_search", "pid": 41, "time_start": 5.0, "time_end": 5.02, "limit": 1})
					page, err := (&TraceQuery{}).Execute(bus, params)
					if err != nil || !page.Success {
						t.Fatalf("actual capped query: %v; %+v", err, page)
					}
					bus.ToolResults = append(bus.ToolResults, page)
					before, _ := json.Marshal(bus.ToolResults)
					doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary, Text: "Original model wording, including S sleep and Binder; unchanged."}}}
					modelWire, _ := modelOwnedAnswerBlockWire(doc)
					result, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
					if err != nil || !result.Success {
						t.Fatalf("actual publication failed: %v; %+v", err, result)
					}
					stored := bus.Mutable.AnswerDocumentV2()
					if err := requireModelOwnedAnswerBlockWirePreserved(modelWire, stored); err != nil {
						t.Fatal(err)
					}
					var footer string
					for _, block := range stored.Blocks {
						if block.ID == runtimeTraceCausalProjectionCoverageBlockID {
							footer = block.Text
						}
					}
					if footer == "" {
						t.Fatal("actual coverage footer was not published")
					}
					marker := "0.000"
					if marked {
						marker = "9.010"
					}
					zh := lang == "zh"
					markedValue := marker + "ms of " + runtimeTraceSleepIOMarkerLabel(zh)
					if zh {
						markedValue = runtimeTraceSleepIOMarkerLabel(zh) + " " + marker + "ms"
					}
					for _, want := range []string{markedValue, runtimeTraceSleepIOMarkerBoundary(zh), "20.000ms", "running=10.000ms", "runnable=0.990ms", "sleep=9.010ms"} {
						if !strings.Contains(footer, want) {
							t.Errorf("actual footer lacks marker-only boundary %q: %s", want, footer)
						}
					}
					if strings.Count(footer, runtimeTraceSleepIOMarkerBoundary(zh)) != 1 {
						t.Errorf("footer must carry one local marker boundary: %s", footer)
					}
					md := render.RenderAnswerDocument(stored, lang)
					if !strings.Contains(md, footer) || !strings.Contains(md, doc.Blocks[0].Text) {
						t.Fatal("public rendering lost the footer or model text")
					}
					after, _ := json.Marshal(bus.ToolResults)
					if string(before) != string(after) {
						t.Fatal("display changed actual native query values")
					}
				})
			}
		}
	}
}

func TestB1646CoverageWithoutStateDoesNotMintZero(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		bus := &types.BusContext{Language: lang, Mutable: types.NewMutableState("no runtime observation")}
		doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary, Text: "Unknown state remains unknown."}}}
		result, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
		if err != nil || !result.Success {
			t.Fatalf("actual publication: %v; %+v", err, result)
		}
		md := render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), lang)
		if strings.Contains(md, "0.000ms") || strings.Contains(md, runtimeTraceSleepIOMarkerLabel(lang == "zh")) {
			t.Fatalf("missing source minted a zero state account: %s", md)
		}
	}
}
