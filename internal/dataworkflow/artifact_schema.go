package dataworkflow

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func ProjectArtifactSchemasNewestFirst(artifacts []ArtifactProjectionSource) []ArtifactSchemaProjection {
	if len(artifacts) == 0 {
		return nil
	}
	var out []ArtifactSchemaProjection
	seen := map[string]bool{}
	var walk func(ArtifactProjectionSource)
	walk = func(artifact ArtifactProjectionSource) {
		aliases := ArtifactAliases(artifact)
		shape := artifactField(artifact, "json_shape")
		if len(aliases) > 0 || strings.TrimSpace(shape) != "" {
			key := ArtifactAccessKey(artifact)
			if key != "" && seen[key] {
				for _, child := range artifact.Children {
					walk(child)
				}
				return
			}
			if key != "" {
				seen[key] = true
			}
			out = append(out, ArtifactSchemaProjection{
				ID:                strings.TrimSpace(artifact.ID),
				Kind:              strings.TrimSpace(artifact.Kind),
				NodeClass:         ArtifactNodeClass(artifact),
				Aliases:           aliases,
				JSONShape:         strings.TrimSpace(shape),
				Fields:            ArtifactFields(artifact),
				Diagnostics:       ArtifactDiagnostics(artifact),
				AccessHint:        ArtifactAccessHint(shape),
				SourcePaths:       cleanStrings(artifact.SourcePaths),
				SourceRecordPaths: cleanStrings(artifact.SourceRecordPaths),
				ReferencePaths:    cleanStrings(artifact.ReferencePaths),
				EvidencePaths:     cleanStrings(artifact.EvidencePaths),
				RowCount:          artifact.RowCount,
			})
		}
		for _, child := range artifact.Children {
			walk(child)
		}
	}
	for _, artifact := range artifacts {
		walk(artifact)
	}
	if len(out) == 0 {
		return nil
	}
	return ArtifactSchemaProjectionsWithTransitiveLineage(out)
}

func ArtifactSchemaProjectionsWithTransitiveLineage(projections []ArtifactSchemaProjection) []ArtifactSchemaProjection {
	if len(projections) == 0 {
		return nil
	}
	out := append([]ArtifactSchemaProjection(nil), projections...)
	aliasIndex := map[string]int{}
	for i, projection := range out {
		for _, alias := range artifactSchemaAliases(projection) {
			for _, key := range artifactLineageKeys(alias) {
				if key != "" {
					aliasIndex[key] = i
				}
			}
		}
	}
	for pass := 0; pass < len(out); pass++ {
		changed := false
		for i := range out {
			refs := cleanStrings(append(append([]string{}, out[i].SourcePaths...), out[i].SourceRecordPaths...))
			refs = append(refs, out[i].ReferencePaths...)
			for _, ref := range refs {
				refIdx, ok := artifactProjectionIndexByAlias(aliasIndex, ref)
				if !ok || refIdx == i {
					continue
				}
				before := lineageSize(out[i])
				refProjection := out[refIdx]
				out[i].SourcePaths = mergeLineageRoots(out[i].SourcePaths, refProjection.SourcePaths, refProjection.SourceRecordPaths)
				out[i].SourceRecordPaths = mergeLineageRoots(out[i].SourceRecordPaths, refProjection.SourceRecordPaths)
				out[i].ReferencePaths = mergeLineageRoots(out[i].ReferencePaths, refProjection.ReferencePaths)
				if lineageSize(out[i]) != before {
					changed = true
				}
			}
		}
		if !changed {
			break
		}
	}
	return out
}

func ArtifactResolutionLineageCompatible(base, ledger ArtifactSchemaProjection) bool {
	sources, precise := ResolutionSourceCandidates(ledger)
	if len(sources) == 0 || !precise {
		return true
	}
	for _, source := range sources {
		if ArtifactLineageContains(base, source) {
			return true
		}
	}
	return false
}

func ResolutionSourceCandidates(ledger ArtifactSchemaProjection) ([]string, bool) {
	references := normalizedLineageSet(ledger.ReferencePaths)
	sourceRecords := nonReferenceLineageRoots(ledger.SourceRecordPaths, references)
	if len(sourceRecords) > 0 {
		return sourceRecords, true
	}
	sourcePaths := nonReferenceLineageRoots(ledger.SourcePaths, references)
	if len(sourcePaths) == 0 {
		return nil, true
	}
	return sourcePaths, true
}

