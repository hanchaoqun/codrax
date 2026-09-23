package tracediag

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// WindowStats' slice fingerprint does not cover leaf fields. All resource
// coordinates stay in generic detail; none acquire a priority/root-cause lane.
func TestDiagResourceIdentityLeafDisposition(t *testing.T) {
	typ := reflect.TypeOf(tracequery.RuntimeResourceSummary{})
	var got []string
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		got = append(got, field.Name+"|"+field.Type.String()+"|"+field.Tag.Get("json"))
	}
	want := []string{
		"Kind|string|kind,omitempty", "Operation|string|operation,omitempty", "Path|string|path,omitempty", "Dev|string|dev,omitempty",
		"Thread|tracequery.ThreadRef|thread,omitempty", "Count|int|count,omitempty", "TotalLatencyMs|float64|total_latency_ms,omitempty",
		"MaxLatencyMs|float64|max_latency_ms,omitempty", "Bytes|int64|bytes,omitempty", "Address|string|address,omitempty",
		"Line|int|line,omitempty", "Ts|float64|ts,omitempty", "Example|string|example,omitempty", "Callstack|string|callstack,omitempty",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resource leaf needs explicit detail/authority review: %q", got)
	}
}

func TestDiagActualQueryKeepsResourceCoordinatesSeparate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resources.ftrace")
	const source = "app-20 (20) [001] .... 1.000000: page_fault_user: operation=major address=0x1234 dev=8,0 duration_us=150 size=4096\n"
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	result := tracequery.Run(idx, tracequery.Query{View: "window_stats", TimeStart: .99, TimeEnd: 1.01})
	before, _ := json.Marshal(result)
	body := renderStepBody(&Step{View: result.View, effMaxLines: 1000}, stepOutcome{result: &result})
	report := strings.Join(body.lines, "\n")
	for _, want := range []string{"page_fault_resources[0]", "address=0x1234", "dev=8,0", "total_latency_ms=0.150"} {
		if !strings.Contains(report, want) {
			t.Fatalf("diagnostic lost resource coordinate %q:\n%s", want, report)
		}
	}
	if strings.Contains(report, "path=0x1234") || strings.Contains(report, "path=8,0") {
		t.Fatalf("diagnostic reintroduced an invented path:\n%s", report)
	}
	after, _ := json.Marshal(result)
	if string(before) != string(after) {
		t.Fatal("rendering changed evidence")
	}
}
