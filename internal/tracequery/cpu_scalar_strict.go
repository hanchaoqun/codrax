package tracequery

import (
	"math"
	"strconv"
	"strings"
)

// cpuScalarProfile is the closed text-wire roster for CPU state/control
// events. The generalized cpu+freq compatibility names intentionally share
// the exact frequency/limits grammar after classification; noisy name
// matching may select a display family, but it cannot relax a hard scalar.
type cpuScalarProfile uint8

const (
	cpuScalarProfileNone cpuScalarProfile = iota
	cpuScalarProfileIdle
	cpuScalarProfileFrequency
	cpuScalarProfileLimits
	cpuScalarProfileClock
)

type cpuScalarIssue struct {
	Field  string
	Raw    string
	Reason string
}

// cpuScalarTypedFields is the one capture-local receipt for CPU scalar rows.
// Fixed-width values are proven before they enter Event; the private Parsed
// bit keeps production text authority distinct from legacy hand-built Events.
type cpuScalarTypedFields struct {
	Parsed      bool
	Profile     cpuScalarProfile
	ValueKnown  bool
	CPUPresent  bool
	CPUKnown    bool
	CPURequired bool
	State       uint32
	Frequency   uint32
	Min         uint32
	Max         uint32
	Rate        int64
	CPU         int
	ClockName   string
	Issues      []cpuScalarIssue
}

type cpuScalarWireToken struct {
	key      string
	rawValue string
	bare     string
}

func cpuScalarProfileForName(raw string) cpuScalarProfile {
	raw = strings.TrimSpace(raw)
	switch raw {
	case "cpu_idle":
		return cpuScalarProfileIdle
	case "cpu_frequency":
		return cpuScalarProfileFrequency
	case "cpu_frequency_limits":
		return cpuScalarProfileLimits
	case "clock_set_rate":
		return cpuScalarProfileClock
	}
	lower := strings.ToLower(raw)
	if strings.Contains(lower, "cpu") && strings.Contains(lower, "freq") {
		if strings.Contains(lower, "limit") {
			return cpuScalarProfileLimits
		}
		return cpuScalarProfileFrequency
	}
	return cpuScalarProfileNone
}

// parseCPUScalarTypedFields is the sole text argset decoder for the four CPU
// scalar profiles. It retains physical key occurrences and treats bare tokens
// as grammar only in the two exact clock profiles; generic parseKV and
// firstNonEmpty never participate in the verdict.
func parseCPUScalarTypedFields(rawType, fields string) (map[string]string, cpuScalarTypedFields) {
	typed := cpuScalarTypedFields{Parsed: true, Profile: cpuScalarProfileForName(rawType)}
	if typed.Profile == cpuScalarProfileNone {
		return nil, cpuScalarTypedFields{}
	}
	typed.CPURequired = typed.Profile != cpuScalarProfileClock
	tokens, ok := tokenizeCPUScalarWire(fields)
	if !ok {
		typed.addIssue("wire", fields, "invalid_quoted_or_token_boundary")
		return map[string]string{}, typed
	}

	switch typed.Profile {
	case cpuScalarProfileIdle:
		typed.parseRequiredU32Family(tokens, []string{"state"}, &typed.State)
		typed.parseCPU(tokens, true)
	case cpuScalarProfileFrequency:
		typed.parseRequiredU32Family(tokens, []string{"state", "frequency", "freq"}, &typed.Frequency)
		typed.parseCPU(tokens, true)
	case cpuScalarProfileLimits:
		typed.parseLimits(tokens)
		typed.parseCPU(tokens, true)
	case cpuScalarProfileClock:
		typed.parseClock(tokens)
	}
	return typed.canonicalMap(), typed
}

