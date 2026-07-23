package types

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// WFID-1 (§29.213 排期件1): repo/task identity for write workflow auto-resume.
//
// The write workflow store is anchored at <CWD>/.codrax/plans/workflows while
// --repo is not anchored, so the same store can hold runs from different
// repositories. Before this identity existed, FindActiveRun returned the first
// non-terminal run by ModTime and loadOrSeedWriteWorkflowRun resumed it with
// zero identity comparison — repo A's active run could be silently continued
// against repo B. WriteWorkflowRepoIdentity is minted once at run seed time and
// MatchWriteWorkflowRepoIdentity is the SINGLE decision point for the
// auto-resume gate: every field is a precise signal (canonical path string,
// git SHA, branch name, stable goal hash) compared by exact equality — no
// similarity, no heuristics. Any mismatch fails closed: the run is not
// auto-resumed and the user is told how to resume it explicitly.
type WriteWorkflowRepoIdentity struct {
	// IdentitySchema marks that the identity was actually minted. Legacy run
	// files (written before WFID-1) decode to a nil pointer or schema 0 and
	// are treated as a mismatch by ruling: no auto-resume, explicit
	// /workflow resume still allowed.
	IdentitySchema int `json:"identity_schema,omitempty"`
	// CanonicalRepoRoot is the repo root after Abs+EvalSymlinks+Clean. The
	// canonicalisation single point is the read-run-snapshot canonicaliser on
	// the orchestrator side (canonicalReadRunSnapshotRepoRoot); this package
	// only stores and compares the resulting string.
	CanonicalRepoRoot string `json:"canonical_repo_root,omitempty"`
	// RepoFingerprint reuses the read-side repo fingerprint source
	// (readRunCurrentRepoFingerprint → git_head kind). Stored in full for
	// audit; the match arm compares it through
	// writeWorkflowIdentityFingerprintForMatch, which drops StatusHash — see
	// that function's comment for why dirty-state drift is deliberately not
	// part of the identity gate.
	RepoFingerprint ReadRunRepoFingerprint `json:"repo_fingerprint,omitempty"`
	// BaseHeadSHA is the main-repo HEAD commit at seed time (same git_head
	// source as RepoFingerprint.Head, kept as a plain scalar per ruling).
	BaseHeadSHA string `json:"base_head_sha,omitempty"`
	// BaseBranch is `git rev-parse --abbrev-ref HEAD` at seed time
	// ("HEAD" when detached).
	BaseBranch string `json:"base_branch,omitempty"`
	// GoalHash is WriteWorkflowGoalHash over the run's seeded goal string.
	GoalHash string `json:"goal_hash,omitempty"`
}

const WriteWorkflowRepoIdentitySchemaVersion = 1

// WriteWorkflowResumeAuthorizationExplicit is the one-shot typed token the
// explicit /workflow resume channel stamps onto a run. The next
// loadOrSeedWriteWorkflowRun consumes it: the identity gate is bypassed once,
// the identity is re-stamped ("adopted") to the current repo/base/goal
// context, and the token is cleared. Without this token the identity gate
// would make /workflow resume inert — the resumed run would be re-refused on
// the very next write turn.
const WriteWorkflowResumeAuthorizationExplicit = "explicit_user_resume"

// Typed mismatch reason codes for the auto-resume identity gate.
const (
	WriteWorkflowIdentityReasonMatch                   = "workflow_identity_match"
	WriteWorkflowIdentityReasonIdentityMissing         = "workflow_identity_missing"
	WriteWorkflowIdentityReasonRepoRootMismatch        = "workflow_repo_root_mismatch"
	WriteWorkflowIdentityReasonBaseHeadMismatch        = "workflow_base_head_mismatch"
	WriteWorkflowIdentityReasonBaseBranchMismatch      = "workflow_base_branch_mismatch"
	WriteWorkflowIdentityReasonRepoFingerprintMismatch = "workflow_repo_fingerprint_mismatch"
	WriteWorkflowIdentityReasonGoalMismatch            = "workflow_goal_mismatch"
)

// WriteWorkflowIdentityMatchDecision is the typed verdict of the single-point
// identity gate.
type WriteWorkflowIdentityMatchDecision struct {
	Match      bool
	ReasonCode string
	Detail     string
}

