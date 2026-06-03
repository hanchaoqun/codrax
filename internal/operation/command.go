package operation

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// OperationStatus is the precise lifecycle state for an operation plan.
// It is intentionally separate from write-mode ChangePlan status.
type OperationStatus string

const (
	StatusNeedsClarification OperationStatus = "needs_clarification"
	StatusReady              OperationStatus = "ready"
	StatusBlocked            OperationStatus = "blocked"
	StatusExecuted           OperationStatus = "executed"
	StatusRejected           OperationStatus = "rejected"
	StatusCancelled          OperationStatus = "cancelled"
	StatusFailed             OperationStatus = "failed"
)

const (
	ApprovalManual      = "manual"
	ApprovalAutoLowRisk = "auto_low_risk"
	ApprovalDenied      = "denied"

	StepAutoEligible = "eligible"
	StepAutoManual   = "manual"
	StepAutoDenied   = "denied"
)

// CommandPolicy controls command-operation approval. Defaults are deliberately
// conservative: no automatic execution, unknown programs are manual, shell form
// is manual, and only obvious catastrophic commands are hard-denied.
type CommandPolicy struct {
	Approval              string
	AutoLowRisk           bool
	UnknownProgram        string
	ShellPolicy           string
	TimeoutMS             int
	OutputPreviewBytes    int
	DefaultWorkDir        string
	HardDenyDestructive   bool
	AllowMkdirPAutoCreate bool
}

func DefaultCommandPolicy() CommandPolicy {
	return CommandPolicy{
		Approval:              ApprovalManual,
		AutoLowRisk:           false,
		UnknownProgram:        ApprovalManual,
		ShellPolicy:           ApprovalManual,
		TimeoutMS:             120_000,
		OutputPreviewBytes:    32 * 1024,
		HardDenyDestructive:   true,
		AllowMkdirPAutoCreate: true,
	}
}

// CommandOperationRequest is the already-structured command plan proposal.
// A later LLM planner may produce this shape, but the policy evaluator only
// consumes typed fields and never user prose.
type CommandOperationRequest struct {
	Text                 string
	ID                   string
	WorkDir              string
	RiskLevel            string
	BlockReason          string
	Steps                []CommandStep
	ClarifyingQuestions  []ClarifyingQuestion
	RequiresConfirmation bool
}

type ClarifyingQuestion struct {
	ID          string
	Question    string
	Suggestions []string
}

type CommandStep struct {
	ID           string
	Title        string
	Program      string
	Args         []string
	Shell        string
	WorkDir      string
	Env          []string
	TimeoutMS    int
	RiskLevel    string
	SideEffects  []string
	AutoApproval string
	Reason       string
	VerifyHint   string
}

type CommandOperationPlan struct {
	ID                  string
	RequestText         string
	Status              OperationStatus
	RiskLevel           string
	ApprovalMode        string
	WorkDir             string
	Steps               []CommandStep
	ClarifyingQuestions []ClarifyingQuestion
	BlockReason         string
	CreatedAt           time.Time
}

type CommandStepResult struct {
	StepID        string
	Status        OperationStatus
	ExitCode      int
	OutputPreview string
	PayloadRef    string
	Error         string
	TimedOut      bool
}

type CommandOperationResult struct {
	PlanID        string
	Status        OperationStatus
	StepResults   []CommandStepResult
	OutputPreview string
	PayloadRef    string
}

