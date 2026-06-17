package writeflow

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
	"gopkg.in/yaml.v3"
)

var (
	unifiedDiffHunkHeaderRE             = regexp.MustCompile(`^@@ -([0-9]+)(?:,([0-9]+))? \+([0-9]+)(?:,([0-9]+))? @@`)
	pythonTopLevelSelfMethodDefRE       = regexp.MustCompile(`^def\s+[A-Za-z_][A-Za-z0-9_]*\s*\(\s*(?:self|cls)\b`)
	dynamicNestedStringKeyAccessRE      = regexp.MustCompile(`(?:\b(?:self|cls|this)\.[A-Za-z_$][A-Za-z0-9_$]*|\b[A-Za-z_$][A-Za-z0-9_$]*)(?:\s*\[\s*['"][^'"]+['"]\s*\]){2,}`)
	rubyNestedSymbolOrStringKeyAccessRE = regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*(?:\s*\[\s*(?::[A-Za-z_][A-Za-z0-9_]*|['"][^'"]+['"])\s*\]){2,}`)
	jvmChainedStringMapGetRE            = regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*(?:\.get\(\s*"[^"]+"\s*\)){2,}`)
	goNestedStringMapAssignmentRE       = regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*(?:\s*\[\s*"[^"]+"\s*\]){2,}\s*=(?:[^=]|$)`)
)

type patchEffectLineShapeRule struct {
	code    string
	message string
	match   *regexp.Regexp
}

func PatchEffectRecordFromUnifiedDiff(planID, sliceID, source, baseRef, headRef, diff string) types.PatchEffectRecord {
	record := types.PatchEffectRecord{
		PlanID:          strings.TrimSpace(planID),
		SliceID:         strings.TrimSpace(sliceID),
		Source:          strings.TrimSpace(source),
		BaseRef:         strings.TrimSpace(baseRef),
		HeadRef:         strings.TrimSpace(headRef),
		DiffFingerprint: patchEffectFingerprint(diff),
		DiffBytes:       len([]byte(diff)),
		CreatedAt:       time.Now(),
	}
	if record.Source == "" {
		record.Source = "unified_diff"
	}
	record.RecordID = patchEffectRecordID(record.PlanID, record.SliceID, record.DiffFingerprint)
	var current *types.PatchEffectFile
	var currentHunk *types.PatchEffectHunk
	currentOldLine := 0
	currentNewLine := 0
	flushFile := func() {
		if current == nil {
			return
		}
		if currentHunk != nil {
			current.Hunks = append(current.Hunks, *currentHunk)
			currentHunk = nil
		}
		normalizePatchEffectFile(current)
		if current.Path != "" || current.OldPath != "" {
			record.Files = append(record.Files, *current)
		}
		current = nil
	}
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			flushFile()
			oldPath, newPath := parsePatchEffectDiffGitLine(line)
			current = &types.PatchEffectFile{
				Path:    newPath,
				OldPath: oldPath,
				Status:  "modified",
			}
			continue
		}
		if current == nil {
			continue
		}
		switch {
		case strings.HasPrefix(line, "new file mode "):
			current.Status = "created"
		case strings.HasPrefix(line, "deleted file mode "):
			current.Status = "deleted"
		case strings.HasPrefix(line, "rename from "):
			current.Status = "renamed"
			current.OldPath = normalizePatchEffectPath(strings.TrimPrefix(line, "rename from "))
		case strings.HasPrefix(line, "rename to "):
			current.Status = "renamed"
			current.Path = normalizePatchEffectPath(strings.TrimPrefix(line, "rename to "))
		case strings.HasPrefix(line, "--- "):
			oldPath := normalizePatchEffectDiffPath(strings.TrimPrefix(line, "--- "))
			if oldPath != "" {
				current.OldPath = oldPath
			}
		case strings.HasPrefix(line, "+++ "):
			newPath := normalizePatchEffectDiffPath(strings.TrimPrefix(line, "+++ "))
			if newPath != "" {
				current.Path = newPath
			}
		case strings.HasPrefix(line, "@@ "):
			if currentHunk != nil {
				current.Hunks = append(current.Hunks, *currentHunk)
			}
			hunk := parsePatchEffectHunkHeader(line)
			currentHunk = &hunk
			currentOldLine = hunk.OldStart
			currentNewLine = hunk.NewStart
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			text := strings.TrimPrefix(line, "+")
			current.AddedLines++
			if currentHunk != nil {
				currentHunk.AddedLines++
				if currentNewLine > 0 {
					currentHunk.AddedLineNumbers = append(currentHunk.AddedLineNumbers, currentNewLine)
					currentHunk.AddedLineTexts = append(currentHunk.AddedLineTexts, types.PatchEffectLine{Line: currentNewLine, Text: text})
				} else {
					currentHunk.AddedLineTexts = append(currentHunk.AddedLineTexts, types.PatchEffectLine{Text: text})
				}
			}
			if currentNewLine > 0 {
				currentNewLine++
			}
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			text := strings.TrimPrefix(line, "-")
			current.RemovedLines++
			if currentHunk != nil {
				currentHunk.RemovedLines++
				if currentOldLine > 0 {
					currentHunk.RemovedLineTexts = append(currentHunk.RemovedLineTexts, types.PatchEffectLine{Line: currentOldLine, Text: text})
				} else {
					currentHunk.RemovedLineTexts = append(currentHunk.RemovedLineTexts, types.PatchEffectLine{Text: text})
				}
			}
			if currentOldLine > 0 {
				currentOldLine++
			}
		case strings.HasPrefix(line, " "):
			if currentOldLine > 0 {
				currentOldLine++
			}
			if currentNewLine > 0 {
				currentNewLine++
			}
		}
	}
	flushFile()
	record.Files = normalizePatchEffectFiles(record.Files)
	return record
}

