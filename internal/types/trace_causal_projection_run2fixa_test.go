package types

// trace_causal_projection_run2fixa_test.go — RUN2FIX-A 件2 constructor pins
// (§29.174 处置②, runnable_2:361-363, 2026-07-20): the fold constructors
// record the MAX member's identity (MergedMaxSubject/MergedMaxStateKind) so
// the fold row can disclose 线程·状态·值; unknown-subject maxima clear both
// carriers (宁漏勿假), and an absorbed fold member passes through its own
// recorded maximum (the true value owner).

import "testing"

func TestRun2FixAOverflowFoldRecordsMaxMember(t *testing.T) {
	fold := traceCausalProjectionOverflowFoldRow([]TraceCausalProjectionNode{
		{Subject: "head-thread-11", StateKind: "runnable", ImpactMS: 4.558, EvidenceID: "e-1"},
		{Subject: "CookieMonsterCl-59843", StateKind: "d_sleep", ImpactMS: 47.282, EvidenceID: "e-2"},
		{Subject: "mid-thread-22", StateKind: "runnable", ImpactMS: 8.0, EvidenceID: "e-3"},
	})
	if fold.MergedMaxSubject != "CookieMonsterCl-59843" || fold.MergedMaxStateKind != "d_sleep" {
		t.Fatalf("the fold must record its MAX member identity: %q/%q",
			fold.MergedMaxSubject, fold.MergedMaxStateKind)
	}
	if fold.MergedMaxMS != 47.282 {
		t.Fatalf("value channel untouched: %v", fold.MergedMaxMS)
	}
	// 宁漏勿假: an unknown-subject maximum clears both carriers.
	blank := traceCausalProjectionOverflowFoldRow([]TraceCausalProjectionNode{
		{Subject: "head-thread-11", StateKind: "runnable", ImpactMS: 4.558, EvidenceID: "e-1"},
		{Subject: "unknown-thread", StateKind: "d_sleep", ImpactMS: 47.282, EvidenceID: "e-2"},
	})
	if blank.MergedMaxSubject != "" || blank.MergedMaxStateKind != "" {
		t.Fatalf("unknown-subject maxima must clear the carriers: %q/%q",
			blank.MergedMaxSubject, blank.MergedMaxStateKind)
	}
	// F1 计数吸收 twin: an absorbed fold member passes through its OWN
	// recorded maximum identity.
	inner := traceCausalProjectionOverflowFoldRow([]TraceCausalProjectionNode{
		{Subject: "inner-max-9", StateKind: "io_wait", ImpactMS: 60.0, EvidenceID: "e-9"},
		{Subject: "inner-min-8", StateKind: "runnable", ImpactMS: 1.0, EvidenceID: "e-8"},
	})
	outer := traceCausalProjectionOverflowFoldRow([]TraceCausalProjectionNode{
		{Subject: "head-thread-11", StateKind: "runnable", ImpactMS: 4.558, EvidenceID: "e-1"},
		inner,
	})
	if outer.MergedMaxSubject != "inner-max-9" || outer.MergedMaxStateKind != "io_wait" {
		t.Fatalf("absorbed fold members pass through their own max identity: %q/%q",
			outer.MergedMaxSubject, outer.MergedMaxStateKind)
	}
}

func TestRun2FixABackgroundFoldRecordsMaxMember(t *testing.T) {
	unknownPeer := func(subject string, impact float64, id string) TraceCausalProjectionNode {
		return TraceCausalProjectionNode{
			Subject: subject, Object: "unknown-thread", ChainRelevance: "background",
			ImpactMS: impact, CumulativeImpactMS: impact, EvidenceID: id,
		}
	}
	nodes := []TraceCausalProjectionNode{
		unknownPeer("bg-a-1", 10.0, "b-1"),
		unknownPeer("bg-b-2", 9.0, "b-2"),
		unknownPeer("bg-max-3", 3.972, "b-3"),
		unknownPeer("bg-c-4", 2.5, "b-4"),
		unknownPeer("bg-d-5", 2.0, "b-5"),
	}
	out := traceCausalProjectionFoldUnknownBackground(nodes)
	var fold *TraceCausalProjectionNode
	for i := range out {
		if out[i].MergedCount > 1 && out[i].Subject == "" {
			fold = &out[i]
		}
	}
	if fold == nil {
		t.Fatalf("fixture must produce the R3 background fold: %+v", out)
	}
	if fold.MergedMaxSubject == "" || fold.MergedMaxMS <= 0 {
		t.Fatalf("the ▒ stanza fold records its MAX member too: %q max=%v",
			fold.MergedMaxSubject, fold.MergedMaxMS)
	}
}
