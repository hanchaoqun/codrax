package tracequery

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Trace-mark endpoints are hard state-machine inputs. ParseLine validates the
// full wire payload before Event.FieldText is bounded for inventory display;
// a searchable row with an empty SpanAction and a retained B/E/S/F/G/H prefix
// is therefore malformed by construction. Never re-admit it by validating the
// bounded FieldText again: truncation can remove the invalid tail. The bounded
// private Index ledger retains the exact full-payload action/reason witness.
type traceMarkInvalidAction uint8

const (
	traceMarkActionValid traceMarkInvalidAction = iota
	traceMarkActionB
	traceMarkActionE
	traceMarkActionS
	traceMarkActionF
	traceMarkActionG
	traceMarkActionH
	traceMarkActionN
	traceMarkActionI
)

func traceMarkActionCode(action string) traceMarkInvalidAction {
	switch action {
	case "B":
		return traceMarkActionB
	case "E":
		return traceMarkActionE
	case "S":
		return traceMarkActionS
	case "F":
		return traceMarkActionF
	case "G":
		return traceMarkActionG
	case "H":
		return traceMarkActionH
	case "N":
		return traceMarkActionN
	case "I":
		return traceMarkActionI
	default:
		return traceMarkActionValid
	}
}

func (a traceMarkInvalidAction) String() string {
	switch a {
	case traceMarkActionB:
		return "B"
	case traceMarkActionE:
		return "E"
	case traceMarkActionS:
		return "S"
	case traceMarkActionF:
		return "F"
	case traceMarkActionG:
		return "G"
	case traceMarkActionH:
		return "H"
	case traceMarkActionN:
		return "N"
	case traceMarkActionI:
		return "I"
	default:
		return ""
	}
}

type traceMarkInvalidReason uint8

const (
	traceMarkReasonValid traceMarkInvalidReason = iota
	traceMarkReasonInvalidPayloadPID
	traceMarkReasonPayloadPIDMustBePositive
	traceMarkReasonEmptyName
	traceMarkReasonEmptyCookie
	traceMarkReasonInvalidCookie
	traceMarkReasonEmptyTrack
	traceMarkReasonInvalidArity
	traceMarkReasonInvalidEndTag
	traceMarkReasonInvalidEmitterPID
	traceMarkReasonInvalidHeaderCPU
	traceMarkReasonInvalidTimestamp
	traceMarkReasonUnparseableHeader
)

func (r traceMarkInvalidReason) String() string {
	switch r {
	case traceMarkReasonInvalidPayloadPID:
		return "invalid_payload_pid"
	case traceMarkReasonPayloadPIDMustBePositive:
		return "payload_pid_must_be_positive"
	case traceMarkReasonEmptyName:
		return "empty_name"
	case traceMarkReasonEmptyCookie:
		return "empty_cookie"
	case traceMarkReasonInvalidCookie:
		return "invalid_cookie"
	case traceMarkReasonEmptyTrack:
		return "empty_track_name"
	case traceMarkReasonInvalidArity:
		return "invalid_arity"
	case traceMarkReasonInvalidEndTag:
		return "invalid_end_tag"
	case traceMarkReasonInvalidEmitterPID:
		return "invalid_emitter_pid"
	case traceMarkReasonInvalidHeaderCPU:
		return "invalid_header_cpu"
	case traceMarkReasonInvalidTimestamp:
		return "invalid_timestamp"
	case traceMarkReasonUnparseableHeader:
		return "unparseable_event_header"
	default:
		return ""
	}
}

type traceMarkParseResult struct {
	action        string
	spanPID       int
	name          string
	track         string
	value         string
	counter       traceCounterSample
	counterParsed bool
	invalidAction traceMarkInvalidAction
	invalidReason traceMarkInvalidReason
}

// parseATraceTrackCookie accepts exactly the signed decimal int32 wire shape
// emitted by AOSP's atrace ASYNC_FOR_TRACK helpers. The canonical decimal is
// used as pairing identity so equivalent numeric spellings cannot split a
// logical lane; FieldText retains the producer's original payload verbatim.
func parseATraceTrackCookie(raw string) (string, bool) {
	if raw == "" || raw != strings.TrimSpace(raw) {
		return "", false
	}
	start := 0
	if raw[0] == '-' {
		start = 1
	}
	if start == len(raw) || !isAllDigits(raw[start:]) {
		return "", false
	}
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return "", false
	}
	return strconv.FormatInt(n, 10), true
}

func parseATraceExtendedPID(raw string) (int, bool) {
	if raw == "" || raw != strings.TrimSpace(raw) || !isAllDigits(raw) {
		return 0, false
	}
	// AOSP writes pid_t with %d. Keep the typed owner inside signed int32 so
	// a 64-bit decimal payload cannot masquerade as an Android process id.
	n, err := strconv.ParseUint(raw, 10, 31)
	if err != nil {
		return 0, false
	}
	return int(n), true
}

func parseUnsignedTraceInt(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if !isAllDigits(raw) {
		return 0, false
	}
	n, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || n > uint64(^uint(0)>>1) {
		return 0, false
	}
	return int(n), true
}

