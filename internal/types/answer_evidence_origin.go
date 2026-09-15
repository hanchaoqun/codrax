package types

import "strings"

// AnswerAggregateSourceContextFromBusContext gathers the complete accepted
// source proof pool for claim admission, not the curated observation display.
// In particular, this must not add direct observations to the general ledger
// merely because an emitted source row is available for member qualification.
func AnswerAggregateSourceContextFromBusContext(ctx *BusContext) ObservationLedgerInput {
	source := ObservationLedgerInputFromBusContext(ctx, 0)
	if ctx != nil && ctx.Mutable != nil {
		source.EvidenceItems = appendObservationLedgerEvidence(source.EvidenceItems, 0, ctx.Mutable.EmittedEvidence()...)
	}
	return source
}

func AnswerAggregateSourceContextFromAgentContext(ctx *AgentContext) ObservationLedgerInput {
	return ObservationLedgerInputFromAgentContext(ctx, 0)
}

// AnswerAggregateFactEvidenceOrigins projects a model-authored aggregate fact
// onto the unified evidence-origin enum. The projection is compatibility glue:
// it consumes structured dimensions and narrow tool-provenance tokens, not
// user prose or model answer text. Later batches should replace provenance
// string fallbacks with first-class tool-emitted origin fields.
func AnswerAggregateFactEvidenceOrigins(fact AnswerAggregateFact, rm *RequestModel) []AnswerEvidenceOrigin {
	seen := map[AnswerEvidenceOrigin]bool{}
	var out []AnswerEvidenceOrigin
	// CSP #63 (2026-07-05): the typed explicit-user-exclusion boundary
	// ("只分析 trace，不分析代码" — precise enum + verbatim quotes) makes the
	// current-source evidence lane semantically impossible for this run.
	// Model-authored aggregate facts must therefore never be projected onto
	// AnswerEvidenceOriginCurrentSource here — neither by the terminal
	// kind-shaped fallback below (the donghu specimen: 10 trace-derived
	// facts stamped current_source set authority CurrentSourceSatisfied in
	// a 不分析代码 run and vetoed runtime citation cleanup) nor by a
	// model-emitted dimension token (the user boundary outranks model
	// claims). This is a single chokepoint inside add(); every current-source
	// add site funnels through it.
	excludesCurrentSource := rm != nil && rm.ExternalObservationPolicy.ExcludesCurrentSource()
	add := func(origin AnswerEvidenceOrigin) {
		if origin == AnswerEvidenceOriginUnknown || !origin.IsValid() || seen[origin] {
			return
		}
		if excludesCurrentSource && origin == AnswerEvidenceOriginCurrentSource {
			return
		}
		seen[origin] = true
		out = append(out, origin)
	}

	if fact.Kind == AnswerAggregateNegativeSearch {
		add(AnswerEvidenceOriginRepoNegativeSearch)
	}
	explicitOrigins := answerAggregateFactExplicitEvidenceOrigins(fact)
	for _, origin := range explicitOrigins {
		add(origin)
	}

	if rm != nil {
		hasExactCurrentSourceSupport := answerAggregateFactHasExactCurrentSourceSupportRef(fact)
		if rm.CurrentSourceLaneDecision().RequiresCurrentSource() && hasExactCurrentSourceSupport {
			add(AnswerEvidenceOriginCurrentSource)
		}
		// A history-shaped request is only a fallback origin for an otherwise
		// untyped aggregate. It must not stamp vcs_metadata onto a fact that is
		// already bound to an exact current-checkout source line or to another
		// explicit origin. Historical and current-source facts may still form a
		// legitimate dual-origin carrier, but only when the producer explicitly
		// supplies the VCS origin instead of inheriting it from request shape.
		if rm.Predicates.IsHistoryLookup &&
			aggregateFactKindCanCarryVCSMetadata(fact.Kind) &&
			len(explicitOrigins) == 0 &&
			!hasExactCurrentSourceSupport {
			add(AnswerEvidenceOriginVCSMetadata)
		}
		if rm.Predicates.IsCountQuestion && aggregateFactKindCanCarryCommandMeasurement(fact.Kind) {
			add(AnswerEvidenceOriginCommandMeasurement)
		}
		// The typed exclude boundary is itself an external-observation
		// carrier: an ExcludesCurrentSource run is an external-observation
		// run by construction, so its aggregate facts restate runtime
		// artifact material even when the compiled RequestModel copy lacks
		// the LogTriage/PerfTrace bundles (the donghu ledger input carried
		// the analyzer RM without the Mutable perf bundle, so
		// HasExternalOnlyRuntimeArtifact alone missed the lane and the facts
		// fell through to the current-source fallback).
		if (rm.HasExternalOnlyRuntimeArtifact() || rm.HasRuntimeArtifactPathReference() ||
			excludesCurrentSource) &&
			aggregateFactKindCanCarryRuntimeArtifact(fact.Kind) {
			add(AnswerEvidenceOriginRuntimeArtifact)
		}
		if len(out) == 0 {
			add(inferAggregateFactExternalOriginFromRequest(fact, rm))
		}
	}
	// CSP-RM (§29.21 ruling, 2026-07-10): a model-authored aggregate fact with
	// NO origin evidence is a pure model claim, and pure model claims must not
	// enter the current-source proof lane that feeds CurrentSourceSatisfied —
	// every satisfied consumer is a hard-gate face (keep-arm short circuit,
	// completion gates, authority views), and model facts are the noisiest
	// possible signal (double witness: donghu 20260703 satisfied=true + 4 blob
	// pseudo-citations; cmp_792 three model facts → satisfied=true → all three
	// retry-suppression arms dead → self-contradicting retry directive).
	// The terminal fallback therefore mints the ADVISORY lane instead:
	// AnswerEvidenceOriginSystemInference (existing enum value — grounding
	// Soft/DisplayOnly, authority ceiling Illustrative, excluded from both the
	// current-source proof count and the external-observation sufficiency
	// candidates). The record is retained losslessly and keeps feeding
	// display/soft guidance; CurrentSourceSatisfied now only comes from
	// deterministic tool witnesses (read_file coverage / grep / negative
	// search / trace_query typed observations). Reusing the existing internal
	// enum value means no LLM-facing schema change and no R2' six-spot sync
	// (precedent: CSR #64 ruling 2 requalified Kind at the compile throat the
	// same way — internal record classification, not a model-emitted field).
	if len(out) == 0 && aggregateFactKindUsuallyCurrentSource(fact.Kind) {
		add(AnswerEvidenceOriginSystemInference)
	}
	return out
}

