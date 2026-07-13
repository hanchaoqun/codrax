package tracequery

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// CPU identifiers are hard attribution keys. Keep their parser deliberately
// narrower than atoi: malformed text must not collapse to cpu0, and a hostile
// range must not allocate in proportion to an untrusted endpoint.
const (
	maxTraceCPUIndex          = 4095
	maxTraceCPUSetExpansion   = 1024
	cpuInputIntegrityIssueCap = 64
	// wakeupTargetCPUDegradedFloor (CR-4 引擎件, 2026-07-12): the all-zero
	// wakeup target_cpu verdict needs a population this large before it
	// speaks — small windows can legitimately land every wakeup on one CPU.
	// The tieba witness carried 1697 all-zero rows.
	wakeupTargetCPUDegradedFloor = 100
)

type cpuInputIntegrityFailure struct {
	EventName  string
	Field      string
	Raw        string
	ReasonCode string
	Line       int
	Ts         float64
	CPU        int
	SourcePath string
	// LocalLine: artifact-local physical line, set at composite rebase time
	// (audit #36); zero when Line is already physical.
	LocalLine int
}

func (f cpuInputIntegrityFailure) reason() string {
	reason := fmt.Sprintf("cpu_input_invalid event=%s field=%s value=%q reason=%s ts=%.6f cpu=%d line=%d",
		f.EventName, f.Field, f.Raw, f.ReasonCode, f.Ts, f.CPU, f.Line)
	if f.SourcePath != "" {
		reason += " source=" + f.SourcePath
		reason += witnessLocalLineSuffix(f.Line, f.LocalLine)
	}
	return reason
}

func validTraceCPUIndex(cpu int) bool {
	return cpu >= 0 && cpu <= maxTraceCPUIndex
}

func setEventCPUForField(ev *Event, raw string) {
	if ev == nil {
		return
	}
	cpu, present, valid, _ := parseTraceCPUScalar(raw)
	ev.CPUForFieldPresent = present
	if valid {
		ev.CPUForField = cpu
		ev.CPUForFieldValid = true
	} else if present {
		ev.CPUInputInvalid = true
	}
}

func setEventTargetCPU(ev *Event, raw string) {
	if ev == nil {
		return
	}
	cpu, present, valid, _ := parseTraceCPUScalar(raw)
	if valid {
		ev.TargetCPU = cpu
		ev.TargetCPUValid = true
	} else if present {
		ev.CPUInputInvalid = true
	}
}

func eventTargetCPU(ev Event) (int, bool) {
	if !ev.TargetCPUValid || !validTraceCPUIndex(ev.TargetCPU) {
		return -1, false
	}
	return ev.TargetCPU, true
}

// parseTraceCPUScalar keeps presence separate from validity. Empty/absent is
// not a value; a present malformed, negative, signed, or excessive token is
// invalid and may never fall back to an emitting CPU.
func parseTraceCPUScalar(raw string) (value int, present, valid bool, reason string) {
	raw = strings.Trim(strings.TrimSpace(raw), ":,")
	if raw == "" {
		return 0, false, false, ""
	}
	present = true
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0, true, false, "not_unsigned_decimal"
		}
	}
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return 0, true, false, "integer_overflow"
	}
	if n > maxTraceCPUIndex {
		return 0, true, false, "cpu_above_limit"
	}
	return int(n), true, true, ""
}

// parseCPURangeListStrict accepts an all-or-nothing decimal CPU roster. One
// bad token rejects the entire roster; reversed ranges are not normalized.
func parseCPURangeListStrict(raw string) ([]int, bool, string) {
	raw = strings.TrimSpace(strings.Trim(raw, "{}[]()"))
	if raw == "" {
		return nil, false, "empty"
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '|' || r == '/' || r == ' ' || r == ','
	})
	if len(parts) == 0 {
		return nil, false, "empty"
	}
	out := make([]int, 0, minInt(len(parts), maxTraceCPUSetExpansion))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, false, "empty_token"
		}
		if strings.Count(part, "-") > 1 {
			return nil, false, "malformed_range"
		}
		if startRaw, endRaw, ranged := strings.Cut(part, "-"); ranged {
			start, startPresent, startValid, startReason := parseTraceCPUScalar(startRaw)
			end, endPresent, endValid, endReason := parseTraceCPUScalar(endRaw)
			if !startPresent || !endPresent {
				return nil, false, "malformed_range"
			}
			if !startValid {
				return nil, false, startReason
			}
			if !endValid {
				return nil, false, endReason
			}
			if start > end {
				return nil, false, "reversed_range"
			}
			width := end - start + 1
			if width > maxTraceCPUSetExpansion-len(out) {
				return nil, false, "expanded_set_too_large"
			}
			for cpu := start; cpu <= end; cpu++ {
				out = append(out, cpu)
			}
			continue
		}
		cpu, present, valid, scalarReason := parseTraceCPUScalar(part)
		if !present || !valid {
			if scalarReason == "" {
				scalarReason = "malformed_cpu"
			}
			return nil, false, scalarReason
		}
		if len(out) >= maxTraceCPUSetExpansion {
			return nil, false, "expanded_set_too_large"
		}
		out = append(out, cpu)
	}
	return uniqueSortedInts(out), true, ""
}

