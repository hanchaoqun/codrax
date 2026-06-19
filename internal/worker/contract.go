package worker

import (
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/loopkernel"
	"github.com/hanchaoqun/codrax/internal/safety"
)

type Kind string

const (
	KindUnknown         Kind = "unknown"
	KindLocalizer       Kind = "localizer"
	KindImpactAnalyzer  Kind = "impact_analyzer"
	KindPatchCritic     Kind = "patch_critic"
	KindProofAuditor    Kind = "proof_auditor"
	KindFailureAnalyzer Kind = "failure_analyzer"
)

type Status string

const (
	StatusUnknown  Status = "unknown"
	StatusPending  Status = "pending"
	StatusComplete Status = "complete"
	StatusBlocked  Status = "blocked"
	StatusFailed   Status = "failed"
)

const (
	DefaultMaxToolCalls = 8
	DefaultMaxFiles     = 24
	DefaultMaxMillis    = 120000
)

type Scope struct {
	RepoRoot      string   `json:"repo_root,omitempty"`
	Paths         []string `json:"paths,omitempty"`
	Symbols       []string `json:"symbols,omitempty"`
	EvidenceRefs  []string `json:"evidence_refs,omitempty"`
	AllowMutation bool     `json:"allow_mutation,omitempty"`
}

type Budget struct {
	MaxToolCalls int `json:"max_tool_calls,omitempty"`
	MaxFiles     int `json:"max_files,omitempty"`
	MaxMillis    int `json:"max_millis,omitempty"`
}

type Request struct {
	ID          string                   `json:"id,omitempty"`
	RunID       string                   `json:"run_id,omitempty"`
	UnitID      string                   `json:"unit_id,omitempty"`
	Kind        Kind                     `json:"kind,omitempty"`
	Role        safety.EffectRole        `json:"role,omitempty"`
	Goal        string                   `json:"goal,omitempty"`
	Scope       Scope                    `json:"scope,omitempty"`
	InputRefs   []loopkernel.ArtifactRef `json:"input_refs,omitempty"`
	Budget      Budget                   `json:"budget,omitempty"`
	RequestedAt time.Time                `json:"requested_at,omitempty"`
}

type Result struct {
	RequestID    string                   `json:"request_id,omitempty"`
	RunID        string                   `json:"run_id,omitempty"`
	UnitID       string                   `json:"unit_id,omitempty"`
	Kind         Kind                     `json:"kind,omitempty"`
	Role         safety.EffectRole        `json:"role,omitempty"`
	Status       Status                   `json:"status,omitempty"`
	ReasonCode   string                   `json:"reason_code,omitempty"`
	OutputRefs   []loopkernel.ArtifactRef `json:"output_refs,omitempty"`
	EvidenceRefs []string                 `json:"evidence_refs,omitempty"`
	CompletedAt  time.Time                `json:"completed_at,omitempty"`
}

type ValidationError struct {
	Code    string `json:"code,omitempty"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message,omitempty"`
}

func NormalizeKind(kind Kind) Kind {
	switch Kind(strings.ToLower(strings.TrimSpace(string(kind)))) {
	case KindLocalizer:
		return KindLocalizer
	case KindImpactAnalyzer, "impact":
		return KindImpactAnalyzer
	case KindPatchCritic, "critic":
		return KindPatchCritic
	case KindProofAuditor, "proof":
		return KindProofAuditor
	case KindFailureAnalyzer, "failure":
		return KindFailureAnalyzer
	default:
		return KindUnknown
	}
}

func NormalizeStatus(status Status) Status {
	switch Status(strings.ToLower(strings.TrimSpace(string(status)))) {
	case StatusPending:
		return StatusPending
	case StatusComplete, "completed":
		return StatusComplete
	case StatusBlocked:
		return StatusBlocked
	case StatusFailed:
		return StatusFailed
	default:
		return StatusUnknown
	}
}

func RoleForKind(kind Kind) safety.EffectRole {
	switch NormalizeKind(kind) {
	case KindLocalizer:
		return safety.EffectRoleLocalizer
	case KindImpactAnalyzer:
		return safety.EffectRoleImpactAnalyzer
	case KindPatchCritic:
		return safety.EffectRolePatchCritic
	case KindProofAuditor:
		return safety.EffectRoleProofAuditor
	case KindFailureAnalyzer:
		return safety.EffectRoleFailureAnalyzer
	default:
		return safety.EffectRoleUnknown
	}
}

