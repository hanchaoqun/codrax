package tool

import (
	"fmt"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

const sourceInventoryReadFilePathMissingSampleReason = "read_file_path_missing_source_inventory_sample"

type sourceInventoryReadFilePathMissAdvice struct {
	Summary    string
	Metadata   map[string]string
	Refinement *types.ToolRefinementHint
}

func sourceInventoryReadFilePathMissHint(ctx *types.BusContext, requested string) string {
	return sourceInventoryReadFilePathMissAdviceFor(ctx, requested).Summary
}

func sourceInventoryReadFilePathMissAdviceFor(ctx *types.BusContext, requested string) sourceInventoryReadFilePathMissAdvice {
	if ctx == nil || ctx.Mutable == nil {
		return sourceInventoryReadFilePathMissAdvice{}
	}
	observation := ctx.Mutable.SourceInventoryObservation()
	if !observation.IsActive() {
		return sourceInventoryReadFilePathMissAdvice{}
	}
	requestedRel, ok := sourceInventoryReadFileRequestedRel(ctx, requested)
	if !ok || requestedRel == "" {
		return sourceInventoryReadFilePathMissAdvice{}
	}
	requestedDir := path.Dir(requestedRel)
	if requestedDir == "." && strings.Contains(requestedRel, "/") {
		requestedDir = path.Dir(strings.Trim(requestedRel, "/"))
	}
	groups := sourceInventorySuggestedFileGroups(observation, types.SourceInventoryLensQuery{})
	if len(groups) == 0 {
		return sourceInventoryReadFilePathMissAdvice{}
	}
	files := sourceInventorySuggestedFilesForRequestedScope(groups, requestedDir, requestedRel)
	if len(files) == 0 {
		return sourceInventoryReadFilePathMissAdvice{}
	}
	const (
		maxFiles = 5
		maxItems = 3
	)
	totalFiles := len(files)
	truncated := totalFiles > maxFiles || sourceInventorySuggestedFilesHaveHiddenCandidates(files, maxItems)
	if len(files) > maxFiles {
		files = files[:maxFiles]
	}
	parts := make([]string, 0, len(files))
	for _, file := range files {
		if part := renderSourceInventorySuggestedFile(file, maxItems); part != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return sourceInventoryReadFilePathMissAdvice{}
	}
	scopeLabel := requestedDir
	if scopeLabel == "." || scopeLabel == "" {
		scopeLabel = "repo root"
	} else {
		scopeLabel = "`" + scopeLabel + "`"
	}
	summary := fmt.Sprintf("\n\nSource-inventory hint (advisory): the requested path is missing. Known candidate files in the same scope %s: %s. These are repo-map navigation suggestions, not a whitelist; read the relevant file before citing.", scopeLabel, strings.Join(parts, "; "))
	scopeParam := requestedDir
	if scopeParam == "" {
		scopeParam = "."
	}
	return sourceInventoryReadFilePathMissAdvice{
		Summary: summary,
		Metadata: map[string]string{
			"source_inventory_candidate_sample":               "true",
			"source_inventory_candidate_scope":                scopeParam,
			"source_inventory_candidate_sample_count":         strconv.Itoa(len(files)),
			"source_inventory_candidate_total_files":          strconv.Itoa(totalFiles),
			"source_inventory_candidate_sample_truncated":     strconv.FormatBool(truncated),
			"source_inventory_candidate_sample_not_whitelist": "true",
			"absence_requires_sibling_enumeration":            "true",
		},
		Refinement: &types.ToolRefinementHint{
			ReasonCode:        sourceInventoryReadFilePathMissingSampleReason,
			ResultTruncated:   truncated,
			PreferredNextTool: "list_files",
			PreferredParams: map[string]string{
				"path":      scopeParam,
				"recursive": "false",
			},
			RequiredFields: []string{"path"},
		},
	}
}

func sourceInventorySuggestedFilesHaveHiddenCandidates(files []sourceInventorySuggestedFile, maxItems int) bool {
	if maxItems <= 0 {
		return len(files) > 0
	}
	for _, file := range files {
		if len(file.Candidates) > maxItems {
			return true
		}
	}
	return false
}

func sourceInventoryReadFileRequestedRel(ctx *types.BusContext, requested string) (string, bool) {
	raw := strings.TrimSpace(strings.ReplaceAll(requested, `\`, `/`))
	if raw == "" {
		return "", false
	}
	if normalized, ok := normalizeWindowsPOSIXPath(raw); ok {
		raw = filepath.ToSlash(normalized)
	}
	if toolPathIsAbs(raw) {
		if ctx == nil || strings.TrimSpace(ctx.RepoRoot) == "" {
			return "", false
		}
		root := filepath.Clean(ctx.RepoRoot)
		abs := filepath.Clean(raw)
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			return "", false
		}
		rel = filepath.ToSlash(filepath.Clean(rel))
		if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
			return "", false
		}
		return rel, true
	}
	cleaned := path.Clean(raw)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	if ctx != nil && strings.TrimSpace(ctx.RepoRoot) != "" {
		parts := strings.Split(cleaned, "/")
		repoLabel := filepath.Base(filepath.Clean(ctx.RepoRoot))
		if len(parts) > 1 && repoLabel != "" && parts[0] == repoLabel {
			cleaned = strings.Join(parts[1:], "/")
		}
	}
	if cleaned == "" || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	return strings.Trim(cleaned, "/"), true
}
