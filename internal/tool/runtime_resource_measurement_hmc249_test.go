package tool

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func resourceMeasurementContextHMC249(t *testing.T, nested bool, pid int, edits ...func(*sql.DB)) (*types.BusContext, types.ToolResult, *types.AnswerSemanticView) {
	t.Helper()
	dir := t.TempDir()
	rel := "capture.data"
	if nested {
		rel = "nested/capture.data"
	}
	data, err := os.ReadFile(filepath.Join("../../eval/fixtures/hmosperf_catalog_objects/captures", rel))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "capture.data")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if len(edits) > 0 {
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		for _, edit := range edits {
			edit(db)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
	rm := nativeFactDisplayRequest(types.RequestedAnswerDimensionObservedValue)
	rm.DiagramHint = &types.DiagramHint{Kind: types.DiagramFlow}
	bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, Language: "zh", Mutable: types.NewMutableState("resource facts"), AnalysisIR: &types.AnalysisIR{RequestModel: rm},
		TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(dir, ".codrax")})}
	args := map[string]any{"source": "path", "path": path, "view": "resource_stack", "time_start": 10, "time_end": 10.05}
	if pid > 0 {
		args["pid"] = pid
	}
	raw, _ := json.Marshal(args)
	result, err := (&TraceQuery{}).Execute(bus, raw)
	if err != nil || !result.Success {
		t.Fatalf("native query: %v %s", err, result.Summary)
	}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	return bus, result, types.BuildAnswerSemanticViewForBusContext(bus)
}

func resourceNativeTablesHMC249(view *types.AnswerSemanticView) []types.RuntimeMeasurementTable {
	var out []types.RuntimeMeasurementTable
	for _, table := range view.RuntimeMeasurementContract.Choices() {
		if table.HasNativeResourceFacts() {
			out = append(out, table)
		}
	}
	return out
}

func TestHMC249ResourcePublicEmitPatchRecovery(t *testing.T) {
	bus, _, view := resourceMeasurementContextHMC249(t, false, 101)
	// Fact binding does not itself request a visual surface. Exercise the
	// existing optional diagram contract without making relations required.
	view.OptionalBlocks = append(view.OptionalBlocks, types.BlockRequirement{Kind: types.BlockDiagram})
	view.DiagramPlan = &types.DiagramFacetGraph{Kind: types.DiagramFlow}
	tables := resourceNativeTablesHMC249(view)
	if len(tables) != 2 {
		t.Fatalf("native resource pairs=%d", len(tables))
	}
	for _, table := range tables {
		for _, kind := range []string{"summary", "table", "diagram"} {
			t.Run(string(table.View)+"/"+kind, func(t *testing.T) {
				block := map[string]any{"id": "facts", "kind": kind, "runtime_measurement": map[string]any{"observation_id": table.ObservationID, "view": table.View}}
				blocks := []any{block}
				if kind != "summary" {
					blocks = append([]any{map[string]any{"id": "lead", "kind": "summary", "text": "以下仅列原件记录。"}}, blocks...)
				}
				args := map[string]any{"blocks": blocks}
				raw, _ := json.Marshal(args)
				if err := toolparam.Validate(raw, BuildAnswerDocumentParametersFor(view)); err != nil {
					t.Fatal(err)
				}
				if out := b1659bExecuteAnswer(t, bus, args, false); !out.Success {
					t.Fatalf("emit: %s", out.Summary)
				}
				doc := bus.Mutable.AnswerDocumentV2()
				visible := html.UnescapeString(render.RenderAnswerDocument(doc, "zh"))
				for _, want := range []string{"源事件", "线程ID=101；", "进程ID=101；", "未知", "不是函数执行顺序"} {
					if !strings.Contains(visible, want) {
						t.Errorf("missing %q: %s", want, visible)
					}
				}
				if table.View == types.RuntimeMeasurementMembers && (!strings.Contains(visible, "原深度") || !strings.Contains(visible, "buffer_cleanup") || !strings.Contains(visible, "物理行=")) {
					t.Fatal("frame identity/depth disappeared")
				}
				if table.View == types.RuntimeMeasurementSummary && (!strings.Contains(visible, "AllocEvent") || !strings.Contains(visible, "源数量/大小=512") || !strings.Contains(visible, "单位未在该载体声明")) {
					t.Fatal("allocation source value or unit boundary changed")
				}
				if strings.Contains(visible, "-->") || strings.Contains(visible, "trace-query-result-") {
					t.Fatal("invented edge or long query identifier in visible facts")
				}
				wire, _ := json.Marshal(doc)
				if strings.Contains(string(wire), "buffer_cleanup") {
					t.Fatal("native values copied into answer JSON")
				}
				var restored types.AnswerDocumentV2
				if json.Unmarshal(wire, &restored) != nil || !types.RebindRuntimeAnswerReceipts(&restored, view) {
					t.Fatal("current rebind failed")
				}
				recovered := textRecoveryWithBadSibling(t, block)
				if !types.RebindRuntimeAnswerReceipts(recovered.Document, view) {
					t.Fatal("text recovery lost current native pair")
				}
				if out := b1659bExecuteAnswer(t, bus, map[string]any{"replace_blocks": []any{block}}, true); !out.Success {
					t.Fatalf("patch: %s", out.Summary)
				}
				bad := map[string]any{"id": "facts", "kind": kind, "text": "invented arrow", "runtime_measurement": block["runtime_measurement"]}
				before, _ := json.Marshal(bus.Mutable.AnswerDocumentV2())
				if out := b1659bExecuteAnswer(t, bus, map[string]any{"replace_blocks": []any{bad}}, true); out.Success {
					t.Fatal("copied parallel fact text accepted")
				}
				after, _ := json.Marshal(bus.Mutable.AnswerDocumentV2())
				if string(before) != string(after) {
					t.Fatal("failed patch changed accepted facts")
				}
			})
		}
	}
}

