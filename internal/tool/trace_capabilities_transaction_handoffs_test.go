package tool

import (
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestTraceCapabilitiesPublicTransactionPopulationContracts(t *testing.T) {
	out := capabilityPublicCall(t, nil, `{"view":"transaction_handoffs","detail":true}`)
	views := out["views"].([]any)
	metrics, ok := out["metrics"].([]any)
	if len(views) != 1 || !ok || len(metrics) != 1 {
		t.Fatalf("missing exact transaction catalog: %+v", out)
	}
	view, metric := views[0].(map[string]any), metrics[0].(map[string]any)
	if !reflect.DeepEqual(view["metric_refs"], []any{tracequery.ViewTransactionHandoffs}) || metric["id"] != tracequery.ViewTransactionHandoffs {
		t.Fatalf("transaction counts reference another measurement family: %+v", out)
	}
	if !reflect.DeepEqual(metric["limitations"], []any{tracequery.TransactionHandoffsTeaching}) || !reflect.DeepEqual(view["limitations"], metric["limitations"]) {
		t.Fatal("catalog forked the protocol's published-source and noncausal boundary")
	}
	fields := map[string]bool{}
	for _, raw := range metric["outputs"].([]any) {
		output := raw.(map[string]any)
		if output["unit"] != "count" || output["caliber"] == "" {
			t.Fatalf("transaction population invented a duration or lacks its denominator: %+v", output)
		}
		for _, field := range output["fields"].([]any) {
			key := output["section"].(string) + "." + field.(string)
			if fields[key] {
				t.Fatalf("duplicate population %s", key)
			}
			fields[key] = true
		}
	}
	want := []string{
		"transaction_handoffs.total_keys", "transaction_handoffs.omitted_keys",
		"transaction_handoffs.window_submission_events", "transaction_handoffs.window_consumption_events",
		"transaction_handoffs.handoffs.submission_count", "transaction_handoffs.handoffs.consumption_count",
		"transaction_handoffs.handoffs.window_submissions", "transaction_handoffs.handoffs.window_consumptions",
		"transaction_handoffs.handoffs.omitted_submissions", "transaction_handoffs.handoffs.omitted_consumptions",
	}
	for _, field := range want {
		if !fields[field] {
			t.Errorf("missing population %s", field)
		}
	}
	if len(fields) != len(want) {
		t.Fatalf("unexpected transaction measures: %+v", fields)
	}
}
