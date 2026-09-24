package skill

import (
	"strings"
	"testing"
)

func TestTraceBusinessTreeTeachingUsesOneWindowStatsSection(t *testing.T) {
	count := 0
	for _, row := range TraceQueryViewTeachings() {
		if strings.Contains(row.When, TraceBusinessTreeTeaching) {
			count++
			if row.View != "window_stats" {
				t.Fatalf("business tree taught as another view: %s", row.View)
			}
		}
		if row.View == "business_tree" {
			t.Fatal("result section became a query view")
		}
	}
	if count != 1 || strings.Count(RenderTraceQueryViewMatrix(), TraceBusinessTreeTeaching) != 1 {
		t.Fatal("tree contract must have one owner in the shared view matrix")
	}
	for _, want := range []string{
		"all emitters in begin-line order", "not target-filtered or duration-ranked", "omitted counts at each handoff",
		"same-source, same-thread synchronous B/E", "union of direct children before display limits",
		"Parent/child inclusive times must not be added", "disjoint segments",
		"unavailable, not zero", "sleep_io_wait_ms is included in sleep_ms",
		"window_unavailable_reason", "not placeholder query endpoints",
		"neither a complete call graph nor a wakeup/root-cause tree",
	} {
		if !strings.Contains(TraceBusinessTreeTeaching, want) {
			t.Errorf("lost measurement boundary %q", want)
		}
	}
}
