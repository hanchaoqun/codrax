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
				if !strings.HasPrefix(note, types.TraceNoteKeyMeasuredStateAccountKey+"=") &&
					!strings.HasPrefix(note, types.TraceNoteKeyMeasuredOccurrenceGroupKey+"=") {
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

var measuredFactCardValueHMC376 = regexp.MustCompile(`^  - (.+?)：.*?已测 ([0-9.]+) 毫秒`)

// measuredFactCardAccountsHMC376 counts the measured accounts the reader card
// states, keyed by subject, cause label and value.
func measuredFactCardAccountsHMC376(t *testing.T, instruction string) map[string]int {
	t.Helper()
	accounts := map[string]int{}
	for _, entry := range measuredFactReaderCardHMC376(t, instruction, true) {
		if m := measuredFactCardValueHMC376.FindStringSubmatch(entry); m != nil {
			accounts[m[1]+" "+m[2]]++
		}
	}
	return accounts
}

// The account identity is window-free, so one account can reach the finalizer
// from queries over different windows. Stating it once must never cost the
// account itself: whatever the reader card lists without the identity stays
// listed with it, exactly once.
func TestHMC376FinalizerKeepsAccountsMeasuredOverDifferentQueryWindows(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/real_traces/donghu_tieba_frame.systrace")
	const pid, start, end = 59566, 34579.490, 34579.500
	query := func(view string, from, to float64) types.ToolResult {
		dir := t.TempDir()
		params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": view, "pid": pid, "time_start": from, "time_end": to, "trace_flavor": "harmony_hitrace"})
		result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
		if err != nil || !result.Success {
			t.Fatalf("public %s: %v %s", view, err, result.Summary)
		}
		return result
	}
	for _, tc := range []struct {
		name                       string
		rankStart, rankEnd         float64
		chainStart, chainEnd       float64
		reportedStart, reportedEnd float64
	}{
		{"rank_wider_report_narrow", start, end + 0.0002, start, end, start, end},
		{"chain_wider_report_narrow", start, end, start, end + 0.0005, start, end},
		{"chain_wider_report_wide", start, end, start, end + 0.001, start, end + 0.001},
		{"rank_wider_both_sides", start - 0.0005, end + 0.0005, start, end, start, end},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rank, chain := query("root_cause_rank", tc.rankStart, tc.rankEnd), query("wakeup_chain", tc.chainStart, tc.chainEnd)
			for order, results := range map[string][]types.ToolResult{"rank_first": {rank, chain}, "chain_first": {chain, rank}} {
				instruction := measuredFactFinalizerInstructionHMC376(t, "zh", "com.baidu.tieba-59566", pid, tc.reportedStart, tc.reportedEnd, results)
				restored := measuredFactFinalizerInstructionHMC376(t, "zh", "com.baidu.tieba-59566", pid, tc.reportedStart, tc.reportedEnd, measuredFactWithoutIdentityHMC376(results))
				got, separate := measuredFactCardAccountsHMC376(t, instruction), measuredFactCardAccountsHMC376(t, restored)
				if len(separate) == 0 {
					t.Fatalf("%s: the reader card lists no measured account; the case exercises nothing", order)
				}
				for account := range separate {
					if got[account] != 1 {
						t.Errorf("%s: %q is listed %d times, want once:\n%s", order, account, got[account],
							strings.Join(measuredFactReaderCardHMC376(t, instruction, true), "\n"))
					}
				}
			}
		})
	}
}

