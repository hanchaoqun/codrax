package tracequery

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestB1598CoverageReaderExplainsOnlyExistingCounts(t *testing.T) {
	for _, zh := range []bool{true, false} {
		if got := FormatEventSearchCoverageForReaders(nil, nil, zh); got != "" {
			t.Fatalf("absent coverage gained a reader claim: %q", got)
		}
		for _, complete := range []bool{true, false} {
			coverage := &EventSearchCoverage{MatchedTotal: 4, Emitted: 4, EnumerationComplete: complete}
			before, _ := json.Marshal(coverage)
			shown := 1
			text := FormatEventSearchCoverageForReaders(coverage, &shown, zh)
			after, _ := json.Marshal(coverage)
			if string(before) != string(after) || shown != 1 {
				t.Fatal("reader formatter changed the existing wire values")
			}
			want := "Match counting is not confirmed complete"
			if complete {
				want = "Match counting is complete"
			}
			shownWord := "this report displays 1 of 4 returned rows"
			if zh {
				want = "匹配统计尚未确认完整"
				if complete {
					want = "匹配统计已完成"
				}
				shownWord = "本报告展示 1 / 4 条已返回记录"
			}
			if !strings.Contains(text, want) || !strings.Contains(text, shownWord) {
				t.Fatalf("equal counts must retain completion uncertainty and report scope: %s", text)
			}
			if strings.Contains(text, "scope_complete") || strings.Contains(text, "enumeration_complete") {
				t.Fatalf("reader explanation repeated a raw control key: %s", text)
			}
		}
	}
}
