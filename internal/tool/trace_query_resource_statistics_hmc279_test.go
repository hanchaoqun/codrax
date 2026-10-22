package tool

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func resourceDistributionHMC279(t *testing.T, view *types.AnswerSemanticView) types.RuntimeMeasurementTable {
	t.Helper()
	for _, table := range view.RuntimeMeasurementContract.Choices() {
		if table.HasNativeResourceFacts() && table.View == types.RuntimeMeasurementDistribution {
			return table
		}
	}
	t.Fatal("source-qualified resource event distribution was not published")
	return types.RuntimeMeasurementTable{}
}

func resourceRecordHMC279(t *testing.T, result types.ToolResult) types.ObservationRecord {
	t.Helper()
	for _, r := range result.Observations {
		if r.Predicate == TraceResourceStackPredicate {
			return r
		}
	}
	t.Fatal("no resource observation")
	return types.ObservationRecord{}
}

func unpublishResourceHMC279(r types.ObservationRecord) types.ObservationRecord {
	notes := []string{}
	for _, n := range r.RichNotes {
		if !strings.HasPrefix(n, types.TraceNoteKeyRuntimeMeasurement+"=") {
			notes = append(notes, n)
		}
	}
	r.RichNotes = notes
	return r
}

func TestHMC279ResourceStatisticsFullPopulationAndUnknown(t *testing.T) {
	_, result, view := resourceMeasurementContextHMC249(t, false, 101, func(db *sql.DB) {
		for i := 0; i < 50; i++ {
			if _, err := db.Exec("INSERT INTO native_hook SELECT ?,start_ts+?,end_ts,event_type,all_heap_size,itid,ipid,heap_size,callchain_id,addr,sub_type_id FROM native_hook WHERE rowid=1", 100+i, i+1); err != nil {
				t.Fatal(err)
			}
		}
		for i, size := range []string{"0", "NULL", "-7"} {
			if _, err := db.Exec("INSERT INTO native_hook SELECT ?,start_ts+1000+?,end_ts,event_type,all_heap_size,itid,ipid,"+size+",callchain_id,addr,sub_type_id FROM native_hook WHERE rowid=1", 200+i, i); err != nil {
				t.Fatal(err)
			}
		}
	})
	table := resourceDistributionHMC279(t, view)
	p, ok := DecodeTraceResourceStack(resourceRecordHMC279(t, result))
	if !ok || p.MatchedEvents != 56 || len(p.Events) >= 56 {
		t.Fatalf("fixture not preview-truncated: %+v", p)
	}
	if len(table.Rows) != 2 {
		t.Fatalf("groups=%d", len(table.Rows))
	}
	for _, row := range table.Rows {
		if strings.HasPrefix(row[2], "AllocEvent") {
			if row[5] != "55" || row[7] != "26368" || row[8] != "bytes" || row[6] != "已知非负=53；空值=1；未提供=0；无效=0；负值=1" {
				t.Fatalf("preview replaced independent denominator: %v", row)
			}
		}
	}
	_, _, zero := resourceMeasurementContextHMC249(t, false, 101, func(db *sql.DB) {
		if _, err := db.Exec("UPDATE native_hook SET heap_size=CASE event_type WHEN 'AllocEvent' THEN 0 ELSE NULL END"); err != nil {
			t.Fatal(err)
		}
	})
	for _, row := range resourceDistributionHMC279(t, zero).Rows {
		if strings.HasPrefix(row[2], "AllocEvent") && row[7] != "0" {
			t.Fatal("known zero lost", row)
		}
		if strings.HasPrefix(row[2], "FreeEvent") && row[7] != "未知（没有已知非负大小）" {
			t.Fatal("unknown replaced by zero", row)
		}
	}
}

