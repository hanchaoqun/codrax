package tool

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// measuredFactPublicResultsHMC376 runs the two public queries a causal
// investigation makes for one target and window.
func measuredFactPublicResultsHMC376(t *testing.T, path string, pid int, start, end float64) []types.ToolResult {
	t.Helper()
	dir := t.TempDir()
	var results []types.ToolResult
	for _, view := range []string{"wakeup_chain", "root_cause_rank"} {
		params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": view, "pid": pid,
			"time_start": start, "time_end": end, "trace_flavor": "harmony_hitrace"})
		r, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
		if err != nil || !r.Success {
			t.Fatalf("public %s: %v %s", view, err, r.Summary)
		}
		results = append(results, r)
	}
	return results
}

// measuredFactOccupancyHMC376 enters the real materialization entry and
// returns the automatic occupancy table with the compiled projection before
// and after it, so the test can prove the display did not touch the seats.
func measuredFactOccupancyHMC376(t *testing.T, results []types.ToolResult, lang string) (types.AnswerBlock, types.TraceCausalProjection, []byte, []byte) {
	t.Helper()
	bus := newBusForMutationTest()
	bus.Language = lang
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck}}
	bus.ToolResults = results
	compile := func() (types.TraceCausalProjection, []byte) {
		set := types.CompileTraceCausalProjectionSet(types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit)))
		if len(set.Projections) != 1 {
			t.Fatalf("projections=%d, want one artifact projection", len(set.Projections))
		}
		encoded, _ := json.Marshal(set.Projections[0])
		return set.Projections[0], encoded
	}
	projection, before := compile()
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "model_conclusion", Kind: types.BlockSection, Title: "模型结论", Text: "原始工作量和按规则估计的改进机会需要分别解释。"}}}
	if !materializeRuntimeTraceCausalProjectionBlock(doc, bus) {
		t.Fatal("real materialization entry must publish the typed trace projection")
	}
	_, after := compile()
	for _, block := range doc.Blocks {
		if block.ID == runtimeTraceCausalProjectionBlockIDBase+runtimeTraceCausalProjectionOccupancySuffix {
			return block, projection, before, after
		}
	}
	t.Fatal("automatic occupancy table missing")
	return types.AnswerBlock{}, projection, nil, nil
}

// measuredFactPathRowsHMC376 returns the critical-path rows of the occupancy
// table (the first column group) as typed cells.
func measuredFactPathRowsHMC376(block types.AnswerBlock, zh bool) [][]string {
	var rows [][]string
	for _, item := range block.Items {
		if len(item.Cells) == 7 && item.Cells[0] == runtimeTraceOccupancyGroupLabel("path", zh) {
			rows = append(rows, item.Cells)
		}
	}
	return rows
}

func measuredFactRowsForHMC376(rows [][]string, subject string) [][]string {
	var out [][]string
	for _, row := range rows {
		if row[1] == subject {
			out = append(out, row)
		}
	}
	return out
}

func measuredFactMSHMC376(v float64) string { return fmt.Sprintf("%.3f", v) }

func measuredFactProjectionNodesHMC376(projection types.TraceCausalProjection) []types.TraceCausalProjectionNode {
	var nodes []types.TraceCausalProjectionNode
	for _, group := range [][]types.TraceCausalProjectionNode{
		projection.PrimaryRootCauses, projection.RankedSeats, projection.OnChainCauses,
		projection.SupportingHops, projection.AdjacentCauses, projection.BackgroundCauses,
	} {
		nodes = append(nodes, group...)
	}
	return nodes
}

