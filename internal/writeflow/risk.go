// Package writeflow contains deterministic write-mode workflow helpers.
//
// The package deliberately reads only structured write artifacts such as
// ChangePlan and WriteAnalysisIR. It must not infer user intent from prose
// keywords; noisy task language belongs in model prompts and soft guidance.
package writeflow

import (
	"fmt"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// RiskLevel is the deterministic system-side write risk band.
type RiskLevel string

const (
	RiskNone     RiskLevel = "none"
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

// ApprovalPolicy describes how a write plan would be handled once the
// workflow approval layer is wired into apply. Batch 2 only renders this as a
// preview; it does not change the current apply behavior.
type ApprovalPolicy string

const (
	ApprovalPolicyManual      ApprovalPolicy = "manual"
	ApprovalPolicyAutoSafe    ApprovalPolicy = "auto_safe"
	ApprovalPolicyAutoLowOnly ApprovalPolicy = "auto_low_only"
)

// ApprovalAction is the deterministic result for a policy + assessment pair.
type ApprovalAction string

const (
	ApprovalActionAutoExecute ApprovalAction = "auto_execute"
	ApprovalActionManual      ApprovalAction = "manual_approval"
	ApprovalActionDeny        ApprovalAction = "deny"
)

// RiskReason explains one deterministic contribution to the risk level.
type RiskReason struct {
	Code   string    `json:"code"`
	Detail string    `json:"detail,omitempty"`
	Path   string    `json:"path,omitempty"`
	Level  RiskLevel `json:"level"`
}

// RiskAssessment is the system-side risk summary for a write plan.
type RiskAssessment struct {
	Level   RiskLevel    `json:"level"`
	Reasons []RiskReason `json:"reasons,omitempty"`
}

// ApprovalDecision is the typed approval result for a risk assessment.
type ApprovalDecision struct {
	Action     ApprovalAction `json:"action"`
	Policy     ApprovalPolicy `json:"policy"`
	ReasonCode string         `json:"reason_code,omitempty"`
	Reason     string         `json:"reason,omitempty"`
}

// AssessmentInput supplies structured write artifacts to the risk assessor.
type AssessmentInput struct {
	Plan *types.ChangePlan
}

// AssessWriteRisk computes deterministic write risk from plan shape, path
// class, change kind, and WriteAnalysisIR risk axes. It intentionally does not
// inspect natural-language request text or rationale prose.
func AssessWriteRisk(input AssessmentInput) RiskAssessment {
	a := RiskAssessment{Level: RiskNone}
	plan := input.Plan
	if plan == nil {
		a.add(RiskLow, "missing_plan", "no change plan is available", "")
		return a.normalized()
	}
	if len(plan.Changes) == 0 {
		a.add(RiskLow, "empty_plan", "plan contains no file changes", "")
		return a.normalized()
	}

	changeCount := len(plan.Changes)
	switch {
	case changeCount >= 10:
		a.add(RiskHigh, "large_change_set", fmt.Sprintf("%d planned file changes", changeCount), "")
	case changeCount >= 4:
		a.add(RiskMedium, "multi_file_change_set", fmt.Sprintf("%d planned file changes", changeCount), "")
	default:
		a.add(RiskLow, "small_change_set", fmt.Sprintf("%d planned file change(s)", changeCount), "")
	}

	for _, c := range plan.Changes {
		a.assessPath(c.Path)
		a.assessPathPolicyDetails(c.Path)
		if c.NewPath != "" {
			a.assessPath(c.NewPath)
			a.assessPathPolicyDetails(c.NewPath)
		}
		switch strings.ToLower(strings.TrimSpace(c.Kind)) {
		case "delete":
			a.add(RiskHigh, "delete_change", "plan deletes a file", c.Path)
		case "rename":
			a.add(RiskMedium, "rename_change", "plan renames a file", c.Path)
		case "patch", "modify":
			a.add(riskForPath(c.Path), "modify_change", "plan modifies an existing file", c.Path)
		case "create":
			a.add(riskForPath(c.Path), "create_change", "plan creates a file", c.Path)
		default:
			a.add(RiskMedium, "unknown_change_kind", "plan uses an unrecognized change kind", c.Path)
		}
	}

	if ir := plan.WriteAnalysisIR; ir != nil {
		if ir.Request.Risk.AffectsPublicAPI {
			a.add(RiskHigh, "affects_public_api", "write analyzer marked public API impact", "")
		}
		if ir.Request.Risk.ChangesPersistence {
			a.add(RiskHigh, "changes_persistence", "write analyzer marked persistence impact", "")
		}
		if ir.Request.Risk.ChangesBuildSystem {
			a.add(RiskHigh, "changes_build_system", "write analyzer marked build-system impact", "")
		}
		switch ir.Request.Risk.Overall {
		case types.RiskBandHigh:
			a.add(RiskHigh, "write_analyzer_high", "write analyzer overall risk is high", "")
		case types.RiskBandMedium:
			a.add(RiskMedium, "write_analyzer_medium", "write analyzer overall risk is medium", "")
		case types.RiskBandLow:
			a.add(RiskLow, "write_analyzer_low", "write analyzer overall risk is low", "")
		}
	}

	return a.normalized()
}

// DecideWriteApproval applies policy semantics to an assessment. Critical risk
// is always denied; high risk always needs manual approval.
func DecideWriteApproval(policy ApprovalPolicy, assessment RiskAssessment) ApprovalDecision {
	policy = NormalizeApprovalPolicy(policy)
	if assessment.Level == RiskCritical {
		return ApprovalDecision{
			Action:     ApprovalActionDeny,
			Policy:     policy,
			ReasonCode: "critical_write_risk",
			Reason:     "critical write risk is rejected before apply",
		}
	}
	if assessment.Level == RiskHigh {
		return ApprovalDecision{
			Action:     ApprovalActionManual,
			Policy:     policy,
			ReasonCode: "high_write_risk",
			Reason:     "high write risk requires manual approval",
		}
	}
	switch policy {
	case ApprovalPolicyAutoSafe:
		return ApprovalDecision{
			Action:     ApprovalActionAutoExecute,
			Policy:     policy,
			ReasonCode: "auto_safe_allows_low_medium",
			Reason:     "auto_safe may auto-execute low and medium write risk",
		}
	case ApprovalPolicyAutoLowOnly:
		if assessment.Level == RiskLow || assessment.Level == RiskNone {
			return ApprovalDecision{
				Action:     ApprovalActionAutoExecute,
				Policy:     policy,
				ReasonCode: "auto_low_only_allows_low",
				Reason:     "auto_low_only may auto-execute low write risk",
			}
		}
		return ApprovalDecision{
			Action:     ApprovalActionManual,
			Policy:     policy,
			ReasonCode: "auto_low_only_blocks_medium",
			Reason:     "auto_low_only requires manual approval for medium write risk",
		}
	default:
		return ApprovalDecision{
			Action:     ApprovalActionManual,
			Policy:     policy,
			ReasonCode: "manual_policy",
			Reason:     "manual policy requires approval before apply",
		}
	}
}

// NormalizeApprovalPolicy returns a safe policy default.
func NormalizeApprovalPolicy(policy ApprovalPolicy) ApprovalPolicy {
	switch policy {
	case ApprovalPolicyManual, ApprovalPolicyAutoSafe, ApprovalPolicyAutoLowOnly:
		return policy
	default:
		return ApprovalPolicyManual
	}
}

// TopReasons returns a stable risk reason preview.
func (a RiskAssessment) TopReasons(limit int) []RiskReason {
	if limit <= 0 || len(a.Reasons) == 0 {
		return nil
	}
	reasons := append([]RiskReason(nil), a.Reasons...)
	sort.SliceStable(reasons, func(i, j int) bool {
		if rankRisk(reasons[i].Level) != rankRisk(reasons[j].Level) {
			return rankRisk(reasons[i].Level) > rankRisk(reasons[j].Level)
		}
		if reasons[i].Code != reasons[j].Code {
			return reasons[i].Code < reasons[j].Code
		}
		return reasons[i].Path < reasons[j].Path
	})
	if len(reasons) > limit {
		reasons = reasons[:limit]
	}
	return reasons
}

func (a *RiskAssessment) assessPath(path string) {
	clean, ok := normalizePlanPath(path)
	if !ok {
		a.add(RiskHigh, "empty_path", "planned change has an empty path", path)
		return
	}
	if looksAbsolutePlanPath(path) || clean == ".." || strings.HasPrefix(clean, "../") {
		a.add(RiskCritical, "outside_repo_path", "planned path is not repo-relative", path)
		return
	}
	if clean == ".git" || strings.HasPrefix(clean, ".git/") {
		a.add(RiskCritical, "git_metadata_path", "planned path targets git metadata", path)
		return
	}
	a.add(riskForCleanPath(clean), "path_class", pathClass(clean), path)
}

func (a *RiskAssessment) assessPathPolicyDetails(path string) {
	clean, ok := normalizePlanPath(path)
	if !ok || looksAbsolutePlanPath(path) || clean == ".." || strings.HasPrefix(clean, "../") || clean == ".git" || strings.HasPrefix(clean, ".git/") {
		return
	}
	base := strings.ToLower(filepath.Base(clean))
	switch {
	case isWorkflowOrAutomationPath(clean, base):
		a.add(RiskHigh, "ci_or_workflow_change", "plan changes CI/workflow automation", path)
	case isHookPolicyPath(clean, base):
		a.add(RiskHigh, "hook_policy_change", "plan changes hook or local automation policy", path)
	case isExecutableScriptPath(clean, base):
		a.add(RiskMedium, "executable_script_change", "plan changes executable/script surface", path)
	}
}

func (a *RiskAssessment) add(level RiskLevel, code, detail, path string) {
	if rankRisk(level) > rankRisk(a.Level) {
		a.Level = level
	}
	a.Reasons = append(a.Reasons, RiskReason{
		Code:   code,
		Detail: detail,
		Path:   path,
		Level:  level,
	})
}

func (a RiskAssessment) normalized() RiskAssessment {
	if a.Level == "" {
		a.Level = RiskNone
	}
	seen := map[string]struct{}{}
	out := make([]RiskReason, 0, len(a.Reasons))
	for _, r := range a.Reasons {
		key := string(r.Level) + "\x00" + r.Code + "\x00" + r.Detail + "\x00" + r.Path
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, r)
	}
	a.Reasons = out
	return a
}

func riskForPath(path string) RiskLevel {
	clean, ok := normalizePlanPath(path)
	if !ok {
		return RiskHigh
	}
	if looksAbsolutePlanPath(path) || clean == ".." || strings.HasPrefix(clean, "../") || clean == ".git" || strings.HasPrefix(clean, ".git/") {
		return RiskCritical
	}
	return riskForCleanPath(clean)
}

func riskForCleanPath(clean string) RiskLevel {
	base := strings.ToLower(filepath.Base(clean))
	if isBuildOrDependencyManifest(base) {
		return RiskHigh
	}
	if isSecretLikePath(clean, base) {
		return RiskHigh
	}
	if isWorkflowOrAutomationPath(clean, base) || isHookPolicyPath(clean, base) {
		return RiskHigh
	}
	if isExecutableScriptPath(clean, base) {
		return RiskMedium
	}
	if isDocsPath(clean) || isTestPath(clean) {
		return RiskLow
	}
	return RiskMedium
}

func pathClass(clean string) string {
	base := strings.ToLower(filepath.Base(clean))
	switch {
	case isBuildOrDependencyManifest(base):
		return "build_or_dependency_manifest"
	case isSecretLikePath(clean, base):
		return "secret_or_credential_like_path"
	case isWorkflowOrAutomationPath(clean, base):
		return "ci_or_workflow_path"
	case isHookPolicyPath(clean, base):
		return "hook_policy_path"
	case isExecutableScriptPath(clean, base):
		return "executable_script_path"
	case isDocsPath(clean):
		return "documentation_path"
	case isTestPath(clean):
		return "test_path"
	default:
		return "source_or_config_path"
	}
}

func normalizePlanPath(path string) (string, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", false
	}
	clean := pathpkg.Clean(strings.ReplaceAll(path, "\\", "/"))
	return clean, true
}

