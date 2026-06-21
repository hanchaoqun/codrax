package orchestrator

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

type readFailureMemorySignal struct {
	Scenario       string
	Family         string
	Stage          string
	RetryCount     int
	ViolationCount int
	RepairCount    int
	ViolationKinds []types.ViolationKind
	RepairKinds    []types.RepairKind
}

// persistReadFailureMemorySoftly records a deterministic, typed read-mode
// learning row into the existing AnswerTaxonomyStore. It is deliberately
// advisory-only: the only production consumer is analyzer pitfall injection,
// never a scheduler hard gate.
func (o *Orchestrator) persistReadFailureMemorySoftly() *types.AnswerPattern {
	if o == nil || o.busCtx == nil || o.answerTaxonomyStore == nil || !o.answerTaxonomyStore.Enabled() {
		return nil
	}
	if o.busCtx.TaskState.LastError != "" {
		return nil
	}
	if o.busCtx.Mode != types.ModeRead && o.busCtx.Mode != "" {
		return nil
	}
	mu := o.busCtx.Mutable
	if mu == nil {
		return nil
	}
	sig := collectReadFailureMemorySignal(o.busCtx, mu)
	pattern := buildReadFailureMemoryPattern(sig)
	if pattern == nil {
		return nil
	}
	saved, err := o.answerTaxonomyStore.Append(pattern)
	if err != nil {
		mu.AppendLearningFailure("read_failure_memory", err.Error())
		logging.Warning("[answer_taxonomy] read failure memory append failed (non-fatal): %v", err)
		return nil
	}
	if saved == nil {
		return nil
	}
	logging.Info("[answer_taxonomy] persisted read failure memory name=%q", saved.Name)
	return saved
}

func collectReadFailureMemorySignal(bus *types.BusContext, mu *types.MutableState) readFailureMemorySignal {
	var sig readFailureMemorySignal
	if bus != nil && bus.AnalysisIR != nil {
		sig.Scenario = string(bus.AnalysisIR.RequestModel.Scenario)
		sig.Family = string(types.ResolveQuestionFamily(bus.AnalysisIR.RequestModel))
	}
	stageCounts := make(map[string]int, 4)
	for _, event := range mu.AnswerRetryEvents() {
		stage := normalizeReadFailureStage(event.Stage)
		if stage == "" {
			continue
		}
		stageCounts[stage]++
	}
	closure := mu.EvidenceClosure()
	var health map[string]types.StageStats
	if closure != nil {
		health = closure.StageHealthSnapshot()
		violationCounts := make(map[types.ViolationKind]int)
		for _, v := range closure.Violations() {
			if v.Kind == "" || isPlumbingFailureViolation(v.Kind) {
				continue
			}
			violationCounts[v.Kind]++
			sig.ViolationCount++
		}
		sig.ViolationKinds = sortedViolationKindsByCount(violationCounts)
		repairCounts := make(map[types.RepairKind]int)
		for _, r := range closure.ActiveRepairs() {
			if r.Kind == "" || !r.Kind.IsValid() {
				continue
			}
			repairCounts[r.Kind]++
			sig.RepairCount++
		}
		sig.RepairKinds = sortedRepairKindsByCount(repairCounts)
	}
	sig.Stage, sig.RetryCount = dominantReadFailureStage(stageCounts, health)
	return sig
}

func buildReadFailureMemoryPattern(sig readFailureMemorySignal) *types.AnswerPattern {
	if sig.RetryCount == 0 && sig.ViolationCount < 2 && sig.RepairCount == 0 {
		return nil
	}
	stage := sig.Stage
	if stage == "" {
		stage = "read"
	}
	classLabel := readFailureMemoryClassLabel(sig)
	name := fmt.Sprintf("read %s stage needed %s", stage, classLabel)
	if len(name) > 100 {
		name = name[:100]
	}
	description := fmt.Sprintf(
		"A prior successful read run needed structured recovery before the final answer was acceptable. Dominant stage=%s, repair_class=%s, retry_count=%d, typed_content_violations=%d, active_repairs=%d. For future matching runs, bias analysis toward closing the same typed proof gap early instead of waiting for late retry rounds.",
		stage, classLabel, sig.RetryCount, sig.ViolationCount, sig.RepairCount)
	trigger := readFailureMemoryTrigger(sig, stage)
	consequence := "The answer can lose evidence coverage or burn repeated retry rounds when this proof gap is deferred."
	confidence := 0.55
	if sig.RetryCount > 1 {
		confidence += 0.05
	}
	if sig.ViolationCount >= 2 {
		confidence += 0.05
	}
	if sig.RepairCount > 0 {
		confidence += 0.05
	}
	if confidence > 0.8 {
		confidence = 0.8
	}
	p := &types.AnswerPattern{
		Name:        name,
		Description: description,
		Trigger:     trigger,
		Consequence: consequence,
		Confidence:  confidence,
	}
	if strings.TrimSpace(sig.Scenario) != "" {
		p.AppliesToScenarios = []string{strings.TrimSpace(sig.Scenario)}
	}
	if strings.TrimSpace(sig.Family) != "" {
		p.AppliesToFamilies = []string{strings.TrimSpace(sig.Family)}
	}
	if !p.IsValid() {
		return nil
	}
	return p
}