// One native scheduler-state account is one row of the automatic occupancy
// table, however many typed publications (a priced rank seat and the wakeup
// view of the same wait) carry it. The estimate and the measurement stay
// separate facts: the seat keeps its price, the view keeps its measurement,
// and only the duplicated display row disappears.
func TestHMC376OccupancyShowsEachMeasuredStateAccountOncePublic(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/real_traces/donghu_tieba_frame.systrace")
	results := measuredFactPublicResultsHMC376(t, path, 59566, 34579.490, 34579.500)
	// The identity travels on typed records only: one rank row publishes it,
	// and it is not part of the text the exploring model reads.
	published := 0
	for _, result := range results {
		if strings.Contains(result.Summary, types.TraceNoteKeyMeasuredStateAccountKey) || strings.Contains(result.Summary, "state_account:v2:") {
			t.Fatalf("%s: the account identity reached the model-facing result text", result.ToolName)
		}
		for _, record := range result.Observations {
			pricing, measured := 0, 0
			for _, note := range record.RichNotes {
				switch {
				case strings.HasPrefix(note, types.TraceNoteKeyStateAccountKey+"="):
					pricing++
				case strings.HasPrefix(note, types.TraceNoteKeyMeasuredStateAccountKey+"="):
					measured++
				}
			}
			if pricing+measured > 1 {
				t.Fatalf("%s carries the account identity more than once: %q", record.ID, record.RichNotes)
			}
			published += measured
		}
	}
	if published == 0 {
		t.Fatal("no rank row published a measured-account identity; the fixture no longer exercises this class")
	}
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			zh := lang == "zh"
			block, projection, before, after := measuredFactOccupancyHMC376(t, results, lang)
			rows := measuredFactPathRowsHMC376(block, zh)
			if len(rows) == 0 {
				t.Fatalf("occupancy table has no critical-path row: %+v", block.Items)
			}
			seen := map[string]int{}
			for _, row := range rows {
				seen[strings.Join(row, " | ")]++
			}
			for row, n := range seen {
				if n > 1 {
					t.Errorf("identical critical-path row shown %d times: %s", n, row)
				}
			}
			runnable := measuredFactRowsForHMC376(rows, "NetworkService-60595 / runnable")
			if len(runnable) != 1 || runnable[0][2] != "5.930ms" {
				t.Errorf("want exactly one measured runnable row of 5.930ms for NetworkService-60595: %q", runnable)
			} else if !strings.Contains(runnable[0][6], "5.951ms") {
				t.Errorf("the separately priced estimate must stay in the caption, never in the value cell: %q", runnable[0])
			}
			sleep := measuredFactRowsForHMC376(rows, "CookieMonsterCl-59843 / sleep")
			if len(sleep) != 1 || sleep[0][2] != "6.406ms" {
				t.Errorf("want exactly one measured sleep row of 6.406ms for CookieMonsterCl-59843: %q", sleep)
			}
			// The table holds five critical-path rows. A duplicate takes a seat
			// from a real on-chain measurement of the analysed thread itself.
			running := measuredFactRowsForHMC376(rows, "com.baidu.tieba-59566 / running")
			if len(running) != 1 || running[0][2] != "1.156ms" {
				t.Errorf("the target thread's own on-chain running time must stay on the table: %q in %q", running, rows)
			}
			for _, row := range rows {
				if row[2] == "5.951ms" {
					t.Errorf("a priced estimate was shown as measured occupancy: %q", row)
				}
			}

			// Protected: the causal projection, its seats and its prices.
			if !bytes.Equal(before, after) {
				t.Fatal("occupancy display mutated the causal projection")
			}
			var seat, view *types.TraceCausalProjectionNode
			nodes := measuredFactProjectionNodesHMC376(projection)
			for i := range nodes {
				n := &nodes[i]
				if n.Subject != "NetworkService-60595" {
					continue
				}
				// EVOLUTION RECORD (HMC377 R-377-4): the thread's ready time
				// outside its dependency windows now rides a ◇ remainder row
				// whose adjacent-channel ordinal is also 1; the priced seat is
				// the CHAIN-channel rank 1.
				switch {
				case n.Rank == 1 && n.Predicate != "wakeup_causal_impact" && n.ChainRelevance == "on_chain":
					seat = n
				case n.Predicate == "wakeup_causal_impact":
					view = n
				}
			}
			if seat == nil || view == nil {
				t.Fatalf("projection must keep both the priced seat and the measured wakeup view: seat=%v view=%v", seat != nil, view != nil)
			}
			if got := measuredFactMSHMC376(seat.ImpactMS); got != "5.951" || seat.StateAccountKey != "" || seat.DuplicatePublications != 0 || len(seat.MergedEvidenceIDs) != 0 {
				t.Fatalf("priced seat changed or absorbed the measurement: impact=%s pricing_key=%q duplicates=%d merged=%v", got, seat.StateAccountKey, seat.DuplicatePublications, seat.MergedEvidenceIDs)
			}
			if got := measuredFactMSHMC376(view.ImpactMS); got != "5.930" || view.StateAccountKey == "" || view.DuplicatePublications != 0 {
				t.Fatalf("measured wakeup view changed: impact=%s key=%q duplicates=%d", got, view.StateAccountKey, view.DuplicatePublications)
			}
			cluster := runtimeTraceCausalProjectionClusterFor(projection, map[bool]string{true: "zh-CN", false: "en"}[zh], runtimeTraceProjUserFocus{}, "hmc376", "")
			priced := false
			for _, b := range cluster {
				priced = priced || b.ID == "hmc376" && strings.Contains(b.Text, "5.951ms")
			}
			if !priced {
				t.Fatal("the eliminable board must keep the original priced estimate")
			}
		})
	}
}

