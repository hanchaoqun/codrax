package tool

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the registered public metadata tool, not a catalog helper. It must
// work without any capture, filesystem root, mutable state, or query execution.
func capabilityPublicCall(t *testing.T, ctx *types.BusContext, params string) map[string]any {
	t.Helper()
	r := NewRegistry()
	RegisterDefaults(r)
	registered, err := r.Get("trace_capabilities")
	if err != nil {
		t.Fatal(err)
	}
	if registered.IsWrite() || registered.Confidence() != 0 {
		t.Fatal("catalog acquired write/evidence authority")
	}
	result, err := r.Execute(ctx, "trace_capabilities", json.RawMessage(params))
	if err != nil || !result.Success {
		t.Fatalf("catalog: %v %+v", err, result)
	}
	if len(result.Observations) != 0 || result.RawRef != "" || result.TraceEvidenceAuthority != nil || result.ReadCoverage != nil || result.SourceInventory != nil {
		t.Fatalf("metadata was published as evidence: %+v", result)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(result.Summary), &out); err != nil {
		t.Fatalf("catalog is not public JSON: %v", err)
	}
	if out["static_only"] != true || out["evidence"] != false || out["capture_availability"] != "not_evaluated" {
		t.Fatalf("lost metadata boundary: %+v", out)
	}
	if params == `{}` || params == `{"view":"window_stats","detail":true}` {
		t.Logf("catalog params=%s serialized_bytes=%d", params, len(result.Summary))
	}
	return out
}

func TestTraceCapabilitiesPublicCompleteDiscovery(t *testing.T) {
	out := capabilityPublicCall(t, nil, `{}`)
	var got []string
	for _, raw := range out["views"].([]any) {
		v := raw.(map[string]any)
		got = append(got, v["view"].(string))
		for _, field := range []string{"summary", "objects", "metric_refs", "input_formats"} {
			if v[field] == nil || v[field] == "" {
				t.Errorf("%s lacks %s", v["view"], field)
			}
		}
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, tracequery.CanonicalViewNames()) {
		t.Fatalf("catalog/engine drift: %v", got)
	}
	if len(got) != 21 || out["metrics"] != nil {
		t.Fatalf("summary should be complete but compact: views=%d metrics=%v", len(got), out["metrics"])
	}
	var schema struct {
		Properties map[string]struct {
			Enum    []string          `json:"enum"`
			Aliases map[string]string `json:"x-codrax-enum-aliases"`
		} `json:"properties"`
	}
	if err := json.Unmarshal((&TraceQuery{}).Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	views := schema.Properties["view"]
	sort.Strings(views.Enum)
	if !reflect.DeepEqual(got, views.Enum) {
		t.Fatal("catalog/schema view drift")
	}
	for alias, canonical := range views.Aliases {
		aliasOut := capabilityPublicCall(t, nil, `{"view":"`+alias+`"}`)
		rows := aliasOut["views"].([]any)
		if len(rows) != 1 || rows[0].(map[string]any)["view"] != canonical {
			t.Fatalf("alias %s lost: %+v", alias, rows)
		}
	}
}

func TestTraceCapabilitiesPublicDetailAndIsolation(t *testing.T) {
	bus := &types.BusContext{RepoRoot: "/does/not/exist", Language: "zh", Mutable: &types.MutableState{}}
	before, err := json.Marshal(bus)
	if err != nil {
		t.Fatal(err)
	}
	first := capabilityPublicCall(t, bus, `{"view":"window_stats","detail":true}`)
	metrics := first["metrics"].([]any)
	byID := map[string]map[string]any{}
	for _, raw := range metrics {
		m := raw.(map[string]any)
		id := m["id"].(string)
		if byID[id] != nil {
			t.Fatalf("duplicate metric %s", id)
		}
		byID[id] = m
		if m["requirements"] == nil || m["outputs"] == nil || m["limitations"] == nil {
			t.Fatalf("incomplete metric: %+v", m)
		}
	}
	for _, id := range []string{"scheduler_states", "io_request_latency", "cpu_frequency", "perf_samples", "trace_counters", "blocked_reasons"} {
		if byID[id] == nil {
			t.Errorf("missing published metric family %s", id)
		}
	}
	ioBody, _ := json.Marshal(byID["io_request_latency"])
	for _, token := range []string{"ms", tracequery.IORequestLatencyQuantileMethod, tracequery.IORequestLatencySamplePolicy, tracequery.IORequestLatencyCaliber} {
		if !strings.Contains(string(ioBody), token) {
			t.Errorf("IO catalog lost existing contract %q", token)
		}
	}
	after, _ := json.Marshal(bus)
	if string(before) != string(after) {
		t.Fatal("catalog mutated the Bus")
	}
	second := capabilityPublicCall(t, nil, `{"view":"window_stats","detail":true}`)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("catalog depends on language, capture, or repository state")
	}
	capabilityPublicCall(t, nil, `{"view":"frame_root_cause_bundle","detail":true}`)
}