// parseExactUnsignedTraceInt is the wire-identity variant used by marker
// payloads. Generic ftrace headers historically tolerate envelope whitespace,
// but a marker scalar sits between literal pipe delimiters: edge whitespace is
// producer data and must not be silently repaired into another identity.
func parseExactUnsignedTraceInt(raw string) (int, bool) {
	if raw == "" || raw != strings.TrimSpace(raw) {
		return 0, false
	}
	return parseUnsignedTraceInt(raw)
}

// parseHarmonyTraceMetadata parses the suffix written by OpenHarmony
// HiTraceMeter: one output-level byte (D/I/C/M) followed by one or two
// two-decimal-digit tag-bit indexes. The current public tag domain is 0..62;
// ParseTagBits prefixes option bit 00 or 05 before one other bit, while its
// ordinary multi-bit scan emits ascending non-option indexes. This token is filtering /
// provenance metadata; it is not an instance, cookie, or logical track id.
func parseHarmonyTraceMetadata(raw string) (level, tagBits string, ok bool) {
	if raw != strings.TrimSpace(raw) {
		return "", "", false
	}
	if len(raw) != 3 && len(raw) != 5 {
		return "", "", false
	}
	if !strings.ContainsRune("DICM", rune(raw[0])) || !isAllDigits(raw[1:]) {
		return "", "", false
	}
	first := int(raw[1]-'0')*10 + int(raw[2]-'0')
	if first > 62 {
		return "", "", false
	}
	if len(raw) == 5 {
		second := int(raw[3]-'0')*10 + int(raw[4]-'0')
		if second > 62 || second == first {
			return "", "", false
		}
		switch first {
		case 0:
			if second == 5 {
				return "", "", false
			}
		case 5:
			// COMMERCIAL option prefix + one other bit. 0501 is
			// producer-reachable even though it is not numerically ascending.
			if second == 0 {
				return "", "", false
			}
		default:
			if first >= second {
				return "", "", false
			}
		}
	}
	containsCommercialBit := first == 5 || len(raw) == 5 && raw[3:5] == "05"
	if containsCommercialBit && raw[0] != 'M' {
		// AddHitraceMeterMarker forces COMMERCIAL-tagged records to level M
		// before formatting. D05/I05/C05 and any two-pair form containing 05
		// are not producer-reachable metadata tokens.
		return "", "", false
	}
	return raw[:1], raw[1:], true
}

func isHarmonyTraceMetadata(raw string) bool {
	_, _, ok := parseHarmonyTraceMetadata(raw)
	return ok
}

// joinTraceMarkEndpointName keeps the producer's complete opaque name byte
// shape. Empty pipe-separated components and edge spaces are valid identity
// bytes, but a sequence made entirely of empty/space components is not a name
// and must fail closed.
func joinTraceMarkEndpointName(parts []string) (string, bool) {
	hasContent := false
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			hasContent = true
			break
		}
	}
	if !hasContent {
		return "", false
	}
	return strings.Join(parts, "|"), true
}

// joinTraceCounterName preserves the producer's complete opaque name. Unlike
// endpoint display normalization, leading/trailing spaces are valid counter
// identity bytes and must not merge two distinct tracks. Only an all-space
// joined name is rejected.
func joinTraceCounterName(parts []string) (string, bool) {
	hasContent := false
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			hasContent = true
			break
		}
	}
	if !hasContent {
		return "", false
	}
	return strings.Join(parts, "|"), true
}