// The same class on a complete user capture: the seat is priced at 36.311 ms,
// the measured runnable account of the same thread is 45.378 ms.
func TestHMC376HoneybirdOccupancyShowsMeasuredRunnableOnce(t *testing.T) {
	path := os.Getenv("CODRAX_HMC365_HONEYBIRD")
	if path == "" {
		t.Skip("set CODRAX_HMC365_HONEYBIRD to the pinned capture")
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.New()
	size, err := io.Copy(digest, file)
	_ = file.Close()
	if err != nil || size != 106896120 || hex.EncodeToString(digest.Sum(nil)) != "21280a96618781b37be37f9289be9ba286d6a85d358e60ff72d9c73c827cf30e" {
		t.Fatalf("pinned capture differs: size=%d sha256=%s err=%v", size, hex.EncodeToString(digest.Sum(nil)), err)
	}
	results := measuredFactPublicResultsHMC376(t, path, 64156, 11343.248, 11343.310)
	block, projection, before, after := measuredFactOccupancyHMC376(t, results, "zh")
	rows := measuredFactPathRowsHMC376(block, true)
	got := measuredFactRowsForHMC376(rows, "Signal Catcher-64176 / runnable")
	if len(got) != 1 || got[0][2] != "45.378ms" || !strings.Contains(got[0][6], "36.311ms") {
		t.Fatalf("want exactly one measured runnable row of 45.378ms whose caption keeps the 36.311ms estimate: %q", got)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("occupancy display mutated the causal projection")
	}
	// Protected: the four on-chain seats keep their prices, and the adjacent
	// whole-window runnable account of the same thread (a different native
	// inventory) is neither promoted nor merged.
	var onChain, adjacent []string
	for _, n := range projection.RankedSeats {
		if n.ChainRelevance == "on_chain" {
			onChain = append(onChain, measuredFactMSHMC376(n.EffectiveImpactMS))
		} else {
			adjacent = append(adjacent, n.ChainRelevance+" "+measuredFactMSHMC376(n.ImpactMS))
		}
	}
	// EVOLUTION RECORD (HMC377 R-377-4, owner ruling "one physical thread,
	// one object"): the adjacent whole-window account (45.878 ms) of the
	// on-chain waker 64176 is no longer a second seat — it rides the chain
	// seat as lossless detail — and only its ready time outside the
	// dependency window remains adjacent: 45.878 − 45.378 = 0.500 ms (raw).
	if strings.Join(onChain, "/") != "36.311/3.868/2.600/0.013" || strings.Join(adjacent, "/") != "adjacent 0.500" {
		t.Fatalf("seats changed: on-chain=%v other=%v", onChain, adjacent)
	}
	if len(measuredFactRowsForHMC376(rows, "ele.crowdsource-64156 / sleep")) != 1 || len(measuredFactRowsForHMC376(rows, "ele.crowdsource-64156 / running")) != 1 {
		t.Fatalf("the target thread's own on-chain rows must stay on the table: %q", rows)
	}
}

// A multi-occurrence account reaches the table as the rank seat built from its
// aggregate and as the aggregate row itself. Both show the same exact segments,
// so they are one row, and the five-row table keeps room for other accounts.
func TestHMC376OccupancyShowsMultiOccurrenceAccountOncePublic(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/real_traces/donghu_tieba_frame.systrace")
	results := measuredFactPublicResultsHMC376(t, path, 59566, 34579.480, 34579.510)
	for _, result := range results {
		if strings.Contains(result.Summary, types.TraceNoteKeyMeasuredOccurrenceGroupKey) {
			t.Fatalf("%s: the occurrence group identity reached the model-facing result text", result.ToolName)
		}
	}
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			zh := lang == "zh"
			block, projection, before, after := measuredFactOccupancyHMC376(t, results, lang)
			rows := measuredFactPathRowsHMC376(block, zh)
			seen := map[string]int{}
			for _, row := range rows {
				seen[row[1]+" | "+row[2]]++
			}
			for row, n := range seen {
				if n > 1 {
					t.Errorf("one measured account shown %d times: %s", n, row)
				}
			}
			for _, want := range [][2]string{
				{"NetworkService-60595 / runnable", "13.699ms"},
				{"CookieMonsterCl-59843 / sleep", "15.038ms"},
				// Evicted from the five-row table while the two accounts above
				// took two rows each.
				{"com.baidu.tieba-59566 / running", "3.169ms"},
				{"CookieMonsterCl-59843 / runnable", "2.154ms"},
			} {
				got := measuredFactRowsForHMC376(rows, want[0])
				if len(got) != 1 || got[0][2] != want[1] {
					t.Errorf("want one %s row of %s, got %q in %q", want[0], want[1], got, rows)
				}
			}
			network := measuredFactRowsForHMC376(rows, "NetworkService-60595 / runnable")
			if len(network) == 1 && !strings.Contains(network[0][6], "13.737ms") {
				t.Errorf("the separately priced estimate must stay in the caption: %q", network[0])
			}

			// Protected: the seat keeps its price, the aggregate and the fold of
			// the occurrences stay in the projection, and nothing is absorbed.
			if !bytes.Equal(before, after) {
				t.Fatal("occupancy display mutated the causal projection")
			}
			var seat, aggregate, fold *types.TraceCausalProjectionNode
			nodes := measuredFactProjectionNodesHMC376(projection)
			for i := range nodes {
				n := &nodes[i]
				if n.Subject != "NetworkService-60595" || n.StateKind != "runnable" {
					continue
				}
				// EVOLUTION RECORD (HMC377 R-377-4): see above — the chain-
				// channel rank 1, not the ◇ remainder's adjacent ordinal 1.
				switch {
				case n.Rank == 1 && n.ChainRelevance == "on_chain":
					seat = n
				case n.Predicate == "wakeup_causal_aggregate":
					aggregate = n
				case n.Predicate == "wakeup_causal_impact" && n.MergedCount > 1:
					fold = n
				}
			}
			if seat == nil || fold == nil {
				t.Fatalf("projection must keep the seat and the occurrence fold: seat=%v fold=%v", seat != nil, fold != nil)
			}
			if got := measuredFactMSHMC376(seat.ImpactMS); got != "13.737" || seat.StateAccountKey != "" {
				t.Fatalf("priced seat changed: impact=%s pricing_key=%q", got, seat.StateAccountKey)
			}
			if seat.MeasuredStateFactKey() == "" || seat.MeasuredStateFactKey() != fold.MeasuredStateFactKey() {
				t.Fatalf("seat account %q and fold account %q must be the producer's one identity", seat.MeasuredStateFactKey(), fold.MeasuredStateFactKey())
			}
			if !types.TraceMeasuredStateFactSame(*seat, *fold) {
				t.Fatal("the seat and the fold of every occurrence show one measured fact")
			}
			if aggregate != nil && aggregate.MeasuredStateFactKey() != seat.MeasuredStateFactKey() {
				t.Fatalf("aggregate account %q differs from the seat's %q", aggregate.MeasuredStateFactKey(), seat.MeasuredStateFactKey())
			}
		})
	}
}