func answerAggregateFactExplicitEvidenceOrigins(fact AnswerAggregateFact) []AnswerEvidenceOrigin {
	seen := map[AnswerEvidenceOrigin]bool{}
	var out []AnswerEvidenceOrigin
	add := func(origin AnswerEvidenceOrigin) {
		if origin == AnswerEvidenceOriginUnknown || !origin.IsValid() || seen[origin] {
			return
		}
		seen[origin] = true
		out = append(out, origin)
	}
	dims := aggregateDimensionMap(fact.Dimensions)
	for _, key := range []string{
		"origin", "evidence_origin", "secondary_origin", "diff_origin", "proof_source", "tool", "source", "source_ref", "tool_result", "producer", "measurement_kind", "measurement_origin",
	} {
		answerEvidenceOriginFromStructuredToken(dims[key], add)
	}
	answerEvidenceOriginFromProvenance(fact.Provenance, add)
	for _, ref := range fact.SupportRefs {
		answerEvidenceOriginFromSupportRef(ref, add)
	}
	return out
}

func AnswerAggregateFactPrimaryEvidenceOrigin(fact AnswerAggregateFact, rm *RequestModel) AnswerEvidenceOrigin {
	origins := AnswerAggregateFactEvidenceOrigins(fact, rm)
	if len(origins) == 0 {
		return AnswerEvidenceOriginUnknown
	}
	return origins[0]
}

// AnswerAggregateFactAuthorizesPrincipalContract reports whether an aggregate
// fact has typed authority beyond a retained model inference. A
// system_inference fact remains useful advisory context, but its Soft /
// Illustrative claim binding cannot drive MUST-render contracts, hard answer
// gates, hallucination exemptions, or system-authored principal blocks.
//
// The two system provenance markers below are attached only after deterministic
// structured matching; accepting them here preserves exact typed-relation and
// source-inventory row contracts without deriving authority from prose.
func AnswerAggregateFactAuthorizesPrincipalContract(fact AnswerAggregateFact, rm *RequestModel) bool {
	return answerAggregateFactAuthorizesPrincipalContract(fact, rm, nil)
}

// AnswerAggregateFactAuthorizesPrincipalContractWithSourceContext is the
// production admission path. A source coordinate witnesses the cited source,
// not an arbitrary member written beside it. The context-free API above is
// retained for callers classifying a fact before observations are available;
// it must not authorize a rendered principal contract in the live pipeline.
func AnswerAggregateFactAuthorizesPrincipalContractWithSourceContext(fact AnswerAggregateFact, rm *RequestModel, source ObservationLedgerInput) bool {
	context := compileAggregateSourceClaimContext(source)
	return answerAggregateFactAuthorizesPrincipalContract(fact, rm, context)
}