func looksAbsolutePlanPath(path string) bool {
	trimmed := strings.TrimSpace(path)
	if filepath.IsAbs(trimmed) || strings.HasPrefix(trimmed, "/") {
		return true
	}
	normalized := strings.ReplaceAll(trimmed, "\\", "/")
	return len(normalized) >= 3 &&
		((normalized[0] >= 'a' && normalized[0] <= 'z') || (normalized[0] >= 'A' && normalized[0] <= 'Z')) &&
		normalized[1] == ':' &&
		normalized[2] == '/'
}

func isDocsPath(clean string) bool {
	lower := strings.ToLower(clean)
	if strings.HasPrefix(lower, "docs/") || strings.Contains(lower, "/docs/") {
		return true
	}
	switch strings.ToLower(filepath.Ext(lower)) {
	case ".md", ".mdx", ".rst", ".txt", ".adoc":
		return true
	default:
		return false
	}
}

func isTestPath(clean string) bool {
	lower := strings.ToLower(clean)
	base := filepath.Base(lower)
	return strings.Contains(lower, "/test/") ||
		strings.Contains(lower, "/tests/") ||
		strings.Contains(lower, "/__tests__/") ||
		strings.HasSuffix(base, "_test.go") ||
		strings.HasSuffix(base, ".test.ts") ||
		strings.HasSuffix(base, ".test.tsx") ||
		strings.HasSuffix(base, ".spec.ts") ||
		strings.HasSuffix(base, ".spec.tsx") ||
		strings.HasSuffix(base, "_test.py") ||
		strings.HasSuffix(base, "test.java")
}

