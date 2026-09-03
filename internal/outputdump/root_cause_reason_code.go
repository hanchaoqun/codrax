package outputdump

// root_cause_reason_code.go — the ONE list of customer-facing `reason_code`
// values a `.root-causes.json` sidecar can carry when `status` is
// "unavailable" (colleague_merge_audit §40.44 residual a). Append-only wire
// vocabulary: every producer (orchestrator finalize, CLI explicit report,
// this package's fallbacks) names a constant from this list, and the
// implementation guide's reason_code table is pinned to it
// (root_cause_reason_code_test.go).
const (
	// RootCauseReasonValidSelectionUnavailable: the model never submitted a
	// selector that bound validly and none was inherited — "never selected".
	RootCauseReasonValidSelectionUnavailable = "valid_model_root_cause_selection_unavailable"
	// RootCauseReasonSelectionRejected: the model DID submit a selector but
	// the binder rejected it (unknown / duplicate candidate_id, wrong
	// schema_version, malformed object) and no valid selection followed.
	RootCauseReasonSelectionRejected = "model_root_cause_selection_rejected"
	// RootCauseReasonNoSelectableCandidates: the contract exposed no typed
	// on-chain candidate the model could select.
	RootCauseReasonNoSelectableCandidates = "no_selectable_typed_on_chain_candidates"
	// RootCauseReasonContractNotActive: no evidence contract for root-cause
	// selection was established this run.
	RootCauseReasonContractNotActive = "trace_root_cause_contract_not_active"
	// RootCauseReasonRuntimeUnavailable: the run ended before any mutable
	// analysis state existed.
	RootCauseReasonRuntimeUnavailable = "trace_root_cause_runtime_unavailable"
	// RootCauseReasonTranscriptNotAvailable: the run exited early or formed
	// no answer; an unfinished draft selection is never published.
	RootCauseReasonTranscriptNotAvailable = "final_answer_transcript_not_available"
	// RootCauseReasonEncodingFailed: the bound report failed to encode and
	// was replaced by the legal empty JSON.
	RootCauseReasonEncodingFailed = "root_cause_report_encoding_failed"
	// RootCauseReasonFallbackUnavailable: a writer was handed no reason at
	// all (defensive fallback; every live producer names a reason).
	RootCauseReasonFallbackUnavailable = "valid_root_cause_selection_unavailable"
)

// AllRootCauseUnavailableReasonCodes lists the closed vocabulary in its
// documented order.
func AllRootCauseUnavailableReasonCodes() []string {
	return []string{
		RootCauseReasonValidSelectionUnavailable,
		RootCauseReasonSelectionRejected,
		RootCauseReasonNoSelectableCandidates,
		RootCauseReasonContractNotActive,
		RootCauseReasonRuntimeUnavailable,
		RootCauseReasonTranscriptNotAvailable,
		RootCauseReasonEncodingFailed,
		RootCauseReasonFallbackUnavailable,
	}
}