// WriteWorkflowIdentitySkip records an active run the identity-aware finder
// skipped, for the fail-closed user message.
type WriteWorkflowIdentitySkip struct {
	RunID      string
	Goal       string
	ReasonCode string
	Detail     string
}

// WriteWorkflowGoalHash is the stable hash of the run goal string used by the
// identity gate. Whitespace is collapsed via the shared envelope trim so the
// hash is insensitive to formatting-only differences; a 16-hex-char sha256
// prefix is ample for an equality gate.
func WriteWorkflowGoalHash(goal string) string {
	goal = trimWriteWorkflowRunText(goal)
	if goal == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("write_workflow_goal\x00" + goal))
	return hex.EncodeToString(sum[:])[:16]
}

func NormalizeWriteWorkflowRepoIdentity(in WriteWorkflowRepoIdentity) WriteWorkflowRepoIdentity {
	out := WriteWorkflowRepoIdentity{
		IdentitySchema:    in.IdentitySchema,
		CanonicalRepoRoot: strings.TrimSpace(in.CanonicalRepoRoot),
		RepoFingerprint:   NormalizeReadRunRepoFingerprint(in.RepoFingerprint),
		BaseHeadSHA:       strings.TrimSpace(in.BaseHeadSHA),
		BaseBranch:        strings.TrimSpace(in.BaseBranch),
		GoalHash:          strings.TrimSpace(in.GoalHash),
	}
	if out.IdentitySchema < 0 {
		out.IdentitySchema = 0
	}
	return out
}

func NormalizeWriteWorkflowRepoIdentityPtr(in *WriteWorkflowRepoIdentity) *WriteWorkflowRepoIdentity {
	if in == nil {
		return nil
	}
	out := NormalizeWriteWorkflowRepoIdentity(*in)
	if out.IdentitySchema == 0 && out.CanonicalRepoRoot == "" && out.BaseHeadSHA == "" &&
		out.BaseBranch == "" && out.GoalHash == "" && out.RepoFingerprint == (ReadRunRepoFingerprint{}) {
		return nil
	}
	return &out
}

// writeWorkflowIdentityFingerprintForMatch projects the stored/current repo
// fingerprint onto the identity-comparison lane: StatusHash (dirty working
// tree hash) is dropped before the shared read-side comparator runs. The
// identity gate answers "is this the same repository at the same base?", not
// "is the working tree byte-identical" — and the workflow store itself lives
// under <CWD>/.codrax, so persisting the run between turns perturbs
// `git status --untracked-files=all` output in non-ignoring repos. Gating on
// StatusHash would therefore hard-fail on tool-induced noise (precise-signals
// red line); commit-level movement is caught by the BaseHeadSHA arm and the
// fingerprint Head comparison below. The full StatusHash remains stored for
// audit.
func writeWorkflowIdentityFingerprintForMatch(fp ReadRunRepoFingerprint) ReadRunRepoFingerprint {
	fp = NormalizeReadRunRepoFingerprint(fp)
	fp.StatusHash = ""
	return fp
}

