package agent

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// renderAnswerDocCallChainFinalEvidenceBoundary keeps the last prompt surface
// aligned with the typed call-chain contract. It is language-agnostic and
// prompt-only: names and request wording never become behavior authority.
func renderAnswerDocCallChainFinalEvidenceBoundary(ctx *types.AgentContext, view *types.AnswerSemanticView) string {
	if view == nil || view.Family != types.QFCallChain {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Final Call-Chain Evidence Boundary\n\n")
	b.WriteString("- You own the explanation. Preserve only directed hops carried by grounded caller-to-callee evidence. A call-site proves that edge, not the callee's body, side effect, storage medium, synchronization mode, or completion semantics.\n")
	b.WriteString("- Keep the requested conceptual destination separate from the current implementation. Class names, method names, comments, layer labels, and request wording do not prove what the endpoint currently does.\n")
	b.WriteString(renderAnswerDocSelectedTerminalImplementationBoundary(ctx))
	b.WriteString("- Keep the model-authored summary useful and concise; this boundary supplies evidence caliber only and does not author a conclusion.\n\n")
	return b.String()
}

// renderAnswerDocSelectedTerminalImplementationBoundary repeats only the
// parser-grounded operations discovered inside an already-read selected
// terminal body. It is deliberately a prompt-tail evidence view, not an
// effect classifier: the system neither infers business semantics from callee
// names nor inspects/replaces model prose.
func renderAnswerDocSelectedTerminalImplementationBoundary(ctx *types.AgentContext) string {
	if ctx == nil {
		return "- selected_terminal_body_calls=`unproven`: use separately grounded terminal definition/mechanism facts when present; otherwise say only that the grounded chain reaches or invokes the endpoint.\n"
	}
	type row struct {
		caller   string
		callee   string
		location string
	}
	groups := make(map[string][]row)
	var groupOrder []string
	seen := make(map[string]bool)
	for _, item := range ctx.EvidenceItems {
		if item.Producer != types.EvidenceProducerRepoMapTerminalBodyCall ||
			types.ClaimFormOf(item) != types.ClaimCallEdge || !item.IsCitable() {
			continue
		}
		caller := strings.TrimSpace(item.Subject)
		callee := strings.TrimSpace(item.Object)
		location := strings.TrimSpace(item.DisplayLocation(true))
		if caller == "" || callee == "" || location == "" {
			continue
		}
		key := strings.ToLower(caller + "\x00" + callee + "\x00" + location)
		if seen[key] {
			continue
		}
		seen[key] = true
		groupKey := strings.ToLower(caller)
		if _, ok := groups[groupKey]; !ok {
			groupOrder = append(groupOrder, groupKey)
		}
		groups[groupKey] = append(groups[groupKey], row{caller: caller, callee: callee, location: location})
	}
	rows := make([]row, 0, 8)
	profile := (*types.CallChainEndpointProfile)(nil)
	if ctx.AnalysisIR != nil {
		profile = ctx.AnalysisIR.RequestModel.CallChainEndpointProfile
	}
	candidateMode := profile == nil || profile.DiscoverTerminalActive() || profile.DiscoverPathActive()
	if candidateMode {
		// A conceptual destination can leave several grounded graph leaves. Keep
		// one exact operation from every leaf instead of letting a utility-heavy
		// helper monopolize the prompt tail. These remain candidates: endpoint
		// selection and business interpretation stay with the model.
		for _, key := range groupOrder {
			if len(groups[key]) == 0 {
				continue
			}
			rows = append(rows, groups[key][0])
			if len(rows) >= 8 {
				break
			}
		}
	} else {
		// Exact endpoints and runtime-selected destinations have typed selection
		// authority, so a bounded set of operations from the selected body is
		// useful and cannot be confused with unrelated graph leaves.
		for depth := 0; len(rows) < 8; depth++ {
			added := false
			for _, key := range groupOrder {
				if depth >= len(groups[key]) {
					continue
				}
				rows = append(rows, groups[key][depth])
				added = true
				if len(rows) >= 8 {
					break
				}
			}
			if !added {
				break
			}
		}
	}
	if len(rows) == 0 {
		return "- selected_terminal_body_calls=`unproven`: use separately grounded terminal definition/mechanism facts when present; otherwise say only that the grounded chain reaches or invokes the endpoint.\n"
	}

	var b strings.Builder
	if candidateMode {
		fmt.Fprintf(&b, "- terminal_body_candidates=`parser_grounded`; candidate_count=`%d`. The typed graph has multiple or not-yet-selected leaf callables because the request supplied a conceptual destination. Each caller below is a candidate endpoint, not automatically the requested business destination. Compare its exact operation with that destination before choosing it; keep storage, durability, flushing, synchronization, and completion unproven unless separate typed evidence establishes them. Investigator closure wording, names, comments, and layer labels do not upgrade these exact operations:\n", len(rows))
	} else {
		b.WriteString("- selected_terminal_body_calls=`parser_grounded`; each row proves only its exact operation. Describe that operation and keep storage, durability, flushing, synchronization, and completion unproven unless separate typed evidence establishes them; separately grounded terminal definition/mechanism facts remain valid:\n")
	}
	for _, row := range rows {
		fmt.Fprintf(&b, "  - caller=`%s`; exact_operation=`%s`; source=`%s`; effect_scope=`exact_call_only`.\n",
			answerDocCallChainInline(row.caller), answerDocCallChainInline(row.callee), answerDocCallChainInline(row.location))
	}
	return b.String()
}

// renderAnswerDocTraceFinalDecisionBoundary replays the small set of typed
// authority limits that must govern the model's synthesis after every other
// dynamic section has been rendered. It deliberately does not copy candidate
// labels or choose a cause/recommendation. The deterministic system continues
// to own facts while the model owns the conclusion.
func renderAnswerDocTraceFinalDecisionBoundary(ctx *types.AgentContext) string {
	if ctx == nil {
		return ""
	}
	authority := answerDocRuntimeTraceGuidanceView(ctx)
	if !authority.RuntimeTrace {
		return ""
	}
	ledger := answerDocObservationLedger(ctx)
	set := types.CompileTraceCausalProjectionSet(ledger)
	var requestModel *types.RequestModel
	if ctx.AnalysisIR != nil {
		requestModel = &ctx.AnalysisIR.RequestModel
	}
	if len(set.Projections) == 0 || !types.RuntimeTraceReportMaterializationAllowed(requestModel, set) {
		return ""
	}

	hasActual, hasEliminable := traceDecisionAxesPresent(set)
	var b strings.Builder
	b.WriteString("## Final Trace Decision Boundary (Typed Facts; Model-Owned Conclusion)\n\n")
	b.WriteString("- You own the diagnosis, prioritization, optimization direction, and wording. The system supplies measurements and authority ceilings only; do not merely restate the projection rows.\n")
	if view := types.BuildAnswerSemanticViewForAgentContext(ctx); view != nil && view.TraceCausalClaimContract.Active() {
		allowed := make([]string, 0, len(view.TraceCausalClaimContract.Allowed))
		for _, caliber := range view.TraceCausalClaimContract.Allowed {
			allowed = append(allowed, string(caliber))
		}
		fmt.Fprintf(&b, "- principal_trace_summary_contract: %s Keep the lead/detail wording within the declared `%s` scope. This is your causal-strength declaration; it does not choose the cause. No conclusion is inferred from prose or written for you.\n",
			tool.TraceCausalClaimPrincipalSummaryShape(view.TraceCausalClaimContract.Allowed), strings.Join(allowed, "|"))
		b.WriteString(renderTraceCausalClaimCaliberMapping(view.TraceCausalClaimContract))
	}
	if authority.CausalUnproven {
		b.WriteString("- causal_conclusion=`unproven`: the strongest supported synthesis is a bounded candidate or first validation direction, not a proven dropped-frame/frame-deadline cause.\n")
	}
	if authority.FrameEvidenceStatus != "" {
		fmt.Fprintf(&b, "- frame_evidence_status=`%s`: do not infer a stronger frame/deadline attribution.\n", authority.FrameEvidenceStatus)
		b.WriteString(renderTraceFrameEvidenceStatusSemantics(authority.FrameEvidenceStatus))
	}
	b.WriteString(renderTraceFinalSelectedWindowAuthority(set, authority.FrameEvidenceStatus))
	b.WriteString(renderTraceFinalTimeRoleAuthority(set))
	b.WriteString(renderTraceFinalBlockedReasonStateRelation(set, ledger))
	if types.RuntimeTraceTargetWaitMaterializationAllowed(requestModel, set) {
		b.WriteString(renderTraceFinalTargetWaitEnumerationAuthority(ledger, requestModel))
	}
	b.WriteString("- scheduler_state_interval_authority=`typed_state_segments`: a typed wakeup ends the preceding sleep/io_wait segment; time from wakeup until the next sched-in is runnable_wait. Do not extend an IO/D/sleep duration to the later run timestamp or relabel the two state segments as one wait state.\n")
	b.WriteString("- trace_value_caliber_authority=`measured_occupancy_vs_effective_attribution`: measured state occupancy/cumulative duration and effective attribution are different axes. Effective attribution is the published ranking/eliminable value; never call it an actual wait/state duration when a distinct measured occupancy is provided.\n")
	b.WriteString(renderTraceFinalWakeupCPUTopologyAuthority(ledger))
	b.WriteString(renderTraceFinalSemanticRelationOnlyAuthority(set))
	b.WriteString(renderTraceFinalStateValueAuthority(set))
	b.WriteString(renderTraceFinalSupplyFoldValueAuthority(set))
	switch {
	case hasActual && hasEliminable:
		b.WriteString("- available_axes=`actual_occupancy,existing_rule_eliminable`: compare both and explain their different decision use. Actual occupancy, existing-rule eliminable impact, and proven frame causality are distinct calibers; none substitutes for another. Their coexistence does not prove physical independence.\n")
	case hasActual:
		b.WriteString("- available_axes=`actual_occupancy`: identify the measured time concentration and a validation/optimization direction without inventing an existing-rule eliminable amount.\n")
	case hasEliminable:
		b.WriteString("- available_axes=`existing_rule_eliminable`: prioritize the typed repair candidates without inventing a separate actual-occupancy ranking.\n")
	default:
		b.WriteString("- available_axes=`none`: stay within the target-state, path, and evidence-boundary facts; do not invent a ranked cause.\n")
	}
	b.WriteString(renderTraceFinalCompactAuthorityLedger(set))
	b.WriteString(renderTraceFinalAggregateScaleAuthority(traceDecisionTypedAggregateFacts(ledger.Records)))
	b.WriteString("- compact_unknowns: evidence_absence_implication=`unknown_not_false`; target_direct_blocking_not_established_does_not_prove_no_external_blocking=`true`; cross_direction_physical_relation=`unresolved_unless_an_exact_pair_row_says_otherwise`; absent_overlap_record_proves_independence=`false`; cause_decomposition_status=`not_closed_by_state_partition_or_ranked_seat_roster`; exhaustive_cause_wording=`requires_one_exact_typed_additive_cause_partition`. An unestablished typed mechanism is unknown, not physically absent; missing relation evidence authorizes neither `independent` nor `no overlap`; a target state partition closes only what state the target experienced, not why it experienced it.\n")
	b.WriteString("- cross_row_addition=`not_authorized_without_exact_typed_relation`: a row-local state breakdown applies only to that row. Do not merge, decompose, compare as one subtotal, or add values from different rows/threads/fix directions unless one exact typed relation/fold carrier names those members and authorizes that operation.\n")
	b.WriteString(renderTraceFinalSynthesisScope(set, authority.FrameEvidenceStatus))
	b.WriteString("- relation_scope=`typed_relations_only`: preserve directed wakeup/path and typed holder/waiter or overlap relations exactly. Temporal order, adjacency, a candidate flag, or a kernel caller symbol alone does not prove synchronous blocking, lock ownership, post-wakeup preemption, or physical coupling.\n\n")
	return b.String()
}

// renderTraceFinalWakeupCPUTopologyAuthority promotes the exact CPU tuple for
// each typed wakeup edge into the pre-generation finalizer context. The same
// compiler also feeds the post-answer fact appendix, preventing a late-only
// correction from disagreeing with what the model saw while synthesizing.
// This is evidence guidance only: it neither inspects nor rewrites prose and
// it does not choose a cause.
func renderTraceFinalWakeupCPUTopologyAuthority(ledger types.ObservationLedger) string {
	rows := types.BuildTraceWakeupCPUTopologyAuthorities(ledger)
	if len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	for _, row := range rows {
		fmt.Fprintf(&b, "- wakeup_cpu_topology_authority waker=`%s`; wakee=`%s`; waker_cpu=`%d`; wakee_target_cpu=`%d`; cpu_relation=`%s`; ",
			traceDecisionPromptScalar(row.Waker), traceDecisionPromptScalar(row.Wakee),
			row.WakerCPU, row.WakeeTargetCPU, row.Relation)
		if row.Relation == types.TraceWakeupCPUTopologyCrossCPU {
			b.WriteString("this exact edge proves cross-CPU wakeup placement. Do not say the waker occupied, preempted, or directly competed on the wakee target CPU, and do not attribute the wakee's post-wakeup runnable delay to the waker's later work without a separate exact typed target-wait/completion or compatible competition relation.\n")
			continue
		}
		b.WriteString("this exact edge proves same-CPU wakeup placement only. Direct competition or preemption still requires a separate compatible typed running/runnable overlap; placement alone is not that evidence.\n")
	}
	b.WriteString("- wakeup_cpu_topology_unknowns=`remain_unknown`: an edge omitted from this exact roster, or carrying an unpublished/unknown relation, must not be guessed as same-CPU or cross-CPU from names, prose, temporal adjacency, or another edge's tuple.\n")
	return b.String()
}

// renderTraceFinalSemanticRelationOnlyAuthority keeps B830's typed semantic
// relation boundary salient at the final synthesis tail. It activates only
// when the compiled projection contains the precise relation-only basis enum;
// it does not inspect request/model/final prose, choose a cause, or mutate an
// answer. The model remains free to explain and prioritize the raw business
// clue, but cannot mistake interval/edge relation for a target wait/completion
// mechanism that the producer explicitly withheld.
func renderTraceFinalSemanticRelationOnlyAuthority(set types.TraceCausalProjectionSet) string {
	for _, projection := range set.Projections {
		pools := [][]types.TraceCausalProjectionNode{
			projection.PrimaryRootCauses,
			projection.RankedSeats,
			projection.OnChainCauses,
			projection.SemanticSpans,
		}
		for _, pool := range pools {
			for _, node := range pool {
				if !node.IsSemanticRelationOnly() {
					continue
				}
				return "- semantic_relation_only_authority=`typed_basis_present`: `semantic_chain_interval_relation` and `host_wakeup_edge_pre_span` prove only an on-chain interval/edge relation plus raw semantic occupancy/business identity. They publish `effective_impact_ms=0` and no rank seat until a separate typed target-wait or semantic-completion binding exists. Keep the raw optimization clue, but do not say the target slept waiting for that operation, that operation completion triggered the wakeup, or the operation directly blocked the target; state the exact wakeup/path relation separately.\n"
			}
		}
	}
	return ""
}

// renderAnswerDocLogPeerFinalDecisionBoundary replays the precise LogBundle
// relation ceiling after the large generic finalizer prompt. It neither reads
// user/model/answer prose nor chooses the answer: the model may identify and
// compare every observed error/frame, but a cross-error cause remains unproven
// unless the bundle contains a validated recursive CauseRelation.
func renderAnswerDocLogPeerFinalDecisionBoundary(ctx *types.AgentContext) string {
	if !answerDocLogPeerRelationUnproven(ctx) {
		return ""
	}
	scopeBoundary := "- If relationship attribution matters, keep the cross-error relationship unproven or present it only as a follow-up hypothesis. The final wording and conclusion remain model-authored.\n"
	if runtimePeerErrorBoundedFactSet(ctx) {
		scopeBoundary = "- runtime_question_scope=`bounded_fact_set`: the requested answer is the finite per-occurrence facts and carries no causal-attribution dimension. Do not add a cross-error root-cause/propagation conclusion; mention the relationship only as unproven when needed for clarity. The final wording and conclusion remain model-authored.\n"
	}
	return "## Final Runtime Error Relation Boundary (Typed Facts; Model-Owned Conclusion)\n\n" +
		"- The attached artifact contains multiple top-level peer error occurrences. Each occurrence's own type, message, frames, and within-stack order are available observations.\n" +
		"- cross_error_relation=`unproven`: no validated explicit artifact marker connects one top-level error as the direct cause, caller/callee continuation, or propagation of another. Similar wording, adjacent lines/timestamps, bridge-like names, and shared IDs do not establish that edge.\n" +
		"- Answer the requested per-error/frame dimensions independently from the typed occurrences: identify each occurrence's own first observed frame and its within-stack callers. Do not label either peer occurrence as the other's true/underlying trigger, capture point, received failure, propagation, caller, or callee; those are cross-peer claims not established by this artifact shape. A message may be quoted as that occurrence's literal text without upgrading it into an edge.\n" +
		scopeBoundary + "\n"
}

// renderAnswerDocPerfSampleStatisticalBoundary gives the answer model the
// producer-owned statistical caliber after generic presentation guidance. It
// consumes only a typed trace_query observation and never inspects or changes
// user/model/final prose.
func renderAnswerDocPerfSampleStatisticalBoundary(ctx *types.AgentContext) string {
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromAgentContext(ctx, 1))
	for _, record := range ledger.Records {
		if record.Origin != types.AnswerEvidenceOriginRuntimeArtifact ||
			!types.RuntimeObservationProducerIsDeterministicQuery(record.Producer) ||
			record.Predicate != "perf_sample_statistical_caliber" {
			continue
		}
		caliber := ""
		prefix := types.TraceNoteKeyPerfStatisticalCaliber + "="
		for _, note := range record.RichNotes {
			if strings.HasPrefix(strings.TrimSpace(note), prefix) {
				caliber = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(note), prefix))
				break
			}
		}
		if caliber == "" {
			continue
		}
		return "## Final Perf-Sample Statistical Boundary (Typed Facts; Model-Owned Conclusion)\n\n" +
			"- observed_sample_count=`" + record.Value + "`; unit=`" + record.Unit + "`; observed_rank_scope=`" + record.Object + "`.\n" +
			"- statistical_caliber=`" + caliber + "`.\n" +
			"- A sample count is an observation count, not elapsed-time coverage. A top-row percentage or sample weight is scoped to the observed same-event/unit cohort; it is not CPU utilization, a fraction of scheduler running time, or temporal profiler coverage.\n" +
			"- With one observation, report the observed IP/module as one hit rather than a comparative workload hotspot. Without a sampling-design/representativeness receipt, workload-hotspot confidence and temporal coverage remain unavailable. The final explanation remains model-authored.\n\n"
	}
	return ""
}

