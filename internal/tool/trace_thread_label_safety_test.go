package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestTraceThreadLabelKeepsDecodedPerfControlsOnOneLine(t *testing.T) {
	label := traceThreadLabel(tracequery.ThreadRef{Comm: "Render\nworker\tphase\rnext", PID: 34})
	if label != "Render worker phase next-34" {
		t.Fatalf("thread label control rendering drift: %q", label)
	}
	if strings.ContainsAny(label, "\r\n\t") {
		t.Fatalf("decoded perf comm escaped the one-line tool result: %q", label)
	}
}

func TestTracePerfQualityPublishesInputIntegrityRepair(t *testing.T) {
	quality := &tracequery.PerfQualitySummary{
		InputIntegrityIssues: []tracequery.PerfValueCount{{Value: "cpu_duplicate_conflict", SampleCount: 1, Period: 1}},
	}
	var full strings.Builder
	writeTracePerfQuality(&full, "perf_quality", quality)
	if !strings.Contains(full.String(), "input_integrity=cpu_duplicate_conflict") {
		t.Fatalf("full perf quality hid input integrity: %q", full.String())
	}
	if compact := traceQueryPerfQualityCompact(quality); !strings.Contains(compact, "input_integrity=cpu_duplicate_conflict") {
		t.Fatalf("compact/typed-note perf quality hid input integrity: %q", compact)
	}
}
