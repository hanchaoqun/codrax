// Package stageauthority exposes checkout-verified read-mode stage facts to
// prompt and validation consumers through one typed provider.  It deliberately
// does not author an answer or a diagram: consumers may teach or validate the
// three exact adjacent precedence relations, while calls, data flow, runtime
// causality, and extra participants still require their own authority.
package stageauthority

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// StageRow is one checkout-verified stage binding.  IdentityAliases contains
// only declaration-backed stage/agent identities; display prose is never an
// alias and therefore cannot mint relation authority.
type StageRow struct {
	StageIdent       string
	StageValue       string
	AgentIdent       string
	AgentValue       string
	Skill            string
	Responsibility   string
	PrimaryArtifacts []string
	Terminal         bool
	File             string
	Line             int
}

func (r StageRow) IdentityAliases() []string {
	return []string{r.StageIdent, r.StageValue, r.AgentIdent, r.AgentValue}
}

// PrecedenceRelation is one adjacent edge in AllMainStages.  It proves only
// order in the canonical read lane; it is not a call or artifact-flow edge.
type PrecedenceRelation struct {
	From       StageRow
	To         StageRow
	SourceFile string
	LineStart  int
	LineEnd    int
}

// ReadModeAuthority is the complete fail-closed provider result.
type ReadModeAuthority struct {
	Main                 []StageRow
	ConditionalPreStages []types.StageBinding
	Precedence           []PrecedenceRelation
}

// LoadReadMode verifies the checked-out source against the compiled canonical
// stage table and sequence.  A same-named customer repository or a stale
// binary/source pairing returns false instead of borrowing Codrax authority.
func LoadReadMode(repoRoot string) (ReadModeAuthority, bool) {
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		return ReadModeAuthority{}, false
	}
	bindingRel := types.ReadModePipelineStageBindingFile
	bindingPath := filepath.Join(repoRoot, filepath.FromSlash(bindingRel))
	bindingData, err := os.ReadFile(bindingPath)
	if err != nil {
		return ReadModeAuthority{}, false
	}
	rows, ok := verifiedBindingRows(bindingPath, bindingRel, bindingData, types.ReadModeMainStageBindings())
	if !ok || len(rows) == 0 || !declaresConditionalPreStages(bindingData, []string{"StageLogTriage", "StagePerfTriage"}) {
		return ReadModeAuthority{}, false
	}

	sequenceRel := types.ReadModePipelineEnumsFile
	sequencePath := filepath.Join(repoRoot, filepath.FromSlash(sequenceRel))
	sequenceData, err := os.ReadFile(sequencePath)
	if err != nil {
		return ReadModeAuthority{}, false
	}
	sequenceLines, ok := verifiedMainStageSequence(sequencePath, sequenceData, rows)
	if !ok || len(sequenceLines) != len(rows) {
		return ReadModeAuthority{}, false
	}

	relations := make([]PrecedenceRelation, 0, len(rows)-1)
	for i := 0; i+1 < len(rows); i++ {
		relations = append(relations, PrecedenceRelation{
			From: rows[i], To: rows[i+1], SourceFile: sequenceRel,
			LineStart: sequenceLines[i], LineEnd: sequenceLines[i+1],
		})
	}
	return ReadModeAuthority{
		Main:                 rows,
		ConditionalPreStages: types.ReadModeConditionalPreStageBindings(),
		Precedence:           relations,
	}, true
}

// BindingIdentifiers returns the exact declaration identifiers for a
// canonical main-stage binding.
func BindingIdentifiers(binding types.StageBinding) (stageIdent string, agentIdent string, ok bool) {
	switch binding.Stage {
	case types.StageAnalyze:
		return "StageAnalyze", "AgentAnalyzer", true
	case types.StageExplore:
		return "StageExplore", "AgentExplorer", true
	case types.StageExtract:
		return "StageExtract", "AgentExtractor", true
	case types.StageFinalize:
		return "StageFinalize", "AgentFinalizer", true
	default:
		return "", "", false
	}
}

