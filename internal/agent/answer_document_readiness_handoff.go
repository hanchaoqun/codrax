package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// This is a display projection of existing source-lane authority, not a new
// query-completion or evidence gate. In particular a legitimate log/MCP fact
// need not have a trace query, while reading bytes does not compute metrics.
func renderAnswerDocRuntimeSourceAuthority(ctx *types.AgentContext, ledger types.ObservationLedger) string {
	if ledger.Empty() && ctx != nil {
		ledger = types.CompileObservationLedger(types.ObservationLedgerInputFromAgentContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
	}
	authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForAgentContext(ctx, ledger)
	if !authority.Active {
		return ""
	}
	var b strings.Builder
	b.WriteString("### Runtime Source Authority\n\n")
	b.WriteString("- Scope: source-lane availability only. These states do not establish requested fact coverage, successful target-query execution, or answer completeness. They do not authorize additional claims, reopen exploration, or invent citations.\n")
	fmt.Fprintf(&b, "- current_source_lane=`%s`; requirement_precision=`%s`; current_source_required=%t; current_source_satisfied=%t\n",
		authority.CurrentSourceLane, runtimeSourceRequirementPromptValue(authority.CurrentSourceRequirement),
		authority.CurrentSourceRequired, authority.CurrentSourceSatisfied)
	fmt.Fprintf(&b, "- runtime_observations=%d; deterministic_runtime_queries=%d; current_source_records=%d; exact_current_source_support=%d\n",
		authority.RuntimeObservationCount, authority.DeterministicRuntimeQueryCount,
		authority.CurrentSourceRecordCount, authority.ExactCurrentSourceSupportCount)
	b.WriteString("- All counts above are observation records, not tool invocation counts. In particular, deterministic_runtime_queries counts result records from a deterministic query producer, not distinct queries; it does not prove this request's source, target, window, or required fields were covered.\n")
	b.WriteString(renderAnswerDocObservationStages(ledger))
	fmt.Fprintf(&b, "- source_lane_state: external_observation_present=%t; runtime_only_with_caveat=%t; hard_block=%t; caveat_only=%t\n",
		answerDocHasExternalObservationRecord(ledger),
		authority.CanUseRuntimeOnlyWithCaveat, authority.CanHardBlockCompletion, authority.CanDowngradeToCaveat)
	if authority.RuntimeCitationPolicy != "" {
		fmt.Fprintf(&b, "- runtime_citation_policy=`%s`\n", authority.RuntimeCitationPolicy)
	}
	var reasons []string
	for _, code := range authority.ReasonCodes {
		// The legacy combined bit is intentionally unchanged in types. Its
		// unqualified name is not an answer-facing readiness declaration.
		if code != types.RuntimeSourceAuthorityCombinedProofReady && strings.TrimSpace(string(code)) != "" {
			reasons = append(reasons, string(code))
		}
	}
	if len(reasons) > 0 {
		fmt.Fprintf(&b, "- source_lane_reason_codes: `%s`\n", strings.Join(reasons, "`, `"))
	}
	switch {
	case authority.CanHardBlockCompletion:
		b.WriteString("- guidance: a precise current-source obligation is still missing; if the final answer proceeds, disclose that source-proof boundary instead of presenting runtime rows as source citations.\n")
	case authority.CanDowngradeToCaveat:
		b.WriteString("- guidance: the missing current-source obligation is soft. Preserve each available external observation within its own evidence scope and disclose the source boundary as a caveat when relevant; do not broaden the answer slate solely to satisfy this soft lane.\n")
	case authority.CanCompleteWithCombinedProof && authority.CurrentSourceSatisfied:
		b.WriteString("- guidance: external observations and current-source support are both present. Preserve both origins separately; their presence does not prove target facts or a causal connection between the sources.\n")
	case authority.RuntimeOnlySufficient || authority.CanUseRuntimeOnlyWithCaveat:
		b.WriteString("- guidance: external observation support is available within its own recorded scope; keep source citations out unless current-source records are present.\n")
	}
	b.WriteString("\n")
	return b.String()
}

func answerDocHasExternalObservationRecord(ledger types.ObservationLedger) bool {
	for _, record := range ledger.Records {
		if record.SourceRef.Kind != types.ObservationSourceRecoveryNavigation &&
			types.AnswerEvidenceOriginCarriesOriginSpecificSupport(record.Origin) {
			return true
		}
	}
	return false
}

func renderAnswerDocObservationStages(ledger types.ObservationLedger) string {
	var reads, queries, otherRuntime, mcp, command int
	for _, record := range ledger.Records {
		switch record.Origin {
		case types.AnswerEvidenceOriginRuntimeArtifact:
			if record.SourceRef.Kind != types.ObservationSourceRuntimeArtifact {
				continue
			}
			producer, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(record.Producer)), ":")
			switch {
			case producer == "read_file":
				reads++
			case types.RuntimeObservationProducerIsDeterministicQuery(record.Producer):
				queries++
			default:
				otherRuntime++
			}
		case types.AnswerEvidenceOriginMCPResource:
			mcp++
		case types.AnswerEvidenceOriginCommandMeasurement:
			command++
		}
	}
	return fmt.Sprintf("- observed_material: artifact_read_records=%d; deterministic_query_records=%d; other_runtime_records=%d; mcp_records=%d; command_measurement_records=%d. Raw/page reads establish only the bytes or lines actually read, not decoding, sample membership, a query result, or measured zero. Log/MCP/command facts remain usable in their own recorded scope without a trace query. Query execution, empty results, partial coverage, and requested fact availability must follow their own typed results and source/window contracts.\n", reads, queries, otherRuntime, mcp, command)
}