func TestHMC249ResourcePublicationBoundaries(t *testing.T) {
	_, result, view := resourceMeasurementContextHMC249(t, false, 101)
	var original types.ObservationRecord
	for _, r := range result.Observations {
		if r.Predicate == TraceResourceStackPredicate {
			original = r
		}
	}
	if _, ok := types.DecodeRuntimeMeasurementPublication(original); !ok {
		t.Fatal("native baseline missing")
	}
	for _, mode := range []string{"json", "source", "payload", "query", "clock", "clock_offset", "window", "target", "scope", "subject", "role", "producer", "native_note", "duplicate", "unknown", "truncated"} {
		t.Run(mode, func(t *testing.T) {
			r := original
			r.RichNotes = append([]string(nil), r.RichNotes...)
			switch mode {
			case "json":
				b, _ := json.Marshal(r)
				var clean types.ObservationRecord
				_ = json.Unmarshal(b, &clean)
				r = clean
			case "source":
				r.SourceRef.Path += ".other"
			case "payload":
				r.SourceRef.PayloadRef += ".other"
			case "query":
				r.SourceRef.QueryScopeID += ".other"
			case "clock":
				r.SourceRef.TimeDomain = "other"
			case "clock_offset":
				v := 1.0
				r.SourceRef.ClockOffsetSec = &v
			case "window":
				r.SourceRef.QueryWindowEndTs++
			case "target":
				r.SourceRef.QueryTargetPID++
			case "scope":
				r.SourceRef.QueryTargetScope = "process"
			case "subject":
				r.Subject = "other"
			case "role":
				r.Role = types.AnswerAggregateRolePrincipalAnswer
			case "producer":
				r.Producer = "model"
			case "native_note":
				r.RichNotes[0] += " "
			default:
				for i, n := range r.RichNotes {
					if strings.HasPrefix(n, types.TraceNoteKeyRuntimeMeasurement+"=") {
						switch mode {
						case "duplicate":
							r.RichNotes = append(r.RichNotes, n)
						case "unknown":
							r.RichNotes[i] = strings.Replace(n, "={", "={\"forged\":true,", 1)
						case "truncated":
							r.RichNotes[i] = n[:len(n)-1]
						}
					}
				}
			}
			if _, ok := types.DecodeRuntimeMeasurementPublication(r); ok {
				t.Fatal("changed/JSON-only source acquired fixed native facts")
			}
		})
	}
	for _, table := range resourceNativeTablesHMC249(view) {
		if table.MemberSet != nil || table.HasCompleteNativeDisplayPopulation() || table.EventData != nil || table.HasNativeSamplingFacts() || table.DefaultPresentation {
			t.Fatal("resource facts acquired another producer's authority")
		}
		r := &types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View}
		if !types.BindRuntimeMeasurementReceipt(r, view.RuntimeMeasurementContract) {
			t.Fatal("bind")
		}
		clone := r.Clone()
		clone.BoundTable.Rows[0][0] = "forged"
		if clone.IsBound() || !r.IsBound() {
			t.Fatal("mutable binding or shallow clone")
		}
	}
}

