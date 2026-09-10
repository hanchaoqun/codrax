package tracequery

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func b1648IOClosureIndex(t *testing.T, state, closure string) *Index {
	t.Helper()
	var body strings.Builder
	body.WriteString("idle-0 (0) [001] .... 5.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=20\n")
	body.WriteString("target-41 (41) [001] .... 5.000100: block_rq_issue: 8,0 R 4096 () 123 + 8 [target]\n")
	fmt.Fprintf(&body, "target-41 (41) [001] .... 5.000120: sched_switch: prev_comm=target prev_pid=41 prev_prio=20 prev_state=%s ==> next_comm=idle next_pid=0 next_prio=120\n", state)
	body.WriteString("irq-2 (2) [001] .... 5.000200: block_rq_complete: 8,0 R () 123 + 8 [0]\n")
	if closure != "no_wakeup" {
		waker := "irq-2 (2)"
		if closure == "different_waker" {
			waker = "other-3 (3)"
		}
		fmt.Fprintf(&body, "%s [001] .... 5.000210: sched_wakeup: comm=target pid=41 prio=20 target_cpu=001\n", waker)
	}
	// A later sched-in alone cannot substitute for the absent directed wake.
	body.WriteString("idle-0 (0) [001] .... 5.000230: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=20\n")
	return buildTraceIndex(t, "b1648_io_closure.ftrace", body.String())
}

func TestB1648ActualIOClosureEvaluationDoesNotDependOnChainAnchors(t *testing.T) {
	for _, state := range []string{"S", "D"} {
		for _, closure := range []string{"closed", "no_wakeup", "different_waker"} {
			t.Run(state+"/"+closure, func(t *testing.T) {
				idx := b1648IOClosureIndex(t, state, closure)
				q := Query{View: "root_cause_rank", PID: 41, TimeStart: 5, TimeEnd: 5.001, MinDurationMs: .001, Limit: 12}
				chain := BuildWakeupChain(idx, q)
				anchors := chainAnchorWindowsByPID(chain)
				if chain.Target.PID != 41 || len(chain.Nodes) == 0 {
					t.Fatal("actual target/self-chain fixture was not constructed")
				}
				if closure == "no_wakeup" && (len(chain.Edges) != 0 || anchors != nil) {
					t.Fatalf("no-wake negative must genuinely lack dependency anchors: edges=%d anchors=%v", len(chain.Edges), anchors)
				}
				if state == "D" && closure == "closed" && anchors != nil {
					t.Fatal("positive D closure must exercise the no-dependency-anchor path")
				}
				result := Run(idx, q)
				if result.WindowStats == nil || result.RootCauseRank == nil || len(result.WindowStats.IOLatencies) != 1 {
					t.Fatal("actual public Run did not publish the IO and root-rank faces")
				}
				io := result.WindowStats.IOLatencies[0]
				proven := closure == "closed"
				if io.IssueThread.PID != 41 || io.CompleteThread.PID != 2 || io.CompletionWokeIssuer != proven ||
					fmt.Sprintf("%.3f", io.DurationMs) != "0.100" || io.IssueLine != 2 || io.CompleteLine != 4 {
					t.Fatalf("native request/proof premise changed: %+v", io)
				}
				if proven {
					wantState := string(StateSSleep)
					if state == "D" {
						wantState = string(StateDSleep)
					}
					if io.IssuerBlockedState != wantState || fmt.Sprintf("%.3f", io.IssuerBlockedMs) != "0.090" || io.WakeupLine != 5 {
						t.Fatalf("closed S/D ruler must remain independent from request residence: %+v", io)
					}
				}
				before, err := json.Marshal(result.WindowStats)
				if err != nil {
					t.Fatal(err)
				}
				for _, entry := range []struct {
					name string
					rank RootCauseRankResult
				}{{"Run", *result.RootCauseRank}, {"BuildRootCauseRank", BuildRootCauseRank(idx, q)}} {
					t.Run(entry.name, func(t *testing.T) {
						ioRows, dRows := 0, 0
						rows := append(append([]RootCauseRankItem(nil), entry.rank.Items...), entry.rank.AbsorbedItems...)
						for _, row := range rows {
							if row.Thread.PID != 41 {
								continue
							}
							if row.Type == "d_state_or_io_wait" {
								dRows++
								want := "0.090"
								if closure == "no_wakeup" {
									want = "0.110"
								}
								if fmt.Sprintf("%.3f", row.CumulativeImpactMs) != want || row.ChainRelevance != "on_chain" {
									t.Errorf("IO proof must not change the target's separately measured D state: %+v", row)
								}
							}
							if row.Type != "io_latency" {
								continue
							}
							ioRows++
							if !row.resourceClosureEvaluated {
								t.Errorf("modern IO closure was evaluated natively but rank marks it unevaluated: proof=%t anchors_nil=%t rank=%d relevance=%s", proven, anchors == nil, row.Rank, row.ChainRelevance)
							}
							if row.ResourceCompletionClosure != proven {
								t.Errorf("rank changed the physical closure result: %+v", row)
							}
							wantMS, wantStart, wantEnd := "0.100", 5.000100, 5.000200
							if proven {
								wantMS, wantStart, wantEnd = "0.090", 5.000120, 5.000210
								if row.ChainRelevance != "on_chain" || fmt.Sprintf("%.3f", row.EffectiveImpactMs) != "0.090" {
									t.Errorf("proven S/D closure lost its existing on-chain blocked ruler: %+v", row)
								}
							} else if row.ChainRelevance == "on_chain" || RootCauseRankOrdinalChannelWord(row) == "chain" || row.OnChainBasis != "" {
								t.Errorf("unproved request residence acquired target causal ranking: rank=%d relevance=%s effective=%.3f basis=%s", row.Rank, row.ChainRelevance, row.EffectiveImpactMs, row.OnChainBasis)
							}
							if fmt.Sprintf("%.3f", row.CumulativeImpactMs) != wantMS || row.StartTs != wantStart || row.EndTs != wantEnd {
								t.Errorf("proof evaluation changed the published physical measurement: %+v", row)
							}
						}
						if ioRows != 1 || state == "D" && dRows != 1 {
							t.Fatalf("original request/state records must remain auditable without minting new seats: IO=%d D=%d", ioRows, dRows)
						}
					})
				}
				after, err := json.Marshal(result.WindowStats)
				if err != nil {
					t.Fatal(err)
				}
				if string(before) != string(after) {
					t.Fatal("rank closure handling mutated the source measurement carrier")
				}
			})
		}
	}
}

