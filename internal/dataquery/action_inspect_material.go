package dataquery

import (
	"fmt"
	"strings"
)

func (r ActionRunner) runInspectMaterial(action DataAction) (DataArtifact, error) {
	paths := cleanStringListPreserveOrder(action.InputPaths)
	if len(paths) == 0 {
		return DataArtifact{}, dataActionMissingParamError(DataActionInspectMaterial, "input_paths", "at least one material path", action.InputPaths)
	}
	root := firstNonEmptyString(strings.TrimSpace(r.RepoRoot), ".")
	all, err := DiscoverCandidateFiles(root, 1000)
	if err != nil {
		return DataArtifact{}, err
	}
	byPath := map[string]CandidateFile{}
	for _, f := range all {
		byPath[normalizeMaterialPath(f.Path)] = f
	}
	children := make([]DataArtifact, 0, len(paths))
	for _, p := range paths {
		key := normalizeMaterialPath(p)
		if abs, ok, resolveErr := r.NamedInputs.resolve(root, p); ok {
			if resolveErr != nil {
				return DataArtifact{}, resolveErr
			}
			children = append(children, candidateArtifact(inspectNamedCandidate(abs, key)))
			continue
		}
		if f, ok := byPath[key]; ok {
			children = append(children, candidateArtifact(f))
			continue
		}
		children = append(children, DataArtifact{
			ID:          key,
			Kind:        "unknown",
			SourcePaths: []string{key},
			Summary:     "material was requested but not found in candidate inventory",
		})
	}
	id := firstNonEmptyString(strings.TrimSpace(action.OutputArtifact), strings.TrimSpace(action.ID), "material_inspection")
	inspected := len(inspectedMaterialPaths(DataArtifact{Children: children}))
	return DataArtifact{
		ID:          id,
		Kind:        string(DataActionInspectMaterial),
		SourcePaths: normalizeMaterialPaths(paths),
		Summary:     fmt.Sprintf("inspected content from %d of %d requested material(s)", inspected, len(paths)),
		Fields:      map[string]string{"count": fmt.Sprintf("%d", len(paths)), "inspected_count": fmt.Sprintf("%d", inspected)},
		Children:    children,
	}, nil
}
