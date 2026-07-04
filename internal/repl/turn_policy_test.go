package repl

// turn_policy_test.go — exercises the structured TurnPolicy
// dispatcher end-to-end. Three layers:
//
//   1. ApplyTurnPolicyGuards: pure, deterministic — no LLM, no REPL.
//      Pins the contradiction-rewrite rules in one place so a future
//      edit to the guard sequence has to deliberately update the
//      table-driven test.
//
//   2. ClassifyPolicy: schema parsing — feeds canned tool-call
//      payloads through the *llmChitchatClassifier and asserts the
//      resulting TurnPolicy struct.
//
//   3. dispatch(): full REPL Loop with a stub TurnPolicyClassifier
//      driving the four routes plus the "no prior answer" guard
//      demotion. Uses the existing scriptedChatAdapter / stubResponder
//      / logAwareRunner / stubSummarizer fixtures so the new tests
//      compose with the legacy chitchat tests rather than re-rolling
//      a parallel harness.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/dataquery"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/memory"
	"github.com/hanchaoqun/codrax/internal/operation"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// stubTurnPolicyClassifier records every ClassifyPolicy call and
// returns a canned policy / error. Implements BOTH ChitchatClassifier
// (legacy binary surface — required so the dispatch's gate accepts
// the wired classifier) and TurnPolicyClassifier (so the dispatch
// prefers the structured path).
//
// Legacy Classify intentionally errors loudly so a regression that
// routes through it instead of ClassifyPolicy is caught by every
// turn_policy test.
type stubTurnPolicyClassifier struct {
	policy TurnPolicy
	err    error

	calls         []string
	hints         []string
	hadPriorFlags []bool
	onPolicy      func(context.Context)
}

func (s *stubTurnPolicyClassifier) Classify(_ context.Context, line, hint string) (bool, error) {
	return false, errors.New("stubTurnPolicyClassifier.Classify should not be invoked when TurnPolicyClassifier path is preferred")
}

func (s *stubTurnPolicyClassifier) ClassifyPolicy(ctx context.Context, line, hint string, hasPriorAnswer bool) (TurnPolicy, error) {
	s.calls = append(s.calls, line)
	s.hints = append(s.hints, hint)
	s.hadPriorFlags = append(s.hadPriorFlags, hasPriorAnswer)
	if s.onPolicy != nil {
		s.onPolicy(ctx)
	}
	return s.policy, s.err
}

type secondTurnBlockingPolicyClassifier struct {
	release chan struct{}

	mu    sync.Mutex
	calls int
}

func (s *secondTurnBlockingPolicyClassifier) Classify(_ context.Context, _, _ string) (bool, error) {
	return false, errors.New("legacy classifier should not run after structured timeout")
}

func (s *secondTurnBlockingPolicyClassifier) ClassifyPolicy(_ context.Context, _, _ string, _ bool) (TurnPolicy, error) {
	s.mu.Lock()
	s.calls++
	call := s.calls
	s.mu.Unlock()
	if call == 1 {
		return TurnPolicy{
			Route:           RouteRepo,
			NeedsRepoAccess: true,
			Operation:       "investigate",
			Source:          "repo",
			Confidence:      0.9,
			Reason:          "first turn repository investigation",
		}, nil
	}
	<-s.release
	return TurnPolicy{}, errors.New("released after dispatcher timeout")
}

func (s *secondTurnBlockingPolicyClassifier) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

type structuredErrorLegacyClassifier struct {
	policyErr    error
	legacyIsChat bool
	legacyErr    error
	policyCalls  int
	legacyCalls  int
}

type blockingTurnPolicyAdapter struct{}

func (blockingTurnPolicyAdapter) Chat(ctx context.Context, _ []llm.Message, _ []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	<-ctx.Done()
	return llm.Response{}, ctx.Err()
}

func (blockingTurnPolicyAdapter) ModelID() string { return "blocking-turn-policy-test" }

func (blockingTurnPolicyAdapter) MaxContextTokens() int { return 0 }

func (blockingTurnPolicyAdapter) MaxOutputTokens() int { return 0 }

func (blockingTurnPolicyAdapter) RequestTimeout() time.Duration { return 0 }

func (blockingTurnPolicyAdapter) RetryMaxAttempts() int { return 0 }

// slowTurnPolicyAdapter answers with a canned response after delay,
// honouring ctx cancellation (a real adapter aborts the HTTP round when its
// ctx dies). It records the effective ctx deadline the adapter sees so
// tests can assert WHICH wall clock — if any — governs a classifier lane.
type slowTurnPolicyAdapter struct {
	delay time.Duration
	resp  llm.Response

	sawDeadline   *bool
	deadlineAhead *time.Duration
}

func (a slowTurnPolicyAdapter) Chat(ctx context.Context, _ []llm.Message, _ []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	deadline, ok := ctx.Deadline()
	if a.sawDeadline != nil {
		*a.sawDeadline = ok
	}
	if a.deadlineAhead != nil && ok {
		*a.deadlineAhead = time.Until(deadline)
	}
	timer := time.NewTimer(a.delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return a.resp, nil
	case <-ctx.Done():
		return llm.Response{}, ctx.Err()
	}
}

func (slowTurnPolicyAdapter) ModelID() string { return "slow-turn-policy-test" }

func (slowTurnPolicyAdapter) MaxContextTokens() int { return 0 }

func (slowTurnPolicyAdapter) MaxOutputTokens() int { return 0 }

func (slowTurnPolicyAdapter) RequestTimeout() time.Duration { return 0 }

func (slowTurnPolicyAdapter) RetryMaxAttempts() int { return 0 }

type nonResponsiveTurnPolicyAdapter struct {
	release <-chan struct{}
}

func (a nonResponsiveTurnPolicyAdapter) Chat(_ context.Context, _ []llm.Message, _ []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	<-a.release
	return llm.Response{}, errors.New("released after caller timeout")
}

func (nonResponsiveTurnPolicyAdapter) ModelID() string { return "nonresponsive-turn-policy-test" }

func (nonResponsiveTurnPolicyAdapter) MaxContextTokens() int { return 0 }

func (nonResponsiveTurnPolicyAdapter) MaxOutputTokens() int { return 0 }

func (nonResponsiveTurnPolicyAdapter) RequestTimeout() time.Duration { return 0 }

func (nonResponsiveTurnPolicyAdapter) RetryMaxAttempts() int { return 0 }

type stubDataTaskPlanner struct {
	plan            dataquery.TaskPlan
	err             error
	calls           int
	candidates      [][]dataquery.CandidateFile
	repairPlan      dataquery.TaskPlan
	repairPlans     []dataquery.TaskPlan
	repairErr       error
	repairCalls     int
	repairErrors    []string
	eval            dataquery.Evaluation
	evals           []dataquery.Evaluation
	evalErr         error
	evalCalls       int
	evalRecordLens  []int
	evalLastAnswers []string
	patchPlan       dataquery.DataResultPatchPlan
	patchErr        error
	patchCalls      int
	continuePlan    dataquery.TaskPlan
	continueErr     error
	continueCalls   int
}

func (s *stubDataTaskPlanner) PlanDataTask(_ context.Context, userLine, repoRoot string, policy TurnPolicy, candidates []dataquery.CandidateFile) (dataquery.TaskPlan, error) {
	s.calls++
	s.candidates = append(s.candidates, append([]dataquery.CandidateFile(nil), candidates...))
	return s.plan, s.err
}

func (s *stubDataTaskPlanner) RepairDataTask(_ context.Context, userLine, repoRoot string, policy TurnPolicy, candidates []dataquery.CandidateFile, previous dataquery.TaskPlan, executionError string) (dataquery.TaskPlan, error) {
	s.repairCalls++
	s.repairErrors = append(s.repairErrors, executionError)
	if len(s.repairPlans) > 0 {
		idx := s.repairCalls - 1
		if idx >= len(s.repairPlans) {
			idx = len(s.repairPlans) - 1
		}
		return s.repairPlans[idx], s.repairErr
	}
	return s.repairPlan, s.repairErr
}

func (s *stubDataTaskPlanner) EvaluateDataTask(_ context.Context, userLine string, records []dataTaskWorkflowRecord, lang string) (dataquery.Evaluation, error) {
	s.evalCalls++
	s.evalRecordLens = append(s.evalRecordLens, len(records))
	lastAnswer := ""
	if len(records) > 0 && records[len(records)-1].Result != nil {
		lastAnswer = records[len(records)-1].Result.Answer
	}
	s.evalLastAnswers = append(s.evalLastAnswers, lastAnswer)
	if len(s.evals) > 0 {
		idx := s.evalCalls - 1
		if idx >= len(s.evals) {
			idx = len(s.evals) - 1
		}
		return s.evals[idx], s.evalErr
	}
	return s.eval, s.evalErr
}

func (s *stubDataTaskPlanner) ProposeDataResultPatch(_ context.Context, userLine string, previous dataquery.TaskPlan, partial dataquery.Result, violations []dataquery.DataTaskViolation, records []dataTaskWorkflowRecord, lang string) (dataquery.DataResultPatchPlan, error) {
	s.patchCalls++
	return s.patchPlan, s.patchErr
}

func (s *stubDataTaskPlanner) ContinueDataTask(_ context.Context, userLine, repoRoot string, policy TurnPolicy, candidates []dataquery.CandidateFile, records []dataTaskWorkflowRecord) (dataquery.TaskPlan, error) {
	s.continueCalls++
	return s.continuePlan, s.continueErr
}

func (s *structuredErrorLegacyClassifier) Classify(_ context.Context, line, hint string) (bool, error) {
	s.legacyCalls++
	return s.legacyIsChat, s.legacyErr
}

func (s *structuredErrorLegacyClassifier) ClassifyPolicy(_ context.Context, line, hint string, hasPriorAnswer bool) (TurnPolicy, error) {
	s.policyCalls++
	return TurnPolicy{}, s.policyErr
}

// stubLocalResponder records every RespondLocal call separately
// from Respond so tests can assert on the structured
// (line, prior, lastAnswer, directive) tuple. Embeds stubResponder
// so the legacy Respond path is also satisfied (the dispatcher's
// LocalResponder type-assert succeeds because RespondLocal is
// defined on *stubLocalResponder).
type stubLocalResponder struct {
	stubResponder

	localCalls []stubLocalCall
	localReply string
	localErr   error
	onLocal    func(context.Context)
}

type stubLocalCall struct {
	userLine   string
	prior      string
	lastAnswer string
	directive  string
}

func (s *stubLocalResponder) RespondLocal(ctx context.Context, userLine, prior, lastAnswer, directive string) (string, error) {
	s.localCalls = append(s.localCalls, stubLocalCall{
		userLine:   userLine,
		prior:      prior,
		lastAnswer: lastAnswer,
		directive:  directive,
	})
	if s.onLocal != nil {
		s.onLocal(ctx)
	}
	if s.localErr != nil {
		return "", s.localErr
	}
	return s.localReply, nil
}

// requestCapturingRunner is a Runner that records every Run() call's
// request string and the typed presentation directive visible at Run
// entry. The directive must stay out of the request body while still
// reaching the orchestrator-equivalent setter.
type requestCapturingRunner struct {
	logAwareRunner
	requests          []string
	directiveSetCalls []string
	seenDirectives    []string
	routeHintSetCalls []types.TurnRouteHint
	seenRouteHints    []types.TurnRouteHint
	curDirective      string
	curRouteHint      types.TurnRouteHint
	modeSetCalls      []types.PipelineMode
	seenModes         []types.PipelineMode
	curMode           types.PipelineMode
}

func (r *requestCapturingRunner) Run(req, repo, branch string) (*types.BusContext, error) {
	r.requests = append(r.requests, req)
	r.seenDirectives = append(r.seenDirectives, r.curDirective)
	r.seenRouteHints = append(r.seenRouteHints, r.curRouteHint)
	r.seenModes = append(r.seenModes, r.curMode)
	return r.logAwareRunner.Run(req, repo, branch)
}

func (r *requestCapturingRunner) SetPresentationDirective(directive string) {
	r.directiveSetCalls = append(r.directiveSetCalls, directive)
	r.curDirective = directive
}

func (r *requestCapturingRunner) SetTurnRouteHint(hint types.TurnRouteHint) {
	r.routeHintSetCalls = append(r.routeHintSetCalls, hint)
	r.curRouteHint = hint
}

func (r *requestCapturingRunner) SetMode(mode types.PipelineMode) {
	r.modeSetCalls = append(r.modeSetCalls, mode)
	r.curMode = mode
}

// seedPriorAnswer appends a recent pipeline turn to the store so
// lastAnswerText() returns non-empty. The tests that need
// hasPriorAnswer=true call this before driving the Loop.
func seedPriorAnswer(t *testing.T, store *memory.Store, request, response string) {
	t.Helper()
	if err := store.Append(memory.Turn{
		ID:       "seed-1",
		Request:  request,
		Response: response,
		Kind:     memory.KindPipeline,
	}); err != nil {
		t.Fatalf("seed prior answer: %v", err)
	}
}

// newTurnPolicyREPL is the shared fixture for the dispatch tests —
// matches the shape of newChitchatTestREPL but accepts a
// requestCapturingRunner so the hybrid-route test can inspect the
// effective request sent to Run.
func newTurnPolicyREPL(
	t *testing.T,
	store *memory.Store,
	classifier ChitchatClassifier,
	responder ChitchatResponder,
	input string,
) (*REPL, *requestCapturingRunner, *bytes.Buffer) {
	t.Helper()
	runner := &requestCapturingRunner{}
	out := &bytes.Buffer{}
	r := New(Config{
		Runner:             runner,
		Store:              store,
		Render:             renderNothing,
		RepoRoot:           ".",
		Branch:             "main",
		In:                 strings.NewReader(input),
		Out:                out,
		Prompt:             ">",
		PromptCont:         ".",
		Banner:             "test-banner",
		Language:           "en",
		ChitchatResponder:  responder,
		ChitchatClassifier: classifier,
	})
	return r, runner, out
}

