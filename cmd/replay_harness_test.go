package cmd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/mcp"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/repl"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// A replay runs the read pipeline a command-line run uses, with every model
// turn supplied from a recording instead of a provider. It answers one
// question without a model: given what the models of a recorded run said,
// what does the code under test ask next?
//
// A state script holds what the run was given and what its models said. It
// holds nothing the code derives from the capture: tool results, prompts and
// answers are produced again by the code under test, which is the point.
//
// A replay never bends to make a script fit. When the code asks for a turn
// the recording does not have, offers a tool set without the recorded call,
// or stops asking while recorded turns remain, the run reports a
// replayDivergence naming the agent, the turn and the expected tool. A script
// is rebuilt from its sealed log, never edited.

const replayStateSchema = "codrax-replay-state/v1"

type replayState struct {
	Schema   string `json:"schema"`
	Name     string `json:"name"`
	Label    string `json:"label"`
	Recorded struct {
		Build string `json:"build"`
		Log   struct {
			Path   string `json:"path"`
			SHA256 string `json:"sha256"`
		} `json:"log"`
		// SessionDir, RepoDir and LogDir are the directories of the recorded
		// process. A replay mirrors them under a root of the same length.
		SessionDir     string `json:"session_dir"`
		RepoDir        string `json:"repo_dir"`
		LogDir         string `json:"log_dir"`
		ParameterFiles struct {
			Dir   string `json:"dir"`
			Files []struct {
				File   string `json:"file"`
				SHA256 string `json:"sha256"`
			} `json:"files"`
		} `json:"parameter_files"`
		AmbiguousCallIDs []string `json:"ambiguous_call_ids"`
	} `json:"recorded"`
	Question string `json:"question"`
	Capture  struct {
		Env    string `json:"env"`
		SHA256 string `json:"sha256"`
		Bytes  int64  `json:"bytes"`
	} `json:"capture"`
	Run struct {
		Language            string                                `json:"language"`
		Branch              string                                `json:"branch"`
		MaxSteps            int                                   `json:"max_steps"`
		RepoFixture         string                                `json:"repo_fixture"`
		Model               string                                `json:"model"`
		ContextWindowTokens int                                   `json:"context_window_tokens"`
		MaxOutputTokens     int                                   `json:"max_output_tokens"`
		ThinkAloud          map[types.AgentName]bool              `json:"think_aloud"`
		ToolParamCompat     map[types.AgentName]replayParamCompat `json:"tool_param_compat"`
		TurnPolicy          replayTurnPolicy                      `json:"turn_policy"`
		// UnrecordedInputs lists what the run was given that its log does not
		// print, with the value the script assumes and what that rests on.
		UnrecordedInputs []struct {
			Field string `json:"field"`
			Value string `json:"value"`
			Basis string `json:"basis"`
		} `json:"unrecorded_inputs"`
	} `json:"run"`
	DynamicContextCut    *replayDynamicContextCut `json:"dynamic_context_cut"`
	RecordedFirstRequest struct {
		Receipt map[string]any `json:"receipt"`
	} `json:"recorded_first_request"`
	Script map[types.AgentName][]replayTurn `json:"script"`
}

type replayParamCompat struct {
	Mode              string `json:"mode"`
	SplitStringArrays bool   `json:"split_string_arrays"`
}

type replayTurnPolicy struct {
	RequiredOutcomes          types.TurnOutcomeSet `json:"required_outcomes"`
	Route                     string               `json:"route"`
	Operation                 string               `json:"operation"`
	OperationKind             string               `json:"operation_kind"`
	DataTaskKind              string               `json:"data_task_kind"`
	NeedsDataAccess           bool                 `json:"needs_data_access"`
	NeedsRepoAccess           bool                 `json:"needs_repo_access"`
	CurrentSourceEvidenceMode string               `json:"current_source_evidence_mode"`
	NeedsOperationAccess      bool                 `json:"needs_operation_access"`
	RiskLevel                 string               `json:"risk_level"`
	Confidence                float64              `json:"confidence"`
	Source                    string               `json:"source"`
	TargetSurface             string               `json:"target_surface"`
	SideEffects               []string             `json:"side_effects"`
	RequiresDiagram           bool                 `json:"requires_diagram"`
	PresentationDirective     string               `json:"presentation_directive"`
	Reason                    string               `json:"reason"`
	DeliveryOwner             string               `json:"delivery_owner"`
	WriteIntent               string               `json:"write_intent"`
}

// replayDynamicContextCut is the position at which the recorded finalizer
// dispatch ran out of its dynamic-context budget. Evidence says how the
// position is known from the sealed log.
type replayDynamicContextCut struct {
	Unit     string `json:"unit"`
	Deferred bool   `json:"deferred"`
	Evidence string `json:"evidence"`
}

type replayTurn struct {
	Dispatch     int    `json:"dispatch"`
	Iter         int    `json:"iter"`
	FinishReason string `json:"finish_reason"`
	Content      string `json:"content"`
	// ContentBytesLive is the size of the text the model returned. The sealed
	// log keeps at most 4000 bytes of it; ContentBytesRecorded is what the
	// script therefore holds.
	ContentBytesLive     int              `json:"content_bytes_live"`
	ContentBytesRecorded int              `json:"content_bytes_recorded"`
	ToolCalls            []replayToolCall `json:"tool_calls"`
}

