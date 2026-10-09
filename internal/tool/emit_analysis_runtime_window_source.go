package tool

import (
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/types"
)

// This is a literal validator, not an intent classifier. The caller has already
// validated explicit_time_window, finite typed bounds and a verbatim quote in
// the current request. Neither the full request nor capture/query prose enters
// this parser. An unresolved expression cannot acquire whole-capture authority.
const runtimeWindowSourceRepair = "use a short verbatim interval quote with its exact endpoints in seconds; for a nonliteral or unsupported selector use bounded_selector without coordinates, not unspecified/full_artifact"

const runtimeWindowNumber = `[+]?(?:[0-9]+(?:\.[0-9]+)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?`
const runtimeWindowUnit = `(?:milliseconds|millisecond|microseconds|microsecond|nanoseconds|nanosecond|seconds|second|minutes|minute|hours|hour|secs|sec|mins|min|毫秒|微秒|纳秒|分钟|小时|ms|µs|μs|us|ns|秒|s|m|h)`
const runtimeWindowEndpoint = runtimeWindowNumber + `(?:\s*` + runtimeWindowUnit + `(?:\s*` + runtimeWindowNumber + `\s*` + runtimeWindowUnit + `)*)?`

var runtimeWindowRangeRE = regexp.MustCompile(`(?i)(?:(between)\s+)?([\[(]?)\s*(` + runtimeWindowEndpoint + `)\s*(\.\.|[-–—~～到至]|to|and)\s*(` + runtimeWindowEndpoint + `)\s*([\])]?)(?:\s*(` + runtimeWindowUnit + `))?`)
var runtimeWindowBracketRE = regexp.MustCompile(`(?i)()([\[(])\s*(` + runtimeWindowEndpoint + `)\s*(,)\s*(` + runtimeWindowEndpoint + `)\s*([\])])(?:\s*(` + runtimeWindowUnit + `))?`)
var runtimeWindowTermRE = regexp.MustCompile(`(?i)(` + runtimeWindowNumber + `)\s*(` + runtimeWindowUnit + `)?`)

func bindRuntimeWindowSource(member types.RuntimeArtifactTimeWindow) (types.RuntimeArtifactTimeWindow, string, error) {
	quote := strings.TrimSpace(member.SourceQuote)
	if len(quote) > 4096 {
		return member, "", fmt.Errorf("interval quote exceeds 4096 bytes; %s", runtimeWindowSourceRepair)
	}
	type bounds struct {
		start, end   float64
		explicitUnit bool
	}
	var candidates []bounds
	matches := runtimeWindowRangeRE.FindAllStringSubmatchIndex(quote, -1)
	matches = append(matches, runtimeWindowBracketRE.FindAllStringSubmatchIndex(quote, -1)...)
	for _, loc := range matches {
		part := func(n int) string {
			if loc[2*n] < 0 {
				return ""
			}
			return quote[loc[2*n]:loc[2*n+1]]
		}
		if !runtimeWindowTokenBoundary(quote, loc[0], loc[1]) {
			continue
		}
		between, open, left, separator, right, close, tailUnit := part(1), part(2), part(3), part(4), part(5), part(6), part(7)
		if strings.EqualFold(separator, "and") && between == "" {
			continue
		}
		// The existing request-window contract is [start,end). Ordinary range
		// phrases use that convention; explicitly different topology is not
		// silently rewritten or rounded by epsilon.
		if open != "" || close != "" {
			if open != "[" || close != ")" {
				return member, "", fmt.Errorf("explicit interval topology is not [start,end); %s", runtimeWindowSourceRepair)
			}
		}
		l, lu, err := runtimeWindowExactEndpoint(left)
		if err != nil {
			return member, "", fmt.Errorf("invalid left endpoint: %w; %s", err, runtimeWindowSourceRepair)
		}
		r, ru, err := runtimeWindowExactEndpoint(right)
		if err != nil {
			return member, "", fmt.Errorf("invalid right endpoint: %w; %s", err, runtimeWindowSourceRepair)
		}
		if ru == "" && tailUnit == "" && runtimeWindowUnresolvedSuffix(quote[loc[1]:]) {
			return member, "", fmt.Errorf("bare endpoint has an unresolved unit or qualifier; %s", runtimeWindowSourceRepair)
		}
		// A shared unit applies only to a bare endpoint. Explicit units are
		// already accounted for, including compound timestamps, exactly once.
		if tailUnit != "" && (lu != "" || ru != "") {
			return member, "", fmt.Errorf("ambiguous shared endpoint unit; %s", runtimeWindowSourceRepair)
		}
		if lu == "compound" && ru == "" || ru == "compound" && lu == "" {
			return member, "", fmt.Errorf("a bare endpoint cannot inherit a compound unit; %s", runtimeWindowSourceRepair)
		}
		if lu == "" {
			u := tailUnit
			if u == "" {
				u = ru
			}
			l.Mul(l, runtimeWindowUnitFactor(u))
		}
		if ru == "" {
			u := tailUnit
			if u == "" {
				u = lu
			}
			r.Mul(r, runtimeWindowUnitFactor(u))
		}
		start, _ := l.Float64()
		end, _ := r.Float64()
		if l.Sign() < 0 || r.Cmp(l) <= 0 || math.IsInf(start, 0) || math.IsInf(end, 0) || l.Sign() > 0 && start == 0 || end <= start {
			return member, "", fmt.Errorf("literal endpoints need finite time_start>=0 and time_end>time_start; %s", runtimeWindowSourceRepair)
		}
		b := bounds{start, end, lu != "" || ru != "" || tailUnit != ""}
		duplicate := false
		for i, old := range candidates {
			if b.start == old.start && b.end == old.end {
				candidates[i].explicitUnit = old.explicitUnit || b.explicitUnit
				duplicate = true
			}
		}
		if !duplicate {
			candidates = append(candidates, b)
		}
	}
	for _, b := range candidates {
		if *member.TimeStart == b.start && *member.TimeEnd == b.end {
			return member, "", nil
		}
	}
	if len(candidates) != 1 {
		return member, "", fmt.Errorf("quote does not uniquely bind the declared endpoints; %s", runtimeWindowSourceRepair)
	}
	b := candidates[0]
	if !b.explicitUnit {
		return member, "", fmt.Errorf("bare interval differs from declared seconds; include the source unit in the quote before correcting coordinates; %s", runtimeWindowSourceRepair)
	}
	member.TimeStart, member.TimeEnd = &b.start, &b.end
	return member, "runtime_artifact_scope_profile normalized to the verbatim literal endpoints (seconds); model or capture extents are not requested bounds", nil
}

