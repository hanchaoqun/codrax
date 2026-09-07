package tracequery

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestB1603BinderWaitUsesItsSchedulerInterval(t *testing.T) {
	for _, tc := range []struct {
		name, state, send string
		closed            bool
	}{
		{"interruptible", "S", "13762.835811", true},
		{"uninterruptible without chain wake edge", "D", "13762.835811", false},
		{"earlier request", "S", "13762.835700", true},
		{"request at sleep start", "S", "13762.835861", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			trace := strings.Replace(donghuP9TrueBinderWaitTrace, "prev_state=S", "prev_state="+tc.state, 1)
			trace = strings.Replace(trace, "13762.835811", tc.send, 1)
			idx := buildTraceIndex(t, "binder-window.ftrace", trace)
			q := Query{PID: 17267, TimeStart: 13762.8355, TimeEnd: 13762.8375, MaxDepth: 3, MaxBranches: 4, MinDurationMs: 0.01}
			chain := BuildWakeupChain(idx, q)
			if len(chain.BinderWaits) != 1 {
				t.Fatalf("expected the existing synchronous wait, got %+v", chain.BinderWaits)
			}
			wait := chain.BinderWaits[0]
			if wait.SleepStartTs != 13762.835861 || math.Abs(wait.DurationMs-1.409) > 0.000001 {
				t.Fatalf("fixture lost its exact state interval: %+v", wait)
			}
			wantStart, wantEnd := wait.SleepStartTs, wait.WakeupTs
			if !tc.closed {
				// The current D branch does not recurse through sched_wakeup;
				// retain its measurement without inventing a closing chain edge.
				if wait.WakeupTs != 0 || wait.WakeupLine != 0 {
					t.Fatalf("D fixture no longer exercises the missing typed closure boundary: %+v", wait)
				}
				wantStart, wantEnd = 0, 0
			} else if wantEnd != 13762.837270 || wait.WakeupLine == 0 {
				t.Fatalf("fixture has an unexpected closing wake: %+v", wait)
			}
			before, _ := json.Marshal(chain)
			critical := BuildCriticalBlockingCalls(idx, q)
			var found bool
			for _, item := range critical.Items {
				if item.Type != "binder_wait" || item.Thread.PID != wait.Thread.PID {
					continue
				}
				found = true
				if item.StartTs != wantStart || item.EndTs != wantEnd || item.DurationMs != wait.DurationMs {
					t.Errorf("critical row borrowed the request envelope: item=%+v wait=%+v", item, wait)
				}
				if item.Peer != wait.Peer || item.PeerSource != wait.PeerSource || item.Flags != wait.Flags ||
					item.LineStart != wait.SendLine || item.LineEnd != firstPositive(wait.WakeupLine, wait.ReceiveLine, wait.SleepLine) || item.Confidence != wait.Confidence {
					t.Errorf("interval repair changed counterpart, request locator, or ranking inputs: item=%+v wait=%+v", item, wait)
				}
				if wantEnd > wantStart && (item.PeerState == nil || item.PeerState.Thread.PID != wait.Peer.PID ||
					item.PeerState.Window.StartTs != wantStart || item.PeerState.Window.EndTs != wantEnd) {
					t.Errorf("peer state must describe the same target wait, not its pre-sleep send phase: %+v", item.PeerState)
				}
			}
			if !found {
				t.Fatal("critical row disappeared")
			}
			facts := evidenceFromChain(chain)
			found = false
			for _, fact := range facts {
				// RootEvidence also publishes a fact with this predicate; the
				// explicit wait counterpart identifies this interval-bearing face.
				if fact.Predicate != "binder_wait" || fact.Object != threadLabel(wait.Peer) {
					continue
				}
				found = true
				if fact.StartTs != wantStart || fact.EndTs != wantEnd || fact.LineStart != wait.SendLine || fact.LineEnd != firstPositive(wait.WakeupLine, wait.ReceiveLine, wait.SleepLine) {
					t.Errorf("wait evidence fact lost its measurement/locator distinction: %+v", fact)
				}
			}
			if !found {
				t.Fatal("wait evidence fact disappeared")
			}
			after, _ := json.Marshal(chain)
			if string(before) != string(after) {
				t.Fatal("measurement projection mutated original send/sleep/receive/reply evidence")
			}
		})
	}
}