// A multi-occurrence account reaches the finalizer as a seat and as a fold of
// its occurrences. It is one measured fact with its value and scope, and the
// fold's "occupancy unavailable" twin no longer takes a card slot.
func TestHMC376FinalizerStatesMultiOccurrenceAccountOnce(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/real_traces/donghu_tieba_frame.systrace")
	const start, end = 34579.480, 34579.510
	results := measuredFactPublicResultsHMC376(t, path, 59566, start, end)
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			zh := lang == "zh"
			instruction := measuredFactFinalizerInstructionHMC376(t, lang, "com.baidu.tieba-59566", 59566, start, end, results)
			measuredFactAssertOnceWithOneScopeHMC376(t, instruction, zh, "NetworkService-60595", "13.699")
			measuredFactAssertOnceWithOneScopeHMC376(t, instruction, zh, "CookieMonsterCl-59843", "15.038")
			unavailable := "original state occupancy not provided"
			if zh {
				unavailable = "原始状态占用未提供"
			}
			for _, entry := range measuredFactReaderCardHMC376(t, instruction, zh) {
				if strings.Contains(entry, unavailable) &&
					(strings.Contains(entry, "NetworkService-60595") || strings.Contains(entry, "CookieMonsterCl-59843")) {
					t.Errorf("a measured account also appears as unavailable: %s", entry)
				}
			}
			// The estimate stays a separate value on the rank line.
			rank := measuredFactLineHMC376(instruction, "  - rank=#1; subject=`NetworkService-60595`;")
			if !strings.Contains(rank, "effective_attribution=13.737ms") || !strings.Contains(rank, "measured_state_occupancy=13.699ms") {
				t.Errorf("rank line merged or lost the estimate and the measurement: %s", rank)
			}
			// The identity is not model-facing text.
			groups := 0
			for _, result := range results {
				for _, record := range result.Observations {
					for _, note := range record.RichNotes {
						if value, ok := strings.CutPrefix(note, types.TraceNoteKeyMeasuredOccurrenceGroupKey+"="); ok {
							groups++
							if strings.Contains(instruction, value) {
								t.Errorf("the occurrence group identity %s reached the finalizer instruction", value)
							}
						}
					}
				}
			}
			if groups == 0 {
				t.Fatal("no occurrence row names its account; the fixture no longer exercises this class")
			}
			if strings.Contains(instruction, types.TraceNoteKeyMeasuredOccurrenceGroupKey) {
				t.Error("the occurrence group note reached the finalizer instruction")
			}
			// Without the identity nothing joins on equal values or names.
			restored := measuredFactFinalizerInstructionHMC376(t, lang, "com.baidu.tieba-59566", 59566, start, end, measuredFactWithoutIdentityHMC376(results))
			separate := 0
			for _, entry := range measuredFactReaderCardHMC376(t, restored, zh) {
				if strings.Contains(entry, "NetworkService-60595") {
					separate++
				}
			}
			if separate != 2 {
				t.Errorf("without the native identity the fold must stay a separate entry: %d entries, want 2", separate)
			}
		})
	}
}

func measuredFactSeatHMC376(evidence string, rank int, priced, measured float64, key string) types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		EvidenceID: evidence, Subject: "dep-21", Predicate: "root_cause_primary", StateKind: "runnable",
		Rank: rank, ImpactMS: priced, EffectiveImpactMS: priced, RunnableMS: measured,
		MeasuredStateAccountKey: key, StartTs: 2, EndTs: 2.06,
	}
}

func measuredFactViewHMC376(evidence string, measured float64, key string) types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		EvidenceID: evidence, Subject: "dep-21", Predicate: "wakeup_causal_impact", StateKind: "runnable",
		ImpactMS: measured, RunnableMS: measured, StateAccountKey: key, StartTs: 2, EndTs: 2.06,
	}
}

func measuredFactEvidenceHMC376(nodes []types.TraceCausalProjectionNode) string {
	ids := make([]string, 0, len(nodes))
	for _, node := range nodes {
		ids = append(ids, node.EvidenceID)
	}
	return strings.Join(ids, ",")
}

// The window preference judges each publication. A publication it would reject
// must not stand in for the one it keeps, in either pool order.
func TestHMC376WindowPreferenceChoosesAmongEveryPublicationOfAnAccount(t *testing.T) {
	const key = "state_account:v2:account"
	other := measuredFactSeatHMC376("seat-other-window", 1, 30, 40, key)
	other.QueryWindowStartTs, other.QueryWindowEndTs = 2, 2.2
	matching := measuredFactViewHMC376("view-report-window", 40, key)
	matching.QueryWindowStartTs, matching.QueryWindowEndTs = 2, 2.1
	for name, pool := range map[string][]types.TraceCausalProjectionNode{
		"other_window_first":    {other, matching},
		"matching_window_first": {matching, other},
	} {
		projection := types.TraceCausalProjection{WindowStartTs: 2, WindowEndTs: 2.1, OnChainCauses: pool}
		if got := measuredFactEvidenceHMC376(traceDecisionActualOccupancyCandidates(projection, 8)); got != "view-report-window" {
			t.Errorf("%s: measured list=%q, want the publication of the reported window", name, got)
		}
		if got := measuredFactEvidenceHMC376(traceFinalReaderActualOccupancyCandidates(projection, 6)); got != "view-report-window" {
			t.Errorf("%s: reader card=%q, want the publication of the reported window", name, got)
		}
	}
}

