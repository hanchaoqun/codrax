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
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/memory"
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
}

func (s *stubTurnPolicyClassifier) Classify(_ context.Context, line, hint string) (bool, error) {
	return false, errors.New("stubTurnPolicyClassifier.Classify should not be invoked when TurnPolicyClassifier path is preferred")
}

func (s *stubTurnPolicyClassifier) ClassifyPolicy(_ context.Context, line, hint string, hasPriorAnswer bool) (TurnPolicy, error) {
	s.calls = append(s.calls, line)
	s.hints = append(s.hints, hint)
	s.hadPriorFlags = append(s.hadPriorFlags, hasPriorAnswer)
	return s.policy, s.err
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
}

type stubLocalCall struct {
	userLine   string
	prior      string
	lastAnswer string
	directive  string
}

func (s *stubLocalResponder) RespondLocal(_ context.Context, userLine, prior, lastAnswer, directive string) (string, error) {
	s.localCalls = append(s.localCalls, stubLocalCall{
		userLine:   userLine,
		prior:      prior,
		lastAnswer: lastAnswer,
		directive:  directive,
	})
	if s.localErr != nil {
		return "", s.localErr
	}
	return s.localReply, nil
}

// requestCapturingRunner is a Runner that records every Run() call's
// request string AND counts attached-log/SetMode invocations the
// dispatch path forwards. Used by the hybrid-route test to assert
// the directive prefix reaches Run() verbatim.
type requestCapturingRunner struct {
	logAwareRunner
	requests []string
}

func (r *requestCapturingRunner) Run(req, repo, branch string) (*types.BusContext, error) {
	r.requests = append(r.requests, req)
	return r.logAwareRunner.Run(req, repo, branch)
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
		localReply: "```mermaid\nsequenceDiagram\n  Explorer->>Runtime: propose\n```",
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
		!strings.Contains(printed, "Browser preview: http://127.0.0.1:49152/preview/local?token=t") {
		t.Fatalf("local markdown/preview hints missing:\n%s", printed)
	}
	if preview.path == "" {
		t.Fatalf("preview was not registered")
	}
	data, err := os.ReadFile(preview.path)
	if err != nil {
		t.Fatalf("read local dump %q: %v", preview.path, err)
	}
	body := string(data)
	if !strings.Contains(body, "# 问题\n\n换成时序图\n") ||
		!strings.Contains(body, "# 回答\n\n```mermaid\nsequenceDiagram") {
		t.Fatalf("local dump body missing request/answer:\n%s", body)
	}
	recent := store.Recent()
	if len(recent) == 0 {
		t.Fatalf("expected local turn in memory")
	}
	if strings.Contains(recent[len(recent)-1].Response, "Raw Markdown saved") ||
		strings.Contains(recent[len(recent)-1].Response, "Browser preview") {
		t.Fatalf("markdown hints leaked into memory: %q", recent[len(recent)-1].Response)
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
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 1 {
		t.Fatalf("runner.Run: got %d, want 1", len(runner.requests))
	}
	if len(responder.localCalls) != 0 {
		t.Errorf("local responder must not fire on route=repo; calls=%d", len(responder.localCalls))
	}
	if strings.Contains(runner.requests[0], "Presentation directive") {
		t.Errorf("repo route without directive must not carry a presentation directive prefix; got %q", runner.requests[0])
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
	if !strings.Contains(req, "## Presentation directive (apply to final answer)") {
		t.Fatalf("repo request with typed presentation directive must carry directive header; got %q", req)
	}
	if !strings.Contains(req, "logic flow diagram showing anti-hallucination mechanisms") {
		t.Fatalf("repo request lost presentation directive; got %q", req)
	}
	if !strings.Contains(req, "输出各自的逻辑视图") {
		t.Fatalf("repo request must preserve original user wording; got %q", req)
	}
	if len(responder.localCalls) != 0 {
		t.Errorf("local responder must not fire on repo; calls=%d", len(responder.localCalls))
	}
}

// TestTurnPolicyDispatch_HybridCarriesDirectiveIntoPipeline is the
// route=hybrid contract: pipeline runs AND the directive prefix
// reaches the runner so the finalizer can render the requested
// shape.
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
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if len(runner.requests) != 1 {
		t.Fatalf("runner.Run: got %d, want 1", len(runner.requests))
	}
	req := runner.requests[0]
	if !strings.Contains(req, "## Presentation directive (apply to final answer)") {
		t.Errorf("hybrid request must carry directive header; got %q", req)
	}
	if !strings.Contains(req, "mermaid 流程图") {
		t.Errorf("hybrid request must embed directive text; got %q", req)
	}
	if !strings.Contains(req, "重新读仓库确认有没有 IO 分析") {
		t.Errorf("hybrid request must preserve original user intent; got %q", req)
	}
	if len(responder.localCalls) != 0 {
		t.Errorf("local responder must not fire on hybrid; calls=%d", len(responder.localCalls))
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
// prior-conversation block + current-turn directive + user request
// assembly. The pipeline splits conversation by header name, so the
// directive must remain inside the Current request section instead of
// being misfiled as prior conversation.
func TestComposeEffectiveRequest(t *testing.T) {
	// Directive only.
	got := composeEffectiveRequest("mermaid 流程图", "", "重新读仓库确认有没有 IO 分析")
	if !strings.HasPrefix(got, "## Current request\n## Presentation directive (apply to final answer)\n") {
		t.Errorf("missing directive header: %q", got)
	}
	if !strings.Contains(got, "重新读仓库确认有没有 IO 分析") {
		t.Errorf("missing user request body: %q", got)
	}
	if prior, current := types.SplitConversation(got); strings.Contains(prior, "Presentation directive") || !strings.Contains(current, "Presentation directive") {
		t.Errorf("directive must be parsed as current request; prior=%q current=%q", prior, current)
	}

	// Directive + prior conversation interleave.
	combo := composeEffectiveRequest("mermaid", "(recent: turn-1)", "second turn")
	if !strings.Contains(combo, "## Presentation directive") {
		t.Errorf("combo missing directive: %q", combo)
	}
	if !strings.Contains(combo, "## Prior conversation\n(recent: turn-1)") {
		t.Errorf("combo missing prior block: %q", combo)
	}
	if !strings.Contains(combo, "## Current request\n## Presentation directive (apply to final answer)\nmermaid\n\nsecond turn") {
		t.Errorf("combo missing current request: %q", combo)
	}
	if prior, current := types.SplitConversation(combo); strings.Contains(prior, "Presentation directive") || !strings.Contains(current, "Presentation directive") {
		t.Errorf("combo directive must be parsed as current request; prior=%q current=%q", prior, current)
	}
	// Section ordering: prior must precede current; directive lives inside current.
	dpos := strings.Index(combo, "## Presentation directive")
	ppos := strings.Index(combo, "## Prior conversation")
	cpos := strings.Index(combo, "## Current request")
	if !(ppos < cpos && cpos < dpos) {
		t.Errorf("section ordering wrong: directive=%d prior=%d current=%d", dpos, ppos, cpos)
	}

	// Empty directive AND empty prior must round-trip the request
	// verbatim (legacy "no decoration" behaviour preserved).
	if got := composeEffectiveRequest("   ", "  ", "raw"); got != "raw" {
		t.Errorf("empty directive+prior must pass-through; got %q", got)
	}

	// hybridRequestPrefix wrapper still works for callers that
	// only care about the directive (no prior).
	if got := hybridRequestPrefix("brief", "show me"); !strings.Contains(got, "## Presentation directive") {
		t.Errorf("hybridRequestPrefix wrapper broken: %q", got)
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
