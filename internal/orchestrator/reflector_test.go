package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// reflectorStubAdapter is a minimal llm.Adapter for unit tests. Only
// implements Chat — every other method returns harmless defaults so
// the type satisfies the interface without dragging real provider
// state in.
type reflectorStubAdapter struct {
	resp        llm.Response
	err         error
	lastSystem  string
	lastUser    string
	lastTools   []llm.ToolSchema
}

func (s *reflectorStubAdapter) Chat(_ context.Context, msgs []llm.Message, tools []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	for _, m := range msgs {
		switch m.Role {
		case "system":
			s.lastSystem = m.Content
		case "user":
			s.lastUser = m.Content
		}
	}
	s.lastTools = tools
	return s.resp, s.err
}
func (s *reflectorStubAdapter) ModelID() string                { return "stub" }
func (s *reflectorStubAdapter) MaxContextTokens() int          { return 128000 }
func (s *reflectorStubAdapter) MaxOutputTokens() int           { return 0 }
func (s *reflectorStubAdapter) RequestTimeout() time.Duration  { return 0 }
func (s *reflectorStubAdapter) RetryMaxAttempts() int          { return 1 }

// TestReflector_Disabled covers the nil-adapter path: NewReflector(nil)
// must return ("", nil) so clearForReplan falls back to heuristic
// hint without aborting the retry.
func TestReflector_Disabled(t *testing.T) {
	r := NewReflector(nil)
	got, err := r.Reflect(context.Background(), ReflectorInput{Attempt: 1})
	if err != nil {
		t.Errorf("nil adapter Reflect should return (\"\", nil); got err=%v", err)
	}
	if got != "" {
		t.Errorf("nil adapter Reflect should return empty critique; got %q", got)
	}
}

// TestReflector_HappyPath covers the canonical structured response
// after the Module G upgrade: adapter returns one
// emit_failure_observation tool call; Reflect assembles the
// observation into "Reviewer observation: ..." text.
func TestReflector_HappyPath(t *testing.T) {
	stub := &reflectorStubAdapter{
		resp: llm.Response{
			ToolCalls: []llm.ToolCall{{
				Name: "emit_failure_observation",
				Params: json.RawMessage(`{
					"observation": "the plan modified parser.go but the failing test test_overflow exercises the multiplication path which the plan left unchanged"
				}`),
			}},
		},
	}
	r := NewReflector(stub)
	got, err := r.Reflect(context.Background(), ReflectorInput{
		Attempt:         1,
		OriginalRequest: "implement parse_trinary",
		PlanSummary:     "trinary parser",
		FailingTests: []ReflectorFailedTest{{
			Suite:       "trinary",
			AssertionID: "TestParseTrinary",
			Detail:      "ParseTrinary(\"0000201\") returned error \"overflow\"",
		}},
	})
	if err != nil {
		t.Fatalf("happy path Reflect err: %v", err)
	}
	if !strings.Contains(got, "Reviewer observation:") {
		t.Errorf("output should carry observation label; got %q", got)
	}
	if !strings.Contains(got, "the plan modified parser.go") {
		t.Errorf("output should carry observation text verbatim; got %q", got)
	}
}

// TestReflector_ChatErrorDegrades covers the retry-must-not-block
// invariant: when the LLM errors, Reflect returns ("", err) so the
// caller falls through to the heuristic hint.
func TestReflector_ChatErrorDegrades(t *testing.T) {
	stub := &reflectorStubAdapter{err: errors.New("provider timeout")}
	r := NewReflector(stub)
	got, err := r.Reflect(context.Background(), ReflectorInput{Attempt: 1, FailingTests: []ReflectorFailedTest{{Suite: "x", AssertionID: "y"}}})
	if err == nil {
		t.Errorf("chat error should surface as Reflect error; got nil")
	}
	if got != "" {
		t.Errorf("chat error should yield empty critique; got %q", got)
	}
}

// TestReflector_NoToolCallDegrades covers the malformed-output path:
// LLM returned content but no structured tool call. Reflect surfaces
// the error so the heuristic hint takes over.
func TestReflector_NoToolCallDegrades(t *testing.T) {
	stub := &reflectorStubAdapter{resp: llm.Response{Content: "I think the bug is..."}}
	r := NewReflector(stub)
	_, err := r.Reflect(context.Background(), ReflectorInput{Attempt: 1, FailingTests: []ReflectorFailedTest{{Suite: "x", AssertionID: "y"}}})
	if err == nil {
		t.Errorf("no-tool-call response should error; got nil")
	}
}