func TestB1648LegacyUnevaluatedResourceAndNonIOGuardUnchanged(t *testing.T) {
	target := ThreadRef{PID: 41}
	ctx := chainCandidateContext{relevance: "on_chain", overlapMs: .090}
	for _, pid := range []int{41, 77} {
		legacy := RootCauseRankItem{Type: "io_latency", Thread: ThreadRef{PID: pid}, StartTs: 5.0001, EndTs: 5.0002}
		if got := rootCauseChainContextForItem(legacy, ctx, target); !reflect.DeepEqual(got, ctx) {
			t.Fatalf("legacy unevaluated shape must retain its existing compatibility behavior: %+v", got)
		}
		legacy.resourceClosureEvaluated = true
		if got := rootCauseChainContextForItem(legacy, ctx, target); got.relevance != "adjacent" || got.overlapMs != 0 {
			t.Fatalf("explicit false closure must retain the existing demotion guard for self and peer: %+v", got)
		}
		legacy.ResourceCompletionClosure = true
		if got := rootCauseChainContextForItem(legacy, ctx, target); !reflect.DeepEqual(got, ctx) {
			t.Fatalf("explicit positive closure must retain the original guard result: %+v", got)
		}
	}
	for _, typ := range []string{"d_state_or_io_wait", "runnable_wait", "running"} {
		row := RootCauseRankItem{Type: typ, Thread: target, StartTs: 5.0001, EndTs: 5.0002}
		before := rootCauseChainContextForItem(row, ctx, target)
		row.resourceClosureEvaluated = true
		if got := rootCauseChainContextForItem(row, ctx, target); !reflect.DeepEqual(got, before) {
			t.Fatalf("the IO-only closure guard changed %s: before=%+v after=%+v", typ, before, got)
		}
	}
}
