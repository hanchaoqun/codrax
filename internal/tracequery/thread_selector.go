package tracequery

import (
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/threadidentity"
)

type threadSelector = threadidentity.Selector

func parseThreadSelector(raw string) threadSelector { return threadidentity.Parse(raw) }

// ParseThreadSelectorIdentity is shared with typed evidence consumers; search
// matching below intentionally remains distinct from exact identity authority.
func ParseThreadSelectorIdentity(raw string) (pid int, name string, ok bool) {
	return threadidentity.Identity(raw)
}

func cleanThreadSelectorName(raw string) string { return threadidentity.CleanName(raw) }

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