func EffectKindForKind(kind Kind) safety.EffectKind {
	switch NormalizeKind(kind) {
	case KindLocalizer:
		return safety.EffectKindRepoMap
	case KindImpactAnalyzer:
		return safety.EffectKindImpactAnalysis
	case KindPatchCritic:
		return safety.EffectKindPatchReview
	case KindProofAuditor:
		return safety.EffectKindProofAudit
	case KindFailureAnalyzer:
		return safety.EffectKindFailureAnalysis
	default:
		return safety.EffectKindUnknown
	}
}

func NormalizeRequest(in Request) Request {
	in.ID = strings.TrimSpace(in.ID)
	in.RunID = strings.TrimSpace(in.RunID)
	in.UnitID = strings.TrimSpace(in.UnitID)
	in.Kind = NormalizeKind(in.Kind)
	in.Role = safety.NormalizeEffectRole(in.Role)
	if in.Role == safety.EffectRoleUnknown {
		in.Role = RoleForKind(in.Kind)
	}
	in.Goal = strings.TrimSpace(in.Goal)
	in.Scope = NormalizeScope(in.Scope)
	in.InputRefs = normalizeArtifactRefs(in.InputRefs)
	in.Budget = NormalizeBudget(in.Budget)
	return in
}

func NormalizeScope(in Scope) Scope {
	in.RepoRoot = strings.TrimSpace(in.RepoRoot)
	in.Paths = normalizeStringSet(in.Paths, normalizePath)
	in.Symbols = normalizeStringSet(in.Symbols, strings.TrimSpace)
	in.EvidenceRefs = normalizeStringSet(in.EvidenceRefs, strings.TrimSpace)
	return in
}

func NormalizeBudget(in Budget) Budget {
	if in.MaxToolCalls <= 0 {
		in.MaxToolCalls = DefaultMaxToolCalls
	}
	if in.MaxFiles <= 0 {
		in.MaxFiles = DefaultMaxFiles
	}
	if in.MaxMillis <= 0 {
		in.MaxMillis = DefaultMaxMillis
	}
	return in
}

func NormalizeResult(in Result) Result {
	in.RequestID = strings.TrimSpace(in.RequestID)
	in.RunID = strings.TrimSpace(in.RunID)
	in.UnitID = strings.TrimSpace(in.UnitID)
	in.Kind = NormalizeKind(in.Kind)
	in.Role = safety.NormalizeEffectRole(in.Role)
	if in.Role == safety.EffectRoleUnknown {
		in.Role = RoleForKind(in.Kind)
	}
	in.Status = NormalizeStatus(in.Status)
	in.ReasonCode = strings.TrimSpace(in.ReasonCode)
	in.OutputRefs = normalizeArtifactRefs(in.OutputRefs)
	in.EvidenceRefs = normalizeStringSet(in.EvidenceRefs, strings.TrimSpace)
	return in
}

func RequestEffectDescriptors(in Request) []safety.EffectDescriptor {
	req := NormalizeRequest(in)
	effects := []safety.EffectDescriptor{{
		Role:              req.Role,
		Kind:              EffectKindForKind(req.Kind),
		Paths:             req.Scope.Paths,
		ExternalDirectory: scopeHasExternalPath(req.Scope.Paths),
		Source:            "worker_contract",
	}}
	if req.Scope.AllowMutation {
		effects = append(effects, safety.EffectDescriptor{
			Role:              req.Role,
			Kind:              safety.EffectKindApplyPatch,
			Paths:             req.Scope.Paths,
			MutatesWorktree:   true,
			ExternalDirectory: scopeHasExternalPath(req.Scope.Paths),
			Source:            "worker_contract",
		})
	}
	for i := range effects {
		effects[i] = safety.NormalizeEffectDescriptor(effects[i])
	}
	return effects
}

func RequestPermissionDecision(in Request) safety.PermissionDecision {
	return safety.FoldEffectPermissions("worker_contract", RequestEffectDescriptors(in)...)
}