func answerAggregateFactAuthorizesPrincipalContract(fact AnswerAggregateFact, rm *RequestModel, source *aggregateSourceClaimContext) bool {
	if aggregateSourceMemberSystemAuthority(fact) {
		return true
	}
	// In a relation/call-chain request, individually true nodes and locations
	// do not prove set membership, direction, order, or a bridge. Only the
	// completion tool's exact typed-relation marker above can authorize the
	// principal relation contract.
	if rm != nil && PrincipalMemberSetRequiresTypedRelationAuthority(*rm) {
		return false
	}
	if AnswerAggregateFactRequiresWorkflowMembershipEvidence(fact, rm) {
		return false
	}
	if source != nil && !answerAggregateFactSourceMembersObserved(fact, source) {
		// Explicit external support owns its separate principal lane; it does
		// not turn an unrelated source coordinate into independent source proof.
		for _, origin := range answerAggregateFactExplicitEvidenceOrigins(fact) {
			if AnswerEvidenceOriginCarriesOriginSpecificSupport(origin) {
				return true
			}
		}
		return false
	}
	// Exact file:line support is itself the precise current-source witness. Some
	// pre-emit compatibility callers do not retain AnalysisIR, so requiring a
	// request model merely to recognize that coordinate would make authority
	// depend on orchestration plumbing rather than on the evidence. An explicit
	// external-only boundary still wins and keeps current-source refs out.
	if answerAggregateFactHasExactCurrentSourceSupportRef(fact) {
		if rm != nil && rm.ExternalObservationPolicy.ExcludesCurrentSource() {
			return false
		}
		return true
	}
	// Request shape may classify an otherwise untyped model aggregate into an
	// evidence lane for display (for example, a trace attachment makes the
	// aggregate runtime-shaped). That classification does not prove the
	// aggregate's value, membership, order, or relation. Principal contracts
	// therefore consume only origins carried explicitly by the fact; inferred
	// origins remain useful advisory metadata through
	// AnswerAggregateFactEvidenceOrigins.
	for _, origin := range answerAggregateFactExplicitEvidenceOrigins(fact) {
		switch origin {
		case AnswerEvidenceOriginUnknown, AnswerEvidenceOriginSystemInference:
			continue
		case AnswerEvidenceOriginCurrentSource:
			if answerAggregateFactHasExactCurrentSourceSupportRef(fact) {
				return true
			}
			continue
		default:
			return true
		}
	}
	return false
}

func aggregateSourceMemberSystemAuthority(fact AnswerAggregateFact) bool {
	return AnswerAggregateFactHasTypedRelationPrincipalAuthority(fact) ||
		strings.Contains(fact.Provenance, SourceInventoryPrincipalRowSetAggregateProvenance)
}

// AnswerAggregateFactAuthorizesSourceMemberCarrier keeps principal external
// support separate from permission to publish a source-member carrier.
func AnswerAggregateFactAuthorizesSourceMemberCarrier(fact AnswerAggregateFact, rm *RequestModel, source ObservationLedgerInput) bool {
	context := compileAggregateSourceClaimContext(source)
	if !answerAggregateFactAuthorizesPrincipalContract(fact, rm, context) {
		return false
	}
	return aggregateSourceMemberSystemAuthority(fact) || AnswerEvidenceOriginsAreOriginSpecificOnly(AnswerAggregateFactEvidenceOrigins(fact, rm)) ||
		!answerAggregateFactHasExactCurrentSourceSupportRef(fact) ||
		answerAggregateFactSourceMembersObserved(fact, context)
}

// answerAggregateFactSourceMembersObserved checks exact typed member identity,
// never summaries, source snippets, numeric shapes, or decorated prose. A
// declaration/operation endpoint proves that member exists at that coordinate;
// model member notes remain model-owned even when membership is admitted.
type aggregateSourceClaimContext struct {
	coordinates currentSourceSupportWitnessIndex
	items       []EvidenceItem
	inventory   []aggregateSourceInventoryMemberWitness
}

type aggregateSourceInventoryMemberWitness struct {
	name string
	path string
	line int
}

