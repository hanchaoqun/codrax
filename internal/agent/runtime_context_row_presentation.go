package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// One protected ledger render owns these references. They do not escape to a
// later dispatch, authenticate a source, sign publication or alter any ledger.
// The full public typed record AND its renderer-owned row must match; equal
// numbers, names, or model-authored text are not a source/query identity.
type runtimeContextRowPresentation struct {
	rows map[string]string
}

func runtimeContextRowKey(record types.ObservationRecord, body string) string {
	ref := record.SourceRef
	if record.ID == "" || record.Origin != types.AnswerEvidenceOriginRuntimeArtifact ||
		!types.RuntimeObservationProducerIsDeterministicQuery(record.Producer) || record.GroundingPolicy != types.ClaimGroundingHard ||
		ref.Path == "" || ref.QueryScopeID == "" || ref.PayloadRef == "" {
		return ""
	}
	if _, _, known := types.TraceObservationContinuousQueryWindow(ref); !known {
		return ""
	}
	key, err := json.Marshal(struct {
		Record types.ObservationRecord
		Body   string
	}{record, body})
	if err != nil {
		return ""
	}
	return string(key)
}

func (p *runtimeContextRowPresentation) write(b *strings.Builder, record types.ObservationRecord, rm *types.RequestModel, lang string, canonical bool) {
	body := answerDocBoundedRuntimeFactAuthorityRow(record, rm, lang)
	source := fmt.Sprintf("; source_path=%q; query_result=%q", record.SourceRef.Path, record.SourceRef.PayloadRef)
	key := runtimeContextRowKey(record, body)
	if p != nil && key != "" {
		if ref := p.rows[key]; ref != "" {
			alias := fmt.Sprintf("  - id=`%s`; context_row_ref=%q%s; reuse the complete earlier row, with this section's boundaries unchanged.\n", record.ID, ref, source)
			if len(alias) < len(body)+len(source)+len("  - \n") {
				b.WriteString(alias)
				return
			}
		} else if canonical {
			if p.rows == nil {
				p.rows = make(map[string]string)
			}
			ref := fmt.Sprintf("context-row-%d", len(p.rows)+1)
			// The definition is actually written before a later consumer can
			// reference it. Both live in the same protected ledger group.
			fmt.Fprintf(b, "  - %s%s; context_row_id=%q\n", body, source, ref)
			p.rows[key] = ref
			return
		}
	}
	if canonical {
		body += source
	}
	fmt.Fprintf(b, "  - %s\n", body)
}
