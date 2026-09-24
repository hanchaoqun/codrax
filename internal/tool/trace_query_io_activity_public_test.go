package tool

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

const ioActivityPublicTrace = "reader-40 (40) [001] .... 2.020000: block_rq_issue: 8,0 R 4096 () 8 + 8 [reader]\n" +
	"reader-40 (40) [001] .... 2.080000: block_rq_issue: 8,0 R 8192 () 16 + 16 [reader]\n" +
	"writer-41 (41) [001] .... 2.220000: block_rq_issue: 8,0 W 8192 () 32 + 16 [writer]\n"

func ioActivityPublicQuery(t *testing.T, body string, params map[string]any) (*types.BusContext, types.ToolResult, *tracequery.IOActivityStats) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "activity.systrace")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	params["source"], params["path"], params["view"] = "path", path, "window_stats"
	rm := types.RequestModel{Language: "en", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric,
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactResourcePressure}}}
	mu := types.NewMutableState("Describe observed IO activity")
	mu.SetRequestModel(rm)
	bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, Language: "en", Mutable: mu, AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
	raw, _ := json.Marshal(params)
	result, err := (&TraceQuery{}).Execute(bus, raw)
	if err != nil || !result.Success {
		t.Fatalf("real query failed: %v / %s", err, result.Summary)
	}
	var payload string
	for _, row := range result.Observations {
		if row.Predicate == "io_activity" {
			payload = row.SourceRef.PayloadRef
			if row.Role != types.AnswerAggregateRoleSupportingCoverage || row.Value == "" {
				t.Fatal("activity lacks measured value or gained causal authority")
			}
			if _, ok := types.DecodeRuntimeMeasurementPublication(row); !ok {
				t.Fatal("native activity cannot bind exact physical source")
			}
		}
	}
	data, err := os.ReadFile(payload)
	if err != nil {
		t.Fatalf("missing native activity payload: %v", err)
	}
	var native tracequery.Result
	if err := json.Unmarshal(data, &native); err != nil || native.WindowStats == nil || native.WindowStats.IOActivity == nil {
		t.Fatalf("invalid native activity: %v", err)
	}
	if after, err := os.ReadFile(path); err != nil || string(after) != body {
		t.Fatal("read query mutated trace")
	}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	return bus, result, native.WindowStats.IOActivity
}

func TestIOActivityPublicNativeTablesSchemaRestoreAndRender(t *testing.T) {
	bus, result, stats := ioActivityPublicQuery(t, ioActivityPublicTrace, map[string]any{"time_start": 2, "time_end": 2.25, "bucket_ms": 100, "pid": 40})
	if len(stats.Groups) != 1 || stats.Groups[0].Values.EventCount != 3 || stats.Groups[0].Rates == nil || stats.Groups[0].Rates.EventsPerSecond != 12 {
		t.Fatalf("unpaired starts or other issuer lost: %+v", stats)
	}
	view := types.BuildAnswerSemanticViewForBusContext(bus)
	var tables []types.RuntimeMeasurementTable
	for _, table := range view.RuntimeMeasurementContract.Choices() {
		if strings.Contains(table.ObservationID, "#io_activity:") {
			tables = append(tables, table)
		}
	}
	if len(tables) != 3 {
		t.Fatalf("activity needs exactly three published projections: %d", len(tables))
	}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2"}
	for _, table := range tables {
		wire := map[string]any{"id": string(table.View), "kind": "table", "runtime_measurement": map[string]any{"observation_id": table.ObservationID, "view": table.View}}
		for _, patch := range []bool{false, true} {
			schema, key := BuildAnswerDocumentParametersFor(view), "blocks"
			if patch {
				schema, key = BuildAnswerDocumentPatchParametersFor(view), "replace_blocks"
			}
			data, _ := json.Marshal(map[string]any{key: []any{wire}})
			if err := toolparam.Validate(data, schema); err != nil {
				t.Fatalf("actual native %s selector rejected patch=%t: %v", table.View, patch, err)
			}
		}
		data, _ := json.Marshal(wire)
		var raw emitAnswerBlockV2
		json.Unmarshal(data, &raw)
		block, err := NormalizeEmitAnswerBlock(raw, "blocks[0]")
		if err != nil {
			t.Fatal(err)
		}
		doc.Blocks = append(doc.Blocks, block)
	}
	if err := bindRuntimeMeasurementReceipts(doc, view); err != nil {
		t.Fatal(err)
	}
	if got := doc.Blocks[0].RuntimeMeasurement.BoundTable.Rows[0]; !reflect.DeepEqual(got, []string{"all operations", "3", "3", "20480", "6826.66666667", "12", "81920"}) {
		t.Fatalf("native whole-window row changed: %v", got)
	}
	before := render.RenderAnswerDocument(doc, "en")
	for _, want := range []string{"20480", "81920", "2.100000", "2.200000", "2.250000", "0.6", "requested bytes", "Size lower bound inclusive", "actual wall-clock width"} {
		if !strings.Contains(before, want) {
			t.Errorf("native display lost %q", want)
		}
	}
	data, _ := json.Marshal(doc)
	var restored types.AnswerDocumentV2
	json.Unmarshal(data, &restored)
	if restored.Blocks[0].RuntimeMeasurement.IsBound() || !types.RebindRuntimeAnswerReceipts(&restored, view) || render.RenderAnswerDocument(&restored, "en") != before {
		t.Fatal("saved selector minted binding or changed native facts on rebind")
	}
	for i := range result.Observations {
		result.Observations[i].SourceRef.Path += ".replaced"
	}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	if types.RebindRuntimeAnswerReceipts(&restored, types.BuildAnswerSemanticViewForBusContext(bus)) {
		t.Fatal("replaced source reused saved measurements")
	}
}

