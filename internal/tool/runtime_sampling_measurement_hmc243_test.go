package tool

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC243PublicSamplingMeasurement(t *testing.T) {
	bundle, direct := samplingSourceBundleHMC241(t)
	for _, path := range []string{bundle, direct} {
		result, _ := samplingIdentityQueryHMC239(t, path, "perf_stats", map[string]any{"pid": 11, "pattern": "symbol=work"})
		contract := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInput{ToolResults: []types.ToolResult{result}})
		var group *types.RuntimeMeasurementTable
		for _, table := range contract.Choices() {
			if table.View == types.RuntimeMeasurementSummary && strings.Contains(table.Label, "/app/A.so") {
				copy := table
				group = &copy
			}
		}
		if group == nil {
			t.Fatal("successful native filtered sampling query has no selectable same-source sampling facts")
		}
		if len(group.Rows) != 3 {
			t.Fatalf("independent populations must remain three rows: %+v", group.Rows)
		}
	}
}

func TestHMC243SamplingImplicitEndpointAndScope(t *testing.T) {
	_, path := samplingSourceBundleHMC241(t)
	dir := t.TempDir()
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("selected samples")}
	raw, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "perf_stats", "pid": 11, "pattern": "dso=/app/A.so"})
	result, err := (&TraceQuery{}).Execute(ctx, raw)
	if err != nil || !result.Success {
		t.Fatalf("query: %v %s", err, result.Summary)
	}
	payload, _, _ := samplingRepairPayloadHMC235(t, result)
	if payload.PerfStats == nil || len(payload.PerfStats.Cohorts) == 0 || !payload.PerfStats.Cohorts[0].Scope.Window.EndInclusive {
		t.Fatal("fixture did not exercise inclusive native capture end")
	}
	for _, mode := range []string{"implicit", "narrow_half_open", "broader_half_open"} {
		input := types.ObservationLedgerInput{ToolResults: []types.ToolResult{result}}
		if mode != "implicit" {
			start, end := 1.0, 3.0
			if mode == "broader_half_open" {
				end = 4
			}
			input.RequestModel = &types.RequestModel{RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, SourceQuote: "selected reporting window", TimeStart: &start, TimeEnd: &end}}
		}
		found := false
		for _, table := range types.BuildRuntimeMeasurementContract(input).Choices() {
			if !table.HasNativeSamplingFacts() || table.View != types.RuntimeMeasurementSummary {
				continue
			}
			if mode == "narrow_half_open" {
				t.Fatal("inclusive aggregate crossed the requested half-open end")
			}
			if !strings.Contains(strings.Join(table.Notes, " "), "[1.100000, 3.000000]") {
				t.Fatalf("native closed endpoint was relabeled: %v", table.Notes)
			}
			for _, row := range table.Rows {
				if row[0] == "cycles" && row[1] == "原始 perf 数据" {
					found = row[4] == "3" && row[5] == "915" && row[6] == "3" && row[7] == "915"
				}
			}
		}
		if mode != "narrow_half_open" && !found {
			t.Fatalf("%s changed inclusive population instead of reporting it", mode)
		}
	}
}

