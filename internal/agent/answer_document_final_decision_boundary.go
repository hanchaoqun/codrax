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
		b.WriteString("- Principal Trace summary contract: emit one principal summary block and fill its causal-strength JSON control field with exactly one value allowed by the dispatch-local tool schema. Keep the visible lead/detail within the matching natural-language scope supplied in the reader handoff below, but never repeat the field name or its machine value in visible prose. This declaration does not choose the cause. No conclusion is inferred from prose or written for you.\n")
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
	b.WriteString(renderTraceFinalRuntimeEnumerationAuthority(ctx))
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
	lang := extractAnswerDocLang(ctx)
	b.WriteString(renderTraceFinalPrincipalRankPopulation(set, lang))
	var causalClaimContract *types.TraceCausalClaimContract
	if view := types.BuildAnswerSemanticViewForAgentContext(ctx); view != nil {
		causalClaimContract = view.TraceCausalClaimContract
	}
	b.WriteString(renderTraceFinalReaderFacingLanguageHandoff(set, causalClaimContract, lang))
	b.WriteString("- relation_scope=`typed_relations_only`: preserve directed wakeup/path and typed holder/waiter or overlap relations exactly. Temporal order, adjacency, a candidate flag, or a kernel caller symbol alone does not prove synchronous blocking, lock ownership, post-wakeup preemption, or physical coupling.\n\n")
	// Keep the final prompt seam in reader language. Earlier sections retain
	// raw typed identities for schema validation and audit, but ending on those
	// control tokens makes them disproportionately easy for the model to copy
	// into visible prose. This card is derived from the same compiled projection
	// and does not inspect, classify, or rewrite any model-authored text.
	b.WriteString(renderTraceFinalReaderDecisionCards(set, causalClaimContract, lang))
	return b.String()
}

// renderTraceFinalReaderFacingLanguageHandoff maps active typed wire values to
// their reader wording at the last synthesis seam. It is prompt-only: raw
// enum fields remain intact for schema validation and audit, while the model
// retains complete ownership of the visible conclusion. No request or answer
// prose is scanned, rejected, translated, or rewritten.
func renderTraceFinalReaderFacingLanguageHandoff(set types.TraceCausalProjectionSet, contract *types.TraceCausalClaimContract, lang string) string {
	if len(set.Projections) == 0 {
		return ""
	}
	zh := strings.HasPrefix(strings.ToLower(strings.TrimSpace(lang)), "zh")
	var b strings.Builder
	b.WriteString("- reader_facing_control_metadata_policy=`json_only_never_visible`: raw JSON field names, enum literals, authority/status keys, and their snake_case values belong only in structured fields and audit carriers. Never repeat them in the model-authored lead, headings, parenthetical explanations, lists, tables, caveats, or diagrams. Express the same evidence boundary naturally; this changes no measurement, rank, causal ceiling, or conclusion. This is authoring guidance only; no model-authored prose is scanned, rejected, deleted, translated, or rewritten.\n")
	if contract != nil && contract.Active() {
		meanings := traceFinalReaderCausalScopeOptions(contract, zh)
		if len(meanings) > 0 {
			// Allowed is a choice set for one summary field, not a conjunction of
			// simultaneously true causal claims.  Joining the translations with a
			// semicolon previously made a no-conclusion option and a typed-chain
			// option read like one combined instruction.  Keep the mutually
			// exclusive meanings explicit so the model selects exactly the one
			// matching its structured caliber declaration.
			separator := " OR "
			if zh {
				separator = " 或者 "
			}
			fmt.Fprintf(&b, "  - reader_causal_scope_options=%q; selection_rule=`choose_exactly_one_matching_summary_caliber_never_combine`. The structured summary selects one allowed caliber value; visible prose states only that option's natural scope and never its control value.\n", strings.Join(meanings, separator))
		}
	}

	seen := make(map[string]bool)
	for _, projection := range set.Projections {
		pools := [][]types.TraceCausalProjectionNode{
			projection.PrimaryRootCauses,
			projection.RankedSeats,
			projection.OnChainCauses,
			projection.AdjacentCauses,
			projection.BackgroundCauses,
			projection.SemanticSpans,
		}
		for _, pool := range pools {
			for _, node := range pool {
				for _, raw := range []string{node.TypeToken, node.SemanticClass, node.Object, node.StateKind} {
					token := strings.TrimSpace(raw)
					key := strings.ToLower(token)
					if key == "" || seen[key] {
						continue
					}
					label := strings.TrimSpace(tool.TraceRootCauseTypeDisplayLabel(token, zh))
					if label == "" || strings.EqualFold(label, token) {
						continue
					}
					seen[key] = true
					fmt.Fprintf(&b, "  - permitted_reader_cause_label=%q; raw_parenthetical_forbidden=true. Use only this reader label in model-authored prose; the wire identity stays in typed/audit fields and is intentionally omitted from this reader-facing handoff.\n", label)
					if permitted, unproved := traceFinalReaderMechanismScope(token, zh); permitted != "" || unproved != "" {
						fmt.Fprintf(&b, "    permitted_reader_mechanism_scope=%q; not_proven_reader_mechanisms=%q. These are evidence boundaries for this exact typed cause family, not a system-authored diagnosis.\n", permitted, unproved)
					}
				}
			}
		}
	}
	return b.String()
}

// traceFinalReaderCausalScopeOptions is the single natural-language mapping
// for the typed causal-strength choice set. The wire enum remains available
// elsewhere for JSON validation; this helper intentionally returns no control
// value so reader-facing prompt sections cannot accidentally teach it as
// answer vocabulary.
func traceFinalReaderCausalScopeOptions(contract *types.TraceCausalClaimContract, zh bool) []string {
	if contract == nil || !contract.Active() {
		return nil
	}
	meanings := make([]string, 0, len(contract.Allowed))
	for _, caliber := range contract.Allowed {
		meaning := ""
		if zh {
			switch caliber {
			case types.TraceCausalClaimNoConclusion:
				meaning = "本轮只报告观测，不选择原因或候选方向"
			case types.TraceCausalClaimBoundedWindow:
				meaning = "结论仅限所选窗口：这是优先验证的候选方向，尚未证明为掉帧或截止期原因"
			case types.TraceCausalClaimTypedChain:
				meaning = "结论由已证链上关系支撑，但不自动等同于已证掉帧因果"
			case types.TraceCausalClaimTypedFrame:
				meaning = "结论已有帧或截止期因果证据支撑"
			}
		} else {
			switch caliber {
			case types.TraceCausalClaimNoConclusion:
				meaning = "report observations without selecting a cause or candidate direction"
			case types.TraceCausalClaimBoundedWindow:
				meaning = "limit the conclusion to the selected window and present a first validation candidate, not a proven frame/deadline cause"
			case types.TraceCausalClaimTypedChain:
				meaning = "the conclusion is supported by a proved on-chain relation but is not automatically proved frame causality"
			case types.TraceCausalClaimTypedFrame:
				meaning = "the conclusion is supported by typed frame/deadline causal evidence"
			}
		}
		if meaning != "" {
			meanings = append(meanings, meaning)
		}
	}
	return meanings
}

