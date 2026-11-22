package tool

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 round 11 (batch-owner ruling R-377-11 ②, 2026-11-21): one carrier per
// instant of a thread's ready time on ◎, the tree and the native board, at
// the public trace_query entry. In-repo donghu fixture, board of
// HiPlayer_79_DEM-17267 [13762.791708, 13763.024898]: JankManager-9655's
// cpu_affinity_or_cpuset seat keeps the 25.748ms of its restricted ready time
// outside the chain seat's walked window (1.759ms inside it stays with the
// chain seat), and the thread's ◇ runnable remainder carries the other
// 3.684ms of its 29.432ms rest, naming the restriction row by its [E#].
// Base (b2325eba6): remainder 29.432 + restriction 27.507 + chain 1.759 on
// 31.191ms of ready time (raw rawpop.py: 27.507ms of closed segments + a
// 3.684ms segment open at the trace end), the same instants on two rows.
func TestHMC377R11RemainderNamesTheRestrictionRowByEvidenceNumber(t *testing.T) {
	const fixture = "../../eval/fixtures/real_traces/donghu.ftrace"
	if _, err := os.Stat(fixture); err != nil {
		t.Skipf("fixture not present: %v", err)
	}
	result, native := hmc376QueryTarget(t, fixture, "root_cause_rank", map[string]any{"pid": 17267, "target_scope": "thread"}, 13762.791708, 13763.024898)
	if native.RootCauseRank == nil {
		t.Fatal("no rank board")
	}
	t.Run("native board", func(t *testing.T) {
		ready := 0.0
		for _, item := range native.RootCauseRank.Items {
			if item.Thread.PID != 9655 || item.Rank <= 0 {
				continue
			}
			switch item.Type {
			case "priority_inversion_candidate":
				ready += item.GatedRunnableMs
			case "runnable_wait", "cpu_affinity_or_cpuset", "scheduler_latency", "priority_inversion_runnable_wait":
				ready += item.EffectiveImpactMs
			}
		}
		if ready <= 0 || ready > 31.191+0.003 {
			t.Fatalf("native board prices %.3fms of JankManager-9655's 31.191ms of ready time", ready)
		}
	})
	projection := hmc376Projection(result)
	t.Run("elimination board", func(t *testing.T) {
		model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
		ready := 0.0
		for _, entry := range runtimeTraceProjElimBoard(model) {
			if strings.HasSuffix(entry.row.Node.Subject, "-9655") && entry.row.Node.StateKind == "runnable" {
				ready += entry.row.Node.EffectiveImpactMS
			}
		}
		if ready <= 0 || ready > 31.191+0.003 {
			t.Fatalf("◎ population prices %.3fms of JankManager-9655's 31.191ms of ready time", ready)
		}
	})
	for _, lang := range []string{"zh", "en"} {
		t.Run("tree "+lang, func(t *testing.T) {
			var text strings.Builder
			for _, block := range runtimeTraceCausalProjectionCluster(projection, lang, runtimeTraceProjUserFocus{}) {
				text.WriteString(block.Text)
				text.WriteString("\n")
			}
			rendered := strings.Join(strings.Fields(text.String()), " ")
			restriction := regexp.MustCompile(`JankManager-9655 · CPU(?:亲和/cpuset限制| affinity…| affinity/cpuset restriction)\s*█*░*\s*25\.748ms\s+\S+\s+\[(E\d+)\]`)
			m := restriction.FindStringSubmatch(rendered)
			if m == nil {
				t.Fatalf("the restriction row must publish its own 25.748ms:\n%s", text.String())
			}
			want := "其余29.432ms(无链上凭证;其中CPU亲和/cpuset 限制下25.748ms另行计入[" + m[1] + "],本行计3.684ms)"
			split := "同源二分:全窗27.507ms=锚定1.759ms(⛓链上席)+本行其余25.748ms(无链上凭证)"
			if lang == "en" {
				want = "remainder 29.432ms (no chain credential; of which 25.748ms under the CPU affinity/cpuset restriction is counted separately on [" + m[1] + "], this row 3.684ms)"
				split = "same-source split: full-window 27.507ms=1.759ms anchored (⛓ chain seat) + this remainder 25.748ms (no chain credential)"
			}
			// The fence wraps long lines; compare on whitespace-joined text.
			joined := strings.ReplaceAll(rendered, "cpuset 限制", "cpuset限制")
			wantJoined := strings.ReplaceAll(want, "cpuset 限制", "cpuset限制")
			if !strings.Contains(joined, wantJoined) {
				t.Fatalf("the remainder must name the restriction row's share by its evidence number (%q):\n%s", want, text.String())
			}
			if !strings.Contains(rendered, split) {
				t.Fatalf("the restriction row must state its own split (%q):\n%s", split, text.String())
			}
		})
	}
}

