package tracediag

import (
	"fmt"
	"strconv"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// These exact typed values have already passed engine admission. Preserve
// measured zero and sub-microsecond coordinates; nil optional measurements
// never reach this renderer. No value, scope or pairing authority is inferred.
func renderIOInFlightDetail(value any, path string, emit func(string)) {
	seconds := func(value float64) string { return strconv.FormatFloat(value, 'f', -1, 64) }
	switch item := value.(type) {
	case tracequery.IOInFlightValues:
		emit(fmt.Sprintf("- %s: peak_requests=%d mean_requests=%s busy_ms=%s request_ms=%s", path,
			item.PeakRequests, formatFloatToken(item.MeanRequests), formatFloatToken(item.BusyMs), formatFloatToken(item.RequestMs)))
	case tracequery.IOInFlightSegment:
		emit(fmt.Sprintf("- %s: start_ts=%s end_ts=%s requests=%d", path, seconds(item.StartTs), seconds(item.EndTs), item.Requests))
	case tracequery.IOInFlightWindow:
		emit(fmt.Sprintf("- %s: start_ts=%s end_ts=%s", path, seconds(item.StartTs), seconds(item.EndTs)))
	}
}
