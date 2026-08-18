package tool

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

const requestedBranchBehaviorMaxDebts = 4

type requestedBranchBehaviorStateDebt struct {
	file             string
	guard            types.EvidenceItem
	branch           repotypes.ControlFlowBranch
	selectedEffect   repotypes.ControlFlowEffect
	groundedState    bool
	stateAssignments []types.EvidenceItem
}

// requestedBranchBehaviorStateDowngrade prevents a selected branch from being
// explained with a state provenance that cannot reach it. Activation is a
// schema-validated requested dimension, not request/final prose. The branch
// ownership comes from parser-owned ControlFlowBranches, the selected effect
// comes from model-authored grounded evidence, and the conflicting state comes
// from typed assignment evidence.
//
// This check is intentionally narrow. It handles only a simple boolean guard
// whose sole grounded state is the opposite of the selected if/else arm. A
// parameter guard, compound expression, switch/match selector, unknown value,
// multiple state values, or ambiguous source file fails open. The system asks
// the model to collect the missing state/input producer; it never creates that
// evidence or decides that the branch is reachable.
func requestedBranchBehaviorStateDowngrade(
	ctx *types.BusContext,
	closure *types.EvidenceClosure,
	evidence []types.EvidenceItem,
) string {
	if ctx == nil || ctx.Mutable == nil || ctx.AnalysisIR == nil || closure == nil ||
		!requestModelRequiresBranchBehavior(ctx.AnalysisIR.RequestModel) || len(evidence) == 0 {
		return ""
	}
	graph, ok := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	if !ok || graph == nil || len(graph.FileIndex) == 0 {
		return ""
	}
	debts := requestedBranchBehaviorStateDebts(graph, evidence)
	if len(debts) == 0 {
		return ""
	}
	if len(debts) > requestedBranchBehaviorMaxDebts {
		debts = debts[:requestedBranchBehaviorMaxDebts]
	}
	for _, debt := range debts {
		closure.AddRepair(types.RepairDirective{
			Kind:      types.RepairEmitEvidence,
			Files:     []string{debt.file},
			Keywords:  []string{debt.guard.AnchorSymbol},
			Tools:     []string{"grep", "read_file", "emit_evidence"},
			Rationale: "a requested branch-behavior explanation selected a parser-owned branch effect, but the only grounded boolean state write proves the opposite arm; collect the exact state/input producer or keep reachability unproven",
			Origin:    fmt.Sprintf("pre_complete.requested_branch_behavior_state.%d", debt.guard.LineStart),
			Stage:     string(types.StageExplore),
		})
	}

	var b strings.Builder
	b.WriteString(EmitInvestigationCompleteDowngradePrefix + " — a requested branch-behavior explanation lacks the state provenance needed to reach its selected branch.\n\n")
	b.WriteString("Parser-owned control flow and grounded evidence currently prove these exact contradictions:\n")
	for _, debt := range debts {
		arm := string(debt.branch.Arm)
		fmt.Fprintf(&b, "  - guard `%s` @ %s:%d selects the `%s` arm effect `%s` @ line %d, but the only grounded boolean state is `%t`",
			debt.guard.AnchorSymbol, debt.file, debt.guard.LineStart, arm,
			debt.selectedEffect.Expression, debt.selectedEffect.LineStart, debt.groundedState)
		if len(debt.stateAssignments) > 0 {
			fmt.Fprintf(&b, " from %s", requestedBranchBehaviorAssignmentLocations(debt.stateAssignments))
		}
		b.WriteString(".\n")
	}
	b.WriteString("\nSearch only the exact typed guard symbol in the named source, then emit citable assignment/initializer and exception-handler or input-provenance evidence that can produce the selected arm. If that producer cannot be proved, preserve the selected branch implementation but mark its reachability unproven. Do not infer that a handler is absent from a single state write, and do not turn source-line adjacency into branch ownership.")
	return b.String()
}

func requestModelRequiresBranchBehavior(rm types.RequestModel) bool {
	profile := rm.RequestedAnswerDimensions
	if profile == nil || !profile.Active() {
		return false
	}
	for _, dim := range profile.Dimensions {
		if dim.Required && dim.Role == types.RequestedAnswerDimensionBranchBehavior {
			return true
		}
	}
	return false
}