// newPolicyStore is a small helper to keep the test bodies short.
func newPolicyStore(t *testing.T) *memory.Store {
	t.Helper()
	store, err := memory.NewStore(t.TempDir(), stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// ─── Layer 1: ApplyTurnPolicyGuards ────────────────────────────

// TestApplyTurnPolicyGuards covers each documented contradiction-
// rewrite rule. Table-driven so a future guard addition slots in
// without code-restructure churn.
func TestApplyTurnPolicyGuards(t *testing.T) {
	for _, tc := range []struct {
		name           string
		in             TurnPolicy
		hasPriorAnswer bool
		hasAttachment  bool
		wantRoute      TurnRoute
		wantNeedsRepo  bool
	}{
		{
			name: "local + needs_repo_access=true → repo (boolean wins)",
			in: TurnPolicy{
				Route:           RouteLocal,
				NeedsRepoAccess: true,
				Operation:       "investigate",
				Source:          "repo",
				Confidence:      0.8,
			},
			hasPriorAnswer: true,
			wantRoute:      RouteRepo,
			wantNeedsRepo:  true,
		},
		{
			name: "repo + needs_repo_access=false → repo with boolean patched",
			in: TurnPolicy{
				Route:           RouteRepo,
				NeedsRepoAccess: false,
				Operation:       "investigate",
				Source:          "repo",
				Confidence:      0.8,
			},
			hasPriorAnswer: false,
			wantRoute:      RouteRepo,
			wantNeedsRepo:  true,
		},
		{
			name: "hybrid clamps needs_repo_access=true",
			in: TurnPolicy{
				Route:           RouteHybrid,
				NeedsRepoAccess: false,
				Operation:       "investigate",
				Source:          "mixed",
				Confidence:      0.85,
			},
			hasPriorAnswer: true,
			wantRoute:      RouteHybrid,
			wantNeedsRepo:  true,
		},
		{
			name: "local + transform + no prior answer → clarify",
			in: TurnPolicy{
				Route:      RouteLocal,
				Operation:  "transform",
				Source:     "last_answer",
				Confidence: 0.9,
			},
			hasPriorAnswer: false,
			wantRoute:      RouteClarify,
			wantNeedsRepo:  false,
		},
		{
			name: "local + chat + no prior answer → still local (no demotion)",
			in: TurnPolicy{
				Route:      RouteLocal,
				Operation:  "chat",
				Source:     "current_message",
				Confidence: 0.95,
			},
			hasPriorAnswer: false,
			wantRoute:      RouteLocal,
			wantNeedsRepo:  false,
		},
		{
			name: "low confidence local → repo (safe default)",
			in: TurnPolicy{
				Route:      RouteLocal,
				Operation:  "transform",
				Source:     "last_answer",
				Confidence: 0.2,
			},
			hasPriorAnswer: true,
			wantRoute:      RouteRepo,
			wantNeedsRepo:  true,
		},
		{
			name: "hybrid + transform + no prior → clarify (transform requires prior)",
			in: TurnPolicy{
				Route:           RouteHybrid,
				NeedsRepoAccess: true,
				Operation:       "transform",
				Source:          "last_answer",
				Confidence:      0.85,
			},
			hasPriorAnswer: false,
			wantRoute:      RouteClarify,
			wantNeedsRepo:  false,
		},
		{
			name: "clarify + fresh investigate + no prior → repo safe default",
			in: TurnPolicy{
				Route:      RouteClarify,
				Operation:  "investigate",
				Source:     "current_message",
				Confidence: 0.74,
				Reason:     "current system not named",
			},
			hasPriorAnswer: false,
			wantRoute:      RouteRepo,
			wantNeedsRepo:  true,
		},
		{
			name: "clarify + missing-prior transform remains clarify",
			in: TurnPolicy{
				Route:      RouteClarify,
				Operation:  "transform",
				Source:     "last_answer",
				Confidence: 0.9,
			},
			hasPriorAnswer: false,
			wantRoute:      RouteClarify,
			wantNeedsRepo:  false,
		},
		{
			name: "clarify + confirmation operation remains clarify",
			in: TurnPolicy{
				Route:                RouteClarify,
				Operation:            "investigate",
				Source:               "current_message",
				RiskLevel:            "high",
				RequiresConfirmation: true,
				Confidence:           0.8,
			},
			hasPriorAnswer: false,
			wantRoute:      RouteClarify,
			wantNeedsRepo:  false,
		},
		{
			name: "directive over the cap is truncated with ellipsis",
			in: TurnPolicy{
				Route:                 RouteLocal,
				Operation:             "transform",
				Source:                "last_answer",
				Confidence:            0.9,
				PresentationDirective: strings.Repeat("a", presentationDirectiveCap+50),
			},
			hasPriorAnswer: true,
			wantRoute:      RouteLocal,
			wantNeedsRepo:  false,
		},
		// Attachment guard (#3 fix). Three structural cases:
		// (a) attachment + no prior + local → demote to repo; the
		// local responder cannot consume an attached log/trace,
		// so re-routing through the pipeline (which DOES, via
		// log_triage / perf_triage) is the only path that
		// produces a faithful answer.
		// (b) attachment + WITH prior + local → stay local; the
		// previous pipeline turn already encoded the attachment-
		// derived facts, so a transform of THAT answer is fine.
		// (c) attachment + WITHOUT prior + chat operation → stay
		// local; pure greeting after a sticky log shouldn't
		// kick to pipeline (matches the chitchat soft-signal
		// rule: only message-level attachment references force
		// the upgrade).
		{
			name: "attachment + no prior + local transform → repo (cannot consume attachment locally)",
			in: TurnPolicy{
				Route:      RouteLocal,
				Operation:  "investigate",
				Source:     "current_message",
				Confidence: 0.6,
			},
			hasPriorAnswer: false,
			hasAttachment:  true,
			wantRoute:      RouteRepo,
			wantNeedsRepo:  true,
		},
		{
			name: "attachment + WITH prior + local transform → stay local (prior already covered attachment)",
			in: TurnPolicy{
				Route:      RouteLocal,
				Operation:  "transform",
				Source:     "last_answer",
				Confidence: 0.9,
			},
			hasPriorAnswer: true,
			hasAttachment:  true,
			wantRoute:      RouteLocal,
			wantNeedsRepo:  false,
		},
		{
			name: "attachment + no prior + chat → stay local (attachment is soft)",
			in: TurnPolicy{
				Route:      RouteLocal,
				Operation:  "chat",
				Source:     "current_message",
				Confidence: 0.95,
			},
			hasPriorAnswer: false,
			hasAttachment:  true,
			// chat operation is exempted by the LLM-side
			// classification (a greeting after sticky log is
			// not an attachment-routing event); but if the LLM
			// chose local and the guard sees no prior, the
			// attachment guard fires for ANY route=local without
			// prior. This is the conservative behavior we want
			// — the operator can always re-ask without a sticky
			// log; sending greetings through the pipeline
			// because of an unrelated attachment is the cheaper
			// failure mode than fabricating around a log.
			wantRoute:     RouteRepo,
			wantNeedsRepo: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ApplyTurnPolicyGuards(tc.in, tc.hasPriorAnswer, tc.hasAttachment)
			if got.Route != tc.wantRoute {
				t.Errorf("Route: got %q, want %q", got.Route, tc.wantRoute)
			}
			if got.NeedsRepoAccess != tc.wantNeedsRepo {
				t.Errorf("NeedsRepoAccess: got %v, want %v", got.NeedsRepoAccess, tc.wantNeedsRepo)
			}
			if r := []rune(got.PresentationDirective); len(r) > presentationDirectiveCap+1 {
				t.Errorf("directive runes %d exceeds cap %d", len(r), presentationDirectiveCap+1)
			}
		})
	}
}

func TestApplyTurnPolicyGuards_OperationRoute(t *testing.T) {
	got := ApplyTurnPolicyGuards(TurnPolicy{
		Route:                RouteOperation,
		NeedsRepoAccess:      true,
		Operation:            "presentation_generation",
		Source:               "mixed",
		Confidence:           0.86,
		SideEffects:          []string{"local_file_write"},
		TargetSurface:        "slides",
		RequiresConfirmation: false,
	}, false, false)
	if got.Route != RouteOperation {
		t.Fatalf("Route=%q, want operation", got.Route)
	}
	if !got.NeedsOperationAccess {
		t.Fatal("route=operation must set NeedsOperationAccess")
	}
	if !got.NeedsRepoAccess {
		t.Fatal("operation may preserve needs_repo_access for mixed repo→artifact workflows")
	}
	if got.OperationKind != "presentation_generation" {
		t.Fatalf("OperationKind=%q, want presentation_generation", got.OperationKind)
	}
	if got.RiskLevel != "low" {
		t.Fatalf("RiskLevel=%q, want default low", got.RiskLevel)
	}

	low := ApplyTurnPolicyGuards(TurnPolicy{
		Route:      RouteOperation,
		Operation:  "browser_operation",
		Source:     "external_tool",
		Confidence: 0.2,
	}, false, false)
	if low.Route != RouteRepo || !low.NeedsRepoAccess || low.NeedsOperationAccess {
		t.Fatalf("low-confidence operation should fail safe to repo without operation access: %+v", low)
	}

	contradiction := ApplyTurnPolicyGuards(TurnPolicy{
		Route:                RouteLocal,
		NeedsOperationAccess: true,
		Operation:            "computer_operation",
		OperationKind:        "computer_operation",
		Source:               "current_message",
		Confidence:           0.88,
	}, false, false)
	if contradiction.Route != RouteOperation || !contradiction.NeedsOperationAccess {
		t.Fatalf("typed operation access must override local contradiction: %+v", contradiction)
	}

	externalObservation := ApplyTurnPolicyGuards(TurnPolicy{
		Route:                RouteOperation,
		NeedsOperationAccess: true,
		Operation:            "investigate",
		Source:               "artifact",
		RiskLevel:            "none",
		TargetSurface:        "unknown",
		Confidence:           0.9,
	}, false, false)
	if externalObservation.Route != RouteRepo ||
		!externalObservation.NeedsRepoAccess ||
		externalObservation.NeedsOperationAccess {
		t.Fatalf("analysis-only external observation must stay in pipeline: %+v", externalObservation)
	}

	externalObservationBoolDrift := ApplyTurnPolicyGuards(TurnPolicy{
		Route:                RouteRepo,
		NeedsOperationAccess: true,
		Operation:            "investigate",
		Source:               "external_tool",
		RiskLevel:            "none",
		Confidence:           0.89,
	}, false, false)
	if externalObservationBoolDrift.Route != RouteRepo ||
		!externalObservationBoolDrift.NeedsRepoAccess ||
		externalObservationBoolDrift.NeedsOperationAccess {
		t.Fatalf("analysis-only external observation must ignore stray operation boolean: %+v", externalObservationBoolDrift)
	}

	currentSourceInvestigation := ApplyTurnPolicyGuards(TurnPolicy{
		Route:                RouteOperation,
		NeedsOperationAccess: true,
		Operation:            "investigate",
		Source:               "repo",
		RiskLevel:            "low",
		Confidence:           0.87,
	}, false, false)
	if currentSourceInvestigation.Route != RouteRepo ||
		!currentSourceInvestigation.NeedsRepoAccess ||
		currentSourceInvestigation.NeedsOperationAccess {
		t.Fatalf("analysis-only current-source investigation must stay in pipeline: %+v", currentSourceInvestigation)
	}

	mcpObservationDrift := ApplyTurnPolicyGuards(TurnPolicy{
		Route:                RouteRepo,
		NeedsRepoAccess:      false,
		NeedsOperationAccess: true,
		Operation:            "investigate",
		OperationKind:        "",
		Source:               "external_tool",
		RiskLevel:            "low",
		Confidence:           0.9,
		Reason:               "MCP rows are external observations handled by the analysis pipeline",
	}, false, false)
	if mcpObservationDrift.Route != RouteRepo ||
		!mcpObservationDrift.NeedsRepoAccess ||
		mcpObservationDrift.NeedsOperationAccess ||
		IsConcreteOperationPolicy(mcpObservationDrift) {
		t.Fatalf("MCP observation drift must not become command operation: %+v", mcpObservationDrift)
	}

	mcpExternalSkillDrift := ApplyTurnPolicyGuards(TurnPolicy{
		Route:                RouteOperation,
		NeedsOperationAccess: true,
		Operation:            "external_skill_workflow",
		OperationKind:        "external_skill_workflow",
		Source:               "external_tool",
		RiskLevel:            "low",
		SideEffects:          []string{"network_read"},
		TargetSurface:        "external_system",
		Confidence:           0.9,
		Reason:               "Read-only external tool rows should be interpreted as observations",
	}, false, false)
	if mcpExternalSkillDrift.Route != RouteRepo ||
		!mcpExternalSkillDrift.NeedsRepoAccess ||
		mcpExternalSkillDrift.NeedsOperationAccess ||
		IsConcreteOperationPolicy(mcpExternalSkillDrift) {
		t.Fatalf("read-only external skill drift must stay in analysis pipeline: %+v", mcpExternalSkillDrift)
	}
	mcpHint := TurnRouteHintFromPolicy(mcpExternalSkillDrift)
	if !mcpHint.ExternalObservationFirst() ||
		mcpHint.Source != "external_tool" ||
		mcpHint.ConcreteOperation ||
		mcpHint.NeedsOperationAccess {
		t.Fatalf("read-only external skill drift should produce external-observation-first hint: %+v", mcpHint)
	}

	realExternalSkill := ApplyTurnPolicyGuards(TurnPolicy{
		Route:                RouteOperation,
		NeedsOperationAccess: true,
		Operation:            "external_skill_workflow",
		OperationKind:        "external_skill_workflow",
		Source:               "current_message",
		RiskLevel:            "low",
		TargetSurface:        "slides",
		Confidence:           0.9,
		Reason:               "A configured external skill will generate a slide artifact",
	}, false, false)
	if realExternalSkill.Route != RouteOperation ||
		!realExternalSkill.NeedsOperationAccess ||
		!IsConcreteOperationPolicy(realExternalSkill) {
		t.Fatalf("concrete external skill with target surface must remain operation: %+v", realExternalSkill)
	}

	externalSkillWrite := ApplyTurnPolicyGuards(TurnPolicy{
		Route:                RouteOperation,
		NeedsOperationAccess: true,
		Operation:            "external_skill_workflow",
		OperationKind:        "external_skill_workflow",
		Source:               "current_message",
		RiskLevel:            "medium",
		SideEffects:          []string{"network_submit"},
		TargetSurface:        "external_system",
		Confidence:           0.9,
		Reason:               "A configured external skill will submit data to an external system",
	}, false, false)
	if externalSkillWrite.Route != RouteOperation ||
		!externalSkillWrite.NeedsOperationAccess ||
		!IsConcreteOperationPolicy(externalSkillWrite) {
		t.Fatalf("external skill with concrete side effect must remain operation: %+v", externalSkillWrite)
	}
}

func TestApplyTurnPolicyGuards_DataRoute(t *testing.T) {
	got := ApplyTurnPolicyGuards(TurnPolicy{
		Route:                RouteOperation,
		NeedsOperationAccess: true,
		NeedsDataAccess:      true,
		Operation:            "data_aggregation",
		DataTaskKind:         "data_aggregation",
		Source:               "data",
		RiskLevel:            "medium",
		SideEffects:          []string{"local_file_write"},
		TargetSurface:        "spreadsheet",
		RequiresConfirmation: true,
		Confidence:           0.88,
	}, false, false)
	if got.Route != RouteData {
		t.Fatalf("Route=%q, want data: %+v", got.Route, got)
	}
	if got.NeedsRepoAccess || got.NeedsOperationAccess || !got.NeedsDataAccess {
		t.Fatalf("data route access flags wrong: %+v", got)
	}
	if got.DataTaskKind != "data_aggregation" || got.Operation != "data_aggregation" {
		t.Fatalf("data operation/kind not retained: %+v", got)
	}
	if got.RiskLevel != "medium" {
		t.Fatalf("RiskLevel=%q, want preserve model risk", got.RiskLevel)
	}
	if len(got.SideEffects) != 0 || got.TargetSurface != "" || got.RequiresConfirmation {
		t.Fatalf("pure data route must not carry operation side effects: %+v", got)
	}

	low := ApplyTurnPolicyGuards(TurnPolicy{
		Route:           RouteData,
		NeedsDataAccess: true,
		Operation:       "data_cleaning",
		Source:          "data",
		Confidence:      0.2,
	}, false, false)
	if low.Route != RouteRepo || !low.NeedsRepoAccess || low.NeedsDataAccess {
		t.Fatalf("low-confidence data should fail safe to repo: %+v", low)
	}

	trace := ApplyTurnPolicyGuards(TurnPolicy{
		Route:                RouteRepo,
		NeedsRepoAccess:      true,
		NeedsOperationAccess: false,
		NeedsDataAccess:      false,
		Operation:            "investigate",
		Source:               "artifact",
		Confidence:           0.9,
	}, false, true)
	if trace.Route != RouteRepo || !trace.NeedsRepoAccess || trace.NeedsDataAccess || trace.NeedsOperationAccess {
		t.Fatalf("trace/log external observation must remain repo pipeline: %+v", trace)
	}

	traceWithStrayData := ApplyTurnPolicyGuards(TurnPolicy{
		Route:                RouteRepo,
		NeedsRepoAccess:      true,
		NeedsOperationAccess: false,
		NeedsDataAccess:      true,
		Operation:            "investigate",
		Source:               "artifact",
		Confidence:           0.95,
		Reason:               "runtime trace analysis uses an attached artifact",
	}, false, true)
	if traceWithStrayData.Route != RouteRepo ||
		!traceWithStrayData.NeedsRepoAccess ||
		traceWithStrayData.NeedsDataAccess ||
		traceWithStrayData.NeedsOperationAccess ||
		traceWithStrayData.DataTaskKind != "" {
		t.Fatalf("analysis-only trace policy must ignore stray data access: %+v", traceWithStrayData)
	}

	traceDataRouteDrift := ApplyTurnPolicyGuards(TurnPolicy{
		Route:           RouteData,
		NeedsDataAccess: true,
		Operation:       "investigate",
		Source:          "external_tool",
		Confidence:      0.95,
		Reason:          "external observation analysis was misrouted as data",
	}, false, true)
	if traceDataRouteDrift.Route != RouteRepo ||
		!traceDataRouteDrift.NeedsRepoAccess ||
		traceDataRouteDrift.NeedsDataAccess ||
		traceDataRouteDrift.NeedsOperationAccess ||
		traceDataRouteDrift.DataTaskKind != "" {
		t.Fatalf("analysis-only external observation must recover from data route drift: %+v", traceDataRouteDrift)
	}
}

func TestApplyTurnPolicyGuards_WriteRoute(t *testing.T) {
	got := ApplyTurnPolicyGuards(TurnPolicy{
		Route:           RouteWrite,
		NeedsRepoAccess: false,
		Operation:       "code_change",
		WriteIntent:     WriteIntentExplicitChange,
		Source:          "repo",
		Confidence:      0.86,
		RiskLevel:       "medium",
		SideEffects:     []string{"local_file_write"},
		TargetSurface:   "file_artifact",
	}, false, false)
	if got.Route != RouteWrite {
		t.Fatalf("Route=%q, want write", got.Route)
	}
	if !got.NeedsRepoAccess || got.NeedsOperationAccess || got.NeedsDataAccess {
		t.Fatalf("write route access flags wrong: %+v", got)
	}
	if got.Operation != "code_change" || got.Source != "repo" {
		t.Fatalf("write operation/source not normalized: %+v", got)
	}
	if got.RiskLevel != "medium" || len(got.SideEffects) != 0 || got.TargetSurface != "" || got.RequiresConfirmation {
		t.Fatalf("write planning must not carry operation-side-effect authorization: %+v", got)
	}

	typedSignal := ApplyTurnPolicyGuards(TurnPolicy{
		Route:       RouteRepo,
		Operation:   "code_change",
		WriteIntent: WriteIntentExplicitChange,
		Source:      "repo",
		Confidence:  0.9,
	}, false, false)
	if typedSignal.Route != RouteWrite || !typedSignal.NeedsRepoAccess {
		t.Fatalf("typed code_change signal should route to write planning: %+v", typedSignal)
	}

	analysisOnly := ApplyTurnPolicyGuards(TurnPolicy{
		Route:       RouteWrite,
		Operation:   "code_change",
		WriteIntent: WriteIntentAnalysisOnly,
		Source:      "repo",
		Confidence:  0.9,
	}, false, false)
	if analysisOnly.Route != RouteRepo || analysisOnly.Operation != "investigate" || analysisOnly.WriteIntent != WriteIntentAnalysisOnly {
		t.Fatalf("analysis-only write drift should recover to repo investigation: %+v", analysisOnly)
	}

	ambiguous := ApplyTurnPolicyGuards(TurnPolicy{
		Route:       RouteRepo,
		Operation:   "code_change",
		WriteIntent: WriteIntentAmbiguous,
		Source:      "repo",
		Confidence:  0.9,
	}, false, false)
	if ambiguous.Route != RouteRepo || ambiguous.Operation != "investigate" || ambiguous.WriteIntent != WriteIntentAmbiguous {
		t.Fatalf("ambiguous code_change should not promote to write: %+v", ambiguous)
	}

	missingIntent := ApplyTurnPolicyGuards(TurnPolicy{
		Route:      RouteWrite,
		Operation:  "code_change",
		Source:     "repo",
		Confidence: 0.9,
	}, false, false)
	if missingIntent.Route != RouteRepo || missingIntent.Operation != "investigate" {
		t.Fatalf("missing write_intent should not enter write Auto Pilot: %+v", missingIntent)
	}

	low := ApplyTurnPolicyGuards(TurnPolicy{
		Route:           RouteWrite,
		NeedsRepoAccess: true,
		Operation:       "code_change",
		WriteIntent:     WriteIntentExplicitChange,
		Source:          "repo",
		Confidence:      0.2,
	}, false, false)
	if low.Route != RouteRepo || !low.NeedsRepoAccess || low.Operation != "investigate" {
		t.Fatalf("low-confidence write should fail safe to repo investigation: %+v", low)
	}
}

// ─── Layer 2: ClassifyPolicy parses tool calls ───────────────

// turnPolicyResp builds a single canned llm.Response carrying an
// emit_turn_policy tool call with the given fields. Mirrors the
// classifierResp helper on the chitchat side.
func turnPolicyResp(payload string) llm.Response {
	return llm.Response{
		ToolCalls: []llm.ToolCall{
			{
				ID:     "call-tp-1",
				Name:   "emit_turn_policy",
				Params: []byte(payload),
			},
		},
	}
}

// TestClassifyPolicy_LocalTransform locks the happy-path parse for
// the most-frequent shape: route=local + transform + last_answer +
// presentation_directive. Pins both the parsed TurnPolicy and the
// fact that the user content carries last_answer_present=true.
func TestClassifyPolicy_LocalTransform(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			turnPolicyResp(`{"route":"local","needs_repo_access":false,"operation":"transform","source":"last_answer","confidence":0.92,"reason":"transformation of previous answer","presentation_directive":"mermaid 流程图"}`),
		},
	}
	c := &llmChitchatClassifier{adapter: adapter}

	policy, err := c.ClassifyPolicy(context.Background(), "换成 mermaid 图例", "kind=pipeline topic=HiTrace flow", true)
	if err != nil {
		t.Fatalf("ClassifyPolicy: %v", err)
	}
	if policy.Route != RouteLocal {
		t.Errorf("Route: got %q, want local", policy.Route)
	}
	if policy.Operation != "transform" {
		t.Errorf("Operation: got %q, want transform", policy.Operation)
	}
	if policy.PresentationDirective != "mermaid 流程图" {
		t.Errorf("PresentationDirective: got %q", policy.PresentationDirective)
	}
	if policy.Confidence < 0.9 {
		t.Errorf("Confidence: got %v, want ≥0.9", policy.Confidence)
	}

	// User content must carry the structured signals the prompt depends on.
	if len(adapter.calls) != 1 {
		t.Fatalf("Chat calls: got %d, want 1", len(adapter.calls))
	}
	user := ""
	for i := len(adapter.calls[0].messages) - 1; i >= 0; i-- {
		if adapter.calls[0].messages[i].Role == "user" {
			user = adapter.calls[0].messages[i].Content
			break
		}
	}
	if !strings.Contains(user, "## last_answer_present: true") {
		t.Errorf("user content must carry last_answer_present=true; got %q", user)
	}
	if !strings.Contains(user, "## current_repository_available: true") {
		t.Errorf("user content must carry repository availability; got %q", user)
	}
	if !strings.Contains(user, "## priorTurn:") {
		t.Errorf("user content must carry priorTurn header; got %q", user)
	}
	if !strings.Contains(user, "换成 mermaid 图例") {
		t.Errorf("user content must carry the current message; got %q", user)
	}
}

