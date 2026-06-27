package orchestrator

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

type cgecViolationLedgerSummary struct {
	Total          int
	Strict         int
	Advisory       int
	LegacyOnly     int
	StrictFields   map[string]int
	AdvisoryFields map[string]int
}

func summarizeCGECViolationLedger(closure *types.EvidenceClosure, stats types.ClosureStats) cgecViolationLedgerSummary {
	var out cgecViolationLedgerSummary
	if closure == nil {
		return out
	}
	violations := closure.Violations()
	out.Total = len(violations)
	for _, v := range violations {
		if isSoftViolationKind(v.Kind) {
			out.Advisory++
			if field := v.SuspectedRoot.IRField; field != "" {
				if out.AdvisoryFields == nil {
					out.AdvisoryFields = make(map[string]int)
				}
				out.AdvisoryFields[field]++
			}
			continue
		}
		out.Strict++
		if field := v.SuspectedRoot.IRField; field != "" {
			if out.StrictFields == nil {
				out.StrictFields = make(map[string]int)
			}
			out.StrictFields[field]++
		}
	}
	if stats.ViolationsLogged > out.Total {
		out.LegacyOnly = stats.ViolationsLogged - out.Total
	}
	return out
}

func formatCGECViolationLedgerSummary(closure *types.EvidenceClosure, stats types.ClosureStats) string {
	s := summarizeCGECViolationLedger(closure, stats)
	if s.Total == 0 && s.LegacyOnly == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, " ledger_events=%d strict_findings=%d advisory_events=%d",
		s.Total+s.LegacyOnly, s.Strict, s.Advisory)
	if len(s.StrictFields) > 0 {
		fmt.Fprintf(&b, " strict_by_field=%s", formatViolationFieldTally(s.StrictFields))
	}
	if len(s.AdvisoryFields) > 0 {
		fmt.Fprintf(&b, " advisory_by_field=%s", formatViolationFieldTally(s.AdvisoryFields))
	}
	if s.LegacyOnly > 0 {
		fmt.Fprintf(&b, " legacy_unclassified_events=%d", s.LegacyOnly)
	}
	return b.String()
}
