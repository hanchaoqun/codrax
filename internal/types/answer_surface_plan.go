package types

import (
	"fmt"
	"sort"
	"strings"
)

// AnswerSurfacePlan is the shared, post-investigation answer-surface
// authority consumed by both the finalizer prompt builder and the
// emit_answer_document validator.
//
// It compiles the current stable investigation state, effective shape,
// effective diagram contract, exact-resolution policy, and the
// categorized answer-surface evidence into one deterministic view so
// downstream stages do not re-derive the same policy independently.
type AnswerSurfacePlan struct {
	RequiredShape                 AnswerShape
	Diagram                       *DiagramContract
	DiagramHardRequirementDropped bool
	CompiledDiagramKind           DiagramKind
	CompiledDiagramFence          string
	StepBackbone                  []StepSurfaceAnchor
	StepBackboneCompleteness      CompletenessClaim
	LogSourceDriftAnchors         []LogSourceDriftAnchor
	LogObservedAnchors            []LogSourceDriftAnchor

	ExactResolution          *ExactResolutionContract
	PreferredExactResolution *AnswerExactResolution
	SummarySurfaceMode       AnswerSummarySurfaceMode

	StableInvestigationResultKind string
	StableAbsenceJustification    string
	StableAbsent                  bool
	ExactContextRequiredFiles     []string

	SurfaceEvidence []EvidenceItem

	AllowedExactContextItems       []EvidenceItem
	CitationGradeExactContextItems []EvidenceItem
	ProseOnlyExactContextItems     []EvidenceItem
	DiagramGradeExactContextItems  []EvidenceItem
	ForbiddenExactContextItems     []EvidenceItem

	AllowedExactContextLabels       []ExactContextSurfaceLabel
	CitationGradeExactContextLabels []ExactContextSurfaceLabel
	ForbiddenExactContextLabels     []ExactContextSurfaceLabel

	RelatedContextCitationCandidates []ConfigTraceRelatedContextCitationCandidate
	ConfigTraceDiagramAnchors        []ConfigTraceDiagramAnchor
}

type AnswerSummarySurfaceMode string

const (
	AnswerSummarySurfaceDefault                 AnswerSummarySurfaceMode = ""
	AnswerSummarySurfaceFollowOnGroundedContext AnswerSummarySurfaceMode = "follow_on_grounded_context_only"
	AnswerSummarySurfaceMinimalScalarRoleLocate AnswerSummarySurfaceMode = "minimal_scalar_role_locate"
	AnswerSummarySurfaceDriftBoundedRootCause   AnswerSummarySurfaceMode = "drift_bounded_root_cause"
)

type ExactContextSurfaceLabel struct {
	Display    string
	MatchLower string
	LookupKey  string
	Kind       string
}

type ConfigTraceDiagramAnchor struct {
	Role  string
	Label string
	Score int
}

type LogSourceDriftAnchor struct {
	File         string
	Func         string
	ObservedLine int
	AnchoredLine int
}

type StepSurfaceAnchor struct {
	Name      string
	File      string
	Line      int
	Kind      AnswerSymbolKind
	Rationale string
	Chain     string
	SurfaceText string
}

func RenderStepSurfaceAnchorDescription(anchor StepSurfaceAnchor) string {
	name := strings.TrimSpace(anchor.Name)
	text := strings.TrimSpace(anchor.SurfaceText)
	if text == "" {
		text = strings.TrimSpace(anchor.Rationale)
	}
	switch {
	case name == "" && text == "":
		return ""
	case name == "":
		return text
	case text == "":
		return fmt.Sprintf("`%s` is one grounded hop in the resolved sequence.", name)
	case strings.Contains(strings.ToLower(text), strings.ToLower(name)):
		return text
	default:
		return fmt.Sprintf("`%s` %s", name, text)
	}
}