// TestReflector_UncertaintySurfaces covers the optional uncertainty
// field. When the reviewer says it's not sure about something, the
// planner should see that uncertainty alongside the observation.
func TestReflector_UncertaintySurfaces(t *testing.T) {
	stub := &reflectorStubAdapter{
		resp: llm.Response{ToolCalls: []llm.ToolCall{{
			Name: "emit_failure_observation",
			Params: json.RawMessage(`{
				"observation": "test_x failed with AssertionError on stub.py:42",
				"uncertainty": "I cannot tell from the input whether stub.py was previously passing or has always failed in this fixture"
			}`),
		}}},
	}
	r := NewReflector(stub)
	got, err := r.Reflect(context.Background(), ReflectorInput{Attempt: 1, FailingTests: []ReflectorFailedTest{{Suite: "x", AssertionID: "y"}}})
	if err != nil {
		t.Fatalf("Reflect err: %v", err)
	}
	if !strings.Contains(got, "Reviewer uncertainty:") {
		t.Errorf("output should include uncertainty section; got %q", got)
	}
}

// TestReflector_FullLedgerPropagatesToUserPrompt verifies Module G's
// load-bearing wiring: the iteration ledger is rendered into the
// reviewer's user message verbatim, so the reviewer can observe
// patterns across attempts. The system never pre-classifies the
// patterns; the reviewer reads the data.
func TestReflector_FullLedgerPropagatesToUserPrompt(t *testing.T) {
	stub := &reflectorStubAdapter{
		resp: llm.Response{ToolCalls: []llm.ToolCall{{
			Name:   "emit_failure_observation",
			Params: json.RawMessage(`{"observation": "ok"}`),
		}}},
	}
	r := NewReflector(stub)
	_, err := r.Reflect(context.Background(), ReflectorInput{
		Attempt: 3,
		IterationLedger: []types.IterationRecord{
			{Attempt: 1, PlanSummary: "first attempt summary", FailureSummary: "first failure stderr"},
			{Attempt: 2, PlanSummary: "second attempt summary", FailureSummary: "second failure stderr"},
		},
		FailingTests: []ReflectorFailedTest{{Suite: "x", AssertionID: "y", Detail: "raw stderr line"}},
	})
	if err != nil {
		t.Fatalf("Reflect err: %v", err)
	}
	// Ledger content propagates verbatim.
	for _, want := range []string{
		"first attempt summary", "first failure stderr",
		"second attempt summary", "second failure stderr",
		"## Iteration history",
	} {
		if !strings.Contains(stub.lastUser, want) {
			t.Errorf("user prompt should include %q; got:\n%s", want, stub.lastUser)
		}
	}
}

// TestReflector_SystemPromptHasNoPrescriptiveGuards locks the
// Module G discipline: the system prompt must NOT contain
// "DO NOT classify X as Y" rule lists. The reviewer is a model and
// is asked to OBSERVE; rule lists pre-encode the system's view of
// what's defensible, which is exactly the anti-pattern this batch
// removes.
func TestReflector_SystemPromptHasNoPrescriptiveGuards(t *testing.T) {
	stub := &reflectorStubAdapter{
		resp: llm.Response{ToolCalls: []llm.ToolCall{{
			Name:   "emit_failure_observation",
			Params: json.RawMessage(`{"observation": "ok"}`),
		}}},
	}
	r := NewReflector(stub)
	_, _ = r.Reflect(context.Background(), ReflectorInput{Attempt: 1, FailingTests: []ReflectorFailedTest{{Suite: "x", AssertionID: "y"}}})
	for _, banned := range []string{
		"DO NOT classify",
		"DO NOT speculate",
		"DO NOT blame",
		"Tests are authoritative", // pre-encodes a stance the reviewer should reach itself
	} {
		if strings.Contains(stub.lastSystem, banned) {
			t.Errorf("system prompt must not contain prescriptive guard %q; got:\n%s", banned, stub.lastSystem)
		}
	}
}