// TestClassifyPolicy_NoPriorAnswerCarriesFalseSignal pins the
// hasPriorAnswer=false code path: the user content reflects the
// missing-prior signal so the LLM can demote to clarify on its own,
// even before our deterministic guard runs.
func TestClassifyPolicy_NoPriorAnswerCarriesFalseSignal(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			turnPolicyResp(`{"route":"clarify","needs_repo_access":false,"operation":"transform","source":"last_answer","confidence":0.85,"reason":"no previous answer to transform"}`),
		},
	}
	c := &llmChitchatClassifier{adapter: adapter}

	policy, err := c.ClassifyPolicy(context.Background(), "换成 mermaid 图例", "", false)
	if err != nil {
		t.Fatalf("ClassifyPolicy: %v", err)
	}
	if policy.Route != RouteClarify {
		t.Errorf("Route: got %q, want clarify", policy.Route)
	}

	user := ""
	for i := len(adapter.calls[0].messages) - 1; i >= 0; i-- {
		if adapter.calls[0].messages[i].Role == "user" {
			user = adapter.calls[0].messages[i].Content
			break
		}
	}
	if !strings.Contains(user, "## last_answer_present: false") {
		t.Errorf("user content must signal last_answer_present=false; got %q", user)
	}
}

// TestClassifyPolicy_RejectsUnknownRoute pins the fail-safe contract:
// any unknown enum value is an error, the dispatcher's gate sees it,
// and falls through to the pipeline.
func TestClassifyPolicy_RejectsUnknownRoute(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			turnPolicyResp(`{"route":"random","needs_repo_access":false,"operation":"chat","source":"current_message","confidence":0.5,"reason":"x"}`),
		},
	}
	c := &llmChitchatClassifier{adapter: adapter}
	if _, err := c.ClassifyPolicy(context.Background(), "hi", "", false); err == nil {
		t.Fatal("expected error on unknown route enum")
	}
}

func TestClassifyPolicy_TimesOutRouteClassifier(t *testing.T) {
	old := turnPolicyClassifierTimeout
	turnPolicyClassifierTimeout = 10 * time.Millisecond
	defer func() { turnPolicyClassifierTimeout = old }()

	c := &llmChitchatClassifier{adapter: blockingTurnPolicyAdapter{}}
	start := time.Now()
	_, err := c.ClassifyPolicy(context.Background(), "分析这个项目的架构", "", false)
	if err == nil {
		t.Fatal("blocking classifier should return a timeout error")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("classifier timeout took too long: %s", elapsed)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error should wrap context deadline exceeded, got %v", err)
	}
}

func TestSetTurnPolicyClassifierTimeout(t *testing.T) {
	old := turnPolicyClassifierTimeout
	defer func() { turnPolicyClassifierTimeout = old }()

	SetTurnPolicyClassifierTimeout(3 * time.Second)
	if turnPolicyClassifierTimeout != 3*time.Second {
		t.Fatalf("timeout = %s, want 3s", turnPolicyClassifierTimeout)
	}
	SetTurnPolicyClassifierTimeout(0)
	if turnPolicyClassifierTimeout != 3*time.Second {
		t.Fatalf("non-positive override must keep current guard, got %s", turnPolicyClassifierTimeout)
	}
}

// singleShotDataPolicyJSON is the canned classifier output used by the
// single-shot lane tests: a data-route answer matching the customer shape
// (aggregate local structured data, strict output).
const singleShotDataPolicyJSON = `{"route":"data","needs_data_access":true,"operation":"transform","data_task_kind":"data_aggregation","source":"current_message","confidence":0.93,"reason":"aggregate attached structured data"}`

// TestClassifyPolicy_SingleShotLaneSurvivesBetweenDeadlinesSleep is the
// structural pin for the 2026-07 data-route fix: the REPL and single-shot
// wall clocks are INDEPENDENT deadlines. The adapter answers strictly
// between the two (repl < sleep < single_shot; scaled 100ms < 300ms < 2s so
// the suite stays fast — the pinned property is the ordering, not the
// production literals 10s/60s):
//
//   - the single-shot lane must still classify route=data, proving no layer
//     below it (ClassifyPolicy's ctx wrap, chatWithClassifierHardTimeout's
//     guard, or anything added later) re-applies the shorter interactive
//     clock as a second deadline;
//   - the REPL lane with the SAME adapter must degrade with
//     context.DeadlineExceeded, proving the interactive guard is intact.
func TestClassifyPolicy_SingleShotLaneSurvivesBetweenDeadlinesSleep(t *testing.T) {
	oldRepl := turnPolicyClassifierTimeout
	oldSingle := singleShotRoutePolicyTimeout
	turnPolicyClassifierTimeout = 100 * time.Millisecond
	singleShotRoutePolicyTimeout = 2 * time.Second
	defer func() {
		turnPolicyClassifierTimeout = oldRepl
		singleShotRoutePolicyTimeout = oldSingle
	}()

	var sawDeadline bool
	var deadlineAhead time.Duration
	adapter := slowTurnPolicyAdapter{
		delay:         300 * time.Millisecond,
		resp:          turnPolicyResp(singleShotDataPolicyJSON),
		sawDeadline:   &sawDeadline,
		deadlineAhead: &deadlineAhead,
	}
	c := &llmChitchatClassifier{adapter: adapter}

	policy, err := c.ClassifyPolicySingleShot(context.Background(), "统计各列平均值并输出 JSON", "", false)
	if err != nil {
		t.Fatalf("single-shot lane must survive an adapter sleep between the two deadlines: %v", err)
	}
	if policy.Route != RouteData {
		t.Errorf("Route: got %q, want data", policy.Route)
	}
	if !policy.NeedsDataAccess {
		t.Error("NeedsDataAccess: got false, want true")
	}
	// The one deadline the adapter sees must be the single-shot clock, not
	// the (shorter) interactive one: with repl=100ms and single-shot=2s, a
	// deadline more than 1s ahead can only be the single-shot clock.
	if !sawDeadline {
		t.Fatal("single-shot lane with a positive timeout must carry exactly one ctx deadline")
	}
	if deadlineAhead <= 1*time.Second {
		t.Fatalf("adapter saw a deadline only %s ahead — a second, shorter clock leaked onto the single-shot lane", deadlineAhead)
	}

	// REPL lane, same adapter and delay: the interactive wall clock still
	// degrades. This half keeps the REPL ergonomics contract pinned so the
	// lane split cannot silently loosen it.
	if _, err := c.ClassifyPolicy(context.Background(), "统计各列平均值并输出 JSON", "", false); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("REPL lane must keep its interactive timeout, got err=%v", err)
	}
}

// TestClassifyPolicySingleShot_TimesOutAtOwnDeadlineAsDeadlineExceeded pins
// two facts: (1) the single-shot lane times out at ITS OWN clock even when
// the interactive clock is much longer, so the degrade decision keys off the
// right knob; (2) the failure surfaces as context.DeadlineExceeded — the
// precise signal cmd/root.go requires to emit the pinned "route-degrade:"
// event instead of the generic classifier-failed warning.
func TestClassifyPolicySingleShot_TimesOutAtOwnDeadlineAsDeadlineExceeded(t *testing.T) {
	oldRepl := turnPolicyClassifierTimeout
	oldSingle := singleShotRoutePolicyTimeout
	turnPolicyClassifierTimeout = 10 * time.Second
	singleShotRoutePolicyTimeout = 50 * time.Millisecond
	defer func() {
		turnPolicyClassifierTimeout = oldRepl
		singleShotRoutePolicyTimeout = oldSingle
	}()

	c := &llmChitchatClassifier{adapter: blockingTurnPolicyAdapter{}}
	start := time.Now()
	_, err := c.ClassifyPolicySingleShot(context.Background(), "统计各列平均值", "", false)
	if err == nil {
		t.Fatal("blocking adapter must time out on the single-shot clock")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("single-shot timeout must wrap context.DeadlineExceeded (drives the route-degrade event), got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("timed out on the wrong (interactive 10s) clock: %s", elapsed)
	}
}

// TestClassifyPolicySingleShot_ZeroTimeoutDisablesOuterDeadline pins the
// 0-means-disabled contract: with the knob at 0 the single-shot lane runs
// with NO outer wall clock (adapter-native first-byte/stall/retry guards
// govern) and — structurally — no residual interactive guard either, even
// when that guard sits far below the adapter latency.
func TestClassifyPolicySingleShot_ZeroTimeoutDisablesOuterDeadline(t *testing.T) {
	oldRepl := turnPolicyClassifierTimeout
	oldSingle := singleShotRoutePolicyTimeout
	turnPolicyClassifierTimeout = 50 * time.Millisecond
	singleShotRoutePolicyTimeout = 0
	defer func() {
		turnPolicyClassifierTimeout = oldRepl
		singleShotRoutePolicyTimeout = oldSingle
	}()

	var sawDeadline bool
	adapter := slowTurnPolicyAdapter{
		delay:       200 * time.Millisecond,
		resp:        turnPolicyResp(singleShotDataPolicyJSON),
		sawDeadline: &sawDeadline,
	}
	c := &llmChitchatClassifier{adapter: adapter}

	policy, err := c.ClassifyPolicySingleShot(context.Background(), "统计各列平均值并输出 JSON", "", false)
	if err != nil {
		t.Fatalf("disabled outer deadline must let the adapter finish: %v", err)
	}
	if policy.Route != RouteData {
		t.Errorf("Route: got %q, want data", policy.Route)
	}
	if sawDeadline {
		t.Fatal("timeout=0 must mean NO ctx deadline reaches the adapter on the single-shot lane")
	}
}

// TestSetSingleShotRoutePolicyTimeout pins the setter contract, which
// deliberately differs from SetTurnPolicyClassifierTimeout: zero is a
// meaningful operator choice (disable the outer deadline), only negatives
// keep the current guard.
func TestSetSingleShotRoutePolicyTimeout(t *testing.T) {
	old := singleShotRoutePolicyTimeout
	defer func() { singleShotRoutePolicyTimeout = old }()

	SetSingleShotRoutePolicyTimeout(45 * time.Second)
	if singleShotRoutePolicyTimeout != 45*time.Second {
		t.Fatalf("timeout = %s, want 45s", singleShotRoutePolicyTimeout)
	}
	if got := SingleShotRoutePolicyTimeout(); got != 45*time.Second {
		t.Fatalf("getter = %s, want 45s", got)
	}
	SetSingleShotRoutePolicyTimeout(0)
	if singleShotRoutePolicyTimeout != 0 {
		t.Fatalf("zero must DISABLE the single-shot outer deadline, got %s", singleShotRoutePolicyTimeout)
	}
	SetSingleShotRoutePolicyTimeout(-time.Second)
	if singleShotRoutePolicyTimeout != 0 {
		t.Fatalf("negative override must keep current value, got %s", singleShotRoutePolicyTimeout)
	}
}

func TestSetMemoryContextTimeout(t *testing.T) {
	old := replMemoryContextTimeout
	defer func() { replMemoryContextTimeout = old }()

	SetMemoryContextTimeout(1500 * time.Millisecond)
	if replMemoryContextTimeout != 1500*time.Millisecond {
		t.Fatalf("timeout = %s, want 1500ms", replMemoryContextTimeout)
	}
	SetMemoryContextTimeout(0)
	if replMemoryContextTimeout != 1500*time.Millisecond {
		t.Fatalf("non-positive override must keep current guard, got %s", replMemoryContextTimeout)
	}
}

func TestRunBoundedMemoryContextTimesOut(t *testing.T) {
	release := make(chan struct{})
	defer close(release)

	start := time.Now()
	prior, timedOut := runBoundedMemoryContext(10*time.Millisecond, func() string {
		<-release
		return "late prior"
	})
	if !timedOut {
		t.Fatalf("timedOut=false, want true")
	}
	if prior != "" {
		t.Fatalf("prior=%q, want empty on timeout", prior)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("memory context timeout took too long: %s", elapsed)
	}
}

func TestClassifyPolicy_HardTimesOutNonResponsiveRouteClassifier(t *testing.T) {
	old := turnPolicyClassifierTimeout
	turnPolicyClassifierTimeout = 10 * time.Millisecond
	defer func() { turnPolicyClassifierTimeout = old }()
	release := make(chan struct{})
	defer close(release)

	c := &llmChitchatClassifier{adapter: nonResponsiveTurnPolicyAdapter{release: release}}
	start := time.Now()
	_, err := c.ClassifyPolicy(context.Background(), "分析这个项目的架构", "", false)
	if err == nil {
		t.Fatal("non-responsive classifier should return a timeout error")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("hard timeout took too long: %s", elapsed)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("hard timeout error should wrap context deadline exceeded, got %v", err)
	}
}

func TestClassify_HardTimesOutNonResponsiveLegacyClassifier(t *testing.T) {
	old := turnPolicyClassifierTimeout
	turnPolicyClassifierTimeout = 10 * time.Millisecond
	defer func() { turnPolicyClassifierTimeout = old }()
	release := make(chan struct{})
	defer close(release)

	c := &llmChitchatClassifier{adapter: nonResponsiveTurnPolicyAdapter{release: release}}
	start := time.Now()
	_, err := c.Classify(context.Background(), "分析这个项目的架构", "")
	if err == nil {
		t.Fatal("non-responsive legacy classifier should return a timeout error")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("legacy hard timeout took too long: %s", elapsed)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("legacy hard timeout error should wrap context deadline exceeded, got %v", err)
	}
}

func TestClassifyPolicy_OperationCompatJSON(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			turnPolicyResp(`{"route":"operation","needs_repo_access":"true","needs_operation_access":"true","operation":"presentation_generation","source":"mixed","confidence":"0.91","reason":"user asked for a PPT","operation_kind":"presentation_generation","risk_level":"low","side_effects":"local_file_write, browser_ui","target_surface":"slides","requires_confirmation":"false",}` + "\ntrailing"),
		},
	}
	c := &llmChitchatClassifier{adapter: adapter}

	policy, err := c.ClassifyPolicy(context.Background(), "基于当前代码生成一份 PPT", "", false)
	if err != nil {
		t.Fatalf("ClassifyPolicy: %v", err)
	}
	if policy.Route != RouteOperation {
		t.Fatalf("Route=%q, want operation", policy.Route)
	}
	if !policy.NeedsRepoAccess || !policy.NeedsOperationAccess {
		t.Fatalf("repo/operation access flags not parsed: %+v", policy)
	}
	if policy.OperationKind != "presentation_generation" {
		t.Fatalf("OperationKind=%q", policy.OperationKind)
	}
	if got := strings.Join(policy.SideEffects, ","); got != "local_file_write,browser_ui" {
		t.Fatalf("SideEffects=%q", got)
	}
	if policy.Confidence < 0.9 {
		t.Fatalf("Confidence=%v, want parsed string number", policy.Confidence)
	}
}