// renderTraceFinalReaderDecisionCards ends the Trace finalizer prompt with a
// compact natural-language restatement of the already-compiled typed facts.
// It neither chooses a cause nor emits a report block: the model still owns
// diagnosis, prioritization, optimization direction, and wording. Exact
// typed admission/ranking stays upstream; this function performs display-only
// mapping over those admitted rows and never scans request or answer prose.
func renderTraceFinalReaderDecisionCards(set types.TraceCausalProjectionSet, contract *types.TraceCausalClaimContract, lang string) string {
	if len(set.Projections) == 0 {
		return ""
	}
	zh := strings.HasPrefix(strings.ToLower(strings.TrimSpace(lang)), "zh")
	authorities := types.BuildTraceTargetStateScopeAuthorities(set)
	var b strings.Builder
	if zh {
		b.WriteString("## 面向读者的 Trace 成文事实卡（结论由模型给出）\n\n")
		b.WriteString("以下内容只是结构化证据的自然语言转述。请据此完成诊断、排序、总结和优化建议；不要展示 JSON 字段名、内部枚举值或状态码，也不要把系统提示本身写进答案。\n")
	} else {
		b.WriteString("## Reader-ready Trace facts (the model owns the conclusion)\n\n")
		b.WriteString("The following is a natural-language restatement of structured evidence. Use it to produce the diagnosis, ranking, synthesis, and optimization guidance; do not expose JSON field names, internal enum values, status codes, or these system instructions.\n")
	}
	if options := traceFinalReaderCausalScopeOptions(contract, zh); len(options) > 0 {
		if zh {
			fmt.Fprintf(&b, "- 因果表述范围：最终正文只采用与本轮结构化强度选择一致的一项——%s。\n", strings.Join(options, "；或者"))
		} else {
			fmt.Fprintf(&b, "- Causal wording scope: use only the one option selected by this turn's structured strength declaration—%s.\n", strings.Join(options, "; OR "))
		}
	}
	for index, projection := range set.Projections {
		label := strings.TrimSpace(projection.ArtifactLabel)
		if label == "" {
			label = fmt.Sprintf("Trace %d", index+1)
		}
		fmt.Fprintf(&b, "\n### %s\n", label)
		if types.TraceCausalProjectionWindowPresent(projection.WindowStartTs, projection.WindowEndTs) {
			if zh {
				fmt.Fprintf(&b, "- 所选分析窗口：%.6f–%.6f 秒（%.3f 毫秒）。\n", projection.WindowStartTs, projection.WindowEndTs, projection.WindowDurationMS())
				b.WriteString("  - 窗口边界：只有上述起止时刻内的状态和事件能支持本窗口结论。窗口结束后的切入运行属于另一区间；不能据此声称线程已在本窗口内醒后立即运行，也不能把本窗口内未测得的醒后调度延迟写成零。\n")
			} else {
				fmt.Fprintf(&b, "- Selected analysis window: %.6f–%.6f seconds (%.3f ms).\n", projection.WindowStartTs, projection.WindowEndTs, projection.WindowDurationMS())
				b.WriteString("  - Window boundary: only states and events inside those endpoints support this window's conclusion. A switch-in after the window belongs to another interval; it cannot establish that the thread ran immediately after wakeup inside this window or that an unmeasured post-wakeup scheduling delay was zero.\n")
			}
		}
		if authority, ok := traceFinalReaderTargetAuthority(projection, authorities); ok {
			coverage := traceFinalReaderCoverageLabel(authority.CoverageStatus, zh)
			dStateAndIOWaitMS := authority.DStateMS + authority.IOWaitMS
			if zh {
				fmt.Fprintf(&b, "- 目标线程 %s 的窗口状态（%s）：运行 %.3f 毫秒、可运行但未获调度 %.3f 毫秒、睡眠 %.3f 毫秒、不可中断等待 %.3f 毫秒，其中已证 IO 等待 %.3f 毫秒；合计 %.3f 毫秒。\n",
					authority.Subject, coverage, authority.RunningMS, authority.RunnableMS, authority.SleepMS, dStateAndIOWaitMS, authority.IOWaitMS, authority.TotalMS)
			} else {
				fmt.Fprintf(&b, "- Target thread %s window states (%s): running %.3f ms, runnable but not scheduled %.3f ms, sleeping %.3f ms, uninterruptible wait %.3f ms, including %.3f ms of proved IO wait; total %.3f ms.\n",
					authority.Subject, coverage, authority.RunningMS, authority.RunnableMS, authority.SleepMS, dStateAndIOWaitMS, authority.IOWaitMS, authority.TotalMS)
			}
		}

		actual := traceFinalReaderActualOccupancyCandidates(projection, 6)
		if len(actual) > 0 {
			if zh {
				b.WriteString("- 真实耗时集中（已测墙钟占用，用于发现新的优化方向）：\n")
			} else {
				b.WriteString("- Measured time concentrations (wall-clock occupancy for discovering new optimization directions):\n")
			}
			for _, node := range actual {
				cause := traceFinalReaderActualCauseLabel(node, zh)
				onChain := traceFinalReaderNodeProvedOnChain(projection, node)
				measured := traceFinalMeasuredStateOccupancy(node)
				if zh && onChain {
					fmt.Fprintf(&b, "  - %s：%s，已测 %.3f 毫秒；已证位于依赖链上，可参与主因推理", strings.TrimSpace(node.Subject), cause, measured)
				} else if zh {
					fmt.Fprintf(&b, "  - %s：%s，已测 %.3f 毫秒；未证位于依赖链上，只能作为耗时与优化线索，不能作为主因", strings.TrimSpace(node.Subject), cause, measured)
				} else if onChain {
					fmt.Fprintf(&b, "  - %s: %s, measured %.3f ms; proved on the dependency chain and eligible for primary-cause reasoning", strings.TrimSpace(node.Subject), cause, measured)
				} else {
					fmt.Fprintf(&b, "  - %s: %s, measured %.3f ms; not proved on the dependency chain, so it is an occupancy and optimization clue rather than a primary cause", strings.TrimSpace(node.Subject), cause, measured)
				}
				traceFinalReaderWriteCumulativeRole(&b, node, measured, zh)
				if zh {
					b.WriteString("。\n")
				} else {
					b.WriteString(".\n")
				}
			}
		}

		seats := traceDecisionEliminableSeats(projection, 6)
		if len(seats) > 0 {
			if zh {
				b.WriteString("- 按现有规则可消除的影响（用于修复优先级，不等同于实测等待时长）：\n")
			} else {
				b.WriteString("- Impact eliminable under existing rules (for repair priority, not automatically a measured wait duration):\n")
			}
			for _, node := range seats {
				cause := traceFinalReaderCauseLabel(node, zh)
				measured := traceFinalMeasuredStateOccupancy(node)
				if zh {
					fmt.Fprintf(&b, "  - 第 %d 位，%s：%s；可消除影响 %.3f 毫秒", node.Rank, strings.TrimSpace(node.Subject), cause, node.EffectiveImpactMS)
				} else {
					fmt.Fprintf(&b, "  - Rank %d, %s: %s; eliminable impact %.3f ms", node.Rank, strings.TrimSpace(node.Subject), cause, node.EffectiveImpactMS)
				}
				if measured > 0 && math.Abs(measured-node.EffectiveImpactMS) > 0.0005 {
					if zh {
						fmt.Fprintf(&b, "，对应已测状态占用 %.3f 毫秒", measured)
					} else {
						fmt.Fprintf(&b, ", with %.3f ms measured state occupancy", measured)
					}
				}
				traceFinalReaderWriteCumulativeRole(&b, node, measured, zh)
				if zh {
					b.WriteString("。\n")
				} else {
					b.WriteString(".\n")
				}
				if permitted, unproved := traceFinalReaderMechanismScope(traceDecisionEliminableSeatKind(node), zh); permitted != "" || unproved != "" {
					if zh {
						fmt.Fprintf(&b, "    证据允许的表述：%s；尚未证明：%s。\n", permitted, unproved)
					} else {
						fmt.Fprintf(&b, "    Supported wording: %s; not proved: %s.\n", permitted, unproved)
					}
				}
			}
		}

		if contexts := traceDecisionNonCausalContextRows(projection, 4); len(contexts) > 0 {
			if zh {
				b.WriteString("- 背景与邻近信息（只能支撑额外排查方向，不得升级为链上主因或参与根因序数）：\n")
			} else {
				b.WriteString("- Background and adjacent context (supporting follow-up only; never promote it to an on-chain primary cause or give it a root-cause ordinal):\n")
			}
			for _, row := range contexts {
				cause := traceFinalReaderContextLabel(row.node, zh)
				if zh {
					fmt.Fprintf(&b, "  - %s：%s；观测值 %.3f，沿用证据原口径。\n", strings.TrimSpace(row.node.Subject), cause, row.node.ImpactMS)
				} else {
					fmt.Fprintf(&b, "  - %s: %s; observed value %.3f in the evidence's original caliber.\n", strings.TrimSpace(row.node.Subject), cause, row.node.ImpactMS)
				}
			}
		}

		if spans := traceDecisionBusinessSpanCandidates(projection.BusinessSpanMentions, 4); len(spans) > 0 {
			if zh {
				b.WriteString("- 业务线索（用于解释链上工作并提出业务修向，不凭名称自行补造因果）：\n")
			} else {
				b.WriteString("- Business clues (use them to explain on-chain work and propose business-facing fixes, without inventing causality from names):\n")
			}
			for _, span := range spans {
				if zh {
					fmt.Fprintf(&b, "  - %s 的 %s：%d 次，合计 %.3f 毫秒，单次最大 %.3f 毫秒。\n", span.Subject, span.Name, span.Count, span.TotalMS, span.MaxMS)
				} else {
					fmt.Fprintf(&b, "  - %s / %s: %d occurrences, %.3f ms total, %.3f ms maximum single occurrence.\n", span.Subject, span.Name, span.Count, span.TotalMS, span.MaxMS)
				}
			}
		}
	}
	if zh {
		b.WriteString("\n请基于以上事实自行给出结论：同时回答真实耗时集中与按现有规则可消除影响两个维度；链外信息只作背景；证据不足处明确限定，不得由系统字段名替代面向用户的解释。\n\n")
	} else {
		b.WriteString("\nNow provide your own conclusion from these facts: address both measured time concentration and impact eliminable under existing rules; keep off-chain information as context; qualify evidence gaps; and use reader language rather than system field names.\n\n")
	}
	return b.String()
}

