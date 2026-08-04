package dataquery

import "strings"

// entityResolutionLineageRoles mints the parent authority from the roles the
// executor actually used. In reference mode those roles may have been swapped
// after field-contract inspection, so re-inferring from original input order
// would invert source and reference lineage at the hard guard boundary.
func entityResolutionLineageRoles(action DataAction, consumed []string, records []EntityResolutionRecord, children []DataArtifact) ([]string, []string, []string) {
	var sourceRecordPaths []string
	var referencePaths []string
	for _, child := range children {
		switch strings.TrimSpace(child.Kind) {
		case "entity_resolution/source", "entity_resolution_source":
			sourceRecordPaths = append(sourceRecordPaths, child.SourceRecordPaths...)
			if len(child.SourceRecordPaths) == 0 {
				sourceRecordPaths = append(sourceRecordPaths, child.SourcePaths...)
			}
		case "entity_resolution/reference":
			referencePaths = append(referencePaths, child.ReferencePaths...)
			if len(child.ReferencePaths) == 0 {
				referencePaths = append(referencePaths, child.SourcePaths...)
			}
		}
	}
	if len(sourceRecordPaths) == 0 && len(referencePaths) == 0 {
		source := firstNonEmptyString(action.Params["source_path"], action.Params["source"], action.Params["base_path"])
		reference := firstNonEmptyString(action.Params["reference_path"], action.Params["lookup_path"], action.Params["mapping_path"])
		inputs := cleanStringList(action.InputPaths)
		if strings.TrimSpace(source) == "" && len(inputs) > 0 {
			source = inputs[0]
		}
		if strings.TrimSpace(reference) == "" && len(inputs) > 1 {
			reference = inputs[1]
		}
		sourceRecordPaths = append(sourceRecordPaths, source)
		referencePaths = append(referencePaths, reference)
	}
	sourceRecordPaths = normalizeMaterialPaths(cleanStringList(sourceRecordPaths))
	referencePaths = normalizeMaterialPaths(cleanStringList(referencePaths))
	var evidence []string
	for _, rec := range records {
		evidence = append(evidence, rec.EvidenceRefs...)
	}
	consumedSet := map[string]bool{}
	for _, path := range append(append([]string(nil), sourceRecordPaths...), referencePaths...) {
		consumedSet[normalizeMaterialPath(path)] = true
	}
	for _, path := range normalizeMaterialPaths(consumed) {
		if !consumedSet[normalizeMaterialPath(path)] {
			evidence = append(evidence, path)
		}
	}
	return sourceRecordPaths, referencePaths, cleanStringList(evidence)
}
