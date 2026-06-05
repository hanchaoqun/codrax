package repl

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
	"github.com/hanchaoqun/codrax/internal/logging"
)

// DataTaskCLIConfig is the single-shot companion to the REPL data workflow.
// It intentionally shares the same planner interfaces and deterministic runner
// as the REPL path so CLI and REPL do not drift into different data-lane
// semantics.
type DataTaskCLIConfig struct {
	Planner         DataTaskPlanner
	RepoRoot        string
	RuntimeAnchor   string
	Language        string
	MaxRepairRounds int
	MaxDataRounds   int
	Progress        io.Writer
}

func RunDataTaskCLI(ctx context.Context, request string, policy TurnPolicy, cfg DataTaskCLIConfig) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if cfg.Planner == nil {
		return "", fmt.Errorf("data task planner is not configured")
	}
	repoRoot := strings.TrimSpace(cfg.RepoRoot)
	if repoRoot == "" {
		repoRoot = "."
	}
	candidates, err := dataquery.DiscoverCandidateFiles(repoRoot, 240)
	if err != nil {
		return "", fmt.Errorf("data task discover files: %w", err)
	}
	plan, err := cfg.Planner.PlanDataTask(ctx, request, repoRoot, policy, candidates)
	if err != nil {
		return "", fmt.Errorf("data task plan: %w", err)
	}
	dataTaskCLIPlanProgress(cfg.Progress, cfg.Language, plan)

	repairRoundsMax := normalizeDataTaskMaxRepairRounds(cfg.MaxRepairRounds)
	dataRoundsMax := normalizeDataTaskMaxDataRounds(cfg.MaxDataRounds)
	runner := dataquery.Runner{
		RepoRoot: repoRoot,
		TempRoot: filepath.Join(firstNonEmptyString(strings.TrimSpace(cfg.RuntimeAnchor), repoRoot), "data"),
	}
	currentPlan := plan
	var records []dataTaskWorkflowRecord
	repairRounds := 0
	dataRounds := 0
	for {
		if handled, answer, err := terminalDataTaskPlanForCLI(currentPlan, records, cfg.Language); handled {
			return answer, err
		}
		if dataRounds >= dataRoundsMax {
			if result, ok := latestDataTaskResult(records); ok {
				return dataTaskAnswerMarkdown(cfg.Language, result), nil
			}
			return "", fmt.Errorf("data task workflow budget exhausted before producing a result")
		}
		if errText := dataTaskPlanStagingGuardError(currentPlan); errText != "" {
			var repaired dataquery.TaskPlan
			var ok bool
			repaired, repairRounds, ok, err = repairDataTaskPlanForCLI(ctx, cfg.Planner, request, repoRoot, policy, candidates, currentPlan, errText, records, repairRounds, repairRoundsMax)
			if err != nil {
				return "", err
			}
			records = append(records, dataTaskWorkflowRecord{Plan: currentPlan, Err: errText})
			if ok {
				dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "repair", repairRounds, dataTaskWorkflowErrorSegment(cfg.Language, errText))
				dataTaskCLIPlanProgress(cfg.Progress, cfg.Language, repaired)
				currentPlan = repaired
				continue
			}
			return "", fmt.Errorf("data task planning: %s", errText)
		}
		dataRounds++
		dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "execute", dataRounds)
		result, err := runner.Run(ctx, currentPlan)
		if err != nil {
			errText := fmt.Sprintf("execute data task: %v", err)
			var repaired dataquery.TaskPlan
			var ok bool
			repaired, repairRounds, ok, err = repairDataTaskPlanForCLI(ctx, cfg.Planner, request, repoRoot, policy, candidates, currentPlan, errText, records, repairRounds, repairRoundsMax)
			records = append(records, dataTaskWorkflowRecord{Plan: currentPlan, Err: errText})
			if err != nil {
				return "", err
			}
			if ok {
				dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "repair", repairRounds, dataTaskWorkflowErrorSegment(cfg.Language, errText))
				dataTaskCLIPlanProgress(cfg.Progress, cfg.Language, repaired)
				currentPlan = repaired
				continue
			}
			return "", fmt.Errorf("%s", errText)
		}
		records = append(records, dataTaskWorkflowRecord{Plan: currentPlan, Result: &result})
		logDataTaskCLIResult(dataRounds, result)
		dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "result", dataRounds, dataTaskWorkflowResultSegment(cfg.Language, result))
		evaluator, evalOK := cfg.Planner.(DataTaskEvaluator)
		continuer, contOK := cfg.Planner.(DataTaskContinuationPlanner)
		if !evalOK {
			return dataTaskAnswerMarkdown(cfg.Language, result), nil
		}
		dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "evaluate", dataRounds)
		eval, err := evaluator.EvaluateDataTask(ctx, request, records, cfg.Language)
		if err != nil {
			return "", fmt.Errorf("evaluate data task: %w", err)
		}
		if len(records) > 0 {
			records[len(records)-1].Evaluation = &eval
		}
		switch eval.Status {
		case dataquery.EvalComplete:
			return dataTaskAnswerMarkdown(cfg.Language, result), nil
		case dataquery.EvalContinueData:
			if !contOK {
				return dataTaskAnswerMarkdown(cfg.Language, result), nil
			}
			nextPlan, err := continuer.ContinueDataTask(ctx, request, repoRoot, policy, candidates, records)
			if err != nil {
				return "", fmt.Errorf("continue data task: %w", err)
			}
			dataTaskCLIWorkflowProgress(cfg.Progress, cfg.Language, "continue", dataRounds)
			dataTaskCLIPlanProgress(cfg.Progress, cfg.Language, nextPlan)
			currentPlan = nextPlan
			continue
		case dataquery.EvalNeedsClarification:
			return dataTaskEvaluationMarkdown(cfg.Language, eval), nil
		case dataquery.EvalBlocked:
			return dataTaskEvaluationMarkdown(cfg.Language, eval), nil
		case dataquery.EvalBudgetExhausted, dataquery.EvalPartialAnswerPossible:
			return dataTaskAnswerMarkdown(cfg.Language, result), nil
		default:
			return dataTaskAnswerMarkdown(cfg.Language, result), nil
		}
	}
}

