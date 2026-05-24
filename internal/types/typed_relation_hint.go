package types

import "strings"

// TypedRelationHint is a system-derived structural-relation candidate
// surfaced to the LLM as input HINTING (not as auto-filled answer).
//
// Generalisation of the s5a / LoopController class of bug: when the
// user asks an enumeration / relational-lookup / count question whose
// answer set is defined by a typed graph relation (interface
// implementation, inheritance, callers, scope membership, override,
// reference, registration), grep-based explorer investigation can
// silently miss members (truncation, blank-identifier params,
// cross-file dispatch). The typed graph (repomap) DOES have the
// complete relation; we surface that via this hint so the LLM has
// the full set as a structured input — but the LLM remains the SOLE
// author of doc.Symbols / Summary etc. Per the
// feedback_no_system_backfill_to_user_panel red line, this struct
// flows into the agent's prompt context, NEVER into AnswerDocument.
//
// Render-time merge contract: the prompt assembler unifies hints with
// the LLM-emitted EvidenceItem pool into a SINGLE "Structured
// Evidence" table with a Provenance column distinguishing
// "llm_evidence" (LLM's emit_evidence rows) from system-derived
// provenance such as "typed_graph" or "typed_observation". Dedup by
// (Subject, Object, AnchorKind) — when both sources cover the same tuple the
// LLM-emit row wins (it carries richer rationale).
type TypedRelationHint struct {
	// Relation names the typed graph relation surfaced in this hint.
	// Closed enum: implements / extends / called-by / scoped-to /
	// registers / overrides / references. Adding a new value requires
	// adding a probe in internal/context/typed_relations.go.
	Relation TypedRelationKind `json:"relation"`

	// SourceName is the user-named anchor entity the relation is
	// keyed on (interface name / class name / function name / package
	// path). Verbatim from analyzer entities.
	SourceName string `json:"source_name"`

	// SourceKind classifies SourceName's entity kind for prompt
	// rendering ("interface" / "trait" / "protocol" / "class" /
	// "function" / "package" / etc.). Read from typed Symbol.Kind.
	SourceKind string `json:"source_kind"`

	// Members is the deterministic set of relation members. Order is
	// stable (alphabetic by Member.Name). Capped at the probe site to
	// prevent prompt bloat on huge relations.
	Members []TypedRelationMember `json:"members"`

	// Provenance names the system carrier family used to derive this hint.
	// Empty means typed_graph for backward compatibility.
	Provenance TypedRelationProvenance `json:"provenance,omitempty"`
}

// TypedRelationMember is one member of a TypedRelationHint set.
type TypedRelationMember struct {
	Name string `json:"name"`
	File string `json:"file"`
	Line int    `json:"line"`
	Kind string `json:"kind"`
	// Distance encodes transitive-closure depth for relations where
	// depth matters (inheritance chain, call-graph). 1 = direct; 2+ =
	// transitive. Direct-only relations leave it 0 / 1.
	Distance int `json:"distance,omitempty"`
}

// TypedRelationKind is the closed relation vocabulary shared by
// prompt hints, coverage checks, and future relation providers.
//
// Values are intentionally stable wire strings: existing JSON,
// prompt rendering, and aggregate relation surfaces continue to use
// the same text values while internal code gets a typed boundary.
type TypedRelationKind string

