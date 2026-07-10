package tracequery

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// The fallback keeps the event-column discriminator precise while allowing
// malformed header identities to be audited instead of disappearing merely
// because the main parser's fully typed ftrace grammar rejected the row.
// Group indexes intentionally mirror ftraceLineRE: comm, pid, tgid, cpu,
// timestamp, event, fields.
var durationEndpointFallbackRE = regexp.MustCompile(`^\s*(.+)-([^\s]+)(?:\s+\(\s*([^\s()]+)\s*\))?\s+\[([^\]]+)\]\s+\S+\s+([^\s:]+):\s+(workqueue_execute_start|workqueue_execute_end|dma_fence_wait_start|dma_fence_wait_end):\s*(.*)$`)

// workqueueEndpointMissingFields and dmaFenceEndpointMissingFields define the
// minimum exact identities permitted to mint elapsed time. Inventory events
// outside the closed start/end families never enter these gates.
func workqueueEndpointMissingFields(ev Event, work string) []string {
	var missing []string
	if ev.PID <= 0 {
		missing = append(missing, "pid")
	}
	if !validWorkqueuePointerIdentity(work) {
		missing = append(missing, "work")
	}
	return missing
}

func validWorkqueuePointerIdentity(raw string) bool {
	raw = strings.TrimRight(cleanTraceValue(raw), ":")
	hadHexPrefix := strings.HasPrefix(strings.ToLower(raw), "0x")
	if hadHexPrefix {
		raw = raw[2:]
	}
	if raw == "" || len(raw) > 16 {
		return false
	}
	// Linux %p renders an unprefixed pointer at the native fixed width. Short
	// tokens are accepted only with an explicit 0x prefix; otherwise ordinary
	// words made solely of [a-f] (for example "bad") could masquerade as a
	// pointer identity.
	if !hadHexPrefix && len(raw) != 8 && len(raw) != 16 {
		return false
	}
	value, err := strconv.ParseUint(raw, 16, 64)
	return err == nil && value != 0
}

func dmaFenceEndpointMissingFields(ev Event, driver, timeline, context, seqno string) []string {
	var missing []string
	if ev.PID <= 0 {
		missing = append(missing, "pid")
	}
	if strings.TrimSpace(driver) == "" {
		missing = append(missing, "driver")
	}
	if strings.TrimSpace(timeline) == "" {
		missing = append(missing, "timeline")
	}
	if !validUnsignedTraceIdentity(context) {
		missing = append(missing, "context")
	}
	if !validUnsignedTraceIdentity(seqno) {
		missing = append(missing, "seqno")
	}
	return missing
}

func validUnsignedTraceIdentity(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "+") || strings.HasPrefix(raw, "-") {
		return false
	}
	base := 10
	if strings.HasPrefix(strings.ToLower(raw), "0x") {
		base = 16
		raw = raw[2:]
		if raw == "" {
			return false
		}
	}
	_, err := strconv.ParseUint(raw, base, 64)
	return err == nil
}

// Exact duration endpoints use only the upstream tracepoint field names.
// Wider aliases remain available to inventory/display parsing, but may not
// satisfy a hard pairing identity.
func workqueueExactEndpointFields(ev Event) (work, function string) {
	kv := parseKV(ev.FieldText)
	work = cleanTraceValue(kv["work"])
	function = cleanTraceValue(kv["function"])
	if work == "" {
		work = workqueuePositionalWorkPointer(ev.FieldText)
	}
	if function == "" {
		function = valueAfterLabel(ev.FieldText, "function")
	}
	return strings.TrimRight(cleanTraceValue(work), ":"), cleanTraceValue(function)
}

func workqueuePositionalWorkPointer(text string) string {
	trimmed := strings.TrimSpace(text)
	const label = "work struct"
	if len(trimmed) < len(label) || !strings.EqualFold(trimmed[:len(label)], label) {
		return ""
	}
	rest := trimmed[len(label):]
	if rest == "" || (rest[0] != '=' && rest[0] != ' ' && rest[0] != '\t') {
		return ""
	}
	rest = strings.TrimLeft(rest, "= \t")
	if rest == "" {
		return ""
	}
	return strings.TrimRight(cleanTraceValue(strings.Fields(rest)[0]), ":")
}

func dmaFenceExactEndpointFields(ev Event) (driver, timeline, context, seqno string) {
	kv := parseKV(ev.FieldText)
	return cleanTraceValue(kv["driver"]), cleanTraceValue(kv["timeline"]), cleanTraceValue(kv["context"]), cleanTraceValue(kv["seqno"])
}

func durationExactEndpointFamily(rawType string) durationOrderFamily {
	switch strings.TrimSpace(rawType) {
	case "workqueue_execute_start", "workqueue_execute_end":
		return durationOrderWorkqueue
	case "dma_fence_wait_start", "dma_fence_wait_end":
		return durationOrderDMAFence
	default:
		return ""
	}
}

// durationEndpointFallbackCandidate is the O(1) prescreen for the fallback
// grammar: durationEndpointFallbackRE can only match a line carrying one of
// the four exact endpoint names, all of which contain one of these two
// prefixes. Without it every sched/print/fs line matching the wide
// duration-order token table paid a second full regex (perf audit #21/#22).
func durationEndpointFallbackCandidate(line string) bool {
	return strings.Contains(line, "workqueue_execute_") || strings.Contains(line, "dma_fence_wait_")
}