// TestReflectFull_BothToolsEmittedReturnObservationAndPattern
// pins commit 35-36's stage 3 path: when the LLM emits BOTH
// emit_failure_observation AND emit_failure_pattern in one
// dispatch, ReflectFull returns both products. The Pattern is
// extracted via unmarshalFailurePattern + validation passes.
func TestReflectFull_BothToolsEmittedReturnObservationAndPattern(t *testing.T) {
	stub := &reflectorStubAdapter{
		resp: llm.Response{
			ToolCalls: []llm.ToolCall{
				{
					Name:   "emit_failure_observation",
					Params: json.RawMessage(`{"observation": "the plan modified handler.py but the failing test exercises a path the plan left unchanged"}`),
				},
				{
					Name: "emit_failure_pattern",
					Params: json.RawMessage(`{
						"name": "stale-fixture-on-interface-widen",
						"description": "When an interface is widened with a new method, the test fixtures stubbing it tend to lag — the new method's stub default falls back to zero/empty values that pass the obvious assertions but break code paths exercising the new method.",
						"trigger": "widening an interface that downstream tests stub",
						"consequence": "test passes but production hits zero-value fallback path",
						"confidence": 0.8,
						"applies_to_kinds": ["feature", "refactor"]
					}`),
				},
			},
		},
	}
	r := NewReflector(stub).(*llmReflector)
	out, err := r.ReflectFull(context.Background(), ReflectorInput{
		Attempt: 1, OriginalRequest: "widen Authenticator interface",
		FailingTests: []ReflectorFailedTest{{Suite: "auth", AssertionID: "TestSession"}},
	})
	if err != nil {
		t.Fatalf("ReflectFull: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil output")
	}
	if !strings.Contains(out.Observation, "the plan modified handler.py") {
		t.Errorf("observation drift; got %q", out.Observation)
	}
	if out.Pattern == nil {
		t.Fatal("expected non-nil pattern")
	}
	if out.Pattern.Name != "stale-fixture-on-interface-widen" {
		t.Errorf("pattern name drift; got %q", out.Pattern.Name)
	}
	if out.Pattern.Confidence != 0.8 {
		t.Errorf("confidence drift; got %f", out.Pattern.Confidence)
	}
	if len(out.Pattern.AppliesToKinds) != 2 {
		t.Errorf("applies_to_kinds drift; got %v", out.Pattern.AppliesToKinds)
	}
}

// TestReflectFull_LowConfidencePatternDropped pins the floor:
// emit_failure_pattern with confidence < 0.5 is dropped at the
// unmarshal validator (cried-wolf noise reduction). The
// observation still flows through; only Pattern is nil.
func TestReflectFull_LowConfidencePatternDropped(t *testing.T) {
	stub := &reflectorStubAdapter{
		resp: llm.Response{
			ToolCalls: []llm.ToolCall{
				{Name: "emit_failure_observation",
					Params: json.RawMessage(`{"observation": "obs text long enough to land here"}`)},
				{Name: "emit_failure_pattern",
					Params: json.RawMessage(`{
						"name": "uncertain low-confidence pattern emit",
						"description": "Some failure that the reviewer was not really sure about but emitted anyway just in case it might be a thing",
						"trigger": "some uncertain trigger condition",
						"confidence": 0.3
					}`)},
			},
		},
	}
	r := NewReflector(stub).(*llmReflector)
	out, err := r.ReflectFull(context.Background(), ReflectorInput{Attempt: 1, FailingTests: []ReflectorFailedTest{{Suite: "x", AssertionID: "y"}}})
	if err != nil {
		t.Fatalf("ReflectFull: %v", err)
	}
	if out.Pattern != nil {
		t.Errorf("low-confidence pattern should be dropped; got %+v", out.Pattern)
	}
	if !strings.Contains(out.Observation, "obs text") {
		t.Errorf("observation should still flow; got %q", out.Observation)
	}
}

// TestReflectFull_RejectsNonASCIIName pins commit 39's P4
// fix: a Chinese-named emit_failure_pattern is dropped at
// the unmarshal validator because SimilarTo's whitespace-
// token dedup heuristic can't compute meaningful overlap on
// CJK names. Description may be in any language; only Name
// is gated.
func TestReflectFull_RejectsNonASCIIName(t *testing.T) {
	stub := &reflectorStubAdapter{
		resp: llm.Response{
			ToolCalls: []llm.ToolCall{
				{Name: "emit_failure_observation",
					Params: json.RawMessage(`{"observation": "obs text long enough to land here for the test"}`)},
				{Name: "emit_failure_pattern",
					Params: json.RawMessage(`{
						"name": "对共享结构添加方法时遗漏锁",
						"description": "在 goroutine 共享的结构体上添加新方法时,常忘记扩展互斥锁的覆盖范围,导致并发访问时数据竞争问题。",
						"trigger": "向共享结构添加方法的时候",
						"confidence": 0.8
					}`)},
			},
		},
	}
	r := NewReflector(stub).(*llmReflector)
	out, err := r.ReflectFull(context.Background(), ReflectorInput{Attempt: 1, FailingTests: []ReflectorFailedTest{{Suite: "x", AssertionID: "y"}}})
	if err != nil {
		t.Fatalf("ReflectFull: %v", err)
	}
	if out.Pattern != nil {
		t.Errorf("non-ASCII name should be rejected; got %+v", out.Pattern)
	}
	if !strings.Contains(out.Observation, "obs text") {
		t.Errorf("observation should still flow; got %q", out.Observation)
	}
}

