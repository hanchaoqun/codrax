package tracequery

import (
	"github.com/hanchaoqun/codrax/internal/tracewire"
	"strings"
)

// This memo is shared by timestamp/integrity/event consumers. Non-carrier
// physical rows pay only the prefix probe and never allocate a measurement.
func (s *lineScan) measureInterval() *tracewire.MeasureInterval {
	if !s.measureTried {
		s.measureTried = true
		if strings.HasPrefix(s.line, tracewire.MeasureIntervalPrefix) {
			if row, ok := tracewire.ParseMeasureInterval(s.line); ok {
				s.measure = &row
			}
		}
	}
	return s.measure
}

func MeasurementSourceTimestamp(event Event) (int64, bool) {
	if event.Type != EventMeasureInterval || event.PluginFields == nil || event.PluginFields.Measure == nil {
		return 0, false
	}
	return event.PluginFields.Measure.StartNS.Integer()
}

func measurementEventInTimeWindow(event Event, q Query) bool {
	startSet := !q.timeStartBackfilled && queryExplicitTimeStart(q)
	endSet := !q.timeEndBackfilled && queryExplicitTimeEnd(q)
	if !startSet && !endSet {
		return true
	}
	ts, known := MeasurementSourceTimestamp(event)
	if !known {
		return false
	}
	if startSet {
		start, ok := processMeasurementNS(q.TimeStart)
		if !ok || ts < start {
			return false
		}
	}
	if endSet {
		end, ok := processMeasurementNS(q.TimeEnd)
		if !ok || ts >= end {
			return false
		}
	}
	return true
}