func TestHMC243SamplingIndependentQualityAndAvailability(t *testing.T) {
	bundle, _ := samplingSourceBundleHMC241(t)
	for _, filter := range []string{"", "symbol=work", "dso=/app/A.so"} {
		r, _ := samplingIdentityQueryHMC239(t, bundle, "perf_stats", map[string]any{"pid": 11, "pattern": filter})
		contract := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInput{ToolResults: []types.ToolResult{r}})
		populations := map[string][]string{}
		for _, table := range contract.Choices() {
			if table.View == types.RuntimeMeasurementSummary && strings.Contains(table.Label, "/app/A.so") {
				for _, row := range table.Rows {
					if row[9] == "" || populations[row[9]] != nil {
						t.Fatal("cohort labels are missing or ambiguous")
					}
					populations[row[9]] = row
				}
			}
		}
		found := false
		for _, table := range contract.Choices() {
			if !strings.Contains(table.Label, "/app/A.so") {
				continue
			}
			if table.View == types.RuntimeMeasurementSummary {
				found = true
				for _, row := range table.Rows {
					if row[0] == "cycles" && row[1] == "原始 perf 数据" {
						wantCount, wantWeight, wantFraction := "3", "35", "15/35"
						if filter == "dso=/app/A.so" {
							wantCount, wantWeight, wantFraction = "2", "15", "15/15"
						}
						if row[4] != "2" || row[5] != "15" || row[6] != wantCount || row[7] != wantWeight || row[8] != wantFraction {
							t.Fatalf("filter=%s changed independent population: %v", filter, row)
						}
					}
				}
			}
			if table.View == types.RuntimeMeasurementDistribution && filter != "dso=/app/A.so" {
				unknown, exact, unavailable := false, false, false
				for _, row := range table.Rows {
					summary := populations[row[9]]
					if summary == nil || summary[0] != row[0] || summary[1] != row[1] || summary[2] != row[10] || summary[3] != row[2] || summary[6] != row[7] || summary[7] != row[8] || summary[10] != row[11] {
						t.Fatalf("quality row lost its exact population identity: %v", row)
					}
					if row[0] != "cycles" || row[1] != "原始 perf 数据" {
						continue
					}
					unknown = unknown || row[3] == "样本类型" && row[4] == "未知" && row[5] == "3" && row[6] == "35"
					exact = exact || row[3] == "帧边界" && row[4] == "精确帧边界" && row[5] == "2" && row[6] == "30"
					unavailable = unavailable || row[3] == "帧边界" && row[4] == "帧边界不可用" && row[5] == "1" && row[6] == "5"
				}
				if !unknown || !exact || !unavailable {
					t.Fatalf("query quality confused sample-kind/frames/subset: %+v", table.Rows)
				}
			}
		}
		if !found {
			t.Fatal("native facts missing")
		}
	}
	for _, mode := range []string{"unknown", "overflow", "topn"} {
		var body strings.Builder
		for i := 0; i < 12; i++ {
			known, weight, symbol := "true", "1", fmt.Sprintf("work%d", i)
			if mode == "unknown" {
				known = "false"
			}
			if mode == "overflow" {
				weight = "9223372036854775807"
				symbol = "work"
			}
			fmt.Fprintf(&body, "worker-11 (10) [000] .... 1.%03d000: perf_sample: pid=10 tid=11 thread_comm=worker thread_identity_known=%s sample_weight=%s event=cycles symbol=%s dso=/app/A.so source=raw\n", 100+i, known, weight, symbol)
		}
		path := filepath.Join(t.TempDir(), "samples.perftrace")
		if err := os.WriteFile(path, []byte(body.String()), 0600); err != nil {
			t.Fatal(err)
		}
		r, _ := samplingIdentityQueryHMC239(t, path, "perf_stats", map[string]any{"pid": 11, "pattern": "dso=/app/A.so"})
		choices := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInput{ToolResults: []types.ToolResult{r}}).Choices()
		if mode != "topn" && len(choices) > 0 {
			t.Fatalf("%s acquired unavailable population facts", mode)
		}
		if mode == "topn" {
			count := 0
			for _, table := range choices {
				if table.View == types.RuntimeMeasurementSummary {
					count++
					if table.Rows[0][6] != "12" || table.Rows[0][7] != "12" || !strings.Contains(strings.Join(table.Notes, " "), "展示截断") {
						t.Fatalf("topn silently changed denominator: %+v", table)
					}
				}
			}
			if count == 0 || count >= 12 {
				t.Fatalf("topn not exercised: %d", count)
			}
		}
	}
}

