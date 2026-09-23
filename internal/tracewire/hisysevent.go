package tracewire

import "strings"

// IsHiSysEventPrintName checks one complete name in the converter's existing
// DOMAIN/ENAME print protocol. This is wire syntax, not source authority.
func IsHiSysEventPrintName(name string) bool {
	if len(name) < 2 || name[0] < 'A' || name[0] > 'Z' {
		return false
	}
	for i := 1; i < len(name); i++ {
		c := name[i]
		if (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '_' {
			return false
		}
	}
	return true
}

// ParseHiSysEventPrintHead preserves the protocol
// [A-Z][A-Z0-9_]+/[A-Z][A-Z0-9_]+:( |$). The caller must separately enforce
// the converter origin; ordinary print prose must not acquire that authority.
func ParseHiSysEventPrintHead(fields string) (domain, event string, ok bool) {
	head, tail, found := strings.Cut(fields, ":")
	if !found || (tail != "" && tail[0] != ' ') {
		return "", "", false
	}
	domain, event, found = strings.Cut(head, "/")
	if !found || !IsHiSysEventPrintName(domain) || !IsHiSysEventPrintName(event) {
		return "", "", false
	}
	return domain, event, true
}
