// Package threadidentity owns the selector grammar shared by trace queries and
// typed evidence consumers. It must never be used to mine free-form prose.
package threadidentity

import (
	"regexp"
	"strconv"
	"strings"
)

type Selector struct {
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

func Parse(raw string) Selector {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "`'\"“”")
	sel := Selector{Raw: raw, Name: cleanThreadSelectorName(raw)}
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

// Identity resolves a complete typed selector. Repeated display IDs must agree;
// they are stripped from the canonical name. Query name searching remains a
// separate, permissive operation and cannot confer identity authority.
func Identity(raw string) (pid int, name string, ok bool) {
	sel := Parse(raw)
	if !sel.HasPID || sel.PID > MaxPID {
		return 0, "", false
	}
	pid, name = sel.PID, sel.Name
	// Check every explicit carrier before cleaning display punctuation; cleaning
	// a trailing bracket can otherwise hide a contradictory earlier ID.
	remaining := sel.Raw
	for _, re := range []*regexp.Regexp{threadSelectorPIDTokenRE, threadSelectorBracketRE} {
		for _, match := range re.FindAllStringSubmatch(remaining, -1) {
			value, valid := parsePositiveInt(match[1])
			if !valid || value != pid {
				return 0, "", false
			}
		}
		remaining = re.ReplaceAllString(remaining, " ")
	}
	name = cleanThreadSelectorName(remaining)
	for name != "" {
		nested := Parse(name)
		if !nested.HasPID {
			break
		}
		if nested.PID != pid || nested.Name == name {
			return 0, "", false
		}
		name = nested.Name
	}
	return pid, name, true
}

// MaxPID is Linux PID_MAX_LIMIT. Trace TIDs use the same domain.
const MaxPID = 4194304

// CleanName normalizes the display wrapper, not identity or fuzzy equivalence.
func CleanName(raw string) string { return cleanThreadSelectorName(raw) }