func TestTraceCapabilitiesPublicRejectsUnsupportedArguments(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	for _, params := range []string{`{"view":"not_a_view"}`, `{"detail":"yes"}`, `{"path":"secret.trace"}`, `{"source":"attached_trace"}`} {
		result, err := r.Execute(nil, "trace_capabilities", json.RawMessage(params))
		if err != nil && strings.Contains(err.Error(), "tool not found") {
			t.Fatal(err)
		}
		if err == nil && result.Success {
			t.Errorf("accepted unsupported metadata arguments %s", params)
		}
	}
}

func TestTraceCapabilitiesPublicContractsMatchRealQueryUnitsAndMissing(t *testing.T) {
	catalog := capabilityPublicCall(t, nil, `{"view":"window_stats","detail":true}`)
	assertUnit := func(metric, section, field, want string) {
		t.Helper()
		for _, raw := range catalog["metrics"].([]any) {
			m := raw.(map[string]any)
			if m["id"] != metric {
				continue
			}
			for _, rawOutput := range m["outputs"].([]any) {
				output := rawOutput.(map[string]any)
				if output["section"] != section {
					continue
				}
				for _, candidate := range output["fields"].([]any) {
					if candidate == field && output["unit"] == want {
						return
					}
				}
			}
		}
		t.Fatalf("catalog lacks %s %s.%s unit=%s", metric, section, field, want)
	}
	assertUnit("io_request_latency", "window_stats.storage_latency_by_layer.request_latency_distribution", "p99_ms", "ms")
	assertUnit("cpu_frequency", "window_stats.cpu", "frequency", "kHz")
	assertUnit("perf_samples", "perf_stats", "sample_count", "count")
	assertUnit("perf_samples", "perf_stats.top_symbols", "period", "cohort.weight_unit")
	assertUnit("compute_supply", "window_stats.compute_supply_balance", "nominal_capacity_ms", "CPU·ms")
	assertUnit("compute_supply", "window_stats.compute_supply_balance", "window_ms", "ms")
	registry := NewRegistry()
	RegisterDefaults(registry)
	query := func(body, view string) tracequery.Result {
		t.Helper()
		bus, _, conversions := hmc17NamedPathContext(t)
		capabilityPublicCall(t, bus, `{"view":"`+view+`","detail":true}`)
		if conversions.Load() != 0 {
			t.Fatal("catalog invoked a conversion")
		}
		path := hmc081WriteTrace(t, body)
		params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": view, "time_start": 0.0, "time_end": .010})
		result, err := registry.Execute(bus, "trace_query", params)
		if err != nil {
			t.Fatal(err)
		}
		payload := hmc17NamedPayload(t, result)
		if payload.TimeUnit != "seconds" || payload.TimeStart != 0 || payload.TimeEnd != .010 {
			t.Fatalf("query timestamp contract drift: %+v", payload)
		}
		return payload
	}
	base := "# tracer: nop\nclock-1 (1) [000] .... 0.000000: cpu_frequency: state=1200000 cpu_id=0\n"
	for _, tc := range []struct {
		name     string
		complete bool
		ms       float64
	}{{"missing", false, 0}, {"measured_zero", true, 0}, {"measured_two_ms", true, 2}} {
		t.Run(tc.name, func(t *testing.T) {
			body := base + "reader-40 (40) [001] .... 0.001000: block_rq_issue: 8,0 R 4096 () 1000 + 8 [reader]\n"
			if tc.complete {
				body += fmt.Sprintf("irq-2 (2) [001] .... %.6f: block_rq_complete: 8,0 R () 1000 + 8 [0]\n", .001+tc.ms/1000)
			}
			payload := query(body, "window_stats")
			stats := payload.WindowStats
			if stats == nil || len(stats.StorageLatencyByLayer) != 1 {
				t.Fatalf("missing native group: %+v", stats)
			}
			d := stats.StorageLatencyByLayer[0].RequestLatencyDistribution
			if !tc.complete {
				if d != nil {
					t.Fatal("unmeasured IO became zero distribution")
				}
			} else if d == nil || d.SampleCount != 1 || math.Abs(d.P99Ms-tc.ms) > 1e-8 {
				t.Fatalf("wrong native ms: %+v", d)
			}
			found := false
			for _, cpu := range stats.CPU {
				if cpu.CPU == 0 && cpu.Frequency == 1200000 {
					found = true
				}
			}
			if !found {
				t.Fatal("CPU raw kHz changed")
			}
		})
	}
	t.Run("sample_weight_is_not_duration", func(t *testing.T) {
		body := "app-40 (40) [001] .... 0.001000: perf_sample: pid=40 tid=40 cpu=1 period=10000 event=cpu-cycles symbol=Work dso=libapp.so callchain=main;Work\n" +
			"app-40 (40) [001] .... 0.002000: perf_sample: pid=40 tid=40 cpu=1 period=30000 event=cpu-cycles symbol=Work dso=libapp.so callchain=main;Work\n"
		payload := query(body, "perf_stats")
		if payload.PerfStats == nil || payload.PerfStats.SampleCount != 2 || payload.PerfStats.TotalPeriod != 40000 {
			t.Fatalf("native sample contract changed: %+v", payload.PerfStats)
		}
	})
	t.Run("absent_samples_do_not_create_weights", func(t *testing.T) {
		payload := query(base, "perf_stats")
		if p := payload.PerfStats; p != nil && (p.SampleCount != 0 || p.TotalPeriod != 0 || len(p.TopSymbols) != 0 || len(p.Cohorts) != 0) {
			t.Fatalf("invented sample weights: %+v", p)
		}
	})
	t.Run("two_cpus_are_not_one_wall_clock", func(t *testing.T) {
		body := "clock-1 (1) [000] .... 1.000000: cpu_frequency: state=1000000 cpu_id=0\n" +
			"clock-1 (1) [001] .... 1.000000: cpu_frequency: state=2000000 cpu_id=1\n" +
			"a-40 (40) [000] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=a next_pid=40 next_prio=120\n" +
			"b-41 (41) [001] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=b next_pid=41 next_prio=120\n" +
			"a-40 (40) [000] .... 1.010000: sched_switch: prev_comm=a prev_pid=40 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n" +
			"b-41 (41) [001] .... 1.010000: sched_switch: prev_comm=b prev_pid=41 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n"
		bus, _, _ := hmc17NamedPathContext(t)
		path := hmc081WriteTrace(t, body)
		params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "time_start": 1.0, "time_end": 1.010})
		result, err := registry.Execute(bus, "trace_query", params)
		if err != nil {
			t.Fatal(err)
		}
		payload := hmc17NamedPayload(t, result)
		if payload.WindowStats == nil || payload.WindowStats.ComputeSupplyBalance == nil {
			t.Fatalf("missing native supply balance: %+v", payload.WindowStats)
		}
		b := payload.WindowStats.ComputeSupplyBalance
		if b.CPUCount != 2 || math.Abs(b.WindowMs-10) > 1e-8 || math.Abs(b.NominalCapacityMs-20) > 1e-8 || math.Abs(b.DeliveredComputeMs-20) > 1e-8 {
			t.Fatalf("wall ms/CPU·ms conflated: %+v", b)
		}
	})
}

