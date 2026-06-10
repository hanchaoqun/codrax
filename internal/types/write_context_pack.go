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
	seen := map[string]struct{}{}
	items := make([]WriteContextItem, 0, len(in.Items))
	for _, item := range in.Items {
		item = normalizeWriteContextItem(item)
		if item.Text == "" && item.EvidenceRef == nil {
			continue
		}
		key := writeContextItemKey(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
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
		return writeContextPriorityRank(items[i].Priority) < writeContextPriorityRank(items[j].Priority)
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
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
		if report.FailureSummaryBlobRef != "" {
			pack.Items = append(pack.Items, writeContextItem("failure_summary_blob_ref", WriteContextP2, report.FailureSummaryBlobRef, "verify",
				WriteConsumerController, WriteConsumerPlanner, WriteConsumerVerifier))
		}
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
		for _, buildErr := range result.BuildErrors {
			pack.Items = append(pack.Items, writeContextItem("build_error", WriteContextP2, renderBuildErrorContext(buildErr), "verify",
				WriteConsumerController, WriteConsumerPlanner, WriteConsumerVerifier))
		}
	}
	for _, assertion := range report.RegressionAssertions {
		pack.Items = append(pack.Items, writeContextItem("regression_assertion", WriteContextP2, assertion, "verify",
			WriteConsumerController, WriteConsumerPlanner, WriteConsumerVerifier))
	}
	for _, runner := range report.NoTestsRunners {
		pack.Items = append(pack.Items, writeContextItem("no_tests_runner", WriteContextP2, runner, "verify",
			WriteConsumerPlanner, WriteConsumerVerifier))
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

func writeContextItemKey(item WriteContextItem) string {
	ref := ""
	if item.EvidenceRef != nil {
		ref = fmt.Sprintf("%s:%d:%d:%s", item.EvidenceRef.Source, item.EvidenceRef.LineStart, item.EvidenceRef.LineEnd, item.EvidenceRef.ID)
	}
	consumers := append([]WriteContextConsumer(nil), item.Consumers...)
	sort.Slice(consumers, func(i, j int) bool { return consumers[i] < consumers[j] })
	consumerParts := make([]string, 0, len(consumers))
	for _, c := range consumers {
		consumerParts = append(consumerParts, string(c))
	}
	return strings.Join([]string{
		string(item.Priority),
		item.Kind,
		strings.ToLower(item.Text),
		item.SourceStage,
		item.SourceID,
		ref,
		strings.Join(consumerParts, ","),
	}, "\x00")
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