const (
	TypedRelationImplements TypedRelationKind = "implements"
	TypedRelationExtends    TypedRelationKind = "extends"
	TypedRelationCalledBy   TypedRelationKind = "called-by"
	TypedRelationScopedTo   TypedRelationKind = "scoped-to"
	TypedRelationRegisters  TypedRelationKind = "registers"
	TypedRelationOverrides  TypedRelationKind = "overrides"
	TypedRelationReferences TypedRelationKind = "references"
	TypedRelationImports    TypedRelationKind = "imports"
	TypedRelationExports    TypedRelationKind = "exports"
	// TypedRelationConfigures links a configuration key/value surface to the
	// code site that reads, validates, or applies it. It is evidence-driven and
	// prompt-first: the initial provider exposes accepted structured evidence as
	// context, not as a system-authored answer or hard coverage gate.
	TypedRelationConfigures TypedRelationKind = "configures"
	// TypedRelationRoutesTo links a route/path/event endpoint literal to the
	// handler or registration site that serves it. Like configures, the initial
	// carrier is evidence-driven and prompt-only until exact hard-gate semantics
	// are proven across languages/frameworks.
	TypedRelationRoutesTo TypedRelationKind = "routes-to"
	// TypedRelationSourceAnchor links an exact external observation
	// (VCS/log/trace/command/MCP/web/connector/etc.) to an exact
	// current-checkout source anchor through the shared relation contract.
	// It is not a current-source citation by itself.
	TypedRelationSourceAnchor TypedRelationKind = "source-anchor"
)

// TypedRelationPurpose tells providers whether candidates are being
// assembled for prompt context or for a hard coverage gate. Providers
// may expose softer rows for prompt context, but coverage gates must
// consume exact rows only.
type TypedRelationPurpose string

const (
	TypedRelationPurposePromptHint   TypedRelationPurpose = "prompt_hint"
	TypedRelationPurposeCoverageGate TypedRelationPurpose = "coverage_gate"
)

// TypedRelationPrecision records how trustworthy a relation carrier is.
// Only exact precision levels may participate in hard coverage gates;
// name-only and heuristic rows are guidance for the model, never proof
// that the model is wrong.
type TypedRelationPrecision string

const (
	TypedRelationPrecisionExactSymbolID TypedRelationPrecision = "exact_symbol_id"
	TypedRelationPrecisionExactFile     TypedRelationPrecision = "exact_file"
	TypedRelationPrecisionExactEvidence TypedRelationPrecision = "exact_evidence"
	TypedRelationPrecisionNameOnly      TypedRelationPrecision = "name_only"
	TypedRelationPrecisionHeuristic     TypedRelationPrecision = "heuristic"
)

// CoverageGateEligible reports whether this precision is exact enough
// to support a hard relation coverage check.
func (p TypedRelationPrecision) CoverageGateEligible() bool {
	switch p {
	case TypedRelationPrecisionExactSymbolID,
		TypedRelationPrecisionExactFile,
		TypedRelationPrecisionExactEvidence:
		return true
	default:
		return false
	}
}

// TypedRelationCarrierKind identifies the structural source of a
// relation candidate. It is diagnostic/provenance metadata, not a
// user-facing answer label.
type TypedRelationCarrierKind string

const (
	TypedRelationCarrierGraph               TypedRelationCarrierKind = "graph"
	TypedRelationCarrierMultiGraph          TypedRelationCarrierKind = "multi_graph"
	TypedRelationCarrierEvidence            TypedRelationCarrierKind = "evidence"
	TypedRelationCarrierExternalObservation TypedRelationCarrierKind = "external_observation"
)

// TypedRelationQuery is the cycle-free request passed to relation
// providers. It contains only typed analyzer/request data and scoped
// source names; providers must not inspect raw user prose.
type TypedRelationQuery struct {
	Kinds      []TypedRelationKind  `json:"kinds,omitempty"`
	Sources    []string             `json:"sources,omitempty"`
	Request    *RequestModel        `json:"-"`
	MaxMembers int                  `json:"max_members,omitempty"`
	Purpose    TypedRelationPurpose `json:"purpose,omitempty"`
}

// TypedRelationSourceFact is an exact carrier-side fact about a query source.
// It lets relation selection use resolved graph/evidence metadata (for example
// "LoopController is an interface") without inspecting raw request prose or
// model text.
type TypedRelationSourceFact struct {
	Name      string                   `json:"name"`
	Kind      string                   `json:"kind,omitempty"`
	File      string                   `json:"file,omitempty"`
	Line      int                      `json:"line,omitempty"`
	Carrier   TypedRelationCarrierKind `json:"carrier,omitempty"`
	Precision TypedRelationPrecision   `json:"precision,omitempty"`
}

