package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/orchestrator"
	"github.com/hanchaoqun/codrax/internal/outputdump"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

type refusalStubAgent struct {
	name types.AgentName
	run  func(*types.AgentContext) (*agent.StageOutput, error)
}

func (a refusalStubAgent) Name() types.AgentName { return a.name }

func (a refusalStubAgent) Execute(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
	if a.run == nil {
		return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
	}
	return a.run(ctx)
}

// refusedRunOrchestrator builds a read pipeline of stub stages whose answer
// stage refuses its request, sending requestsSent requests first.
func refusedRunOrchestrator(requestsSent int) *orchestrator.Orchestrator {
	ir := &types.AnalysisIR{
		Version:      types.AnalysisIRVersion,
		RequestModel: types.RequestModel{Language: "en", Intent: types.IntentExplain},
		TaskGraph: types.TaskGraph{
			Nodes: []types.TaskNode{
				{ID: "n0", Type: types.NodeProbe, Objective: "scan repo", SearchHints: types.SearchHints{KeywordIDs: []string{"k1"}}},
				{ID: "n1", Type: types.NodeEvidence, Objective: "collect evidence", SearchHints: types.SearchHints{EntityIDs: []string{"e1"}}},
				{ID: "n2", Type: types.NodeValidate, Objective: "check chains"},
				{ID: "n3", Type: types.NodeFinalize, Objective: "render answer"},
			},
			Edges: []types.TaskEdge{
				{From: "n0", To: "n1", EdgeType: types.EdgeHardDependency},
				{From: "n1", To: "n2", EdgeType: types.EdgeHardDependency},
				{From: "n2", To: "n3", EdgeType: types.EdgeHardDependency},
			},
			ExecutionPolicy: types.ExecutionPolicy{MaxParallelism: 1, RetryBudget: 1, CriticalPath: []string{"n0", "n1", "n2", "n3"}},
		},
		EvidencePlan:   types.EvidencePlan{Budget: types.EvidenceBudget{MaxFiles: 30, MaxBytes: 200000, MaxReactIters: 10, MaxToolCalls: 40}},
		AnswerContract: types.AnswerContract{Language: "en"},
	}
	stages := map[types.AgentName]func(*types.AgentContext) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(*types.AgentContext) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingFacts, AnalysisIR: ir}, nil
		},
		types.AgentExplorer: func(ctx *types.AgentContext) (*agent.StageOutput, error) {
			// A trace evidence contract is what makes the root-cause side
			// file mandatory for the run.
			ctx.Mutable.SetTraceFindingContract(&types.TraceFindingContract{RootCauseReportEnabled: true})
			return &agent.StageOutput{MissingPiece: types.MissingFacts, EvidenceItems: []types.EvidenceItem{{
				ID: "ev", Source: "src.go", LineStart: 1, AnchorKind: types.AnchorDefinition, AnchorSymbol: "Source",
			}}}, nil
		},
		types.AgentFinalizer: func(*types.AgentContext) (*agent.StageOutput, error) {
			return nil, &agent.AnswerContextByteBudgetError{ActualBytes: 742912, AllocatedBytes: 700000, RequestsSent: requestsSent}
		},
	}
	agents := agent.NewRegistry()
	for _, name := range []types.AgentName{types.AgentLogTriager, types.AgentPerfTriager, types.AgentAnalyzer, types.AgentExplorer, types.AgentExtractor, types.AgentFinalizer} {
		agents.Register(refusalStubAgent{name: name, run: stages[name]})
	}
	skills := skill.NewRegistry()
	for _, name := range []string{"log-triage-skill", "log-segmentation-skill", "analysis-skill", "explore-skill", "extract-skill", "answer-document-skill"} {
		skills.Register(&skill.Config{Name: name, Goal: name + " goal"})
	}
	orch := orchestrator.New(types.PipelineSettings{}, agents, skills, agent.NewSubAgentRegistry())
	orch.SetMaxSteps(20)
	return orch
}

