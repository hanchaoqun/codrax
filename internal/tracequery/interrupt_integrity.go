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

// interruptEndpointValidationFailure records only duration-bearing endpoint
// families. *_raise is an instantaneous inventory signal and deliberately
// does not participate in the pairing integrity gate.
func interruptEndpointValidationFailure(lineNo int, line string) *durationOrderViolation {
	m := ftraceLineRE.FindStringSubmatch(line)
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

	ts, _ := parseLineTimestamp(line)
	cpu, cpuOK := atoiMaybe(m[4])
	kv := parseKV(strings.TrimSpace(m[7]))
	fields := make([]string, 0, 2)
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
		Fields: fields, CurrentTs: ts, Line: lineNo,
	}
}