// renderAnswerDocTargetCPUIdentityBoundary publishes the deterministic
// scheduler CPU roster at the final decision tail. It prevents ftrace's
// task/PID field from competing with the actual [CPU] column without reading
// or rewriting model prose. A migration conclusion still belongs to the model
// and requires typed multi-CPU or migration evidence.
func renderAnswerDocTargetCPUIdentityBoundary(ctx *types.AgentContext) string {
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromAgentContext(ctx, 1))
	type cpuRow struct {
		subject    string
		object     string
		value      string
		unit       string
		roster     string
		assignment string
	}
	seen := map[string]bool{}
	rows := make([]cpuRow, 0)
	for _, record := range ledger.Records {
		if record.Origin != types.AnswerEvidenceOriginRuntimeArtifact ||
			!types.RuntimeObservationProducerIsDeterministicQuery(record.Producer) ||
			record.Predicate != "target_cpu_running" ||
			strings.TrimSpace(record.Subject) == "" || strings.TrimSpace(record.Object) == "" {
			continue
		}
		row := cpuRow{
			subject:    strings.TrimSpace(record.Subject),
			object:     strings.TrimSpace(record.Object),
			value:      strings.TrimSpace(record.Value),
			unit:       strings.TrimSpace(record.Unit),
			roster:     strings.TrimSpace(traceDecisionRichNoteValue(record.RichNotes, types.TraceNoteKeyTargetCPURunningRosterStatus)),
			assignment: strings.TrimSpace(traceDecisionRichNoteValue(record.RichNotes, types.TraceNoteKeyTargetCPURunningAssignmentStatus)),
		}
		key := strings.Join([]string{row.subject, row.object, row.value, row.unit, row.roster, row.assignment}, "\x00")
		if seen[key] {
			continue
		}
		seen[key] = true
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return ""
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].subject != rows[j].subject {
			return rows[i].subject < rows[j].subject
		}
		return rows[i].object < rows[j].object
	})
	var b strings.Builder
	b.WriteString("## Final Target CPU Identity Boundary (Typed Facts; Model-Owned Conclusion)\n\n")
	for _, row := range rows {
		fmt.Fprintf(&b, "- target=`%s`; scheduler_cpu=`%s`; running=`%s%s`; roster_status=`%s`; assignment_status=`%s`.\n",
			row.subject, row.object, row.value, row.unit, firstNonEmpty(row.roster, "unknown"), firstNonEmpty(row.assignment, "unknown"))
	}
	b.WriteString("- ftrace_header_identity: the task header's parenthesized numeric field is PID/TGID identity, not a CPU number. Scheduler CPU identity comes from the `[NNN]` CPU column and deterministic target CPU rows; perf-sample CPU identity comes from an explicit typed `cpu=` field when `cpu_known=true`.\n")
	b.WriteString("- migration_evidence_boundary: do not infer CPU migration by comparing PID/TGID with a CPU field. A migration statement requires a typed migration event or compatible target-running rows on multiple CPUs. The final explanation remains model-authored.\n\n")
	return b.String()
}