// replayToolCall carries the parameter bytes exactly as the model returned
// them, which need not be JSON: a run records a malformed call and refuses
// it, and so must its replay.
type replayToolCall struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	ParamsJSON      string `json:"params_json"`
	ParamsSHA256    string `json:"params_sha256"`
	ParamsValidJSON bool   `json:"params_valid_json"`
	IDMatchedBy     string `json:"id_matched_by"`
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func sha256File(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// loadReplayState reads <dir>/state.json. The manifest beside it pins the
// script's bytes, so a script edited by hand no longer loads; every recorded
// tool call's parameters are pinned a second time inside the script.
func loadReplayState(dir string) (*replayState, error) {
	body, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil {
		return nil, fmt.Errorf("replay state %s: %w", dir, err)
	}
	manifest, err := os.ReadFile(filepath.Join(dir, "MANIFEST.sha256"))
	if err != nil {
		return nil, fmt.Errorf("replay state %s: %w", dir, err)
	}
	want := strings.Fields(string(manifest))
	if len(want) != 2 || want[1] != "state.json" {
		return nil, fmt.Errorf("replay state %s: manifest does not pin state.json", dir)
	}
	if got := sha256Hex(body); got != want[0] {
		return nil, fmt.Errorf("replay state %s: state.json is not the script its manifest pins (sha256 %s, manifest %s); rebuild the script from its sealed log", dir, got, want[0])
	}
	var state replayState
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&state); err != nil {
		return nil, fmt.Errorf("replay state %s: %w", dir, err)
	}
	if state.Schema != replayStateSchema {
		return nil, fmt.Errorf("replay state %s: schema %q, want %q", dir, state.Schema, replayStateSchema)
	}
	if strings.TrimSpace(state.Question) == "" || state.Capture.Env == "" || state.Run.ContextWindowTokens <= 0 {
		return nil, fmt.Errorf("replay state %s: question, capture variable and recorded context window are required", dir)
	}
	for name, turns := range state.Script {
		for i, turn := range turns {
			if len(turn.Content) != turn.ContentBytesRecorded || turn.ContentBytesRecorded > turn.ContentBytesLive {
				return nil, fmt.Errorf("replay state %s: %s turn %d holds %d bytes of text, recorded %d of %d", dir, name, i, len(turn.Content), turn.ContentBytesRecorded, turn.ContentBytesLive)
			}
			for _, call := range turn.ToolCalls {
				if call.Name == "" || sha256Hex([]byte(call.ParamsJSON)) != call.ParamsSHA256 {
					return nil, fmt.Errorf("replay state %s: %s turn %d call %s (%s) does not carry the recorded parameter bytes", dir, name, i, call.ID, call.Name)
				}
			}
		}
	}
	return &state, nil
}

// replayDivergence reports that the code under test left the recording.
// Turn is the zero-based model turn of Agent at which it happened.
// ExpectedTool is the tool the recording calls at that turn when the code did
// not offer it. Unused counts recorded turns the code never asked for.
type replayDivergence struct {
	State        string
	Agent        types.AgentName
	Turn         int
	ExpectedTool string
	Offered      []string
	Unused       int
	// RecordedDispatch and ObservedDispatch differ when the code starts or
	// continues a dispatch where the recording does the other.
	RecordedDispatch int
	ObservedDispatch int
}

func (e *replayDivergence) Error() string {
	switch {
	case e.Unused > 0:
		return fmt.Sprintf("replay of %s diverged: %s stopped asking after %d model turn(s) and the recording has %d more", e.State, e.Agent, e.Turn, e.Unused)
	case e.ExpectedTool != "":
		return fmt.Sprintf("replay of %s diverged: %s turn %d was answered with a call to %s in the recording, and the code offers only %v", e.State, e.Agent, e.Turn, e.ExpectedTool, e.Offered)
	case e.RecordedDispatch != e.ObservedDispatch:
		return fmt.Sprintf("replay of %s diverged: %s turn %d belongs to dispatch %d in the recording, and the code asks for it in dispatch %d", e.State, e.Agent, e.Turn, e.RecordedDispatch, e.ObservedDispatch)
	default:
		return fmt.Sprintf("replay of %s diverged: %s asked for model turn %d and the recording has no such turn", e.State, e.Agent, e.Turn)
	}
}

// errReplayEnded is returned to the agent a replay ends at.
var errReplayEnded = errors.New("replay ended where it was asked to")

// replayRun is one replay in progress: the script cursors, the first
// divergence, and what the finalizer's first dispatch did.
type replayRun struct {
	state *replayState
	// endAt is the agent whose first request ends the replay. For the
	// finalizer the replay ends when its first dispatch returns.
	endAt types.AgentName
	// recordedRoot and replayRoot are the run directory of the recording and
	// of this replay; see replayParams.
	recordedRoot string
	replayRoot   string

	mu         sync.Mutex
	next       map[types.AgentName]int
	dispatch   map[types.AgentName]int
	divergence *replayDivergence
	cancel     func(reason string)
	ended      bool
	cutTexts   []replayCutText

	finalizerDispatches   int
	finalizerRequestBytes int
	finalizerToolBytes    int
	finalizerRefusal      *agent.AnswerContextByteBudgetError
	finalizerErr          error
}

func newReplayRun(state *replayState, endAt types.AgentName) *replayRun {
	if endAt == "" {
		endAt = types.AgentFinalizer
	}
	return &replayRun{state: state, endAt: endAt, next: map[types.AgentName]int{}, dispatch: map[types.AgentName]int{}, cancel: func(string) {}}
}

func (r *replayRun) diverge(d *replayDivergence) error {
	r.mu.Lock()
	if r.divergence == nil {
		r.divergence = d
	}
	cancel := r.cancel
	r.mu.Unlock()
	cancel(d.Error())
	return d
}

func (r *replayRun) end() error {
	r.mu.Lock()
	r.ended = true
	cancel := r.cancel
	r.mu.Unlock()
	cancel(errReplayEnded.Error())
	return errReplayEnded
}