func TestHMC249ResourceEmptyAndBroadBoundaries(t *testing.T) {
	_, _, view := resourceMeasurementContextHMC249(t, true, 202)
	tables := resourceNativeTablesHMC249(view)
	if len(tables) != 2 {
		t.Fatal("empty native query not published")
	}
	for _, table := range tables {
		raw, _ := json.Marshal(table.Rows)
		if strings.Contains(string(raw), "background") || strings.Contains(string(raw), "线程ID=0") {
			t.Fatal("empty query borrowed a name or invented zero identity")
		}
		if table.View == types.RuntimeMeasurementMembers && len(table.Rows) != 0 {
			t.Fatal("empty source invented frames")
		}
	}
	_, result, broad := resourceMeasurementContextHMC249(t, false, 0)
	if len(resourceNativeTablesHMC249(broad)) != 0 {
		t.Fatal("broad query silently gained exact selector authority")
	}
	kept := false
	for _, r := range result.Observations {
		if _, ok := DecodeTraceResourceStack(r); ok {
			kept = true
		}
	}
	if !kept {
		t.Fatal("broad query lost existing native resource facts")
	}
}

func TestHMC249ResourceUnitsOwnersDepthAndBudget(t *testing.T) {
	_, result, view := resourceMeasurementContextHMC249(t, false, 101, func(db *sql.DB) {
		if _, err := db.Exec("UPDATE process SET pid=10 WHERE pid=101; UPDATE native_hook SET event_type='FD_Open_Event' WHERE id=1; UPDATE native_hook SET event_type='THREAD_Create_Event' WHERE id=3"); err != nil {
			t.Fatal(err)
		}
	})
	var native types.ObservationRecord
	for _, r := range result.Observations {
		if r.Predicate == TraceResourceStackPredicate {
			native = r
		}
	}
	p, ok := DecodeTraceResourceStack(native)
	if !ok {
		t.Fatal("native fixture")
	}
	if p.Events[0].Source.TID != 101 || p.Events[0].Source.PID != 10 {
		t.Fatal("fixture did not separate thread and process")
	}
	for _, table := range resourceNativeTablesHMC249(view) {
		ref := &types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View}
		if !types.BindRuntimeMeasurementReceipt(ref, view.RuntimeMeasurementContract) {
			t.Fatal("bind")
		}
		for _, kind := range []types.AnswerBlockKind{types.BlockSummary, types.BlockTable, types.BlockDiagram} {
			text := html.UnescapeString(render.RenderAnswerDocument(&types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "facts", Kind: kind, RuntimeMeasurement: ref}}}, "zh"))
			for _, want := range []string{"线程ID=101；进程ID=10；", "缺深度=1", "重复深度=1", "已记录深度连续"} {
				if !strings.Contains(text, want) {
					t.Fatalf("lost %q: %s", want, text)
				}
			}
			if strings.Contains(text, " 字节") || strings.Contains(text, "--> ") {
				t.Fatal("invented unit or ordered edge")
			}
			if table.View == types.RuntimeMeasurementSummary {
				for _, want := range []string{"FD_Open_Event", "THREAD_Create_Event", "单位未在该载体声明"} {
					if !strings.Contains(text, want) {
						t.Fatalf("lost resource field %q", want)
					}
				}
			}
		}
		if table.View == types.RuntimeMeasurementMembers {
			var got []string
			for _, row := range table.Rows {
				got = append(got, row[0]+":"+row[4])
			}
			// Native fixture row ids, not source-event IDs, are authoritative.
			var want []string
			for _, e := range p.Events {
				for _, f := range e.Frames {
					want = append(want, fmt.Sprint(e.RowID)+":"+resourceFactScalar(f.Depth))
				}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("depth/row identity changed: %v != %v", got, want)
			}
		}
	}
	longSymbol := strings.Repeat("<", 4096) // valid native text, large JSON/render encoding
	_, _, bounded := resourceMeasurementContextHMC249(t, false, 101, func(db *sql.DB) {
		if _, err := db.Exec("UPDATE data_dict SET data=? WHERE id=100", longSymbol); err != nil {
			t.Fatal(err)
		}
	})
	if len(resourceNativeTablesHMC249(bounded)) != 2 {
		t.Fatal("bounded publication absent")
	}
	omitted := false
	for _, table := range resourceNativeTablesHMC249(bounded) {
		if table.HasCompleteNativeDisplayPopulation() || table.MemberSet != nil {
			t.Fatal("bounded rows acquired completeness")
		}
		for _, row := range table.Rows {
			for _, cell := range row {
				if strings.Contains(cell, "展示省略帧=1") || strings.Contains(cell, "展示省略帧=2") || strings.Contains(cell, "展示省略帧=3") {
					omitted = true
				}
				if strings.Contains(cell, "<<<<") && cell != longSymbol {
					t.Fatal("native symbol clipped instead of whole row omitted")
				}
			}
		}
	}
	if !omitted {
		t.Fatal("byte-bound fixture did not exercise a whole-frame omission")
	}
	for _, state := range []string{"contiguous_observed", "incomplete", "ambiguous", "unavailable", "invalid", "future_state"} {
		got := resourceFactState(state)
		if got == "" || state == "future_state" && got != state {
			t.Fatal("state lost or inferred")
		}
	}
}

