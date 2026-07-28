package tracequery

// TraceMarkPayloadVerdict is the exported, source-neutral verdict for one
// complete trace-marker endpoint payload. Conversion code uses this verdict
// instead of carrying a second B/E/S/F/G/H/N/I grammar.
//
// Recognized means the payload declares one governed endpoint action, even
// when its schema is malformed. Admitted is true only when the single
// parseTraceMarkValidated authority accepted the complete payload.
type TraceMarkPayloadVerdict struct {
	Recognized   bool
	Admitted     bool
	Action       string
	SpanPID      int
	Name         string
	Track        string
	Value        string
	InvalidCause string
}

// DecodeTraceMarkEndpointPayload delegates directly to the same complete
// payload authority used by trace query. Counter and unknown actions are not
// endpoint verdicts and therefore return Recognized=false.
func DecodeTraceMarkEndpointPayload(payload string) TraceMarkPayloadVerdict {
	parsed := parseTraceMarkValidated(payload)
	if parsed.invalidAction != traceMarkActionValid {
		return TraceMarkPayloadVerdict{
			Recognized:   true,
			Action:       parsed.invalidAction.String(),
			InvalidCause: parsed.invalidReason.String(),
		}
	}
	switch parsed.action {
	case "B", "E", "S", "F", "G", "H", "N", "I":
		return TraceMarkPayloadVerdict{
			Recognized: true,
			Admitted:   true,
			Action:     parsed.action,
			SpanPID:    parsed.spanPID,
			Name:       parsed.name,
			Track:      parsed.track,
			Value:      parsed.value,
		}
	default:
		return TraceMarkPayloadVerdict{}
	}
}