// replayAdapter is one agent's model in a replay. It declares the recorded
// model and sizing, so every budget derived from an adapter is the recorded
// one, and answers each request with the next recorded turn of its agent.
type replayAdapter struct {
	run   *replayRun
	agent types.AgentName
}

func (a *replayAdapter) ModelID() string               { return a.run.state.Run.Model }
func (a *replayAdapter) MaxContextTokens() int         { return a.run.state.Run.ContextWindowTokens }
func (a *replayAdapter) MaxOutputTokens() int          { return a.run.state.Run.MaxOutputTokens }
func (a *replayAdapter) RequestTimeout() time.Duration { return 10 * time.Minute }
func (a *replayAdapter) RetryMaxAttempts() int         { return 1 }

func (a *replayAdapter) Chat(_ context.Context, messages []llm.Message, tools []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	run := a.run
	if a.agent == types.AgentFinalizer {
		// The replay is about the request, not the answer: size it and stop.
		run.mu.Lock()
		if run.finalizerRequestBytes == 0 {
			run.finalizerRequestBytes = llm.EstimateRequestBytes(messages, tools)
			run.finalizerToolBytes = llm.EstimateRequestBytes(nil, tools)
		}
		run.mu.Unlock()
		return llm.Response{}, run.end()
	}
	if a.agent == run.endAt {
		return llm.Response{}, run.end()
	}

	// A request that carries no assistant message opens a dispatch.
	opens := true
	for _, m := range messages {
		if m.Role == "assistant" {
			opens = false
			break
		}
	}
	run.mu.Lock()
	turnIndex := run.next[a.agent]
	run.next[a.agent] = turnIndex + 1
	if opens && turnIndex > 0 {
		run.dispatch[a.agent]++
	}
	observedDispatch := run.dispatch[a.agent]
	run.mu.Unlock()

	script := run.state.Script[a.agent]
	if turnIndex >= len(script) {
		return llm.Response{}, run.diverge(&replayDivergence{State: run.state.Name, Agent: a.agent, Turn: turnIndex})
	}
	turn := script[turnIndex]
	if turn.Dispatch != observedDispatch {
		return llm.Response{}, run.diverge(&replayDivergence{State: run.state.Name, Agent: a.agent, Turn: turnIndex, RecordedDispatch: turn.Dispatch, ObservedDispatch: observedDispatch})
	}
	offered := make([]string, 0, len(tools))
	for _, schema := range tools {
		offered = append(offered, schema.Name)
	}
	sort.Strings(offered)
	text, closed := replayModelText(turn)
	if turn.ContentBytesRecorded < turn.ContentBytesLive {
		run.mu.Lock()
		run.cutTexts = append(run.cutTexts, replayCutText{Agent: string(a.agent), Turn: turnIndex, LiveBytes: turn.ContentBytesLive, RecordedBytes: turn.ContentBytesRecorded, ReasoningClosed: closed})
		run.mu.Unlock()
	}
	resp := llm.Response{Content: text, StopReason: turn.FinishReason}
	for _, call := range turn.ToolCalls {
		if i := sort.SearchStrings(offered, call.Name); i >= len(offered) || offered[i] != call.Name {
			return llm.Response{}, run.diverge(&replayDivergence{State: run.state.Name, Agent: a.agent, Turn: turnIndex, ExpectedTool: call.Name, Offered: offered})
		}
		resp.ToolCalls = append(resp.ToolCalls, llm.ToolCall{ID: call.ID, Name: call.Name, Params: json.RawMessage(replayParams(call.ParamsJSON, run.recordedRoot, run.replayRoot))})
	}
	return resp, nil
}

// replayModelText returns the text of a recorded turn as the replay hands it
// to the code, and whether the replay had to close a reasoning block.
//
// The debug log keeps at most 4000 bytes of a model's text, so a longer text
// is recorded without its end. The code removes <think>…</think> blocks from
// model text before it reaches a later prompt and leaves an unclosed block in
// place; a text cut inside its reasoning block would therefore put reasoning
// into prompts that the recorded run kept it out of. Such a text is replayed
// with the block closed. Whatever followed the block in the live text was not
// recorded and is not invented.
func replayModelText(turn replayTurn) (string, bool) {
	if turn.ContentBytesRecorded >= turn.ContentBytesLive {
		return turn.Content, false
	}
	lower := strings.ToLower(turn.Content)
	if strings.LastIndex(lower, "<think>") > strings.LastIndex(lower, "</think>") {
		return turn.Content + "</think>", true
	}
	return turn.Content, false
}

// replayCutText names one recorded turn whose text the log cut.
type replayCutText struct {
	Agent         string `json:"agent"`
	Turn          int    `json:"turn"`
	LiveBytes     int    `json:"live_bytes"`
	RecordedBytes int    `json:"recorded_bytes"`
	// ReasoningClosed is true when the cut fell inside a reasoning block and
	// the replay closed it.
	ReasoningClosed bool `json:"reasoning_block_closed"`
}

// replayParams returns the parameters of a recorded tool call as the replay
// hands them to the code. A recorded model names files by the paths its run
// showed it, which lie under the recorded run directory; the replay shows the
// same names under its own run directory, of the same length. The recorded
// directory is therefore rewritten to the replay's wherever it stands as a
// whole path prefix, so a replayed call acts on the replay's files and never
// on the directory of the recorded process. Nothing else in a recorded call
// is touched.
func replayParams(params, recordedRoot, replayRoot string) string {
	if recordedRoot == "" || replayRoot == "" || len(recordedRoot) != len(replayRoot) {
		return params
	}
	var out strings.Builder
	for {
		i := strings.Index(params, recordedRoot)
		if i < 0 {
			break
		}
		end := i + len(recordedRoot)
		out.WriteString(params[:i])
		// "/a/run" is not a prefix of the path "/a/run-2/file".
		if end < len(params) && replayPathNameByte(params[end]) {
			out.WriteString(recordedRoot)
		} else {
			out.WriteString(replayRoot)
		}
		params = params[end:]
	}
	out.WriteString(params)
	return out.String()
}

