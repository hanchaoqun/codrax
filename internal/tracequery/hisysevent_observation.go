package tracequery

import (
	"fmt"
	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func hiSysEventObservationEvent(lineNo int, row tracewire.HiSysEvent, intern *stringInterner) Event {
	name := func(n *tracewire.HiSysEventName) string {
		n.Status = intern.intern(n.Status)
		if n.Name == nil {
			return ""
		}
		v := intern.intern(*n.Name)
		n.Name = &v
		return v
	}
	domain, event := name(&row.Domain), name(&row.Event)
	row.Contents.StorageClass = intern.intern(row.Contents.StorageClass)
	if row.Contents.Text != nil {
		v := intern.intern(*row.Contents.Text)
		row.Contents.Text = &v
	}
	row.Contents.BytesBase64 = intern.intern(row.Contents.BytesBase64)
	// The source TID is retained only inside the typed row. It is not a
	// physical ftrace emitter, and an observation cannot create a thread lane.
	return Event{Line: lineNo, Ts: float64(row.TimestampNS) / 1e9, CPU: -1,
		Type: EventHiSystemEvent, Name: intern.intern("codrax_hisysevent"),
		PluginFields: &PluginFields{Domain: domain, EventName: event, HiSysEvent: &row},
		FieldText:    intern.intern(fmt.Sprintf("SQL HiSys observation; domain=%s event=%s contents=%s", row.Domain.Status, row.Event.Status, row.Contents.StorageClass)),
	}
}
