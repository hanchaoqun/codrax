package hitraceconv

import (
	"bytes"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

const traceRoutingTextProbeBytes = 16 << 10

// A physical first data row is navigation, not complete text admission or
// semantic evidence. The parser owns the origin grammar; neither event names,
// container metadata, filenames nor user prose can grant this capability.
func traceRoutingTextPrefix(prefix []byte) bool {
	for _, line := range bytes.Split(prefix, []byte{'\n'}) {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 || trimmed[0] == '#' {
			continue
		}
		_, ok := tracequery.ProbeLeadingExactEventNamePrefix(string(line))
		return ok
	}
	return false
}
