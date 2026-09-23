package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The engine's family fold and the display's same-thread evidence group are separate
// populations. A family may become a display peer, leaving its IO note as its
// only visible value carrier; that note must keep the family's own ruler.
func TestIOFoldFamilyMeasurementSurvivesFinalFaces(t *testing.T) {
	for _, zh := range []bool{true, false} {
		t.Run(fmt.Sprintf("zh=%t", zh), func(t *testing.T) {
			host := revisit76IONode("host", "worker-9", "io_wait", 80, 100, 200)
			host.ChainRelevance, host.ChainDepth = "background", 0
			nodes := []types.TraceCausalProjectionNode{host}
			for i, fold := range []string{"max_overlap_fallback", "interval_union", "sum_disjoint"} {
				node := revisit76IONode(fmt.Sprintf("family-%d", i), "worker-9", "io_latency", 23-float64(i), 110+i, 190-i)
				node.Predicate = "root_cause_background"
				node.ChainRelevance, node.ChainDepth = "background", 0
				node.RankValueCaliber = types.TraceRankValueCaliberNativeDuration
				node.FamilyMemberCount, node.FamilyMemberMaxMS = 2, 12
				node.FamilyFoldCaliber = fold
				if fold == "max_overlap_fallback" {
					node.FamilyMemberMaxMS = node.ImpactMS
				}
				nodes = append(nodes, node)
			}
			projection := types.TraceCausalProjection{
				WakeupPath: []string{"waker-2", "main-1"}, BackgroundCauses: nodes,
			}
			before, _ := json.Marshal(projection)
			model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
			var folded *runtimeTraceProjTreeRow
			for i := range model.Background {
				if model.Background[i].Node.EvidenceID == "host" {
					folded = &model.Background[i]
				}
			}
			if folded == nil || len(folded.IOFoldPeers) != 3 || len(model.Background) != 1 {
				t.Fatalf("fixture must actually fold three family rows into one background seat: %+v", model.Background)
			}
			if folded.Node.ImpactMS != 80 || folded.Node.EffectiveImpactMS != 0 || folded.Node.ChainRelevance != "background" || folded.Node.Rank != 0 {
				t.Fatalf("family disclosure changed host value/authority: %+v", folded.Node)
			}
			lang := "en"
			if zh {
				lang = "zh"
			}
			for face, rendered := range map[string]string{
				"note":   runtimeTraceProjIOFoldNoteText(folded.IOFoldPeers, zh),
				"tree":   runtimeTraceProjTreeFence(model, zh),
				"detail": runtimeTraceProjDetailFullText(model, zh),
			} {
				// The tree's existing line-width wrapper may split the shared
				// explanation between words; compare text independently of that
				// presentation whitespace, not against an unwrapped golden.
				compact := func(s string) string {
					return strings.Map(func(r rune) rune {
						if unicode.IsSpace(r) {
							return -1
						}
						return r
					}, s)
				}
				for _, node := range nodes[1:] {
					want := types.FormatTraceFamilyMeasurement(node.FamilyMemberCount, node.FamilyMemberMaxMS, node.FamilyFoldCaliber, lang)
					if !strings.Contains(compact(rendered), compact(want)) {
						t.Errorf("%s lost folded family's own measurement ruler %q:\n%s", face, want, rendered)
					}
					if !strings.Contains(rendered, fmt.Sprintf("%.3fms", node.ImpactMS)) {
						t.Errorf("%s lost separate family value %.3f: %s", face, node.ImpactMS, rendered)
					}
				}
				if strings.Contains(rendered, "23.000/22.000/21.000ms") {
					t.Errorf("%s collapsed distinct family rulers into one number group: %s", face, rendered)
				}
			}
			after, _ := json.Marshal(projection)
			if string(before) != string(after) {
				t.Fatal("family disclosure mutated projection facts")
			}
		})
	}
}

func TestIOFoldSingleMeasurementKeepsExistingNote(t *testing.T) {
	for _, count := range []int{0, 1} {
		for _, zh := range []bool{true, false} {
			node := types.TraceCausalProjectionNode{
				TypeToken: "io_latency", Predicate: "root_cause_background", ImpactMS: 23,
				RankValueCaliber:  types.TraceRankValueCaliberNativeDuration,
				FamilyMemberCount: count, FamilyMemberMaxMS: 23, FamilyFoldCaliber: "max_overlap_fallback",
			}
			peer := runtimeTraceProjNewIOFoldPeer(node, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
			peer.EvidenceTag = "E9"
			got := runtimeTraceProjIOFoldNoteText([]runtimeTraceProjIOFoldPeer{peer}, zh)
			want := "same-thread IO evidence group observed duration·I/O duration (measurement unspecified) 23.000ms [E9] (locator range unpublished; query range unpublished)"
			if zh {
				want = "同线程IO证据组 观测计时·IO观测时长（口径未明确） 23.000ms [E9] (定位范围未发布; 查询范围未发布)"
			}
			if got != want {
				t.Errorf("count=%d zh=%t changed single-record output:\ngot %s\nwant %s", count, zh, got, want)
			}
		}
	}
}
