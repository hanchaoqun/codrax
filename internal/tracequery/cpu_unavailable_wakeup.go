package tracequery

import (
	"fmt"
	"strconv"
	"strings"
)

const cpuUnavailableWakeupPrefix = "# codrax_sched_wakeup_cpu_unavailable/v1"

const (
	SchedulerEmitterCPUStatusUnavailable = "unavailable"

	SchedulerEmitterCPUReasonUnknown       = "missing_or_ambiguous_emitter_running_cpu"
	SchedulerEmitterCPUReasonSourceTainted = "tainted_emitter_running_cpu"
)

// CPUUnavailableWakeup is a converter-authored scheduler dependency whose
// endpoints, timestamp, target CPU, and optional inferred priority are proven,
// but whose physical emitter CPU is not. The wire record remains a comment for
// generic ftrace readers and is reconstructed by Codrax without fabricating
// cpu0 or any other physical header CPU.
type CPUUnavailableWakeup struct {
	TimestampNS    uint64
	EventName      string
	WakerTID       int
	WakerTGID      int
	WakeeTID       int
	TargetCPU      int
	Priority       int
	PrioritySource string
	WakerComm      string
	WakeeComm      string
	Reason         string
}

// FormatCPUUnavailableWakeup is the sole formatter for the versioned
// CPU-unavailable wakeup record.
func FormatCPUUnavailableWakeup(wakeup CPUUnavailableWakeup) (string, error) {
	if !validCPUUnavailableWakeup(wakeup) {
		return "", fmt.Errorf("invalid cpu-unavailable wakeup")
	}
	priority := cpuUnavailableTraceMarkEmptyTextWire
	if wakeup.PrioritySource == WakeePrioritySourceInferredNextSchedSlice {
		priority = strconv.Itoa(wakeup.Priority)
	}
	return strings.Join([]string{
		cpuUnavailableWakeupPrefix,
		"ts_ns=" + strconv.FormatUint(wakeup.TimestampNS, 10),
		"event=" + wakeup.EventName,
		"waker_tid=" + strconv.Itoa(wakeup.WakerTID),
		"waker_tgid=" + strconv.Itoa(wakeup.WakerTGID),
		"wakee_tid=" + strconv.Itoa(wakeup.WakeeTID),
		"target_cpu=" + strconv.Itoa(wakeup.TargetCPU),
		"priority=" + priority,
		"priority_source=" + wakeup.PrioritySource,
		"waker_comm=" + encodeCPUUnavailableTraceMarkText(wakeup.WakerComm),
		"wakee_comm=" + encodeCPUUnavailableTraceMarkText(wakeup.WakeeComm),
		"reason=" + wakeup.Reason,
	}, " "), nil
}