// traceFinalReaderWriteCumulativeRole keeps the projection's three duration
// roles adjacent in the final reader card. CumulativeImpactMS is a chain
// account (the node plus any admitted downstream/sub-chain contribution), not
// a state-specific occupancy. It must therefore never replace ImpactMS or a
// more specific per-state split merely because it is numerically larger.
// This is display-only typed guidance: no answer prose is inspected or
// rewritten and no ranking/value is changed.
func traceFinalReaderWriteCumulativeRole(b *strings.Builder, node types.TraceCausalProjectionNode, measured float64, zh bool) {
	if b == nil || node.CumulativeImpactMS <= 0 || math.Abs(node.CumulativeImpactMS-measured) <= 0.0005 {
		return
	}
	if zh {
		fmt.Fprintf(b, "；另有链上累计 %.3f 毫秒，这是不同的链路累计口径，不能改称为该状态的实测占用", node.CumulativeImpactMS)
		return
	}
	fmt.Fprintf(b, "; the separate on-chain cumulative account is %.3f ms and must not be renamed as measured occupancy of this state", node.CumulativeImpactMS)
}

func traceFinalReaderTargetAuthority(projection types.TraceCausalProjection, authorities []types.TraceTargetStateScopeAuthority) (types.TraceTargetStateScopeAuthority, bool) {
	if projection.TargetStateAccount == nil {
		return types.TraceTargetStateScopeAuthority{}, false
	}
	for _, authority := range authorities {
		if authority.Subject == strings.TrimSpace(projection.TargetStateAccount.Subject) &&
			authority.EvidenceID == strings.TrimSpace(projection.TargetStateAccount.EvidenceID) &&
			(authority.ArtifactLabel == strings.TrimSpace(projection.ArtifactLabel) || authority.ArtifactLabel == "" || projection.ArtifactLabel == "") {
			return authority, true
		}
	}
	return types.TraceTargetStateScopeAuthority{}, false
}

func traceFinalReaderCoverageLabel(status string, zh bool) string {
	switch status {
	case "complete":
		if zh {
			return "覆盖完整"
		}
		return "complete coverage"
	case "partial_unaccounted":
		if zh {
			return "部分覆盖，仍有未计入时间"
		}
		return "partial coverage with unaccounted time"
	default:
		if zh {
			return "窗口覆盖范围未知"
		}
		return "window coverage unknown"
	}
}

func traceFinalReaderCauseLabel(node types.TraceCausalProjectionNode, zh bool) string {
	label := strings.TrimSpace(tool.TraceRootCauseTypeDisplayLabel(traceDecisionEliminableSeatKind(node), zh))
	if label != "" {
		return label
	}
	if label := traceFinalReaderStateLabel(node.StateKind, zh); label != "" {
		return label
	}
	if label := traceFinalReaderStateLabel(traceDecisionEliminableSeatKind(node), zh); label != "" {
		return label
	}
	if zh {
		return "已测链上候选"
	}
	return "measured on-chain candidate"
}