func compileAggregateSourceClaimContext(source ObservationLedgerInput) *aggregateSourceClaimContext {
	if source.aggregateSourceClaims != nil {
		return source.aggregateSourceClaims
	}
	out := &aggregateSourceClaimContext{coordinates: compileCurrentSourceSupportWitnessIndex(source.EvidenceItems, source.ToolResults)}
	for _, item := range source.EvidenceItems {
		if !currentSourceSupportGroundingAccepted(item.GroundingStatus) || EvidenceIsDerivationCandidate(item) ||
			evidenceItemObservationOrigin(item) != AnswerEvidenceOriginCurrentSource || item.Source == "" || item.LineStart <= 0 {
			continue
		}
		// Retain only scalar typed fields read by the matcher. No raw text,
		// mutable tool carriers, or model-authored explanations are cached.
		out.items = append(out.items, EvidenceItem{Source: item.Source, LineStart: item.LineStart, LineEnd: item.LineEnd,
			Subject: item.Subject, AnchorSymbol: item.AnchorSymbol, Object: item.Object,
			AnchorKind: item.AnchorKind, Kind: item.Kind, Scope: item.Scope, DiagramRole: item.DiagramRole,
			Producer: item.Producer, Predicate: item.Predicate})
	}
	// The native inventory is an independent typed member witness, not a
	// synthesized EvidenceItem or a model provenance token. Attributes, notes,
	// unobserved candidates and ambiguous coverage cannot declare members here.
	appendInventory := func(inventory SourceInventoryObservation) {
		if !inventory.Active {
			return
		}
		for _, set := range inventory.Sets {
			for _, member := range set.Members {
				name, path := strings.TrimSpace(member.Name), strings.TrimSpace(member.File)
				if member.CoverageState != SourceInventoryCoverageObserved || name == "" || path == "" || member.Line <= 0 {
					continue
				}
				out.inventory = append(out.inventory, aggregateSourceInventoryMemberWitness{name: name, path: path, line: member.Line})
				out.coordinates.append(currentSourceSupportWitness{path: path, lineStart: member.Line, lineEnd: member.Line,
					status: sourceInventoryObservationGrounding(member.CoverageState)})
			}
		}
	}
	appendInventory(source.SourceInventoryObservation)
	for _, result := range source.ToolResults {
		if result.Success && result.SourceInventory != nil {
			appendInventory(*result.SourceInventory)
		}
	}
	return out
}

// AnswerSourceSymbolDefinitionObserved is the narrow admission path for
// materializing a source symbol. It requires the actual output name and line
// to match a current-source definition or an observed native inventory row.
// Unlike aggregate principal authority, no external origin or system marker
// can substitute for that identity witness. Native inventory anchors retain
// non-identifier names too; this does not infer symbol kind or completeness.
func AnswerSourceSymbolDefinitionObserved(name, file string, line int, source ObservationLedgerInput) bool {
	if source.RequestModel != nil && source.RequestModel.ExternalObservationPolicy.ExcludesCurrentSource() {
		return false
	}
	name, file = strings.TrimSpace(name), strings.TrimSpace(file)
	if name == "" || file == "" || line <= 0 {
		return false
	}
	context := compileAggregateSourceClaimContext(source)
	coordinate, ok := context.coordinates.bindLocation(AnswerSourceLocationSurface{File: file, LineStart: line})
	if !ok {
		return false
	}
	path := normalizeAnswerLocationFile(coordinate.path)
	for _, item := range context.items {
		if item.LineStart == coordinate.lineStart && normalizeAnswerLocationFile(item.Source) == path &&
			ClaimFormOf(item) == ClaimDefinitionFact && aggregateSourceClaimNamesMember(item, name) {
			return true
		}
	}
	for _, member := range context.inventory {
		if member.name == name && member.line == coordinate.lineStart && normalizeAnswerLocationFile(member.path) == path {
			return true
		}
	}
	return false
}

// AnswerAggregateFactHasObservedSourceMembers qualifies only source member
// identity and coordinates for automatic source-citation candidates. It is not
// principal, relation, workflow, or completeness authority; external origins
// and model/system provenance tokens cannot stand in for observed members.
func AnswerAggregateFactHasObservedSourceMembers(fact AnswerAggregateFact, source ObservationLedgerInput) bool {
	if source.RequestModel != nil && source.RequestModel.ExternalObservationPolicy.ExcludesCurrentSource() {
		return false
	}
	return fact.Kind == AnswerAggregateMemberSet && len(fact.Members) > 0 &&
		answerAggregateFactSourceMembersObserved(fact, compileAggregateSourceClaimContext(source))
}