func readFailureMemoryClassLabel(sig readFailureMemorySignal) string {
	if len(sig.RepairKinds) > 0 {
		switch sig.RepairKinds[0] {
		case types.RepairReadFile:
			return "file coverage repair"
		case types.RepairEmitEvidence:
			return "evidence emit repair"
		case types.RepairExpandSearch:
			return "search expansion repair"
		case types.RepairStructuredHandoff:
			return "structured handoff repair"
		case types.RepairSwapView:
			return "view selection repair"
		case types.RepairRebindSubject:
			return "subject binding repair"
		case types.RepairForceCompleteDowngrade:
			return "completion downgrade"
		}
	}
	if len(sig.ViolationKinds) > 0 {
		return "answer contract repair"
	}
	return "proof repair"
}

func readFailureMemoryTrigger(sig readFailureMemorySignal, stage string) string {
	var parts []string
	if strings.TrimSpace(sig.Scenario) != "" {
		parts = append(parts, "scenario="+strings.TrimSpace(sig.Scenario))
	}
	if strings.TrimSpace(sig.Family) != "" {
		parts = append(parts, "family="+strings.TrimSpace(sig.Family))
	}
	if len(parts) == 0 {
		parts = append(parts, "read-mode request")
	}
	return fmt.Sprintf("When %s reaches stage=%s with typed retry, repair, or content-contract pressure recorded in execution state.",
		strings.Join(parts, " and "), stage)
}

func normalizeReadFailureStage(stage string) string {
	switch types.PipelineStage(strings.TrimSpace(stage)) {
	case types.StageAnalyze:
		return string(types.StageAnalyze)
	case types.StageExplore:
		return string(types.StageExplore)
	case types.StageExtract:
		return string(types.StageExtract)
	case types.StageFinalize:
		return string(types.StageFinalize)
	default:
		return ""
	}
}

func dominantReadFailureStage(retryCounts map[string]int, health map[string]types.StageStats) (string, int) {
	stages := []string{
		string(types.StageAnalyze),
		string(types.StageExplore),
		string(types.StageExtract),
		string(types.StageFinalize),
	}
	bestStage := ""
	bestScore := 0
	bestRetries := 0
	for _, stage := range stages {
		retries := retryCounts[stage]
		st := health[stage]
		if st.Retries > retries {
			retries = st.Retries
		}
		score := retries*10 + st.Violations + st.Repairs + st.PendingReads + st.ForcedReads
		if score > bestScore {
			bestScore = score
			bestStage = stage
			bestRetries = retries
		}
	}
	return bestStage, bestRetries
}

func sortedViolationKindsByCount(counts map[types.ViolationKind]int) []types.ViolationKind {
	if len(counts) == 0 {
		return nil
	}
	out := make([]types.ViolationKind, 0, len(counts))
	for kind := range counts {
		out = append(out, kind)
	}
	sort.Slice(out, func(i, j int) bool {
		if counts[out[i]] != counts[out[j]] {
			return counts[out[i]] > counts[out[j]]
		}
		return string(out[i]) < string(out[j])
	})
	return out
}

func sortedRepairKindsByCount(counts map[types.RepairKind]int) []types.RepairKind {
	if len(counts) == 0 {
		return nil
	}
	out := make([]types.RepairKind, 0, len(counts))
	for kind := range counts {
		out = append(out, kind)
	}
	sort.Slice(out, func(i, j int) bool {
		if counts[out[i]] != counts[out[j]] {
			return counts[out[i]] > counts[out[j]]
		}
		return string(out[i]) < string(out[j])
	})
	return out
}
