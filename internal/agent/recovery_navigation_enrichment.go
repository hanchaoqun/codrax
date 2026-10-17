package agent

// Source discovery remains available. Only materials whose current physical
// generation was actually consumed as recovery navigation are excluded from
// current-code enrichment; exact coordinates alone do not define their purpose.
func (e *explorerEvaluator) sourceEnrichmentFileSet(files map[string]bool) map[string]bool {
	if e == nil || e.mutable == nil {
		return files
	}
	out := make(map[string]bool, len(files))
	for path, read := range files {
		if !e.mutable.IsRecoveryNavigationMaterial(e.repoRoot, path) {
			out[path] = read
		}
	}
	return out
}
