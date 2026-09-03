package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// prepareTraceFindingContract activates typed findings only for an actual
// runtime-trace finalizer dispatch. Ordinary source-code, log-only, and narrow
// runtime fact answers retain their historical answer-document schema.
func prepareTraceFindingContract(ctx *types.AgentContext) error {
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	if current := ctx.Mutable.TraceFindingContract(); current != nil && strings.TrimSpace(current.CandidateSetID) != "" {
		return nil
	}
	authority := answerDocRuntimeTraceGuidanceView(ctx)
	// The guidance view is evidence-derived and therefore may remain false when
	// trace parsing/querying produced no publication-grade row. The typed
	// preflight/direct attachment is still sufficient to require an unresolved
	// finding for a causal trace analysis; otherwise the visible short-root-
	// cause section disappears exactly when the trace evidence is incomplete.
	if !authority.RuntimeTrace && !traceFindingHasTraceCarrier(ctx) {
		return nil
	}
	ledger := answerDocObservationLedger(ctx)
	set := types.CompileTraceCausalProjectionSet(ledger)
	var requestModel *types.RequestModel
	if ctx.AnalysisIR != nil {
		requestModel = &ctx.AnalysisIR.RequestModel
	}
	// An explicit bounded-fact request (for example "list this TID's state")
	// must not be widened into a root-cause conclusion. Undecided trace shapes
	// are allowed here only because the carrier check above proves that this is
	// an actual trace run; an empty candidate set then fails closed to
	// trace_finding.unresolved.
	if decided, allowed := types.RuntimeTraceReportShapeAuthority(requestModel); decided && !allowed {
		return nil
	}
	// SIDECAR-Q1 (user ruling 2026-09-02, §40.28 ②): the contract consumes the
	// SEAT-LEVEL evidence-ID authority index — the same one the Markdown crown
	// face consults — never the session-wide ANY aggregate (advisory-only; see
	// TestSessionAnyCausalSignalFeedsAdvisoryLanesOnly). Built from the same
	// ledger input the compiled ledger reads (limit 64, answerDocObservationLedger).
	seatInput := types.ObservationLedgerInputFromAgentContext(ctx, 64)
	seatAuthority := tracefinding.BuildSeatFrameCausalityAuthority(seatInput)
	logging.Debug("[trace_finding] seat authority: frame_question=%v tool_results=%d supplements=%d unproven_keys=%d",
		seatAuthority.Applicable, len(seatInput.ToolResults), len(seatInput.SystemTraceSupplementResults), len(seatAuthority.Index))
	contract, err := tracefinding.CompileCandidateContract(ledger, set, seatAuthority)
	if err != nil {
		return fmt.Errorf("compile trace finding contract: %w", err)
	}
	for _, candidate := range contract.Candidates {
		logging.Debug("[trace_finding] candidate %s subject=%q evidence=%v qualifier=%s",
			candidate.Decision.CandidateID, candidate.Decision.SubjectName, candidate.Decision.EvidenceRefs, candidate.Decision.CausalQualifier)
	}
	// The optional report accepts only exact ids from the typed on-chain
	// roster; model order owns the conclusion while the runtime owns every
	// copied fact.
	contract.RootCauseReportEnabled = len(tracefinding.SelectableRootCauseCandidates(contract)) > 0
	ctx.Mutable.SetTraceFindingContract(contract)
	return nil
}

func traceFindingHasTraceCarrier(ctx *types.AgentContext) bool {
	if ctx == nil {
		return false
	}
	if ctx.RuntimeArtifactPreflight.HasTraceArtifact() ||
		strings.TrimSpace(ctx.AttachedHitrace) != "" ||
		strings.TrimSpace(ctx.AttachedHitraceSource) != "" ||
		ctx.PerfTrace != nil {
		return true
	}
	if ctx.Mutable != nil && ctx.Mutable.PerfTrace() != nil {
		return true
	}
	return ctx.AnalysisIR != nil && ctx.AnalysisIR.RequestModel.PerfTrace != nil
}

// traceRootCauseRosterGroup is one rendered roster fence: the candidates of
// one trace file (label = the fold's partition key) as indented JSON.
type traceRootCauseRosterGroup struct {
	label string
	json  []byte
}

