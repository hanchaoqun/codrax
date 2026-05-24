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
// It compiles the current stable investigation state, the effective
// diagram contract, exact-resolution policy, and the categorized
// answer-surface evidence into one deterministic view so downstream
// stages do not re-derive the same policy independently.
//
// Per docs/migration/answer_shape_retirement.md the legacy
// RequiredShape field has been retired; downstream callers consume
// AnswerSemanticView (NeedsPrincipalScalar / NeedsOrderedPrincipalList /
// NeedsEnumerationSlate / etc.) for the obligation signals that
// previously came from the shape enum.
type AnswerSurfacePlan struct {
	RequestedEnumerationBoundary    *RequestedEnumerationBoundary
	Diagram                         *DiagramContract
	DiagramHardRequirementDropped   bool
	CompiledDiagramKind             DiagramKind
	CompiledDiagramFence            string
	StepBackbone                    []StepSurfaceAnchor
	StepBackboneCompleteness        CompletenessClaim
	ExplanationAnchorBackbone       []StepSurfaceAnchor
	ExplanationAnchorCompleteness   CompletenessClaim
	ExplanationAnchorMissingTopics  []string
	ExternalObservationSeeds        []ExternalObservationSeed
	LogSourceDriftAnchors           []LogSourceDriftAnchor
	LogObservedAnchors              []LogSourceDriftAnchor
	DriftBoundedSurfaceItems        []EvidenceItem
	RuntimeGroundingDisposition     *RuntimeGroundingDisposition
	CurrentStatusDiagnosticRequired bool
	CurrentSourceEvidenceOrigin     bool

	ExactResolution          *ExactResolutionContract
	PreferredExactResolution *AnswerExactResolution
	SummarySurfaceMode       AnswerSummarySurfaceMode

	StableInvestigationResultKind string
	StableAbsenceJustification    string
	StableInvestigationReason     string
	StableValidationBoundaryNotes []string
	StableAggregateFacts          []AnswerAggregateFact
	StableAbsent                  bool
	ExactContextRequiredFiles     []string
	CapabilitySurface             *CapabilitySurfaceHint
	CapabilityAuthorityFiles      []string
	ChangeImpactProfile           *ChangeImpactProfile
	RequestedAnswerDimensions     []RequestedAnswerDimension

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

	// FacetCoverage (Phase 1 of Semantic Surface Contract,
	// docs/design/semantic_surface_contract_phases.md §1) compiles
	// the per-question facet plan from the family resolver +
	// surface evidence. Phase 1 is OBSERVATION ONLY — set on the
	// plan, surfaced via trace logs, but does NOT drive any gate
	// or finalizer prompt yet. Phase 2+ will start consuming.
	// Nil when ResolveQuestionFamily returned no matching template
	// or when the analyzer produced no usable RequestModel.
	FacetCoverage *FacetCoverageContract

	// SubRepoNames is the snapshot of multi-repo topology
	// identifiers stamped at plan-build time: every sub-repo's
	// Slug, RootRel, and RootRel basename. Consumers that need to
	// distinguish "this string is a code symbol" from "this string
	// is a project / repository name" use this set as a hard typed
	// signal (matches the 2026-05-16 finalize-loop fix family — a
	// noisy identifier-shape heuristic must not drive hard gate
	// behaviour when a precise typed alternative exists).
	//
	// Empty in single-repo / unwired-MultiGraph postures, in which
	// case consumers should fall through to the legacy "treat
	// identifier-shaped tokens as symbols" path.
	SubRepoNames []string
}