func TestHMC249ResourceExactNameAndInclusiveWindow(t *testing.T) {
	bus, _, _ := resourceMeasurementContextHMC249(t, false, 101, func(db *sql.DB) {
		if _, err := db.Exec("UPDATE thread SET name='Owner' WHERE tid=101"); err != nil {
			t.Fatal(err)
		}
	})
	for _, mode := range []string{"same_name", "different_case", "inclusive", "outside_end"} {
		t.Run(mode, func(t *testing.T) {
			args := map[string]any{"source": "path", "path": filepath.Join(bus.RepoRoot, "capture.data"), "view": "resource_stack", "thread": "Owner"}
			if mode == "same_name" || mode == "different_case" {
				args["time_start"], args["time_end"] = 10, 10.05
			}
			raw, _ := json.Marshal(args)
			result, err := (&TraceQuery{}).Execute(bus, raw)
			if err != nil || !result.Success {
				t.Fatalf("query: %v %s", err, result.Summary)
			}
			rm := nativeFactDisplayRequest(types.RequestedAnswerDimensionObservedValue)
			name := "Owner"
			if mode == "different_case" {
				name = "owner"
			}
			rm.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, Thread: name, Source: "user_explicit"}}
			if mode == "outside_end" {
				start, end := 10.0, 10.05
				rm.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "[10,10.05)"}
			}
			bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm}
			bus.Mutable.SetRequestModel(rm)
			bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
			tables := resourceNativeTablesHMC249(types.BuildAnswerSemanticViewForBusContext(bus))
			want := mode == "same_name" || mode == "inclusive"
			if (len(tables) == 2) != want {
				t.Fatalf("%s choices=%d", mode, len(tables))
			}
			if mode == "inclusive" {
				for _, table := range tables {
					if !strings.Contains(strings.Join(table.Notes, " "), "10.050000000]") {
						t.Fatal("inclusive source endpoint relabeled")
					}
				}
			}
		})
	}
}