// parseTraceMarkValidated is the single post-normalization schema authority.
// C remains an inventory action even when its identity/value is malformed;
// parseTraceCounterSample owns that typed quality lane. B/E/S/F and G/H are
// duration endpoints and therefore fail closed here; N/I use the same strict
// typed parser but never enter a duration state machine.
func parseTraceMarkValidated(fields string) traceMarkParseResult {
	fields = normalizeTraceMarkPayload(fields)
	if fields == "" {
		return traceMarkParseResult{}
	}
	parts := strings.Split(fields, "|")
	// The action is a closed wire scalar. normalizeTraceMarkPayload removes
	// only the ftrace envelope on the far left; it must not repair bytes inside
	// this pipe-delimited payload.
	action := parts[0]
	result := traceMarkParseResult{action: action}
	invalid := func(reason traceMarkInvalidReason) traceMarkParseResult {
		return traceMarkParseResult{
			invalidAction: traceMarkActionCode(action),
			invalidReason: reason,
		}
	}

	switch action {
	case "B":
		var nameParts []string
		switch {
		case len(parts) == 3:
			nameParts = parts[2:3]
		case len(parts) >= 4 && isHarmonyTraceMetadata(parts[len(parts)-1]):
			// A final exact OpenHarmony metadata token is the only safe right
			// boundary in this no-custom-args shape. B customArgs can follow
			// metadata and require their own unique-boundary parser.
			nameParts = parts[2 : len(parts)-1]
			result.value = parts[len(parts)-1]
		default:
			return invalid(traceMarkReasonInvalidArity)
		}
		pid, ok := parseExactUnsignedTraceInt(parts[1])
		if !ok {
			return invalid(traceMarkReasonInvalidPayloadPID)
		}
		name, ok := joinTraceMarkEndpointName(nameParts)
		if !ok {
			return invalid(traceMarkReasonEmptyName)
		}
		result.spanPID, result.name = pid, name
	case "E":
		if fields == "E" {
			result.name = "E"
			return result
		}
		if len(parts) != 2 && len(parts) != 3 {
			return invalid(traceMarkReasonInvalidArity)
		}
		pid, ok := parseExactUnsignedTraceInt(parts[1])
		if !ok {
			return invalid(traceMarkReasonInvalidPayloadPID)
		}
		result.spanPID = pid
		if len(parts) == 3 {
			tag := parts[2]
			if tag != "" && !isHarmonyTraceMetadata(tag) {
				return invalid(traceMarkReasonInvalidEndTag)
			}
			result.name = tag
		}
		if result.name == "" {
			result.name = fields
		}
	case "S", "F":
		var nameParts []string
		var cookie string
		switch {
		case len(parts) == 4:
			nameParts = parts[2:3]
			cookie = parts[3]
		case len(parts) >= 5 && isHarmonyTraceMetadata(parts[len(parts)-1]):
			// Parse from the right: final exact metadata, then cookie, with every
			// middle field retained as the opaque name. Untagged multi-pipe
			// rows remain ambiguous and fail closed.
			nameParts = parts[2 : len(parts)-2]
			cookie = parts[len(parts)-2]
		default:
			return invalid(traceMarkReasonInvalidArity)
		}
		pid, ok := parseExactUnsignedTraceInt(parts[1])
		if !ok {
			return invalid(traceMarkReasonInvalidPayloadPID)
		}
		if pid == 0 {
			return invalid(traceMarkReasonPayloadPIDMustBePositive)
		}
		name, ok := joinTraceMarkEndpointName(nameParts)
		if !ok {
			return invalid(traceMarkReasonEmptyName)
		}
		if strings.TrimSpace(cookie) == "" {
			return invalid(traceMarkReasonEmptyCookie)
		}
		result.spanPID, result.name, result.value = pid, name, cookie
	case "G":
		// AOSP ASYNC_FOR_TRACK begin:
		// G|pid|track_name|name|cookie. Pipes in names are sanitized by the
		// producer, so any extra field is ambiguous and fails closed.
		if len(parts) != 5 {
			return invalid(traceMarkReasonInvalidArity)
		}
		pid, ok := parseATraceExtendedPID(parts[1])
		if !ok {
			return invalid(traceMarkReasonInvalidPayloadPID)
		}
		if pid == 0 {
			return invalid(traceMarkReasonPayloadPIDMustBePositive)
		}
		track, name := parts[2], parts[3]
		if strings.TrimSpace(track) == "" {
			return invalid(traceMarkReasonEmptyTrack)
		}
		if strings.TrimSpace(name) == "" {
			return invalid(traceMarkReasonEmptyName)
		}
		cookie, ok := parseATraceTrackCookie(parts[4])
		if !ok {
			if strings.TrimSpace(parts[4]) == "" {
				return invalid(traceMarkReasonEmptyCookie)
			}
			return invalid(traceMarkReasonInvalidCookie)
		}
		result.spanPID, result.track, result.name, result.value = pid, track, name, cookie
	case "H":
		// AOSP ASYNC_FOR_TRACK end has no span name:
		// H|pid|track_name|cookie.
		if len(parts) != 4 {
			return invalid(traceMarkReasonInvalidArity)
		}
		pid, ok := parseATraceExtendedPID(parts[1])
		if !ok {
			return invalid(traceMarkReasonInvalidPayloadPID)
		}
		if pid == 0 {
			return invalid(traceMarkReasonPayloadPIDMustBePositive)
		}
		track := parts[2]
		if strings.TrimSpace(track) == "" {
			return invalid(traceMarkReasonEmptyTrack)
		}
		cookie, ok := parseATraceTrackCookie(parts[3])
		if !ok {
			if strings.TrimSpace(parts[3]) == "" {
				return invalid(traceMarkReasonEmptyCookie)
			}
			return invalid(traceMarkReasonInvalidCookie)
		}
		result.spanPID, result.track, result.value = pid, track, cookie
	case "N":
		// AOSP INSTANT_FOR_TRACK: N|pid|track_name|name.
		if len(parts) != 4 {
			return invalid(traceMarkReasonInvalidArity)
		}
		pid, ok := parseATraceExtendedPID(parts[1])
		if !ok {
			return invalid(traceMarkReasonInvalidPayloadPID)
		}
		if pid == 0 {
			return invalid(traceMarkReasonPayloadPIDMustBePositive)
		}
		track, name := parts[2], parts[3]
		if strings.TrimSpace(track) == "" {
			return invalid(traceMarkReasonEmptyTrack)
		}
		if strings.TrimSpace(name) == "" {
			return invalid(traceMarkReasonEmptyName)
		}
		result.spanPID, result.track, result.name = pid, track, name
	case "I":
		// AOSP process instant: I|pid|name.
		if len(parts) != 3 {
			return invalid(traceMarkReasonInvalidArity)
		}
		pid, ok := parseATraceExtendedPID(parts[1])
		if !ok {
			return invalid(traceMarkReasonInvalidPayloadPID)
		}
		if pid == 0 {
			return invalid(traceMarkReasonPayloadPIDMustBePositive)
		}
		name := parts[2]
		if strings.TrimSpace(name) == "" {
			return invalid(traceMarkReasonEmptyName)
		}
		result.spanPID, result.name = pid, name
	case "C":
		// Counter rows remain inventory even when malformed, but their typed
		// owner/name/value verdict is made exactly once from the complete
		// payload. ParseLine stores this result before FieldText is clamped.
		result.counter = parseTraceCounterPayload(fields)
		result.counterParsed = true
		result.spanPID = result.counter.ownerPID
		result.name = result.counter.name
		result.value = result.counter.valueRaw
	default:
		if len(parts) >= 2 {
			result.spanPID, _ = parseUnsignedTraceInt(parts[1])
		}
		if len(parts) >= 3 {
			result.name = strings.TrimSpace(parts[2])
		}
		if len(parts) >= 4 {
			result.value = strings.TrimSpace(parts[3])
		}
	}
	return result
}