// TypedRelationSourceFactProvider is the optional provider boundary for exact
// source-kind facts used by BuildTypedRelationQueryWithResolvedSources.
type TypedRelationSourceFactProvider interface {
	TypedRelationSourceFacts(sources []string) []TypedRelationSourceFact
}

// BuildTypedRelationQuery is the single request-shape selector for typed
// relation providers. It maps typed analyzer/request fields onto the closed
// relation vocabulary and carries the provenance-aware source candidates that
// providers may resolve.
//
// The selector intentionally does not inspect RawRequest, analyzer EntityAxes
// free-form text, model prose, or localized keywords. A selected relation kind
// is only permission to query an exact provider; hard gates still require exact
// carrier precision, current-source scope, grounded member evidence, and a
// model-authored member_set before they may complain.
func BuildTypedRelationQuery(rm RequestModel, purpose TypedRelationPurpose, maxMembers int) TypedRelationQuery {
	return BuildTypedRelationQueryWithResolvedSources(rm, purpose, maxMembers, nil)
}

// BuildTypedRelationQueryWithResolvedSources extends BuildTypedRelationQuery
// with exact source facts from a relation provider. This is still a typed
// selector: source facts are graph/evidence records with precision metadata,
// not prose-derived guesses.
func BuildTypedRelationQueryWithResolvedSources(rm RequestModel, purpose TypedRelationPurpose, maxMembers int, resolved []TypedRelationSourceFact) TypedRelationQuery {
	kinds := appendTypedRelationKinds(nil, TypedRelationKindsForRequest(rm, purpose)...)
	kinds = appendTypedRelationKinds(kinds, TypedRelationKindsForResolvedSources(rm, purpose, resolved)...)
	sources := TypedRelationQuerySources(rm, purpose)
	if len(kinds) == 0 {
		return TypedRelationQuery{
			Sources:    sources,
			Purpose:    purpose,
			MaxMembers: maxMembers,
		}
	}
	return TypedRelationQuery{
		Kinds:      kinds,
		Sources:    sources,
		Request:    &rm,
		MaxMembers: maxMembers,
		Purpose:    purpose,
	}
}

// TypedRelationKindsForRequest maps typed request-shape fields onto relation
// kinds. It preserves the historical prompt hint behavior for bounded
// category/count/relational shapes by asking for implementer rows, while future
// providers can key off the same query for call, registration, inheritance, and
// import/export families.
func TypedRelationKindsForRequest(rm RequestModel, purpose TypedRelationPurpose) []TypedRelationKind {
	var out []TypedRelationKind
	add := func(kinds ...TypedRelationKind) {
		out = appendTypedRelationKinds(out, kinds...)
	}

	switch rm.PredicateAxis {
	case AxisImplement:
		add(TypedRelationImplements)
	case AxisCall:
		add(TypedRelationCalledBy)
	case AxisRegister:
		add(TypedRelationRegisters)
	case AxisConfigure:
		if purpose == TypedRelationPurposePromptHint {
			add(TypedRelationConfigures)
		}
	}

	switch NormalizeRequirementKind(rm.AnalyzerHints.Kind) {
	case ReqCallChain:
		add(TypedRelationCalledBy)
	case ReqRegistration:
		add(TypedRelationRegisters)
	case ReqConfigMapping:
		if purpose == TypedRelationPurposePromptHint {
			add(TypedRelationConfigures)
		}
	}

	if HasInterfaceTypedRelationDiagramShape(rm) {
		add(TypedRelationImplements, TypedRelationExtends)
	}

	if rm.Predicates.IsCategoryEnumeration ||
		rm.Predicates.IsRelationalLookup ||
		rm.Predicates.IsCountQuestion {
		// Compatibility lane: the only exact provider shipped today is
		// interface/trait/protocol implementer membership. Keeping this in
		// the central selector prevents call sites from reintroducing their
		// own "if enumeration then implements" branches.
		add(TypedRelationImplements)
		if rm.AnswerSubject.Kind == SubjectInterface {
			add(TypedRelationExtends)
		}
	}

	if sourceInventoryRequestsImportPath(rm) {
		add(TypedRelationImports, TypedRelationExports)
	}
	if purpose == TypedRelationPurposePromptHint && requestNeedsRouteRelation(rm) {
		add(TypedRelationRoutesTo)
	}
	if requestNeedsExternalObservationSourceAnchorRelation(rm) {
		add(TypedRelationSourceAnchor)
	}
	if purpose == TypedRelationPurposePromptHint && changeImpactRequestsReferenceRelation(rm) {
		add(TypedRelationReferences)
	}

	return out
}

