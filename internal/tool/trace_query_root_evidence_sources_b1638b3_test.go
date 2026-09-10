package tool

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The committed carve exercises the actual impact -> reduced root witness ->
// publication lane. It does not infer provenance from equal numerical values:
// only the native impact with the same thread and physical endpoints is used.
func TestB1638B3RootEvidenceActualTiebaPublication(t *testing.T) {
	if _, err := os.Stat(ispgapTiebaCarve); err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	path, err := filepath.Abs(ispgapTiebaCarve)
	if err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: filepath.Dir(path), WorkDir: t.TempDir(), Mutable: types.NewMutableState("root evidence source round trip")}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "root_cause_rank", "pid": 59843, "time_start": 34579.450627, "time_end": 34579.595131, "min_duration_ms": 0.05})
	published, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !published.Success {
		t.Fatalf("actual Execute failed: %v %+v", err, published)
	}
	ctx.Mutable.AppendDispatchToolResult(published)
	var payloadRef string
	for _, row := range published.Observations {
		if row.SourceRef.PayloadRef != "" {
			payloadRef = row.SourceRef.PayloadRef
			break
		}
	}
	if payloadRef == "" {
		t.Fatal("actual result did not publish its payload receipt")
	}
	wire, err := os.ReadFile(payloadRef)
	if err != nil {
		t.Fatal(err)
	}
	var native tracequery.Result
	if err := json.Unmarshal(wire, &native); err != nil || native.WakeupChain == nil {
		t.Fatalf("native payload unavailable: %v", err)
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
	for _, stage := range []struct {
		name    string
		records []types.ObservationRecord
	}{{"publication", published.Observations}, {"ledger", ledger.Records}} {
		t.Run(stage.name, func(t *testing.T) {
			checked := 0
			for _, row := range stage.records {
				if !strings.HasPrefix(row.ClaimKey, "root_evidence:") || row.Subject != "CookieMonsterCl-59843" || row.Predicate != "runnable_wait" {
					continue
				}
				var matches []tracequery.WakeupCausalImpact
				for _, impact := range native.WakeupChain.CausalImpacts {
					if impact.Thread.PID == 59843 && impact.DominantState == string(tracequery.StateRunnable) && impact.LineStart == row.Span.LineStart && impact.LineEnd == row.Span.LineEnd {
						matches = append(matches, impact)
					}
				}
				if len(matches) != 1 {
					t.Fatalf("fixture root witness lacks unique actual impact endpoints: %s matches=%d", row.ID, len(matches))
				}
				impact := matches[0]
				if impact.MeasurementSources == nil || len(impact.MeasurementSources.Domains) == 0 {
					t.Fatalf("actual impact source missing: %+v", impact)
				}
				checked++
				if !reflect.DeepEqual(row.MeasurementSources, impact.MeasurementSources) {
					t.Errorf("%s lost root witness native source: id=%s value=%s sources=%+v want=%+v", stage.name, row.ID, row.Value, row.MeasurementSources, impact.MeasurementSources)
				}
				if row.Value != traceQueryObservationMSValue(impact.DominantImpactMs) || row.SourceRef.QueryScopeID == "" {
					t.Errorf("root value/parent receipt drift: %+v", row)
				}
				if row.MeasurementSources != nil {
					rowWire, _ := json.Marshal(row)
					if !bytes.Contains(rowWire, []byte(impact.MeasurementSources.Domains[0].PartitionID)) {
						t.Error("JSON round trip lost original source")
					}
				}
			}
			if checked < 4 {
				t.Fatalf("must exercise the four real runnable root witnesses, got %d", checked)
			}
		})
	}
	// The existing full-seat scanner keeps its nonempty merged arm. These are
	// unchanged physical values, not a new expected count chosen to force green.
	projection := types.CompileTraceCausalProjection(ledger)
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background} {
		for _, row := range rows {
			if row.Node.Subject == "CookieMonsterCl-59843" && row.Node.MergedCount == 3 && math.Abs(row.Node.ImpactMS-12.773) < .001 {
				return
			}
		}
	}
	t.Error("the original three-member 12.773 ms seat did not survive the real publication chain")
}

