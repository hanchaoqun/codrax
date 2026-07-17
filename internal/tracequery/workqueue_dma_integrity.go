package tracequery

import (
	"sort"
	"strconv"
	"strings"
)

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
	_, ok := canonicalWorkqueuePointerIdentity(raw)
	return ok
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
	_, ok := canonicalUnsignedTraceIdentity(raw)
	return ok
}

// Exact duration endpoints use only the upstream tracepoint field names.
// Wider aliases remain available to inventory/display parsing, but may not
// satisfy a hard pairing identity.
func workqueueExactEndpointFields(ev Event) (work, function string) {
	tokens, lexOK := tokenizePairingKV(ev.FieldText)
	return workqueueExactEndpointFieldsFromTokens(ev.FieldText, tokens, lexOK)
}

func workqueueExactEndpointFieldsFromTokens(fieldText string, tokens []pairingKVToken, lexOK bool) (work, function string) {
	work, workPresent, workValid := strictOptionalPairingAliasTokens(tokens, lexOK, "work")
	if !workValid {
		return "", ""
	}
	if !workPresent {
		work = workqueuePositionalWorkPointer(fieldText)
	}
	function, functionPresent, functionValid := strictOptionalPairingAliasTokens(tokens, lexOK, "function")
	if !functionValid {
		function = ""
	} else if !functionPresent {
		function = valueAfterLabel(fieldText, "function")
		if strict, ok := strictPairingScalar(function); ok {
			function = strict
		} else {
			function = ""
		}
	}
	return strings.TrimRight(work, ":"), function
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
	value, ok := strictPairingScalar(strings.Fields(rest)[0])
	if !ok {
		return ""
	}
	return strings.TrimRight(value, ":")
}

func dmaFenceExactEndpointFields(ev Event) (driver, timeline, context, seqno string) {
	tokens, lexOK := tokenizePairingKV(ev.FieldText)
	return dmaFenceExactEndpointFieldsFromTokens(tokens, lexOK)
}

func dmaFenceExactEndpointFieldsFromTokens(tokens []pairingKVToken, lexOK bool) (driver, timeline, context, seqno string) {
	driver, _, _ = strictOptionalPairingAliasTokens(tokens, lexOK, "driver")
	timeline, _, _ = strictOptionalPairingAliasTokens(tokens, lexOK, "timeline")
	context, _, _ = strictOptionalPairingAliasTokens(tokens, lexOK, "context")
	seqno, _, _ = strictOptionalPairingAliasTokens(tokens, lexOK, "seqno")
	return driver, timeline, context, seqno
}

func durationExactEndpointFamily(rawType string) durationOrderFamily {
	profile, ok := pairingEndpointProfileForName(rawType)
	if ok {
		switch profile.Family {
		case PairingEndpointWorkqueue:
			return durationOrderWorkqueue
		case PairingEndpointDMAFence:
			return durationOrderDMAFence
		case PairingEndpointBinder:
			return durationOrderBinder
		case PairingEndpointBlock:
			return durationOrderBlockIO
		}
	}
	lowerType := strings.ToLower(strings.TrimSpace(rawType))
	if isStorageEvent(lowerType) || isFilesystemEvent(lowerType) {
		profile, ok = genericStoragePairingProfile(rawType)
	}
	if ok && profile.Family == PairingEndpointStorage {
		return durationOrderStorage
	}
	return ""
}

