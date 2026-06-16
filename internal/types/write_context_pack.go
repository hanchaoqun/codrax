package types

import (
	"fmt"
	"sort"
	"strings"
)

const (
	writeContextPackMaxItems = 96
	writeContextPackTextLen  = 240
)

// WriteContextPriority is the stable priority lane for write workflow context.
// Lower-numbered priorities are rendered first by consumer views.
type WriteContextPriority string

const (
	WriteContextP0 WriteContextPriority = "p0"
	WriteContextP1 WriteContextPriority = "p1"
	WriteContextP2 WriteContextPriority = "p2"
	WriteContextP3 WriteContextPriority = "p3"
)

// WriteContextConsumer names the stage/controller that intends to consume a
// context item. Empty item Consumers means the item is visible to every view.
type WriteContextConsumer string

const (
	WriteConsumerController WriteContextConsumer = "controller"
	WriteConsumerPlanner    WriteContextConsumer = "planner"
	WriteConsumerVerifier   WriteContextConsumer = "verifier"
	WriteConsumerApproval   WriteContextConsumer = "approval"
)

// WriteContextItem is one prioritized, typed handoff fact. Text is advisory
// prompt material; hard gates must continue to read the underlying typed
// artifacts such as ChangePlan, ChangeReport, Approval, and EvidenceRef fields.
type WriteContextItem struct {
	ID          string                       `json:"id,omitempty"`
	Priority    WriteContextPriority         `json:"priority"`
	Kind        string                       `json:"kind"`
	Text        string                       `json:"text"`
	SourceStage string                       `json:"source_stage,omitempty"`
	SourceID    string                       `json:"source_id,omitempty"`
	EvidenceRef *WriteExplorationEvidenceRef `json:"evidence_ref,omitempty"`
	Consumers   []WriteContextConsumer       `json:"consumers,omitempty"`
}

// WriteContextPack is the bounded handoff artifact shared across the write
// workflow. It keeps rich exploration / planning / verification signals
// persisted as typed items while allowing each consumer to render only its
// relevant Top-N view.
type WriteContextPack struct {
	PackID      string             `json:"pack_id,omitempty"`
	BatchID     string             `json:"batch_id,omitempty"`
	Goal        string             `json:"goal,omitempty"`
	SourceStage string             `json:"source_stage,omitempty"`
	Items       []WriteContextItem `json:"items,omitempty"`
}

// WriteContextView is the consumer-filtered Top-N projection of a pack.
type WriteContextView struct {
	Consumer WriteContextConsumer `json:"consumer,omitempty"`
	Items    []WriteContextItem   `json:"items,omitempty"`
}

func NormalizeWriteContextPack(in WriteContextPack) WriteContextPack {
	in.PackID = trimWriteContextText(in.PackID)
	in.BatchID = trimWriteContextText(in.BatchID)
	in.Goal = trimWriteContextText(in.Goal)
	in.SourceStage = trimWriteContextText(in.SourceStage)
	seenIndex := map[string]int{}
	items := make([]WriteContextItem, 0, len(in.Items))
	for _, item := range in.Items {
		item = normalizeWriteContextItem(item)
		if item.Text == "" && item.EvidenceRef == nil {
			continue
		}
		key := writeContextItemKey(item)
		if idx, ok := seenIndex[key]; ok {
			items[idx] = mergeWriteContextDuplicateItem(items[idx], item)
			continue
		}
		seenIndex[key] = len(items)
		items = append(items, item)
		if len(items) >= writeContextPackMaxItems {
			break
		}
	}
	in.Items = items
	return in
}

func MergeWriteContextPacks(batchID, goal string, packs ...WriteContextPack) WriteContextPack {
	out := WriteContextPack{
		PackID:  "merged",
		BatchID: trimWriteContextText(batchID),
		Goal:    trimWriteContextText(goal),
	}
	for _, pack := range packs {
		pack = NormalizeWriteContextPack(pack)
		if out.BatchID == "" {
			out.BatchID = pack.BatchID
		}
		if out.Goal == "" {
			out.Goal = pack.Goal
		}
		out.Items = append(out.Items, pack.Items...)
	}
	return NormalizeWriteContextPack(out)
}

