package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRuntimeEventActualFinalizerHandoff236(t *testing.T) {
	results := traceEventInventoryPublicResults(t, 30, "2")
	before, _ := json.Marshal(results)
	ctx := traceEventInventoryPublicContext(results)
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, required := range []string{"event_selection", "native_event_fields event-fields-1=", "selector_row_ids", "jank.appid", "jank.start_ts_ns", "replace field_keys with group_by", "not event identity across queries"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("missing native selector teaching %q", required)
		}
	}
	if strings.Contains(prompt, types.TraceNoteKeyRuntimeMeasurement+"=") {
		t.Fatal("duplicated raw publication consumed prompt budget")
	}
	after, _ := json.Marshal(results)
	if string(before) != string(after) {
		t.Fatal("presentation mutated raw receipts")
	}
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	found := 0
	for _, table := range view.RuntimeMeasurementContract.Choices() {
		if table.EventData == nil {
			continue
		}
		found++
		if len(table.EventData.Rows) != len(table.Rows) || len(table.EventData.Fields) != len(table.Columns) {
			t.Fatal("preview IDs and columns are not aligned")
		}
		if !strings.Contains(prompt, "observation_id=\""+table.ObservationID+"\"") {
			t.Fatal("native table not offered")
		}
	}
	if found == 0 {
		t.Fatal("test did not reach native event provider")
	}
}
