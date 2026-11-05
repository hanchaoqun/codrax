package agent

import (
	"encoding/json"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Exact retained record content is only a soft display preference. Keep the
// immutable payload or independently revalidated physical generation, clock,
// target, window, line scope, full
// node accounts, census, caveats and supporting coordinates. Only publication
// IDs, observation time and call/text-output locators differ between copies.
// Nothing here grants provenance, completeness, causal or publication rights;
// the original record is still selected and rendered unchanged, or explicitly
// counted as omitted under the existing 16-record budget.
func answerDocBusinessAccountNoveltyKey(a answerDocBusinessAccount) string {
	if a.key == "" || a.record.SourceRef.PayloadRef == "" {
		return ""
	}
	r := a.record
	r.ID, r.ObservedAt = "", ""
	r.SourceRef.QueryScopeID, r.SourceRef.ToolCallID, r.SourceRef.RawRef = "", "", ""
	if a.source != "" {
		// Index/cache scan metadata may change the enclosing payload while
		// this entire measured record remains identical. Only a current
		// private source receipt may separate that locator from the body.
		r.SourceRef.PayloadRef = ""
	}
	raw, err := json.Marshal([]any{a.source, r})
	if err != nil {
		return ""
	}
	return string(raw)
}

// Reuse the existing native physical-generation owner, then replay the same
// pure ledger compiler to associate it with the exact compiled record. This
// neither opens a new path nor re-queries a trace. Bare IDs, equal text, JSON,
// stale sources and ambiguous original records cannot mint this private key.
func answerDocBusinessAccountSources(ctx *types.AgentContext) map[string]string {
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	input := types.ObservationLedgerInputFromAgentContext(ctx, 0)
	type witness struct{ source, record string }
	witnesses, conflicts := map[string]witness{}, map[string]bool{}
	for _, results := range [][]types.ToolResult{input.ToolResults, input.SystemTraceSupplementResults} {
		for _, result := range results {
			for _, r := range result.Observations {
				if _, ok := tool.DecodeTraceBusinessTreeFact(r); !ok {
					continue
				}
				source := types.RuntimeObservationPresentationSourceKey(input, result, r)
				w := witness{source, traceEventPublicationRecordKey(r)}
				if prior, found := witnesses[r.ID]; found && prior != w {
					conflicts[r.ID] = true
				}
				witnesses[r.ID] = w
			}
		}
	}
	if len(witnesses) == 0 {
		return nil
	}
	// No row-set persistence or other side effects in this pure replay.
	input.RowSetWriter = nil
	out := map[string]string{}
	for _, r := range types.CompileObservationLedger(input).Records {
		w, found := witnesses[r.ID]
		if !found || w.source == "" || conflicts[r.ID] {
			continue
		}
		if _, valid := tool.DecodeTraceBusinessTreeFact(r); valid {
			out[traceEventPublicationRecordKey(r)] = w.source
		}
	}
	return out
}
