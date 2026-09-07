package tracequery

import "fmt"

// FormatEventSearchCoverageForReaders explains the existing census/display
// distinction without changing coverage or granting enumeration authority.
// reportEmitted is optional: a report can trim rows already returned by the
// engine. Keep that third count separate from both the match census and the
// engine's display limit. No caller should parse this reader-facing text.
func FormatEventSearchCoverageForReaders(coverage *EventSearchCoverage, reportEmitted *int, zh bool) string {
	if coverage == nil {
		return ""
	}
	var text string
	if zh {
		if coverage.EnumerationComplete {
			text = fmt.Sprintf("当前扫描范围内匹配统计已完成；引擎返回 %d / %d 条匹配记录", coverage.Emitted, coverage.MatchedTotal)
		} else {
			text = fmt.Sprintf("当前扫描范围内匹配统计尚未确认完整；目前已计数 %d 条匹配记录，引擎返回 %d 条", coverage.MatchedTotal, coverage.Emitted)
		}
		if reportEmitted != nil {
			text += fmt.Sprintf("；本报告展示 %d / %d 条已返回记录", *reportEmitted, coverage.Emitted)
		}
		return text + "；统计完成不代表匹配记录已全部返回或展示。"
	}
	if coverage.EnumerationComplete {
		text = fmt.Sprintf("Match counting is complete for the current scan scope; engine returned %d of %d matched rows", coverage.Emitted, coverage.MatchedTotal)
	} else {
		text = fmt.Sprintf("Match counting is not confirmed complete for the current scan scope; %d matches counted so far; engine returned %d rows", coverage.MatchedTotal, coverage.Emitted)
	}
	if reportEmitted != nil {
		text += fmt.Sprintf("; this report displays %d of %d returned rows", *reportEmitted, coverage.Emitted)
	}
	return text + ". Counting completion does not mean all matched rows were returned or displayed."
}
