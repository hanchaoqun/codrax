package agent

import (
	"encoding/json"
	"fmt"
	"maps"
	"sort"
	"strings"

	promptctx "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// This optional protocol does not change the assemble/render/append contract
// for other evaluators. resolve runs after the evaluator's dispatch reset, so
// the canonical schema filter and the one dynamic construction share state.
type budgetedInitialInstructionEvaluator interface {
	BuildInitialInstructionWithBudget(*types.AgentContext, *skill.Config, func() *finalizerPromptBudget) string
}

// These are local content-byte allocations, not provider token measurements.
// The soft ratio only sizes optional previews. Required facts may exceed that
// target. One eighth of the derived byte window is explicitly unallocated for
// density uncertainty only: no request, first or repair, may spend it, and it
// is NOT an invented output token limit. A repair request is held to the same
// allocation as the first request, so repair is planned inside the allocation
// (the Repair* fields below).
type finalizerPromptBudget struct {
	Policy                  types.FinalizerRequestBytePolicy `json:"policy"`
	WindowDerivedBytes      int                              `json:"window_derived_bytes"`
	SafetyReserveBytes      int                              `json:"safety_reserve_bytes"`
	OutputKnown             bool                             `json:"output_known"`
	OutputReserveBytes      *int                             `json:"output_reserve_bytes"`
	StaticBytes             int                              `json:"static_bytes"`
	SchemaBytes             int                              `json:"schema_bytes"`
	TailBytes               int                              `json:"tail_bytes"`
	RequestAllocationBytes  int                              `json:"request_allocation_bytes"`
	OptionalTargetBytes     int                              `json:"optional_target_bytes"`
	DynamicAllocatedBytes   int                              `json:"dynamic_allocated_bytes"`
	DynamicPublishedBytes   int                              `json:"dynamic_published_bytes"`
	PreviewOmittedBytes     int                              `json:"preview_omitted_bytes"`
	PreviewOmittedGroups    int                              `json:"preview_omitted_dependency_groups"`
	DynamicUnexecutedGroups int                              `json:"dynamic_groups_not_executed"`
	EquivalentGroups        int                              `json:"equivalent_groups_compacted,omitempty"`
	EquivalentSavedBytes    int                              `json:"equivalent_bytes_saved,omitempty"`
	NativeLedgerPlan        *finalizerNativeLedgerPlan       `json:"native_ledger_plan,omitempty"`
	// Planned repair frame, enforce policy only (all zero otherwise). The
	// envelope is what one repair round is planned to add. The dividend is the
	// exact static teaching a lane-matched repair request no longer carries;
	// zero when the teaching is not replaceable. The reserve is what optional
	// previews and expanded equivalent forms must additionally leave free.
	// The shortfall says by how much required units already sit inside the
	// reserve: operator accounting, never a reason to drop a required unit.
	RepairFrameEnvelopeBytes    int `json:"repair_frame_envelope_bytes"`
	RepairTeachingDividendBytes int `json:"repair_teaching_dividend_bytes"`
	RepairFrameReserveBytes     int `json:"repair_frame_reserve_bytes"`
	RepairFrameShortfallBytes   int `json:"repair_frame_shortfall_bytes"`
	protectedDynamicBytes       int
}

// planRepairFrame records the repair plan of one dispatch before its dynamic
// instruction is built. Byte lengths only; nil-safe for an unknown window.
func (p *finalizerPromptBudget) planRepairFrame(teaching *finalizerRepairTeaching) {
	if p == nil || p.Policy != types.FinalizerRequestByteEnforce {
		return
	}
	p.RepairFrameEnvelopeBytes, p.RepairTeachingDividendBytes = p.WindowDerivedBytes/8, 0
	if teaching != nil && teaching.patchSystem != "" {
		p.RepairTeachingDividendBytes = max(0, len(teaching.draftSystem)-len(teaching.patchSystem))
	}
	p.RepairFrameReserveBytes = max(0, p.RepairFrameEnvelopeBytes-p.RepairTeachingDividendBytes)
}

// Operator-only accounting for two closed alternatives. These contain no
// source identities, facts or credentials and convey no evidence authority.
type finalizerNativeLedgerPlan struct {
	WithInventoryBytes    int  `json:"with_inventory_bytes"`
	WithoutInventoryBytes int  `json:"without_inventory_bytes"`
	InventoryPublished    bool `json:"inventory_published"`
	PublishedLedgerBytes  int  `json:"published_ledger_bytes"`
}

// Unresolved custom callers retain their advisory behavior. Production
// callers already use ResolvedAgentSettings, whose default is enforcement.
// Both modes share the same complete-unit planner and truthful receipt.
func finalizerRequestBytePolicy(settings types.AgentSettings) types.FinalizerRequestBytePolicy {
	if settings.FinalizerRequestBytePolicy == "" || settings.FinalizerRequestBytePolicy == types.FinalizerRequestByteLegacyAdvisory {
		return types.FinalizerRequestByteLegacyAdvisory
	}
	return types.FinalizerRequestByteEnforce
}

func (b *BaseAgent) initialFinalizerBudget(ctx *types.AgentContext, sk *skill.Config, messages []llm.Message) *finalizerPromptBudget {
	base := b.buildToolSchemas(sk, ctx)
	effective := base
	if filter, ok := b.eval.(ToolSchemaFilter); ok {
		effective = filter.FilterToolSchemas(ctx, effective)
	}
	effective, _ = filterExploreToolSchemasByBudget(ctx, effective)
	effective = llm.CompactToolSchemas(effective)
	var tails []llm.Message
	var surface toolSurfaceTransition
	if message, ok := surface.message(ctx, base, effective); ok {
		tails = append(tails, message)
	}
	if ctx != nil {
		_, request := types.SplitConversation(ctx.Objective)
		if nudge := promptctx.ReasoningLanguageTailNudge(ctx.Language, request); nudge != "" {
			tails = append(tails, llm.Message{Role: "user", Content: nudge})
		}
	}
	p := measureContextRequestPressure(b.deps.LLM, messages, effective)
	if p.WindowBytes <= 0 {
		return nil
	}
	budget := &finalizerPromptBudget{Policy: finalizerRequestBytePolicy(b.deps.AgentSettings), WindowDerivedBytes: p.WindowBytes, SafetyReserveBytes: p.WindowBytes / 8,
		OutputKnown: p.OutputKnown, StaticBytes: p.MessageBytes, SchemaBytes: p.ToolBytes, TailBytes: llm.EstimateMessagesBytes(tails)}
	budget.RequestAllocationBytes = p.WindowBytes - budget.SafetyReserveBytes
	if p.OutputKnown {
		reserve := int(^uint(0) >> 1)
		if p.OutputTokens <= reserve/types.BytesPerToken {
			reserve = p.OutputTokens * types.BytesPerToken
		}
		budget.OutputReserveBytes = &reserve
		budget.RequestAllocationBytes = max(0, budget.RequestAllocationBytes-min(reserve, budget.RequestAllocationBytes))
	}
	soft := b.deps.AgentSettings.ContextPressureSoftRatio
	if soft <= 0 || soft >= 1 {
		soft = .7
	}
	budget.OptionalTargetBytes = min(budget.RequestAllocationBytes, int(float64(p.WindowBytes)*soft))
	fixed := budget.StaticBytes + budget.SchemaBytes + budget.TailBytes + len("user")
	budget.DynamicAllocatedBytes = max(0, budget.RequestAllocationBytes-fixed)
	return budget
}

// Optional previews also leave the planned repair reserve free. The reserve
// never applies to required units or to the dispatch guard.
func (p *finalizerPromptBudget) optionalFits(nativeBytes int) bool {
	if p == nil {
		return true
	}
	target := min(p.OptionalTargetBytes, p.RequestAllocationBytes-p.RepairFrameReserveBytes)
	return nativeBytes <= target-p.StaticBytes-p.SchemaBytes-p.TailBytes-len("user")-p.protectedDynamicBytes
}

func (p *finalizerPromptBudget) requiredFits(nativeBytes int) bool {
	return p == nil || nativeBytes <= p.RequestAllocationBytes-p.StaticBytes-p.SchemaBytes-p.TailBytes-len("user")-p.protectedDynamicBytes
}

// AnswerContextByteBudgetError describes a local content-byte policy, not an
// exact tokenizer result or a provider capacity assertion. Structured callers
// can inspect its values without depending on prompt-facing error vocabulary.
type AnswerContextByteBudgetError struct {
	ActualBytes    int
	AllocatedBytes int
}

func (e *AnswerContextByteBudgetError) Error() string {
	return fmt.Sprintf("answer context exceeds its content-byte allocation: messages and tool schemas=%d bytes; allocation=%d bytes; complete required units retained, no request sent. This local byte policy is not an exact provider token limit", e.ActualBytes, e.AllocatedBytes)
}

func (e *answerDocumentEvaluator) finalizerRequestByteError(ctx *types.AgentContext, messages []llm.Message, tools []llm.ToolSchema) error {
	if e == nil || e.promptBudget == nil || e.requestBytePolicy != types.FinalizerRequestByteEnforce || ctx == nil || e.mu != ctx.Mutable || ctx.Stage != types.StageFinalize {
		return nil
	}
	actual := llm.EstimateRequestBytes(messages, tools)
	if actual <= e.promptBudget.RequestAllocationBytes {
		return nil
	}
	return &AnswerContextByteBudgetError{ActualBytes: actual, AllocatedBytes: e.promptBudget.RequestAllocationBytes}
}

// finalizerRepairTeaching retains the representations of the canonical
// system message and the attempt history of one dispatch. patchSystem is
// empty when the teaching is not replaceable (a custom skill or assembler);
// the history still serves that dispatch. followUpSystem is the accepted-answer
// follow-up representation, set only beside a patchSystem it does not exceed.
// Every projection is request-only: the durable history, each submitted tool
// call, validation feedback and evidence stay intact.
type finalizerRepairTeaching struct {
	draftSystem    string
	patchSystem    string
	followUpSystem string
	history        finalizerRepairHistory
}

// project applies the repair ladder to one request. A request that fits is
// returned untouched. Each rung runs only when the request is still over the
// unchanged allocation after the rung before it:
//
//  1. lane-matched static teaching (patch-only lane, canonical teaching): the
//     repair representation for a necessary repair, the follow-up one for the
//     optional follow-up after an accepted answer;
//  2. the staged-draft checkpoint, where the latest call proves it (necessary
//     repair only: an accepted answer has no staged draft);
//  3. the free text that accompanied superseded answer attempts and calls to
//     an answer tool the request did not offer;
//  4. the free text that accompanied the latest answer attempt.
//
// Rungs 3 and 4 apply in every lane and keep every submitted call with its
// arguments, its paired result and every correction. They give up the content
// channel only: reasoning a provider returned on its own channel stays with
// its tool call, so the request size still grows with that channel. What is
// still over the allocation afterwards is refused by the unchanged dispatch
// guard. With an accepted answer, the only request is the follow-up (HMC
// §377 D1); the accepted answer itself is never part of any projection.
func (t *finalizerRepairTeaching) project(ctx *types.AgentContext, eval Evaluator, messages []llm.Message, tools []llm.ToolSchema) []llm.Message {
	e, ok := eval.(*answerDocumentEvaluator)
	if !ok || e == nil || ctx == nil || ctx.Mutable == nil || len(messages) == 0 {
		return messages
	}
	err, over := e.finalizerRequestByteError(ctx, messages, tools).(*AnswerContextByteBudgetError)
	followUp := e.keepAcceptedAnswerUnderPressure(ctx)
	if !over || (ctx.Mutable.AnswerDocumentV2() != nil && !followUp) {
		return messages
	}
	out := messages
	lane, phase := "", "repair_prompt_budget"
	switch {
	case followUp:
		lane, phase = t.followUpSystem, "follow_up_prompt_budget"
	case (e.preferPatchNext || e.emitFullDocFailStreak >= 2) && !e.forceFullEmitNext:
		lane = t.patchSystem
	}
	if lane != "" && len(tools) == 1 && tools[0].Name == answerDocumentPatchToolName &&
		messages[0].Role == "system" && messages[0].Content == t.draftSystem {
		out = append([]llm.Message(nil), messages...)
		out[0].Content = lane
		logging.Debug("[diag finalizer] phase=%s draft_request_bytes=%d repair_request_bytes=%d allocation_bytes=%d replaced_system_bytes=%d",
			phase, err.ActualBytes, llm.EstimateRequestBytes(out, tools), err.AllocatedBytes, len(t.draftSystem)-len(lane))
		if e.finalizerRequestByteError(ctx, out, tools) == nil {
			return out
		}
		if checkpoint := t.history.project(ctx.Mutable, out); checkpoint != nil {
			before, after := llm.EstimateRequestBytes(out, tools), llm.EstimateRequestBytes(checkpoint, tools)
			if after < before {
				logging.Debug("[diag finalizer] phase=repair_generation_budget previous_bytes=%d current_bytes=%d allocation_bytes=%d", before, after, err.AllocatedBytes)
				out = checkpoint
			}
		}
	}
	for _, latest := range []bool{false, true} {
		if e.finalizerRequestByteError(ctx, out, tools) == nil {
			return out
		}
		if projected, removed := t.history.projectFreeText(out, latest); projected != nil {
			logging.Debug("[diag finalizer] phase=repair_free_text_budget previous_bytes=%d current_bytes=%d allocation_bytes=%d free_text_bytes=%d latest_attempt=%t",
				llm.EstimateRequestBytes(out, tools), llm.EstimateRequestBytes(projected, tools), err.AllocatedBytes, removed, latest)
			out = projected
		}
	}
	return out
}

type answerDocDeferredSection struct {
	offset int
	name   string
	render func() string
}

type finalizerTextProjection struct {
	expanded string
	compact  string
}

// These are builder-owned byte ranges, never matches in source, user or
// model prose. Replacement is permitted only by the producer of the section.
type finalizerEquivalentSection struct {
	start, end int
	compact    string
}

func (s finalizerEquivalentSection) savings() int { return s.end - s.start - len(s.compact) }

func compactEquivalentFinalizerSections(full string, sections []finalizerEquivalentSection, limit int) (string, int, int) {
	if len(full) <= limit || len(sections) == 0 {
		return full, 0, 0
	}
	order := make([]int, len(sections))
	for i := range order {
		order[i] = i
	}
	// Prefer replacing fewer sections; ties keep their assembly order.
	sort.SliceStable(order, func(i, j int) bool { return sections[order[i]].savings() > sections[order[j]].savings() })
	selected := make([]bool, len(sections))
	saved, count := 0, 0
	for _, i := range order {
		selected[i] = true
		saved += sections[i].savings()
		count++
		if len(full)-saved <= limit {
			break
		}
	}
	var out strings.Builder
	out.Grow(len(full) - saved)
	pos := 0
	for i, s := range sections {
		if selected[i] {
			out.WriteString(full[pos:s.start])
			out.WriteString(s.compact)
			pos = s.end
		}
	}
	out.WriteString(full[pos:])
	// If even the equivalent minimum cannot fit, the unchanged dispatch
	// guard still refuses it. No required unit is dropped to force a fit.
	return out.String(), count, saved
}

// Required outer sections are built once before allocating optional native
// previews. Offsets are builder positions, never markers scanned from prose.
// Deferred sections and their dependent narrative are each rendered once.
func (t *answerDocDynamicTrace) finishBudgeted(base string) string {
	if t == nil || t.promptBudget == nil {
		return base
	}
	p := t.promptBudget
	// Keep the existing conservative allowance for the already bounded
	// narrative and boundary notes. Actual bytes are rechecked before Chat;
	// operator accounting itself is not model context.
	p.protectedDynamicBytes = len(base) + 8*1024
	var out strings.Builder
	pos := 0
	var relocated []finalizerEquivalentSection
	nextEquivalent := 0
	copyBase := func(end int) {
		shift := out.Len() - pos
		for nextEquivalent < len(t.equivalent) && t.equivalent[nextEquivalent].end <= end {
			section := t.equivalent[nextEquivalent]
			section.start += shift
			section.end += shift
			relocated = append(relocated, section)
			nextEquivalent++
		}
		out.WriteString(base[pos:end])
		pos = end
	}
	for _, section := range t.deferred {
		copyBase(section.offset)
		if t.shouldStop(&out, section.name) {
			p.DynamicUnexecutedGroups++
			continue
		}
		stop := t.startSection(section.name)
		text := section.render()
		stop()
		appendAnswerDocDynamicSectionOutput(&out, text)
		out.WriteString("\n\n")
	}
	copyBase(len(base))
	// Local resource-policy numbers belong to this dispatch and diagnostics,
	// not answer evidence. Native omissions/completeness/navigation are still
	// rendered by their actual fact groups, with no operational duplicate.
	// Equivalent compact forms are lossless, so they also yield to the planned
	// repair reserve; what required units still occupy of it is only recorded.
	var instruction string
	planned := p.DynamicAllocatedBytes - p.RepairFrameReserveBytes
	instruction, p.EquivalentGroups, p.EquivalentSavedBytes = compactEquivalentFinalizerSections(out.String(), relocated, planned)
	p.DynamicPublishedBytes = len(instruction)
	p.RepairFrameShortfallBytes = min(p.RepairFrameReserveBytes, max(0, len(instruction)-planned))
	receipt, _ := json.Marshal(p)
	logging.Debug("[diag finalizer] phase=prompt_byte_budget receipt=%s", receipt)
	return instruction
}

// Optional native dependencies are committed together. A rejected candidate
// cannot populate the receipt later used to suppress another fact or claim
// that an independent investigation's native references were fully shown.
func cloneNativePresentationReceipt(p *answerDocNativePresentationReceipt) *answerDocNativePresentationReceipt {
	c := *p
	c.measurementViews = maps.Clone(p.measurementViews)
	c.measurementViewDetails = maps.Clone(p.measurementViewDetails)
	c.queries = maps.Clone(p.queries)
	c.records = maps.Clone(p.records)
	c.eventMembers = maps.Clone(p.eventMembers)
	c.samplingSelections = maps.Clone(p.samplingSelections)
	c.samplingAggregates = maps.Clone(p.samplingAggregates)
	c.samplingIdentities = maps.Clone(p.samplingIdentities)
	c.nativeSources = maps.Clone(p.nativeSources)
	c.samplingTableIdentities = maps.Clone(p.samplingTableIdentities)
	c.samplingCohortIDs = maps.Clone(p.samplingCohortIDs)
	for key, value := range c.samplingCohortIDs {
		c.samplingCohortIDs[key] = maps.Clone(value)
	}
	c.businessAnnotations = append([]types.NativeBusinessDiagramAnnotation(nil), p.businessAnnotations...)
	return &c
}

type answerDocNativeDeferredGroups struct {
	inventory, measurements, business, genericOffset int
	destination, inventoryPublication                *answerDocNativePresentationReceipt
	inventoryText                                    string
	publishedMetadata                                map[string]bool
	generic                                          []answerDocObservationRecordPlan
}

func (g *answerDocNativeDeferredGroups) finish(base string, ctx *types.AgentContext, ledger, principal types.ObservationLedger, p *answerDocNativePresentationReceipt) string {
	// Inventory was planned first on its own receipt. These protected groups
	// keep their existing order/budgets and are also constructed only once.
	measurements := ""
	if ctx.AnalysisIR != nil {
		measurements = renderAnswerDocRuntimeMeasurementChoices(ctx, p)
	}
	business := renderAnswerDocScopedBusinessTreeFacts(ctx, ledger, principal, p)
	generic := g.planGeneric(principal, p)
	footer := ""
	if p.navigationOmitted > 0 {
		footer = fmt.Sprintf("- Native presentation reference records omitted=%d by the separate whole-record navigation budget; original references remain in the audit ledger. Omitted navigation is not missing evidence, zero results or complete presentation.\n", p.navigationOmitted)
	}
	inventory := g.inventoryText
	protectedBytes := len(base) + len(measurements) + len(business) + len(footer)
	fullBytes := protectedBytes + generic.bytes(true) + len(inventory)
	withInventory := inventory != "" && p.promptBudget.optionalFits(fullBytes)
	if inventory != "" {
		navigation := renderAnswerDocBudgetedInventoryNavigation(ctx, ledger, principal)
		omittedBytes := protectedBytes + generic.bytes(false) + len(navigation)
		// Omitting a preview can cost MORE when it also removes the complete
		// source dictionary. Compare the two closed ledger plans, not an alias-
		// bearing body with a missing dictionary. Soft is only a preference;
		// the fixed outer reservation and final whole-request guard
		// still apply before this byte-saving complete plan may be sent.
		withInventory = withInventory || fullBytes < omittedBytes && p.promptBudget.requiredFits(fullBytes)
		p.promptBudget.NativeLedgerPlan = &finalizerNativeLedgerPlan{WithInventoryBytes: fullBytes,
			WithoutInventoryBytes: omittedBytes, InventoryPublished: withInventory}
		if !withInventory {
			p.promptBudget.PreviewOmittedBytes += len(inventory)
			p.promptBudget.PreviewOmittedGroups++
			inventory = navigation
		}
	}
	// Assemble exact renderer-owned units. No string search, clipping, second
	// fact compilation or provisional source alias can affect the final text.
	type insertion struct {
		offset int
		text   string
	}
	items := []insertion{{g.inventory, inventory}, {g.measurements, measurements}, {g.business, business}}
	if len(g.generic) > 0 {
		items = append(items, insertion{g.genericOffset, generic.render(withInventory)})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].offset < items[j].offset })
	var out strings.Builder
	pos := 0
	for _, item := range items {
		out.WriteString(base[pos:item.offset])
		out.WriteString(item.text)
		pos = item.offset
	}
	out.WriteString(base[pos:])
	out.WriteString(footer)
	if plan := p.promptBudget.NativeLedgerPlan; plan != nil {
		plan.PublishedLedgerBytes = out.Len()
	}
	if withInventory {
		commitAnswerDocInventoryPresentation(p, g.inventoryPublication)
	}
	*g.destination = *p
	return out.String()
}

