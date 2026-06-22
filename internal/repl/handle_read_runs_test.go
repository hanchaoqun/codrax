package repl

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestReadRunsListShowClear(t *testing.T) {
	store := NewReadRunSnapshotStore(t.TempDir())
	snapshot := &types.ReadRunSnapshot{
		SchemaVersion: types.ReadRunSnapshotSchemaVersion,
		RunID:         "read-audit-1",
		CreatedAt:     time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC),
		Request:       "which agent can call subagents",
		RequestHash:   types.ReadRunRequestHash("which agent can call subagents"),
		RepoRoot:      "/tmp/repo",
		RepoFingerprint: types.ReadRunRepoFingerprint{
			Kind:       types.ReadRunRepoFingerprintKindGitHead,
			Available:  true,
			Head:       "abcdef1234567890",
			StatusHash: "123456abcdef",
		},
		Environment: types.ReadRunEnvironmentFingerprint{
			Kind:            types.ReadRunEnvironmentKindCodrax,
			Available:       true,
			CodraxVersion:   "0.1.test",
			CodraxBuildTime: "2026-06-22T00:00:00Z",
			GoVersion:       "go1.22.5",
			GOOS:            "darwin",
			GOARCH:          "arm64",
			Tools: []types.ReadRunToolFingerprint{{
				Name:        "git",
				Available:   true,
				Executable:  "/usr/bin/git",
				VersionHash: strings.Repeat("d", 64),
			}},
			Configs: []types.ReadRunConfigFingerprint{
				types.ReadRunConfigFingerprintFromStringSlice(types.ReadRunConfigFingerprintSearchExcludeRoots, []string{"out"}),
			},
		},
		Attachments: []types.ReadRunAttachmentFingerprint{
			types.ReadRunAttachmentFingerprintFromPayload(types.ReadRunAttachmentKindLog, "panic: sample\n", ""),
			types.ReadRunAttachmentFingerprintFromPayload(types.ReadRunAttachmentKindTrace, "sched_switch\n", "android_atrace"),
		},
		TaskGraphHash: strings.Repeat("a", 64),
		TaskNodeCount: 3,
		NodeStatuses: map[string]types.NodeExecStatus{
			"explore": types.NodeExecDone,
			"final":   types.NodeExecPending,
		},
		ReadSet: []string{"internal/agent/subagent_runtime.go", "internal/agent/subagent.go"},
		AcceptedEvidence: []types.AcceptedEvidenceRef{{
			ID:        "ev-subagent",
			Source:    "internal/agent/subagent_runtime.go",
			LineStart: 218,
		}},
		ProgressDecision: types.ProgressDecision{
			ShouldReplan: true,
			ReasonCode:   types.ProgressReasonContinue,
		},
		ActiveState: types.ReadRunActiveState{
			TransientRetryPending:    true,
			TransientRetryReasonCode: types.ReadRunTransientRetryReasonCheckpoint,
			TransientRetryHintHash:   strings.Repeat("c", 64),
			TransientRetryHintBytes:  72,
			ReadLoopNextAction: types.ReadRunNextActionState{
				Active:     true,
				Action:     types.ReadDispatchPolicyActionAddProof,
				ReasonCode: "proof_weak",
			},
			ReadDispatchPolicy: types.ReadDispatchPolicy{
				Active:       true,
				Action:       types.ReadDispatchPolicyActionAddProof,
				AllowedTools: []string{"read_file", "emit_evidence"},
				ScopePaths:   []string{"internal/agent/subagent_runtime.go"},
				MaxToolCalls: 2,
				OneShot:      true,
			},
		},
		SourceInventory: types.SourceInventoryObservation{
			Active:   true,
			Complete: true,
			SourceClasses: []types.SourceInventorySourceClassCount{{
				Role:     types.SourcePathRoleProduction,
				Count:    2,
				Complete: true,
			}},
		},
	}
	if _, err := store.Save(snapshot); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out := &bytes.Buffer{}
	r := New(Config{
		Runner:               stubRunner{},
		In:                   strings.NewReader(""),
		Out:                  out,
		RepoRoot:             "/tmp/repo",
		Branch:               "main",
		Render:               renderNothing,
		ReadRunSnapshotStore: store,
		Language:             "en",
	})

	r.handleReadRunsCmd("/read-runs list")
	got := out.String()
	for _, want := range []string{"Read run snapshots", "`read-audit-1`", "reads=2", "evidence=1", "graph=aaaaaaaaaaaa"} {
		if !strings.Contains(got, want) {
			t.Fatalf("list output missing %q:\n%s", want, got)
		}
	}

	out.Reset()
	r.handleReadRunsCmd("/read-runs show read-audit-1")
	got = out.String()
	for _, want := range []string{
		"Read run `read-audit-1`",
		fmt.Sprintf("Schema: `%d`", types.ReadRunSnapshotSchemaVersion),
		"Repo: `/tmp/repo`",
		"Request fingerprint:",
		"Repo fingerprint: kind=`git_head`",
		"head=`abcdef123456`",
		"Environment fingerprint: kind=`codrax_runtime`",
		"tools=1",
		"configs=1",
		"tool_hashes=git=dddddddddddd",
		"config_hashes=search_exclude_roots=",
		"Attachment fingerprints:",
		"log=",
		"trace=",
		"source=android_atrace",
		"Task graph: hash=`aaaaaaaaaaaa` nodes=3",
		"Node statuses: pending=1 done=1",
		"Read files: 2",
		"Accepted evidence refs: 1",
		"Progress: reason=`progress_delta_continue`",
		"should_replan=t",
		"Active state:",
		"transient_retry=present",
		"hint_hash=`cccccccccccc`",
		"policy=active",
		"Source inventory: complete=true",
		"classes: production=2",
		"Advanced: `/read-runs resume read-audit-1`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("show output missing %q:\n%s", want, got)
		}
	}
	rawMarkdown := readRunSnapshotMarkdown(types.NormalizeReadRunSnapshot(*snapshot))
	for _, want := range []string{
		"Repo fingerprint: kind=`git_head` head=`abcdef123456` status=`123456abcdef`",
		"Environment fingerprint: kind=`codrax_runtime` codrax=`0.1.test` build=`2026-06-22T00:00:00Z` go=`go1.22.5` platform=`darwin/arm64` tools=1 configs=1 tool_hashes=git=dddddddddddd config_hashes=search_exclude_roots=",
		"Attachment fingerprints: log=",
		"trace=421d10005960/13B source=android_atrace",
		"Active state: transient_retry=present reason=`transient_retry_checkpoint` hint_hash=`cccccccccccc` hint_bytes=72",
	} {
		if !strings.Contains(rawMarkdown, want) {
			t.Fatalf("raw markdown missing %q:\n%s", want, rawMarkdown)
		}
	}
	if strings.Contains(rawMarkdown, "raw retry hint") {
		t.Fatalf("read-runs show must not expose raw retry hint text:\n%s", rawMarkdown)
	}
	if strings.Contains(rawMarkdown, "git version 2.") {
		t.Fatalf("read-runs show must not expose raw tool version output:\n%s", rawMarkdown)
	}

	out.Reset()
	r.handleReadRunsCmd("/read-runs clear read-audit-1")
	got = out.String()
	if !strings.Contains(got, "read-runs cleared: read-audit-1") {
		t.Fatalf("clear output unexpected:\n%s", got)
	}
	loaded, err := store.Load("read-audit-1")
	if err != nil {
		t.Fatalf("Load after clear: %v", err)
	}
	if loaded != nil {
		t.Fatalf("snapshot should be cleared, got %+v", loaded)
	}
}

