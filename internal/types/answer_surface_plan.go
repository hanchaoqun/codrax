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
	RequiredShape                  AnswerShape
	RequestedEnumerationBoundary   *RequestedEnumerationBoundary
	Diagram                        *DiagramContract
	DiagramHardRequirementDropped  bool
	CompiledDiagramKind            DiagramKind
	CompiledDiagramFence           string
	StepBackbone                   []StepSurfaceAnchor
	StepBackboneCompleteness       CompletenessClaim
	ExplanationAnchorBackbone      []StepSurfaceAnchor
	ExplanationAnchorCompleteness  CompletenessClaim
	ExplanationAnchorMissingTopics []string
	LogSourceDriftAnchors          []LogSourceDriftAnchor
	LogObservedAnchors             []LogSourceDriftAnchor
	DriftBoundedSurfaceItems       []EvidenceItem

	ExactResolution          *ExactResolutionContract
	PreferredExactResolution *AnswerExactResolution
	SummarySurfaceMode       AnswerSummarySurfaceMode

	StableInvestigationResultKind string
	StableAbsenceJustification    string
	StableInvestigationReason     string
	StableAbsent                  bool
	ExactContextRequiredFiles     []string
	CapabilitySurface             *CapabilitySurfaceHint
	CapabilityAuthorityFiles      []string

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
	Role   string
	Label  string
	Source string
	Line   int
	Score  int
}

type LogSourceDriftAnchor struct {
	File         string
	Func         string
	ObservedLine int
	AnchoredLine int

	// OriginalFile carries the LOG-side path when Tier 3
	// (cross-file move) detected the source moved between
	// the build that produced the log and the current
	// checkout. Empty for Tier 1 (line drift) and Tier 2
	// (rename within same file). Renderers can show "log
	// pointed at OriginalFile, current code lives at File"
	// when this is non-empty.
	OriginalFile string

	// OriginalFunc carries the LOG-side function/symbol
	// name when Tier 2 (tail rename) detected the function
	// was renamed. Empty for Tier 1 and Tier 3.
	OriginalFunc string

	// Reason classifies why this anchor was emitted so the
	// downstream renderer can pick prose:
	//   line_drift  — same file + same function, line numbers shifted
	//   tail_rename — same file, function name changed
	//   file_moved  — function lives in a different file now
	// Empty / "" treated as line_drift for backward compat
	// with anchors emitted before this field existed.
	Reason DriftReason
}

// DriftReason classifies log-source drift for renderer
// dispatch. Strings (rather than int enum) so the values
// survive YAML / JSON round-trips and tests stay readable.
type DriftReason string

const (
	DriftReasonLineDrift  DriftReason = "line_drift"
	DriftReasonTailRename DriftReason = "tail_rename"
	DriftReasonFileMoved  DriftReason = "file_moved"
)

