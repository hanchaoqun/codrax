package orchestrator

import (
	"errors"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/worktree"
)

const noToolStallSignature = "<no-tools>"

// computeStallSignature summarizes a failed dispatch by the ordered tool path
// it reached. Empty means the attempt made structured progress and should not
// count toward plateau suppression.
func computeStallSignature(toolResults []types.ToolResult, out *agent.StageOutput, stage types.PipelineStage) string {
	_ = stage
	if out != nil && out.Error == "" {
		return ""
	}
	if len(toolResults) == 0 {
		return noToolStallSignature
	}
	names := make([]string, 0, len(toolResults))
	for _, r := range toolResults {
		names = append(names, r.ToolName)
	}
	return strings.Join(names, "|")
}

// transientHardPlateauEligible decides whether a transient dispatch failure is
// precise enough to suppress further retries when the tool signature repeats.
func transientHardPlateauEligible(err error, sig string) bool {
	if err == nil || sig == "" || sig == noToolStallSignature {
		return false
	}
	return errors.Is(err, llm.ErrStreamStalled)
}

// stallPlateauMessage builds the user-facing terminal message when transient
// stall plateau fires.
func stallPlateauMessage(busCtx *types.BusContext, stage types.PipelineStage, transientReason string, autoInitRepo, scaffoldEnabled bool) string {
	if busCtx == nil {
		return fmt.Sprintf("%s repeatedly stalled (%s); aborting", stage, transientReason)
	}
	mode := busCtx.Mode
	emptyRepo := false
	if busCtx.MainRepoRoot != "" {
		if state, err := worktree.DetectRepoState(busCtx.MainRepoRoot); err == nil {
			emptyRepo = state.NeedsInit()
		} else if busCtx.EnvFacts != nil {
			switch busCtx.EnvFacts.GitRepoState {
			case "not_initialized", "no_commits":
				emptyRepo = true
			}
		}
	} else if busCtx.EnvFacts != nil {
		switch busCtx.EnvFacts.GitRepoState {
		case "not_initialized", "no_commits":
			emptyRepo = true
		}
	}
	zh := strings.HasPrefix(strings.ToLower(busCtx.Language), "zh") || busCtx.Language == ""
	writeMode := mode == types.ModePlan || mode == types.ModeApply || mode == types.ModeVerify
	if zh {
		stageLabel := writeStageZhLabel(stage)
		base := fmt.Sprintf("%s连续多次没产出可用结果,已中止重试", stageLabel)
		switch {
		case writeMode && emptyRepo && !autoInitRepo:
			return base + "。当前目录还不是 git 仓库;先加 --auto-init-repo 授权初始化,再决定是否需要 --allow-scaffold。"
		case writeMode && emptyRepo && !scaffoldEnabled:
			return base + "。目录是空的,模型没有源代码可以参考;从零创建新项目需要加 --allow-scaffold (或在配置里设 write_scaffold_enabled: true)。"
		case writeMode && emptyRepo:
			return base + "。空目录已开启 scaffold,但模型仍然给不出可用的方案。在配置文件里换更强的模型再试。"
		case writeMode:
			return base + "。模型重复给不出可用的方案。在配置文件里换更强的模型再试。"
		default:
			return base + "。两次结果完全相同,继续重试也不会有变化。"
		}
	}
	stageLabel := writeStageEnLabel(stage)
	base := fmt.Sprintf("%s repeatedly produced no usable result; retry aborted", stageLabel)
	switch {
	case writeMode && emptyRepo && !autoInitRepo:
		return base + ". The target directory is not yet a git repo; first authorize initialization via --auto-init-repo, then decide whether --allow-scaffold is also needed."
	case writeMode && emptyRepo && !scaffoldEnabled:
		return base + ". The directory is empty so the model has no existing source to read; creating a new project from scratch needs --allow-scaffold (or write_scaffold_enabled: true in the config file)."
	case writeMode && emptyRepo:
		return base + ". The empty directory has scaffold authorized, but the model keeps producing nothing usable. Switch to a stronger model in the config file and retry."
	case writeMode:
		return base + ". The model keeps producing nothing usable. Switch to a stronger model in the config file and retry."
	default:
		return base + ". Two outcomes were identical; continued retry will not change anything."
	}
}