func parseCPUSetListStrict(raw string) ([]int, bool, string) {
	raw = strings.TrimSpace(strings.Trim(raw, "{}[]()"))
	if raw == "" {
		return nil, false, "empty"
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "0x") || containsHexAlpha(lower) {
		trimmed := strings.Trim(strings.TrimSpace(raw), "{}[](),")
		trimmed = strings.TrimPrefix(strings.ToLower(trimmed), "0x")
		if trimmed == "" {
			return nil, false, "malformed_hex_mask"
		}
		mask, err := strconv.ParseUint(trimmed, 16, 64)
		if err != nil {
			return nil, false, "malformed_hex_mask"
		}
		var cpus []int
		for cpu := 0; cpu < 64; cpu++ {
			if mask&(uint64(1)<<uint(cpu)) != 0 {
				cpus = append(cpus, cpu)
			}
		}
		return cpus, true, ""
	}
	return parseCPURangeListStrict(raw)
}

// cpuInputRawCandidate is the O(1) token prescreen shared by the parse hot
// loop and the standalone entry point: only these event families (or a header
// CPU token wide enough to exceed the limit) can mint a cpu_input witness.
func cpuInputRawCandidate(line string) bool {
	if traceHeaderCPUCouldExceedLimit(line) {
		return true
	}
	return strings.Contains(line, "cpu_") || strings.Contains(line, "clock_set_rate:") ||
		strings.Contains(line, "perf_sample:") ||
		strings.Contains(line, "sched_wakeup:") || strings.Contains(line, "sched_wakeup_new:") ||
		strings.Contains(line, "sched_waking:") || strings.Contains(line, "sched_migrate_task:") ||
		strings.Contains(line, "sched_setaffinity:") || strings.Contains(line, "cpuset_attach:") ||
		strings.Contains(line, "cgroup_attach_task:")
}

func cpuInputValidationFailures(lineNo int, line string) []cpuInputIntegrityFailure {
	if !cpuInputRawCandidate(line) {
		return nil
	}
	var scan lineScan
	scan.reset(lineNo, line)
	return cpuInputValidationFailuresScan(&scan)
}

// cpuInputValidationFailuresScan consumes the shared per-line memo; callers
// on the hot loop gate it behind cpuInputRawCandidate.
func cpuInputValidationFailuresScan(s *lineScan) []cpuInputIntegrityFailure {
	lineNo := s.lineNo
	m := s.match()
	if len(m) == 0 {
		return nil
	}
	rawType := strings.TrimSuffix(strings.TrimSpace(m[6]), ":")
	typ := classifyEventType(strings.TrimSpace(m[1]), rawType, strings.TrimSpace(m[7]))
	headerCPU, _ := strconv.Atoi(m[4])
	ts, _ := s.timestamp()
	var out []cpuInputIntegrityFailure
	if !validTraceCPUIndex(headerCPU) {
		out = append(out, cpuInputIntegrityFailure{EventName: rawType, Field: "header_cpu", Raw: m[4], ReasonCode: "cpu_above_limit", Line: lineNo, Ts: ts, CPU: headerCPU})
	}
	switch typ {
	case EventCPUIdle, EventCPUFrequency, EventCPUFrequencyLimit, EventClockSetRate, EventCPUConstraint, EventSchedWakeup, EventSchedWaking, EventPerfSample:
	default:
		return out
	}
	kv := s.keyValues()
	addScalar := func(field string) {
		raw, exists := kv[field]
		if !exists {
			return
		}
		_, present, valid, reason := parseTraceCPUScalar(raw)
		if present && valid {
			return
		}
		if reason == "" {
			reason = "empty_value"
		}
		out = append(out, cpuInputIntegrityFailure{EventName: rawType, Field: field, Raw: clampString(raw, 80), ReasonCode: reason, Line: lineNo, Ts: ts, CPU: headerCPU})
	}
	if typ == EventCPUIdle || typ == EventCPUFrequency || typ == EventCPUFrequencyLimit || typ == EventClockSetRate {
		addScalar("cpu_id")
	}
	if typ == EventSchedWakeup || typ == EventSchedWaking || typ == EventCPUConstraint {
		addScalar("target_cpu")
	}
	if typ == EventCPUConstraint {
		addScalar("cpu")
		addScalar("orig_cpu")
		addScalar("dest_cpu")
		for _, field := range []string{"allowed_cpus", "cpus_allowed", "cpumask", "cpus", "affinity", "mask"} {
			raw, exists := kv[field]
			if !exists {
				continue
			}
			if _, valid, reason := parseCPUSetListStrict(cleanTraceValue(raw)); !valid {
				out = append(out, cpuInputIntegrityFailure{EventName: rawType, Field: field, Raw: clampString(raw, 80), ReasonCode: reason, Line: lineNo, Ts: ts, CPU: headerCPU})
			}
		}
	}
	if typ == EventPerfSample {
		if !perfSampleCPUIsExplicitNoClaim(kv) {
			addScalar("cpu")
		}
	}
	return out
}