func TestClassifyPolicy_CompatJSONSchemaKeyAliases(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			turnPolicyResp(`{"route":"operation","needsRepoAccess":"false","needsOperationAccess":"true","operation":"computer_operation","operationKind":"computer_operation","source":"current_message","confidence":"0.88","reason":"current machine query","riskLevel":"low","sideEffects":"network_read","targetSurface":"desktop","requiresConfirmation":"false"}`),
		},
	}
	c := &llmChitchatClassifier{adapter: adapter}

	policy, err := c.ClassifyPolicy(context.Background(), "当前机器有哪些模型服务", "", false)
	if err != nil {
		t.Fatalf("ClassifyPolicy: %v", err)
	}
	if policy.Route != RouteOperation || policy.NeedsRepoAccess || !policy.NeedsOperationAccess {
		t.Fatalf("route/access aliases not decoded: %+v", policy)
	}
	if policy.OperationKind != "computer_operation" || policy.RiskLevel != "low" || policy.TargetSurface != "desktop" {
		t.Fatalf("operation aliases not decoded: %+v", policy)
	}
	if policy.RequiresConfirmation {
		t.Fatalf("requiresConfirmation alias/string bool not decoded: %+v", policy)
	}
	if got := strings.Join(policy.SideEffects, ","); got != "network_read" {
		t.Fatalf("sideEffects alias not decoded: %q", got)
	}
}

func TestClassifyPolicy_TeachesUnsafeOperationRoute(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			turnPolicyResp(`{"route":"operation","needs_repo_access":false,"needs_operation_access":true,"operation":"computer_operation","operation_kind":"computer_operation","source":"current_message","confidence":0.91,"reason":"dangerous operation should be blocked by operation policy","risk_level":"high","side_effects":["destructive"],"target_surface":"desktop","requires_confirmation":true}`),
		},
	}
	c := &llmChitchatClassifier{adapter: adapter}

	policy, err := c.ClassifyPolicy(context.Background(), "dangerous command request", "", false)
	if err != nil {
		t.Fatalf("ClassifyPolicy: %v", err)
	}
	if policy.Route != RouteOperation || policy.RiskLevel != "high" {
		t.Fatalf("policy=%+v, want high-risk operation route", policy)
	}
	if len(adapter.calls) != 1 || len(adapter.calls[0].messages) == 0 {
		t.Fatalf("missing classifier call capture: %+v", adapter.calls)
	}
	system := adapter.calls[0].messages[0].Content
	for _, want := range []string{
		"High-risk or forbidden command-operation requests still use",
		"deterministic operation policy will block dangerous commands",
		"Do not reroute unsafe computer-operation requests into repo/source",
	} {
		if !strings.Contains(system, want) {
			t.Fatalf("classifier system prompt missing %q:\n%s", want, system)
		}
	}
}

func TestClassifyPolicy_TeachesBroadComputerOperations(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			turnPolicyResp(`{"route":"operation","needs_repo_access":false,"needs_operation_access":true,"operation":"computer_operation","operation_kind":"computer_operation","source":"current_message","confidence":0.9,"reason":"non-code computer operation","risk_level":"medium","side_effects":["package_install","remote_exec"],"target_surface":"external_system","requires_confirmation":true}`),
		},
	}
	c := &llmChitchatClassifier{adapter: adapter}

	policy, err := c.ClassifyPolicy(context.Background(), "安装一个工具后 ssh 到远端机器检查环境", "", false)
	if err != nil {
		t.Fatalf("ClassifyPolicy: %v", err)
	}
	if policy.Route != RouteOperation || policy.OperationKind != "computer_operation" {
		t.Fatalf("policy=%+v, want computer operation", policy)
	}
	if got := strings.Join(policy.SideEffects, ","); got != "package_install,remote_exec" {
		t.Fatalf("side effects=%q", got)
	}
	system := adapter.calls[0].messages[0].Content
	for _, want := range []string{
		"querying the current machine/environment",
		"downloading files",
		"installing or uninstalling packages",
		"SSH/remote shell work",
		"Clear non-code computer operations remain route=operation",
	} {
		if !strings.Contains(system, want) {
			t.Fatalf("classifier system prompt missing %q:\n%s", want, system)
		}
	}
}

func TestClassifyPolicy_TeachesDataRoute(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			turnPolicyResp(`{"route":"data","needs_repo_access":false,"needs_operation_access":false,"needs_data_access":"true","operation":"data_aggregation","data_task_kind":"data_aggregation","source":"data","confidence":"0.88","reason":"read-only local data aggregation","risk_level":"low","side_effects":[],"requires_confirmation":false}`),
		},
	}
	c := &llmChitchatClassifier{adapter: adapter}

	policy, err := c.ClassifyPolicy(context.Background(), "读取 CSV，按分组键汇总数值字段，只输出 JSON", "", false)
	if err != nil {
		t.Fatalf("ClassifyPolicy: %v", err)
	}
	if policy.Route != RouteData || !policy.NeedsDataAccess || policy.NeedsRepoAccess || policy.NeedsOperationAccess {
		t.Fatalf("policy=%+v, want data route only", policy)
	}
	if policy.DataTaskKind != "data_aggregation" {
		t.Fatalf("DataTaskKind=%q", policy.DataTaskKind)
	}
	system := adapter.calls[0].messages[0].Content
	for _, want := range []string{
		"route=data",
		"structured or semi-structured files/materials",
		"These examples are not exhaustive",
		"Strict output format alone is NOT sufficient for data",
		"JSON-only, CSV-only",
		"source-code implementation analysis",
		"root-cause diagnosis",
	} {
		if !strings.Contains(system, want) {
			t.Fatalf("classifier system prompt missing %q:\n%s", want, system)
		}
	}
}

func TestClassifyPolicy_TeachesWriteRoute(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			turnPolicyResp(`{"route":"write","needs_repo_access":true,"needs_operation_access":false,"needs_data_access":false,"operation":"code_change","write_intent":"explicit_change","source":"repo","confidence":0.87,"reason":"user asked for a repository change","risk_level":"none","side_effects":[],"requires_confirmation":false}`),
		},
	}
	c := &llmChitchatClassifier{adapter: adapter}

	policy, err := c.ClassifyPolicy(context.Background(), "给 CLI 新增 --json 输出，并更新测试", "", false)
	if err != nil {
		t.Fatalf("ClassifyPolicy: %v", err)
	}
	if policy.Route != RouteWrite || !policy.NeedsRepoAccess || policy.NeedsOperationAccess || policy.Operation != "code_change" || policy.WriteIntent != WriteIntentExplicitChange {
		t.Fatalf("policy=%+v, want write Auto Pilot route", policy)
	}
	system := adapter.calls[0].messages[0].Content
	for _, want := range []string{
		"The seven routes",
		"write   —",
		"write Auto Pilot",
		"apply allowed changes in an isolated git worktree",
		"approval record",
		"operation=code_change",
		"write_intent",
		"explicit_change",
		"analysis_only",
		"ambiguous",
		"not enough to enter write Auto Pilot",
	} {
		if !strings.Contains(system, want) {
			t.Fatalf("classifier system prompt missing %q:\n%s", want, system)
		}
	}
}

func TestApplyTurnPolicyGuards_StrictFormatAloneDoesNotBecomeData(t *testing.T) {
	p := ApplyTurnPolicyGuards(TurnPolicy{
		Route:                 RouteRepo,
		NeedsRepoAccess:       true,
		NeedsOperationAccess:  false,
		NeedsDataAccess:       false,
		Operation:             "investigate",
		Source:                "repo",
		PresentationDirective: "JSON-only",
		Confidence:            0.9,
	}, false, false)
	if p.Route != RouteRepo || !p.NeedsRepoAccess || p.NeedsDataAccess || p.DataTaskKind != "" {
		t.Fatalf("policy=%+v, want repo route preserved for source JSON-only output", p)
	}
}

func TestClassifyPolicy_TeachesExternalObservationStaysPipeline(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			turnPolicyResp(`{"route":"repo","needs_repo_access":true,"needs_operation_access":false,"operation":"investigate","source":"artifact","confidence":0.9,"reason":"trace/log analysis without current source"}`),
		},
	}
	c := &llmChitchatClassifier{adapter: adapter}

	policy, err := c.ClassifyPolicy(context.Background(), "只分析这个 trace，不要看代码，找一下 jank 原因", "", false)
	if err != nil {
		t.Fatalf("ClassifyPolicy: %v", err)
	}
	if policy.Route != RouteRepo || policy.NeedsOperationAccess || policy.Operation != "investigate" {
		t.Fatalf("policy=%+v, want repo pipeline external-observation investigation", policy)
	}
	system := adapter.calls[0].messages[0].Content
	for _, want := range []string{
		"fresh LOG / TRACE / MCP / connector external-observation",
		"Do NOT reroute external-observation analysis to operation",
		"direct computer/file operation = operation",
		"evidence interpretation/diagnosis/root-cause = pipeline",
		"只分析这个 trace，不要看代码",
		"只看这段客户日志，不要读取源码",
		"根据 MCP 返回的外部观测解释现象，不要看代码",
		"runtime_artifact=true",
		"runtime_artifact_kind=<log|trace|mixed>",
		"Never use this signal to enter write unless",
	} {
		if !strings.Contains(system, want) {
			t.Fatalf("classifier system prompt missing %q:\n%s", want, system)
		}
	}
}

func TestLocalResponderPromptForbidsVisibleReasoning(t *testing.T) {
	for _, want := range []string{
		"reveal hidden reasoning",
		"meta commentary",
		"the user is asking",
		"I should",
	} {
		if !strings.Contains(localResponderSystemPrompt, want) {
			t.Fatalf("local responder prompt missing %q:\n%s", want, localResponderSystemPrompt)
		}
	}
}

func TestClassifyPolicy_TeachesExplicitFileExtractionOperation(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			turnPolicyResp(`{"route":"operation","needs_repo_access":false,"needs_operation_access":true,"operation":"computer_operation","operation_kind":"computer_operation","source":"current_message","confidence":0.88,"reason":"explicit computer-operation file extraction","risk_level":"low","side_effects":[],"target_surface":"desktop","requires_confirmation":false}`),
		},
	}
	c := &llmChitchatClassifier{adapter: adapter}

	policy, err := c.ClassifyPolicy(context.Background(), "请作为电脑操作读取 docs/design/foo.md，提取某段任务，不要分析源码", "", false)
	if err != nil {
		t.Fatalf("ClassifyPolicy: %v", err)
	}
	if policy.Route != RouteOperation || policy.NeedsRepoAccess {
		t.Fatalf("policy=%+v, want operation without repo access", policy)
	}
	system := adapter.calls[0].messages[0].Content
	for _, want := range []string{
		"Explicit command-operation file reads/searches/extractions",
		"even when the file path is inside",
		"请作为电脑操作读取 docs/design/foo.md",
	} {
		if !strings.Contains(system, want) {
			t.Fatalf("classifier system prompt missing %q:\n%s", want, system)
		}
	}
}

// ─── Layer 3: dispatch() full Loop ─────────────────────────────

// TestTurnPolicyDispatch_LocalTransformReusesAnswer is the marquee
// test for the user-reported bug: "换成 mermaid 图例" with a prior
// repo answer must route to the local responder and never call
// runner.Run.
func TestTurnPolicyDispatch_LocalTransformReusesAnswer(t *testing.T) {
	store := newPolicyStore(t)
	seedPriorAnswer(t, store,
		"HiTraceAnalyzer 处理鸿蒙 trace 的流程",
		"HiTraceAnalyzer 的处理流程包含三步:解析、聚合、渲染。每一步都……")

	classifier := &stubTurnPolicyClassifier{
		policy: TurnPolicy{
			Route:                 RouteLocal,
			Operation:             "transform",
			Source:                "last_answer",
			Confidence:            0.92,
			PresentationDirective: "mermaid 流程图",
			Reason:                "transformation of previous answer",
		},
	}
	responder := &stubLocalResponder{
		localReply: "```mermaid\nflowchart LR\n  A-->B\n```",
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, responder, "换成 mermaid 图例\n/exit\n")
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(classifier.calls) != 1 {
		t.Fatalf("classifier calls: got %d, want 1", len(classifier.calls))
	}
	if !classifier.hadPriorFlags[0] {
		t.Error("hasPriorAnswer should be true with seeded prior turn")
	}
	if len(runner.requests) != 0 {
		t.Errorf("runner.Run must NOT be called for route=local; got %d calls (requests=%v)", len(runner.requests), runner.requests)
	}
	if len(responder.localCalls) != 1 {
		t.Fatalf("local responder calls: got %d, want 1", len(responder.localCalls))
	}
	got := responder.localCalls[0]
	if got.userLine != "换成 mermaid 图例" {
		t.Errorf("local userLine: got %q", got.userLine)
	}
	if got.directive != "mermaid 流程图" {
		t.Errorf("local directive: got %q", got.directive)
	}
	if !strings.Contains(got.lastAnswer, "HiTraceAnalyzer") {
		t.Errorf("local lastAnswer must include the seeded prior answer; got %q", got.lastAnswer)
	}
	// Route summary: dock shutdown line carries "◇ <label> · <segs>"
	// for light routes (renderer-attached path) or the same shape
	// emitted directly via FormatLightRouteSummary (renderer-nil
	// fallback used by this test). Either way the EN label "local
	// reply" must appear in stdout. Substring check survives ANSI
	// styling because pterm only adds escape sequences around the
	// literal text.
	if !strings.Contains(out.String(), "local reply") {
		t.Errorf("local reply route summary missing in stdout; got %q", out.String())
	}
	if !strings.Contains(out.String(), "mermaid") {
		t.Errorf("local reply body missing; got %q", out.String())
	}
}

func TestTurnPolicyDispatch_ClassifierRunsInInFlightLifecycle(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{
		policy: TurnPolicy{
			Route:           RouteRepo,
			NeedsRepoAccess: true,
			Operation:       "investigate",
			Source:          "repo",
			Confidence:      0.9,
			Reason:          "fresh repository investigation",
		},
	}
	r, runner, _ := newTurnPolicyREPL(t, store, classifier, &stubResponder{reply: "unused"}, "分析当前系统有哪些 agent\n/exit\n")
	classifier.onPolicy = func(ctx context.Context) {
		if ctx == nil {
			t.Fatal("classifier ctx is nil")
		}
		if !r.runInFlight.Load() {
			t.Fatal("turn-policy classifier must run inside REPL in-flight lifecycle")
		}
		select {
		case <-ctx.Done():
			t.Fatal("classifier ctx should be live during normal classification")
		default:
		}
	}

	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}
	if len(runner.requests) != 1 {
		t.Fatalf("repo route should dispatch runner once, got %d", len(runner.requests))
	}
	if r.runInFlight.Load() {
		t.Fatal("in-flight lifecycle must clear after dispatch")
	}
}

func TestTurnPolicyDispatch_SecondTurnClassifierOuterTimeoutFallsThroughToPipeline(t *testing.T) {
	old := turnPolicyClassifierTimeout
	turnPolicyClassifierTimeout = 10 * time.Millisecond
	defer func() { turnPolicyClassifierTimeout = old }()

	store := newPolicyStore(t)
	release := make(chan struct{})
	classifier := &secondTurnBlockingPolicyClassifier{release: release}
	defer close(release)
	r, runner, _ := newTurnPolicyREPL(t, store, classifier, &stubResponder{reply: "unused"}, "分析当前系统有哪些 agent\n继续分析关系\n/exit\n")

	done := make(chan error, 1)
	go func() {
		done <- r.Loop()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Loop: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("REPL must not hang when the second-turn classifier ignores context")
	}
	if got := classifier.callCount(); got != 2 {
		t.Fatalf("classifier calls: got %d, want 2", got)
	}
	if len(runner.requests) != 2 {
		t.Fatalf("both turns should fail-safe to the repo pipeline, got %d requests: %v", len(runner.requests), runner.requests)
	}
	if r.runInFlight.Load() {
		t.Fatal("in-flight lifecycle must clear after second-turn timeout")
	}
}