func verifiedBindingRows(path, rel string, data []byte, bindings []types.StageBinding) ([]StageRow, bool) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, data, 0)
	if err != nil {
		return nil, false
	}
	rows := make([]StageRow, 0, len(bindings))
	for _, binding := range bindings {
		stageIdent, agentIdent, ok := BindingIdentifiers(binding)
		if !ok {
			return nil, false
		}
		want := StageRow{
			StageIdent: stageIdent, StageValue: string(binding.Stage),
			AgentIdent: agentIdent, AgentValue: string(binding.Agent),
			Skill: binding.Skill, Responsibility: binding.Responsibility,
			PrimaryArtifacts: append([]string(nil), binding.PrimaryArtifacts...),
			Terminal:         binding.Terminal, File: rel,
		}
		want.Line = verifiedBindingLine(file, fset, want)
		if want.Line <= 0 {
			return nil, false
		}
		rows = append(rows, want)
	}
	return rows, true
}

func verifiedBindingLine(file *ast.File, fset *token.FileSet, want StageRow) int {
	if file == nil || fset == nil {
		return 0
	}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok || len(value.Names) != 1 || value.Names[0].Name != "builtinStageBindings" || len(value.Values) != 1 {
				continue
			}
			list, ok := value.Values[0].(*ast.CompositeLit)
			if !ok {
				return 0
			}
			for _, element := range list.Elts {
				entry, ok := element.(*ast.CompositeLit)
				if ok && bindingEntryMatches(entry, want) {
					return fset.Position(entry.Pos()).Line
				}
			}
			return 0
		}
	}
	return 0
}

func bindingEntryMatches(entry *ast.CompositeLit, want StageRow) bool {
	fields := make(map[string]ast.Expr, len(entry.Elts))
	for _, element := range entry.Elts {
		keyed, ok := element.(*ast.KeyValueExpr)
		if !ok {
			return false
		}
		key, ok := keyed.Key.(*ast.Ident)
		if !ok {
			return false
		}
		fields[key.Name] = keyed.Value
	}
	return identName(fields["Stage"]) == want.StageIdent &&
		identName(fields["Agent"]) == want.AgentIdent &&
		stringValue(fields["Skill"]) == want.Skill &&
		stringValue(fields["Responsibility"]) == want.Responsibility &&
		boolValue(fields["Terminal"]) == want.Terminal &&
		equalStrings(stringSliceValue(fields["PrimaryArtifacts"]), want.PrimaryArtifacts)
}

func verifiedMainStageSequence(path string, data []byte, rows []StageRow) ([]int, bool) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, data, 0)
	if err != nil {
		return nil, false
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Name.Name != "AllMainStages" || fn.Body == nil {
			continue
		}
		var lines []int
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			lit, ok := node.(*ast.CompositeLit)
			if !ok || !pipelineStageSliceType(lit.Type) || len(lit.Elts) != len(rows) {
				return true
			}
			candidate := make([]int, len(rows))
			for i, elt := range lit.Elts {
				ident, ok := elt.(*ast.Ident)
				if !ok || ident.Name != rows[i].StageIdent {
					return true
				}
				candidate[i] = fset.Position(elt.Pos()).Line
			}
			lines = candidate
			return false
		})
		return lines, len(lines) == len(rows)
	}
	return nil, false
}

func declaresConditionalPreStages(data []byte, want []string) bool {
	file, err := parser.ParseFile(token.NewFileSet(), "stage_binding.go", data, 0)
	if err != nil {
		return false
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Name.Name != "ReadModeConditionalPreStageBindings" || fn.Body == nil {
			continue
		}
		matched := false
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			lit, ok := node.(*ast.CompositeLit)
			if !ok || !pipelineStageSliceType(lit.Type) || len(lit.Elts) != len(want) {
				return true
			}
			for i, elt := range lit.Elts {
				ident, ok := elt.(*ast.Ident)
				if !ok || ident.Name != want[i] {
					return true
				}
			}
			matched = true
			return false
		})
		return matched
	}
	return false
}

func pipelineStageSliceType(expr ast.Expr) bool {
	array, ok := expr.(*ast.ArrayType)
	if !ok || array.Len != nil {
		return false
	}
	ident, ok := array.Elt.(*ast.Ident)
	return ok && ident.Name == "PipelineStage"
}

func identName(expr ast.Expr) string {
	ident, _ := expr.(*ast.Ident)
	if ident == nil {
		return ""
	}
	return ident.Name
}

func stringValue(expr ast.Expr) string {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return ""
	}
	return value
}

func boolValue(expr ast.Expr) bool { return identName(expr) == "true" }

func stringSliceValue(expr ast.Expr) []string {
	list, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list.Elts))
	for _, element := range list.Elts {
		value := stringValue(element)
		if value == "" {
			return nil
		}
		out = append(out, value)
	}
	return out
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
