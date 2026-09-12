package tool

import (
	"encoding/json"
	"testing"
)

func TestTraceQueryPublishesParentWindowIdentityB1626(t *testing.T) {
	ctx := suppCoreContext(t)
	result, err := (&TraceQuery{}).Execute(ctx, json.RawMessage(`{"view":"root_cause_rank","pid":200,"time_start":3.0,"time_end":3.2}`))
	if err != nil || !result.Success || len(result.Observations) == 0 {
		t.Fatalf("real trace query: %+v %v", result, err)
	}
	for _, record := range result.Observations {
		data, err := json.Marshal(record.SourceRef)
		if err != nil {
			t.Fatal(err)
		}
		var ref map[string]any
		if err := json.Unmarshal(data, &ref); err != nil {
			t.Fatal(err)
		}
		if ref["query_window_known"] != true || ref["query_window_start_ts"] != 3.0 || ref["query_window_end_ts"] != 3.2 || ref["query_target_pid"] != 200.0 {
			t.Fatalf("record %s lost its producer-owned parent query identity: %s", record.ID, data)
		}
		if ref["query_scope_id"] == "" || ref["path"] == "" {
			t.Fatal("window membership cannot replace capture/result provenance")
		}
	}
}
