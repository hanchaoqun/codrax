package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
)

func TestJankSourceClockPublicFinalizerAndSchema(t *testing.T) {
	ctx := traceEventInventoryPublicContext(traceEventInventoryPublicResults(t, 20, "2"))
	before, _ := json.Marshal(answerDocObservationLedger(ctx))
	for lane, prompt := range map[string]string{
		"ledger":  renderAnswerDocObservationLedger(ctx),
		"initial": (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil),
	} {
		for _, want := range []string{`"time_domain_status":"source_trace_clock"`, `"start_ts_ns":"9007199354740993"`, `"matched_total":3`, skill.TraceJankClockContract} {
			if !strings.Contains(prompt, want) {
				t.Errorf("%s handoff lost %q", lane, want)
			}
		}
		for _, old := range []string{"An unverified native-to-trace clock mapping cannot be inferred from proximity", "no header-clock alignment"} {
			if strings.Contains(prompt, old) {
				t.Errorf("%s retained contrary default teaching %q", lane, old)
			}
		}
	}
	after, _ := json.Marshal(answerDocObservationLedger(ctx))
	if string(before) != string(after) {
		t.Fatal("rendering changed accepted values/clock state")
	}
	var schema struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal((&tool.TraceQuery{}).Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	contract := schema.Properties["event_field_filters"].Description
	if contract != skill.TraceJankQueryContract {
		t.Fatal("schema and exploration teaching differ")
	}
	for _, want := range []string{"same source Trace time axis", "1,000,000,000", "Legacy unverified receipts", "different captures", "Only proven chain evidence"} {
		if !strings.Contains(contract, want) {
			t.Errorf("missing field-specific boundary %q", want)
		}
	}
	for _, old := range []string{"Never assume those clocks agree", "establish the target thread and time-domain mapping from independent evidence"} {
		if strings.Contains(contract, old) {
			t.Errorf("schema retains conflicting instruction %q", old)
		}
	}
}