func (p WriteContextPack) View(consumer WriteContextConsumer, limit int) WriteContextView {
	p = NormalizeWriteContextPack(p)
	items := make([]WriteContextItem, 0, len(p.Items))
	for _, item := range p.Items {
		if writeContextItemVisibleTo(item, consumer) {
			items = append(items, item)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		ri := writeContextViewRank(items[i], consumer)
		rj := writeContextViewRank(items[j], consumer)
		if ri != rj {
			return ri < rj
		}
		return writeContextPriorityRank(items[i].Priority) < writeContextPriorityRank(items[j].Priority)
	})
	if limit > 0 && len(items) > limit {
		items = writeContextLimitedViewItems(items, consumer, limit)
	}
	return WriteContextView{Consumer: consumer, Items: cloneWriteContextItems(items)}
}

func WriteContextPackFromWriteAnalysisIR(ir *WriteAnalysisIR) WriteContextPack {
	if ir == nil {
		return WriteContextPack{}
	}
	goal := trimWriteContextText(ir.Request.Task.Summary)
	if goal == "" {
		goal = trimWriteContextText(ir.Request.RawRequest)
	}
	pack := WriteContextPack{
		PackID:      "write-analysis",
		BatchID:     "batch-1",
		Goal:        goal,
		SourceStage: "write_analysis",
	}
	for _, c := range ir.Request.Constraints {
		text := renderWriteConstraintContext(c)
		pack.Items = append(pack.Items, writeContextItem("constraint", WriteContextP0, text, "write_analysis",
			WriteConsumerController, WriteConsumerPlanner, WriteConsumerVerifier, WriteConsumerApproval))
	}
	if text := renderWriteRiskContext(ir.Request.Risk); text != "" {
		priority := WriteContextP1
		if ir.Request.Risk.Overall == RiskBandHigh || ir.Request.Risk.AffectsPublicAPI || ir.Request.Risk.ChangesPersistence || ir.Request.Risk.ChangesBuildSystem {
			priority = WriteContextP0
		}
		pack.Items = append(pack.Items, writeContextItem("risk_profile", priority, text, "write_analysis",
			WriteConsumerController, WriteConsumerPlanner, WriteConsumerVerifier, WriteConsumerApproval))
	}
	for _, path := range ir.Request.ScopeAnchors {
		pack.Items = append(pack.Items, writeContextItem("scope_anchor", WriteContextP1, path, "write_analysis",
			WriteConsumerController, WriteConsumerPlanner))
	}
	for _, outcome := range ir.Request.ExpectedOutcomes {
		pack.Items = append(pack.Items, writeContextItem("success_criterion", WriteContextP1, outcome, "write_analysis",
			WriteConsumerController, WriteConsumerPlanner, WriteConsumerVerifier))
	}
	for _, contract := range ir.Request.BehaviorContracts {
		text := renderWriteBehaviorContractContext(contract)
		if text == "" {
			continue
		}
		priority := WriteContextP1
		if contract.Required && contract.Polarity != WriteBehaviorPolarityObserved {
			priority = WriteContextP0
		}
		item := writeContextItem("behavior_contract", priority, text, "write_analysis",
			WriteConsumerController, WriteConsumerPlanner, WriteConsumerVerifier)
		item.SourceID = contract.ID
		item.ID = writeContextStableID("behavior_contract", contract.ID, string(contract.Kind), contract.Subject, contract.Expected)
		pack.Items = append(pack.Items, item)
	}
	for _, manifest := range ir.RepoFacts.Manifests {
		pack.Items = append(pack.Items, writeContextItem("manifest", WriteContextP2, manifest, "write_analysis",
			WriteConsumerPlanner, WriteConsumerVerifier))
	}
	if ir.RepoFacts.TestRunner != "" {
		pack.Items = append(pack.Items, writeContextItem("test_runner", WriteContextP2, ir.RepoFacts.TestRunner, "write_analysis",
			WriteConsumerPlanner, WriteConsumerVerifier))
	}
	for i, phase := range ir.PhaseProposal.Phases {
		for _, path := range phase.RoughTargetPaths {
			item := writeContextItem("phase_target", WriteContextP1, fmt.Sprintf("batch-%d: %s", i+1, path), "write_analysis",
				WriteConsumerController, WriteConsumerPlanner)
			item.SourceID = fmt.Sprintf("batch-%d", i+1)
			pack.Items = append(pack.Items, item)
		}
	}
	return NormalizeWriteContextPack(pack)
}

