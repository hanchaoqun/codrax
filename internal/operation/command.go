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

// CommandPolicy controls command-operation approval. Defaults let ordinary
// non-high-risk command steps run without interrupting the user, while
// high-risk actions require policy review and catastrophic commands are
// hard-denied.
type CommandPolicy struct {
	Approval              string
	AutoApprove           bool
	AutoLowRisk           bool
	UnknownProgram        string
	ShellPolicy           string
	NetworkPolicy         string
	InstallPolicy         string
	OverwritePolicy       string
	AllowedWriteRoots     []string
	TimeoutMS             int
	OutputPreviewBytes    int
	DefaultWorkDir        string
	HardDenyDestructive   bool
	AllowMkdirPAutoCreate bool
}

func DefaultCommandPolicy() CommandPolicy {
	return CommandPolicy{
		Approval:              ApprovalManual,
		AutoApprove:           true,
		AutoLowRisk:           true,
		UnknownProgram:        ApprovalManual,
		ShellPolicy:           ApprovalManual,
		NetworkPolicy:         ApprovalManual,
		InstallPolicy:         ApprovalManual,
		OverwritePolicy:       ApprovalManual,
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
	ContinueAfter        bool
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
	ContinueAfter       bool
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
	FailureClass  string
	Verification  OperationVerificationResult
}

type CommandOperationResult struct {
	PlanID        string
	Status        OperationStatus
	StepResults   []CommandStepResult
	OutputPreview string
	PayloadRef    string
}

type OperationVerificationResult struct {
	Status  string
	Kind    string
	Path    string
	Summary string
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
		ContinueAfter:       req.ContinueAfter,
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
		if policy.ShellPolicy == ApprovalDenied {
			return commandStepDecision{AutoApproval: StepAutoDenied, RiskLevel: "high", Reason: "shell-form commands are denied by command policy"}
		}
		if policy.NetworkPolicy == ApprovalDenied && hasNetworkEffect("", step.SideEffects) {
			return commandStepDecision{AutoApproval: StepAutoDenied, RiskLevel: "high", Reason: "network operation is denied by command policy"}
		}
		if policy.InstallPolicy == ApprovalDenied && hasInstallEffect("", nil, step.SideEffects) {
			return commandStepDecision{AutoApproval: StepAutoDenied, RiskLevel: "high", Reason: "install or uninstall operation is denied by command policy"}
		}
		if policy.OverwritePolicy == ApprovalDenied && hasOverwriteEffect("", nil, step.SideEffects) {
			return commandStepDecision{AutoApproval: StepAutoDenied, RiskLevel: "high", Reason: "overwrite operation is denied by command policy"}
		}
		if len(policy.AllowedWriteRoots) > 0 && hasLocalWriteEffect(step.SideEffects) {
			return commandStepDecision{AutoApproval: StepAutoDenied, RiskLevel: "high", Reason: "shell-form local writes cannot be proven within configured allowed write roots"}
		}
		if stepHasHighRiskSignals("", nil, step) {
			return commandStepDecision{AutoApproval: StepAutoManual, RiskLevel: "high", Reason: "shell-form high-risk command requires manual approval"}
		}
		if policy.AutoApprove {
			return commandStepDecision{AutoApproval: StepAutoEligible, RiskLevel: highestRisk(step.RiskLevel, "medium"), Reason: "operation auto-approval enabled; shell command has no high-risk or hard-deny signal"}
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
	if policy.NetworkPolicy == ApprovalDenied && hasNetworkEffect(program, step.SideEffects) {
		return commandStepDecision{AutoApproval: StepAutoDenied, RiskLevel: "high", Reason: "network operation is denied by command policy"}
	}
	if policy.InstallPolicy == ApprovalDenied && hasInstallEffect(program, step.Args, step.SideEffects) {
		return commandStepDecision{AutoApproval: StepAutoDenied, RiskLevel: "high", Reason: "install or uninstall operation is denied by command policy"}
	}
	if policy.OverwritePolicy == ApprovalDenied && hasOverwriteEffect(program, step.Args, step.SideEffects) {
		return commandStepDecision{AutoApproval: StepAutoDenied, RiskLevel: "high", Reason: "overwrite operation is denied by command policy"}
	}
	if len(policy.AllowedWriteRoots) > 0 && hasLocalWriteEffect(step.SideEffects) {
		if !stepLocalWritesWithinRoots(step, policy.AllowedWriteRoots) {
			return commandStepDecision{AutoApproval: StepAutoDenied, RiskLevel: "high", Reason: "local file write target is outside configured allowed write roots or cannot be proven within them"}
		}
	}
	if stepHasHighRiskSignals(program, step.Args, step) {
		return commandStepDecision{AutoApproval: StepAutoManual, RiskLevel: "high", Reason: "high-risk operation requires manual approval"}
	}
	if policy.AutoApprove {
		return commandStepDecision{AutoApproval: StepAutoEligible, RiskLevel: highestRisk(step.RiskLevel, "low"), Reason: "operation auto-approval enabled; no high-risk or hard-deny signal matched"}
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
	p.NetworkPolicy = normalizeManualDenyPolicy(p.NetworkPolicy, def.NetworkPolicy)
	p.InstallPolicy = normalizeManualDenyPolicy(p.InstallPolicy, def.InstallPolicy)
	p.OverwritePolicy = normalizeManualDenyPolicy(p.OverwritePolicy, def.OverwritePolicy)
	p.AllowedWriteRoots = cleanList(p.AllowedWriteRoots)
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
	// AutoApprove is intentionally not inferred here: production callers seed
	// DefaultCommandPolicy before applying YAML pointer overrides, while tests
	// and direct callers can pass a zero-value policy to exercise manual-only
	// behavior. This avoids conflating an explicit false with an omitted field.
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
		"uname", "sw_vers", "sysctl", "system_profiler", "nproc", "lscpu", "free", "vm_stat",
		"mkdir", "touch", "mv", "cp", "rm", "tee", "curl", "wget", "ssh", "scp",
		"rsync", "brew", "apt", "apt-get", "pip", "pip3", "cargo",
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
	case "uname", "sw_vers", "nproc", "lscpu", "free", "vm_stat", "system_profiler":
		return !argsContainWriteOrExec(args)
	case "sysctl":
		return sysctlArgsReadOnly(args)
	case "node", "python", "python3", "npm", "pip", "pip3", "brew", "apt", "apt-get":
		return argsContainAny(args, "--version", "-version", "version")
	}
	_ = policy
	return false
}

func sysctlArgsReadOnly(args []string) bool {
	if len(args) == 0 {
		return true
	}
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			continue
		}
		if arg == "-w" || strings.HasPrefix(arg, "-w") || strings.Contains(arg, "=") {
			return false
		}
		if strings.Contains(arg, ">") || arg == "-exec" || arg == "-execdir" || arg == "-delete" {
			return false
		}
	}
	return true
}