func ArtifactLineageContains(projection ArtifactSchemaProjection, alias string) bool {
	wants := artifactLineageKeys(alias)
	if len(wants) == 0 {
		return false
	}
	candidates := append([]string{projection.ID}, projection.Aliases...)
	candidates = append(candidates, projection.SourceRecordPaths...)
	candidates = append(candidates, projection.ReferencePaths...)
	candidates = append(candidates, projection.EvidencePaths...)
	candidates = append(candidates, projection.SourcePaths...)
	for _, candidate := range candidates {
		for _, got := range artifactLineageKeys(candidate) {
			if got == "" {
				continue
			}
			for _, want := range wants {
				if got == want {
					return true
				}
			}
		}
	}
	return false
}

func ArtifactLineageSummary(alias string, projection ArtifactSchemaProjection) string {
	values := cleanStrings(append(append(append(append(append([]string{alias, projection.ID}, projection.Aliases...), projection.SourceRecordPaths...), projection.ReferencePaths...), projection.EvidencePaths...), projection.SourcePaths...))
	return strings.Join(clampStrings(values, 8), ", ")
}

func ArtifactSourceLineageSummary(projection ArtifactSchemaProjection) string {
	values, _ := ResolutionSourceCandidates(projection)
	if len(values) == 0 {
		values = cleanStrings(projection.SourceRecordPaths)
	}
	if len(values) == 0 {
		values = cleanStrings(projection.SourcePaths)
	}
	return strings.Join(clampStrings(values, 8), ", ")
}

func BuildArtifactGraphState(artifacts []ArtifactProjectionSource, limit int) ArtifactGraphState {
	projections := ProjectArtifactSchemasNewestFirst(artifacts)
	return ArtifactGraphStateFromProjections(projections, limit)
}

func ArtifactsNewestFirst(records []WorkflowRecord) []ArtifactProjectionSource {
	var artifacts []ArtifactProjectionSource
	for i := len(records) - 1; i >= 0; i-- {
		rec := records[i]
		if rec.Result == nil {
			continue
		}
		for j := len(rec.Result.Artifacts) - 1; j >= 0; j-- {
			artifacts = append(artifacts, rec.Result.Artifacts[j])
		}
	}
	return artifacts
}

func ArtifactGraphStateFromProjections(projections []ArtifactSchemaProjection, limit int) ArtifactGraphState {
	if len(projections) == 0 {
		return ArtifactGraphState{}
	}
	if limit < 0 {
		limit = 0
	}
	nodeLimit := len(projections)
	if limit > 0 && limit < nodeLimit {
		nodeLimit = limit
	}
	state := ArtifactGraphState{
		NodeCount: len(projections),
		Truncated: limit > 0 && len(projections) > limit,
		Nodes:     make([]ArtifactGraphNode, 0, nodeLimit),
	}
	relationLimit := relationLimitForArtifactGraph(nodeLimit, len(projections))
	relations := ArtifactRelationsFromProjections(projections, relationLimit+1)
	if relationLimit > 0 && len(relations) > relationLimit {
		state.RelationsTruncated = true
		relations = relations[:relationLimit]
	}
	state.Relations = relations
	state.RelationCount = len(relations)
	seenAlias := map[string]bool{}
	seenExecutable := map[string]bool{}
	for i := 0; i < nodeLimit; i++ {
		projection := projections[i]
		executable := ArtifactUsableForRecordAction(projection)
		primary := firstArtifactAlias(projection)
		node := ArtifactGraphNode{
			ID:                    strings.TrimSpace(projection.ID),
			Kind:                  strings.TrimSpace(projection.Kind),
			ProducerKind:          artifactProducerKind(projection.Kind),
			NodeClass:             strings.TrimSpace(projection.NodeClass),
			PrimaryAlias:          primary,
			Aliases:               cleanStrings(projection.Aliases),
			JSONShape:             strings.TrimSpace(projection.JSONShape),
			Fields:                cleanStrings(projection.Fields),
			Diagnostics:           copyDiagnostics(projection.Diagnostics),
			AccessHint:            strings.TrimSpace(projection.AccessHint),
			RowCount:              projection.RowCount,
			ExecutableRecordInput: executable,
			Lineage: ArtifactLineage{
				SourcePaths:       cleanStrings(projection.SourcePaths),
				SourceRecordPaths: cleanStrings(projection.SourceRecordPaths),
				ReferencePaths:    cleanStrings(projection.ReferencePaths),
				EvidencePaths:     cleanStrings(projection.EvidencePaths),
			},
		}
		state.Nodes = append(state.Nodes, node)
		for _, alias := range artifactSchemaAliases(projection) {
			alias = strings.TrimSpace(alias)
			if alias == "" || seenAlias[alias] {
				continue
			}
			seenAlias[alias] = true
			state.AliasIndex = append(state.AliasIndex, ArtifactAliasBinding{
				Alias:                 alias,
				NodeID:                strings.TrimSpace(projection.ID),
				NodeClass:             strings.TrimSpace(projection.NodeClass),
				ExecutableRecordInput: executable,
			})
			if executable && !seenExecutable[alias] {
				seenExecutable[alias] = true
				state.ExecutableRecordAliases = append(state.ExecutableRecordAliases, alias)
			}
		}
	}
	return state
}