func answerAggregateFactSourceMembersObserved(fact AnswerAggregateFact, source *aggregateSourceClaimContext) bool {
	if fact.Kind != AnswerAggregateMemberSet {
		return true
	}
	if len(fact.Members) == 0 {
		return false
	}
	index := source.coordinates
	if answerAggregateFactHasExactCurrentSourceSupportRef(fact) {
		if _, ok := index.bindFact(fact); !ok {
			return false
		}
	}
	for memberIndex, member := range fact.Members {
		member = strings.TrimSpace(member)
		path, line, _ := aggregateMemberStructuredLocation(fact, memberIndex, member)
		var coordinate currentSourceSupportBinding
		if path != "" && line > 0 {
			var ok bool
			coordinate, ok = index.bindLocation(AnswerSourceLocationSurface{File: path, LineStart: line})
			if !ok {
				return false
			}
		}
		if location, ok := ParseAnswerSourceLocationSurface(member); ok {
			if _, witnessed := index.bindLocation(location); !witnessed {
				return false
			}
			continue
		}
		if file, ok := ParseAnswerFilePathSurface(member); ok {
			if coordinate.path == "" || normalizeAnswerLocationFile(file) != normalizeAnswerLocationFile(coordinate.path) {
				return false
			}
			continue
		}
		if label, _, ok := ParseAnswerSupportRefMemberLocation(member); ok && strings.TrimSpace(label) != "" {
			member = strings.TrimSpace(label)
		}
		matched := map[string]bool{}
		for _, item := range source.items {
			if !aggregateSourceClaimNamesMember(item, member) {
				continue
			}
			itemEnd := item.LineEnd
			if itemEnd < item.LineStart {
				itemEnd = item.LineStart
			}
			if coordinate.path != "" && (normalizeAnswerLocationFile(coordinate.path) != normalizeAnswerLocationFile(item.Source) ||
				coordinate.lineStart < item.LineStart || coordinate.lineStart > itemEnd) {
				continue
			}
			matched[aggregateSupportLocationKeyForDisplay(item.Source, item.LineStart)] = true
		}
		for _, item := range source.inventory {
			if member != item.name || coordinate.path != "" &&
				(normalizeAnswerLocationFile(coordinate.path) != normalizeAnswerLocationFile(item.path) || coordinate.lineStart != item.line) {
				continue
			}
			matched[aggregateSupportLocationKeyForDisplay(item.path, item.line)] = true
		}
		if len(matched) != 1 {
			return false
		}
	}
	return true
}

func aggregateSourceClaimNamesMember(item EvidenceItem, member string) bool {
	subject, anchor, object := strings.TrimSpace(item.Subject), strings.TrimSpace(item.AnchorSymbol), strings.TrimSpace(item.Object)
	switch ClaimFormOf(item) {
	case ClaimDefinitionFact:
		// The definition anchor owns identity. A model's unrelated Subject or
		// Object beside a valid declaration must not become another member.
		if anchor == "" {
			return member == subject
		}
		if member == anchor {
			return true
		}
		return member == subject && (strings.HasSuffix(subject, "."+anchor) || strings.HasSuffix(subject, "::"+anchor))
	case ClaimCallEdge, ClaimCallbackHandoff, ClaimArgumentFlow, ClaimImportEdge,
		ClaimAssignmentFact, ClaimReturnFact, ClaimLiteralValueFact:
		return member == anchor && anchor != "" || member == subject && subject != "" || member == object && object != ""
	default:
		// In particular, text-reference/precedence/guard/branch prose can
		// document an operation without declaring its labels as source members.
		return false
	}
}

// AnswerAggregateFactRequiresWorkflowMembershipEvidence distinguishes a cited
// declaration from proof that it belongs to the requested workflow. It does
// not reject or rewrite the retained model fact: its locations and notes remain
// usable support. Only promotion into mandatory membership is withheld.
// Independent typed relation/inventory proofs and explicit non-source evidence
// lanes retain their existing authority; request-inferred origins do not count.
func AnswerAggregateFactRequiresWorkflowMembershipEvidence(fact AnswerAggregateFact, rm *RequestModel) bool {
	if fact.Kind != AnswerAggregateMemberSet || rm == nil ||
		!SourceInventoryLaneConflictsWithConceptualWorkflowDimension(*rm) {
		return false
	}
	if AnswerAggregateFactHasTypedRelationPrincipalAuthority(fact) ||
		strings.Contains(fact.Provenance, SourceInventoryPrincipalRowSetAggregateProvenance) {
		return false
	}
	for _, origin := range answerAggregateFactExplicitEvidenceOrigins(fact) {
		if AnswerEvidenceOriginCarriesOriginSpecificSupport(origin) {
			return false
		}
	}
	return true
}