func TestReadRunsShowIncludesComparableReplayAuditCard(t *testing.T) {
	store := NewReadRunSnapshotStore(t.TempDir())
	requestHash := types.ReadRunRequestHash("same typed request")
	prior := &types.ReadRunSnapshot{
		SchemaVersion: types.ReadRunSnapshotSchemaVersion,
		RunID:         "read-prior",
		Request:       "prior raw request text should not leak",
		RequestHash:   requestHash,
		RepoRoot:      "/tmp/repo",
		TaskGraphHash: "graph-replay",
		NodeStatuses:  map[string]types.NodeExecStatus{"explore": types.NodeExecDone},
		ReadSet:       []string{"a.go"},
		AcceptedEvidence: []types.AcceptedEvidenceRef{{
			ID:        "ev-a",
			Source:    "a.go",
			LineStart: 1,
		}},
		NodeArtifacts: []types.NodeArtifactRecord{{
			ProducerNodeID: "explore",
			Artifact: types.RuntimeArtifactRef{
				Kind:      types.RuntimeArtifactEvidenceItem,
				ID:        "ev-a",
				Path:      "a.go",
				LineStart: 1,
			},
		}},
		SourceInventory: types.SourceInventoryObservation{
			Active: true,
			Scopes: []string{"."},
			Sets: []types.SourceInventoryObservationSet{{
				Role:    types.AnswerCandidateRoleFile,
				Members: []types.SourceInventoryObservationMember{{Name: "a.go", File: "a.go"}},
			}},
		},
		ActiveState: types.ReadRunActiveState{TransientRetryPending: true},
	}
	current := types.NormalizeReadRunSnapshot(*prior)
	current.RunID = "read-current"
	current.Request = "same typed request"
	current.NodeStatuses = map[string]types.NodeExecStatus{
		"explore": types.NodeExecDone,
		"extract": types.NodeExecDone,
	}
	current.ReadSet = []string{"a.go", "b.go"}
	current.AcceptedEvidence = append(current.AcceptedEvidence, types.AcceptedEvidenceRef{
		ID:        "ev-b",
		Source:    "b.go",
		LineStart: 7,
	})
	current.NodeArtifacts = append(current.NodeArtifacts, types.NodeArtifactRecord{
		Direction:      types.RuntimeArtifactConsumed,
		ProducerNodeID: "explore",
		ConsumerNodeID: "extract",
		Consumer:       types.RuntimeArtifactConsumerExtract,
		Artifact: types.RuntimeArtifactRef{
			Kind:      types.RuntimeArtifactEvidenceItem,
			ID:        "ev-a",
			Path:      "a.go",
			LineStart: 1,
		},
	})
	current.SourceInventory.Scopes = []string{".", "internal"}
	current.SourceInventory.Sets[0].Members = append(current.SourceInventory.Sets[0].Members, types.SourceInventoryObservationMember{Name: "b.go", File: "b.go"})
	current.ActiveState = types.ReadRunActiveState{}

	if _, err := store.Save(prior); err != nil {
		t.Fatalf("Save prior: %v", err)
	}
	if _, err := store.Save(&current); err != nil {
		t.Fatalf("Save current: %v", err)
	}
	baseTime := time.Date(2026, 6, 22, 11, 0, 0, 0, time.UTC)
	for i, id := range []string{"read-prior", "read-current"} {
		path := filepath.Join(store.RunDir(), id+".json")
		ts := baseTime.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(path, ts, ts); err != nil {
			t.Fatalf("Chtimes %s: %v", id, err)
		}
	}

	out := &bytes.Buffer{}
	r := New(Config{
		Runner:               stubRunner{},
		In:                   strings.NewReader(""),
		Out:                  out,
		RepoRoot:             "/tmp/repo",
		Branch:               "main",
		Render:               renderNothing,
		ReadRunSnapshotStore: store,
		Language:             "en",
	})
	r.handleReadRunsCmd("/read-runs show read-current")
	got := out.String()
	if !strings.Contains(got, "Replay audit:") || !strings.Contains(got, "baseline=`read-prior`") {
		t.Fatalf("show output missing replay audit card:\n%s", got)
	}
	raw := readRunSnapshotMarkdownWithReplayAudit(types.NormalizeReadRunSnapshot(current), prior)
	for _, want := range []string{
		"Replay audit: baseline=`read-prior`",
		"request=match",
		"graph=match",
		"repo=unknown",
		"Replay audit status delta:",
		"done=1→2 (+1)",
		"evidence=1→2",
		"read_set=1→2",
		"artifacts=1→2 +1 -0",
		"Replay audit source inventory:",
		"members=1→2",
		"scopes=1→2",
		"Replay audit active state: true→false",
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("raw replay audit markdown missing %q:\n%s", want, raw)
		}
	}
	if strings.Contains(raw, "prior raw request text should not leak") {
		t.Fatalf("replay card leaked prior request text:\n%s", raw)
	}
}