// The labels are derived from facts actually emitted by the existing typed
// readers, not merely from the analyzer's requested families. An empty card
// is omitted without claiming that the underlying facts are absent or zero.
func renderAnswerDocBoundedRuntimeReaderCard(facts string, profile *types.RuntimeQuestionProfile, presented map[types.RuntimeQuestionFactFamily]bool, zh bool) string {
	if facts == "" {
		return ""
	}
	var labels []string
	seen := make(map[string]bool)
	for _, family := range profile.FactFamilies {
		if presented[family] {
			if label := answerDocBoundedRuntimeFactFamilyReaderLabel(family, zh); label != "" && !seen[label] {
				labels = append(labels, label)
				seen[label] = true
			}
		}
	}
	var b strings.Builder
	if zh {
		b.WriteString("## 有限窗口查询的读者事实卡（结论由模型给出）\n\n")
		b.WriteString("- 以下只复述本卡实际取得的结构化事实，不代表请求的全部维度已齐备；未展示不是未发生或测得为零。其它已提供的结构化事实仍应保留原始字段、精确值、来源、区间和覆盖边界。" + answerDocReaderFieldVisibilityZH + "精确数值、窗口、覆盖边界和证据强度保持不变。系统不检查或修改模型正文，也不代替模型给结论。\n")
		if len(labels) > 0 {
			fmt.Fprintf(&b, "- 本卡已复述的事实维度：%s。\n", strings.Join(labels, "、"))
		}
	} else {
		b.WriteString("## Reader-ready finite-window facts (the model owns the conclusion)\n\n")
		b.WriteString("- This card restates only structured facts actually provided here, not completeness across requested dimensions; omission is not absence or a measured zero. Other provided structured facts retain their original fields, exact values, sources, intervals, and coverage boundaries. " + answerDocReaderFieldVisibilityEN + " Preserve exact values, windows, coverage boundaries, and evidence strength. The system neither checks nor rewrites model prose and does not choose the conclusion.\n")
		if len(labels) > 0 {
			fmt.Fprintf(&b, "- Fact dimensions restated in this card: %s.\n", strings.Join(labels, ", "))
		}
	}
	b.WriteString(facts)
	b.WriteString("\n")
	return b.String()
}