func TestHMC279ResourceBroadTargetlessOnlyAndLegacy(t *testing.T) {
	bus, result, view := resourceMeasurementContextHMC249(t, false, 0)
	table := resourceDistributionHMC279(t, view)
	if len(table.Rows) != 3 || !strings.Contains(table.Notes[1], "原查询全集事件=4") {
		t.Fatalf("broad population %v", table)
	}
	for _, c := range view.RuntimeMeasurementContract.Choices() {
		if c.HasNativeResourceFacts() && (c.MemberSet != nil || c.DisplayPopulation != nil || strings.Contains(c.Label, "请求线程名 ")) {
			t.Fatal("broad facts gained target or population authority", c)
		}
	}
	for _, kind := range []types.RuntimeTargetKind{types.RuntimeTargetKindThread, types.RuntimeTargetKindProcess, types.RuntimeTargetKind("other")} {
		bus.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{Kind: kind, PID: 101, Source: "user_explicit"}}
		for _, c := range types.BuildAnswerSemanticViewForBusContext(bus).RuntimeMeasurementContract.Choices() {
			if c.HasNativeResourceFacts() {
				t.Fatal("broad query acquired explicit target scope", kind)
			}
		}
	}
	input := types.ObservationLedgerInputFromBusContext(bus, 0)
	input.RequestModel = nil // Keep the actual current-source consumer/witness.
	for _, c := range types.BuildRuntimeMeasurementContract(input).Choices() {
		if c.HasNativeResourceFacts() {
			t.Fatal("nil request admitted broad native publication despite a current source")
		}
	}
	bus.AnalysisIR = nil
	if noRequest := types.BuildAnswerSemanticViewForBusContext(bus); noRequest != nil {
		for _, c := range noRequest.RuntimeMeasurementContract.Choices() {
			if c.HasNativeResourceFacts() {
				t.Fatal("nil request acquired broad facts")
			}
		}
	}
	r := resourceRecordHMC279(t, result)
	broadPublication, _ := types.DecodeRuntimeMeasurementPublication(r)
	p, ok := DecodeTraceResourceStack(r)
	if !ok {
		t.Fatal("missing payload")
	}
	p.EventStatistics = nil
	r = unpublishResourceHMC279(r)
	legacy := traceQueryResourceMeasurement(r, p)
	if _, ok := types.DecodeRuntimeMeasurementPublication(legacy); ok {
		t.Fatal("legacy broad acquired new publication")
	}
	// Exact legacy producers retain their original two views and no fake stats.
	_, exactResult, _ := resourceMeasurementContextHMC249(t, false, 101)
	exact := unpublishResourceHMC279(resourceRecordHMC279(t, exactResult))
	ep, _ := DecodeTraceResourceStack(exact)
	ep.EventStatistics = nil
	publication, ok := types.DecodeRuntimeMeasurementPublication(traceQueryResourceMeasurement(exact, ep))
	if !ok || len(publication.Tables) != 2 {
		t.Fatal("legacy exact pair changed")
	}
	for _, mutate := range []func(*types.ObservationRecord){
		func(r *types.ObservationRecord) { r.SourceRef.QueryTargetScope = "all" },
		func(r *types.ObservationRecord) { r.SourceRef.QueryTargetScope = "process" },
		func(r *types.ObservationRecord) { r.SourceRef.QueryTargetPID = -1 },
		func(r *types.ObservationRecord) { r.SourceRef.QueryTargetThread = " " },
	} {
		changed := r
		mutate(&changed)
		if _, ok := types.PublishRuntimeResourceMeasurement(changed, broadPublication.Tables, false); ok {
			t.Fatal("malformed broad accepted")
		}
	}
}