func replayPathNameByte(b byte) bool {
	return b == '-' || b == '_' || b == '.' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= 0x80
}

// replayFirstFinalizerDispatch lets the finalizer build and size its first
// request and ends the run there, so a refused request is not followed by a
// second dispatch that would build the same context again.
type replayFirstFinalizerDispatch struct {
	inner agent.Agent
	run   *replayRun
}

func (w *replayFirstFinalizerDispatch) Name() types.AgentName { return w.inner.Name() }

func (w *replayFirstFinalizerDispatch) Execute(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
	out, err := w.inner.Execute(ctx, sk)
	run := w.run
	run.mu.Lock()
	run.finalizerDispatches++
	if run.finalizerDispatches == 1 {
		run.finalizerErr = err
		var refusal *agent.AnswerContextByteBudgetError
		if errors.As(err, &refusal) {
			run.finalizerRefusal = refusal
		}
	}
	run.mu.Unlock()
	_ = run.end()
	return out, err
}

// replayLayout is where one replay keeps its files. Root stands in for the
// directory the recorded process ran in. It is exactly as long as the
// recorded one and everything below it keeps its recorded name, so a path
// that reaches a request differs from the recorded path only in the root,
// byte for byte in place.
type replayLayout struct {
	RecordedRoot string
	Root         string
	Repo         string
	Anchor       string
	SessionDir   string
	LogDir       string
}

// newReplayLayout creates the layout under base. The recorded session
// directory is <root>/.codrax/blob/<session>; the recorded repository and log
// directory must lie under the same root.
//
// The run directory's name is the first free one of the needed length, or
// name when the caller pins it. The name is an input like any other: digests
// are computed over paths below it, so two replays of one state can agree
// byte for byte, digests included, only when they ran in one directory.
func newReplayLayout(base string, state *replayState, name string) (replayLayout, error) {
	recorded := state.Recorded
	recordedBlob := filepath.Dir(recorded.SessionDir)
	recordedAnchor := filepath.Dir(recordedBlob)
	recordedRoot := filepath.Dir(recordedAnchor)
	if filepath.Base(recordedBlob) != "blob" || filepath.Base(recordedAnchor) != ".codrax" || recordedRoot == "/" || recordedRoot == "." {
		return replayLayout{}, fmt.Errorf("recorded session directory %q is not <root>/.codrax/blob/<session>", recorded.SessionDir)
	}
	under := func(path string) (string, error) {
		rel, err := filepath.Rel(recordedRoot, path)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			return "", fmt.Errorf("recorded path %q does not lie under the recorded run directory %q", path, recordedRoot)
		}
		return rel, nil
	}
	repoRel, err := under(recorded.RepoDir)
	if err != nil {
		return replayLayout{}, err
	}
	logRel, err := under(recorded.LogDir)
	if err != nil {
		return replayLayout{}, err
	}
	base, err = filepath.EvalSymlinks(filepath.Clean(base))
	if err != nil {
		return replayLayout{}, fmt.Errorf("replay work base: %w", err)
	}
	nameLen := len(recordedRoot) - len(base) - 1
	if nameLen < 4 {
		return replayLayout{}, fmt.Errorf("replay work base %q is too long: the recorded run directory is %d bytes, so the base may be at most %d", base, len(recordedRoot), len(recordedRoot)-5)
	}
	// A fresh directory of exactly the needed length; the exclusive create
	// keeps concurrent replays apart.
	root := ""
	if name != "" {
		if len(name) != nameLen || name != filepath.Base(name) {
			return replayLayout{}, fmt.Errorf("replay run directory name %q must be one path element of exactly %d bytes under %q", name, nameLen, base)
		}
		root = filepath.Join(base, name)
		if err := os.Mkdir(root, 0o755); err != nil {
			return replayLayout{}, fmt.Errorf("replay run directory: %w", err)
		}
	}
	for n := 0; n < 100000 && root == ""; n++ {
		digits := strconv.Itoa(n)
		if len(digits) > nameLen-1 {
			break
		}
		candidate := filepath.Join(base, "r"+strings.Repeat("0", nameLen-1-len(digits))+digits)
		switch err := os.Mkdir(candidate, 0o755); {
		case err == nil:
			root = candidate
		case !os.IsExist(err):
			return replayLayout{}, fmt.Errorf("replay work directory: %w", err)
		}
	}
	if root == "" {
		return replayLayout{}, fmt.Errorf("replay work base %q has no free directory name of %d bytes", base, nameLen)
	}
	layout := replayLayout{
		RecordedRoot: recordedRoot,
		Root:         root,
		Repo:         filepath.Join(root, repoRel),
		Anchor:       filepath.Join(root, ".codrax"),
		SessionDir:   filepath.Join(root, ".codrax", "blob", filepath.Base(recorded.SessionDir)),
		LogDir:       filepath.Join(root, logRel),
	}
	if len(layout.Root) != len(recordedRoot) || len(layout.SessionDir) != len(recorded.SessionDir) || len(layout.Repo) != len(recorded.RepoDir) {
		_ = os.RemoveAll(root)
		return replayLayout{}, fmt.Errorf("replay layout under %q does not have the recorded path lengths", root)
	}
	for _, dir := range []string{layout.Repo, layout.SessionDir, layout.LogDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			_ = os.RemoveAll(root)
			return replayLayout{}, err
		}
	}
	return layout, nil
}