// BuildCommandOperationPlan applies deterministic policy to a typed command
// proposal. It does no IO and does not execute anything.
func BuildCommandOperationPlan(req CommandOperationRequest, policy CommandPolicy) CommandOperationPlan {
	policy = normalizeCommandPolicy(policy)
	workDir := strings.TrimSpace(req.WorkDir)
	if workDir == "" {
		workDir = strings.TrimSpace(policy.DefaultWorkDir)
	}
	if workDir == "" {
		workDir = "."
	}

	plan := CommandOperationPlan{
		ID:                  firstNonEmpty(req.ID, defaultOperationID()),
		RequestText:         strings.TrimSpace(req.Text),
		Status:              StatusReady,
		RiskLevel:           normalizeRisk(req.RiskLevel),
		ApprovalMode:        ApprovalManual,
		WorkDir:             filepath.Clean(workDir),
		ClarifyingQuestions: cleanClarifyingQuestions(req.ClarifyingQuestions),
		CreatedAt:           time.Now().UTC(),
	}
	if len(plan.ClarifyingQuestions) > 0 {
		plan.Status = StatusNeedsClarification
		plan.ApprovalMode = ""
		return plan
	}
	if strings.TrimSpace(req.BlockReason) != "" {
		plan.Status = StatusBlocked
		plan.RiskLevel = highestRisk(plan.RiskLevel, req.RiskLevel)
		plan.ApprovalMode = ApprovalDenied
		plan.BlockReason = strings.TrimSpace(req.BlockReason)
		return plan
	}
	if len(req.Steps) == 0 {
		plan.Status = StatusNeedsClarification
		plan.ApprovalMode = ""
		plan.ClarifyingQuestions = []ClarifyingQuestion{{
			ID:       "command",
			Question: "What command or target should Codrax operate on?",
			Suggestions: []string{
				"provide the source and destination paths",
				"provide the package/tool name and desired action",
			},
		}}
		return plan
	}

	allAutoEligible := policy.AutoLowRisk
	var sideEffects []string
	for i, raw := range req.Steps {
		step := normalizeCommandStep(raw, i, policy, plan.WorkDir)
		decision := evaluateCommandStep(step, policy)
		step.AutoApproval = decision.AutoApproval
		step.RiskLevel = highestRisk(step.RiskLevel, decision.RiskLevel)
		if step.Reason == "" {
			step.Reason = decision.Reason
		}
		plan.Steps = append(plan.Steps, step)
		plan.RiskLevel = highestRisk(plan.RiskLevel, step.RiskLevel)
		sideEffects = append(sideEffects, step.SideEffects...)
		if decision.AutoApproval == StepAutoDenied {
			plan.Status = StatusBlocked
			plan.ApprovalMode = ApprovalDenied
			plan.BlockReason = decision.Reason
		}
		if decision.AutoApproval != StepAutoEligible {
			allAutoEligible = false
		}
	}
	if plan.Status == StatusBlocked {
		return plan
	}
	if req.RequiresConfirmation {
		plan.ApprovalMode = ApprovalManual
		return plan
	}
	if allAutoEligible {
		plan.ApprovalMode = ApprovalAutoLowRisk
	} else {
		plan.ApprovalMode = ApprovalManual
	}
	_ = cleanList(sideEffects) // retained for future renderer/result summaries.
	return plan
}

type commandStepDecision struct {
	AutoApproval string
	RiskLevel    string
	Reason       string
}

func evaluateCommandStep(step CommandStep, policy CommandPolicy) commandStepDecision {
	if strings.TrimSpace(step.Shell) != "" {
		if policy.HardDenyDestructive && looksCatastrophicShell(step.Shell) {
			return commandStepDecision{AutoApproval: StepAutoDenied, RiskLevel: "high", Reason: "shell command matches a hard-deny destructive pattern"}
		}
		return commandStepDecision{AutoApproval: StepAutoManual, RiskLevel: highestRisk(step.RiskLevel, "medium"), Reason: "shell-form commands require manual approval"}
	}
	program := strings.TrimSpace(step.Program)
	if program == "" {
		return commandStepDecision{AutoApproval: StepAutoDenied, RiskLevel: "medium", Reason: "command step is missing a program"}
	}
	if policy.HardDenyDestructive && looksCatastrophicProgram(program, step.Args) {
		return commandStepDecision{AutoApproval: StepAutoDenied, RiskLevel: "high", Reason: "command matches a hard-deny destructive pattern"}
	}
	if isLowRiskProgramArgs(program, step.Args, policy) && len(cleanList(step.SideEffects)) == 0 {
		return commandStepDecision{AutoApproval: StepAutoEligible, RiskLevel: highestRisk(step.RiskLevel, "low"), Reason: "read-only command eligible for low-risk auto approval"}
	}
	if isMkdirPCreate(program, step.Args) && policy.AllowMkdirPAutoCreate && onlyLocalFileWrite(step.SideEffects) {
		return commandStepDecision{AutoApproval: StepAutoEligible, RiskLevel: highestRisk(step.RiskLevel, "low"), Reason: "mkdir -p is eligible for low-risk auto approval when configured"}
	}
	if knownProgram(program) {
		return commandStepDecision{AutoApproval: StepAutoManual, RiskLevel: highestRisk(step.RiskLevel, "medium"), Reason: "known command has side effects or is not auto-eligible"}
	}
	return commandStepDecision{AutoApproval: StepAutoManual, RiskLevel: highestRisk(step.RiskLevel, "medium"), Reason: "unknown command requires manual approval"}
}