func traceMarkEventMalformed(ev Event) bool {
	if ev.Type != EventTraceMark || ev.SpanAction != "" {
		return false
	}
	return traceMarkClassicEndpointAction(traceMarkRetainedEndpointAction(ev.FieldText))
}

func traceMarkClassicEndpointAction(action traceMarkInvalidAction) bool {
	return action == traceMarkActionB || action == traceMarkActionE || action == traceMarkActionS || action == traceMarkActionF
}

func traceMarkTrackEndpointAction(action traceMarkInvalidAction) bool {
	return action == traceMarkActionG || action == traceMarkActionH
}

func traceMarkInstantAction(action traceMarkInvalidAction) bool {
	return action == traceMarkActionN || action == traceMarkActionI
}

// traceMarkRetainedEndpointAction reads only the closed action prefix. It is
// deliberately not a schema validator: Event.FieldText is a bounded inventory
// copy, so a syntactically valid retained prefix cannot overturn the verdict
// already made from the complete payload when SpanAction was minted.
func traceMarkRetainedEndpointAction(fields string) traceMarkInvalidAction {
	normalized := strings.TrimSpace(normalizeTraceMarkPayload(fields))
	if normalized == "E" {
		return traceMarkActionE
	}
	action, _, _ := strings.Cut(normalized, "|")
	return traceMarkActionCode(strings.TrimSpace(action))
}

func traceMarkEventInvalidCodes(ev Event) (traceMarkInvalidAction, traceMarkInvalidReason) {
	if ev.Type != EventTraceMark || ev.SpanAction != "" {
		return traceMarkActionValid, traceMarkReasonValid
	}
	retainedAction := traceMarkRetainedEndpointAction(ev.FieldText)
	if retainedAction == traceMarkActionValid {
		return traceMarkActionValid, traceMarkReasonValid
	}
	parsed := parseTraceMarkValidated(ev.FieldText)
	// Short retained payloads preserve the exact schema reason and keep the
	// existing diagnostic helper behavior. If the inventory copy was truncated
	// into a valid-looking prefix, do not invent a reason: the raw-line Index
	// ledger is the exact authority and traceMarkIntegrityCaveats publishes it.
	if parsed.invalidAction == traceMarkActionValid || parsed.invalidReason == traceMarkReasonValid {
		return retainedAction, traceMarkReasonValid
	}
	return parsed.invalidAction, parsed.invalidReason
}

const traceMarkIntegrityFailureCap = 64

type traceMarkIntegrityFailure struct {
	Action         string
	Reason         string
	Line           int
	LocalLine      int
	Ts             float64
	TimestampKnown bool
	RowPID         int
	EmitterKnown   bool
	// Unmaterialized means ParseLine could not retain this physical marker row
	// at all (invalid event header/timestamp/CPU). A known emitter is not enough
	// for local recovery because endpoint consumers iterate Events and therefore
	// cannot observe the reset at this row's physical position. Such a witness
	// globally fail-closes span/interface pairing for every relevant query.
	Unmaterialized bool
	SourcePath     string
}

func (f traceMarkIntegrityFailure) reason() string {
	rowPID := "unknown"
	if f.EmitterKnown {
		rowPID = strconv.Itoa(f.RowPID)
	}
	ts := "unknown"
	if f.TimestampKnown {
		ts = fmt.Sprintf("%.6f", f.Ts)
	}
	reason := fmt.Sprintf("trace_mark_invalid action=%s reason=%s ts=%s row_pid=%s line=%d",
		f.Action, f.Reason, ts, rowPID, f.Line)
	if f.SourcePath != "" {
		reason += " source=" + f.SourcePath
		if f.LocalLine > 0 {
			reason += fmt.Sprintf(" local_line=%d", f.LocalLine)
		}
	}
	return reason
}

func traceMarkRawCandidate(line string) bool {
	return strings.Contains(line, "tracing_mark_write:") ||
		strings.Contains(line, "tracing_mark_write_xacct:") ||
		strings.Contains(line, "xacct_tracing_mark_write:") ||
		strings.Contains(line, ": print:")
}