func TestReadRunsResumeDispatchesTypedSeed(t *testing.T) {
	store := NewReadRunSnapshotStore(t.TempDir())
	snapshot := &types.ReadRunSnapshot{
		SchemaVersion: types.ReadRunSnapshotSchemaVersion,
		RunID:         "read-audit-1",
		Request:       "resume this typed read run",
		RepoRoot:      "/tmp/snapshot-repo",
		TaskGraphHash: strings.Repeat("b", 64),
		TaskNodeCount: 2,
		NodeStatuses: map[string]types.NodeExecStatus{
			"explore": types.NodeExecDone,
		},
		ReadSet: []string{"internal/agent/subagent_runtime.go"},
	}
	if _, err := store.Save(snapshot); err != nil {
		t.Fatalf("Save: %v", err)
	}
	runner := &readRunResumeRunner{result: "resumed answer"}
	out := &bytes.Buffer{}
	r := New(Config{
		Runner:               runner,
		In:                   strings.NewReader(""),
		Out:                  out,
		RepoRoot:             "/tmp/current-repo",
		Branch:               "feature/resume",
		Render:               renderReadRunResumeResult,
		ReadRunSnapshotStore: store,
		Language:             "en",
	})
	r.currentMode = types.ModeApply

	r.handleReadRunsCmd("/read-runs resume read-audit-1")

	if len(runner.requests) != 1 || runner.requests[0] != snapshot.Request {
		t.Fatalf("Run requests = %#v, want %q", runner.requests, snapshot.Request)
	}
	if len(runner.repos) != 1 || runner.repos[0] != snapshot.RepoRoot {
		t.Fatalf("Run repos = %#v, want %q", runner.repos, snapshot.RepoRoot)
	}
	if len(runner.branches) != 1 || runner.branches[0] != "feature/resume" {
		t.Fatalf("Run branches = %#v", runner.branches)
	}
	if len(runner.seedRunIDs) != 2 || runner.seedRunIDs[0] != "read-audit-1" || runner.seedRunIDs[1] != "" {
		t.Fatalf("seed run ids = %#v, want installed then cleared", runner.seedRunIDs)
	}
	if len(runner.modeCalls) != 2 || runner.modeCalls[0] != types.ModeRead || runner.modeCalls[1] != types.ModeApply {
		t.Fatalf("mode calls = %#v, want read then restore apply", runner.modeCalls)
	}
	if got := strings.Join(runner.transcriptRequests, "|"); got != snapshot.Request+"|" {
		t.Fatalf("transcript requests = %#v, want request then clear", runner.transcriptRequests)
	}
	if got := out.String(); !strings.Contains(got, "resumed answer") {
		t.Fatalf("resume output missing answer:\n%s", got)
	}
}