func answerDocLogPeerRelationUnproven(ctx *types.AgentContext) bool {
	if ctx == nil || ctx.Mutable == nil {
		return false
	}
	bundle := ctx.Mutable.LogTriage()
	return bundle != nil && len(bundle.Errors) > 1
}

// renderTraceFinalTargetWaitEnumerationAuthority keeps a complete target-wait
// occurrence rowset authoritative at the final decision tail. Generic
// root-cause/blocking candidate views may be compacted independently; their
// display cap cannot turn an already-complete same-result target roster into
// "missing" physical occurrences. This is typed prompt context only and does
// not inspect or rewrite the model answer.
func renderTraceFinalTargetWaitEnumerationAuthority(ledger types.ObservationLedger, rm *types.RequestModel) string {
	waits := types.BuildTraceTargetWaitSummaryAuthorities(ledger, rm)
	if len(waits) == 0 {
		return ""
	}
	hasRequestedPrincipal := false
	for _, wait := range waits {
		if wait.IsRequestedScopePrincipal() {
			hasRequestedPrincipal = true
			break
		}
	}
	var b strings.Builder
	for _, wait := range waits {
		if hasRequestedPrincipal && !wait.IsRequestedScopePrincipal() {
			continue
		}
		role := strings.TrimSpace(string(wait.RequestedScopeRole))
		if role == "" {
			role = "unclassified"
		}
		fmt.Fprintf(&b, "- target_wait_enumeration_authority artifact=`%s`; subject=`%s`; selected_window=`%.6f..%.6f`; scope_role=`%s`; rowset_permission=`exact_complete_same_result`; occurrence_count=%d; complete_occurrence_ordinals=`1..%d`; wall_clock_sum=%.3fms; candidate_view_compaction_role=`does_not_downgrade_this_rowset`; missing_occurrence_inference=`forbidden`; residual_count_or_duration_estimation=`forbidden`. Every declared occurrence row is already present in the typed principal roster above. A capped root-cause, blocking, or display view may omit candidates from its own view, but it does not prove that any occurrence in this complete target-wait rowset is missing from the trace.\n",
			traceDecisionPromptScalar(wait.ArtifactLabel), traceDecisionPromptScalar(wait.Subject),
			wait.WindowStartTs, wait.WindowEndTs, traceDecisionPromptScalar(role),
			wait.Count, wait.Count, wait.WallClockMS)
	}
	return b.String()
}

