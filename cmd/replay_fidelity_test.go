package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The replay of a recorded run is only an instrument if it asks what the
// recorded run asked. TestReplayReproducesRecordedFirstRequests is that
// check: for every state it is pointed at, each agent's first request of each
// recorded dispatch must come out of the replay byte for byte, apart from the
// fields replayMasks names.
//
// The states, the captures and the sealed logs live outside the repository,
// so the test runs only when it is told where they are:
//
//	CODRAX_REPLAY_STATES      directory holding one directory per state
//	CODRAX_REPLAY_WORK_BASE   short directory the replay may create its run
//	                          directory in (see newReplayLayout)
//	CODRAX_REPLAY_ONLY        optional comma-separated state names
//	CODRAX_REPLAY_AGENTS      optional comma-separated agents whose first
//	                          requests must be reproduced; default is all of
//	                          analyzer, explorer, finalizer. A name that is
//	                          no agent, such as none, requires no request:
//	                          the state is replayed, compared and reported,
//	                          and only a divergence fails
//	CODRAX_REPLAY_END_AT      optional agent whose first request ends the
//	                          replay; default finalizer
//	CODRAX_REPLAY_RUN_DIR     optional name of the run directory to create in
//	                          the work base instead of the first free one
//	CODRAX_REPLAY_REPORT_DIR  optional directory for per-state results
//
// plus, per state, the variable its script names for the pinned capture.
// Once the states directory is given nothing is skipped: a state that cannot
// be loaded, a capture or sealed log that is missing or is not the pinned
// one, fails the test.

const (
	replayStatesEnv    = "CODRAX_REPLAY_STATES"
	replayWorkBaseEnv  = "CODRAX_REPLAY_WORK_BASE"
	replayOnlyEnv      = "CODRAX_REPLAY_ONLY"
	replayAgentsEnv    = "CODRAX_REPLAY_AGENTS"
	replayEndAtEnv     = "CODRAX_REPLAY_END_AT"
	replayRunDirEnv    = "CODRAX_REPLAY_RUN_DIR"
	replayReportDirEnv = "CODRAX_REPLAY_REPORT_DIR"
)

var replayPipelineOrder = []types.AgentName{types.AgentAnalyzer, types.AgentExplorer, types.AgentFinalizer}

func replayCommaList(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// replayRepoRoot is the repository the test binary was built from; the
// repository fixture of a state is named relative to it.
func replayRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(wd)
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("repository root not found above %s: %v", wd, err)
	}
	return root
}

var (
	replayRefusalLine = regexp.MustCompile(`messages and tool schemas=(\d+) bytes; allocation=(\d+)`)
	replayReceiptLine = regexp.MustCompile(`(?m)\[diag finalizer\] phase=prompt_byte_budget receipt=(\{.*\})$`)
)

// replayReceiptDifferences compares the byte receipt of the recorded first
// finalizer request with the replayed one. Every field must be equal, except
// that the bytes the variable-length fields account for move with them:
// staticDelta bytes (recorded minus replayed) out of the fixed part and into
// the allocation of the rest, dynamicDelta bytes out of the published rest.
func replayReceiptDifferences(recorded, replayed map[string]any, staticDelta, dynamicDelta int) []string {
	shift := map[string]int{"static_bytes": staticDelta, "dynamic_allocated_bytes": -staticDelta, "dynamic_published_bytes": dynamicDelta}
	keys := map[string]bool{}
	for k := range recorded {
		keys[k] = true
	}
	for k := range replayed {
		keys[k] = true
	}
	var out []string
	for k := range keys {
		a, b := recorded[k], replayed[k]
		if delta, ok := shift[k]; ok {
			an, aok := a.(float64)
			bn, bok := b.(float64)
			if aok && bok && int(an)-int(bn) == delta {
				continue
			}
		} else if reflect.DeepEqual(a, b) {
			continue
		}
		out = append(out, fmt.Sprintf("%s: recorded %v, replayed %v", k, a, b))
	}
	sort.Strings(out)
	return out
}

