package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// investigation_planner.go — deterministic question decomposition.
//
// DecomposeQuestion transforms an analyzer-classified RequestModel
// into an InvestigationPlan: an ordered list of InvestigationSteps
// the explorer will navigate sequentially. The decomposition is
// purely deterministic — no LLM call — so it is reproducible,
// testable, and fast.
//
// Design principle: simple questions get 1 step (degradation to
// current ERM behavior); complex questions get 2-4 steps with
// structural completion criteria. The strategy per step replaces
// the flat "count >= N" threshold with "have I achieved the
// structural goal?" checks.

// DecomposeQuestion produces an InvestigationPlan from the
// analyzer's classification. Called once at the start of the
// explorer's BuildInitialInstruction.
//
// Decomposition rules (evaluated in order, first match wins):
//
//  1. mechanism + 2+ entities → locate → trace_chain → aggregate
//  2. enumeration + relational verb → enumerate → verify_condition → aggregate
//  3. call_chain + 2+ entities → locate → trace_chain → aggregate
//  4. compare (2+ entities, cross-component cue) → compare → aggregate
//  5. complex + mechanism (1 entity) → locate → trace_chain
//  6. default → 1 step with StrategyGeneric (degradation path)
func DecomposeQuestion(rm types.RequestModel) *types.InvestigationPlan {
	kind := rm.AnalyzerHints.Kind
	entities := rm.AnalyzerHints.Entities
	complexity := rm.Complexity
	raw := strings.ToLower(rm.RawRequest)

	var steps []types.InvestigationStep

	switch {
	// Rule 1: mechanism + 2+ entities → 3-step chain trace
	case (kind == "mechanism" || kind == "call_chain") && len(entities) >= 2:
		steps = mechanismMultiEntitySteps(entities, raw)

	// Rule 2: enumeration + relational verb → 3-step enumerate-verify-count
	case kind == "enumeration" && hasRelationalVerbInDecomposer(raw):
		steps = enumerationWithConditionSteps(entities, raw)

	// Rule 3: complex mechanism with 1 entity → 2-step locate+trace
	case kind == "mechanism" && complexity == types.ComplexityComplex:
		steps = complexMechanismSteps(entities)

	// Rule 4: compare scenario with 2+ entities
	case complexity == types.ComplexityComplex && len(entities) >= 2 && containsCompareCue(raw):
		steps = compareSteps(entities)

	// Default: single step (degradation to current ERM behavior)
	default:
		steps = []types.InvestigationStep{{
			ID:        "step-0",
			Objective: rm.RawRequest,
			Strategy:  strategyFromQuestionKind(kind),
			Entities:  entities,
			Status:    types.StepPending,
		}}
	}

	plan := &types.InvestigationPlan{Steps: steps}
	logging.Debug("[planner] decomposed %q into %d steps: %s",
		truncate(rm.RawRequest, 60), len(steps), plan.Summary())
	return plan
}

// mechanismMultiEntitySteps produces a 3-step plan for
// "how does A call/invoke B" questions.
func mechanismMultiEntitySteps(entities []string, raw string) []types.InvestigationStep {
	src := entities[0]
	tgt := entities[1]
	return []types.InvestigationStep{
		{
			ID:        "step-0",
			Objective: fmt.Sprintf("定位 %s 与 %s 的定义入口和关键文件", src, tgt),
			Strategy:  types.StrategyLocate,
			Entities:  entities,
			Status:    types.StepPending,
		},
		{
			ID:        "step-1",
			Objective: fmt.Sprintf("追踪从 %s 到 %s 的完整调用/交互链路（每一跳都需要 file:line 证据）", src, tgt),
			Strategy:  types.StrategyTraceChain,
			Entities:  []string{src, tgt},
			DependsOn: []string{"step-0"},
			Status:    types.StepPending,
		},
		{
			ID:        "step-2",
			Objective: fmt.Sprintf("汇总从 %s 到 %s 的完整链路", src, tgt),
			Strategy:  types.StrategyAggregate,
			DependsOn: []string{"step-1"},
			Status:    types.StepPending,
		},
	}
}

