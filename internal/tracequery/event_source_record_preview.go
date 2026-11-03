package tracequery

import (
	"encoding/json"
	"strconv"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

// TraceEventSourceRecordPreview is a copy for a bounded visible-row display,
// not an observation, query receipt or source admission. An empty TimestampNS
// means unknown; the legitimate zero and signed source timestamps stay exact.
type TraceEventSourceRecordPreview struct {
	Fields      json.RawMessage
	TimestampNS string
	TimeRole    string
}

// ProjectTraceEventSourceRecordPreview covers native carriers which do not
// have scalar TraceEventSemantics. Reuse parser-owned typed fields and the two
// strict wire owners which deliberately keep their large objects out of the
// event index. Never decode an arbitrary base64 token or raw SQL archive block.
// Fields is freshly marshaled: the caller cannot mutate the original event.
func ProjectTraceEventSourceRecordPreview(event Event) TraceEventSourceRecordPreview {
	var value any
	var ns, role string
	unsignedTime := func(n uint64) string { return strconv.FormatUint(n, 10) }
	switch event.Type {
	case EventResourceStack:
		row, ok := tracewire.ParseResourceStack(event.FieldText)
		if !ok {
			return TraceEventSourceRecordPreview{}
		}
		value, ns, role = row, strconv.FormatInt(row.TimestampNS, 10), "resource_event_time"
	case EventCPUMeasureInterval:
		row, ok := tracewire.ParseCPUMeasureInterval(event.FieldText)
		if !ok {
			return TraceEventSourceRecordPreview{}
		}
		value, role = row, "source_interval_start"
		if row.StartNS != nil {
			ns = strconv.FormatInt(*row.StartNS, 10)
		}
	default:
		fields := event.PluginFields
		if fields == nil {
			return TraceEventSourceRecordPreview{}
		}
		switch event.Type {
		case EventFrameMap:
			if row := fields.FrameMap; row != nil {
				value, ns, role = row, unsignedTime(row.DestinationTimestampNS), "destination_frame_time"
			}
		case EventFrameCallstack:
			if row := fields.FrameCallstack; row != nil {
				value, ns, role = row, unsignedTime(row.TimestampNS), "frame_reference_time"
			}
		case EventFrameGPU:
			if row := fields.FrameGPU; row != nil {
				value, ns, role = row, unsignedTime(row.TimestampNS), "frame_reference_time"
			}
		case EventPerfNAPIAsync:
			if row := fields.PerfNAPIAsync; row != nil && utf8.ValidString(row.TraceID) {
				value, ns, role = row, unsignedTime(row.TimestampNS), "source_point_time"
			}
		case EventEBPFInterval:
			if row := fields.EBPFInterval; row != nil {
				value, ns, role = row, unsignedTime(row.TimestampNS), "source_interval_start"
			}
		}
	}
	if value == nil {
		return TraceEventSourceRecordPreview{}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return TraceEventSourceRecordPreview{}
	}
	return TraceEventSourceRecordPreview{Fields: data, TimestampNS: ns, TimeRole: role}
}