func traceMarkPayloadFromRawCandidate(line string) (string, bool) {
	payload, _, ok := traceMarkPayloadAndPrefixFromRawCandidate(line)
	return payload, ok
}

func traceMarkPayloadAndPrefixFromRawCandidate(line string) (string, string, bool) {
	for _, marker := range []string{
		"tracing_mark_write:",
		"tracing_mark_write_xacct:",
		"xacct_tracing_mark_write:",
		": print:",
	} {
		at := strings.LastIndex(line, marker)
		if at < 0 {
			continue
		}
		fields := trimTraceMarkEnvelopeLeft(line[at+len(marker):])
		if isTraceMarkPayload(fields) {
			return fields, line[:at], true
		}
	}
	return "", "", false
}

// traceMarkCorruptedRowRemnantRE is the precise structural signal that a line
// which failed the full ftrace header match is nevertheless a (corrupted)
// ftrace ROW rather than free prose quoting a mark payload: an ftrace-style
// seconds timestamp terminated by ": " or a bracketed CPU column surviving in
// the text BEFORE the marker.
var traceMarkCorruptedRowRemnantRE = regexp.MustCompile(`\d+\.\d{3,9}: |\[\d{1,4}\]`)

// traceMarkValidationFailure records both a malformed endpoint payload and a
// trace-mark row whose header emitter cannot be represented as int. The latter
// cannot be materialized as Event and is therefore a global span-pairing poison.
func traceMarkValidationFailure(lineNo int, line string) *traceMarkIntegrityFailure {
	var scan lineScan
	scan.reset(lineNo, line)
	return traceMarkValidationFailureScan(&scan)
}

// traceMarkValidationFailureScan consumes the shared per-line memo so the hot
// loop pays a single header match per physical line (perf audit #21).
func traceMarkValidationFailureScan(s *lineScan) *traceMarkIntegrityFailure {
	lineNo, line := s.lineNo, s.line
	if !traceMarkRawCandidate(line) {
		return nil
	}
	m := s.match()
	if len(m) == 0 {
		fields, prefix, ok := traceMarkPayloadAndPrefixFromRawCandidate(line)
		if !ok {
			return nil
		}
		action := strings.TrimSpace(strings.SplitN(normalizeTraceMarkPayload(fields), "|", 2)[0])
		actionCode := traceMarkActionCode(action)
		if !traceMarkClassicEndpointAction(actionCode) && !traceMarkTrackEndpointAction(actionCode) && !traceMarkInstantAction(actionCode) {
			return nil
		}
		// ENG audit #4a (§29.25 处置委托 2026-07-10): global span poison is a
		// hard gate and must key on a precise structural signal. A free-prose
		// line that merely QUOTES a mark payload (the common mixed-log
		// artifact shape) used to mint an artifact-global Unmaterialized
		// poison and fail-close every span face for every query. Structural
		// '#' comments never poison, and a poison now requires a
		// corrupted-ftrace-row remnant before the marker; a really corrupted
		// trace-mark row keeps its timestamp/CPU-column remnant and stays
		// fail-closed (pinned by the invalid-header/invalid-timestamp cases in
		// TestUnmaterializedTraceMarkEndpointGloballyFailsClosed, which travel
		// the matched-header branch below).
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			return nil
		}
		if !traceMarkCorruptedRowRemnantRE.MatchString(prefix) {
			return nil
		}
		return &traceMarkIntegrityFailure{
			Action: action, Reason: traceMarkReasonUnparseableHeader.String(),
			Line: lineNo, LocalLine: lineNo, Unmaterialized: true,
		}
	}
	rawType := strings.TrimSuffix(strings.TrimSpace(m[6]), ":")
	fields := trimTraceMarkEnvelopeLeft(m[7])
	if !isPrintFamilyRaw(rawType) || !isTraceMarkPayload(fields) {
		return nil
	}
	normalized := normalizeTraceMarkPayload(fields)
	action := strings.TrimSpace(strings.SplitN(normalized, "|", 2)[0])
	actionCode := traceMarkActionCode(action)
	if !traceMarkClassicEndpointAction(actionCode) && !traceMarkTrackEndpointAction(actionCode) && !traceMarkInstantAction(actionCode) {
		return nil
	}
	failure := &traceMarkIntegrityFailure{Action: action, Line: lineNo, LocalLine: lineNo}
	pid, ok := parseUnsignedTraceInt(m[2])
	if !ok {
		failure.Reason = traceMarkReasonInvalidEmitterPID.String()
		failure.Unmaterialized = true
		return failure
	}
	failure.RowPID, failure.EmitterKnown = pid, true
	ts, ok := parseTraceTimestampSeconds(m[5])
	if !ok {
		failure.Reason = traceMarkReasonInvalidTimestamp.String()
		failure.Unmaterialized = true
		return failure
	}
	failure.Ts, failure.TimestampKnown = ts, true
	if _, present, valid, _ := parseTraceCPUScalar(m[4]); !present || !valid {
		failure.Reason = traceMarkReasonInvalidHeaderCPU.String()
		failure.Unmaterialized = true
		return failure
	}
	parsed := parseTraceMarkValidated(fields)
	if parsed.invalidAction == traceMarkActionValid {
		return nil
	}
	failure.Action = parsed.invalidAction.String()
	failure.Reason = parsed.invalidReason.String()
	return failure
}