// MatchWriteWorkflowRepoIdentity is the single decision point for write
// workflow auto-resume identity. It is shared by the store-side
// FindActiveRunMatching and the orchestrator-side loadOrSeedWriteWorkflowRun —
// do not add a second comparison site. Every arm is an exact-equality
// comparison of precise signals; the first mismatching arm wins and names the
// typed reason. A nil / unstamped stored identity (legacy pre-WFID-1 run
// file) is a mismatch by ruling.
func MatchWriteWorkflowRepoIdentity(stored *WriteWorkflowRepoIdentity, current WriteWorkflowRepoIdentity) WriteWorkflowIdentityMatchDecision {
	if stored == nil || stored.IdentitySchema < WriteWorkflowRepoIdentitySchemaVersion {
		return WriteWorkflowIdentityMatchDecision{
			ReasonCode: WriteWorkflowIdentityReasonIdentityMissing,
			Detail:     "stored run carries no repo identity (legacy run file)",
		}
	}
	s := NormalizeWriteWorkflowRepoIdentity(*stored)
	c := NormalizeWriteWorkflowRepoIdentity(current)
	if s.CanonicalRepoRoot != c.CanonicalRepoRoot {
		return WriteWorkflowIdentityMatchDecision{
			ReasonCode: WriteWorkflowIdentityReasonRepoRootMismatch,
			Detail:     fmt.Sprintf("stored repo root %q, current repo root %q", s.CanonicalRepoRoot, c.CanonicalRepoRoot),
		}
	}
	if s.BaseHeadSHA != c.BaseHeadSHA {
		return WriteWorkflowIdentityMatchDecision{
			ReasonCode: WriteWorkflowIdentityReasonBaseHeadMismatch,
			Detail:     fmt.Sprintf("stored base HEAD %q, current HEAD %q", s.BaseHeadSHA, c.BaseHeadSHA),
		}
	}
	if s.BaseBranch != c.BaseBranch {
		return WriteWorkflowIdentityMatchDecision{
			ReasonCode: WriteWorkflowIdentityReasonBaseBranchMismatch,
			Detail:     fmt.Sprintf("stored base branch %q, current branch %q", s.BaseBranch, c.BaseBranch),
		}
	}
	if !ReadRunRepoFingerprintsEqual(
		writeWorkflowIdentityFingerprintForMatch(s.RepoFingerprint),
		writeWorkflowIdentityFingerprintForMatch(c.RepoFingerprint)) {
		return WriteWorkflowIdentityMatchDecision{
			ReasonCode: WriteWorkflowIdentityReasonRepoFingerprintMismatch,
			Detail:     fmt.Sprintf("stored repo fingerprint head %q, current head %q", s.RepoFingerprint.Head, c.RepoFingerprint.Head),
		}
	}
	if s.GoalHash != c.GoalHash {
		return WriteWorkflowIdentityMatchDecision{
			ReasonCode: WriteWorkflowIdentityReasonGoalMismatch,
			Detail:     fmt.Sprintf("stored goal hash %q, current goal hash %q", s.GoalHash, c.GoalHash),
		}
	}
	return WriteWorkflowIdentityMatchDecision{Match: true, ReasonCode: WriteWorkflowIdentityReasonMatch}
}

// WriteWorkflowRunHasExplicitResumeAuthorization reports whether the run
// carries the one-shot explicit-resume token at all (regardless of the repo
// context it was minted in). Consumption sites MUST use
// WriteWorkflowRunExplicitResumeAuthorizationValid instead — presence alone
// never authorizes; this predicate exists so consumption sites can tell a
// cross-context token (present but invalid, must be cleared) apart from no
// token.
func WriteWorkflowRunHasExplicitResumeAuthorization(run *WriteWorkflowRun) bool {
	return run != nil && run.ResumeAuthorization == WriteWorkflowResumeAuthorizationExplicit
}

// WriteWorkflowRunExplicitResumeAuthorizationValid is the single decision
// point for consuming the one-shot explicit-resume token: the token must be
// present AND bound to the same canonical repo root as the current context.
// Both sides are canonical roots produced by the read-run single-point
// canonicaliser at their respective mint sites (the /workflow resume stamp and
// the write-turn identity mint); the comparison here is exact string equality
// — a precise signal, no path heuristics. Two empty roots compare equal by
// design: a no-repo context can only spend a token minted in a no-repo
// context (this also keeps pre-binding legacy stamps consumable exactly where
// pre-binding semantics would have allowed: the same rootless test/unit
// context).
func WriteWorkflowRunExplicitResumeAuthorizationValid(run *WriteWorkflowRun, currentCanonicalRepoRoot string) bool {
	if !WriteWorkflowRunHasExplicitResumeAuthorization(run) {
		return false
	}
	return strings.TrimSpace(run.ResumeAuthorizedRepoRoot) == strings.TrimSpace(currentCanonicalRepoRoot)
}

func normalizeWriteWorkflowResumeAuthorization(in string) (string, bool) {
	if strings.TrimSpace(in) == WriteWorkflowResumeAuthorizationExplicit {
		return WriteWorkflowResumeAuthorizationExplicit, true
	}
	return "", false
}

func normalizeWriteWorkflowResumeAuthorizationFields(authorization, repoRoot string, at time.Time) (string, string, time.Time) {
	out, ok := normalizeWriteWorkflowResumeAuthorization(authorization)
	if !ok {
		return "", "", time.Time{}
	}
	return out, strings.TrimSpace(repoRoot), at
}