func requestedBranchBehaviorStateDebts(graph *repotypes.Graph, evidence []types.EvidenceItem) []requestedBranchBehaviorStateDebt {
	var out []requestedBranchBehaviorStateDebt
	seen := map[string]bool{}
	for _, guard := range evidence {
		if guard.Producer != types.EvidenceProducerExplorerEmitEvidence || !guard.IsCitable() ||
			types.ClaimFormOf(guard) != types.ClaimGuardCondition || strings.TrimSpace(guard.Source) == "" ||
			guard.LineStart <= 0 || strings.TrimSpace(guard.AnchorSymbol) == "" {
			continue
		}
		file, fi, ok := requestedBranchBehaviorUniqueFile(graph, guard.Source)
		if !ok {
			continue
		}
		for branchIndex, branch := range fi.ControlFlowBranches {
			if branch.Selector != "" ||
				!requestedBranchBehaviorGuardOwnsBranch(fi.ControlFlowBranches, branchIndex, guard.LineStart, guard.AnchorSymbol) {
				continue
			}
			selected, ok := requestedBranchBehaviorSelectedEffect(evidence, file, branch)
			if !ok {
				continue
			}
			state, assignments, ok := requestedBranchBehaviorSingleBooleanState(evidence, file, guard.AnchorSymbol)
			if !ok || !requestedBranchBehaviorStateOpposesArm(state, branch.Arm) {
				continue
			}
			key := canonicalRelationSourcePath(file) + "\x00" + strconv.Itoa(branch.GuardLine) + "\x00" + string(branch.Arm)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, requestedBranchBehaviorStateDebt{
				file: file, guard: guard, branch: branch, selectedEffect: selected,
				groundedState: state, stateAssignments: assignments,
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].file != out[j].file {
			return out[i].file < out[j].file
		}
		return out[i].guard.LineStart < out[j].guard.LineStart
	})
	return out
}

// requestedBranchBehaviorGuardOwnsBranch accepts the direct if-arm shape and
// one parser-owned alternate-arm shape used by the Cangjie extractor. The
// latter records the alternative GuardLine on `else`; it is paired only when
// the immediately preceding parser record is the matching consequence arm.
// This consumes structured record order, not source-line adjacency.
func requestedBranchBehaviorGuardOwnsBranch(branches []repotypes.ControlFlowBranch, index, guardLine int, guardSymbol string) bool {
	if index < 0 || index >= len(branches) || guardLine <= 0 {
		return false
	}
	branch := branches[index]
	if !requestedBranchBehaviorSimpleGuardMatches(branch.Condition, guardSymbol) {
		return false
	}
	if branch.GuardLine == guardLine {
		return true
	}
	if branch.Arm != repotypes.ControlFlowArmAlternative || index == 0 {
		return false
	}
	consequence := branches[index-1]
	return consequence.Arm == repotypes.ControlFlowArmConsequence &&
		consequence.Selector == branch.Selector && consequence.GuardLine == guardLine &&
		strings.TrimSpace(consequence.Condition) == strings.TrimSpace(branch.Condition)
}

func requestedBranchBehaviorUniqueFile(graph *repotypes.Graph, source string) (string, *repotypes.FileInfo, bool) {
	if graph == nil || strings.TrimSpace(source) == "" {
		return "", nil, false
	}
	var file string
	var match *repotypes.FileInfo
	for key, fi := range graph.FileIndex {
		if fi == nil {
			continue
		}
		candidate := fi.RelPath
		if strings.TrimSpace(candidate) == "" {
			candidate = key
		}
		if !callChainSourcePathEquivalent(canonicalRelationSourcePath(candidate), canonicalRelationSourcePath(source)) {
			continue
		}
		if match != nil && match != fi {
			return "", nil, false
		}
		file, match = canonicalRelationSourcePath(candidate), fi
	}
	return file, match, match != nil && file != ""
}

func requestedBranchBehaviorSimpleGuardMatches(condition, symbol string) bool {
	condition = strings.TrimSpace(condition)
	for len(condition) >= 2 && condition[0] == '(' && condition[len(condition)-1] == ')' {
		condition = strings.TrimSpace(condition[1 : len(condition)-1])
	}
	return types.IsCodeIdentitySurface(condition) &&
		types.AnswerCodeIdentitySurfacesCompatible(condition, strings.TrimSpace(symbol))
}

func requestedBranchBehaviorSelectedEffect(evidence []types.EvidenceItem, file string, branch repotypes.ControlFlowBranch) (repotypes.ControlFlowEffect, bool) {
	for _, effect := range branch.Effects {
		expected := requestedBranchBehaviorEffectAnchor(effect.Kind)
		if expected == types.AnchorKind("") {
			continue
		}
		for _, item := range evidence {
			if item.Producer != types.EvidenceProducerExplorerEmitEvidence || !item.IsCitable() ||
				item.AnchorKind != expected || !callChainSourcePathEquivalent(canonicalRelationSourcePath(item.Source), canonicalRelationSourcePath(file)) ||
				item.LineStart < effect.LineStart || item.LineStart > effect.LineEnd {
				continue
			}
			if requestedBranchBehaviorEffectMatchesEvidence(effect, item) {
				return effect, true
			}
		}
	}
	return repotypes.ControlFlowEffect{}, false
}

// requestedBranchBehaviorEffectMatchesEvidence aligns one already-grounded
// model row with one parser-owned effect. A short call anchor may name the
// terminal operation of a qualified expression (for example _tokenize_slow
// for self._tokenize_slow(data)), so raw substring/token matching is too
// strict at member separators. The effect remains the authority: its AST
// kind, exact source interval, and callable expression were checked before
// this identity comparison.
func requestedBranchBehaviorEffectMatchesEvidence(effect repotypes.ControlFlowEffect, item types.EvidenceItem) bool {
	identities := requestedBranchBehaviorEffectIdentities(effect)
	if len(identities) == 0 {
		return false
	}
	candidates := []string{item.AnchorSymbol, item.Object, item.Subject}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		for _, identity := range identities {
			if types.AnswerCodeIdentitySurfacesCompatible(candidate, identity) {
				return true
			}
		}
	}
	return false
}