func writeStageZhLabel(stage types.PipelineStage) string {
	switch stage {
	case types.StagePlan:
		return "生成改动方案"
	case types.StageApply:
		return "应用改动"
	case types.StageVerify:
		return "运行验证测试"
	}
	return string(stage)
}

func writeStageEnLabel(stage types.PipelineStage) string {
	switch stage {
	case types.StagePlan:
		return "Drafting the change plan"
	case types.StageApply:
		return "Applying changes"
	case types.StageVerify:
		return "Running verification tests"
	}
	return string(stage)
}

// friendlyDispatchErr translates typed transport/streaming errors into a
// user-readable single-sentence surface.
func friendlyDispatchErr(err error) string {
	if err == nil {
		return ""
	}
	var stall *llm.StreamStalledError
	if errors.As(err, &stall) && stall != nil {
		return fmt.Sprintf("upstream LLM stream stalled (no bytes for %s)", stall.IdleFor)
	}
	var firstByte *llm.StreamFirstByteTimeoutError
	if errors.As(err, &firstByte) && firstByte != nil {
		return fmt.Sprintf("upstream LLM produced no SSE bytes within %s of the request being accepted", firstByte.IdleFor)
	}
	return err.Error()
}

// shouldSuppressVerifyRetry reports whether a verify failure came from the
// local verifier/tooling environment rather than an authoritative code-test
// verdict.
func shouldSuppressVerifyRetry(report *types.ChangeReport) bool {
	if report == nil {
		return false
	}
	return report.FailureKind == types.FailureKindRunnerMissing ||
		report.FailureKind == types.FailureKindParserError ||
		report.FailureKind == types.FailureKindPreexistingBuildFailure ||
		len(report.NoTestsRunners) > 0
}

// verifyStallReason returns a non-empty reason when consecutive verify rounds
// produced identical typed signal.
func verifyStallReason(closure *types.WriteClosure) string {
	if closure == nil {
		return ""
	}
	hist := closure.Fingerprints()
	if len(hist) < 2 {
		return ""
	}
	cur := hist[len(hist)-1]
	prev := hist[len(hist)-2]
	if !cur.SameSignal(prev) {
		return ""
	}
	return fmt.Sprintf("two consecutive verify rounds produced identical signal (applied=%d passed=%d failed=%d failureHash=%s) — planner stuck on the same outcome",
		cur.AppliedCount, cur.VerifyPassed, cur.VerifyFailed, cur.FailureSummaryHash)
}

func plannerStallRecoveryHint() string {
	return "RETRY DIRECTIVE — your previous response was cut off mid-stream because a single emit_change_plan exceeded what the streaming response can carry.\n" +
		"\n" +
		"On this retry use the multi-round path:\n" +
		"  1. Call emit_plan_skeleton ONCE: request, summary, changes[] metadata only (path + kind + rationale; no new_content, no patch).\n" +
		"  2. Call emit_plan_change once per file with kind ∈ {create, modify, patch}, with that single file's body. Skip kind=delete.\n" +
		"\n" +
		"Do NOT call emit_change_plan again on this retry — same wall. The previous response was discarded; start the skeleton fresh."
}

func plannerScaffoldHint() string {
	return "SCAFFOLD DIRECTIVE — the target directory is empty; this run scaffolds a project from scratch. A single emit_change_plan with every file's body inline is likely too large to stream.\n" +
		"\n" +
		"Use the multi-round path:\n" +
		"  1. Call emit_plan_skeleton ONCE with every file's metadata (path + kind + rationale; no new_content, no patch).\n" +
		"  2. Call emit_plan_change once per non-delete file to send its body.\n" +
		"\n" +
		"Begin the changes[] with the language manifest the user's request implies (go.mod for Go, package.json for Node, pyproject.toml for Python, Cargo.toml for Rust, etc.). For files that import other files in this same plan, set depends_on so the manifest and any imported files appear earlier in apply order."
}
