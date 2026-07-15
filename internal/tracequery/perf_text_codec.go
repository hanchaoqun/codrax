package tracequery

import (
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

type perfTextKVIssue struct {
	Code   string
	Field  string
	Raw    string
	Reason string
}

type perfTextTypedFields struct {
	Parsed                bool
	WireValid             bool
	PIDPresent            bool
	TIDPresent            bool
	CPUInvalid            bool
	ThreadIdentityInvalid bool
	WeightInvalid         bool
	SampleKindInvalid     bool
	Issues                []perfTextKVIssue
	CPUIssues             []perfTextKVIssue
}

type perfTextFamilyResult struct {
	Present bool
	Valid   bool
	Value   string
}

type perfTextValueValidator func(string) (string, bool, string)

type perfTextMetadataFamily struct {
	Canonical string
	Aliases   []string
	Preserve  map[string]bool
}

var perfTextMetadataFamilies = [...]perfTextMetadataFamily{
	{Canonical: "thread_comm", Aliases: []string{"thread_comm", "comm", "name"}},
	{Canonical: "event", Aliases: []string{"event", "type"}},
	{Canonical: "symbol", Aliases: []string{"symbol", "func", "function"}},
	{Canonical: "dso", Aliases: []string{"dso", "file", "path"}},
	{Canonical: "ip", Aliases: []string{"ip", "addr", "address"}, Preserve: map[string]bool{"addr": true}},
	{Canonical: "callchain", Aliases: []string{"callchain", "call_stack", "stack"}},
	{Canonical: "symbolization_status", Aliases: []string{"symbolization_status", "symbol_status", "symbols"}},
	{Canonical: "clock", Aliases: []string{"clock", "clockid"}},
	{Canonical: "clock_confidence", Aliases: []string{"clock_confidence", "time_alignment", "time_alignment_confidence"}},
	{Canonical: "callchain_status", Aliases: []string{"callchain_status", "stack_status", "call_stack_status"}},
}

var perfTextMetadataAliasToFamily = func() map[string]int {
	out := make(map[string]int, 32)
	for index, family := range perfTextMetadataFamilies {
		for _, alias := range family.Aliases {
			out[alias] = index
		}
	}
	return out
}()

var perfTextHardAliasFamilies = [][]string{
	{"pid", "process_pid", "tgid"},
	{"tid", "thread_pid"},
	{"cpu"},
	{"cpu_known", "cpu_valid", "cpu_available"},
	{"sample_weight", "period_weight", "period", "sample_period", "event_count", "count"},
	{"sample_kind", "sample_type", "perf_sample_kind"},
	{"thread_identity_known"},
	{"lifecycle_unverified"},
	{"source", "producer"},
	{"resolution"},
	{"perf_source_pid"},
	{"perf_source_tid"},
}

// parsePerfTextKV is the sole perf_sample body authority. It deliberately
// consumes the shared tracewire lexer rather than generic parseKV: colon/space
// fallback and context-free regex recovery are valid compatibility tools for
// noisy vendor events, but cannot mint perf thread/CPU/weight identities.
func parsePerfTextKV(fields string) (map[string]string, perfTextTypedFields) {
	typed := perfTextTypedFields{Parsed: true}
	tokens, wireErr := tracewire.ParsePerfKV(fields)
	if wireErr != nil {
		typed.addIssueCode("wire_"+firstNonEmpty(wireErr.Reason, "invalid"), firstNonEmpty(wireErr.Field, "wire"), "", wireErr.Reason, false)
		// A lexical failure makes every later boundary unknowable. Keep a
		// dedicated CPU-family witness even when the malformed literal was
		// metadata, because the row's apparent cpu= token is withdrawn too.
		typed.addIssueCode("cpu_wire_boundary_unproven", "cpu", "", "wire_boundary_unproven", true)
		typed.CPUInvalid = true
		typed.ThreadIdentityInvalid = true
		typed.WeightInvalid = true
		typed.SampleKindInvalid = true
		return nil, typed
	}
	typed.WireValid = true

	byKey := make(map[string][]tracewire.PerfKVField, len(tokens))
	kv := make(map[string]string, len(tokens))
	for _, token := range tokens {
		byKey[token.Key] = append(byKey[token.Key], token)
		if _, exists := kv[token.Key]; !exists {
			// Metadata is first-physical-occurrence until the duplicate audit
			// below withdraws an ambiguous key. Hard families are normalized
			// separately from their complete occurrence inventory.
			kv[token.Key] = token.Value
		}
	}

	pid := normalizePerfTextFamily(kv, byKey, &typed, "pid",
		[]string{"pid", "process_pid", "tgid"}, true, validateCanonicalPerfPID, false)
	tid := normalizePerfTextFamily(kv, byKey, &typed, "tid",
		[]string{"tid", "thread_pid"}, true, validateCanonicalPerfPID, false)
	typed.PIDPresent, typed.TIDPresent = pid.Present, tid.Present
	if !pid.Valid || !tid.Valid {
		typed.ThreadIdentityInvalid = true
	}

	cpuKnown := normalizePerfTextFamily(kv, byKey, &typed, "cpu_known",
		[]string{"cpu_known", "cpu_valid", "cpu_available"}, false, validateCanonicalPerfBool, true)
	cpu := normalizePerfTextFamily(kv, byKey, &typed, "cpu",
		[]string{"cpu"}, true, validateCanonicalPerfCPU, true)
	if !cpu.Valid || cpuKnown.Present && !cpuKnown.Valid {
		typed.CPUInvalid = true
	}
	if cpu.Valid && cpu.Value == "-1" && !cpuKnown.Present {
		delete(kv, "cpu")
		typed.addIssueRaw("cpu", "-1", "minus_one_without_false_cpu_known", true)
		typed.CPUInvalid = true
	} else if cpu.Valid && cpu.Value == "-1" && cpuKnown.Valid && cpuKnown.Value != "false" {
		delete(kv, "cpu")
		typed.addIssueRaw("cpu", "-1", "minus_one_with_positive_cpu_known", true)
		typed.CPUInvalid = true
	}

	weight := normalizePerfTextFamily(kv, byKey, &typed, "sample_weight",
		[]string{"sample_weight", "period_weight", "period", "sample_period", "event_count", "count"},
		true, validateCanonicalPositivePerfWeight, false)
	if !weight.Valid {
		typed.WeightInvalid = true
	}

	sampleKind := normalizePerfTextFamily(kv, byKey, &typed, "sample_kind",
		[]string{"sample_kind", "sample_type", "perf_sample_kind"}, false, validateCanonicalPerfSampleKind, false)
	if sampleKind.Present && !sampleKind.Valid {
		typed.SampleKindInvalid = true
	}

	threadKnown := normalizePerfTextFamily(kv, byKey, &typed, "thread_identity_known",
		[]string{"thread_identity_known"}, false, validateCanonicalPerfBool, false)
	lifecycle := normalizePerfTextFamily(kv, byKey, &typed, "lifecycle_unverified",
		[]string{"lifecycle_unverified"}, false, validateCanonicalPerfBool, false)
	source := normalizePerfTextFamily(kv, byKey, &typed, "source",
		[]string{"source", "producer"}, false, validateCanonicalPerfToken, false)
	resolution := normalizePerfTextFamily(kv, byKey, &typed, "resolution",
		[]string{"resolution"}, false, validateCanonicalPerfResolution, false)
	if threadKnown.Present && !threadKnown.Valid ||
		lifecycle.Present && !lifecycle.Valid ||
		source.Present && !source.Valid ||
		resolution.Present && !resolution.Valid {
		typed.ThreadIdentityInvalid = true
	}
	if threadKnown.Valid && threadKnown.Value == "true" &&
		(pid.Valid && pid.Value == "0" || tid.Valid && tid.Value == "0") {
		typed.addIssue("thread_identity", "positive_claim_with_zero_pid_or_tid", false)
		typed.ThreadIdentityInvalid = true
	}
	if resolution.Valid && resolution.Value == "resolved" &&
		(pid.Valid && pid.Value == "0" || tid.Valid && tid.Value == "0") {
		typed.addIssue("resolution", "resolved_with_zero_pid_or_tid", false)
		typed.ThreadIdentityInvalid = true
	}
	sourcePID := normalizePerfTextFamily(kv, byKey, &typed, "perf_source_pid",
		[]string{"perf_source_pid"}, false, validateCanonicalPerfPID, false)
	sourceTID := normalizePerfTextFamily(kv, byKey, &typed, "perf_source_tid",
		[]string{"perf_source_tid"}, false, validateCanonicalPerfPID, false)
	if sourcePID.Present && !sourcePID.Valid || sourceTID.Present && !sourceTID.Valid {
		typed.ThreadIdentityInvalid = true
	}

	hardKeys := make(map[string]bool, 24)
	for _, family := range perfTextHardAliasFamilies {
		for _, key := range family {
			hardKeys[key] = true
		}
	}
	var metadataFirst [len(perfTextMetadataFamilies)]tracewire.PerfKVField
	var metadataPresent [len(perfTextMetadataFamilies)]bool
	for _, token := range tokens {
		if family, ok := perfTextMetadataAliasToFamily[token.Key]; ok && !metadataPresent[family] {
			metadataFirst[family] = token
			metadataPresent[family] = true
		}
	}
	for index, family := range perfTextMetadataFamilies {
		normalizePerfTextMetadataFamily(kv, byKey, &typed, family, metadataFirst[index], metadataPresent[index])
	}
	for key, values := range byKey {
		_, handledMetadata := perfTextMetadataAliasToFamily[key]
		if len(values) <= 1 || hardKeys[key] || handledMetadata {
			continue
		}
		// Optional/noisy metadata never becomes a row-wide hard gate. An
		// ambiguous metadata key is simply omitted and disclosed.
		delete(kv, key)
		reason := duplicatePerfTextReason(values, "metadata_duplicate")
		typed.addIssueCode(reason, key, perfTextOccurrenceRaw(values), reason, false)
	}
	return kv, typed
}

func normalizePerfTextMetadataFamily(kv map[string]string, byKey map[string][]tracewire.PerfKVField,
	typed *perfTextTypedFields, family perfTextMetadataFamily, first tracewire.PerfKVField, present bool,
) {
	duplicate := false
	duplicateConflict := false
	for _, alias := range family.Aliases {
		values := byKey[alias]
		if len(values) > 1 {
			duplicate = true
			if duplicatePerfTextReason(values, "metadata_duplicate") == "metadata_duplicate_conflict" {
				duplicateConflict = true
			}
		}
	}
	if duplicate {
		for _, alias := range family.Aliases {
			delete(kv, alias)
		}
		reason := "metadata_duplicate_identical"
		if duplicateConflict {
			reason = "metadata_duplicate_conflict"
		}
		typed.addIssueCode(family.Canonical+"_"+reason, family.Canonical, "", reason, false)
		return
	}
	for _, alias := range family.Aliases {
		if alias != family.Canonical && !family.Preserve[alias] {
			delete(kv, alias)
		}
	}
	if present {
		kv[family.Canonical] = first.Value
	}
}

func normalizePerfTextFamily(kv map[string]string, byKey map[string][]tracewire.PerfKVField,
	typed *perfTextTypedFields, canonical string, aliases []string, required bool,
	validate perfTextValueValidator, cpuIssue bool,
) perfTextFamilyResult {
	var values []tracewire.PerfKVField
	for _, alias := range aliases {
		values = append(values, byKey[alias]...)
		delete(kv, alias)
	}
	if len(values) == 0 {
		if required {
			typed.addIssueRaw(canonical, "", "missing", cpuIssue)
		}
		return perfTextFamilyResult{}
	}
	result := perfTextFamilyResult{Present: true}
	if len(values) != 1 {
		typed.addIssueRaw(canonical, perfTextOccurrenceRaw(values), duplicatePerfTextReason(values, "duplicate"), cpuIssue)
		return result
	}
	value := values[0]
	if value.Quoted {
		typed.addIssueRaw(canonical, value.Raw, "quoted_scalar", cpuIssue)
		return result
	}
	normalized, ok, reason := validate(value.Value)
	if !ok {
		typed.addIssueRaw(canonical, value.Raw, firstNonEmpty(reason, "invalid"), cpuIssue)
		return result
	}
	kv[canonical] = normalized
	result.Valid, result.Value = true, normalized
	return result
}

func duplicatePerfTextReason(values []tracewire.PerfKVField, prefix string) string {
	if len(values) <= 1 {
		return prefix
	}
	first := values[0].Value
	for _, value := range values[1:] {
		if value.Value != first {
			return prefix + "_conflict"
		}
	}
	return prefix + "_identical"
}

func perfTextOccurrenceRaw(values []tracewire.PerfKVField) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, 0, minInt(len(values), 2))
	for i, value := range values {
		if i >= 2 {
			break
		}
		parts = append(parts, value.Raw)
	}
	return strings.Join(parts, "|")
}