func isBuildOrDependencyManifest(base string) bool {
	switch base {
	case "go.mod", "go.sum", "package.json", "package-lock.json", "pnpm-lock.yaml",
		"yarn.lock", "bun.lockb", "cargo.toml", "cargo.lock", "pyproject.toml",
		"poetry.lock", "requirements.txt", "pom.xml", "build.gradle",
		"settings.gradle", "gradle.properties", "cmakelists.txt", "makefile",
		"dockerfile", "compose.yaml", "compose.yml", "docker-compose.yaml",
		"docker-compose.yml", "oh-package.json5", "hvigorfile.ts":
		return true
	default:
		return false
	}
}

func isSecretLikePath(clean, base string) bool {
	if base == ".env" || strings.HasPrefix(base, ".env.") {
		return true
	}
	switch filepath.Ext(base) {
	case ".pem", ".key", ".p12", ".pfx":
		return true
	}
	return strings.Contains(clean, "/.ssh/") || base == "id_rsa" || base == "id_ed25519"
}

func isWorkflowOrAutomationPath(clean, base string) bool {
	lower := strings.ToLower(clean)
	if strings.HasPrefix(lower, ".github/workflows/") ||
		strings.HasPrefix(lower, ".circleci/") ||
		strings.HasPrefix(lower, ".buildkite/") ||
		strings.HasPrefix(lower, ".azure-pipelines/") ||
		strings.HasPrefix(lower, ".woodpecker/") ||
		strings.HasPrefix(lower, ".drone/") ||
		strings.HasPrefix(lower, "fastlane/") {
		return true
	}
	switch base {
	case ".gitlab-ci.yml", ".gitlab-ci.yaml", "jenkinsfile", "azure-pipelines.yml",
		"azure-pipelines.yaml", "bitrise.yml", "bitrise.yaml", "codemagic.yaml",
		"codemagic.yml", ".travis.yml", "appveyor.yml", "buildkite.yml",
		"buildkite.yaml", ".drone.yml", ".drone.yaml", ".woodpecker.yml",
		".woodpecker.yaml":
		return true
	default:
		return false
	}
}