// IsCrashSourcedRootCause reports whether the surface plan was
// derived from a runtime artifact — a panic / exception / sanitizer
// log or a perf jank trace — rather than from a code-only debugging
// question ("Foo returns nil — why?", "why does config X get the
// default?") where return / assignment anchors ARE the mechanism the
// user is asking about.
//
// The judgement uses typed structural signals already present on the
// surface plan, both populated only by the log_triage / perf_triage
// pipelines:
//
//  1. RuntimeGroundingDisposition — a model-declared or system-detected
//     statement that runtime artifact facts are observation-first.
//  2. ExternalObservationSeeds — populated by
//     CollectExternalObservationSeeds from a non-nil LogBundle (kinds:
//     "error_type" / "signal" / "log_observation" / "log_frame" /
//     "log_residue"). Empty
//     when no runtime artifact was attached.
//  3. LogObservedAnchors — runtime frames lifted from the typed
//     bundle's resolved files. Empty when no log_triage ran or no
//     frame resolved.
//
// Both are typed signals — no prose-cue / keyword tables. When both
// are empty the plan is treated as a non-runtime root-cause flow and
// downstream gates that exclude code-mechanism evidence (such as the
// AnchorReturn exclusion in the principal mechanism lane) stay open.
//
// Naming: the historical name "crash-sourced" reflects the dominant
// failure mode but the gate is correctly broader — any runtime
// artifact (including non-crash perf jank traces) routes through the
// same lane discipline because the runtime-vs-current-code drift
// argument applies identically.
func (plan *AnswerSurfacePlan) IsCrashSourcedRootCause() bool {
	if plan == nil {
		return false
	}
	if plan.RuntimeGroundingDisposition.IsActive() {
		return true
	}
	if len(plan.ExternalObservationSeeds) > 0 {
		return true
	}
	return len(plan.LogObservedAnchors) > 0
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

// ExternalObservationSeed captures an answer-grade fact whose source
// of truth is an attached runtime / external observation stream rather
// than current repo code. Finalizers may surface these facts in
// summary / steps / scalar rationale, but should default the
// corresponding citation_ref to -1 unless a repo citation literally
// corroborates the claim.
type ExternalObservationSeed struct {
	Kind         string
	Raw          string
	Role         string
	Lang         string
	File         string
	Line         int
	Func         string
	AnchoredFile string
	AnchoredLine int
}

const (
	// ExternalObservationPromptSeedLimit is the compact render budget for
	// finalizer-facing runtime observations. The collector keeps a richer
	// typed pool; prompt renderers then choose a balanced slice of this size.
	ExternalObservationPromptSeedLimit = 6

	// Summary/residue pool caps are pathological-log guards only. They do
	// not control answer coverage: frame coverage uses the existing
	// root-cause stack budget, and the prompt selector separately balances
	// messages, languages, files, and frames.
	externalObservationSummarySeedPoolLimit = 8
	externalObservationResidueSeedPoolLimit = 4
)

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
	LineEnd     int
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

// answerWantsStepBackbone reports whether the question's family
// produces an ordered principal list (the legacy "RequiredShape ==
// ShapeStepList" gate). True for the families whose compile_<family>
// emits BlockOrderedList at SurfacePrincipal: QFRootCauseTrace,
// QFCallChain, QFEnumeration. Read directly from the typed
// QuestionFamily so no shape detour is needed.
func answerWantsStepBackbone(ir *AnalysisIR) bool {
	if ir == nil {
		return false
	}
	switch ResolveQuestionFamily(ir.RequestModel) {
	case QFRootCauseTrace, QFCallChain, QFEnumeration:
		return true
	}
	return false
}

// answerWantsLongFormDriftSurface reports whether the question's
// family produces a long-form prose answer where log-source drift
// caveats land naturally (the legacy "RequiredShape == ShapeStepList ||
// ShapeExplanation" gate). Excludes scalar / role-lookup / config-
// precedence / enumeration families whose principal payload is a
// scalar or list of items rather than a prose narrative.
func answerWantsLongFormDriftSurface(ir *AnalysisIR) bool {
	if ir == nil {
		return false
	}
	switch ResolveQuestionFamily(ir.RequestModel) {
	case QFRootCauseTrace, QFCallChain, QFArchitecture, QFGeneric:
		return true
	}
	return false
}

func ApplyAnswerSymbolStepBackbone(plan *AnswerSurfacePlan, ir *AnalysisIR, symbols []AnswerSymbol, claim CompletenessClaim) {
	if plan == nil || !answerWantsStepBackbone(ir) || len(symbols) == 0 {
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
		case AnchorDefinition, AnchorCall, AnchorCondition, AnchorAssignment, AnchorInitializer:
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
			LineEnd:     item.LineEnd,
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

func ApplyEvidenceStepBackbone(plan *AnswerSurfacePlan, ir *AnalysisIR, evidence []EvidenceItem) {
	if plan == nil || !answerWantsStepBackbone(ir) || len(evidence) == 0 {
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

// ApplyPrincipalEvidenceStepBackbone promotes facet-bound principal
// evidence into the ordered-list backbone for enumeration answers.
// The generic evidence backbone intentionally requires a 3-anchor
// same-file run so mechanism/call-chain answers do not grow from
// isolated helper facts. Enumerations are different: a valid answer
// can have one member, and the facet contract has already selected
// which evidence items are principal members. This helper runs after
// FacetCoverage is compiled so it consumes that typed principal lane
// instead of raw evidence or search output.
func ApplyPrincipalEvidenceStepBackbone(plan *AnswerSurfacePlan, ir *AnalysisIR) {
	if plan == nil || ir == nil || len(plan.StepBackbone) > 0 {
		return
	}
	if ResolveQuestionFamily(ir.RequestModel) != QFEnumeration {
		return
	}
	items := PrincipalSupportEvidenceItemsForFamily(QFEnumeration, plan)
	if len(items) == 0 {
		return
	}
	anchors := make([]StepSurfaceAnchor, 0, len(items))
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		name := principalEvidenceStepAnchorName(item, items)
		file := strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`))
		if name == "" || file == "" || item.LineStart <= 0 {
			continue
		}
		key := fmt.Sprintf("%s:%d:%s", file, item.LineStart, strings.ToLower(name))
		if seen[key] {
			continue
		}
		seen[key] = true
		anchors = append(anchors, StepSurfaceAnchor{
			Name:        name,
			File:        file,
			Line:        item.LineStart,
			SurfaceText: strings.TrimSpace(EvidencePreferredSurfaceText(item, nil, true)),
		})
	}
	if len(anchors) == 0 {
		return
	}
	plan.StepBackbone = anchors
	if plan.StepBackboneCompleteness == "" {
		plan.StepBackboneCompleteness = CompletenessLowerBound
	}
}

func principalEvidenceStepAnchorName(item EvidenceItem, peers []EvidenceItem) string {
	if PrincipalMemberSurfaceForEvidenceSet(item, peers) == PrincipalMemberSurfaceSourceLocation {
		if source := strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`)); source != "" {
			if ClaimFormOf(item) == ClaimImportEdge {
				return source
			}
			if item.LineStart > 0 {
				return fmt.Sprintf("%s:%d", source, item.LineStart)
			}
			return source
		}
	}
	switch ClaimFormOf(item) {
	case ClaimReturnFact:
		if name := firstNonEmptySurfaceString(item.Object, item.AnchorSymbol, item.Subject); name != "" {
			return name
		}
	case ClaimImportEdge:
		if name := firstNonEmptySurfaceString(item.Object, item.AnchorSymbol, item.Subject); name != "" {
			return name
		}
	case ClaimAssignmentFact:
		if name := firstNonEmptySurfaceString(item.Subject, item.AnchorSymbol, item.Object); name != "" {
			return name
		}
	}
	return firstNonEmptySurfaceString(item.AnchorSymbol, item.Subject, item.Object)
}

func applyRequestedEnumerationBoundaryStepBackbone(plan *AnswerSurfacePlan, ir *AnalysisIR) {
	if plan == nil || ir == nil || !answerWantsStepBackbone(ir) || len(plan.StepBackbone) == 0 {
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
	if !IRAllowsAnchorSkeleton(ir) || len(evidence) == 0 {
		return nil, nil, CompletenessUnknown
	}
	if !subTopicsMayMaterializeCodeAnchorSkeleton(ir.RequestModel) {
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
				LineEnd:     item.LineEnd,
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
	case AnchorAssignment, AnchorInitializer:
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

// renderMermaidLinearFence builds a flowchart TD with `id1 --> id2`
// edges chaining the nodes in order. Vertical is the default because
// code-bearing labels (file:line / symbol tails / role prose) are
// often wider than they are tall; LR quickly overflows terminals and
// decks. Nodes with non-identifier characters get a synthetic short
// id and a quoted label.
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
	b.WriteString("flowchart TD\n")
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
	b.WriteString("flowchart TD\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "    %s[%q]\n", r.id, r.label)
	}
	for i := 0; i < len(rows)-1; i++ {
		fmt.Fprintf(&b, "    %s --> %s\n", rows[i].id, rows[i+1].id)
	}
	b.WriteString("```")
	return b.String()
}

// RenderLogSequenceDiagramFence emits the same grounded stack-frame
// sequence as RenderLogDiagramFence, but preserves an explicit sequence
// contract by using Mermaid sequenceDiagram syntax instead of handing
// the finalizer a flowchart seed to reinterpret.
func RenderLogSequenceDiagramFence(bundle *LogBundle) string {
	frames := collectDiagramLogFrames(bundle)
	if len(frames) < 2 {
		return ""
	}
	labels := make([]string, 0, len(frames))
	for i, frame := range frames {
		location := fmt.Sprintf("%s:%d", frame.File, frame.Line)
		name := strings.TrimSpace(frame.Func)
		if name == "" {
			name = "(no symbol)"
		}
		switch {
		case i == 0:
			labels = append(labels, fmt.Sprintf("innermost: %s in %s", location, name))
		case i == len(frames)-1:
			labels = append(labels, fmt.Sprintf("outermost caller: %s in %s", location, name))
		default:
			labels = append(labels, fmt.Sprintf("caller: %s in %s", location, name))
		}
	}
	return RenderSequenceDiagramFence(labels, 0)
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
	b.WriteString("flowchart TD\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "    %s[%q]\n", r.id, r.label)
	}
	for i := 0; i < len(rows)-1; i++ {
		fmt.Fprintf(&b, "    %s --> %s\n", rows[i].id, rows[i+1].id)
	}
	b.WriteString("```")
	return b.String()
}

// RenderLogAnchorSequenceDiagramFence is the sequence-preserving twin of
// RenderLogAnchorDiagramFence for hard sequence diagram contracts.
func RenderLogAnchorSequenceDiagramFence(anchors []LogSourceDriftAnchor) string {
	if len(anchors) < 2 {
		return ""
	}
	labels := make([]string, 0, len(anchors))
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
		switch {
		case i == 0:
			labels = append(labels, fmt.Sprintf("innermost: %s in %s", location, name))
		case i == len(anchors)-1:
			labels = append(labels, fmt.Sprintf("outermost caller: %s in %s", location, name))
		default:
			labels = append(labels, fmt.Sprintf("caller: %s in %s", location, name))
		}
	}
	return RenderSequenceDiagramFence(labels, 0)
}

func RenderFlowFindingDiagramFence(findings []FlowFindingDigest) string {
	for _, finding := range findings {
		if fence := RenderLinearDiagramFence(flowFindingDiagramNodes(finding), 6); fence != "" {
			return fence
		}
	}
	return ""
}

func RenderFlowFindingSequenceDiagramFence(findings []FlowFindingDigest) string {
	for _, finding := range findings {
		if fence := RenderSequenceDiagramFence(flowFindingDiagramNodes(finding), 6); fence != "" {
			return fence
		}
	}
	return ""
}

// RenderSequenceDiagramFence emits a Mermaid sequenceDiagram from a
// grounded ordered node list. It mirrors RenderLinearDiagramFence's
// de-duplication and limit semantics, but keeps the requested
// actor-to-actor carrier when the answer contract asks for a sequence
// diagram instead of handing the model a flowchart to reinterpret.
func RenderSequenceDiagramFence(nodes []string, limit int) string {
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
	return renderMermaidSequenceFence(out)
}

func renderMermaidSequenceFence(nodes []string) string {
	if len(nodes) < 2 {
		return ""
	}
	var b strings.Builder
	b.WriteString("```mermaid\n")
	b.WriteString("sequenceDiagram\n")
	for i, label := range nodes {
		fmt.Fprintf(&b, "    participant p%d as %q\n", i, label)
	}
	for i := 0; i < len(nodes)-1; i++ {
		fmt.Fprintf(&b, "    p%d->>p%d: next\n", i, i+1)
	}
	b.WriteString("```")
	return b.String()
}

type evidenceDiagramEdge struct {
	From  string
	To    string
	Label string
}

// RenderEvidenceArchitectureDiagramFence emits a conservative diagram seed
// from already-grounded EvidenceItems. It is intentionally typed:
// support comes from ClaimForm / AnchorKind projection, not from matching
// model prose or user text. Relation-shaped anchors become graph edges;
// otherwise definition anchors become a simple component spine.
func RenderEvidenceArchitectureDiagramFence(items []EvidenceItem) string {
	if fence := renderEvidenceRelationDiagramFence(items, ClaimCallEdge, ClaimImportEdge); fence != "" {
		return fence
	}
	return RenderLinearDiagramFence(EvidenceDiagramNodes(items, 8), 8)
}

// RenderEvidenceCallDiagramFence emits a Mermaid flowchart for grounded call
// edges. The call_dag diagram kind is rendered as a flowchart because Mermaid's
// flowchart subset is the renderer-supported, terminal-stable carrier.
func RenderEvidenceCallDiagramFence(items []EvidenceItem) string {
	return renderEvidenceRelationDiagramFence(items, ClaimCallEdge)
}

// RenderEvidenceSequenceDiagramFence uses the same validated call-edge
// pool as RenderEvidenceCallDiagramFence, but emits actor-to-actor
// sequence syntax for explicit sequence diagram requests.
func RenderEvidenceSequenceDiagramFence(items []EvidenceItem) string {
	return renderEvidenceRelationSequenceFence(items, ClaimCallEdge)
}

func renderEvidenceRelationSequenceFence(items []EvidenceItem, forms ...ClaimForm) string {
	edges := evidenceDiagramEdges(items, 8, forms...)
	if len(edges) == 0 {
		return ""
	}
	return renderMermaidSequenceEdgeFence(edges)
}

func renderMermaidSequenceEdgeFence(edges []evidenceDiagramEdge) string {
	type participant struct {
		id    string
		label string
	}
	byLabel := make(map[string]participant)
	order := make([]string, 0, len(edges)*2)
	ensure := func(label string) participant {
		label = strings.TrimSpace(label)
		if p, ok := byLabel[label]; ok {
			return p
		}
		p := participant{
			id:    fmt.Sprintf("p%d", len(byLabel)),
			label: label,
		}
		byLabel[label] = p
		order = append(order, label)
		return p
	}
	for _, edge := range edges {
		ensure(edge.From)
		ensure(edge.To)
	}
	if len(order) < 2 {
		return ""
	}
	var b strings.Builder
	b.WriteString("```mermaid\n")
	b.WriteString("sequenceDiagram\n")
	for _, label := range order {
		p := byLabel[label]
		fmt.Fprintf(&b, "    participant %s as %q\n", p.id, p.label)
	}
	for _, edge := range edges {
		from := ensure(edge.From)
		to := ensure(edge.To)
		label := strings.TrimSpace(edge.Label)
		if label == "" {
			label = "calls"
		}
		fmt.Fprintf(&b, "    %s->>%s: %s\n", from.id, to.id, mermaidEdgeLabel(label))
	}
	b.WriteString("```")
	return b.String()
}

func renderEvidenceRelationDiagramFence(items []EvidenceItem, forms ...ClaimForm) string {
	edges := evidenceDiagramEdges(items, 6, forms...)
	if len(edges) == 0 {
		return ""
	}
	type rendered struct {
		id   string
		decl string
	}
	nodeByLabel := make(map[string]rendered)
	nodeOrder := make([]string, 0, len(edges)*2)
	ensureNode := func(label string) string {
		label = strings.TrimSpace(label)
		if existing, ok := nodeByLabel[label]; ok {
			return existing.id
		}
		id, decl := mermaidNodeIdentity(label, len(nodeByLabel))
		nodeByLabel[label] = rendered{id: id, decl: decl}
		nodeOrder = append(nodeOrder, label)
		return id
	}

	var b strings.Builder
	b.WriteString("```mermaid\n")
	b.WriteString("flowchart TD\n")
	for _, edge := range edges {
		ensureNode(edge.From)
		ensureNode(edge.To)
	}
	for _, label := range nodeOrder {
		if node := nodeByLabel[label]; node.decl != "" {
			b.WriteString("    ")
			b.WriteString(node.decl)
			b.WriteByte('\n')
		}
	}
	for _, edge := range edges {
		fromID := ensureNode(edge.From)
		toID := ensureNode(edge.To)
		if edge.Label != "" {
			fmt.Fprintf(&b, "    %s -->|%s| %s\n", fromID, mermaidEdgeLabel(edge.Label), toID)
		} else {
			fmt.Fprintf(&b, "    %s --> %s\n", fromID, toID)
		}
	}
	b.WriteString("```")
	return b.String()
}

func evidenceDiagramEdges(items []EvidenceItem, limit int, forms ...ClaimForm) []evidenceDiagramEdge {
	allowed := make(map[ClaimForm]bool, len(forms))
	for _, form := range forms {
		if form != ClaimUnknown {
			allowed[form] = true
		}
	}
	var out []evidenceDiagramEdge
	seen := map[string]bool{}
	for _, item := range items {
		if !DiagramEvidenceEligible(item) {
			continue
		}
		form := ClaimFormOf(item)
		if !allowed[form] {
			continue
		}
		from := strings.TrimSpace(firstNonEmptySurfaceString(item.Subject, item.OwnerSymbol))
		to := strings.TrimSpace(firstNonEmptySurfaceString(item.Object, item.AnchorSymbol))
		if from == "" || to == "" || from == to {
			continue
		}
		label := evidenceDiagramRelationLabel(form, item)
		key := from + "\x00" + to + "\x00" + label
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, evidenceDiagramEdge{From: from, To: to, Label: label})
		if limit > 0 && len(out) >= limit {
			return out
		}
	}
	return out
}

func evidenceDiagramRelationLabel(form ClaimForm, item EvidenceItem) string {
	var rel string
	switch form {
	case ClaimCallEdge:
		rel = "calls"
	case ClaimImportEdge:
		rel = "imports"
	default:
		rel = "relates"
	}
	if loc := strings.TrimSpace(item.DisplayLocation(true)); loc != "" {
		return rel + " @ " + loc
	}
	return rel
}

func mermaidEdgeLabel(s string) string {
	s = strings.TrimSpace(strings.Join(strings.Fields(s), " "))
	s = strings.NewReplacer("|", "/", "\n", " ", "\r", " ").Replace(s)
	return s
}

// EvidenceDiagramNodes returns grounded node labels that can seed a diagram
// without assuming language-specific syntax. Labels prefer file:line anchors
// when citable, then typed symbols from the validated evidence item.
func EvidenceDiagramNodes(items []EvidenceItem, limit int) []string {
	seen := make(map[string]bool)
	nodes := make([]string, 0, len(items))
	for _, item := range items {
		if !DiagramEvidenceEligible(item) {
			continue
		}
		label := evidenceDiagramNodeLabel(item)
		if label == "" || seen[label] {
			continue
		}
		seen[label] = true
		nodes = append(nodes, label)
		if limit > 0 && len(nodes) >= limit {
			break
		}
	}
	return nodes
}

func evidenceDiagramNodeLabel(item EvidenceItem) string {
	loc := strings.TrimSpace(item.DisplayLocation(true))
	name := strings.TrimSpace(firstNonEmptySurfaceString(
		item.AnchorSymbol,
		item.Subject,
		item.Object,
		item.OwnerSymbol,
	))
	switch {
	case loc != "" && name != "":
		return name + " @ " + loc
	case loc != "":
		return loc
	case name != "":
		return name
	default:
		return strings.TrimSpace(item.Source)
	}
}

func RenderDriftBoundedSurfaceDiagramFence(items []EvidenceItem) string {
	items = DriftBoundedRenderableSurfaceItems(items)
	if len(items) == 0 {
		return ""
	}
	nodes := make([]string, 0, len(items))
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		label := strings.TrimSpace(driftBoundedDiagramNodeLabel(item))
		if label == "" || seen[label] {
			continue
		}
		seen[label] = true
		nodes = append(nodes, label)
	}
	return RenderLinearDiagramFence(nodes, 0)
}

func DriftBoundedRenderableSurfaceItems(items []EvidenceItem) []EvidenceItem {
	if len(items) == 0 {
		return nil
	}
	focusTail := ""
	focusFiles := map[string]bool{}
	for _, item := range items {
		if !driftBoundedIsCallItem(item) {
			continue
		}
		if focusTail == "" {
			focusTail = normalizedSurfaceSymbolTail(firstNonEmptySurfaceString(item.Object, item.AnchorSymbol))
		}
		// Track every file that hosts a call item so non-call items
		// (Conditional / Assignment / Return statements at nearby
		// lines) in the same file survive the focus filter — without
		// this, the Conditional / Assignment items the LLM emits
		// alongside the call get dropped just because their
		// AnchorSymbol/OwnerSymbol/Subject/Object don't share a
		// symbol tail with the call's target.
		if f := strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`)); f != "" {
			focusFiles[f] = true
		}
	}
	if focusTail == "" && len(focusFiles) == 0 {
		return append([]EvidenceItem(nil), items...)
	}
	filtered := make([]EvidenceItem, 0, len(items))
	for _, item := range items {
		if driftBoundedIsCallItem(item) {
			filtered = append(filtered, item)
			continue
		}
		// Keep when symbol tail matches the focus call's target.
		// hasSymbolAnchoring uses ONLY AnchorSymbol/OwnerSymbol to
		// detect "the LLM explicitly anchored this item to a symbol"
		// — Object/Subject can carry code expressions (e.g.
		// `ctx.Mutable.RequestModel()` as an assignment value),
		// which are NOT symbol anchorings and should not block the
		// same-file fallback for statement-level items.
		matched := false
		hasSymbolAnchoring := strings.TrimSpace(item.AnchorSymbol) != "" ||
			strings.TrimSpace(item.OwnerSymbol) != ""
		if focusTail != "" {
			for _, raw := range []string{item.AnchorSymbol, item.OwnerSymbol, item.Subject, item.Object} {
				if normalizedSurfaceSymbolTail(raw) == focusTail {
					matched = true
					break
				}
			}
		}
		// Same-file fallback ONLY when the item has NO symbol info at
		// all (statement-level Condition / Assignment / Return whose
		// AnchorSymbol/OwnerSymbol/Subject/Object are all empty —
		// e.g. a bare condition string like "ctx == nil"). These items
		// are part of the failure mechanism but have no symbol-name
		// tie to the callee.
		//
		// Items WITH symbol info but mismatching tail (e.g. the outer
		// caller's guard with AnchorSymbol="ParseOutput" when the
		// focusTail is "buildAnalysisIR") are intentionally dropped —
		// the focus-on-callee discipline still holds.
		if !matched && !hasSymbolAnchoring && len(focusFiles) > 0 {
			f := strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`))
			if f != "" && focusFiles[f] {
				matched = true
			}
		}
		if matched {
			filtered = append(filtered, item)
		}
	}
	if len(filtered) == 0 {
		return append([]EvidenceItem(nil), items...)
	}
	return filtered
}