func TestHMC243NativeFactDiagramPersistBoundary(t *testing.T) {
	ctx, event := eventDisplayPublicContext236(t)
	block := eventBlock236("facts", "diagram", event.ID, map[string]any{"field_keys": []string{"jank.appid"}})
	input := map[string]any{"blocks": []any{map[string]any{"id": "lead", "kind": "summary", "text": "Observed event facts."}, block}}
	if out := b1659bExecuteAnswer(t, ctx, input, false); !out.Success {
		t.Fatalf("native event diagram persist: %s", out.Summary)
	}
	bound := blockByID(t, ctx.Mutable.AnswerDocumentV2(), "facts")
	if !runtimeMeasurementBoundFactSurface(bound) {
		t.Fatal("diagram did not bind native authority")
	}
	for _, mode := range []string{"stale", "json", "ordinary"} {
		b := bound
		b.RuntimeMeasurement = bound.RuntimeMeasurement.Clone()
		switch mode {
		case "stale":
			b.RuntimeMeasurement.ObservationID += ".other"
		case "json":
			raw, _ := json.Marshal(b)
			b = types.AnswerBlock{}
			_ = json.Unmarshal(raw, &b)
		case "ordinary":
			table := runtimeMeasurementTestView().RuntimeMeasurementContract.Choices()[0]
			b.RuntimeMeasurement = &types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View, BoundTable: &table}
		}
		if len(collectMergedV2DocBlockViolations(&types.AnswerDocumentV2{Blocks: []types.AnswerBlock{b}})) == 0 {
			t.Fatalf("%s bypassed missing diagram without native fact authority", mode)
		}
	}
	if bound.Diagram != nil || len(bound.EdgeAnchors) != 0 || len(bound.RelationClaims) != 0 {
		t.Fatal("fact diagram minted relation evidence")
	}
}

func TestHMC243SamplingMixedSchema(t *testing.T) {
	_, view, sampling := samplingMeasurementContextHMC243(t)
	eventCtx, event := eventDisplayPublicContext236(t)
	events := types.BuildAnswerSemanticViewForBusContext(eventCtx).RuntimeMeasurementContract.Choices()
	legacy := runtimeMeasurementTestView().RuntimeMeasurementContract.Choices()
	view.RuntimeMeasurementContract.Tables = append(view.RuntimeMeasurementContract.Tables, events...)
	view.RuntimeMeasurementContract.Tables = append(view.RuntimeMeasurementContract.Tables, legacy...)
	view.RequiredBlocks = []types.BlockRequirement{{Kind: types.BlockSummary, Required: true}}
	view.OptionalBlocks = []types.BlockRequirement{{Kind: types.BlockTable}, {Kind: types.BlockDiagram}}
	view.DiagramPlan = &types.DiagramFacetGraph{Kind: types.DiagramFlow}
	for _, patch := range []bool{false, true} {
		schema, key := BuildAnswerDocumentParametersFor(view), "blocks"
		if patch {
			schema, key = BuildAnswerDocumentPatchParametersFor(view), "replace_blocks"
		}
		encoded, _ := json.Marshal(schema)
		for _, teaching := range []string{"event_selection chooses row_ids", "Published native sampling facts"} {
			if !strings.Contains(string(encoded), teaching) {
				t.Fatalf("mixed teaching lost %q", teaching)
			}
		}
		for _, family := range []string{"event", "sampling", "legacy"} {
			for _, kind := range []string{"summary", "table", "diagram"} {
				pair := map[string]any{"observation_id": sampling.ObservationID, "view": sampling.View}
				if family == "event" {
					pair = map[string]any{"observation_id": event.ID, "view": "members", "event_selection": map[string]any{"field_keys": []string{"jank.appid"}}}
				}
				if family == "legacy" {
					pair = map[string]any{"observation_id": legacy[0].ObservationID, "view": legacy[0].View}
				}
				block := map[string]any{"id": "facts", "kind": kind, "runtime_measurement": pair}
				raw, _ := json.Marshal(map[string]any{key: []any{block}})
				want := family != "legacy" || kind == "table"
				if err := toolparam.Validate(raw, schema); (err == nil) != want {
					t.Fatalf("family=%s kind=%s patch=%t schema=%v", family, kind, patch, err)
				}
				b, _ := json.Marshal(block)
				var wire emitAnswerBlockV2
				_ = json.Unmarshal(b, &wire)
				normal, err := NormalizeEmitAnswerBlock(wire, "blocks[0]", view.RuntimeMeasurementContract)
				if (err == nil && runtimeMeasurementBindingValid(normal, view)) != want {
					t.Fatalf("family=%s kind=%s binding differs: %v", family, kind, err)
				}
				if !want {
					continue
				}
				block["text"] = "copied measurements"
				raw, _ = json.Marshal(map[string]any{key: []any{block}})
				if toolparam.Validate(raw, schema) == nil {
					t.Fatal("competing copied text accepted")
				}
			}
		}
	}
}