func isHookPolicyPath(clean, base string) bool {
	lower := strings.ToLower(clean)
	if strings.HasPrefix(lower, ".husky/") ||
		strings.HasPrefix(lower, ".githooks/") ||
		strings.HasPrefix(lower, "hooks/") ||
		strings.Contains(lower, "/hooks/") {
		return true
	}
	switch base {
	case ".pre-commit-config.yaml", ".pre-commit-config.yml", "lefthook.yml",
		"lefthook.yaml", "pre-commit", "pre-push", "commit-msg", "post-checkout",
		"post-merge", "post-rewrite":
		return true
	default:
		return false
	}
}

func isExecutableScriptPath(clean, base string) bool {
	lower := strings.ToLower(clean)
	if strings.HasPrefix(lower, "scripts/") || strings.Contains(lower, "/scripts/") ||
		strings.HasPrefix(lower, "bin/") || strings.Contains(lower, "/bin/") {
		return true
	}
	switch strings.ToLower(filepath.Ext(base)) {
	case ".sh", ".bash", ".zsh", ".fish", ".ps1", ".cmd", ".bat", ".command":
		return true
	default:
		return false
	}
}

func rankRisk(level RiskLevel) int {
	switch level {
	case RiskCritical:
		return 4
	case RiskHigh:
		return 3
	case RiskMedium:
		return 2
	case RiskLow:
		return 1
	default:
		return 0
	}
}