func WriteContextPackFromExplorationHandoff(h WriteExplorationHandoff) WriteContextPack {
	h = NormalizeWriteExplorationHandoff(h)
	pack := WriteContextPack{
		PackID:      "exploration-handoff",
		BatchID:     h.BatchID,
		Goal:        h.Goal,
		SourceStage: "explore",
	}
	for _, note := range h.RiskNotes {
		pack.Items = append(pack.Items, writeContextItem("risk_note", WriteContextP0, note, "explore",
			WriteConsumerController, WriteConsumerPlanner, WriteConsumerVerifier, WriteConsumerApproval))
	}
	for _, q := range h.ExplorationQuestions {
		pack.Items = append(pack.Items, writeContextItem("exploration_question", WriteContextP2, q, "explore",
			WriteConsumerController, WriteConsumerPlanner))
	}
	for _, file := range h.TargetFiles {
		pack.Items = append(pack.Items, writeContextItem("target_file", WriteContextP1, file, "explore",
			WriteConsumerController, WriteConsumerPlanner, WriteConsumerVerifier))
	}
	for _, symbol := range h.RelevantSymbols {
		pack.Items = append(pack.Items, writeContextItem("symbol", WriteContextP1, symbol, "explore",
			WriteConsumerController, WriteConsumerPlanner))
	}
	for _, invariant := range h.Invariants {
		pack.Items = append(pack.Items, writeContextItem("invariant", WriteContextP1, invariant, "explore",
			WriteConsumerController, WriteConsumerPlanner, WriteConsumerVerifier))
	}
	for _, ref := range h.EvidenceRefs {
		item := writeContextItem("evidence_ref", WriteContextP1, renderWriteEvidenceRefText(ref), "explore",
			WriteConsumerController, WriteConsumerPlanner, WriteConsumerVerifier)
		refCopy := normalizeWriteExplorationEvidenceRef(ref)
		item.EvidenceRef = &refCopy
		pack.Items = append(pack.Items, item)
	}
	for _, test := range h.TestSurface {
		pack.Items = append(pack.Items, writeContextItem("test_surface", WriteContextP2, test, "explore",
			WriteConsumerPlanner, WriteConsumerVerifier))
	}
	for _, unknown := range h.Unknowns {
		pack.Items = append(pack.Items, writeContextItem("unknown", WriteContextP2, unknown, "explore",
			WriteConsumerController, WriteConsumerPlanner))
	}
	for _, pattern := range h.ExistingPatterns {
		pack.Items = append(pack.Items, writeContextItem("pattern_hint", WriteContextP3, pattern, "explore",
			WriteConsumerPlanner))
	}
	return NormalizeWriteContextPack(pack)
}

func WriteContextPackFromApprovalRecord(batchID, goal string, approval *WriteApprovalRecord) WriteContextPack {
	if approval == nil {
		return WriteContextPack{}
	}
	pack := WriteContextPack{
		PackID:      "approval",
		BatchID:     trimWriteContextText(batchID),
		Goal:        trimWriteContextText(goal),
		SourceStage: "approval",
	}
	text := fmt.Sprintf("policy=%s risk=%s action=%s decision=%s reason=%s",
		approval.Policy, approval.RiskLevel, approval.Action, approval.UserDecision, approval.ReasonCode)
	pack.Items = append(pack.Items, writeContextItem("approval", WriteContextP0, text, "approval",
		WriteConsumerController, WriteConsumerPlanner, WriteConsumerVerifier, WriteConsumerApproval))
	for _, reason := range approval.Reasons {
		detail := strings.TrimSpace(reason.Detail)
		if reason.Path != "" {
			detail = strings.TrimSpace(detail + " path=" + reason.Path)
		}
		if detail == "" {
			detail = reason.Code
		}
		pack.Items = append(pack.Items, writeContextItem("approval_reason", WriteContextP0, detail, "approval",
			WriteConsumerController, WriteConsumerPlanner, WriteConsumerVerifier, WriteConsumerApproval))
	}
	return NormalizeWriteContextPack(pack)
}