func ValidateRequest(in Request) []ValidationError {
	req := NormalizeRequest(in)
	var errs []ValidationError
	if req.ID == "" {
		errs = append(errs, validationError("worker_request_id_required", "id", "worker request id is required"))
	}
	if req.Kind == KindUnknown {
		errs = append(errs, validationError("worker_kind_required", "kind", "worker kind must be a supported typed evidence role"))
	}
	expectedRole := RoleForKind(req.Kind)
	if expectedRole == safety.EffectRoleUnknown {
		errs = append(errs, validationError("worker_role_unknown", "role", "worker role is unknown"))
	} else if req.Role != expectedRole {
		errs = append(errs, validationError("worker_role_kind_mismatch", "role", "worker role must match worker kind"))
	}
	if req.Goal == "" {
		errs = append(errs, validationError("worker_goal_required", "goal", "worker goal is required"))
	}
	if req.Scope.AllowMutation {
		errs = append(errs, validationError("worker_mutation_not_allowed", "scope.allow_mutation", "evidence workers cannot request mutation authority"))
	}
	for _, effect := range RequestEffectDescriptors(req) {
		decision := safety.DecideEffectPermission(effect)
		if decision.Action != safety.PermissionAllow {
			errs = append(errs, validationError(decision.ReasonCode, "effect", decision.Reason))
		}
	}
	return errs
}

func ValidateResult(in Result) []ValidationError {
	result := NormalizeResult(in)
	var errs []ValidationError
	if result.RequestID == "" {
		errs = append(errs, validationError("worker_result_request_id_required", "request_id", "worker result request id is required"))
	}
	if result.Kind == KindUnknown {
		errs = append(errs, validationError("worker_result_kind_required", "kind", "worker result kind must be a supported typed evidence role"))
	}
	expectedRole := RoleForKind(result.Kind)
	if expectedRole == safety.EffectRoleUnknown {
		errs = append(errs, validationError("worker_result_role_unknown", "role", "worker result role is unknown"))
	} else if result.Role != expectedRole {
		errs = append(errs, validationError("worker_result_role_kind_mismatch", "role", "worker result role must match worker kind"))
	}
	if result.Status == StatusUnknown {
		errs = append(errs, validationError("worker_result_status_required", "status", "worker result status is required"))
	}
	if result.Status == StatusComplete && len(result.OutputRefs) == 0 && len(result.EvidenceRefs) == 0 {
		errs = append(errs, validationError("worker_result_typed_output_required", "output_refs", "completed worker result must carry typed output or evidence refs"))
	}
	if (result.Status == StatusBlocked || result.Status == StatusFailed) && result.ReasonCode == "" {
		errs = append(errs, validationError("worker_result_reason_required", "reason_code", "blocked or failed worker result must carry a typed reason code"))
	}
	return errs
}

func validationError(code, field, message string) ValidationError {
	return ValidationError{
		Code:    strings.TrimSpace(code),
		Field:   strings.TrimSpace(field),
		Message: strings.TrimSpace(message),
	}
}

func normalizeArtifactRefs(in []loopkernel.ArtifactRef) []loopkernel.ArtifactRef {
	seen := map[string]bool{}
	var out []loopkernel.ArtifactRef
	for _, ref := range in {
		ref.Kind = strings.TrimSpace(ref.Kind)
		ref.ID = strings.TrimSpace(ref.ID)
		ref.Path = normalizePath(ref.Path)
		if ref.Kind == "" && ref.ID == "" && ref.Path == "" {
			continue
		}
		key := ref.Kind + "\x00" + ref.ID + "\x00" + ref.Path
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, ref)
	}
	sort.Slice(out, func(i, j int) bool {
		a := out[i].Kind + "\x00" + out[i].ID + "\x00" + out[i].Path
		b := out[j].Kind + "\x00" + out[j].ID + "\x00" + out[j].Path
		return a < b
	})
	return out
}

func normalizeStringSet(in []string, normalize func(string) string) []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range in {
		item = normalize(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = filepath.ToSlash(filepath.Clean(path))
	if !pathLooksAbsolute(path) {
		path = strings.TrimPrefix(path, "./")
	}
	if path == "." {
		return ""
	}
	return path
}

func scopeHasExternalPath(paths []string) bool {
	for _, path := range paths {
		if pathLooksAbsolute(path) || path == ".." || strings.HasPrefix(path, "../") {
			return true
		}
	}
	return false
}

func pathLooksAbsolute(path string) bool {
	path = filepath.ToSlash(strings.TrimSpace(path))
	if filepath.IsAbs(path) || strings.HasPrefix(path, "/") {
		return true
	}
	return len(path) >= 2 && path[1] == ':'
}