func parsePatchEffectDiffGitLine(line string) (string, string) {
	parts := strings.Fields(strings.TrimSpace(line))
	if len(parts) < 4 {
		return "", ""
	}
	return normalizePatchEffectDiffPath(parts[2]), normalizePatchEffectDiffPath(parts[3])
}

func parsePatchEffectHunkHeader(line string) types.PatchEffectHunk {
	match := unifiedDiffHunkHeaderRE.FindStringSubmatch(line)
	if len(match) == 0 {
		return types.PatchEffectHunk{}
	}
	return types.PatchEffectHunk{
		OldStart: parsePatchEffectInt(match[1], 0),
		OldLines: parsePatchEffectIntDefault(match[2], 1),
		NewStart: parsePatchEffectInt(match[3], 0),
		NewLines: parsePatchEffectIntDefault(match[4], 1),
	}
}

func parsePatchEffectInt(raw string, fallback int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}

func parsePatchEffectIntDefault(raw string, fallback int) int {
	return parsePatchEffectInt(raw, fallback)
}

func normalizePatchEffectFiles(in []types.PatchEffectFile) []types.PatchEffectFile {
	out := make([]types.PatchEffectFile, 0, len(in))
	seen := map[string]int{}
	for _, file := range in {
		normalizePatchEffectFile(&file)
		key := file.OldPath + "->" + file.Path + ":" + file.Status
		if idx, ok := seen[key]; ok {
			out[idx].AddedLines += file.AddedLines
			out[idx].RemovedLines += file.RemovedLines
			out[idx].Hunks = append(out[idx].Hunks, file.Hunks...)
			continue
		}
		seen[key] = len(out)
		out = append(out, file)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].OldPath < out[j].OldPath
	})
	return out
}

func normalizePatchEffectFile(file *types.PatchEffectFile) {
	if file == nil {
		return
	}
	file.Path = normalizePatchEffectPath(file.Path)
	file.OldPath = normalizePatchEffectPath(file.OldPath)
	if file.Path == "" && file.OldPath != "" {
		file.Path = file.OldPath
	}
	file.Status = strings.TrimSpace(file.Status)
	if file.Status == "" {
		file.Status = "modified"
	}
	ext := strings.ToLower(filepath.Ext(file.Path))
	if ext == "" {
		ext = strings.ToLower(filepath.Ext(file.OldPath))
	}
	file.Language = strings.TrimPrefix(ext, ".")
	if file.PathRole == "" {
		file.PathRole = types.ClassifySourcePathRole(file.Path)
	}
}

