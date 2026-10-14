package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/threadidentity"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TraceQueryAggregateSelectionSubject recovers the selector's resolved subject
// from a validated aggregate producer, not from a query hint on arbitrary rows.
// Selection scope is different from factual ownership: selecting a thread's
// process can return its peers, but cannot make those peers causal ancestors.
// New aggregate families must supply a decoder checking their own typed payload,
// source, window, role and binding; query metadata alone never grants retention.
func TraceQueryAggregateSelectionSubject(record types.ObservationRecord) (string, bool) {
	if subject, ok, _ := TraceQueryFilteredPerfMemberSelection(record); ok {
		return subject, true
	}
	if profile, ok := DecodeTraceProcessProfile(record); ok {
		return traceQueryAggregateSelectedThread(record.SourceRef, profile.SourceThread.PID, profile.SourceThread.Comm)
	}
	tree, ok := DecodeTracePerfSamplingTree(record)
	if !ok || tree.Status != "available" || tree.SampleCount == 0 || tree.OmittedCohorts != 0 || len(tree.Cohorts) == 0 {
		return "", false
	}
	// A sampling query may contain independent event/unit/producer cohorts,
	// but that does not prove they describe one selected thread. Establish the
	// same physical capture, source generation and thread incarnation across
	// every cohort before offering a subject to the display-only matcher.
	// Missing or omitted identities cannot borrow the first cohort's name.
	first := tree.Cohorts[0]
	if first.SourcePath != tree.SourcePath && !traceQuerySamplingSourceBindingMatches(record, tree) {
		return "", false
	}
	identity := first.ThreadIdentity
	if identity == nil || identity.Generation <= 0 || identity.TGID < 0 || identity.TGID > types.RuntimeTargetMaxPID {
		return "", false
	}
	subject, selected := traceQuerySamplingSelectedThread(record.SourceRef, identity.TID, identity.DisplayComm)
	if !selected {
		return "", false
	}
	for _, cohort := range tree.Cohorts {
		current := cohort.ThreadIdentity
		if cohort.SourcePath != first.SourcePath || cohort.SourceGeneration != first.SourceGeneration || current == nil ||
			current.TID != identity.TID || current.Generation != identity.Generation || current.TGID != identity.TGID || current.DisplayComm != identity.DisplayComm {
			return "", false
		}
		if selectedSubject, ok := traceQuerySamplingSelectedThread(record.SourceRef, current.TID, current.DisplayComm); !ok || selectedSubject != subject {
			return "", false
		}
	}
	return subject, true
}

func traceQuerySamplingSelectedThread(ref types.ObservationSourceRef, tid int, comm string) (string, bool) {
	pid, name, valid := tracequery.NormalizePerfSamplingSelector(ref.QueryTargetPID, ref.QueryTargetThread)
	if !valid || pid != ref.QueryTargetPID || name != ref.QueryTargetThread {
		return "", false // a native sampling receipt must already be canonical
	}
	if pid > 0 {
		// Its validated numeric selection is authoritative. The canonical comm
		// may end in a number and must not be parsed again as a second TID.
		ref.QueryTargetThread = ""
	}
	return traceQueryAggregateSelectedThread(ref, tid, comm)
}

// The caller must first decode a source/window/role-bound native aggregate.
// This shared comparison does not qualify arbitrary SourceRef query hints.
func traceQueryAggregateSelectedThread(ref types.ObservationSourceRef, tid int, comm string) (string, bool) {
	if tid <= 0 || tid > types.RuntimeTargetMaxPID || ref.QueryTargetPID < 0 || ref.QueryTargetPID > types.RuntimeTargetMaxPID {
		return "", false
	}
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
		if pid != tid {
			return "", false
		}
	} else if name == "" || name != comm {
		return "", false // fuzzy discovery is not aggregate target authority
	}
	return fmt.Sprintf("%s-%d", comm, tid), true
}