func (t *perfTextTypedFields) addIssue(field, reason string, cpu bool) {
	t.addIssueRaw(field, "", reason, cpu)
}

func (t *perfTextTypedFields) addIssueRaw(field, raw, reason string, cpu bool) {
	t.addIssueCode(firstNonEmpty(field, "wire")+"_"+firstNonEmpty(reason, "invalid"), field, raw, reason, cpu)
}

func (t *perfTextTypedFields) addIssueCode(code, field, raw, reason string, cpu bool) {
	if t == nil {
		return
	}
	issue := perfTextKVIssue{Code: firstNonEmpty(code, "wire_invalid"), Field: firstNonEmpty(field, "wire"), Raw: raw, Reason: firstNonEmpty(reason, "invalid")}
	t.Issues = append(t.Issues, issue)
	if cpu {
		t.CPUIssues = append(t.CPUIssues, issue)
	}
}

func (t perfTextTypedFields) integritySummary() string {
	if len(t.Issues) == 0 {
		return ""
	}
	seen := make(map[string]bool, len(t.Issues))
	out := make([]string, 0, len(t.Issues))
	for _, issue := range t.Issues {
		code := firstNonEmpty(issue.Code, issue.Field+"_"+issue.Reason)
		if seen[code] {
			continue
		}
		seen[code] = true
		out = append(out, code)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func validateCanonicalPerfPID(raw string) (string, bool, string) {
	value, ok := parseCanonicalSchedPID(raw)
	if !ok {
		return "", false, "not_canonical_pid"
	}
	return strconv.Itoa(value), true, ""
}

func validateCanonicalPerfCPU(raw string) (string, bool, string) {
	if raw == "-1" {
		return raw, true, ""
	}
	value, present, valid, reason := parseTraceCPUScalar(raw)
	if !present || !valid {
		return "", false, firstNonEmpty(reason, "invalid_cpu")
	}
	if strconv.Itoa(value) != raw {
		return "", false, "not_canonical_cpu"
	}
	return raw, true, ""
}

func validateCanonicalPositivePerfWeight(raw string) (string, bool, string) {
	if raw == "" || !isAllDigits(raw) {
		return "", false, "not_unsigned_decimal"
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return "", false, "integer_overflow"
	}
	if value <= 0 {
		return "", false, "not_positive"
	}
	if strconv.FormatInt(value, 10) != raw {
		return "", false, "not_canonical_weight"
	}
	return raw, true, ""
}

func validateCanonicalPerfBool(raw string) (string, bool, string) {
	if raw != "true" && raw != "false" {
		return "", false, "not_canonical_bool"
	}
	return raw, true, ""
}

func validateCanonicalPerfSampleKind(raw string) (string, bool, string) {
	switch raw {
	case "on_cpu", "off_cpu", "unknown":
		return raw, true, ""
	default:
		return "", false, "not_closed_enum"
	}
}

func validateCanonicalPerfToken(raw string) (string, bool, string) {
	if raw == "" {
		return "", false, "empty"
	}
	if len(raw) > 128 {
		return "", false, "token_too_long"
	}
	for i := 0; i < len(raw); i++ {
		b := raw[i]
		if i == 0 {
			if !isPerfTextASCIILetterOrDigit(b) {
				return "", false, "not_canonical_token"
			}
			continue
		}
		if !isPerfTextASCIILetterOrDigit(b) && b != '_' && b != '.' && b != ':' && b != '-' {
			return "", false, "not_canonical_token"
		}
	}
	return raw, true, ""
}

func validateCanonicalPerfResolution(raw string) (string, bool, string) {
	switch raw {
	case "resolved", perfSourceOnlyResolution:
		return raw, true, ""
	default:
		return "", false, "not_closed_enum"
	}
}

func isPerfTextASCIILetterOrDigit(b byte) bool {
	return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9'
}
