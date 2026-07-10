package tracediag

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// Berlin's reusable template is a strict v2 contract: one CLI window and one
// CLI TID, with the TID consumed only by the target aggregate. Every raw lane
// is globally scoped and trace markers use parser-validated exact actions,
// never payload substring patterns or a hand-edited payload PID.
func TestBerlinWitnessV2TemplateParseAndScope(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "tracediag", "collect_berlin_pairing_witness.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseScript(data); err == nil || !strings.Contains(err.Error(), "requires --trace-window") {
		t.Fatalf("ParseScript must fail loud without the required window, got %v", err)
	}
	if _, err := parseScript(data, ScriptOverrides{Window: "100..101"}); err == nil || !strings.Contains(err.Error(), "requires --trace-tid") {
		t.Fatalf("Berlin template must fail loud without the required TID, got %v", err)
	}
	script, err := parseScript(data, ScriptOverrides{Window: "100..101", TID: "00042"})
	if err != nil {
		t.Fatal(err)
	}
	if script.Version != ScriptVersionV2 || script.Inputs == nil || script.Inputs.Window != "required" || script.Inputs.TID != "required" {
		t.Fatalf("Berlin typed inputs/version = version:%d inputs:%+v", script.Version, script.Inputs)
	}
	if script.v2Limits.MaxGeneratedWindows != 2 || script.v2Limits.MaxExpandedSteps != 11 || script.v2Limits.MaxReportLines != 1000 || script.v2WorstReportLines != 910 {
		t.Fatalf("Berlin mechanical budget drift: limits=%+v worst=%d", script.v2Limits, script.v2WorstReportLines)
	}
	if len(script.Discoveries) != 1 {
		t.Fatalf("Berlin discoveries=%d, want one pairing discovery", len(script.Discoveries))
	}
	discovery := script.Discoveries[0]
	if discovery.Label != "io_pairing_windows" || discovery.Strategy != string(tracequery.WindowDiscoveryPairingIntegrity) ||
		!reflect.DeepEqual(discovery.Families, []string{"block", "storage"}) || discovery.MaxWindows != 2 || discovery.MaxWindowMS > 50 || !discovery.windowInherited {
		t.Fatalf("Berlin pairing discovery = %+v", discovery)
	}

	wantActions := map[string][]string{
		"raw_sync_marks":     {"B", "E"},
		"raw_counter_marks":  {"C"},
		"raw_async_starts":   {"S"},
		"raw_async_finishes": {"F"},
		"raw_track_marks":    {"G", "H"},
		"raw_instant_marks":  {"N", "I"},
	}
	foundActions := map[string]bool{}
	for i := range script.Steps {
		step := script.Steps[i]
		if step.Label == "target_window_stats" {
			if step.View != "window_stats" || step.PIDFrom != "tid" || step.PID != 42 || step.Thread != "" || !step.windowInherited {
				t.Fatalf("Berlin target selector/window = %+v", step)
			}
			continue
		}
		if step.PID != 0 || step.PIDFrom != "" || step.Thread != "" {
			t.Errorf("Berlin raw step %s inherited target selector: pid=%d pid_from=%q thread=%q", step.Label, step.PID, step.PIDFrom, step.Thread)
		}
		if globals := tracequery.CPUGlobalEventSearchTypes(toTraceEventTypes(step.EventTypes)); len(globals) > 0 && (step.PID > 0 || step.Thread != "") {
			t.Errorf("Berlin CPU-global lane %s is selector-polluted: %+v", step.Label, step)
		}
		if actions, ok := wantActions[step.Label]; ok {
			foundActions[step.Label] = true
			if step.View != tracequery.FallbackViewEventSearch || !reflect.DeepEqual(step.EventTypes, []string{string(tracequery.EventTraceMark)}) ||
				!reflect.DeepEqual(step.TraceMarkActions, actions) || step.Pattern != "" {
				t.Errorf("Berlin exact marker lane %s = %+v, want actions=%v and no pattern", step.Label, step, actions)
			}
		}
	}
	if !reflect.DeepEqual(foundActions, map[string]bool{
		"raw_sync_marks": true, "raw_counter_marks": true, "raw_async_starts": true,
		"raw_async_finishes": true, "raw_track_marks": true, "raw_instant_marks": true,
	}) {
		t.Fatalf("Berlin exact marker lanes = %v", foundActions)
	}
	if len(script.Steps) != 10 {
		t.Fatalf("Berlin logical steps=%d, want 10", len(script.Steps))
	}
	interrupt := findBerlinStep(script.Steps, "raw_interrupt_endpoints")
	if interrupt == nil || !reflect.DeepEqual(interrupt.EventTypes, []string{"irq", "softirq", "ipi"}) {
		t.Fatalf("Berlin interrupt lane = %+v", interrupt)
	}
	unknown := findBerlinStep(script.Steps, "raw_unknown_events")
	if unknown == nil || !reflect.DeepEqual(unknown.EventTypes, []string{"unknown"}) {
		t.Fatalf("Berlin unknown lane = %+v", unknown)
	}
	ioRows := findBerlinStep(script.Steps, "raw_io_pairing_rows")
	if ioRows == nil || ioRows.WindowsFrom == nil || ioRows.WindowsFrom.Discovery != discovery.Label || ioRows.Window != "" {
		t.Fatalf("Berlin IO fan-out lane = %+v", ioRows)
	}
}

func toTraceEventTypes(values []string) []tracequery.EventType {
	out := make([]tracequery.EventType, len(values))
	for i := range values {
		out[i] = tracequery.EventType(values[i])
	}
	return out
}

func findBerlinStep(steps []Step, label string) *Step {
	for i := range steps {
		if steps[i].Label == label {
			return &steps[i]
		}
	}
	return nil
}
