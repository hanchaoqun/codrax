package safety

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

type EffectRole string

const (
	EffectRoleUnknown         EffectRole = "unknown"
	EffectRoleController      EffectRole = "controller"
	EffectRolePlanner         EffectRole = "planner"
	EffectRoleReplanner       EffectRole = "replanner"
	EffectRoleCoder           EffectRole = "coder"
	EffectRoleVerifier        EffectRole = "verifier"
	EffectRoleLocalizer       EffectRole = "localizer"
	EffectRoleImpactAnalyzer  EffectRole = "impact_analyzer"
	EffectRolePatchCritic     EffectRole = "patch_critic"
	EffectRoleProofAuditor    EffectRole = "proof_auditor"
	EffectRoleFailureAnalyzer EffectRole = "failure_analyzer"
	EffectRoleExplorer        EffectRole = "explorer"
)

type EffectKind string

const (
	EffectKindUnknown            EffectKind = "unknown"
	EffectKindControllerDecision EffectKind = "controller_decision"
	EffectKindApproval           EffectKind = "approval"
	EffectKindStatus             EffectKind = "status"
	EffectKindReadRepo           EffectKind = "read_repo"
	EffectKindSearchRepo         EffectKind = "search_repo"
	EffectKindRepoMap            EffectKind = "repo_map"
	EffectKindCommand            EffectKind = "command"
	EffectKindDryRunTest         EffectKind = "dry_run_test"
	EffectKindTestSuite          EffectKind = "test_suite"
	EffectKindVerifyReport       EffectKind = "verify_report"
	EffectKindPlanEmit           EffectKind = "plan_emit"
	EffectKindEdit               EffectKind = "edit"
	EffectKindApplyPatch         EffectKind = "apply_patch"
	EffectKindPatchReview        EffectKind = "patch_review"
	EffectKindImpactAnalysis     EffectKind = "impact_analysis"
	EffectKindProofAudit         EffectKind = "proof_audit"
	EffectKindFailureAnalysis    EffectKind = "failure_analysis"
	EffectKindExternalDirectory  EffectKind = "external_directory"
	EffectKindDoomLoop           EffectKind = "doom_loop"
)

// EffectDescriptor is the deterministic permission input for one agent/tool or
// runtime-unit effect. It is intentionally structural: hard policy must read
// only these typed fields, never user prose, model rationale, prompt text, or
// tool-output narrative.
type EffectDescriptor struct {
	Role               EffectRole `json:"role,omitempty"`
	Kind               EffectKind `json:"kind,omitempty"`
	Tool               string     `json:"tool,omitempty"`
	Stage              string     `json:"stage,omitempty"`
	Mode               string     `json:"mode,omitempty"`
	Paths              []string   `json:"paths,omitempty"`
	CommandClass       string     `json:"command_class,omitempty"`
	TestMode           string     `json:"test_mode,omitempty"`
	DryRun             bool       `json:"dry_run,omitempty"`
	MutatesWorktree    bool       `json:"mutates_worktree,omitempty"`
	MutatesMainRepo    bool       `json:"mutates_main_repo,omitempty"`
	ExternalDirectory  bool       `json:"external_directory,omitempty"`
	TargetsGitMetadata bool       `json:"targets_git_metadata,omitempty"`
	RequiresNetwork    bool       `json:"requires_network,omitempty"`
	RepeatCount        int        `json:"repeat_count,omitempty"`
	RepeatLimit        int        `json:"repeat_limit,omitempty"`
	Source             string     `json:"source,omitempty"`
}

type RolePermissionProfile struct {
	Role       EffectRole   `json:"role"`
	AllowKinds []EffectKind `json:"allow_kinds,omitempty"`
	AskKinds   []EffectKind `json:"ask_kinds,omitempty"`
	DenyKinds  []EffectKind `json:"deny_kinds,omitempty"`
	Source     string       `json:"source,omitempty"`
}

func NormalizeEffectRole(role EffectRole) EffectRole {
	switch EffectRole(strings.ToLower(strings.TrimSpace(string(role)))) {
	case EffectRoleController, "write_controller":
		return EffectRoleController
	case EffectRolePlanner, "plan":
		return EffectRolePlanner
	case EffectRoleReplanner, "replan":
		return EffectRoleReplanner
	case EffectRoleCoder, "writer":
		return EffectRoleCoder
	case EffectRoleVerifier, "verify":
		return EffectRoleVerifier
	case EffectRoleLocalizer:
		return EffectRoleLocalizer
	case EffectRoleImpactAnalyzer, "impact":
		return EffectRoleImpactAnalyzer
	case EffectRolePatchCritic, "critic":
		return EffectRolePatchCritic
	case EffectRoleProofAuditor, "proof":
		return EffectRoleProofAuditor
	case EffectRoleFailureAnalyzer, "failure":
		return EffectRoleFailureAnalyzer
	case EffectRoleExplorer:
		return EffectRoleExplorer
	default:
		return EffectRoleUnknown
	}
}

