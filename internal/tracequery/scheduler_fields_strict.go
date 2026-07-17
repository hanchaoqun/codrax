package tracequery

import (
	"sort"
	"strconv"
	"strings"
)

// schedulerCPUFieldIssue is the occurrence-aware CPU verdict emitted by the
// strict scheduler-field parser. It deliberately contains no line/header
// metadata: lineScan owns that physical-row context and cpu_input_integrity
// adds it when publishing the bounded witness.
type schedulerCPUFieldIssue struct {
	Field  string
	Raw    string
	Reason string
}

// blockedReasonFieldIssue is the field-local verdict for one
// sched_blocked_reason declaration.  Unlike HardFields, these issues must not
// poison the scheduler state machine: a malformed optional delay or iowait
// flag withdraws only that blocked-reason projection while the independently
// proven sched_switch/wakeup ledger remains usable.
type blockedReasonFieldIssue struct {
	Field  string
	Reason string
}

// schedulerTypedFields is a transient, per-line authority. It never enters the
// public Event schema: the strict values in KV build the Event, while the same
// hard/field verdicts are consumed by scheduler and CPU integrity. Keeping all
// three consumers on this one object prevents a normalized map from erasing
// duplicate/conflicting declarations before a hard decision is made.
type schedulerTypedFields struct {
	Active               bool
	HardFields           []string
	HardPID              int
	HardPIDs             []int
	HardAffectsAllPIDs   bool
	WakePriorityUnknown  bool
	SuppressWakeEdge     bool
	CPUIssues            []schedulerCPUFieldIssue
	BlockedReason        bool
	BlockedPIDKnown      bool
	BlockedIOWaitKnown   bool
	BlockedDelayKnown    bool
	BlockedPIDCandidates []int
	// BlockedPIDCandidatesTruncated is set only when one physical row names
	// more distinct canonical PID identities than the bounded candidate set can
	// retain. It is deliberately separate from malformed/noncanonical PID
	// declarations: a token that cannot denote a canonical PID must not acquire
	// authority over every thread, while a genuinely truncated exact set must
	// fail closed for targets whose membership can no longer be decided.
	BlockedPIDCandidatesTruncated bool
	BlockedIssues                 []blockedReasonFieldIssue
	// PriorityMutationPID is non-zero only when an exact mutation row has
	// one unique canonical subject. Zero intentionally means artifact-global
	// priority-range poison; old/new values are not interpreted here.
	PriorityMutationPID int
}

const schedulerPIDCandidateScopeCap = 32

// forEachSchedulerKeyDeclaration is the shared occurrence lexer for governed
// scheduler suffix fields. quoteAware is deliberately false for PID: the
// producer wire does not quote comm, so allowing comm bytes to open a quote
// could hide one PID and elect another. Once one exact PID has been elected,
// quoted values in the suffix may safely make their contents opaque.
func forEachSchedulerKeyDeclaration(fields, key string, start int, quoteAware bool, visit func(position, equalsPosition int) bool) {
	var quote byte
	if start < 0 {
		start = 0
	}
	for position := start; position < len(fields); {
		current := fields[position]
		if quoteAware && quote != 0 {
			if current == '\\' && position+1 < len(fields) {
				position += 2
				continue
			}
			if current == quote {
				quote = 0
			}
			position++
			continue
		}
		if quoteAware && (current == '\'' || current == '"') {
			previous := position - 1
			for previous >= start && isASCIIHorizontalSpace(fields[previous]) {
				previous--
			}
			if previous >= start && fields[previous] == '=' {
				quote = current
				position++
				continue
			}
		}
		if position+len(key) <= len(fields) && fields[position:position+len(key)] == key &&
			(position == 0 || isASCIIHorizontalSpace(fields[position-1])) {
			afterKey := position + len(key)
			equalsPosition := afterKey
			for equalsPosition < len(fields) && isASCIIHorizontalSpace(fields[equalsPosition]) {
				equalsPosition++
			}
			if equalsPosition < len(fields) && fields[equalsPosition] == '=' {
				if !visit(position, equalsPosition) {
					return
				}
			}
		}
		position++
	}
}