// The default side file is written by the orchestrator and the explicit one
// by this package. For a run that formed no answer both name the same
// reason, because both read the run's one refusal record: the capacity
// reason when nothing was sent, the transcript reason for a refused repair
// request.
func TestExplicitAndDefaultRootCauseFilesNameTheSameUnformedAnswerReason(t *testing.T) {
	for _, tc := range []struct {
		name         string
		requestsSent int
		defaultFile  bool
		want         string
	}{
		{"first request refused, nothing sent", 0, true, "final_answer_request_exceeds_capacity"},
		{"repair request refused", 1, true, "final_answer_transcript_not_available"},
		// The explicit file does not depend on the default one being written.
		{"first request refused, default side file off", 0, false, "final_answer_request_exceeds_capacity"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldOrch, oldMD, oldHTML, oldRoot := app.orch, flagReportMD, flagReportHTML, flagRootCausesOut
			t.Cleanup(func() {
				app.orch, flagReportMD, flagReportHTML, flagRootCausesOut = oldOrch, oldMD, oldHTML, oldRoot
				outputdump.SetExplicitReport(outputdump.ExplicitReport{})
			})
			explicitPath := filepath.Join(t.TempDir(), "api", "root-causes.json")
			defaultDir := t.TempDir()
			flagReportMD, flagReportHTML, flagRootCausesOut = "", "", explicitPath
			outputdump.SetExplicitReport(outputdump.ExplicitReport{RootCauseJSONPath: explicitPath})

			app.orch = refusedRunOrchestrator(tc.requestsSent)
			if tc.defaultFile {
				app.orch.SetOutputDump(defaultDir, 10)
			}
			repo := t.TempDir()
			if err := os.WriteFile(filepath.Join(repo, "src.go"), []byte("package src\n\nfunc Source() {}\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			bus, err := app.orch.Run("explain Source", repo, "main")
			if err != nil || bus == nil || bus.Mutable.Result() != "" || bus.TaskState.LastError == "" {
				t.Fatalf("fixture must end without an answer: err=%v", err)
			}
			if err := printExplicitReportStatus(); err != nil {
				t.Fatalf("explicit report status: %v", err)
			}

			var explicit outputdump.ExplicitRootCauseArtifact
			readJSONFile(t, explicitPath, &explicit)
			if explicit.Status != outputdump.ExplicitRootCauseStatusUnavailable || explicit.ReasonCode != tc.want {
				t.Errorf("explicit side file: status=%q reason=%q, want unavailable/%s", explicit.Status, explicit.ReasonCode, tc.want)
			}
			defaults, err := filepath.Glob(filepath.Join(defaultDir, "*.root-causes.json"))
			if !tc.defaultFile {
				if err != nil || len(defaults) != 0 {
					t.Fatalf("fixture: the default side file is off for this run: %v %v", defaults, err)
				}
				return
			}
			if err != nil || len(defaults) != 1 {
				t.Fatalf("the run must publish exactly one default side file: %v %v", defaults, err)
			}
			var byDefault outputdump.DefaultRootCauseArtifact
			readJSONFile(t, defaults[0], &byDefault)
			if byDefault.ReasonCode != explicit.ReasonCode {
				t.Errorf("the two side files disagree: default=%q explicit=%q", byDefault.ReasonCode, explicit.ReasonCode)
			}
		})
	}
}

// Without an orchestrator there was no run and no record: the explicit side
// file keeps the transcript reason.
func TestExplicitRootCauseFileWithoutARunNamesTheTranscriptReason(t *testing.T) {
	oldOrch, oldMD, oldHTML, oldRoot := app.orch, flagReportMD, flagReportHTML, flagRootCausesOut
	t.Cleanup(func() {
		app.orch, flagReportMD, flagReportHTML, flagRootCausesOut = oldOrch, oldMD, oldHTML, oldRoot
		outputdump.SetExplicitReport(outputdump.ExplicitReport{})
	})
	explicitPath := filepath.Join(t.TempDir(), "root-causes.json")
	flagReportMD, flagReportHTML, flagRootCausesOut = "", "", explicitPath
	outputdump.SetExplicitReport(outputdump.ExplicitReport{RootCauseJSONPath: explicitPath})
	app.orch = nil
	if err := printExplicitReportStatus(); err != nil {
		t.Fatalf("explicit report status: %v", err)
	}
	var explicit outputdump.ExplicitRootCauseArtifact
	readJSONFile(t, explicitPath, &explicit)
	if explicit.Status != outputdump.ExplicitRootCauseStatusUnavailable || explicit.ReasonCode != "final_answer_transcript_not_available" {
		t.Errorf("explicit side file: status=%q reason=%q, want unavailable/final_answer_transcript_not_available", explicit.Status, explicit.ReasonCode)
	}
}

func readJSONFile(t *testing.T, path string, into any) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, into); err != nil {
		t.Fatalf("decode %s: %v\n%s", path, err, body)
	}
}