// enumerationWithConditionSteps produces a 3-step plan for
// "how many X can Y" questions.
func enumerationWithConditionSteps(entities []string, raw string) []types.InvestigationStep {
	subject := "candidates"
	if len(entities) > 0 {
		subject = entities[0]
	}
	return []types.InvestigationStep{
		{
			ID:        "step-0",
			Objective: fmt.Sprintf("列举所有 %s 候选（找到注册/定义的完整集合）", subject),
			Strategy:  types.StrategyEnumerate,
			Entities:  entities,
			Status:    types.StepPending,
		},
		{
			ID:        "step-1",
			Objective: "逐个验证每个候选是否满足条件（读取每个候选的实现，确认条件是否成立）",
			Strategy:  types.StrategyVerifyCondition,
			Entities:  entities,
			DependsOn: []string{"step-0"},
			Status:    types.StepPending,
		},
		{
			ID:        "step-2",
			Objective: "汇总：计数满足条件的候选，列出它们的名称",
			Strategy:  types.StrategyAggregate,
			DependsOn: []string{"step-1"},
			Status:    types.StepPending,
		},
	}
}

// complexMechanismSteps produces a 2-step plan for complex
// mechanism questions with a single entity.
func complexMechanismSteps(entities []string) []types.InvestigationStep {
	target := "target"
	if len(entities) > 0 {
		target = entities[0]
	}
	return []types.InvestigationStep{
		{
			ID:        "step-0",
			Objective: fmt.Sprintf("定位 %s 涉及的所有相关组件和文件", target),
			Strategy:  types.StrategyLocate,
			Entities:  entities,
			Status:    types.StepPending,
		},
		{
			ID:        "step-1",
			Objective: fmt.Sprintf("追踪 %s 的完整 mechanism 链路（每个阶段/步骤都需要 evidence）", target),
			Strategy:  types.StrategyTraceChain,
			Entities:  entities,
			DependsOn: []string{"step-0"},
			Status:    types.StepPending,
		},
	}
}

// compareSteps produces a 2-step plan for comparison questions.
func compareSteps(entities []string) []types.InvestigationStep {
	return []types.InvestigationStep{
		{
			ID:        "step-0",
			Objective: fmt.Sprintf("收集 %s 各自的特征、行为、实现细节", strings.Join(entities, " 和 ")),
			Strategy:  types.StrategyCompare,
			Entities:  entities,
			Status:    types.StepPending,
		},
		{
			ID:        "step-1",
			Objective: "汇总对比：列出共同点和差异点",
			Strategy:  types.StrategyAggregate,
			DependsOn: []string{"step-0"},
			Status:    types.StepPending,
		},
	}
}

// strategyFromQuestionKind maps a question_kind to the appropriate
// fallback InvestStrategy for single-step decomposition.
func strategyFromQuestionKind(kind string) types.InvestStrategy {
	switch kind {
	case "mechanism":
		return types.StrategyTraceChain
	case "enumeration":
		return types.StrategyEnumerate
	case "call_chain":
		return types.StrategyTraceChain
	case "registration":
		return types.StrategyLocate
	case "return_value":
		return types.StrategyLocate
	case "conditional":
		return types.StrategyLocate
	case "config_mapping":
		return types.StrategyLocate
	default:
		return types.StrategyGeneric
	}
}

// hasRelationalVerbInDecomposer checks for relational verbs that
// imply a conditional-enumeration question. Uses the same verb set
// as the complexity reconciler but is a separate function to avoid
// import cycles.
func hasRelationalVerbInDecomposer(lower string) bool {
	for _, verb := range []string{
		"调用", "invoke", "call", "register", "注册",
		"实现", "implement", "use", "使用", "access",
		"触发", "trigger", "可以", "能",
	} {
		if strings.Contains(lower, verb) {
			return true
		}
	}
	return false
}

// containsCompareCue checks for comparison language.
func containsCompareCue(lower string) bool {
	for _, cue := range []string{
		"compare", "对比", "区别", "difference", "versus", "vs",
		"不同", "相比", "异同",
	} {
		if strings.Contains(lower, cue) {
			return true
		}
	}
	return false
}

// truncate is a helper for log messages.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