func answerAggregateFactHasExactCurrentSourceSupportRef(fact AnswerAggregateFact) bool {
	for _, ref := range fact.SupportRefs {
		if answerSupportRefHasSourceLine(ref) {
			return true
		}
	}
	return false
}

// AnswerEvidenceOriginCarriesOriginSpecificSupport reports whether an origin is
// a first-class non-current-source observation lane. These origins may support
// user-visible claims, but they must not be converted into current-checkout
// file:line citation pressure.
func AnswerEvidenceOriginCarriesOriginSpecificSupport(origin AnswerEvidenceOrigin) bool {
	switch origin {
	case AnswerEvidenceOriginVCSMetadata,
		AnswerEvidenceOriginVCSDiff,
		AnswerEvidenceOriginRuntimeArtifact,
		AnswerEvidenceOriginCommandMeasurement,
		AnswerEvidenceOriginRepoNegativeSearch,
		AnswerEvidenceOriginCrossRepoIndex,
		AnswerEvidenceOriginExternalDocument,
		AnswerEvidenceOriginWebPage,
		AnswerEvidenceOriginMCPResource,
		AnswerEvidenceOriginConnectorResource:
		return true
	default:
		return false
	}
}

// AnswerEvidenceOriginsAreOriginSpecificOnly reports whether a set of typed
// evidence origins is entirely non-current-source origin-specific support. This
// is the shared guard for suppressing current-source citation fallbacks without
// every consumer re-implementing origin switches.
func AnswerEvidenceOriginsAreOriginSpecificOnly(origins []AnswerEvidenceOrigin) bool {
	hasOriginSpecific := false
	for _, origin := range origins {
		if origin == AnswerEvidenceOriginUnknown {
			continue
		}
		if !AnswerEvidenceOriginCarriesOriginSpecificSupport(origin) {
			return false
		}
		hasOriginSpecific = true
	}
	return hasOriginSpecific
}

// AnswerEvidenceOriginFromStructuredToken converts a typed producer/origin
// token into an evidence origin. It is the public, single-origin companion to
// the aggregate-fact projection helpers above. Callers should use this for
// tool parameter compatibility and prompt/schema repair; it must not be fed
// raw user prose to infer intent.
func AnswerEvidenceOriginFromStructuredToken(raw string) AnswerEvidenceOrigin {
	var out []AnswerEvidenceOrigin
	answerEvidenceOriginFromStructuredToken(raw, func(origin AnswerEvidenceOrigin) {
		if origin == AnswerEvidenceOriginUnknown || !origin.IsValid() || len(out) > 0 {
			return
		}
		out = append(out, origin)
	})
	if len(out) == 0 {
		return AnswerEvidenceOriginUnknown
	}
	return out[0]
}

func answerEvidenceOriginFromStructuredToken(raw string, add func(AnswerEvidenceOrigin)) {
	token := strings.ToLower(strings.TrimSpace(raw))
	if idx := strings.Index(token, "["); idx > 0 && strings.HasSuffix(token, "]") {
		token = strings.TrimSpace(token[:idx])
	}
	candidates := []string{token}
	for _, sep := range []string{" ", ".", ":", "/", "#"} {
		if idx := strings.Index(token, sep); idx > 0 {
			prefix := strings.TrimSpace(token[:idx])
			if prefix != "" && prefix != token && answerEvidenceOriginAllowsQualifiedPrefix(prefix) {
				candidates = append(candidates, prefix)
			}
		}
	}
	for _, token := range candidates {
		switch token {
		case "", "model_emitted":
			continue
		case "current_source", "current_repo", "repo_source", "source_file", "file_line":
			add(AnswerEvidenceOriginCurrentSource)
		case "vcs_metadata", "git_metadata", "git_history", "git_history_search", "git_log", "git_show", "commit", "git_commit", "exec_command_git_history", "vcs_history_count":
			add(AnswerEvidenceOriginVCSMetadata)
		case "vcs_diff", "git_diff", "diff_hunk":
			add(AnswerEvidenceOriginVCSDiff)
		case "runtime_artifact", "artifact_frame", "log_bundle", "perf_trace", "emit_log_triage", "emit_perf_trace", "trace_query",
			"attached-log", "attached_log", "attached-trace", "attached_trace":
			add(AnswerEvidenceOriginRuntimeArtifact)
		case "command_measurement", "exec_command", "command_count", "line_count", "file_count":
			add(AnswerEvidenceOriginCommandMeasurement)
		case "repo_negative_search", "negative_search", "grep_negative":
			add(AnswerEvidenceOriginRepoNegativeSearch)
		case "cross_repo_index", "repo_map", "multi_repo_index":
			add(AnswerEvidenceOriginCrossRepoIndex)
		case "external_document", "external_doc", "document_resource", "external_resource":
			add(AnswerEvidenceOriginExternalDocument)
		case "web_page", "webpage", "web", "url", "http", "https":
			add(AnswerEvidenceOriginWebPage)
		case "mcp_resource", "mcp", "mcp_tool", "mcp_response":
			add(AnswerEvidenceOriginMCPResource)
		case "connector_resource", "connector", "app_connector", "app_resource":
			add(AnswerEvidenceOriginConnectorResource)
		case "system_inference", "system":
			add(AnswerEvidenceOriginSystemInference)
		}
	}
}

