package tracediag

import (
	"fmt"
	"strconv"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// These are exact projections, not reconstructions of omitted state. A zero
// is a value of the admitted interval population; nil remains unavailable.
func renderSchedulerConcurrencyDetail(value any, path string, emit func(string)) {
	seconds := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	switch item := value.(type) {
	case tracequery.SchedulerConcurrencyValues:
		emit(fmt.Sprintf("- %s: peak_threads=%d mean_threads=%s busy_ms=%s thread_ms=%s", path,
			item.PeakThreads, formatFloatToken(item.MeanThreads), formatFloatToken(item.BusyMs), formatFloatToken(item.ThreadMs)))
	case tracequery.SchedulerConcurrencySegment:
		emit(fmt.Sprintf("- %s: start_ts=%s end_ts=%s threads=%d", path, seconds(item.StartTs), seconds(item.EndTs), item.Threads))
	case tracequery.SchedulerConcurrencyWindow:
		emit(fmt.Sprintf("- %s: start_ts=%s end_ts=%s", path, seconds(item.StartTs), seconds(item.EndTs)))
	}
}
