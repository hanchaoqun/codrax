package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// B/E, scheduler and block-I/O locators are useful even without a model-created
// scalar. Required frame/stall durations must not force arithmetic or dummy 0s.
func TestEmitPerfTraceAcceptsLocatorsWithoutDerivedScalars(t *testing.T) {
	for _, tc := range []struct {
		name, trace, evidence, summary string
	}{
		{
			name: "business_B_E",
			trace: "app-100 (100) [000] .... 1.000000: tracing_mark_write: B|100|OpenDocument\n" +
				"app-100 (100) [000] .... 1.050000: tracing_mark_write: E|100\n",
			evidence: "1.000000: tracing_mark_write: B|100|OpenDocument\n1.050000: tracing_mark_write: E|100",
			summary:  "OpenDocument has begin and end markers on the same thread; source timestamps are in seconds.",
		},
		{
			name: "scheduler_S_then_wakeup",
			trace: "worker-200 (100) [000] .... 1.009010: sched_switch: prev_comm=worker prev_pid=200 prev_prio=120 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120\n" +
				"irq-80 (2) [000] .... 1.040010: sched_wakeup: comm=worker pid=200 prio=120 target_cpu=000\n",
			evidence: "1.009010: prev_pid=200 prev_state=S\n1.040010: sched_wakeup: comm=worker pid=200",
			summary:  "The worker sleeps in S and later has a wakeup event; these are navigation endpoints, not a measured wait or an I/O cause.",
		},
		{
			name: "block_request_identity",
			trace: "worker-200 (100) [000] .... 1.005000: block_rq_issue: 12,80 R 32768 () 923339752 + 64 [worker]\n" +
				"irq-80 (2) [000] .... 1.040000: block_rq_complete: 12,80 R () 923339752 + 64 [0]\n",
			evidence: "1.005000: block_rq_issue: 12,80 R 32768 () 923339752 + 64 [worker]\n1.040000: block_rq_complete: 12,80 R () 923339752 + 64 [0]",
			summary:  "Issue and completion rows share device and sector; the response-wait relationship remains unproven here.",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params, err := json.Marshal(emitPerfTraceParams{
				Meta: emitPerfTraceMeta{Source: "systrace"},
				Observations: []emitPerfTraceObservation{{
					Kind: "line_anchor", Subject: tc.name, Summary: tc.summary,
					Evidence: tc.evidence, LineStart: 1, LineEnd: 2,
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			for _, numericField := range []string{"duration_ms", "start_ts_ms", "end_ts_ms"} {
				if strings.Contains(string(params), numericField) {
					t.Fatalf("locator payload unexpectedly contains %s: %s", numericField, params)
				}
			}
			bus := &types.BusContext{Mutable: types.NewMutableState(tc.name), AttachedHitrace: tc.trace}
			result, err := (&EmitPerfTrace{}).Execute(bus, params)
			if err != nil || !result.Success {
				t.Fatalf("locator-only emission rejected: result=%+v err=%v", result, err)
			}
			bundle := bus.Mutable.PerfTrace()
			if bundle == nil || len(bundle.Frames)+len(bundle.Stalls)+len(bundle.Janks) != 0 {
				t.Fatalf("observation-only payload fabricated a measured span: %+v", bundle)
			}
			var found bool
			for _, obs := range bundle.Observations {
				if obs.Subject != tc.name {
					continue // The existing deterministic validator may add time provenance.
				}
				found = true
				if obs.Authority != types.PerfObservationAuthorityPreTriageModelExtraction ||
					obs.Evidence != tc.evidence || obs.Summary != tc.summary ||
					obs.LineStart != 1 || obs.LineEnd != 2 ||
					obs.DurationMs != 0 || obs.StartTsMs != 0 || obs.EndTsMs != 0 {
					t.Fatalf("locator-only observation changed: %+v", obs)
				}
			}
			if !found {
				t.Fatalf("locator-only observation was lost: %+v", bundle)
			}
		})
	}
}

func TestEmitPerfTraceUnparsedObservationIsAcceptedWithoutFabricatedTraceFacts(t *testing.T) {
	const source = "custom-event payload: ???\nunrecognized-stream-entry\n"
	bus := &types.BusContext{Mutable: types.NewMutableState("unparsed trace"), AttachedHitrace: source}
	params := json.RawMessage(`{
		"meta":{"source":"unknown"},
		"observations":[{
			"kind":"unparsed","subject":"Unrecognized trace lines",
			"summary":"Cannot identify an event or span from these source lines.",
			"evidence":"custom-event payload: ???\nunrecognized-stream-entry",
			"line_start":1,"line_end":2
		}],
		"residue":["custom-event payload: ???\nunrecognized-stream-entry"]
	}`)
	result, err := (&EmitPerfTrace{}).Execute(bus, params)
	if err != nil || !result.Success {
		t.Fatalf("unparsed observation rejected: result=%+v err=%v", result, err)
	}
	bundle := bus.Mutable.PerfTrace()
	if bundle == nil || len(bundle.Observations) != 1 || len(bundle.Residue) != 1 {
		t.Fatalf("unparsed observation/residue was lost: %+v", bundle)
	}
	obs := bundle.Observations[0]
	if obs.Kind != "unparsed" || obs.Authority != types.PerfObservationAuthorityPreTriageModelExtraction ||
		obs.LineStart != 1 || obs.LineEnd != 2 || obs.Evidence != strings.TrimSpace(source) ||
		obs.DurationMs != 0 || obs.StartTsMs != 0 || obs.EndTsMs != 0 {
		t.Fatalf("unparsed observation was changed or promoted: %+v", obs)
	}
	if len(bundle.Frames)+len(bundle.Stalls)+len(bundle.Janks) != 0 || bundle.Startup != nil ||
		bundle.Meta.DurationMs != 0 || len(bundle.Meta.Signals) != 0 {
		t.Fatalf("unparsed observation fabricated measured/business facts: %+v", bundle)
	}
}

func TestEmitPerfTraceResidueOnlyRemainsRejected(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("unparsed trace"), AttachedHitrace: "unrecognized-stream-entry\n"}
	result, err := (&EmitPerfTrace{}).Execute(bus, json.RawMessage(`{
		"meta":{"source":"unknown"},"residue":["unrecognized-stream-entry"]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Success || !strings.Contains(result.Summary, "frames / janks / stalls / startup / observations all empty") {
		t.Fatalf("residue-only result bypassed structured-fact requirement: %+v", result)
	}
	if bus.Mutable.PerfTrace() != nil {
		t.Fatalf("rejected residue-only payload was stored: %+v", bus.Mutable.PerfTrace())
	}
}