func TestReadRunsResumeRequiresStableIDAndCapability(t *testing.T) {
	store := NewReadRunSnapshotStore(t.TempDir())
	out := &bytes.Buffer{}
	runner := &readRunResumeRunner{}
	r := New(Config{
		Runner:               runner,
		In:                   strings.NewReader(""),
		Out:                  out,
		RepoRoot:             "/tmp/current-repo",
		Branch:               "main",
		Render:               renderReadRunResumeResult,
		ReadRunSnapshotStore: store,
		Language:             "en",
	})

	r.handleReadRunsCmd("/read-runs resume")
	if len(runner.requests) != 0 {
		t.Fatalf("Run should not be called without id: %#v", runner.requests)
	}
	if got := out.String(); !strings.Contains(got, "/read-runs resume <run-id>") {
		t.Fatalf("resume usage output unexpected:\n%s", got)
	}

	out.Reset()
	r.handleReadRunsCmd("/read-runs resume missing-run")
	if len(runner.requests) != 0 {
		t.Fatalf("Run should not be called for missing snapshot: %#v", runner.requests)
	}
	if got := out.String(); !strings.Contains(got, `snapshot "missing-run" not found`) {
		t.Fatalf("missing snapshot output unexpected:\n%s", got)
	}

	out.Reset()
	rNoSeed := New(Config{
		Runner:               stubRunner{},
		In:                   strings.NewReader(""),
		Out:                  out,
		RepoRoot:             "/tmp/current-repo",
		Branch:               "main",
		Render:               renderReadRunResumeResult,
		ReadRunSnapshotStore: store,
		Language:             "en",
	})
	rNoSeed.handleReadRunsCmd("/read-runs resume missing-run")
	if got := out.String(); !strings.Contains(got, "does not support typed read snapshot seeds") {
		t.Fatalf("unsupported runner output unexpected:\n%s", got)
	}
}

func TestReadRunsCommandDisabledWithoutStore(t *testing.T) {
	out := &bytes.Buffer{}
	r := New(Config{
		Runner:   stubRunner{},
		In:       strings.NewReader(""),
		Out:      out,
		RepoRoot: "/tmp/repo",
		Branch:   "main",
		Render:   renderNothing,
		Language: "en",
	})
	r.handleReadRunsCmd("/read-runs list")
	if got := out.String(); !strings.Contains(got, "/read-runs disabled") {
		t.Fatalf("disabled output unexpected:\n%s", got)
	}
}

type readRunResumeRunner struct {
	requests           []string
	repos              []string
	branches           []string
	seedRunIDs         []string
	modeCalls          []types.PipelineMode
	transcriptRequests []string
	result             string
}

func (r *readRunResumeRunner) SetReadRunSnapshotSeed(snapshot *types.ReadRunSnapshot) {
	if snapshot == nil {
		r.seedRunIDs = append(r.seedRunIDs, "")
		return
	}
	r.seedRunIDs = append(r.seedRunIDs, snapshot.RunID)
}

func (r *readRunResumeRunner) SetMode(mode types.PipelineMode) {
	r.modeCalls = append(r.modeCalls, mode)
}

func (r *readRunResumeRunner) SetOutputTranscriptRequest(request string) {
	r.transcriptRequests = append(r.transcriptRequests, request)
}

func (r *readRunResumeRunner) Run(request, repoRoot, branch string) (*types.BusContext, error) {
	r.requests = append(r.requests, request)
	r.repos = append(r.repos, repoRoot)
	r.branches = append(r.branches, branch)
	mut := types.NewMutableState(request)
	mut.SetResult(r.result)
	return &types.BusContext{Mutable: mut}, nil
}

func renderReadRunResumeResult(ctx *types.BusContext) string {
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	return ctx.Mutable.Result()
}
