package tool

import (
	"encoding/json"
	"html"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the actual query -> independent comparison read -> public emit path.
// The later standalone memo is deliberately not a current display credential.
func TestHMC277NativeMeasurementPairFixedSurfacesPublic(t *testing.T) {
	for _, singleFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "pair_first", true: "single_first_memo"}[singleFirst], func(t *testing.T) {
			ctx, single, pair, _ := hmc221CPUComposition(t, singleFirst)
			report, _ := pair.RuntimeMeasurementPair.Report()
			for _, table := range hmc221CompositionChoices(ctx, single) {
				if table.HasNativeMeasurementFacts() != singleFirst {
					t.Fatal("standalone memo and independent current read were not distinguished")
				}
			}
			for _, results := range [][]types.ToolResult{{pair}, {single, pair}, {pair, single}} {
				for _, table := range hmc221CompositionChoices(ctx, results...) {
					if table.ObservationID == report.ID {
						if table.HasNativeMeasurementFacts() {
							t.Fatal("pair status table borrowed generic measurement qualification")
						}
						continue
					}
					if !table.HasNativeMeasurementFacts() {
						t.Fatal("independently current comparison lost fixed measurement display qualification")
					}
					if table.View != types.RuntimeMeasurementDistribution {
						continue
					}
					for _, kind := range []string{"summary", "table", "diagram"} {
						block := map[string]any{"id": "facts", "kind": kind, "runtime_measurement": map[string]any{"observation_id": table.ObservationID, "view": table.View}}
						blocks := []any{block}
						if kind != "summary" {
							blocks = append([]any{map[string]any{"id": "lead", "kind": "summary", "text": "Independent CPU residency measurements."}}, blocks...)
						}
						if out := b1659bExecuteAnswer(t, ctx, map[string]any{"blocks": blocks}, false); !out.Success {
							t.Fatalf("%s rejected current pair facts: %s", kind, out.Summary)
						}
						visible := html.UnescapeString(render.RenderAnswerDocument(ctx.Mutable.AnswerDocumentV2(), "en"))
						for _, value := range []string{"CPU0", "CPU1", "CPU2", "Query Baseline"} {
							if !strings.Contains(visible, value) {
								t.Fatalf("%s lost original value/source %q", kind, value)
							}
						}
						// A serialized selector/table is still public data only.
						encoded, _ := json.Marshal(table)
						var restored types.RuntimeMeasurementTable
						if err := json.Unmarshal(encoded, &restored); err != nil || restored.HasNativeFixedFacts() {
							t.Fatal("table JSON recreated current fixed-fact permission", err)
						}
					}
				}
			}
		})
	}
}