func WriteContextPackFromChangePlan(plan *ChangePlan) WriteContextPack {
	if plan == nil {
		return WriteContextPack{}
	}
	pack := WriteContextPack{
		PackID:      "change-plan",
		BatchID:     plan.ID,
		Goal:        plan.Summary,
		SourceStage: "plan",
	}
	for _, path := range plan.TargetPaths {
		pack.Items = append(pack.Items, writeContextItem("target_file", WriteContextP1, path, "plan",
			WriteConsumerController, WriteConsumerPlanner, WriteConsumerVerifier))
	}
	if plan.Summary != "" {
		pack.Items = append(pack.Items, writeContextItem("plan_summary", WriteContextP2, plan.Summary, "plan",
			WriteConsumerController, WriteConsumerPlanner, WriteConsumerVerifier))
	}
	for _, test := range plan.AcceptanceTests {
		pack.Items = append(pack.Items, writeContextItem("acceptance_test", WriteContextP2, test, "plan",
			WriteConsumerPlanner, WriteConsumerVerifier))
	}
	for _, reason := range plan.UnvalidatedReasons {
		pack.Items = append(pack.Items, writeContextItem("unvalidated_reason", WriteContextP2, reason, "plan",
			WriteConsumerController, WriteConsumerPlanner, WriteConsumerVerifier))
	}
	if plan.PlanCritique != "" {
		pack.Items = append(pack.Items, writeContextItem("plan_critique", WriteContextP2, plan.PlanCritique, "plan_critic",
			WriteConsumerController, WriteConsumerPlanner, WriteConsumerApproval))
	}
	if plan.Approval != nil {
		pack = MergeWriteContextPacks(pack.BatchID, pack.Goal, pack, WriteContextPackFromApprovalRecord(plan.ID, plan.Summary, plan.Approval))
	}
	return NormalizeWriteContextPack(pack)
}

func WriteContextPackFromChangeReport(report *ChangeReport) WriteContextPack {
	if report == nil {
		return WriteContextPack{}
	}
	pack := WriteContextPack{
		PackID:      "change-report",
		BatchID:     report.PlanID,
		SourceStage: "verify",
	}
	if !report.Passed {
		kind := "verify_failure"
		if report.BuildFailed {
			kind = "build_failure"
		}
		pack.Items = append(pack.Items, writeContextItem(kind, WriteContextP2, report.FailureSummary, "verify",
			WriteConsumerController, WriteConsumerPlanner, WriteConsumerVerifier))
		pack.Items[len(pack.Items)-1].ID = writeVerifyFailureContextID(report, kind)
		if report.FailureSummaryBlobRef != "" {
			pack.Items = append(pack.Items, writeContextItem("failure_summary_blob_ref", WriteContextP2, report.FailureSummaryBlobRef, "verify",
				WriteConsumerController, WriteConsumerPlanner, WriteConsumerVerifier))
			pack.Items[len(pack.Items)-1].ID = writeContextStableID("failure_summary_blob_ref", report.PlanID, report.FailureSummaryBlobRef)
		}
		if report.FailureReasonCode != "" {
			pack.Items = append(pack.Items, writeContextItem("failure_reason_code", WriteContextP2, report.FailureReasonCode, "verify",
				WriteConsumerController, WriteConsumerPlanner, WriteConsumerVerifier))
			pack.Items[len(pack.Items)-1].ID = writeContextStableID("failure_reason_code", report.PlanID, report.FailureReasonCode)
		}
	}
	for _, diag := range report.VerificationDiagnostics {
		text := renderVerificationDiagnosticContext(diag)
		if text == "" {
			continue
		}
		pack.Items = append(pack.Items, writeContextItem("verification_diagnostic", WriteContextP2, text, "verify",
			WriteConsumerController, WriteConsumerPlanner, WriteConsumerVerifier))
		pack.Items[len(pack.Items)-1].ID = writeVerificationDiagnosticContextID(diag)
	}
	for _, result := range report.TestResults {
		if result.Passed {
			continue
		}
		text := result.AssertionID
		if text == "" {
			text = result.Suite
		}
		if result.FailureDetail != "" {
			text = strings.TrimSpace(text + " — " + result.FailureDetail)
		}
		pack.Items = append(pack.Items, writeContextItem("failed_test", WriteContextP2, text, "verify",
			WriteConsumerController, WriteConsumerPlanner, WriteConsumerVerifier))
		pack.Items[len(pack.Items)-1].ID = writeFailedTestContextID(result)
		for _, buildErr := range result.BuildErrors {
			pack.Items = append(pack.Items, writeContextItem("build_error", WriteContextP2, renderBuildErrorContext(buildErr), "verify",
				WriteConsumerController, WriteConsumerPlanner, WriteConsumerVerifier))
			pack.Items[len(pack.Items)-1].ID = writeBuildErrorContextID(buildErr)
		}
	}
	for _, cmd := range report.ExecutedCommands {
		text := renderExecutedCommandContext(cmd)
		if text == "" {
			continue
		}
		pack.Items = append(pack.Items, writeContextItem("executed_command", WriteContextP2, text, "verify",
			WriteConsumerController, WriteConsumerPlanner, WriteConsumerVerifier))
		pack.Items[len(pack.Items)-1].ID = writeExecutedCommandContextID(cmd)
	}
	for _, assertion := range report.RegressionAssertions {
		pack.Items = append(pack.Items, writeContextItem("regression_assertion", WriteContextP2, assertion, "verify",
			WriteConsumerController, WriteConsumerPlanner, WriteConsumerVerifier))
		pack.Items[len(pack.Items)-1].ID = writeContextStableID("regression_assertion", assertion)
	}
	for _, runner := range report.NoTestsRunners {
		pack.Items = append(pack.Items, writeContextItem("no_tests_runner", WriteContextP2, runner, "verify",
			WriteConsumerController, WriteConsumerPlanner, WriteConsumerVerifier))
		pack.Items[len(pack.Items)-1].ID = writeContextStableID("no_tests_runner", runner)
	}
	return NormalizeWriteContextPack(pack)
}

