package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceWaitPrevStateProducerKeepsObjectIdentity(t *testing.T) {
	for _, raw := range []string{"D", "S", ""} {
		t.Run("raw_"+raw, func(t *testing.T) {
			var account tracequery.TargetWindowStateAccount
			data := `{"window":{"start_ts":1,"end_ts":2},"wait_occurrence_status":"complete","wait_occurrence_total":1,"wait_occurrence_emitted":1,"wait_occurrences":[{"ordinal":1,"state":"io_wait","start_ts":1.1,"end_ts":1.101,"duration_ms":1,"prev_state_raw":"` + raw + `","io_wait":true,"io_wait_known":true,"caller":"wait_site","reason_line":3}]}`
			if err := json.Unmarshal([]byte(data), &account); err != nil {
				t.Fatal(err)
			}
			rows := traceQueryTargetWindowWaitOccurrenceObservations(&account, "app-1", types.ObservationSourceRef{}, "scope", "now")
			if len(rows) != 2 || rows[1].Object != "state=io_wait;iowait=1;caller=wait_site;reason_line=3" || rows[1].Value != "1.000" {
				t.Fatalf("producer changed original identity/value: %+v", rows)
			}
			preview := strings.Join(rows[0].RichNotes, "\n")
			leaf := strings.Join(rows[1].RichNotes, "\n")
			if raw == "" {
				if strings.Contains(preview, "prev_state_raw=") || strings.Contains(leaf, "prev_state_raw=") {
					t.Fatal("missing native state was inferred")
				}
			} else if !strings.Contains(preview, " prev_state_raw="+raw) || !strings.Contains(leaf, "target_wait_occurrence_prev_state_raw="+raw) {
				t.Errorf("native state missing from compact/full paths: preview=%q leaf=%q", preview, leaf)
			}
		})
	}
}