func normalizePatchEffectDiffPath(raw string) string {
	raw = strings.Trim(strings.TrimSpace(raw), `"`)
	switch raw {
	case "", "/dev/null":
		return ""
	}
	raw = strings.TrimPrefix(raw, "a/")
	raw = strings.TrimPrefix(raw, "b/")
	return normalizePatchEffectPath(raw)
}

func normalizePatchEffectPath(raw string) string {
	path := filepath.ToSlash(strings.TrimSpace(raw))
	path = strings.TrimPrefix(path, "./")
	if path == "." {
		return ""
	}
	return path
}

func patchEffectFingerprint(diff string) string {
	sum := sha256.Sum256([]byte(diff))
	return hex.EncodeToString(sum[:])
}

func patchEffectRecordID(planID, sliceID, fp string) string {
	planID = strings.TrimSpace(planID)
	sliceID = strings.TrimSpace(sliceID)
	if len(fp) > 12 {
		fp = fp[:12]
	}
	if sliceID == "" {
		return fmt.Sprintf("patch-effect:%s:%s", planID, fp)
	}
	return fmt.Sprintf("patch-effect:%s:%s:%s", planID, sliceID, fp)
}

func AnnotatePatchEffectStructuredFileParses(record *types.PatchEffectRecord, root string) {
	if record == nil || strings.TrimSpace(root) == "" {
		return
	}
	for i := range record.Files {
		file := &record.Files[i]
		if strings.TrimSpace(file.Path) == "" || strings.TrimSpace(file.Status) == "deleted" {
			continue
		}
		kind := patchEffectStructuredFileKind(file.Path)
		sourceKind := patchEffectSourceShapeKind(file.Path)
		if kind == "" && sourceKind == "" {
			continue
		}
		abs, ok := patchEffectSafeRepoPath(root, file.Path)
		if !ok {
			file.Events = append(file.Events, types.PatchEffectEvent{
				Code:     "patch_effect_path_outside_worktree",
				Severity: "error",
				Path:     file.Path,
				Message:  "patch effect path is outside the worktree root",
			})
			continue
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			file.Events = append(file.Events, types.PatchEffectEvent{
				Code:     "structured_file_missing",
				Severity: "error",
				Path:     file.Path,
				Message:  kind + " file could not be read after apply",
			})
			continue
		}
		if kind != "" {
			if err := validatePatchEffectStructuredFile(kind, data); err != nil {
				file.Events = append(file.Events, types.PatchEffectEvent{
					Code:     "structured_file_parse_error",
					Severity: "error",
					Path:     file.Path,
					Message:  kind + " parse failed: " + err.Error(),
				})
			}
		}
		annotatePatchEffectSourceShape(file, data, sourceKind)
	}
}

func annotatePatchEffectSourceShape(file *types.PatchEffectFile, data []byte, sourceKind string) {
	if file == nil || len(file.Hunks) == 0 {
		return
	}
	sourceKind = strings.TrimSpace(sourceKind)
	lines := strings.Split(string(data), "\n")
	for _, hunk := range file.Hunks {
		if sourceKind == "python" {
			annotatePatchEffectPythonOwnerShape(file, lines, hunk)
		}
		for _, added := range hunk.AddedLineTexts {
			appendPatchEffectLineShapeEvents(file, sourceKind, added)
		}
	}
}

func annotatePatchEffectPythonOwnerShape(file *types.PatchEffectFile, lines []string, hunk types.PatchEffectHunk) {
	for _, lineNo := range hunk.AddedLineNumbers {
		if lineNo <= 0 || lineNo > len(lines) {
			continue
		}
		line := lines[lineNo-1]
		if !pythonTopLevelSelfMethodDefRE.MatchString(line) {
			continue
		}
		file.Events = append(file.Events, types.PatchEffectEvent{
			Code:        "python_top_level_self_method",
			Severity:    "error",
			Path:        file.Path,
			Message:     "added top-level Python function uses self/cls as its first parameter; this usually means a class method was de-indented out of its owner class",
			EvidenceRef: fmt.Sprintf("%s:%d", file.Path, lineNo),
		})
	}
}