func NormalizeEffectKind(kind EffectKind) EffectKind {
	switch EffectKind(strings.ToLower(strings.TrimSpace(string(kind)))) {
	case EffectKindControllerDecision,
		EffectKindApproval,
		EffectKindStatus,
		EffectKindReadRepo,
		EffectKindSearchRepo,
		EffectKindRepoMap,
		EffectKindCommand,
		EffectKindDryRunTest,
		EffectKindTestSuite,
		EffectKindVerifyReport,
		EffectKindPlanEmit,
		EffectKindEdit,
		EffectKindApplyPatch,
		EffectKindPatchReview,
		EffectKindImpactAnalysis,
		EffectKindProofAudit,
		EffectKindFailureAnalysis,
		EffectKindExternalDirectory,
		EffectKindDoomLoop:
		return EffectKind(strings.ToLower(strings.TrimSpace(string(kind))))
	default:
		return EffectKindUnknown
	}
}

func NormalizeEffectDescriptor(in EffectDescriptor) EffectDescriptor {
	in.Role = NormalizeEffectRole(in.Role)
	in.Kind = NormalizeEffectKind(in.Kind)
	in.Tool = strings.TrimSpace(in.Tool)
	in.Stage = strings.TrimSpace(in.Stage)
	in.Mode = strings.TrimSpace(in.Mode)
	in.CommandClass = strings.TrimSpace(in.CommandClass)
	in.TestMode = strings.TrimSpace(in.TestMode)
	in.Source = strings.TrimSpace(in.Source)
	if in.Source == "" {
		in.Source = "effect_policy"
	}
	in.Paths = normalizeEffectPaths(in.Paths)
	if in.RepeatCount < 0 {
		in.RepeatCount = 0
	}
	if in.RepeatLimit < 0 {
		in.RepeatLimit = 0
	}
	if in.Kind == EffectKindEdit || in.Kind == EffectKindApplyPatch {
		in.MutatesWorktree = true
	}
	for _, p := range in.Paths {
		if p == ".git" || strings.HasPrefix(p, ".git/") {
			in.TargetsGitMetadata = true
			break
		}
	}
	return in
}

func EffectDescriptorForTool(role EffectRole, toolName string) EffectDescriptor {
	toolName = strings.TrimSpace(toolName)
	return NormalizeEffectDescriptor(EffectDescriptor{
		Role: NormalizeEffectRole(role),
		Kind: effectKindForTool(toolName),
		Tool: toolName,
	})
}

func BuiltinRolePermissionProfile(role EffectRole) RolePermissionProfile {
	role = NormalizeEffectRole(role)
	profile := RolePermissionProfile{Role: role, Source: "builtin_role_effect_profile"}
	switch role {
	case EffectRoleController:
		profile.AllowKinds = []EffectKind{EffectKindControllerDecision, EffectKindApproval, EffectKindStatus}
		profile.DenyKinds = []EffectKind{EffectKindCommand, EffectKindEdit, EffectKindApplyPatch, EffectKindTestSuite, EffectKindVerifyReport}
	case EffectRolePlanner, EffectRoleReplanner:
		profile.AllowKinds = []EffectKind{EffectKindReadRepo, EffectKindSearchRepo, EffectKindRepoMap, EffectKindDryRunTest, EffectKindPlanEmit}
		profile.DenyKinds = []EffectKind{EffectKindCommand, EffectKindEdit, EffectKindApplyPatch, EffectKindTestSuite, EffectKindVerifyReport}
	case EffectRoleCoder:
		profile.AllowKinds = []EffectKind{EffectKindReadRepo, EffectKindSearchRepo, EffectKindRepoMap, EffectKindEdit, EffectKindApplyPatch}
		profile.DenyKinds = []EffectKind{EffectKindCommand, EffectKindTestSuite, EffectKindVerifyReport}
	case EffectRoleVerifier:
		profile.AllowKinds = []EffectKind{EffectKindReadRepo, EffectKindTestSuite, EffectKindVerifyReport}
		profile.DenyKinds = []EffectKind{EffectKindCommand, EffectKindEdit, EffectKindApplyPatch, EffectKindDryRunTest}
	case EffectRoleLocalizer:
		profile.AllowKinds = []EffectKind{EffectKindReadRepo, EffectKindSearchRepo, EffectKindRepoMap}
		profile.DenyKinds = []EffectKind{EffectKindCommand, EffectKindEdit, EffectKindApplyPatch, EffectKindTestSuite}
	case EffectRoleImpactAnalyzer:
		profile.AllowKinds = []EffectKind{EffectKindReadRepo, EffectKindSearchRepo, EffectKindRepoMap, EffectKindImpactAnalysis}
		profile.DenyKinds = []EffectKind{EffectKindCommand, EffectKindEdit, EffectKindApplyPatch, EffectKindTestSuite}
	case EffectRolePatchCritic:
		profile.AllowKinds = []EffectKind{EffectKindReadRepo, EffectKindSearchRepo, EffectKindRepoMap, EffectKindPatchReview}
		profile.DenyKinds = []EffectKind{EffectKindCommand, EffectKindEdit, EffectKindApplyPatch, EffectKindTestSuite}
	case EffectRoleProofAuditor:
		profile.AllowKinds = []EffectKind{EffectKindReadRepo, EffectKindSearchRepo, EffectKindRepoMap, EffectKindProofAudit}
		profile.DenyKinds = []EffectKind{EffectKindCommand, EffectKindEdit, EffectKindApplyPatch, EffectKindTestSuite}
	case EffectRoleFailureAnalyzer:
		profile.AllowKinds = []EffectKind{EffectKindReadRepo, EffectKindSearchRepo, EffectKindRepoMap, EffectKindFailureAnalysis}
		profile.DenyKinds = []EffectKind{EffectKindCommand, EffectKindEdit, EffectKindApplyPatch, EffectKindTestSuite}
	default:
		profile.AskKinds = []EffectKind{EffectKindUnknown}
	}
	return profile
}

