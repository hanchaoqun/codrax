package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestHMC248ResourceSelectionTextKeepsOriginalCensus(t *testing.T) {
	p := tracequery.ResourceStackResult{Status: "available", TargetScope: "thread", MatchedEvents: 5, OmittedEvents: 3, Events: make([]tracequery.ResourceStackEvent, 2)}
	p.Events[0].RowID = 11
	p.Events[1].RowID = 22
	got := TraceResourceStackSelectedText(p, []int{1}, 12)
	for _, want := range []string{"原查询匹配事件=5", "原交接省略事件=3", "展示成员=1", "排除未匹配owner的已保留事件=1", "物理rowid=22", "省略事件=4"} {
		if !strings.Contains(got, want) {
			t.Errorf("lost %q", want)
		}
	}
	if strings.Contains(got, "物理rowid=11") || len(p.Events) != 2 || p.MatchedEvents != 5 || p.OmittedEvents != 3 {
		t.Fatal("selection changed the source population")
	}
	for _, invalid := range [][]int{{-1}, {2}, {1, 0}, {0, 0}} {
		if TraceResourceStackSelectedText(p, invalid, 12) != "" {
			t.Fatal("invalid member indices rendered")
		}
	}
}
