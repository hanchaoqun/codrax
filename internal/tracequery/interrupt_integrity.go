package tracequery

import (
	"fmt"
	"sort"
	"strings"
)

// interruptLaneKey is the exact duration identity. IRQ/softirq expose a
// numeric vector; IPI exposes a reason string instead. CPU is always part of
// the key, so unrelated CPUs or vectors can never close one another's lane.
func interruptLaneKey(ev Event) (string, bool) {
	switch ev.Type {
	case EventIRQ, EventSoftIRQ:
		if ev.IRQID < 0 {
			return "", false
		}
		return fmt.Sprintf("%s\x00%d\x00%d", ev.Type, ev.CPU, ev.IRQID), true
	case EventIPI:
		vector := strings.ToLower(strings.TrimSpace(ev.IRQName))
		if vector == "" {
			return "", false
		}
		return fmt.Sprintf("%s\x00%d\x00%s", ev.Type, ev.CPU, vector), true
	default:
		return "", false
	}
}

// interruptEndpointRawCandidate is the O(1) token prescreen for the six
// duration-bearing interrupt endpoint names (perf audit #22): a violation can
// only be minted for irq_handler_/softirq_/ipi_ entry/exit rows, so the full
// header regex is never spent on sched/print/fs lines that the wide
// duration-order token table also matches.
func interruptEndpointRawCandidate(line string) bool {
	return strings.Contains(line, "irq_handler_") ||
		strings.Contains(line, "softirq_") ||
		strings.Contains(line, "ipi_")
}

// interruptEndpointValidationFailure records only duration-bearing endpoint
// families. *_raise is an instantaneous inventory signal and deliberately
// does not participate in the pairing integrity gate.
func interruptEndpointValidationFailure(lineNo int, line string) *durationOrderViolation {
	var scan lineScan
	scan.reset(lineNo, line)
	return interruptEndpointValidationFailureScan(&scan)
}

func interruptEndpointValidationFailureScan(s *lineScan) *durationOrderViolation {
	lineNo := s.lineNo
	if !interruptEndpointRawCandidate(s.line) {
		return nil
	}
	m := s.match()
	if len(m) == 0 {
		return nil
	}
	rawType := strings.TrimSuffix(strings.TrimSpace(m[6]), ":")
	family := durationOrderFamily("")
	switch rawType {
	case "irq_handler_entry", "irq_handler_exit":
		family = durationOrderIRQ
	case "softirq_entry", "softirq_exit":
		family = durationOrderSoftIRQ
	case "ipi_entry", "ipi_exit":
		family = durationOrderIPI
	default:
		return nil
	}

	ts, tsOK := s.timestamp()
	cpu, cpuOK := atoiMaybe(m[4])
	kv := s.keyValues()
	fields := make([]string, 0, 2)
	// ENG audit #4b (§29.25 处置委托 2026-07-10): an unparseable/overflowed
	// timestamp is itself a damaged required field. Swallowing it minted a
	// violation with CurrentTs=0 (excluded by every windowed relevance check
	// and unreachable by the per-pair interval match) — or, with intact lane
	// keys, no violation at all. Record it and mark the violation TsUnknown so
	// both suppression nets treat it conservatively.
	if !tsOK {
		fields = append(fields, "ts")
	}
	if !cpuOK || cpu < 0 {
		fields = append(fields, "cpu")
	}
	switch family {
	case durationOrderIRQ:
		if vector, ok := atoiMaybe(kv["irq"]); !ok || vector < 0 {
			fields = append(fields, "irq")
		}
	case durationOrderSoftIRQ:
		if vector, ok := atoiMaybe(kv["vec"]); !ok || vector < 0 {
			fields = append(fields, "vec")
		}
	case durationOrderIPI:
		if strings.TrimSpace(parseIPIReason(m[7])) == "" {
			fields = append(fields, "reason")
		}
	}
	if len(fields) == 0 {
		return nil
	}
	sort.Strings(fields)
	return &durationOrderViolation{
		Family: family, Issue: "endpoint_parse_incomplete", EventName: rawType,
		Fields: fields, CurrentTs: ts, TsUnknown: !tsOK, Line: lineNo,
	}
}