func answerEvidenceOriginAllowsQualifiedPrefix(prefix string) bool {
	switch strings.ToLower(strings.TrimSpace(prefix)) {
	case "vcs_metadata",
		"git_metadata",
		"git_history",
		"git_history_search",
		"git_log",
		"git_show",
		"git_diff",
		"vcs_diff",
		"runtime_artifact",
		"log_bundle",
		"perf_trace",
		"emit_log_triage",
		"emit_perf_trace",
		"trace_query",
		"command_measurement",
		"exec_command",
		"repo_negative_search",
		"negative_search",
		"cross_repo_index",
		"repo_map",
		"multi_repo_index",
		"external_document",
		"external_doc",
		"web_page",
		"webpage",
		"mcp_resource",
		"mcp",
		"mcp_tool",
		"connector_resource",
		"connector":
		return true
	default:
		return false
	}
}

func answerEvidenceOriginFromProvenance(raw string, add func(AnswerEvidenceOrigin)) {
	prov := strings.ToLower(strings.TrimSpace(raw))
	if prov == "" {
		return
	}
	for _, part := range strings.FieldsFunc(prov, func(r rune) bool {
		return r == ';' || r == ',' || r == '|' || r == '\n' || r == '\t'
	}) {
		token := strings.TrimSpace(part)
		if strings.HasPrefix(token, "command:") {
			add(AnswerEvidenceOriginCommandMeasurement)
			if strings.Contains(token, "git ") {
				add(AnswerEvidenceOriginVCSMetadata)
			}
			continue
		}
		answerEvidenceOriginFromStructuredToken(token, add)
	}
}

// AnswerEvidenceOriginFromSupportRef is the public, single-origin companion to
// the support-ref-shaped origin grammar below, mirroring how
// AnswerEvidenceOriginFromStructuredToken exposes the dimension-token grammar.
// It exists for consumers that hold a model-emitted support_refs entry (not a
// dimension token) and need the SAME ref grammar the aggregate-fact projection
// already applies to that field — including artifact-path recognition such as
// the reserved attached_trace.txt blob basename (SUPPREF-TOL §29.104.13: the
// h9 witness ref "attached_trace.txt: wakeup_chain path" names the attached
// runtime artifact but is invisible to the dimension-token grammar). The
// underlying parser body is shared and unchanged; this is an export, not a
// second grammar.
func AnswerEvidenceOriginFromSupportRef(raw string) AnswerEvidenceOrigin {
	var out []AnswerEvidenceOrigin
	answerEvidenceOriginFromSupportRef(raw, func(origin AnswerEvidenceOrigin) {
		if origin == AnswerEvidenceOriginUnknown || !origin.IsValid() || len(out) > 0 {
			return
		}
		out = append(out, origin)
	})
	if len(out) == 0 {
		return AnswerEvidenceOriginUnknown
	}
	return out[0]
}

func answerEvidenceOriginFromSupportRef(raw string, add func(AnswerEvidenceOrigin)) {
	ref := strings.ToLower(strings.TrimSpace(raw))
	if ref == "" {
		return
	}
	for _, part := range strings.FieldsFunc(ref, func(r rune) bool {
		return r == ';' || r == ',' || r == '|' || r == '\n' || r == '\t'
	}) {
		token := strings.TrimSpace(part)
		if token == "" {
			continue
		}
		if idx := strings.Index(token, ":"); idx > 0 {
			prefix := strings.TrimSpace(token[:idx])
			if AnswerEvidenceOriginFromStructuredToken(prefix) != AnswerEvidenceOriginUnknown {
				answerEvidenceOriginFromStructuredToken(prefix, add)
				after := strings.TrimSpace(token[idx+1:])
				if origin := answerEvidenceOriginFromExternalReference(after); origin != AnswerEvidenceOriginUnknown {
					add(origin)
				}
				continue
			}
		}
		answerEvidenceOriginFromStructuredToken(token, add)
		if origin := answerEvidenceOriginFromExternalReference(token); origin != AnswerEvidenceOriginUnknown {
			add(origin)
		}
	}
}

