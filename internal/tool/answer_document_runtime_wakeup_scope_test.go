package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRuntimeWakeupSearchEnvelopeAndThreadLanes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		extra map[string]any
		count int
	}{
		{"unbounded", nil, 4},
		{"singleton", map[string]any{"line_start": 19, "line_end": 19}, 1},
		{"inclusive_engine_endpoint", map[string]any{"time_start": 5.039, "time_end": 5.040}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _, _ := runtimeWakeFixture(t)
			ctx.AnalysisIR.RequestModel.RuntimeTargets = nil
			ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = nil
			path, _ := filepath.Abs("../../eval/fixtures/hmosperf_sleep_dependencies/events.systrace")
			args := map[string]any{"path": path, "view": "event_search", "event_types": []string{"sched_wakeup"}}
			for k, v := range tc.extra {
				args[k] = v
			}
			result := businessRefTestQuery(t, ctx, args)
			ctx.Mutable = types.NewMutableState(tc.name)
			ctx.Mutable.AppendDispatchToolResult(result)
			rows := runtimeDiagramRelationsForContext(ctx)
			if len(rows) != tc.count {
				t.Fatalf("lost event envelope endpoint: want %d got %+v", tc.count, rows)
			}
			if tc.count == 4 {
				if rows[0].ToNode != rows[1].ToNode || rows[1].ToNode != rows[2].ToNode || rows[2].ToNode != rows[3].FromNode {
					t.Fatalf("thread lane duplicated: %+v", rows)
				}
				if rows[0].ToIdentity == rows[1].ToIdentity {
					t.Fatal("shared lane merged event credentials")
				}
				if runtimeDiagramEndpointIdentity(rows, rows[0].ToNode, rows[0].ToNode) != rows[0].ToNode {
					t.Fatal("ambiguous lane guessed first event credential")
				}
			}
			doc := runtimeNestingDoc(rows[0], true)
			doc.Blocks[1].Diagram.Kind = types.DiagramSequence
			doc.Blocks[1].EdgeAnchors = nil
			var b strings.Builder
			b.WriteString("sequenceDiagram\n")
			declared := map[string]bool{}
			for _, r := range rows {
				for _, e := range [][2]string{{r.FromNode, r.FromLabel}, {r.ToNode, r.ToLabel}} {
					if !declared[e[0]] {
						b.WriteString(" participant " + e[0] + " as " + e[1] + "\n")
						declared[e[0]] = true
					}
				}
			}
			for _, r := range rows {
				b.WriteString(" " + r.FromNode + "->>" + r.ToNode + ": 唤醒\n")
				doc.Blocks[1].EdgeAnchors = append(doc.Blocks[1].EdgeAnchors, types.DiagramEdgeAnchor{FromNode: r.FromNode, ToNode: r.ToNode, FromIdentity: r.FromIdentity, ToIdentity: r.ToIdentity, RelationKind: r.Kind, VisibleLabel: "唤醒"})
			}
			doc.Blocks[1].Diagram.Body = b.String()
			raw, _ := json.Marshal(doc)
			res, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
			if err != nil || !res.Success {
				t.Fatalf("shared lanes rejected: %v %+v", err, res)
			}
			if got := DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(ctx, ctx.Mutable.AnswerDocumentV2(), types.BuildAnswerSemanticViewForBusContext(ctx), nil); len(got) > 0 {
				t.Fatalf("post gate: %+v", got)
			}
			if tc.count == 4 {
				// A shared thread node does not allow cross-event endpoint borrowing.
				doc.Blocks[1].EdgeAnchors[0].ToIdentity = rows[1].ToIdentity
				raw, _ = json.Marshal(doc)
				res, err = (&EmitAnswerDocument{}).Execute(ctx, raw)
				if err != nil || res.Success {
					t.Fatalf("cross-event credential accepted: %v %+v", err, res)
				}
			}
			// An explicit user half-open bound still excludes the right endpoint.
			start, end := 5.0, 5.04
			ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "5 to 5.04"}
			if got := runtimeDiagramRelationsForContext(ctx); len(got) != tc.count-1 {
				t.Fatalf("explicit right endpoint admitted: %+v", got)
			}
		})
	}
}