type StepSurfaceAnchor struct {
	Name        string
	File        string
	Line        int
	Kind        AnswerSymbolKind
	Rationale   string
	Chain       string
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

func compileEvidenceStepBackbone(evidence []EvidenceItem) []StepSurfaceAnchor {
	if len(evidence) == 0 {
		return nil
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
	return best
}

func stepBackboneIsBoundedPrincipal(plan *AnswerSurfacePlan) bool {
	if plan == nil || plan.RequestedEnumerationBoundary == nil {
		return false
	}
	return plan.RequestedEnumerationBoundary.DeclaredCount > 0
}

func mergeStepBackboneAnchors(base []StepSurfaceAnchor, extra []StepSurfaceAnchor) []StepSurfaceAnchor {
	if len(base) == 0 {
		return append([]StepSurfaceAnchor(nil), extra...)
	}
	if len(extra) == 0 {
		return append([]StepSurfaceAnchor(nil), base...)
	}
	merged := append([]StepSurfaceAnchor(nil), base...)
	seen := make(map[string]bool, len(merged))
	for _, anchor := range merged {
		key := fmt.Sprintf("%s:%d", strings.TrimSpace(strings.ReplaceAll(anchor.File, `\`, `/`)), anchor.Line)
		if key != ":" && key != "" {
			seen[key] = true
		}
	}
	for _, anchor := range extra {
		file := strings.TrimSpace(strings.ReplaceAll(anchor.File, `\`, `/`))
		key := fmt.Sprintf("%s:%d", file, anchor.Line)
		if file == "" || anchor.Line <= 0 || seen[key] {
			continue
		}
		insertAt := -1
		lastSameFile := -1
		for i, existing := range merged {
			existingFile := strings.TrimSpace(strings.ReplaceAll(existing.File, `\`, `/`))
			if existingFile != file {
				continue
			}
			lastSameFile = i
			if existing.Line > anchor.Line {
				insertAt = i
				break
			}
		}
		switch {
		case insertAt >= 0:
			merged = append(merged[:insertAt], append([]StepSurfaceAnchor{anchor}, merged[insertAt:]...)...)
		case lastSameFile >= 0:
			pos := lastSameFile + 1
			merged = append(merged[:pos], append([]StepSurfaceAnchor{anchor}, merged[pos:]...)...)
		default:
			merged = append(merged, anchor)
		}
		seen[key] = true
	}
	return merged
}

func ApplyEvidenceStepBackbone(plan *AnswerSurfacePlan, evidence []EvidenceItem) {
	if plan == nil || plan.RequiredShape != ShapeStepList || len(evidence) == 0 {
		return
	}
	best := compileEvidenceStepBackbone(evidence)
	if len(best) == 0 {
		return
	}
	if len(plan.StepBackbone) == 0 {
		plan.StepBackbone = best
		if plan.StepBackboneCompleteness == "" {
			plan.StepBackboneCompleteness = CompletenessLowerBound
		}
		return
	}
	if plan.StepBackboneCompleteness == CompletenessComplete || stepBackboneIsBoundedPrincipal(plan) {
		return
	}
	plan.StepBackbone = mergeStepBackboneAnchors(plan.StepBackbone, best)
	if plan.StepBackboneCompleteness == "" {
		plan.StepBackboneCompleteness = CompletenessLowerBound
	}
}

func applyRequestedEnumerationBoundaryStepBackbone(plan *AnswerSurfacePlan, ir *AnalysisIR) {
	if plan == nil || ir == nil || plan.RequiredShape != ShapeStepList || len(plan.StepBackbone) == 0 {
		return
	}
	boundary := plan.RequestedEnumerationBoundary
	if boundary == nil || boundary.DeclaredCount <= 0 {
		return
	}
	if len(plan.StepBackbone) != boundary.DeclaredCount+1 {
		return
	}
	owner := RequestedEnumerationBoundaryOwner(ir.RequestModel)
	if owner == "" {
		return
	}
	first := strings.TrimSpace(plan.StepBackbone[0].Name)
	if first == "" || normalizedSurfaceSymbolTail(first) != normalizedSurfaceSymbolTail(owner) {
		return
	}
	plan.StepBackbone = append([]StepSurfaceAnchor(nil), plan.StepBackbone[1:]...)
	if plan.StepBackboneCompleteness == CompletenessComplete {
		plan.StepBackboneCompleteness = CompletenessLowerBound
	}
}

// CompileExplanationAnchorBackbone derives one grounded anchor per
// analyzer sub-topic for multi-topic explanation answers. The analyzer
// already uses the LLM to identify the independently-answerable
// sub-topics; this helper keeps downstream completion/finalization
// deterministic by validating that each topic has at least one
// grounded anchor line the extractor can hang a Key Anchors skeleton
// on.
func CompileExplanationAnchorBackbone(ir *AnalysisIR, evidence []EvidenceItem) ([]StepSurfaceAnchor, []string, CompletenessClaim) {
	if !ExplanationAllowsAnchorSkeleton(ir) || len(evidence) == 0 {
		return nil, nil, CompletenessUnknown
	}
	topics := ir.RequestModel.SubTopics
	if len(topics) == 0 {
		return nil, nil, CompletenessUnknown
	}
	used := make(map[string]bool, len(topics))
	anchors := make([]StepSurfaceAnchor, 0, len(topics))
	missing := make([]string, 0, len(topics))
	for i, topic := range topics {
		best := StepSurfaceAnchor{}
		bestKey := ""
		bestScore := 0
		for _, item := range evidence {
			score := explanationAnchorCandidateScore(topic, item)
			if score <= 0 {
				continue
			}
			name := explanationAnchorPreferredName(item)
			if name == "" {
				continue
			}
			key := fmt.Sprintf("%s:%d:%s",
				strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`)),
				item.LineStart,
				name,
			)
			if used[key] {
				continue
			}
			candidate := StepSurfaceAnchor{
				Name:        name,
				File:        strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`)),
				Line:        item.LineStart,
				Rationale:   strings.TrimSpace(topic.Summary),
				SurfaceText: strings.TrimSpace(EvidencePreferredSurfaceText(item, nil, true)),
			}
			if score > bestScore || (score == bestScore && explanationAnchorWinsTie(candidate, best)) {
				best = candidate
				bestKey = key
				bestScore = score
			}
		}
		if bestScore == 0 {
			missing = append(missing, explanationAnchorTopicLabel(topic, i))
			continue
		}
		used[bestKey] = true
		anchors = append(anchors, best)
	}
	claim := CompletenessUnknown
	if len(anchors) > 0 {
		if len(missing) == 0 && len(anchors) == len(topics) {
			claim = CompletenessComplete
		} else {
			claim = CompletenessLowerBound
		}
	}
	return anchors, missing, claim
}

func ApplyEvidenceExplanationAnchorBackbone(plan *AnswerSurfacePlan, ir *AnalysisIR, evidence []EvidenceItem) {
	if plan == nil {
		return
	}
	anchors, missing, claim := CompileExplanationAnchorBackbone(ir, evidence)
	plan.ExplanationAnchorBackbone = anchors
	plan.ExplanationAnchorMissingTopics = missing
	if claim != "" {
		plan.ExplanationAnchorCompleteness = claim
	}
}

func explanationAnchorTopicLabel(topic SubTopic, index int) string {
	if summary := strings.TrimSpace(topic.Summary); summary != "" {
		return summary
	}
	return fmt.Sprintf("sub-topic %d", index+1)
}

func explanationAnchorTopicTerms(topic SubTopic) []string {
	seen := make(map[string]bool, len(topic.Entities)+1)
	out := make([]string, 0, len(topic.Entities)+1)
	for _, raw := range topic.Entities {
		term := strings.TrimSpace(raw)
		if term == "" {
			continue
		}
		key := strings.ToLower(term)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, term)
	}
	if summary := strings.TrimSpace(topic.Summary); summary != "" {
		key := strings.ToLower(summary)
		if !seen[key] {
			out = append(out, summary)
		}
	}
	return out
}

func explanationAnchorPreferredName(item EvidenceItem) string {
	return firstNonEmptySurfaceString(
		item.AnchorSymbol,
		item.Subject,
		item.Object,
	)
}

func explanationAnchorCandidateScore(topic SubTopic, item EvidenceItem) int {
	if item.Source == "" || item.LineStart <= 0 || LooksLikeAuxiliaryEvidencePath(item.Source) {
		return 0
	}
	if item.ContextRole == EvidenceContextRoleIllustrativeOnly || item.GroundingStatus == GroundingUngrounded {
		return 0
	}
	terms := explanationAnchorTopicTerms(topic)
	if len(terms) == 0 {
		return 0
	}
	name := explanationAnchorPreferredName(item)
	if name == "" {
		return 0
	}
	score := 0
	for _, term := range terms {
		trimmed := strings.TrimSpace(term)
		if trimmed == "" {
			continue
		}
		switch {
		case strings.EqualFold(strings.TrimSpace(item.AnchorSymbol), trimmed):
			score += 20
		case strings.EqualFold(strings.TrimSpace(item.Subject), trimmed):
			score += 18
		case strings.EqualFold(strings.TrimSpace(item.Object), trimmed):
			score += 16
		case strings.EqualFold(strings.TrimSpace(name), trimmed):
			score += 14
		case EvidenceItemStructurallyMentionsAnyTerm(item, []string{trimmed}):
			score += 8
		case EvidenceItemMentionsAnyTerm(item, []string{trimmed}):
			score += 4
		}
	}
	if summary := strings.TrimSpace(topic.Summary); summary != "" {
		if overlap := explanationAnchorSharedSummaryRun(summary, item.Summary); overlap >= 5 {
			score += minInt(10, overlap)
		}
	}
	if score == 0 {
		return 0
	}
	switch item.Kind {
	case EvidenceDirect:
		score += 6
	case EvidenceRelationship, EvidenceMechanism, EvidenceRegistration:
		score += 4
	case EvidenceConditional:
		score += 2
	}
	switch item.AnchorKind {
	case AnchorDefinition:
		score += 6
	case AnchorAssignment:
		score += 4
	case AnchorCall, AnchorCondition:
		score += 3
	case AnchorReturn:
		score += 1
	}
	return score
}

func explanationAnchorWinsTie(candidate, incumbent StepSurfaceAnchor) bool {
	if candidate.Line > 0 && (incumbent.Line == 0 || candidate.Line < incumbent.Line) {
		return true
	}
	if candidate.Line == incumbent.Line && candidate.File != incumbent.File {
		return candidate.File < incumbent.File
	}
	if candidate.Line == incumbent.Line && candidate.File == incumbent.File {
		return candidate.Name < incumbent.Name
	}
	return false
}

func explanationAnchorSharedSummaryRun(a, b string) int {
	a = explanationAnchorNormalizeSurfaceText(a)
	b = explanationAnchorNormalizeSurfaceText(b)
	if a == "" || b == "" {
		return 0
	}
	ar := []rune(a)
	br := []rune(b)
	prev := make([]int, len(br)+1)
	best := 0
	for i := 1; i <= len(ar); i++ {
		curr := make([]int, len(br)+1)
		for j := 1; j <= len(br); j++ {
			if ar[i-1] != br[j-1] {
				continue
			}
			curr[j] = prev[j-1] + 1
			if curr[j] > best {
				best = curr[j]
			}
		}
		prev = curr
	}
	return best
}

func explanationAnchorNormalizeSurfaceText(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	return strings.Join(strings.Fields(s), " ")
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// RenderLinearDiagramFence emits a ```mermaid``` flowchart skeleton
// for a linear chain of nodes. Pre-2026-04-30 this rendered ASCII
// art (bare ``` + `node\n  ->\n`); the model treated that skeleton
// as a "copy verbatim" template and never reached for Mermaid even
// though the skill OutputFormat declared Mermaid the preferred form.
// Switching the seed itself to Mermaid puts the runtime injection
// in agreement with the prompt-level preference.
//
// Each node string becomes one Mermaid node. If a node label
// contains special characters (`:` `;` `(` `)` `{` `}` `[` `]` or
// whitespace), it is wrapped as `id["raw label"]` so the parser
// accepts it; otherwise the label is used as the bare id directly
// (single token, parser-safe).
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
	return renderMermaidLinearFence(out)
}

// renderMermaidLinearFence builds a flowchart LR with `id1 --> id2`
// edges chaining the nodes in order. Nodes with non-identifier
// characters get a synthetic short id and a quoted label.
func renderMermaidLinearFence(nodes []string) string {
	type rendered struct {
		id   string
		decl string // optional `id["label"]` declaration line; empty if id == label
	}
	prepared := make([]rendered, 0, len(nodes))
	for i, raw := range nodes {
		id, decl := mermaidNodeIdentity(raw, i)
		prepared = append(prepared, rendered{id: id, decl: decl})
	}
	var b strings.Builder
	b.WriteString("```mermaid\n")
	b.WriteString("flowchart LR\n")
	for _, p := range prepared {
		if p.decl != "" {
			b.WriteString("    ")
			b.WriteString(p.decl)
			b.WriteByte('\n')
		}
	}
	for i := 0; i < len(prepared)-1; i++ {
		fmt.Fprintf(&b, "    %s --> %s\n", prepared[i].id, prepared[i+1].id)
	}
	b.WriteString("```")
	return b.String()
}

// mermaidNodeIdentity decides how to express a raw label in a
// Mermaid node. Returns (id, decl) where:
//   - id is the identifier used in edge expressions
//   - decl is an optional `id["label"]` declaration line (empty
//     when the raw label is already a parser-safe single token)
//
// The "parser-safe single token" rule: ASCII letters / digits /
// underscore / dot / slash / hyphen, no whitespace. Anything else
// gets a synthetic id `n<i>` and a quoted label.
func mermaidNodeIdentity(raw string, idx int) (id string, decl string) {
	if mermaidLabelIsBareSafe(raw) {
		return raw, ""
	}
	id = fmt.Sprintf("n%d", idx)
	decl = fmt.Sprintf("%s[%q]", id, raw)
	return
}

func mermaidLabelIsBareSafe(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9':
			continue
		case r == '_', r == '.', r == '/', r == '-':
			continue
		default:
			return false
		}
	}
	return true
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

func ConfigTraceDiagramRoleNodeLabel(role EvidenceDiagramRole) string {
	switch CanonicalEvidenceDiagramRole(string(role)) {
	case EvidenceDiagramRoleOverride:
		return "operator override"
	case EvidenceDiagramRoleConfig:
		return "config file"
	case EvidenceDiagramRoleRuntime:
		return "runtime binding"
	case EvidenceDiagramRoleDefault:
		return "code default"
	default:
		return ""
	}
}

func ConfigTraceDiagramAnchorSupportLabel(anchor ConfigTraceDiagramAnchor) string {
	source := strings.TrimSpace(anchor.Source)
	switch {
	case source == "":
		return ""
	case anchor.Line > 0:
		return fmt.Sprintf("%s:%d", source, anchor.Line)
	default:
		return source
	}
}

// RenderLogDiagramFence emits a ```mermaid``` flowchart skeleton
// for a stack-frame call chain (innermost → outer). Same rationale
// as RenderLinearDiagramFence — a runtime-injected ASCII skeleton
// trains the model to copy ASCII shape verbatim, contradicting the
// skill's Mermaid preference.
func RenderLogDiagramFence(bundle *LogBundle) string {
	frames := collectDiagramLogFrames(bundle)
	if len(frames) < 2 {
		return ""
	}
	type prepared struct {
		id    string
		label string
	}
	rows := make([]prepared, 0, len(frames))
	for i, frame := range frames {
		location := fmt.Sprintf("%s:%d", frame.File, frame.Line)
		name := strings.TrimSpace(frame.Func)
		if name == "" {
			name = "(no symbol)"
		}
		// Distinct prefix per role so the model reads the chain
		// direction in the rendered output even after the bare
		// id-only edge form.
		var label string
		switch {
		case i == 0:
			label = fmt.Sprintf("innermost: %s in %s", location, name)
		case i == len(frames)-1:
			label = fmt.Sprintf("outermost caller: %s in %s", location, name)
		default:
			label = fmt.Sprintf("caller: %s in %s", location, name)
		}
		rows = append(rows, prepared{
			id:    fmt.Sprintf("frame%d", i),
			label: label,
		})
	}
	var b strings.Builder
	b.WriteString("```mermaid\n")
	b.WriteString("flowchart LR\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "    %s[%q]\n", r.id, r.label)
	}
	for i := 0; i < len(rows)-1; i++ {
		fmt.Fprintf(&b, "    %s --> %s\n", rows[i].id, rows[i+1].id)
	}
	b.WriteString("```")
	return b.String()
}

// RenderLogAnchorDiagramFence emits a grounded Mermaid call chain from
// already-reconciled log anchors. Unlike RenderLogDiagramFence, which
// mirrors the observed log frames directly, this helper prefers the
// current anchored file:line bindings recovered during drift
// reconciliation so downstream answers can keep their diagrams aligned
// with the citations they are allowed to show.
func RenderLogAnchorDiagramFence(anchors []LogSourceDriftAnchor) string {
	if len(anchors) < 2 {
		return ""
	}
	type prepared struct {
		id    string
		label string
	}
	rows := make([]prepared, 0, len(anchors))
	for i, anchor := range anchors {
		file := strings.TrimSpace(anchor.File)
		if file == "" {
			file = strings.TrimSpace(anchor.OriginalFile)
		}
		line := anchor.AnchoredLine
		if line <= 0 {
			line = anchor.ObservedLine
		}
		if file == "" || line <= 0 {
			return ""
		}
		location := fmt.Sprintf("%s:%d", file, line)
		name := strings.TrimSpace(firstNonEmptySurfaceString(anchor.Func, anchor.OriginalFunc))
		if name == "" {
			name = "(no symbol)"
		}
		var label string
		switch {
		case i == 0:
			label = fmt.Sprintf("innermost: %s in %s", location, name)
		case i == len(anchors)-1:
			label = fmt.Sprintf("outermost caller: %s in %s", location, name)
		default:
			label = fmt.Sprintf("caller: %s in %s", location, name)
		}
		rows = append(rows, prepared{
			id:    fmt.Sprintf("frame%d", i),
			label: label,
		})
	}
	var b strings.Builder
	b.WriteString("```mermaid\n")
	b.WriteString("flowchart LR\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "    %s[%q]\n", r.id, r.label)
	}
	for i := 0; i < len(rows)-1; i++ {
		fmt.Fprintf(&b, "    %s --> %s\n", rows[i].id, rows[i+1].id)
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
	logObserved []LogSourceDriftAnchor,
	logDrift []LogSourceDriftAnchor,
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
			if fence := RenderLogAnchorDiagramFence(logObserved); fence != "" {
				return kind, fence
			}
			if fence := RenderLogAnchorDiagramFence(logDrift); fence != "" {
				return kind, fence
			}
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
		RequiredShape:                EffectiveRequiredAnswerShape(ir, mutable),
		RequestedEnumerationBoundary: ir.RequestModel.EnumerationBoundary,
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
		plan.StableInvestigationReason = strings.TrimSpace(mutable.StableInvestigationCompleteReason())
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
	plan.CapabilitySurface = NormalizeCapabilitySurfaceHint(ir.RequestModel.AnalyzerHints.CapabilitySurface)
	if plan.CapabilitySurface != nil {
		plan.CapabilityAuthorityFiles = append([]string(nil), plan.CapabilitySurface.AuthorityFiles...)
	}

	emitted := []EvidenceItem(nil)
	if mutable != nil {
		emitted = mutable.EmittedEvidence()
	}
	plan.SurfaceEvidence = ExactResolutionSurfaceEvidencePool(emitted, evidence, answerChains)
	ApplyEvidenceStepBackbone(plan, plan.SurfaceEvidence)
	applyRequestedEnumerationBoundaryStepBackbone(plan, ir)
	ApplyEvidenceExplanationAnchorBackbone(plan, ir, plan.SurfaceEvidence)
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
	plan.DriftBoundedSurfaceItems = CollectDriftBoundedSurfaceItems(
		plan.LogObservedAnchors,
		plan.LogSourceDriftAnchors,
		plan.SurfaceEvidence,
	)

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
		plan.LogObservedAnchors,
		plan.LogSourceDriftAnchors,
		flowFindings,
		answerChains,
		plan.ConfigTraceDiagramAnchors,
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

// BuildAnswerSurfacePlanForAgentContext compiles the current
// answer-surface authority from an AgentContext. This keeps prompt
// builders and loop controllers on the same effective surface plan
// without each stage manually repacking AnalysisIR / Mutable /
// FlowFindings / AnswerChains / EvidenceItems.
func BuildAnswerSurfacePlanForAgentContext(ctx *AgentContext) *AnswerSurfacePlan {
	if ctx == nil {
		return nil
	}
	logBundle := ctx.LogTriage
	if logBundle == nil && ctx.Mutable != nil {
		logBundle = ctx.Mutable.LogTriage()
	}
	return BuildAnswerSurfacePlan(
		ctx.AnalysisIR,
		ctx.Mutable,
		logBundle,
		ctx.FlowFindings,
		ctx.AnswerChains,
		ctx.EvidenceItems,
	)
}

// BuildAnswerSurfacePlanForBusContext is the BusContext equivalent of
// BuildAnswerSurfacePlanForAgentContext. Shared validators and
// tool-stage gates consume the same compiled surface plan instead of
// re-deriving explanation / exact-resolution policy from local slices.
func BuildAnswerSurfacePlanForBusContext(ctx *BusContext) *AnswerSurfacePlan {
	if ctx == nil {
		return nil
	}
	var logBundle *LogBundle
	if ctx.Mutable != nil {
		logBundle = ctx.Mutable.LogTriage()
	}
	return BuildAnswerSurfacePlan(
		ctx.AnalysisIR,
		ctx.Mutable,
		logBundle,
		ctx.FlowFindings,
		ctx.AnswerChains,
		ctx.EvidenceItems,
	)
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
		source := strings.TrimSpace(ev.Source)
		if source == "" {
			continue
		}
		score := configTraceCitationCandidateScore(contractForConfigTraceSurfacePlan(contract, stableAbsent), role)
		if ev.LineStart > 0 {
			score += 2
		}
		key := string(role)
		if cur, ok := best[key]; ok && cur.Score >= score {
			continue
		}
		best[key] = ConfigTraceDiagramAnchor{
			Role:   key,
			Label:  ConfigTraceDiagramRoleNodeLabel(role),
			Source: source,
			Line:   ev.LineStart,
			Score:  score,
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

func CollectDriftBoundedSurfaceItems(observed, drift []LogSourceDriftAnchor, items []EvidenceItem) []EvidenceItem {
	if len(items) == 0 {
		return nil
	}
	anchors := observed
	if len(anchors) == 0 {
		anchors = drift
	}
	if len(anchors) == 0 {
		return nil
	}
	anchorFiles := make(map[string]bool, len(anchors))
	for _, anchor := range anchors {
		file := strings.TrimSpace(strings.ReplaceAll(anchor.File, `\`, `/`))
		if file != "" {
			anchorFiles[file] = true
		}
	}
	innerFunc := strings.TrimSpace(firstNonEmptySurfaceString(
		func() string {
			if len(observed) > 0 {
				return observed[0].Func
			}
			if len(drift) > 0 {
				return drift[0].OriginalFunc
			}
			return ""
		}(),
		func() string {
			if len(drift) > 0 {
				return drift[0].Func
			}
			return ""
		}(),
	))
	outerFunc := strings.TrimSpace(firstNonEmptySurfaceString(
		func() string {
			if len(observed) > 1 {
				return observed[1].Func
			}
			if len(drift) > 1 {
				return drift[1].OriginalFunc
			}
			return ""
		}(),
		func() string {
			if len(drift) > 1 {
				return drift[1].Func
			}
			return ""
		}(),
	))
	var out []EvidenceItem
	if ev, ok := bestDriftBoundedCallEdge(items, anchorFiles, outerFunc, innerFunc); ok {
		out = append(out, ev)
	}
	if ev, ok := bestDriftBoundedFunctionItem(items, anchorFiles, innerFunc, out); ok {
		out = append(out, ev)
	}
	if ev, ok := bestDriftBoundedFunctionItem(items, anchorFiles, outerFunc, out); ok {
		out = append(out, ev)
	}
	if len(out) == 0 {
		if ev, ok := bestDriftBoundedFallbackItem(items, anchorFiles); ok {
			out = append(out, ev)
		}
	}
	return out
}

func bestDriftBoundedCallEdge(items []EvidenceItem, anchorFiles map[string]bool, outerFunc, innerFunc string) (EvidenceItem, bool) {
	outerTail := normalizedSurfaceSymbolTail(outerFunc)
	innerTail := normalizedSurfaceSymbolTail(innerFunc)
	if outerTail == "" || innerTail == "" {
		return EvidenceItem{}, false
	}
	bestScore := -1
	var best EvidenceItem
	for _, item := range items {
		if !driftBoundedSurfaceItemAllowed(item, anchorFiles) || !driftBoundedIsCallItem(item) {
			continue
		}
		subjectTail := normalizedSurfaceSymbolTail(item.Subject)
		objectTail := normalizedSurfaceSymbolTail(firstNonEmptySurfaceString(item.Object, item.AnchorSymbol))
		if subjectTail == "" || objectTail == "" || subjectTail != outerTail || objectTail != innerTail {
			continue
		}
		score := 1000
		if item.LineStart > 0 {
			score += 10000 - item.LineStart
		}
		if score > bestScore {
			bestScore = score
			best = item
		}
	}
	return best, bestScore >= 0
}

func bestDriftBoundedFunctionItem(items []EvidenceItem, anchorFiles map[string]bool, fn string, existing []EvidenceItem) (EvidenceItem, bool) {
	target := normalizedSurfaceSymbolTail(fn)
	if target == "" {
		return EvidenceItem{}, false
	}
	bestScore := -1
	var best EvidenceItem
	for _, item := range items {
		if !driftBoundedSurfaceItemAllowed(item, anchorFiles) || driftBoundedItemSeen(existing, item) {
			continue
		}
		if driftBoundedIsCallItem(item) {
			continue
		}
		if !driftBoundedMentionsFunc(item, target) {
			continue
		}
		score := 0
		switch item.AnchorKind {
		case AnchorCondition:
			score += 500
		case AnchorDefinition:
			score += 400
		case AnchorAssignment:
			score += 300
		case AnchorReturn:
			score += 200
		case AnchorCall:
			score += 100
		default:
			score += 50
		}
		switch item.Kind {
		case EvidenceConditional:
			score += 40
		case EvidenceDirect:
			score += 30
		case EvidenceMechanism:
			score += 20
		case EvidenceRelationship:
			score += 10
		}
		if item.LineStart > 0 {
			score += 10000 - item.LineStart
		}
		if score > bestScore {
			bestScore = score
			best = item
		}
	}
	return best, bestScore >= 0
}

func bestDriftBoundedFallbackItem(items []EvidenceItem, anchorFiles map[string]bool) (EvidenceItem, bool) {
	bestScore := -1
	var best EvidenceItem
	for _, item := range items {
		if !driftBoundedSurfaceItemAllowed(item, anchorFiles) {
			continue
		}
		score := 0
		switch item.AnchorKind {
		case AnchorCondition:
			score += 300
		case AnchorCall:
			score += 250
		case AnchorDefinition:
			score += 200
		case AnchorReturn, AnchorAssignment:
			score += 150
		default:
			score += 100
		}
		switch item.Kind {
		case EvidenceConditional:
			score += 40
		case EvidenceRelationship:
			score += 30
		case EvidenceDirect:
			score += 20
		}
		if item.LineStart > 0 {
			score += 10000 - item.LineStart
		}
		if score > bestScore {
			bestScore = score
			best = item
		}
	}
	return best, bestScore >= 0
}

func driftBoundedSurfaceItemAllowed(item EvidenceItem, anchorFiles map[string]bool) bool {
	file := strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`))
	if file == "" || !anchorFiles[file] || LooksLikeAuxiliaryEvidencePath(file) {
		return false
	}
	switch item.GroundingStatus {
	case GroundingGrounded, GroundingRecovered, "":
	default:
		return false
	}
	switch item.ContextRole {
	case EvidenceContextRoleIllustrativeOnly, EvidenceContextRoleAbsenceSupport:
		return false
	}
	return true
}

func driftBoundedIsCallItem(item EvidenceItem) bool {
	if item.AnchorKind == AnchorCall {
		if strings.TrimSpace(item.Subject) != "" && strings.TrimSpace(firstNonEmptySurfaceString(item.Object, item.AnchorSymbol)) != "" {
			return true
		}
		if IsCallLikeEvidencePredicate(item.Predicate) {
			return true
		}
	}
	return IsCallLikeEvidencePredicate(item.Predicate)
}

func driftBoundedMentionsFunc(item EvidenceItem, target string) bool {
	for _, raw := range []string{item.AnchorSymbol, item.Subject, item.Object} {
		if normalizedSurfaceSymbolTail(raw) == target {
			return true
		}
	}
	return false
}

func driftBoundedItemSeen(existing []EvidenceItem, candidate EvidenceItem) bool {
	file := strings.TrimSpace(strings.ReplaceAll(candidate.Source, `\`, `/`))
	for _, item := range existing {
		if strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`)) != file {
			continue
		}
		if item.LineStart != candidate.LineStart {
			continue
		}
		if normalizedSurfaceSymbolTail(firstNonEmptySurfaceString(item.AnchorSymbol, item.Object, item.Subject)) ==
			normalizedSurfaceSymbolTail(firstNonEmptySurfaceString(candidate.AnchorSymbol, candidate.Object, candidate.Subject)) {
			return true
		}
	}
	return false
}

func collectLogSourceAnchors(rm RequestModel, bundle *LogBundle, items []EvidenceItem, minGap int) []LogSourceDriftAnchor {
	if (rm.Scenario != ScenarioRootCause && rm.Intent != IntentRootCause) || bundle == nil || len(items) == 0 {
		return nil
	}
	frames := collectDiagramLogFrames(bundle)
	if len(frames) == 0 {
		return nil
	}
	authoritativeTailsByFile := logSourceAuthoritativeTailsByFile(frames)

	byFile := make(map[string][]EvidenceItem)
	for _, item := range items {
		source := strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`))
		if source == "" || item.LineStart <= 0 {
			continue
		}
		if !logSourceAnchorItemAllowed(item, authoritativeTailsByFile[source]) {
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
		tail := normalizedSurfaceSymbolTail(frame.Func)
		if tail == "" {
			continue
		}

		// Tier 1 — exact file + exact tail (line drift in same
		// place). Pre-T1.2 was the only path; preserves byte-
		// identical behaviour when tail matches verbatim.
		if candidates := byFile[file]; len(candidates) > 0 {
			bestLine, bestDelta := nearestLogSourceDriftAnchorLine(candidates, tail, frame.Line)
			if bestLine != 0 && bestDelta > minGap {
				key := fmt.Sprintf("t1|%s|%s|%d|%d", file, tail, frame.Line, bestLine)
				if !seen[key] {
					seen[key] = true
					out = append(out, LogSourceDriftAnchor{
						File:         file,
						Func:         strings.TrimSpace(frame.Func),
						ObservedLine: frame.Line,
						AnchoredLine: bestLine,
						Reason:       DriftReasonLineDrift,
					})
				}
				continue
			}

			// Tier 2 — exact file + fuzzy tail (function renamed
			// within the same file). Only fires when same-file
			// candidates exist but Tier 1 found no exact-tail match
			// AND the candidate set is small enough (<=10) that a
			// fuzzy match is statistically meaningful (large files
			// with 30 functions and one matches "fooBar" → too many
			// false positives).
			if len(candidates) <= 10 {
				if anchor, ok := nearestLogSourceDriftFuzzyAnchor(candidates, tail, frame); ok {
					key := fmt.Sprintf("t2|%s|%s|%d|%d", file, tail, frame.Line, anchor.AnchoredLine)
					if !seen[key] {
						seen[key] = true
						out = append(out, anchor)
					}
					continue
				}
			}
		}

		// Tier 3 — cross-file move (same tail in a different file).
		// Fires only when frame.File has zero evidence AND the tail
		// uniquely identifies one evidence item elsewhere. Both
		// gates protect against ambiguous matches: a tail like
		// "handle" could appear in many files, but if it appears in
		// EXACTLY ONE evidence item the move is unambiguous.
		if _, sameFileSeen := byFile[file]; !sameFileSeen {
			if anchor, ok := nearestLogSourceDriftCrossFileAnchor(items, tail, frame); ok {
				key := fmt.Sprintf("t3|%s|%s|%d|%d|%s", file, tail, frame.Line, anchor.AnchoredLine, anchor.File)
				if !seen[key] {
					seen[key] = true
					out = append(out, anchor)
				}
			}
		}
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

func logSourceAuthoritativeTailsByFile(frames []LogFrame) map[string]map[string]bool {
	if len(frames) == 0 {
		return nil
	}
	out := make(map[string]map[string]bool)
	for _, frame := range frames {
		file := strings.TrimSpace(strings.ReplaceAll(frame.File, `\`, `/`))
		tail := normalizedSurfaceSymbolTail(frame.Func)
		if file == "" || tail == "" {
			continue
		}
		if out[file] == nil {
			out[file] = make(map[string]bool)
		}
		out[file][tail] = true
	}
	return out
}

func logSourceAnchorItemAllowed(item EvidenceItem, authoritativeTails map[string]bool) bool {
	if len(authoritativeTails) == 0 {
		return true
	}
	if item.GroundingStatus == GroundingUngrounded || item.ContextRole == EvidenceContextRoleIllustrativeOnly {
		return false
	}
	// Keep definition anchors eligible even when the current symbol tail no
	// longer matches verbatim: Tier 2 rename recovery needs those candidates
	// to compare the log-side tail against the current definition name.
	if item.AnchorKind == AnchorDefinition {
		return true
	}
	for _, tail := range EvidenceSurfaceSymbolTails(item) {
		if authoritativeTails[tail] {
			return true
		}
	}
	return false
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

// nearestLogSourceDriftFuzzyAnchor implements Tier 2: same-file
// candidates exist but no exact tail match. Try a fuzzy similarity
// check: substring containment (one is in the other) for length
// >= 4 OR Levenshtein distance ≤ 2 for length >= 5. Symmetric
// length floor avoids 3-char false positives like `do` matching
// `doX`. The picked candidate's tail is recorded as OriginalFunc.
func nearestLogSourceDriftFuzzyAnchor(candidates []EvidenceItem, tail string, frame LogFrame) (LogSourceDriftAnchor, bool) {
	bestLine := 0
	bestDelta := 0
	bestRenamedTail := ""
	for _, item := range candidates {
		if item.AnchorKind != AnchorDefinition {
			continue
		}
		for _, candidateTail := range logSourceDriftCandidateTails(item, true) {
			if candidateTail == tail {
				continue // exact match would have hit Tier 1
			}
			if !logSourceDriftFuzzyTailMatch(candidateTail, tail) {
				continue
			}
			line := item.LineStart
			if line <= 0 {
				continue
			}
			delta := absInt(line - frame.Line)
			if bestLine == 0 || delta < bestDelta {
				bestLine = line
				bestDelta = delta
				bestRenamedTail = candidateTail
			}
		}
	}
	if bestLine == 0 {
		return LogSourceDriftAnchor{}, false
	}
	return LogSourceDriftAnchor{
		File:         strings.TrimSpace(strings.ReplaceAll(frame.File, `\`, `/`)),
		Func:         bestRenamedTail,
		ObservedLine: frame.Line,
		AnchoredLine: bestLine,
		OriginalFunc: strings.TrimSpace(frame.Func),
		Reason:       DriftReasonTailRename,
	}, true
}

// nearestLogSourceDriftCrossFileAnchor implements Tier 3: the
// log's frame.File is absent from the evidence pool (file was
// moved / renamed at the path level), but the symbol tail
// matches EXACTLY ONE evidence item elsewhere. The unique-match
// gate is critical: a generic tail like "handle" might appear
// in many files; only when there's a single hit do we consider
// the move resolved.
func nearestLogSourceDriftCrossFileAnchor(items []EvidenceItem, tail string, frame LogFrame) (LogSourceDriftAnchor, bool) {
	type hit struct {
		file string
		line int
	}
	var matches []hit
	for _, item := range items {
		if item.AnchorKind != AnchorDefinition {
			continue
		}
		for _, candidateTail := range logSourceDriftCandidateTails(item, true) {
			if candidateTail != tail {
				continue
			}
			source := strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`))
			if source == "" || item.LineStart <= 0 {
				continue
			}
			matches = append(matches, hit{file: source, line: item.LineStart})
			break // one tail-match per item is enough
		}
	}
	if len(matches) != 1 {
		return LogSourceDriftAnchor{}, false
	}
	return LogSourceDriftAnchor{
		File:         matches[0].file,
		Func:         strings.TrimSpace(frame.Func),
		ObservedLine: frame.Line,
		AnchoredLine: matches[0].line,
		OriginalFile: strings.TrimSpace(strings.ReplaceAll(frame.File, `\`, `/`)),
		Reason:       DriftReasonFileMoved,
	}, true
}

// logSourceDriftFuzzyTailMatch returns true when two tail tokens
// are similar enough to plausibly represent the same renamed
// function. Two checks:
//  1. Substring containment (one is in the other) for length ≥ 4 —
//     catches prefix/suffix renames (FooHandler → FooHandlerV2).
//  2. Levenshtein distance ≤ 2 for length ≥ 5 — catches typo /
//     single-letter rename (handleAuth → handleAuthN).
//
// Bounds are conservative: short tokens (do, run, go) and long
// tokens (>30 chars) bypass the fuzzy match — the former because
// false-positive risk is too high, the latter because Levenshtein
// becomes O(n²) on long strings without buying anything.
func logSourceDriftFuzzyTailMatch(a, b string) bool {
	a = strings.ToLower(a)
	b = strings.ToLower(b)
	if a == b || a == "" || b == "" {
		return false // exact match path is Tier 1
	}
	if len(a) >= 4 && len(b) >= 4 {
		if strings.Contains(a, b) || strings.Contains(b, a) {
			return true
		}
	}
	if len(a) >= 5 && len(b) >= 5 && len(a) <= 30 && len(b) <= 30 {
		if logSourceDriftLevenshtein(a, b) <= 2 {
			return true
		}
	}
	return false
}

// logSourceDriftLevenshtein returns the edit distance between two
// strings using a single-row DP. Caller-bounded by length so the
// O(n*m) cost stays cheap (max 30*30=900 operations).
func logSourceDriftLevenshtein(a, b string) int {
	if a == b {
		return 0
	}
	la, lb := len(a), len(b)
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			curr[j] = del
			if ins < curr[j] {
				curr[j] = ins
			}
			if sub < curr[j] {
				curr[j] = sub
			}
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func logSourceDriftCandidateTails(item EvidenceItem, definitionOnly bool) []string {
	var raws []string
	if item.AnchorKind == AnchorDefinition {
		raws = append(raws, item.AnchorSymbol, item.Subject)
	} else if !definitionOnly {
		// Non-definition evidence often carries the authoritative symbol in
		// Subject/Object even when AnchorSymbol names the local owner/callee.
		// Keeping all three aligned with EvidenceSurfaceSymbolTails lets the
		// log-drift surface reuse the same structural symbol binding the rest
		// of the answer pipeline already trusts.
		raws = append(raws, item.AnchorSymbol, item.Subject, item.Object)
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

func EvidenceSurfaceSymbolTails(item EvidenceItem) []string {
	return evidenceSurfaceSymbolTails(item)
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
		item.Kind == EvidenceUnresolved ||
		item.Kind == EvidenceTruncated ||
		item.GroundingStatus == GroundingUngrounded {
		return EvidenceDiagramRoleUnknown
	}
	if item.ContextRole == EvidenceContextRoleAbsenceSupport &&
		!configTraceAbsenceSupportCanCarryDiagramRole(item) {
		return EvidenceDiagramRoleUnknown
	}
	switch item.DiagramRole {
	case EvidenceDiagramRoleConfig:
		if ConfigTraceDiagramRoleAnchorCompatible(item.DiagramRole, item) &&
			LooksLikeConfigFilePath(item.Source) &&
			configTraceDiagramEvidenceWithinScope(contract, item, requiredFiles) {
			return item.DiagramRole
		}
	case EvidenceDiagramRoleDefault, EvidenceDiagramRoleRuntime, EvidenceDiagramRoleOverride:
		if ConfigTraceDiagramRoleAnchorCompatible(item.DiagramRole, item) &&
			item.Source != "" &&
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