func FlattenedArtifactCount(artifacts []ArtifactProjectionSource) int {
	seen := map[string]bool{}
	var count int
	var walk func(ArtifactProjectionSource)
	walk = func(artifact ArtifactProjectionSource) {
		key := ArtifactAccessKey(artifact)
		if key != "" && !seen[key] {
			seen[key] = true
			count++
		}
		for _, child := range artifact.Children {
			walk(child)
		}
	}
	for _, artifact := range artifacts {
		walk(artifact)
	}
	return count
}

func ArtifactAliases(artifact ArtifactProjectionSource) []string {
	var out []string
	if id := strings.TrimSpace(artifact.ID); id != "" {
		out = append(out, id)
		if !strings.HasSuffix(id, ".json") {
			out = append(out, id+".json")
		}
	}
	for _, alias := range strings.Split(artifactField(artifact, "artifact_aliases"), ",") {
		if alias = strings.TrimSpace(alias); alias != "" {
			out = append(out, alias)
		}
	}
	if path := strings.TrimSpace(artifactField(artifact, "artifact_path")); path != "" {
		out = append(out, path)
		if base := filepath.Base(path); base != "." && base != "" && base != path {
			out = append(out, base)
		}
	}
	return cleanStrings(out)
}

func ArtifactFields(artifact ArtifactProjectionSource) []string {
	seen := map[string]bool{}
	var out []string
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		out = append(out, value)
	}
	for _, header := range artifact.Headers {
		add(header)
	}
	for _, key := range []string{"output_headers", "headers", "fields"} {
		for _, field := range strings.Split(artifactField(artifact, key), ",") {
			add(field)
		}
	}
	return out
}