func samplingMeasurementContextHMC243(t *testing.T) (*types.BusContext, *types.AnswerSemanticView, types.RuntimeMeasurementTable) {
	t.Helper()
	bundle, _ := samplingSourceBundleHMC241(t)
	dir := t.TempDir()
	rm := nativeFactDisplayRequest(types.RequestedAnswerDimensionObservedValue)
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("sampling facts"), AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
	ctx.Mutable.SetRequestModel(rm)
	params, _ := json.Marshal(map[string]any{"source": "path", "path": bundle, "view": "perf_stats", "pid": 11, "time_start": 1, "time_end": 2, "pattern": "symbol=work"})
	r, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !r.Success {
		t.Fatalf("query: %v %s", err, r.Summary)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{r}})
	view := types.BuildAnswerSemanticViewForBusContext(ctx)
	for _, table := range view.RuntimeMeasurementContract.Choices() {
		if table.View == types.RuntimeMeasurementSummary && strings.Contains(table.Label, "/app/A.so") {
			return ctx, view, table
		}
	}
	t.Fatal("missing native group")
	return nil, nil, types.RuntimeMeasurementTable{}
}

func TestHMC243SamplingFactsPublicEmitPatchRecovery(t *testing.T) {
	ctx, view, table := samplingMeasurementContextHMC243(t)
	for _, kind := range []types.AnswerBlockKind{types.BlockSummary, types.BlockTable, types.BlockDiagram} {
		t.Run(string(kind), func(t *testing.T) {
			selector := map[string]any{"observation_id": table.ObservationID, "view": "summary"}
			block := map[string]any{"id": "facts", "kind": kind, "runtime_measurement": selector}
			blocks := []any{block}
			if kind != types.BlockSummary {
				blocks = append([]any{map[string]any{"id": "lead", "kind": "summary", "text": "Independent sampling populations are shown below."}}, blocks...)
			}
			out := b1659bExecuteAnswer(t, ctx, map[string]any{"blocks": blocks}, false)
			if !out.Success {
				t.Fatalf("public emit: %s", out.Summary)
			}
			doc := ctx.Mutable.AnswerDocumentV2()
			visible := html.UnescapeString(render.RenderAnswerDocument(doc, "zh"))
			for _, value := range []string{"15/35", "7/7", "100/100", "查询总体记录数", "未知"} {
				if !strings.Contains(visible, value) {
					t.Errorf("surface lost %q: %s", value, visible)
				}
			}
			if strings.Contains(visible, "-->") || strings.Contains(visible, "900") {
				t.Fatal("fact diagram minted edge or outside-window value")
			}
			wire, _ := json.Marshal(doc)
			var restored types.AnswerDocumentV2
			if json.Unmarshal(wire, &restored) != nil || !types.RebindRuntimeAnswerReceipts(&restored, view) {
				t.Fatal("current-answer rebind failed")
			}
			if strings.Contains(string(wire), "15/35") {
				t.Fatal("producer values serialized as model answer input")
			}
			recovered := textRecoveryWithBadSibling(t, block)
			if b := blockByID(t, recovered.Document, "facts"); b.Kind != kind || b.RuntimeMeasurement.IsBound() {
				t.Fatal("recovery changed shape or minted authority")
			}
			if !types.RebindRuntimeAnswerReceipts(recovered.Document, view) {
				t.Fatal("recovery failed current bind")
			}
			patched := b1659bExecuteAnswer(t, ctx, map[string]any{"replace_blocks": []any{block}}, true)
			if !patched.Success {
				t.Fatalf("public patch: %s", patched.Summary)
			}
		})
	}
}