func compileStepSurfaceAnchors(symbols []AnswerSymbol) []StepSurfaceAnchor {
	if len(symbols) == 0 {
		return nil
	}
	out := make([]StepSurfaceAnchor, 0, len(symbols))
	for _, sym := range symbols {
		if strings.TrimSpace(sym.Name) == "" {
			continue
		}
		out = append(out, StepSurfaceAnchor{
			Name:      strings.TrimSpace(sym.Name),
			File:      strings.TrimSpace(sym.File),
			Line:      sym.Line,
			Kind:      sym.Kind,
			Rationale: strings.TrimSpace(sym.Rationale),
			Chain:     strings.TrimSpace(sym.Chain),
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func ApplyAnswerSymbolStepBackbone(plan *AnswerSurfacePlan, symbols []AnswerSymbol, claim CompletenessClaim) {
	if plan == nil || plan.RequiredShape != ShapeStepList || len(symbols) == 0 {
		return
	}
	anchors := compileStepSurfaceAnchors(symbols)
	if len(anchors) == 0 {
		return
	}
	plan.StepBackbone = anchors
	if claim != "" {
		plan.StepBackboneCompleteness = claim
	}
}

func ApplyEvidenceStepBackbone(plan *AnswerSurfacePlan, evidence []EvidenceItem) {
	if plan == nil || plan.RequiredShape != ShapeStepList || len(plan.StepBackbone) > 0 || len(evidence) == 0 {
		return
	}
	type group struct {
		file    string
		anchors []StepSurfaceAnchor
	}
	groups := make(map[string][]StepSurfaceAnchor)
	for _, item := range evidence {
		if item.GroundingStatus == GroundingUngrounded || strings.TrimSpace(item.Source) == "" || item.LineStart <= 0 {
			continue
		}
		switch item.Kind {
		case EvidenceDirect, EvidenceConditional, EvidenceRelationship, EvidenceMechanism:
		default:
			continue
		}
		switch item.AnchorKind {
		case AnchorDefinition, AnchorCall, AnchorCondition, AnchorAssignment:
		default:
			continue
		}
		name := firstNonEmptySurfaceString(
			strings.TrimSpace(item.AnchorSymbol),
			strings.TrimSpace(item.Subject),
			strings.TrimSpace(item.Object),
		)
		if name == "" {
			continue
		}
		file := strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`))
		groups[file] = append(groups[file], StepSurfaceAnchor{
			Name:        name,
			File:        file,
			Line:        item.LineStart,
			SurfaceText: strings.TrimSpace(EvidencePreferredSurfaceText(item, nil, true)),
		})
	}
	var best []StepSurfaceAnchor
	for _, anchors := range groups {
		if len(anchors) < 3 {
			continue
		}
		sort.SliceStable(anchors, func(i, j int) bool {
			if anchors[i].Line == anchors[j].Line {
				return anchors[i].Name < anchors[j].Name
			}
			return anchors[i].Line < anchors[j].Line
		})
		dedup := anchors[:0]
		seen := make(map[string]bool, len(anchors))
		for _, anchor := range anchors {
			key := fmt.Sprintf("%s:%d:%s", anchor.File, anchor.Line, anchor.Name)
			if seen[key] {
				continue
			}
			seen[key] = true
			dedup = append(dedup, anchor)
		}
		if len(dedup) > len(best) {
			best = append([]StepSurfaceAnchor(nil), dedup...)
		}
	}
	if len(best) == 0 {
		return
	}
	plan.StepBackbone = best
	if plan.StepBackboneCompleteness == "" {
		plan.StepBackboneCompleteness = CompletenessLowerBound
	}
}

func RenderLinearDiagramFence(nodes []string, limit int) string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(nodes))
	for _, node := range nodes {
		node = strings.TrimSpace(node)
		if node == "" || seen[node] {
			continue
		}
		seen[node] = true
		out = append(out, node)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	if len(out) < 2 {
		return ""
	}
	var b strings.Builder
	b.WriteString("```\n")
	for i, node := range out {
		b.WriteString(node)
		b.WriteByte('\n')
		if i < len(out)-1 {
			b.WriteString("  ->\n")
		}
	}
	b.WriteString("```")
	return b.String()
}

func RenderConfigTraceDiagramFence(anchors []ConfigTraceDiagramAnchor) string {
	nodes := make([]string, 0, len(anchors))
	for _, anchor := range anchors {
		label := strings.TrimSpace(anchor.Label)
		if label != "" {
			nodes = append(nodes, label)
		}
	}
	return RenderLinearDiagramFence(nodes, 0)
}

func RenderLogDiagramFence(bundle *LogBundle) string {
	frames := collectDiagramLogFrames(bundle)
	if len(frames) < 2 {
		return ""
	}
	var b strings.Builder
	b.WriteString("```\n")
	for i, frame := range frames {
		location := fmt.Sprintf("%s:%d", frame.File, frame.Line)
		name := strings.TrimSpace(frame.Func)
		if name == "" {
			name = "(no symbol)"
		}
		switch {
		case i == 0:
			fmt.Fprintf(&b, "innermost failure: %s in %s\n", location, name)
		case i == len(frames)-1:
			fmt.Fprintf(&b, "  -> caller (outermost): %s in %s\n", location, name)
		default:
			fmt.Fprintf(&b, "  -> caller:            %s in %s\n", location, name)
		}
	}
	b.WriteString("```")
	return b.String()
}

func RenderFlowFindingDiagramFence(findings []FlowFindingDigest) string {
	for _, finding := range findings {
		if fence := RenderLinearDiagramFence(flowFindingDiagramNodes(finding), 6); fence != "" {
			return fence
		}
	}
	return ""
}

func RenderAnswerChainDiagramFence(chains []AnswerChain) string {
	nodes := make([]string, 0, len(chains))
	for _, chain := range chains {
		label := firstNonEmptySurfaceString(
			chain.Item.DisplayLocation(true),
			strings.TrimSpace(chain.Item.Source),
			strings.TrimSpace(chain.Item.Subject),
			strings.TrimSpace(chain.Item.AnchorSymbol),
		)
		if label != "" {
			nodes = append(nodes, label)
		}
	}
	return RenderLinearDiagramFence(nodes, 5)
}

func CompileDiagramSurfaceFence(
	dc *DiagramContract,
	scenario Scenario,
	logBundle *LogBundle,
	flowFindings []FlowFindingDigest,
	answerChains []AnswerChain,
	configAnchors []ConfigTraceDiagramAnchor,
) (DiagramKind, string) {
	for _, kind := range diagramSurfaceKinds(dc) {
		switch kind {
		case DiagramFlow:
			if scenario == ScenarioConfigTrace {
				if fence := RenderConfigTraceDiagramFence(configAnchors); fence != "" {
					return kind, fence
				}
			}
			if fence := RenderFlowFindingDiagramFence(flowFindings); fence != "" {
				return kind, fence
			}
		case DiagramSequence, DiagramCallDAG:
			if fence := RenderLogDiagramFence(logBundle); fence != "" {
				return kind, fence
			}
			if kind == DiagramCallDAG {
				if fence := RenderFlowFindingDiagramFence(flowFindings); fence != "" {
					return kind, fence
				}
			}
		case DiagramArchitecture:
			if fence := RenderAnswerChainDiagramFence(answerChains); fence != "" {
				return kind, fence
			}
		}
	}
	return DiagramNone, ""
}

// BuildAnswerSurfacePlan compiles the current answer-surface authority
// from the already-grounded analysis and investigation state. It is a
// pure helper shared by the finalizer prompt path and the structured
// answer validator so both consume the same effective shape,
// diagram/exact-resolution contract, and context-surface categories.
func BuildAnswerSurfacePlan(
	ir *AnalysisIR,
	mutable *MutableState,
	logBundle *LogBundle,
	flowFindings []FlowFindingDigest,
	answerChains []AnswerChain,
	evidence []EvidenceItem,
) *AnswerSurfacePlan {
	if ir == nil {
		return nil
	}

	plan := &AnswerSurfacePlan{
		RequiredShape: EffectiveRequiredAnswerShape(ir, mutable),
	}
	if plan.RequiredShape == ShapeNone {
		plan.RequiredShape = ir.AnswerContract.RequiredAnswerShape
	}

	plan.ExactResolution = ir.AnswerContract.ExactResolution
	if plan.ExactResolution == nil {
		plan.ExactResolution = BuildExactResolutionContract(ir.RequestModel)
	}

	if mutable != nil {
		plan.StableInvestigationResultKind = strings.TrimSpace(mutable.StableInvestigationResultKind())
		plan.StableAbsenceJustification = strings.TrimSpace(mutable.StableAbsenceJustification())
		plan.ExactContextRequiredFiles = mutable.ExactContextRequiredFiles()
		if syms, claim := mutable.EmittedAnswerSymbols(); len(syms) > 0 {
			ApplyAnswerSymbolStepBackbone(plan, syms, claim)
		}
		if logBundle == nil {
			logBundle = mutable.LogTriage()
		}
	}
	plan.StableAbsent = strings.EqualFold(plan.StableInvestigationResultKind, "absence") &&
		plan.StableAbsenceJustification != ""

	emitted := []EvidenceItem(nil)
	if mutable != nil {
		emitted = mutable.EmittedEvidence()
	}
	plan.SurfaceEvidence = ExactResolutionSurfaceEvidencePool(emitted, evidence, answerChains)
	ApplyEvidenceStepBackbone(plan, plan.SurfaceEvidence)

	supported := SupportedDiagramKindsForAnswer(
		ir.RequestModel.Scenario,
		plan.StableAbsent,
		plan.ExactResolution,
		plan.ExactContextRequiredFiles,
		logBundle,
		flowFindings,
		answerChains,
		plan.SurfaceEvidence,
	)
	plan.Diagram = EffectiveDiagramContract(ir.AnswerContract.Diagram, supported)
	plan.DiagramHardRequirementDropped = ir.AnswerContract.Diagram != nil &&
		ir.AnswerContract.Diagram.Required &&
		(plan.Diagram == nil || !plan.Diagram.Required)

	plan.ConfigTraceDiagramAnchors = CollectConfigTraceDiagramAnchors(
		ir.RequestModel.Scenario,
		plan.StableAbsent,
		plan.ExactResolution,
		plan.ExactContextRequiredFiles,
		plan.SurfaceEvidence,
	)
	plan.CompiledDiagramKind, plan.CompiledDiagramFence = CompileDiagramSurfaceFence(
		plan.Diagram,
		ir.RequestModel.Scenario,
		logBundle,
		flowFindings,
		answerChains,
		plan.ConfigTraceDiagramAnchors,
	)
	plan.LogObservedAnchors = CollectLogObservedAnchors(
		ir.RequestModel,
		logBundle,
		plan.SurfaceEvidence,
	)
	plan.LogSourceDriftAnchors = CollectLogSourceDriftAnchors(
		ir.RequestModel,
		logBundle,
		plan.SurfaceEvidence,
	)
	plan.PreferredExactResolution = preferredExactResolutionSurface(plan)
	plan.SummarySurfaceMode = preferredAnswerSummarySurfaceMode(plan, ir.RequestModel)
	if plan.ExactResolution == nil || len(plan.ExactResolution.Targets) == 0 {
		return plan
	}

	plan.AllowedExactContextItems = collectAllowedExactContextItems(
		plan.ExactResolution,
		ir.RequestModel.Scenario,
		plan.StableAbsent,
		plan.SurfaceEvidence,
		plan.ExactContextRequiredFiles,
	)
	plan.CitationGradeExactContextItems = collectCitationGradeExactContextItems(
		plan.ExactResolution,
		ir.RequestModel.Scenario,
		plan.StableAbsent,
		plan.SurfaceEvidence,
		plan.ExactContextRequiredFiles,
	)
	plan.ProseOnlyExactContextItems = collectProseOnlyExactContextItems(
		plan.ExactResolution,
		ir.RequestModel.Scenario,
		plan.StableAbsent,
		plan.SurfaceEvidence,
		plan.ExactContextRequiredFiles,
	)
	plan.DiagramGradeExactContextItems = collectDiagramGradeExactContextItems(
		plan.ExactResolution,
		ir.RequestModel.Scenario,
		plan.StableAbsent,
		plan.SurfaceEvidence,
		plan.ExactContextRequiredFiles,
	)
	plan.ForbiddenExactContextItems = collectForbiddenExactContextItems(
		plan.ExactResolution,
		ir.RequestModel.Scenario,
		plan.StableAbsent,
		plan.SurfaceEvidence,
		plan.ExactContextRequiredFiles,
	)
	plan.AllowedExactContextLabels = collectAllowedExactContextLabels(
		plan.ExactResolution,
		ir.RequestModel.Scenario,
		plan.StableAbsent,
		plan.SurfaceEvidence,
		plan.ExactContextRequiredFiles,
	)
	plan.CitationGradeExactContextLabels = collectCitationGradeExactContextLabels(
		plan.ExactResolution,
		ir.RequestModel.Scenario,
		plan.StableAbsent,
		plan.SurfaceEvidence,
		plan.ExactContextRequiredFiles,
	)
	plan.ForbiddenExactContextLabels = collectForbiddenExactContextLabels(
		plan.ExactResolution,
		ir.RequestModel.Scenario,
		plan.StableAbsent,
		plan.SurfaceEvidence,
		plan.ExactContextRequiredFiles,
		plan.AllowedExactContextLabels,
	)
	plan.RelatedContextCitationCandidates = ConfigTraceRelatedContextCitationCandidates(
		plan.ExactResolution,
		plan.ExactContextRequiredFiles,
		plan.SurfaceEvidence,
	)
	plan.PreferredExactResolution = preferredExactResolutionSurface(plan)
	plan.SummarySurfaceMode = preferredAnswerSummarySurfaceMode(plan, ir.RequestModel)

	return plan
}

func preferredExactResolutionSurface(plan *AnswerSurfacePlan) *AnswerExactResolution {
	if plan == nil || plan.ExactResolution == nil {
		return nil
	}
	resolved := &AnswerExactResolution{
		ContextMode: AnswerExactResolutionContextNone,
	}
	if plan.StableAbsent {
		resolved.Status = AnswerExactResolutionAbsent
		if hasVisibleNearbyGroundedContext(plan) {
			resolved.ContextMode = AnswerExactResolutionContextGroundedOnly
		}
		return resolved
	}
	return nil
}

func preferredAnswerSummarySurfaceMode(plan *AnswerSurfacePlan, rm RequestModel) AnswerSummarySurfaceMode {
	if plan == nil {
		if IsScalarRoleLocateLookup(rm) {
			return AnswerSummarySurfaceMinimalScalarRoleLocate
		}
		return AnswerSummarySurfaceDefault
	}
	if plan.PreferredExactResolution == nil {
		if rm.Scenario == ScenarioRootCause &&
			len(plan.LogSourceDriftAnchors) > 0 &&
			(plan.RequiredShape == ShapeStepList || plan.RequiredShape == ShapeExplanation) {
			return AnswerSummarySurfaceDriftBoundedRootCause
		}
		if IsScalarRoleLocateLookup(rm) {
			return AnswerSummarySurfaceMinimalScalarRoleLocate
		}
		return AnswerSummarySurfaceDefault
	}
	if rm.Scenario == ScenarioRootCause &&
		len(plan.LogSourceDriftAnchors) > 0 &&
		(plan.RequiredShape == ShapeStepList || plan.RequiredShape == ShapeExplanation) {
		return AnswerSummarySurfaceDriftBoundedRootCause
	}
	if plan.PreferredExactResolution.Status == AnswerExactResolutionAbsent &&
		plan.PreferredExactResolution.ContextMode == AnswerExactResolutionContextGroundedOnly &&
		hasVisibleNearbyGroundedContext(plan) {
		return AnswerSummarySurfaceFollowOnGroundedContext
	}
	if IsScalarRoleLocateLookup(rm) {
		return AnswerSummarySurfaceMinimalScalarRoleLocate
	}
	return AnswerSummarySurfaceDefault
}

func hasVisibleNearbyGroundedContext(plan *AnswerSurfacePlan) bool {
	if plan == nil {
		return false
	}
	for _, ev := range plan.CitationGradeExactContextItems {
		if ev.ContextRole != EvidenceContextRoleAbsenceSupport {
			return true
		}
	}
	for _, ev := range plan.ProseOnlyExactContextItems {
		if ev.ContextRole != EvidenceContextRoleAbsenceSupport {
			return true
		}
	}
	return false
}

// FormatExactResolutionSeed formats one grounded evidence item into the
// human-readable seed text used in finalizer prompts and repair hints.
func FormatExactResolutionSeed(ev EvidenceItem) string {
	parts := make([]string, 0, 3)
	if triple := strings.TrimSpace(strings.Join(filterEmptySurfaceStrings(ev.Subject, ev.Predicate, ev.Object), " ")); triple != "" {
		parts = append(parts, triple)
	}
	if summary := strings.TrimSpace(ev.Summary); summary != "" {
		if len(parts) == 0 || !strings.Contains(summary, parts[0]) {
			parts = append(parts, summary)
		}
	}
	text := strings.Join(parts, " - ")
	if text == "" {
		text = strings.TrimSpace(ev.Source)
	}
	if ev.Source != "" {
		if ev.LineStart > 0 {
			text += fmt.Sprintf(" (%s:%d)", ev.Source, ev.LineStart)
		} else {
			text += fmt.Sprintf(" (%s)", ev.Source)
		}
	}
	return text
}

func JoinExactContextSurfaceDisplays(items []ExactContextSurfaceLabel) string {
	if len(items) == 0 {
		return ""
	}
	seen := make(map[string]bool)
	var out []string
	for _, item := range items {
		display := strings.TrimSpace(item.Display)
		if display == "" || seen[display] {
			continue
		}
		seen[display] = true
		out = append(out, display)
	}
	sort.Strings(out)
	if len(out) > 6 {
		out = out[:6]
	}
	return strings.Join(out, ", ")
}

func ExactContextSurfaceLabelsForItem(contract *ExactResolutionContract, item EvidenceItem) []ExactContextSurfaceLabel {
	return exactContextSurfaceLabelsForItem(contract, item)
}

func CollectConfigTraceDiagramAnchors(
	scenario Scenario,
	stableAbsent bool,
	contract *ExactResolutionContract,
	requiredFiles []string,
	items []EvidenceItem,
) []ConfigTraceDiagramAnchor {
	if scenario != ScenarioConfigTrace || len(items) == 0 {
		return nil
	}
	roleOrder := []EvidenceDiagramRole{
		EvidenceDiagramRoleOverride,
		EvidenceDiagramRoleConfig,
		EvidenceDiagramRoleRuntime,
		EvidenceDiagramRoleDefault,
	}
	best := make(map[string]ConfigTraceDiagramAnchor, len(roleOrder))
	for _, ev := range items {
		role := configTraceSeedDiagramRoleInFiles(contractForConfigTraceSurfacePlan(contract, stableAbsent), ev, requiredFiles)
		if role == EvidenceDiagramRoleUnknown {
			continue
		}
		label := strings.TrimSpace(ev.Source)
		if label == "" {
			continue
		}
		if ev.LineStart > 0 {
			label = fmt.Sprintf("%s:%d", label, ev.LineStart)
		}
		score := configTraceCitationCandidateScore(role)
		if ev.LineStart > 0 {
			score += 2
		}
		key := string(role)
		if cur, ok := best[key]; ok && cur.Score >= score {
			continue
		}
		best[key] = ConfigTraceDiagramAnchor{
			Role:  key,
			Label: label,
			Score: score,
		}
	}
	var out []ConfigTraceDiagramAnchor
	for _, role := range roleOrder {
		if anchor, ok := best[string(role)]; ok {
			out = append(out, anchor)
		}
	}
	return out
}

func diagramSurfaceKinds(dc *DiagramContract) []DiagramKind {
	if dc != nil && len(dc.PreferredKinds) > 0 {
		out := make([]DiagramKind, 0, len(dc.PreferredKinds))
		for _, kind := range dc.PreferredKinds {
			if kind == DiagramNone {
				continue
			}
			out = append(out, kind)
		}
		if len(out) > 0 {
			return out
		}
	}
	return []DiagramKind{
		DiagramFlow,
		DiagramSequence,
		DiagramCallDAG,
		DiagramArchitecture,
	}
}

const logSourceDriftLineGap = 48

func collectDiagramLogFrames(bundle *LogBundle) []LogFrame {
	if bundle == nil || len(bundle.Errors) == 0 {
		return nil
	}
	resolved := make([]LogFrame, 0, 8)
	for _, err := range bundle.Errors {
		for _, frame := range err.Frames {
			if frame.File == "" || frame.Line <= 0 {
				continue
			}
			resolved = append(resolved, frame)
			if len(resolved) >= 8 {
				return resolved
			}
		}
	}
	return resolved
}

func CollectLogObservedAnchors(rm RequestModel, bundle *LogBundle, items []EvidenceItem) []LogSourceDriftAnchor {
	return collectLogSourceAnchors(rm, bundle, items, 0)
}

func CollectLogSourceDriftAnchors(rm RequestModel, bundle *LogBundle, items []EvidenceItem) []LogSourceDriftAnchor {
	return collectLogSourceAnchors(rm, bundle, items, logSourceDriftLineGap)
}

func collectLogSourceAnchors(rm RequestModel, bundle *LogBundle, items []EvidenceItem, minGap int) []LogSourceDriftAnchor {
	if (rm.Scenario != ScenarioRootCause && rm.Intent != IntentRootCause) || bundle == nil || len(items) == 0 {
		return nil
	}
	frames := collectDiagramLogFrames(bundle)
	if len(frames) == 0 {
		return nil
	}

	byFile := make(map[string][]EvidenceItem)
	for _, item := range items {
		source := strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`))
		if source == "" || item.LineStart <= 0 {
			continue
		}
		byFile[source] = append(byFile[source], item)
	}

	var out []LogSourceDriftAnchor
	seen := make(map[string]bool)
	for _, frame := range frames {
		file := strings.TrimSpace(strings.ReplaceAll(frame.File, `\`, `/`))
		if file == "" || frame.Line <= 0 {
			continue
		}
		candidates := byFile[file]
		if len(candidates) == 0 {
			continue
		}
		tail := normalizedSurfaceSymbolTail(frame.Func)
		if tail == "" {
			continue
		}
		bestLine, bestDelta := nearestLogSourceDriftAnchorLine(candidates, tail, frame.Line)
		if bestLine == 0 || bestDelta <= minGap {
			continue
		}
		key := fmt.Sprintf("%s|%s|%d|%d", file, tail, frame.Line, bestLine)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, LogSourceDriftAnchor{
			File:         file,
			Func:         strings.TrimSpace(frame.Func),
			ObservedLine: frame.Line,
			AnchoredLine: bestLine,
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		if out[i].ObservedLine != out[j].ObservedLine {
			return out[i].ObservedLine < out[j].ObservedLine
		}
		return out[i].AnchoredLine < out[j].AnchoredLine
	})
	return out
}

func nearestLogSourceDriftAnchorLine(candidates []EvidenceItem, tail string, observedLine int) (bestLine, bestDelta int) {
	bestLine, bestDelta = nearestLogSourceDriftAnchorLineByPass(candidates, tail, observedLine, true)
	if bestLine != 0 {
		return bestLine, bestDelta
	}
	return nearestLogSourceDriftAnchorLineByPass(candidates, tail, observedLine, false)
}

func nearestLogSourceDriftAnchorLineByPass(candidates []EvidenceItem, tail string, observedLine int, definitionOnly bool) (bestLine, bestDelta int) {
	for _, item := range candidates {
		if definitionOnly && item.AnchorKind != AnchorDefinition {
			continue
		}
		for _, candidateTail := range logSourceDriftCandidateTails(item, definitionOnly) {
			if candidateTail != tail {
				continue
			}
			line := item.LineStart
			if line <= 0 {
				continue
			}
			delta := absInt(line - observedLine)
			if bestLine == 0 || delta < bestDelta {
				bestLine = line
				bestDelta = delta
			}
		}
	}
	return bestLine, bestDelta
}

func logSourceDriftCandidateTails(item EvidenceItem, definitionOnly bool) []string {
	var raws []string
	if item.AnchorKind == AnchorDefinition {
		raws = append(raws, item.AnchorSymbol, item.Subject)
	} else if !definitionOnly {
		raws = append(raws, item.AnchorSymbol)
	}
	var out []string
	seen := make(map[string]bool)
	for _, raw := range raws {
		tail := normalizedSurfaceSymbolTail(raw)
		if tail == "" || seen[tail] {
			continue
		}
		seen[tail] = true
		out = append(out, tail)
	}
	return out
}

func evidenceSurfaceSymbolTails(item EvidenceItem) []string {
	var out []string
	seen := make(map[string]bool)
	for _, raw := range []string{item.AnchorSymbol, item.Subject, item.Object} {
		tail := normalizedSurfaceSymbolTail(raw)
		if tail == "" || seen[tail] {
			continue
		}
		seen[tail] = true
		out = append(out, tail)
	}
	return out
}

func NormalizedSurfaceSymbolTail(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if fields := strings.Fields(raw); len(fields) > 0 {
		raw = fields[0]
	}
	raw = strings.TrimSuffix(raw, "()")
	raw = strings.Trim(raw, "`")
	if idx := strings.LastIndex(raw, "::"); idx >= 0 {
		raw = raw[idx+2:]
	}
	if idx := strings.LastIndexAny(raw, `/\`); idx >= 0 {
		raw = raw[idx+1:]
	}
	if idx := strings.LastIndex(raw, "."); idx >= 0 {
		raw = raw[idx+1:]
	}
	raw = strings.Trim(raw, "()")
	raw = strings.TrimLeft(raw, "*&")
	return strings.ToLower(strings.TrimSpace(raw))
}

func normalizedSurfaceSymbolTail(raw string) string {
	return NormalizedSurfaceSymbolTail(raw)
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func flowFindingDiagramNodes(ff FlowFindingDigest) []string {
	var nodes []string
	nodes = append(nodes, ff.Path...)
	if len(nodes) < 2 {
		for _, hop := range ff.Hops {
			for _, part := range strings.Split(hop, "->") {
				part = strings.TrimSpace(part)
				if part != "" {
					nodes = append(nodes, part)
				}
			}
		}
	}
	if len(nodes) < 2 {
		nodes = append(nodes, ff.Sources...)
		nodes = append(nodes, ff.Sinks...)
	}
	return nodes
}

func firstNonEmptySurfaceString(items ...string) string {
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			return item
		}
	}
	return ""
}

func configTraceSeedDiagramRoleInFiles(contract *ExactResolutionContract, item EvidenceItem, requiredFiles []string) EvidenceDiagramRole {
	if item.Source == "" {
		return EvidenceDiagramRoleUnknown
	}
	if item.ContextRole == EvidenceContextRoleIllustrativeOnly ||
		item.ContextRole == EvidenceContextRoleAbsenceSupport ||
		item.Kind == EvidenceUnresolved ||
		item.Kind == EvidenceTruncated ||
		item.GroundingStatus == GroundingUngrounded {
		return EvidenceDiagramRoleUnknown
	}
	switch item.DiagramRole {
	case EvidenceDiagramRoleConfig:
		if LooksLikeConfigFilePath(item.Source) &&
			configTraceDiagramEvidenceWithinScope(contract, item, requiredFiles) {
			return item.DiagramRole
		}
	case EvidenceDiagramRoleDefault, EvidenceDiagramRoleRuntime, EvidenceDiagramRoleOverride:
		if item.Source != "" &&
			!LooksLikeConfigFilePath(item.Source) &&
			!LooksLikeAuxiliaryEvidencePath(item.Source) &&
			configTraceDiagramEvidenceWithinScope(contract, item, requiredFiles) {
			return item.DiagramRole
		}
	}
	return EvidenceDiagramRoleUnknown
}

func collectAllowedExactContextItems(
	contract *ExactResolutionContract,
	scenario Scenario,
	stableAbsent bool,
	items []EvidenceItem,
	requiredFiles []string,
) []EvidenceItem {
	var out []EvidenceItem
	seen := make(map[string]bool)
	for _, ev := range items {
		if !ExactResolutionAnswerContextAnchorAllowedInFiles(contract, scenario, stableAbsent, ev, requiredFiles) {
			continue
		}
		key := exactContextItemKey(ev)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, ev)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if scoreAllowedExactContextItem(out[i]) != scoreAllowedExactContextItem(out[j]) {
			return scoreAllowedExactContextItem(out[i]) > scoreAllowedExactContextItem(out[j])
		}
		return exactContextItemKey(out[i]) < exactContextItemKey(out[j])
	})
	return out
}

func collectProseOnlyExactContextItems(
	contract *ExactResolutionContract,
	scenario Scenario,
	stableAbsent bool,
	items []EvidenceItem,
	requiredFiles []string,
) []EvidenceItem {
	if contract == nil {
		return nil
	}
	var out []EvidenceItem
	seen := make(map[string]bool)
	for _, ev := range items {
		if !ExactResolutionAnswerContextAnchorAllowedInFiles(contract, scenario, stableAbsent, ev, requiredFiles) {
			continue
		}
		if ExactResolutionCitationContextAnchorAllowedInFiles(contract, scenario, stableAbsent, ev, requiredFiles) {
			continue
		}
		key := exactContextItemKey(ev)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, ev)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return exactContextItemKey(out[i]) < exactContextItemKey(out[j])
	})
	return out
}

func collectCitationGradeExactContextItems(
	contract *ExactResolutionContract,
	scenario Scenario,
	stableAbsent bool,
	items []EvidenceItem,
	requiredFiles []string,
) []EvidenceItem {
	if contract == nil {
		return nil
	}
	var out []EvidenceItem
	seen := make(map[string]bool)
	for _, ev := range items {
		if !ExactResolutionCitationContextAnchorAllowedInFiles(contract, scenario, stableAbsent, ev, requiredFiles) {
			continue
		}
		key := exactContextItemKey(ev)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, ev)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if scoreAllowedExactContextItem(out[i]) != scoreAllowedExactContextItem(out[j]) {
			return scoreAllowedExactContextItem(out[i]) > scoreAllowedExactContextItem(out[j])
		}
		return exactContextItemKey(out[i]) < exactContextItemKey(out[j])
	})
	return out
}

func collectDiagramGradeExactContextItems(
	contract *ExactResolutionContract,
	scenario Scenario,
	stableAbsent bool,
	items []EvidenceItem,
	requiredFiles []string,
) []EvidenceItem {
	if contract == nil || scenario != ScenarioConfigTrace || contract.TargetKind != SubjectConfigKey || !stableAbsent {
		return nil
	}
	var out []EvidenceItem
	seen := make(map[string]bool)
	for _, ev := range items {
		if !ConfigTraceGroundedContextAnchorAllowedInFiles(contract, ev, requiredFiles) {
			continue
		}
		key := exactContextItemKey(ev)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, ev)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if scoreAllowedExactContextItem(out[i]) != scoreAllowedExactContextItem(out[j]) {
			return scoreAllowedExactContextItem(out[i]) > scoreAllowedExactContextItem(out[j])
		}
		return exactContextItemKey(out[i]) < exactContextItemKey(out[j])
	})
	return out
}

func collectForbiddenExactContextItems(
	contract *ExactResolutionContract,
	scenario Scenario,
	stableAbsent bool,
	items []EvidenceItem,
	requiredFiles []string,
) []EvidenceItem {
	if contract == nil || !stableAbsent {
		return nil
	}
	var out []EvidenceItem
	seen := make(map[string]bool)
	for _, ev := range items {
		if !ExactResolutionContextSurfaceRelevant(contract, ev) {
			continue
		}
		if ExactResolutionAnswerContextAnchorAllowedInFiles(contract, scenario, stableAbsent, ev, requiredFiles) {
			continue
		}
		key := exactContextItemKey(ev)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, ev)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if scoreForbiddenExactContextItem(out[i]) != scoreForbiddenExactContextItem(out[j]) {
			return scoreForbiddenExactContextItem(out[i]) > scoreForbiddenExactContextItem(out[j])
		}
		return exactContextItemKey(out[i]) < exactContextItemKey(out[j])
	})
	return out
}

func collectAllowedExactContextLabels(
	contract *ExactResolutionContract,
	scenario Scenario,
	stableAbsent bool,
	items []EvidenceItem,
	requiredFiles []string,
) []ExactContextSurfaceLabel {
	return exactContextSurfaceLabelsFromItems(
		contract,
		collectAllowedExactContextItems(contract, scenario, stableAbsent, items, requiredFiles),
		nil,
	)
}

func collectCitationGradeExactContextLabels(
	contract *ExactResolutionContract,
	scenario Scenario,
	stableAbsent bool,
	items []EvidenceItem,
	requiredFiles []string,
) []ExactContextSurfaceLabel {
	return exactContextSurfaceLabelsFromItems(
		contract,
		collectCitationGradeExactContextItems(contract, scenario, stableAbsent, items, requiredFiles),
		nil,
	)
}

func collectForbiddenExactContextLabels(
	contract *ExactResolutionContract,
	scenario Scenario,
	stableAbsent bool,
	items []EvidenceItem,
	requiredFiles []string,
	allowed []ExactContextSurfaceLabel,
) []ExactContextSurfaceLabel {
	return exactContextSurfaceLabelsFromItemsWithKinds(
		contract,
		collectForbiddenExactContextItems(contract, scenario, stableAbsent, items, requiredFiles),
		allowed,
		true,
		true,
	)
}

func exactContextSurfaceLabelsFromItems(contract *ExactResolutionContract, items []EvidenceItem, allowed []ExactContextSurfaceLabel) []ExactContextSurfaceLabel {
	return exactContextSurfaceLabelsFromItemsWithKinds(contract, items, allowed, true, true)
}

func exactContextSurfaceLabelsFromItemsWithKinds(
	contract *ExactResolutionContract,
	items []EvidenceItem,
	allowed []ExactContextSurfaceLabel,
	includeSymbols bool,
	includePaths bool,
) []ExactContextSurfaceLabel {
	if contract == nil || len(items) == 0 {
		return nil
	}
	allowedKeys := make(map[string]bool, len(allowed))
	for _, label := range allowed {
		if label.LookupKey == "" {
			continue
		}
		allowedKeys[label.Kind+"|"+label.LookupKey] = true
	}
	seen := make(map[string]bool)
	var out []ExactContextSurfaceLabel
	for _, item := range items {
		for _, label := range exactContextSurfaceLabelsForItemWithKinds(contract, item, includeSymbols, includePaths) {
			key := label.Kind + "|" + label.LookupKey
			if label.Display == "" || label.MatchLower == "" || seen[label.MatchLower] || allowedKeys[key] {
				continue
			}
			seen[label.MatchLower] = true
			out = append(out, label)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Display < out[j].Display
	})
	return out
}

func exactContextSurfaceLabelsForItem(contract *ExactResolutionContract, item EvidenceItem) []ExactContextSurfaceLabel {
	return exactContextSurfaceLabelsForItemWithKinds(contract, item, true, true)
}

func exactContextSurfaceLabelsForItemWithKinds(
	contract *ExactResolutionContract,
	item EvidenceItem,
	includeSymbols bool,
	includePaths bool,
) []ExactContextSurfaceLabel {
	var out []ExactContextSurfaceLabel
	appendLabel := func(kind, display string) {
		display = strings.TrimSpace(display)
		if display == "" {
			return
		}
		lookupKey := ExactResolutionLookupKey(kind, display)
		if lookupKey == "" {
			return
		}
		for _, target := range contract.Targets {
			if lookupKey == ExactResolutionLookupKey(kind, target) {
				return
			}
		}
		out = append(out, ExactContextSurfaceLabel{
			Display:    display,
			MatchLower: strings.ToLower(display),
			LookupKey:  lookupKey,
			Kind:       kind,
		})
	}
	if includeSymbols && item.AnchorKind != AnchorImport {
		appendLabel("symbol", item.AnchorSymbol)
		appendLabel("symbol", item.Subject)
	}
	if includePaths {
		appendLabel("path", item.Source)
	}
	return out
}

func exactContextItemKey(ev EvidenceItem) string {
	return fmt.Sprintf("%s:%d:%s:%s", ev.Source, ev.LineStart, ev.AnchorSymbol, ev.Summary)
}

func scoreAllowedExactContextItem(ev EvidenceItem) int {
	switch ev.ContextRole {
	case EvidenceContextRoleAbsenceSupport:
		return 40
	}
	switch ev.DiagramRole {
	case EvidenceDiagramRoleOverride:
		return 36
	case EvidenceDiagramRoleConfig:
		return 34
	case EvidenceDiagramRoleRuntime:
		return 32
	case EvidenceDiagramRoleDefault:
		return 30
	default:
		return 18
	}
}

func scoreForbiddenExactContextItem(ev EvidenceItem) int {
	score := 8
	if ev.ContextRole == EvidenceContextRoleIllustrativeOnly {
		score += 8
	}
	if LooksLikeAuxiliaryEvidencePath(ev.Source) {
		score += 4
	}
	return score
}

func contractForConfigTraceSurfacePlan(contract *ExactResolutionContract, stableAbsent bool) *ExactResolutionContract {
	if !stableAbsent {
		return nil
	}
	return contract
}

func filterEmptySurfaceStrings(items ...string) []string {
	var out []string
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			out = append(out, strings.TrimSpace(item))
		}
	}
	return out
}