func normalizeCommandPolicy(p CommandPolicy) CommandPolicy {
	def := DefaultCommandPolicy()
	if strings.TrimSpace(p.Approval) == "" {
		p.Approval = def.Approval
	}
	if strings.TrimSpace(p.UnknownProgram) == "" {
		p.UnknownProgram = def.UnknownProgram
	}
	if strings.TrimSpace(p.ShellPolicy) == "" {
		p.ShellPolicy = def.ShellPolicy
	}
	if p.TimeoutMS <= 0 {
		p.TimeoutMS = def.TimeoutMS
	}
	if p.OutputPreviewBytes <= 0 {
		p.OutputPreviewBytes = def.OutputPreviewBytes
	}
	if !p.HardDenyDestructive {
		// Keep the production default conservative unless tests explicitly
		// opt into a non-default zero-value policy by setting Approval.
		if p.Approval == def.Approval && p.UnknownProgram == def.UnknownProgram && p.ShellPolicy == def.ShellPolicy {
			p.HardDenyDestructive = def.HardDenyDestructive
		}
	}
	if !p.AllowMkdirPAutoCreate {
		if p.Approval == def.Approval && p.UnknownProgram == def.UnknownProgram && p.ShellPolicy == def.ShellPolicy {
			p.AllowMkdirPAutoCreate = def.AllowMkdirPAutoCreate
		}
	}
	return p
}

func normalizeCommandStep(s CommandStep, idx int, policy CommandPolicy, planWorkDir string) CommandStep {
	if strings.TrimSpace(s.ID) == "" {
		s.ID = fmt.Sprintf("step-%d", idx+1)
	}
	if strings.TrimSpace(s.Title) == "" {
		if strings.TrimSpace(s.Program) != "" {
			s.Title = strings.TrimSpace(s.Program)
		} else {
			s.Title = "shell command"
		}
	}
	s.Program = strings.TrimSpace(s.Program)
	s.Shell = strings.TrimSpace(s.Shell)
	s.WorkDir = strings.TrimSpace(s.WorkDir)
	if s.WorkDir == "" {
		s.WorkDir = planWorkDir
	}
	s.WorkDir = filepath.Clean(s.WorkDir)
	if s.TimeoutMS <= 0 {
		s.TimeoutMS = policy.TimeoutMS
	}
	s.RiskLevel = normalizeRisk(s.RiskLevel)
	s.SideEffects = cleanList(s.SideEffects)
	return s
}

func cleanClarifyingQuestions(values []ClarifyingQuestion) []ClarifyingQuestion {
	out := make([]ClarifyingQuestion, 0, len(values))
	for i, q := range values {
		q.Question = strings.TrimSpace(q.Question)
		if q.Question == "" {
			continue
		}
		if strings.TrimSpace(q.ID) == "" {
			q.ID = fmt.Sprintf("q%d", i+1)
		}
		q.Suggestions = cleanList(q.Suggestions)
		out = append(out, q)
	}
	return out
}

func defaultOperationID() string {
	return "op-" + time.Now().UTC().Format("20060102-150405")
}

func normalizeRisk(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "none", "low", "medium", "high":
		return strings.ToLower(strings.TrimSpace(v))
	case "":
		return "low"
	default:
		return "medium"
	}
}

func highestRisk(a, b string) string {
	order := map[string]int{"none": 0, "low": 1, "medium": 2, "high": 3}
	a = normalizeRisk(a)
	b = normalizeRisk(b)
	if order[b] > order[a] {
		return b
	}
	return a
}

func knownProgram(program string) bool {
	program = baseProgram(program)
	return slices.Contains([]string{
		"pwd", "ls", "find", "stat", "du", "df", "which", "cat", "head", "tail",
		"sed", "grep", "rg", "git", "go", "node", "npm", "python", "python3",
		"mkdir", "mv", "cp", "rm", "brew", "apt", "apt-get", "pip", "pip3",
	}, program)
}

