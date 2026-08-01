package tracequery

import (
	"regexp"
	"strconv"
	"strings"
)

type threadSelector struct {
	Raw       string
	Name      string
	PID       int
	HasPID    bool
	HyphenPID bool
}

var (
	threadSelectorPIDTokenRE = regexp.MustCompile(`(?i)\b(?:pid|tid|thread_id|threadid)\s*[:=#]?\s*([0-9]{1,10})\b`)
	threadSelectorBracketRE  = regexp.MustCompile(`[\[\(]\s*([0-9]{1,10})\s*[\]\)]`)
	threadSelectorSpaceRE    = regexp.MustCompile(`^(.*?)[\s,;]+([0-9]{1,10})$`)
)

func parseThreadSelector(raw string) threadSelector {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "`'\"“”")
	sel := threadSelector{Raw: raw, Name: cleanThreadSelectorName(raw)}
	if raw == "" {
		return sel
	}
	if pid, ok := parsePositiveInt(raw); ok {
		sel.Name = ""
		sel.PID = pid
		sel.HasPID = true
		return sel
	}
	if match := lastRegexpMatch(threadSelectorPIDTokenRE, raw); match != nil {
		if pid, ok := parsePositiveInt(raw[match[2]:match[3]]); ok {
			sel.PID = pid
			sel.HasPID = true
			sel.Name = cleanThreadSelectorName(strings.TrimSpace(raw[:match[0]] + " " + raw[match[1]:]))
			return sel
		}
	}
	if match := lastRegexpMatch(threadSelectorBracketRE, raw); match != nil {
		if pid, ok := parsePositiveInt(raw[match[2]:match[3]]); ok {
			sel.PID = pid
			sel.HasPID = true
			sel.Name = cleanThreadSelectorName(strings.TrimSpace(raw[:match[0]] + " " + raw[match[1]:]))
			return sel
		}
	}
	if match := threadSelectorSpaceRE.FindStringSubmatch(raw); len(match) == 3 {
		if pid, ok := parsePositiveInt(match[2]); ok && strings.TrimSpace(match[1]) != "" {
			sel.PID = pid
			sel.HasPID = true
			sel.Name = cleanThreadSelectorName(match[1])
			return sel
		}
	}
	if hyphen := strings.LastIndex(raw, "-"); hyphen > 0 && hyphen < len(raw)-1 {
		suffix := raw[hyphen+1:]
		if pid, ok := parsePositiveInt(suffix); ok {
			sel.PID = pid
			sel.HasPID = true
			sel.HyphenPID = true
			sel.Name = cleanThreadSelectorName(raw[:hyphen])
			return sel
		}
	}
	return sel
}

// ParseThreadSelectorIdentity exposes the canonical identity parsed by the
// trace engine's single selector grammar. Callers that record/query typed
// target identity must use this helper rather than re-parsing the accepted
// numeric, pid=/tid=, bracket, space, or name-pid spellings independently.
//
// A selector without a positive pid is not a complete identity and returns
// ok=false. Name is the canonical bare name and may be empty for pid-only
// selectors.
func ParseThreadSelectorIdentity(raw string) (pid int, name string, ok bool) {
	sel := parseThreadSelector(raw)
	if !sel.HasPID || sel.PID <= 0 {
		return 0, "", false
	}
	return sel.PID, sel.Name, true
}

func lastRegexpMatch(re *regexp.Regexp, s string) []int {
	matches := re.FindAllStringSubmatchIndex(s, -1)
	if len(matches) == 0 {
		return nil
	}
	return matches[len(matches)-1]
}

func parsePositiveInt(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

func cleanThreadSelectorName(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "`'\"“”")
	for _, prefix := range []string{"thread=", "thread:", "comm=", "comm:", "name=", "name:"} {
		if strings.HasPrefix(strings.ToLower(s), prefix) {
			s = strings.TrimSpace(s[len(prefix):])
			break
		}
	}
	s = strings.Trim(s, " \t\r\n-,:;[]()")
	return strings.Join(strings.Fields(s), " ")
}

func normalizedThreadText(s string) string {
	return strings.ToLower(cleanThreadSelectorName(s))
}

func threadSelectorMatchesName(sel threadSelector, candidate string) bool {
	needle := normalizedThreadText(sel.Name)
	if needle == "" {
		needle = normalizedThreadText(sel.Raw)
	}
	if needle == "" {
		return true
	}
	haystack := normalizedThreadText(candidate)
	return haystack != "" && strings.Contains(haystack, needle)
}

func threadSelectorSummary(raw string) string {
	sel := parseThreadSelector(raw)
	var parts []string
	if strings.TrimSpace(sel.Raw) != "" {
		parts = append(parts, "raw="+sel.Raw)
	}
	if sel.Name != "" {
		parts = append(parts, "name="+sel.Name)
	}
	if sel.HasPID {
		parts = append(parts, "pid="+strconv.Itoa(sel.PID))
	}
	return strings.Join(parts, " ")
}
