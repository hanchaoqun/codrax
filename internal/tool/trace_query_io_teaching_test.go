package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
)

func TestTraceQueryPublicSurfacesShareIORequestLatencyTeaching(t *testing.T) {
	tool := &TraceQuery{}
	var schema struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(tool.Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{
		"tool_description": tool.Description(),
		"view_description": schema.Properties["view"].Description,
	} {
		if count := strings.Count(text, skill.TraceIORequestLatencyDistributionTeaching); count != 1 {
			t.Errorf("%s must publish the shared IO request-latency teaching exactly once; got %d", name, count)
		}
	}
}