func appendTraceMarkIntegrityFailure(idx *Index, failure traceMarkIntegrityFailure) {
	if idx == nil {
		return
	}
	for _, existing := range idx.traceMarkIntegrityFailures {
		if existing.Line == failure.Line && existing.Action == failure.Action && existing.Reason == failure.Reason && existing.SourcePath == failure.SourcePath {
			return
		}
	}
	if len(idx.traceMarkIntegrityFailures) >= traceMarkIntegrityFailureCap {
		idx.traceMarkIntegrityFailuresCapped = true
		action := traceMarkActionCode(failure.Action)
		if traceMarkTrackEndpointAction(action) {
			// A materialized malformed G/H row still cannot identify which
			// payload-owner track lane to reset, so a dropped witness closes
			// the whole track-duration face conservatively.
			idx.traceTrackIntegrityDroppedPoison = true
		} else if traceMarkClassicEndpointAction(action) && (!failure.EmitterKnown || failure.Unmaterialized) {
			idx.traceMarkIntegrityDroppedGlobalPoison = true
		}
		return
	}
	idx.traceMarkIntegrityFailures = append(idx.traceMarkIntegrityFailures, failure)
}

func traceMarkIntegrityFailureRelevantToQuery(f traceMarkIntegrityFailure, q Query) bool {
	if q.LineEnd > 0 && f.Line > q.LineEnd {
		return false
	}
	// A malformed endpoint before the lower boundary may reset or corrupt a
	// carry-in stack. Only an upper bound proves it irrelevant.
	if f.TimestampKnown && q.LineStart == 0 && q.LineEnd == 0 && q.TimeEnd > 0 && f.Ts > q.TimeEnd {
		return false
	}
	return true
}

func traceMarkUnknownEmitterFailureForQuery(idx *Index, q Query) bool {
	if idx == nil {
		return false
	}
	// A witness-cap hit made entirely of materialized, known-emitter rows is
	// still locally recoverable: those malformed Events remain in idx.Events
	// and reset their own emitter during the pairing scan. Global fail-close is
	// required only when the bounded ledger actually dropped a row whose
	// emitter/physical position was unavailable.
	if idx.traceMarkIntegrityDroppedGlobalPoison {
		return true
	}
	for _, failure := range idx.traceMarkIntegrityFailures {
		if traceMarkClassicEndpointAction(traceMarkActionCode(failure.Action)) &&
			(!failure.EmitterKnown || failure.Unmaterialized) && traceMarkIntegrityFailureRelevantToQuery(failure, q) {
			return true
		}
	}
	return false
}

func traceTrackIntegrityFailureForQuery(idx *Index, q Query) *traceMarkIntegrityFailure {
	if idx == nil {
		return nil
	}
	if idx.traceTrackIntegrityDroppedPoison {
		return &traceMarkIntegrityFailure{Action: "G/H", Reason: "integrity_witness_cap_exceeded"}
	}
	for i := range idx.traceMarkIntegrityFailures {
		failure := &idx.traceMarkIntegrityFailures[i]
		if traceMarkTrackEndpointAction(traceMarkActionCode(failure.Action)) && traceMarkIntegrityFailureRelevantToQuery(*failure, q) {
			copy := *failure
			return &copy
		}
	}
	return nil
}

func traceMarkIntegrityCaveats(idx *Index, q Query) []string {
	if idx == nil {
		return nil
	}
	actions, reasons := map[string]bool{}, map[string]bool{}
	count := 0
	var first *traceMarkIntegrityFailure
	for i := range idx.traceMarkIntegrityFailures {
		failure := idx.traceMarkIntegrityFailures[i]
		if !traceMarkIntegrityFailureRelevantToQuery(failure, q) {
			continue
		}
		count++
		actions[failure.Action] = true
		reasons[failure.Reason] = true
		if first == nil {
			copy := failure
			first = &copy
		}
	}
	if count == 0 && !idx.traceMarkIntegrityFailuresCapped {
		return nil
	}
	actionList, reasonList := make([]string, 0, len(actions)), make([]string, 0, len(reasons))
	for action := range actions {
		actionList = append(actionList, action)
	}
	for reason := range reasons {
		reasonList = append(reasonList, reason)
	}
	sort.Strings(actionList)
	sort.Strings(reasonList)
	scope := fmt.Sprintf("invalid_rows=%d actions=%v reasons=%v", count, actionList, reasonList)
	if idx.traceMarkIntegrityFailuresCapped {
		scope += fmt.Sprintf(" witness_cap=%d additional_rows_unknown=true", traceMarkIntegrityFailureCap)
	}
	if first != nil {
		scope += "; first=" + first.reason()
	}
	return []string{"trace_mark_integrity_degraded=true; " + scope + "; malformed B/E/S/F/G/H/N/I rows remain searchable trace_mark inventory and are excluded from their typed duration/instant lanes"}
}