func stepHasHighRiskSignals(program string, args []string, step CommandStep) bool {
	if normalizeRisk(step.RiskLevel) == "high" {
		return true
	}
	for _, effect := range cleanList(step.SideEffects) {
		switch effect {
		case "destructive", "external_system_write", "network_submit", "remote_exec",
			"install", "uninstall", "package_install", "package_uninstall",
			"software_install", "software_uninstall", "local_file_delete",
			"file_delete", "local_file_overwrite", "file_overwrite",
			"destructive_write":
			return true
		}
	}
	if hasInstallEffect(program, args, step.SideEffects) || hasOverwriteEffect(program, args, step.SideEffects) {
		return true
	}
	if baseProgram(program) == "sysctl" && !sysctlArgsReadOnly(args) {
		return true
	}
	switch baseProgram(program) {
	case "rm", "rmdir", "mv", "dd", "mkfs", "chmod", "chown", "sudo", "su":
		return true
	}
	return false
}

func normalizeManualDenyPolicy(v, def string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case ApprovalManual:
		return ApprovalManual
	case ApprovalDenied, "deny":
		return ApprovalDenied
	case "":
		return def
	default:
		return def
	}
}

func hasNetworkEffect(program string, sideEffects []string) bool {
	for _, effect := range cleanList(sideEffects) {
		switch effect {
		case "network", "network_read", "network_write", "network_submit", "download", "upload", "remote_exec":
			return true
		}
	}
	switch baseProgram(program) {
	case "curl", "wget", "ssh", "scp", "rsync":
		return true
	}
	return false
}