func appendPatchEffectLineShapeEvents(file *types.PatchEffectFile, sourceKind string, line types.PatchEffectLine) {
	for _, rule := range patchEffectLineShapeRules(sourceKind) {
		if rule.match == nil || !rule.match.MatchString(line.Text) {
			continue
		}
		evidence := file.Path
		if line.Line > 0 {
			evidence = fmt.Sprintf("%s:%d", file.Path, line.Line)
		}
		file.Events = append(file.Events, types.PatchEffectEvent{
			Code:        rule.code,
			Severity:    "warning",
			Path:        file.Path,
			Message:     rule.message,
			EvidenceRef: evidence,
		})
	}
}

func patchEffectLineShapeRules(sourceKind string) []patchEffectLineShapeRule {
	switch strings.TrimSpace(sourceKind) {
	case "python":
		return []patchEffectLineShapeRule{{
			code:    "python_nested_string_key_direct_access_added",
			match:   dynamicNestedStringKeyAccessRE,
			message: "added Python code uses nested string-key direct mapping access; verify the absent-key/default boundary or prefer the repository's nullable lookup convention where appropriate",
		}}
	case "javascript", "typescript":
		return []patchEffectLineShapeRule{{
			code:    sourceKind + "_nested_string_key_direct_access_added",
			match:   dynamicNestedStringKeyAccessRE,
			message: "added JS/TS code uses nested string-key direct mapping access; verify the undefined/null boundary or prefer the repository's nullable lookup convention where appropriate",
		}}
	case "ruby":
		return []patchEffectLineShapeRule{{
			code:    "ruby_nested_key_direct_access_added",
			match:   rubyNestedSymbolOrStringKeyAccessRE,
			message: "added Ruby code uses nested hash-key direct access; verify the nil/default boundary or prefer the repository's safe lookup convention where appropriate",
		}}
	case "java", "kotlin":
		return []patchEffectLineShapeRule{{
			code:    sourceKind + "_chained_string_map_get_added",
			match:   jvmChainedStringMapGetRE,
			message: "added JVM code uses chained string-key map get calls; verify the null/default boundary or prefer the repository's safe lookup convention where appropriate",
		}}
	case "go":
		return []patchEffectLineShapeRule{{
			code:    "go_nested_string_map_assignment_added",
			match:   goNestedStringMapAssignmentRE,
			message: "added Go code assigns through nested string-key map indexes; verify nested map initialization or prefer the repository's ensure-map convention where appropriate",
		}}
	default:
		return nil
	}
}

func patchEffectStructuredFileKind(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".xml":
		return "xml"
	default:
		return ""
	}
}

func patchEffectSourceShapeKind(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".py":
		return "python"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".ts", ".tsx", ".mts", ".cts":
		return "typescript"
	case ".rb":
		return "ruby"
	case ".java":
		return "java"
	case ".kt", ".kts":
		return "kotlin"
	case ".go":
		return "go"
	default:
		return ""
	}
}

func validatePatchEffectStructuredFile(kind string, data []byte) error {
	switch kind {
	case "json":
		var v any
		return json.Unmarshal(data, &v)
	case "yaml":
		var v any
		return yaml.Unmarshal(data, &v)
	case "xml":
		decoder := xml.NewDecoder(bytes.NewReader(data))
		for {
			if _, err := decoder.Token(); err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}
		}
	default:
		return nil
	}
}

func patchEffectSafeRepoPath(root, rel string) (string, bool) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	rel = normalizePatchEffectPath(rel)
	if rel == "" {
		return "", false
	}
	abs, err := filepath.Abs(filepath.Join(rootAbs, filepath.FromSlash(rel)))
	if err != nil {
		return "", false
	}
	if abs == rootAbs || strings.HasPrefix(abs, rootAbs+string(filepath.Separator)) {
		return abs, true
	}
	return "", false
}
