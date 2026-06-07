package dataworkflow

import (
	"path/filepath"
	"strings"
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
				ID:          strings.TrimSpace(artifact.ID),
				Kind:        strings.TrimSpace(artifact.Kind),
				NodeClass:   ArtifactNodeClass(artifact),
				Aliases:     aliases,
				JSONShape:   strings.TrimSpace(shape),
				Fields:      ArtifactFields(artifact),
				AccessHint:  ArtifactAccessHint(shape),
				SourcePaths: cleanStrings(artifact.SourcePaths),
				RowCount:    artifact.RowCount,
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
	return out
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

func IsDiagnosticChildArtifact(id, kind string) bool {
	id = strings.ToLower(strings.TrimSpace(id))
	kind = strings.ToLower(strings.TrimSpace(kind))
	for _, suffix := range []string{"#base", "#mapping", "#entity_source", "#entity_reference"} {
		if strings.HasSuffix(id, suffix) {
			return true
		}
	}
	for _, suffix := range []string{"/base", "/mapping", "/entity_source", "/entity_reference"} {
		if strings.HasSuffix(kind, suffix) {
			return true
		}
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