func driftBoundedDiagramNodeLabel(item EvidenceItem) string {
	file := strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`))
	if file == "" || item.LineStart <= 0 {
		return ""
	}
	location := fmt.Sprintf("%s:%d", file, item.LineStart)
	switch {
	case driftBoundedIsCallItem(item):
		subject := strings.TrimSpace(item.Subject)
		object := strings.TrimSpace(firstNonEmptySurfaceString(item.Object, item.AnchorSymbol))
		if subject == "" || object == "" {
			return ""
		}
		return fmt.Sprintf("%s calls %s @ %s", subject, object, location)
	case item.AnchorKind == AnchorCondition:
		condition := strings.TrimSpace(item.Condition)
		if condition == "" {
			return ""
		}
		return fmt.Sprintf("if %s @ %s", condition, location)
	case item.AnchorKind == AnchorAssignment || item.AnchorKind == AnchorInitializer:
		expr := strings.TrimSpace(firstNonEmptySurfaceString(item.Object, item.Snippet, item.AnchorSymbol, item.Subject))
		if expr == "" {
			return ""
		}
		return fmt.Sprintf("%s @ %s", expr, location)
	case item.AnchorKind == AnchorReturn:
		expr := strings.TrimSpace(firstNonEmptySurfaceString(item.Object, item.Snippet, item.AnchorSymbol, item.Subject))
		if expr == "" {
			return ""
		}
		return fmt.Sprintf("return %s @ %s", expr, location)
	case item.AnchorKind == AnchorDefinition:
		name := strings.TrimSpace(firstNonEmptySurfaceString(item.AnchorSymbol, item.OwnerSymbol, item.Subject, item.Object))
		if name == "" {
			return ""
		}
		return fmt.Sprintf("%s @ %s", name, location)
	}
	return ""
}

func CompileDiagramSurfaceFence(
	dc *DiagramContract,
	scenario Scenario,
	logBundle *LogBundle,
	logObserved []LogSourceDriftAnchor,
	logDrift []LogSourceDriftAnchor,
	flowFindings []FlowFindingDigest,
	answerChains []AnswerChain,
	evidence []EvidenceItem,
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
		case DiagramSequence:
			if fence := RenderLogAnchorSequenceDiagramFence(logObserved); fence != "" {
				return kind, fence
			}
			if fence := RenderLogAnchorSequenceDiagramFence(logDrift); fence != "" {
				return kind, fence
			}
			if fence := RenderLogSequenceDiagramFence(logBundle); fence != "" {
				return kind, fence
			}
			if fence := RenderEvidenceSequenceDiagramFence(evidence); fence != "" {
				return kind, fence
			}
			if fence := RenderFlowFindingSequenceDiagramFence(flowFindings); fence != "" {
				return kind, fence
			}
		case DiagramCallDAG:
			if fence := RenderLogAnchorDiagramFence(logObserved); fence != "" {
				return kind, fence
			}
			if fence := RenderLogAnchorDiagramFence(logDrift); fence != "" {
				return kind, fence
			}
			if fence := RenderLogDiagramFence(logBundle); fence != "" {
				return kind, fence
			}
			if fence := RenderEvidenceCallDiagramFence(evidence); fence != "" {
				return kind, fence
			}
			if fence := RenderFlowFindingDiagramFence(flowFindings); fence != "" {
				return kind, fence
			}
		case DiagramArchitecture:
			if fence := RenderEvidenceArchitectureDiagramFence(evidence); fence != "" {
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
		RequestedEnumerationBoundary: ir.RequestModel.EnumerationBoundary,
		ChangeImpactProfile:          ir.RequestModel.ChangeImpactProfile,
	}
	if ir.RequestModel.RequestedAnswerDimensions != nil && ir.RequestModel.RequestedAnswerDimensions.Active() {
		plan.RequestedAnswerDimensions = append([]RequestedAnswerDimension(nil), ir.RequestModel.RequestedAnswerDimensions.Dimensions...)
	}
	plan.CurrentStatusDiagnosticRequired = ir.RequestModel.DiagnosticProfile.RequiresCurrentStatusDiagnostic()
	if ir.AnswerContract.CurrentStatusDiagnostic != nil && ir.AnswerContract.CurrentStatusDiagnostic.Required {
		plan.CurrentStatusDiagnosticRequired = true
	}
	plan.CurrentSourceEvidenceOrigin = CompileAnswerIntentContract(
		ir.RequestModel,
		&ir.AnswerContract,
	).HasOrigin(AnswerEvidenceOriginCurrentSource)

	plan.ExactResolution = ir.AnswerContract.ExactResolution
	if plan.ExactResolution == nil {
		plan.ExactResolution = BuildExactResolutionContract(ir.RequestModel)
	}

	var perfBundle *PerfBundle
	if mutable != nil {
		plan.StableInvestigationResultKind = strings.TrimSpace(mutable.StableInvestigationResultKind())
		plan.StableAbsenceJustification = strings.TrimSpace(mutable.StableAbsenceJustification())
		plan.StableInvestigationReason = strings.TrimSpace(mutable.StableInvestigationCompleteReason())
		plan.StableAggregateFacts = mutable.StableInvestigationAggregateFacts()
		if ta := mutable.TurnAArtifacts(); ta != nil && len(ta.ValidationBoundaryNotes) > 0 {
			plan.StableValidationBoundaryNotes = append([]string(nil), ta.ValidationBoundaryNotes...)
		}
		plan.ExactContextRequiredFiles = mutable.ExactContextRequiredFiles()
		if syms, claim := mutable.EmittedAnswerSymbols(); len(syms) > 0 {
			ApplyAnswerSymbolStepBackbone(plan, ir, syms, claim)
			if ResolveQuestionFamily(ir.RequestModel) == QFEnumeration {
				plan.StableAggregateFacts = ProjectPrincipalAggregateFactsOntoCompleteAnswerSymbols(
					plan.StableAggregateFacts,
					&ir.RequestModel,
					syms,
					claim,
				)
			}
		}
		plan.StableAggregateFacts = DemoteAggregateCountFactsConflictingWithPrincipalMemberSets(
			plan.StableAggregateFacts,
			&ir.RequestModel,
		)
		plan.StableAggregateFacts = NormalizeAggregateFactRolesForRequest(
			plan.StableAggregateFacts,
			&ir.RequestModel,
		)
		if logBundle == nil {
			logBundle = mutable.LogTriage()
		}
		// Round-8: pull PerfBundle so trace-attached runs participate
		// in the drift-anchor derive pipeline symmetrically with logs.
		perfBundle = mutable.PerfTrace()
		plan.RuntimeGroundingDisposition = RuntimeGroundingDispositionFromWaiver(
			mutable.StableEvidenceFloorWaiver(),
			logBundle,
			perfBundle,
		)
	}
	if plan.RuntimeGroundingDisposition == nil {
		plan.RuntimeGroundingDisposition = SystemRuntimeGroundingDisposition(logBundle, perfBundle)
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
	ApplyEvidenceStepBackbone(plan, ir, plan.SurfaceEvidence)
	applyRequestedEnumerationBoundaryStepBackbone(plan, ir)
	ApplyEvidenceExplanationAnchorBackbone(plan, ir, plan.SurfaceEvidence)
	plan.LogObservedAnchors = CollectArtifactObservedAnchors(
		ir.RequestModel,
		logBundle,
		perfBundle,
		plan.SurfaceEvidence,
	)
	plan.ExternalObservationSeeds = CollectArtifactExternalObservationSeeds(
		logBundle,
		perfBundle,
		plan.LogObservedAnchors,
	)
	plan.LogSourceDriftAnchors = CollectArtifactSourceDriftAnchors(
		ir.RequestModel,
		logBundle,
		perfBundle,
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
	supported = augmentSupportedDiagramKindsForRequiredDiagram(
		supported,
		ir.AnswerContract.Diagram,
		ir.RequestModel,
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
		plan.SurfaceEvidence,
		plan.ConfigTraceDiagramAnchors,
	)
	plan.PreferredExactResolution = preferredExactResolutionSurface(plan)
	plan.SummarySurfaceMode = preferredAnswerSummarySurfaceMode(plan, ir.RequestModel)

	// Phase 1 of Semantic Surface Contract: compile the per-question
	// facet coverage plan from the resolved family + surface evidence.
	// Done BEFORE the no-ExactResolution.Targets early return so every
	// AnswerSurfacePlan carries FacetCoverage regardless of exact-
	// resolution outcome. Pure deterministic projection — no LLM input.
	// B5-F2/F3 telemetry sink: the BusContext-aware caller wires
	// MutableState in here so silent hard→soft downgrades + family-
	// fit fallbacks land in the per-Run RichnessTelemetry channel.
	// Callers without MutableState in scope (tests, raw IR
	// construction) pass nil mutable → the sinks slice is empty and
	// CompileFacetCoverage stays byte-identical to pre-B5-F2.
	var richnessSink RichnessTelemetrySink
	if mutable != nil {
		richnessSink = mutable
	}
	plan.FacetCoverage = CompileFacetCoverage(ir.RequestModel, plan.SurfaceEvidence, richnessSink)
	ApplyPrincipalEvidenceStepBackbone(plan, ir)
	applyRequestedEnumerationBoundaryStepBackbone(plan, ir)

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
	if cached := ctx.cachedAnswerSurfacePlan(); cached != nil {
		return cached
	}
	logBundle := ctx.LogTriage
	if logBundle == nil && ctx.Mutable != nil {
		logBundle = ctx.Mutable.LogTriage()
	}
	plan := BuildAnswerSurfacePlan(
		ctx.AnalysisIR,
		ctx.Mutable,
		logBundle,
		ctx.FlowFindings,
		ctx.AnswerChains,
		ctx.EvidenceItems,
	)
	if plan != nil {
		plan.SubRepoNames = subRepoNamesFrom(ctx.MultiGraph)
	}
	ctx.storeAnswerSurfacePlan(plan)
	return cloneAnswerSurfacePlan(plan)
}

// BuildAnswerSurfacePlanForBusContext is the BusContext equivalent of
// BuildAnswerSurfacePlanForAgentContext. Shared validators and
// tool-stage gates consume the same compiled surface plan instead of
// re-deriving explanation / exact-resolution policy from local slices.
func BuildAnswerSurfacePlanForBusContext(ctx *BusContext) *AnswerSurfacePlan {
	if ctx == nil {
		return nil
	}
	if cached := ctx.cachedAnswerSurfacePlan(); cached != nil {
		return cached
	}
	var logBundle *LogBundle
	if ctx.Mutable != nil {
		logBundle = ctx.Mutable.LogTriage()
	}
	plan := BuildAnswerSurfacePlan(
		ctx.AnalysisIR,
		ctx.Mutable,
		logBundle,
		ctx.FlowFindings,
		ctx.AnswerChains,
		ctx.EvidenceItems,
	)
	if plan != nil {
		plan.SubRepoNames = subRepoNamesFrom(ctx.MultiGraph)
	}
	ctx.storeAnswerSurfacePlan(plan)
	return cloneAnswerSurfacePlan(plan)
}

// subRepoNameSource is the narrow contract that internal/types relies
// on to read multi-repo identifiers without taking a direct dependency
// on the multigraph package (which transitively depends back on
// internal/types). The multigraph.MultiGraph type satisfies this
// interface via its SubRepoNames method.
type subRepoNameSource interface {
	SubRepoNames() []string
}

// subRepoNamesFrom extracts the flattened sub-repo identifier set
// from ctx.MultiGraph (Slug + RootRel + RootRel basename). Returns
// nil when no multigraph is wired or the topology is empty so the
// downstream consumers can treat absence as "single-repo posture".
func subRepoNamesFrom(mg any) []string {
	if mg == nil {
		return nil
	}
	provider, ok := mg.(subRepoNameSource)
	if !ok {
		return nil
	}
	names := provider.SubRepoNames()
	if len(names) == 0 {
		return nil
	}
	return append([]string(nil), names...)
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
	// Round-9 user red line: gate on LLM-classified user intent
	// (rm.Intent), not the partly-system-derived rm.Scenario. The
	// user said: "对齐用户问题意图 (LLM 已前置分析出来) ... 触非用户问题
	// 明确命中, 否则不能系统硬假设" + "当前系统已经对log和trace的场景
	// 进行了用户意图的对齐分类, 请审视, 是否可以复用".
	//
	// Reuses the existing Intent enum (single source of truth set
	// by analyzer's emit_analysis from the LLM's classification of
	// the user's question). Drift surface mode is artifact-driven
	// hedged-causality rendering; it fires when the LLM classified
	// the user as asking either:
	//   - IntentRootCause: "why did X happen" / "explain this panic"
	//   - IntentTrace:     "trace this call path" / "what's the
	//                       sequence here"
	// Both intents inherently care about the artifact's structure
	// vs. current code, so drift between them IS the answer's
	// substance. For IntentExplain / IntentEnumerate / IntentConfig
	// Query / IntentReturnValue / IntentUnknown the drift signal
	// stays visible via the AuthorityCeiling axis (per-step markers
	// + Authority caveat) — the user gets honest provenance without
	// the system overriding their intent.
	diagnosticIntent := rm.Intent == IntentRootCause || rm.Intent == IntentTrace
	if plan.PreferredExactResolution == nil {
		if diagnosticIntent &&
			len(plan.LogSourceDriftAnchors) > 0 &&
			answerWantsLongFormDriftSurface(&AnalysisIR{RequestModel: rm}) {
			return AnswerSummarySurfaceDriftBoundedRootCause
		}
		if IsScalarRoleLocateLookup(rm) {
			return AnswerSummarySurfaceMinimalScalarRoleLocate
		}
		return AnswerSummarySurfaceDefault
	}
	if diagnosticIntent &&
		len(plan.LogSourceDriftAnchors) > 0 &&
		answerWantsLongFormDriftSurface(&AnalysisIR{RequestModel: rm}) {
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

// collectDiagramLogFrames returns frames for diagram-rendering paths
// that REQUIRE both file and line. Pre-round-9 this was also the
// frame source for drift detection — but drift detection should
// not require line. Use collectAllLogFrames for drift-pipeline
// consumers that should process symbol-only frames too.
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

// collectAllLogFrames returns EVERY frame the LLM extracted, with
// only the constraint that Func is non-empty. Frames without
// File/Line are preserved so symbol-axis drift detection can fire
// on them (round-9 user red line: "无行号也不能被丢"). Cap mirrors
// the diagram path (8) but counts the symbol-only frames toward it
// so unsymbolicated traces don't crowd out file-anchored frames.
func collectAllLogFrames(bundle *LogBundle) []LogFrame {
	if bundle == nil || len(bundle.Errors) == 0 {
		return nil
	}
	resolved := make([]LogFrame, 0, 8)
	// Pass 1: file+line frames first (priority).
	WalkLogFrames(bundle, func(frame LogFrame) {
		if len(resolved) >= 8 {
			return
		}
		if frame.File == "" || frame.Line <= 0 {
			return
		}
		resolved = append(resolved, frame)
	})
	if len(resolved) >= 8 {
		return resolved
	}
	// Pass 2: symbol-only frames (Func set, File or Line missing).
	WalkLogFrames(bundle, func(frame LogFrame) {
		if len(resolved) >= 8 {
			return
		}
		if frame.File != "" && frame.Line > 0 {
			return // already taken in pass 1
		}
		if strings.TrimSpace(frame.Func) == "" {
			return
		}
		resolved = append(resolved, frame)
	})
	return resolved
}

// intentFrameCap (B.7 audit followup, 2026-05-02) maps the request
// Intent to the maximum number of artifact frames the drift pipeline
// will keep. Trace and RootCause questions naturally need deeper
// frame walks (the answer often hangs off a 30+-frame stack /
// startup trace), while the default 16-cap remains for explain /
// config-query / scalar questions that don't need that depth.
//
// The B.7 cap is a soft upper bound: log frames go first, then
// perf stalls, jank, startup — so even at the smallest cap the
// most informative frames survive. Raising the cap on Trace /
// RootCause widens the suffix that perf signals get to occupy.
func intentFrameCap(intent Intent) int {
	switch intent {
	case IntentTrace:
		return 32
	case IntentRootCause:
		return 48
	}
	return 16
}

// collectArtifactFrames is the trace-aware counterpart used by the
// drift-anchor derive path. It merges LogBundle frames (Errors[].Frames)
// AND synthesised frames from PerfBundle.Stalls so trace-only runs
// surface drift identically to log-only runs. The synthesised perf
// frames carry the same {File, Line, Func} shape as LogFrame, with
// PerfStall.Symbol mapped to LogFrame.Func.
//
// Cap mirrors collectDiagramLogFrames (8 total frames) to bound the
// drift-detection cost on huge traces. Log frames take precedence
// in the cap order so panic-style log diagnostics stay first.
//
// B.7 audit followup: the cap is now intent-adaptive via
// collectArtifactFramesWithOrigin's intent parameter. Existing call
// sites that have no Intent context pass IntentUnknown which yields
// the historical 16-cap (byte-identical legacy behaviour).
func collectArtifactFrames(logBundle *LogBundle, perfBundle *PerfBundle, intent Intent) []LogFrame {
	frames, _ := collectArtifactFramesWithOrigin(logBundle, perfBundle, intent)
	return frames
}

// collectArtifactFramesWithOrigin returns parallel slices of frames
// and per-frame ClaimOrigin tags, so backfill can propagate the
// correct Origin (log vs perf) when a stall-source frame matches an
// evidence item. The two slices have identical length.
//
// Round-9: log frames preserved even when File/Line missing (Func is
// enough for symbol-axis drift detection); perf surface extended
// from Stalls to include PerfJank (TriggerSpan as Func) and
// PerfStartup (Mode marker). All artifact-derived signals
// participate so the drift pipeline doesn't silently drop trace
// observations whose file/line resolution failed at extraction.
//
// B.7 audit followup (2026-05-02): the previously-hardcoded 16-cap
// is now intent-adaptive via intentFrameCap. Trace=32, RootCause=48,
// default=16. Empty intent (IntentUnknown) yields the 16 default so
// existing callers that don't have RequestModel context stay byte-
// identical to pre-B.7 behaviour.
func collectArtifactFramesWithOrigin(logBundle *LogBundle, perfBundle *PerfBundle, intent Intent) ([]LogFrame, []ClaimOrigin) {
	cap := intentFrameCap(intent)
	resolved := collectAllLogFrames(logBundle)
	origins := make([]ClaimOrigin, len(resolved))
	for i := range origins {
		origins[i] = ClaimOriginLog
	}
	if perfBundle == nil {
		return resolved, origins
	}
	// Perf frames: typed (Symbol, File?, Line?) stalls plus symbol-only
	// jank/startup markers. PerfBundle.LogFrames is the single source for
	// this LogFrame-compatible projection.
	for _, frame := range perfBundle.LogFrames() {
		if len(resolved) >= cap {
			break
		}
		if strings.TrimSpace(frame.Func) == "" {
			continue
		}
		resolved = append(resolved, frame)
		origins = append(origins, ClaimOriginPerf)
	}
	return resolved, origins
}

// CollectLogObservedAnchors returns the FULL set of anchors where a
// log frame matched (any drift OR no drift). Plan-A unification (P3):
// the AuthorityCeiling axis is the SOLE source of drift truth. Items
// without projection (legacy / un-emitted-via-axis) return empty —
// production paths always project via emit_evidence's hook +
// AppendEvidence's BackfillEvidenceProjector. Non-empty rm and
// bundle are accepted for backward compat with the public API
// signature; rm is no longer consulted (the projection ran with
// full ctx awareness at emit time).
//
// Round-8: this signature only sees LogBundle. For trace-aware
// drift detection (perfetto / atrace / hitrace stall drift), prefer
// CollectArtifactObservedAnchors which takes both bundles. Existing
// log-only callers stay byte-identical.
func CollectLogObservedAnchors(rm RequestModel, bundle *LogBundle, items []EvidenceItem) []LogSourceDriftAnchor {
	return CollectArtifactObservedAnchors(rm, bundle, nil, items)
}

// CollectLogSourceDriftAnchors returns ONLY the drifted subset
// (line_drift / tail_rename / file_moved). Plan-A unification (P3):
// derives from items whose Authority is non-factual + Origin in
// {log, perf}. Same projection guarantee as CollectLogObservedAnchors.
//
// Round-8: log-only signature; prefer CollectArtifactSourceDriftAnchors
// for trace-aware drift.
func CollectLogSourceDriftAnchors(rm RequestModel, bundle *LogBundle, items []EvidenceItem) []LogSourceDriftAnchor {
	return CollectArtifactSourceDriftAnchors(rm, bundle, nil, items)
}

// CollectArtifactObservedAnchors is the trace-aware variant: it
// merges PerfBundle stalls into the frame source so traces with or
// without symbolication surface drift symmetrically with logs.
// BuildAnswerSurfacePlan calls this entry point with both bundles.
//
// B.7 audit followup: rm.Intent now drives intentFrameCap, raising
// the cap to 32 for trace-shaped questions and 48 for root-cause
// questions where deeper stack walks materially help drift detection.
func CollectArtifactObservedAnchors(rm RequestModel, logBundle *LogBundle, perfBundle *PerfBundle, items []EvidenceItem) []LogSourceDriftAnchor {
	out := deriveAnchorsFromArtifacts(logBundle, perfBundle, items, false, rm.Intent)
	if out == nil {
		return nil
	}
	return out
}

// CollectArtifactSourceDriftAnchors is the drift-only trace-aware
// variant. Same coverage policy as CollectArtifactObservedAnchors,
// filtered to evidence items whose Authority is non-factual.
func CollectArtifactSourceDriftAnchors(rm RequestModel, logBundle *LogBundle, perfBundle *PerfBundle, items []EvidenceItem) []LogSourceDriftAnchor {
	out := deriveAnchorsFromArtifacts(logBundle, perfBundle, items, true, rm.Intent)
	if out == nil {
		return nil
	}
	return out
}

// deriveLogObservedAnchorsFromEvidence walks emitted evidence items
// that the AuthorityCeiling axis classified as log/perf-derived OR
// cross-source-confirmed. For each, locates the matching log frame
// (by file + tail) to recover ObservedLine, then constructs a
// LogSourceDriftAnchor. Returns nil when NO items carry projection
// data (signals: legacy fallback path required).
//
// Intent is unknown at this internal call-site (no RequestModel in
// scope); IntentUnknown yields the historical 16-cap.
func deriveLogObservedAnchorsFromEvidence(bundle *LogBundle, items []EvidenceItem) []LogSourceDriftAnchor {
	return deriveAnchorsFromArtifacts(bundle, nil, items, false, IntentUnknown)
}

// backfillUnprojectedItemsForDerive synthesises basic Origin /
// Authority / DriftReason projection on items that lack it, using
// bundle frame matching alone. Used by the derive helpers as a
// safety net so test fixtures and any un-projected runtime path
// still produce anchors. Pure function: returns a new slice with
// projected items; original slice is not mutated.
//
// Frame-driven algorithm (matches the legacy detector's per-frame
// "best candidate wins" semantic):
//   - For each frame, find the SAME-FILE same-tail item closest to
//     frame.Line. Project that item only:
//   - gap ≤ 4   → cross-source factual (observed, no drift)
//   - gap > 48  → log conditional, DriftReason = line_drift
//   - 4 < gap ≤ 48 → cross-source factual (close enough, sibling
//     within function but observed line)
//   - Same-file items not picked as "best" by any frame are LEFT
//     un-projected (sibling anchors in the same function don't
//     count as drift of the frame).
//   - For frames with no same-file item, look for cross-file
//     unique tail match → file_moved historical.
//   - Items still un-projected after the frame walk: same-file +
//     no matching tail anywhere → tail_rename conditional, but
//     ONLY when the item is a definition anchor and the frame's
//     same file has at least one frame.Func.
func backfillUnprojectedItemsForDerive(items []EvidenceItem, bundle *LogBundle) []EvidenceItem {
	return backfillUnprojectedItemsForDeriveArtifacts(items, bundle, nil, IntentUnknown)
}

// backfillUnprojectedItemsForDeriveArtifacts is the artifact-aware
// backfill. Trace-only runs need projection too; the merged frame
// source from collectArtifactFrames covers both bundles.
//
// B.7 audit followup: intent threads into the frame collection so
// the cap matches the request's downstream needs. Internal callers
// without RequestModel context pass IntentUnknown.
func backfillUnprojectedItemsForDeriveArtifacts(items []EvidenceItem, logBundle *LogBundle, perfBundle *PerfBundle, intent Intent) []EvidenceItem {
	if (logBundle == nil && perfBundle == nil) || len(items) == 0 {
		return items
	}
	frames, frameOrigins := collectArtifactFramesWithOrigin(logBundle, perfBundle, intent)
	if len(frames) == 0 {
		return items
	}
	out := make([]EvidenceItem, len(items))
	copy(out, items)
	const driftLineGap = 48 // matches legacy logSourceDriftLineGap
	const observedSlack = 4 // legacy near-line tolerance

	// Pass 1: frame-driven best-match selection. For each frame, pick
	// the SAME-FILE same-tail item closest to frame.Line. Definition
	// anchors are preferred; if no definition matches, fall back to
	// any anchor kind. Once an item is projected (Origin/Authority
	// non-zero) it is considered "claimed" and won't be re-picked
	// by a later frame — mirrors legacy "best per frame" semantic.
	pickBest := func(frame LogFrame, definitionOnly bool) int {
		frameFile := strings.TrimSpace(strings.ReplaceAll(frame.File, `\`, `/`))
		frameTail := normalizedSurfaceSymbolTail(frame.Func)
		// Round-9: frame.Line==0 is OK (unsymbolicated / no-line frame).
		// Symbol-axis match still works; classify uses gap=∞ when line
		// missing → conditional via DriftReasonLineDrift only when
		// frame has a real line. With frame.Line==0 we fall through to
		// the cross-source path (no detectable drift, but signal is
		// preserved). frameFile==""可以,frameTail必须有.
		if frameTail == "" {
			return -1
		}
		bestIdx := -1
		bestGap := -1
		for i := range out {
			item := &out[i]
			if item.Origin != ClaimOriginUnknown || item.Authority != AuthorityUnknown {
				continue
			}
			if definitionOnly && item.AnchorKind != AnchorDefinition {
				continue
			}
			if !itemTailMatches(*item, frameTail) {
				continue
			}
			if frameFile != "" && strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`)) != frameFile {
				continue
			}
			if item.LineStart <= 0 {
				continue
			}
			gap := item.LineStart - frame.Line
			if gap < 0 {
				gap = -gap
			}
			if bestIdx < 0 || gap < bestGap {
				bestIdx = i
				bestGap = gap
			}
		}
		return bestIdx
	}
	classify := func(idx, frameLine int, frameOrigin ClaimOrigin) {
		if idx < 0 {
			return
		}
		item := &out[idx]
		gap := item.LineStart - frameLine
		if gap < 0 {
			gap = -gap
		}
		if gap <= observedSlack {
			item.Origin = ClaimOriginCrossSource
			item.Authority = AuthorityFactual
		} else if gap > driftLineGap {
			// Use the frame's source origin (log vs perf) so a
			// trace-detected drift carries ClaimOriginPerf rather
			// than collapsing to ClaimOriginLog.
			item.Origin = frameOrigin
			if item.Origin == ClaimOriginUnknown {
				item.Origin = ClaimOriginLog
			}
			item.Authority = AuthorityConditional
			item.DriftReason = DriftReasonLineDrift
		} else {
			item.Origin = ClaimOriginCrossSource
			item.Authority = AuthorityFactual
		}
	}
	for fi, frame := range frames {
		idx := pickBest(frame, true) // definition-preferred pass
		if idx < 0 {
			idx = pickBest(frame, false) // any-kind fallback
		}
		origin := ClaimOriginLog
		if fi < len(frameOrigins) {
			origin = frameOrigins[fi]
		}
		classify(idx, frame.Line, origin)
	}

	// Pass 2: cross-file unique-tail match → file_moved historical.
	for i := range out {
		item := &out[i]
		if item.Origin != ClaimOriginUnknown || item.Authority != AuthorityUnknown {
			continue
		}
		itemFile := strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`))
		if itemFile == "" {
			continue
		}
		itemTails := itemTailCandidates(*item)
		if len(itemTails) == 0 {
			continue
		}
		matchCount := 0
		matchedOrigin := ClaimOriginLog
		for _, t := range itemTails {
			for fi, f := range frames {
				if normalizedSurfaceSymbolTail(f.Func) != t {
					continue
				}
				ff := strings.TrimSpace(strings.ReplaceAll(f.File, `\`, `/`))
				if ff != itemFile {
					matchCount++
					if fi < len(frameOrigins) && frameOrigins[fi] != ClaimOriginUnknown {
						matchedOrigin = frameOrigins[fi]
					}
				}
			}
		}
		if matchCount == 1 {
			item.Origin = matchedOrigin
			item.Authority = AuthorityHistorical
			item.DriftReason = DriftReasonFileMoved
		}
	}
	return out
}

// itemTailMatches reports whether any of an item's tail-bearing
// fields (AnchorSymbol / OwnerSymbol / Subject / Object) normalises
// to the given frame tail. Mirrors the legacy detector's multi-
// field tail policy so call/relation evidence whose Subject carries
// the symbol identity matches correctly.
func itemTailMatches(item EvidenceItem, frameTail string) bool {
	if frameTail == "" {
		return false
	}
	for _, raw := range []string{item.AnchorSymbol, item.OwnerSymbol, item.Subject, item.Object} {
		if normalizedSurfaceSymbolTail(raw) == frameTail {
			return true
		}
	}
	return false
}

// itemTailCandidates returns the deduplicated set of tail tokens
// from an item's tail-bearing fields. Used by the cross-file file_
// moved pass.
func itemTailCandidates(item EvidenceItem) []string {
	seen := make(map[string]bool)
	var out []string
	for _, raw := range []string{item.AnchorSymbol, item.OwnerSymbol, item.Subject, item.Object} {
		t := normalizedSurfaceSymbolTail(raw)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

// deriveLogSourceDriftAnchorsFromEvidence is the drift-only subset:
// items whose AuthorityCeiling is non-factual (the projection
// detected drift). Mirrors deriveLogObservedAnchorsFromEvidence but
// filters down to drifted items.
func deriveLogSourceDriftAnchorsFromEvidence(bundle *LogBundle, items []EvidenceItem) []LogSourceDriftAnchor {
	return deriveAnchorsFromArtifacts(bundle, nil, items, true, IntentUnknown)
}

// deriveAnchorsFromArtifacts is the frame-driven anchor builder
// shared by the observed (driftedOnly=false) and drift-subset
// (driftedOnly=true) entry points. For each frame from the merged
// log + perf source, picks the best same-file evidence item
// (definition-preferred, line-closest) and constructs ONE anchor
// per frame. Items not matched to any frame produce no anchor.
//
// Round-8: PerfBundle stalls are synthesised into the frame stream
// so trace-only runs (no log attached) produce drift anchors
// identically to log-only runs.
//
// When driftedOnly is true, the result is filtered to anchors whose
// underlying item carries a non-factual Authority OR an explicit
// DriftReason (i.e. the projection determined this is real drift).
func deriveAnchorsFromArtifacts(logBundle *LogBundle, perfBundle *PerfBundle, items []EvidenceItem, driftedOnly bool, intent Intent) []LogSourceDriftAnchor {
	if (logBundle == nil && perfBundle == nil) || len(items) == 0 {
		return nil
	}
	items = backfillUnprojectedItemsForDeriveArtifacts(items, logBundle, perfBundle, intent)
	frames := collectArtifactFrames(logBundle, perfBundle, intent)
	if len(frames) == 0 {
		return nil
	}

	// Frame-driven walk: for each frame, pick the best matching
	// item and emit at most one anchor. Same item MAY satisfy
	// multiple frames (legacy semantic — one log frame per anchor,
	// items are reusable across frames).
	pickFor := func(frame LogFrame, definitionOnly bool) int {
		frameFile := strings.TrimSpace(strings.ReplaceAll(frame.File, `\`, `/`))
		frameTail := normalizedSurfaceSymbolTail(frame.Func)
		// Round-9: frame.Line==0 is OK (unsymbolicated / no-line frame).
		// Symbol-axis match still works; classify uses gap=∞ when line
		// missing → conditional via DriftReasonLineDrift only when
		// frame has a real line. With frame.Line==0 we fall through to
		// the cross-source path (no detectable drift, but signal is
		// preserved). frameFile==""可以,frameTail必须有.
		if frameTail == "" {
			return -1
		}
		bestIdx := -1
		bestGap := -1
		for i := range items {
			item := &items[i]
			if !evidenceItemIsLogOrPerfRelated(*item) {
				continue
			}
			if definitionOnly && item.AnchorKind != AnchorDefinition {
				continue
			}
			if !itemTailMatches(*item, frameTail) {
				continue
			}
			if frameFile != "" && strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`)) != frameFile {
				continue
			}
			if item.LineStart <= 0 {
				continue
			}
			gap := item.LineStart - frame.Line
			if gap < 0 {
				gap = -gap
			}
			if bestIdx < 0 || gap < bestGap {
				bestIdx = i
				bestGap = gap
			}
		}
		return bestIdx
	}

	out := make([]LogSourceDriftAnchor, 0, len(frames))
	seen := make(map[string]bool, len(frames))
	for _, frame := range frames {
		idx := pickFor(frame, true)
		if idx < 0 {
			idx = pickFor(frame, false)
		}
		if idx < 0 {
			continue
		}
		item := items[idx]
		if driftedOnly && !evidenceItemIsDrifted(item) {
			continue
		}
		// Build anchor with the frame's line as ObservedLine and
		// the item's line as AnchoredLine. Frame Func wins for the
		// rendered Func unless DriftReason is tail_rename (in which
		// case the item's AnchorSymbol is the current name).
		anchor := LogSourceDriftAnchor{
			File:         strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`)),
			Func:         strings.TrimSpace(frame.Func),
			ObservedLine: frame.Line,
			AnchoredLine: item.LineStart,
			Reason:       item.DriftReason,
		}
		if item.DriftReason == DriftReasonTailRename {
			anchor.OriginalFunc = strings.TrimSpace(frame.Func)
			if cur := strings.TrimSpace(item.AnchorSymbol); cur != "" {
				anchor.Func = cur
			}
		}
		if item.DriftReason == DriftReasonFileMoved {
			frameFile := strings.TrimSpace(strings.ReplaceAll(frame.File, `\`, `/`))
			if frameFile != "" && frameFile != anchor.File {
				anchor.OriginalFile = frameFile
			}
		}
		key := anchorDedupKey(anchor)
		if !seen[key] {
			seen[key] = true
			out = append(out, anchor)
		}
	}

	if len(out) == 0 {
		return []LogSourceDriftAnchor{}
	}
	sortLogSourceDriftAnchors(out)
	return out
}

// evidenceItemIsLogOrPerfRelated reports whether an item's
// projection ties it to a log/perf frame (cross-source AND the
// log-derived/perf-derived families).
func evidenceItemIsLogOrPerfRelated(item EvidenceItem) bool {
	switch item.Origin {
	case ClaimOriginLog, ClaimOriginPerf, ClaimOriginCrossSource:
		return true
	}
	return false
}

// evidenceItemIsDrifted reports whether an item's projection
// indicates real drift (non-factual ceiling OR explicit DriftReason).
func evidenceItemIsDrifted(item EvidenceItem) bool {
	if item.DriftReason != "" {
		return true
	}
	switch item.Authority {
	case AuthorityConditional, AuthorityHistorical, AuthorityIllustrative:
		return true
	}
	return false
}

// anchorDedupKey produces a stable dedup key for a derived anchor.
func anchorDedupKey(a LogSourceDriftAnchor) string {
	return fmt.Sprintf("%s|%s|%d|%d|%s|%s|%s",
		a.File, a.Func, a.ObservedLine, a.AnchoredLine,
		a.OriginalFile, a.OriginalFunc, a.Reason)
}

// sortLogSourceDriftAnchors mirrors collectLogSourceAnchors's stable
// ordering so legacy and derived outputs are byte-identical when
// they cover the same anchor set.
func sortLogSourceDriftAnchors(anchors []LogSourceDriftAnchor) {
	sort.SliceStable(anchors, func(i, j int) bool {
		if anchors[i].File != anchors[j].File {
			return anchors[i].File < anchors[j].File
		}
		if anchors[i].ObservedLine != anchors[j].ObservedLine {
			return anchors[i].ObservedLine < anchors[j].ObservedLine
		}
		return anchors[i].AnchoredLine < anchors[j].AnchoredLine
	})
}

func CollectExternalObservationSeeds(bundle *LogBundle, observed []LogSourceDriftAnchor) []ExternalObservationSeed {
	return CollectArtifactExternalObservationSeeds(bundle, nil, observed)
}

// SelectExternalObservationSeedsForPrompt returns a compact, balanced
// subset of typed runtime observations for prompt/support-lane surfaces.
// The collector intentionally keeps more than one prompt's worth of facts;
// this selector prevents low-specificity meta facts (error type / signal)
// from starving the stack/span frames that answer frame, trace, and
// cross-language questions. It uses only typed seed fields.
func SelectExternalObservationSeedsForPrompt(seeds []ExternalObservationSeed, limit int) []ExternalObservationSeed {
	if limit <= 0 || len(seeds) == 0 {
		return nil
	}
	if len(seeds) <= limit {
		out := make([]ExternalObservationSeed, len(seeds))
		copy(out, seeds)
		return out
	}
	out := make([]ExternalObservationSeed, 0, limit)
	seen := make(map[string]bool, limit)
	add := func(seed ExternalObservationSeed) bool {
		if len(out) >= limit {
			return false
		}
		key := externalObservationSeedKey(seed)
		if key == "" || seen[key] {
			return false
		}
		seen[key] = true
		out = append(out, seed)
		return true
	}

	summaryCap := 2
	if limit < 4 {
		summaryCap = 1
	}
	addSummaryKind := func(kind string) {
		for _, seed := range seeds {
			if len(out) >= summaryCap {
				return
			}
			if strings.TrimSpace(seed.Kind) == kind {
				add(seed)
			}
		}
	}
	for _, kind := range []string{"error_chain", "error_message", "log_observation", "perf_jank", "perf_stall", "error_type", "signal"} {
		if len(out) >= summaryCap {
			break
		}
		addSummaryKind(kind)
	}
	if len(out) == 0 {
		for _, seed := range seeds {
			if strings.TrimSpace(seed.Kind) == "signal" && add(seed) {
				break
			}
		}
	}

	var frames []ExternalObservationSeed
	for _, seed := range seeds {
		if externalObservationSeedIsFrame(seed) {
			frames = append(frames, seed)
		}
	}
	addDiverseFrameGroups := func(group func(ExternalObservationSeed) string) {
		groups := make(map[string]bool)
		for _, seed := range frames {
			if len(out) >= limit {
				return
			}
			g := group(seed)
			if g == "" || groups[g] {
				continue
			}
			if add(seed) {
				groups[g] = true
			}
		}
	}
	addDiverseFrameGroups(func(seed ExternalObservationSeed) string {
		return strings.ToLower(strings.TrimSpace(seed.Lang))
	})
	addDiverseFrameGroups(func(seed ExternalObservationSeed) string {
		return strings.ToLower(strings.TrimSpace(strings.ReplaceAll(seed.File, `\`, `/`)))
	})
	for _, seed := range frames {
		if len(out) >= limit {
			break
		}
		add(seed)
	}
	for _, seed := range seeds {
		if len(out) >= limit {
			break
		}
		add(seed)
	}
	return out
}

func externalObservationSeedKey(seed ExternalObservationSeed) string {
	kind := strings.TrimSpace(seed.Kind)
	raw := strings.TrimSpace(seed.Raw)
	role := strings.TrimSpace(strings.ToLower(seed.Role))
	lang := strings.TrimSpace(strings.ToLower(seed.Lang))
	file := strings.TrimSpace(strings.ReplaceAll(seed.File, `\`, `/`))
	fn := strings.TrimSpace(seed.Func)
	if kind == "" && raw == "" && role == "" && lang == "" && file == "" && seed.Line <= 0 && fn == "" {
		return ""
	}
	return kind + "|" + raw + "|" + role + "|" + lang + "|" + file + "|" + fmt.Sprintf("%d", seed.Line) + "|" + fn
}

func externalObservationSeedIsFrame(seed ExternalObservationSeed) bool {
	switch strings.TrimSpace(seed.Kind) {
	case "log_frame", "frame", "perf_frame":
		return true
	default:
		return false
	}
}

func ExternalObservationSeedIsFrame(seed ExternalObservationSeed) bool {
	return externalObservationSeedIsFrame(seed)
}

func externalObservationSeedIsErrorHeadFrame(seed ExternalObservationSeed) bool {
	return externalObservationSeedIsFrame(seed) && strings.TrimSpace(seed.Role) == "error_head_frame"
}

func externalObservationFrameSeedPoolLimit() int {
	// Reuse the existing root-cause frame budget: attached artifacts are
	// most often consumed by diagnostic/root-cause/trace surfaces, and this
	// budget was already raised for deeper stack walks and mixed-language
	// cause chains.
	if n := intentFrameCap(IntentRootCause); n > 0 {
		return n
	}
	return 48
}

func CollectArtifactExternalObservationSeeds(bundle *LogBundle, perf *PerfBundle, observed []LogSourceDriftAnchor) []ExternalObservationSeed {
	if bundle == nil {
		return collectPerfExternalObservationSeeds(perf, nil)
	}
	anchorByObserved := make(map[string]LogSourceDriftAnchor, len(observed))
	anchorByFunc := make(map[string]LogSourceDriftAnchor, len(observed))
	for _, anchor := range observed {
		file := strings.TrimSpace(strings.ReplaceAll(anchor.File, `\`, `/`))
		if file != "" && anchor.ObservedLine > 0 {
			anchorByObserved[fmt.Sprintf("%s:%d", file, anchor.ObservedLine)] = anchor
		}
		if tail := normalizedSurfaceSymbolTail(anchor.Func); tail != "" {
			anchorByFunc[tail] = anchor
		}
	}
	var out []ExternalObservationSeed
	seen := make(map[string]bool)
	record := func(seed ExternalObservationSeed) bool {
		key := externalObservationSeedKey(seed)
		if key == "" || seen[key] {
			return false
		}
		seen[key] = true
		out = append(out, seed)
		return true
	}
	summaryCount := 0
	recordSummary := func(seed ExternalObservationSeed) {
		if summaryCount >= externalObservationSummarySeedPoolLimit {
			return
		}
		if record(seed) {
			summaryCount++
		}
	}
	frameCount := 0
	frameLimit := externalObservationFrameSeedPoolLimit()
	recordFrame := func(seed ExternalObservationSeed) {
		if frameCount >= frameLimit {
			return
		}
		if record(seed) {
			frameCount++
		}
	}
	residueCount := 0
	recordResidue := func(seed ExternalObservationSeed) {
		if residueCount >= externalObservationResidueSeedPoolLimit {
			return
		}
		if record(seed) {
			residueCount++
		}
	}
	for _, msg := range LogBundleErrorMessages(bundle) {
		recordSummary(ExternalObservationSeed{
			Kind: "error_message",
			Raw:  strings.TrimSpace(msg),
		})
	}
	WalkLogErrors(bundle, func(err *LogError) {
		raw := renderLogErrorCauseChainSeed(err)
		if raw == "" {
			return
		}
		recordSummary(ExternalObservationSeed{
			Kind: "error_chain",
			Raw:  raw,
		})
	})
	for _, typ := range LogBundleErrorTypes(bundle) {
		recordSummary(ExternalObservationSeed{
			Kind: "error_type",
			Raw:  strings.TrimSpace(typ),
		})
	}
	for _, sig := range bundle.Meta.Signals {
		name := strings.TrimSpace(string(sig))
		if name == "" {
			continue
		}
		recordSummary(ExternalObservationSeed{
			Kind: "signal",
			Raw:  name,
		})
	}
	for _, obs := range bundle.Observations {
		summary := strings.TrimSpace(obs.Summary)
		if summary == "" {
			continue
		}
		raw := summary
		if obs.LineStart > 0 {
			if obs.LineEnd > obs.LineStart {
				raw = fmt.Sprintf("log_lines=%d-%d %s", obs.LineStart, obs.LineEnd, raw)
			} else {
				raw = fmt.Sprintf("log_line=%d %s", obs.LineStart, raw)
			}
		}
		if len(raw) > 240 {
			raw = raw[:240]
		}
		recordSummary(ExternalObservationSeed{
			Kind: "log_observation",
			Raw:  raw,
			Func: strings.TrimSpace(obs.Subject),
		})
	}
	WalkLogErrors(bundle, func(err *LogError) {
		if frameCount >= frameLimit {
			return
		}
		if err == nil {
			return
		}
		for i, frame := range err.Frames {
			if frameCount >= frameLimit {
				return
			}
			if strings.TrimSpace(frame.Raw) == "" && strings.TrimSpace(frame.Func) == "" {
				continue
			}
			role := "caller_frame"
			if i == 0 {
				role = "error_head_frame"
			}
			seed := ExternalObservationSeed{
				Kind: "log_frame",
				Raw:  strings.TrimSpace(frame.Raw),
				Role: role,
				Lang: firstNonEmptySurfaceString(frame.Lang, bundle.Meta.Lang),
				File: strings.TrimSpace(strings.ReplaceAll(frame.File, `\`, `/`)),
				Line: frame.Line,
				Func: strings.TrimSpace(frame.Func),
			}
			if seed.File != "" && seed.Line > 0 {
				if anchor, ok := anchorByObserved[fmt.Sprintf("%s:%d", seed.File, seed.Line)]; ok {
					seed.AnchoredFile = strings.TrimSpace(strings.ReplaceAll(anchor.File, `\`, `/`))
					seed.AnchoredLine = anchor.AnchoredLine
				}
			}
			if (seed.AnchoredFile == "" || seed.AnchoredLine <= 0) && seed.Func != "" {
				if anchor, ok := anchorByFunc[normalizedSurfaceSymbolTail(seed.Func)]; ok {
					seed.AnchoredFile = strings.TrimSpace(strings.ReplaceAll(anchor.File, `\`, `/`))
					seed.AnchoredLine = anchor.AnchoredLine
				}
			}
			recordFrame(seed)
		}
	})
	for _, seed := range collectPerfExternalObservationSeeds(perf, observed) {
		record(seed)
	}
	// Round-9 user red line: "log/trace 里也有大量没有行号的不规则
	// 输出和打印, 这些信息如果对齐了用户问题意图, 也不能在后续流程中
	// 被轻易被忽略". Surface Residue.UnknownChunks as residue seeds
	// so downstream consumers (skill prompts, evidence searches)
	// can still consult unstructured log/print output that the LLM
	// extractor couldn't parse into typed errors. The collector keeps a
	// larger typed pool; prompt/support renderers choose a balanced
	// subset so residue never crowds out frames or structured signals.
	for _, chunk := range bundle.Residue.UnknownChunks {
		raw := strings.TrimSpace(chunk)
		if raw == "" {
			continue
		}
		// Truncate to a reasonable preview length for prompt budgets.
		if len(raw) > 240 {
			raw = raw[:240]
		}
		recordResidue(ExternalObservationSeed{
			Kind: "log_residue",
			Raw:  raw,
		})
	}
	return out
}

func renderLogErrorCauseChainSeed(err *LogError) string {
	if err == nil || err.Cause == nil {
		return ""
	}
	outer := logErrorSurface(err)
	inner := logErrorSurface(err.Cause)
	if outer == "" || inner == "" {
		return ""
	}
	return fmt.Sprintf("runtime cause chain: upstream %s is wrapped / re-raised as top-level %s", inner, outer)
}

func logErrorSurface(err *LogError) string {
	if err == nil {
		return ""
	}
	typ := strings.TrimSpace(err.Type)
	msg := strings.TrimSpace(strings.Join(strings.Fields(err.Message), " "))
	switch {
	case typ != "" && msg != "":
		return fmt.Sprintf("%s(%q)", typ, msg)
	case typ != "":
		return typ
	case msg != "":
		return fmt.Sprintf("%q", msg)
	default:
		return ""
	}
}

func collectPerfExternalObservationSeeds(bundle *PerfBundle, observed []LogSourceDriftAnchor) []ExternalObservationSeed {
	if bundle == nil || !bundle.HasStructuredObservations() {
		return nil
	}
	anchorByFunc := make(map[string]LogSourceDriftAnchor, len(observed))
	for _, anchor := range observed {
		if tail := normalizedSurfaceSymbolTail(anchor.Func); tail != "" {
			anchorByFunc[tail] = anchor
		}
	}
	var out []ExternalObservationSeed
	record := func(seed ExternalObservationSeed) {
		if strings.TrimSpace(seed.Raw) == "" && strings.TrimSpace(seed.Func) == "" {
			return
		}
		if seed.Func != "" {
			if anchor, ok := anchorByFunc[normalizedSurfaceSymbolTail(seed.Func)]; ok {
				seed.AnchoredFile = strings.TrimSpace(strings.ReplaceAll(anchor.File, `\`, `/`))
				seed.AnchoredLine = anchor.AnchoredLine
			}
		}
		out = append(out, seed)
	}
	for _, j := range bundle.Janks {
		raw := strings.TrimSpace(j.Reason)
		if raw == "" {
			raw = fmt.Sprintf("%.2fms jank", j.DurationMs)
		}
		record(ExternalObservationSeed{Kind: "perf_jank", Raw: raw, Func: strings.TrimSpace(j.TriggerSpan)})
		if len(out) >= 6 {
			return out
		}
	}
	for _, s := range bundle.Stalls {
		raw := strings.TrimSpace(s.Kind)
		if raw == "" {
			raw = fmt.Sprintf("%.2fms stall", s.DurationMs)
		}
		record(ExternalObservationSeed{
			Kind: "perf_stall",
			Raw:  raw,
			File: strings.TrimSpace(strings.ReplaceAll(s.File, `\`, `/`)),
			Line: s.Line,
			Func: strings.TrimSpace(s.Symbol),
		})
		if len(out) >= 6 {
			return out
		}
	}
	for _, f := range bundle.Frames {
		record(ExternalObservationSeed{
			Kind: "perf_frame",
			Raw:  fmt.Sprintf("frame %d duration %.2fms", f.FrameNo, f.DurationMs),
		})
		if len(out) >= 6 {
			return out
		}
	}
	for _, obs := range bundle.Observations {
		raw := strings.TrimSpace(firstNonEmptySurfaceString(obs.Summary, obs.Evidence, obs.Subject))
		if raw == "" {
			continue
		}
		record(ExternalObservationSeed{
			Kind: "perf_observation",
			Raw:  raw,
			Func: strings.TrimSpace(obs.Subject),
		})
		if len(out) >= 6 {
			return out
		}
	}
	if bundle.Startup != nil {
		record(ExternalObservationSeed{
			Kind: "perf_startup",
			Raw:  fmt.Sprintf("%s startup %.2fms", bundle.Startup.Mode, bundle.Startup.AppLaunchMs),
		})
	}
	return out
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
	haveCallEdge := false
	if ev, ok := bestDriftBoundedCallEdge(items, anchorFiles, outerFunc, innerFunc); ok {
		out = append(out, ev)
		haveCallEdge = true
	}
	if ev, ok := bestDriftBoundedFunctionItem(items, anchorFiles, innerFunc, out); ok {
		out = append(out, ev)
	}
	if ev, ok := bestDriftBoundedCompanionItem(items, anchorFiles, innerFunc, out); ok {
		out = append(out, ev)
	}
	if !haveCallEdge {
		if ev, ok := bestDriftBoundedFunctionItem(items, anchorFiles, outerFunc, out); ok {
			out = append(out, ev)
		}
	}
	if len(out) == 0 {
		if ev, ok := bestDriftBoundedFunctionItem(items, anchorFiles, outerFunc, out); ok {
			out = append(out, ev)
		}
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
		case AnchorAssignment, AnchorInitializer:
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

func bestDriftBoundedCompanionItem(items []EvidenceItem, anchorFiles map[string]bool, fn string, existing []EvidenceItem) (EvidenceItem, bool) {
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
		if !driftBoundedMentionsFunc(item, target) {
			continue
		}
		if driftBoundedIsCallItem(item) && item.Kind != EvidenceMechanism && item.Kind != EvidenceDirect {
			continue
		}
		score := 0
		switch item.AnchorKind {
		case AnchorAssignment, AnchorInitializer:
			score += 500
		case AnchorReturn:
			score += 450
		case AnchorCall:
			score += 420
		case AnchorCondition:
			score += 350
		case AnchorDefinition:
			score += 300
		default:
			score += 100
		}
		switch item.Kind {
		case EvidenceDirect:
			score += 40
		case EvidenceConditional:
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
		case AnchorReturn, AnchorAssignment, AnchorInitializer:
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
	for _, raw := range []string{item.AnchorSymbol, item.OwnerSymbol, item.Subject, item.Object} {
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

func evidenceSurfaceSymbolTails(item EvidenceItem) []string {
	var out []string
	seen := make(map[string]bool)
	for _, raw := range []string{item.AnchorSymbol, item.OwnerSymbol, item.Subject, item.Object} {
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
	return ConfigTraceSurfaceDiagramRoleInFiles(contract, item, requiredFiles)
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