func ArtifactDiagnostics(artifact ArtifactProjectionSource) map[string]string {
	if len(artifact.Fields) == 0 {
		return nil
	}
	keys := make([]string, 0, len(artifact.Fields))
	for key, value := range artifact.Fields {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" || artifactDiagnosticSkipKey(key) {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	const maxDiagnostics = 18
	if len(keys) > maxDiagnostics {
		keys = keys[:maxDiagnostics]
	}
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		out[key] = clampArtifactDiagnostic(artifact.Fields[key], 640)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func ArtifactAccessHint(shape string) string {
	shape = strings.TrimSpace(strings.ToLower(shape))
	switch {
	case strings.HasPrefix(shape, "array"):
		return "read with json_records(alias) or iterate json_load(alias) as a list; do not call .get() on the top-level value"
	case strings.HasPrefix(shape, "object") && strings.Contains(shape, "records"):
		return "read with json_records(alias); this wrapper object contains a records array plus metadata"
	case strings.HasPrefix(shape, "object"):
		return "read with json_load(alias) for object fields, or json_records(alias) when treating object values/wrappers as records"
	case strings.HasPrefix(shape, "scalar"):
		return "read with json_load(alias) as a scalar value"
	default:
		return "use json_records(alias) for record-oriented access; inspect artifact json_shape before assuming dict/list shape"
	}
}

func ArtifactSchemaByAlias(projections []ArtifactSchemaProjection, alias string) (ArtifactSchemaProjection, bool) {
	target := normalizeAccessPath(alias)
	if target == "" {
		return ArtifactSchemaProjection{}, false
	}
	for _, projection := range projections {
		for _, candidate := range artifactSchemaAliases(projection) {
			if normalizeAccessPath(candidate) == target {
				return projection, true
			}
		}
	}
	return ArtifactSchemaProjection{}, false
}

func artifactProjectionIndexByAlias(index map[string]int, alias string) (int, bool) {
	for _, key := range artifactLineageKeys(alias) {
		if key == "" {
			continue
		}
		if idx, ok := index[key]; ok {
			return idx, true
		}
	}
	return 0, false
}

func MissingFieldsOnArtifactSchema(projections []ArtifactSchemaProjection, alias string, fields []string) []string {
	projection, ok := ArtifactSchemaByAlias(projections, alias)
	if !ok || len(projection.Fields) == 0 {
		return nil
	}
	known := artifactFieldSet(projection.Fields)
	var missing []string
	seen := map[string]bool{}
	for _, field := range cleanStrings(fields) {
		key := strings.ToLower(strings.TrimSpace(field))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		if known[key] == "" {
			missing = append(missing, field)
		}
	}
	return cleanStrings(missing)
}

func artifactSchemaAliases(projection ArtifactSchemaProjection) []string {
	aliases := append([]string(nil), projection.Aliases...)
	if strings.TrimSpace(projection.ID) != "" {
		aliases = append(aliases, projection.ID)
	}
	return cleanStrings(aliases)
}

func artifactFieldSet(fields []string) map[string]string {
	out := map[string]string{}
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		out[strings.ToLower(field)] = field
	}
	return out
}

const (
	ArtifactNodeClassArtifact        = "artifact"
	ArtifactNodeClassRecord          = "record"
	ArtifactNodeClassDiagnosticChild = "diagnostic_child"
	ArtifactNodeClassWorkflowLedger  = "workflow_ledger"
)

func ArtifactNodeClass(artifact ArtifactProjectionSource) string {
	if IsWorkflowLedgerArtifact(artifact.ID, artifact.Kind, artifact.Fields) {
		return ArtifactNodeClassWorkflowLedger
	}
	if IsDiagnosticChildArtifact(artifact.ID, artifact.Kind) {
		return ArtifactNodeClassDiagnosticChild
	}
	shape := strings.ToLower(strings.TrimSpace(artifactField(artifact, "json_shape")))
	if strings.HasPrefix(shape, "array") || (strings.HasPrefix(shape, "object") && strings.Contains(shape, "records")) || len(artifact.Headers) > 0 {
		return ArtifactNodeClassRecord
	}
	return ArtifactNodeClassArtifact
}

func ArtifactUsableForRecordAction(projection ArtifactSchemaProjection) bool {
	if len(projection.Fields) == 0 {
		return false
	}
	if projection.NodeClass == ArtifactNodeClassDiagnosticChild || projection.NodeClass == ArtifactNodeClassWorkflowLedger {
		return false
	}
	if artifactKindHasPrefix(projection.Kind,
		dataquery.DataActionMaterialInventory,
		dataquery.DataActionInspectMaterial,
		dataquery.DataActionDeriveRules,
		dataquery.DataActionNormalizeEntities,
		dataquery.DataActionReconcile,
		dataquery.DataActionAssembleAnswer,
	) {
		return false
	}
	if projection.NodeClass == ArtifactNodeClassRecord || artifactSchemaShapeLooksRecordLike(projection) {
		return true
	}
	return artifactKindHasPrefix(projection.Kind,
		dataquery.DataActionExtractRecords,
		dataquery.DataActionDeriveFields,
		dataquery.DataActionExtractFields,
		dataquery.DataActionGroupRecords,
		dataquery.DataActionExpandRecords,
		dataquery.DataActionFilterRecords,
		dataquery.DataActionQualifyRecords,
		dataquery.DataActionApplyResolutions,
		dataquery.DataActionEnrichRecords,
		dataquery.DataActionJoinRecords,
		dataquery.DataActionComputeContribs,
		dataquery.DataActionCustomTransform,
	)
}

func artifactSchemaShapeLooksRecordLike(projection ArtifactSchemaProjection) bool {
	shape := strings.ToLower(strings.TrimSpace(projection.JSONShape))
	return strings.Contains(shape, "array") || strings.Contains(shape, "records")
}

func artifactKindHasPrefix(kind string, wants ...dataquery.DataActionKind) bool {
	got := strings.ToLower(strings.TrimSpace(kind))
	for _, want := range wants {
		prefix := strings.ToLower(strings.TrimSpace(string(want)))
		if prefix == "" {
			continue
		}
		if got == prefix || strings.HasPrefix(got, prefix+"/") {
			return true
		}
	}
	return false
}

func IsDiagnosticChildArtifact(id, kind string) bool {
	id = strings.ToLower(strings.TrimSpace(id))
	kind = strings.ToLower(strings.TrimSpace(kind))
	for _, suffix := range []string{"#base", "#mapping", "#mapping_candidate_source", "#mapping_candidate_reference", "#entity_source", "#entity_reference", "#entity_resolutions", "#entity_resolution_source", "#entity_resolution_reference"} {
		if strings.HasSuffix(id, suffix) {
			return true
		}
	}
	for _, suffix := range []string{"/base", "/mapping", "/mapping_candidate_source", "/mapping_candidate_reference", "/entity_source", "/entity_reference", "/entity_resolution_source", "/entity_resolution_reference"} {
		if strings.HasSuffix(kind, suffix) {
			return true
		}
	}
	switch kind {
	case "entity_resolution_source", "entity_resolution_reference":
		return true
	}
	return false
}

func IsWorkflowLedgerArtifact(id, kind string, fields map[string]string) bool {
	id = strings.ToLower(strings.TrimSpace(id))
	kind = strings.ToLower(strings.TrimSpace(kind))
	if strings.HasPrefix(kind, "workflow_ledger/") {
		return true
	}
	switch id {
	case "workflow_entity_resolutions", "workflow_contributions", "workflow_rule_coverage", "workflow_decision_records":
		return true
	}
	return fields != nil && strings.EqualFold(strings.TrimSpace(fields["workflow_ledger"]), "true")
}

func ArtifactAccessKey(artifact ArtifactProjectionSource) string {
	for _, alias := range strings.Split(artifactField(artifact, "artifact_aliases"), ",") {
		if key := normalizeAccessPath(alias); key != "" {
			return key
		}
	}
	if key := normalizeAccessPath(artifactField(artifact, "artifact_path")); key != "" {
		return key
	}
	if id := strings.TrimSpace(artifact.ID); id != "" {
		if key := normalizeAccessPath(id); key != "" {
			return key
		}
	}
	kind := strings.TrimSpace(artifact.Kind)
	if kind != "" || len(artifact.SourcePaths) > 0 {
		return "kind:" + kind + "\x00" + strings.Join(cleanStrings(artifact.SourcePaths), "\x00")
	}
	return ""
}

func artifactField(artifact ArtifactProjectionSource, key string) string {
	if artifact.Fields == nil {
		return ""
	}
	return strings.TrimSpace(artifact.Fields[key])
}

func cleanStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, value := range in {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func mergeLineageRoots(base []string, groups ...[]string) []string {
	out := append([]string(nil), base...)
	seen := normalizedLineageSet(out)
	for _, group := range groups {
		for _, value := range group {
			root := normalizeLineageRoot(value)
			if root == "" || seen[root] {
				continue
			}
			seen[root] = true
			out = append(out, root)
		}
	}
	return cleanStrings(out)
}

func nonReferenceLineageRoots(values []string, references map[string]bool) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		root := normalizeLineageRoot(value)
		if root == "" || references[root] || seen[root] {
			continue
		}
		seen[root] = true
		out = append(out, root)
	}
	return out
}

func normalizedLineageSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		for _, key := range artifactLineageKeys(value) {
			if key != "" {
				out[key] = true
			}
		}
	}
	return out
}

func artifactLineageKeys(value string) []string {
	exact := normalizeAccessPath(value)
	root := normalizeLineageRoot(value)
	return cleanStrings([]string{exact, root})
}

func normalizeLineageRoot(value string) string {
	value = normalizeAccessPath(value)
	if value == "" {
		return ""
	}
	idx := strings.LastIndex(value, ":")
	if idx <= 0 || idx == len(value)-1 {
		return value
	}
	for _, r := range value[idx+1:] {
		if r < '0' || r > '9' {
			return value
		}
	}
	return value[:idx]
}

func lineageSize(projection ArtifactSchemaProjection) int {
	return len(projection.SourcePaths) + len(projection.SourceRecordPaths) + len(projection.ReferencePaths)
}

func firstArtifactAlias(projection ArtifactSchemaProjection) string {
	for _, alias := range artifactSchemaAliases(projection) {
		if alias = strings.TrimSpace(alias); alias != "" {
			return alias
		}
	}
	return ""
}

func artifactProducerKind(kind string) string {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return ""
	}
	if idx := strings.Index(kind, "/"); idx > 0 {
		return kind[:idx]
	}
	return kind
}

func copyDiagnostics(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if value := strings.TrimSpace(in[key]); key != "" && value != "" {
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func artifactDiagnosticSkipKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "artifact_aliases", "artifact_path", "json_shape", "headers", "output_headers", "fields":
		return true
	default:
		return false
	}
}

func clampArtifactDiagnostic(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	const suffix = "..."
	if limit <= len(suffix) {
		return value[:limit]
	}
	return value[:limit-len(suffix)] + suffix
}

func normalizeAccessPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = filepath.Clean(value)
	if value == "." {
		return ""
	}
	return strings.ToLower(value)
}