// The reader's symptom filter also judges each publication: removing the
// target's unpriced view leaves its positive ranked seat of the same account.
func TestHMC376ReaderSymptomFilterKeepsTheRankedSeatOfTheAccount(t *testing.T) {
	const key = "state_account:v2:account"
	view := measuredFactViewHMC376("target-view", 30, key)
	seat := measuredFactSeatHMC376("target-seat", 2, 10, 30, key)
	projection := types.TraceCausalProjection{
		TargetStateAccount: &types.TraceCausalProjectionTargetStateAccount{Subject: "dep-21", TotalMS: 60},
		OnChainCauses:      []types.TraceCausalProjectionNode{view, seat},
	}
	if got := measuredFactEvidenceHMC376(traceFinalReaderActualOccupancyCandidates(projection, 6)); got != "target-seat" {
		t.Fatalf("reader card=%q, want the ranked seat", got)
	}
	// The measured list states the same account once, on the ranked seat.
	if got := measuredFactEvidenceHMC376(traceDecisionActualOccupancyCandidates(projection, 8)); got != "target-seat" {
		t.Fatalf("measured list=%q, want the ranked seat", got)
	}
}

// One account keeps the list position of its best publication. A seat priced
// far below its measurement cannot sink the measurement out of the cut, and
// every other row keeps the order it has without the identity.
func TestHMC376AccountKeepsTheListPositionOfItsBestPublication(t *testing.T) {
	const key = "state_account:v2:account"
	seat := measuredFactSeatHMC376("seat", 1, 0.4, 40, key)
	view := measuredFactViewHMC376("view", 40, key)
	projection := types.TraceCausalProjection{PrimaryRootCauses: []types.TraceCausalProjectionNode{seat}}
	projection.OnChainCauses = append(projection.OnChainCauses, view)
	for i := 1; i <= 8; i++ {
		other := measuredFactViewHMC376(fmt.Sprintf("row-%d", i), float64(i), fmt.Sprintf("state_account:v2:other-%d", i))
		other.Subject = fmt.Sprintf("other-%d", i)
		projection.OnChainCauses = append(projection.OnChainCauses, other)
	}
	if got, want := measuredFactEvidenceHMC376(traceDecisionActualOccupancyCandidates(projection, 8)), "seat,row-8,row-7,row-6,row-5,row-4,row-3,row-2"; got != want {
		t.Errorf("measured list=%q, want %q", got, want)
	}
	if got, want := measuredFactEvidenceHMC376(traceFinalReaderActualOccupancyCandidates(projection, 6)), "seat,row-8,row-7,row-6,row-5,row-4"; got != want {
		t.Errorf("reader card=%q, want %q", got, want)
	}
	// Without the identity both publications stay, each at its own position.
	bare := projection
	bare.PrimaryRootCauses = []types.TraceCausalProjectionNode{measuredFactSeatHMC376("seat", 1, 0.4, 40, "")}
	bare.OnChainCauses = append([]types.TraceCausalProjectionNode{measuredFactViewHMC376("view", 40, "")}, projection.OnChainCauses[1:]...)
	if got, want := measuredFactEvidenceHMC376(traceDecisionActualOccupancyCandidates(bare, 0)), "view,row-8,row-7,row-6,row-5,row-4,row-3,row-2,row-1,seat"; got != want {
		t.Errorf("without identity=%q, want %q", got, want)
	}
}