func traceMarkSyncPairingKey(source string, pid int) string {
	return source + "\x00sync\x00" + strconv.Itoa(pid)
}

// resetTraceMarkSyncPairingState invalidates only the known physical source
// and emitter. Async S/F markers use their payload pid rather than their row
// emitter and are reset by traceMarkAsyncPairer below.
func resetTraceMarkSyncPairingState(source string, pid int, stacks map[string][]Event) {
	if source == "" || pid < 0 {
		return
	}
	delete(stacks, traceMarkSyncPairingKey(source, pid))
}

// traceMarkAsyncOwner is the lifecycle namespace for ATrace S/F markers. The
// payload pid is the logical owner; the ftrace row emitter may legitimately
// differ between S and F and therefore cannot be part of the wire identity.
type traceMarkAsyncOwner struct {
	source     string
	payloadPID int
}

// traceMarkAsyncKey is the exact S/F duration identity. generation advances
// only on an exact scheduler lifecycle boundary for payloadPID in the same
// physical source, so a reused numeric pid cannot close an older occupant's
// async marker. A comparable struct avoids delimiter collisions in opaque
// span names and cookies.
type traceMarkAsyncKey struct {
	owner      traceMarkAsyncOwner
	generation uint64
	name       string
	cookie     string
}

type traceMarkAsyncLane struct {
	cohort   pairingCohortState
	emitters map[int]struct{}
}

type traceMarkAsyncPair struct {
	source string
	start  Event
	end    Event
}

// traceMarkAsyncPairer is shared by window_stats/root-rank and span_window.
// A second S on one exact key makes the whole overlapping cohort ambiguous;
// F endpoints then only discharge depth until zero, and no guessed LIFO/FIFO
// pair is published. The lane is reusable immediately after that zero point.
type traceMarkAsyncPairer struct {
	q            Query
	startMatches func(Event) bool
	onPair       func(traceMarkAsyncPair)

	lanes       map[traceMarkAsyncKey]*traceMarkAsyncLane
	generations map[traceMarkAsyncOwner]uint64

	ambiguousCohorts int
	ambiguousStarts  int
	incompleteBegins int
	orphanEnds       int
	lifecycleCuts    int
	malformedCuts    int
}

func newTraceMarkAsyncPairer(q Query, startMatches func(Event) bool, onPair func(traceMarkAsyncPair)) *traceMarkAsyncPairer {
	return &traceMarkAsyncPairer{
		q:            q,
		startMatches: startMatches,
		onPair:       onPair,
		lanes:        map[traceMarkAsyncKey]*traceMarkAsyncLane{},
		generations:  map[traceMarkAsyncOwner]uint64{},
	}
}

// traceMarkAsyncPayloadPID retains compatibility for hand-built Event values
// that predate SpanPID. Production S/F rows always take the first branch:
// parseTraceMarkValidated requires a positive payload pid before SpanAction is
// minted.
func traceMarkAsyncPayloadPID(ev Event) int {
	if ev.SpanPID > 0 {
		return ev.SpanPID
	}
	if ev.TGID > 0 {
		return ev.TGID
	}
	return ev.PID
}

func (p *traceMarkAsyncPairer) key(source string, ev Event) (traceMarkAsyncKey, bool) {
	if p == nil || source == "" || (ev.SpanAction != "S" && ev.SpanAction != "F") {
		return traceMarkAsyncKey{}, false
	}
	payloadPID := traceMarkAsyncPayloadPID(ev)
	if payloadPID <= 0 || strings.TrimSpace(ev.SpanName) == "" || strings.TrimSpace(ev.SpanValue) == "" {
		return traceMarkAsyncKey{}, false
	}
	owner := traceMarkAsyncOwner{source: source, payloadPID: payloadPID}
	return traceMarkAsyncKey{
		owner:      owner,
		generation: p.generations[owner],
		name:       ev.SpanName,
		cookie:     ev.SpanValue,
	}, true
}

func (p *traceMarkAsyncPairer) matchesStart(ev Event) bool {
	return p != nil && (p.startMatches == nil || p.startMatches(ev))
}

func traceMarkAsyncStartMayReachQuery(start Event, q Query) bool {
	if q.LineEnd > 0 && start.Line > q.LineEnd {
		return false
	}
	if q.LineStart == 0 && q.LineEnd == 0 && queryBoundedTimeEnd(q) && start.Ts > q.TimeEnd {
		return false
	}
	return true
}

func (p *traceMarkAsyncPairer) observeEndpoint(source string, ev Event) {
	key, ok := p.key(source, ev)
	if !ok {
		return
	}
	lane := p.lanes[key]
	if ev.SpanAction == "S" {
		if lane == nil {
			lane = &traceMarkAsyncLane{emitters: map[int]struct{}{}}
			p.lanes[key] = lane
		}
		if ev.PID >= 0 {
			lane.emitters[ev.PID] = struct{}{}
		}
		lane.cohort.observeStart(ev)
		return
	}
	if lane == nil {
		// F carries the same payload name/cookie, but its row emitter is not
		// proof of the missing begin emitter. Name scoping remains precise;
		// thread-scoped span_window queries conservatively retain the orphan
		// disclosure because no start exists from which to prove it unrelated.
		if pairingEventInsideQuery(ev, p.q) && traceMarkAsyncNameMatchesQuery(ev, p.q) {
			p.orphanEnds++
		}
		return
	}
	transition := lane.cohort.observeDone(ev)
	if !transition.cohortClosed {
		return
	}
	delete(p.lanes, key)
	if transition.ambiguous {
		p.accountClosedAmbiguous(transition)
		return
	}
	if transition.pairReady && ev.Ts >= transition.pairStart.Ts && p.onPair != nil {
		p.onPair(traceMarkAsyncPair{source: source, start: transition.pairStart, end: ev})
	}
}

