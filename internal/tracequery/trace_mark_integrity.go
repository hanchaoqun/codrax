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
	invalidAction traceMarkInvalidAction
	invalidReason traceMarkInvalidReason
}

// parseATraceTrackCookie accepts exactly the signed decimal int32 wire shape
// emitted by AOSP's atrace ASYNC_FOR_TRACK helpers. The canonical decimal is
// used as pairing identity so equivalent numeric spellings cannot split a
// logical lane; FieldText retains the producer's original payload verbatim.
func parseATraceTrackCookie(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
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
	raw = strings.TrimSpace(raw)
	if !isAllDigits(raw) {
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

// isTraceMarkSpanInstanceTag is the precise right-boundary token for native
// B/S/F span endpoints. Production Harmony emitters use I<number> and
// M<number>; only those two witnessed families may unlock the otherwise
// ambiguous pipe-containing name grammar. isInstanceTag remains broader for
// the carved-payload heuristic and legacy E/C compatibility, where a single
// terminal field cannot accidentally absorb a name/cookie boundary.
func isTraceMarkSpanInstanceTag(raw string) bool {
	raw = strings.TrimSpace(raw)
	if len(raw) < 2 || raw[0] != 'I' && raw[0] != 'M' {
		return false
	}
	return isAllDigits(raw[1:])
}

// joinTraceMarkEndpointName keeps the producer's opaque name byte shape after
// trimming only outer whitespace. Empty pipe-separated components are valid
// inside a real name (including a trailing empty component before an exact
// instance tag), but a sequence made entirely of empty/space components is
// not a name and must fail closed.
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
	return strings.TrimSpace(strings.Join(parts, "|")), true
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
	action := strings.TrimSpace(parts[0])
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
		case len(parts) >= 4 && isTraceMarkSpanInstanceTag(parts[len(parts)-1]):
			// The exact I/M tag is the only safe right boundary when an
			// opaque span name itself contains pipes or empty components.
			nameParts = parts[2 : len(parts)-1]
			result.value = strings.TrimSpace(parts[len(parts)-1])
		default:
			return invalid(traceMarkReasonInvalidArity)
		}
		pid, ok := parseUnsignedTraceInt(parts[1])
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
		pid, ok := parseUnsignedTraceInt(parts[1])
		if !ok {
			return invalid(traceMarkReasonInvalidPayloadPID)
		}
		result.spanPID = pid
		if len(parts) == 3 {
			tag := strings.TrimSpace(parts[2])
			if tag != "" && !isInstanceTag(tag) {
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
			cookie = strings.TrimSpace(parts[3])
		case len(parts) >= 5 && isTraceMarkSpanInstanceTag(parts[len(parts)-1]):
			// Parse from the right: final exact tag, then cookie, with every
			// middle field retained as the opaque name. Untagged multi-pipe
			// rows remain ambiguous and fail closed.
			nameParts = parts[2 : len(parts)-2]
			cookie = strings.TrimSpace(parts[len(parts)-2])
		default:
			return invalid(traceMarkReasonInvalidArity)
		}
		pid, ok := parseUnsignedTraceInt(parts[1])
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
		if cookie == "" {
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
		track, name := strings.TrimSpace(parts[2]), strings.TrimSpace(parts[3])
		if track == "" {
			return invalid(traceMarkReasonEmptyTrack)
		}
		if name == "" {
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
		track := strings.TrimSpace(parts[2])
		if track == "" {
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
		track, name := strings.TrimSpace(parts[2]), strings.TrimSpace(parts[3])
		if track == "" {
			return invalid(traceMarkReasonEmptyTrack)
		}
		if name == "" {
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
		name := strings.TrimSpace(parts[2])
		if name == "" {
			return invalid(traceMarkReasonEmptyName)
		}
		result.spanPID, result.name = pid, name
	case "C":
		// C is deliberately permissive here. Its inventory row remains visible;
		// numeric aggregation and identity voting both re-enter the closed schema
		// in parseTraceCounterSample.
		if len(parts) >= 2 {
			result.spanPID, _ = parseUnsignedTraceInt(parts[1])
		}
		if len(parts) >= 3 {
			result.name = strings.TrimSpace(parts[2])
		}
		if len(parts) >= 4 {
			result.value = strings.TrimSpace(parts[3])
		}
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
		fields := strings.TrimSpace(line[at+len(marker):])
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
	if !traceMarkRawCandidate(line) {
		return nil
	}
	m := ftraceLineRE.FindStringSubmatch(line)
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
	fields := strings.TrimSpace(m[7])
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

func traceMarkPairingSourcePrefix(source string) string {
	return source + "\x00"
}

func traceMarkSyncPairingKey(source string, pid int) string {
	return traceMarkPairingSourcePrefix(source) + "sync\x00" + strconv.Itoa(pid)
}

func traceMarkAsyncPairingKey(source string, ev Event) string {
	key := traceAsyncSpanKey(ev)
	if key == "" {
		return ""
	}
	return traceMarkPairingSourcePrefix(source) + "async\x00" + key
}

// resetTraceMarkPairingState invalidates only the known physical source and
// emitter. A lifecycle/malformed row in one attachment must never erase an
// identically numbered emitter's open state in another attachment.
func resetTraceMarkPairingState(source string, pid int, stacks map[string][]Event, asyncStarts map[string][]Event) {
	if source == "" || pid < 0 {
		return
	}
	delete(stacks, traceMarkSyncPairingKey(source, pid))
	asyncPrefix := traceMarkPairingSourcePrefix(source) + "async\x00"
	for key, starts := range asyncStarts {
		if !strings.HasPrefix(key, asyncPrefix) {
			continue
		}
		kept := starts[:0]
		for _, start := range starts {
			if start.PID != pid {
				kept = append(kept, start)
			}
		}
		if len(kept) == 0 {
			delete(asyncStarts, key)
			continue
		}
		// Clear the removed tail slots before retaining the filtered stack: Event
		// carries interned strings and side-table pointers which should not stay
		// reachable through spare slice capacity after an emitter reset.
		clear(starts[len(kept):])
		asyncStarts[key] = kept
	}
}