func changeImpactRequestsReferenceRelation(rm RequestModel) bool {
	profile := rm.ChangeImpactProfile
	if profile == nil || !profile.Active() || strings.TrimSpace(profile.Target) == "" {
		return false
	}
	if profile.RequestedBroadAffectedSites() {
		return true
	}
	for _, kind := range profile.AffectedSiteKinds {
		switch kind {
		case ImpactSiteRead,
			ImpactSiteCall,
			ImpactSiteImport,
			ImpactSiteConstruction,
			ImpactSiteValidation,
			ImpactSiteSerialization,
			ImpactSiteConfig,
			ImpactSiteBuild:
			return true
		}
	}
	return false
}

func requestNeedsExternalObservationSourceAnchorRelation(rm RequestModel) bool {
	contract := CompileAnswerIntentContract(rm, nil)
	if !contract.HasOrigin(AnswerEvidenceOriginCurrentSource) {
		return false
	}
	hasExternal := false
	for _, origin := range contract.Origins {
		if origin != AnswerEvidenceOriginCurrentSource && AnswerEvidenceOriginCarriesOriginSpecificSupport(origin) {
			hasExternal = true
			break
		}
	}
	if !hasExternal {
		return false
	}
	if rm.CurrentSourceExplanationProfile != nil && rm.CurrentSourceExplanationProfile.Active() {
		return true
	}
	if rm.HasRuntimeArtifactCurrentVerificationAnchor() {
		return true
	}
	if rm.Predicates.IsHistoryLookup &&
		(contract.HasOutput(AnswerRequestedOutputMechanism) ||
			contract.HasOutput(AnswerRequestedOutputChangeImpact) ||
			contract.HasOutput(AnswerRequestedOutputDiagram) ||
			contract.HasOutput(AnswerRequestedOutputComparison)) {
		return true
	}
	return false
}

// TypedRelationKindsForResolvedSources selects relation families from exact
// provider facts. It currently enables interface / trait / protocol diagrams to
// receive implementer/inheritance candidates even when the analyzer only emitted
// a broad diagram hint. This is prompt guidance only; coverage gates still need
// independently grounded evidence before they can reject anything.
func TypedRelationKindsForResolvedSources(rm RequestModel, purpose TypedRelationPurpose, resolved []TypedRelationSourceFact) []TypedRelationKind {
	if purpose != TypedRelationPurposePromptHint || rm.DiagramHint == nil || rm.DiagramHint.Kind == DiagramNone {
		return nil
	}
	for _, fact := range resolved {
		if !fact.Precision.CoverageGateEligible() {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(fact.Kind)) {
		case "interface", "trait", "protocol":
			return []TypedRelationKind{TypedRelationImplements, TypedRelationExtends}
		}
	}
	return nil
}