func isLowRiskProgramArgs(program string, args []string, policy CommandPolicy) bool {
	program = baseProgram(program)
	switch program {
	case "pwd", "ls", "stat", "du", "df", "which", "cat", "head", "tail", "grep", "rg":
		return !argsContainWriteOrExec(args)
	case "sed":
		return !containsArg(args, "-i") && !argsContainWriteOrExec(args)
	case "find":
		return !argsContainAny(args, "-delete", "-exec", "-execdir", "-ok", "-okdir")
	case "git":
		if len(args) == 0 {
			return false
		}
		return slices.Contains([]string{"status", "log", "show", "diff", "branch", "rev-parse"}, args[0])
	case "go":
		return len(args) > 0 && slices.Contains([]string{"version", "env", "list"}, args[0])
	case "node", "python", "python3", "npm", "pip", "pip3", "brew", "apt", "apt-get":
		return argsContainAny(args, "--version", "-version", "version")
	}
	_ = policy
	return false
}

func isMkdirPCreate(program string, args []string) bool {
	if baseProgram(program) != "mkdir" {
		return false
	}
	return argsContainAny(args, "-p", "--parents")
}

func onlyLocalFileWrite(sideEffects []string) bool {
	sideEffects = cleanList(sideEffects)
	if len(sideEffects) == 0 {
		return true
	}
	return len(sideEffects) == 1 && sideEffects[0] == "local_file_write"
}

func looksCatastrophicProgram(program string, args []string) bool {
	program = baseProgram(program)
	joined := strings.Join(args, " ")
	switch program {
	case "mkfs", "shutdown", "reboot", "halt":
		return true
	case "rm":
		return argsContainRecursiveForce(args) && argsContainRootPath(args)
	case "dd":
		return strings.Contains(joined, "of=/dev/") || strings.Contains(joined, "of=/")
	case "chmod", "chown":
		return argsContainRecursive(args) && argsContainRootPath(args)
	}
	return false
}

func looksCatastrophicShell(command string) bool {
	lower := strings.ToLower(strings.TrimSpace(command))
	return strings.Contains(lower, "rm -rf /") ||
		strings.Contains(lower, "rm -fr /") ||
		strings.Contains(lower, "mkfs.") ||
		strings.Contains(lower, "mkfs ") ||
		strings.Contains(lower, "shutdown ") ||
		strings.Contains(lower, "reboot") ||
		strings.Contains(lower, "dd if=") && strings.Contains(lower, " of=/dev/")
}

func baseProgram(program string) string {
	program = strings.TrimSpace(program)
	if program == "" {
		return ""
	}
	return filepath.Base(program)
}

func argsContainAny(args []string, needles ...string) bool {
	for _, arg := range args {
		for _, needle := range needles {
			if arg == needle {
				return true
			}
		}
	}
	return false
}

func containsArg(args []string, needle string) bool {
	for _, arg := range args {
		if arg == needle {
			return true
		}
	}
	return false
}

func argsContainWriteOrExec(args []string) bool {
	for _, arg := range args {
		if strings.Contains(arg, ">") || arg == "-exec" || arg == "-execdir" || arg == "-delete" {
			return true
		}
	}
	return false
}

func argsContainRecursiveForce(args []string) bool {
	recursive := false
	force := false
	for _, arg := range args {
		if arg == "-r" || arg == "-R" || arg == "--recursive" || strings.Contains(arg, "r") && strings.HasPrefix(arg, "-") {
			recursive = true
		}
		if arg == "-f" || arg == "--force" || strings.Contains(arg, "f") && strings.HasPrefix(arg, "-") {
			force = true
		}
	}
	return recursive && force
}

func argsContainRecursive(args []string) bool {
	for _, arg := range args {
		if arg == "-R" || arg == "-r" || arg == "--recursive" || strings.Contains(arg, "R") && strings.HasPrefix(arg, "-") {
			return true
		}
	}
	return false
}

func argsContainRootPath(args []string) bool {
	for _, arg := range args {
		cleaned := filepath.Clean(strings.TrimSpace(arg))
		if cleaned == "/" || cleaned == "/." {
			return true
		}
	}
	return false
}
