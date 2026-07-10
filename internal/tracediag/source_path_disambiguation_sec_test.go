package tracediag

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// SEC 捎带⑤ pin: source_path renders basename-only (SEC #27), and when one
// result carries DISTINCT source paths sharing a basename (CMP dual-trace
// comparison shape), each token carries a short non-path disambiguator so
// the two artifacts stay tellable-apart; distinct basenames stay plain.
func TestSourcePathBasenameCollisionDisambiguated(t *testing.T) {
	res := &tracequery.Result{
		View: "event_search",
		TraceArtifacts: []tracequery.TraceArtifactSource{
			{SourcePath: "/Users/alice/before/trace.systrace", Kind: "systrace"},
			{SourcePath: "/Users/alice/after/trace.systrace", Kind: "systrace"},
			{SourcePath: "/Users/alice/other/unique.systrace", Kind: "systrace"},
		},
	}
	step := &Step{Label: "cmp", View: "event_search", effMaxLines: 100}
	body := renderStepBody(step, stepOutcome{result: res})
	report := strings.Join(body.lines, "\n")

	if strings.Contains(report, "/Users/alice") {
		t.Fatalf("absolute path leaked into rendered body:\n%s", report)
	}
	// Colliding basename: both rows must carry a disambiguator.
	collided := 0
	for _, line := range body.lines {
		if strings.Contains(line, "source_path=trace.systrace(") {
			collided++
		}
		if strings.Contains(line, "source_path=trace.systrace ") || strings.HasSuffix(line, "source_path=trace.systrace") {
			t.Fatalf("colliding basename rendered without disambiguator: %s", line)
		}
	}
	if collided != 2 {
		t.Fatalf("want 2 disambiguated trace.systrace rows, got %d:\n%s", collided, report)
	}
	// Unique basename stays plain.
	if !strings.Contains(report, "source_path=unique.systrace") || strings.Contains(report, "unique.systrace(") {
		t.Fatalf("unique basename must render plain:\n%s", report)
	}
	// Render state must reset after the step body.
	if sourcePathAmbiguousBases != nil {
		t.Fatalf("sourcePathAmbiguousBases not reset after renderStepBody")
	}
}
