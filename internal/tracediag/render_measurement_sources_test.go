package tracediag

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1638B2BMeasurementSourcesDetailDisposition(t *testing.T) {
	typ := reflect.TypeOf(types.TraceSchedulerMeasurementSources{})
	if typ.NumField() != 2 || typ.Field(0).Name != "Domains" || typ.Field(0).Tag.Get("json") != "domains,omitempty" ||
		typ.Field(1).Name != "HasUnknown" || typ.Field(1).Tag.Get("json") != "has_unknown,omitempty" {
		t.Fatal("new source collection field needs explicit diagnostic disposition")
	}
	for _, owner := range []reflect.Type{reflect.TypeOf(tracequery.ThreadDuration{}), reflect.TypeOf(tracequery.StateDrilldownStep{}),
		reflect.TypeOf(tracequery.RootCauseRankItem{}), reflect.TypeOf(tracequery.WakeupCausalImpact{}), reflect.TypeOf(tracequery.WakeupCausalAggregate{})} {
		f, ok := owner.FieldByName("MeasurementSources")
		if !ok || f.Type != reflect.PointerTo(typ) || f.Tag.Get("json") != "measurement_sources,omitempty" ||
			policySkipsDetailField(&nonEventDetailPolicy, owner, "MeasurementSources") {
			t.Fatalf("source inventory hidden or malformed on %s", owner)
		}
	}
	d := types.TraceSchedulerMeasurementDomain{Version: 1, Status: "constructed_partition", Method: "thread_timeline", TargetTID: 7,
		WindowStartTs: 6793224, WindowEndTs: 6793224.25, PartitionID: "first-partition"}
	s := types.TraceSchedulerMeasurementSourcesFromDomain(&d)
	d.PartitionID = "second-partition"
	s.Domains = append(s.Domains, d)
	s.HasUnknown = true
	r := tracequery.Result{RootCauseRank: &tracequery.RootCauseRankResult{Items: []tracequery.RootCauseRankItem{{MeasurementSources: s}}}}
	before, _ := json.Marshal(r)
	for _, policy := range []*detailRenderPolicy{nil, &nonEventDetailPolicy} {
		var lines []string
		renderResultDetailWithPolicy(&r, func(line string) { lines = append(lines, line) }, policy)
		out := strings.Join(lines, "\n")
		for _, want := range []string{"has_unknown=true", "first-partition", "second-partition", "window_start_ts=6793224.000000", "query_line_start=0"} {
			if !strings.Contains(out, want) {
				t.Errorf("missing source detail %q: %s", want, out)
			}
		}
		if strings.Contains(out, "e+") {
			t.Fatal("nested native coordinate escaped fixed-point renderer")
		}
	}
	after, _ := json.Marshal(r)
	if string(before) != string(after) {
		t.Fatal("diagnostic changed source inventory")
	}
}