func inferAggregateFactExternalOriginFromRequest(fact AnswerAggregateFact, rm *RequestModel) AnswerEvidenceOrigin {
	if rm == nil || rm.CurrentSourceLaneDecision().RequiresCurrentSource() {
		return AnswerEvidenceOriginUnknown
	}
	if rm.HasExternalOnlyRuntimeArtifact() && aggregateFactKindCanCarryRuntimeArtifact(fact.Kind) {
		return AnswerEvidenceOriginRuntimeArtifact
	}
	if rm.ExternalObservationPolicy == nil || !rm.ExternalObservationPolicy.ArtifactCitationsExternalOnly() {
		return AnswerEvidenceOriginUnknown
	}
	for _, raw := range requestModelExternalOriginCandidates(rm) {
		if origin := answerEvidenceOriginFromExternalReference(raw); origin != AnswerEvidenceOriginUnknown {
			return origin
		}
	}
	return AnswerEvidenceOriginUnknown
}

func requestModelExternalOriginCandidates(rm *RequestModel) []string {
	if rm == nil {
		return nil
	}
	var out []string
	out = append(out, rm.AnalyzerHints.ExactTargets...)
	out = append(out, rm.AnalyzerHints.Entities...)
	out = append(out, rm.AnalyzerHints.PrimaryEntities...)
	if rm.CurrentSourceExplanationProfile != nil {
		out = append(out, rm.CurrentSourceExplanationProfile.SourceQuotes...)
	}
	if rm.ExternalObservationPolicy != nil {
		out = append(out, rm.ExternalObservationPolicy.SourceQuotes...)
	}
	if rm.RequestedAnswerDimensions != nil {
		for _, dim := range rm.RequestedAnswerDimensions.Dimensions {
			out = append(out, dim.SourceQuote, dim.Label)
		}
	}
	return out
}

func answerEvidenceOriginFromExternalReference(raw string) AnswerEvidenceOrigin {
	ref := strings.ToLower(strings.TrimSpace(raw))
	if ref == "" {
		return AnswerEvidenceOriginUnknown
	}
	switch {
	case strings.HasPrefix(ref, "mcp://"):
		return AnswerEvidenceOriginMCPResource
	case strings.HasPrefix(ref, "http://"), strings.HasPrefix(ref, "https://"):
		return AnswerEvidenceOriginWebPage
	case RuntimeArtifactPathKindInText(ref) != "":
		return AnswerEvidenceOriginRuntimeArtifact
	case strings.Contains(ref, "://"):
		return AnswerEvidenceOriginExternalDocument
	default:
		return AnswerEvidenceOriginUnknown
	}
}

func aggregateFactKindCanCarryVCSMetadata(kind AnswerAggregateKind) bool {
	switch kind {
	case AnswerAggregateTotalCount,
		AnswerAggregateUniqueCount,
		AnswerAggregateGroupedCount,
		AnswerAggregateBucketCount,
		AnswerAggregateScalar,
		AnswerAggregateMemberSet,
		AnswerAggregateNegativeObservation:
		return true
	default:
		return false
	}
}

func aggregateFactKindCanCarryCommandMeasurement(kind AnswerAggregateKind) bool {
	switch kind {
	case AnswerAggregateTotalCount,
		AnswerAggregateUniqueCount,
		AnswerAggregateGroupedCount,
		AnswerAggregateBucketCount,
		AnswerAggregateExcluded,
		AnswerAggregateScalar,
		AnswerAggregateMemberSet,
		AnswerAggregateNegativeObservation:
		return true
	default:
		return false
	}
}

func aggregateFactKindCanCarryRuntimeArtifact(kind AnswerAggregateKind) bool {
	switch kind {
	case AnswerAggregateScalar,
		AnswerAggregateTotalCount,
		AnswerAggregateUniqueCount,
		AnswerAggregateMemberSet,
		AnswerAggregateGroupedCount,
		AnswerAggregateBucketCount,
		AnswerAggregateNegativeObservation,
		AnswerAggregateBehaviorOutcome,
		AnswerAggregateErrorGranularity:
		return true
	default:
		return false
	}
}

func aggregateFactKindUsuallyCurrentSource(kind AnswerAggregateKind) bool {
	switch kind {
	case AnswerAggregateUnknown:
		return false
	case AnswerAggregateNegativeSearch,
		AnswerAggregateNegativeObservation:
		return false
	default:
		return true
	}
}
