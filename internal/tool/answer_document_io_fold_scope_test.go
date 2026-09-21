package tool

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func ioFoldScopeCompact(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || strings.ContainsRune("│┃", r) {
			return -1
		}
		return r
	}, s)
}

func TestIOFoldScopePublicQueriesKeepPeerRanges(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "eval", "fixtures", "hmosperf_business_io_chain", "events.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	sawDifferentQuery := false
	for _, renamed := range []bool{false, true} {
		t.Run(fmt.Sprintf("renamed=%t", renamed), func(t *testing.T) {
			trace := string(body)
			if renamed {
				trace = strings.NewReplacer("backup", "archiver", "document-worker", "loader", "app-main", "client").Replace(trace)
			}
			path := hmc081WriteTrace(t, trace)
			bus, _, _ := hmc17NamedPathContext(t)
			var records []types.ObservationRecord
			for _, window := range [][2]float64{{1, 1.051}, {1.004, 1.05}} {
				for _, view := range []string{"window_stats", "wakeup_chain", "root_cause_rank", "critical_blocking_calls"} {
					result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": view, "pid": 100, "time_start": window[0], "time_end": window[1]})
					if !result.Success {
						t.Fatalf("public %s query: %s", view, result.Summary)
					}
					records = append(records, result.Observations...)
				}
			}
			before, _ := json.Marshal(records)
			nativeByID := map[string]types.ObservationRecord{}
			for _, record := range records {
				nativeByID[record.ID] = record
			}
			set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: records})
			for _, lang := range []string{"zh", "en"} {
				md := p3mRenderUserFace(t, records, lang)
				old, want := "同段IO", "同线程IO证据组"
				if lang == "en" {
					old, want = "same-segment IO", "same-thread IO evidence group"
				}
				if strings.Contains(md, old) {
					t.Errorf("public query→mutation→renderer promoted a locator group to one physical IO segment (%s)", lang)
				}
				if !strings.Contains(md, old) && !strings.Contains(md, want) {
					t.Fatalf("fixture must reach an actual IO display fold (%s): %s", lang, md)
				}
				if !strings.Contains(md, want) {
					t.Errorf("public display must preserve a compact neutral evidence group (%s)", lang)
				}
				checked, otherQuery := 0, false
				for _, projection := range set.Projections {
					evidence := newRuntimeTraceCausalProjectionEvidenceIndex()
					model := buildRuntimeTraceProjTreeModel(projection, evidence, lang == "zh")
					nodes := append(append(append([]types.TraceCausalProjectionNode(nil), projection.OnChainCauses...), projection.AdjacentCauses...), projection.BackgroundCauses...)
					rows := append(append(append(append([]runtimeTraceProjTreeRow(nil), model.TreeRows...), model.SelfRows...), model.Adjacent...), model.Background...)
					for _, row := range rows {
						for _, peer := range row.IOFoldPeers {
							for _, node := range nodes {
								if !evidence.has(node) || runtimeTraceProjEvidenceTag(node, evidence, lang == "zh") != peer.EvidenceTag {
									continue
								}
								// Exact record ID resolves the native source. Expected values
								// and ranges below do not read the new peer scope fields.
								record, ok := nativeByID[node.EvidenceID]
								value, err := strconv.ParseFloat(record.Value, 64)
								qs, qe, queryOK := types.TraceCausalProjectionSelectedWindowNote(record.RichNotes)
								if !ok || err != nil || record.Unit != "ms" || !queryOK || !types.TraceCausalProjectionWindowPresent(record.Span.StartTs, record.Span.EndTs) || value != node.ImpactMS {
									continue
								}
								loc, query := "locator range", "query range"
								if lang == "zh" {
									loc, query = "定位范围", "查询范围"
								}
								pair := fmt.Sprintf("%.3fms [%s] (%s %.6f–%.6fs; %s %.6f–%.6fs)", value, peer.EvidenceTag, loc, record.Span.StartTs, record.Span.EndTs, query, qs, qe)
								if !strings.Contains(ioFoldScopeCompact(md), ioFoldScopeCompact(pair)) {
									t.Errorf("native record %s lost its own value/evidence/range pairing in final %s output: %s", record.ID, lang, pair)
								}
								checked++
								otherQuery = otherQuery || qs != row.Node.QueryWindowStartTs || qe != row.Node.QueryWindowEndTs
							}
						}
					}
				}
				sawDifferentQuery = sawDifferentQuery || otherQuery
				if checked == 0 {
					t.Fatal("fixture must pair at least one native folded record by exact evidence identity")
				}
			}
			after, _ := json.Marshal(records)
			if string(before) != string(after) {
				t.Fatal("display mutated public observations")
			}
		})
	}
	if !sawDifferentQuery {
		t.Fatal("the public fixture must include a folded native record from a query range different from its seat")
	}
}