func parseCPUUnavailableWakeup(line string) (CPUUnavailableWakeup, bool) {
	// The prefix check keeps the ordinary physical scheduler lane allocation
	// free. Version and field order are closed so future schemas cannot be
	// partially interpreted as v1.
	if !strings.HasPrefix(line, cpuUnavailableWakeupPrefix+" ") {
		return CPUUnavailableWakeup{}, false
	}
	parts := strings.Split(line, " ")
	if len(parts) != 13 || parts[0] != "#" || parts[1] != "codrax_sched_wakeup_cpu_unavailable/v1" {
		return CPUUnavailableWakeup{}, false
	}
	value := func(index int, prefix string) (string, bool) {
		if !strings.HasPrefix(parts[index], prefix) {
			return "", false
		}
		out := strings.TrimPrefix(parts[index], prefix)
		return out, out != ""
	}
	tsRaw, tsOK := value(2, "ts_ns=")
	eventName, eventOK := value(3, "event=")
	wakerTIDRaw, wakerTIDOK := value(4, "waker_tid=")
	wakerTGIDRaw, wakerTGIDOK := value(5, "waker_tgid=")
	wakeeTIDRaw, wakeeTIDOK := value(6, "wakee_tid=")
	targetCPURaw, targetCPUOK := value(7, "target_cpu=")
	priorityRaw, priorityOK := value(8, "priority=")
	prioritySource, prioritySourceOK := value(9, "priority_source=")
	wakerCommRaw, wakerCommOK := value(10, "waker_comm=")
	wakeeCommRaw, wakeeCommOK := value(11, "wakee_comm=")
	reason, reasonOK := value(12, "reason=")
	if !tsOK || !eventOK || !wakerTIDOK || !wakerTGIDOK || !wakeeTIDOK ||
		!targetCPUOK || !priorityOK || !prioritySourceOK || !wakerCommOK ||
		!wakeeCommOK || !reasonOK {
		return CPUUnavailableWakeup{}, false
	}
	timestamp, err := strconv.ParseUint(tsRaw, 10, 64)
	if err != nil {
		return CPUUnavailableWakeup{}, false
	}
	wakerTID, err := strconv.ParseInt(wakerTIDRaw, 10, 32)
	if err != nil {
		return CPUUnavailableWakeup{}, false
	}
	wakerTGID, err := strconv.ParseInt(wakerTGIDRaw, 10, 32)
	if err != nil {
		return CPUUnavailableWakeup{}, false
	}
	wakeeTID, err := strconv.ParseInt(wakeeTIDRaw, 10, 32)
	if err != nil {
		return CPUUnavailableWakeup{}, false
	}
	targetCPU, err := strconv.ParseInt(targetCPURaw, 10, 32)
	if err != nil {
		return CPUUnavailableWakeup{}, false
	}
	priority := int64(0)
	if priorityRaw != cpuUnavailableTraceMarkEmptyTextWire {
		priority, err = strconv.ParseInt(priorityRaw, 10, 32)
		if err != nil {
			return CPUUnavailableWakeup{}, false
		}
	}
	wakerComm, wakerCommOK := decodeCPUUnavailableTraceMarkText(wakerCommRaw)
	wakeeComm, wakeeCommOK := decodeCPUUnavailableTraceMarkText(wakeeCommRaw)
	if !wakerCommOK || !wakeeCommOK {
		return CPUUnavailableWakeup{}, false
	}
	wakeup := CPUUnavailableWakeup{
		TimestampNS:    timestamp,
		EventName:      eventName,
		WakerTID:       int(wakerTID),
		WakerTGID:      int(wakerTGID),
		WakeeTID:       int(wakeeTID),
		TargetCPU:      int(targetCPU),
		Priority:       int(priority),
		PrioritySource: prioritySource,
		WakerComm:      wakerComm,
		WakeeComm:      wakeeComm,
		Reason:         reason,
	}
	if prioritySource == WakeePrioritySourceUnknown && priorityRaw != cpuUnavailableTraceMarkEmptyTextWire {
		return CPUUnavailableWakeup{}, false
	}
	if prioritySource == WakeePrioritySourceInferredNextSchedSlice &&
		priorityRaw == cpuUnavailableTraceMarkEmptyTextWire {
		return CPUUnavailableWakeup{}, false
	}
	return wakeup, validCPUUnavailableWakeup(wakeup)
}

func validCPUUnavailableWakeup(wakeup CPUUnavailableWakeup) bool {
	if wakeup.WakerTID <= 0 || wakeup.WakerTGID <= 0 || wakeup.WakeeTID <= 0 ||
		!validTraceCPUIndex(wakeup.TargetCPU) ||
		!validCPUUnavailableTraceMarkText(wakeup.WakerComm, false) ||
		!validCPUUnavailableTraceMarkText(wakeup.WakeeComm, false) {
		return false
	}
	switch wakeup.EventName {
	case "sched_wakeup", "sched_wakeup_new", "sched_waking":
	default:
		return false
	}
	switch wakeup.PrioritySource {
	case WakeePrioritySourceUnknown:
		if wakeup.Priority != 0 {
			return false
		}
	case WakeePrioritySourceInferredNextSchedSlice:
	default:
		return false
	}
	switch wakeup.Reason {
	case SchedulerEmitterCPUReasonUnknown, SchedulerEmitterCPUReasonSourceTainted:
		return true
	default:
		return false
	}
}

func cpuUnavailableWakeupEvent(lineNo int, wakeup CPUUnavailableWakeup, intern *stringInterner) Event {
	eventType := EventSchedWakeup
	if wakeup.EventName == "sched_waking" {
		eventType = EventSchedWaking
	}
	event := Event{
		Line:           lineNo,
		Ts:             float64(wakeup.TimestampNS) / 1e9,
		CPU:            -1,
		Type:           eventType,
		Name:           intern.intern(wakeup.EventName),
		Comm:           intern.intern(wakeup.WakerComm),
		PID:            wakeup.WakerTID,
		TGID:           wakeup.WakerTGID,
		WakeeComm:      intern.intern(wakeup.WakeeComm),
		WakeePID:       wakeup.WakeeTID,
		WakeePrio:      wakeup.Priority,
		TargetCPU:      wakeup.TargetCPU,
		TargetCPUValid: true,
		PluginFields: &PluginFields{
			SchedulerEmitterCPUStatus: SchedulerEmitterCPUStatusUnavailable,
			SchedulerEmitterCPUReason: intern.intern(wakeup.Reason),
		},
	}
	if wakeup.PrioritySource == WakeePrioritySourceInferredNextSchedSlice {
		event.WakeePrioInferred = true
	} else {
		event.WakeePrioUnknown = true
	}
	event.FieldText = intern.intern(fmt.Sprintf(
		"comm=%s pid=%d target_cpu=%d codrax_prio_source=%s",
		wakeup.WakeeComm, wakeup.WakeeTID, wakeup.TargetCPU, wakeup.PrioritySource))
	return event
}
