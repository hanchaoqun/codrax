package tracequery

// TraceEventInventoryCoordinates describes observed row coordinates only.
// It confers no scheduler generation, execution, duration or causal authority.
// Unknown is explicit and never encoded as the legitimate CPU/idle TID zero.
type TraceEventInventoryCoordinates struct {
	CPU, EmitterTID, EmitterTGID                int
	CPUKnown, EmitterTIDKnown, EmitterTGIDKnown bool
	CPUUnknownReason                            string
}

// ProjectTraceEventInventoryCoordinates reads only parser-owned typed fields.
// It does not reparse Raw/FieldText, resolve payload IDs or borrow neighboring
// rows. The source Event and its historical JSON remain untouched.
func ProjectTraceEventInventoryCoordinates(event Event) TraceEventInventoryCoordinates {
	p := TraceEventInventoryCoordinates{
		CPU: event.CPU, CPUKnown: validTraceCPUIndex(event.CPU),
		EmitterTID: event.PID, EmitterTIDKnown: event.PID >= 0,
		EmitterTGID: event.TGID, EmitterTGIDKnown: event.TGID > 0,
	}
	noEmitter := false
	switch event.Type {
	case EventFrameMap, EventFrameCallstack, EventFrameGPU, EventTraceDBRecord, EventCPUMeasureInterval:
		noEmitter = true
		p.CPUKnown, p.CPUUnknownReason = false, "source_has_no_cpu_coordinate"
	case EventResourceStack:
		// Event rows have independently resolved owners; frame rows have none.
		// No resource row is a CPU execution observation.
		noEmitter = event.PID <= 0
		p.CPUKnown, p.CPUUnknownReason = false, "source_has_no_cpu_coordinate"
	case EventEBPFInterval:
		noEmitter = event.PluginFields == nil || event.PluginFields.EBPFInterval == nil || event.PluginFields.EBPFInterval.IdentityStatus != "resolved"
		p.CPUKnown, p.CPUUnknownReason = false, "source_has_no_cpu_coordinate"
	case EventPerfSample:
		p.CPUKnown = perfSampleHasKnownCPU(event)
		if !p.CPUKnown {
			p.CPUUnknownReason = "perf_cpu_identity_unavailable"
		}
		thread := perfSampleThread(event)
		p.EmitterTID, p.EmitterTGID = thread.PID, thread.TGID
		p.EmitterTIDKnown, p.EmitterTGIDKnown = thread.PID > 0, thread.TGID > 0
	case EventTraceMark:
		if event.SpanAction == "source_begin" || event.SpanAction == "source_end" {
			noEmitter = true
			p.CPUKnown, p.CPUUnknownReason = false, "source_has_no_cpu_coordinate"
		}
	case EventHiSystemEvent:
		if event.PluginFields != nil && event.PluginFields.HiSysEvent != nil {
			noEmitter = true
			p.CPUKnown, p.CPUUnknownReason = false, "source_has_no_cpu_coordinate"
		}
	}
	if fields := event.PluginFields; fields != nil {
		if fields.TraceMarkerCPUStatus == TraceMarkCPUStatusUnavailable {
			p.CPUKnown, p.CPUUnknownReason = false, fields.TraceMarkerCPUReason
		}
		if fields.SchedulerEmitterCPUStatus == SchedulerEmitterCPUStatusUnavailable {
			p.CPUKnown, p.CPUUnknownReason = false, fields.SchedulerEmitterCPUReason
		}
	}
	if noEmitter {
		p.EmitterTIDKnown, p.EmitterTGIDKnown = false, false
	}
	if !p.EmitterTIDKnown {
		p.EmitterTID = -1
	}
	if !p.EmitterTGIDKnown {
		p.EmitterTGID = -1
	}
	if !p.CPUKnown {
		p.CPU = -1
		if p.CPUUnknownReason == "" {
			p.CPUUnknownReason = "not_recorded"
		}
	}
	return p
}
