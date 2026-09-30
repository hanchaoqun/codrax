package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/threadidentity"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TraceQueryAggregateSelectionSubject recovers the selector's resolved subject
// from a validated aggregate producer, not from a query hint on arbitrary rows.
// Selection scope is different from factual ownership: selecting a thread's
// process can return its peers, but cannot make those peers causal ancestors.
// New aggregate families must supply a decoder checking their own typed payload,
// source, window, role and binding; query metadata alone never grants retention.
func TraceQueryAggregateSelectionSubject(record types.ObservationRecord) (string, bool) {
	profile, ok := DecodeTraceProcessProfile(record)
	if !ok || profile.SourceThread.PID <= 0 || profile.SourceThread.PID > types.RuntimeTargetMaxPID {
		return "", false
	}
	ref := record.SourceRef
	if ref.QueryTargetScope != "thread" || ref.QueryLineStart != 0 || ref.QueryLineEnd != 0 {
		return "", false
	}
	pid, name := ref.QueryTargetPID, strings.TrimSpace(ref.QueryTargetThread)
	if parsed := threadidentity.Parse(name); parsed.HasPID {
		parsedPID, parsedName, valid := threadidentity.Identity(name)
		if !valid || (pid > 0 && pid != parsedPID) {
			return "", false
		}
		pid, name = parsedPID, parsedName
	}
	if pid > 0 {
		if pid != profile.SourceThread.PID {
			return "", false
		}
	} else if name == "" || name != profile.SourceThread.Comm {
		return "", false // fuzzy discovery is not aggregate target authority
	}
	return fmt.Sprintf("%s-%d", profile.SourceThread.Comm, profile.SourceThread.PID), true
}