func runtimeWindowUnresolvedSuffix(suffix string) bool {
	suffix = strings.TrimSpace(suffix)
	if suffix == "" {
		return false
	}
	// These are literal-list separators, not request-intent cues. In particular,
	// an unknown unit such as "ticks"/"帧" is never silently treated as seconds.
	for _, separator := range []string{"and", "or"} {
		if len(suffix) > len(separator) && strings.EqualFold(suffix[:len(separator)], separator) {
			r, _ := utf8.DecodeRuneInString(suffix[len(separator):])
			if unicode.IsSpace(r) {
				return false
			}
		}
	}
	r, _ := utf8.DecodeRuneInString(suffix)
	return unicode.IsLetter(r)
}

func runtimeWindowTokenBoundary(quote string, start, end int) bool {
	matched := quote[start:end]
	trimmed := strings.TrimSpace(matched)
	start += strings.Index(matched, trimmed)
	end = start + len(trimmed)
	// Avoid accepting a numeric suffix of an identifier, signed value, malformed
	// decimal or unsupported unit. Chinese surrounding prose is not a token.
	if start > 0 {
		r, _ := utf8.DecodeLastRuneInString(quote[:start])
		if r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || strings.ContainsRune("_.+-:/\\", r) {
			return false
		}
	}
	if end < len(quote) {
		r, size := utf8.DecodeRuneInString(quote[end:])
		if r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || strings.ContainsRune("_:/\\", r) || r == 'µ' || r == 'μ' {
			return false
		}
		if r == '.' && end+size < len(quote) {
			next := quote[end+size]
			if next == '.' || next >= '0' && next <= '9' {
				return false
			}
		}
	}
	return true
}

// Compute decimal/unit arithmetic as a rational and round only once, at the
// float64 request contract boundary. Event-search matching tolerance never
// participates in source binding.
func runtimeWindowExactEndpoint(text string) (*big.Rat, string, error) {
	terms := runtimeWindowTermRE.FindAllStringSubmatchIndex(text, -1)
	sum := new(big.Rat)
	unit := ""
	pos := 0
	for _, term := range terms {
		if strings.TrimSpace(text[pos:term[0]]) != "" {
			return nil, "", fmt.Errorf("unrecognized timestamp syntax")
		}
		number := text[term[2]:term[3]]
		if len(number) > 128 {
			return nil, "", fmt.Errorf("numeric literal too long")
		}
		if i := strings.IndexAny(number, "eE"); i >= 0 {
			exponent, err := strconv.Atoi(number[i+1:])
			if err != nil || exponent < -400 || exponent > 400 {
				return nil, "", fmt.Errorf("numeric exponent out of range")
			}
		}
		value, ok := new(big.Rat).SetString(number)
		if !ok {
			return nil, "", fmt.Errorf("invalid decimal timestamp")
		}
		currentUnit := ""
		if term[4] >= 0 {
			currentUnit = text[term[4]:term[5]]
		}
		if len(terms) > 1 && currentUnit == "" {
			return nil, "", fmt.Errorf("compound timestamp requires units on every term")
		}
		value.Mul(value, runtimeWindowUnitFactor(currentUnit))
		sum.Add(sum, value)
		unit, pos = currentUnit, term[1]
	}
	if len(terms) == 0 || strings.TrimSpace(text[pos:]) != "" {
		return nil, "", fmt.Errorf("unrecognized timestamp syntax")
	}
	if len(terms) > 1 {
		unit = "compound"
	}
	return sum, unit, nil
}

func runtimeWindowUnitFactor(unit string) *big.Rat {
	switch strings.ToLower(unit) {
	case "ms", "millisecond", "milliseconds", "毫秒":
		return big.NewRat(1, 1000)
	case "us", "µs", "μs", "microsecond", "microseconds", "微秒":
		return big.NewRat(1, 1000000)
	case "ns", "nanosecond", "nanoseconds", "纳秒":
		return big.NewRat(1, 1000000000)
	case "m", "min", "mins", "minute", "minutes", "分钟":
		return big.NewRat(60, 1)
	case "h", "hour", "hours", "小时":
		return big.NewRat(3600, 1)
	default:
		return big.NewRat(1, 1)
	}
}