func hasInstallEffect(program string, args []string, sideEffects []string) bool {
	for _, effect := range cleanList(sideEffects) {
		switch effect {
		case "install", "uninstall", "package_install", "package_uninstall", "software_install", "software_uninstall":
			return true
		}
	}
	if len(args) == 0 {
		return false
	}
	first := strings.ToLower(strings.TrimSpace(args[0]))
	switch baseProgram(program) {
	case "apt", "apt-get":
		return slices.Contains([]string{"install", "remove", "purge", "autoremove"}, first)
	case "brew":
		return slices.Contains([]string{"install", "uninstall", "remove", "upgrade"}, first)
	case "npm", "pip", "pip3", "cargo":
		return slices.Contains([]string{"install", "uninstall", "remove", "update"}, first)
	case "go":
		return first == "install"
	}
	return false
}

func hasOverwriteEffect(program string, args []string, sideEffects []string) bool {
	for _, effect := range cleanList(sideEffects) {
		switch effect {
		case "overwrite", "file_overwrite", "local_file_overwrite", "destructive_write":
			return true
		}
	}
	switch baseProgram(program) {
	case "cp", "mv":
		return argsContainAny(args, "-f", "--force")
	case "tee":
		return !argsContainAny(args, "-a", "--append")
	}
	return false
}

func hasLocalWriteEffect(sideEffects []string) bool {
	for _, effect := range cleanList(sideEffects) {
		switch effect {
		case "local_file_write", "file_write", "directory_write", "local_file_overwrite", "local_file_delete", "file_delete", "install", "uninstall", "package_install", "package_uninstall":
			return true
		}
	}
	return false
}

func stepLocalWritesWithinRoots(step CommandStep, roots []string) bool {
	paths := commandWritePathCandidates(step)
	if len(paths) == 0 {
		return false
	}
	for _, p := range paths {
		if !pathWithinAnyRoot(p, step.WorkDir, roots) {
			return false
		}
	}
	return true
}

func commandWritePathCandidates(step CommandStep) []string {
	program := baseProgram(step.Program)
	operands := nonFlagArgs(step.Args)
	switch program {
	case "mkdir", "touch", "rm", "tee":
		return operands
	case "cp", "mv":
		if len(operands) == 0 {
			return nil
		}
		return []string{operands[len(operands)-1]}
	}
	return nil
}

func nonFlagArgs(args []string) []string {
	var out []string
	stopFlags := false
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			continue
		}
		if arg == "--" {
			stopFlags = true
			continue
		}
		if !stopFlags && strings.HasPrefix(arg, "-") {
			continue
		}
		out = append(out, arg)
	}
	return out
}

func pathWithinAnyRoot(pathValue, workDir string, roots []string) bool {
	candidate := strings.TrimSpace(pathValue)
	if candidate == "" {
		return false
	}
	if !filepath.IsAbs(candidate) {
		base := strings.TrimSpace(workDir)
		if base == "" {
			base = "."
		}
		candidate = filepath.Join(base, candidate)
	}
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	candidateAbs = filepath.Clean(candidateAbs)
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		if !filepath.IsAbs(root) {
			root = filepath.Join(".", root)
		}
		rootAbs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		rootAbs = filepath.Clean(rootAbs)
		if candidateAbs == rootAbs {
			return true
		}
		if rel, err := filepath.Rel(rootAbs, candidateAbs); err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." {
			return true
		}
	}
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

func shellStartsWithNoInputFilter(command string) bool {
	first := firstShellPipelineSegment(command)
	if first == "" || strings.Contains(first, "<") {
		return false
	}
	tokens := strings.Fields(first)
	if len(tokens) == 0 {
		return false
	}
	program := baseProgram(tokens[0])
	positionals := shellPositionalArgs(tokens[1:])
	switch program {
	case "grep", "egrep", "fgrep":
		// grep needs at least pattern + file when it is the first pipeline
		// segment. A lone pattern reads stdin and yields a false negative in
		// non-interactive command execution.
		return len(positionals) < 2
	case "awk", "sed", "perl":
		// script/expression + file operand.
		return len(positionals) < 2
	case "head", "tail", "wc", "cut", "sort", "uniq":
		return len(positionals) < 1
	default:
		return false
	}
}

func firstShellPipelineSegment(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return ""
	}
	stop := len(command)
	for _, sep := range []string{"|", "||", "&&", ";"} {
		if idx := strings.Index(command, sep); idx >= 0 && idx < stop {
			stop = idx
		}
	}
	return strings.TrimSpace(command[:stop])
}

func shellPositionalArgs(args []string) []string {
	var out []string
	stopFlags := false
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			continue
		}
		if arg == "--" {
			stopFlags = true
			continue
		}
		if !stopFlags && strings.HasPrefix(arg, "-") {
			continue
		}
		out = append(out, arg)
	}
	return out
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