// renderTraceFinalBlockedReasonStateRelation keeps two independent kernel
// observation domains separate at the point where the model forms its final
// diagnosis. A target state account is sched_switch interval wall clock;
// blocked_reason is a record census whose delay field is vendor-reported.
// Matching the already-selected projection subject/window/capture is precise
// typed data plumbing. It does not infer a user target from request prose and
// does not bind a census record to any particular state occurrence or cause
// seat.
func renderTraceFinalBlockedReasonStateRelation(set types.TraceCausalProjectionSet, ledger types.ObservationLedger) string {
	var b strings.Builder
	seen := map[string]bool{}
	for _, projection := range set.Projections {
		account := projection.TargetStateAccount
		if account == nil || strings.TrimSpace(account.Subject) == "" ||
			!types.TraceCausalProjectionWindowPresent(account.WindowStartTs, account.WindowEndTs) {
			continue
		}
		for _, record := range ledger.Records {
			if record.Origin != types.AnswerEvidenceOriginRuntimeArtifact ||
				!types.RuntimeObservationProducerIsDeterministicQuery(record.Producer) ||
				record.GroundingPolicy != types.ClaimGroundingHard ||
				strings.TrimSpace(record.Predicate) != "blocked_reason_census" ||
				!strings.EqualFold(strings.TrimSpace(record.Subject), strings.TrimSpace(account.Subject)) {
				continue
			}
			count, err := strconv.Atoi(strings.TrimSpace(record.Value))
			if err != nil || count <= 0 || record.ResultCount == nil || *record.ResultCount != count {
				continue
			}
			start, end, ok := types.TraceCausalProjectionSelectedWindowNote(record.RichNotes)
			if !ok || math.Abs(start-account.WindowStartTs) > types.TraceCausalProjectionSameWindowToleranceS ||
				math.Abs(end-account.WindowEndTs) > types.TraceCausalProjectionSameWindowToleranceS ||
				!traceFinalProjectionOwnsObservation(projection, record) {
				continue
			}
			callers := strings.TrimSpace(traceDecisionRichNoteValue(record.RichNotes, types.TraceNoteKeyBlockedReasonCensus))
			if callers == "" {
				continue
			}
			key := strings.ToLower(strings.TrimSpace(account.Subject)) + "\x00" +
				fmt.Sprintf("%.6f\x00%.6f\x00%d\x00%s", start, end, count, callers)
			if seen[key] {
				continue
			}
			seen[key] = true
			fmt.Fprintf(&b, "- blocked_reason_state_relation subject=`%s`; selected_window=`%.6f..%.6f`; scheduler_state_caliber=`sched_switch_interval_wall_clock`; d_state=%.3fms; io_wait=%.3fms; blocked_reason_records=%d; blocked_reason_census=`%s`; blocked_reason_caliber=`kernel_record_count_and_vendor_reported_delay_sum`; relation=`unjoined_distinct_observation_domains`; record_to_state_occurrence_mapping=`not_provided`; count_or_delay_difference_interpretation=`forbidden`; arithmetic_recomposition=`forbidden`. Report both observations under their own rulers. Do not pair records with state segments, substitute the census delay sum for state wall clock, or explain a count/duration difference as missing, extra, omitted, or mismatched events unless a separate typed interval join provides that mapping.\n",
				traceDecisionPromptScalar(account.Subject), start, end, account.DStateMS, account.IOWaitMS,
				count, traceDecisionPromptScalar(callers))
		}
	}
	return b.String()
}