func TestTurnPolicyDispatch_LocalResponderRunsInInFlightLifecycle(t *testing.T) {
	store := newPolicyStore(t)
	seedPriorAnswer(t, store,
		"当前系统有哪些 agent",
		"系统包含 Analyzer、Explorer 等 agent。")

	classifier := &stubTurnPolicyClassifier{
		policy: TurnPolicy{
			Route:                 RouteLocal,
			Operation:             "transform",
			Source:                "last_answer",
			Confidence:            0.92,
			PresentationDirective: "markdown table",
			Reason:                "transformation of previous answer",
		},
	}
	responder := &stubLocalResponder{localReply: "| agent | role |\n|---|---|\n| Analyzer | classify |"}
	r, runner, _ := newTurnPolicyREPL(t, store, classifier, responder, "换成表格\n/exit\n")
	responder.onLocal = func(ctx context.Context) {
		if ctx == nil {
			t.Fatal("local responder ctx is nil")
		}
		if !r.runInFlight.Load() {
			t.Fatal("local responder must run inside REPL in-flight lifecycle")
		}
		select {
		case <-ctx.Done():
			t.Fatal("local responder ctx should be live during normal response")
		default:
		}
	}

	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}
	if len(runner.requests) != 0 {
		t.Fatalf("route=local must not dispatch runner, got %d", len(runner.requests))
	}
	if r.runInFlight.Load() {
		t.Fatal("in-flight lifecycle must clear after local reply")
	}
}

func TestTurnPolicyDispatch_LocalTransformWritesMarkdownAndPreview(t *testing.T) {
	store := newPolicyStore(t)
	seedPriorAnswer(t, store,
		"Explorer 调用 SubAgent 的流程",
		"Explorer 先提出子任务，再由运行时验证、执行、归并。")

	classifier := &stubTurnPolicyClassifier{
		policy: TurnPolicy{
			Route:                 RouteLocal,
			Operation:             "transform",
			Source:                "last_answer",
			Confidence:            0.92,
			PresentationDirective: "mermaid sequence diagram",
		},
	}
	responder := &stubLocalResponder{
		localReply: "```mermaid sequenceDiagram\n  Explorer->>Runtime: propose\n```",
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, responder, "换成时序图\n/exit\n")
	preview := &stubMarkdownPreviewer{url: "http://127.0.0.1:49152/preview/local?token=t"}
	r.outputDumpDir = filepath.Join(t.TempDir(), "output")
	r.outputDumpMax = 10
	r.markdownPreview = preview

	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}
	if len(runner.requests) != 0 {
		t.Fatalf("local transform must not enter pipeline; runner requests=%v", runner.requests)
	}
	printed := out.String()
	if !strings.Contains(printed, "Raw Markdown saved: ") ||
		!strings.Contains(printed, "Standalone HTML saved: ") ||
		!strings.Contains(printed, "Browser preview: http://127.0.0.1:49152/preview/local?token=t") {
		t.Fatalf("local markdown/preview hints missing:\n%s", printed)
	}
	if strings.Contains(printed, "```mermaid sequenceDiagram") ||
		strings.Contains(printed, "终端 Mermaid 渲染器解析失败") {
		t.Fatalf("local REPL surface should render info-string sequence diagrams while keeping raw dump separate:\n%s", printed)
	}
	if preview.path == "" {
		t.Fatalf("preview was not registered")
	}
	data, err := os.ReadFile(preview.path)
	if err != nil {
		t.Fatalf("read local dump %q: %v", preview.path, err)
	}
	body := string(data)
	if !strings.Contains(body, "# Question\n\n换成时序图\n") ||
		!strings.Contains(body, "# Answer\n\n```mermaid\nsequenceDiagram\n") ||
		!strings.Contains(body, "Explorer->>Runtime: propose") {
		t.Fatalf("local dump body missing request/answer:\n%s", body)
	}
	recent := store.Recent()
	if len(recent) == 0 {
		t.Fatalf("expected local turn in memory")
	}
	if strings.Contains(recent[len(recent)-1].Response, "Raw Markdown saved") ||
		strings.Contains(recent[len(recent)-1].Response, "Standalone HTML saved") ||
		strings.Contains(recent[len(recent)-1].Response, "Browser preview") {
		t.Fatalf("markdown hints leaked into memory: %q", recent[len(recent)-1].Response)
	}
	if !strings.Contains(recent[len(recent)-1].Response, "```mermaid sequenceDiagram") {
		t.Fatalf("memory must retain raw local reply, got: %q", recent[len(recent)-1].Response)
	}
}

// TestTurnPolicyDispatch_LocalTableTransform exercises the second
// transform shape (table) so the local route is not over-fitted to
// mermaid. Same invariants: no runner.Run, local responder fires.
func TestTurnPolicyDispatch_LocalTableTransform(t *testing.T) {
	store := newPolicyStore(t)
	seedPriorAnswer(t, store, "概览", "答案中提到了 A、B、C 三个组件,各自承担不同职责。")

	classifier := &stubTurnPolicyClassifier{
		policy: TurnPolicy{
			Route:                 RouteLocal,
			Operation:             "transform",
			Source:                "last_answer",
			Confidence:            0.9,
			PresentationDirective: "markdown 表格",
		},
	}
	responder := &stubLocalResponder{localReply: "| col | val |\n|---|---|\n| A | 1 |"}
	r, runner, _ := newTurnPolicyREPL(t, store, classifier, responder, "把上面的结论换成表格\n/exit\n")
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Errorf("runner.Run must NOT be called for table transform; got %d", len(runner.requests))
	}
	if len(responder.localCalls) != 1 {
		t.Fatalf("local responder calls: got %d, want 1", len(responder.localCalls))
	}
	if responder.localCalls[0].directive != "markdown 表格" {
		t.Errorf("directive lost; got %q", responder.localCalls[0].directive)
	}
}

// TestTurnPolicyDispatch_RepoRouteEntersPipeline pins the legacy
// pipeline path under the new dispatcher. A "重新读" intent without a
// presentation request must reach runner.Run unchanged.
func TestTurnPolicyDispatch_RepoRouteEntersPipeline(t *testing.T) {
	store := newPolicyStore(t)
	seedPriorAnswer(t, store, "概览", "答案省略")

	classifier := &stubTurnPolicyClassifier{
		policy: TurnPolicy{
			Route:           RouteRepo,
			NeedsRepoAccess: true,
			Operation:       "investigate",
			Source:          "repo",
			Confidence:      0.85,
		},
	}
	responder := &stubLocalResponder{localReply: "should-not-appear"}
	r, runner, _ := newTurnPolicyREPL(t, store, classifier, responder, "重新读一下仓库确认这个流程\n/exit\n")
	adapter := &scriptedChatAdapter{}
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 1 {
		t.Fatalf("runner.Run: got %d, want 1", len(runner.requests))
	}
	if len(responder.localCalls) != 0 {
		t.Errorf("local responder must not fire on route=repo; calls=%d", len(responder.localCalls))
	}
	if len(adapter.calls) != 0 {
		t.Fatalf("repo/source analysis route must not call operation planner/evaluator; calls=%d", len(adapter.calls))
	}
	if strings.Contains(runner.requests[0], "Presentation directive") {
		t.Errorf("repo route without directive must not carry a presentation directive prefix; got %q", runner.requests[0])
	}
	if len(runner.seenDirectives) != 1 || runner.seenDirectives[0] != "" {
		t.Errorf("repo route without directive must clear typed presentation metadata; seen=%q setCalls=%q",
			runner.seenDirectives, runner.directiveSetCalls)
	}
}

func TestTurnPolicyDispatch_WriteRouteEntersAutoPilotApplyMode(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{
		policy: TurnPolicy{
			Route:           RouteWrite,
			NeedsRepoAccess: true,
			Operation:       "code_change",
			WriteIntent:     WriteIntentExplicitChange,
			Source:          "repo",
			Confidence:      0.9,
			Reason:          "repository change request",
		},
	}
	responder := &stubLocalResponder{localReply: "should-not-appear"}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, responder, "给 CLI 新增 --json 输出，并更新测试\n/exit\n")
	r.writeEnabled = true
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 1 {
		t.Fatalf("runner.Run: got %d, want 1", len(runner.requests))
	}
	if len(classifier.calls) != 1 {
		t.Fatalf("auto route=write should classify exactly once, got %d calls", len(classifier.calls))
	}
	if len(runner.modeSetCalls) != 1 {
		t.Fatalf("auto route=write should set runner mode once in the active dispatch, got %d calls: %+v",
			len(runner.modeSetCalls), runner.modeSetCalls)
	}
	if got := runner.seenModes[0]; got != types.ModeApply {
		t.Fatalf("auto route=write should dispatch Auto Pilot ModeApply, got %q", got)
	}
	if r.userMode != UserModeAuto || r.currentMode != types.ModeRead {
		t.Fatalf("auto write route must be one-shot; userMode=%q currentMode=%q", r.userMode, r.currentMode)
	}
	if strings.Contains(out.String(), "write_enabled") {
		t.Fatalf("write-enabled plan route should not print gate rejection:\n%s", out.String())
	}
	if len(responder.localCalls) != 0 {
		t.Fatalf("local responder must not fire on route=write; calls=%d", len(responder.localCalls))
	}
}

func TestEnterOneShotUserMode_RestoresMode(t *testing.T) {
	r := New(Config{
		Runner:   &requestCapturingRunner{},
		Store:    newPolicyStore(t),
		Render:   renderNothing,
		RepoRoot: ".",
		Branch:   "main",
		In:       strings.NewReader("/exit\n"),
		Out:      &bytes.Buffer{},
		Prompt:   ">",
		Language: "en",
	})
	r.writeEnabled = true
	r.userMode = UserModeAuto
	r.currentMode = types.ModeRead

	restore, ok := r.enterOneShotUserMode(UserModeWrite)
	if !ok {
		t.Fatal("write-enabled one-shot mode should enter")
	}
	if r.userMode != UserModeWrite || r.currentMode != types.ModeApply {
		t.Fatalf("one-shot write mode did not switch to apply: userMode=%q currentMode=%q", r.userMode, r.currentMode)
	}
	restore()
	if r.userMode != UserModeAuto || r.currentMode != types.ModeRead {
		t.Fatalf("one-shot mode did not restore previous state: userMode=%q currentMode=%q", r.userMode, r.currentMode)
	}
}

func TestTurnPolicyDispatch_WriteRouteRespectsWriteEnabledGate(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{
		policy: TurnPolicy{
			Route:           RouteWrite,
			NeedsRepoAccess: true,
			Operation:       "code_change",
			WriteIntent:     WriteIntentExplicitChange,
			Source:          "repo",
			Confidence:      0.9,
		},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "修复这个项目里的边界问题\n/exit\n")
	r.writeEnabled = false
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}
	if len(runner.requests) != 0 {
		t.Fatalf("write_disabled route must not call runner: %v", runner.requests)
	}
	if !strings.Contains(out.String(), "write_enabled") {
		t.Fatalf("write route rejection must name write_enabled:\n%s", out.String())
	}
}

func TestTurnPolicyDispatch_ExternalObservationAnalysisDoesNotCallOperationEvaluator(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		source string
	}{
		{
			name:   "trace",
			input:  "只分析这个 trace，不要看代码，找一下 jank 原因",
			source: "artifact",
		},
		{
			name:   "log",
			input:  "只看这段客户日志，不要读取源码，分析系统短板",
			source: "artifact",
		},
		{
			name:   "mcp",
			input:  "根据 MCP 返回的外部观测解释现象，不要看代码",
			source: "external_tool",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newPolicyStore(t)
			classifier := &stubTurnPolicyClassifier{
				policy: TurnPolicy{
					Route:           RouteRepo,
					NeedsRepoAccess: true,
					Operation:       "investigate",
					OperationKind:   "investigate",
					Source:          tc.source,
					RiskLevel:       "low",
					Confidence:      0.9,
					Reason:          "external observation diagnosis belongs to the analysis pipeline",
				},
			}
			adapter := &scriptedChatAdapter{}
			responder := &stubLocalResponder{localReply: "should-not-appear"}
			r, runner, _ := newTurnPolicyREPL(t, store, classifier, responder, tc.input+"\n/exit\n")
			r.operationEnabled = true
			r.operationPlanner = NewCommandOperationPlanner(adapter)
			if err := r.Loop(); err != nil {
				t.Fatalf("Loop: %v", err)
			}
			if len(runner.requests) != 1 {
				t.Fatalf("runner.Run: got %d, want 1", len(runner.requests))
			}
			if !strings.Contains(runner.requests[0], tc.input) {
				t.Fatalf("pipeline request lost original external-observation intent: %q", runner.requests[0])
			}
			if len(runner.seenRouteHints) != 1 || !runner.seenRouteHints[0].ExternalObservationFirst() {
				t.Fatalf("external observation analysis should pass typed route hint to pipeline; seen=%+v setCalls=%+v",
					runner.seenRouteHints, runner.routeHintSetCalls)
			}
			if runner.seenRouteHints[0].Source != tc.source {
				t.Fatalf("route hint source: got %q, want %q", runner.seenRouteHints[0].Source, tc.source)
			}
			if len(responder.localCalls) != 0 {
				t.Fatalf("external observation analysis must not use local responder; calls=%d", len(responder.localCalls))
			}
			if len(adapter.calls) != 0 {
				t.Fatalf("external observation analysis must not call operation planner/evaluator; calls=%d", len(adapter.calls))
			}
		})
	}
}

func TestTurnPolicyDispatch_DataRouteBypassesSourcePipeline(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount\nA,10\nA,7\n"), 0600); err != nil {
		t.Fatal(err)
	}
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{
		policy: TurnPolicy{
			Route:           RouteData,
			NeedsDataAccess: true,
			Operation:       "data_aggregation",
			DataTaskKind:    "data_aggregation",
			Source:          "data",
			RiskLevel:       "low",
			Confidence:      0.9,
		},
	}
	planner := &stubDataTaskPlanner{plan: dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"orders.csv"},
		OutputContract: dataquery.OutputContract{
			Format:             dataquery.OutputCSVLine,
			ExplanationAllowed: false,
		},
		Script: `rows = csv_rows("orders.csv")
total = sum(int(r["amount"]) for r in rows)
emit({"answer": "A," + str(total), "output_contract": {"format": "csv_line", "explanation_allowed": False}})`,
	}}
	responder := &stubLocalResponder{localReply: "should-not-appear"}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, responder, "汇总 orders.csv，只输出一行 CSV\n/exit\n")
	r.repoRoot = root
	r.runtimeAnchor = t.TempDir()
	r.dataTaskPlanner = planner
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}
	if len(runner.requests) != 0 {
		t.Fatalf("data route must not enter source pipeline; requests=%v", runner.requests)
	}
	if planner.calls != 1 {
		t.Fatalf("planner calls=%d, want 1", planner.calls)
	}
	if !strings.Contains(out.String(), "A,17") {
		t.Fatalf("strict data answer missing from REPL output:\n%s", out.String())
	}
	if strings.Contains(out.String(), "审计摘要") || strings.Contains(out.String(), "Audit summary") {
		t.Fatalf("strict output contract should not add explanation:\n%s", out.String())
	}
}

func TestTurnPolicyDispatch_DataRouteRepairsFailedScript(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount\nA,10\nA,7\n"), 0600); err != nil {
		t.Fatal(err)
	}
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{
		policy: TurnPolicy{
			Route:           RouteData,
			NeedsDataAccess: true,
			Operation:       "data_aggregation",
			DataTaskKind:    "data_aggregation",
			Source:          "data",
			RiskLevel:       "low",
			Confidence:      0.9,
		},
	}
	planner := &stubDataTaskPlanner{
		plan: dataquery.TaskPlan{
			Status:     "ready",
			InputPaths: []string{"orders.csv"},
			OutputContract: dataquery.OutputContract{
				Format:             dataquery.OutputCSVLine,
				ExplanationAllowed: false,
			},
			Script: `raise ValueError("simulated script bug")
emit({"answer":"unreachable"})`,
		},
		repairPlan: dataquery.TaskPlan{
			Status:     "ready",
			InputPaths: []string{"orders.csv"},
			OutputContract: dataquery.OutputContract{
				Format:             dataquery.OutputCSVLine,
				ExplanationAllowed: false,
			},
			Script: `rows = csv_rows("orders.csv")
total = sum(int(r["amount"]) for r in rows)
emit({"answer": "A," + str(total), "output_contract": {"format": "csv_line", "explanation_allowed": False}})`,
		},
	}
	responder := &stubLocalResponder{localReply: "should-not-appear"}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, responder, "汇总 orders.csv，只输出一行 CSV\n/exit\n")
	r.repoRoot = root
	r.runtimeAnchor = t.TempDir()
	r.dataTaskPlanner = planner
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}
	if len(runner.requests) != 0 {
		t.Fatalf("data route must not enter source pipeline; requests=%v", runner.requests)
	}
	if planner.calls != 1 || planner.repairCalls != 1 {
		t.Fatalf("planner calls=%d repairCalls=%d, want 1/1", planner.calls, planner.repairCalls)
	}
	if len(planner.repairErrors) != 1 || !strings.Contains(planner.repairErrors[0], "simulated script bug") {
		t.Fatalf("repairErrors=%v, want execution error", planner.repairErrors)
	}
	if !strings.Contains(out.String(), "A,17") {
		t.Fatalf("repaired data answer missing from REPL output:\n%s", out.String())
	}
}

