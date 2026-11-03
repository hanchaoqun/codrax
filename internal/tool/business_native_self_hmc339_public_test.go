package tool

import (
	"encoding/json"
	"html"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func businessNativeSelfQuery339(t *testing.T, start, end float64) (*types.BusContext, types.ToolResult) {
	t.Helper()
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_marker_tree/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	rm := types.RequestModel{Language: "en", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, PerfTrace: &types.PerfBundle{}}
	ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "en", Mutable: types.NewMutableState("List the observed business interval and self-state accounts"), AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
	ctx.Mutable.SetRequestModel(rm)
	result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "window_stats", "pid": 700, "time_start": start, "time_end": end})
	ctx.Mutable.AppendDispatchToolResult(result)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	return ctx, result
}

func businessNativeRows339(t *testing.T, result types.ToolResult, subject, name string) (types.ObservationRecord, map[string]string, string) {
	t.Helper()
	for _, r := range result.Observations {
		if r.Predicate != types.TraceBusinessSpanPredicate || r.Subject != subject || r.Object != name {
			continue
		}
		p, ok := types.DecodeRuntimeMeasurementPublication(r)
		if !ok || len(p.Tables) != 1 {
			t.Fatalf("missing existing native summary for %s: %+v", name, r)
		}
		rows := map[string]string{}
		for _, row := range p.Tables[0].Rows {
			rows[row[0]] = row[1]
		}
		if len(p.Tables[0].Rows) != 13 {
			t.Fatal("self delivery changed the established row-reference coordinates")
		}
		return r, rows, strings.Join(p.Tables[0].Notes, "\n")
	}
	t.Fatalf("missing public business record %s/%s", subject, name)
	return types.ObservationRecord{}, nil, ""
}

func TestHMC339BusinessNativeSelfPublicExactWindow(t *testing.T) {
	for _, w := range [][2]float64{{5, 5.012}, {5.002, 5.007}} {
		t.Run(strconv.FormatFloat(w[0], 'f', 3, 64), func(t *testing.T) {
			_, result := businessNativeSelfQuery339(t, w[0], w[1])
			before, _ := json.Marshal(result.Observations)
			for _, r := range result.Observations {
				f, ok := DecodeTraceBusinessTreeFact(r)
				if !ok || f.Node.Thread.PID != 700 || f.Node.Closure != "closed" {
					continue
				}
				span, rows, notes := businessNativeRows339(t, result, r.Subject, r.Object)
				wantAccount := "自身账户（窗口内墙钟，不是运行时间）：" + traceBusinessTreeAccountText(f.Node.Self)
				if !strings.Contains(notes, wantAccount) {
					t.Errorf("%s lost the complete self wall/state/segments/omissions account: want %s; got %s", r.Object, wantAccount, notes)
				}
				if rows["业务区间开始（秒，含）"] != traceQueryDisplaySeconds(span.Span.StartTs) || rows["业务区间结束（秒，不含）"] != traceQueryDisplaySeconds(span.Span.EndTs) {
					t.Error("native self account lost its own clipped interval or independent segment omissions")
				}
			}
			_, _, parentNotes := businessNativeRows339(t, result, "business-700", "OpenDocument")
			want := "5"
			if w[0] == 5.002 {
				want = "0"
			}
			if !strings.Contains(parentNotes, "自身账户（窗口内墙钟，不是运行时间）："+want+" 毫秒；") {
				t.Errorf("parent self borrowed a whole-instance or child denominator: %s", parentNotes)
			}
			after, _ := json.Marshal(result.Observations)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("reading the source-bound display mutated query facts")
			}
		})
	}
}

func TestHMC339BusinessNativeSelfPublicUnavailableNotZero(t *testing.T) {
	_, result := businessNativeSelfQuery339(t, 5, 5.012)
	_, _, notes := businessNativeRows339(t, result, "background-900", "OpenDocument")
	if !strings.Contains(notes, "自身账户（窗口内墙钟，不是运行时间）：5.5 毫秒") || !strings.Contains(notes, "线程状态未能计量，不按零处理") {
		t.Fatal("unknown scheduler account erased a known self wall duration")
	}
}