func traceFinalProjectionOwnsObservation(projection types.TraceCausalProjection, record types.ObservationRecord) bool {
	projectionPath := strings.TrimSpace(projection.ArtifactPath)
	recordPath := strings.TrimSpace(types.RuntimeArtifactCaptureIdentityPath(record.SourceRef))
	if projectionPath != "" && recordPath != "" {
		return len(types.TraceArtifactCaptureIdentityPaths([]string{projectionPath, recordPath})) == 1
	}
	projectionLabel := strings.TrimSpace(projection.ArtifactLabel)
	artifactID := strings.TrimSpace(record.SourceRef.ArtifactID)
	return projectionLabel != "" && artifactID != "" && strings.EqualFold(projectionLabel, artifactID)
}

// renderTraceFinalTimeRoleAuthority repeats the selected-window and target
// state roles at the final prompt tail, where a whole-attachment preview or a
// later switch-in timestamp cannot outcompete the exact trace_query account.
// It consumes only the typed projection and remains soft reasoning context:
// no model prose is inspected and no answer value is rewritten.
func renderTraceFinalTimeRoleAuthority(set types.TraceCausalProjectionSet) string {
	var b strings.Builder
	for index, projection := range set.Projections {
		if !types.TraceCausalProjectionWindowPresent(projection.WindowStartTs, projection.WindowEndTs) {
			continue
		}
		label := strings.TrimSpace(projection.ArtifactLabel)
		if label == "" {
			label = fmt.Sprintf("trace-%d", index+1)
		}
		fmt.Fprintf(&b, "- time_role_authority artifact=`%s`; selected_query_window=`%.6f..%.6f`; selected_query_window_duration=%.3fms; attachment_extent_role=`artifact_navigation_only_not_selected_window_duration`; out_of_window_switch_in_role=`separate_event_not_selected_window_state_duration`.\n",
			traceDecisionPromptScalar(label), projection.WindowStartTs, projection.WindowEndTs, projection.WindowDurationMS())
		account := projection.TargetStateAccount
		if account == nil || strings.TrimSpace(account.Subject) == "" || account.TotalMS <= 0 {
			continue
		}
		fmt.Fprintf(&b, "  - selected_window_target_state subject=`%s`; running=%.3fms; runnable=%.3fms; sleep=%.3fms; d_state=%.3fms; io_wait=%.3fms; accounted_total=%.3fms; value_role=`target_thread_wall_clock_partition_inside_selected_query_window`; partition_members=`five_engine_lanes`.\n",
			traceDecisionPromptScalar(strings.TrimSpace(account.Subject)), account.RunningMS, account.RunnableMS,
			account.SleepMS, account.DStateMS, account.IOWaitMS, account.TotalMS)
		b.WriteString("  - sleep_state_semantics=`state_only_mechanism_unproven`; S-state proves a selected-window sleep interval, not that it was normal pacing, downstream-response waiting, lock/condition waiting, IPC, timer/event waiting, or the root cause. A separately typed relation is required for that mechanism.\n")
		b.WriteString("  - duration_selection_rule=`use_the_value_whose_typed_role_matches_the_sentence`; do not replace selected-window sleep/total with whole-attachment extent, and do not extend sleep to a later sched-in after the selected window. A post-wakeup runnable/dispatch duration requires its own typed interval.\n")
	}
	return b.String()
}

// renderTraceCausalClaimCaliberMapping keeps the report-local JSON enum and
// its evidence-status vocabulary visibly distinct. The model still chooses
// both the diagnosis and the declaration; this text only explains the exact
// wire values already projected by the typed contract.
func renderTraceCausalClaimCaliberMapping(contract *types.TraceCausalClaimContract) string {
	if contract == nil || !contract.Active() {
		return ""
	}
	allowed := make(map[types.TraceCausalClaimCaliber]bool, len(contract.Allowed))
	for _, caliber := range contract.Allowed {
		allowed[caliber] = true
	}
	var parts []string
	if allowed[types.TraceCausalClaimNoConclusion] {
		parts = append(parts, "`no_causal_conclusion` only when the lead makes no cause or candidate attribution")
	}
	if allowed[types.TraceCausalClaimBoundedWindow] {
		parts = append(parts, "`bounded_window_candidate` when the lead names or ranks selected-window candidates while keeping frame/deadline causality unproven")
	}
	if allowed[types.TraceCausalClaimTypedChain] {
		parts = append(parts, "`typed_chain_cause` only when the lead's causal attribution is bounded by a typed causal chain")
	}
	if allowed[types.TraceCausalClaimTypedFrame] {
		parts = append(parts, "`typed_frame_cause` only when typed frame/deadline causality supports that claim")
	}
	if len(parts) == 0 {
		return ""
	}
	return "- trace_causal_claim_caliber_mapping: " + strings.Join(parts, "; ") + ". Evidence-status values such as `unproven` are not JSON enum values for this field.\n"
}

// renderTraceFinalSelectedWindowAuthority prevents attachment previews and
// pre-triage navigation rows from silently widening a typed selected window.
// A producer-owned typed relation may still bind evidence across windows; the
// prompt does not inspect or reject model prose.
func renderTraceFinalSelectedWindowAuthority(set types.TraceCausalProjectionSet, frameEvidenceStatus string) string {
	var b strings.Builder
	for index, projection := range set.Projections {
		if !types.TraceCausalProjectionWindowPresent(projection.WindowStartTs, projection.WindowEndTs) {
			continue
		}
		label := strings.TrimSpace(projection.ArtifactLabel)
		if label == "" {
			label = fmt.Sprintf("trace-%d", index+1)
		}
		fmt.Fprintf(&b, "- selected_window_authority artifact=`%s`; selected_window=`%.6f..%.6f`; out_of_window_artifact_preview=`navigation_only_not_selected_window_evidence`; a preview/triage row outside this interval cannot establish selected-window state, event order, duration, frame boundary, completion, or deadline unless a separate typed relation explicitly binds it into this projection.\n",
			traceDecisionPromptScalar(label), projection.WindowStartTs, projection.WindowEndTs)
		if frameEvidenceStatus == "absent" || frameEvidenceStatus == "unavailable" {
			fmt.Fprintf(&b, "  frame_boundary_authority=`not_provided`; frame_evidence_status=`%s`; do not turn an unbound preview marker into this selected window's frame boundary or cadence explanation.\n", frameEvidenceStatus)
		}
	}
	return b.String()
}