func TestB1638B3RootEvidencePublicationPreservesOldFieldsAndUnknown(t *testing.T) {
	for _, tc := range []struct {
		name    string
		sources *types.TraceSchedulerMeasurementSources
	}{
		{"known", b1638b2bSource("root-own")},
		{"mixed", types.MergeTraceSchedulerMeasurementSources(b1638b2bSource("root-own"), nil)},
		{"unknown", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := tracequery.RootEvidence{Type: "runnable_wait", Thread: tracequery.ThreadRef{PID: 55, Comm: "worker"}, DurationMs: 10, LineStart: 3, LineEnd: 7, StartTs: 1.01, EndTs: 1.02, Summary: "original root witness text", Confidence: .8, DominantState: "runnable"}
			// Decode the wire extension through the real carrier, preserving
			// every legacy field. This test also compiles before the extension.
			wire, _ := json.Marshal(base)
			var shape map[string]any
			if err := json.Unmarshal(wire, &shape); err != nil {
				t.Fatal(err)
			}
			if tc.sources != nil {
				shape["measurement_sources"] = tc.sources
			}
			wire, _ = json.Marshal(shape)
			var root tracequery.RootEvidence
			if err := json.Unmarshal(wire, &root); err != nil {
				t.Fatal(err)
			}
			before, _ := json.Marshal(root)
			got := traceQueryPriorityRootEvidenceForPublication([]tracequery.RootEvidence{root})
			if len(got) != 1 {
				t.Fatalf("source metadata changed old publication admission: %+v", got)
			}
			gotWire, _ := json.Marshal(got[0])
			var oldFields map[string]json.RawMessage
			if err := json.Unmarshal(gotWire, &oldFields); err != nil {
				t.Fatal(err)
			}
			var source *types.TraceSchedulerMeasurementSources
			if raw := oldFields["measurement_sources"]; len(raw) > 0 {
				if err := json.Unmarshal(raw, &source); err != nil {
					t.Fatal(err)
				}
			}
			if !reflect.DeepEqual(source, tc.sources) {
				t.Errorf("publication did not preserve exact known/unknown source: %+v want %+v", source, tc.sources)
			}
			delete(oldFields, "measurement_sources")
			oldWire, _ := json.Marshal(oldFields)
			var oldRoot tracequery.RootEvidence
			if err := json.Unmarshal(oldWire, &oldRoot); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(oldRoot, base) {
				t.Fatalf("legacy fields changed: %+v want %+v", oldRoot, base)
			}
			q := tracequery.Query{View: "wakeup_chain", PID: 55, TimeStart: 1, TimeEnd: 2}
			with := traceQueryTypedObservations(tracequery.Result{View: q.View, WakeupChain: &tracequery.ChainResult{RootEvidence: []tracequery.RootEvidence{root}}}, "/fixture/root.ftrace", "payload", "raw", "", time.Unix(1, 0), q)
			without := traceQueryTypedObservations(tracequery.Result{View: q.View, WakeupChain: &tracequery.ChainResult{RootEvidence: []tracequery.RootEvidence{base}}}, "/fixture/root.ftrace", "payload", "raw", "", time.Unix(1, 0), q)
			if len(with) != 1 || len(without) != 1 {
				t.Fatalf("original witness count changed: %d %d", len(with), len(without))
			}
			if !reflect.DeepEqual(with[0].MeasurementSources, tc.sources) {
				t.Errorf("observation lost exact sources: %+v want %+v", with[0].MeasurementSources, tc.sources)
			}
			with[0].MeasurementSources = nil
			if !reflect.DeepEqual(with, without) {
				t.Fatal("source-only extension changed old typed observation fields")
			}
			after, _ := json.Marshal(root)
			if !bytes.Equal(before, after) {
				t.Fatal("publication mutated original root")
			}
		})
	}
}

func TestB1638B3RootEvidencePublicationDeepCopyAndPriorityBoundary(t *testing.T) {
	for _, publisher := range []string{"root_slice", "result_chain", "observation"} {
		t.Run(publisher, func(t *testing.T) {
			root := tracequery.RootEvidence{Type: "runnable_wait", Thread: tracequery.ThreadRef{PID: 55}, DurationMs: 10, Summary: "unchanged root", MeasurementSources: types.MergeTraceSchedulerMeasurementSources(b1638b2bSource("owned"), nil)}
			before, _ := json.Marshal(root)
			var sources *types.TraceSchedulerMeasurementSources
			switch publisher {
			case "root_slice":
				sources = traceQueryPriorityRootEvidenceForPublication([]tracequery.RootEvidence{root})[0].MeasurementSources
			case "result_chain":
				result := traceQueryPriorityResultForPublication(tracequery.Result{WakeupChain: &tracequery.ChainResult{RootEvidence: []tracequery.RootEvidence{root}}})
				sources = result.WakeupChain.RootEvidence[0].MeasurementSources
			case "observation":
				rows := traceQueryTypedObservations(tracequery.Result{WakeupChain: &tracequery.ChainResult{RootEvidence: []tracequery.RootEvidence{root}}}, "/fixture/root.ftrace", "payload", "raw", "", time.Unix(1, 0), tracequery.Query{View: "wakeup_chain", PID: 55, TimeStart: 1, TimeEnd: 2})
				if len(rows) != 1 {
					t.Fatalf("unexpected row count: %d", len(rows))
				}
				sources = rows[0].MeasurementSources
			}
			if sources == nil || !sources.HasUnknown || len(sources.Domains) != 1 {
				t.Fatalf("source/unknown marker lost: %+v", sources)
			}
			sources.Domains[0].PartitionID = "changed returned copy"
			sources.HasUnknown = false
			after, _ := json.Marshal(root)
			if !bytes.Equal(before, after) {
				t.Fatal("published source aliases its native root")
			}
		})
	}
	// Source metadata cannot upgrade reduced-shape priority twins. Existing
	// publication sanitization still excludes them even with known sources.
	for _, typ := range []string{"priority_inversion_runnable_wait", "priority_inversion_candidate"} {
		root := tracequery.RootEvidence{Type: typ, DurationMs: 10, MeasurementSources: b1638b2bSource("not-a-priority-proof")}
		if got := traceQueryPriorityRootEvidenceForPublication([]tracequery.RootEvidence{root}); len(got) != 0 {
			t.Fatalf("source changed priority proof admission: %+v", got)
		}
	}
	for _, typ := range []string{"trace_gap", "missing_wakeup", "binder_wait"} {
		root := tracequery.RootEvidence{Type: typ, Summary: "literal root has no native source"}
		got := traceQueryPriorityRootEvidenceForPublication([]tracequery.RootEvidence{root})
		if len(got) != 1 || got[0].MeasurementSources != nil {
			t.Fatalf("literal source was guessed: %+v", got)
		}
	}
}