func (s *schedulerTypedFields) addHard(field, reason string) {
	if s == nil {
		return
	}
	add := func(value string) {
		for _, existing := range s.HardFields {
			if existing == value {
				return
			}
		}
		s.HardFields = append(s.HardFields, value)
	}
	add(field)
	if reason != "" {
		add(field + "_" + reason)
	}
}

func (s *schedulerTypedFields) addBlockedIssue(field, reason string) {
	if s == nil {
		return
	}
	for _, issue := range s.BlockedIssues {
		if issue.Field == field && issue.Reason == reason {
			return
		}
	}
	s.BlockedIssues = append(s.BlockedIssues, blockedReasonFieldIssue{Field: field, Reason: reason})
}

type schedulerFieldOccurrence struct {
	Position  int
	End       int
	Count     int
	Raw       string
	Canonical bool
}

type schedulerSuffixDeclaration struct {
	Key   string
	Value string
}

// parseSchedulerSuffixDeclarations recognizes only structured key/value
// tokens. It also recognizes the malformed-but-occurrence-bearing `key =v`
// and `key = v` spellings so a bad soft dimension can be degraded locally
// without allowing an arbitrary bare word between producer core fields.
func parseSchedulerSuffixDeclarations(suffix string) ([]schedulerSuffixDeclaration, bool) {
	if !schedulerSuffixQuotesWellFormed(suffix) {
		return nil, false
	}
	tokens, ok := tokenizeSchedSwitchSuffix(suffix)
	if !ok {
		return nil, false
	}
	declarations := make([]schedulerSuffixDeclaration, 0, len(tokens))
	for index := 0; index < len(tokens); index++ {
		raw := strings.TrimSpace(tokens[index])
		if key, value, found := strings.Cut(raw, "="); found && isTraceKVKey(key) {
			if value == "" && index+1 < len(tokens) {
				next := strings.TrimSpace(tokens[index+1])
				if !schedulerTokenDeclaresKey(next) && next != "=" {
					index++
					value = next
				}
			}
			declarations = append(declarations, schedulerSuffixDeclaration{Key: key, Value: value})
			continue
		}
		if !isTraceKVKey(raw) || index+1 >= len(tokens) {
			return nil, false
		}
		equals := strings.TrimSpace(tokens[index+1])
		if !strings.HasPrefix(equals, "=") {
			return nil, false
		}
		index++
		value := strings.TrimPrefix(equals, "=")
		if value == "" && index+1 < len(tokens) {
			next := strings.TrimSpace(tokens[index+1])
			if !schedulerTokenDeclaresKey(next) && next != "=" {
				index++
				value = next
			}
		}
		declarations = append(declarations, schedulerSuffixDeclaration{Key: raw, Value: value})
	}
	return declarations, true
}

func schedulerTokenDeclaresKey(token string) bool {
	key, _, found := strings.Cut(token, "=")
	return found && isTraceKVKey(key)
}

// schedulerSuffixQuotesWellFormed keeps the local grammar and occurrence
// lexer on the same quote profile. A quote may open only at the beginning of a
// structured value (after '=' and optional horizontal space); quote bytes in
// an unquoted value are not allowed to swallow later producer declarations.
func schedulerSuffixQuotesWellFormed(suffix string) bool {
	var quote byte
	for position := 0; position < len(suffix); position++ {
		current := suffix[position]
		if quote != 0 {
			if current == '\\' {
				position++
				if position >= len(suffix) {
					return false
				}
				continue
			}
			if current == quote {
				quote = 0
			}
			continue
		}
		if current != '\'' && current != '"' {
			continue
		}
		previous := position - 1
		for previous >= 0 && isASCIIHorizontalSpace(suffix[previous]) {
			previous--
		}
		if previous < 0 || suffix[previous] != '=' {
			return false
		}
		quote = current
	}
	return quote == 0
}

