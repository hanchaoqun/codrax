package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc346QueryContext(t *testing.T, path string, pid int, start, end float64) (*types.AgentContext, []types.ToolResult) {
	t.Helper()
	rm := types.RequestModel{Language: "zh", Intent: types.IntentExplain, PerfTrace: &types.PerfBundle{}, RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration}}, RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end}}
	ctx := &types.AgentContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize, Mutable: types.NewMutableState("说明窗口内目标线程各状态的时间分布"), AnalysisIR: &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "zh"}}}
	ctx.Mutable.SetRequestModel(rm)
	ctx.RuntimeArtifactPreflight.Artifacts = []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: path, Carrier: "path"}}
	var results []types.ToolResult
	for _, view := range []string{"window_stats", "thread_timeline"} {
		args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": view, "pid": pid, "time_start": start, "time_end": end})
		r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
		if err != nil || !r.Success {
			t.Fatalf("actual native %s: %v / %s", view, err, r.Summary)
		}
		results = append(results, types.AttachToolHandoffCarrier(r))
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results, HandoffCarriers: types.ToolHandoffCarriersFromTurnAInputs(results, nil, nil)})
	return ctxbuilder.BuildAgentContext(types.ToolBusContext(ctx, types.AgentFinalizer), types.AgentFinalizer, types.StageFinalize), results
}