// durationEndpointFallbackCandidate is the O(1) prescreen for the fallback
// grammar. Keep this closed to families consumed by the hard elapsed-pairing
// authority: malformed headers do not match ftraceLineRE, so without this
// fallback an exact Block/Storage endpoint could disappear before the
// physical-topology audit sees it. The outer parser already applies the wider
// durationOrderRawCandidate gate; this narrower check avoids a second full
// regex for scheduler/interrupt/counter rows.
func durationEndpointFallbackCandidate(line string) bool {
	for _, token := range [...]string{
		"binder_transaction", "workqueue_execute_", "dma_fence_wait_",
		"block_rq_issue", "block_rq_complete", "block_bio_queue", "block_bio_complete",
		"ufshcd_", "mmc_", "scsi_", "i2c_", "smbus_", "bio_", "ebpf_bio",
		"f2fs_", "hmfs_", "android_fs_", "ext4_",
		"filesystem", "file_system", "ebpf_file",
	} {
		if strings.Contains(line, token) {
			return true
		}
	}
	return false
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
		// The main grammar has already identified the one physical top-level
		// event column. Never run the looser malformed-header fallback over its
		// body: a print/prose payload may itself quote a complete ftrace line,
		// and the fallback's permissive header scalars must not reinterpret that
		// nested text as a second hard pairing endpoint.
		return nil
	}
	if !durationEndpointFallbackCandidate(s.line) {
		return nil
	}
	// Canonical parsing rejected the physical row. Reuse the same bounded
	// outer-header locator as the main grammar instead of running a second
	// global lazy-comm regex: the latter can both cross a malformed outer
	// print header and mis-split numeric suffixes in real thread names.
	return loosePhysicalFtraceLine(s.line)
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
	// A main-grammar Binder row will materialize as an Event even when its
	// transaction payload is invalid; auditBinderPairing then applies the same
	// source/exact-lane verdict with physical provenance. Avoid decoding every
	// valid Binder payload twice on this hot path. If Event admission later
	// rejects the row, durationEndpointRejectedRowFailureScan performs the raw
	// decode and records the barrier. Header rows that miss the main grammar
	// (malformed PID/CPU/timestamp shape) still take the fallback validation
	// below and cannot disappear.
	if family == durationOrderBinder && len(s.match()) != 0 {
		return nil
	}

	pid, pidOK := parseFtraceHeaderTID(m[2])
	cpu, cpuPresent, cpuValid, _ := parseTraceCPUScalar(m[4])
	ts, tsOK := parseTraceTimestampSeconds(m[5])
	ev := Event{PID: pid, CPU: cpu, Type: EventWorkqueue, Name: rawType, FieldText: strings.TrimSpace(m[7])}
	headerTID := int64(-1)
	if pidOK {
		headerTID = int64(pid)
	}
	verdict := DecodePairingEndpoint(rawType, ev.FieldText, headerTID)
	var missing []string
	if !pidOK || !verdict.EmitterKnown || (verdict.RequiresPositiveEmitter && !verdict.EmitterAdmitted) {
		missing = append(missing, "pid")
	}
	if !cpuPresent || !cpuValid {
		missing = append(missing, "header_cpu")
	}
	if !tsOK {
		missing = append(missing, "timestamp")
	}
	switch family {
	case durationOrderBinder:
		if !verdict.Recognized || !verdict.KeyKnown {
			missing = append(missing, "canonical_pairing_identity")
		} else if !verdict.PayloadAdmitted {
			missing = append(missing, "payload_admission")
		}
	case durationOrderWorkqueue:
		work, _ := workqueueExactEndpointFields(ev)
		if !validWorkqueuePointerIdentity(work) {
			missing = append(missing, "work")
		}
	case durationOrderDMAFence:
		ev.Type = EventDMAFence
		driver, timeline, context, seqno := dmaFenceExactEndpointFields(ev)
		missing = append(missing, dmaFenceEndpointMissingFields(ev, driver, timeline, context, seqno)...)
	case durationOrderBlockIO, durationOrderStorage:
		if !verdict.Recognized || !verdict.KeyKnown {
			missing = append(missing, "canonical_pairing_identity")
		} else if !verdict.PayloadAdmitted {
			missing = append(missing, "payload_admission")
		}
	}
	missing = uniqueSortedStrings(missing)
	if len(missing) == 0 {
		return nil
	}
	laneKey := ""
	if verdict.KeyKnown {
		laneKey = verdict.SemanticKey
	}
	return &durationOrderViolation{
		Family: family, LaneKey: laneKey, Issue: "endpoint_parse_incomplete", EventName: rawType,
		Fields: missing, CurrentTs: ts, TsUnknown: !tsOK, Line: lineNo,
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
	headerTID := int64(-1)
	if pid, pidOK := parseFtraceHeaderTID(m[2]); pidOK {
		headerTID = int64(pid)
	}
	verdict := DecodePairingEndpoint(rawType, strings.TrimSpace(m[7]), headerTID)
	laneKey := ""
	if verdict.KeyKnown {
		laneKey = verdict.SemanticKey
	}
	return &durationOrderViolation{
		Family: family, LaneKey: laneKey, Issue: "endpoint_parse_incomplete", EventName: rawType,
		Fields: []string{"parser_rejected_row"}, CurrentTs: ts, TsUnknown: !ok, Line: s.lineNo,
	}
}

// MalformedPairingEndpointProbe is the smallest source-neutral result needed
// by ingestion adapters that must retain exact endpoint provenance even when
// their normal ftrace parser rejects the physical row. It intentionally
// exposes neither duration-order internals nor a family guess: callers must
// still apply their own exact, closed endpoint-name roster.
type MalformedPairingEndpointProbe struct {
	Name        string
	SemanticKey string
	KeyKnown    bool
}