func TestB1603BinderWaitRespectsSelectedWindow(t *testing.T) {
	idx := buildTraceIndex(t, "binder-window-clip.ftrace", donghuP9TrueBinderWaitTrace)
	for _, tc := range []struct {
		name       string
		start, end float64
		wantDonors int
	}{
		// The existing IPC query excludes sends before the selected start.
		// This is a no-donor boundary pin, not complete clipped-wait coverage.
		{"left clipped request outside query", 13762.836, 13762.8375, 0},
		{"right clipped before wake", 13762.8355, 13762.837, 1},
		{"after wait", 13762.83731, 13762.8375, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := Query{PID: 17267, TimeStart: tc.start, TimeEnd: tc.end, MaxDepth: 3, MaxBranches: 4, MinDurationMs: 0.01}
			chain := BuildWakeupChain(idx, q)
			if len(chain.BinderWaits) != tc.wantDonors {
				t.Fatalf("fixture donor count changed: got=%d want=%d waits=%+v", len(chain.BinderWaits), tc.wantDonors, chain.BinderWaits)
			}
			critical := BuildCriticalBlockingCalls(idx, q)
			var published int
			for _, item := range critical.Items {
				if item.Type != "binder_wait" {
					continue
				}
				published++
				var matched bool
				for _, wait := range chain.BinderWaits {
					if item.Thread != wait.Thread || item.LineStart != wait.SendLine {
						continue
					}
					matched = true
					if wait.WakeupTs <= wait.SleepStartTs {
						if item.StartTs != 0 || item.EndTs != 0 {
							t.Errorf("query without a closing wake must not borrow an external endpoint: %+v", item)
						}
					} else if item.StartTs != wait.SleepStartTs || item.EndTs != wait.WakeupTs || item.StartTs < tc.start || item.EndTs > tc.end {
						t.Errorf("wait extent borrowed another window or request phase: %+v, wait=%+v", item, wait)
					}
					if item.DurationMs != wait.DurationMs || item.Peer != wait.Peer {
						t.Errorf("window projection changed measured duration or peer: %+v", item)
					}
				}
				if !matched {
					t.Errorf("critical wait has no same-query chain donor: %+v", item)
				}
			}
			if published != tc.wantDonors {
				t.Fatalf("critical publication lost or borrowed donor rows: got=%d want=%d", published, tc.wantDonors)
			}
			t.Logf("same-query wait donors=%d", len(chain.BinderWaits))
		})
	}
}

func TestB1603BinderWaitMissingClosureDoesNotInventExtent(t *testing.T) {
	idx := buildTraceIndex(t, "binder-window-legacy.ftrace", donghuP9TrueBinderWaitTrace)
	q := Query{PID: 17267, TimeStart: 13762.8355, TimeEnd: 13762.8375, MaxDepth: 3, MaxBranches: 4, MinDurationMs: 0.01}
	base := BuildWakeupChain(idx, q)
	if len(base.BinderWaits) != 1 {
		t.Fatal("fixture missing wait")
	}
	for _, tc := range []struct {
		name string
		edit func(*BinderWaitSummary)
	}{
		{"legacy missing sleep", func(w *BinderWaitSummary) { w.SleepStartTs = 0 }},
		{"missing wakeup", func(w *BinderWaitSummary) { w.WakeupTs = 0 }},
		{"reverse", func(w *BinderWaitSummary) { w.WakeupTs = w.SleepStartTs - 0.001 }},
		{"zero width", func(w *BinderWaitSummary) { w.WakeupTs = w.SleepStartTs }},
		{"nonfinite sleep", func(w *BinderWaitSummary) { w.SleepStartTs = math.NaN() }},
		{"nonfinite wakeup", func(w *BinderWaitSummary) { w.WakeupTs = math.Inf(1) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chain := base
			chain.BinderWaits = append([]BinderWaitSummary(nil), base.BinderWaits...)
			tc.edit(&chain.BinderWaits[0])
			wait := chain.BinderWaits[0]
			critical := buildCriticalBlockingCallsFromStats(idx, q, WindowStats{}, &chain)
			var found bool
			for _, item := range critical.Items {
				if item.Type == "binder_wait" {
					found = true
					if item.StartTs != 0 || item.EndTs != 0 || item.DurationMs != wait.DurationMs || item.Peer != wait.Peer {
						t.Errorf("incomplete interval must not be manufactured from send/duration; preserve measured row: %+v", item)
					}
				}
			}
			if !found {
				t.Fatal("an incomplete interval must not delete the original candidate")
			}
			for _, fact := range evidenceFromChain(chain) {
				if fact.Predicate == "binder_wait" && fact.Object == threadLabel(wait.Peer) && (fact.StartTs != 0 || fact.EndTs != 0) {
					t.Errorf("fact invented an interval without both state endpoints: %+v", fact)
				}
			}
		})
	}
}