// Most traces use one-to-three digit CPU headers. Avoid a second full ftrace
// regexp match on that hot path; only a bracket token with at least four
// decimal digits can exceed maxTraceCPUIndex and warrants exact validation.
func traceHeaderCPUCouldExceedLimit(line string) bool {
	for offset := 0; offset < len(line); {
		open := strings.IndexByte(line[offset:], '[')
		if open < 0 {
			return false
		}
		open += offset
		closeRel := strings.IndexByte(line[open+1:], ']')
		if closeRel < 0 {
			return false
		}
		close := open + 1 + closeRel
		token := strings.TrimSpace(line[open+1 : close])
		if len(token) >= 4 {
			allDigits := true
			for _, r := range token {
				if r < '0' || r > '9' {
					allDigits = false
					break
				}
			}
			if allDigits {
				return true
			}
		}
		offset = close + 1
	}
	return false
}

func appendCPUInputIntegrityFailure(idx *Index, failure cpuInputIntegrityFailure) {
	if idx == nil {
		return
	}
	for _, existing := range idx.cpuInputIntegrityFailures {
		if existing.Line == failure.Line && existing.EventName == failure.EventName && existing.Field == failure.Field {
			return
		}
	}
	if len(idx.cpuInputIntegrityFailures) >= cpuInputIntegrityIssueCap {
		idx.cpuInputIntegrityFailuresCapped = true
		return
	}
	idx.cpuInputIntegrityFailures = append(idx.cpuInputIntegrityFailures, failure)
}

func cpuInputIntegrityFailureRelevantToQuery(f cpuInputIntegrityFailure, q Query) bool {
	if q.LineStart > 0 && f.Line < q.LineStart {
		return false
	}
	if q.LineEnd > 0 && f.Line > q.LineEnd {
		return false
	}
	return q.TimeEnd <= 0 || f.Ts <= q.TimeEnd
}

func cpuInputIntegrityCaveats(idx *Index, q Query) []string {
	if idx == nil {
		return nil
	}
	fields := map[string]bool{}
	count := 0
	var first *cpuInputIntegrityFailure
	for i := range idx.cpuInputIntegrityFailures {
		failure := idx.cpuInputIntegrityFailures[i]
		if !cpuInputIntegrityFailureRelevantToQuery(failure, q) {
			continue
		}
		count++
		fields[failure.Field] = true
		if first == nil {
			copy := failure
			first = &copy
		}
	}
	if count == 0 && !idx.cpuInputIntegrityFailuresCapped {
		return nil
	}
	fieldList := make([]string, 0, len(fields))
	for field := range fields {
		fieldList = append(fieldList, field)
	}
	sort.Strings(fieldList)
	scope := fmt.Sprintf("invalid_rows=%d fields=%v", count, fieldList)
	if idx.cpuInputIntegrityFailuresCapped {
		scope += fmt.Sprintf(" witness_cap=%d additional_rows_unknown=true", cpuInputIntegrityIssueCap)
	}
	if first != nil {
		scope += "; first=" + first.reason()
	}
	return []string{"cpu_input_integrity_degraded=true; " + scope + "; malformed/out-of-range CPU identities were ignored and did not enter scheduler/interrupt/perf/resource attribution or frequency residency/topology/fold/census/constraint outputs"}
}
