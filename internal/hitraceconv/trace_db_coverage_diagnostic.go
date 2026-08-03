package hitraceconv

import (
	"sort"
	"strings"
)

// TraceDBCoverageDiagnosticWitnessKeys returns metadata keys whose values are
// bounded diagnostic witness payloads rather than witness accounting scalars.
// Producers and the diagnostic-report adapter share this naming contract so a
// new witness family cannot silently fall out of the 8 KiB sideband merely
// because a second package forgot to extend a hand-written allowlist.
func TraceDBCoverageDiagnosticWitnessKeys(metadata map[string]string) []string {
	if len(metadata) == 0 {
		return nil
	}
	out := make([]string, 0)
	for key := range metadata {
		if traceDBCoverageDiagnosticWitnessKey(key) {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}

func traceDBCoverageDiagnosticWitnessKey(key string) bool {
	if key == "" {
		return false
	}
	for _, ch := range key {
		if (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') && ch != '_' {
			return false
		}
	}
	if strings.HasSuffix(key, "_witness") || strings.HasSuffix(key, "_witnesses") {
		return true
	}
	const marker = "_witnesses_"
	index := strings.Index(key, marker)
	if index < 0 {
		return false
	}
	suffix := key[index+len(marker):]
	if suffix == "" || suffix == "emitted" || suffix == "omitted" || suffix == "cap" ||
		strings.HasSuffix(suffix, "_emitted") || strings.HasSuffix(suffix, "_omitted") ||
		strings.HasSuffix(suffix, "_cap") {
		return false
	}
	return true
}