func tokenizeCPUScalarWire(fields string) ([]cpuScalarWireToken, bool) {
	rawTokens, ok := tokenizeSchedSwitchSuffix(fields)
	if !ok {
		return nil, false
	}
	out := make([]cpuScalarWireToken, 0, len(rawTokens))
	for _, raw := range rawTokens {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		key, value, found := strings.Cut(raw, "=")
		if !found {
			out = append(out, cpuScalarWireToken{bare: raw})
			continue
		}
		key = strings.TrimSpace(key)
		if !isTraceKVKey(key) {
			// A quoted pseudo-key is data, not a declaration. Retain it as a
			// bare token so a closed profile cannot silently accept extra prose.
			out = append(out, cpuScalarWireToken{bare: raw})
			continue
		}
		out = append(out, cpuScalarWireToken{key: key, rawValue: value})
	}
	return out, true
}

func (typed *cpuScalarTypedFields) parseRequiredU32Family(tokens []cpuScalarWireToken, aliases []string, target *uint32) {
	if typed == nil {
		return
	}
	if bare := firstCPUScalarBare(tokens); bare != "" {
		typed.addIssue("wire", bare, "unexpected_positional_token")
		return
	}
	raw, present, unique := cpuScalarUniqueAlias(tokens, aliases...)
	field := strings.Join(aliases, "|")
	if !present {
		typed.addIssue(field, "", "missing")
		return
	}
	if !unique {
		typed.addIssue(field, raw, "duplicate_or_alias_conflict")
		return
	}
	value, ok := parseCanonicalCPUU32(raw)
	if !ok {
		typed.addIssue(field, raw, "not_canonical_uint32_or_exact_dot_zero")
		return
	}
	*target = value
	typed.ValueKnown = true
}

func (typed *cpuScalarTypedFields) parseLimits(tokens []cpuScalarWireToken) {
	if bare := firstCPUScalarBare(tokens); bare != "" {
		typed.addIssue("wire", bare, "unexpected_positional_token")
		return
	}
	shortMin, shortMinPresent, shortMinUnique := cpuScalarUniqueAlias(tokens, "min")
	shortMax, shortMaxPresent, shortMaxUnique := cpuScalarUniqueAlias(tokens, "max")
	longMin, longMinPresent, longMinUnique := cpuScalarUniqueAlias(tokens, "min_freq")
	longMax, longMaxPresent, longMaxUnique := cpuScalarUniqueAlias(tokens, "max_freq")
	shortAny := shortMinPresent || shortMaxPresent
	longAny := longMinPresent || longMaxPresent
	if shortAny && longAny {
		typed.addIssue("min|max|min_freq|max_freq", "", "mixed_alias_profiles")
		return
	}
	var minRaw, maxRaw string
	var minPresent, maxPresent, minUnique, maxUnique bool
	if shortAny {
		minRaw, maxRaw = shortMin, shortMax
		minPresent, maxPresent = shortMinPresent, shortMaxPresent
		minUnique, maxUnique = shortMinUnique, shortMaxUnique
	} else {
		minRaw, maxRaw = longMin, longMax
		minPresent, maxPresent = longMinPresent, longMaxPresent
		minUnique, maxUnique = longMinUnique, longMaxUnique
	}
	if !minPresent || !maxPresent {
		typed.addIssue("min|max", "", "incomplete_limit_tuple")
		return
	}
	if !minUnique || !maxUnique {
		typed.addIssue("min|max", "", "duplicate_limit_member")
		return
	}
	minValue, minOK := parseCanonicalCPUU32(minRaw)
	maxValue, maxOK := parseCanonicalCPUU32(maxRaw)
	if !minOK || !maxOK {
		typed.addIssue("min|max", minRaw+"|"+maxRaw, "not_canonical_uint32_or_exact_dot_zero")
		return
	}
	if minValue > maxValue {
		typed.addIssue("min|max", minRaw+"|"+maxRaw, "min_above_max")
		return
	}
	typed.Min, typed.Max, typed.ValueKnown = minValue, maxValue, true
}