// replayFidelityReport is what one state's check writes out.
type replayFidelityReport struct {
	State               string                   `json:"state"`
	RunDirectory        string                   `json:"run_directory"`
	RecordedBuild       string                   `json:"recorded_build"`
	EndAt               string                   `json:"end_at"`
	Agents              []string                 `json:"agents_required"`
	Masks               []replayMaskDescription  `json:"masks"`
	Messages            []firstRequestComparison `json:"messages"`
	NotReplayed         []string                 `json:"recorded_messages_after_the_replay_end"`
	TurnsUsed           map[string]int           `json:"recorded_turns_used"`
	TurnsRecorded       map[string]int           `json:"recorded_turns"`
	Divergence          string                   `json:"divergence,omitempty"`
	CutTexts            []replayCutText          `json:"model_texts_cut_by_the_log"`
	RecordedReceipt     map[string]any           `json:"recorded_first_request_receipt,omitempty"`
	ReplayedReceipt     map[string]any           `json:"replayed_first_request_receipt,omitempty"`
	RecordedRefusal     *replayRefusal           `json:"recorded_first_request_refusal,omitempty"`
	ReplayedRefusal     *replayRefusal           `json:"replayed_first_request_refusal,omitempty"`
	ReplayedRequest     int                      `json:"replayed_finalizer_request_bytes,omitempty"`
	ReplayedToolSchemas int                      `json:"replayed_finalizer_tool_schema_bytes,omitempty"`
	Identical           bool                     `json:"identical_after_masks"`
}

type replayMaskDescription struct {
	Name           string `json:"name"`
	VariableLength bool   `json:"variable_length"`
	Why            string `json:"why"`
}

type replayRefusal struct {
	ActualBytes    int `json:"actual_bytes"`
	AllocatedBytes int `json:"allocated_bytes"`
}