func schedulerSuffixGrammarValid(fields string, pid schedulerFieldOccurrence, migrate bool) bool {
	if pid.Count != 1 || !pid.Canonical || pid.Position < 0 {
		return true
	}
	declarations, ok := parseSchedulerSuffixDeclarations(fields[pid.Position:])
	if !ok || len(declarations) == 0 || declarations[0].Key != "pid" {
		return false
	}
	opaqueTail := false
	for index, declaration := range declarations {
		if index == 0 {
			continue
		}
		// Migrate prio is not consumed by the CPU migration tuple and may not
		// gate or reopen hard authority.
		if migrate && declaration.Key == "prio" {
			continue
		}
		governed := declaration.Key == "pid"
		if migrate {
			governed = governed || declaration.Key == "orig_cpu" || declaration.Key == "dest_cpu"
		} else {
			governed = governed || declaration.Key == "prio" || declaration.Key == "success" ||
				declaration.Key == "target_cpu" || declaration.Key == "codrax_prio_source"
		}
		if !governed {
			opaqueTail = true
			continue
		}
		if declaration.Key == "pid" || opaqueTail {
			return false
		}
	}
	return true
}

func scanSchedulerFieldOccurrence(fields, key string, start int, quoteAware bool) schedulerFieldOccurrence {
	position, count := -1, 0
	forEachSchedulerKeyDeclaration(fields, key, start, quoteAware, func(found, _ int) bool {
		if count == 0 {
			position = found
		}
		count++
		return count < 2
	})
	occurrence := schedulerFieldOccurrence{Position: position, End: position, Count: count}
	if count != 1 || position < 0 {
		return occurrence
	}
	raw, end, ok := consumeSchedToken(fields, position, key+"=")
	occurrence.Raw, occurrence.End, occurrence.Canonical = raw, end, ok
	return occurrence
}

func schedulerOccurrenceFailure(occurrence schedulerFieldOccurrence) string {
	switch {
	case occurrence.Count == 0:
		return "missing"
	case occurrence.Count > 1:
		return "duplicate"
	case !occurrence.Canonical:
		return "noncanonical"
	default:
		return ""
	}
}

// schedulerExactPositivePIDCandidates narrows a duplicate-PID poison only
// when every physical declaration is itself an exact positive identity. The
// bounded union prevents one ambiguous row from disabling unrelated TIDs;
// any malformed candidate or an oversized set falls back to global poison.
func schedulerExactPositivePIDCandidates(fields string) ([]int, bool) {
	seen := make(map[int]struct{})
	valid := true
	forEachSchedulerKeyDeclaration(fields, "pid", 0, false, func(position, _ int) bool {
		raw, _, canonical := consumeSchedToken(fields, position, "pid=")
		pid, parsed := parseCanonicalPositiveSchedPID(raw)
		if !canonical || !parsed {
			valid = false
			return false
		}
		seen[pid] = struct{}{}
		if len(seen) > schedulerPIDCandidateScopeCap {
			valid = false
			return false
		}
		return true
	})
	if !valid || len(seen) == 0 {
		return nil, false
	}
	result := make([]int, 0, len(seen))
	for pid := range seen {
		result = append(result, pid)
	}
	sort.Ints(result)
	return result, true
}

// parseSchedulerTypedFields is the sole field authority for the unquoted
// wakeup/migration producer profiles. A key-shaped token inside comm cannot be
// distinguished from a second producer declaration on this wire; every
// ASCII-boundary declaration therefore participates in occurrence counting.
// Ambiguity fails closed instead of electing the leftmost/rightmost token.
func parseSchedulerTypedFields(rawType, fields string) (map[string]string, schedulerTypedFields) {
	fields = trimASCIIHorizontalSpace(fields)
	switch rawType {
	case "sched_wakeup", "sched_wakeup_new", "sched_waking":
		return parseSchedWakeFields(fields)
	case "sched_migrate_task":
		return parseSchedMigrateFields(fields)
	case "sched_blocked_reason":
		return parseSchedBlockedReasonFields(fields)
	case "sched_pi_setprio":
		return parsePriorityMutationFields(fields)
	case "binder_set_priority":
		// No production-backed Harmony binder subject profile is frozen yet.
		// Preserve the exact event as a global range poison.
		return map[string]string{}, schedulerTypedFields{Active: true}
	default:
		return nil, schedulerTypedFields{}
	}
}