func TestHMC279ResourceStatisticsBudgetPreservesOldFacts(t *testing.T) {
	_, result, _ := resourceMeasurementContextHMC249(t, false, 101)
	r := resourceRecordHMC279(t, result)
	p, _ := DecodeTraceResourceStack(r)
	pub, _ := types.DecodeRuntimeMeasurementPublication(r)
	base := append([]types.RuntimeMeasurementTable(nil), pub.Tables[:2]...)
	// Erase private table seals by JSON, then construct a native producer
	// boundary fixture whose valid old pair nearly fills the old 64KiB limit.
	raw, _ := json.Marshal(base)
	_ = json.Unmarshal(raw, &base)
	r = unpublishResourceHMC279(r)
	base[0].Notes = append(base[0].Notes, "")
	measure, _ := json.Marshal(types.RuntimeMeasurementPublication{Version: 1, ObservationID: r.ID, Source: r.SourceRef, Tables: base})
	base[0].Notes[len(base[0].Notes)-1] = strings.Repeat("x", (64<<10)-len(measure)-1)
	old, ok := types.PublishRuntimeResourceMeasurement(r, base, p.Window.EndInclusive)
	if !ok {
		t.Fatal("old pair fixture does not fit")
	}
	before, _ := json.Marshal(p)
	got := publishResourceEventStatistics(r, p, base)
	if !types.RuntimeResourceStatisticsDisplayOmitted(got) {
		t.Fatal("budget fallback silently lost statistics disposition")
	}
	gotPub, ok := types.DecodeRuntimeMeasurementPublication(got)
	oldPub, _ := types.DecodeRuntimeMeasurementPublication(old)
	if !ok || !reflect.DeepEqual(gotPub.Tables, oldPub.Tables) {
		t.Fatal("new statistics evicted or changed old facts")
	}
	after, _ := json.Marshal(p)
	if string(before) != string(after) {
		t.Fatal("budget mutated original payload")
	}
	var restored types.ObservationRecord
	encoded, _ := json.Marshal(got)
	_ = json.Unmarshal(encoded, &restored)
	if types.RuntimeResourceStatisticsDisplayOmitted(restored) {
		t.Fatal("JSON recreated private disposition")
	}
	// Many long whole groups are bounded independently; census is not reset.
	full := hmc278ManyResourceGroups(t)
	_, many, bounded := nativeStackPublicQuery(t, full.SourcePath, map[string]any{"pid": 2, "time_start": 1, "time_end": 2})
	fullBytes, _ := os.ReadFile(full.SourcePath)
	statistics, ok := types.DecodeRuntimeMeasurementPublication(many)
	if !ok || len(statistics.Tables) != 3 {
		t.Fatal("bounded statistics unavailable")
	}
	group := statistics.Tables[2]
	data, _ := json.Marshal(group)
	if len(data) > resourceStatisticsTableByteBudget || len(group.Rows) >= 80 || !strings.Contains(group.Notes[1], "原查询全集事件=80") || !strings.Contains(group.Notes[1], "分组展示完整=false") {
		t.Fatal("whole-group budget/census lost", len(data), group.Notes)
	}
	for _, row := range group.Rows {
		if row[4] != strings.Repeat("完整名称", 250) {
			t.Fatal("subtype field truncated", row[4])
		}
	}
	if !tracequery.ValidResourceStack(bounded) || bounded.EventStatistics.EventCount != 80 {
		t.Fatal("handoff population changed")
	}
	currentBytes, _ := os.ReadFile(full.SourcePath)
	if string(fullBytes) != string(currentBytes) {
		t.Fatal("read modified original")
	}
	// Independently isolate the third-table budget with the complete valid
	// scan, before the already-tested 24KiB handoff chooses its own omission.
	base = statistics.Tables[:2]
	fullPublication := publishResourceEventStatistics(unpublishResourceHMC279(many), full, base)
	fullTables, ok := types.DecodeRuntimeMeasurementPublication(fullPublication)
	if !ok || len(fullTables.Tables) != 3 {
		t.Fatal("complete producer projection failed")
	}
	whole := fullTables.Tables[2]
	if len(whole.Rows) == 0 || len(whole.Rows) >= 80 || !strings.Contains(whole.Notes[1], "交接省略组=0") || !strings.Contains(whole.Notes[1], "本表另省略组=") {
		t.Fatal("independent table budget not exercised", whole.Notes)
	}
	for _, row := range whole.Rows {
		if row[4] != strings.Repeat("完整名称", 250) {
			t.Fatal("table split a group field")
		}
	}
}

func TestHMC279ResourceStatisticsBroadBudgetDoesNotInventLegacyAuthority(t *testing.T) {
	_, result, _ := resourceMeasurementContextHMC249(t, false, 0)
	r := resourceRecordHMC279(t, result)
	p, _ := DecodeTraceResourceStack(r)
	pub, _ := types.DecodeRuntimeMeasurementPublication(r)
	base := pub.Tables[:2]
	raw, _ := json.Marshal(base)
	_ = json.Unmarshal(raw, &base)
	base[0].Notes = append(base[0].Notes, strings.Repeat("x", 64<<10))
	r = unpublishResourceHMC279(r)
	got := publishResourceEventStatistics(r, p, base)
	if _, ok := types.DecodeRuntimeMeasurementPublication(got); ok || types.RuntimeResourceStatisticsDisplayOmitted(got) {
		t.Fatal("broad budget fallback minted old exact table/state authority")
	}
	retained, ok := DecodeTraceResourceStack(got)
	if !ok || !reflect.DeepEqual(retained, p) || retained.EventStatistics.EventCount != 4 || retained.EventStatistics.TotalGroups != 3 {
		t.Fatal("unpublished broad table became unknown/zero or lost full payload navigation")
	}
}