// seedReplayRepo makes the repository the question is asked against: the
// fixture's files as one commit on the recorded branch, as the evaluation
// harness prepares it.
func seedReplayRepo(fixture, repo, branch string) error {
	if err := os.CopyFS(repo, os.DirFS(fixture)); err != nil {
		return fmt.Errorf("copy repository fixture: %w", err)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", branch},
		{"-c", "user.email=eval@codrax", "-c", "user.name=eval", "add", "-A"},
		{"-c", "user.email=eval@codrax", "-c", "user.name=eval", "commit", "-q", "-m", "seed"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, out)
		}
	}
	return nil
}

// replayInputs are the things a replay needs besides the state script.
type replayInputs struct {
	// CapturePath is the pinned capture; empty for a state without one.
	CapturePath string
	Layout      replayLayout
	// EndAt ends the replay at this agent's first request; empty means the
	// finalizer.
	EndAt types.AgentName
}

type replayOutcome struct {
	// Log is the debug log the replay wrote, whole.
	Log []byte
	// RunErr is what the orchestrator returned.
	RunErr error
	// Ended is true when the replay reached the request it was asked to end at.
	Ended bool
	// Divergence is non-nil when the code left the recording.
	Divergence *replayDivergence
	// TurnsUsed counts the recorded turns each agent consumed.
	TurnsUsed map[types.AgentName]int
	// CutTexts lists the replayed turns whose text the log had cut.
	CutTexts []replayCutText
	// FinalizerDispatches is how often the finalizer was dispatched before the
	// run ended. FinalizerRequestBytes is the size of the request its adapter
	// received and FinalizerToolBytes the tool schemas' share of it;
	// FinalizerRefusal is the typed refusal when no request was sent.
	FinalizerDispatches   int
	FinalizerRequestBytes int
	FinalizerToolBytes    int
	FinalizerRefusal      *agent.AnswerContextByteBudgetError
	FinalizerErr          error
}

// replayClock is the dispatch-owned clock of a replay. A recorded dispatch
// that stayed within its budget is replayed with a clock that never passes
// it, so the load of the replaying host decides nothing; a recorded dispatch
// that ran out is replayed with a clock that passes the budget at the
// recorded unit and stays past it.
func replayClock(cut *replayDynamicContextCut) agent.DynamicContextClock {
	if cut == nil {
		return func(agent.DynamicContextCheckpoint) time.Duration { return 0 }
	}
	var mu sync.Mutex
	expired := false
	return func(at agent.DynamicContextCheckpoint) time.Duration {
		mu.Lock()
		defer mu.Unlock()
		if at.Unit == cut.Unit && at.Deferred == cut.Deferred {
			expired = true
		}
		if expired {
			return 24 * time.Hour
		}
		return 0
	}
}

