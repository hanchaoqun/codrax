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
	pythonTextExecutableStatementRE     = regexp.MustCompile(`^\s*(?:if|elif|else|for|while|try|except|finally|with|def|class|return|raise|assert|yield|import|from)\b`)
	pythonDocSectionUnderlineRE         = regexp.MustCompile(`^\s*[-=~^]{3,}\s*$`)
	dynamicNestedStringKeyAccessRE      = regexp.MustCompile(`(?:\b(?:self|cls|this)\.[A-Za-z_$][A-Za-z0-9_$]*|\b[A-Za-z_$][A-Za-z0-9_$]*)(?:\s*\[\s*['"][^'"]+['"]\s*\]){2,}`)
	rubyNestedSymbolOrStringKeyAccessRE = regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*(?:\s*\[\s*(?::[A-Za-z_][A-Za-z0-9_]*|['"][^'"]+['"])\s*\]){2,}`)
	jvmChainedStringMapGetRE            = regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*(?:\.get\(\s*"[^"]+"\s*\)){2,}`)
	goNestedStringMapAssignmentRE       = regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*(?:\s*\[\s*"[^"]+"\s*\]){2,}\s*=(?:[^=]|$)`)
	pythonNestedCollectionShapeCheckRE  = regexp.MustCompile(`\bisinstance\s*\([^,\n]+,\s*(?:\([^)]*\b(?:list|tuple|set|dict)\b[^)]*\)|(?:list|tuple|set|dict))\s*\)`)
	jsNestedCollectionShapeCheckRE      = regexp.MustCompile(`\b(?:Array\.isArray\s*\(|[A-Za-z_$][A-Za-z0-9_$]*\s+instanceof\s+(?:Array|Map|Set)\b)`)
	rubyNestedCollectionShapeCheckRE    = regexp.MustCompile(`\b(?:is_a\?|kind_of\?|instance_of\?)\s*\(?\s*(?:Array|Hash)\b`)
	jvmNestedCollectionShapeCheckRE     = regexp.MustCompile(`\b(?:instanceof\s+(?:List|Map|Set|Collection|Iterable)\b|is\s+(?:List|Map|Set|Collection|Iterable)\b)`)
	goNestedCollectionShapeCheckRE      = regexp.MustCompile(`(?:\.\(\s*(?:\[\]|map\[)|^\s*case\s+(?:\[\]|map\[))`)
	pythonBranchExclusionActionRE       = regexp.MustCompile(`\b(?:continue|break)\b|^\s*return\s+(?:None|False|\[\]|\{\})?\s*$|\bis_[A-Za-z0-9_]*\s*=\s*False\b|\bskip[A-Za-z0-9_]*\s*=\s*True\b`)
	jsBranchExclusionActionRE           = regexp.MustCompile(`\b(?:continue|break)\b|\breturn\s+(?:null|undefined|false|\[\]|\{\})\s*;?|\bis[A-Za-z0-9_]*\s*=\s*false\b|\bskip[A-Za-z0-9_]*\s*=\s*true\b`)
	rubyBranchExclusionActionRE         = regexp.MustCompile(`\b(?:next|break)\b|\breturn\s+(?:nil|false|\[\]|\{\})\b|\bis_[A-Za-z0-9_]*\s*=\s*false\b|\bskip[A-Za-z0-9_]*\s*=\s*true\b`)
	jvmBranchExclusionActionRE          = regexp.MustCompile(`\b(?:continue|break)\b|\breturn\s+(?:null|false|Collections\.emptyList\(\)|Optional\.empty\(\))\s*;?|\bis[A-Za-z0-9_]*\s*=\s*false\b|\bskip[A-Za-z0-9_]*\s*=\s*true\b`)
	goBranchExclusionActionRE           = regexp.MustCompile(`\b(?:continue|break)\b|\breturn\s+(?:nil|false)\b|\bis[A-Za-z0-9_]*\s*:=\s*false\b|\bskip[A-Za-z0-9_]*\s*:=\s*true\b`)
	sourceValidationSignalRE            = regexp.MustCompile(`(?i)\b(?:validat|check|error|exception|raise|throw|assert|diagnostic|warn|warning|max|min|len|length|size|count)\b`)
	pythonTestScaffoldDeclarationRE     = regexp.MustCompile(`^\s*(?:class\s+[A-Za-z_][A-Za-z0-9_]*(?:Test|Tests)\b|def\s+test_[A-Za-z0-9_]*\s*\()`)
	jvmTestScaffoldDeclarationRE        = regexp.MustCompile(`^\s*(?:(?:public|private|protected|internal|open|final|abstract|static)\s+)*class\s+[A-Za-z_][A-Za-z0-9_]*(?:Test|Tests)\b`)
	goTestScaffoldDeclarationRE         = regexp.MustCompile(`^\s*func\s+Test[A-Z0-9_][A-Za-z0-9_]*\s*\(`)
	jsTestScaffoldDeclarationRE         = regexp.MustCompile(`^\s*(?:export\s+)?(?:class|function)\s+[A-Za-z_$][A-Za-z0-9_$]*(?:Test|Tests|Spec)\b`)
	rubyTestScaffoldDeclarationRE       = regexp.MustCompile(`^\s*(?:class\s+[A-Za-z_:][A-Za-z0-9_:]*(?:Test|Tests)\b|def\s+test_[A-Za-z0-9_!?=]*\b)`)
	sourceReturnStatementRE             = regexp.MustCompile(`^\s*return\s+(.+?)\s*;?\s*$`)
	sourceConditionalGuardLineRE        = regexp.MustCompile(`^\s*(?:if|elif|else\s+if|unless)\b`)
	braceReturnStatementRE              = regexp.MustCompile(`^\s*return\b.*;?\s*$`)
	pythonDiagnosticCallRE              = regexp.MustCompile(`\b(?:warnings\.)?(?:warn|warning)\s*\(`)
	jsDiagnosticCallRE                  = regexp.MustCompile(`\b(?:console|logger|log)\.(?:warn|warning|error)\s*\(`)
	jvmDiagnosticCallRE                 = regexp.MustCompile(`\b(?:logger|log)\.(?:warn|warning|error)\s*\(`)
	rubyDiagnosticCallRE                = regexp.MustCompile(`\b(?:warn|warning|logger\.(?:warn|warning|error))\s*(?:\(|\s)`)
	externalUnderscoreFieldAssignRE     = regexp.MustCompile(`\b([A-Za-z_$][A-Za-z0-9_$]*)\._[A-Za-z_$][A-Za-z0-9_$]*\s*=`)
	rubyExternalPrivateStateAssignRE    = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\.instance_variable_set\s*\(\s*:@[A-Za-z_][A-Za-z0-9_]*`)
	sourceAssignmentStatementRE         = regexp.MustCompile(`^(?:[A-Za-z_$][A-Za-z0-9_$]*|\([A-Za-z_$][A-Za-z0-9_$]*(?:\s*,\s*[A-Za-z_$][A-Za-z0-9_$]*)*\)|[A-Za-z_$][A-Za-z0-9_$]*(?:\s*,\s*[A-Za-z_$][A-Za-z0-9_$]*)+)\s*(?::=|=)\s*[^=].+$`)
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
	CollectionBoundary patchEffectNestedCollectionBoundaryRules
	AnnotateFile       func(file *types.PatchEffectFile, lines []string)
	AnnotateHunk       func(file *types.PatchEffectFile, lines []string, hunk types.PatchEffectHunk)
}

type patchEffectOwnerBoundaryRules struct {
	ReturnWrapper             bool
	DiagnosticCall            *regexp.Regexp
	ExternalPrivateAssignment *regexp.Regexp
	InternalReceivers         []string
}

type patchEffectNestedCollectionBoundaryRules struct {
	ShapeCheck       *regexp.Regexp
	ExclusionAction  *regexp.Regexp
	ValidationSignal *regexp.Regexp
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
		CollectionBoundary: patchEffectNestedCollectionBoundaryRules{
			ShapeCheck:       pythonNestedCollectionShapeCheckRE,
			ExclusionAction:  pythonBranchExclusionActionRE,
			ValidationSignal: sourceValidationSignalRE,
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
		CollectionBoundary: patchEffectNestedCollectionBoundaryRules{
			ShapeCheck:       jsNestedCollectionShapeCheckRE,
			ExclusionAction:  jsBranchExclusionActionRE,
			ValidationSignal: sourceValidationSignalRE,
		},
		AnnotateHunk: annotatePatchEffectBraceReturnShape,
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
		CollectionBoundary: patchEffectNestedCollectionBoundaryRules{
			ShapeCheck:       jsNestedCollectionShapeCheckRE,
			ExclusionAction:  jsBranchExclusionActionRE,
			ValidationSignal: sourceValidationSignalRE,
		},
		AnnotateHunk: annotatePatchEffectBraceReturnShape,
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
		CollectionBoundary: patchEffectNestedCollectionBoundaryRules{
			ShapeCheck:       rubyNestedCollectionShapeCheckRE,
			ExclusionAction:  rubyBranchExclusionActionRE,
			ValidationSignal: sourceValidationSignalRE,
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
		CollectionBoundary: patchEffectNestedCollectionBoundaryRules{
			ShapeCheck:       jvmNestedCollectionShapeCheckRE,
			ExclusionAction:  jvmBranchExclusionActionRE,
			ValidationSignal: sourceValidationSignalRE,
		},
		AnnotateHunk: annotatePatchEffectBraceReturnShape,
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
		CollectionBoundary: patchEffectNestedCollectionBoundaryRules{
			ShapeCheck:       jvmNestedCollectionShapeCheckRE,
			ExclusionAction:  jvmBranchExclusionActionRE,
			ValidationSignal: sourceValidationSignalRE,
		},
		AnnotateHunk: annotatePatchEffectBraceReturnShape,
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
		CollectionBoundary: patchEffectNestedCollectionBoundaryRules{
			ShapeCheck:       goNestedCollectionShapeCheckRE,
			ExclusionAction:  goBranchExclusionActionRE,
			ValidationSignal: sourceValidationSignalRE,
		},
		AnnotateHunk: annotatePatchEffectBraceReturnShape,
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
		appendPatchEffectDuplicateInsertedBlockEvent(file, hunk)
		if provider.AnnotateHunk != nil {
			provider.AnnotateHunk(file, lines, hunk)
		}
		appendPatchEffectOwnerBoundaryEvents(file, provider, hunk)
		appendPatchEffectNestedCollectionExclusionEvent(file, provider, hunk)
		for _, added := range hunk.AddedLineTexts {
			appendPatchEffectNonASCIISourceCommentEvent(file, added)
			appendPatchEffectNearbyDuplicateStatementEvent(file, lines, hunk, added)
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
		if pythonTopLevelSelfMethodDefRE.MatchString(line) {
			file.Events = append(file.Events, types.PatchEffectEvent{
				Code:        "python_top_level_self_method",
				Severity:    "warning",
				Path:        file.Path,
				Message:     "added top-level Python function uses self/cls as its first parameter; this usually means a class method was de-indented out of its owner class",
				EvidenceRef: fmt.Sprintf("%s:%d", file.Path, lineNo),
			})
		}
		appendPatchEffectPythonUnreachableAfterAddedReturn(file, lines, lineNo)
	}
	appendPatchEffectPythonDocstringSectionExecutableEvent(file, lines, hunk)
}

func appendPatchEffectPythonDocstringSectionExecutableEvent(file *types.PatchEffectFile, lines []string, hunk types.PatchEffectHunk) {
	if file == nil || file.PathRole != types.SourcePathRoleProduction || len(lines) == 0 {
		return
	}
	docLines := pythonPatchEffectTripleQuotedLineSet(lines)
	if len(docLines) == 0 {
		return
	}
	added := map[int]bool{}
	for _, lineNo := range hunk.AddedLineNumbers {
		if lineNo > 0 {
			added[lineNo] = true
		}
	}
	for _, addedLine := range hunk.AddedLineTexts {
		lineNo := addedLine.Line
		if lineNo <= 0 || !docLines[lineNo] || !pythonTextExecutableStatementRE.MatchString(addedLine.Text) {
			continue
		}
		if !pythonPatchEffectDocstringSectionDisrupted(lines, docLines, added, lineNo) {
			continue
		}
		file.Events = append(file.Events, types.PatchEffectEvent{
			Code:        "python_docstring_section_executable_added",
			Severity:    "warning",
			Path:        file.Path,
			Message:     "added executable-looking Python statement inside a docstring section header region; move behavior changes into executable source code",
			EvidenceRef: fmt.Sprintf("%s:%d", file.Path, lineNo),
		})
		return
	}
}

func pythonPatchEffectDocstringSectionDisrupted(lines []string, docLines map[int]bool, added map[int]bool, lineNo int) bool {
	next := pythonPatchEffectNextNonAddedDocstringLine(lines, docLines, added, lineNo+1, 8)
	if next <= 0 || !pythonDocSectionUnderlineRE.MatchString(lines[next-1]) {
		return false
	}
	prev := pythonPatchEffectPrevNonAddedDocstringLine(lines, docLines, added, lineNo-1, 8)
	if prev <= 0 {
		return false
	}
	prevText := strings.TrimSpace(lines[prev-1])
	if prevText == "" || pythonDocSectionUnderlineRE.MatchString(prevText) ||
		strings.HasSuffix(prevText, "::") ||
		strings.HasPrefix(prevText, ">>>") ||
		strings.HasPrefix(prevText, "...") {
		return false
	}
	return true
}

func pythonPatchEffectNextNonAddedDocstringLine(lines []string, docLines map[int]bool, added map[int]bool, start, limit int) int {
	seen := 0
	for lineNo := start; lineNo <= len(lines) && seen < limit; lineNo++ {
		if !docLines[lineNo] {
			return 0
		}
		seen++
		if added[lineNo] || strings.TrimSpace(lines[lineNo-1]) == "" {
			continue
		}
		return lineNo
	}
	return 0
}

func pythonPatchEffectPrevNonAddedDocstringLine(lines []string, docLines map[int]bool, added map[int]bool, start, limit int) int {
	seen := 0
	for lineNo := start; lineNo >= 1 && seen < limit; lineNo-- {
		if !docLines[lineNo] {
			return 0
		}
		seen++
		if added[lineNo] || strings.TrimSpace(lines[lineNo-1]) == "" {
			continue
		}
		return lineNo
	}
	return 0
}

func pythonPatchEffectTripleQuotedLineSet(lines []string) map[int]bool {
	out := map[int]bool{}
	inString := false
	delim := ""
	for i, line := range lines {
		lineNo := i + 1
		if inString {
			out[lineNo] = true
			if strings.Contains(line, delim) {
				inString = false
				delim = ""
			}
			continue
		}
		start, marker := pythonPatchEffectFirstTripleQuote(line)
		if start < 0 {
			continue
		}
		if strings.Contains(line[start+len(marker):], marker) {
			continue
		}
		out[lineNo] = true
		inString = true
		delim = marker
	}
	return out
}

func pythonPatchEffectFirstTripleQuote(line string) (int, string) {
	doubleIdx := strings.Index(line, `"""`)
	singleIdx := strings.Index(line, `'''`)
	switch {
	case doubleIdx < 0 && singleIdx < 0:
		return -1, ""
	case doubleIdx >= 0 && (singleIdx < 0 || doubleIdx < singleIdx):
		return doubleIdx, `"""`
	default:
		return singleIdx, `'''`
	}
}

func appendPatchEffectPythonUnreachableAfterAddedReturn(file *types.PatchEffectFile, lines []string, lineNo int) {
	if file == nil || lineNo <= 0 || lineNo > len(lines) {
		return
	}
	line := lines[lineNo-1]
	if _, ok := patchEffectReturnExpression(line); !ok {
		return
	}
	lineIndent := pythonPatchEffectIndent(line)
	defIndent, bodyIndent, ok := pythonPatchEffectFunctionBodyIndent(lines, lineNo, lineIndent)
	if !ok || lineIndent != bodyIndent {
		return
	}
	for idx := lineNo; idx < len(lines); idx++ {
		next := lines[idx]
		trimmed := strings.TrimSpace(next)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		nextIndent := pythonPatchEffectIndent(next)
		if nextIndent <= defIndent {
			return
		}
		if nextIndent >= bodyIndent {
			file.Events = append(file.Events, types.PatchEffectEvent{
				Code:        "python_unreachable_body_after_added_return",
				Severity:    "warning",
				Path:        file.Path,
				Message:     "added Python function-body return leaves later statements in the same function unreachable; remove the stale body or place the return under a narrower guard",
				EvidenceRef: fmt.Sprintf("%s:%d", file.Path, lineNo),
			})
			return
		}
	}
}

func annotatePatchEffectBraceReturnShape(file *types.PatchEffectFile, lines []string, hunk types.PatchEffectHunk) {
	for _, lineNo := range hunk.AddedLineNumbers {
		appendPatchEffectBraceReturnBeforeExistingStatementEvent(file, lines, lineNo)
	}
}

func appendPatchEffectBraceReturnBeforeExistingStatementEvent(file *types.PatchEffectFile, lines []string, lineNo int) {
	if file == nil || lineNo <= 0 || lineNo > len(lines) {
		return
	}
	line := lines[lineNo-1]
	if !braceReturnStatementRE.MatchString(line) {
		return
	}
	lineDepth := patchEffectBraceDepthBefore(lines, lineNo-1)
	if lineDepth <= 0 {
		return
	}
	for idx := lineNo; idx < len(lines); idx++ {
		next := strings.TrimSpace(lines[idx])
		if next == "" || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/*") || strings.HasPrefix(next, "*") {
			continue
		}
		nextDepth := patchEffectBraceDepthBefore(lines, idx)
		if nextDepth < lineDepth || strings.HasPrefix(next, "}") {
			return
		}
		file.Events = append(file.Events, types.PatchEffectEvent{
			Code:        "brace_return_before_existing_statement_added",
			Severity:    "warning",
			Path:        file.Path,
			Message:     "added return in a brace-delimited block leaves later statements in the same block; verify the control-flow change or remove stale code",
			EvidenceRef: fmt.Sprintf("%s:%d", file.Path, lineNo),
		})
		return
	}
}

func patchEffectBraceDepthBefore(lines []string, lineIndex int) int {
	depth := 0
	for i := 0; i < lineIndex && i < len(lines); i++ {
		for _, r := range lines[i] {
			switch r {
			case '{':
				depth++
			case '}':
				depth--
				if depth < 0 {
					depth = 0
				}
			}
		}
	}
	return depth
}

func pythonPatchEffectFunctionBodyIndent(lines []string, lineNo, lineIndent int) (int, int, bool) {
	for idx := lineNo - 2; idx >= 0; idx-- {
		line := lines[idx]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := pythonPatchEffectIndent(line)
		if indent >= lineIndent {
			continue
		}
		if !pythonDefLineRE.MatchString(line) {
			continue
		}
		bodyIndent := lineIndent
		for bodyIdx := idx + 1; bodyIdx < lineNo; bodyIdx++ {
			bodyLine := lines[bodyIdx]
			bodyTrimmed := strings.TrimSpace(bodyLine)
			if bodyTrimmed == "" || strings.HasPrefix(bodyTrimmed, "#") {
				continue
			}
			bodyLineIndent := pythonPatchEffectIndent(bodyLine)
			if bodyLineIndent > indent {
				bodyIndent = bodyLineIndent
				break
			}
		}
		if lineIndent < bodyIndent {
			return 0, 0, false
		}
		return indent, bodyIndent, true
	}
	return 0, 0, false
}

func appendPatchEffectDuplicateInsertedBlockEvent(file *types.PatchEffectFile, hunk types.PatchEffectHunk) {
	if file == nil || len(hunk.AddedLineTexts) < 6 {
		return
	}
	lines := make([]string, 0, len(hunk.AddedLineTexts))
	original := make([]types.PatchEffectLine, 0, len(hunk.AddedLineTexts))
	for _, line := range hunk.AddedLineTexts {
		normalized := patchEffectDuplicateBlockLine(line.Text)
		if normalized == "" {
			continue
		}
		lines = append(lines, normalized)
		original = append(original, line)
	}
	if len(lines) < 6 {
		return
	}
	maxWindow := len(lines) / 2
	for size := maxWindow; size >= 3; size-- {
		for start := 0; start+2*size <= len(lines); start++ {
			if !patchEffectDuplicateBlockHasCode(lines[start : start+size]) {
				continue
			}
			if !patchEffectStringSlicesEqual(lines[start:start+size], lines[start+size:start+2*size]) {
				continue
			}
			line := original[start+size]
			evidence := file.Path
			if line.Line > 0 {
				evidence = fmt.Sprintf("%s:%d", file.Path, line.Line)
			}
			file.Events = append(file.Events, types.PatchEffectEvent{
				Code:        "duplicate_inserted_block_added",
				Severity:    "warning",
				Path:        file.Path,
				Message:     fmt.Sprintf("added adjacent duplicate code block of %d nonblank lines; remove the repeated block or prove the duplication is intentional", size),
				EvidenceRef: evidence,
			})
			return
		}
	}
}

func patchEffectDuplicateBlockLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}
	return trimmed
}

func patchEffectDuplicateBlockHasCode(lines []string) bool {
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") ||
			strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
			continue
		}
		for _, r := range trimmed {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '_' {
				return true
			}
		}
	}
	return false
}

func patchEffectStringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
			Severity:    "warning",
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

func appendPatchEffectNestedCollectionExclusionEvent(file *types.PatchEffectFile, provider *patchEffectSourceProvider, hunk types.PatchEffectHunk) {
	if file == nil || provider == nil {
		return
	}
	rules := provider.CollectionBoundary
	if rules.ShapeCheck == nil || rules.ExclusionAction == nil {
		return
	}
	validationSignal := rules.ValidationSignal
	if validationSignal == nil {
		validationSignal = sourceValidationSignalRE
	}
	if !patchEffectHunkHasValidationSignal(hunk, validationSignal) {
		return
	}
	shapeIndexes := make([]int, 0, 1)
	for idx, added := range hunk.AddedLineTexts {
		if patchEffectSourceLineMatches(added.Text, rules.ShapeCheck) {
			shapeIndexes = append(shapeIndexes, idx)
		}
	}
	if len(shapeIndexes) == 0 {
		return
	}
	for _, shapeIdx := range shapeIndexes {
		for idx := shapeIdx; idx < len(hunk.AddedLineTexts) && idx <= shapeIdx+6; idx++ {
			added := hunk.AddedLineTexts[idx]
			if !patchEffectSourceLineMatches(added.Text, rules.ExclusionAction) {
				continue
			}
			appendPatchEffectProviderWarning(file, "nested_collection_branch_exclusion_added",
				fmt.Sprintf("added %s code detects a nested collection and excludes that branch from nearby validation or handling; verify nested collection semantics with targeted coverage instead of only flat inputs", provider.Kind),
				added)
			return
		}
	}
}

func patchEffectHunkHasValidationSignal(hunk types.PatchEffectHunk, re *regexp.Regexp) bool {
	if re == nil {
		return false
	}
	for _, added := range hunk.AddedLineTexts {
		if patchEffectSourceLineMatches(added.Text, re) {
			return true
		}
	}
	return false
}

func patchEffectSourceLineMatches(line string, re *regexp.Regexp) bool {
	if re == nil {
		return false
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") ||
		strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") ||
		strings.HasPrefix(trimmed, "*") {
		return false
	}
	return re.MatchString(line)
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

func appendPatchEffectNonASCIISourceCommentEvent(file *types.PatchEffectFile, line types.PatchEffectLine) {
	if file == nil || file.PathRole != types.SourcePathRoleProduction {
		return
	}
	if !patchEffectLineIsCommentOnly(line.Text) || !patchEffectLineHasNonASCII(line.Text) {
		return
	}
	appendPatchEffectProviderWarning(
		file,
		"non_ascii_source_comment_added",
		"added production-source comment contains non-ASCII text; verify this matches the repository's source-comment convention",
		line,
	)
}

func appendPatchEffectNearbyDuplicateStatementEvent(file *types.PatchEffectFile, lines []string, hunk types.PatchEffectHunk, line types.PatchEffectLine) {
	if file == nil || file.PathRole != types.SourcePathRoleProduction || line.Line <= 0 || line.Line > len(lines) {
		return
	}
	stmt := patchEffectNormalizedAssignmentStatement(line.Text)
	if stmt == "" {
		return
	}
	added := make(map[int]bool, len(hunk.AddedLineNumbers))
	for _, lineNo := range hunk.AddedLineNumbers {
		if lineNo > 0 {
			added[lineNo] = true
		}
	}
	const nearbyWindow = 8
	start := line.Line - nearbyWindow
	if start < 1 {
		start = 1
	}
	for priorLineNo := line.Line - 1; priorLineNo >= start; priorLineNo-- {
		if added[priorLineNo] || priorLineNo <= 0 || priorLineNo > len(lines) {
			continue
		}
		if patchEffectNormalizedAssignmentStatement(lines[priorLineNo-1]) != stmt {
			continue
		}
		appendPatchEffectProviderWarning(
			file,
			"nearby_duplicate_statement_added",
			"added production-source assignment duplicates a nearby existing assignment; verify the duplicate statement is intentional or remove it",
			line,
		)
		return
	}
}

func patchEffectLineHasNonASCII(text string) bool {
	for _, r := range text {
		if r > 127 {
			return true
		}
	}
	return false
}

func patchEffectLineIsCommentOnly(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	for _, prefix := range []string{"#", "//", "/*", "*", "*/", "<!--", "-->", `"""`, `'''`} {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

func patchEffectNormalizedAssignmentStatement(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || patchEffectLineIsCommentOnly(trimmed) {
		return ""
	}
	if idx := strings.Index(trimmed, "#"); idx >= 0 {
		trimmed = strings.TrimSpace(trimmed[:idx])
	}
	trimmed = strings.TrimSuffix(trimmed, ";")
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" || strings.HasSuffix(trimmed, ":") {
		return ""
	}
	for _, op := range []string{"==", "!=", "<=", ">=", "=>", "=<"} {
		if strings.Contains(trimmed, op) {
			return ""
		}
	}
	if !sourceAssignmentStatementRE.MatchString(trimmed) {
		return ""
	}
	return trimmed
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