func TestIOFoldScopePublicArtifactsRemainSeparated(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "eval", "fixtures", "hmosperf_business_io_chain", "events.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	bus, _, _ := hmc17NamedPathContext(t)
	paths := []string{hmc081WriteTrace(t, string(body)), hmc081WriteTrace(t, string(body))}
	var records []types.ObservationRecord
	for _, path := range paths {
		for _, view := range []string{"wakeup_chain", "root_cause_rank", "critical_blocking_calls"} {
			result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": view, "pid": 100, "time_start": 1, "time_end": 1.051})
			if !result.Success {
				t.Fatal(result.Summary)
			}
			records = append(records, result.Observations...)
		}
	}
	for _, reverse := range []bool{false, true} {
		ordered := append([]types.ObservationRecord(nil), records...)
		if reverse {
			for i, j := 0, len(ordered)-1; i < j; i, j = i+1, j-1 {
				ordered[i], ordered[j] = ordered[j], ordered[i]
			}
		}
		set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: ordered})
		if len(set.Projections) != 2 {
			t.Fatalf("same-name subjects in actual distinct captures must stay separate: %d", len(set.Projections))
		}
		for _, projection := range set.Projections {
			for _, node := range append(append(append([]types.TraceCausalProjectionNode(nil), projection.OnChainCauses...), projection.AdjacentCauses...), projection.BackgroundCauses...) {
				for _, origin := range node.MeasurementOrigins {
					if origin.SourceRef.Path != "" && origin.SourceRef.Path != projection.ArtifactPath {
						t.Fatalf("native artifact partition borrowed an observation: projection=%q source=%q", projection.ArtifactPath, origin.SourceRef.Path)
					}
				}
			}
		}
	}
}

// A synthetic projection exercises display capacity independently of the
// public query fixture above. All 40 peers must survive the actual tree,
// lossless detail and final Markdown renderer, without raising any row cap.
func TestIOFoldScopeManyPeersRemainReachable(t *testing.T) {
	nodes := []types.TraceCausalProjectionNode{smr1N1BackgroundIONode("seat", "io_wait", 100, 10, 11, 1, 100)}
	for i := 1; i <= 40; i++ {
		node := smr1N1BackgroundIONode(fmt.Sprintf("peer-%d", i), "io_latency", 1+float64(i)/1000, 10+float64(i)/1000, 10.5, i+100, i+101)
		node.QueryWindowStartTs, node.QueryWindowEndTs = 9, 12+float64(i)/1000
		nodes = append(nodes, node)
	}
	for _, zh := range []bool{false, true} {
		model := buildRuntimeTraceProjTreeModel(types.TraceCausalProjection{WakeupPath: []string{"waker-1", "target-1"}, BackgroundCauses: nodes}, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		if len(model.Background) != 1 || len(model.Background[0].IOFoldPeers) != 40 {
			t.Fatal("display capacity fixture lost the one-seat compact group")
		}
		tree, detail := runtimeTraceProjTreeFence(model, zh), runtimeTraceProjDetailFullText(model, zh)
		lang := "en"
		if zh {
			lang = "zh"
		}
		md := render.RenderAnswerDocument(&types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "tree", Kind: types.BlockSection, Text: tree}, {ID: "detail", Kind: types.BlockSection, Text: detail}}}, lang)
		if strings.Count(tree, "\n") < 40 {
			t.Fatal("fixture must pressure normal line budgets")
		}
		for face, text := range map[string]string{"tree": tree, "detail": detail, "final renderer": md} {
			for _, peer := range model.Background[0].IOFoldPeers {
				pair := fmt.Sprintf("%.3fms [%s] ", peer.ImpactMS, peer.EvidenceTag) + runtimeTraceProjIOFoldScopeText(peer, zh)
				if !strings.Contains(ioFoldScopeCompact(text), ioFoldScopeCompact(pair)) {
					t.Errorf("%s elided a load-bearing member at capacity: %s", face, pair)
				}
			}
		}
	}
}

