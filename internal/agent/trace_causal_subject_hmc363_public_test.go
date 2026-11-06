package agent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc363NativeRank(t *testing.T, path string, tid int, start, end float64) (*types.AgentContext, types.ToolResult) {
	t.Helper()
	dir := t.TempDir()
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "root_cause_rank", "pid": tid, "time_start": start, "time_end": end, "trace_flavor": "harmony_hitrace"})
	r, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
	if err != nil || !r.Success {
		t.Fatalf("native rank: %v / %s", err, r.Summary)
	}
	objective := "In this window, what caused the delay?"
	mu := types.NewMutableState(objective)
	rm := types.RequestModel{Language: "en", Intent: types.IntentRootCause, PerfTrace: &types.PerfBundle{},
		RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis},
		RuntimeTargetProfile:        &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNoNamedTarget},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end}}
	mu.SetRequestModel(rm)
	r = types.AttachToolHandoffCarrier(r)
	mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{r}, HandoffCarriers: types.ToolHandoffCarriersFromTurnAInputs([]types.ToolResult{r}, nil, nil)})
	bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, Language: "en", Mutable: mu,
		AnalysisIR:               &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "en"}},
		RuntimeArtifactPreflight: types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: path, Carrier: "path"}}})}
	return ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize), r
}

func hmc363CheckDeliveredRankSubject(t *testing.T, ctx *types.AgentContext, tid int, wantAlias bool) {
	t.Helper()
	transport, _, err := hmc362CaptureFinalizer(t, ctx, 2_000_000)
	if transport.calls != 1 || !errors.Is(err, transport.stop) {
		t.Fatalf("actual finalizer transport: calls=%d %v", transport.calls, err)
	}
	got := hmc326DynamicRequestMessage(t, transport)
	rosters, _ := hmc362RosterRows(t, got, 1)
	rows := rosters[0]
	if len(rows) == 0 {
		t.Fatal("fixture must publish a selectable on-chain roster")
	}
	type identity struct {
		TID  int    `json:"tid"`
		TGID int    `json:"tgid"`
		Comm string `json:"comm"`
	}
	found, alias := 0, false
	var selection *types.TraceRootCauseItemV2
	var wantScope types.TraceRankSubjectScope
	for _, row := range rows {
		var subject struct {
			Target   identity `json:"query_target"`
			Member   identity `json:"member"`
			Relation string   `json:"request_binding"`
		}
		if raw := row["query_subject_scope"]; len(raw) == 0 {
			continue
		} else if err := json.Unmarshal(raw, &subject); err != nil {
			t.Fatal(err)
		}
		if subject.Target.TID != tid || subject.Relation != "unbound" {
			t.Fatalf("matching window incorrectly bound a request subject: %+v", subject)
		}
		if subject.Member.TID == tid && subject.Member.TGID > 0 && subject.Member.TGID == subject.Target.TGID && subject.Member.Comm != subject.Target.Comm {
			alias = true
		}
		if selection == nil {
			var id string
			if err := json.Unmarshal(row["candidate_id"], &id); err != nil || id == "" {
				t.Fatalf("missing native candidate identity: %v", err)
			}
			if err := json.Unmarshal(row["query_subject_scope"], &wantScope); err != nil {
				t.Fatal(err)
			}
			forged := wantScope
			forged.Target.TID++
			selection = &types.TraceRootCauseItemV2{CandidateID: id, SubjectScope: &forged}
		}
		found++
	}
	if found == 0 || wantAlias && !alias || strings.Count(got, "A matching time window does not bind an application") != 1 {
		t.Fatalf("native identity/claim scope was lost before actual Finalizer transport: rows=%d scoped=%d alias=%t", len(rows), found, alias)
	}
	bound, err := tracefinding.BindRootCauseReportSelection(&types.TraceRootCauseReportV2{
		SchemaVersion: types.TraceRootCauseReportSchemaVersion, RootCauses: []*types.TraceRootCauseItemV2{selection},
	}, ctx.Mutable.TraceFindingContract())
	if err != nil || bound == nil || len(bound.RootCauses) != 1 || !reflect.DeepEqual(bound.RootCauses[0].SubjectScope, &wantScope) {
		t.Fatalf("published sidecar must reuse the roster's scope, not submitted scope: %v / %+v", err, bound)
	}
}

func TestHMC363PublicTraceSubjectScopeAtActualFinalizer(t *testing.T) {
	caseBytes, err := os.ReadFile("../../eval/cases/trace_query_wakeup_causal_io_chain.case")
	if err != nil {
		t.Fatal(err)
	}
	_, tail, _ := strings.Cut(string(caseBytes), "HTRACE='")
	fixture, _, ok := strings.Cut(tail, "\n'\n")
	if !ok || fixture == "" {
		t.Fatal("missing bounded trace fixture")
	}
	path := filepath.Join(t.TempDir(), "chain.ftrace")
	if err := os.WriteFile(path, []byte(fixture), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, _ := hmc363NativeRank(t, path, 100, 2, 2.02)
	hmc363CheckDeliveredRankSubject(t, ctx, 100, false)
}

func TestHMC363WholeNetEaseTraceSubjectScope(t *testing.T) {
	path := os.Getenv("CODRAX_HMC363_NETEASE")
	if path == "" {
		t.Skip("requires pinned whole NetEase capture")
	}
	sha, n := hmc333FullSourceHash(t, path)
	if sha != "12349db5bbdc5a4343360d9633c7d6c36f18c8be5f39fb64e78b53c9e345bfe4" || n != 82838676 {
		t.Fatal("not the original whole capture")
	}
	ctx, _ := hmc363NativeRank(t, path, 36435, 5969.994941, 5970.622586)
	hmc363CheckDeliveredRankSubject(t, ctx, 36435, true)
}

func TestHMC363RankSubjectSurvivesNativeHistoryCompaction(t *testing.T) {
	ctx, path := hmc320BusinessFinalizer(t)
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	original := hmc320BusinessQuery(t, ctx, path, map[string]any{"view": "root_cause_rank", "pid": 100,
		"time_start": .999, "time_end": 1.051, "platform": "harmony", "trace_flavor": "harmony_hitrace"})
	want := map[string]types.TraceRankSubjects{}
	for _, row := range original.Observations {
		if row.TraceRankSubjects != nil {
			want[row.ID] = *row.TraceRankSubjects
		}
	}
	if len(want) == 0 || ctx.Mutable.NativeFactRetentionStatus().RetainedQueries == 0 {
		t.Fatal("expected a real retained rank publication")
	}
	ctx.Mutable.PublishTurnAArtifacts(types.TurnAArtifacts{}, types.TurnAHandoffHistoryCompacted)
	ctx.Mutable.ResetDispatchToolResults()
	for pass := 0; pass < 2; pass++ {
		input := hmc325BoundaryInput(ctx)
		if len(input.ToolResults) != 1 {
			t.Fatalf("expected only the retained native publication: %d", len(input.ToolResults))
		}
		n := 0
		for _, row := range input.ToolResults[0].Observations {
			if subjects, ok := want[row.ID]; ok {
				if row.TraceRankSubjects == nil || *row.TraceRankSubjects != subjects {
					t.Fatalf("compaction or a copy-out changed the exact rank identity: %s %+v", row.ID, row.TraceRankSubjects)
				}
				row.TraceRankSubjects.Target.TID++
				n++
			}
		}
		if n != len(want) {
			t.Fatalf("retained %d/%d rank identities", n, len(want))
		}
	}
}