// parsePriorityMutationFields binds only the mutation subject. The Linux
// tracepoint also carries oldprio/newprio, but those values are poison-only
// until the target platform's numeric domain is proven by a production wire.
func parsePriorityMutationFields(fields string) (map[string]string, schedulerTypedFields) {
	result := schedulerTypedFields{Active: true}
	out := make(map[string]string, 1)
	pidField := scanSchedulerFieldOccurrence(fields, "pid", 0, false)
	if schedulerOccurrenceFailure(pidField) != "" {
		return out, result
	}
	pid, ok := parseCanonicalPositiveSchedPID(pidField.Raw)
	if !ok {
		return out, result
	}
	result.PriorityMutationPID = pid
	out["pid"] = strconv.Itoa(pid)
	return out, result
}

// parseSchedBlockedReasonFields is the single occurrence-aware authority for
// the direct-text and converter-owned blocked-reason profiles.  Every governed
// declaration is counted before a normalized map can erase duplicates.  The
// dimensions deliberately degrade independently: only PID is required to bind
// a typed marker to a thread; bad iowait/delay/caller metadata remains visible
// through the integrity ledger without deleting valid siblings.
func parseSchedBlockedReasonFields(fields string) (map[string]string, schedulerTypedFields) {
	result := schedulerTypedFields{Active: true, BlockedReason: true}
	out := make(map[string]string, 6)

	pidField := scanSchedulerFieldOccurrence(fields, "pid", 0, false)
	pidFailure := schedulerOccurrenceFailure(pidField)
	if pidFailure == "" && pidField.Position != 0 {
		pidFailure = "misordered"
	}
	if pidFailure == "" {
		if pid, ok := parseCanonicalSchedPID(pidField.Raw); ok {
			result.BlockedPIDKnown = true
			out["pid"] = strconv.Itoa(pid)
			// PID 0 is a canonical physical marker identity, not an unknown
			// subject. Retain its exact scope so a malformed soft field does not
			// poison every positive-TID refinement.
			result.BlockedPIDCandidates = []int{pid}
		} else {
			pidFailure = "invalid"
		}
	}
	if pidFailure != "" {
		result.addBlockedIssue("pid", pidFailure)
		result.BlockedPIDCandidates, _, result.BlockedPIDCandidatesTruncated = schedulerExactNonNegativePIDCandidates(fields)
	}

	iowaitField := scanSchedulerFieldOccurrence(fields, "iowait", 0, false)
	iowaitFailure := schedulerOccurrenceFailure(iowaitField)
	if iowaitFailure == "" && result.BlockedPIDKnown && iowaitField.Position < pidField.End {
		iowaitFailure = "misordered"
	}
	if iowaitFailure == "" {
		if iowaitField.Raw == "0" || iowaitField.Raw == "1" {
			result.BlockedIOWaitKnown = true
			out["iowait"] = iowaitField.Raw
		} else {
			iowaitFailure = "invalid"
		}
	}
	if iowaitFailure != "" {
		result.addBlockedIssue("iowait", iowaitFailure)
	}

	callerField := scanSchedulerFieldOccurrence(fields, "caller", 0, false)
	callerFailure := schedulerOccurrenceFailure(callerField)
	if callerFailure == "" && iowaitField.Count == 1 && iowaitField.Canonical && callerField.Position < iowaitField.End {
		callerFailure = "misordered"
	}
	if callerFailure == "" {
		out["caller"] = callerField.Raw
	} else {
		result.addBlockedIssue("caller", callerFailure)
		out["caller"] = "unknown"
	}

	qualityField := scanSchedulerFieldOccurrence(fields, "caller_quality", 0, false)
	if qualityField.Count != 0 {
		qualityFailure := schedulerOccurrenceFailure(qualityField)
		if qualityFailure == "" {
			switch qualityField.Raw {
			case "opaque", "symbolized":
				out["caller_quality"] = qualityField.Raw
			default:
				qualityFailure = "invalid"
			}
		}
		if qualityFailure != "" {
			result.addBlockedIssue("caller_quality", qualityFailure)
			// An untrusted quality declaration cannot grant a caller symbol.
			out["caller"] = "unknown"
		}
	}

	delayField := scanSchedulerFieldOccurrence(fields, "delay", 0, false)
	if delayField.Count != 0 {
		delayFailure := schedulerOccurrenceFailure(delayField)
		if delayFailure == "" {
			if delay, ok := parseCanonicalPositiveInt32(delayField.Raw); ok {
				result.BlockedDelayKnown = true
				out["delay"] = strconv.Itoa(delay)
			} else {
				delayFailure = "invalid"
			}
		}
		if delayFailure != "" {
			result.addBlockedIssue("delay", delayFailure)
		}
	}

	return out, result
}

