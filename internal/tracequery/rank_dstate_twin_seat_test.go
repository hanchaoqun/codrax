package tracequery

import "testing"

// ENG audit #65 (P2, §29.25 处置委托 2026-07-10): the §20.1 rank-twin gate is
// an identity-key skip (rootEvidenceRankSeatKey), but expandChain MUTATES the
// D-state RootEvidence twin in place after construction when a
// sched_blocked_reason resolves (Type→"io_wait" iff reason.IOWait>0, and
// LineEnd→reason.Line whenever the reason row is found). The mutated key no
// longer matched the seed minted from the unmutated constructor output, so
// the same physical occurrence seated twice: a d_state row from the
// CausalImpacts lane plus an io_wait row from the RootEvidence lane. The seed
// now also records a mutation-invariant family key (thread+LineStart+
// DurationMs) for the d_state/io_wait family.

func dstateTwinImpact(dep ThreadRef) WakeupCausalImpact {
	return WakeupCausalImpact{
		Thread:            dep,
		Window:            TimeWindow{StartTs: 5.001, EndTs: 5.009},
		ActualWindow:      TimeWindow{StartTs: 5.001, EndTs: 5.009},
		ChainDepth:        1,
		OnChain:           true,
		DominantState:     string(StateDSleep),
		DominantImpactMs:  8,
		ProjectedImpactMs: 8,
		ProjectedTotalMs:  8,
		ActualImpactMs:    8,
		ActualTotalMs:     8,
		TotalMs:           8,
		DStateMs:          8,
		ActualDStateMs:    8,
		TargetBlockedMs:   8,
		LineStart:         20,
		LineEnd:           30,
		Summary:           "dependency d-state occurrence",
	}
}

func dstateFamilyCarriers(rank RootCauseRankResult, pid, lineStart int) []RootCauseRankItem {
	var out []RootCauseRankItem
	for _, item := range rank.Items {
		if item.Thread.PID == pid && item.LineStart == lineStart &&
			(item.Type == "d_state_or_io_wait" || item.Type == "io_wait") {
			out = append(out, item)
		}
	}
	return out
}

func TestRootCauseRankMutatedDStateTwinOwnsOneSeat(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mutate   func(*RootEvidence)
	}{
		{
			// reason.IOWait>0: both Type and LineEnd diverge from the seed.
			name: "iowait_type_and_line_mutation",
			mutate: func(root *RootEvidence) {
				root.Type = "io_wait"
				root.LineEnd = 27
			},
		},
		{
			// reason.IOWait==0 but reason.Line found: LineEnd alone breaks the
			// exact seat key.
			name: "line_only_mutation",
			mutate: func(root *RootEvidence) {
				root.LineEnd = 27
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dep := ThreadRef{Comm: "io-dep", PID: 300}
			impact := dstateTwinImpact(dep)
			// Exact production expandChain shape: the twin derives from the SAME
			// WakeupCausalImpact then is mutated in place by the blocked-reason
			// resolution (query.go StateDSleep/StateIOWait case).
			twin := rootEvidenceFromCausalImpact(impact, "thread slept in D state", 0.88)
			tc.mutate(&twin)
			chain := ChainResult{
				Target: ThreadRef{Comm: "app", PID: 100},
				Nodes: []ChainNode{
					{ID: "target", Thread: ThreadRef{Comm: "app", PID: 100}, Window: TimeWindow{StartTs: 5.000, EndTs: 5.010}},
					{ID: "dep", Thread: dep, Window: impact.Window, Impact: &impact, Depth: 1},
				},
				CausalImpacts: []WakeupCausalImpact{impact},
				RootEvidence:  []RootEvidence{twin},
			}
			rank := buildRootCauseRankFrom(nil, Query{TimeStart: 5.000, TimeEnd: 5.010, Limit: 12}, chain, WindowStats{})
			carriers := dstateFamilyCarriers(rank, dep.PID, impact.LineStart)
			if len(carriers) != 1 {
				t.Fatalf("one physical D-state occurrence must own one rank seat even after blocked-reason mutation; got %d carriers: %+v", len(carriers), carriers)
			}
			if carriers[0].Source != "wakeup_chain.causal_impacts" {
				t.Fatalf("the CausalImpacts lane is the single rank carrier: %+v", carriers[0])
			}
		})
	}
}

// Negative control: a D-state RootEvidence row that is NOT the twin of any
// seeded impact (different occurrence identity) keeps minting — the family
// key must not over-suppress independent evidence. A NON-family lane sharing
// the exact thread+LineStart+DurationMs coordinates (missing_wakeup) must
// also keep minting: the family fold is typed to the two D-state tokens only.
func TestRootCauseRankIndependentDStateEvidenceStillMints(t *testing.T) {
	dep := ThreadRef{Comm: "io-dep", PID: 300}
	impact := dstateTwinImpact(dep)
	independent := RootEvidence{
		Type:       "io_wait",
		Thread:     dep,
		DurationMs: 3,
		LineStart:  50,
		LineEnd:    55,
		Summary:    "independent io wait occurrence",
		Confidence: 0.88,
	}
	// Same coordinates as the seeded impact but a different typed lane — the
	// exact shape the family key's type gate exists to protect.
	sameCoordsOtherLane := RootEvidence{
		Type:       "missing_wakeup",
		Thread:     dep,
		DurationMs: 8,
		LineStart:  impact.LineStart,
		LineEnd:    impact.LineEnd,
		Summary:    "sleep interval has no matching sched_wakeup row",
		Confidence: 0.7,
	}
	chain := ChainResult{
		Target: ThreadRef{Comm: "app", PID: 100},
		Nodes: []ChainNode{
			{ID: "target", Thread: ThreadRef{Comm: "app", PID: 100}, Window: TimeWindow{StartTs: 5.000, EndTs: 5.010}},
			{ID: "dep", Thread: dep, Window: impact.Window, Impact: &impact, Depth: 1},
		},
		CausalImpacts: []WakeupCausalImpact{impact},
		RootEvidence:  []RootEvidence{independent, sameCoordsOtherLane},
	}
	rank := buildRootCauseRankFrom(nil, Query{TimeStart: 5.000, TimeEnd: 5.010, Limit: 12}, chain, WindowStats{})
	if got := dstateFamilyCarriers(rank, dep.PID, 50); len(got) != 1 {
		t.Fatalf("independent d-state evidence (different occurrence) must keep its seat: %+v", rank.Items)
	}
	if got := dstateFamilyCarriers(rank, dep.PID, impact.LineStart); len(got) != 1 {
		t.Fatalf("the seeded occurrence still owns exactly one seat: %+v", rank.Items)
	}
	missingWakeup := 0
	for _, item := range rank.Items {
		if item.Type == "missing_wakeup" && item.Thread.PID == dep.PID && item.LineStart == impact.LineStart {
			missingWakeup++
		}
	}
	if missingWakeup != 1 {
		t.Fatalf("a non-family lane sharing coordinates must keep minting (type gate): %+v", rank.Items)
	}
}
