package agent

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Replace only exact mirrors of fields that this composition already wrote.
// This is not a suppression rule for source, causal, log/MCP or unknown facts.
// A tree preview with omitted references cannot sign full tree publication.
func (p *answerDocNativePresentationReceipt) samplingMirrorReferences(ledger types.ObservationLedger) map[string]string {
	out := map[string]string{}
	if p == nil {
		return out
	}
	for _, r := range ledger.Records {
		switch r.Predicate {
		case tool.TracePerfSamplingTreePredicate, "perf_sample_top_symbol":
		default:
			// A sampling measurement table supplies its typed fields/notes,
			// not necessarily its observation's independent Summary.
			continue
		}
		if r.RawExcerpt != "" {
			// These presenters do not supply independent raw excerpts.
			continue
		}
		if p.nativeSources[traceEventPublicationRecordKey(r)] == "" || !p.records[traceEventPublicationRecordKey(r)] {
			continue
		}
		out[r.ID] = fmt.Sprintf("- native_record_reuse observation_id=%q source=%q query_scope=%q payload=%q: native fact fields and summary already supplied in the source-bound detail above; original record and query remain separate in the audit ledger.\n", r.ID, r.SourceRef.Path, r.SourceRef.QueryScopeID, r.SourceRef.PayloadRef)
	}
	return out
}