func traceMarkAsyncNameMatchesQuery(ev Event, q Query) bool {
	name := strings.TrimSpace(q.SpanName)
	return name == "" || strings.Contains(strings.ToLower(ev.SpanName), strings.ToLower(name))
}

func (p *traceMarkAsyncPairer) accountClosedAmbiguous(transition pairingCohortTransition) {
	if !p.matchesStart(transition.first) || !pairingIntervalIntersectsQuery(transition.first, transition.last, p.q) {
		return
	}
	p.ambiguousCohorts++
	p.ambiguousStarts += transition.cohortStarts
}

func (p *traceMarkAsyncPairer) accountCut(transition pairingCohortTransition, boundary Event, lifecycle bool) {
	if !transition.cohortClosed || !p.matchesStart(transition.first) || !pairingIntervalIntersectsQuery(transition.first, boundary, p.q) {
		return
	}
	if lifecycle {
		p.lifecycleCuts++
	} else {
		p.malformedCuts++
	}
	if transition.ambiguous {
		p.ambiguousCohorts++
		p.ambiguousStarts += transition.cohortStarts
		return
	}
	p.incompleteBegins += transition.cohortStarts
}

// observeLifecycle closes every old-generation lane for this payload owner,
// then advances the exact (source,payload pid) generation even when no lane is
// currently open. A later F therefore cannot reach across the boundary.
func (p *traceMarkAsyncPairer) observeLifecycle(source string, payloadPID int, boundary Event) {
	if p == nil || source == "" || payloadPID <= 0 {
		return
	}
	owner := traceMarkAsyncOwner{source: source, payloadPID: payloadPID}
	for key, lane := range p.lanes {
		if key.owner != owner {
			continue
		}
		transition := lane.cohort.finishEOF()
		p.accountCut(transition, boundary, true)
		delete(p.lanes, key)
	}
	p.generations[owner]++
}

// observeMalformed preserves the established known-emitter recovery scope:
// only cohorts with a begin physically emitted by this source/pid are cut.
// The malformed row has no trustworthy payload identity and therefore cannot
// advance or select a payload-owner generation.
func (p *traceMarkAsyncPairer) observeMalformed(source string, emitterPID int, boundary Event) {
	if p == nil || source == "" || emitterPID < 0 {
		return
	}
	for key, lane := range p.lanes {
		if key.owner.source != source {
			continue
		}
		if _, ok := lane.emitters[emitterPID]; !ok {
			continue
		}
		transition := lane.cohort.finishEOF()
		p.accountCut(transition, boundary, false)
		delete(p.lanes, key)
	}
}

func (p *traceMarkAsyncPairer) openStarts() []Event {
	if p == nil || len(p.lanes) == 0 {
		return nil
	}
	out := make([]Event, 0, len(p.lanes))
	for _, lane := range p.lanes {
		if lane.cohort.depth > 0 {
			out = append(out, lane.cohort.first)
		}
	}
	return out
}

func (p *traceMarkAsyncPairer) finishEOF() {
	if p == nil {
		return
	}
	for key, lane := range p.lanes {
		transition := lane.cohort.finishEOF()
		if p.matchesStart(transition.first) && traceMarkAsyncStartMayReachQuery(transition.first, p.q) {
			if transition.ambiguous {
				p.ambiguousCohorts++
				p.ambiguousStarts += transition.cohortStarts
			} else {
				p.incompleteBegins += transition.cohortStarts
			}
		}
		delete(p.lanes, key)
	}
}

func (p *traceMarkAsyncPairer) caveats() []string {
	if p == nil {
		return nil
	}
	var out []string
	if p.ambiguousCohorts > 0 {
		out = append(out, fmt.Sprintf("trace_mark_async_duplicate_key_fail_closed=true; ambiguous_cohorts=%d ambiguous_starts=%d; concurrent/repeated S endpoints with the same source+payload_pid+generation+name+cookie were withheld instead of LIFO-paired", p.ambiguousCohorts, p.ambiguousStarts))
	}
	if p.incompleteBegins > 0 || p.orphanEnds > 0 || p.lifecycleCuts > 0 || p.malformedCuts > 0 {
		out = append(out, fmt.Sprintf("trace_mark_async_pairing_incomplete=true; incomplete_begins=%d orphan_ends=%d lifecycle_cuts=%d malformed_cuts=%d; incomplete S/F endpoints remain searchable trace_mark inventory and mint no duration", p.incompleteBegins, p.orphanEnds, p.lifecycleCuts, p.malformedCuts))
	}
	return out
}