// traceRootCauseRosterGroups renders the selectable roster. Single-artifact
// contracts yield exactly ONE group (the whole roster, label-less — the
// legacy byte-identical fence); a multi-artifact contract (V1-4, §40.26 ②)
// yields one group per trace file in the contract's first-appearance
// partition order, candidates with no label (cannot exist in a multi-artifact
// fold by construction) trailing under a literal "unlabeled" heading rather
// than silently folding into another file's group.
//
// The second result names the contract's partition-roster labels that
// contributed no selectable candidate (§40.48 fold-in): the teaching counts
// the partition roster, so a silent partition is disclosed, not miscounted.
func traceRootCauseRosterGroups(selectable []types.TraceFindingCandidateV1, contract *types.TraceFindingContract) ([]traceRootCauseRosterGroup, []string, error) {
	type candidateView struct {
		CandidateID      string   `json:"candidate_id"`
		ArtifactLabel    string   `json:"artifact_label,omitempty"`
		CauseKind        string   `json:"cause_kind"`
		Subject          string   `json:"subject,omitempty"`
		Resource         string   `json:"resource,omitempty"`
		Phase            string   `json:"phase,omitempty"`
		Rank             int      `json:"rank,omitempty"`
		ImpactMS         float64  `json:"impact_ms"`
		ImpactCaliber    string   `json:"impact_caliber"`
		CausalQualifier  string   `json:"causal_qualifier"`
		ValueDescription string   `json:"value_description,omitempty"`
		EvidenceRefs     []string `json:"evidence_refs"`
	}
	view := func(candidate types.TraceFindingCandidateV1) candidateView {
		decision := candidate.Decision
		return candidateView{
			CandidateID: decision.CandidateID, ArtifactLabel: decision.ArtifactLabel, CauseKind: decision.Token.Token,
			Subject: decision.SubjectName, Resource: decision.ResourceName,
			Phase: decision.PhaseName, Rank: decision.Rank,
			ImpactMS: decision.Magnitude.Value, EvidenceRefs: decision.EvidenceRefs,
			ImpactCaliber: decision.Magnitude.Caliber, CausalQualifier: decision.CausalQualifier,
			ValueDescription: tracefinding.RootCauseValueDescription(decision),
		}
	}
	labels := []string{""}
	if contract.MultiArtifact() {
		labels = append(append([]string(nil), contract.ArtifactLabels...), "")
	}
	byLabel := map[string][]candidateView{}
	for _, candidate := range selectable {
		key := ""
		if contract.MultiArtifact() {
			key = strings.TrimSpace(candidate.Decision.ArtifactLabel)
			if _, known := byLabel[key]; !known && key != "" && !rosterLabelListed(labels, key) {
				// A label outside the contract roster still gets its own group.
				labels = append(labels[:len(labels)-1], key, "")
			}
		}
		byLabel[key] = append(byLabel[key], view(candidate))
	}
	var groups []traceRootCauseRosterGroup
	var silent []string
	for _, label := range labels {
		roster := byLabel[label]
		if len(roster) == 0 {
			if label != "" && rosterLabelListed(contract.ArtifactLabels, label) {
				silent = append(silent, label)
			}
			continue
		}
		b, err := json.MarshalIndent(roster, "", "  ")
		if err != nil {
			return nil, nil, err
		}
		heading := label
		if heading == "" && contract.MultiArtifact() {
			heading = "unlabeled"
		}
		groups = append(groups, traceRootCauseRosterGroup{label: heading, json: b})
	}
	return groups, silent, nil
}

func rosterLabelListed(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func renderTraceFindingContract(ctx *types.AgentContext) string {
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	contract := ctx.Mutable.TraceFindingContract()
	if contract == nil || strings.TrimSpace(contract.CandidateSetID) == "" {
		return ""
	}
	var out strings.Builder
	selectable := tracefinding.SelectableRootCauseCandidates(contract)
	if contract.RootCauseReportEnabled && len(selectable) > 0 {
		groups, silent, err := traceRootCauseRosterGroups(selectable, contract)
		if err == nil {
			out.WriteString("\n\n## Optional Trace Root Cause JSON\n\n")
			out.WriteString("- The full answer is the primary deliverable. `trace_root_causes` never replaces or blocks it. " + types.TraceRootCauseSelectorOutcomeTeaching() + " Omitting it on a later re-emit keeps the previously accepted selection; send `\"root_causes\": []` to withdraw it.\n")
			out.WriteString("- If useful, choose zero or more exact `candidate_id` values from the typed on-chain roster below and order them strongest to weakest. The runtime binds category, identity, impact, summary, and evidence from those receipts; do not author those fields.\n")
			out.WriteString("- Use `trace_root_causes` in `emit_answer_document`; use `replace_trace_root_causes` in `emit_answer_document_patch`. Both fields take the same native object: `{\"schema_version\":2,\"root_causes\":[{\"candidate_id\":\"<exact id from roster>\"}]}`. Keep `schema_version` inside that object, not at the document top level. Do not quote the object or the number.\n")
			out.WriteString("- A patch that only revises answer blocks should omit `replace_trace_root_causes`; the previously accepted report is retained. A replacement lists the complete ordered selection, not only the added entries.\n")
			out.WriteString("- Omit the field when no candidate should be selected. Background and adjacent observations are intentionally absent.\n")
			out.WriteString("- Optionally add `description` to each selection: " + types.TraceRootCauseDescriptionTeaching() + "\n")
			out.WriteString("- Each candidate carries its own `impact_caliber` (" + strings.Join(types.AllTraceImpactCalibers(), " vs ") + ") and `causal_qualifier`, a closed set copied verbatim onto the published sidecar: `proven` (this candidate's own trace evidence was checked for frame evidence and none withholds it), `frame_unproven` (checked and absent or unproven; seat-level — the same qualifier the answer headline wears), or `not_applicable` (this request is not a frame/jank question, so frame causality is not a claim the report makes and no headline qualifier appears). Selecting a frame_unproven candidate is allowed and stays disclosed as such.\n")
			if contract.MultiArtifact() {
				// V1-4 (§40.26 ②): the fold's partition key is taught and shown
				// grouped — a same-named thread in two trace files is two
				// candidates, and the label is the one the answer's per-trace
				// sections wear.
				// The count is the contract's partition roster (every trace
				// file in the fold), never the number of rendered groups: a
				// partition with no selectable candidate is named instead.
				fmt.Fprintf(&out, "- Candidates come from %d trace files. Each carries `artifact_label` — the same trace-file name used by the answer's per-trace sections. A thread with the same name in two trace files is two different candidates; select by `candidate_id`, never by name.", len(contract.ArtifactLabels))
				if len(silent) > 0 {
					fmt.Fprintf(&out, " Trace files with no selectable candidate: %s.", strings.Join(silent, ", "))
				}
				out.WriteString("\n")
			}
			out.WriteString("\n")
			for index, group := range groups {
				if index > 0 {
					out.WriteString("\n")
				}
				if contract.MultiArtifact() {
					fmt.Fprintf(&out, "**Trace file: %s**\n\n", group.label)
				}
				out.WriteString("```json\n")
				out.Write(group.json)
				out.WriteString("\n```\n")
			}
		}
	}
	return strings.TrimSpace(out.String())
}