func TestHMC279ResourceStatisticsBroadFreshnessAndMutation(t *testing.T) {
	bus, result, view := resourceMeasurementContextHMC249(t, false, 0)
	table := resourceDistributionHMC279(t, view)
	r := resourceRecordHMC279(t, result)
	for name, mutate := range map[string]func(*types.ObservationRecord){
		"query":  func(r *types.ObservationRecord) { r.SourceRef.QueryScopeID += "other" },
		"window": func(r *types.ObservationRecord) { r.SourceRef.QueryWindowEndTs++ },
		"thread": func(r *types.ObservationRecord) { r.SourceRef.QueryTargetPID = 101 },
		"json": func(r *types.ObservationRecord) {
			raw, _ := json.Marshal(r)
			var restored types.ObservationRecord
			_ = json.Unmarshal(raw, &restored)
			*r = restored
		},
	} {
		changed := r
		mutate(&changed)
		if _, ok := types.DecodeRuntimeMeasurementPublication(changed); ok {
			t.Fatal(name, "altered resource distribution admitted")
		}
	}
	if err := os.WriteFile(r.SourceRef.Path, []byte(fmt.Sprint("changed source")), 0600); err != nil {
		t.Fatal(err)
	}
	fresh := types.BuildAnswerSemanticViewForBusContext(bus)
	ref := &types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View}
	if types.BindRuntimeMeasurementReceipt(ref, fresh.RuntimeMeasurementContract) {
		t.Fatal("stale source retained statistics authority")
	}
}

func TestHMC279ResourceStatisticsSubtypesUnitsAndTrueZero(t *testing.T) {
	bus, _, view := resourceMeasurementContextHMC249(t, false, 0, func(db *sql.DB) {
		for _, statement := range []string{
			`INSERT INTO data_dict VALUES (0,''),(8,'缓存|纹理'),(9,NULL),(10,'first'),(10,'second')`,
			`UPDATE native_hook SET sub_type_id=CASE rowid WHEN 1 THEN 0 WHEN 2 THEN 8 WHEN 3 THEN 9 WHEN 4 THEN 10 END`,
			`UPDATE native_hook SET event_type='FD_Open_Event' WHERE rowid=4`,
			`INSERT INTO native_hook VALUES (6,10020000000,NULL,'THREAD_Create_Event',0,2,2,0,NULL,0,99)`,
			`INSERT INTO native_hook VALUES (7,10030000000,NULL,'MmapEvent',0,1,1,NULL,NULL,0,NULL)`,
			`CREATE TABLE native_hook_statistic (ipid,type,apply_count,release_count,apply_size,release_size)`,
			`INSERT INTO native_hook_statistic VALUES (1,0,999999,888888,777777,666666)`,
		} {
			if _, err := db.Exec(statement); err != nil {
				t.Fatal(err)
			}
		}
	})
	table := resourceDistributionHMC279(t, view)
	if len(table.Rows) != 6 {
		t.Fatal("owner/action/subtype collapsed", table.Rows)
	}
	seen := map[string]bool{}
	for _, row := range table.Rows {
		key := row[2] + ":" + row[3]
		seen[key] = true
		switch key {
		case "AllocEvent / allocate:0":
			if row[4] != "空文本" || row[8] != "bytes" {
				t.Fatal("zero subtype/empty name lost", row)
			}
		case "FreeEvent / release:8":
			if row[4] != "缓存|纹理" || row[7] != "512" {
				t.Fatal("exact release or source text lost", row)
			}
		case "AllocEvent / allocate:9":
			if row[4] != "未知（空值）" {
				t.Fatal("NULL subtype guessed", row)
			}
		case "FD_Open_Event / open:10":
			if row[4] != "未知（有歧义）" || row[8] != "未声明（unspecified）" {
				t.Fatal("ambiguous subtype/unit guessed", row)
			}
		case "THREAD_Create_Event / create:99":
			if row[4] != "未知（未提供）" || row[7] != "0" || row[8] != "未声明（unspecified）" {
				t.Fatal("unknown subtype or thread count inferred bytes", row)
			}
		case "MmapEvent / allocate:未知（空值）":
			if row[7] != "未知（没有已知非负大小）" || row[8] != "bytes" {
				t.Fatal("mapping NULL replaced with zero", row)
			}
		default:
			t.Fatal("unexpected group", row)
		}
	}
	if len(seen) != 6 {
		t.Fatal("duplicate group rows")
	}
	for _, kind := range []types.AnswerBlockKind{types.BlockSummary, types.BlockTable} {
		ref := &types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View}
		if !types.BindRuntimeMeasurementReceipt(ref, view.RuntimeMeasurementContract) {
			t.Fatal("bind")
		}
		out := render.RenderAnswerDocument(&types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "statistics", Kind: kind, RuntimeMeasurement: ref}}}, "zh")
		if strings.Contains(out, "777777") || strings.Contains(out, "999999") {
			t.Fatal("unproven statistic report was summed")
		}
	}
	// A real executed empty query retains its selector and zero population;
	// no fabricated group row or unknown-size zero is introduced.
	query := map[string]any{"source": "path", "path": bus.RepoRoot + "/capture.data", "view": "resource_stack", "time_start": 10.041, "time_end": 10.042}
	raw, _ := json.Marshal(query)
	result, err := (&TraceQuery{}).Execute(bus, raw)
	if err != nil || !result.Success {
		t.Fatal(err, result.Summary)
	}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	zero := resourceDistributionHMC279(t, types.BuildAnswerSemanticViewForBusContext(bus))
	if len(zero.Rows) != 0 || !strings.Contains(zero.Notes[1], "原查询全集事件=0") || !strings.Contains(zero.Notes[1], "独立分组=0") || !strings.Contains(zero.Notes[1], "分组展示完整=true") {
		t.Fatal("true empty query invented/omitted groups", zero)
	}
}