// runReplay builds the pipeline through the composition start-up uses and
// runs the question of the state through it.
//
// Shared with start-up: resolvePipelineSettings, resolveLogTriageSettings,
// resolvePerfTriageSettings, newPipelineSkillRegistry, composePipeline,
// installRuntimeStores, loadPreparedAttachedTrace, applySingleShotRoutePolicy
// and Orchestrator.Run.
//
// Built by hand here, because start-up derives them from the providers file,
// from a model call or from the process: the adapters and their sizing, the
// parameter compatibility table, the think-aloud table, the turn policy, the
// language, branch and step limit, the directories, and the dispatch-owned
// clock.
func runReplay(t *testing.T, state *replayState, in replayInputs) replayOutcome {
	t.Helper()
	run := newReplayRun(state, in.EndAt)
	run.recordedRoot, run.replayRoot = in.Layout.RecordedRoot, in.Layout.Root

	// Process-wide state a command-line run sets from its flags.
	previousLogger := logging.Default
	logger, err := logging.NewFromFlags(in.Layout.LogDir, "debug", false)
	if err != nil {
		t.Fatalf("replay logger: %v", err)
	}
	logging.SetDefault(logger)
	oldHitrace, oldLang, oldRepo, oldBranch := flagAttachHitrace, flagLang, flagRepo, flagBranch
	flagAttachHitrace, flagLang, flagRepo, flagBranch = nil, state.Run.Language, in.Layout.Repo, state.Run.Branch
	if in.CapturePath != "" {
		flagAttachHitrace = []string{in.CapturePath}
	}
	t.Chdir(in.Layout.Root)
	closed := false
	closeLog := func() {
		if !closed {
			closed = true
			_ = logger.Close()
		}
	}
	defer func() {
		flagAttachHitrace, flagLang, flagRepo, flagBranch = oldHitrace, oldLang, oldRepo, oldBranch
		logging.SetDefault(previousLogger)
		closeLog()
	}()

	compat := make(map[types.AgentName]types.ToolParamCompatConfig, len(state.Run.ToolParamCompat))
	for name, c := range state.Run.ToolParamCompat {
		split := c.SplitStringArrays
		compat[name] = types.ToolParamCompatConfig{Mode: c.Mode, SplitStringArrays: &split}
	}

	none := func(string) bool { return false }
	composed := composePipeline(pipelineCompositionInputs{
		Settings:                     resolvePipelineSettings(nil, none, nil),
		DefaultLLM:                   &replayAdapter{run: run, agent: "default"},
		Resolver:                     func(name types.AgentName) llm.Adapter { return &replayAdapter{run: run, agent: name} },
		ToolParamCompatByAgent:       compat,
		MCPServers:                   mcp.NewRegistry(),
		Skills:                       newPipelineSkillRegistry(),
		Emit:                         render.New(io.Discard, false).Emitter(),
		LogTriage:                    resolveLogTriageSettings(nil),
		PerfTriage:                   resolvePerfTriageSettings(nil),
		MaxSteps:                     state.Run.MaxSteps,
		Language:                     state.Run.Language,
		BlobSessionDir:               in.Layout.SessionDir,
		RuntimeAnchor:                in.Layout.Anchor,
		FinalizerDynamicContextClock: replayClock(state.DynamicContextCut),
	})
	orch := composed.Orchestrator
	run.mu.Lock()
	run.cancel = orch.Cancel
	run.mu.Unlock()
	installRuntimeStores(orch, newRuntimeStores(in.Layout.Anchor))
	orch.SetThinkAloudMap(state.Run.ThinkAloud)
	finalizer, err := composed.Agents.Get(types.AgentFinalizer)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	composed.Agents.Register(&replayFirstFinalizerDispatch{inner: finalizer, run: run})

	hasAttachment := false
	if in.CapturePath != "" {
		prepared, err := loadPreparedAttachedTrace(context.Background(), nil)
		if err != nil {
			t.Fatalf("replay: attach capture: %v", err)
		}
		orch.SetAttachedHitrace(prepared.body)
		orch.SetAttachedHitraceSource(prepared.source)
		orch.SetAttachedTraceMaterial(prepared.material)
		hasAttachment = prepared.body != ""
	}

	recorded := state.Run.TurnPolicy
	if recorded.Route != "" {
		policy := repl.ApplyTurnPolicyGuards(repl.TurnPolicy{
			RequiredOutcomes:          recorded.RequiredOutcomes,
			DeliveryOwner:             recorded.DeliveryOwner,
			Route:                     repl.TurnRoute(recorded.Route),
			NeedsRepoAccess:           recorded.NeedsRepoAccess,
			NeedsOperationAccess:      recorded.NeedsOperationAccess,
			NeedsDataAccess:           recorded.NeedsDataAccess,
			Operation:                 recorded.Operation,
			OperationKind:             recorded.OperationKind,
			DataTaskKind:              recorded.DataTaskKind,
			WriteIntent:               recorded.WriteIntent,
			Source:                    recorded.Source,
			CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceMode(recorded.CurrentSourceEvidenceMode),
			RiskLevel:                 recorded.RiskLevel,
			SideEffects:               recorded.SideEffects,
			TargetSurface:             recorded.TargetSurface,
			Confidence:                recorded.Confidence,
			Reason:                    recorded.Reason,
			PresentationDirective:     recorded.PresentationDirective,
			RequiresDiagram:           recorded.RequiresDiagram,
		}, false, hasAttachment)
		// The log recorded the policy after its guards; applying them again
		// must leave every recorded field where it was.
		if string(policy.Route) != recorded.Route || policy.Operation != recorded.Operation || policy.Source != recorded.Source ||
			string(policy.CurrentSourceEvidenceMode) != recorded.CurrentSourceEvidenceMode || policy.DeliveryOwner != recorded.DeliveryOwner ||
			policy.NeedsRepoAccess != recorded.NeedsRepoAccess || policy.NeedsDataAccess != recorded.NeedsDataAccess ||
			policy.NeedsOperationAccess != recorded.NeedsOperationAccess || policy.RequiresDiagram != recorded.RequiresDiagram ||
			policy.PresentationDirective != recorded.PresentationDirective {
			t.Fatalf("replay: the recorded turn policy is not stable under the policy guards: recorded %+v, guarded %+v", recorded, policy)
		}
		applySingleShotRoutePolicy(orch, policy, true)
	}

	_, runErr := orch.Run(state.Question, in.Layout.Repo, state.Run.Branch)
	closeLog()

	out := replayOutcome{RunErr: runErr, TurnsUsed: map[types.AgentName]int{}}
	run.mu.Lock()
	out.Divergence = run.divergence
	out.Ended = run.ended
	for name, n := range run.next {
		out.TurnsUsed[name] = n
	}
	out.CutTexts = append(out.CutTexts, run.cutTexts...)
	out.FinalizerDispatches = run.finalizerDispatches
	out.FinalizerRequestBytes = run.finalizerRequestBytes
	out.FinalizerToolBytes = run.finalizerToolBytes
	out.FinalizerRefusal = run.finalizerRefusal
	out.FinalizerErr = run.finalizerErr
	run.mu.Unlock()

	if out.Divergence == nil && out.Ended {
		out.Divergence = replayUnusedTurns(state, out.TurnsUsed, run.endAt)
	}

	logs, err := filepath.Glob(filepath.Join(in.Layout.LogDir, "*.log"))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	sort.Strings(logs)
	for _, path := range logs {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		out.Log = append(out.Log, body...)
	}
	return out
}

// replayUnusedTurns reports the first agent, in pipeline order before the
// agent the replay ended at, that finished without asking for every turn the
// recording has for it. A replay that reaches its end with turns left over
// did not follow the recording, however well its last request compares.
func replayUnusedTurns(state *replayState, used map[types.AgentName]int, endAt types.AgentName) *replayDivergence {
	for _, name := range []types.AgentName{types.AgentAnalyzer, types.AgentExplorer} {
		if name == endAt {
			return nil
		}
		if asked, have := used[name], len(state.Script[name]); asked < have {
			return &replayDivergence{State: state.Name, Agent: name, Turn: asked, Unused: have - asked}
		}
	}
	return nil
}

// firstRequestMessage is one message of the first request of one dispatch, as
// the debug log records it.
type firstRequestMessage struct {
	Agent    string
	Dispatch int
	Index    int
	Role     string
	Body     []byte
}

