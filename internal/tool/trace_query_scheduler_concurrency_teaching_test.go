package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
)

func TestSchedulerConcurrencyTeachingSharesOneWindowStatsContract(t *testing.T) {
	tool := &TraceQuery{}
	var schema struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(tool.Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	var window string
	for _, row := range skill.TraceQueryViewTeachings() {
		if row.View == "window_stats" {
			window = row.When
		}
	}
	for label, text := range map[string]string{"description": tool.Description(), "schema": schema.Properties["view"].Description, "workflow": window} {
		if strings.Count(text, skill.TraceSchedulerConcurrencyTeaching) != 1 {
			t.Errorf("%s must contain exactly one shared contract", label)
		}
	}
	for _, part := range []string{"confirmed intervals only", "query_pid is context", "full query window", "thread·ms", "open tails", "on-chain"} {
		if !strings.Contains(skill.TraceSchedulerConcurrencyTeaching, part) {
			t.Errorf("missing measurement/authority boundary %q", part)
		}
	}
}
