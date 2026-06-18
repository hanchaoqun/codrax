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
	pythonClassDefLineRE                = regexp.MustCompile(`^(\s*)class\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	pythonDefLineRE                     = regexp.MustCompile(`^(\s*)def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	pythonTopLevelSelfMethodDefRE       = regexp.MustCompile(`^def\s+[A-Za-z_][A-Za-z0-9_]*\s*\(\s*(?:self|cls)\b`)
	dynamicNestedStringKeyAccessRE      = regexp.MustCompile(`(?:\b(?:self|cls|this)\.[A-Za-z_$][A-Za-z0-9_$]*|\b[A-Za-z_$][A-Za-z0-9_$]*)(?:\s*\[\s*['"][^'"]+['"]\s*\]){2,}`)
	rubyNestedSymbolOrStringKeyAccessRE = regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*(?:\s*\[\s*(?::[A-Za-z_][A-Za-z0-9_]*|['"][^'"]+['"])\s*\]){2,}`)
	jvmChainedStringMapGetRE            = regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*(?:\.get\(\s*"[^"]+"\s*\)){2,}`)
	goNestedStringMapAssignmentRE       = regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*(?:\s*\[\s*"[^"]+"\s*\]){2,}\s*=(?:[^=]|$)`)
	pythonTestScaffoldDeclarationRE     = regexp.MustCompile(`^\s*(?:class\s+[A-Za-z_][A-Za-z0-9_]*(?:Test|Tests)\b|def\s+test_[A-Za-z0-9_]*\s*\()`)
	jvmTestScaffoldDeclarationRE        = regexp.MustCompile(`^\s*(?:(?:public|private|protected|internal|open|final|abstract|static)\s+)*class\s+[A-Za-z_][A-Za-z0-9_]*(?:Test|Tests)\b`)
	goTestScaffoldDeclarationRE         = regexp.MustCompile(`^\s*func\s+Test[A-Z0-9_][A-Za-z0-9_]*\s*\(`)
	jsTestScaffoldDeclarationRE         = regexp.MustCompile(`^\s*(?:export\s+)?(?:class|function)\s+[A-Za-z_$][A-Za-z0-9_$]*(?:Test|Tests|Spec)\b`)
	rubyTestScaffoldDeclarationRE       = regexp.MustCompile(`^\s*(?:class\s+[A-Za-z_:][A-Za-z0-9_:]*(?:Test|Tests)\b|def\s+test_[A-Za-z0-9_!?=]*\b)`)
	sourceReturnStatementRE             = regexp.MustCompile(`^\s*return\s+(.+?)\s*;?\s*$`)
	sourceConditionalGuardLineRE        = regexp.MustCompile(`^\s*(?:if|elif|else\s+if|unless)\b`)
	pythonDiagnosticCallRE              = regexp.MustCompile(`\b(?:warnings\.)?(?:warn|warning)\s*\(`)
	jsDiagnosticCallRE                  = regexp.MustCompile(`\b(?:console|logger|log)\.(?:warn|warning|error)\s*\(`)
	jvmDiagnosticCallRE                 = regexp.MustCompile(`\b(?:logger|log)\.(?:warn|warning|error)\s*\(`)
	rubyDiagnosticCallRE                = regexp.MustCompile(`\b(?:warn|warning|logger\.(?:warn|warning|error))\s*(?:\(|\s)`)
	externalUnderscoreFieldAssignRE     = regexp.MustCompile(`\b([A-Za-z_$][A-Za-z0-9_$]*)\._[A-Za-z_$][A-Za-z0-9_$]*\s*=`)
	rubyExternalPrivateStateAssignRE    = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\.instance_variable_set\s*\(\s*:@[A-Za-z_][A-Za-z0-9_]*`)
)

type patchEffectLineShapeRule struct {
	code    string
	message string
	match   *regexp.Regexp
}

type patchEffectSourceProvider struct {
	Kind               string
	Extensions         []string
	LineRules          []patchEffectLineShapeRule
	ProductionTestRule patchEffectLineShapeRule
	OwnerBoundary      patchEffectOwnerBoundaryRules
	AnnotateFile       func(file *types.PatchEffectFile, lines []string)
	AnnotateHunk       func(file *types.PatchEffectFile, lines []string, hunk types.PatchEffectHunk)
}

type patchEffectOwnerBoundaryRules struct {
	ReturnWrapper             bool
	DiagnosticCall            *regexp.Regexp
	ExternalPrivateAssignment *regexp.Regexp
	InternalReceivers         []string
}

var patchEffectSourceProviders = []patchEffectSourceProvider{
	{
		Kind:       "python",
		Extensions: []string{".py"},
		LineRules: []patchEffectLineShapeRule{{
			code:    "python_nested_string_key_direct_access_added",
			match:   dynamicNestedStringKeyAccessRE,
			message: "added Python code uses nested string-key direct mapping access; verify the absent-key/default boundary or prefer the repository's nullable lookup convention where appropriate",
		}},
		ProductionTestRule: patchEffectLineShapeRule{
			code:    "production_test_scaffold_added",
			match:   pythonTestScaffoldDeclarationRE,
			message: "added test-shaped Python declaration in a production path; verify that test scaffolding belongs in production source or move it to the repository's test surface",
		},
		OwnerBoundary: patchEffectOwnerBoundaryRules{
			ReturnWrapper:             true,
			DiagnosticCall:            pythonDiagnosticCallRE,
			ExternalPrivateAssignment: externalUnderscoreFieldAssignRE,
			InternalReceivers:         []string{"self", "cls"},
		},
		AnnotateFile: annotatePatchEffectPythonDuplicateDeclarations,
		AnnotateHunk: annotatePatchEffectPythonOwnerShape,
	},
	{
		Kind:       "javascript",
		Extensions: []string{".js", ".jsx", ".mjs", ".cjs"},
		LineRules: []patchEffectLineShapeRule{{
			code:    "javascript_nested_string_key_direct_access_added",
			match:   dynamicNestedStringKeyAccessRE,
			message: "added JS/TS code uses nested string-key direct mapping access; verify the undefined/null boundary or prefer the repository's nullable lookup convention where appropriate",
		}},
		ProductionTestRule: patchEffectLineShapeRule{
			code:    "production_test_scaffold_added",
			match:   jsTestScaffoldDeclarationRE,
			message: "added test-shaped JS/TS declaration in a production path; verify that test scaffolding belongs in production source or move it to the repository's test surface",
		},
		OwnerBoundary: patchEffectOwnerBoundaryRules{
			ReturnWrapper:             true,
			DiagnosticCall:            jsDiagnosticCallRE,
			ExternalPrivateAssignment: externalUnderscoreFieldAssignRE,
			InternalReceivers:         []string{"this", "super"},
		},
	},
	{
		Kind:       "typescript",
		Extensions: []string{".ts", ".tsx", ".mts", ".cts"},
		LineRules: []patchEffectLineShapeRule{{
			code:    "typescript_nested_string_key_direct_access_added",
			match:   dynamicNestedStringKeyAccessRE,
			message: "added JS/TS code uses nested string-key direct mapping access; verify the undefined/null boundary or prefer the repository's nullable lookup convention where appropriate",
		}},
		ProductionTestRule: patchEffectLineShapeRule{
			code:    "production_test_scaffold_added",
			match:   jsTestScaffoldDeclarationRE,
			message: "added test-shaped JS/TS declaration in a production path; verify that test scaffolding belongs in production source or move it to the repository's test surface",
		},
		OwnerBoundary: patchEffectOwnerBoundaryRules{
			ReturnWrapper:             true,
			DiagnosticCall:            jsDiagnosticCallRE,
			ExternalPrivateAssignment: externalUnderscoreFieldAssignRE,
			InternalReceivers:         []string{"this", "super"},
		},
	},
	{
		Kind:       "ruby",
		Extensions: []string{".rb"},
		LineRules: []patchEffectLineShapeRule{{
			code:    "ruby_nested_key_direct_access_added",
			match:   rubyNestedSymbolOrStringKeyAccessRE,
			message: "added Ruby code uses nested hash-key direct access; verify the nil/default boundary or prefer the repository's safe lookup convention where appropriate",
		}},
		ProductionTestRule: patchEffectLineShapeRule{
			code:    "production_test_scaffold_added",
			match:   rubyTestScaffoldDeclarationRE,
			message: "added test-shaped Ruby declaration in a production path; verify that test scaffolding belongs in production source or move it to the repository's test surface",
		},
		OwnerBoundary: patchEffectOwnerBoundaryRules{
			ReturnWrapper:             true,
			DiagnosticCall:            rubyDiagnosticCallRE,
			ExternalPrivateAssignment: rubyExternalPrivateStateAssignRE,
			InternalReceivers:         []string{"self", "super"},
		},
	},
	{
		Kind:       "java",
		Extensions: []string{".java"},
		LineRules: []patchEffectLineShapeRule{{
			code:    "java_chained_string_map_get_added",
			match:   jvmChainedStringMapGetRE,
			message: "added JVM code uses chained string-key map get calls; verify the null/default boundary or prefer the repository's safe lookup convention where appropriate",
		}},
		ProductionTestRule: patchEffectLineShapeRule{
			code:    "production_test_scaffold_added",
			match:   jvmTestScaffoldDeclarationRE,
			message: "added test-shaped JVM declaration in a production path; verify that test scaffolding belongs in production source or move it to the repository's test surface",
		},
		OwnerBoundary: patchEffectOwnerBoundaryRules{
			ReturnWrapper:             true,
			DiagnosticCall:            jvmDiagnosticCallRE,
			ExternalPrivateAssignment: externalUnderscoreFieldAssignRE,
			InternalReceivers:         []string{"this", "super"},
		},
	},
	{
		Kind:       "kotlin",
		Extensions: []string{".kt", ".kts"},
		LineRules: []patchEffectLineShapeRule{{
			code:    "kotlin_chained_string_map_get_added",
			match:   jvmChainedStringMapGetRE,
			message: "added JVM code uses chained string-key map get calls; verify the null/default boundary or prefer the repository's safe lookup convention where appropriate",
		}},
		ProductionTestRule: patchEffectLineShapeRule{
			code:    "production_test_scaffold_added",
			match:   jvmTestScaffoldDeclarationRE,
			message: "added test-shaped JVM declaration in a production path; verify that test scaffolding belongs in production source or move it to the repository's test surface",
		},
		OwnerBoundary: patchEffectOwnerBoundaryRules{
			ReturnWrapper:             true,
			DiagnosticCall:            jvmDiagnosticCallRE,
			ExternalPrivateAssignment: externalUnderscoreFieldAssignRE,
			InternalReceivers:         []string{"this", "super"},
		},
	},
	{
		Kind:       "go",
		Extensions: []string{".go"},
		LineRules: []patchEffectLineShapeRule{{
			code:    "go_nested_string_map_assignment_added",
			match:   goNestedStringMapAssignmentRE,
			message: "added Go code assigns through nested string-key map indexes; verify nested map initialization or prefer the repository's ensure-map convention where appropriate",
		}},
		ProductionTestRule: patchEffectLineShapeRule{
			code:    "production_test_scaffold_added",
			match:   goTestScaffoldDeclarationRE,
			message: "added Go test function in a production path; verify that test scaffolding belongs in production source or move it to the repository's test surface",
		},
		OwnerBoundary: patchEffectOwnerBoundaryRules{
			ReturnWrapper: true,
		},
	},
}

type pythonPatchEffectScope struct {
	Indent int
	Name   string
}

type pythonPatchEffectDeclaration struct {
	Owner string
	Name  string
	Kind  string
	Line  int
	Key   string
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
		sourceProvider := patchEffectSourceProviderForPath(file.Path)
		if kind == "" && sourceProvider == nil {
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
		annotatePatchEffectSourceShape(file, data, sourceProvider)
	}
}

func annotatePatchEffectSourceShape(file *types.PatchEffectFile, data []byte, provider *patchEffectSourceProvider) {
	if file == nil || len(file.Hunks) == 0 {
		return
	}
	if provider == nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	if provider.AnnotateFile != nil {
		provider.AnnotateFile(file, lines)
	}
	for _, hunk := range file.Hunks {
		if provider.AnnotateHunk != nil {
			provider.AnnotateHunk(file, lines, hunk)
		}
		appendPatchEffectOwnerBoundaryEvents(file, provider, hunk)
		for _, added := range hunk.AddedLineTexts {
			appendPatchEffectLineShapeEvents(file, provider, added)
			appendPatchEffectProductionTestScaffoldEvent(file, provider, added)
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

func annotatePatchEffectPythonDuplicateDeclarations(file *types.PatchEffectFile, lines []string) {
	if file == nil {
		return
	}
	added := map[int]bool{}
	for _, hunk := range file.Hunks {
		for _, lineNo := range hunk.AddedLineNumbers {
			if lineNo > 0 {
				added[lineNo] = true
			}
		}
	}
	if len(added) == 0 {
		return
	}
	decls := pythonPatchEffectDeclarations(lines)
	byKey := map[string][]pythonPatchEffectDeclaration{}
	for _, decl := range decls {
		byKey[decl.Key] = append(byKey[decl.Key], decl)
	}
	reported := map[string]bool{}
	for _, decl := range decls {
		if !added[decl.Line] || len(byKey[decl.Key]) < 2 || reported[decl.Key] {
			continue
		}
		reported[decl.Key] = true
		file.Events = append(file.Events, types.PatchEffectEvent{
			Code:        "python_duplicate_symbol_added",
			Severity:    "error",
			Path:        file.Path,
			Message:     fmt.Sprintf("added Python %s %q duplicates another declaration in owner scope %q", decl.Kind, decl.Name, decl.Owner),
			EvidenceRef: fmt.Sprintf("%s:%d", file.Path, decl.Line),
		})
	}
}

func pythonPatchEffectDeclarations(lines []string) []pythonPatchEffectDeclaration {
	var scopes []pythonPatchEffectScope
	var out []pythonPatchEffectDeclaration
	for i, line := range lines {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		indent := pythonPatchEffectIndent(line)
		for len(scopes) > 0 && indent <= scopes[len(scopes)-1].Indent {
			scopes = scopes[:len(scopes)-1]
		}
		if match := pythonClassDefLineRE.FindStringSubmatch(line); match != nil {
			owner := pythonPatchEffectOwner(scopes)
			name := strings.TrimSpace(match[2])
			out = append(out, pythonPatchEffectDeclaration{
				Owner: owner,
				Name:  name,
				Kind:  "class",
				Line:  i + 1,
				Key:   owner + "|class|" + name,
			})
			scopes = append(scopes, pythonPatchEffectScope{Indent: indent, Name: name})
			continue
		}
		if match := pythonDefLineRE.FindStringSubmatch(line); match != nil {
			owner := pythonPatchEffectOwner(scopes)
			name := strings.TrimSpace(match[2])
			out = append(out, pythonPatchEffectDeclaration{
				Owner: owner,
				Name:  name,
				Kind:  "function",
				Line:  i + 1,
				Key:   owner + "|function|" + name,
			})
			scopes = append(scopes, pythonPatchEffectScope{Indent: indent, Name: name})
		}
	}
	return out
}

func pythonPatchEffectOwner(scopes []pythonPatchEffectScope) string {
	if len(scopes) == 0 {
		return "<module>"
	}
	names := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		if scope.Name != "" {
			names = append(names, scope.Name)
		}
	}
	if len(names) == 0 {
		return "<module>"
	}
	return strings.Join(names, ".")
}

func pythonPatchEffectIndent(line string) int {
	indent := 0
	for _, r := range line {
		switch r {
		case ' ':
			indent++
		case '\t':
			indent += 4
		default:
			return indent
		}
	}
	return indent
}

func appendPatchEffectOwnerBoundaryEvents(file *types.PatchEffectFile, provider *patchEffectSourceProvider, hunk types.PatchEffectHunk) {
	if file == nil || provider == nil {
		return
	}
	rules := provider.OwnerBoundary
	if rules.ReturnWrapper {
		appendPatchEffectReturnWrapperEvent(file, provider, hunk)
	}
	if rules.DiagnosticCall != nil {
		appendPatchEffectGuardedDiagnosticEvent(file, provider, hunk)
	}
	if rules.ExternalPrivateAssignment != nil {
		appendPatchEffectExternalPrivateAssignmentEvent(file, provider, hunk)
	}
}

func appendPatchEffectReturnWrapperEvent(file *types.PatchEffectFile, provider *patchEffectSourceProvider, hunk types.PatchEffectHunk) {
	for _, removed := range hunk.RemovedLineTexts {
		oldExpr, ok := patchEffectReturnExpression(removed.Text)
		if !ok || !strings.Contains(oldExpr, "(") {
			continue
		}
		for _, added := range hunk.AddedLineTexts {
			newExpr, ok := patchEffectReturnExpression(added.Text)
			if !ok || !patchEffectExpressionWraps(newExpr, oldExpr) {
				continue
			}
			appendPatchEffectProviderWarning(file, "caller_return_shape_adapter_added",
				fmt.Sprintf("added %s return wraps an existing returned call; verify the owner boundary instead of only adapting the caller shape", provider.Kind),
				added)
			return
		}
	}
}

func appendPatchEffectGuardedDiagnosticEvent(file *types.PatchEffectFile, provider *patchEffectSourceProvider, hunk types.PatchEffectHunk) {
	if provider.OwnerBoundary.DiagnosticCall == nil || !patchEffectLinesMatch(hunk.RemovedLineTexts, provider.OwnerBoundary.DiagnosticCall) {
		return
	}
	for idx, added := range hunk.AddedLineTexts {
		if !provider.OwnerBoundary.DiagnosticCall.MatchString(added.Text) {
			continue
		}
		if !patchEffectPriorAddedGuard(hunk.AddedLineTexts, idx) {
			continue
		}
		appendPatchEffectProviderWarning(file, "diagnostic_signal_conditionally_suppressed",
			fmt.Sprintf("added %s diagnostic call is guarded where the previous diagnostic was unguarded; verify the diagnostic semantics rather than suppressing the symptom", provider.Kind),
			added)
		return
	}
}

func appendPatchEffectExternalPrivateAssignmentEvent(file *types.PatchEffectFile, provider *patchEffectSourceProvider, hunk types.PatchEffectHunk) {
	rule := provider.OwnerBoundary.ExternalPrivateAssignment
	if rule == nil {
		return
	}
	for _, added := range hunk.AddedLineTexts {
		match := rule.FindStringSubmatch(added.Text)
		if len(match) < 2 {
			continue
		}
		receiver := strings.TrimSpace(match[1])
		if patchEffectReceiverIsInternal(receiver, provider.OwnerBoundary.InternalReceivers) {
			continue
		}
		appendPatchEffectProviderWarning(file, "external_private_state_sync_workaround",
			fmt.Sprintf("added %s code writes private state on external receiver %q; verify the owner API or callback boundary instead of synchronizing private state", provider.Kind, receiver),
			added)
		return
	}
}

func patchEffectReturnExpression(line string) (string, bool) {
	match := sourceReturnStatementRE.FindStringSubmatch(strings.TrimSpace(line))
	if len(match) < 2 {
		return "", false
	}
	expr := strings.TrimSpace(match[1])
	expr = strings.TrimSuffix(expr, ";")
	expr = strings.TrimSpace(expr)
	return expr, expr != ""
}

func patchEffectExpressionWraps(newExpr, oldExpr string) bool {
	newNorm := patchEffectCompactExpression(newExpr)
	oldNorm := patchEffectCompactExpression(oldExpr)
	if newNorm == "" || oldNorm == "" || newNorm == oldNorm {
		return false
	}
	idx := strings.Index(newNorm, oldNorm)
	if idx <= 0 {
		return false
	}
	prefix := newNorm[:idx]
	suffix := newNorm[idx+len(oldNorm):]
	if prefix == "(" || !strings.Contains(prefix, "(") || suffix == "" {
		return false
	}
	return strings.HasPrefix(suffix, ")") || strings.HasPrefix(suffix, ",")
}

func patchEffectCompactExpression(expr string) string {
	expr = strings.TrimSpace(expr)
	expr = strings.TrimSuffix(expr, ";")
	var b strings.Builder
	for _, r := range expr {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func patchEffectLinesMatch(lines []types.PatchEffectLine, re *regexp.Regexp) bool {
	if re == nil {
		return false
	}
	for _, line := range lines {
		if re.MatchString(line.Text) {
			return true
		}
	}
	return false
}

func patchEffectPriorAddedGuard(lines []types.PatchEffectLine, idx int) bool {
	for i := 0; i < idx && i < len(lines); i++ {
		if sourceConditionalGuardLineRE.MatchString(lines[i].Text) {
			return true
		}
	}
	return false
}

func patchEffectReceiverIsInternal(receiver string, internal []string) bool {
	receiver = strings.TrimSpace(receiver)
	if receiver == "" {
		return true
	}
	for _, candidate := range internal {
		if receiver == candidate {
			return true
		}
	}
	return false
}

func appendPatchEffectProviderWarning(file *types.PatchEffectFile, code, message string, line types.PatchEffectLine) {
	if file == nil {
		return
	}
	evidence := file.Path
	if line.Line > 0 {
		evidence = fmt.Sprintf("%s:%d", file.Path, line.Line)
	}
	file.Events = append(file.Events, types.PatchEffectEvent{
		Code:        code,
		Severity:    "warning",
		Path:        file.Path,
		Message:     message,
		EvidenceRef: evidence,
	})
}

func appendPatchEffectLineShapeEvents(file *types.PatchEffectFile, provider *patchEffectSourceProvider, line types.PatchEffectLine) {
	if provider == nil {
		return
	}
	for _, rule := range provider.LineRules {
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

func appendPatchEffectProductionTestScaffoldEvent(file *types.PatchEffectFile, provider *patchEffectSourceProvider, line types.PatchEffectLine) {
	if file == nil || file.PathRole != types.SourcePathRoleProduction {
		return
	}
	if provider == nil {
		return
	}
	rule := provider.ProductionTestRule
	if rule.match == nil || !rule.match.MatchString(line.Text) {
		return
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

func patchEffectSourceProviderForPath(path string) *patchEffectSourceProvider {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return nil
	}
	for i := range patchEffectSourceProviders {
		provider := &patchEffectSourceProviders[i]
		for _, candidate := range provider.Extensions {
			if strings.ToLower(strings.TrimSpace(candidate)) == ext {
				return provider
			}
		}
	}
	return nil
}

func patchEffectSourceShapeKind(path string) string {
	if provider := patchEffectSourceProviderForPath(path); provider != nil {
		return provider.Kind
	}
	return ""
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