func TestTraceCapabilitiesPublicJankFieldContract(t *testing.T) {
	catalog := capabilityPublicCall(t, nil, `{"view":"event_search","detail":true}`)
	var contract map[string]any
	for _, raw := range catalog["metrics"].([]any) {
		if m := raw.(map[string]any); m["id"] == "reported_jank" {
			contract = m
		}
	}
	if contract == nil {
		t.Fatal("event_search catalog lost reported_jank")
	}
	filter := contract["filter"].(map[string]any)
	fields, _ := json.Marshal(filter["fields"])
	wantFields, _ := json.Marshal(tracequery.EventFieldFilterFields())
	ops, _ := json.Marshal(filter["operators"])
	wantOps, _ := json.Marshal(tracequery.EventFieldFilterOps())
	if string(fields) != string(wantFields) || string(ops) != string(wantOps) || filter["max_predicates"] != float64(tracequery.EventFieldFilterLimit) {
		t.Fatalf("field-filter contract drift: %+v", filter)
	}
	body := "app-40 (40) [001] .... 0.006000: tracing_mark_write: B|40|jank_event_sync: start_ts=3000000, end_ts=5000000, jank_frames=2, appid=40\n" +
		"app-40 (40) [001] .... 0.007000: tracing_mark_write: B|40|jank_event_sync: start_ts=3000000, end_ts=5000000, jank_frames=0, appid=40\n"
	bus, _, _ := hmc17NamedPathContext(t)
	path := hmc081WriteTrace(t, body)
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "event_search", "time_start": 0.0, "time_end": .01, "event_field_filters": []map[string]string{{"field": "jank_frames", "op": "gte", "value": "2"}}})
	r := NewRegistry()
	RegisterDefaults(r)
	result, err := r.Execute(bus, "trace_query", params)
	if err != nil {
		t.Fatal(err)
	}
	payload := hmc17NamedPayload(t, result)
	if len(payload.Events) != 1 || payload.Events[0].PluginFields == nil || payload.Events[0].JankEvent == nil || payload.Events[0].JankEvent.Values == nil {
		t.Fatalf("native typed filter failed: %+v", payload.Events)
	}
	v := payload.Events[0].JankEvent.Values
	if v.StartTSNS != 3000000 || v.EndTSNS != 5000000 || v.ReportedDurationNS != 2000000 || v.JankFrames != 2 || v.AppID != 40 || payload.Events[0].Ts != .006 || payload.Events[0].JankEvent.TimeDomainStatus != types.TraceJankSourceClock {
		t.Fatalf("native same-axis ns/seconds or reported count changed: %+v", payload.Events[0])
	}
}