// A row the engine absorbed into another row is that row's lossless detail and
// holds no ◎ seat of its own, even where the projection keeps it in its
// bucket because the absorbing row is not on the board (review 10, 105 元宝
// 后台销毁 #2 worker #IM-BACKGROUND--63813: the R-377-4 inversion detail's
// 0.614ms entered ◎ beside the ◇ remainder 22.543ms that already counts
// those instants, Σ 23.157 > raw ready 22.567).
func TestHMC377R11AbsorbedDetailHoldsNoEliminationSeat(t *testing.T) {
	const fixture = "../../eval/fixtures/real_traces/donghu.ftrace"
	if _, err := os.Stat(fixture); err != nil {
		t.Skipf("fixture not present: %v", err)
	}
	result, _ := hmc376QueryTarget(t, fixture, "root_cause_rank", map[string]any{"pid": 17267, "target_scope": "thread"}, 13762.791708, 13763.024898)
	projection := hmc376Projection(result)
	var host *types.TraceCausalProjectionNode
	for i := range projection.AdjacentCauses {
		node := &projection.AdjacentCauses[i]
		if node.ChainAnchorRemainderSeat && node.TypeToken == "runnable_wait" && node.EffectiveImpactMS > 0 {
			host = node
			break
		}
	}
	if host == nil {
		t.Fatal("no ◇ remainder row on the donghu board")
	}
	// The absorbed copy: the engine's typed absorption verdict on a record
	// whose absorbing family is not on this projection (fail-open bucket).
	detail := *host
	detail.EvidenceID = host.EvidenceID + "-absorbed-detail"
	detail.Predicate = "root_cause_absorbed"
	detail.AbsorbedByRankFamily = true
	detail.AbsorbedInto = "rank_pair:runnable_wait>priority_inversion_runnable_wait|cut-by-the-cap"
	detail.ChainAnchorRemainderSeat = false
	detail.ChainAnchorFullMS, detail.ChainAnchoredMS = 0, 0
	detail.TypeToken = "priority_inversion_runnable_wait"
	detail.Object = "priority_inversion_runnable_wait"
	detail.EffectiveImpactMS = 0.614
	projection.AdjacentCauses = append(projection.AdjacentCauses, detail)
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	t.Run("detail row is drawn", func(t *testing.T) {
		drawn := false
		for _, row := range model.Adjacent {
			if row.Node.EvidenceID == detail.EvidenceID {
				drawn = true
			}
		}
		if !drawn {
			t.Fatal("the absorbed record whose host is off the board must stay drawn (lossless)")
		}
	})
	t.Run("detail row holds no seat", func(t *testing.T) {
		for _, entry := range runtimeTraceProjElimBoard(model) {
			if entry.row.Node.EvidenceID == detail.EvidenceID {
				t.Fatalf("an absorbed detail row must not enter the ◎ population: %+v", entry.row.Node)
			}
		}
	})
}

// A ◇ remainder whose rest is partly counted on the same thread's
// cause-specific row no longer restores the account with its ⛓ counterpart
// alone: the pair sentence 「合计还原全窗账」 would be false, so its 行2
// sentence names the three parts instead (宁漏勿假指). Witness: the donghu
// CompThread_0-2955 D remainder (36.757 = ⛓ 3.598 + ◇ 33.159, paired at the
// base) with a typed, resolved subset share added on its record — the same
// typed claim the engine writes for a CPU-restriction carve.
func TestHMC377R11SubsetShareReplacesTheTwoSeatRestoration(t *testing.T) {
	const fixture = "../../eval/fixtures/real_traces/donghu.ftrace"
	if _, err := os.Stat(fixture); err != nil {
		t.Skipf("fixture not present: %v", err)
	}
	result, _ := hmc376QueryTarget(t, fixture, "root_cause_rank", map[string]any{"pid": 17267, "target_scope": "thread"}, 13762.791708, 13763.024898)
	render := func(projection types.TraceCausalProjection) string {
		var text strings.Builder
		for _, block := range runtimeTraceCausalProjectionCluster(projection, "zh", runtimeTraceProjUserFocus{}) {
			text.WriteString(block.Text)
			text.WriteString("\n")
		}
		return strings.Join(strings.Fields(text.String()), " ")
	}
	projection := hmc376Projection(result)
	if got := strings.Count(render(projection), "合计还原全窗账 36.757ms"); got != 2 {
		t.Fatalf("witness drifted: the CompThread_0-2955 pair must restore 36.757ms on both rows (got %d)", got)
	}
	var holder string
	for _, node := range projection.AdjacentCauses {
		if strings.HasSuffix(node.Subject, "-9655") && node.TypeToken == "cpu_affinity_or_cpuset" {
			holder = node.EvidenceID
		}
	}
	injected := false
	for i := range projection.AdjacentCauses {
		node := &projection.AdjacentCauses[i]
		if strings.HasSuffix(node.Subject, "-2955") && node.ChainAnchorRemainderSeat && node.TypeToken == "d_state_or_io_wait" {
			node.ShareClaims = append(append([]types.TraceCausalProjectionShareClaim(nil), node.ShareClaims...),
				types.TraceCausalProjectionShareClaim{Part: "subset", Role: "subset_seat", ShareMS: 10, Resolved: true, Exact: true, HolderEvidenceIDs: []string{holder}})
			injected = true
		}
	}
	if holder == "" || !injected {
		t.Fatalf("witness rows missing: holder=%q injected=%v", holder, injected)
	}
	rendered := render(projection)
	t.Run("no two-seat restoration", func(t *testing.T) {
		if strings.Contains(rendered, "合计还原全窗账 36.757ms") {
			t.Fatalf("a remainder whose rest is partly counted elsewhere must not pair as restoring the account:\n%s", rendered)
		}
	})
	t.Run("three parts named", func(t *testing.T) {
		if !regexp.MustCompile(`其余33\.159ms\(无链上凭证;其中CPU亲和/cpuset 限制下10\.000ms另行计入\[E\d+\],本行计23\.159ms\)|其余33\.159ms\(无链上凭证;其中CPU亲和/cpuset限制下10\.000ms另行计入\[E\d+\],本行计23\.159ms\)`).MatchString(rendered) {
			t.Fatalf("the remainder must name the part counted on the restriction row:\n%s", rendered)
		}
	})
}
