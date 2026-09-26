package tracequery

import (
	"encoding/json"
	"strconv"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/tracewire"
	"github.com/hanchaoqun/codrax/internal/types"
)

// ProjectTraceEventSemantics copies already parsed display semantics only.
// It never reparses Raw/FieldText, creates intervals, resolves owners or grants
// causal/pairing authority. The native parser remains the single admission site.
func ProjectTraceEventSemantics(event Event) *types.TraceEventSemantics {
	p := traceEventSemanticProjector{value: types.TraceEventSemantics{SchemaVersion: types.TraceEventSemanticsVersion}}
	switch event.Type {
	case EventAbilityMonitor, EventXPower, EventHiSystemEvent:
		if event.PluginFields == nil {
			return nil
		}
		p.plugin(event)
	case EventTraceMark:
		if event.SpanAction == "" {
			return nil
		}
		p.marker(event)
	default:
		return nil
	}
	return p.finish()
}

type traceEventSemanticProjector struct{ value types.TraceEventSemantics }

func (p *traceEventSemanticProjector) known(key, value string) {
	d, ok := types.LookupTraceEventSemanticDescriptor(key)
	if !ok {
		return
	}
	field := types.TraceEventSemanticField{Key: key, Type: d.Type, Unit: d.Unit, Status: "known", Value: &value}
	if !utf8.ValidString(value) {
		field.Status, field.Value, field.IssueReason = "invalid", nil, "invalid_utf8"
	} else if len(value) > types.TraceEventSemanticValueByteLimit {
		field = types.OmitTraceEventSemanticValue(field, "value_exceeds_limit")
	}
	p.value.Fields = append(p.value.Fields, field)
}

func (p *traceEventSemanticProjector) unknown(key, status, reason string) {
	d, ok := types.LookupTraceEventSemanticDescriptor(key)
	if !ok {
		return
	}
	p.value.Fields = append(p.value.Fields, types.TraceEventSemanticField{Key: key, Type: d.Type, Unit: d.Unit, Status: status, IssueReason: reason})
}

func (p *traceEventSemanticProjector) optional(key, value string) {
	if value == "" {
		p.unknown(key, "unavailable", "not_recorded")
		return
	}
	p.known(key, value)
}

func (p *traceEventSemanticProjector) plugin(event Event) {
	pl := event.PluginFields
	if pl.HiSysEvent != nil {
		p.hiSysEvent(*pl.HiSysEvent)
		return
	}
	p.known("source.representation", "parsed_plugin_fields")
	p.optional("plugin.domain", pl.Domain)
	// The historical parser falls back to the raw tracepoint name. Equality
	// cannot prove whether that name also occurred in the original payload.
	// Keep the exact parsed label without promoting it to an observed name.
	if pl.EventName != "" && pl.EventName == event.Name {
		p.unknown("plugin.event_name", "unavailable", "source_field_presence_unverified")
		p.known("plugin.event_label", pl.EventName)
	} else {
		p.optional("plugin.event_name", pl.EventName)
	}
	p.optional("plugin.metric", pl.Metric)
	p.optional("plugin.value", pl.Value)
	p.optional("plugin.category", pl.Category)
	if pl.Contents != nil {
		p.known("plugin.contents", *pl.Contents)
	}
}

func (p *traceEventSemanticProjector) hiSysEvent(event tracewire.HiSysEvent) {
	p.known("source.representation", "sql_hisysevent")
	p.known("source.timestamp_ns", strconv.FormatInt(event.TimestampNS, 10))
	if event.SourceTID == nil {
		p.unknown("source.tid", "unavailable", "null_source_tid")
	} else {
		p.known("source.tid", strconv.FormatInt(*event.SourceTID, 10))
	}
	for _, field := range []struct {
		key, referenceKey string
		value             tracewire.HiSysEventName
	}{
		{"plugin.domain", "plugin.domain_ref", event.Domain},
		{"plugin.event_name", "plugin.event_name_ref", event.Event},
	} {
		if field.value.Status == "resolved" && field.value.Name != nil {
			p.known(field.key, *field.value.Name)
		} else {
			status, reason := "unavailable", field.value.Status
			if reason == "invalid_reference_storage_class" {
				status = "invalid"
			}
			if reason == "" || reason == "resolved" {
				status, reason = "invalid", "invalid_name_state"
			}
			p.unknown(field.key, status, reason)
		}
		if field.value.Reference != nil {
			p.known(field.referenceKey, strconv.FormatInt(*field.value.Reference, 10))
		}
	}
	p.known("source.contents_storage_class", event.Contents.StorageClass)
	if event.Contents.StorageClass == "blob" {
		p.known("source.contents_base64", event.Contents.BytesBase64)
	} else if event.Contents.Text != nil {
		p.known("source.contents", *event.Contents.Text)
	} else {
		p.unknown("source.contents", "unavailable", "null_contents")
	}
}