func schedulerExactNonNegativePIDCandidates(fields string) ([]int, bool, bool) {
	seen := make(map[int]struct{})
	complete := true
	truncated := false
	forEachSchedulerKeyDeclaration(fields, "pid", 0, false, func(position, _ int) bool {
		raw, _, canonical := consumeSchedToken(fields, position, "pid=")
		pid, parsed := parseCanonicalSchedPID(raw)
		if !canonical || !parsed {
			complete = false
			// Keep scanning: a mixed declaration such as `pid=42 pid=bad`
			// still proves that 42 is one physical contender. Dropping that exact
			// subset would let a neighboring valid marker masquerade as unique.
			return true
		}
		if _, ok := seen[pid]; ok {
			return true
		}
		if len(seen) >= schedulerPIDCandidateScopeCap {
			complete = false
			truncated = true
			return true
		}
		seen[pid] = struct{}{}
		return true
	})
	if len(seen) == 0 {
		return nil, complete, truncated
	}
	result := make([]int, 0, len(seen))
	for pid := range seen {
		result = append(result, pid)
	}
	sort.Ints(result)
	return result, complete, truncated
}

func parseCanonicalPositiveInt32(raw string) (int, bool) {
	if raw == "" || !isAllDigits(raw) {
		return 0, false
	}
	value, err := strconv.ParseUint(raw, 10, 31)
	if err != nil || value == 0 || raw != strconv.FormatUint(value, 10) {
		return 0, false
	}
	return int(value), true
}