// TestIsASCII pins the helper used by the non-ASCII name
// guard.
func TestIsASCII(t *testing.T) {
	for _, c := range []struct {
		in   string
		want bool
	}{
		{"plain english name", true},
		{"name-with-dashes_and_underscores", true},
		{"", true},
		{"含中文", false},
		{"mixed eng 中", false},
		{"emoji 🚀", false},
	} {
		if got := isASCII(c.in); got != c.want {
			t.Errorf("isASCII(%q) = %v; want %v", c.in, got, c.want)
		}
	}
}

// TestSetFailureTaxonomyStore_TogglesReflectorPatternTool
// pins commit 40 P1's wiring: when the orchestrator's
// SetFailureTaxonomyStore is called with a non-nil store,
// it must propagate the "you have a destination for the
// pattern" signal to the reflector via the patternToolToggler
// interface. Calling with nil propagates "off".
func TestSetFailureTaxonomyStore_TogglesReflectorPatternTool(t *testing.T) {
	stub := &reflectorStubAdapter{}
	r := NewReflector(stub).(*llmReflector)
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.reflector = r

	// Default: off.
	if r.patternToolOn {
		t.Errorf("default state should be off; got on")
	}

	// Wire a non-nil store → toggle on.
	dir := t.TempDir()
	store := NewFailureTaxonomyStore(dir, "test-repo", 50, 90)
	o.SetFailureTaxonomyStore(store)
	if !r.patternToolOn {
		t.Errorf("after SetFailureTaxonomyStore(non-nil) reflector should be on; got off")
	}

	// Wire nil → toggle off.
	o.SetFailureTaxonomyStore(nil)
	if r.patternToolOn {
		t.Errorf("after SetFailureTaxonomyStore(nil) reflector should be off; got on")
	}
}

// TestReflectFull_PreservationClausesParsed pins keyword-gate audit
// HIGH-2 (2026-05-17): emit_failure_observation now accepts a typed
// preservation_clauses[] array; the parse path unmarshals it into
// ReflectorOutput.PreservationClauses, trimming whitespace and
// dropping empties so a non-nil populated slice is the
// authoritative downstream signal.
func TestReflectFull_PreservationClausesParsed(t *testing.T) {
	cases := []struct {
		name      string
		args      string
		wantLen   int
		wantFirst string
	}{
		{
			name: "two clauses preserved verbatim",
			args: `{
				"observation": "the plan's new helper function is correct but the call-site rename broke the existing import",
				"preservation_clauses": [
					"the new validateInput helper in parser.go",
					"the migration ordering in 0042_users.sql"
				]
			}`,
			wantLen:   2,
			wantFirst: "the new validateInput helper in parser.go",
		},
		{
			name: "whitespace-only entries dropped; empties dropped",
			args: `{
				"observation": "obs text long enough to land",
				"preservation_clauses": ["  ", "", "real clause", "   "]
			}`,
			wantLen:   1,
			wantFirst: "real clause",
		},
		{
			name: "field absent → nil slice",
			args: `{
				"observation": "obs text long enough to land"
			}`,
			wantLen: 0,
		},
		{
			name: "empty array → nil slice",
			args: `{
				"observation": "obs text long enough to land",
				"preservation_clauses": []
			}`,
			wantLen: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := &reflectorStubAdapter{
				resp: llm.Response{
					ToolCalls: []llm.ToolCall{{
						Name:   "emit_failure_observation",
						Params: json.RawMessage(tc.args),
					}},
				},
			}
			r := NewReflector(stub).(*llmReflector)
			out, err := r.ReflectFull(context.Background(), ReflectorInput{
				Attempt:      1,
				FailingTests: []ReflectorFailedTest{{Suite: "x", AssertionID: "y"}},
			})
			if err != nil {
				t.Fatalf("ReflectFull: %v", err)
			}
			if out == nil {
				t.Fatal("expected non-nil output")
			}
			if got := len(out.PreservationClauses); got != tc.wantLen {
				t.Fatalf("len(PreservationClauses) = %d, want %d (got %#v)", got, tc.wantLen, out.PreservationClauses)
			}
			if tc.wantLen > 0 && out.PreservationClauses[0] != tc.wantFirst {
				t.Errorf("PreservationClauses[0] = %q, want %q", out.PreservationClauses[0], tc.wantFirst)
			}
		})
	}
}

