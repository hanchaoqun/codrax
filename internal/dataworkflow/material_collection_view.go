package dataworkflow

import (
	"path"
	"sort"
	"strings"
)

type MaterialCollectionView struct {
	ID                string   `json:"id,omitempty"`
	Kind              string   `json:"kind,omitempty"`
	Scope             string   `json:"scope,omitempty"`
	MemberPaths       []string `json:"member_paths,omitempty"`
	TextEvidencePaths []string `json:"text_evidence_paths,omitempty"`
	AccessHint        string   `json:"access_hint,omitempty"`
}

func BuildMaterialCollectionViews(artifacts []ArtifactProjectionSource, limit int) []MaterialCollectionView {
	if limit <= 0 || len(artifacts) == 0 {
		return nil
	}
	type group struct {
		kind    string
		members []string
	}
	groups := map[string]*group{}
	var related []MaterialCollectionView
	var walk func(ArtifactProjectionSource)
	walk = func(artifact ArtifactProjectionSource) {
		for _, p := range cleanStrings(artifact.SourcePaths) {
			dir := path.Dir(p)
			if dir != "." && dir != "" {
				g := groups[dir]
				if g == nil {
					g = &group{kind: strings.TrimSpace(artifact.Kind)}
					groups[dir] = g
				}
				g.members = append(g.members, p)
			}
		}
		textPaths := cleanStrings(strings.Split(artifactField(artifact, "text_evidence_paths"), ","))
		if len(textPaths) > 0 {
			related = append(related, MaterialCollectionView{
				ID:                "related_text:" + strings.TrimSpace(artifact.ID),
				Kind:              "related_text_evidence",
				Scope:             strings.TrimSpace(artifact.ID),
				MemberPaths:       ClampRecordViewStringSlice(cleanStrings(artifact.SourcePaths), 4),
				TextEvidencePaths: ClampRecordViewStringSlice(textPaths, 8),
				AccessHint:        "if this source material is relevant to the data goal, add the concrete text_evidence_paths to a bounded coverage/action batch before compute",
			})
		}
		for _, child := range artifact.Children {
			walk(child)
		}
	}
	for _, artifact := range artifacts {
		walk(artifact)
	}
	var out []MaterialCollectionView
	sort.Slice(related, func(i, j int) bool { return related[i].ID < related[j].ID })
	for _, h := range related {
		if len(out) >= limit {
			return out
		}
		out = append(out, h)
	}
	keys := make([]string, 0, len(groups))
	for key, g := range groups {
		g.members = uniqueSortedMaterialCollectionStrings(g.members)
		if len(g.members) >= 2 {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		if len(out) >= limit {
			break
		}
		g := groups[key]
		out = append(out, MaterialCollectionView{
			ID:          "dir:" + key,
			Kind:        firstMaterialCollectionString(g.kind, "material_group"),
			Scope:       key,
			MemberPaths: ClampRecordViewStringSlice(g.members, 12),
			AccessHint:  "candidate file group from inventory/inspection; expand only the concrete members required by the current data goal",
		})
	}
	return out
}

func uniqueSortedMaterialCollectionStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range cleanStrings(in) {
		if seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func firstMaterialCollectionString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
