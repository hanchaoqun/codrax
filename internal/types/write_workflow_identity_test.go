package types

import (
	"encoding/json"
	"testing"
	"time"
)

func testIdentityFixture() WriteWorkflowRepoIdentity {
	return WriteWorkflowRepoIdentity{
		IdentitySchema:    WriteWorkflowRepoIdentitySchemaVersion,
		CanonicalRepoRoot: "/srv/checkouts/repo-a",
		RepoFingerprint: ReadRunRepoFingerprint{
			Kind:       ReadRunRepoFingerprintKindGitHead,
			Available:  true,
			Head:       "aaaa1111",
			StatusHash: "status-one",
		},
		BaseHeadSHA: "aaaa1111",
		BaseBranch:  "main",
		GoalHash:    WriteWorkflowGoalHash("add feature X"),
	}
}

func TestMatchWriteWorkflowRepoIdentityArms(t *testing.T) {
	current := testIdentityFixture()

	t.Run("nil stored identity is legacy mismatch", func(t *testing.T) {
		decision := MatchWriteWorkflowRepoIdentity(nil, current)
		if decision.Match || decision.ReasonCode != WriteWorkflowIdentityReasonIdentityMissing {
			t.Fatalf("nil identity must fail closed as legacy, got %+v", decision)
		}
	})

	t.Run("schema zero is legacy mismatch even with populated fields", func(t *testing.T) {
		stored := testIdentityFixture()
		stored.IdentitySchema = 0
		decision := MatchWriteWorkflowRepoIdentity(&stored, current)
		if decision.Match || decision.ReasonCode != WriteWorkflowIdentityReasonIdentityMissing {
			t.Fatalf("schema-0 identity must fail closed as legacy, got %+v", decision)
		}
	})

	t.Run("repo root mismatch", func(t *testing.T) {
		stored := testIdentityFixture()
		stored.CanonicalRepoRoot = "/srv/checkouts/repo-b"
		decision := MatchWriteWorkflowRepoIdentity(&stored, current)
		if decision.Match || decision.ReasonCode != WriteWorkflowIdentityReasonRepoRootMismatch {
			t.Fatalf("root mismatch arm missing, got %+v", decision)
		}
	})

	t.Run("base head mismatch", func(t *testing.T) {
		stored := testIdentityFixture()
		stored.BaseHeadSHA = "bbbb2222"
		stored.RepoFingerprint.Head = "bbbb2222"
		decision := MatchWriteWorkflowRepoIdentity(&stored, current)
		if decision.Match || decision.ReasonCode != WriteWorkflowIdentityReasonBaseHeadMismatch {
			t.Fatalf("base head mismatch arm missing, got %+v", decision)
		}
	})

	t.Run("base branch mismatch", func(t *testing.T) {
		stored := testIdentityFixture()
		stored.BaseBranch = "feature/wfid"
		decision := MatchWriteWorkflowRepoIdentity(&stored, current)
		if decision.Match || decision.ReasonCode != WriteWorkflowIdentityReasonBaseBranchMismatch {
			t.Fatalf("base branch mismatch arm missing, got %+v", decision)
		}
	})

	// The fingerprint arm has independent judging power only when the plain
	// BaseHeadSHA scalar agrees but the fingerprint head does not (tampered /
	// hand-edited run file). It must still refuse.
	t.Run("fingerprint head mismatch with matching scalar sha", func(t *testing.T) {
		stored := testIdentityFixture()
		stored.RepoFingerprint.Head = "cccc3333"
		decision := MatchWriteWorkflowRepoIdentity(&stored, current)
		if decision.Match || decision.ReasonCode != WriteWorkflowIdentityReasonRepoFingerprintMismatch {
			t.Fatalf("fingerprint mismatch arm missing, got %+v", decision)
		}
	})

	t.Run("goal mismatch", func(t *testing.T) {
		stored := testIdentityFixture()
		stored.GoalHash = WriteWorkflowGoalHash("different goal")
		decision := MatchWriteWorkflowRepoIdentity(&stored, current)
		if decision.Match || decision.ReasonCode != WriteWorkflowIdentityReasonGoalMismatch {
			t.Fatalf("goal mismatch arm missing, got %+v", decision)
		}
	})

	t.Run("exact match", func(t *testing.T) {
		stored := testIdentityFixture()
		decision := MatchWriteWorkflowRepoIdentity(&stored, current)
		if !decision.Match || decision.ReasonCode != WriteWorkflowIdentityReasonMatch {
			t.Fatalf("identical identity must match, got %+v", decision)
		}
	})

	// StatusHash is deliberately outside the identity gate: the workflow
	// store itself lives under <CWD>/.codrax, so persisting between turns
	// perturbs the dirty-tree hash in non-ignoring repos. A hard gate on it
	// would fire on tool-induced noise (precise-signals red line). Commit
	// movement is caught by the BaseHeadSHA / fingerprint-head arms.
	t.Run("status hash drift alone still matches", func(t *testing.T) {
		stored := testIdentityFixture()
		stored.RepoFingerprint.StatusHash = "status-two"
		decision := MatchWriteWorkflowRepoIdentity(&stored, current)
		if !decision.Match {
			t.Fatalf("status-hash-only drift must not refuse auto-resume, got %+v", decision)
		}
	})
}