func TestB1603BinderIntervalRepairDoesNotPromoteIPCInventory(t *testing.T) {
	for _, tc := range []struct{ name, trace string }{
		{"oneway", strings.Replace(donghuP9TrueBinderWaitTrace, "flags=0x10", "flags=0x11", 1)},
		{"reply", strings.Replace(donghuP9TrueBinderWaitTrace, "reply=0", "reply=1", 1)},
		{"request after sleep", strings.Replace(donghuP9TrueBinderWaitTrace, "13762.835811", "13762.835900", 1)},
		{"completed request before pacing", donghuP9FalseAttributionTrace},
	} {
		t.Run(tc.name, func(t *testing.T) {
			idx := buildTraceIndex(t, "binder-nonwait.ftrace", tc.trace)
			q := Query{PID: 17267, TimeStart: 13762.8355, TimeEnd: 13763.010, MaxDepth: 3, MaxBranches: 4, MinDurationMs: 0.01}
			if chain := BuildWakeupChain(idx, q); len(chain.BinderWaits) != 0 {
				t.Fatalf("non-wait IPC inventory was promoted: %+v", chain.BinderWaits)
			}
			for _, item := range BuildCriticalBlockingCalls(idx, q).Items {
				if item.Type == "binder_wait" {
					t.Fatalf("non-wait acquired critical wait interval: %+v", item)
				}
			}
		})
	}
}

func TestB1603MissingClosureKeepsIdentityWithoutOverlapCredential(t *testing.T) {
	trace := strings.Replace(donghuP9TrueBinderWaitTrace, "prev_state=S", "prev_state=D", 1)
	idx := buildTraceIndex(t, "binder-d-identity.ftrace", trace)
	q := Query{PID: 17267, TimeStart: 13762.8355, TimeEnd: 13762.8375, MaxDepth: 3, MaxBranches: 4, MinDurationMs: 0.01}
	chain := BuildWakeupChain(idx, q)
	if len(chain.BinderWaits) != 1 || chain.BinderWaits[0].WakeupTs != 0 {
		t.Fatalf("fixture must retain a measured wait without a typed closing edge: %+v", chain.BinderWaits)
	}
	var candidate *CriticalBlockingCandidate
	for _, item := range BuildCriticalBlockingCalls(idx, q).Items {
		if item.Type == "binder_wait" {
			copy := item
			candidate = &copy
		}
	}
	if candidate == nil || candidate.StartTs != 0 || candidate.EndTs != 0 ||
		candidate.ChainRelevance != "on_chain" || candidate.OverlapMs != 0 || candidate.ChainIdentityInheritance {
		t.Fatalf("target-self legacy identity membership must not claim overlap or a non-self inheritance marker: %+v", candidate)
	}
	if candidate.OnChainBasis != "" || len(candidate.ChainCredentialSegments) != 0 || candidate.ResourceCompletionClosure {
		t.Fatalf("missing endpoints invented a stronger interval/resource credential: %+v", candidate)
	}
	// The existing identity-only lane has an explicit disclosure for a
	// non-target member; the target-self exception above intentionally omits
	// that marker. Exercise the same enrichment function, not a new policy.
	otherTarget := chain
	otherTarget.Target = ThreadRef{PID: 42, TGID: 42, Comm: "other-target"}
	member := enrichCriticalBlockingWithChainContext(otherTarget, []CriticalBlockingCandidate{*candidate})[0]
	if member.ChainRelevance != "on_chain" || member.OverlapMs != 0 || !member.ChainIdentityInheritance || member.OnChainBasis != "" {
		t.Fatalf("identity-only non-target membership must remain disclosed without overlap proof: %+v", member)
	}
	var foundRank bool
	for _, item := range BuildRootCauseRank(idx, q).Items {
		if item.Type != "binder_wait" || item.Thread.PID != q.PID {
			continue
		}
		foundRank = true
		if item.Rank != 0 || item.Tier != RootCauseTierTargetSelfState {
			t.Fatalf("an unclosed target symptom must not become a root-cause board seat: %+v", item)
		}
	}
	if !foundRank {
		t.Fatal("the original measured symptom must remain available without a root-cause seat")
	}
}
