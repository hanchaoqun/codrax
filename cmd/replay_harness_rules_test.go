package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The rules a replay must keep hold without a capture, so they are pinned
// here in the default suite: a script is loaded only as pinned, leaving the
// recording is a typed failure, first requests are read out of a log by their
// declared length, and the comparison forgives the named fields and nothing
// else.

func testReplayState(t *testing.T, base string) *replayState {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatal(err)
	}
	// A recorded run directory eight bytes longer than the place the replay
	// may use, so the replay's own directory gets a seven-byte name.
	root := "/" + strings.Repeat("r", len(resolved)+7)
	state := &replayState{Schema: replayStateSchema, Name: "rules", Label: "synthetic", Question: "Where does the program start?"}
	state.Recorded.SessionDir = root + "/.codrax/blob/20260101-000000-000-1"
	state.Recorded.RepoDir = root + "/work/case/run-1.repo"
	state.Recorded.LogDir = root + "/work/case/run-1.logs"
	state.Capture.Env = "CODRAX_REPLAY_RULES_NO_CAPTURE"
	state.Run.Language = "zh"
	state.Run.Branch = "main"
	state.Run.MaxSteps = 15
	state.Run.RepoFixture = "eval/fixtures/stub_repo"
	state.Run.Model = "recorded-model"
	state.Run.ContextWindowTokens = 200000
	state.Script = map[types.AgentName][]replayTurn{}
	return state
}

func testReplayCall(name, params string) replayToolCall {
	return replayToolCall{ID: "call-1", Name: name, ParamsJSON: params, ParamsSHA256: sha256Hex([]byte(params)), ParamsValidJSON: json.Valid([]byte(params)), IDMatchedBy: "exact"}
}