func TestIOFoldScopePrivatePeerOwnsRangesAndOrigins(t *testing.T) {
	node := smr1N1BackgroundIONode("peer", "io_latency", 3, 0, .003, 1, 3)
	node.QueryWindowStartTs, node.QueryWindowEndTs = 0, .01
	node.MeasurementOrigins = b1638b3TestOrigins("peer-query")
	peer := runtimeTraceProjNewIOFoldPeer(node, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	if peer.StartTs != 0 || peer.EndTs != .003 || peer.QueryWindowStartTs != 0 || peer.QueryWindowEndTs != .01 || !reflect.DeepEqual(peer.MeasurementOrigins, node.MeasurementOrigins) {
		t.Fatalf("factory lost the peer's own scopes/origins: %+v", peer)
	}
	peer.MeasurementOrigins[0].SourceRef.Path = "other"
	peer.MeasurementOrigins[0].MeasurementSources.Domains[0].PartitionID = "other"
	if node.MeasurementOrigins[0].SourceRef.Path == "other" || node.MeasurementOrigins[0].MeasurementSources.Domains[0].PartitionID == "other" {
		t.Fatal("private receipt aliases mutable native origins")
	}
	for _, bounds := range [][2]float64{{0, 0}, {2, 1}, {math.NaN(), 1}, {0, math.Inf(1)}} {
		peer.StartTs, peer.EndTs = bounds[0], bounds[1]
		peer.QueryWindowStartTs, peer.QueryWindowEndTs = bounds[0], bounds[1]
		for _, zh := range []bool{false, true} {
			got := runtimeTraceProjIOFoldScopeText(peer, zh)
			want := "(locator range unpublished; query range unpublished)"
			if zh {
				want = "(定位范围未发布; 查询范围未发布)"
			}
			if got != want {
				t.Fatalf("missing/invalid peer bounds must not fabricate a range: %q", got)
			}
		}
	}
}

func TestIOFoldScopeTransitiveLocatorsAreNotOccurrenceIdentity(t *testing.T) {
	for _, zh := range []bool{false, true} {
		for _, subject := range []string{"worker-9", "arbitrary-name-73"} {
			t.Run(fmt.Sprintf("zh=%t/%s", zh, subject), func(t *testing.T) {
				nodes := []types.TraceCausalProjectionNode{
					smr1N1BackgroundIONode("a", "io_wait", 4, 10, 10.01, 10, 20),
					smr1N1BackgroundIONode("b", "io_latency", 3, 10.005, 10.025, 30, 40),
					smr1N1BackgroundIONode("c", "io_latency", 2, 10.02, 10.03, 50, 60),
				}
				for i := range nodes {
					nodes[i].Subject = subject
					nodes[i].QueryWindowStartTs, nodes[i].QueryWindowEndTs = 9+float64(i), 12+float64(i)
				}
				before, _ := json.Marshal(nodes)
				projection := types.TraceCausalProjection{WakeupPath: []string{"waker-1", "target-1"}, BackgroundCauses: nodes}
				model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
				if len(model.Background) != 1 || len(model.Background[0].IOFoldPeers) != 2 {
					t.Fatalf("keep the existing transitive compact group and row capacity: %+v", model.Background)
				}
				row := model.Background[0]
				if row.Node.ImpactMS != 4 || row.Node.Rank != 0 || row.Node.ChainRelevance != "background" {
					t.Fatalf("display changed value/rank/causal eligibility: %+v", row.Node)
				}
				for face, rendered := range map[string]string{"note": runtimeTraceProjIOFoldNoteText(row.IOFoldPeers, zh), "tree": runtimeTraceProjTreeFence(model, zh), "detail": runtimeTraceProjDetailFullText(model, zh)} {
					text := ioFoldScopeCompact(rendered)
					for i, peer := range row.IOFoldPeers {
						n := nodes[i+1]
						pair := fmt.Sprintf("%.3fms [%s]", n.ImpactMS, peer.EvidenceTag)
						loc, query := "locator range", "query range"
						if zh {
							loc, query = "定位范围", "查询范围"
						}
						pair += fmt.Sprintf(" (%s %.6f–%.6fs; %s %.6f–%.6fs)", loc, n.StartTs, n.EndTs, query, n.QueryWindowStartTs, n.QueryWindowEndTs)
						if !strings.Contains(text, ioFoldScopeCompact(pair)) {
							t.Errorf("%s must pair each peer value/evidence with its own locator and query, not the seat's: want %q\n%s", face, pair, rendered)
						}
					}
					if strings.Contains(rendered, "同段IO") || strings.Contains(rendered, "same-segment IO") {
						t.Errorf("%s falsely asserts same occurrence from A∩B/B∩C: %s", face, rendered)
					}
				}
				after, _ := json.Marshal(nodes)
				if string(before) != string(after) {
					t.Fatal("display changed original projection nodes")
				}
			})
		}
	}
}