func (typed *cpuScalarTypedFields) parseClock(tokens []cpuScalarWireToken) {
	if typed == nil {
		return
	}
	// Exact positional profile: <name> <rate>, with no key declarations.
	if len(tokens) == 2 && tokens[0].key == "" && tokens[1].key == "" {
		name, nameOK := canonicalCPUClockName(tokens[0].bare)
		rate, rateOK := parseCanonicalCPUNonNegativeInt64(tokens[1].bare)
		if nameOK {
			typed.ClockName = name
		}
		if !nameOK {
			typed.addIssue("clock_name", tokens[0].bare, "invalid_single_token_name")
		}
		if !rateOK {
			typed.addIssue("rate", tokens[1].bare, "not_canonical_nonnegative_int64_or_exact_dot_zero")
		}
		if nameOK && rateOK {
			typed.Rate, typed.ValueKnown = rate, true
		}
		return
	}
	if len(tokens) == 0 || tokens[0].key != "" {
		typed.addIssue("clock_name", "", "missing_positional_name")
		return
	}
	name, nameOK := canonicalCPUClockName(tokens[0].bare)
	if !nameOK {
		typed.addIssue("clock_name", tokens[0].bare, "invalid_single_token_name")
	} else {
		typed.ClockName = name
	}
	for _, token := range tokens[1:] {
		if token.key == "" {
			typed.addIssue("wire", token.bare, "keyed_and_positional_profiles_mixed")
			return
		}
	}
	raw, present, unique := cpuScalarUniqueAlias(tokens[1:], "state", "frequency", "freq")
	if !present {
		typed.addIssue("state|frequency|freq", "", "missing")
	} else if !unique {
		typed.addIssue("state|frequency|freq", raw, "duplicate_or_alias_conflict")
	} else if rate, ok := parseCanonicalCPUNonNegativeInt64(raw); ok {
		typed.Rate = rate
		typed.ValueKnown = nameOK
	} else {
		typed.addIssue("state|frequency|freq", raw, "not_canonical_nonnegative_int64_or_exact_dot_zero")
	}
	typed.parseCPU(tokens[1:], false)
}

func (typed *cpuScalarTypedFields) parseCPU(tokens []cpuScalarWireToken, required bool) {
	raw, present, unique := cpuScalarUniqueAlias(tokens, "cpu_id")
	typed.CPUPresent = present
	if !present {
		if required {
			typed.addIssue("cpu_id", "", "missing")
		}
		return
	}
	if !unique {
		typed.addIssue("cpu_id", raw, "duplicate")
		return
	}
	cpu, ok := parseCanonicalTraceCPUIndex(raw)
	if !ok {
		typed.addIssue("cpu_id", raw, "not_canonical_cpu_0_4095")
		return
	}
	typed.CPU, typed.CPUKnown = cpu, true
}

func cpuScalarUniqueAlias(tokens []cpuScalarWireToken, aliases ...string) (raw string, present, unique bool) {
	unique = true
	for _, token := range tokens {
		if token.key == "" || !cpuScalarAliasAllowed(token.key, aliases) {
			continue
		}
		if present {
			return raw, true, false
		}
		raw, present = token.rawValue, true
	}
	return raw, present, unique
}

func cpuScalarAliasAllowed(key string, aliases []string) bool {
	for _, alias := range aliases {
		if key == alias {
			return true
		}
	}
	return false
}

func firstCPUScalarBare(tokens []cpuScalarWireToken) string {
	for _, token := range tokens {
		if token.key == "" {
			return token.bare
		}
	}
	return ""
}

func parseCanonicalCPUU32(raw string) (uint32, bool) {
	base, ok := canonicalCPUIntegralDecimal(raw, true)
	if !ok {
		return 0, false
	}
	value, err := strconv.ParseUint(base, 10, 32)
	if err != nil || value > math.MaxUint32 {
		return 0, false
	}
	return uint32(value), true
}

func parseCanonicalCPUNonNegativeInt64(raw string) (int64, bool) {
	base, ok := canonicalCPUIntegralDecimal(raw, true)
	if !ok {
		return 0, false
	}
	value, err := strconv.ParseInt(base, 10, 64)
	if err != nil || value < 0 {
		return 0, false
	}
	return value, true
}