func DecideEffectPermission(effect EffectDescriptor) PermissionDecision {
	effect = NormalizeEffectDescriptor(effect)
	if effect.MutatesMainRepo {
		return DenyPermission("effect_policy", "mutates_main_repo", "effect would mutate the main repository")
	}
	if effect.TargetsGitMetadata {
		return DenyPermission("effect_policy", "git_metadata_effect", "effect targets git metadata")
	}
	if effect.ExternalDirectory {
		if effect.MutatesWorktree {
			return DenyPermission("effect_policy", "external_directory_write", "effect would write outside the controlled worktree")
		}
		return AskPermission("effect_policy", "external_directory_effect", "effect touches an external directory")
	}
	if effect.RepeatLimit > 0 && effect.RepeatCount > effect.RepeatLimit {
		if effect.MutatesWorktree {
			return DenyPermission("effect_policy", "mutating_doom_loop", "mutating effect exceeded its repetition limit")
		}
		return AskPermission("effect_policy", "effect_doom_loop", "effect exceeded its repetition limit")
	}
	profile := BuiltinRolePermissionProfile(effect.Role)
	if kindInEffectList(effect.Kind, profile.DenyKinds) {
		return DenyPermission(profile.Source, "role_effect_denied", fmt.Sprintf("%s cannot perform %s", effect.Role, effect.Kind))
	}
	if effect.Kind == EffectKindDryRunTest && (effect.Role == EffectRolePlanner || effect.Role == EffectRoleReplanner) && !effect.DryRun {
		return DenyPermission(profile.Source, "planner_probe_requires_dry_run", "planner test probes must be dry-run effects")
	}
	if kindInEffectList(effect.Kind, profile.AllowKinds) {
		return AllowPermission(profile.Source, "role_effect_allowed", fmt.Sprintf("%s may perform %s", effect.Role, effect.Kind))
	}
	if kindInEffectList(effect.Kind, profile.AskKinds) {
		return AskPermission(profile.Source, "role_effect_needs_review", fmt.Sprintf("%s requires review for %s", effect.Role, effect.Kind))
	}
	return AskPermission(profile.Source, "role_effect_unknown", fmt.Sprintf("%s has no policy for %s", effect.Role, effect.Kind))
}

func FoldEffectPermissions(source string, effects ...EffectDescriptor) PermissionDecision {
	decisions := make([]PermissionDecision, 0, len(effects))
	for _, effect := range effects {
		decisions = append(decisions, DecideEffectPermission(effect))
	}
	return FoldPermissionDecisions(source, decisions...)
}

func EffectFingerprint(effect EffectDescriptor) string {
	effect = NormalizeEffectDescriptor(effect)
	raw, err := json.Marshal(effect)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum[:])
}

func effectKindForTool(toolName string) EffectKind {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "emit_write_workflow_decision":
		return EffectKindControllerDecision
	case "read_file", "list_files":
		return EffectKindReadRepo
	case "grep":
		return EffectKindSearchRepo
	case "repo_map":
		return EffectKindRepoMap
	case "exec_command":
		return EffectKindCommand
	case "run_tests":
		return EffectKindTestSuite
	case "emit_change_plan", "emit_plan_skeleton", "emit_plan_change":
		return EffectKindPlanEmit
	case "apply_patch":
		return EffectKindApplyPatch
	case "emit_test_results":
		return EffectKindVerifyReport
	default:
		return EffectKindUnknown
	}
}

func normalizeEffectPaths(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range in {
		p = filepath.ToSlash(filepath.Clean(strings.TrimSpace(p)))
		p = strings.TrimPrefix(p, "./")
		if p == "." || p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func kindInEffectList(kind EffectKind, list []EffectKind) bool {
	kind = NormalizeEffectKind(kind)
	for _, item := range list {
		if NormalizeEffectKind(item) == kind {
			return true
		}
	}
	return false
}