// renderTraceFinalAggregateScaleAuthority distinguishes a measured aggregate
// value/density from an absolute severity category. It is derived only from
// typed calibration fields and remains soft reasoning guidance.
func renderTraceFinalAggregateScaleAuthority(facts []traceDecisionAggregateFact) string {
	missing := make(map[string]bool)
	for _, fact := range facts {
		hasAbsoluteLevel := false
		for _, calibration := range fact.Calibration {
			if calibration[0] == "absolute_level" && strings.TrimSpace(calibration[1]) != "" {
				hasAbsoluteLevel = true
				break
			}
		}
		if !hasAbsoluteLevel {
			key := strings.TrimSpace(fact.Signal)
			if key == "" {
				key = strings.TrimSpace(fact.Kind)
			}
			if key != "" {
				missing[key] = true
			}
		}
	}
	if len(missing) == 0 {
		return ""
	}
	keys := make([]string, 0, len(missing))
	for key := range missing {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return fmt.Sprintf("- aggregate_absolute_level_authority=`not_provided`; affected_signals=`%s`; numeric value or density may be compared only within a typed comparison/calibration scope and does not by itself mean low/medium/high or serious/not-serious. Use the neutral form `observed value/density; absolute level unavailable without calibration` when the raw aggregate is relevant; do not supply an absolute severity adjective.\n", strings.Join(keys, ","))
}

// renderTraceFinalCompactAuthorityLedger brings two high-cost relation
// decisions to the final prompt tail: whether the selected target has an exact
// typed waiter/holder row, and which single seat leads each typed fix
// direction. It does not choose a diagnosis or calculate a direction subtotal.
// Inputs are projection fields only; user/model/final prose never participates.
func renderTraceFinalCompactAuthorityLedger(set types.TraceCausalProjectionSet) string {
	if len(set.Projections) == 0 {
		return ""
	}
	var b strings.Builder
	for index, projection := range set.Projections {
		label := strings.TrimSpace(projection.ArtifactLabel)
		if label == "" {
			label = fmt.Sprintf("trace-%d", index+1)
		}
		target := ""
		if projection.TargetStateAccount != nil {
			target = strings.TrimSpace(projection.TargetStateAccount.Subject)
		}
		relations := traceFinalTargetBlockingRelations(projection, target)
		switch {
		case target == "":
			fmt.Fprintf(&b, "- compact_authority artifact=`%s`: target_direct_blocking_authority=`unavailable_without_typed_target`; direct_blocking_decision=`not_established`; wakeup_path_blocking_authority=`not_implied`. If the question asks for a direct blocker, disclose that the typed target is unavailable instead of promoting a wakeup peer or adjacent blocking row.\n", label)
		case len(relations) == 0:
			fmt.Fprintf(&b, "- compact_authority artifact=`%s`: target=`%s`; target_direct_blocking_authority=`not_provided_by_projection`; direct_blocking_decision=`not_established`; wakeup_path_blocking_authority=`not_implied`. If the question asks for a direct blocker, say that no typed direct blocker was established for this target. Describe wakeup edges as wakeup/dependency relations; do not promote a wakeup peer, IRQ peer, kernel caller, adjacent row, or another thread's blocking interval into the target's direct blocker.\n", label, target)
		default:
			for _, relation := range relations {
				fmt.Fprintf(&b, "- compact_authority artifact=`%s`: target=`%s`; target_direct_blocking_authority=`typed_waiter_holder`; direct_blocking_decision=`established_by_typed_relation`; waiter=`%s`; holder=`%s`; blocking_kind=`%s`; row_identity=`%s`.\n",
					label, target, relation.waiter, relation.holder, relation.kind, relation.rowIdentity)
			}
		}

		leaders := traceFinalFixDirectionLeaders(projection, 6)
		if len(leaders) == 0 {
			continue
		}
		sections := map[string]types.TraceAnswerDirectionSection{}
		for _, section := range tool.TraceAnswerDecisionDirectionSections(projection) {
			sections[section.Direction] = section
		}
		fmt.Fprintf(&b, "- compact_authority artifact=`%s`: fix_direction_summary_authority=`exact_typed_subtotal_when_published_else_single_leader`; cross_direction_joint_total_authority=`not_provided`. Do not sum same-direction seats merely because their labels share a direction, and never add direction values across directions without a separate typed carrier.\n", label)
		traceDecisionWriteRepairDirectionRelationRoster(&b, projection, label, 8)
		for _, node := range leaders {
			section, sectionOK := sections[strings.TrimSpace(node.FixDirection)]
			if sectionOK && section.Leader.Rank > 0 {
				node = section.Leader
			}
			key, value, ok := traceDecisionModelFacingDirection(node)
			if !ok {
				continue
			}
			fmt.Fprintf(&b, "  - %s=`%s`", key, value)
			fmt.Fprintf(&b, "; leader_rank=#%d; leader_subject=`%s`; leader_effective_attribution=%.3fms; row_identity=`%s`",
				node.Rank, strings.TrimSpace(node.Subject), node.EffectiveImpactMS, traceDecisionNodeIdentity(node))
			switch {
			case sectionOK && section.Arithmetic == types.TraceAnswerDirectionArithmeticSubtotal:
				fmt.Fprintf(&b, "; direction_subtotal_authority=`typed_pairwise_disjoint_section`; direction_subtotal=%.3fms; subtotal_member_count=%d",
					section.SubtotalMS, len(section.Members))
				if len(section.MemberRefs) == len(section.Members) {
					fmt.Fprintf(&b, "; subtotal_member_refs=`%s`", strings.Join(section.MemberRefs, ","))
				}
			case sectionOK && section.Arithmetic == types.TraceAnswerDirectionArithmeticOverlap:
				b.WriteString("; direction_subtotal_authority=`forbidden_by_typed_overlap`")
			default:
				b.WriteString("; direction_subtotal_authority=`not_provided_without_exact_fold`")
			}
			if stateKind := strings.TrimSpace(node.StateKind); stateKind != "" {
				fmt.Fprintf(&b, "; leader_state_kind=`%s`", stateKind)
			}
			if measured := traceFinalMeasuredStateOccupancy(node); measured > 0 {
				fmt.Fprintf(&b, "; leader_measured_state_occupancy=%.3fms", measured)
			}
			if node.StartTs > 0 && node.EndTs > node.StartTs {
				fmt.Fprintf(&b, "; occurrence_interval=`%.6f..%.6f`", node.StartTs, node.EndTs)
			}
			if start, end, ok := traceDecisionNodeQueryWindow(node); ok {
				role := "supporting_query_window"
				if traceDecisionSameWindow(start, end, projection.WindowStartTs, projection.WindowEndTs) {
					role = "requested_or_elected_window"
				}
				fmt.Fprintf(&b, "; query_window=`%.6f..%.6f`; window_role=`%s`", start, end, role)
			}
			traceDecisionWritePhase(&b, node)
			traceDecisionWritePriorityCandidateClaimEnvelope(&b, node)
			traceDecisionWriteNodeBlockingReasonAuthority(&b, node)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// renderTraceFinalSynthesisScope is the last, compact population/authority
// reminder before the model writes. It composes only typed projection lanes
// and frame status. It does not scan or rewrite prose and it does not choose a
// root cause; its job is to prevent a long evidence appendix from obscuring
// the already-established semantic ceiling.
func renderTraceFinalSynthesisScope(set types.TraceCausalProjectionSet, frameEvidenceStatus string) string {
	if len(set.Projections) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("- final_synthesis_scope: principal_root_cause_population=`typed_on_chain_only`; adjacent_and_background_role=`supporting_context_and_additional_investigation_only`; actual_occupancy_and_existing_rule_eliminable=`separate_decision_axes`.\n")
	seen := make(map[string]bool)
	emitted := 0
	for _, projection := range set.Projections {
		for _, node := range traceDecisionEliminableSeats(projection, 8) {
			if emitted >= 3 || !traceDecisionNodeIsPriorityInversionCandidate(node) {
				continue
			}
			key := traceDecisionNodeIdentity(node)
			if seen[key] {
				continue
			}
			seen[key] = true
			fmt.Fprintf(&b, "  - candidate_subject=`%s`; effective_attribution=%.3fms", strings.TrimSpace(node.Subject), node.EffectiveImpactMS)
			traceDecisionWritePriorityCandidateClaimEnvelope(&b, node)
			b.WriteString("\n")
			emitted++
		}
	}
	b.WriteString(renderTraceFinalLeaderMechanismCeiling(set))
	if frameEvidenceStatus == "absent" || frameEvidenceStatus == "unavailable" {
		fmt.Fprintf(&b, "  - frame_claim_scope=`selected_window_observations_only`; frame_evidence_status=`%s`; out_of_window_marker_role=`navigation_only`; frame_boundary_completion_deadline_authority=`not_provided`.\n", frameEvidenceStatus)
	}
	return b.String()
}

// renderTraceFinalLeaderMechanismCeiling gives the model one concise reminder
// at the final synthesis tail when a published direction leader is upstream
// work before the target wakeup but no typed target blocker exists. The
// detailed per-row authority remains in the ledger above; this line only keeps
// that typed distinction salient after a long prompt. An exact target
// waiter/holder relation suppresses the negative reminder so stronger typed
// authority is never erased. No request or answer prose is inspected.
func renderTraceFinalLeaderMechanismCeiling(set types.TraceCausalProjectionSet) string {
	var b strings.Builder
	for index, projection := range set.Projections {
		target := ""
		if projection.TargetStateAccount != nil {
			target = strings.TrimSpace(projection.TargetStateAccount.Subject)
		}
		if target == "" || len(traceFinalTargetBlockingRelations(projection, target)) > 0 {
			continue
		}
		label := strings.TrimSpace(projection.ArtifactLabel)
		if label == "" {
			label = fmt.Sprintf("trace-%d", index+1)
		}
		seen := map[string]bool{}
		emitted := 0
		for _, node := range traceFinalFixDirectionLeaders(projection, 6) {
			if traceDecisionNodePhase(node) != "pre_wakeup_dependency" || strings.TrimSpace(node.BlockingKind) != "" {
				continue
			}
			identity := traceDecisionNodeIdentity(node)
			if seen[identity] {
				continue
			}
			seen[identity] = true
			fmt.Fprintf(&b, "  - final_answer_mechanism_scope artifact=`%s`; subject=`%s`; target=`%s`: describe this selected leader only as on-chain work overlapping the interval before the target wakeup. No typed target-blocking relation establishes that the target waited for this work, waited for its completion, or was directly blocked by it.\n",
				traceDecisionPromptScalar(label), traceDecisionPromptScalar(strings.TrimSpace(node.Subject)), traceDecisionPromptScalar(target))
			emitted++
			if emitted >= 3 {
				break
			}
		}
	}
	return b.String()
}

// renderTraceFinalStateValueAuthority surfaces only typed rows where the
// measured state duration and published effective attribution differ. The
// compact distinction is deliberately prompt-only: it gives the model exact
// caliber without inspecting or rewriting its prose. Rows are bounded and
// deduped by typed identity so exploratory duplicates cannot flood the tail.
func renderTraceFinalStateValueAuthority(set types.TraceCausalProjectionSet) string {
	var b strings.Builder
	for index, projection := range set.Projections {
		label := strings.TrimSpace(projection.ArtifactLabel)
		if label == "" {
			label = fmt.Sprintf("trace-%d", index+1)
		}
		pool := make([]types.TraceCausalProjectionNode, 0,
			len(projection.RankedSeats)+len(projection.OnChainCauses)+len(projection.BackgroundCauses))
		pool = append(pool, projection.RankedSeats...)
		pool = append(pool, projection.OnChainCauses...)
		pool = append(pool, projection.BackgroundCauses...)
		seen := map[string]bool{}
		emitted := 0
		for _, node := range pool {
			identity := traceDecisionNodeIdentity(node)
			if seen[identity] {
				continue
			}
			seen[identity] = true
			stateKind := strings.TrimSpace(node.StateKind)
			measured := traceFinalMeasuredStateOccupancy(node)
			effective := node.EffectiveImpactMS
			if stateKind == "" || measured <= 0 || effective <= 0 || math.Abs(measured-effective) < 0.0005 {
				continue
			}
			fmt.Fprintf(&b, "- state_value_authority artifact=`%s`; subject=`%s`; state_kind=`%s`; measured_state_occupancy=%.3fms; effective_attribution=%.3fms; relation=`distinct_do_not_substitute`; row_identity=`%s`",
				traceDecisionPromptScalar(label), traceDecisionPromptScalar(strings.TrimSpace(node.Subject)),
				traceDecisionPromptScalar(stateKind), measured, effective, traceDecisionPromptScalar(identity))
			if node.StartTs > 0 && node.EndTs > node.StartTs {
				fmt.Fprintf(&b, "; occurrence_interval=`%.6f..%.6f`", node.StartTs, node.EndTs)
			}
			b.WriteString("\n")
			emitted++
			if emitted >= 8 {
				break
			}
		}
	}
	return b.String()
}

// renderTraceFinalSupplyFoldValueAuthority publishes the exact role equation
// already carried by supply_fold_deficit_ms / supply_fold_ideal_ms /
// fold_basis. It prevents a measured occupancy minus an effective attribution
// from being re-labelled as the supply deficit. Values remain engine-owned;
// diagnosis and wording remain model-owned, and no final prose is scanned.
func renderTraceFinalSupplyFoldValueAuthority(set types.TraceCausalProjectionSet) string {
	var b strings.Builder
	for index, projection := range set.Projections {
		label := strings.TrimSpace(projection.ArtifactLabel)
		if label == "" {
			label = fmt.Sprintf("trace-%d", index+1)
		}
		pool := make([]types.TraceCausalProjectionNode, 0,
			len(projection.RankedSeats)+len(projection.OnChainCauses)+len(projection.PrimaryRootCauses))
		pool = append(pool, projection.RankedSeats...)
		pool = append(pool, projection.PrimaryRootCauses...)
		pool = append(pool, projection.OnChainCauses...)
		seen := map[string]bool{}
		emitted := 0
		for _, node := range pool {
			identity := traceDecisionNodeIdentity(node)
			if seen[identity] || !node.SupplyFoldComputed || strings.TrimSpace(node.StateKind) != "running" {
				continue
			}
			seen[identity] = true
			deficit := node.SupplyFoldDeficitMS
			ideal := node.SupplyFoldIdealMS
			known := node.SupplyFoldKnownMS
			unknown := node.SupplyFoldUnknownMS
			foldedTotal := deficit + ideal
			if !traceFinalFiniteNonNegative(deficit) || !traceFinalFiniteNonNegative(ideal) ||
				!traceFinalFiniteNonNegative(known) || !traceFinalFiniteNonNegative(unknown) || foldedTotal <= 0 {
				continue
			}
			coverageTotal := known + unknown
			if coverageTotal > 0 && math.Abs(coverageTotal-foldedTotal) > 0.002 {
				continue
			}
			measured := traceFinalMeasuredStateOccupancy(node)
			effectiveRelation := "separate_typed_value"
			if node.EffectiveImpactMS > 0 && math.Abs(node.EffectiveImpactMS-deficit) < 0.0005 {
				effectiveRelation = "same_numeric_value_as_supply_deficit_for_this_seat"
			}
			measuredRelation := "separate_typed_value"
			if measured > 0 && math.Abs(measured-foldedTotal) < 0.0005 {
				measuredRelation = "same_numeric_value_as_folded_running_total"
			}
			fmt.Fprintf(&b, "- supply_fold_value_authority artifact=`%s`; subject=`%s`; state_kind=`running`; folded_running_total=%.3fms; ideal_equivalent_running=%.3fms; low_frequency_supply_deficit=%.3fms; equation=`ideal_equivalent_running + low_frequency_supply_deficit = folded_running_total`; frequency_covered_running=%.3fms; frequency_uncovered_running=%.3fms; measured_state_occupancy=%.3fms; measured_to_folded_relation=`%s`; effective_attribution=%.3fms; effective_to_deficit_relation=`%s`; occupancy_minus_effective_role=`not_a_supply_deficit_formula`; row_identity=`%s`. Use the engine-published deficit for the supply-fold opportunity. Do not derive or rename measured occupancy minus effective attribution as another supply deficit.\n",
				traceDecisionPromptScalar(label), traceDecisionPromptScalar(strings.TrimSpace(node.Subject)),
				foldedTotal, ideal, deficit, known, unknown, measured, measuredRelation,
				node.EffectiveImpactMS, effectiveRelation, traceDecisionPromptScalar(identity))
			emitted++
			if emitted >= 8 {
				break
			}
		}
	}
	return b.String()
}

func traceFinalFiniteNonNegative(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

func traceFinalMeasuredStateOccupancy(node types.TraceCausalProjectionNode) float64 {
	if node.CumulativeImpactMS > 0 {
		return node.CumulativeImpactMS
	}
	return node.ImpactMS
}

type traceFinalBlockingRelation struct {
	waiter      string
	holder      string
	kind        string
	rowIdentity string
}

func traceFinalTargetBlockingRelations(projection types.TraceCausalProjection, target string) []traceFinalBlockingRelation {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}
	pool := make([]types.TraceCausalProjectionNode, 0,
		len(projection.RankedSeats)+len(projection.PrimaryRootCauses)+len(projection.OnChainCauses)+
			len(projection.AdjacentCauses)+len(projection.BackgroundCauses)+len(projection.SupportingHops))
	pool = append(pool, projection.RankedSeats...)
	pool = append(pool, projection.PrimaryRootCauses...)
	pool = append(pool, projection.OnChainCauses...)
	pool = append(pool, projection.AdjacentCauses...)
	pool = append(pool, projection.BackgroundCauses...)
	pool = append(pool, projection.SupportingHops...)
	seen := map[string]bool{}
	out := make([]traceFinalBlockingRelation, 0, 2)
	for _, node := range pool {
		if strings.TrimSpace(node.BlockingKind) == "" ||
			(node.WithinRequestedWindow != nil && !*node.WithinRequestedWindow) {
			continue
		}
		subject := strings.TrimSpace(node.Subject)
		peer := strings.TrimSpace(node.BlockingPeer)
		waiter, holder := "", ""
		switch {
		case !node.BlockingSubjectIsHolder && subject == target:
			waiter, holder = subject, peer
		case node.BlockingSubjectIsHolder && peer == target:
			waiter, holder = peer, subject
		default:
			continue
		}
		if holder == "" {
			holder = "unresolved"
		}
		identity := traceDecisionNodeIdentity(node)
		key := waiter + "\x00" + holder + "\x00" + strings.TrimSpace(node.BlockingKind) + "\x00" + identity
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, traceFinalBlockingRelation{
			waiter: waiter, holder: holder, kind: strings.TrimSpace(node.BlockingKind), rowIdentity: identity,
		})
		if len(out) >= 3 {
			break
		}
	}
	return out
}

func traceFinalFixDirectionLeaders(projection types.TraceCausalProjection, limit int) []types.TraceCausalProjectionNode {
	seats := traceDecisionEliminableSeats(projection, 0)
	onChain := traceFinalOnChainSeatIdentities(projection)
	leaders := map[string]types.TraceCausalProjectionNode{}
	for _, node := range seats {
		direction := strings.TrimSpace(node.FixDirection)
		if direction == "" {
			continue
		}
		current, ok := leaders[direction]
		if !ok || traceFinalDirectionSeatBefore(node, current, onChain) {
			leaders[direction] = node
		}
	}
	out := make([]types.TraceCausalProjectionNode, 0, len(leaders))
	for _, node := range leaders {
		out = append(out, node)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].EffectiveImpactMS != out[j].EffectiveImpactMS {
			return out[i].EffectiveImpactMS > out[j].EffectiveImpactMS
		}
		if out[i].Rank != out[j].Rank {
			return out[i].Rank < out[j].Rank
		}
		return traceDecisionNodeIdentity(out[i]) < traceDecisionNodeIdentity(out[j])
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// traceFinalOnChainSeatIdentities mirrors the published eliminable board's
// authority boundary. Rank ordinals are local to a query board/channel, so an
// adjacent rank #1 cannot displace an on-chain rank #3 merely because its
// ordinal is smaller. Evidence identity is the join key; absence of an
// on-chain roster preserves the legacy all-seat fallback.
func traceFinalOnChainSeatIdentities(projection types.TraceCausalProjection) map[string]bool {
	out := map[string]bool{}
	if projection.PrimaryRootCause != nil {
		out[traceDecisionNodeIdentity(*projection.PrimaryRootCause)] = true
	}
	for _, nodes := range [][]types.TraceCausalProjectionNode{projection.PrimaryRootCauses, projection.OnChainCauses} {
		for _, node := range nodes {
			out[traceDecisionNodeIdentity(node)] = true
		}
	}
	return out
}

func traceFinalDirectionSeatBefore(candidate, current types.TraceCausalProjectionNode, onChain map[string]bool) bool {
	if len(onChain) > 0 {
		candidateOnChain := onChain[traceDecisionNodeIdentity(candidate)]
		currentOnChain := onChain[traceDecisionNodeIdentity(current)]
		if candidateOnChain != currentOnChain {
			return candidateOnChain
		}
	}
	if candidate.EffectiveImpactMS != current.EffectiveImpactMS {
		return candidate.EffectiveImpactMS > current.EffectiveImpactMS
	}
	if candidate.Rank != current.Rank {
		return candidate.Rank < current.Rank
	}
	return traceDecisionNodeIdentity(candidate) < traceDecisionNodeIdentity(current)
}