func TestReplayReproducesRecordedFirstRequests(t *testing.T) {
	statesDir := os.Getenv(replayStatesEnv)
	if statesDir == "" {
		t.Skipf("%s is not set: it names the directory of replay state scripts, which live outside the repository with the captures they pin", replayStatesEnv)
	}
	workBase := os.Getenv(replayWorkBaseEnv)
	if workBase == "" {
		t.Fatalf("%s is not set: a replay needs a short directory to create its run directory in", replayWorkBaseEnv)
	}
	names := replayCommaList(os.Getenv(replayOnlyEnv))
	if len(names) == 0 {
		entries, err := os.ReadDir(statesDir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				names = append(names, entry.Name())
			}
		}
	}
	if len(names) == 0 {
		t.Fatalf("%s=%s holds no state", replayStatesEnv, statesDir)
	}
	required := replayCommaList(os.Getenv(replayAgentsEnv))
	if len(required) == 0 {
		for _, name := range replayPipelineOrder {
			required = append(required, string(name))
		}
	}
	endAt := types.AgentName(os.Getenv(replayEndAtEnv))
	if endAt == "" {
		endAt = types.AgentFinalizer
	}
	repoRoot := replayRepoRoot(t)

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			state, err := loadReplayState(filepath.Join(statesDir, name))
			if err != nil {
				t.Fatal(err)
			}
			sealed, err := os.ReadFile(state.Recorded.Log.Path)
			if err != nil {
				t.Fatalf("sealed log of %s: %v", name, err)
			}
			if got := sha256Hex(sealed); got != state.Recorded.Log.SHA256 {
				t.Fatalf("sealed log %s is not the one the script was built from (sha256 %s, script %s)", state.Recorded.Log.Path, got, state.Recorded.Log.SHA256)
			}
			capture := os.Getenv(state.Capture.Env)
			if capture == "" {
				t.Fatalf("%s is not set: it names the capture state %s pins", state.Capture.Env, name)
			}
			sum, size, err := sha256File(capture)
			if err != nil {
				t.Fatal(err)
			}
			if sum != state.Capture.SHA256 || size != state.Capture.Bytes {
				t.Fatalf("capture %s=%s is not the pinned one (sha256 %s, %d bytes; script pins %s, %d bytes)", state.Capture.Env, capture, sum, size, state.Capture.SHA256, state.Capture.Bytes)
			}

			layout, err := newReplayLayout(workBase, state, os.Getenv(replayRunDirEnv))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(layout.Root) })
			if err := seedReplayRepo(filepath.Join(repoRoot, state.Run.RepoFixture), layout.Repo, state.Run.Branch); err != nil {
				t.Fatal(err)
			}

			out := runReplay(t, state, replayInputs{CapturePath: capture, Layout: layout, EndAt: endAt})

			recorded, err := firstRequestMessages(sealed)
			if err != nil {
				t.Fatalf("sealed log: %v", err)
			}
			replayed, err := firstRequestMessages(out.Log)
			if err != nil {
				t.Fatalf("replay log: %v", err)
			}
			byKey := map[string]firstRequestMessage{}
			replayedDispatch := map[string]bool{}
			for _, m := range replayed {
				byKey[m.key()] = m
				replayedDispatch[m.dispatchKey()] = true
			}
			requiredAgent := map[string]bool{}
			for _, a := range required {
				requiredAgent[a] = true
			}
			// A replay that ends at an agent has that agent's first dispatch
			// and nothing after it in pipeline order.
			reaches := map[string]bool{}
			for _, a := range replayPipelineOrder {
				reaches[string(a)] = true
				if a == endAt {
					break
				}
			}

			masks := replayMasks()
			side := replayMaskSide{RecordedRoot: layout.RecordedRoot, ReplayRoot: layout.Root}
			report := replayFidelityReport{State: name, RunDirectory: layout.Root, RecordedBuild: state.Recorded.Build, EndAt: string(endAt), Agents: required,
				TurnsUsed: map[string]int{}, TurnsRecorded: map[string]int{}, Identical: true,
				ReplayedRequest: out.FinalizerRequestBytes, ReplayedToolSchemas: out.FinalizerToolBytes}
			for _, mask := range masks {
				report.Masks = append(report.Masks, replayMaskDescription{Name: mask.Name, VariableLength: mask.VariableLength, Why: mask.Why})
			}
			for agentName, turns := range state.Script {
				report.TurnsRecorded[string(agentName)] = len(turns)
			}
			for agentName, n := range out.TurnsUsed {
				report.TurnsUsed[string(agentName)] = n
			}
			if m := replayRefusalLine.FindSubmatch(sealed); m != nil {
				actual, _ := strconv.Atoi(string(m[1]))
				allocated, _ := strconv.Atoi(string(m[2]))
				report.RecordedRefusal = &replayRefusal{ActualBytes: actual, AllocatedBytes: allocated}
			}
			if out.FinalizerRefusal != nil {
				report.ReplayedRefusal = &replayRefusal{ActualBytes: out.FinalizerRefusal.ActualBytes, AllocatedBytes: out.FinalizerRefusal.AllocatedBytes}
			}

			reportDir := os.Getenv(replayReportDirEnv)
			if reportDir != "" {
				reportDir = filepath.Join(reportDir, name)
				if err := os.MkdirAll(reportDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(reportDir, "replay.log"), out.Log, 0o644); err != nil {
					t.Fatal(err)
				}
			}

			var failures []string
			if out.Divergence != nil {
				report.Divergence = out.Divergence.Error()
				failures = append(failures, out.Divergence.Error())
			} else if !out.Ended {
				failures = append(failures, fmt.Sprintf("the run ended before the %s's first request: %v", endAt, out.RunErr))
			}
			report.CutTexts = out.CutTexts
			report.RecordedReceipt = state.RecordedFirstRequest.Receipt
			if m := replayReceiptLine.FindSubmatch(out.Log); m != nil {
				if err := json.Unmarshal(m[1], &report.ReplayedReceipt); err != nil {
					t.Fatalf("replayed receipt: %v", err)
				}
			}
			finalizerLengthDelta, finalizerCompared := 0, false
			finalizerStaticDelta, finalizerDynamicDelta := 0, 0
			for _, want := range recorded {
				if !reaches[want.Agent] || (want.Agent == string(endAt) && want.Dispatch > 0) {
					report.NotReplayed = append(report.NotReplayed, want.key())
					continue
				}
				got, ok := byKey[want.key()]
				if !ok {
					if requiredAgent[want.Agent] {
						failures = append(failures, fmt.Sprintf("%s: the replay made no such message", want.key()))
						report.Identical = false
					}
					continue
				}
				cmp, a, b := compareFirstRequest(want, got, masks, side)
				report.Messages = append(report.Messages, cmp)
				if want.Agent == string(types.AgentFinalizer) {
					finalizerLengthDelta += cmp.RecordedBytes - cmp.ReplayedBytes
					finalizerCompared = true
					// The last message of the first request is the part
					// built within the allocation; the others are fixed.
					if want.Index == 2 {
						finalizerDynamicDelta += cmp.RecordedBytes - cmp.ReplayedBytes
					} else {
						finalizerStaticDelta += cmp.RecordedBytes - cmp.ReplayedBytes
					}
				}
				if cmp.identical() {
					continue
				}
				if reportDir != "" {
					_ = os.WriteFile(filepath.Join(reportDir, want.key()+".recorded.masked"), a, 0o644)
					_ = os.WriteFile(filepath.Join(reportDir, want.key()+".replayed.masked"), b, 0o644)
				}
				if requiredAgent[want.Agent] {
					report.Identical = false
					failures = append(failures, fmt.Sprintf("%s (%s): recorded %d bytes, replayed %d bytes, %d bytes differ after the masks (variable-length fields %d and %d bytes), first at masked offset %d\n%s",
						want.key(), want.Role, cmp.RecordedBytes, cmp.ReplayedBytes, cmp.DiffBytes, cmp.VariableLengthBytes[0], cmp.VariableLengthBytes[1], cmp.FirstDiff, diffContext(a, b, cmp.FirstDiff)))
				}
			}
			// The replay must not make a first-request message the recording
			// lacks in a dispatch both have.
			recordedKeys := map[string]bool{}
			for _, m := range recorded {
				recordedKeys[m.key()] = true
			}
			for _, m := range replayed {
				if !recordedKeys[m.key()] && requiredAgent[m.Agent] {
					report.Identical = false
					failures = append(failures, fmt.Sprintf("%s: the recording has no such message", m.key()))
				}
			}
			// The refused size is the measure of the same request: it may
			// differ from the recorded one only by the variable-length fields.
			if requiredAgent[string(types.AgentFinalizer)] && endAt == types.AgentFinalizer && finalizerCompared {
				switch {
				case (report.RecordedRefusal == nil) != (report.ReplayedRefusal == nil):
					report.Identical = false
					failures = append(failures, fmt.Sprintf("first finalizer request: recorded refusal %+v, replayed refusal %+v", report.RecordedRefusal, report.ReplayedRefusal))
				case report.RecordedRefusal != nil:
					if report.RecordedRefusal.AllocatedBytes != report.ReplayedRefusal.AllocatedBytes ||
						report.RecordedRefusal.ActualBytes-report.ReplayedRefusal.ActualBytes != finalizerLengthDelta {
						report.Identical = false
						failures = append(failures, fmt.Sprintf("first finalizer request: recorded refusal %+v, replayed refusal %+v, and the messages account for a difference of %d bytes", *report.RecordedRefusal, *report.ReplayedRefusal, finalizerLengthDelta))
					}
				}
			}

			if requiredAgent[string(types.AgentFinalizer)] && endAt == types.AgentFinalizer && finalizerCompared {
				if report.RecordedReceipt == nil || report.ReplayedReceipt == nil {
					report.Identical = false
					failures = append(failures, fmt.Sprintf("first finalizer request: recorded receipt present=%v, replayed receipt present=%v", report.RecordedReceipt != nil, report.ReplayedReceipt != nil))
				} else if diffs := replayReceiptDifferences(report.RecordedReceipt, report.ReplayedReceipt, finalizerStaticDelta, finalizerDynamicDelta); len(diffs) > 0 {
					report.Identical = false
					failures = append(failures, "first finalizer request receipt differs: "+strings.Join(diffs, "; "))
				}
			}

			sort.Strings(report.NotReplayed)
			if reportDir != "" {
				body, err := json.MarshalIndent(report, "", "  ")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(reportDir, "fidelity.json"), append(body, '\n'), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			for _, cmp := range report.Messages {
				t.Logf("%s %-9s recorded=%d replayed=%d masked_diff=%d variable_length=%v masked=%v", cmp.Key, cmp.Role, cmp.RecordedBytes, cmp.ReplayedBytes, cmp.DiffBytes, cmp.VariableLengthBytes, cmp.MaskedFields)
			}
			t.Logf("recorded refusal %+v; replayed refusal %+v; request handed to the finalizer's model %d bytes (tool schemas %d)", report.RecordedRefusal, report.ReplayedRefusal, report.ReplayedRequest, report.ReplayedToolSchemas)
			t.Logf("recorded receipt %v", report.RecordedReceipt)
			t.Logf("replayed receipt %v", report.ReplayedReceipt)
			t.Logf("model texts cut by the log: %+v", report.CutTexts)
			for _, failure := range failures {
				t.Error(failure)
			}
		})
	}
}