func TestRunDataTaskCLIRepairsFailedScript(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount\nA,10\nA,7\n"), 0600); err != nil {
		t.Fatal(err)
	}
	planner := &stubDataTaskPlanner{
		plan: dataquery.TaskPlan{
			Status:     "ready",
			InputPaths: []string{"orders.csv"},
			OutputContract: dataquery.OutputContract{
				Format:             dataquery.OutputPlainSingleLine,
				ExplanationAllowed: false,
			},
			Script: `raise ValueError("simulated cli data bug")
emit({"answer":"unreachable"})`,
		},
		repairPlan: dataquery.TaskPlan{
			Status:     "ready",
			InputPaths: []string{"orders.csv"},
			OutputContract: dataquery.OutputContract{
				Format:             dataquery.OutputPlainSingleLine,
				ExplanationAllowed: false,
			},
			Script: `rows = csv_rows("orders.csv")
total = sum(int(r["amount"]) for r in rows)
emit({"answer": str(total), "output_contract": {"format": "plain_single_line", "explanation_allowed": False}})`,
		},
		eval: dataquery.Evaluation{Status: dataquery.EvalComplete, Reason: "repaired result satisfies strict scalar output", Confidence: "high"},
	}
	var progress bytes.Buffer
	runtimeAnchor := t.TempDir()
	answer, err := RunDataTaskCLI(context.Background(), "汇总 orders.csv，只输出总额", TurnPolicy{Route: RouteData, NeedsDataAccess: true, Source: "data"}, DataTaskCLIConfig{
		Planner:         planner,
		RepoRoot:        root,
		RuntimeAnchor:   runtimeAnchor,
		Language:        "zh",
		MaxRepairRounds: 2,
		MaxDataRounds:   4,
		Progress:        &progress,
	})
	if err != nil {
		t.Fatalf("RunDataTaskCLI: %v", err)
	}
	if planner.calls != 1 || planner.repairCalls != 1 || planner.evalCalls != 1 {
		t.Fatalf("calls plan/repair/eval=%d/%d/%d, want 1/1/1", planner.calls, planner.repairCalls, planner.evalCalls)
	}
	if len(planner.evalRecordLens) != 1 || planner.evalRecordLens[0] != 2 {
		t.Fatalf("evalRecordLens=%v, want evaluator fed from runtime records after failed+repaired batches", planner.evalRecordLens)
	}
	if len(planner.evalLastAnswers) != 1 || planner.evalLastAnswers[0] != "17" {
		t.Fatalf("evalLastAnswers=%v, want evaluator to see repaired runtime result", planner.evalLastAnswers)
	}
	if strings.TrimSpace(answer) != "17" {
		t.Fatalf("answer=%q, want strict scalar 17", answer)
	}
	progressText := progress.String()
	for _, want := range []string{"◇ 数据请求 · 已接收", "问题：汇总 orders.csv，只输出总额", "◇ 数据计划", "◇ 数据工作流 · 执行第 1 批", "◇ 数据工作流 · 修复第 1 次", "◇ 数据工作流 · 结果第 2 批", "◇ 数据工作流 · 评估第 2 批"} {
		if !strings.Contains(progressText, want) {
			t.Fatalf("progress missing %q:\n%s", want, progressText)
		}
	}
	terminalFiles, err := filepath.Glob(filepath.Join(runtimeAnchor, "data-audit", "*-terminal.json"))
	if err != nil {
		t.Fatalf("glob terminal audit: %v", err)
	}
	if len(terminalFiles) != 1 {
		t.Fatalf("terminal audit files=%v, want one terminal audit", terminalFiles)
	}
	rawTerminal, err := os.ReadFile(terminalFiles[0])
	if err != nil {
		t.Fatalf("read terminal audit: %v", err)
	}
	terminalText := string(rawTerminal)
	for _, want := range []string{`"data_rounds": 2`, `"repair_rounds": 1`, `"kind": "execute"`, `"kind": "repair"`, `"kind": "result"`, `"kind": "evaluate"`, `"plan_transitions"`, `"source": "repair"`, `"current_plan"`} {
		if !strings.Contains(terminalText, want) {
			t.Fatalf("terminal audit missing live process event %q:\n%s", want, terminalText)
		}
	}
}

func TestRunDataTaskCLIResumesFromCheckpointWithoutInitialPlan(t *testing.T) {
	root := t.TempDir()
	contract := dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false}
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			Status:         "ready",
			OutputContract: contract,
			Goal:           "resume data workflow",
		},
		Result: &dataquery.Result{
			Answer:         "17",
			OutputContract: contract,
		},
	}}
	checkpoint := writeDataTaskWorkflowCheckpointFile(t.TempDir(), root, records, dataquery.TaskPlan{
		Status:         "complete",
		OutputContract: contract,
		Goal:           "resume data workflow",
	}, dataquery.TaskPlan{}, 1, 0, "test checkpoint", "test")
	if checkpoint == "" {
		t.Fatal("checkpoint path empty")
	}
	planner := &stubDataTaskPlanner{
		continuePlan: dataquery.TaskPlan{},
	}
	var progress bytes.Buffer
	answer, err := RunDataTaskCLI(context.Background(), "继续数据任务", TurnPolicy{Route: RouteData, NeedsDataAccess: true, Source: "data"}, DataTaskCLIConfig{
		Planner:       planner,
		RepoRoot:      root,
		RuntimeAnchor: t.TempDir(),
		Language:      "zh",
		MaxDataRounds: 2,
		Progress:      &progress,
		ResumePath:    checkpoint,
	})
	if err != nil {
		t.Fatalf("RunDataTaskCLI resume: %v", err)
	}
	if strings.TrimSpace(answer) != "17" {
		t.Fatalf("answer=%q, want 17", answer)
	}
	if planner.calls != 0 {
		t.Fatalf("initial planner calls=%d, want 0 for resume", planner.calls)
	}
	if planner.continueCalls != 1 {
		t.Fatalf("continue planner calls=%d, want one resumed continuation attempt before checkpoint fallback", planner.continueCalls)
	}
	if !strings.Contains(progress.String(), "checkpoint") {
		t.Fatalf("progress missing checkpoint resume detail:\n%s", progress.String())
	}
}

func TestRunDataTaskCLIPatchesStructuralResultBeforeScriptRepair(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount\nA,10\n"), 0600); err != nil {
		t.Fatal(err)
	}
	planner := &stubDataTaskPlanner{
		plan: dataquery.TaskPlan{
			Status:     "ready",
			InputPaths: []string{"orders.csv"},
			Actions: []dataquery.DataAction{{
				ID:         "transform",
				Kind:       dataquery.DataActionCustomTransform,
				InputPaths: []string{"orders.csv"},
				Script: `rows = csv_rows("orders.csv")
emit({
  "answer": "10",
  "output_contract": {"format": "plain_single_line", "explanation_allowed": False},
  "contributions": [{"item_id": "row-1", "source": "orders.csv", "source_locator": "row 2", "group_key": "A", "metric": "amount", "value": "10", "operation": "totalize"}],
  "reconcile": {"status": "pass", "actual_answer": "10", "groups": [{"group_key": "A", "metric": "amount", "expected": "10", "actual": "10"}]}
})`,
			}},
			CoverageContract: dataquery.CoverageContract{
				RequiredMaterials:          []dataquery.CoverageMaterial{{Path: "orders.csv", Required: true}},
				ContributionLedgerRequired: true,
				ReconcileRequired:          true,
			},
			OutputContract: dataquery.OutputContract{
				Format:             dataquery.OutputPlainSingleLine,
				ExplanationAllowed: false,
			},
		},
		patchPlan: dataquery.DataResultPatchPlan{
			Status: "patch",
			Patches: []dataquery.DataResultPatch{{
				Target: "result",
				Op:     "replace",
				Path:   "/contributions/0/operation",
				Value:  json.RawMessage(`"add"`),
				Reason: "canonical structural aggregation operation",
			}},
			Reason:     "safe structural patch",
			Confidence: "high",
		},
		eval: dataquery.Evaluation{Status: dataquery.EvalComplete, Reason: "patched result satisfies strict scalar output", Confidence: "high"},
	}
	var progress bytes.Buffer
	answer, err := RunDataTaskCLI(context.Background(), "汇总 orders.csv，只输出总额", TurnPolicy{Route: RouteData, NeedsDataAccess: true, Source: "data"}, DataTaskCLIConfig{
		Planner:         planner,
		RepoRoot:        root,
		RuntimeAnchor:   t.TempDir(),
		Language:        "zh",
		MaxRepairRounds: 2,
		MaxDataRounds:   4,
		Progress:        &progress,
	})
	if err != nil {
		t.Fatalf("RunDataTaskCLI: %v", err)
	}
	if strings.TrimSpace(answer) != "10" {
		t.Fatalf("answer=%q, want strict scalar 10", answer)
	}
	if planner.patchCalls != 1 {
		t.Fatalf("patchCalls=%d, want 1", planner.patchCalls)
	}
	if planner.repairCalls != 0 {
		t.Fatalf("repairCalls=%d, want 0; structural result patch should avoid script rewrite", planner.repairCalls)
	}
	if !strings.Contains(progress.String(), "结构修复第 1 批") {
		t.Fatalf("progress missing structural patch line:\n%s", progress.String())
	}
}

func TestTurnPolicyDispatch_DataRouteUsesConfiguredRepairBudget(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount\nA,10\nA,7\n"), 0600); err != nil {
		t.Fatal(err)
	}
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{
		policy: TurnPolicy{
			Route:           RouteData,
			NeedsDataAccess: true,
			Operation:       "data_aggregation",
			DataTaskKind:    "data_aggregation",
			Source:          "data",
			RiskLevel:       "low",
			Confidence:      0.9,
		},
	}
	failingPlan := func(msg string) dataquery.TaskPlan {
		return dataquery.TaskPlan{
			Status:     "ready",
			InputPaths: []string{"orders.csv"},
			OutputContract: dataquery.OutputContract{
				Format:             dataquery.OutputPlainSingleLine,
				ExplanationAllowed: false,
			},
			Script: fmt.Sprintf(`rows = csv_rows("orders.csv")
raise KeyError(%q)
emit_result("0", output_contract={"format": "plain_single_line", "explanation_allowed": False})`, msg),
		}
	}
	planner := &stubDataTaskPlanner{
		plan: failingPlan("first"),
		repairPlans: []dataquery.TaskPlan{
			failingPlan("second"),
			failingPlan("third"),
			{
				Status:     "ready",
				InputPaths: []string{"orders.csv"},
				OutputContract: dataquery.OutputContract{
					Format:             dataquery.OutputPlainSingleLine,
					ExplanationAllowed: false,
				},
				Script: `rows = csv_rows("orders.csv")
total = sum(int(r["amount"]) for r in rows)
emit({"answer": str(total), "output_contract": {"format": "plain_single_line", "explanation_allowed": False}})`,
			},
		},
	}
	responder := &stubLocalResponder{localReply: "should-not-appear"}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, responder, "汇总 orders.csv，只输出总额\n/exit\n")
	r.repoRoot = root
	r.runtimeAnchor = t.TempDir()
	r.dataTaskPlanner = planner
	r.dataTaskMaxRepairRounds = 3
	r.dataTaskMaxDataRounds = 6
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}
	if len(runner.requests) != 0 {
		t.Fatalf("data route must not enter source pipeline; requests=%v", runner.requests)
	}
	if planner.repairCalls != 3 {
		t.Fatalf("repairCalls=%d, want 3", planner.repairCalls)
	}
	if !strings.Contains(out.String(), "17") {
		t.Fatalf("configured repair budget did not reach repaired answer:\n%s", out.String())
	}
}

func TestDataTaskPlanStagingGuardRequiresBoundedBatch(t *testing.T) {
	largeScript := strings.Repeat("x = 1\n", dataTaskOneShotScriptLineSoftLimit+5) + `emit({"answer":"ok"})` + "\n"
	plan := dataquery.TaskPlan{
		Status: "ready",
		Script: largeScript,
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "a.csv"}, {Path: "b.csv"}, {Path: "c.csv"}, {Path: "d.csv"},
				{Path: "e.csv"}, {Path: "f.csv"}, {Path: "g.csv"}, {Path: "h.csv"},
			},
			DecisionRecordsRequired:    true,
			RuleCoverageRequired:       true,
			ContributionLedgerRequired: true,
		},
	}
	errText := dataTaskPlanStagingGuardError(plan)
	if !strings.Contains(errText, "complex data task") {
		t.Fatalf("guard err=%q, want complex action repair", errText)
	}
	plan.ContinueAfter = true
	if got := dataTaskPlanStagingGuardError(plan); !strings.Contains(got, "complex data task") {
		t.Fatalf("continue_after plan should still require bounded atomic work, got %q", got)
	}
}

func TestDataTaskPlanStagingGuardRejectsComplexOneShotBeforeExecution(t *testing.T) {
	script := strings.Repeat("x = 1\n", 40) + `emit({"answer":"ok"})` + "\n"
	plan := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"a.csv", "b.csv", "c.csv", "d.csv"},
		Script:     script,
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "a.csv"}, {Path: "b.csv"}, {Path: "c.csv"}, {Path: "d.csv"},
			},
			DecisionRecordsRequired:    true,
			ContributionLedgerRequired: true,
		},
	}
	errText := dataTaskPlanStagingGuardError(plan)
	if !strings.Contains(errText, "atomic actions[] batch") {
		t.Fatalf("guard err=%q, want atomic action repair", errText)
	}
}

func TestDataTaskPlanStagingGuardAllowsSimpleOneShot(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"single.csv"},
		Script:     strings.Repeat("x = 1\n", 120) + `emit({"answer":"ok"})` + "\n",
	}
	if got := dataTaskPlanStagingGuardError(plan); got != "" {
		t.Fatalf("simple plan should pass staging guard, got %q", got)
	}
}

func TestDataTaskPlanStagingGuardRejectsScriptWithoutEmitter(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"single.csv"},
		Script:     "rows = csv_rows('single.csv')\nprint(len(rows))\n",
	}
	errText := dataTaskPlanStagingGuardError(plan)
	if !strings.Contains(errText, "no result emitter") {
		t.Fatalf("guard err=%q, want emitter rejection", errText)
	}
}

func TestDataTaskPlanStagingGuardRejectsCustomTransformWithoutEmitter(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:     "transform",
			Kind:   dataquery.DataActionCustomTransform,
			Script: "rows = csv_rows('single.csv')\nprint(len(rows))\n",
		}},
	}
	errText := dataTaskPlanStagingGuardError(plan)
	if !strings.Contains(errText, "script has no result emitter") {
		t.Fatalf("guard err=%q, want emitter rejection", errText)
	}
}

func TestDataTaskPlanStagingGuardRejectsTopLevelScriptWithActions(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status: "ready",
		Script: "x = 1\n",
		Actions: []dataquery.DataAction{{
			ID:   "transform",
			Kind: dataquery.DataActionCustomTransform,
		}},
	}
	errText := dataTaskPlanStagingGuardError(plan)
	if !strings.Contains(errText, "must not carry a top-level script") {
		t.Fatalf("guard err=%q, want top-level script rejection", errText)
	}
}

func TestDataTaskRepeatedNodeFailureDetectsTypedAction(t *testing.T) {
	errText := `execute data task: data action failed action_id="transform_1" action_kind="custom_transform": KeyError: missing`
	key, count, repeated := dataTaskRepeatedNodeFailure(nil, errText, 2)
	if key != "transform_1|custom_transform" || count != 1 || repeated {
		t.Fatalf("first failure key=%q count=%d repeated=%v", key, count, repeated)
	}
	records := []dataTaskWorkflowRecord{{Err: errText}}
	key, count, repeated = dataTaskRepeatedNodeFailure(records, errText, 2)
	if key != "transform_1|custom_transform" || count != 2 || !repeated {
		t.Fatalf("second failure key=%q count=%d repeated=%v", key, count, repeated)
	}
}

func TestDataTaskRequiredLedgerCompletionPlanDoesNotInventEntityNode(t *testing.T) {
	current := dataquery.TaskPlan{
		OutputContract: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{
				{Path: "entities.csv", Required: true},
			},
			EntityResolutionRequired:   true,
			ContributionLedgerRequired: true,
		},
	}
	result := dataquery.Result{
		Answer:         "42",
		OutputContract: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false},
		ConsumedPaths:  []string{"entities.csv"},
		Contributions: []dataquery.ContributionRecord{{
			ItemID: "row-1", Source: "entities.csv", SourceLocator: "line:2", GroupKey: "all", Metric: "amount", Value: "42", Operation: "add", Reason: "seed",
		}},
	}
	guard := dataTaskWorkflowCompletionGateGuardResult(nil, current, result)
	plan, ok := dataTaskRequiredLedgerCompletionPlan(nil, current, result, guard)
	if ok {
		t.Fatalf("plan=%+v, deterministic completion must not invent entity_resolution semantics without field contracts", plan)
	}
}