// TypedRelationQuerySources returns the source-entity lane for typed relation
// probes. Prompt hints retain the previous soft fallback to analyzer entities
// when no provenance lane exists; coverage gates retain their stricter
// primary/entity fallback because candidate rows are filtered again by exact
// precision, source scope, and grounded evidence before any hard complaint.
func TypedRelationQuerySources(rm RequestModel, purpose TypedRelationPurpose) []string {
	out := StructuralRelationScopeCandidates(rm)
	if len(out) > 0 {
		return out
	}
	switch purpose {
	case TypedRelationPurposePromptHint:
		return dedupTypedRelationStrings(rm.AnalyzerHints.Entities)
	case TypedRelationPurposeCoverageGate:
		return dedupTypedRelationStrings(append(append([]string{}, rm.AnalyzerHints.PrimaryEntities...), rm.AnalyzerHints.Entities...))
	default:
		return nil
	}
}

// HasTypedRelationQueryShape reports whether the typed request model selects
// any relation family for the supplied purpose.
func HasTypedRelationQueryShape(rm RequestModel, purpose TypedRelationPurpose) bool {
	return len(TypedRelationKindsForRequest(rm, purpose)) > 0
}

func appendTypedRelationKinds(dst []TypedRelationKind, kinds ...TypedRelationKind) []TypedRelationKind {
	for _, kind := range kinds {
		if kind == "" {
			continue
		}
		seen := false
		for _, existing := range dst {
			if existing == kind {
				seen = true
				break
			}
		}
		if !seen {
			dst = append(dst, kind)
		}
	}
	return dst
}

func sourceInventoryRequestsImportPath(rm RequestModel) bool {
	if rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		return false
	}
	for _, role := range rm.SourceInventoryProfile.PrincipalTargetRoles() {
		if role == AnswerCandidateRoleImportPath {
			return true
		}
	}
	return false
}

func requestNeedsRouteRelation(rm RequestModel) bool {
	if rm.AnswerSubject.Kind == SubjectHandlerRoute {
		return true
	}
	if rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.Active() {
		return false
	}
	for _, role := range rm.SourceInventoryProfile.PrincipalTargetRoles() {
		if role == AnswerCandidateRoleRoute {
			return true
		}
	}
	return false
}

func dedupTypedRelationStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

// AllowsKind reports whether a provider should return rows for kind.
// An empty Kinds list means "any supported kind".
func (q TypedRelationQuery) AllowsKind(kind TypedRelationKind) bool {
	if kind == "" {
		return false
	}
	if len(q.Kinds) == 0 {
		return true
	}
	for _, candidate := range q.Kinds {
		if candidate == kind {
			return true
		}
	}
	return false
}

// TypedRelationCandidate is the internal carrier used by relation
// coverage checks. Prompt rendering may still use TypedRelationHint,
// but both should be derived from the same provider data over time.
type TypedRelationCandidate struct {
	Relation   TypedRelationKind        `json:"relation"`
	SourceName string                   `json:"source_name,omitempty"`
	SourceKind string                   `json:"source_kind,omitempty"`
	SourceFile string                   `json:"source_file,omitempty"`
	SourceLine int                      `json:"source_line,omitempty"`
	Member     TypedRelationMember      `json:"member"`
	Carrier    TypedRelationCarrierKind `json:"carrier,omitempty"`
	Precision  TypedRelationPrecision   `json:"precision,omitempty"`
}

// CoverageGateEligible reports whether the candidate can be considered
// by a hard relation coverage gate. Callers must still verify source
// scope and same-member grounded evidence before rejecting anything.
func (c TypedRelationCandidate) CoverageGateEligible() bool {
	return c.Relation != "" && c.Precision.CoverageGateEligible()
}

// TypedRelationCandidateSource is the common, cycle-free relation
// provider boundary. Multi-repo carriers, external observation ledgers,
// and future graph adapters can implement this without downstream
// packages importing their concrete types.
type TypedRelationCandidateSource interface {
	TypedRelationCandidates(TypedRelationQuery) []TypedRelationCandidate
}

