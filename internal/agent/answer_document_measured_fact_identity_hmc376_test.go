package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// measuredFactPublicResultsHMC376 runs the two public queries a causal
// investigation makes for one target and window.
func measuredFactPublicResultsHMC376(t *testing.T, path string, pid int, start, end float64) []types.ToolResult {
	t.Helper()
	dir := t.TempDir()
	var results []types.ToolResult
	for _, view := range []string{"wakeup_chain", "root_cause_rank"} {
		params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": view, "pid": pid, "time_start": start, "time_end": end, "trace_flavor": "harmony_hitrace"})
		result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
		if err != nil || !result.Success {
			t.Fatalf("public %s: %v %s", view, err, result.Summary)
		}
		results = append(results, result)
	}
	return results
}

// measuredFactFinalizerInstructionHMC376 builds the real finalizer
// instruction for a causal diagnosis of one target thread in one window.
func measuredFactFinalizerInstructionHMC376(t *testing.T, lang, target string, pid int, start, end float64, results []types.ToolResult) string {
	t.Helper()
	dir := t.TempDir()
	var observations []types.ObservationRecord
	for _, result := range results {
		observations = append(observations, result.Observations...)
	}
	ctx := tracePrincipalValueAuthorityTestContext(target, pid, observations)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis}
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end}
	ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
	ctx = ctxbuilder.BuildAgentContext(&types.BusContext{RepoRoot: dir, WorkDir: dir, Language: lang, Mutable: ctx.Mutable, AnalysisIR: ctx.AnalysisIR}, types.AgentFinalizer, types.StageFinalize)
	instruction := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(instruction, "## Final Trace Decision Boundary") {
		t.Fatal("actual finalizer context missing")
	}
	return instruction
}

// measuredFactWithoutIdentityHMC376 returns the same results as a session
// restored without the measured identity would hold them.
func measuredFactWithoutIdentityHMC376(results []types.ToolResult) []types.ToolResult {
	out := make([]types.ToolResult, len(results))
	for i, result := range results {
		out[i] = result
		out[i].Observations = make([]types.ObservationRecord, len(result.Observations))
		for j, record := range result.Observations {
			out[i].Observations[j] = record
			var notes []string
			for _, note := range record.RichNotes {
				if !strings.HasPrefix(note, types.TraceNoteKeyMeasuredStateAccountKey+"=") {
					notes = append(notes, note)
				}
			}
			out[i].Observations[j].RichNotes = notes
		}
	}
	return out
}

// measuredFactReaderCardHMC376 returns the entries of the reader card that
// lists where measured time concentrates.
func measuredFactReaderCardHMC376(t *testing.T, instruction string, zh bool) []string {
	t.Helper()
	heading := "- Measured time concentrations"
	if zh {
		heading = "- 真实耗时集中"
	}
	var entries []string
	inside := false
	for _, line := range strings.Split(instruction, "\n") {
		switch {
		case strings.HasPrefix(line, heading):
			inside = true
		case inside && strings.HasPrefix(line, "  - "):
			entries = append(entries, line)
		case inside:
			return entries
		}
	}
	t.Fatal("finalizer instruction lost the measured-time reader card")
	return nil
}

var measuredFactLocatorRoleHMC376 = regexp.MustCompile(`locator_role="([^"]*)"`)

// measuredFactAssertOnceWithOneScopeHMC376 asserts that the finalizer input
// states one measured state account once, with one scope, on every carrier.
func measuredFactAssertOnceWithOneScopeHMC376(t *testing.T, instruction string, zh bool, subject, value string) {
	t.Helper()
	scope, readerValue, readerSubject := "not one continuous interval", "measured "+value+" ms", "  - "+subject+": "
	if zh {
		scope, readerValue, readerSubject = "不是单次连续区间", "已测 "+value+" 毫秒", "  - "+subject+"："
	}
	var card []string
	for _, entry := range measuredFactReaderCardHMC376(t, instruction, zh) {
		if strings.HasPrefix(entry, readerSubject) && strings.Contains(entry, readerValue) {
			card = append(card, entry)
		}
	}
	if len(card) != 1 {
		t.Errorf("%s %sms: reader card states the account %d times, want once:\n%s", subject, value, len(card), strings.Join(card, "\n"))
	} else if !strings.Contains(card[0], scope) {
		t.Errorf("%s %sms: reader card states the account without its cumulative scope: %s", subject, value, card[0])
	}
	occupancy := "measured_state_occupancy=" + value + "ms"
	listed := 0
	roles := map[string]bool{}
	for _, line := range strings.Split(instruction, "\n") {
		if !strings.Contains(line, "subject=`"+subject+"`") || !strings.Contains(line, occupancy) {
			continue
		}
		if strings.HasPrefix(line, "  - subject=`"+subject+"`;") {
			listed++
		}
		if m := measuredFactLocatorRoleHMC376.FindStringSubmatch(line); m != nil {
			roles[m[1]] = true
		}
	}
	if listed != 1 {
		t.Errorf("%s %sms: measured list states the account %d times, want once", subject, value, listed)
	}
	var got []string
	for role := range roles {
		got = append(got, role)
	}
	sort.Strings(got)
	if len(got) != 1 || got[0] == "record range (single occurrence unproven)" {
		t.Errorf("%s %sms: every carrier must give the same proven scope, got %q", subject, value, got)
	}
}

