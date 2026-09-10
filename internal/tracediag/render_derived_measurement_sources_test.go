package tracediag

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1638B3DerivedSourceSchemaReview(t *testing.T) {
	// Complete pre-change fingerprints recorded before implementation. Only
	// the optional source pointer is added; every old field stays pinned.
	const added = "MeasurementSources|*types.TraceSchedulerMeasurementSources|measurement_sources,omitempty"
	for _, tc := range []struct {
		typ      reflect.Type
		previous string
	}{
		{reflect.TypeOf(tracequery.SchedulerLatencyItem{}), "309f8f539bf7d453d01d155d08cd8cbcf7e7c8c8e050f1fe41bc0d8868bda724"},
		{reflect.TypeOf(tracequery.CPUConstraintSummary{}), "bae309190defdeb3147c5c2872f391558aefcf860f7230775ffc57efedc449e2"},
		{reflect.TypeOf(tracequery.ComputeSupplySummary{}), "715e1b41cb4d7a0e04eac99dd5d3d0cd4ddc2744a5917d6676742fedb52aaf42"},
		{reflect.TypeOf(tracequery.RootEvidence{}), "407b6d974ffc19bcb67dde1ce7496bd4fde7fdf682de755f2866ac31bf155714"},
	} {
		t.Run(tc.typ.Name(), func(t *testing.T) {
			current, schema := detailSchemaFingerprint(tc.typ)
			t.Logf("owner=%s hash=%s schema=%s", tc.typ.Name(), current, schema)
			var prior []string
			count := 0
			for _, field := range strings.Split(schema, ";") {
				if field == added {
					count++
				} else {
					prior = append(prior, field)
				}
			}
			if count != 1 {
				t.Fatalf("expected exactly one optional source inventory on %s, got %d", tc.typ, count)
			}
			sum := sha256.Sum256([]byte(strings.Join(prior, ";")))
			if hex.EncodeToString(sum[:]) != tc.previous {
				t.Fatalf("a non-source field changed on %s: %s", tc.typ, schema)
			}
			if policySkipsDetailField(&nonEventDetailPolicy, tc.typ, "MeasurementSources") {
				t.Fatal("derived measurement source must remain visible in diagnostic detail")
			}
			// Build through reflection so absence is a behavioral RED, not a
			// compilation failure. Exercise the real generic/fixed-point walker.
			v := reflect.New(tc.typ)
			domain := types.TraceSchedulerMeasurementDomain{Version: 1, Status: "constructed_partition", Method: "off_cpu_sweep",
				TargetTID: 19, WindowStartTs: 6793224, WindowEndTs: 6793224.25, PartitionID: "derived-source", QueryLineEnd: 400}
			sources := types.TraceSchedulerMeasurementSourcesFromDomain(&domain)
			sources.HasUnknown = true
			v.Elem().FieldByName("MeasurementSources").Set(reflect.ValueOf(sources))
			r := &tracequery.Result{}
			switch item := v.Interface().(type) {
			case *tracequery.SchedulerLatencyItem:
				r.SchedulerLatency = &tracequery.SchedulerLatencyResult{Items: []tracequery.SchedulerLatencyItem{*item}}
			case *tracequery.CPUConstraintSummary:
				r.WindowStats = &tracequery.WindowStats{CPUConstraints: []tracequery.CPUConstraintSummary{*item}}
			case *tracequery.ComputeSupplySummary:
				r.WindowStats = &tracequery.WindowStats{ComputeSupply: []tracequery.ComputeSupplySummary{*item}}
			case *tracequery.RootEvidence:
				r.WakeupChain = &tracequery.ChainResult{RootEvidence: []tracequery.RootEvidence{*item}}
			}
			before, _ := json.Marshal(r)
			var lines []string
			renderResultDetailWithPolicy(r, func(line string) { lines = append(lines, line) }, &nonEventDetailPolicy)
			out := strings.Join(lines, "\n")
			for _, want := range []string{"has_unknown=true", "derived-source", "window_start_ts=6793224.000000", "query_line_end=400"} {
				if !strings.Contains(out, want) {
					t.Errorf("missing %q: %s", want, out)
				}
			}
			if strings.Contains(out, "e+") {
				t.Fatal("derived coordinates escaped fixed-point rendering")
			}
			after, _ := json.Marshal(r)
			if string(before) != string(after) {
				t.Fatal("diagnostic mutated source inventory")
			}
		})
	}
}
