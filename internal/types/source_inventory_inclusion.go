package types

// source_inventory_inclusion.go — source-class inclusion guidance for the
// source-inventory lane (a concern sub-file so the census/observation files
// stay under their convergence ceilings).

// SourceInventoryAuxiliaryInclusionDirective returns soft guidance for the
// source-inventory lane when the repo's source-class universe contains
// non-production source classes (test / fixture / example / thirdparty / vendor /
// corpus / generated / prompt-support / documentation). It counters the observed
// failure where the model FINDS those git-tracked source files but then excludes
// them from a repo-wide "what exists" enumeration purely because of their
// path-role — answering an empty/zero set ("no .cj source", "0 @Entry pages")
// even though the corpus/fixture/thirdparty files are tracked in the repo and DO
// contain the requested constructs.
//
// It is precise (driven by the typed ClassifySourcePathRole class counts, never
// prose) and SOFT (the model still owns the answer and the user's explicit scope
// still narrows it). Returns "" when only production sources exist, so
// production-only repos see no extra steer.
func SourceInventoryAuxiliaryInclusionDirective(classes []SourceInventorySourceClassCount) string {
	hasAux := false
	for _, class := range classes {
		if class.Count <= 0 {
			continue
		}
		switch class.Role {
		case SourcePathRoleProduction, SourcePathRoleUnknown:
			// production is the default-included class; unknown ("") is noise.
		default:
			hasAux = true
		}
	}
	if !hasAux {
		return ""
	}
	return "These source_classes are git-tracked repository source files. For a repository-wide enumeration of what EXISTS in the repo, members in test / fixture / example / thirdparty / corpus / vendored / generated classes COUNT as answer members — do NOT drop them or report an empty/zero set solely because of their path-role. Narrow the class set only when the request explicitly scopes to production / first-party code or a named path."
}