func TestHMC243SamplingPublicationTampering(t *testing.T) {
	bundle, _ := samplingSourceBundleHMC241(t)
	result, _ := samplingIdentityQueryHMC239(t, bundle, "perf_stats", map[string]any{"pid": 11, "pattern": "symbol=work"})
	var original types.ObservationRecord
	for _, r := range result.Observations {
		if r.Predicate == types.RuntimeSamplingMeasurementPredicate {
			original = r
			break
		}
	}
	if _, ok := types.DecodeRuntimeMeasurementPublication(original); !ok {
		t.Fatal("native baseline missing")
	}
	for _, mode := range []string{"json", "source", "window", "query", "clock", "target", "subject", "role", "value", "filter", "duplicate", "unknown", "truncated"} {
		t.Run(mode, func(t *testing.T) {
			r := original
			r.RichNotes = append([]string(nil), r.RichNotes...)
			switch mode {
			case "json":
				b, _ := json.Marshal(r)
				_ = json.Unmarshal(b, &r) // Explicitly use a fresh zero record below.
				var clean types.ObservationRecord
				_ = json.Unmarshal(b, &clean)
				r = clean
			case "source":
				r.SourceRef.Path += ".other"
			case "window":
				r.SourceRef.QueryWindowEndTs++
			case "query":
				r.SourceRef.QueryScopeID += ".other"
			case "clock":
				r.SourceRef.TimeDomain = "other"
			case "target":
				r.SourceRef.QueryTargetScope = "process"
			case "subject":
				r.Subject = "other-12"
			case "role":
				r.Role = types.AnswerAggregateRolePrincipalAnswer
			case "value":
				r.Value = "900"
			default:
				for i, n := range r.RichNotes {
					if !strings.HasPrefix(n, types.TraceNoteKeyRuntimeMeasurement+"=") {
						continue
					}
					switch mode {
					case "duplicate":
						r.RichNotes = append(r.RichNotes, n)
					case "unknown":
						r.RichNotes[i] = strings.Replace(n, "={", "={\"forged\":true,", 1)
					case "truncated":
						r.RichNotes[i] = n[:len(n)-1]
					case "filter":
						r.RichNotes[i] = strings.ReplaceAll(n, "symbol=work", "symbol=other")
					}
				}
			}
			if _, ok := types.DecodeRuntimeMeasurementPublication(r); ok {
				t.Fatal("mutated/JSON-only receipt acquired native fact capability")
			}
		})
	}
	pub, _ := types.DecodeRuntimeMeasurementPublication(original)
	for _, table := range pub.Tables {
		if table.HasCompleteNativeDisplayPopulation() || table.MemberSet != nil || !table.HasNativeSamplingFacts() {
			t.Fatal("wrong display authority")
		}
	}
	contract := &types.RuntimeMeasurementContract{Tables: pub.Tables}
	ref := &types.AnswerRuntimeMeasurementReceipt{ObservationID: original.ID, View: types.RuntimeMeasurementSummary}
	if !types.BindRuntimeMeasurementReceipt(ref, contract) || !ref.IsBound() {
		t.Fatal("native bind")
	}
	clone := ref.Clone()
	clone.BoundTable.Rows[0][0] = "changed"
	if clone.IsBound() || !ref.IsBound() {
		t.Fatal("mutation accepted or clone aliases")
	}
}