func normalizeWriteContextItem(in WriteContextItem) WriteContextItem {
	in.ID = trimWriteContextText(in.ID)
	in.Priority = normalizeWriteContextPriority(in.Priority)
	in.Kind = trimWriteContextText(in.Kind)
	in.Text = trimWriteContextText(in.Text)
	in.SourceStage = trimWriteContextText(in.SourceStage)
	in.SourceID = trimWriteContextText(in.SourceID)
	if in.EvidenceRef != nil {
		ref := normalizeWriteExplorationEvidenceRef(*in.EvidenceRef)
		in.EvidenceRef = &ref
	}
	in.Consumers = normalizeWriteContextConsumers(in.Consumers)
	return in
}

func cloneWriteContextPack(in WriteContextPack) WriteContextPack {
	in.Items = cloneWriteContextItems(in.Items)
	return in
}

func cloneWriteContextItems(in []WriteContextItem) []WriteContextItem {
	out := make([]WriteContextItem, len(in))
	for i, item := range in {
		out[i] = item
		out[i].Consumers = append([]WriteContextConsumer(nil), item.Consumers...)
		if item.EvidenceRef != nil {
			ref := *item.EvidenceRef
			out[i].EvidenceRef = &ref
		}
	}
	return out
}

func writeContextItem(kind string, priority WriteContextPriority, text, source string, consumers ...WriteContextConsumer) WriteContextItem {
	return WriteContextItem{
		Priority:    priority,
		Kind:        kind,
		Text:        text,
		SourceStage: source,
		Consumers:   append([]WriteContextConsumer(nil), consumers...),
	}
}

func writeContextItemVisibleTo(item WriteContextItem, consumer WriteContextConsumer) bool {
	if consumer == "" || len(item.Consumers) == 0 {
		return true
	}
	for _, c := range item.Consumers {
		if c == consumer {
			return true
		}
	}
	return false
}

func normalizeWriteContextPriority(in WriteContextPriority) WriteContextPriority {
	switch in {
	case WriteContextP0, WriteContextP1, WriteContextP2, WriteContextP3:
		return in
	default:
		return WriteContextP3
	}
}

func writeContextPriorityRank(priority WriteContextPriority) int {
	switch normalizeWriteContextPriority(priority) {
	case WriteContextP0:
		return 0
	case WriteContextP1:
		return 1
	case WriteContextP2:
		return 2
	default:
		return 3
	}
}