func TestHMC279ResourceStatisticsPublicSurfaces(t *testing.T) {
	bus, result, view := resourceMeasurementContextHMC249(t, false, 101)
	table := resourceDistributionHMC279(t, view)
	if len(table.Rows) != 2 || table.MemberSet != nil || table.DisplayPopulation != nil || table.DefaultPresentation {
		t.Fatalf("independent event groups/authority: %+v", table)
	}
	before, _ := json.Marshal(result)
	view.OptionalBlocks = append(view.OptionalBlocks, types.BlockRequirement{Kind: types.BlockDiagram})
	view.DiagramPlan = &types.DiagramFacetGraph{Kind: types.DiagramFlow}
	for _, kind := range []string{"summary", "table", "diagram"} {
		t.Run(kind, func(t *testing.T) {
			block := map[string]any{"id": "statistics", "kind": kind, "runtime_measurement": map[string]any{"observation_id": table.ObservationID, "view": "distribution"}}
			blocks := []any{block}
			if kind != "summary" {
				blocks = append([]any{map[string]any{"id": "lead", "kind": "summary", "text": "以下为资源事件统计。"}}, blocks...)
			}
			args := map[string]any{"blocks": blocks}
			raw, _ := json.Marshal(args)
			if err := toolparam.Validate(raw, BuildAnswerDocumentParametersFor(view)); err != nil {
				t.Fatal(err)
			}
			if out := b1659bExecuteAnswer(t, bus, args, false); !out.Success {
				t.Fatal(out.Summary)
			}
			doc := bus.Mutable.AnswerDocumentV2()
			visible := html.UnescapeString(render.RenderAnswerDocument(doc, "zh"))
			for _, want := range []string{"AllocEvent", "FreeEvent", "768", "512", "bytes", "事件流量", "不是净存量", "线程ID=101", "gallery-main"} {
				if !strings.Contains(visible, want) {
					t.Errorf("%s missing %q: %s", kind, want, visible)
				}
			}
			if strings.Contains(visible, "-->") {
				t.Fatal("event groups gained an ordered/causal edge")
			}
			wire, _ := json.Marshal(doc)
			if strings.Contains(string(wire), "768") {
				t.Fatal("native cells copied into model document")
			}
			var restored types.AnswerDocumentV2
			if json.Unmarshal(wire, &restored) != nil || !types.RebindRuntimeAnswerReceipts(&restored, view) {
				t.Fatal("same-source statistics cannot rebind")
			}
			if !reflect.DeepEqual(table.Rows, resourceDistributionHMC279(t, view).Rows) {
				t.Fatal("presentation changed source groups")
			}
		})
	}
	after, _ := json.Marshal(result)
	if string(before) != string(after) {
		t.Fatal("native publication mutated source result")
	}
}
