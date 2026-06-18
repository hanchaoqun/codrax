package hitraceconv

import "strings"

func perfTraceSymbolizationStatus(symbol, dso, source string) string {
	source = strings.ToLower(strings.TrimSpace(source))
	switch {
	case strings.TrimSpace(symbol) != "" && !perfTraceLabelLooksIP(symbol) && strings.TrimSpace(symbol) != "unknown":
		return "symbolized"
	case strings.Contains(source, "raw_perfdata"):
		return "unsymbolized"
	case strings.TrimSpace(dso) != "" && strings.TrimSpace(dso) != "unknown":
		return "partial"
	default:
		return "unknown"
	}
}

func perfTraceCallchainStatus(callchain, source string) string {
	callchain = strings.TrimSpace(callchain)
	source = strings.ToLower(strings.TrimSpace(source))
	switch {
	case callchain == "" || callchain == "unknown":
		return "missing"
	case perfTraceCallchainLooksIPOnly(callchain):
		return "ip_only"
	case strings.Contains(source, "raw_perfdata"):
		return "symbolized"
	default:
		return "symbolized"
	}
}

func perfTraceCallchainLooksIPOnly(callchain string) bool {
	parts := strings.FieldsFunc(callchain, func(r rune) bool {
		return r == ';' || r == ',' || r == '>' || r == '|'
	})
	seen := false
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		seen = true
		if !perfTraceLabelLooksIP(part) {
			return false
		}
	}
	return seen
}

func perfTraceLabelLooksIP(raw string) bool {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "[")
	raw = strings.TrimSuffix(raw, "]")
	raw = strings.TrimPrefix(strings.ToLower(raw), "0x")
	if raw == "" {
		return false
	}
	for _, r := range raw {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}