// ProbeMalformedPairingEndpoint reports a precise physical pairing endpoint
// that failed canonical row admission. The same raw grammar and typed payload
// decoder used by tracequery's duration-order barrier are the sole authority;
// prose, substrings, case variants, and near names cannot match the anchored
// ftrace shape. A normally parseable row is never reported by the rejected-row
// fallback.
func ProbeMalformedPairingEndpoint(line string) (MalformedPairingEndpointProbe, bool) {
	var scan lineScan
	scan.reset(0, line)
	failure := durationEndpointRawValidationFailureScan(&scan)
	if failure == nil {
		if _, parsed := parseLineScan(&scan, nil); parsed {
			return MalformedPairingEndpointProbe{}, false
		}
		failure = durationEndpointRejectedRowFailureScan(&scan)
	}
	if failure == nil || failure.EventName == "" {
		return MalformedPairingEndpointProbe{}, false
	}
	return MalformedPairingEndpointProbe{
		Name:        failure.EventName,
		SemanticKey: failure.LaneKey,
		KeyKnown:    failure.LaneKey != "",
	}, true
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
	case EventCPUFrequency:
		// Reclassified clock_set_rate lanes are corroboration-only and never
		// govern per-CPU frequency carry-in. Exact/generalized cpu_frequency
		// rows, however, are state transitions: one malformed transition is a
		// barrier, not a row that older state may silently bridge across.
		if !ev.CPUInputInvalid || ev.Name == "clock_set_rate" {
			return nil
		}
		family = durationOrderCPUFrequency
		if !ev.CPUForFieldValid {
			missing = append(missing, "cpu_id")
		} else {
			missing = append(missing, "frequency")
		}
	case EventCPUFrequencyLimit:
		if !ev.CPUInputInvalid {
			return nil
		}
		family = durationOrderCPUFreqLimit
		if !ev.CPUForFieldValid {
			missing = append(missing, "cpu_id")
		} else {
			missing = append(missing, "min|max")
		}
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
	if family == durationOrderCPUFrequency || family == durationOrderCPUFreqLimit {
		laneKey := ""
		if ev.CPUForFieldValid {
			laneKey = strconv.Itoa(ev.CPUForField)
		}
		return &durationOrderViolation{
			Family: family, LaneKey: laneKey, Issue: "endpoint_parse_incomplete", EventName: strings.TrimSpace(ev.Name),
			Fields: missing, CurrentTs: ev.Ts, Line: ev.Line,
		}
	}
	verdict := fingerprintPairingEvent(ev)
	laneKey := ""
	if verdict.KeyKnown {
		laneKey = verdict.SemanticKey
	}
	return &durationOrderViolation{
		Family: family, LaneKey: laneKey, Issue: "endpoint_parse_incomplete", EventName: strings.TrimSpace(ev.Name),
		Fields: missing, CurrentTs: ev.Ts, Line: ev.Line,
	}
}

// cpuScalarRejectedRowFailureScan preserves a CPU state-transition barrier
// when the outer ftrace envelope is rejected before Event construction. The
// occurrence-aware payload receipt can still localize the controlled CPU; if
// it cannot, only this scalar family is failed closed.
func cpuScalarRejectedRowFailureScan(s *lineScan) *durationOrderViolation {
	if s == nil {
		return nil
	}
	m := s.match()
	if len(m) == 0 {
		// Header PID/CPU/timestamp damage can prevent the canonical envelope
		// regex from matching while the physical event column and strict CPU
		// scalar payload remain recoverable. Use the same bounded, outer-shell
		// locator as other rejected endpoint audits. It cannot reinterpret a
		// nested print payload when a canonical outer event already matched: the
		// fallback is entered only on a complete canonical miss.
		if !fullFreqCurveRawCandidate(s.line) {
			return nil
		}
		m = loosePhysicalFtraceLine(s.line)
		if len(m) == 0 {
			return nil
		}
	}
	rawType := strings.TrimSuffix(strings.TrimSpace(m[6]), ":")
	profile := cpuScalarProfileForName(rawType)
	var family durationOrderFamily
	var scalarField string
	switch profile {
	case cpuScalarProfileFrequency:
		family, scalarField = durationOrderCPUFrequency, "frequency"
	case cpuScalarProfileLimits:
		family, scalarField = durationOrderCPUFreqLimit, "min|max"
	default:
		return nil
	}
	_, typed := parseCPUScalarTypedFields(rawType, strings.TrimSpace(m[7]))
	if !typed.Parsed {
		return nil
	}
	fields := []string{"event_header"}
	if !typed.ValueKnown {
		fields = append(fields, scalarField)
	}
	if !typed.CPUKnown {
		fields = append(fields, "cpu_id")
	}
	laneKey := ""
	if typed.CPUKnown {
		laneKey = strconv.Itoa(typed.CPU)
	}
	ts, tsOK := parseTraceTimestampSeconds(m[5])
	return &durationOrderViolation{
		Family: family, LaneKey: laneKey, Issue: "endpoint_parse_incomplete", EventName: rawType,
		Fields: fields, CurrentTs: ts, TsUnknown: !tsOK, Line: s.lineNo,
	}
}