func requestedBranchBehaviorEffectIdentities(effect repotypes.ControlFlowEffect) []string {
	expression := strings.TrimSpace(effect.Expression)
	if expression == "" {
		return nil
	}
	var candidates []string
	switch effect.Kind {
	case repotypes.ControlFlowEffectCall:
		if open := requestedBranchBehaviorCallArgumentOpen(expression); open > 0 {
			candidates = append(candidates, requestedBranchBehaviorStripTrailingGeneric(strings.TrimSpace(expression[:open])))
		}
	case repotypes.ControlFlowEffectAssignment:
		if op := strings.Index(expression, ":="); op > 0 {
			candidates = append(candidates, strings.TrimSpace(expression[:op]))
		} else if op := strings.IndexByte(expression, '='); op > 0 {
			candidates = append(candidates, strings.TrimSpace(expression[:op]))
		}
	case repotypes.ControlFlowEffectReturn:
		candidates = append(candidates, strings.TrimSpace(strings.TrimPrefix(expression, "return")))
	case repotypes.ControlFlowEffectExit:
		candidates = append(candidates, expression)
	}
	var out []string
	for _, candidate := range candidates {
		if types.AnswerCodeIdentitySurfacesCompatible(candidate, candidate) {
			out = append(out, candidate)
		}
	}
	return out
}

func requestedBranchBehaviorCallArgumentOpen(expression string) int {
	depth := 0
	for i := 0; i < len(expression); i++ {
		switch expression[i] {
		case '(':
			if depth == 0 && strings.TrimSpace(expression[:i]) != "" {
				return i
			}
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		}
	}
	return -1
}

func requestedBranchBehaviorStripTrailingGeneric(surface string) string {
	surface = strings.TrimSpace(surface)
	if !strings.HasSuffix(surface, ">") {
		return surface
	}
	depth := 0
	for i := len(surface) - 1; i >= 0; i-- {
		switch surface[i] {
		case '>':
			depth++
		case '<':
			depth--
			if depth == 0 {
				prefix := strings.TrimSpace(surface[:i])
				return strings.TrimSuffix(prefix, "::")
			}
		}
	}
	return surface
}

func requestedBranchBehaviorEffectAnchor(kind repotypes.ControlFlowEffectKind) types.AnchorKind {
	switch kind {
	case repotypes.ControlFlowEffectCall:
		return types.AnchorCall
	case repotypes.ControlFlowEffectReturn:
		return types.AnchorReturn
	case repotypes.ControlFlowEffectAssignment:
		return types.AnchorAssignment
	case repotypes.ControlFlowEffectExit:
		return types.AnchorTextReference
	default:
		return types.AnchorKind("")
	}
}

func requestedBranchBehaviorSingleBooleanState(evidence []types.EvidenceItem, file, symbol string) (bool, []types.EvidenceItem, bool) {
	states := map[bool]bool{}
	var assignments []types.EvidenceItem
	for _, item := range evidence {
		if !item.IsCitable() || types.ClaimFormOf(item) != types.ClaimAssignmentFact ||
			!callChainSourcePathEquivalent(canonicalRelationSourcePath(item.Source), canonicalRelationSourcePath(file)) ||
			!requestedBranchBehaviorAssignmentTargetsSymbol(item, symbol) {
			continue
		}
		value, err := strconv.ParseBool(strings.ToLower(strings.Trim(strings.TrimSpace(item.Object), "`'\"")))
		if err != nil {
			continue
		}
		states[value] = true
		assignments = append(assignments, item)
	}
	if len(states) != 1 {
		return false, nil, false
	}
	for state := range states {
		return state, assignments, true
	}
	return false, nil, false
}

func requestedBranchBehaviorAssignmentTargetsSymbol(item types.EvidenceItem, symbol string) bool {
	for _, endpoint := range []string{item.Subject, item.AnchorSymbol} {
		if types.AnswerCodeIdentitySurfacesCompatible(strings.TrimSpace(endpoint), strings.TrimSpace(symbol)) {
			return true
		}
	}
	return false
}

func requestedBranchBehaviorStateOpposesArm(state bool, arm repotypes.ControlFlowBranchArm) bool {
	switch arm {
	case repotypes.ControlFlowArmConsequence:
		return !state
	case repotypes.ControlFlowArmAlternative:
		return state
	default:
		return false
	}
}

func requestedBranchBehaviorAssignmentLocations(items []types.EvidenceItem) string {
	parts := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		part := fmt.Sprintf("%s:%d=%s", canonicalRelationSourcePath(item.Source), item.LineStart, strings.TrimSpace(item.Object))
		if seen[part] {
			continue
		}
		seen[part] = true
		parts = append(parts, "`"+part+"`")
	}
	return strings.Join(parts, ", ")
}
