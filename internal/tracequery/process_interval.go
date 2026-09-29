package tracequery

import "github.com/hanchaoqun/codrax/internal/tracewire"

func processIntervalEvent(lineNo int, row tracewire.ProcessInterval, intern *stringInterner) Event {
	label := "AppStartup:startup"
	if n := row.Origin.Name.Name; n != nil && *n != "" {
		label = "AppStartup:" + *n
	}
	// These actions deliberately differ from B/E. Process-owned source records
	// must not enter the physical thread stacks or mint synchronous call edges.
	return Event{Line: lineNo, Ts: float64(row.TimestampNS()) / 1e9, CPU: -1,
		Type: EventTraceMark, Name: intern.intern("codrax_process_interval"),
		SpanAction: intern.intern("source_" + row.Endpoint), SpanName: intern.intern(label),
		PluginFields: &PluginFields{MarkerNameOrigin: &row.Origin},
		FieldText:    intern.intern("SQL process interval " + row.Endpoint),
	}
}