// TypedRelationImplementerSource is the narrow, cycle-free bridge for
// relation consumers that need multi-repo implementer members without
// importing the concrete repomap/multigraph package. Single-repo callers may
// still read *repomap/types.Graph directly; multi-repo carriers implement this
// interface and return path-from-parent file surfaces.
type TypedRelationImplementerSource interface {
	ImplementerMembersOf(interfaceName string) []TypedRelationMember
}

// TypedRelationProvenance is the Provenance column tag used in the unified
// Structured Evidence rendering. LLM evidence is model-authored; typed_graph
// and typed_observation are system-derived prompt hints and never answer
// authors.
type TypedRelationProvenance string

const (
	// TypedRelationProvenanceLLMEvidence marks rows derived from LLM
	// emit_evidence. These carry the LLM's authored rationale and
	// flow through the EvidenceClosure ledger.
	TypedRelationProvenanceLLMEvidence TypedRelationProvenance = "llm_evidence"

	// TypedRelationProvenanceTypedGraph marks rows synthesised at
	// prompt-build time from typed graph relation traversal. These do
	// NOT flow through EvidenceClosure (the ledger stays clean as a
	// pure "what the LLM observed" record); they are recomputed each
	// dispatch from the typed graph, so persistence is unnecessary.
	TypedRelationProvenanceTypedGraph TypedRelationProvenance = "typed_graph"

	// TypedRelationProvenanceTypedObservation marks rows synthesised from the
	// accepted ObservationLedger, such as an exact log/diff/command/MCP row
	// linked to a current-source anchor. These are prompt hints, not
	// current-source citations and not system-authored final answers.
	TypedRelationProvenanceTypedObservation TypedRelationProvenance = "typed_observation"

	// TypedRelationProvenanceTypedEvidence marks rows projected from accepted
	// structured EvidenceItems. These rows are not graph guesses; they preserve
	// model/tool-authored evidence semantics while giving relation consumers a
	// uniform carrier.
	TypedRelationProvenanceTypedEvidence TypedRelationProvenance = "typed_evidence"
)

// TypedRelationAnchorKind maps a relation tag to the AnchorKind that
// LLM emit_evidence rows would use when describing the same tuple,
// so render-time dedup can match (Subject, Object, AnchorKind)
// across the two provenance types.
//
// Adding a new Relation value requires adding the matching AnchorKind
// row here. The TestTypedRelationAnchorKindCoverage structural test
// (in internal/types/typed_relation_hint_test.go) enforces it.
func TypedRelationAnchorKind(relation TypedRelationKind) AnchorKind {
	switch relation {
	case TypedRelationImplements, TypedRelationExtends, TypedRelationScopedTo:
		return AnchorDefinition
	case TypedRelationOverrides, TypedRelationExports:
		return AnchorDefinition
	case TypedRelationCalledBy, TypedRelationReferences:
		return AnchorCall
	case TypedRelationRegisters:
		return AnchorAssignment
	case TypedRelationConfigures, TypedRelationRoutesTo:
		return AnchorStringLiteral
	case TypedRelationImports:
		return AnchorImport
	case TypedRelationSourceAnchor:
		return AnchorTextReference
	}
	return ""
}

// AllTypedRelations enumerates every Relation value the shared
// relation contract knows about. A value in this list is not
// automatically hard-gate eligible; precision policy and provider
// support decide that per relation family.
func AllTypedRelations() []TypedRelationKind {
	return []TypedRelationKind{
		TypedRelationImplements,
		TypedRelationExtends,
		TypedRelationCalledBy,
		TypedRelationScopedTo,
		TypedRelationRegisters,
		TypedRelationOverrides,
		TypedRelationReferences,
		TypedRelationImports,
		TypedRelationExports,
		TypedRelationConfigures,
		TypedRelationRoutesTo,
		TypedRelationSourceAnchor,
	}
}