func (m firstRequestMessage) dispatchKey() string { return fmt.Sprintf("%s.d%d", m.Agent, m.Dispatch) }
func (m firstRequestMessage) key() string         { return fmt.Sprintf("%s.m%d", m.dispatchKey(), m.Index) }

var (
	replayLogDispatch = regexp.MustCompile(`(?m)^\S+ DEBUG \[diag (\w+)\] DISPATCH stage=\w+ attempt=\d+\n`)
	replayLogInit     = regexp.MustCompile(`(?m)^\S+ DEBUG \[diag (\w+)\] INIT msg role=(\w+) len=(\d+)\n`)
)

// firstRequestMessages reads every first-request message out of a debug log.
// A message's body is taken by its declared length, so text inside a body
// that looks like a log line is never read as one.
func firstRequestMessages(log []byte) ([]firstRequestMessage, error) {
	type event struct {
		at, end  int
		dispatch bool
		groups   [][]byte
	}
	var events []event
	for _, m := range replayLogDispatch.FindAllSubmatchIndex(log, -1) {
		events = append(events, event{at: m[0], end: m[1], dispatch: true, groups: [][]byte{log[m[2]:m[3]]}})
	}
	for _, m := range replayLogInit.FindAllSubmatchIndex(log, -1) {
		events = append(events, event{at: m[0], end: m[1], groups: [][]byte{log[m[2]:m[3]], log[m[4]:m[5]], log[m[6]:m[7]]}})
	}
	sort.Slice(events, func(i, j int) bool { return events[i].at < events[j].at })
	dispatches := map[string]int{}
	index := map[string]int{}
	var out []firstRequestMessage
	guard := 0
	for _, e := range events {
		if e.at < guard {
			continue
		}
		name := string(e.groups[0])
		if e.dispatch {
			if _, seen := dispatches[name]; seen {
				dispatches[name]++
			} else {
				dispatches[name] = 0
			}
			index[name] = 0
			continue
		}
		n, err := strconv.Atoi(string(e.groups[2]))
		if err != nil || e.end+n+5 > len(log) || string(log[e.end+n:e.end+n+5]) != "\n---\n" {
			return nil, fmt.Errorf("first-request message of %s at byte %d does not end where its length says", name, e.at)
		}
		out = append(out, firstRequestMessage{Agent: name, Dispatch: dispatches[name], Index: index[name], Role: string(e.groups[1]), Body: log[e.end : e.end+n]})
		index[name]++
		guard = e.end + n
	}
	return out, nil
}

// replayMask is one substitution applied to both sides before first requests
// are compared. Every mask but the one marked VariableLength overwrites a
// field in place with filler of the same length, and Why says why two runs
// cannot differ in that field's length. The variable-length mask is the
// exception the product forces: Why says what the field is, and its length
// difference is accounted for separately, to the byte.
type replayMask struct {
	Name           string
	Why            string
	VariableLength bool
	// Apply returns text with every occurrence of the field replaced, how many
	// occurrences it covered, and the bytes those occurrences took before.
	Apply func(text []byte, side replayMaskSide) (masked []byte, fields, fieldBytes int)
}

// replayMaskSide is what a mask may know about the runs being compared.
type replayMaskSide struct {
	RecordedRoot string
	ReplayRoot   string
}

// maskPattern overwrites every capture group of every match with fill and
// reports the matches and the bytes covered. The text keeps its length.
func maskPattern(text []byte, pattern *regexp.Regexp, fill byte) ([]byte, int, int) {
	matches := pattern.FindAllSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return text, 0, 0
	}
	out := append([]byte(nil), text...)
	covered := 0
	for _, m := range matches {
		for g := 2; g+1 < len(m); g += 2 {
			if m[g] < 0 {
				continue
			}
			for i := m[g]; i < m[g+1]; i++ {
				out[i] = fill
			}
			covered += m[g+1] - m[g]
		}
	}
	return out, len(matches), covered
}

var (
	replayTimestampPattern = regexp.MustCompile(`observed_at="(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2}))"`)
	// A stored result's name is printed whole, or clipped inside its digest
	// by a preview that is cut at a byte position.
	replayStoredNamePattern   = regexp.MustCompile(`(?:trace-query-result-|trace_query-)(?:([0-9a-f]{64})|([0-9a-f]{1,63})\.\.\.\[truncated)`)
	replayPublicationPattern  = regexp.MustCompile(`trace_query:publication-([0-9a-f]{64})`)
	replayInstancePattern     = regexp.MustCompile(`runtime_instance_([0-9a-f]{64})`)
	replayDiagramNodePattern  = regexp.MustCompile(`\brt_(?:thread_)?([0-9a-f]{64})`)
	replayCauseCandidateMatch = regexp.MustCompile(`candidate-([0-9a-f]{32})\b`)
)

// replayDigestMask masks the hexadecimal digits of one identifier family.
func replayDigestMask(name, why string, pattern *regexp.Regexp) replayMask {
	return replayMask{Name: name, Why: why, Apply: func(text []byte, _ replayMaskSide) ([]byte, int, int) {
		return maskPattern(text, pattern, '0')
	}}
}