func TestIOActivityPublicLineSelectionHasNoRates(t *testing.T) {
	bus, _, stats := ioActivityPublicQuery(t, ioActivityPublicTrace, map[string]any{"line_start": 2, "line_end": 2, "time_start": 2.2, "time_end": 2.25})
	if stats.Window != nil || len(stats.Groups) != 1 || stats.Groups[0].Values.EventCount != 1 || stats.Groups[0].Rates != nil || len(stats.Groups[0].Buckets) != 0 {
		t.Fatalf("line-first population invented a time denominator: %+v", stats)
	}
	for _, table := range types.BuildAnswerSemanticViewForBusContext(bus).RuntimeMeasurementContract.Choices() {
		if table.View == types.RuntimeMeasurementSummary && strings.Contains(table.ObservationID, "#io_activity:") {
			if table.Rows[0][3] != "8192" || table.Rows[0][5] != "unavailable" || table.Rows[0][6] != "unavailable" {
				t.Fatalf("line-selected exact bytes or unavailable rates lost: %v", table.Rows)
			}
		}
	}
}

func TestIOActivityPublicSharedTeaching(t *testing.T) {
	description := traceQueryDescriptionWithoutEventNameSuffix(t)
	// Reverse only the later, separately pinned cumulative-state correction.
	description = traceQueryDescriptionBeforeStateAccountingEvolution(t, description)
	suffix := " " + skill.TraceIOActivityTeaching
	if !strings.HasSuffix(description, suffix) {
		t.Fatal("new IO capability must remain at the terminal teaching slot")
	}
	// Only the prior SQLite preparation paragraph intentionally evolved in
	// HMC-17.7-C; the full reverse-delta test protects the rest of this prefix.
	if got := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.TrimSuffix(description, suffix)))); got != "275c7bd892a72ca0ff78d71f6b999dca13e915f543f30fcc3fc63d7f3c3b151f" {
		t.Fatalf("IO capability changed preceding dispatch teaching: %s", got)
	}
	var schema struct {
		Properties map[string]struct{ Description string }
	}
	json.Unmarshal((&TraceQuery{}).Parameters(), &schema)
	for _, text := range []string{(&TraceQuery{}).Description(), schema.Properties["view"].Description, skill.RenderTraceQueryViewMatrix()} {
		if strings.Count(text, skill.TraceIOActivityTeaching) != 1 {
			t.Fatal("activity teaching must be reused once per surface")
		}
	}
	for _, want := range []string{"window_sweep", "50..500", "window_stats.io_activity", "1..60000"} {
		if !strings.Contains(schema.Properties["bucket_ms"].Description, want) {
			t.Errorf("bucket parameter lost view-specific semantics %q", want)
		}
	}
}

func TestIOActivityPublicImplicitCaptureEndIsNotHalfOpenAuthority(t *testing.T) {
	_, result, stats := ioActivityPublicQuery(t, ioActivityPublicTrace, map[string]any{})
	if stats.Window == nil || !stats.Window.EndInclusive || len(stats.Groups) != 1 || stats.Groups[0].Values.EventCount != 3 {
		t.Fatalf("default capture extent lost its last endpoint: %+v", stats)
	}
	start, end := stats.Window.StartTs, stats.Window.EndTs
	rm := &types.RequestModel{RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "explicit half-open range"}}
	for _, row := range result.Observations {
		if row.Predicate == "io_activity" && row.SourceRef.QueryWindowKnown {
			t.Fatal("inclusive capture inventory impersonated an explicit half-open query")
		}
	}
	contract := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInput{ToolResults: []types.ToolResult{result}, RequestModel: rm})
	for _, table := range contract.Choices() {
		if strings.Contains(table.ObservationID, "#io_activity:") && (!strings.Contains(table.Label, "Supplementary query") || !strings.Contains(strings.Join(table.Notes, " "), "includes the final endpoint")) {
			t.Fatalf("capture end semantics or supplementary scope lost: %+v", table)
		}
	}
}
