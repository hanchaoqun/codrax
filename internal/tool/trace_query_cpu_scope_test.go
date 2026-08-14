package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestThreadCPULoadPublishesRepresentativeNonExclusiveCPUScope(t *testing.T) {
	item := tracequery.ThreadCPULoadSummary{
		Thread:         tracequery.ThreadRef{PID: 17267, Comm: ".ugc.aweme.lite"},
		RunningMs:      157.248,
		RunnableWaitMs: 5.604,
		CPU:            12,
		CoreClass:      "big",
		Frequency:      2075000,
	}

	for name, got := range map[string]string{
		"typed summary": traceQueryTypedThreadCPULoadSummary(item),
		"text banner": func() string {
			var b strings.Builder
			writeTraceThreadCPULoad(&b, item)
			return b.String()
		}(),
	} {
		if !strings.Contains(got, "cpu=12") || !strings.Contains(got, "cpu_scope=dominant_state_slice_representative_not_exclusive") {
			t.Fatalf("%s left representative CPU semantics ambiguous: %s", name, got)
		}
	}
}