// TestReflectorTool_PreservationClausesInSchema pins the schema-side
// contract: emit_failure_observation MUST advertise the
// preservation_clauses array so the LLM has a typed channel to
// populate. Without the schema declaration, the LLM has no way to
// emit the field and the HIGH-2 typed gate stays dormant.
func TestReflectorTool_PreservationClausesInSchema(t *testing.T) {
	if !strings.Contains(string(reflectorTool.Parameters), `"preservation_clauses"`) {
		t.Errorf("reflectorTool schema must declare preservation_clauses; got:\n%s", string(reflectorTool.Parameters))
	}
	// Ensure observation stays required; preservation_clauses must
	// NOT be required (it's optional and signals "partial-success
	// failure"; mandatory would force the LLM to invent clauses for
	// total-failure cases).
	if !strings.Contains(string(reflectorTool.Parameters), `"required": ["observation"]`) {
		t.Errorf("preservation_clauses must remain OPTIONAL — observation alone is required; schema:\n%s", string(reflectorTool.Parameters))
	}
}

// TestTrimPreservationClauses_Edge pins the trim helper's edge
// behaviour independent of the reflector's parse loop, so future
// refactors can move the helper without losing pin coverage.
func TestTrimPreservationClauses_Edge(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{name: "nil → nil", in: nil, want: nil},
		{name: "empty → nil", in: []string{}, want: nil},
		{name: "all whitespace → nil", in: []string{"  ", "\t\n", ""}, want: nil},
		{name: "trims around content", in: []string{"  keep me  "}, want: []string{"keep me"}},
		{name: "preserves order", in: []string{"a", "b", "c"}, want: []string{"a", "b", "c"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := trimPreservationClauses(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d (got %#v)", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestReflectFull_PatternToolGatedByStore pins commit 40 P1:
// the dispatch tool list includes emit_failure_pattern ONLY
// when the orchestrator wired a FailureTaxonomyStore (signalled
// via SetPatternToolEnabled(true)). Without a destination, the
// schema is omitted to save LLM tokens.
func TestReflectFull_PatternToolGatedByStore(t *testing.T) {
	stub := &reflectorStubAdapter{
		resp: llm.Response{
			ToolCalls: []llm.ToolCall{{Name: "emit_failure_observation",
				Params: json.RawMessage(`{"observation": "ok"}`)}},
		},
	}
	r := NewReflector(stub).(*llmReflector)
	// Default (store-less): only observation tool offered.
	if _, err := r.ReflectFull(context.Background(), ReflectorInput{Attempt: 1, FailingTests: []ReflectorFailedTest{{Suite: "x", AssertionID: "y"}}}); err != nil {
		t.Fatalf("ReflectFull: %v", err)
	}
	if len(stub.lastTools) != 1 {
		t.Fatalf("disabled state: expected 1 tool; got %d (%+v)", len(stub.lastTools), stub.lastTools)
	}
	if stub.lastTools[0].Name != "emit_failure_observation" {
		t.Errorf("disabled state: expected only observation tool; got %q", stub.lastTools[0].Name)
	}

	// After SetPatternToolEnabled(true): both tools.
	r.SetPatternToolEnabled(true)
	stub.lastTools = nil
	if _, err := r.ReflectFull(context.Background(), ReflectorInput{Attempt: 1, FailingTests: []ReflectorFailedTest{{Suite: "x", AssertionID: "y"}}}); err != nil {
		t.Fatalf("ReflectFull: %v", err)
	}
	if len(stub.lastTools) != 2 {
		t.Fatalf("enabled state: expected 2 tools; got %d", len(stub.lastTools))
	}
	hasObs, hasPat := false, false
	for _, tool := range stub.lastTools {
		if tool.Name == "emit_failure_observation" {
			hasObs = true
		}
		if tool.Name == "emit_failure_pattern" {
			hasPat = true
		}
	}
	if !hasObs || !hasPat {
		t.Errorf("enabled state: both tools required; got obs=%v pat=%v", hasObs, hasPat)
	}
}
