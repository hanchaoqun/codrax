package dataworkflow

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type ArtifactAccessView struct {
	ID                string              `json:"id,omitempty"`
	Kind              string              `json:"kind,omitempty"`
	NodeClass         string              `json:"node_class,omitempty"`
	Aliases           []string            `json:"aliases,omitempty"`
	JSONShape         string              `json:"json_shape,omitempty"`
	Fields            []string            `json:"fields,omitempty"`
	FieldSamples      map[string][]string `json:"field_samples,omitempty"`
	AccessHint        string              `json:"access_hint,omitempty"`
	SourcePaths       []string            `json:"source_paths,omitempty"`
	SourceRecordPaths []string            `json:"source_record_paths,omitempty"`
	ReferencePaths    []string            `json:"reference_paths,omitempty"`
	EvidencePaths     []string            `json:"evidence_paths,omitempty"`
}

func BuildArtifactAccessViews(artifacts []ArtifactProjectionSource, limit int) []ArtifactAccessView {
	return BuildArtifactAccessViewsWithFieldSamples(artifacts, limit, 6, 2)
}

func BuildArtifactAccessViewsWithFieldSamples(artifacts []ArtifactProjectionSource, limit, maxSampleFields, maxSampleValues int) []ArtifactAccessView {
	if limit <= 0 || len(artifacts) == 0 {
		return nil
	}
	var out []ArtifactAccessView
	seen := map[string]bool{}
	var walk func(ArtifactProjectionSource)
	walk = func(artifact ArtifactProjectionSource) {
		if len(out) >= limit {
			return
		}
		aliases := ArtifactAliases(artifact)
		shape := artifactField(artifact, "json_shape")
		if len(aliases) > 0 || shape != "" {
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
			fields := ClampRecordViewStringSlice(ArtifactFields(artifact), 32)
			out = append(out, ArtifactAccessView{
				ID:                strings.TrimSpace(artifact.ID),
				Kind:              strings.TrimSpace(artifact.Kind),
				NodeClass:         ArtifactNodeClass(artifact),
				Aliases:           ClampRecordViewStringSlice(aliases, 8),
				JSONShape:         ClampRecordViewText(shape, 240),
				Fields:            fields,
				FieldSamples:      artifactAccessViewFieldSamples(artifact, fields, maxSampleFields, maxSampleValues),
				AccessHint:        ArtifactAccessHint(shape),
				SourcePaths:       ClampRecordViewStringSlice(cleanStrings(artifact.SourcePaths), 6),
				SourceRecordPaths: ClampRecordViewStringSlice(cleanStrings(artifact.SourceRecordPaths), 6),
				ReferencePaths:    ClampRecordViewStringSlice(cleanStrings(artifact.ReferencePaths), 6),
				EvidencePaths:     ClampRecordViewStringSlice(cleanStrings(artifact.EvidencePaths), 6),
			})
		}
		for _, child := range artifact.Children {
			walk(child)
		}
	}
	for _, artifact := range artifacts {
		walk(artifact)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func ArtifactAccessViewsFromProjections(projections []ArtifactSchemaProjection, limit int) []ArtifactAccessView {
	if limit <= 0 || len(projections) == 0 {
		return nil
	}
	out := make([]ArtifactAccessView, 0, min(limit, len(projections)))
	for _, projection := range projections {
		if len(out) >= limit {
			break
		}
		out = append(out, ArtifactAccessView{
			ID:                strings.TrimSpace(projection.ID),
			Kind:              strings.TrimSpace(projection.Kind),
			NodeClass:         strings.TrimSpace(projection.NodeClass),
			Aliases:           cleanStrings(projection.Aliases),
			JSONShape:         strings.TrimSpace(projection.JSONShape),
			Fields:            cleanStrings(projection.Fields),
			AccessHint:        strings.TrimSpace(projection.AccessHint),
			SourcePaths:       cleanStrings(projection.SourcePaths),
			SourceRecordPaths: cleanStrings(projection.SourceRecordPaths),
			ReferencePaths:    cleanStrings(projection.ReferencePaths),
			EvidencePaths:     cleanStrings(projection.EvidencePaths),
		})
	}
	return out
}

func artifactAccessViewFieldSamples(artifact ArtifactProjectionSource, fields []string, maxFields, maxValues int) map[string][]string {
	if maxFields <= 0 || maxValues <= 0 || len(fields) == 0 || len(artifact.Sample) == 0 {
		return nil
	}
	allowed := map[string]string{}
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		if trimmed == "" {
			continue
		}
		allowed[strings.ToLower(trimmed)] = trimmed
	}
	if len(allowed) == 0 {
		return nil
	}
	out := map[string][]string{}
	seen := map[string]map[string]bool{}
	for _, sample := range artifact.Sample {
		if len(out) >= maxFields {
			break
		}
		sample = strings.TrimSpace(sample)
		if sample == "" {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(sample), &obj); err != nil {
			continue
		}
		keys := make([]string, 0, len(obj))
		for key := range obj {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			canonical := allowed[strings.ToLower(strings.TrimSpace(key))]
			if canonical == "" {
				continue
			}
			values := out[canonical]
			if len(values) >= maxValues {
				continue
			}
			value := artifactAccessViewSampleScalar(obj[key])
			if value == "" {
				continue
			}
			if seen[canonical] == nil {
				seen[canonical] = map[string]bool{}
			}
			if seen[canonical][value] {
				continue
			}
			seen[canonical][value] = true
			out[canonical] = append(out[canonical], value)
			if len(out) >= maxFields {
				break
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func artifactAccessViewSampleScalar(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return ClampRecordViewText(typed, 120)
	case bool:
		return fmt.Sprintf("%t", typed)
	case float64:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.12f", typed), "0"), ".")
	case int:
		return fmt.Sprintf("%d", typed)
	case int64:
		return fmt.Sprintf("%d", typed)
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}
