package tool

import (
	"strings"
	"testing"
)

// HMC377 round 13 (batch-owner ruling R-377-13 ①): one object per physical
// thread on the tree, on complete original captures (opt-in; the review
// windows of fix12/review-population/win/all_windows265.txt). Raw: the
// review's own parse (review-population raw/raw_all.json).

func r13WholeRows(faces hmc377R11Faces, tid string) []*runtimeTraceProjTreeRow {
	var out []*runtimeTraceProjTreeRow
	for _, row := range runtimeTraceProjSMR1AllRows(&faces.model) {
		if row.HasData && strings.HasSuffix(strings.TrimSpace(row.Node.Subject), "-"+tid) {
			out = append(out, row)
		}
	}
	return out
}

// 100 抖音极速版 容器重启 #1 (CODRAX_HMC376_GALLERY_FULLGC), review window w01
// [2493.173005, 2494.208465) TID 56888: binder 57014's chain hop (runnable
// 6.484 on [2494.05334, 2494.066535]) and its inversion rank seat (8.966 =
// runnable 6.484 + running deficit 2.482, the same measured account) were two
// additive rows beside the ◇ census remainder 136.853 (6.484 + 6.484 + 136.853
// = 149.821 against raw ready 147.902). They are one row now, carrying the
// seat's rank, its badge, its [E#] and its chain cumulative.
func TestHMC377R13WholeOriginalHopAndInversionSeatAreOneRow(t *testing.T) {
	path := hmc376Original(t, "CODRAX_HMC376_GALLERY_FULLGC", "d27ff641787dfa0768928e0ce3d11a7d2a6caa0f2910c941fae3e97f2256e22e", 245420208)
	faces := hmc377R11QueryPID(t, path, 56888, 2493.173005, 2494.208465)
	var host *runtimeTraceProjTreeRow
	for _, row := range r13WholeRows(faces, "57014") {
		if strings.HasPrefix(row.Node.Predicate, "root_cause_") && runtimeTraceCausalProjectionInversionRow(row.Node) {
			t.Fatalf("the inversion seat must not be a second row of 57014: %s %s", row.EvidenceTag, row.Node.EvidenceID)
		}
		if row.Node.Predicate == "wakeup_causal_impact" && len(row.RankFoldPeers) == 1 {
			host = row
		}
	}
	if host == nil || host.Node.Rank <= 0 || host.Badge != host.Node.Rank {
		t.Fatalf("57014's hop row hosts its seat with the seat's rank and badge: %+v", host)
	}
	joined := hmc377R12Joined(faces.zh)
	if !strings.Contains(joined, "["+host.EvidenceTag+"+"+host.RankFoldPeers[0].EvidenceTag+"]") {
		t.Fatal("the one row's bracket names both records")
	}
	// The seat's own chain cumulative stays on the one row.
	if !strings.Contains(joined, "链上累计13.195ms") {
		t.Fatal("the seat's 链上累计13.195ms stays on the one row")
	}
}

// 105 元宝 容器重启 #2 (CODRAX_HMC377_YUANBAO_CONTAINER2), review window w17
// [3820.346447, 3821.062086) TID 50874: dtmp-51025 / dtmp-51035's chain hops
// (inversion candidacy, priced 0.475 / 0.292 running deficit) and their case-B
// clipped census seats (调度供给, 5.355 / 7.126) are two rulers on the same
// ready segments (raw: 51025 ready [3820.898136, 3820.903491] inside the hop
// window, 51035 [3820.92113, 3820.928256]): both stay drawn and each wears the
// mirror tag naming the other.
func TestHMC377R13WholeOriginalTwoRulersOnOneAccountWearTheMirror(t *testing.T) {
	path := hmc376Original(t, "CODRAX_HMC377_YUANBAO_CONTAINER2", "22c9ba3708a9bbc0c1c31703138859c8b00f9f6bfe39df1e1cdd9e18802edd4a", 62845037)
	faces := hmc377R11QueryPID(t, path, 50874, 3820.346447, 3821.062086)
	for _, tid := range []string{"51025", "51035"} {
		var hop, seat *runtimeTraceProjTreeRow
		for _, row := range r13WholeRows(faces, tid) {
			switch {
			case row.Node.Predicate == "wakeup_causal_impact" && row.Node.StateKind == "runnable":
				hop = row
			case strings.HasPrefix(row.Node.Predicate, "root_cause_") && row.Node.Object == "runnable_wait" && row.Node.ChainAnchorFullMS > 0 && !row.Node.ChainAnchorRemainderSeat:
				seat = row
			}
		}
		if hop == nil || seat == nil || hop.ValueMirrorRef != seat.EvidenceTag || seat.ValueMirrorRef != hop.EvidenceTag {
			t.Fatalf("%s: the hop and its clipped seat name each other: hop=%+v seat=%+v", tid, hop, seat)
		}
		if !strings.Contains(hmc377R12Joined(faces.zh), "同段镜像·与["+seat.EvidenceTag+"]同一物理时间,不可相加") {
			t.Fatalf("%s: the zh tree wears the mirror tag", tid)
		}
	}
}

// 105 元宝 应用后台销毁 #2 (CODRAX_HMC377_YUANBAO_BGKILL2), review window w22m
// [10835.034648, 10835.183738) TID 58533: tbs_basemodule_-58968's span
// VerifyClass …ThreadPoolSnapshot walls 1.202ms (raw), 0.292ms of it inside
// the analysed thread's waits its thread's wakeup ended; the ◇ remainder row
// repeated the anchored 0.292 beside the ✦ record that draws the whole 1.202
// (1.202 + 0.292 + the family's 12.120 = 13.614 against raw 13.322). The
// remainder rides the record now, and its native item publishes 0.910.
func TestHMC377R13WholeOriginalSemanticRemainderRidesItsRecord(t *testing.T) {
	path := hmc376Original(t, "CODRAX_HMC377_YUANBAO_BGKILL2", hmc377YuanbaoBgKill2Digest, hmc377YuanbaoBgKill2Size)
	faces := hmc377R11QueryPID(t, path, 58533, 10835.034648, 10835.183738)
	var record *runtimeTraceProjTreeRow
	for _, row := range r13WholeRows(faces, "58968") {
		if row.Node.ChainAnchorRemainderSeat && row.Node.SemanticClass == "class_verification" {
			t.Fatalf("the semantic remainder must not be a row beside its record: %s", row.EvidenceTag)
		}
		if runtimeTraceCausalProjectionSemanticSpanRow(row.Node) && row.Node.LineStart == 182417 {
			record = row
		}
	}
	if record == nil {
		t.Fatal("the span record is drawn")
	}
	var rem bool
	for _, peer := range record.RankFoldPeers {
		if peer.DisplayImpactMS > 0.909 && peer.DisplayImpactMS < 0.911 {
			rem = true
		}
	}
	if !rem {
		t.Fatalf("the record carries its 0.910 remainder: %+v", record.RankFoldPeers)
	}
}