func TestHMC339BusinessNativeSelfPublicSurfacesAndPatch(t *testing.T) {
	for _, kind := range []string{"table", "summary", "diagram"} {
		t.Run(kind, func(t *testing.T) {
			ctx, result := businessNativeSelfQuery339(t, 5, 5.012)
			record, _, _ := businessNativeRows339(t, result, "business-700", "LoadIndex")
			view := types.BuildAnswerSemanticViewForBusContext(ctx)
			if view == nil {
				t.Fatal("public typed request model did not build a semantic view")
			}
			receipt := types.AnswerRuntimeMeasurementReceipt{ObservationID: record.ID, View: types.RuntimeMeasurementSummary}
			if !types.BindRuntimeMeasurementReceipt(&receipt, view.RuntimeMeasurementContract) || !receipt.BoundTable.HasNativeMeasurementFacts() {
				t.Fatal("real current query did not qualify its complete native business account")
			}
			before, _ := json.Marshal(result.Observations)
			facts := map[string]any{"id": "facts", "kind": kind, "runtime_measurement": map[string]any{"observation_id": record.ID, "view": "summary"}}
			blocks := []any{facts}
			if kind != "summary" {
				blocks = append([]any{map[string]any{"id": "lead", "kind": "summary", "text": "Native business accounts; wall time and execution are separate."}}, facts)
			}
			if out := b1659bExecuteAnswer(t, ctx, map[string]any{"blocks": blocks}, false); !out.Success {
				t.Fatal(out.Summary)
			}
			if out := b1659bExecuteAnswer(t, ctx, map[string]any{"replace_blocks": []any{facts}}, true); !out.Success {
				t.Fatal(out.Summary)
			}
			block := blockByID(t, ctx.Mutable.AnswerDocumentV2(), "facts")
			if !block.RuntimeMeasurement.IsBound() || !reflect.DeepEqual(block.RuntimeMeasurement.BoundTable.Rows, receipt.BoundTable.Rows) || block.RuntimeMeasurement.BoundTable.MemberSet != nil || block.RuntimeMeasurement.BoundTable.DisplayPopulation != nil {
				t.Fatal("native reuse changed the account or acquired population authority")
			}
			visible := html.UnescapeString(render.RenderAnswerDocument(ctx.Mutable.AnswerDocumentV2(), "zh"))
			if !strings.Contains(visible, "自身账户（窗口内墙钟，不是运行时间）") || !strings.Contains(visible, "状态交集") || !strings.Contains(visible, "未展示分段") {
				t.Fatal("native self fields did not reach the reader surface")
			}
			if kind == "diagram" && len(block.EdgeAnchors) != 0 {
				t.Fatal("a measured business account invented a relation")
			}
			after, _ := json.Marshal(result.Observations)
			if string(before) != string(after) {
				t.Fatal("full or patch changed the original native facts")
			}
		})
	}
}

func TestHMC339BusinessNativeSelfExactJoinNegativeMatrix(t *testing.T) {
	_, result := businessNativeSelfQuery339(t, 5, 5.012)
	payload := businessSpanSchedulerPublicPayload(t, result)
	for _, mode := range []string{"positive", "source", "payload", "query", "window", "target", "physical_line", "owner", "full_end", "invalid_value", "duplicate", "duplicate_note", "missing"} {
		t.Run(mode, func(t *testing.T) {
			var records []types.ObservationRecord
			for _, original := range result.Observations {
				raw, _ := json.Marshal(original)
				var r types.ObservationRecord
				if err := json.Unmarshal(raw, &r); err != nil {
					t.Fatal(err)
				}
				if r.Predicate == types.TraceBusinessSpanPredicate {
					var notes []string
					for _, note := range r.RichNotes {
						if !strings.HasPrefix(note, types.TraceNoteKeyRuntimeMeasurement+"=") && !strings.HasPrefix(note, types.TraceNoteKeyBusinessDiagramAnnotation+"=") {
							notes = append(notes, note)
						}
					}
					r.RichNotes = notes
				}
				f, tree := DecodeTraceBusinessTreeFact(r)
				if tree && r.Object == "OpenDocument" && f.Node.Thread.PID == 700 {
					switch mode {
					case "source":
						r.SourceRef.CaptureIdentityPath += ".other"
					case "payload":
						r.SourceRef.PayloadRef += ".other"
					case "query":
						r.SourceRef.QueryScopeID += ".other"
					case "window":
						r.SourceRef.QueryWindowEndTs += .001
					case "target":
						r.SourceRef.QueryTargetPID++
					case "physical_line":
						r.Span.LineStart++
					case "owner":
						r.Subject = "other-700"
					case "full_end":
						end := *f.Node.ActualEndTs + .001
						f.Node.ActualEndTs = &end
					case "invalid_value":
						f.Node.Self.DurationMs++
					case "duplicate":
						records = append(records, r)
					case "duplicate_note":
						r.RichNotes = append(r.RichNotes, types.TraceNoteKeyBusinessTreeNode+"={}")
					case "missing":
						continue
					}
					if mode == "full_end" || mode == "invalid_value" {
						data, _ := json.Marshal(f)
						r.RichNotes = []string{types.TraceNoteKeyBusinessTreeNode + "=" + string(data)}
					}
				}
				records = append(records, r)
			}
			treeRows := func(rows []types.ObservationRecord) []types.ObservationRecord {
				var trees []types.ObservationRecord
				for _, r := range rows {
					if r.Predicate == types.TraceBusinessTreePredicate {
						trees = append(trees, r)
					}
				}
				return trees
			}
			before, _ := json.Marshal(treeRows(records))
			out := traceQueryBusinessMeasurementPublications(payload, records)
			_, rows, notes := businessNativeRows339(t, types.ToolResult{Observations: out}, "business-700", "OpenDocument")
			want := "自身账户未验证："
			if mode == "positive" {
				want = "自身账户（窗口内墙钟，不是运行时间）：5 毫秒；"
			}
			if !strings.Contains(notes, want) || rows["窗口内业务耗时（ms）"] != "10" {
				t.Fatalf("borrowed an almost-matching self account or changed inclusive denominator: rows=%+v notes=%s", rows, notes)
			}
			after, _ := json.Marshal(treeRows(out))
			if string(before) != string(after) {
				t.Fatal("publication changed the independently measured original tree")
			}
		})
	}
}