func durationEndpointRawMatch(line string) []string {
	var scan lineScan
	scan.reset(0, line)
	return durationEndpointRawMatchScan(&scan)
}

func durationEndpointRawMatchScan(s *lineScan) []string {
	if match := s.match(); len(match) != 0 {
		if durationExactEndpointFamily(strings.TrimSuffix(strings.TrimSpace(match[6]), ":")) != "" {
			return match
		}
	}
	if !durationEndpointFallbackCandidate(s.line) {
		return nil
	}
	return durationEndpointFallbackRE.FindStringSubmatch(s.line)
}

// durationEndpointRawValidationFailure audits the physical row before Event
// admission. This closes the parser-reject path: a malformed exact endpoint
// cannot disappear into the generic unparsed census while another pair in the
// same family still mints elapsed time.
func durationEndpointRawValidationFailure(lineNo int, line string) *durationOrderViolation {
	var scan lineScan
	scan.reset(lineNo, line)
	return durationEndpointRawValidationFailureScan(&scan)
}

func durationEndpointRawValidationFailureScan(s *lineScan) *durationOrderViolation {
	lineNo := s.lineNo
	m := durationEndpointRawMatchScan(s)
	if len(m) == 0 {
		return nil
	}
	rawType := strings.TrimSuffix(strings.TrimSpace(m[6]), ":")
	family := durationExactEndpointFamily(rawType)
	if family == "" {
		return nil
	}

	pid, pidOK := parseUnsignedTraceInt(m[2])
	cpu, cpuPresent, cpuValid, _ := parseTraceCPUScalar(m[4])
	ts, tsOK := parseTraceTimestampSeconds(m[5])
	if !tsOK {
		ts = math.NaN()
	}
	ev := Event{PID: pid, CPU: cpu, Type: EventWorkqueue, Name: rawType, FieldText: strings.TrimSpace(m[7])}
	var missing []string
	if !pidOK || pid <= 0 {
		missing = append(missing, "pid")
	}
	if !cpuPresent || !cpuValid {
		missing = append(missing, "header_cpu")
	}
	if !tsOK {
		missing = append(missing, "timestamp")
	}
	switch family {
	case durationOrderWorkqueue:
		work, _ := workqueueExactEndpointFields(ev)
		if !validWorkqueuePointerIdentity(work) {
			missing = append(missing, "work")
		}
	case durationOrderDMAFence:
		ev.Type = EventDMAFence
		driver, timeline, context, seqno := dmaFenceExactEndpointFields(ev)
		missing = append(missing, dmaFenceEndpointMissingFields(ev, driver, timeline, context, seqno)...)
	}
	missing = uniqueSortedStrings(missing)
	if len(missing) == 0 {
		return nil
	}
	return &durationOrderViolation{
		Family: family, Issue: "endpoint_parse_incomplete", EventName: rawType,
		Fields: missing, CurrentTs: ts, Line: lineNo,
	}
}

func durationEndpointRejectedRowFailure(lineNo int, line string) *durationOrderViolation {
	var scan lineScan
	scan.reset(lineNo, line)
	return durationEndpointRejectedRowFailureScan(&scan)
}

func durationEndpointRejectedRowFailureScan(s *lineScan) *durationOrderViolation {
	m := durationEndpointRawMatchScan(s)
	if len(m) == 0 {
		return nil
	}
	rawType := strings.TrimSuffix(strings.TrimSpace(m[6]), ":")
	family := durationExactEndpointFamily(rawType)
	if family == "" || durationEndpointRawValidationFailureScan(s) != nil {
		return nil
	}
	ts, ok := parseTraceTimestampSeconds(m[5])
	if !ok {
		ts = math.NaN()
	}
	return &durationOrderViolation{
		Family: family, Issue: "endpoint_parse_incomplete", EventName: rawType,
		Fields: []string{"parser_rejected_row"}, CurrentTs: ts, Line: s.lineNo,
	}
}

func uniqueSortedStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, value := range in {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func durationEndpointValidationFailureFromEvent(ev Event) *durationOrderViolation {
	var family durationOrderFamily
	var phase string
	var missing []string
	switch ev.Type {
	case EventWorkqueue:
		_, phase = workqueueBaseAndPhase(ev.Name)
		if phase == "" {
			return nil
		}
		work, _ := workqueueExactEndpointFields(ev)
		family = durationOrderWorkqueue
		missing = workqueueEndpointMissingFields(ev, work)
	case EventDMAFence:
		_, phase = dmaFenceBaseAndPhase(ev.Name)
		if phase == "" {
			return nil
		}
		driver, timeline, context, seqno := dmaFenceExactEndpointFields(ev)
		family = durationOrderDMAFence
		missing = dmaFenceEndpointMissingFields(ev, driver, timeline, context, seqno)
	default:
		return nil
	}
	if len(missing) == 0 {
		return nil
	}
	return &durationOrderViolation{
		Family: family, Issue: "endpoint_parse_incomplete", EventName: strings.TrimSpace(ev.Name),
		Fields: missing, CurrentTs: ev.Ts, Line: ev.Line,
	}
}