func TestHMC249ResourceMixedSchema(t *testing.T) {
	_, _, view := resourceMeasurementContextHMC249(t, false, 101)
	resource := resourceNativeTablesHMC249(view)[0]
	_, sampleView, sampling := samplingMeasurementContextHMC243(t)
	eventBus, event := eventDisplayPublicContext236(t)
	legacy := runtimeMeasurementTestView().RuntimeMeasurementContract.Choices()[0]
	view.RuntimeMeasurementContract.Tables = append(view.RuntimeMeasurementContract.Tables, sampleView.RuntimeMeasurementContract.Choices()...)
	view.RuntimeMeasurementContract.Tables = append(view.RuntimeMeasurementContract.Tables, types.BuildAnswerSemanticViewForBusContext(eventBus).RuntimeMeasurementContract.Choices()...)
	view.RuntimeMeasurementContract.Tables = append(view.RuntimeMeasurementContract.Tables, legacy)
	view.OptionalBlocks = append(view.OptionalBlocks, types.BlockRequirement{Kind: types.BlockDiagram})
	view.DiagramPlan = &types.DiagramFacetGraph{Kind: types.DiagramFlow}
	for _, patch := range []bool{false, true} {
		schema, key := BuildAnswerDocumentParametersFor(view), "blocks"
		if patch {
			schema, key = BuildAnswerDocumentPatchParametersFor(view), "replace_blocks"
		}
		for _, teaching := range []string{"event_selection chooses row_ids", "Published native sampling facts", "Published native resource facts"} {
			if !strings.Contains(string(schema), teaching) {
				t.Fatalf("lost teaching %q", teaching)
			}
		}
		for _, family := range []string{"resource", "sampling", "event", "legacy"} {
			for _, kind := range []string{"summary", "table", "diagram"} {
				pair := map[string]any{"observation_id": resource.ObservationID, "view": resource.View}
				switch family {
				case "sampling":
					pair = map[string]any{"observation_id": sampling.ObservationID, "view": sampling.View}
				case "event":
					pair = map[string]any{"observation_id": event.ID, "view": "members", "event_selection": map[string]any{"field_keys": []string{"jank.appid"}}}
				case "legacy":
					pair = map[string]any{"observation_id": legacy.ObservationID, "view": legacy.View}
				}
				block := map[string]any{"id": "facts", "kind": kind, "runtime_measurement": pair}
				raw, _ := json.Marshal(map[string]any{key: []any{block}})
				want := family != "legacy" || kind == "table"
				if err := toolparam.Validate(raw, schema); (err == nil) != want {
					t.Fatalf("%s/%s patch=%t: %v", family, kind, patch, err)
				}
				b, _ := json.Marshal(block)
				var wire emitAnswerBlockV2
				_ = json.Unmarshal(b, &wire)
				normal, err := NormalizeEmitAnswerBlock(wire, "blocks[0]", view.RuntimeMeasurementContract)
				if (err == nil && runtimeMeasurementBindingValid(normal, view)) != want {
					t.Fatalf("binding %s/%s: %v", family, kind, err)
				}
				if family == "resource" {
					block["event_selection"] = map[string]any{"row_ids": []string{"1"}}
					raw, _ = json.Marshal(map[string]any{key: []any{block}})
					if toolparam.Validate(raw, schema) == nil {
						t.Fatal("resource acquired event-selection shape")
					}
				}
			}
		}
	}
}