func hmc346LargeTrace(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "capture.ftrace")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	trace := "# tracer: nop\napp-100 (100) [001] .... 1.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52\napp-100 (100) [001] .... 2.004000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120\nirq-2 (2) [002] .... 2.010000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001\napp-100 (100) [001] .... 2.015000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52\napp-100 (100) [001] .... 2.021000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120\n"
	if _, err := f.WriteString(trace); err != nil {
		t.Fatal(err)
	}
	// Exercise the ordinary 64 MiB scoped-index policy without changing its
	// threshold or inventing additional scheduler events.
	padding := []byte("# " + strings.Repeat("x", 4093) + "\n")
	for i := 0; i < (65<<20)/len(padding); i++ {
		if _, err := f.Write(padding); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func hmc346TimelineTables(t *testing.T, ctx *types.AgentContext) []types.RuntimeMeasurementTable {
	t.Helper()
	var tables []types.RuntimeMeasurementTable
	for _, table := range types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices() {
		if table.View == types.RuntimeMeasurementTimeline && strings.HasSuffix(table.ObservationID, "#thread_timeline_observation") {
			tables = append(tables, table)
		}
	}
	if len(tables) != 2 || len(tables[0].Rows) == 0 || !reflect.DeepEqual(tables[0].Rows, tables[1].Rows) || reflect.DeepEqual(tables[0].Notes, tables[1].Notes) {
		t.Fatalf("need actual equal-data/different-coverage publications, tables=%d", len(tables))
	}
	return tables
}

func hmc346AssertIndependentContent(t *testing.T, ctx *types.AgentContext, tables []types.RuntimeMeasurementTable, text string, receipt *answerDocNativePresentationReceipt) {
	t.Helper()
	data := hmc302Dictionary(t, text, "native_measurement_data")
	bodies := hmc302Dictionary(t, text, "native_measurement_content")
	notes := hmc302Dictionary(t, text, "native_measurement_note")
	queries := hmc302Dictionary(t, text, "native_measurement_query")
	dataRefs, contentRefs, queryRefs := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, table := range tables {
		found := false
		for _, line := range strings.Split(text, "\n") {
			if !strings.Contains(line, fmt.Sprintf("observation_id=%q view=%q", table.ObservationID, table.View)) || !strings.Contains(line, "; content_ref=") {
				continue
			}
			found = true
			contentRef := hmc302NavigationField(line, "content_ref")
			queryRef := hmc302NavigationField(line, "query_receipt")
			var body struct {
				DataRef  string   `json:"data_ref"`
				NoteRefs []string `json:"note_refs"`
			}
			if err := json.Unmarshal(bodies[contentRef], &body); err != nil || body.DataRef == "" {
				t.Fatalf("native identical rows lack separate data/coverage handoff: %s %v", line, err)
			}
			var preview measurementHandoffPreview
			if err := json.Unmarshal(data[body.DataRef], &preview); err != nil || !reflect.DeepEqual(preview, runtimeMeasurementHandoffPreview(table, len(table.Rows))) {
				t.Fatal("shared data lost full small table rows, indices, unit or order")
			}
			var actualNotes []string
			for _, ref := range body.NoteRefs {
				var value string
				if err := json.Unmarshal(notes[ref], &value); err != nil {
					t.Fatal(err)
				}
				actualNotes = append(actualNotes, value)
			}
			var query types.ObservationSourceRef
			validQuery := json.Unmarshal(queries[queryRef], &query) == nil
			originalQuery := false
			for _, r := range types.ObservationLedgerInputFromAgentContext(ctx, 0).ToolResults {
				for _, record := range r.Observations {
					originalQuery = originalQuery || record.ID == table.ObservationID && reflect.DeepEqual(query, record.SourceRef)
				}
			}
			if !reflect.DeepEqual(actualNotes, table.Notes) || !validQuery || !originalQuery {
				t.Fatal("one query borrowed the other's complete coverage or lost its receipt")
			}
			dataRefs[body.DataRef], contentRefs[contentRef], queryRefs[queryRef] = true, true, true
		}
		if !found {
			t.Errorf("independent coverage was not published for %s", table.ObservationID)
		}
		if detail := receipt.measurementViewPresentation(table); !detail.rowsComplete || !detail.notesComplete {
			t.Error("complete written rows/own notes not reflected by the selector receipt")
		}
	}
	if len(dataRefs) != 1 || len(contentRefs) != 2 || len(queryRefs) != 2 {
		t.Fatalf("want one data body and two separate content/receipt pairs; got %d/%d/%d", len(dataRefs), len(contentRefs), len(queryRefs))
	}
}

func TestHMC346PublicEqualRowsKeepDistinctCoverage(t *testing.T) {
	ctx, results := hmc346QueryContext(t, hmc346LargeTrace(t), 100, 2, 2.02)
	tables := hmc346TimelineTables(t, ctx)
	before, _ := json.Marshal(results)
	projection := hmc218CausalProjection(t, ctx)
	p := &answerDocNativePresentationReceipt{}
	text := renderAnswerDocRuntimeMeasurementChoices(ctx, p)
	hmc346AssertIndependentContent(t, ctx, tables, text, p)
	_, actual := finalizerReaderFieldMessages(t, ctx)
	if data := hmc302Dictionary(t, actual, "native_measurement_data"); len(data) != 1 {
		t.Fatal("actual finalizer adapter did not receive the shared native data")
	}
	blocks := []any{map[string]any{"id": "summary", "kind": "summary", "text": "以下各查询保留各自线程和时间窗；同值展示不合并查询。"}}
	for i, table := range tables {
		blocks = append(blocks, map[string]any{"id": fmt.Sprintf("timeline-%d", i), "kind": "table", "runtime_measurement": map[string]any{"observation_id": table.ObservationID, "view": table.View}})
	}
	args, _ := json.Marshal(map[string]any{"blocks": blocks})
	out, err := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), args)
	if err != nil || !out.Success {
		t.Fatalf("original native selector binding: %v / %s", err, out.Summary)
	}
	for i, table := range tables {
		bound := ctx.Mutable.AnswerDocumentV2().Blocks[i+1].RuntimeMeasurement
		if bound == nil || !bound.IsBound() || !reflect.DeepEqual(bound.BoundTable.Rows, table.Rows) || !reflect.DeepEqual(bound.BoundTable.Notes, table.Notes) {
			t.Fatal("display-data reference replaced the original full table/coverage binding")
		}
	}
	after, _ := json.Marshal(results)
	if !bytes.Equal(before, after) || !bytes.Equal(projection, hmc218CausalProjection(t, ctx)) {
		t.Fatal("display sharing mutated native facts or causal projection")
	}
}