func TestRuntimeWakeupRepeatedPairsKeepSeparateCredentials(t *testing.T) {
	ctx, _, _ := runtimeWakeFixture(t)
	ctx.AnalysisIR.RequestModel.RuntimeTargets = nil
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = nil
	path := filepath.Join(t.TempDir(), "repeated.systrace")
	if err := os.WriteFile(path, []byte("loader-7 (7) [001] .... 1.001000: sched_wakeup: comm=target pid=8 prio=120 target_cpu=001\nloader-7 (7) [001] .... 1.002000: sched_wakeup: comm=target pid=8 prio=120 target_cpu=001\n"), 0600); err != nil {
		t.Fatal(err)
	}
	r := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "event_search", "event_types": []string{"sched_wakeup"}})
	ctx.Mutable = types.NewMutableState("repeated")
	ctx.Mutable.AppendDispatchToolResult(r)
	rows := runtimeDiagramRelationsForContext(ctx)
	if len(rows) != 2 || rows[0].FromNode != rows[1].FromNode || rows[0].ToNode != rows[1].ToNode || rows[0].FromIdentity == rows[1].FromIdentity {
		t.Fatalf("repeated pair identity lost: %+v", rows)
	}
	if _, _, ok := runtimeDiagramPairIdentities(rows, rows[0].FromNode, rows[0].ToNode); ok {
		t.Fatal("repeated pair was resolved without event identity")
	}
	doc := runtimeNestingDoc(rows[0], true)
	doc.Blocks[1].Diagram.Kind = types.DiagramSequence
	doc.Blocks[1].Diagram.Body = "sequenceDiagram\n participant " + rows[0].FromNode + " as loader-7\n participant " + rows[0].ToNode + " as target-8\n " + rows[0].FromNode + "->>" + rows[0].ToNode + ": 第一次唤醒\n " + rows[1].FromNode + "->>" + rows[1].ToNode + ": 第二次唤醒\n"
	doc.Blocks[1].EdgeAnchors = nil
	for i, row := range rows {
		label := "第一次唤醒"
		if i == 1 {
			label = "第二次唤醒"
		}
		doc.Blocks[1].EdgeAnchors = append(doc.Blocks[1].EdgeAnchors, types.DiagramEdgeAnchor{FromNode: row.FromNode, ToNode: row.ToNode, FromIdentity: row.FromIdentity, ToIdentity: row.ToIdentity, RelationKind: row.Kind, VisibleLabel: label})
	}
	raw, _ := json.Marshal(doc)
	res, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
	if err != nil || !res.Success {
		t.Fatalf("repeated proved occurrences rejected: %v %+v", err, res)
	}
	if got := DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(ctx, ctx.Mutable.AnswerDocumentV2(), types.BuildAnswerSemanticViewForBusContext(ctx), nil); len(got) > 0 {
		t.Fatalf("post gate: %+v", got)
	}

	for _, change := range []string{"physical_source", "query", "time_domain"} {
		copyR := r
		copyR.Observations = append([]types.ObservationRecord(nil), r.Observations...)
		for i := range copyR.Observations {
			o := &copyR.Observations[i]
			if o.Predicate != "scheduler_wakeup_event" {
				continue
			}
			switch change {
			case "physical_source":
				o.SupportRefs = []string{"another.systrace:1"}
			case "query":
				o.SourceRef.QueryScopeID += "other"
			case "time_domain":
				o.SourceRef.TimeDomain = "other"
			}
		}
		other := RuntimeDiagramRelations(types.ObservationLedger{Records: copyR.Observations}, nil)
		if len(other) != 2 || other[0].FromNode == rows[0].FromNode || other[0].ToNode == rows[0].ToNode {
			t.Fatalf("%s shared display lanes: %+v", change, other)
		}
	}
	for _, change := range []string{"missing_selector", "changed_timestamp", "outside_selector"} {
		copyR := r
		copyR.Observations = append([]types.ObservationRecord(nil), r.Observations...)
		for i := range copyR.Observations {
			o := &copyR.Observations[i]
			if o.Predicate != "scheduler_wakeup_event" {
				continue
			}
			f, ok := decodeRuntimeWakeupEvent(*o)
			if !ok {
				t.Fatal("fixture not authorized")
			}
			if change == "missing_selector" {
				f.ScanScope = nil
			} else if change == "changed_timestamp" {
				f.Timestamp += 1
			} else {
				f.ScanScope = types.CloneTraceEventSearchScanScope(f.ScanScope)
				f.ScanScope.TimeStartApplied = true
				f.ScanScope.TimeStart = 2
			}
			b, _ := json.Marshal(f)
			o.RichNotes = []string{traceWakeupEventNote + string(b)}
		}
		if other := RuntimeDiagramRelations(types.ObservationLedger{Records: copyR.Observations}, nil); len(other) != 0 {
			t.Fatalf("%s still authorized: %+v", change, other)
		}
	}
}