func TestTurnPolicyDispatch_DataRouteContinuesAfterIntermediateBatch(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount\nA,10\nA,7\n"), 0600); err != nil {
		t.Fatal(err)
	}
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{
		policy: TurnPolicy{
			Route:           RouteData,
			NeedsDataAccess: true,
			Operation:       "data_aggregation",
			DataTaskKind:    "data_aggregation",
			Source:          "data",
			RiskLevel:       "low",
			Confidence:      0.9,
		},
	}
	planner := &stubDataTaskPlanner{
		plan: dataquery.TaskPlan{
			Status:        "ready",
			InputPaths:    []string{"orders.csv"},
			ContinueAfter: true,
			OutputContract: dataquery.OutputContract{
				Format:             dataquery.OutputMarkdown,
				ExplanationAllowed: true,
			},
			Script: `rows = csv_rows("orders.csv")
emit({"answer": "intermediate rows loaded", "output_contract": {"format": "markdown", "explanation_allowed": True}})`,
		},
		evals: []dataquery.Evaluation{
			{Status: dataquery.EvalContinueData, Reason: "need final aggregation", Confidence: "high"},
			{Status: dataquery.EvalComplete, Reason: "final aggregation satisfies the goal", Confidence: "high"},
		},
		continuePlan: dataquery.TaskPlan{
			Status:     "ready",
			InputPaths: []string{"orders.csv"},
			OutputContract: dataquery.OutputContract{
				Format:             dataquery.OutputCSVLine,
				ExplanationAllowed: false,
			},
			Script: `rows = csv_rows("orders.csv")
total = sum(int(r["amount"]) for r in rows)
emit({"answer": "A," + str(total), "output_contract": {"format": "csv_line", "explanation_allowed": False}})`,
		},
	}
	responder := &stubLocalResponder{localReply: "should-not-appear"}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, responder, "汇总 orders.csv，只输出一行 CSV\n/exit\n")
	r.repoRoot = root
	r.runtimeAnchor = t.TempDir()
	r.dataTaskPlanner = planner
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}
	if len(runner.requests) != 0 {
		t.Fatalf("data route must not enter source pipeline; requests=%v", runner.requests)
	}
	if planner.calls != 1 || planner.evalCalls != 2 || planner.continueCalls != 1 {
		t.Fatalf("calls plan/eval/continue=%d/%d/%d, want 1/2/1", planner.calls, planner.evalCalls, planner.continueCalls)
	}
	if !strings.Contains(out.String(), "A,17") {
		t.Fatalf("continued data answer missing from REPL output:\n%s", out.String())
	}
}

func TestTurnPolicyDispatch_DataRouteEvaluatesSuccessfulBatchBeforeFinalAnswer(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "orders.csv"), []byte("vendor,amount\nA,10\nA,7\n"), 0600); err != nil {
		t.Fatal(err)
	}
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{
		policy: TurnPolicy{
			Route:           RouteData,
			NeedsDataAccess: true,
			Operation:       "data_aggregation",
			DataTaskKind:    "data_aggregation",
			Source:          "data",
			RiskLevel:       "low",
			Confidence:      0.9,
		},
	}
	planner := &stubDataTaskPlanner{
		plan: dataquery.TaskPlan{
			Status:     "ready",
			InputPaths: []string{"orders.csv"},
			OutputContract: dataquery.OutputContract{
				Format:             dataquery.OutputPlainSingleLine,
				ExplanationAllowed: false,
			},
			Script: `rows = csv_rows("orders.csv")
emit({"answer": "0", "output_contract": {"format": "plain_single_line", "explanation_allowed": False}})`,
		},
		evals: []dataquery.Evaluation{
			{Status: dataquery.EvalContinueData, Reason: "computed answer does not satisfy the goal", Confidence: "high"},
			{Status: dataquery.EvalComplete, Reason: "final aggregation satisfies the goal", Confidence: "high"},
		},
		continuePlan: dataquery.TaskPlan{
			Status:     "ready",
			InputPaths: []string{"orders.csv"},
			OutputContract: dataquery.OutputContract{
				Format:             dataquery.OutputPlainSingleLine,
				ExplanationAllowed: false,
			},
			Script: `rows = csv_rows("orders.csv")
total = sum(int(r["amount"]) for r in rows)
emit({"answer": str(total), "output_contract": {"format": "plain_single_line", "explanation_allowed": False}})`,
		},
	}
	responder := &stubLocalResponder{localReply: "should-not-appear"}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, responder, "汇总 orders.csv，只输出总额\n/exit\n")
	r.repoRoot = root
	r.runtimeAnchor = t.TempDir()
	r.dataTaskPlanner = planner
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}
	if len(runner.requests) != 0 {
		t.Fatalf("data route must not enter source pipeline; requests=%v", runner.requests)
	}
	if planner.calls != 1 || planner.evalCalls != 2 || planner.continueCalls != 1 {
		t.Fatalf("calls plan/eval/continue=%d/%d/%d, want 1/2/1", planner.calls, planner.evalCalls, planner.continueCalls)
	}
	if !strings.Contains(out.String(), "17") {
		t.Fatalf("evaluated data answer missing from REPL output:\n%s", out.String())
	}
}

func TestTurnPolicyDispatch_RepoRouteCarriesCurrentPresentationDirective(t *testing.T) {
	store := newPolicyStore(t)

	classifier := &stubTurnPolicyClassifier{
		policy: TurnPolicy{
			Route:                 RouteRepo,
			NeedsRepoAccess:       true,
			Operation:             "investigate",
			Source:                "repo",
			Confidence:            0.9,
			PresentationDirective: "logic flow diagram showing anti-hallucination mechanisms",
		},
	}
	responder := &stubLocalResponder{localReply: "should-not-appear"}
	r, runner, _ := newTurnPolicyREPL(t, store, classifier, responder,
		"读取代码，对比 codrax 和 opencode，并输出各自的逻辑视图\n/exit\n")
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 1 {
		t.Fatalf("runner.Run: got %d, want 1", len(runner.requests))
	}
	req := runner.requests[0]
	if strings.Contains(req, "Presentation directive") ||
		strings.Contains(req, "logic flow diagram showing anti-hallucination mechanisms") {
		t.Fatalf("repo request must not be polluted by typed presentation metadata; got %q", req)
	}
	if len(runner.seenDirectives) != 1 ||
		runner.seenDirectives[0] != "logic flow diagram showing anti-hallucination mechanisms" {
		t.Fatalf("repo directive not delivered through typed setter: seen=%q setCalls=%q",
			runner.seenDirectives, runner.directiveSetCalls)
	}
	if !strings.Contains(req, "输出各自的逻辑视图") {
		t.Fatalf("repo request must preserve original user wording; got %q", req)
	}
	if len(responder.localCalls) != 0 {
		t.Errorf("local responder must not fire on repo; calls=%d", len(responder.localCalls))
	}
}

func TestTurnPolicyDispatch_OperationRouteUnavailableDoesNotRunPipeline(t *testing.T) {
	store := newPolicyStore(t)

	classifier := &stubTurnPolicyClassifier{
		policy: TurnPolicy{
			Route:                RouteOperation,
			NeedsOperationAccess: true,
			NeedsRepoAccess:      true,
			Operation:            "presentation_generation",
			OperationKind:        "presentation_generation",
			Source:               "mixed",
			RiskLevel:            "low",
			SideEffects:          []string{"local_file_write"},
			TargetSurface:        "slides",
			Confidence:           0.9,
			Reason:               "user requested slide generation",
		},
	}
	responder := &stubLocalResponder{localReply: "should-not-appear"}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, responder, "基于当前代码生成一份 PPT\n/exit\n")
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("operation route is disabled and must not enter source pipeline; runner requests=%v", runner.requests)
	}
	if len(responder.localCalls) != 0 {
		t.Fatalf("operation route must not use local responder; calls=%d", len(responder.localCalls))
	}
	printed := out.String()
	if !strings.Contains(printed, "operation pipeline is not enabled") {
		t.Fatalf("operation unavailable message missing:\n%s", printed)
	}
	recent := store.Recent()
	if len(recent) == 0 || !strings.Contains(recent[len(recent)-1].Response, "operation pipeline is not enabled") {
		t.Fatalf("operation refusal should be persisted for follow-ups; recent=%+v", recent)
	}
}

func TestTurnPolicyDispatch_OperationRoutePlansWithoutSourcePipeline(t *testing.T) {
	store := newPolicyStore(t)

	classifier := &stubTurnPolicyClassifier{
		policy: TurnPolicy{
			Route:                RouteOperation,
			NeedsOperationAccess: true,
			NeedsRepoAccess:      true,
			Operation:            "presentation_generation",
			OperationKind:        "presentation_generation",
			Source:               "mixed",
			RiskLevel:            "low",
			SideEffects:          []string{"local_file_write"},
			TargetSurface:        "slides",
			Confidence:           0.9,
			Reason:               "user requested slide generation",
		},
	}
	responder := &stubLocalResponder{localReply: "should-not-appear"}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, responder, "基于当前代码生成一份 PPT\n/exit\n")
	r.renderer = render.New(out, true)
	r.operationEnabled = true
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("operation route should not enter source pipeline in plan-only mode; runner requests=%v", runner.requests)
	}
	if len(responder.localCalls) != 0 {
		t.Fatalf("operation route must not use local responder; calls=%d", len(responder.localCalls))
	}
	recent := store.Recent()
	if len(recent) == 0 {
		t.Fatalf("operation plan should be persisted for follow-ups; recent=%+v", recent)
	}
	response := recent[len(recent)-1].Response
	for _, want := range []string{"operation planning path", "presentation_generation", "Not executed"} {
		if !strings.Contains(response, want) {
			t.Fatalf("operation plan memory response missing %q:\n%s\nprinted:\n%s", want, response, out.String())
		}
	}
	if !strings.Contains(out.String(), "presentation_generation") {
		t.Fatalf("operation plan should be persisted for follow-ups; recent=%+v", recent)
	}
	plain := stripANSIOnly(out.String())
	for _, want := range []string{"operation plan", "capability missing", "not executed"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("operation plan header missing %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "awaiting approval") {
		t.Fatalf("non-executable provider plan must not look like pending approval:\n%s", plain)
	}
}

func TestTurnPolicyDispatch_CommandOperationClarifiesWithoutSourcePipeline(t *testing.T) {
	store := newPolicyStore(t)

	classifier := &stubTurnPolicyClassifier{
		policy: TurnPolicy{
			Route:                RouteOperation,
			NeedsOperationAccess: true,
			Operation:            "computer_operation",
			OperationKind:        "computer_operation",
			Source:               "current_message",
			RiskLevel:            "medium",
			TargetSurface:        "desktop",
			Confidence:           0.9,
			Reason:               "user asked Codrax to operate the computer",
		},
	}
	responder := &stubLocalResponder{localReply: "should-not-appear"}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, responder, "帮我移动一个文件\n/exit\n")
	r.operationEnabled = true
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("command operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if r.pendingOperation != nil {
		t.Fatalf("clarification plan should not become pending approval: %+v", r.pendingOperation)
	}
	printed := out.String()
	for _, want := range []string{"command-line operation request", "will not guess"} {
		if !strings.Contains(printed, want) {
			t.Fatalf("command operation clarification missing %q:\n%s", want, printed)
		}
	}
}

func TestRejectPrefersPendingOperationOverWritePlan(t *testing.T) {
	store := newPolicyStore(t)
	r, _, out := newTurnPolicyREPL(t, store, nil, nil, "/reject no thanks\n/exit\n")
	r.pendingOperation = &operation.CommandOperationPlan{
		ID:           "op-test",
		Status:       operation.StatusReady,
		RiskLevel:    "medium",
		ApprovalMode: operation.ApprovalManual,
		WorkDir:      ".",
		Steps: []operation.CommandStep{{
			ID:           "step-1",
			Title:        "probe",
			Program:      "corp-tool",
			AutoApproval: operation.StepAutoManual,
			RiskLevel:    "medium",
		}},
	}
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if r.pendingOperation != nil {
		t.Fatalf("pending operation was not cleared: %+v", r.pendingOperation)
	}
	printed := out.String()
	if strings.Contains(printed, "no pending plan") {
		t.Fatalf("/reject should not fall through to write-plan reject while operation is pending:\n%s", printed)
	}
	if !strings.Contains(printed, "Rejected operation plan `op-test`: no thanks") {
		t.Fatalf("operation reject message missing:\n%s", printed)
	}
}

func TestCancelClearsPendingCommandClarification(t *testing.T) {
	store := newPolicyStore(t)
	r, _, out := newTurnPolicyREPL(t, store, nil, nil, "/cancel\n/exit\n")
	r.pendingCommandClarification = &pendingCommandClarification{
		OriginalLine: "move a file",
		Policy:       commandOperationPolicy("medium"),
		Plan: operation.CommandOperationPlan{
			ID:        "op-clarify",
			Status:    operation.StatusNeedsClarification,
			RiskLevel: "medium",
			CreatedAt: time.Now().UTC(),
			ClarifyingQuestions: []operation.ClarifyingQuestion{{
				ID:       "paths",
				Question: "Which paths?",
			}},
		},
	}
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if r.pendingCommandClarification != nil {
		t.Fatalf("pending clarification was not cleared: %+v", r.pendingCommandClarification)
	}
	if !strings.Contains(out.String(), "op-clarify") {
		t.Fatalf("cancel message missing plan id:\n%s", out.String())
	}
}

func TestCommandOperationPlanApproveExecutesWithoutSourcePipeline(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{
		policy: TurnPolicy{
			Route:                RouteOperation,
			NeedsOperationAccess: true,
			Operation:            "computer_operation",
			OperationKind:        "computer_operation",
			Source:               "current_message",
			RiskLevel:            "low",
			TargetSurface:        "desktop",
			Confidence:           0.9,
			Reason:               "user asked for command operation",
		},
	}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			commandOperationPlanResp(`{"status":"ready","risk_level":"low","requires_confirmation":false,"work_dir":".","steps":[{"id":"s1","title":"show go version","program":"go","args":["version"],"risk_level":"low","side_effects":[]}]}`),
		},
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, &stubLocalResponder{}, "查询 go 版本\n/approve\n/exit\n")
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	r.operationPolicy = operation.DefaultCommandPolicy()
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Fatalf("command operation should not enter source pipeline; runner requests=%v", runner.requests)
	}
	if r.pendingOperation != nil {
		t.Fatalf("pending operation should be cleared after approve: %+v", r.pendingOperation)
	}
	printed := out.String()
	if !strings.Contains(printed, "Operation plan") || !strings.Contains(printed, "completed") {
		t.Fatalf("operation execution result missing:\n%s", printed)
	}
	if !strings.Contains(printed, "go version") {
		t.Fatalf("command output missing:\n%s", printed)
	}
}

// TestTurnPolicyDispatch_HybridCarriesDirectiveIntoPipeline is the
// route=hybrid contract: pipeline runs AND the typed directive
// reaches the runner so the finalizer can render the requested shape,
// without polluting the user-authored objective string.
func TestTurnPolicyDispatch_HybridCarriesDirectiveIntoPipeline(t *testing.T) {
	store := newPolicyStore(t)
	seedPriorAnswer(t, store, "原流程", "原流程的描述。")

	classifier := &stubTurnPolicyClassifier{
		policy: TurnPolicy{
			Route:                 RouteHybrid,
			NeedsRepoAccess:       true,
			Operation:             "investigate",
			Source:                "mixed",
			Confidence:            0.85,
			PresentationDirective: "mermaid 流程图",
		},
	}
	responder := &stubLocalResponder{localReply: "should-not-appear"}
	r, runner, _ := newTurnPolicyREPL(t, store, classifier, responder,
		"把上面的流程换成 mermaid，同时重新读仓库确认有没有 IO 分析\n/exit\n")
	adapter := &scriptedChatAdapter{}
	r.operationEnabled = true
	r.operationPlanner = NewCommandOperationPlanner(adapter)
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 1 {
		t.Fatalf("runner.Run: got %d, want 1", len(runner.requests))
	}
	req := runner.requests[0]
	if strings.Contains(req, "Presentation directive") || strings.Contains(req, "mermaid 流程图") {
		t.Errorf("hybrid request must not embed typed directive text; got %q", req)
	}
	if len(runner.seenDirectives) != 1 || runner.seenDirectives[0] != "mermaid 流程图" {
		t.Fatalf("hybrid directive not delivered through typed setter: seen=%q setCalls=%q",
			runner.seenDirectives, runner.directiveSetCalls)
	}
	if !strings.Contains(req, "重新读仓库确认有没有 IO 分析") {
		t.Errorf("hybrid request must preserve original user intent; got %q", req)
	}
	if len(responder.localCalls) != 0 {
		t.Errorf("local responder must not fire on hybrid; calls=%d", len(responder.localCalls))
	}
	if len(adapter.calls) != 0 {
		t.Fatalf("mixed source/external-observation pipeline route must not call operation planner/evaluator; calls=%d", len(adapter.calls))
	}
}

// TestTurnPolicyDispatch_HybridDoesNotPolluteMemory is the #6
// regression guard: the directive header MUST live on the
// effective-request channel only, NOT on the persisted
// memory.Turn.Request. Pre-fix the dispatch overwrote `line`
// before recordTurn, so every hybrid turn injected a
// "## Presentation directive..." prefix into BuildContext on
// the next turn — corrupting prior-conversation context for
// every downstream dispatch in the session.
func TestTurnPolicyDispatch_HybridDoesNotPolluteMemory(t *testing.T) {
	store := newPolicyStore(t)
	seedPriorAnswer(t, store, "原流程", "原流程的描述。")

	classifier := &stubTurnPolicyClassifier{
		policy: TurnPolicy{
			Route:                 RouteHybrid,
			NeedsRepoAccess:       true,
			Operation:             "investigate",
			Source:                "mixed",
			Confidence:            0.85,
			PresentationDirective: "mermaid 流程图",
		},
	}
	originalRequest := "把上面的流程换成 mermaid，同时重新读仓库确认有没有 IO 分析"
	responder := &stubLocalResponder{}
	r, _, _ := newTurnPolicyREPL(t, store, classifier, responder, originalRequest+"\n/exit\n")
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	recent := store.Recent()
	// Find the hybrid turn — it's the most recent one with the
	// pipeline-kind label (seed turn was also pipeline; this is
	// the second).
	if len(recent) < 2 {
		t.Fatalf("expected at least 2 recent turns (seed + hybrid); got %d", len(recent))
	}
	hybridTurn := recent[len(recent)-1]
	if strings.Contains(hybridTurn.Request, "## Presentation directive") {
		t.Errorf("memory.Turn.Request leaked directive header: %q", hybridTurn.Request)
	}
	if hybridTurn.Request != originalRequest {
		t.Errorf("memory.Turn.Request must be user-authentic; got %q want %q",
			hybridTurn.Request, originalRequest)
	}
}