// The scope one record proves reaches another publication of the same account
// only through the producer's identity, inside one artifact, for one subject
// and one half-open range. Every other record leaves the neutral wording.
func TestHMC376MeasuredFactScopeJoinsOnlyTheSameAccount(t *testing.T) {
	const key, other = "state_account:v2:account", "state_account:v2:other"
	node := types.TraceCausalProjectionNode{
		EvidenceID: "own", Subject: "dep-21", Predicate: "root_cause_primary", StateKind: "runnable",
		RunnableMS: 45, StartTs: 2, EndTs: 2.06, MeasuredStateAccountKey: key,
	}
	record := func(id, artifact, subject string, start, end float64, keyNote, source string) types.ObservationRecord {
		return types.ObservationRecord{
			ID: id, Subject: subject, Predicate: "wakeup_causal_impact", Producer: "trace_query",
			SourceRef: types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: artifact},
			Span:      types.ObservationSpan{StartTs: start, EndTs: end},
			RichNotes: []string{keyNote, types.TraceNoteKeySource + "=" + source},
		}
	}
	// The node's own record proves no scope of its own, so whatever the
	// resolver returns can only come from the sibling under test.
	own := types.ObservationRecord{ID: "own", Subject: "dep-21", Predicate: "root_cause_primary", Producer: "trace_query",
		SourceRef: types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: "/traces/a.systrace"}}
	const stateKey, measuredKey = types.TraceNoteKeyStateAccountKey + "=", types.TraceNoteKeyMeasuredStateAccountKey + "="
	sibling := record("view", "/traces/a.systrace", "dep-21", 2, 2.06, stateKey+key, "")
	role := types.TraceObservationMeasurementWindowDisplayRole(sibling, false)
	if role == "" {
		t.Fatal("the fixture record must prove a scope")
	}
	drilldown := record("drill", "/traces/a.systrace", "dep-21", 2, 2.06, stateKey+key, "")
	drilldown.Predicate = "state_drilldown"
	if got := types.TraceObservationMeasurementWindowDisplayRole(drilldown, false); got == "" || got == role {
		t.Fatalf("the disagreement case needs a second scope, got %q beside %q", got, role)
	}
	measuredSibling := record("view", "/traces/a.systrace", "dep-21", 2, 2.06, measuredKey+key, "")
	for _, tc := range []struct {
		name    string
		node    types.TraceCausalProjectionNode
		records []types.ObservationRecord
		want    string
	}{
		{"same account through the pricing note", node, []types.ObservationRecord{own, sibling}, role},
		{"same account through the measured note", node, []types.ObservationRecord{own, measuredSibling}, role},
		{"another artifact", node, []types.ObservationRecord{own, record("view", "/traces/b.systrace", "dep-21", 2, 2.06, stateKey+key, "")}, ""},
		{"another subject", node, []types.ObservationRecord{own, record("view", "/traces/a.systrace", "dep-22", 2, 2.06, stateKey+key, "")}, ""},
		{"another start", node, []types.ObservationRecord{own, record("view", "/traces/a.systrace", "dep-21", 2.01, 2.06, stateKey+key, "")}, ""},
		{"another end", node, []types.ObservationRecord{own, record("view", "/traces/a.systrace", "dep-21", 2, 2.05, stateKey+key, "")}, ""},
		{"another account", node, []types.ObservationRecord{own, record("view", "/traces/a.systrace", "dep-21", 2, 2.06, stateKey+other, "")}, ""},
		{"no account on the record", node, []types.ObservationRecord{own, record("view", "/traces/a.systrace", "dep-21", 2, 2.06, "unrelated=1", "")}, ""},
		{"records disagree on the scope", node, []types.ObservationRecord{own, sibling, drilldown}, ""},
		{"records disagree in the other order", node, []types.ObservationRecord{own, drilldown, sibling}, ""},
		{"the row has no account", func() types.TraceCausalProjectionNode { n := node; n.MeasuredStateAccountKey = ""; return n }(), []types.ObservationRecord{own, sibling}, ""},
		{"the row has no published occupancy", func() types.TraceCausalProjectionNode { n := node; n.RunnableMS = 0; return n }(), []types.ObservationRecord{own, sibling}, ""},
		{"the row has no record of its own", node, []types.ObservationRecord{sibling}, ""},
		{"no artifact identifies the row or the record", node, []types.ObservationRecord{record("view", "", "dep-21", 2, 2.06, stateKey+key, "")}, ""},
		{"the row's id belongs to two artifacts", node, []types.ObservationRecord{own, func() types.ObservationRecord {
			twin := own
			twin.SourceRef.Path = "/traces/b.systrace"
			return twin
		}(), sibling}, ""},
	} {
		if got := traceDecisionMeasuredFactScope(tc.node, tc.records, false); got != tc.want {
			t.Errorf("%s: scope=%q, want %q", tc.name, got, tc.want)
		}
	}
}