func normalizeWriteContextConsumers(in []WriteContextConsumer) []WriteContextConsumer {
	seen := map[WriteContextConsumer]struct{}{}
	out := make([]WriteContextConsumer, 0, len(in))
	for _, c := range in {
		switch c {
		case WriteConsumerController, WriteConsumerPlanner, WriteConsumerVerifier, WriteConsumerApproval:
		default:
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	return out
}

func mergeWriteContextDuplicateItem(existing, incoming WriteContextItem) WriteContextItem {
	out := existing
	if out.ID == "" {
		out.ID = incoming.ID
	}
	if out.Text == "" {
		out.Text = incoming.Text
	}
	if out.SourceStage == "" {
		out.SourceStage = incoming.SourceStage
	}
	if out.SourceID == "" {
		out.SourceID = incoming.SourceID
	}
	if out.EvidenceRef == nil && incoming.EvidenceRef != nil {
		ref := *incoming.EvidenceRef
		out.EvidenceRef = &ref
	}
	if len(existing.Consumers) == 0 || len(incoming.Consumers) == 0 {
		out.Consumers = nil
		return out
	}
	out.Consumers = normalizeWriteContextConsumers(append(append([]WriteContextConsumer(nil), existing.Consumers...), incoming.Consumers...))
	return out
}

func writeContextItemKey(item WriteContextItem) string {
	ref := ""
	if item.EvidenceRef != nil {
		ref = fmt.Sprintf("%s:%d:%d:%s", item.EvidenceRef.Source, item.EvidenceRef.LineStart, item.EvidenceRef.LineEnd, item.EvidenceRef.ID)
	}
	// Items anchored to a typed evidence location dedupe on the fact, not
	// its wording: retries re-project the same file:line finding with
	// slightly different text, and re-worded duplicates of one fact must
	// not crowd consumer Top-N views.
	//
	// Consumers are intentionally excluded. The same typed fact may be
	// projected for planner in one stage and verifier in another; normalize
	// merges those consumer sets instead of preserving duplicate prompt rows.
	text := strings.ToLower(item.Text)
	if item.ID != "" || (item.EvidenceRef != nil && strings.TrimSpace(item.EvidenceRef.Source) != "" && item.EvidenceRef.LineStart > 0) {
		text = ""
	}
	return strings.Join([]string{
		string(item.Priority),
		item.Kind,
		item.ID,
		text,
		item.SourceStage,
		item.SourceID,
		ref,
	}, "\x00")
}

func writeContextViewRank(item WriteContextItem, consumer WriteContextConsumer) int {
	if consumer == WriteConsumerPlanner && item.SourceStage == "verify" && item.Priority == WriteContextP2 && writeContextVerifyFailureKind(item.Kind) {
		return 1
	}
	rank := writeContextPriorityRank(item.Priority)
	if consumer == WriteConsumerPlanner && rank >= 1 {
		return rank + 1
	}
	return rank
}

func writeContextLimitedViewItems(items []WriteContextItem, consumer WriteContextConsumer, limit int) []WriteContextItem {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	selected := cloneWriteContextItems(items[:limit])
	selectedKeys := map[string]struct{}{}
	for _, item := range selected {
		selectedKeys[writeContextItemKey(item)] = struct{}{}
	}
	for _, item := range items[limit:] {
		if !writeContextMustCarryInLimitedView(item, consumer) {
			continue
		}
		key := writeContextItemKey(item)
		if _, ok := selectedKeys[key]; ok {
			continue
		}
		replace := writeContextLimitedViewReplacementIndex(selected, consumer, limit)
		if replace < 0 {
			continue
		}
		delete(selectedKeys, writeContextItemKey(selected[replace]))
		selected[replace] = item
		selectedKeys[key] = struct{}{}
	}
	sort.SliceStable(selected, func(i, j int) bool {
		ri := writeContextViewRank(selected[i], consumer)
		rj := writeContextViewRank(selected[j], consumer)
		if ri != rj {
			return ri < rj
		}
		return writeContextPriorityRank(selected[i].Priority) < writeContextPriorityRank(selected[j].Priority)
	})
	return selected
}

func writeContextMustCarryInLimitedView(item WriteContextItem, consumer WriteContextConsumer) bool {
	return consumer == WriteConsumerPlanner &&
		item.SourceStage == "verify" &&
		item.Priority == WriteContextP2 &&
		writeContextVerifyFailureKind(item.Kind)
}

func writeContextLimitedViewReplacementIndex(selected []WriteContextItem, consumer WriteContextConsumer, limit int) int {
	p0Count := 0
	for _, item := range selected {
		if item.Priority == WriteContextP0 {
			p0Count++
		}
	}
	minP0 := writeContextMinimumP0InLimitedView(limit)
	best := -1
	bestRank := -1
	for i, item := range selected {
		if writeContextMustCarryInLimitedView(item, consumer) {
			continue
		}
		if item.Priority == WriteContextP0 {
			continue
		}
		rank := writeContextViewRank(item, consumer)
		if rank > bestRank || (rank == bestRank && i > best) {
			best = i
			bestRank = rank
		}
	}
	if best >= 0 {
		return best
	}
	if p0Count <= minP0 {
		return -1
	}
	for i := len(selected) - 1; i >= 0; i-- {
		if selected[i].Priority == WriteContextP0 && !writeContextMustCarryInLimitedView(selected[i], consumer) {
			return i
		}
	}
	return -1
}

func writeContextMinimumP0InLimitedView(limit int) int {
	switch {
	case limit <= 0:
		return 0
	case limit == 1:
		return 1
	case limit < 4:
		return 1
	default:
		return 3
	}
}

func writeContextVerifyFailureKind(kind string) bool {
	switch kind {
	case "verify_failure", "build_failure", "failed_test", "build_error", "regression_assertion", "no_tests_runner", "executed_command", "failure_summary_blob_ref":
		return true
	default:
		return false
	}
}

func trimWriteContextText(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = strings.Join(strings.Fields(raw), " ")
	runes := []rune(raw)
	if len(runes) > writeContextPackTextLen {
		return string(runes[:writeContextPackTextLen]) + "..."
	}
	return raw
}

func renderWriteConstraintContext(c WriteConstraint) string {
	parts := []string{}
	if c.Kind != "" {
		parts = append(parts, c.Kind)
	}
	if c.Target != "" {
		parts = append(parts, c.Target)
	}
	if c.Note != "" {
		parts = append(parts, c.Note)
	}
	return strings.Join(parts, ": ")
}

func renderWriteRiskContext(r WriteRiskProfile) string {
	parts := []string{}
	if r.Overall != "" {
		parts = append(parts, "overall="+string(r.Overall))
	}
	if r.AffectsPublicAPI {
		parts = append(parts, "affects_public_api=true")
	}
	if r.ChangesPersistence {
		parts = append(parts, "changes_persistence=true")
	}
	if r.ChangesBuildSystem {
		parts = append(parts, "changes_build_system=true")
	}
	return strings.Join(parts, " ")
}

func renderWriteEvidenceRefText(ref WriteExplorationEvidenceRef) string {
	label := ref.Subject
	if label == "" {
		label = ref.AnchorSymbol
	}
	if label == "" {
		label = ref.Kind
	}
	loc := ref.Source
	if ref.LineStart > 0 {
		loc += fmt.Sprintf(":%d", ref.LineStart)
	}
	if ref.LineEnd > ref.LineStart {
		loc += fmt.Sprintf("-%d", ref.LineEnd)
	}
	switch {
	case label != "" && loc != "":
		return trimWriteContextText(label + " @ " + loc + " — " + ref.Summary)
	case loc != "":
		return trimWriteContextText(loc + " — " + ref.Summary)
	default:
		return trimWriteContextText(label + " — " + ref.Summary)
	}
}

func renderBuildErrorContext(err BuildError) string {
	loc := err.File
	if err.Line > 0 {
		loc += fmt.Sprintf(":%d", err.Line)
	}
	if err.Column > 0 {
		loc += fmt.Sprintf(":%d", err.Column)
	}
	if err.Symbol != "" {
		return trimWriteContextText(loc + " " + err.Symbol + " " + err.Message)
	}
	return trimWriteContextText(loc + " " + err.Message)
}

func renderExecutedCommandContext(cmd ExecutedCommand) string {
	parts := []string{}
	if cmd.Command != "" {
		parts = append(parts, "command="+cmd.Command)
	}
	if cmd.WorkingDir != "" {
		parts = append(parts, "cwd="+cmd.WorkingDir)
	}
	if cmd.Runner != "" {
		parts = append(parts, "runner="+cmd.Runner)
	}
	if cmd.Framework != "" {
		parts = append(parts, "framework="+cmd.Framework)
	}
	parts = append(parts, fmt.Sprintf("exit=%d", cmd.ExitCode))
	if cmd.Outcome != "" {
		parts = append(parts, "outcome="+cmd.Outcome)
	}
	if cmd.ReasonCode != "" {
		parts = append(parts, "reason_code="+cmd.ReasonCode)
	}
	if len(parts) == 1 && parts[0] == "exit=0" {
		return ""
	}
	return trimWriteContextText(strings.Join(parts, " "))
}

func renderVerificationDiagnosticContext(diag VerificationDiagnostic) string {
	parts := []string{}
	if diag.Category != "" {
		parts = append(parts, "category="+diag.Category)
	}
	if diag.Severity != "" {
		parts = append(parts, "severity="+diag.Severity)
	}
	if diag.ReasonCode != "" {
		parts = append(parts, "reason_code="+diag.ReasonCode)
	}
	if diag.Runner != "" {
		parts = append(parts, "runner="+diag.Runner)
	}
	if diag.Framework != "" {
		parts = append(parts, "framework="+diag.Framework)
	}
	if diag.WorkingDir != "" {
		parts = append(parts, "cwd="+diag.WorkingDir)
	}
	if diag.Outcome != "" {
		parts = append(parts, "outcome="+diag.Outcome)
	}
	if diag.ExitCode != 0 {
		parts = append(parts, fmt.Sprintf("exit=%d", diag.ExitCode))
	}
	if diag.Source != "" {
		parts = append(parts, "source="+diag.Source)
	}
	if diag.Command != "" {
		parts = append(parts, "command="+diag.Command)
	}
	if diag.Detail != "" && diag.Detail != diag.ReasonCode && diag.Detail != diag.Outcome {
		parts = append(parts, "detail="+diag.Detail)
	}
	return trimWriteContextText(strings.Join(parts, " "))
}

func renderWriteBehaviorContractContext(c WriteBehaviorContract) string {
	parts := []string{}
	if c.ID != "" {
		parts = append(parts, "id="+c.ID)
	}
	if c.Kind != "" {
		parts = append(parts, "kind="+string(c.Kind))
	}
	if c.Operator != "" {
		parts = append(parts, "operator="+string(c.Operator))
	}
	if c.Polarity != "" {
		parts = append(parts, "polarity="+string(c.Polarity))
	}
	if c.Subject != "" {
		parts = append(parts, "subject="+c.Subject)
	}
	if c.Expected != "" {
		parts = append(parts, "expected="+c.Expected)
	}
	if c.Required {
		parts = append(parts, "required=true")
	}
	if c.EvidenceRef != "" {
		parts = append(parts, "evidence_ref="+c.EvidenceRef)
	}
	return trimWriteContextText(strings.Join(parts, " "))
}

func writeVerifyFailureContextID(report *ChangeReport, kind string) string {
	if report == nil {
		return writeContextStableID(kind)
	}
	return writeContextStableID(kind, report.PlanID, string(report.FailureKind))
}

func writeFailedTestContextID(result TestResult) string {
	if strings.TrimSpace(result.Suite) == "" && strings.TrimSpace(result.AssertionID) == "" {
		return ""
	}
	return writeContextStableID("failed_test", result.Suite, result.AssertionID)
}

func writeBuildErrorContextID(err BuildError) string {
	if strings.TrimSpace(err.File) == "" && err.Line == 0 && err.Column == 0 && strings.TrimSpace(err.Symbol) == "" {
		return ""
	}
	return writeContextStableID("build_error", err.File, fmt.Sprintf("%d", err.Line), fmt.Sprintf("%d", err.Column), err.Symbol)
}

func writeExecutedCommandContextID(cmd ExecutedCommand) string {
	if strings.TrimSpace(cmd.Runner) == "" && strings.TrimSpace(cmd.Framework) == "" &&
		strings.TrimSpace(cmd.WorkingDir) == "" && strings.TrimSpace(cmd.Command) == "" {
		return ""
	}
	return writeContextStableID("executed_command", cmd.Runner, cmd.Framework, cmd.WorkingDir, cmd.Command, cmd.Outcome, cmd.ReasonCode)
}

func writeVerificationDiagnosticContextID(diag VerificationDiagnostic) string {
	if strings.TrimSpace(diag.Category) == "" && strings.TrimSpace(diag.ReasonCode) == "" &&
		strings.TrimSpace(diag.Outcome) == "" && strings.TrimSpace(diag.Runner) == "" {
		return ""
	}
	return writeContextStableID("verification_diagnostic", diag.Source, diag.Category, diag.Severity,
		diag.ReasonCode, diag.Runner, diag.Framework, diag.WorkingDir, diag.Command, diag.Outcome)
}

func writeContextStableID(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(strings.ToLower(part))
		if part == "" || part == "0" {
			continue
		}
		out = append(out, part)
	}
	return strings.Join(out, "|")
}