// replayMasks are the only differences a reproduced first request may have
// from the recorded one. The run directory is the one input a replay cannot
// share with the recording; every other fixed-length mask covers a digest
// computed over it.
func replayMasks() []replayMask {
	return []replayMask{
		{
			Name: "run_directory",
			Why:  "the directory the process ran in; the replay creates its own with exactly the recorded length and keeps every name below it, so each path differs in place only",
			Apply: func(text []byte, side replayMaskSide) ([]byte, int, int) {
				fields, covered := 0, 0
				for _, root := range []string{side.RecordedRoot, side.ReplayRoot} {
					if root == "" {
						continue
					}
					if n := bytes.Count(text, []byte(root)); n > 0 {
						text = bytes.ReplaceAll(text, []byte(root), bytes.Repeat([]byte{'/'}, len(root)))
						fields += n
						covered += n * len(root)
					}
				}
				return text, fields, covered
			},
		},
		replayDigestMask("stored_result_name",
			"a stored tool result is named by the SHA-256 of its bytes, 64 hexadecimal digits, and those bytes contain paths under the run directory; where a preview clips the name inside the digest it does so at a byte position, which equal lengths before it keep equal",
			replayStoredNamePattern),
		replayDigestMask("publication_id",
			"a publication id ends in a SHA-256, 64 hexadecimal digits, over the stored result it publishes, whose name is covered above",
			replayPublicationPattern),
		replayDigestMask("runtime_instance_id",
			"an instance id is a SHA-256, 64 hexadecimal digits, over the source reference of the observation, which names the stored result",
			replayInstancePattern),
		replayDigestMask("diagram_node_id",
			"a diagram node id is an instance id under another prefix, or a SHA-256, 64 hexadecimal digits, over the same source reference and a thread",
			replayDiagramNodePattern),
		replayDigestMask("cause_candidate_id",
			"a cause candidate id is the first 16 bytes of a SHA-256, 32 hexadecimal digits, over the evidence id of its node, which is a publication id",
			replayCauseCandidateMatch),
		{
			Name:           "observed_at",
			VariableLength: true,
			Why:            "the wall-clock time a tool result was produced, printed with trailing zeros of the fraction removed, so its length varies between two runs of one build; the length difference of a message must equal the length difference of these fields exactly",
			Apply: func(text []byte, _ replayMaskSide) ([]byte, int, int) {
				matches := replayTimestampPattern.FindAllSubmatchIndex(text, -1)
				if len(matches) == 0 {
					return text, 0, 0
				}
				var out []byte
				last, covered := 0, 0
				for _, m := range matches {
					out = append(out, text[last:m[2]]...)
					out = append(out, "<time>"...)
					covered += m[3] - m[2]
					last = m[3]
				}
				return append(out, text[last:]...), len(matches), covered
			},
		},
	}
}

// firstRequestComparison is the result of comparing one recorded message with
// its replayed counterpart.
type firstRequestComparison struct {
	Key           string `json:"message"`
	Role          string `json:"role"`
	RecordedBytes int    `json:"recorded_bytes"`
	ReplayedBytes int    `json:"replayed_bytes"`
	// MaskedFields counts, per mask, the fields covered on the recorded and on
	// the replayed side.
	MaskedFields map[string][2]int `json:"masked_fields_recorded_replayed"`
	// VariableLengthBytes is the bytes the variable-length fields took on the
	// recorded and on the replayed side. A byte-identical reproduction has
	// RecordedBytes-ReplayedBytes equal to their difference.
	VariableLengthBytes [2]int `json:"variable_length_field_bytes_recorded_replayed"`
	// DiffBytes is the number of positions at which the masked texts differ,
	// plus their length difference; zero means identical after the masks.
	DiffBytes int `json:"masked_diff_bytes"`
	// FirstDiff is the offset of the first difference in the masked text, -1
	// when there is none.
	FirstDiff int `json:"first_diff_offset"`
}

func (c firstRequestComparison) identical() bool {
	return c.DiffBytes == 0 && c.RecordedBytes-c.ReplayedBytes == c.VariableLengthBytes[0]-c.VariableLengthBytes[1]
}

func compareFirstRequest(recorded, replayed firstRequestMessage, masks []replayMask, side replayMaskSide) (firstRequestComparison, []byte, []byte) {
	a, b := recorded.Body, replayed.Body
	cmp := firstRequestComparison{Key: recorded.key(), Role: recorded.Role, RecordedBytes: len(a), ReplayedBytes: len(b),
		MaskedFields: map[string][2]int{}, FirstDiff: -1}
	for _, mask := range masks {
		var na, nb, ba, bb int
		a, na, ba = mask.Apply(a, side)
		b, nb, bb = mask.Apply(b, side)
		if na > 0 || nb > 0 {
			cmp.MaskedFields[mask.Name] = [2]int{na, nb}
		}
		if mask.VariableLength {
			cmp.VariableLengthBytes[0] += ba
			cmp.VariableLengthBytes[1] += bb
		}
	}
	shorter := min(len(a), len(b))
	for i := 0; i < shorter; i++ {
		if a[i] != b[i] {
			cmp.DiffBytes++
			if cmp.FirstDiff < 0 {
				cmp.FirstDiff = i
			}
		}
	}
	if len(a) != len(b) {
		cmp.DiffBytes += max(len(a), len(b)) - shorter
		if cmp.FirstDiff < 0 {
			cmp.FirstDiff = shorter
		}
	}
	if recorded.Role != replayed.Role && cmp.DiffBytes == 0 {
		cmp.DiffBytes, cmp.FirstDiff = 1, 0
	}
	return cmp, a, b
}

// diffContext renders the surroundings of the first difference for a failure
// message.
func diffContext(a, b []byte, at int) string {
	clip := func(text []byte) string {
		lo, hi := max(0, at-200), min(len(text), at+200)
		if lo > hi {
			lo = hi
		}
		return strings.ToValidUTF8(string(text[lo:hi]), "?")
	}
	return fmt.Sprintf("recorded …%s…\nreplayed …%s…", clip(a), clip(b))
}