func traceFinalReaderActualCauseLabel(node types.TraceCausalProjectionNode, zh bool) string {
	if label := strings.TrimSpace(tool.TraceRootCauseTypeDisplayLabel(node.SemanticClass, zh)); label != "" {
		return label
	}
	if label := traceFinalReaderStateLabel(node.StateKind, zh); label != "" {
		return label
	}
	if label := strings.TrimSpace(tool.TraceRootCauseTypeDisplayLabel(node.StateKind, zh)); label != "" {
		return label
	}
	if name := strings.TrimSpace(node.SpanName); name != "" {
		if zh {
			return "业务或运行时工作 “" + name + "”"
		}
		return "business or runtime work “" + name + "”"
	}
	if zh {
		return "已测链上耗时"
	}
	return "measured on-chain time"
}

// traceFinalReaderActualOccupancyCandidates removes the focused thread's
// unpriced self-state symptom from the reader-ready root-cause population.
// Some wakeup causal-impact rows live in OnChainCauses even though they are
// the target's rank-0/effective-0 sleep symptom and therefore do not satisfy
// TraceCausalProjectionNode.IsTargetSelfStateRow's narrower rank-row shape.
// Positive ranked target runnable/D/IO/compute-supply rows and typed semantic
// work remain eligible. The decision consumes only typed target identity,
// numeric authority, and semantic/span carriers.
func traceFinalReaderActualOccupancyCandidates(projection types.TraceCausalProjection, limit int) []types.TraceCausalProjectionNode {
	pool := traceDecisionActualOccupancyCandidates(projection, 0)
	out := make([]types.TraceCausalProjectionNode, 0, len(pool))
	for _, node := range pool {
		if traceFinalReaderTargetSelfStateSymptom(projection, node) {
			continue
		}
		out = append(out, node)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func traceFinalReaderTargetSelfStateSymptom(projection types.TraceCausalProjection, node types.TraceCausalProjectionNode) bool {
	if node.IsTargetSelfStateRow() {
		return true
	}
	account := projection.TargetStateAccount
	if account == nil || strings.TrimSpace(account.Subject) == "" ||
		strings.TrimSpace(node.Subject) != strings.TrimSpace(account.Subject) {
		return false
	}
	if node.Rank > 0 || node.EffectiveImpactMS > 0 {
		return false
	}
	if strings.TrimSpace(node.SemanticClass) != "" || strings.TrimSpace(node.SpanName) != "" || strings.TrimSpace(node.SpanKind) != "" {
		return false
	}
	return strings.TrimSpace(node.StateKind) != "" || strings.TrimSpace(node.Object) != ""
}

func traceFinalReaderContextLabel(node types.TraceCausalProjectionNode, zh bool) string {
	label := strings.TrimSpace(tool.TraceRootCauseTypeDisplayLabel(traceDecisionEliminableSeatKind(node), zh))
	if label != "" {
		return label
	}
	if label := traceFinalReaderStateLabel(node.StateKind, zh); label != "" {
		return label
	}
	if zh {
		return "资源或活动背景"
	}
	return "resource or activity context"
}

func traceFinalReaderNodeProvedOnChain(projection types.TraceCausalProjection, node types.TraceCausalProjectionNode) bool {
	if strings.TrimSpace(node.ChainRelevance) == "on_chain" {
		return true
	}
	identity := traceDecisionNodeIdentity(node)
	for _, candidate := range projection.OnChainCauses {
		if traceDecisionNodeIdentity(candidate) == identity {
			return true
		}
	}
	for _, candidate := range projection.PrimaryRootCauses {
		if traceDecisionNodeIdentity(candidate) == identity {
			return true
		}
	}
	return false
}

func traceFinalReaderStateLabel(token string, zh bool) string {
	switch strings.ToLower(strings.TrimSpace(token)) {
	case "running", "fragmented_running":
		if zh {
			return "运行耗时"
		}
		return "running time"
	case "runnable", "runnable_wait", "fragmented_runnable_wait", "scheduler_latency":
		if zh {
			return "调度延迟"
		}
		return "scheduling latency"
	case "s_sleep", "sleep", "sleep_wait", "fragmented_sleep_wait":
		if zh {
			return "睡眠等待"
		}
		return "sleep wait"
	case "d", "d_sleep", "d_state", "d_state_or_io_wait", "fragmented_d_state_or_io_wait":
		if zh {
			return "不可中断等待"
		}
		return "uninterruptible wait"
	case "io_wait", "io_latency":
		if zh {
			return "IO 等待"
		}
		return "IO wait"
	default:
		return ""
	}
}

// traceFinalReaderMechanismScope keeps the final reader-facing cause label and
// its mechanism ceiling adjacent. The switch consumes exact typed tokens only:
// it never classifies request/answer prose, chooses a cause, or rewrites the
// model's conclusion. Unmapped families retain the existing generic typed
// relation boundary.
func traceFinalReaderMechanismScope(token string, zh bool) (permitted, unproved string) {
	switch strings.ToLower(strings.TrimSpace(token)) {
	case "priority_inversion_candidate", "priority_inversion_runnable_wait":
		if zh {
			return "只陈述证据行实际计入的链上低优先级依赖方贡献：被唤醒后处于 runnable 的调度等待，和/或 running 期间明确计入的算力供给提升空间；二者不得互换", "候选标签或唤醒先后本身不证明该线程持有 CPU、锁或资源，不证明目标当时已 runnable 却被抢占，也不证明同步阻塞、等待其工作完成或直接因果"
		}
		return "state only the contribution actually carried by the evidence row for the lower-priority on-chain dependency: runnable scheduling delay after wakeup and/or an explicitly accounted compute-supply opportunity while running; do not interchange them", "the candidate label or wakeup order alone does not prove CPU occupation, lock/resource ownership, target post-wakeup preemption, synchronous blocking, waiting for work completion, or direct causality"
	case "runnable_wait", "fragmented_runnable_wait", "scheduler_latency":
		if zh {
			return "陈述线程已经可运行但尚未获得调度的已测延迟，以及证据行明确给出的同核竞争或调度供给信息", "runnable 不等于正在 CPU 上执行；仅凭该行不证明锁、IO、同步阻塞或具体竞争者"
		}
		return "state the measured delay while the thread was ready to run but not scheduled, plus only same-CPU contention or scheduling-supply facts explicitly carried by the evidence row", "runnable is not CPU execution; this row alone does not prove a lock, IO, synchronous blocking, or a particular competitor"
	case "compute_supply", "low_frequency", "cpu_frequency_limit", "running", "fragmented_running":
		if zh {
			return "陈述 running 区间内证据行测得的算力供给、频率或可折算提升空间", "算力供给项不等于 runnable 调度等待，也不证明锁、直接阻塞或线程持有 CPU 之外的业务依赖"
		}
		return "state the compute capacity, frequency, or explicitly quantified improvement opportunity measured while running", "a compute-supply row is not runnable scheduling delay and does not prove a lock, direct blocking, or a business dependency beyond the measured execution"
	case "d_state_or_io_wait", "io_wait", "io_latency", "fragmented_d_state_or_io_wait", "io_burst_episode":
		if zh {
			return "只陈述证据行明确覆盖的 D-state、iowait 或 IO 发起到完成区间，并保持状态占用与设备延迟口径分离", "裸状态或时长不证明具体设备、文件、请求、资源持有者或唤醒者；这些身份和关联必须由独立证据给出"
		}
		return "state only the D-state, iowait, or IO issue-to-completion interval explicitly covered by the evidence row, keeping state occupancy separate from device latency", "a bare state or duration does not prove a particular device, file, request, resource holder, or waker; those identities and joins require their own structured evidence"
	case "sleep_wait", "fragmented_sleep_wait", "pacing_idle", "periodic_idle":
		if zh {
			return "陈述已测 sleep 区间及精确唤醒记录明确给出的前后关系", "sleep 本身不证明等待谁、等待工作完成、属于正常协作，或构成根因；必须另有结构化依赖证据"
		}
		return "state the measured sleep interval and only the before/after relation provided by an exact structured wakeup edge", "sleep alone does not prove whom the thread awaited, work completion, normal coordination, or root-cause status; that requires separate structured dependency evidence"
	case "jit_compile", "class_verification", "shader_compile", "runtime_compile", "texture_upload", "gc_pause", "trace_span":
		if zh {
			return "陈述 trace 中已观测到的链上语义工作及其已测占用，并据业务含义提出验证或优化方向", "语义标签或时间邻近本身不证明该工作完成后才唤醒下游，也不自动证明掉帧、截止期或同步阻塞因果"
		}
		return "state the observed on-chain semantic work and its measured occupancy, then use its business meaning to propose a validation or optimization direction", "the semantic label or temporal proximity alone does not prove downstream wakeup on completion, dropped-frame/deadline causality, or synchronous blocking"
	case "binder_wait", "blocking_span":
		if zh {
			return "只陈述结构化证据中实际存在的阻塞、对端、持有者/等待者或请求语义", "类别标签本身不补齐缺失的对端、锁、持有者/等待者、同步性、回复或直接因果"
		}
		return "state only the blocking, peer, holder/waiter, or request-semantics facts actually present in structured evidence", "the category label alone does not fill a missing peer, lock, holder/waiter relation, synchronicity, reply, or direct causality"
	case "cpu_pressure", "io_pressure", "supply_pressure", "irq_burst", "irq_activity", "ipi_activity", "workqueue_activity", "dma_fence_activity", "page_cache_churn", "block_io_by_inode", "file_io_hot_inode", "state_churn":
		if zh {
			return "按已标注的链路位置使用：只有明确在链上的已测席位可参与根因；其余压力、活动和聚合只作背景与额外排查方向", "窗口邻近、总量较大或代表线程重合都不把背景聚合升级为链上根因，也不证明它阻塞了目标"
		}
		return "follow the declared chain position: only an explicitly on-chain measured seat may enter root-cause reasoning; other pressure, activity, and aggregate rows remain background or follow-up directions", "window proximity, a large aggregate, or a shared representative thread does not promote background context into an on-chain cause or prove that it blocked the target"
	default:
		return "", ""
	}
}

// renderTraceFinalPrincipalRankPopulation repeats the exact selected-window
// ordinal population at the final synthesis seam. Earlier rank boards remain
// losslessly available for investigation, but a row measured in a different
// query window is contextual evidence for this answer and cannot retain its
// local board ordinal in the elected-window conclusion. This consumes only
// compiled typed window/rank fields; it neither inspects nor rewrites prose.
func renderTraceFinalPrincipalRankPopulation(set types.TraceCausalProjectionSet, lang string) string {
	zh := strings.HasPrefix(strings.ToLower(strings.TrimSpace(lang)), "zh")
	var b strings.Builder
	for index, projection := range set.Projections {
		if !types.TraceCausalProjectionPrincipalWindowAuthoritative(projection) {
			continue
		}
		label := strings.TrimSpace(projection.ArtifactLabel)
		if label == "" {
			label = fmt.Sprintf("trace-%d", index+1)
		}
		principal := types.TraceAnswerDecisionEliminableSeats(projection, 8)
		excluded := traceFinalDifferentWindowRankedSeats(projection, 8)
		if len(principal) == 0 && len(excluded) == 0 {
			continue
		}
		allowedOrdinals := make([]string, 0, len(principal))
		for _, node := range principal {
			allowedOrdinals = append(allowedOrdinals, fmt.Sprintf("#%d", node.Rank))
		}
		fmt.Fprintf(&b, "- selected_window_reader_rank_roster artifact=`%s`; selected_window=`%.6f..%.6f`; ranked_row_count=`%d`; allowed_visible_ordinals=`%s`; every_other_row=`unranked_context_or_symptom`. The model owns the conclusion, but only the rows below may receive these ordinals in the selected-window answer.\n",
			traceDecisionPromptScalar(label), projection.WindowStartTs, projection.WindowEndTs,
			len(principal), strings.Join(allowedOrdinals, ","))
		for _, node := range principal {
			causeLabel := strings.TrimSpace(tool.TraceRootCauseTypeDisplayLabel(traceDecisionEliminableSeatKind(node), zh))
			if causeLabel == "" {
				if zh {
					causeLabel = "已测链上候选"
				} else {
					causeLabel = "measured on-chain candidate"
				}
			}
			fmt.Fprintf(&b, "  - reader_rank=`#%d`; subject=`%s`; reader_cause_label=%q; effective_attribution=%.3fms",
				node.Rank, traceDecisionPromptScalar(strings.TrimSpace(node.Subject)), causeLabel, node.EffectiveImpactMS)
			if start, end, ok := traceDecisionNodeQueryWindow(node); ok {
				fmt.Fprintf(&b, "; query_window=`%.6f..%.6f`", start, end)
			}
			b.WriteByte('\n')
		}
		for _, node := range excluded {
			fmt.Fprintf(&b, "  - unranked_context_row subject=`%s`; effective_attribution=%.3fms; selected_window_role=`supporting_context_only`; selected_window_ordinal_permission=`forbidden`",
				traceDecisionPromptScalar(strings.TrimSpace(node.Subject)), node.EffectiveImpactMS)
			if start, end, ok := traceDecisionNodeQueryWindow(node); ok {
				fmt.Fprintf(&b, "; row_query_window=`%.6f..%.6f`", start, end)
			}
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func traceFinalDifferentWindowRankedSeats(projection types.TraceCausalProjection, limit int) []types.TraceCausalProjectionNode {
	if !types.TraceCausalProjectionPrincipalWindowAuthoritative(projection) {
		return nil
	}
	seen := map[string]bool{}
	out := make([]types.TraceCausalProjectionNode, 0)
	for _, node := range projection.RankedSeats {
		if node.Rank <= 0 || node.EffectiveImpactMS <= 0 ||
			types.TraceCausalProjectionNodeMatchesPrincipalWindow(node, projection.WindowStartTs, projection.WindowEndTs) {
			continue
		}
		identity := traceDecisionNodeIdentity(node)
		if seen[identity] {
			continue
		}
		seen[identity] = true
		out = append(out, node)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Rank != out[j].Rank {
			return out[i].Rank < out[j].Rank
		}
		if out[i].EffectiveImpactMS != out[j].EffectiveImpactMS {
			return out[i].EffectiveImpactMS > out[j].EffectiveImpactMS
		}
		return traceDecisionNodeIdentity(out[i]) < traceDecisionNodeIdentity(out[j])
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
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

// renderAnswerDocBoundedRuntimeFinalReaderHandoff ends a finite runtime
// question on reader language rather than on the raw observation ledger. The
// analyzer's typed RuntimeQuestionProfile selects the dimensions and the hard
// deterministic ledger supplies every value. Raw predicates/enums remain in
// the preceding audit carriers for validation, but this final prompt seam does
// not repeat them. It never scans request/model/final prose, never creates an
// answer block, and never chooses or rewrites the model-owned verdict.
func renderAnswerDocBoundedRuntimeFinalReaderHandoff(ctx *types.AgentContext) string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return ""
	}
	rm := &ctx.AnalysisIR.RequestModel
	profile := rm.RuntimeQuestionProfile
	if profile == nil || !profile.CarriesBoundedFactFamilies() {
		return ""
	}
	ledger := answerDocObservationLedger(ctx)
	if ledger.Empty() {
		return ""
	}
	zh := strings.HasPrefix(strings.ToLower(strings.TrimSpace(extractAnswerDocLang(ctx))), "zh")

	requestedLabels := make([]string, 0, len(profile.FactFamilies))
	seenLabels := make(map[string]bool, len(profile.FactFamilies))
	for _, family := range profile.FactFamilies {
		label := answerDocBoundedRuntimeFactFamilyReaderLabel(family, zh)
		if label == "" || seenLabels[label] {
			continue
		}
		seenLabels[label] = true
		requestedLabels = append(requestedLabels, label)
	}

	var b strings.Builder
	if zh {
		b.WriteString("## 有限窗口查询的读者事实卡（结论由模型给出）\n\n")
		b.WriteString("- 这是成文前的最后一张自然语言事实卡。此前结构化行中的字段名、枚举值、状态码和机器键值只用于校验，不得出现在面向客户的正文、标题、表格、括注或图中；精确数值、窗口、覆盖边界和证据强度保持不变。系统不检查或修改模型正文，也不代替模型给结论。\n")
		if len(requestedLabels) > 0 {
			fmt.Fprintf(&b, "- 本次请求的可见事实维度：%s。\n", strings.Join(requestedLabels, "、"))
		}
	} else {
		b.WriteString("## Reader-ready finite-window facts (the model owns the conclusion)\n\n")
		b.WriteString("- This is the final natural-language fact card before authoring. Field names, enum values, status codes, and machine key/value pairs in earlier structured rows are validation metadata and must not appear in customer prose, headings, tables, parentheses, or diagrams. Preserve exact values, windows, coverage boundaries, and evidence strength. The system neither checks nor rewrites model prose and does not choose the conclusion.\n")
		if len(requestedLabels) > 0 {
			fmt.Fprintf(&b, "- Requested visible fact dimensions: %s.\n", strings.Join(requestedLabels, ", "))
		}
	}

	stateAuthorities := types.BuildTraceTargetStateScopeAuthoritiesFromLedger(ledger)
	if profile.RequestsFactFamily(types.RuntimeQuestionFactTargetSchedulerState) {
		for _, authority := range stateAuthorities {
			coverage := traceFinalReaderCoverageLabel(authority.CoverageStatus, zh)
			if zh {
				fmt.Fprintf(&b, "- 目标线程 %s 在 %.6f–%.6f 秒窗口内的状态分布（%s）：运行 %.3f 毫秒、可运行但尚未获调度 %.3f 毫秒、可中断睡眠 %.3f 毫秒、不可中断等待 %.3f 毫秒、其中由调度器标记的 IO 等待 %.3f 毫秒；合计 %.3f 毫秒。\n",
					authority.Subject, authority.WindowStartTs, authority.WindowEndTs, coverage,
					authority.RunningMS, authority.RunnableMS, authority.SleepMS,
					authority.DStateMS, authority.IOWaitMS, authority.TotalMS)
			} else {
				fmt.Fprintf(&b, "- Target thread %s in %.6f–%.6f seconds (%s): running %.3f ms, runnable but not yet scheduled %.3f ms, interruptible sleep %.3f ms, uninterruptible wait %.3f ms, including %.3f ms marked by the scheduler as IO wait; total %.3f ms.\n",
					authority.Subject, authority.WindowStartTs, authority.WindowEndTs, coverage,
					authority.RunningMS, authority.RunnableMS, authority.SleepMS,
					authority.DStateMS, authority.IOWaitMS, authority.TotalMS)
			}
			if authority.DStateMS == 0 && authority.IOWaitMS == 0 && authority.SleepIOWaitMS == 0 {
				if zh {
					b.WriteString("  - 窄口径结论：该目标与窗口内没有匹配到由调度器标记的 D 状态或 IO 等待；这不等于没有普通睡眠、没有 IO 活动，也不等于没有由其他证据证明的等待或阻塞。\n")
				} else {
					b.WriteString("  - Narrow finding: no scheduler-marked D-state or IO-wait occurrence matched this target and window. This does not mean there was no ordinary sleep, IO activity, or wait/blocking proved by another evidence family.\n")
				}
			}
			b.WriteString(renderAnswerDocBoundedRuntimeCompletionClosedReaderFact(ledger, rm, authority, zh))
		}
	}

	waitRequested := false
	for _, record := range ledger.Records {
		if strings.TrimSpace(record.Predicate) != "target_window_wait_occurrences" {
			continue
		}
		for _, family := range types.RuntimeObservationRecordFactFamilies(record) {
			if profile.RequestsFactFamily(family) {
				waitRequested = true
				break
			}
		}
	}
	if waitRequested {
		for _, authority := range types.BuildTargetWaitOccurrenceAuthorities(ledger, rm) {
			if zh {
				fmt.Fprintf(&b, "- %s 的调度器标记等待清单已完整覆盖所选窗口：共 %d 次，合计 %.3f 毫秒。该清单只包含 D 状态、明确的 IO 等待，以及带有 IO 等待标记的 S 状态。\n", authority.Subject, authority.Count, authority.SumMS)
			} else {
				fmt.Fprintf(&b, "- The scheduler-marked wait roster for %s completely covers the selected window: %d occurrence(s), totaling %.3f ms. It includes only D-state, explicit IO wait, and S-state carrying an IO-wait marker.\n", authority.Subject, authority.Count, authority.SumMS)
			}
		}
	}

	if profile.RequestsFactFamily(types.RuntimeQuestionFactFrequencyResidency) {
		for _, witness := range answerDocRuntimeTraceGuidanceView(ctx).FrequencyLimitWitnesses {
			if zh {
				fmt.Fprintf(&b, "- CPU %d 在 %.6f–%.6f 秒窗口内出现 %d 条频率策略上限记录，策略范围为 %d–%d kHz。这只证明该 CPU 的策略上限在窗口内存在；是否限制了目标线程，仍需同一 CPU 上目标运行切片与策略的重叠或其他目标绑定证据。\n",
					witness.CPU, witness.WindowStartTs, witness.WindowEndTs, witness.LimitRowCount,
					witness.MinFrequencyKHz, witness.MaxFrequencyKHz)
			} else {
				fmt.Fprintf(&b, "- CPU %d has %d frequency-policy limit record(s) in %.6f–%.6f seconds, with a policy range of %d–%d kHz. This proves only that the CPU policy ceiling existed in the window; showing that it constrained the target still requires same-CPU target-slice overlap or another target-binding witness.\n",
					witness.CPU, witness.LimitRowCount, witness.WindowStartTs, witness.WindowEndTs,
					witness.MinFrequencyKHz, witness.MaxFrequencyKHz)
			}
		}
	}

	if zh {
		b.WriteString("- 其余请求事实沿用此前结构化事实行中的精确值与区间，但正文只使用本卡中的读者维度名称和自然语言边界。\n\n")
	} else {
		b.WriteString("- For any other requested fact, preserve the exact value and interval from the preceding structured fact row, but use only the reader dimension names and natural-language boundaries from this card in visible prose.\n\n")
	}
	return b.String()
}

func answerDocBoundedRuntimeFactFamilyReaderLabel(family types.RuntimeQuestionFactFamily, zh bool) string {
	if zh {
		switch family {
		case types.RuntimeQuestionFactTargetSchedulerState:
			return "目标线程状态分布"
		case types.RuntimeQuestionFactTargetWaitOccurrences:
			return "目标线程的调度器标记等待清单"
		case types.RuntimeQuestionFactRecordedReason:
			return "已记录的内核或工具原因"
		case types.RuntimeQuestionFactOccurrenceTime:
			return "事件发生时间"
		case types.RuntimeQuestionFactCountOrDuration:
			return "次数或持续时间"
		case types.RuntimeQuestionFactRelationPeer:
			return "关系对端"
		case types.RuntimeQuestionFactTransactionID:
			return "事务标识"
		case types.RuntimeQuestionFactDirectWaker:
			return "直接唤醒方"
		case types.RuntimeQuestionFactIOLatency:
			return "IO 请求时延与已证线程等待（分尺呈现）"
		case types.RuntimeQuestionFactResourcePressure:
			return "资源压力（不与墙钟时长相加）"
		case types.RuntimeQuestionFactFrequencyResidency:
			return "CPU 频率驻留与策略上限"
		case types.RuntimeQuestionFactOtherObservedValue:
			return "其他已观测数值"
		}
		return ""
	}
	switch family {
	case types.RuntimeQuestionFactTargetSchedulerState:
		return "target-thread state distribution"
	case types.RuntimeQuestionFactTargetWaitOccurrences:
		return "target scheduler-marked wait roster"
	case types.RuntimeQuestionFactRecordedReason:
		return "recorded kernel/tool reason"
	case types.RuntimeQuestionFactOccurrenceTime:
		return "occurrence time"
	case types.RuntimeQuestionFactCountOrDuration:
		return "count or duration"
	case types.RuntimeQuestionFactRelationPeer:
		return "relation peer"
	case types.RuntimeQuestionFactTransactionID:
		return "transaction identifier"
	case types.RuntimeQuestionFactDirectWaker:
		return "direct waker"
	case types.RuntimeQuestionFactIOLatency:
		return "IO request latency and proved thread wait (separate rulers)"
	case types.RuntimeQuestionFactResourcePressure:
		return "resource pressure (not additive with wall clock)"
	case types.RuntimeQuestionFactFrequencyResidency:
		return "CPU frequency residency and policy ceiling"
	case types.RuntimeQuestionFactOtherObservedValue:
		return "other observed value"
	}
	return ""
}

func renderAnswerDocBoundedRuntimeCompletionClosedReaderFact(
	ledger types.ObservationLedger,
	rm *types.RequestModel,
	state types.TraceTargetStateScopeAuthority,
	zh bool,
) string {
	wantWindow := fmt.Sprintf("%.6f..%.6f", state.WindowStartTs, state.WindowEndTs)
	for _, authority := range types.BuildTraceBlockingWallClockAuthorities(ledger, rm) {
		if authority.Type != "block_io_completion_closed_issuer_wait" ||
			!strings.EqualFold(strings.TrimSpace(authority.Subject), strings.TrimSpace(state.Subject)) ||
			strings.TrimSpace(authority.SelectedWindow) != wantWindow {
			continue
		}
		coverage := traceFinalReaderCoverageLabel(authority.CoverageStatus, zh)
		if zh {
			return fmt.Sprintf("  - 独立 IO 完成闭合口径（%s）：已证目标线程等待 %d 次，区间并集 %.3f 毫秒。它与调度器标记的 D/IO 等待是两把尺，不能相加或互相否定。\n", coverage, len(authority.Occurrences), authority.ObservedMS)
		}
		return fmt.Sprintf("  - Separate completion-closed IO ruler (%s): %d proved target-thread wait occurrence(s), interval union %.3f ms. This and scheduler-marked D/IO wait are different rulers and must neither be added nor used to negate one another.\n", coverage, len(authority.Occurrences), authority.ObservedMS)
	}
	if zh {
		return "  - 本次状态分布没有评估由 IO 完成事件闭合的 S 状态等待；该口径缺席表示未评估，不是测得为零。\n"
	}
	return "  - This state distribution did not assess S-state waits closed by IO completion events. An absent ruler means not assessed, not measured zero.\n"
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
			measuredIOWaitAuthority := "published_nonzero_state_account"
			if account.IOWaitMS == 0 {
				measuredIOWaitAuthority = "exact_zero_for_this_state_account"
			}
			fmt.Fprintf(&b, "- blocked_reason_state_relation subject=`%s`; selected_window=`%.6f..%.6f`; scheduler_state_caliber=`sched_switch_interval_wall_clock`; d_state=%.3fms; io_wait=%.3fms; measured_io_wait_authority=`%s`; zero_io_wait_inference_scope=`accounting_bucket_only`; underlying_storage_or_dependency_mechanism_exclusion_permission=`forbidden_without_separate_typed_evidence`; blocked_reason_records=%d; blocked_reason_census=`%s`; blocked_reason_caliber=`kernel_record_count_and_vendor_reported_delay_sum`; blocked_reason_caller_identity_role=`kernel_call_site_symbol_only`; waited_object_identity=`not_provided_by_census_alone`; resource_holder_identity=`not_provided_by_census_alone`; subsystem_mechanism=`not_provided_by_census_alone`; caller_to_wait_cause_relation=`unproven_without_separate_typed_identity_or_dependency`; relation=`unjoined_distinct_observation_domains`; record_to_state_occurrence_mapping=`not_provided`; count_or_delay_difference_interpretation=`forbidden`; arithmetic_recomposition=`forbidden`; fix_direction_caliber=`coarse_validation_family_only`; specific_io_mechanism_authority=`not_provided_by_fix_direction`; identifier_lexical_semantics_authority=`none`; neutral_answer_shape=`measured_state_wall_clock_plus_kernel_call_site_plus_unknown_object_boundary`. A raw fix-direction token is a broad family for prioritizing follow-up validation; it does not prove IO, a waited object, a resource holder, a subsystem, or a mechanism. Do not infer resource semantics from a call-site identifier's spelling, prefix, suffix, or module-like label. When measured_io_wait_authority is exact_zero_for_this_state_account, report only that this scheduler accounting bucket is zero; do not say it proves the absence of underlying storage IO, device wait, or another dependency mechanism. Such mechanism exclusion requires separate typed evidence. Do not describe this seat as measured IO wait. Report the measured D-state wall clock, the kernel-recorded call site, and the unknown object/holder/subsystem boundary separately; the diagnosis and wording remain model-owned. Report the caller only as the kernel call site unless separate typed identity/dependency evidence establishes the waited object, resource holder, subsystem mechanism, or causal relation. Report both observation domains under their own rulers. Do not pair records with state segments, substitute the census delay sum for state wall clock, or explain a count/duration difference as missing, extra, omitted, or mismatched events unless a separate typed interval join provides that mapping.\n",
				traceDecisionPromptScalar(account.Subject), start, end, account.DStateMS, account.IOWaitMS,
				measuredIOWaitAuthority, count, traceDecisionPromptScalar(callers))
		}
	}
	return b.String()
}

// renderTraceFinalRuntimeEnumerationAuthority repeats the exact runtime
// enumeration permission at the final synthesis seam. The earlier authority
// section can be separated from generation by a large ledger; repeating the
// same typed value here changes salience only. It never inspects or validates
// model prose and never creates a diagnosis. A separately proved complete
// rowset (for example target_wait_enumeration_authority) keeps its local exact
// permission without widening any incomplete sibling scope.
func renderTraceFinalRuntimeEnumerationAuthority(ctx *types.AgentContext) string {
	authority := answerDocRuntimeEnumerationAuthorityForAnswer(ctx)
	if !authority.Incomplete || len(authority.Boundaries) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "- runtime_enumeration_final_authority status=`incomplete`; affected_scopes=`%s`; emitted_rows_role=`bounded_sample_or_lower_bound`; exhaustive_claim_permission=`forbidden`; exact_total_count_extrema_absence_permission=`requires_separate_complete_typed_authority`; scope_local_complete_rowset_permission=`preserved_only_for_the_exact_separately_named_rowset`. Do not describe the affected scopes as all, only, exhaustive, complete, or without omissions. You still own the conclusion and wording.\n",
		strings.Join(authority.Scopes, ","))
	limit := len(authority.Boundaries)
	if limit > answerDocRuntimeEnumerationBoundaryLimit {
		limit = answerDocRuntimeEnumerationBoundaryLimit
	}
	for _, boundary := range authority.Boundaries[:limit] {
		total := "unknown"
		if boundary.TotalKnown {
			total = strconv.Itoa(boundary.Total)
		}
		fmt.Fprintf(&b, "  - incomplete_boundary scope=`%s`; dimension=`%s`; emitted=%d; total=%s; total_known=%t; reason=`%s`.\n",
			firstNonEmptyString(strings.TrimSpace(boundary.Scope), "unknown"),
			firstNonEmptyString(strings.TrimSpace(boundary.Dimension), "rows"),
			boundary.Emitted, total, boundary.TotalKnown, strings.TrimSpace(boundary.Reason))
	}
	if omitted := len(authority.Boundaries) - limit; omitted > 0 {
		fmt.Fprintf(&b, "  - omitted_exact_boundaries=%d; status remains incomplete.\n", omitted)
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
		traceDecisionWriteRepairDirectionPresentationPlan(&b, projection, label, 8, 12)
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

// renderTraceFinalStateValueAuthority surfaces typed rows where either the
// measured state duration, the chain cumulative account, or the published
// effective attribution differs. The compact distinction is deliberately
// prompt-only: it gives the model exact caliber without inspecting or
// rewriting its prose. Rows are bounded and deduped by typed identity so
// exploratory duplicates cannot flood the tail.
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
			cumulative := node.CumulativeImpactMS
			measuredDiffersFromEffective := effective > 0 && math.Abs(measured-effective) >= 0.0005
			cumulativeDiffersFromMeasured := cumulative > 0 && math.Abs(cumulative-measured) >= 0.0005
			if stateKind == "" || measured <= 0 || effective <= 0 || (!measuredDiffersFromEffective && !cumulativeDiffersFromMeasured) {
				continue
			}
			fmt.Fprintf(&b, "- state_value_authority artifact=`%s`; subject=`%s`; state_kind=`%s`; measured_state_occupancy=%.3fms; effective_attribution=%.3fms; relation=`distinct_do_not_substitute`; row_identity=`%s`",
				traceDecisionPromptScalar(label), traceDecisionPromptScalar(strings.TrimSpace(node.Subject)),
				traceDecisionPromptScalar(stateKind), measured, effective, traceDecisionPromptScalar(identity))
			if cumulative > 0 {
				fmt.Fprintf(&b, "; chain_cumulative=%.3fms; chain_cumulative_role=`node_or_subchain_account_not_state_occupancy`", cumulative)
			}
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
	// ImpactMS is the row's selected-window state/span projection. A larger
	// CumulativeImpactMS is the node/sub-chain account and may include other
	// states (for example running+runnable+sleep on one wakeup-chain member).
	// Treating cumulative as the state occupancy produced the impossible phrase
	// "9ms runnable" beside a typed runnable=8.3ms split. RunnableMS is the
	// strictest state-specific carrier where available; ImpactMS is the honest
	// row-local fallback for every other state/span family.
	if strings.EqualFold(strings.TrimSpace(node.StateKind), "runnable") && node.RunnableMS > 0 {
		return node.RunnableMS
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
