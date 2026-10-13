package agent

import (
	"encoding/json"
	"reflect"

	"github.com/hanchaoqun/codrax/internal/types"
)

// A display bridge connects a strictly verified original tool publication to
// its exact ledger-compiled record. It does not reverse or approximate capture
// enrichment, and cannot confer binding, source or causal authority.
type traceEventPublicationBridge map[string]types.RuntimeMeasurementPublication

func traceEventPublicationRecordKey(record types.ObservationRecord) string {
	data, err := json.Marshal(record)
	if err != nil {
		return ""
	}
	return string(data)
}

func buildTraceEventPublicationBridge(ctx *types.AgentContext) traceEventPublicationBridge {
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	input := types.ObservationLedgerInputFromAgentContext(ctx, 0)
	publications := map[string]types.RuntimeMeasurementPublication{}
	ambiguous := map[string]bool{}
	for _, results := range [][]types.ToolResult{input.ToolResults, input.SystemTraceSupplementResults} {
		for _, result := range results {
			if !result.Success || result.ToolName != "trace_query" {
				continue
			}
			for _, record := range result.Observations {
				if record.Predicate != types.TraceEventSearchInventoryPredicate {
					continue
				}
				publication, valid := types.DecodeRuntimeMeasurementPublication(record)
				prior, found := publications[record.ID]
				if !valid || found && !reflect.DeepEqual(prior, publication) {
					ambiguous[record.ID] = true
					continue
				}
				publications[record.ID] = publication
			}
		}
	}
	if len(publications) == 0 {
		// Context was available, but no original native publication verified.
		// An explicit empty bridge forbids post-enrichment validation from
		// accidentally rehabilitating a forged or stale original source.
		return traceEventPublicationBridge{}
	}
	// Replay the existing pure compiler with the same tool order and run-entry
	// preflight. It owns ToolCallID, ArtifactID and CaptureIdentityPath changes;
	// this adapter has no independent list of fields it is allowed to clear.
	compiled := types.CompileObservationLedger(types.ObservationLedgerInput{
		RepoRoot: input.RepoRoot, RuntimeArtifactPreflight: input.RuntimeArtifactPreflight,
		RequestModel: input.RequestModel, ToolResults: traceEventPublicationReplayResults(input.ToolResults),
		SystemTraceSupplementResults: traceEventPublicationReplayResults(input.SystemTraceSupplementResults),
	})
	bridge := traceEventPublicationBridge{}
	for _, record := range compiled.Records {
		publication, found := publications[record.ID]
		if !found || ambiguous[record.ID] || record.Predicate != types.TraceEventSearchInventoryPredicate {
			continue
		}
		if key := traceEventPublicationRecordKey(record); key != "" {
			bridge[key] = publication
		}
	}
	return bridge
}

func traceEventPublicationReplayResults(results []types.ToolResult) []types.ToolResult {
	// Keep the original indices and supplement boundary, but do not recompile
	// unrelated observations, summaries, row sets or typed carrier payloads.
	out := make([]types.ToolResult, len(results))
	for n, result := range results {
		if !result.Success || result.ToolName != "trace_query" {
			continue
		}
		var records []types.ObservationRecord
		for _, record := range result.Observations {
			if record.Predicate == types.TraceEventSearchInventoryPredicate {
				records = append(records, record)
			}
		}
		if len(records) > 0 {
			out[n] = types.ToolResult{ToolName: result.ToolName, Success: true, Timestamp: result.Timestamp, RawRef: result.RawRef, Observations: records}
		}
	}
	return out
}