func measuredFactLineHMC376(instruction, prefix string) string {
	for _, line := range strings.Split(instruction, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	return ""
}

// One native scheduler-state account is one fact in the finalizer input: one
// entry in the measured list, one entry in the reader card, and one scope on
// every carrier. The priced estimate of the seat stays a separate value.
func TestHMC376FinalizerStatesEachMeasuredStateAccountOnceWithOneScope(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/real_traces/donghu_tieba_frame.systrace")
	const start, end = 34579.490, 34579.500
	results := measuredFactPublicResultsHMC376(t, path, 59566, start, end)
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			zh := lang == "zh"
			instruction := measuredFactFinalizerInstructionHMC376(t, lang, "com.baidu.tieba-59566", 59566, start, end, results)
			// A seat priced differently from its measurement (5.951 vs 5.930),
			// and a separately priced sub-seat beside the wakeup view of the
			// same sleep account: two shapes of one class.
			measuredFactAssertOnceWithOneScopeHMC376(t, instruction, zh, "NetworkService-60595", "5.930")
			measuredFactAssertOnceWithOneScopeHMC376(t, instruction, zh, "CookieMonsterCl-59843", "6.406")

			// The estimate and the measurement stay separate fields.
			rank := measuredFactLineHMC376(instruction, "  - rank=#1; subject=`NetworkService-60595`;")
			if !strings.Contains(rank, "effective_attribution=5.951ms") || !strings.Contains(rank, "measured_state_occupancy=5.930ms") {
				t.Errorf("rank line merged or lost the estimate and the measurement: %s", rank)
			}
			reader, want := measuredFactLineHMC376(instruction, "  - Rank 1, NetworkService-60595:"), "modeled potential 5.951 ms; separate measured state account (scheduling latency): 5.930 ms (total in the dependency analysis window [34579.490000, 34579.496406) s, not one continuous interval)"
			if zh {
				reader, want = measuredFactLineHMC376(instruction, "  - 第 1 位，NetworkService-60595："), "估算优化潜力 5.951 毫秒；另账：调度延迟，实测占用 5.930 毫秒（依赖分析窗口 [34579.490000, 34579.496406) 秒内的累计量，不是单次连续区间）"
			}
			if !strings.Contains(reader, want) {
				t.Errorf("reader rank line must keep both values and state the cumulative scope: %s", reader)
			}

			// The identity is not model-facing text, and without it the
			// consumers fail open to separate entries: nothing joins on equal
			// values or names. One fact stated once can only shorten the input.
			if strings.Contains(instruction, types.TraceNoteKeyMeasuredStateAccountKey) {
				t.Error("the measured identity reached the finalizer instruction")
			}
			restored := measuredFactFinalizerInstructionHMC376(t, lang, "com.baidu.tieba-59566", 59566, start, end, measuredFactWithoutIdentityHMC376(results))
			separate := 0
			for _, entry := range measuredFactReaderCardHMC376(t, restored, zh) {
				if strings.Contains(entry, "NetworkService-60595") {
					separate++
				}
			}
			if separate != 2 {
				t.Errorf("without the native identity equal subject and value must not join: reader card has %d entries, want 2", separate)
			}
			if len(instruction) > len(restored) {
				t.Errorf("stating each account once grew the instruction: %d > %d bytes", len(instruction), len(restored))
			}
		})
	}
}

// The same class on a complete user capture: the seat is priced at 36.311 ms,
// the measured runnable account of the same thread is 45.378 ms.
func TestHMC376HoneybirdFinalizerStatesMeasuredRunnableOnce(t *testing.T) {
	path := os.Getenv("CODRAX_HMC365_HONEYBIRD")
	if path == "" {
		t.Skip("requires the pinned complete Honeybird ftrace")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.New()
	n, err := io.Copy(h, f)
	_ = f.Close()
	if err != nil || n != 106896120 || hex.EncodeToString(h.Sum(nil)) != "21280a96618781b37be37f9289be9ba286d6a85d358e60ff72d9c73c827cf30e" {
		t.Fatalf("wrong whole capture: %v", err)
	}
	const start, end = 11343.248, 11343.310
	results := measuredFactPublicResultsHMC376(t, path, 64156, start, end)
	instruction := measuredFactFinalizerInstructionHMC376(t, "zh", "ele.crowdsource-64156", 64156, start, end, results)
	measuredFactAssertOnceWithOneScopeHMC376(t, instruction, true, "Signal Catcher-64176", "45.378")
	reader := measuredFactLineHMC376(instruction, "  - 第 1 位，Signal Catcher-64176：")
	if !strings.Contains(reader, "估算优化潜力 36.311 毫秒；另账：调度延迟，实测占用 45.378 毫秒（依赖分析窗口 [11343.250704, 11343.297396) 秒内的累计量，不是单次连续区间）") {
		t.Errorf("reader rank line must keep both values and state the cumulative scope: %s", reader)
	}
	// Protected: the seats and their prices.
	var observations []types.ObservationRecord
	for _, result := range results {
		observations = append(observations, result.Observations...)
	}
	var seats []string
	for _, seat := range types.TraceCausalProjectionFromObservationRecords(observations).RankedSeats {
		if seat.ChainRelevance == "on_chain" {
			seats = append(seats, fmt.Sprintf("%.3f", seat.EffectiveImpactMS))
		}
	}
	if strings.Join(seats, "/") != "36.311/3.868/2.600/0.013" {
		t.Fatalf("on-chain seat impacts changed: %v", seats)
	}
}