// TestTurnPolicyDispatch_LocalRecordsAsKindPipeline is the #5
// regression guard. A local-route turn is structurally a
// derivative of a previous pipeline answer; persisting it as
// KindChitchat (the pre-fix shape) capped recent_body_chars at
// 800, which truncated multi-row tables / diagrams in the next
// turn's lastAnswerText and broke "transform of THIS transform"
// chains. KindPipeline is the existing channel that preserves
// long-form derived content — no new Kind constant needed.
func TestTurnPolicyDispatch_LocalRecordsAsKindPipeline(t *testing.T) {
	store := newPolicyStore(t)
	seedPriorAnswer(t, store, "概览", "原始 pipeline 答案。")

	classifier := &stubTurnPolicyClassifier{
		policy: TurnPolicy{
			Route:                 RouteLocal,
			Operation:             "transform",
			Source:                "last_answer",
			Confidence:            0.92,
			PresentationDirective: "markdown 表格",
		},
	}
	responder := &stubLocalResponder{localReply: "| col | val |\n|---|---|\n| A | 1 |"}
	r, _, _ := newTurnPolicyREPL(t, store, classifier, responder, "换成 markdown 表格\n/exit\n")
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	recent := store.Recent()
	// Last entry is the local turn we just drove.
	if len(recent) == 0 {
		t.Fatalf("expected local turn in store; recent empty")
	}
	localTurn := recent[len(recent)-1]
	if localTurn.Kind != memory.KindPipeline {
		t.Errorf("local turn Kind: got %q, want %q (pipeline retention preserves long-form derived content)",
			localTurn.Kind, memory.KindPipeline)
	}
	if !strings.Contains(localTurn.Response, "| col | val |") {
		t.Errorf("local turn Response missing transformation body: %q", localTurn.Response)
	}
}

// TestTurnPolicyDispatch_AttachmentNoPriorDemotesLocalToRepo is
// the #3 regression guard at the dispatch level. With a sticky
// attached log AND no prior answer, even an LLM that emits
// route=local must end up in the pipeline so log_triage gets a
// chance to consume the attachment. Companion to the unit-level
// guard table above.
func TestTurnPolicyDispatch_AttachmentNoPriorDemotesLocalToRepo(t *testing.T) {
	store := newPolicyStore(t)
	// No seedPriorAnswer — first turn shape.

	classifier := &stubTurnPolicyClassifier{
		policy: TurnPolicy{
			Route:      RouteLocal,
			Operation:  "investigate",
			Source:     "current_message",
			Confidence: 0.7,
		},
	}
	responder := &stubLocalResponder{localReply: "should-not-appear"}
	out := &bytes.Buffer{}
	runner := &requestCapturingRunner{}
	r := New(Config{
		Runner:             runner,
		Store:              store,
		Render:             renderNothing,
		RepoRoot:           ".",
		Branch:             "main",
		In:                 strings.NewReader("分析这条 panic\n/exit\n"),
		Out:                out,
		Prompt:             ">",
		PromptCont:         ".",
		Banner:             "test-banner",
		Language:           "en",
		ChitchatResponder:  responder,
		ChitchatClassifier: classifier,
	})
	r.attachedLog = "panic: runtime error: index out of range\n  goroutine 1 ..."

	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	// hint sent to classifier must carry the attachment marker.
	if len(classifier.hints) != 1 || !strings.Contains(classifier.hints[0], "attachment=true") {
		t.Errorf("classifier hint must carry attachment=true; got %v", classifier.hints)
	}
	// Guard demoted local→repo because attachment was present
	// and no prior answer existed; runner.Run must have fired.
	if len(runner.requests) != 1 {
		t.Errorf("attachment+no-prior must demote local to pipeline; runner calls=%d, want 1", len(runner.requests))
	}
	if len(responder.localCalls) != 0 {
		t.Errorf("local responder must not fire when attachment present without prior; calls=%d", len(responder.localCalls))
	}
}

// TestTurnPolicyDispatch_FirstTurnTransformDemotedToClarify is test
// case 5 from the task brief: "换成 mermaid 图例" with no prior
// answer must NOT fabricate. The deterministic guard demotes
// Local→Clarify; the dispatcher prints the clarify message and
// does NOT call the LLM or the pipeline.
func TestTurnPolicyDispatch_FirstTurnTransformDemotedToClarify(t *testing.T) {
	store := newPolicyStore(t)
	// No seedPriorAnswer — this is a first-turn scenario.

	// Even if the LLM (incorrectly) emits route=local, the guard
	// catches it. Test the guard path explicitly.
	classifier := &stubTurnPolicyClassifier{
		policy: TurnPolicy{
			Route:                 RouteLocal,
			Operation:             "transform",
			Source:                "last_answer",
			Confidence:            0.9,
			PresentationDirective: "mermaid",
		},
	}
	responder := &stubLocalResponder{localReply: "should-not-appear"}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, responder, "换成 mermaid 图例\n/exit\n")
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Errorf("clarify must NOT call runner.Run; got %d", len(runner.requests))
	}
	if len(responder.localCalls) != 0 {
		t.Errorf("clarify must NOT call local responder; calls=%d", len(responder.localCalls))
	}
	if len(responder.calls) != 0 {
		t.Errorf("clarify must NOT call legacy responder.Respond either; calls=%d", len(responder.calls))
	}
	// Route summary: clarify emits "◇ clarify · <seg>" via the
	// renderer-nil fallback in this test (no Renderer attached);
	// the EN label "clarify" must appear in stdout.
	if !strings.Contains(out.String(), "clarify") {
		t.Errorf("clarify route summary missing in stdout; got %q", out.String())
	}
	// Memory must not be polluted by the unanswered turn.
	if n := len(store.Recent()); n != 0 {
		t.Errorf("clarify must not record a memory turn; recent=%d", n)
	}
}

// TestTurnPolicyDispatch_GreetingStaysLocal pins test case 6: a
// pure greeting routes to local even with no prior answer (the
// "no prior" guard only fires for transform/summarize/translate/
// elaborate operations).
func TestTurnPolicyDispatch_GreetingStaysLocal(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{
		policy: TurnPolicy{
			Route:      RouteLocal,
			Operation:  "chat",
			Source:     "current_message",
			Confidence: 0.95,
		},
	}
	responder := &stubLocalResponder{localReply: "你好!我是 CODRAX。"}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, responder, "你好\n/exit\n")
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 0 {
		t.Errorf("greeting must NOT call runner.Run; got %d", len(runner.requests))
	}
	if len(responder.localCalls) != 1 {
		t.Fatalf("greeting must call local responder; calls=%d, want 1", len(responder.localCalls))
	}
	if !strings.Contains(out.String(), "你好") {
		t.Errorf("greeting reply missing in stdout; got %q", out.String())
	}
}

// TestTurnPolicyDispatch_ClassifierErrorFallsThroughToPipeline pins
// the fail-safe: any ClassifyPolicy error routes the turn to the
// pipeline, NOT to the local responder. A broken classifier must
// not silently misroute real questions.
func TestTurnPolicyDispatch_ClassifierErrorFallsThroughToPipeline(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &stubTurnPolicyClassifier{
		err: errors.New("upstream timeout"),
	}
	responder := &stubLocalResponder{localReply: "should-not-appear"}
	r, runner, _ := newTurnPolicyREPL(t, store, classifier, responder, "how does X work?\n/exit\n")
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 1 {
		t.Errorf("classifier error must fall through to pipeline; runner calls=%d, want 1", len(runner.requests))
	}
	if len(responder.localCalls) != 0 {
		t.Errorf("local responder must not fire on classifier error; calls=%d", len(responder.localCalls))
	}
}

func TestTurnPolicyDispatch_StructuredErrorFallsBackToLegacyChitchat(t *testing.T) {
	store := newPolicyStore(t)
	classifier := &structuredErrorLegacyClassifier{
		policyErr:    errors.New("structured schema timeout"),
		legacyIsChat: true,
	}
	responder := &stubLocalResponder{
		stubResponder: stubResponder{reply: "hello from fallback"},
		localReply:    "should-not-use-local-route",
	}
	r, runner, out := newTurnPolicyREPL(t, store, classifier, responder, "hi there\n/exit\n")
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if classifier.policyCalls != 1 {
		t.Fatalf("structured classifier calls=%d, want 1", classifier.policyCalls)
	}
	if classifier.legacyCalls != 1 {
		t.Fatalf("legacy fallback classifier calls=%d, want 1", classifier.legacyCalls)
	}
	if len(runner.requests) != 0 {
		t.Errorf("legacy chitchat fallback must not enter pipeline; runner calls=%d", len(runner.requests))
	}
	if len(responder.calls) != 1 {
		t.Fatalf("legacy chitchat responder calls=%d, want 1", len(responder.calls))
	}
	if len(responder.localCalls) != 0 {
		t.Fatalf("legacy fallback should not use structured local route; local calls=%d", len(responder.localCalls))
	}
	if !strings.Contains(out.String(), "hello from fallback") {
		t.Errorf("fallback reply missing in output: %q", out.String())
	}
}

// ─── Layer 4: legacy classifier compat ─────────────────────────

// TestTurnPolicyDispatch_LegacyClassifierUsesBinaryPath verifies
// the dispatch's type-assert fallback: when the wired
// ChitchatClassifier does NOT implement TurnPolicyClassifier (e.g.
// the existing stubClassifier), the legacy binary
// chitchat/repo_question path is used unchanged. This is the
// guarantee that lets the existing chitchat_classifier_*_test.go
// tests continue to pass byte-identically after the new wiring
// lands.
func TestTurnPolicyDispatch_LegacyClassifierUsesBinaryPath(t *testing.T) {
	store := newPolicyStore(t)
	// stubClassifier (defined in chitchat_test.go) implements ONLY
	// ChitchatClassifier, NOT TurnPolicyClassifier — exactly the
	// shape we want to guard.
	legacy := &stubClassifier{isChitchat: true}
	responder := &stubResponder{reply: "hello"}

	out := &bytes.Buffer{}
	runner := &requestCapturingRunner{}
	r := New(Config{
		Runner:             runner,
		Store:              store,
		Render:             renderNothing,
		RepoRoot:           ".",
		Branch:             "main",
		In:                 strings.NewReader("hi there\n/exit\n"),
		Out:                out,
		Prompt:             ">",
		PromptCont:         ".",
		Banner:             "test-banner",
		Language:           "en",
		ChitchatResponder:  responder,
		ChitchatClassifier: legacy,
	})
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	// Legacy classifier got called, dispatched to chitchat,
	// runner.Run never fired — same shape the binary-path test in
	// chitchat_test.go locks.
	if len(legacy.calls) != 1 {
		t.Errorf("legacy classifier must be called via Classify; got %d", len(legacy.calls))
	}
	if len(responder.calls) != 1 {
		t.Errorf("responder must fire via legacy chitchat path; calls=%d", len(responder.calls))
	}
	if len(runner.requests) != 0 {
		t.Errorf("runner.Run must NOT fire on legacy chitchat path; got %d", len(runner.requests))
	}
}

// ─── Layer 5: helpers ─────────────────────────────────────────

// TestComposeEffectiveRequest locks the wire format for the
// prior-conversation block + user request assembly. Presentation
// directives are carried through typed runner metadata and must not
// appear in this string.
func TestComposeEffectiveRequest(t *testing.T) {
	// No prior conversation: raw request is preserved byte-for-byte.
	got := composeEffectiveRequest("", "重新读仓库确认有没有 IO 分析")
	if got != "重新读仓库确认有没有 IO 分析" {
		t.Errorf("no-prior request should pass through; got %q", got)
	}

	// Prior conversation stays separated from the current request.
	combo := composeEffectiveRequest("(recent: turn-1)", "second turn")
	if !strings.Contains(combo, "## Prior conversation\n(recent: turn-1)") {
		t.Errorf("combo missing prior block: %q", combo)
	}
	if !strings.Contains(combo, "## Current request\nsecond turn") {
		t.Errorf("combo missing current request: %q", combo)
	}
	if strings.Contains(combo, "Presentation directive") {
		t.Errorf("combo must not contain presentation directive header: %q", combo)
	}
	if prior, current := types.SplitConversation(combo); strings.Contains(prior, "second turn") || current != "second turn" {
		t.Errorf("conversation split wrong; prior=%q current=%q", prior, current)
	}
	// Section ordering: prior must precede current.
	ppos := strings.Index(combo, "## Prior conversation")
	cpos := strings.Index(combo, "## Current request")
	if !(ppos < cpos) {
		t.Errorf("section ordering wrong: prior=%d current=%d", ppos, cpos)
	}

	// Empty prior must round-trip the request verbatim.
	if got := composeEffectiveRequest("  ", "raw"); got != "raw" {
		t.Errorf("empty prior must pass-through; got %q", got)
	}

	// hybridRequestPrefix wrapper remains source-compatible but no
	// longer embeds the directive.
	if got := hybridRequestPrefix("brief", "show me"); got != "show me" {
		t.Errorf("hybridRequestPrefix should not embed directive; got %q", got)
	}
}

// TestRenderRichResponse_MermaidBlocksGetTransformed is the
// regression guard for the user-reported "图表/markdown 未渲染"
// bug: local + chitchat dispatch used to call renderBordered
// directly on the LLM's raw markdown source, so a ```mermaid```
// fence printed as literal source. renderRichResponse now runs
// RenderMermaidBlocks on the way through. The transformation
// rewrites the fence info-string from "mermaid" to "text" — that
// rename is the contract this test pins (the output may be the
// same plain mermaid source when the library bails on the body,
// but the fence MUST have been processed; a "mermaid" → "text"
// info-string flip is the unambiguous "we tried" signal).
func TestRenderRichResponse_MermaidBlocksGetTransformed(t *testing.T) {
	r := &REPL{} // nil renderer is OK — RenderMermaidBlocks is package-level
	in := "前缀文字\n\n```mermaid\nflowchart LR\n  A --> B\n  B --> C\n```\n\n后缀文字"
	got := r.renderRichResponse(in)
	if strings.Contains(got, "```mermaid") {
		t.Errorf("renderRichResponse must rewrite ```mermaid``` fence; got %q", got)
	}
	if !strings.Contains(got, "前缀文字") || !strings.Contains(got, "后缀文字") {
		t.Errorf("renderRichResponse must preserve surrounding prose; got %q", got)
	}
}

// TestRenderRichResponse_NilRendererKeepsMarkdown verifies the
// fallback contract for tests / scripts that pass Renderer=nil:
// the function still runs RenderMermaidBlocks (self-contained),
// but skips glamour and returns the markdown source verbatim.
// Without this the existing test fixtures that asserted on raw
// markdown content (e.g. "[local] reply built from") would break.
func TestRenderRichResponse_NilRendererKeepsMarkdown(t *testing.T) {
	r := &REPL{} // renderer == nil
	in := "## 标题\n**bold** plain\n- item 1\n- item 2"
	got := r.renderRichResponse(in)
	// No glamour → input passes through (modulo the optional
	// RenderMermaidBlocks pass which is a no-op without a fence).
	if got != in {
		t.Errorf("nil renderer must preserve markdown source; got %q want %q", got, in)
	}
}

// TestLastAnswerText_SkipsErrorPlaceholder verifies the dispatcher
// does not hand a sanitised error string to the local responder as
// "the previous answer". recordTurn writes that placeholder when
// the pipeline ended with TaskState.LastError; the local path must
// treat such a turn as if no prior answer exists.
func TestLastAnswerText_SkipsErrorPlaceholder(t *testing.T) {
	store := newPolicyStore(t)
	if err := store.Append(memory.Turn{
		ID:       "t-err",
		Request:  "fail",
		Response: "(previous attempt ended in error — details omitted from memory)",
		Kind:     memory.KindPipeline,
	}); err != nil {
		t.Fatalf("seed error turn: %v", err)
	}
	if err := store.Append(memory.Turn{
		ID:       "t-real",
		Request:  "ok",
		Response: "real answer text",
		Kind:     memory.KindPipeline,
	}); err != nil {
		t.Fatalf("seed real turn: %v", err)
	}

	r := &REPL{store: store}
	got := r.lastAnswerText()
	if got != "real answer text" {
		t.Errorf("lastAnswerText must skip the error placeholder; got %q", got)
	}

	// With ONLY an error turn, the helper returns empty.
	storeErrOnly := newPolicyStore(t)
	if err := storeErrOnly.Append(memory.Turn{
		ID:       "t-err-only",
		Request:  "fail",
		Response: "(previous attempt ended in error — only this turn)",
		Kind:     memory.KindPipeline,
	}); err != nil {
		t.Fatalf("seed error-only turn: %v", err)
	}
	rErr := &REPL{store: storeErrOnly}
	if got := rErr.lastAnswerText(); got != "" {
		t.Errorf("error-only store must yield empty lastAnswer; got %q", got)
	}
}
