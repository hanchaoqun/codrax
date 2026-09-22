package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/types"
)

// A schema-valid model roster and validator-owned supplemental observations
// have separate bounds. The public emitter must not drop accepted tail rows.
func TestEmitPerfTracePublicPreservesSchemaLimitWithSystemObservations(t *testing.T) {
	emitter := &EmitPerfTrace{}
	var schema struct {
		Properties map[string]struct {
			MaxItems int `json:"maxItems"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(emitter.Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	if got := schema.Properties["observations"].MaxItems; got != 50 {
		t.Fatalf("model observation schema bound changed: got=%d want=50", got)
	}
	const timeRow = "app-100 (100) [000] .... 10.000000: tracing_mark_write: B|100|Work\n"
	const schedulerRow = "app-100 (100) [000] .... 10.001000: sched_wakeup: comm=worker pid=200 prio=120 target_cpu=000\n"
	for _, tc := range []struct {
		name, source, prefix string
		scoped               bool
		wantSystem           []string
	}{
		{name: "no_system_control", source: "unknown"},
		{name: "time_only", source: "systrace", prefix: timeRow, wantSystem: []string{"time_semantics"}},
		{name: "time_and_priority", source: "hitrace", prefix: timeRow + schedulerRow, wantSystem: []string{"priority_semantics", "time_semantics"}},
		{name: "scoped_time_and_priority", source: "hitrace", prefix: timeRow + schedulerRow, scoped: true, wantSystem: []string{"priority_semantics", "time_semantics"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var raw strings.Builder
			raw.WriteString(tc.prefix)
			params := emitPerfTraceParams{Meta: emitPerfTraceMeta{Source: tc.source}}
			var want []types.PerfObservation
			for i := 0; i < 50; i++ {
				subject := fmt.Sprintf("marker-%02d", i)
				line := strings.Count(raw.String(), "\n") + 1
				evidence := "# " + subject + " source text"
				raw.WriteString(evidence + "\n")
				row := emitPerfTraceObservation{Kind: "line_anchor", Subject: subject, Summary: "The source includes " + subject, Evidence: evidence, LineStart: line, LineEnd: line, Tags: []string{"marker", subject}, Confidence: 0.75}
				params.Observations = append(params.Observations, row)
				want = append(want, types.PerfObservation{Authority: types.PerfObservationAuthorityPreTriageModelExtraction, Kind: row.Kind, Subject: row.Subject, Summary: row.Summary, Evidence: row.Evidence, LineStart: line, LineEnd: line, Tags: row.Tags, Confidence: row.Confidence})
			}
			bus := &types.BusContext{Ctx: context.Background(), Mutable: types.NewMutableState("preserve source observations"), AttachedHitrace: raw.String()}
			if tc.scoped {
				bus.AttachedHitrace = "# outside prefix\n" + raw.String() + "# outside suffix\n"
				view, err := attachment.NewTraceExcerpt(bus.Ctx, bus.AttachedHitrace, nil, len("# outside prefix\n"), len("# outside prefix\n")+raw.Len())
				if err != nil {
					t.Fatal(err)
				}
				bus.AttachedTraceExcerpt = view
				_, scope, err := view.Resolve(bus.Ctx, bus.AttachedHitrace, nil)
				if err != nil {
					t.Fatal(err)
				}
				for i := range want {
					want[i].SourceScope = &scope
					want[i].LineStart += scope.LineStart - 1
					want[i].LineEnd += scope.LineStart - 1
				}
			}
			payload, err := json.Marshal(params)
			if err != nil {
				t.Fatal(err)
			}
			beforePayload, beforeTrace := string(payload), bus.AttachedHitrace
			result, err := emitter.Execute(bus, payload)
			if err != nil || !result.Success || bus.Mutable.PerfTrace() == nil {
				t.Fatalf("schema-valid 50-row emission rejected: result=%+v err=%v", result, err)
			}
			bundle := bus.Mutable.PerfTrace()
			if len(bundle.Observations) != 50+len(tc.wantSystem) {
				t.Errorf("system supplementation dropped accepted rows: got=%d want=%d", len(bundle.Observations), 50+len(tc.wantSystem))
			}
			for i, kind := range tc.wantSystem {
				if i >= len(bundle.Observations) || bundle.Observations[i].Kind != kind || bundle.Observations[i].Authority != types.PerfObservationAuthorityDeterministicValidator {
					t.Fatalf("system row order/authority changed at %d: %+v", i, bundle.Observations)
				}
			}
			if got := bundle.Observations[len(tc.wantSystem):]; !reflect.DeepEqual(got, want) {
				t.Errorf("complete model roster/order/authority changed: kept=%d want=50 (last expected subject=%s)", len(got), want[49].Subject)
			}
			if bus.AttachedHitrace != beforeTrace || string(payload) != beforePayload {
				t.Fatal("emission mutated caller-owned trace or payload")
			}
			if len(bundle.Frames)+len(bundle.Janks)+len(bundle.Stalls) != 0 || bundle.Startup != nil {
				t.Fatal("observation-only emission fabricated another structured measurement")
			}
		})
	}
}
