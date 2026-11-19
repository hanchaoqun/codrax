package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/outputdump"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// hmc377FollowUpFailureLLM accepts a first answer that omits the root-cause
// selection, then fails the offered follow-up request with err.
type hmc377FollowUpFailureLLM struct {
	err   error
	calls int
}

func (l *hmc377FollowUpFailureLLM) Chat(_ context.Context, _ []llm.Message, _ []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	l.calls++
	if l.calls == 1 {
		return llm.Response{ToolCalls: []llm.ToolCall{{ID: "answer-1", Name: "emit_answer_document",
			Params: json.RawMessage(`{"blocks":[{"id":"summary","kind":"summary","text":"The main thread waits for the collection to finish; this conclusion is the model's own."}]}`)}}}, nil
	}
	if l.calls == 2 {
		return llm.Response{}, l.err
	}
	return llm.Response{}, errors.New("no further request expected")
}
func (*hmc377FollowUpFailureLLM) ModelID() string               { return "hmc377-follow-up-failure" }
func (*hmc377FollowUpFailureLLM) MaxContextTokens() int         { return 1_000_000 }
func (*hmc377FollowUpFailureLLM) MaxOutputTokens() int          { return 0 }
func (*hmc377FollowUpFailureLLM) RequestTimeout() time.Duration { return 0 }
func (*hmc377FollowUpFailureLLM) RetryMaxAttempts() int         { return 0 }

// HMC377 F6, end to end: the real finalizer accepts a first answer without a
// selection and offers the selection follow-up; the provider then fails that
// request mid-stream. The orchestrator keeps the accepted answer (transient
// failure after usable progress) and the customer side file names the
// undelivered follow-up, never "the model never selected".
func TestHMC377FailedFollowUpRequestReachesTheSideFileAsNotSent(t *testing.T) {
	mut := types.NewMutableState("Where did the main thread spend this frame?")
	mut.SetTraceFindingContract(&types.TraceFindingContract{
		RootCauseReportEnabled: true, CandidateSetID: "hmc377-fixed-roster",
		Candidates: []types.TraceFindingCandidateV1{{PrimaryEligible: true, Decision: types.TraceCauseDecision{
			CandidateID: "candidate-collection-wait", SubjectName: "main-11077",
			Token:           types.TraceCausalTokenSnapshot{Token: "gc_pause", Lane: "deterministic_runtime_work"},
			Magnitude:       &types.TypedMagnitude{Value: 123.003, Unit: "ms", Additivity: "wall_clock_per_thread", Caliber: "effective_attribution"},
			CausalQualifier: types.TraceCausalQualifierProven, EvidenceRefs: []string{"E-wait"},
		}}},
	})
	ctx := &types.AgentContext{Ctx: context.Background(), Objective: mut.Objective(), AgentName: types.AgentFinalizer, Stage: types.StageFinalize, Language: "en", Mutable: mut}
	reg := skill.NewRegistry()
	skill.RegisterDefaults(reg)
	sk, err := reg.Get("answer-document-skill")
	if err != nil {
		t.Fatal(err)
	}
	tools := tool.NewRegistry()
	tools.Register(&tool.EmitAnswerDocument{})
	tools.Register(&tool.EmitAnswerDocumentPatch{})
	model := &hmc377FollowUpFailureLLM{err: io.ErrUnexpectedEOF}
	finalizer := agent.NewFinalizerAgent(&agent.Dependencies{LLM: model, Tools: tools, MaxIterations: 4, AgentSettings: types.DefaultAgentSettings(), Emit: func(render.Event) {}})
	out, err := finalizer.Execute(ctx, sk)
	if model.calls != 2 || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("fixture: the follow-up request must be the one that failed: calls=%d err=%v", model.calls, err)
	}
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut}, outputDumpDir: t.TempDir(), outputDumpMax: 10, emit: func(render.Event) {}}
	if !o.canUseFinalizerOutputAfterTransientProgress(out, err) || !strings.Contains(out.FinalAnswer, "collection to finish") {
		t.Fatalf("fixture: the accepted answer must survive the failed follow-up: %+v", out)
	}
	o.recordTaskFinalize(out)
	body, err := os.ReadFile(mut.FinalAnswerRootCauseJSONPath())
	if err != nil {
		t.Fatal(err)
	}
	var got outputdump.DefaultRootCauseArtifact
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "unavailable" || len(got.RootCauses) != 0 || got.ReasonCode != outputdump.RootCauseReasonSelectionFollowUpNotSent {
		t.Fatalf("a follow-up whose request failed must not read as a declined selection: %s", body)
	}
}