func parseSchedWakeFields(fields string) (map[string]string, schedulerTypedFields) {
	result := schedulerTypedFields{Active: true}
	out := make(map[string]string, 5)

	pidField := scanSchedulerFieldOccurrence(fields, "pid", 0, false)
	if failure := schedulerOccurrenceFailure(pidField); failure != "" {
		result.addHard("pid", failure)
		if failure == "duplicate" {
			result.HardPIDs, _ = schedulerExactPositivePIDCandidates(fields)
		}
		result.HardAffectsAllPIDs = len(result.HardPIDs) == 0
		return out, result
	} else if parsed, ok := parseCanonicalPositiveSchedPID(pidField.Raw); !ok {
		result.addHard("pid", "invalid")
		result.HardAffectsAllPIDs = true
		return out, result
	} else {
		result.HardPID = parsed
		out["pid"] = strconv.Itoa(parsed)
		if !schedulerSuffixGrammarValid(fields, pidField, false) {
			result.addHard("scheduler_core", "noncanonical")
			return out, result
		}
	}
	if strings.HasPrefix(fields, "comm=") && pidField.Position >= len("comm=") {
		out["comm"] = cleanTraceValue(trimASCIIHorizontalSpace(fields[len("comm="):pidField.Position]))
	}

	prioField := scanSchedulerFieldOccurrence(fields, "prio", pidField.End, true)
	successField := scanSchedulerFieldOccurrence(fields, "success", pidField.End, true)
	targetField := scanSchedulerFieldOccurrence(fields, "target_cpu", pidField.End, true)
	sourceField := scanSchedulerFieldOccurrence(fields, "codrax_prio_source", pidField.End, true)
	pidStructurallyPlaced := pidField.Count == 1 && pidField.Canonical
	prioOrdered := !pidStructurallyPlaced || prioField.Position >= pidField.End
	if targetField.Count == 1 && targetField.Canonical && targetField.Position >= pidField.End {
		prioOrdered = prioOrdered && prioField.Position < targetField.Position
	}
	if prioField.Count == 1 && prioField.Canonical && prioOrdered {
		if priority, ok := parseCanonicalPositiveSchedPriority(prioField.Raw); ok {
			out["prio"] = strconv.Itoa(priority)
		} else {
			result.WakePriorityUnknown = true
		}
	} else {
		// Missing priority is a supported field-level degradation (used by
		// converter rows that carry codrax_prio_source=unknown). Every other
		// non-exact shape, including same-value duplicates, degrades identically.
		result.WakePriorityUnknown = true
	}

	if successField.Count != 0 {
		successOrdered := !pidStructurallyPlaced || successField.Position >= pidField.End
		switch {
		case successField.Count != 1:
			result.addHard("success", "duplicate")
		case !successField.Canonical:
			result.addHard("success", "noncanonical")
		case !successOrdered:
			result.addHard("success", "misordered")
		default:
			success, valid := parseCanonicalSchedPriority(successField.Raw)
			switch {
			case !valid || (success != 0 && success != 1):
				result.addHard("success", "invalid")
			case success == 0:
				// Old Linux/Android text profiles emit success=0 for a no-op
				// wake attempt against an already-active task. Keep the physical
				// observation, but it is not a causal wake edge and wakeup_new
				// must not reset a numeric TID generation.
				result.SuppressWakeEdge = true
			}
		}
	}

	targetOrdered := !pidStructurallyPlaced || targetField.Position >= pidField.End
	if targetField.Count == 1 && targetField.Canonical && targetOrdered {
		if cpu, ok, reason := parseCanonicalSchedulerCPU(targetField.Raw); ok {
			out["target_cpu"] = strconv.Itoa(cpu)
		} else {
			result.CPUIssues = append(result.CPUIssues, schedulerCPUFieldIssue{
				Field: "target_cpu", Raw: targetField.Raw, Reason: reason,
			})
		}
	} else {
		reason := schedulerOccurrenceFailure(targetField)
		if reason == "" {
			reason = "misordered"
		}
		result.CPUIssues = append(result.CPUIssues, schedulerCPUFieldIssue{
			Field: "target_cpu", Raw: targetField.Raw, Reason: reason,
		})
	}

	sourceOrdered := !pidStructurallyPlaced || sourceField.Position >= pidField.End
	if prioField.Count == 1 && prioField.Canonical && prioField.Position >= pidField.End {
		sourceOrdered = sourceOrdered && sourceField.Position >= prioField.End
	}
	switch {
	case sourceField.Count == 0:
	case sourceField.Count == 1 && sourceField.Canonical && sourceOrdered:
		out["codrax_prio_source"] = sourceField.Raw
	default:
		// The Event provenance switch treats every unknown token as untrusted.
		// Use a stable internal marker rather than electing one occurrence.
		out["codrax_prio_source"] = "scheduler_source_untrusted"
	}

	return out, result
}

