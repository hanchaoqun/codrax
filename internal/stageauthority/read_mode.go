// Package stageauthority exposes checkout-verified read-mode stage facts to
// prompt and validation consumers through one typed provider.  It deliberately
// does not author an answer or a diagram: consumers may teach or validate the
// three exact adjacent precedence relations, while calls, data flow, runtime
// causality, and extra participants still require their own authority.
package stageauthority

import (
	"bytes"
	"go/ast"
	"go/format"
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

// StateCarrierField is one checkout-verified field ownership fact from the
// read pipeline's shared BusContext/MutableState carrier. It proves only that
// the named field and exact Go type exist on that owner. It does not prove
// that every stage reads/writes the field or authorize a diagram edge.
type StateCarrierField struct {
	Owner string
	Field string
	Type  string
	File  string
	Line  int
}

var readModeStateCarrierFieldSpecs = []StateCarrierField{
	{Owner: "BusContext", Field: "Mutable", Type: "*MutableState"},
	{Owner: "BusContext", Field: "PipelineStage", Type: "PipelineStage"},
	{Owner: "BusContext", Field: "ActiveAgent", Type: "AgentName"},
	{Owner: "BusContext", Field: "EvidenceItems", Type: "[]EvidenceItem"},
	{Owner: "BusContext", Field: "AnswerChains", Type: "[]AnswerChain"},
	{Owner: "BusContext", Field: "AnswerSymbols", Type: "[]AnswerSymbol"},
	{Owner: "BusContext", Field: "StageReports", Type: "[]StageReport"},
	{Owner: "BusContext", Field: "AnalysisIR", Type: "*AnalysisIR"},
	{Owner: "MutableState", Field: "answerDocumentV2", Type: "*AnswerDocumentV2"},
}

// WorkflowSelection is the checkout-verified contiguous read-mode stage span
// relevant to one typed request. It carries precedence only; calls, artifact
// transfer, shared-state connectivity, and runtime causality remain outside
// this authority.
type WorkflowSelection struct {
	Main       []StageRow
	Precedence []PrecedenceRelation
}

// MatchesRequiredMainStageParticipantSlate reports whether a schema-validated
// source-flow diagram asks for every canonical read-mode main stage as an
// incident participant. The participant slate is planning input only: this
// helper cannot create a relation. It merely decides whether the already
// checkout-verified precedence rows are relevant to the current request.
//
// Matching is fail-closed. Every stage must resolve to exactly one distinct
// incident participant through declaration-backed stage/agent aliases. Trace,
// optional diagrams, context-only participants, partial slates, and ambiguous
// aliases cannot activate the authority.
func MatchesRequiredMainStageParticipantSlate(rm types.RequestModel, main []StageRow) bool {
	if rm.Intent == types.IntentTrace || types.ResolveQuestionFamily(rm) == types.QFRootCauseTrace ||
		rm.PredicateAxis != types.AxisFlow || rm.DiagramHint == nil || !rm.DiagramHint.Required ||
		len(main) == 0 {
		return false
	}
	participants := make([]types.DiagramParticipantHint, 0, len(rm.DiagramHint.Participants))
	for _, participant := range rm.DiagramHint.Participants {
		if strings.TrimSpace(participant.Identity) != "" && participant.Role == types.DiagramParticipantIncidentRequired {
			participants = append(participants, participant)
		}
	}
	if len(participants) < len(main) {
		return false
	}
	used := make(map[int]bool, len(main))
	for _, row := range main {
		matches := make([]int, 0, 1)
		for i, participant := range participants {
			if ParticipantMatchesStageRow(rm, participant, row) {
				matches = append(matches, i)
			}
		}
		if len(matches) != 1 || used[matches[0]] {
			return false
		}
		used[matches[0]] = true
	}
	return true
}

// RelevantToRequiredReadModeWorkflow is the shared authority-admission rule
// for completion, prompt, pre-emit, and post-emit consumers. A complete typed
// stage participant slate remains sufficient. The second arm covers a required
// typed stage/workflow answer dimension only when the investigation also
// carries citable grounded evidence from the current read-pipeline authority
// sources. This is the same precise signal used by the finalizer prompt; it
// prevents a prompt recipe from being rejected by sibling validators merely
// because the analyzer supplied a partial participant slate.
//
// The result authorizes only checkout-verified adjacent stage precedence. It
// never authorizes calls, data flow, participant connectivity, or runtime
// causality, and it does not inspect request or answer prose.
func RelevantToRequiredReadModeWorkflow(rm types.RequestModel, evidence []types.EvidenceItem, main []StageRow) bool {
	if MatchesRequiredMainStageParticipantSlate(rm, main) {
		return true
	}
	if rm.Intent == types.IntentTrace || types.ResolveQuestionFamily(rm) == types.QFRootCauseTrace ||
		rm.PredicateAxis != types.AxisFlow || rm.DiagramHint == nil || !rm.DiagramHint.Required ||
		len(main) == 0 {
		return false
	}
	if !hasGroundedReadModeAuthorityEvidence(evidence) {
		return false
	}
	if hasRequiredStageWorkflowDimension(rm) {
		return true
	}
	_, _, ok := requiredReadModeParticipantStageSpan(rm, main)
	return ok
}

// SelectRequiredReadModeWorkflow returns the narrowest verified stage span
// authorized by the typed request. A complete stage slate or a required typed
// stage/workflow dimension selects the whole main lane. Otherwise two or more
// unambiguous incident participants that resolve to current read stages select
// only their contiguous canonical interval. Unmatched participants remain
// independent coverage obligations and do not invalidate a valid stage span.
//
// The endpoint arm additionally requires grounded current-pipeline evidence;
// it never reads request prose, participant source_quote, answer text, or
// display labels beyond the schema-validated participant identity surfaces.
func SelectRequiredReadModeWorkflow(rm types.RequestModel, evidence []types.EvidenceItem, authority ReadModeAuthority) WorkflowSelection {
	if len(authority.Main) == 0 || len(authority.Precedence) != len(authority.Main)-1 {
		return WorkflowSelection{}
	}
	if MatchesRequiredMainStageParticipantSlate(rm, authority.Main) {
		return copyWorkflowSelection(authority.Main, authority.Precedence)
	}
	if rm.Intent == types.IntentTrace || types.ResolveQuestionFamily(rm) == types.QFRootCauseTrace ||
		rm.PredicateAxis != types.AxisFlow || rm.DiagramHint == nil || !rm.DiagramHint.Required ||
		!hasGroundedReadModeAuthorityEvidence(evidence) {
		return WorkflowSelection{}
	}
	if hasRequiredStageWorkflowDimension(rm) {
		return copyWorkflowSelection(authority.Main, authority.Precedence)
	}
	start, end, ok := requiredReadModeParticipantStageSpan(rm, authority.Main)
	if !ok || start < 0 || end >= len(authority.Main) || start >= end {
		return WorkflowSelection{}
	}
	return copyWorkflowSelection(authority.Main[start:end+1], authority.Precedence[start:end])
}

func copyWorkflowSelection(main []StageRow, precedence []PrecedenceRelation) WorkflowSelection {
	return WorkflowSelection{
		Main:       append([]StageRow(nil), main...),
		Precedence: append([]PrecedenceRelation(nil), precedence...),
	}
}

func hasGroundedReadModeAuthorityEvidence(evidence []types.EvidenceItem) bool {
	for _, item := range evidence {
		if item.LineStart <= 0 || item.GroundingStatus != types.GroundingGrounded || !item.IsCitable() {
			continue
		}
		switch normalizedReadModeAuthorityPath(item.Source) {
		case types.ReadModePipelineStageBindingFile,
			types.ReadModePipelineTopologyFile,
			types.ReadModePipelineOrchestratorFile:
			return true
		}
	}
	return false
}

func requiredReadModeParticipantStageSpan(rm types.RequestModel, main []StageRow) (start, end int, ok bool) {
	if rm.DiagramHint == nil || len(main) < 2 {
		return 0, 0, false
	}
	matched := map[int]bool{}
	for _, participant := range rm.DiagramHint.Participants {
		if participant.Role != types.DiagramParticipantIncidentRequired || strings.TrimSpace(participant.Identity) == "" {
			continue
		}
		var indexes []int
		for i, row := range main {
			if ParticipantMatchesStageRow(rm, participant, row) {
				indexes = append(indexes, i)
			}
		}
		if len(indexes) > 1 {
			return 0, 0, false
		}
		if len(indexes) == 1 {
			matched[indexes[0]] = true
		}
	}
	if len(matched) < 2 {
		return 0, 0, false
	}
	start, end = len(main), -1
	for index := range matched {
		if index < start {
			start = index
		}
		if index > end {
			end = index
		}
	}
	return start, end, start < end
}

func hasRequiredStageWorkflowDimension(rm types.RequestModel) bool {
	profile := rm.RequestedAnswerDimensions
	if profile == nil || !profile.Active() {
		return false
	}
	for _, dimension := range profile.Dimensions {
		if dimension.Required && dimension.Role == types.RequestedAnswerDimensionStageWorkflow {
			return true
		}
	}
	return false
}

func normalizedReadModeAuthorityPath(path string) string {
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	path = strings.TrimPrefix(path, "./")
	for strings.Contains(path, "//") {
		path = strings.ReplaceAll(path, "//", "/")
	}
	return path
}

// ParticipantMatchesStageRow resolves one typed participant against one
// checkout-verified row. It is shared by completion, prompt projection, and
// participant coverage so those consumers cannot drift on stage aliases.
func ParticipantMatchesStageRow(rm types.RequestModel, participant types.DiagramParticipantHint, row StageRow) bool {
	for _, participantSurface := range types.DiagramParticipantIdentitySurfaces(rm, participant) {
		for _, alias := range row.IdentityAliases() {
			if types.AnswerCodeIdentitySurfacesEquivalent(participantSurface, alias) ||
				types.AnswerCodeIdentitySurfacesCompatible(participantSurface, alias) {
				return true
			}
		}
	}
	return false
}

// ParticipantHasIncidentPrecedence reports whether one typed incident
// participant is an endpoint of the checkout-verified precedence component.
// It does not inspect Mermaid labels, request prose, or model output.
func ParticipantHasIncidentPrecedence(rm types.RequestModel, participant types.DiagramParticipantHint, relations []PrecedenceRelation) bool {
	if participant.Role != types.DiagramParticipantIncidentRequired {
		return false
	}
	for _, relation := range relations {
		if ParticipantMatchesStageRow(rm, participant, relation.From) ||
			ParticipantMatchesStageRow(rm, participant, relation.To) {
			return true
		}
	}
	return false
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

// LoadReadModeStateCarriers verifies the exact shared carrier fields in the
// checked-out context.go. It is intentionally independent from LoadReadMode:
// failure here suppresses only state-carrier guidance and must not erase a
// separately verified stage membership/order authority.
func LoadReadModeStateCarriers(repoRoot string) ([]StateCarrierField, bool) {
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		return nil, false
	}
	rel := types.ReadModePipelineContextFile
	path := filepath.Join(repoRoot, filepath.FromSlash(rel))
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return verifiedReadModeStateCarrierFields(path, rel, data)
}

func verifiedReadModeStateCarrierFields(path, rel string, data []byte) ([]StateCarrierField, bool) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, data, 0)
	if err != nil {
		return nil, false
	}
	typeFields := make(map[string]map[string]StateCarrierField)
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok || structType.Fields == nil {
				continue
			}
			owner := typeSpec.Name.Name
			if owner != "BusContext" && owner != "MutableState" {
				continue
			}
			if typeFields[owner] == nil {
				typeFields[owner] = make(map[string]StateCarrierField)
			}
			for _, field := range structType.Fields.List {
				var rendered bytes.Buffer
				if err := format.Node(&rendered, fset, field.Type); err != nil {
					return nil, false
				}
				for _, name := range field.Names {
					if _, duplicate := typeFields[owner][name.Name]; duplicate {
						return nil, false
					}
					typeFields[owner][name.Name] = StateCarrierField{
						Owner: owner, Field: name.Name, Type: rendered.String(),
						File: rel, Line: fset.Position(name.Pos()).Line,
					}
				}
			}
		}
	}
	out := make([]StateCarrierField, 0, len(readModeStateCarrierFieldSpecs))
	for _, want := range readModeStateCarrierFieldSpecs {
		got, ok := typeFields[want.Owner][want.Field]
		if !ok || got.Type != want.Type || got.Line <= 0 {
			return nil, false
		}
		out = append(out, got)
	}
	return out, true
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