func TestWriteWorkflowGoalHashStable(t *testing.T) {
	if WriteWorkflowGoalHash("") != "" {
		t.Fatalf("empty goal must hash to empty")
	}
	a := WriteWorkflowGoalHash("add   feature\tX")
	b := WriteWorkflowGoalHash("add feature X")
	if a == "" || a != b {
		t.Fatalf("goal hash must collapse whitespace: %q vs %q", a, b)
	}
	if len(a) != 16 {
		t.Fatalf("goal hash must be a 16-hex-char sha256 prefix, got %q", a)
	}
	if a == WriteWorkflowGoalHash("add feature Y") {
		t.Fatalf("different goals must hash differently")
	}
}

func TestNormalizeWriteWorkflowRunIdentityAndAuthorization(t *testing.T) {
	identity := testIdentityFixture()
	now := time.Now()
	run := NormalizeWriteWorkflowRun(WriteWorkflowRun{
		RunID:               "wf-id",
		Identity:            &identity,
		ResumeAuthorization: WriteWorkflowResumeAuthorizationExplicit,
		ResumeAuthorizedAt:  now,
		Status:              WriteWorkflowRunInProgress,
	})
	if run.Identity == nil || run.Identity.IdentitySchema != WriteWorkflowRepoIdentitySchemaVersion ||
		run.Identity.CanonicalRepoRoot != "/srv/checkouts/repo-a" ||
		run.Identity.GoalHash != WriteWorkflowGoalHash("add feature X") {
		t.Fatalf("normalize must preserve minted identity, got %+v", run.Identity)
	}
	if run.ResumeAuthorization != WriteWorkflowResumeAuthorizationExplicit || !run.ResumeAuthorizedAt.Equal(now) {
		t.Fatalf("normalize must preserve the explicit resume token, got %q at %v", run.ResumeAuthorization, run.ResumeAuthorizedAt)
	}

	bogus := NormalizeWriteWorkflowRun(WriteWorkflowRun{
		RunID:               "wf-bogus",
		ResumeAuthorization: "some_other_value",
		ResumeAuthorizedAt:  now,
	})
	if bogus.ResumeAuthorization != "" || !bogus.ResumeAuthorizedAt.IsZero() {
		t.Fatalf("unknown authorization values must be cleared, got %q at %v", bogus.ResumeAuthorization, bogus.ResumeAuthorizedAt)
	}

	empty := NormalizeWriteWorkflowRun(WriteWorkflowRun{RunID: "wf-empty", Identity: &WriteWorkflowRepoIdentity{}})
	if empty.Identity != nil {
		t.Fatalf("all-zero identity must normalize to nil (legacy), got %+v", empty.Identity)
	}
}

func TestLegacyWriteWorkflowRunJSONDecodesWithoutIdentity(t *testing.T) {
	// Pre-WFID-1 run file shape: no repo_identity, no resume_authorization.
	raw := []byte(`{"run_id":"wf-legacy","goal":"old goal","status":"in_progress","active_batch_id":"batch-1"}`)
	var run WriteWorkflowRun
	if err := json.Unmarshal(raw, &run); err != nil {
		t.Fatalf("legacy JSON must stay decodable: %v", err)
	}
	run = NormalizeWriteWorkflowRun(run)
	if run.Identity != nil || run.ResumeAuthorization != "" {
		t.Fatalf("legacy file must decode to nil identity and empty authorization, got %+v %q", run.Identity, run.ResumeAuthorization)
	}
	decision := MatchWriteWorkflowRepoIdentity(run.Identity, testIdentityFixture())
	if decision.Match || decision.ReasonCode != WriteWorkflowIdentityReasonIdentityMissing {
		t.Fatalf("legacy file must be an identity mismatch by ruling, got %+v", decision)
	}
}