func (p *traceEventSemanticProjector) marker(event Event) {
	p.known("source.representation", "parsed_trace_marker")
	p.known("marker.action", event.SpanAction)
	if event.SpanAction == "C" {
		p.counter(event)
		return
	}
	if event.SpanAction == "E" {
		// An E row carries no independent business name. A bare E has no
		// payload PID; the old Event does not distinguish it from E|0.
		p.unknown("marker.name", "unavailable", "not_carried_by_end_marker")
	} else {
		p.optional("marker.name", event.SpanName)
	}
	if event.SpanAction == "E" && event.SpanPID == 0 {
		p.unknown("marker.payload_pid", "unavailable", "source_field_presence_unverified")
	} else {
		p.known("marker.payload_pid", strconv.Itoa(event.SpanPID))
	}
	if event.PluginFields != nil && event.PluginFields.SpanTrack != "" {
		p.known("marker.track", event.PluginFields.SpanTrack)
	}
	if event.SpanValue != "" {
		p.known("marker.value", event.SpanValue)
	}
}

func (p *traceEventSemanticProjector) counter(event Event) {
	var c *TraceCounterFields
	if event.PluginFields != nil {
		c = event.PluginFields.Counter
	}
	if c == nil || !c.Parsed {
		p.unknown("marker.name", "unavailable", "parser_receipt_unavailable")
		p.unknown("marker.payload_pid", "unavailable", "parser_receipt_unavailable")
		p.unknown("counter.value", "unavailable", "parser_receipt_unavailable")
		if event.SpanValue != "" {
			p.known("counter.raw_value", event.SpanValue)
		}
		return
	}
	if c.IdentityValid {
		p.known("marker.name", event.SpanName)
		p.known("marker.payload_pid", strconv.Itoa(event.SpanPID))
		p.known("counter.owner_scope", c.OwnerScope)
	} else {
		reason := c.IssueReason
		if reason == "" {
			reason = "invalid_counter_identity"
		}
		p.unknown("marker.name", "invalid", reason)
		p.unknown("marker.payload_pid", "invalid", reason)
	}
	// A native int64 outside exact binary64 range is still an exact inventory
	// number. It does NOT become eligible for the existing float aggregator.
	_, integerValued, exactInteger := traceCounterExactInteger(event.SpanValue)
	valueKnown := c.IdentityValid && (c.NumericValid || c.IssueReason == "numeric_precision_unsafe" && integerValued && exactInteger)
	if valueKnown {
		p.known("counter.value", event.SpanValue)
	} else {
		reason := c.IssueReason
		if reason == "" {
			reason = "invalid_counter_value"
		}
		p.unknown("counter.value", "invalid", reason)
		if event.SpanValue != "" {
			p.known("counter.raw_value", event.SpanValue)
		}
	}
	if c.NumericValid {
		p.known("counter.aggregation_status", "admitted")
	} else {
		p.known("counter.aggregation_status", "excluded")
	}
	if c.IssueReason != "" {
		p.known("counter.issue", c.IssueReason)
	}
	if c.Metadata != "" {
		p.known("counter.metadata", c.Metadata)
	}
	if c.OutputLevel != "" {
		p.known("counter.output_level", c.OutputLevel)
	}
	if c.TagBits != "" {
		p.known("counter.tag_bits", c.TagBits)
	}
}

func (p *traceEventSemanticProjector) finish() *types.TraceEventSemantics {
	if len(p.value.Fields) == 0 {
		return nil
	}
	// A deterministic fixed-registry projection is naturally field-bounded.
	// Escaped text can still expand JSON, so reduce largest complete values
	// to omission receipts until the whole typed envelope fits its bound.
	for {
		wire, err := json.Marshal(&p.value)
		if err != nil {
			return nil
		}
		if len(wire) <= types.TraceEventSemanticsByteLimit {
			break
		}
		largest := -1
		for n, field := range p.value.Fields {
			if field.Value != nil && len(*field.Value) > 0 && (largest < 0 || len(*field.Value) > len(*p.value.Fields[largest].Value)) {
				largest = n
			}
		}
		if largest < 0 {
			return nil
		}
		p.value.Fields[largest] = types.OmitTraceEventSemanticValue(p.value.Fields[largest], "semantic_budget_exceeded")
	}
	if !types.ValidateTraceEventSemantics(&p.value) {
		return nil
	}
	return &p.value
}
