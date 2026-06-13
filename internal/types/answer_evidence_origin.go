package types

import "strings"

// AnswerAggregateFactEvidenceOrigins projects a model-authored aggregate fact
// onto the unified evidence-origin enum. The projection is compatibility glue:
// it consumes structured dimensions and narrow tool-provenance tokens, not
// user prose or model answer text. Later batches should replace provenance
// string fallbacks with first-class tool-emitted origin fields.
func AnswerAggregateFactEvidenceOrigins(fact AnswerAggregateFact, rm *RequestModel) []AnswerEvidenceOrigin {
	seen := map[AnswerEvidenceOrigin]bool{}
	var out []AnswerEvidenceOrigin
	add := func(origin AnswerEvidenceOrigin) {
		if origin == AnswerEvidenceOriginUnknown || !origin.IsValid() || seen[origin] {
			return
		}
		seen[origin] = true
		out = append(out, origin)
	}

	if fact.Kind == AnswerAggregateNegativeSearch {
		add(AnswerEvidenceOriginRepoNegativeSearch)
	}
	for _, origin := range answerAggregateFactExplicitEvidenceOrigins(fact) {
		add(origin)
	}

	if rm != nil {
		if rm.Predicates.IsHistoryLookup && aggregateFactKindCanCarryVCSMetadata(fact.Kind) {
			add(AnswerEvidenceOriginVCSMetadata)
		}
		if rm.Predicates.IsCountQuestion && aggregateFactKindCanCarryCommandMeasurement(fact.Kind) {
			add(AnswerEvidenceOriginCommandMeasurement)
		}
		if rm.HasExternalOnlyRuntimeArtifact() && aggregateFactKindCanCarryRuntimeArtifact(fact.Kind) {
			add(AnswerEvidenceOriginRuntimeArtifact)
		}
		if len(out) == 0 {
			add(inferAggregateFactExternalOriginFromRequest(fact, rm))
		}
	}
	if len(out) == 0 && aggregateFactKindUsuallyCurrentSource(fact.Kind) {
		add(AnswerEvidenceOriginCurrentSource)
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
		case "runtime_artifact", "artifact_frame", "log_bundle", "perf_trace", "emit_log_triage", "emit_perf_trace", "trace_query":
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
	case LooksLikeRuntimeArtifactPath(ref):
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