func parseSchedMigrateFields(fields string) (map[string]string, schedulerTypedFields) {
	result := schedulerTypedFields{Active: true}
	out := make(map[string]string, 5)

	pidField := scanSchedulerFieldOccurrence(fields, "pid", 0, false)
	if failure := schedulerOccurrenceFailure(pidField); failure != "" {
		result.addHard("pid", failure)
		if failure == "duplicate" {
			result.HardPIDs, _ = schedulerExactPositivePIDCandidates(fields)
		}
		result.HardAffectsAllPIDs = len(result.HardPIDs) == 0
		return out, result
	} else if parsed, ok := parseCanonicalPositiveSchedPID(pidField.Raw); !ok {
		result.addHard("pid", "invalid")
		result.HardAffectsAllPIDs = true
		return out, result
	} else {
		result.HardPID = parsed
		out["pid"] = strconv.Itoa(parsed)
		if !schedulerSuffixGrammarValid(fields, pidField, true) {
			result.addHard("scheduler_core", "noncanonical")
			return out, result
		}
	}
	if strings.HasPrefix(fields, "comm=") && pidField.Position >= len("comm=") {
		out["comm"] = cleanTraceValue(trimASCIIHorizontalSpace(fields[len("comm="):pidField.Position]))
	}

	prioField := scanSchedulerFieldOccurrence(fields, "prio", pidField.End, true)
	origField := scanSchedulerFieldOccurrence(fields, "orig_cpu", pidField.End, true)
	destField := scanSchedulerFieldOccurrence(fields, "dest_cpu", pidField.End, true)
	if prioField.Count == 1 && prioField.Canonical &&
		(pidField.Count != 1 || !pidField.Canonical || prioField.Position >= pidField.End) {
		if priority, ok := parseCanonicalSchedPriority(prioField.Raw); ok {
			out["prio"] = strconv.Itoa(priority)
		}
	}

	origStructurallyPlaced := pidField.Count != 1 || !pidField.Canonical || origField.Position >= pidField.End
	parseStrictMigrateCPU(&result, out, "orig_cpu", origField, origStructurallyPlaced)
	destStructurallyPlaced := pidField.Count != 1 || !pidField.Canonical || destField.Position >= pidField.End
	if origField.Count == 1 && origField.Canonical && origField.Position >= pidField.End {
		destStructurallyPlaced = destStructurallyPlaced && destField.Position >= origField.End
	}
	parseStrictMigrateCPU(&result, out, "dest_cpu", destField,
		destStructurallyPlaced)

	return out, result
}

func parseStrictMigrateCPU(result *schedulerTypedFields, out map[string]string, field string, occurrence schedulerFieldOccurrence, ordered bool) bool {
	if failure := schedulerOccurrenceFailure(occurrence); failure != "" {
		result.addHard(field, failure)
		result.CPUIssues = append(result.CPUIssues, schedulerCPUFieldIssue{Field: field, Raw: occurrence.Raw, Reason: failure})
		return false
	}
	if !ordered {
		result.addHard(field, "misordered")
		result.CPUIssues = append(result.CPUIssues, schedulerCPUFieldIssue{Field: field, Raw: occurrence.Raw, Reason: "misordered"})
		return false
	}
	cpu, ok, reason := parseCanonicalSchedulerCPU(occurrence.Raw)
	if !ok {
		result.addHard(field, "invalid")
		result.CPUIssues = append(result.CPUIssues, schedulerCPUFieldIssue{Field: field, Raw: occurrence.Raw, Reason: reason})
		return false
	}
	out[field] = strconv.Itoa(cpu)
	return true
}

func parseCanonicalPositiveSchedPID(raw string) (int, bool) {
	value, ok := parseCanonicalSchedPID(raw)
	return value, ok && value > 0
}

func parseCanonicalPositiveSchedPriority(raw string) (int, bool) {
	value, ok := parseCanonicalSchedPriority(raw)
	return value, ok && value > 0
}

func parseCanonicalSchedulerCPU(raw string) (int, bool, string) {
	if raw == "" || !isAllDigits(raw) {
		return 0, false, "not_unsigned_decimal"
	}
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return 0, false, "integer_overflow"
	}
	if value > maxTraceCPUIndex {
		return 0, false, "cpu_above_limit"
	}
	return int(value), true, ""
}