func terminalDataTaskPlanForCLI(plan dataquery.TaskPlan, records []dataTaskWorkflowRecord, lang string) (bool, string, error) {
	switch strings.ToLower(strings.TrimSpace(plan.Status)) {
	case "needs_clarification":
		return true, dataTaskClarificationMarkdown(lang, plan), nil
	case "blocked":
		return true, dataTaskBlockedMarkdown(lang, plan), nil
	case "complete", "completed", "partial_answer_possible", "budget_exhausted":
		if result, ok := latestDataTaskResult(records); ok {
			return true, dataTaskAnswerMarkdown(lang, result), nil
		}
		return true, "", fmt.Errorf("terminal data task plan ended without a result: %s", strings.TrimSpace(plan.Status))
	default:
		return false, "", nil
	}
}

func repairDataTaskPlanForCLI(ctx context.Context, planner DataTaskPlanner, request, repoRoot string, policy TurnPolicy, candidates []dataquery.CandidateFile, currentPlan dataquery.TaskPlan, errText string, records []dataTaskWorkflowRecord, repairRounds, repairRoundsMax int) (dataquery.TaskPlan, int, bool, error) {
	repairer, ok := planner.(DataTaskRepairPlanner)
	if !ok || repairRounds >= repairRoundsMax {
		return dataquery.TaskPlan{}, repairRounds, false, nil
	}
	repairRounds++
	repairedPlan, err := repairer.RepairDataTask(ctx, request, repoRoot, policy, candidates, currentPlan, errText)
	if err != nil {
		return dataquery.TaskPlan{}, repairRounds, false, fmt.Errorf("%s\nrepair data task: %w", errText, err)
	}
	repairedPlan = preserveDataTaskMaterialRepairCoverage(currentPlan, repairedPlan)
	return repairedPlan, repairRounds, true, nil
}

func logDataTaskCLIResult(round int, result dataquery.Result) {
	reconcileStatus := ""
	if result.Reconcile != nil {
		reconcileStatus = result.Reconcile.Status.String()
	}
	logging.Info("[cli/data] data task result round=%d answer_len=%d decisions=%d rules=%d contributions=%d resolutions=%d reconcile=%s consumed=%d warnings=%d",
		round,
		len(strings.TrimSpace(result.Answer)),
		len(result.Rows),
		len(result.RuleCoverage),
		len(result.Contributions),
		len(result.EntityResolutions),
		reconcileStatus,
		len(result.ConsumedPaths),
		len(result.ContractWarnings))
}

func dataTaskCLIPlanProgress(w io.Writer, lang string, plan dataquery.TaskPlan) {
	if w == nil {
		return
	}
	label, segs := dataTaskPlanAuditSummary(plan, lang)
	cliLightRouteSummary(w, label, segs)
}

func dataTaskCLIWorkflowProgress(w io.Writer, lang, kind string, round int, details ...string) {
	if w == nil {
		return
	}
	label := "数据工作流"
	segs := []string{}
	if isZh(lang) {
		switch kind {
		case "execute":
			segs = append(segs, fmt.Sprintf("执行第 %d 批", round))
		case "repair":
			segs = append(segs, fmt.Sprintf("修复第 %d 次", round))
		case "result":
			segs = append(segs, fmt.Sprintf("结果第 %d 批", round))
		case "evaluate":
			segs = append(segs, fmt.Sprintf("评估第 %d 批", round))
		case "continue":
			segs = append(segs, fmt.Sprintf("继续第 %d 批", round+1))
		default:
			segs = append(segs, strings.TrimSpace(kind))
		}
		segs = append(segs, "未读源码")
	} else {
		label = "data workflow"
		switch kind {
		case "execute":
			segs = append(segs, fmt.Sprintf("execute batch %d", round))
		case "repair":
			segs = append(segs, fmt.Sprintf("repair %d", round))
		case "result":
			segs = append(segs, fmt.Sprintf("result batch %d", round))
		case "evaluate":
			segs = append(segs, fmt.Sprintf("evaluate batch %d", round))
		case "continue":
			segs = append(segs, fmt.Sprintf("continue batch %d", round+1))
		default:
			segs = append(segs, strings.TrimSpace(kind))
		}
		segs = append(segs, "no source read")
	}
	for _, detail := range details {
		if detail = strings.TrimSpace(detail); detail != "" {
			segs = append(segs, detail)
		}
	}
	cliLightRouteSummary(w, label, segs)
}

func cliLightRouteSummary(w io.Writer, label string, segs []string) {
	if w == nil {
		return
	}
	label = strings.TrimSpace(label)
	if label == "" {
		return
	}
	clean := make([]string, 0, len(segs))
	for _, seg := range segs {
		seg = strings.TrimSpace(seg)
		if seg != "" {
			clean = append(clean, seg)
		}
	}
	if len(clean) == 0 {
		fmt.Fprintf(w, "◇ %s\n", label)
		return
	}
	fmt.Fprintf(w, "◇ %s · %s\n", label, strings.Join(clean, " · "))
}