func writeReplayState(t *testing.T, dir string, state *replayState) {
	t.Helper()
	body, err := json.MarshalIndent(state, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	writeReplayStateBytes(t, dir, body)
}

func writeReplayStateBytes(t *testing.T, dir string, body []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "state.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := sha256Hex(body) + "  state.json\n"
	if err := os.WriteFile(filepath.Join(dir, "MANIFEST.sha256"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReplayStateLoadsOnlyTheScriptItsManifestPins(t *testing.T) {
	build := func() *replayState {
		state := testReplayState(t, t.TempDir())
		// The parameters of a recorded call need not be JSON: a model that
		// returns a malformed call is part of the recording.
		state.Script[types.AgentAnalyzer] = []replayTurn{{
			Content: "thinking", ContentBytesLive: 5000, ContentBytesRecorded: 8,
			ToolCalls: []replayToolCall{testReplayCall("emit_analysis", `{"intent":"locate"`)},
		}}
		return state
	}

	t.Run("pinned script loads", func(t *testing.T) {
		dir := t.TempDir()
		writeReplayState(t, dir, build())
		state, err := loadReplayState(dir)
		if err != nil {
			t.Fatalf("a script pinned by its manifest must load: %v", err)
		}
		if got := state.Script[types.AgentAnalyzer][0].ToolCalls[0].ParamsJSON; got != `{"intent":"locate"` {
			t.Fatalf("the recorded parameter bytes must come through unchanged, got %q", got)
		}
	})

	t.Run("edited script", func(t *testing.T) {
		dir := t.TempDir()
		writeReplayState(t, dir, build())
		path := filepath.Join(dir, "state.json")
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		edited := strings.Replace(string(body), "thinking", "thinkinG", 1)
		if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := loadReplayState(dir); err == nil || !strings.Contains(err.Error(), "manifest") {
			t.Fatalf("a script changed after it was pinned must not load, got %v", err)
		}
	})

	t.Run("no manifest", func(t *testing.T) {
		dir := t.TempDir()
		writeReplayState(t, dir, build())
		if err := os.Remove(filepath.Join(dir, "MANIFEST.sha256")); err != nil {
			t.Fatal(err)
		}
		if _, err := loadReplayState(dir); err == nil {
			t.Fatal("a script without a manifest must not load")
		}
	})

	t.Run("edited parameters pinned again", func(t *testing.T) {
		// Pinning the file again does not make edited parameters the
		// recorded ones: each call carries the digest of its own bytes.
		dir := t.TempDir()
		state := build()
		state.Script[types.AgentAnalyzer][0].ToolCalls[0].ParamsJSON = `{"intent":"explain"}`
		writeReplayState(t, dir, state)
		if _, err := loadReplayState(dir); err == nil || !strings.Contains(err.Error(), "recorded parameter bytes") {
			t.Fatalf("a call whose parameters are not the recorded bytes must not load, got %v", err)
		}
	})

	t.Run("text longer than recorded", func(t *testing.T) {
		dir := t.TempDir()
		state := build()
		state.Script[types.AgentAnalyzer][0].Content = "thinking harder"
		writeReplayState(t, dir, state)
		if _, err := loadReplayState(dir); err == nil || !strings.Contains(err.Error(), "bytes of text") {
			t.Fatalf("a turn whose text is not the recorded length must not load, got %v", err)
		}
	})

	t.Run("field the harness does not know", func(t *testing.T) {
		// A script field nothing reads would be an input silently dropped.
		dir := t.TempDir()
		body, err := json.Marshal(build())
		if err != nil {
			t.Fatal(err)
		}
		withExtra := append([]byte(`{"derived_tool_results":[],`), body[1:]...)
		writeReplayStateBytes(t, dir, withExtra)
		if _, err := loadReplayState(dir); err == nil || !strings.Contains(err.Error(), "derived_tool_results") {
			t.Fatalf("a script with a field the harness does not read must not load, got %v", err)
		}
	})

	t.Run("other schema", func(t *testing.T) {
		dir := t.TempDir()
		state := build()
		state.Schema = "codrax-replay-state/v0"
		writeReplayState(t, dir, state)
		if _, err := loadReplayState(dir); err == nil || !strings.Contains(err.Error(), "schema") {
			t.Fatalf("a script of another schema must not load, got %v", err)
		}
	})
}

// replayRules runs a synthetic state, which has no capture, through the real
// composition against the stub repository.
func replayRules(t *testing.T, state *replayState, endAt types.AgentName) replayOutcome {
	t.Helper()
	fixture := filepath.Join(replayRepoRoot(t), state.Run.RepoFixture)
	layout, err := newReplayLayout(t.TempDir(), state, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := seedReplayRepo(fixture, layout.Repo, state.Run.Branch); err != nil {
		t.Fatal(err)
	}
	return runReplay(t, state, replayInputs{Layout: layout, EndAt: endAt})
}

func TestReplayFailsWithATypedDivergenceWhenTheCodeLeavesTheRecording(t *testing.T) {
	t.Run("turn the recording does not have", func(t *testing.T) {
		state := testReplayState(t, t.TempDir())
		out := replayRules(t, state, "")
		d := out.Divergence
		if d == nil {
			t.Fatalf("a model turn without a recording must be a divergence; run error %v", out.RunErr)
		}
		if d.Agent != types.AgentAnalyzer || d.Turn != 0 || d.ExpectedTool != "" || d.Unused != 0 {
			t.Fatalf("divergence must name the analyzer's turn 0, got %+v", d)
		}
		if msg := d.Error(); !strings.Contains(msg, "analyzer") || !strings.Contains(msg, "model turn 0") || !strings.Contains(msg, "rules") {
			t.Fatalf("divergence text must name state, agent and turn, got %q", msg)
		}
		if out.Ended {
			t.Fatal("a diverged replay did not reach the request it was asked to end at")
		}
	})

	t.Run("recorded tool not offered", func(t *testing.T) {
		state := testReplayState(t, t.TempDir())
		state.Script[types.AgentAnalyzer] = []replayTurn{{ToolCalls: []replayToolCall{testReplayCall("tool_of_another_build", `{}`)}}}
		out := replayRules(t, state, "")
		d := out.Divergence
		if d == nil {
			t.Fatalf("a recorded call to a tool the code does not offer must be a divergence; run error %v", out.RunErr)
		}
		if d.Agent != types.AgentAnalyzer || d.Turn != 0 || d.ExpectedTool != "tool_of_another_build" {
			t.Fatalf("divergence must name the analyzer, turn 0 and the expected tool, got %+v", d)
		}
		offered := strings.Join(d.Offered, ",")
		if !strings.Contains(offered, "emit_analysis") {
			t.Fatalf("divergence must list the tools the code offered, got %v", d.Offered)
		}
		if msg := d.Error(); !strings.Contains(msg, "tool_of_another_build") || !strings.Contains(msg, "analyzer turn 0") {
			t.Fatalf("divergence text must name agent, turn and expected tool, got %q", msg)
		}
	})

	t.Run("turn recorded in another dispatch", func(t *testing.T) {
		state := testReplayState(t, t.TempDir())
		state.Script[types.AgentAnalyzer] = []replayTurn{{Dispatch: 1, ToolCalls: []replayToolCall{testReplayCall("emit_analysis", `{}`)}}}
		out := replayRules(t, state, "")
		d := out.Divergence
		if d == nil {
			t.Fatalf("a turn asked for in another dispatch than recorded must be a divergence; run error %v", out.RunErr)
		}
		if d.Agent != types.AgentAnalyzer || d.Turn != 0 || d.RecordedDispatch != 1 || d.ObservedDispatch != 0 {
			t.Fatalf("divergence must name both dispatches, got %+v", d)
		}
		if msg := d.Error(); !strings.Contains(msg, "dispatch 1 in the recording") || !strings.Contains(msg, "in dispatch 0") {
			t.Fatalf("divergence text must name both dispatches, got %q", msg)
		}
	})

	t.Run("replay ends at the agent it is told to", func(t *testing.T) {
		// Ending at the analyzer needs no recorded turn at all, and the log
		// holds the analyzer's first request and nothing of a later agent.
		state := testReplayState(t, t.TempDir())
		out := replayRules(t, state, types.AgentAnalyzer)
		if out.Divergence != nil || !out.Ended {
			t.Fatalf("a replay ending at the analyzer's first request must end there, got divergence %v ended %v", out.Divergence, out.Ended)
		}
		messages, err := firstRequestMessages(out.Log)
		if err != nil {
			t.Fatal(err)
		}
		if len(messages) == 0 {
			t.Fatal("the replay log must hold the analyzer's first request")
		}
		for _, m := range messages {
			if m.Agent != string(types.AgentAnalyzer) || m.Dispatch != 0 {
				t.Fatalf("unexpected first-request message %s", m.key())
			}
		}
		carried := false
		for _, m := range messages {
			carried = carried || strings.Contains(string(m.Body), state.Question)
		}
		if !carried {
			t.Fatal("the analyzer's first request must carry the question of the state")
		}
	})
}

func TestReplayedCallsNameTheReplayRunDirectoryNotTheRecordedOne(t *testing.T) {
	recorded, replay := "/recorded/run/dir", "/replays/r0000001"
	command := `{"command":"cd \"/recorded/run/dir/.codrax/blob/s\" && ls /recorded/run/dir && cat /recorded/run/dir-2/x /recorded/run/dirs"}`
	want := `{"command":"cd \"/replays/r0000001/.codrax/blob/s\" && ls /replays/r0000001 && cat /recorded/run/dir-2/x /recorded/run/dirs"}`
	if got := replayParams(command, recorded, replay); got != want {
		t.Fatalf("the recorded run directory must become the replay's where it is a whole path prefix:\n got %s\nwant %s", got, want)
	}
	if got := replayParams(command, recorded, replay); len(got) != len(command) {
		t.Fatalf("rewriting the run directory must keep the length of the call, %d became %d", len(command), len(got))
	}
	// Without a directory of the same length there is no faithful rewrite.
	if got := replayParams(command, recorded, "/short"); got != command {
		t.Fatalf("a run directory of another length must leave the call alone, got %s", got)
	}

	// The adapter hands the rewritten call to the code.
	state := testReplayState(t, t.TempDir())
	state.Script[types.AgentExplorer] = []replayTurn{{ToolCalls: []replayToolCall{testReplayCall("exec_command", command)}}}
	run := newReplayRun(state, "")
	run.recordedRoot, run.replayRoot = recorded, replay
	adapter := &replayAdapter{run: run, agent: types.AgentExplorer}
	resp, err := adapter.Chat(context.Background(), []llm.Message{{Role: "user", Content: "q"}}, []llm.ToolSchema{{Name: "exec_command"}}, llm.ChatOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.ToolCalls) != 1 || string(resp.ToolCalls[0].Params) != want {
		t.Fatalf("the adapter must hand the code the call under the replay's run directory, got %+v", resp.ToolCalls)
	}
}

func TestReplayClosesAReasoningBlockTheLogCutAndInventsNothing(t *testing.T) {
	cases := []struct {
		name         string
		turn         replayTurn
		want         string
		wantRepaired bool
	}{
		{"whole text", replayTurn{Content: "<think>plan</think>\n\nanswer", ContentBytesLive: 27, ContentBytesRecorded: 27}, "<think>plan</think>\n\nanswer", false},
		{"whole text with an open block", replayTurn{Content: "<think>plan", ContentBytesLive: 11, ContentBytesRecorded: 11}, "<think>plan", false},
		{"cut inside the block", replayTurn{Content: "<think>plan", ContentBytesLive: 5000, ContentBytesRecorded: 11}, "<think>plan</think>", true},
		{"cut inside a second block", replayTurn{Content: "<think>a</think>b<THINK>c", ContentBytesLive: 5000, ContentBytesRecorded: 25}, "<think>a</think>b<THINK>c</think>", true},
		{"cut after the block", replayTurn{Content: "<think>plan</think>\n\nansw", ContentBytesLive: 5000, ContentBytesRecorded: 25}, "<think>plan</think>\n\nansw", false},
		{"cut text without a block", replayTurn{Content: "answ", ContentBytesLive: 5000, ContentBytesRecorded: 4}, "answ", false},
	}
	for _, tc := range cases {
		got, repaired := replayModelText(tc.turn)
		if got != tc.want || repaired != tc.wantRepaired {
			t.Fatalf("%s: got %q closed=%v, want %q closed=%v", tc.name, got, repaired, tc.want, tc.wantRepaired)
		}
	}

	// The adapter hands the code the closed text and reports the turn.
	state := testReplayState(t, t.TempDir())
	state.Script[types.AgentExplorer] = []replayTurn{
		{Content: "<think>plan", ContentBytesLive: 5000, ContentBytesRecorded: 11, FinishReason: "tool_use"},
		{Iter: 1, Content: "<think>next</think>", ContentBytesLive: 19, ContentBytesRecorded: 19},
	}
	run := newReplayRun(state, "")
	adapter := &replayAdapter{run: run, agent: types.AgentExplorer}
	first, err := adapter.Chat(context.Background(), []llm.Message{{Role: "user", Content: "q"}}, nil, llm.ChatOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if first.Content != "<think>plan</think>" || first.StopReason != "tool_use" {
		t.Fatalf("the adapter must return the recorded turn with its reasoning block closed and its stop reason, got %+v", first)
	}
	if _, err := adapter.Chat(context.Background(), []llm.Message{{Role: "user", Content: "q"}, {Role: "assistant", Content: first.Content}}, nil, llm.ChatOptions{}); err != nil {
		t.Fatal(err)
	}
	want := []replayCutText{{Agent: "explorer", Turn: 0, LiveBytes: 5000, RecordedBytes: 11, ReasoningClosed: true}}
	if len(run.cutTexts) != 1 || run.cutTexts[0] != want[0] {
		t.Fatalf("the replay must report exactly the turns whose text the log cut, got %+v", run.cutTexts)
	}
}

func TestReplayFollowsTheRecordedDispatchesOfAnAgent(t *testing.T) {
	state := testReplayState(t, t.TempDir())
	state.Script[types.AgentAnalyzer] = []replayTurn{
		{Dispatch: 0, Content: "first"},
		{Dispatch: 0, Iter: 1, Content: "second"},
		{Dispatch: 1, Content: "third"},
	}
	// A request without an assistant message opens a dispatch; one that
	// carries the model's earlier turn continues it.
	opening := []llm.Message{{Role: "system", Content: "s"}, {Role: "user", Content: "q"}}
	continuing := append(append([]llm.Message(nil), opening...), llm.Message{Role: "assistant", Content: "first"})
	ask := func(adapter *replayAdapter, messages []llm.Message) (string, *replayDivergence) {
		resp, err := adapter.Chat(context.Background(), messages, nil, llm.ChatOptions{})
		var d *replayDivergence
		if err != nil && !errors.As(err, &d) {
			t.Fatalf("leaving the recording must be a replayDivergence, got %v", err)
		}
		return resp.Content, d
	}
	fresh := func() *replayAdapter {
		return &replayAdapter{run: newReplayRun(state, ""), agent: types.AgentAnalyzer}
	}

	adapter := fresh()
	for i, step := range []struct {
		messages []llm.Message
		want     string
	}{{opening, "first"}, {continuing, "second"}, {opening, "third"}} {
		if got, d := ask(adapter, step.messages); d != nil || got != step.want {
			t.Fatalf("request %d: got %q divergence %v, want the recorded turn %q", i, got, d, step.want)
		}
	}

	adapter = fresh()
	ask(adapter, opening)
	ask(adapter, continuing)
	if _, d := ask(adapter, continuing); d == nil || d.Agent != types.AgentAnalyzer || d.Turn != 2 || d.RecordedDispatch != 1 || d.ObservedDispatch != 0 {
		t.Fatalf("continuing a dispatch where the recording opens the next must be a divergence at turn 2, got %+v", d)
	}
	if adapter.run.divergence == nil {
		t.Fatal("the run must keep its first divergence")
	}

	adapter = fresh()
	ask(adapter, opening)
	if _, d := ask(adapter, opening); d == nil || d.Turn != 1 || d.RecordedDispatch != 0 || d.ObservedDispatch != 1 {
		t.Fatalf("opening a dispatch where the recording continues one must be a divergence at turn 1, got %+v", d)
	}
}

func TestReplayReportsRecordedTurnsTheCodeNeverAskedFor(t *testing.T) {
	state := testReplayState(t, t.TempDir())
	state.Script[types.AgentAnalyzer] = make([]replayTurn, 2)
	state.Script[types.AgentExplorer] = make([]replayTurn, 5)

	if d := replayUnusedTurns(state, map[types.AgentName]int{types.AgentAnalyzer: 2, types.AgentExplorer: 5}, types.AgentFinalizer); d != nil {
		t.Fatalf("a replay that used every recorded turn did not diverge, got %v", d)
	}
	d := replayUnusedTurns(state, map[types.AgentName]int{types.AgentAnalyzer: 2, types.AgentExplorer: 3}, types.AgentFinalizer)
	if d == nil || d.Agent != types.AgentExplorer || d.Turn != 3 || d.Unused != 2 {
		t.Fatalf("turns left over must be reported for the explorer at turn 3 with 2 unused, got %+v", d)
	}
	if msg := d.Error(); !strings.Contains(msg, "explorer stopped asking after 3 model turn(s)") || !strings.Contains(msg, "2 more") {
		t.Fatalf("divergence text must say how far the agent got, got %q", msg)
	}
	d = replayUnusedTurns(state, map[types.AgentName]int{types.AgentAnalyzer: 1}, types.AgentExplorer)
	if d == nil || d.Agent != types.AgentAnalyzer || d.Unused != 1 {
		t.Fatalf("the analyzer's unused turn must be reported when the replay ends at the explorer, got %+v", d)
	}
	// The agent the replay ends at is stopped at its first request by the
	// replay itself, so its recorded turns are not owed.
	if d := replayUnusedTurns(state, map[types.AgentName]int{types.AgentAnalyzer: 2}, types.AgentExplorer); d != nil {
		t.Fatalf("turns of the agent the replay ends at are not owed, got %v", d)
	}
	if d := replayUnusedTurns(state, map[types.AgentName]int{}, types.AgentAnalyzer); d != nil {
		t.Fatalf("turns of the agent the replay ends at are not owed, got %v", d)
	}
}

func TestReplayLayoutKeepsTheRecordedPathLengthsAndNames(t *testing.T) {
	base := t.TempDir()
	state := testReplayState(t, base)
	layout, err := newReplayLayout(base, state, "")
	if err != nil {
		t.Fatal(err)
	}
	recordedRoot := strings.TrimSuffix(state.Recorded.SessionDir, "/.codrax/blob/20260101-000000-000-1")
	if layout.RecordedRoot != recordedRoot {
		t.Fatalf("recorded run directory %q, want %q", layout.RecordedRoot, recordedRoot)
	}
	for _, pair := range [][2]string{{layout.Root, recordedRoot}, {layout.SessionDir, state.Recorded.SessionDir}, {layout.Repo, state.Recorded.RepoDir}, {layout.LogDir, state.Recorded.LogDir}} {
		if len(pair[0]) != len(pair[1]) {
			t.Fatalf("replay path %q must be as long as the recorded %q", pair[0], pair[1])
		}
		if strings.TrimPrefix(pair[0], layout.Root) != strings.TrimPrefix(pair[1], recordedRoot) {
			t.Fatalf("replay path %q must keep every name of %q below the run directory", pair[0], pair[1])
		}
		if info, err := os.Stat(pair[0]); err != nil || !info.IsDir() {
			t.Fatalf("replay directory %q must exist: %v", pair[0], err)
		}
	}
	if layout.Anchor != filepath.Join(layout.Root, ".codrax") {
		t.Fatalf("the runtime directory must be the run directory's .codrax, got %q", layout.Anchor)
	}

	// Two replays under one base never share a run directory.
	second, err := newReplayLayout(base, state, "")
	if err != nil {
		t.Fatal(err)
	}
	if second.Root == layout.Root || len(second.Root) != len(layout.Root) {
		t.Fatalf("a second replay must get its own run directory of the same length, got %q and %q", layout.Root, second.Root)
	}

	// A caller may pin the name, which decides digests computed over the
	// run directory; it must have exactly the needed length and be new.
	pinned, err := newReplayLayout(base, state, "pinned1")
	if err != nil || filepath.Base(pinned.Root) != "pinned1" || len(pinned.Root) != len(layout.Root) {
		t.Fatalf("a pinned name of the needed length must be used as given, got %q, %v", pinned.Root, err)
	}
	if _, err := newReplayLayout(base, state, "pinned1"); err == nil {
		t.Fatal("a pinned run directory that exists must be refused, not shared")
	}
	for _, bad := range []string{"short", "much-too-long", "a/b/c/d"} {
		if _, err := newReplayLayout(base, state, bad); err == nil || !strings.Contains(err.Error(), "exactly 7 bytes") {
			t.Fatalf("a pinned name %q that would change the path length must be refused, got %v", bad, err)
		}
	}

	// A base too long to fit a run directory of the recorded length is
	// refused: padding or truncating a path would move bytes in a request.
	long := filepath.Join(base, "a-directory-name-longer-than-the-room-left")
	if err := os.Mkdir(long, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := newReplayLayout(long, state, ""); err == nil || !strings.Contains(err.Error(), "too long") {
		t.Fatalf("a base that leaves no room for a run directory of the recorded length must be refused, got %v", err)
	}

	outside := testReplayState(t, base)
	outside.Recorded.RepoDir = "/elsewhere/run-1.repo"
	if _, err := newReplayLayout(base, outside, ""); err == nil || !strings.Contains(err.Error(), "does not lie under") {
		t.Fatalf("a recorded repository outside the recorded run directory must be refused, got %v", err)
	}
	odd := testReplayState(t, base)
	odd.Recorded.SessionDir = recordedRoot + "/sessions/20260101-000000-000-1"
	if _, err := newReplayLayout(base, odd, ""); err == nil {
		t.Fatal("a recorded session directory of another shape must be refused")
	}
}

func TestReplayClockPassesTheBudgetAtTheRecordedUnitAndStaysPastIt(t *testing.T) {
	never := replayClock(nil)
	for _, at := range []agent.DynamicContextCheckpoint{{Unit: "a"}, {Unit: "b", Deferred: true}} {
		if got := never(at); got != 0 {
			t.Fatalf("a dispatch recorded within its budget must be replayed with a clock at zero, got %v at %+v", got, at)
		}
	}

	clock := replayClock(&replayDynamicContextCut{Unit: "narrative", Deferred: true})
	steps := []struct {
		at   agent.DynamicContextCheckpoint
		past bool
	}{
		{agent.DynamicContextCheckpoint{Unit: "ledger"}, false},
		// The same unit name in the first pass is another position.
		{agent.DynamicContextCheckpoint{Unit: "narrative"}, false},
		{agent.DynamicContextCheckpoint{Unit: "ledger", Deferred: true}, false},
		{agent.DynamicContextCheckpoint{Unit: "narrative", Deferred: true}, true},
		{agent.DynamicContextCheckpoint{Unit: "later"}, true},
	}
	for i, step := range steps {
		got := clock(step.at)
		if past := got > time.Hour; past != step.past {
			t.Fatalf("step %d at %+v: elapsed %v, want past the budget = %v", i, step.at, got, step.past)
		}
	}
}

func TestFirstRequestMessagesAreReadByTheirDeclaredLength(t *testing.T) {
	line := func(format string, args ...any) string {
		return "2026-01-01T00:00:00.000Z DEBUG " + fmt.Sprintf(format, args...) + "\n"
	}
	message := func(agentName, role, body string) string {
		return line("[diag %s] INIT msg role=%s len=%d", agentName, role, len(body)) + body + "\n---\n"
	}
	// A body may itself hold what looks like a dispatch, a message header and
	// a message end.
	tricky := "before\n" + line("[diag analyzer] DISPATCH stage=analyze attempt=9") +
		message("explorer", "user", "inner") + "after"
	log := line("[cmd] start") +
		line("[diag analyzer] DISPATCH stage=analyze attempt=1") +
		message("analyzer", "system", "S1") +
		message("analyzer", "user", tricky) +
		line("[diag analyzer] iter=0 ASSISTANT content_len=0 tool_calls=1") +
		line("[diag analyzer] DISPATCH stage=analyze attempt=2") +
		message("analyzer", "system", "S2") +
		line("[diag explorer] DISPATCH stage=explore attempt=1") +
		message("explorer", "system", "多字节正文")

	got, err := firstRequestMessages([]byte(log))
	if err != nil {
		t.Fatal(err)
	}
	want := []struct{ key, role, body string }{
		{"analyzer.d0.m0", "system", "S1"},
		{"analyzer.d0.m1", "user", tricky},
		{"analyzer.d1.m0", "system", "S2"},
		{"explorer.d0.m0", "system", "多字节正文"},
	}
	if len(got) != len(want) {
		t.Fatalf("read %d messages, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].key() != w.key || got[i].Role != w.role || string(got[i].Body) != w.body {
			t.Fatalf("message %d: got %s %s %q, want %s %s %q", i, got[i].key(), got[i].Role, got[i].Body, w.key, w.role, w.body)
		}
	}

	// A log cut inside a message is not read as a shorter message.
	cut := line("[diag analyzer] DISPATCH stage=analyze attempt=1") + line("[diag analyzer] INIT msg role=user len=50") + "short\n---\n"
	if _, err := firstRequestMessages([]byte(cut)); err == nil {
		t.Fatal("a message that does not end where its length says must be an error")
	}
}

func TestFirstRequestComparisonForgivesTheNamedFieldsAndNothingElse(t *testing.T) {
	side := replayMaskSide{RecordedRoot: "/recorded/run/dir", ReplayRoot: "/replays/r0000001"}
	if len(side.RecordedRoot) != len(side.ReplayRoot) {
		t.Fatal("test roots must have one length")
	}
	hexA, hexB := strings.Repeat("a1", 32), strings.Repeat("b2", 32)
	body := func(root, digest, stamp, tail string) firstRequestMessage {
		text := "cwd " + root + "/.codrax/blob/s/trace-query-result-" + digest + ".json\n" +
			"id trace_query:publication-" + digest + "\n" +
			"wake runtime_instance_" + digest + " node rt_" + digest + " lane rt_thread_" + digest + "\n" +
			"pick candidate-" + digest[:32] + "\n" +
			"clip trace_query-" + digest[:10] + "...[truncated 54 bytes]\n" +
			`<fact observed_at="` + stamp + `">` + tail
		return firstRequestMessage{Agent: "finalizer", Role: "user", Body: []byte(text)}
	}
	masks := replayMasks()
	recorded := body(side.RecordedRoot, hexA, "2026-11-25T14:51:47.413942-08:00", "same")

	// Every fixed-length mask leaves the length of the text alone.
	for _, mask := range masks {
		masked, fields, _ := mask.Apply(recorded.Body, side)
		if fields == 0 {
			t.Fatalf("mask %s must cover its field in the sample", mask.Name)
		}
		if !mask.VariableLength && len(masked) != len(recorded.Body) {
			t.Fatalf("mask %s is declared fixed-length and changed the length from %d to %d", mask.Name, len(recorded.Body), len(masked))
		}
		if mask.Why == "" {
			t.Fatalf("mask %s must say why it is allowed", mask.Name)
		}
	}

	same := body(side.ReplayRoot, hexB, "2026-11-25T14:51:47.413942-08:00", "same")
	if cmp, _, _ := compareFirstRequest(recorded, same, masks, side); !cmp.identical() || cmp.DiffBytes != 0 || cmp.FirstDiff != -1 {
		t.Fatalf("messages that differ only in the named fields must compare identical, got %+v", cmp)
	}

	// The time stamp drops trailing zeros, so its length varies; the
	// comparison accounts for exactly that many bytes.
	shorter := body(side.ReplayRoot, hexB, "2026-11-25T14:51:47.41394-08:00", "same")
	cmp, _, _ := compareFirstRequest(recorded, shorter, masks, side)
	if !cmp.identical() {
		t.Fatalf("a time stamp one digit shorter is the same message, got %+v", cmp)
	}
	if cmp.RecordedBytes-cmp.ReplayedBytes != 1 || cmp.VariableLengthBytes[0]-cmp.VariableLengthBytes[1] != 1 {
		t.Fatalf("the one byte must be accounted to the time stamp, got %+v", cmp)
	}

	for name, other := range map[string]firstRequestMessage{
		"one byte of content":            body(side.ReplayRoot, hexB, "2026-11-25T14:51:47.413942-08:00", "samE"),
		"one byte more content":          body(side.ReplayRoot, hexB, "2026-11-25T14:51:47.413942-08:00", "same!"),
		"a directory below the root":     body(side.ReplayRoot+"/x", hexB, "2026-11-25T14:51:47.413942-08:00", "same"),
		"a digest of another length":     body(side.ReplayRoot, hexB[:62], "2026-11-25T14:51:47.413942-08:00", "same"),
		"a digest that is not one":       body(side.ReplayRoot, strings.Repeat("zz", 32), "2026-11-25T14:51:47.413942-08:00", "same"),
		"another root than the replay's": body("/somewhere/r0000001", hexB, "2026-11-25T14:51:47.413942-08:00", "same"),
	} {
		if cmp, _, _ := compareFirstRequest(recorded, other, masks, side); cmp.identical() || cmp.FirstDiff < 0 {
			t.Fatalf("%s must be a difference, got %+v", name, cmp)
		}
	}

	// A length difference the time stamps do not account for is a
	// difference even when the masked texts agree.
	lost := body(side.ReplayRoot, hexB, "2026-11-25T14:51:47.413942-08:00", "same")
	cmp, _, _ = compareFirstRequest(recorded, lost, masks, side)
	cmp.ReplayedBytes--
	if cmp.identical() {
		t.Fatal("an unaccounted byte must not compare identical")
	}

	other := body(side.ReplayRoot, hexB, "2026-11-25T14:51:47.413942-08:00", "same")
	other.Role = "system"
	if cmp, _, _ := compareFirstRequest(recorded, other, masks, side); cmp.identical() {
		t.Fatal("the same text in another role is another message")
	}
}
