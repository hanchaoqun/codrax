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
		"origin", "evidence_origin", "secondary_origin", "diff_origin", "proof_source", "tool", "source", "measurement_kind", "measurement_origin",
	} {
		answerEvidenceOriginFromStructuredToken(dims[key], add)
	}
	answerEvidenceOriginFromProvenance(fact.Provenance, add)
	return out
}

func AnswerAggregateFactPrimaryEvidenceOrigin(fact AnswerAggregateFact, rm *RequestModel) AnswerEvidenceOrigin {
	origins := AnswerAggregateFactEvidenceOrigins(fact, rm)
	if len(origins) == 0 {
		return AnswerEvidenceOriginUnknown
	}
	return origins[0]
}

func answerEvidenceOriginFromStructuredToken(raw string, add func(AnswerEvidenceOrigin)) {
	token := strings.ToLower(strings.TrimSpace(raw))
	switch token {
	case "", "model_emitted":
		return
	case "current_source", "current_repo", "repo_source", "source_file", "file_line":
		add(AnswerEvidenceOriginCurrentSource)
	case "vcs_metadata", "git_metadata", "git_history", "git_history_search", "git_log", "git_show", "exec_command_git_history", "vcs_history_count":
		add(AnswerEvidenceOriginVCSMetadata)
	case "vcs_diff", "git_diff", "diff_hunk":
		add(AnswerEvidenceOriginVCSDiff)
	case "runtime_artifact", "artifact_frame", "log_bundle", "perf_trace", "emit_log_triage", "emit_perf_trace":
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
		AnswerAggregateMemberSet,
		AnswerAggregateGroupedCount,
		AnswerAggregateBucketCount,
		AnswerAggregateNegativeObservation:
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