// Inventory planning has no sampling/detail/navigation budget side effects.
// Its detached receipt contains only these display-completeness maps. Commit
// them after the complete inventory and all member references survive; no
// original observation, source authority or query population is re-signed.
func commitAnswerDocInventoryPresentation(dst, inventory *answerDocNativePresentationReceipt) {
	merge := func(base, additions map[string]bool) map[string]bool {
		if base == nil && len(additions) > 0 {
			base = make(map[string]bool, len(additions))
		}
		for key, value := range additions {
			if prior, exists := base[key]; exists {
				value = value && prior
			}
			base[key] = value
		}
		return base
	}
	dst.queries = merge(dst.queries, inventory.queries)
	dst.records = merge(dst.records, inventory.records)
	if dst.eventMembers == nil && len(inventory.eventMembers) > 0 {
		dst.eventMembers = make(map[answerDocEventMemberKey]bool, len(inventory.eventMembers))
	}
	for key, shown := range inventory.eventMembers {
		if shown {
			dst.eventMembers[key] = true
		}
	}
}

func renderAnswerDocBudgetedInventoryNavigation(ctx *types.AgentContext, accepted, principal types.ObservationLedger) string {
	ledger, request := answerDocTraceEventInventoryDisplayInput(ctx, accepted, principal)
	requested := ledger.RuntimeArtifactScopeProfile
	if request != nil {
		requested = request.RuntimeArtifactScopeProfile
	}
	var b strings.Builder
	b.WriteString("### Native event preview dependency group not published\n\n")
	b.WriteString("Full source/query receipts below remain navigation and census only. No event row, row_ref or field-value dictionary is supplied by this omitted group. Use an exact native table selector without event_selection.row_ids for its retained original rows; do not invent row IDs, infer missing fields or expand preview timestamps into total/query coverage.\n")
	contents, conflicts := map[string]string{}, map[string]bool{}
	for _, r := range ledger.Records {
		if !types.IsValidTraceEventSearchInventoryRecord(r) {
			continue
		}
		if _, qualified := ledger.RuntimeDisplayScope(r, requested); !qualified {
			continue
		}
		key := nativePresentationQueryKey(r)
		if key == "" {
			continue
		}
		encoded, _ := json.Marshal(r.EventSearchInventory)
		if prior, exists := contents[key]; exists && prior != string(encoded) {
			conflicts[key] = true
		}
		contents[key] = string(encoded)
	}
	seen := map[string]bool{}
	shown := 0
	for _, r := range ledger.Records {
		if !types.IsValidTraceEventSearchInventoryRecord(r) {
			continue
		}
		if _, qualified := ledger.RuntimeDisplayScope(r, requested); !qualified {
			continue
		}
		key := nativePresentationQueryKey(r)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		if shown == traceEventInventoryPromptQueryLimit {
			continue
		}
		shown++
		if conflicts[key] {
			nav, _ := json.Marshal(struct {
				Source   types.ObservationSourceRef `json:"source"`
				Boundary string                     `json:"boundary"`
			}{r.SourceRef, "same_query_payload_conflict; census and rows not published"})
			fmt.Fprintf(&b, "- original_query_navigation=%s\n", nav)
			continue
		}
		v := r.EventSearchInventory
		nav, _ := json.Marshal(struct {
			Source       types.ObservationSourceRef              `json:"source"`
			Query        types.TraceEventSearchInventoryQuery    `json:"query"`
			Coverage     types.TraceEventSearchInventoryCoverage `json:"coverage"`
			RetainedRows int                                     `json:"retained_rows"`
			PromptRows   int                                     `json:"prompt_rows"`
		}{r.SourceRef, v.Query, v.Coverage, len(v.Rows), 0})
		fmt.Fprintf(&b, "- original_query_navigation=%s\n", nav)
	}
	fmt.Fprintf(&b, "- query_navigation_shown=%d; query_navigation_omitted=%d; preview_rows_shown=0. Other complete query originals remain in the lossless ledger.\n", shown, len(seen)-shown)
	b.WriteString("\n")
	return b.String()
}
