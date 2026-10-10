package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Reuse the exact finalizer selection rather than deriving a second roster
// from pre-triage seeds. This is a presentation link, not new causal, source,
// completeness or relation authority. The lossless ledger remains untouched.
func answerDocPublishedNativeFacts(ctx *types.AgentContext) []types.ObservationRecord {
	ledger := answerDocObservationLedger(ctx)
	records, _ := answerDocFinalizerObservationRecords(ctx, ledger.Records)
	records, _, _ = answerDocIOWindowObservationRecords(ctx, records)
	records, _ = answerDocSelectedWindowObservationRecords(ctx, records)
	byID := make(map[string]types.ObservationRecord, len(records))
	for _, record := range records {
		kind := types.NativeRuntimeFactPresentationKind(record)
		if kind != "" && kind != "log_query_coverage" && answerDocNativeFactInRequestedWindow(ctx, record, kind) {
			byID[record.ID] = record
		}
	}
	var out []types.ObservationRecord
	for _, row := range answerDocObservationPromptRecords(ctx, records, answerDocObservationLedgerPromptLimit) {
		if record, ok := byID[row.ID]; ok {
			out = append(out, record)
		}
	}
	return out
}

// Some native rows carry only the parent query's typed window. Do not promote
// a known outside/wider query into the main fact roster just because it lacks
// a local projection note. An existing local selected_window was already
// checked above and may legitimately project a broader causal query.
func answerDocNativeFactInRequestedWindow(ctx *types.AgentContext, record types.ObservationRecord, kind string) bool {
	if kind != "trace_record" || ctx == nil || ctx.AnalysisIR == nil ||
		ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile == nil || !record.SourceRef.QueryWindowKnown {
		return true
	}
	requested := ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile
	if !requested.HasExplicitTimeWindows() {
		return true
	}
	if _, _, present := types.TraceCausalProjectionSelectedWindowNote(record.RichNotes); present {
		return true
	}
	return requested.ContainsExplicitTimeWindow(record.SourceRef.QueryWindowStartTs, record.SourceRef.QueryWindowEndTs)
}

func answerDocHasNativeFactHandoff(ctx *types.AgentContext) bool {
	if len(answerDocPublishedNativeFacts(ctx)) > 0 {
		return true
	}
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	return view != nil && view.RuntimeMeasurementContract.Active()
}

// Support-lane teaching previously described only the pre-triage seed list as
// eligible, silently making fuller native data elsewhere look like background.
// Link the existing rows/tables once; do not reprint their values or invent
// source citations. On-chain/adjacent/background qualifications stay row-local.
func renderAnswerDocNativeFactSupport(ctx *types.AgentContext) string {
	records := answerDocPublishedNativeFacts(ctx)
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	measurements := view != nil && view.RuntimeMeasurementContract.Active()
	if len(records) == 0 && !measurements {
		return ""
	}
	var b strings.Builder
	b.WriteString("### Published native artifact facts\n\n")
	b.WriteString("The source-bound Observation Ledger rows and verified measurement tables are also eligible observation support for the requested facts, not merely background because they are absent from the older preprocessing seed list. Use their original fields, source/query scope and unknown values; earlier model summaries or planning labels do not override them. Choose answer blocks allowed by the current answer schema. This presentation link grants no new causal edge, on-chain status, shared clock, current-source proof or completeness. Preserve each row's existing causal qualifications and any independent grounded business explanation.\n")
	for _, kind := range []string{"log_record", "trace_record"} {
		var ids []string
		for _, record := range records {
			if types.NativeRuntimeFactPresentationKind(record) == kind {
				ids = append(ids, record.ID)
			}
		}
		if len(ids) > 0 {
			encoded, _ := json.Marshal(struct {
				Kind string   `json:"producer_kind"`
				IDs  []string `json:"observation_ids"`
			}{kind, ids})
			fmt.Fprintf(&b, "- %s\n", encoded)
		}
	}
	if measurements {
		b.WriteString("- Use the published runtime_measurement observation_id/view selectors for native measurement rows; their table-specific units, scope, coverage and missing-value boundaries remain authoritative. The original selector roster contains the full data; it is not duplicated here.\n")
	}
	b.WriteString("\n")
	return b.String()
}