func canonicalCPUIntegralDecimal(raw string, allowDotZero bool) (string, bool) {
	if raw == "" || raw != strings.TrimSpace(raw) || strings.ContainsAny(raw, "'\"+, \t\r\n") {
		return "", false
	}
	base := raw
	if strings.ContainsRune(raw, '.') {
		if !allowDotZero || strings.Count(raw, ".") != 1 || !strings.HasSuffix(raw, ".0") {
			return "", false
		}
		base = strings.TrimSuffix(raw, ".0")
	}
	if base == "" || !isAllDigits(base) || len(base) > 1 && base[0] == '0' {
		return "", false
	}
	return base, true
}

func parseCanonicalTraceCPUIndex(raw string) (int, bool) {
	base, ok := canonicalCPUIntegralDecimal(raw, false)
	if !ok {
		return 0, false
	}
	value, err := strconv.ParseUint(base, 10, 12)
	if err != nil || value > maxTraceCPUIndex {
		return 0, false
	}
	return int(value), true
}

func canonicalCPUClockName(raw string) (string, bool) {
	if raw == "" || raw != strings.TrimSpace(raw) || len(raw) > 128 || strings.ContainsAny(raw, "'\"=, \t\r\n") {
		return "", false
	}
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c < 0x21 || c > 0x7e {
			return "", false
		}
	}
	return raw, true
}

func (typed *cpuScalarTypedFields) addIssue(field, raw, reason string) {
	if typed == nil {
		return
	}
	typed.Issues = append(typed.Issues, cpuScalarIssue{Field: field, Raw: clampString(raw, 80), Reason: reason})
}

func (typed cpuScalarTypedFields) canonicalMap() map[string]string {
	out := map[string]string{}
	if typed.ValueKnown {
		switch typed.Profile {
		case cpuScalarProfileIdle:
			out["state"] = strconv.FormatUint(uint64(typed.State), 10)
		case cpuScalarProfileFrequency:
			out["state"] = strconv.FormatUint(uint64(typed.Frequency), 10)
		case cpuScalarProfileLimits:
			out["min"] = strconv.FormatUint(uint64(typed.Min), 10)
			out["max"] = strconv.FormatUint(uint64(typed.Max), 10)
		case cpuScalarProfileClock:
			out["state"] = strconv.FormatInt(typed.Rate, 10)
		}
	}
	if typed.CPUKnown {
		out["cpu_id"] = strconv.Itoa(typed.CPU)
	}
	return out
}

func (typed cpuScalarTypedFields) apply(ev *Event, intern *stringInterner) {
	if ev == nil || !typed.Parsed {
		return
	}
	ev.CPUForFieldPresent = typed.CPUPresent
	if typed.CPUKnown {
		ev.CPUForField = typed.CPU
		ev.CPUForFieldValid = true
	}
	if !typed.ValueKnown || typed.CPURequired && !typed.CPUKnown || typed.CPUPresent && !typed.CPUKnown {
		ev.CPUInputInvalid = true
	}
	switch typed.Profile {
	case cpuScalarProfileIdle:
		ev.State = int64(typed.State)
	case cpuScalarProfileFrequency:
		ev.Frequency = int64(typed.Frequency)
	case cpuScalarProfileLimits:
		ev.FrequencyMin, ev.FrequencyMax = int64(typed.Min), int64(typed.Max)
	case cpuScalarProfileClock:
		ev.Frequency = typed.Rate
		if intern != nil {
			ev.ClockName = intern.intern(typed.ClockName)
		} else {
			ev.ClockName = typed.ClockName
		}
	}
}

func eventCPUScalarKnown(ev Event) bool {
	switch ev.Type {
	case EventCPUIdle:
		return !ev.CPUInputInvalid && ev.State >= 0
	case EventCPUFrequency, EventClockSetRate:
		return !ev.CPUInputInvalid && ev.Frequency >= 0
	case EventCPUFrequencyLimit:
		return !ev.CPUInputInvalid && ev.FrequencyMin >= 0 && ev.FrequencyMax >= ev.FrequencyMin
	default:
		return false
	}
}