func TestHMC346DataIdentityKeepsEveryNonNoteBoundary(t *testing.T) {
	ctx, results := hmc302MeasurementContext(t)
	input := types.ObservationLedgerInputFromAgentContext(ctx, 0)
	for _, record := range results[0].Observations {
		p, valid := types.DecodeRuntimeMeasurementPublication(record)
		if !valid {
			continue
		}
		for _, table := range p.Tables {
			if table.View != types.RuntimeMeasurementMembers || len(table.Rows) < 2 {
				continue
			}
			source := types.RuntimeObservationPresentationSourceKey(input, results[0], record)
			key := runtimeMeasurementDataKey(source, record.Predicate, p, table)
			if key == "" {
				continue
			}
			for _, mode := range []string{"own_notes", "units", "unseen_tail", "row_order", "default", "population", "window", "target", "clock", "generation"} {
				t.Run(mode, func(t *testing.T) {
					copy, pub, physical := table.Clone(), p, source
					switch mode {
					case "own_notes":
						copy.Notes = append(copy.Notes, "another query's exact coverage")
					case "units":
						copy.Columns[0] += " ms"
					case "unseen_tail":
						copy.Rows[len(copy.Rows)-1][0] += " changed"
					case "row_order":
						copy.Rows[0], copy.Rows[1] = copy.Rows[1], copy.Rows[0]
					case "default":
						copy.DefaultPresentation = !copy.DefaultPresentation
					case "population":
						copy.DisplayPopulation = &types.RuntimeMeasurementDisplayPopulation{QueryScopeID: p.Source.QueryScopeID, MatchedRows: len(copy.Rows), OmittedRows: 1}
					case "window":
						pub.Source.QueryWindowEndTs++
					case "target":
						pub.Source.QueryTargetPID++
					case "clock":
						pub.Source.ClockAlignment += " other clock"
					case "generation":
						physical += " another generation"
					}
					got := runtimeMeasurementDataKey(physical, record.Predicate, pub, copy)
					if (got == key) != (mode == "own_notes") {
						t.Fatal("data storage equivalence crossed a non-note boundary")
					}
					if runtimeMeasurementContentKey(physical, record.Predicate, pub, copy) == runtimeMeasurementContentKey(source, record.Predicate, p, table) {
						t.Fatal("full content identity was weakened")
					}
				})
			}
			return
		}
	}
	t.Fatal("actual native member table absent")
}

func TestHMC346OwnCoverageCannotBorrowSharedDataBudget(t *testing.T) {
	ctx, _ := hmc346QueryContext(t, hmc346LargeTrace(t), 100, 2, 2.02)
	tables := hmc346TimelineTables(t, ctx)
	keys := runtimeMeasurementPreviewIdentities(ctx, tables, &answerDocNativePresentationReceipt{})
	w := runtimeMeasurementContentWitnesses(ctx, tables, keys)
	if len(w) != 2 || w[0].dataKey == "" || w[0].dataKey != w[1].dataKey || w[0].key == w[1].key {
		t.Fatal("public fixture lacks separately qualified data/coverage identities")
	}
	d, notes, p := runtimeMeasurementContentDictionary{}, runtimeMeasurementNoteNavigation{}, &answerDocNativePresentationReceipt{}
	var b strings.Builder
	if shown, rows, charge := d.write(&b, w[0], tables[0], 1, "", "", notes, p); !shown || rows != 1 || charge != 1 {
		t.Fatal("first partial data publication failed")
	}
	noteCount, contentCount := len(notes), len(d.bodies)
	p.presentationLimit = p.presentationBytes
	if shown, _, _ := d.write(&b, w[1], tables[1], 0, "", "", notes, p); shown {
		t.Fatal("second query's own coverage bypassed the exhausted detail budget")
	}
	if detail := p.measurementViewPresentation(tables[1]); detail.rowsComplete || detail.notesComplete || len(notes) != noteCount || len(d.bodies) != contentCount || len(d.data) != 1 {
		t.Fatal("unwritten query inherited completeness or committed unseen notes")
	}
	p.presentationLimit = 96 << 10
	if shown, rows, charge := d.write(&b, w[1], tables[1], 0, "", "", notes, p); !shown || rows != 1 || charge != 0 {
		t.Fatal("legal later coverage publication recharged shared data or lost the partial preview")
	}
	if detail := p.measurementViewPresentation(tables[1]); detail.rowsComplete || detail.notesComplete {
		t.Fatal("same-render omission lost the existing conservative receipt boundary")
	}
	// A new render gets new definitions and receipts. Do not weaken the old
	// same-render AND merge to rehabilitate a previously omitted selector.
	d, notes, p = runtimeMeasurementContentDictionary{}, runtimeMeasurementNoteNavigation{}, &answerDocNativePresentationReceipt{}
	var fresh strings.Builder
	d.write(&fresh, w[0], tables[0], 1, "", "", notes, p)
	if shown, rows, charge := d.write(&fresh, w[1], tables[1], 0, "", "", notes, p); !shown || rows != 1 || charge != 0 {
		t.Fatal("fresh complete coverage failed to reuse its own render's data")
	}
	if detail := p.measurementViewPresentation(tables[1]); detail.rowsComplete || !detail.notesComplete {
		t.Fatal("fresh partial data and full own coverage lost their separate boundaries")
	}
}
