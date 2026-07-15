package tool

// answer_document_mutation_runtime_supplyfold_sfd_test.go — SFD (§15.A
// display half, user q6 issue 1): the supply-fold twin join rendered through
// the REAL chain (observations → compile/join → blocks → markdown). q6
// specimen: the running root-cause row ("⚙ running 58.919ms · 优先级反转候选 ·
// 有效归因58.919ms") carried NO fold caliber while the SAME segment's
// causal-impact sibling (:45689-79142, same subject) published the full
// 供给折算 accounting that only surfaced in the sibling's 机制构成 — the join
// (trace_causal_projection.go traceCausalProjectionJoinSupplyFoldTwins) now
// re-uses that engine-published basis on the running row, so both rows'
// deficit numbers are same-source by construction.

import (
	"regexp"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// sfdQ6Anchor is the 3000ms query-window anchor (window mode; shared
// significance gate min(300,100)=100ms — the donor's runnable 150ms clears
// it, the running twin publishes no runnable of its own).
func sfdQ6Anchor() types.ObservationRecord {
	return types.ObservationRecord{
		ID: "anchor", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: types.ClaimGroundingHard, Predicate: "frame_target_resolution", ClaimKey: "frame_target_resolution:f",
		Subject: "app-100", Object: "frame",
		Span:      types.ObservationSpan{StartTs: 100.0, EndTs: 103.0},
		RichNotes: []string{"window_source=query_window"},
	}
}

// sfdQ6TwinObs is the q6 running twin: rank-funnel published (basis=nil by
// construction → NO fold notes), inversion candidate, effective == its own
// running projection. Distinct projected ms from the donor (58.919 vs 37.409)
// keeps the two rows apart exactly like the customer render (no R1 merge).
func sfdQ6TwinObs() types.ObservationRecord {
	return projV3Obs("root-running", "root_cause_primary", "root_cause_primary:running",
		"#RxComputationT-16816", "running", "58.919", 58.919, 45689, 79142,
		"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
		"chain_depth=1", "dominant_state=running", "priority_inversion_candidate=true",
		"effective_impact_ms=58.919")
}

// sfdQ6DonorObs is the causal-impact sibling carrying the engine-published
// fold accounting (deficit 17.702 = 58.919 − ideal 41.217, fully-known basis)
// plus its own demand/gated lanes → its own row renders the Triple 机制构成.
func sfdQ6DonorObs(foldNotes ...string) types.ObservationRecord {
	return projV3Obs("hop-impact", "wakeup_causal_impact", "wakeup_causal_impact:running",
		"#RxComputationT-16816", "running", "37.409", 37.409, 45689, 79142,
		append([]string{
			"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1",
			"dominant_state=running", "priority_inversion_candidate=true",
			"runnable=150.000", "gated_runnable=20.713", "gated_running_deficit=16.697",
			"effective_impact_ms=37.409",
		}, foldNotes...)...)
}

func sfdQ6FoldNotes() []string {
	return []string{
		"supply_fold_deficit_ms=17.702", "supply_fold_ideal_ms=41.217",
		"fold_basis=known=58.919ms,unknown=0.000ms",
	}
}

// Pin ①: the q6 two-row shape — the running row now wears the supply-fold
// caliber through the EXISTING tag lane (single-source clause; its verdict is
// the deficit-dominant branch: no own runnable → no scheduling-pressure
// claim), its 有效归因 value is untouched, and the donor keeps its Triple
// 机制构成. Every 供给折算缺口 number on the page is the ONE engine-published
// 17.702 (same-source by construction — the consistency half of the batch).
func TestSFDRunningTwinJoinRendersFoldCaliberZH(t *testing.T) {
	records := []types.ObservationRecord{sfdQ6Anchor(), sfdQ6TwinObs(), sfdQ6DonorObs(sfdQ6FoldNotes()...)}
	md := audit730Render(t, audit730Bus(""), records, "")
	despaced := vs2Despace(md)
	// The running twin's joined clause (deficit-dominant branch wording —
	// only reachable on the twin: the donor's own runnable 150ms renders the
	// Triple branch instead).
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: "running含跑慢成分"
	// → "running时间含降频/小核导致的跑慢成分" (供给折算族).
	if !strings.Contains(despaced, "供给折算缺口17.702ms(运行频点非最高,按全域最大核最高频折算,下界)为主,running时间含降频/小核导致的跑慢成分") {
		t.Fatalf("running twin must render the joined supply-fold caliber:\n%s", md)
	}
	// PTV8-RCR-A EVOLUTION RECORD (§24 ②): the donor is an inversion cause
	// node — its Triple 机制构成 sentence is RETIRED; the deficit keeps its
	// lossless home on the donor's detail 供给折算 line (unified sub-row
	// grammar) and the composition on 有效归因构成.
	if strings.Contains(md, "机制构成") {
		t.Fatalf("the retired mechanism sentence must not render on inversion nodes:\n%s", md)
	}
	if !strings.Contains(despaced, "供给折算缺口17.702ms(运行频点非最高,折算,按全域最大核最高频,下界;与有效归因中running计入同源同值)") {
		t.Fatalf("donor deficit must keep its lossless detail home:\n%s", md)
	}
	// 有效归因 stays the twin's own measured value — the fold is a
	// supplementary caliber disclosure, never a re-attribution.
	if !strings.Contains(despaced, "有效归因58.919ms") {
		t.Fatalf("the twin's attribution value must not change:\n%s", md)
	}
	// Consistency pin (batch item 3): every 供给折算缺口 magnitude on the page
	// is the ONE engine value — E4 tag and E7 机制构成 can never diverge.
	matches := regexp.MustCompile(`供给折算缺口(\d+\.\d+)ms`).FindAllStringSubmatch(despaced, -1)
	if len(matches) < 2 {
		t.Fatalf("both rows must carry the deficit (got %d occurrences):\n%s", len(matches), md)
	}
	for _, m := range matches {
		if m[1] != "17.702" {
			t.Fatalf("deficit must be the single engine-published 17.702 everywhere, got %s:\n%s", m[1], md)
		}
	}
}

// Pin ②: join-key precision at the render surface — a sibling with a
// different line range (or subject) is a different segment: the running row
// keeps its bare-value shape (no deficit-dominant clause anywhere), the
// donor's own clause stays.
func TestSFDRunningTwinNoJoinOnKeyMismatch(t *testing.T) {
	for name, mutate := range map[string]func(*types.ObservationRecord){
		"line_range": func(donor *types.ObservationRecord) {
			donor.Span.LineEnd = 79143
			donor.SupportRefs = []string{"berlin.systrace:45689-79143"}
		},
		"subject": func(donor *types.ObservationRecord) { donor.Subject = "OtherThread-999" },
	} {
		donor := sfdQ6DonorObs(sfdQ6FoldNotes()...)
		mutate(&donor)
		records := []types.ObservationRecord{sfdQ6Anchor(), sfdQ6TwinObs(), donor}
		md := audit730Render(t, audit730Bus(""), records, "")
		despaced := vs2Despace(md)
		// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: negative pin
		// migrates with the clause — "running含跑慢成分" → "running时间含降频".
		if strings.Contains(despaced, "为主,running时间含降频") {
			t.Fatalf("%s mismatch must never join (fail-open to the bare value):\n%s", name, md)
		}
		if !strings.Contains(despaced, "有效归因58.919ms") {
			t.Fatalf("%s mismatch: the twin's bare attribution row must stay:\n%s", name, md)
		}
		// PTV8-RCR-A EVOLUTION RECORD (§24 ②): the donor's deficit now lives
		// on its detail 供给折算 line (the Triple sentence is retired).
		if !strings.Contains(despaced, "供给折算缺口17.702ms(运行频点非最高,折算,按全域最大核最高频,下界;与有效归因中running计入同源同值)") {
			t.Fatalf("%s mismatch: the donor's own deficit disclosure must stay:\n%s", name, md)
		}
	}
}

// Pin ③: no sibling at all → the running row's bare-value behavior is
// stable: not a byte of fold wording anywhere, attribution intact.
func TestSFDRunningTwinBareWithoutSibling(t *testing.T) {
	records := []types.ObservationRecord{sfdQ6Anchor(), sfdQ6TwinObs()}
	md := audit730Render(t, audit730Bus(""), records, "")
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: banned clause words
	// migrate — 已满频满核→已按大核满频, 频点数据不全→频率数据不全 (new "CPU 频率数据不全").
	for _, banned := range []string{"供给折算缺口", "机制构成", "已按全域最大核最高频", "频率数据不全"} {
		if strings.Contains(md, banned) {
			t.Fatalf("no sibling → no fold wording (%q leaked):\n%s", banned, md)
		}
	}
	if !strings.Contains(vs2Despace(md), "有效归因58.919ms") {
		t.Fatalf("bare attribution row must render:\n%s", md)
	}
}

// SFD 复核 F3 (render surface): an R2 ×N SUM row clears the group-first
// seed's inherited fold group — the member SUM must render WITHOUT any fold
// clause beside it (the 伪对比 shape: a single member's 供给折算缺口 5.000ms
// pasted next to the three-member 42.000ms sum).
func TestSFDXNSumRowCarriesNoFoldClause(t *testing.T) {
	member := func(id, claimKey string, impact float64, lineStart, lineEnd int, extra ...string) types.ObservationRecord {
		return projV3Obs(id, "root_cause_primary", claimKey,
			"#RxComputationT-16816", "running", "", impact, lineStart, lineEnd,
			append([]string{
				"tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
				"chain_depth=1", "dominant_state=running",
			}, extra...)...)
	}
	records := []types.ObservationRecord{
		sfdQ6Anchor(),
		member("m1", "root_cause_primary:m1", 10.000, 45689, 50000, "rank=2"),
		member("m2", "root_cause_primary:m2", 12.000, 50001, 60000, "rank=3"),
		// The impact-major seed carries the fold notes (the inheritance shape).
		member("m3", "root_cause_primary:m3", 20.000, 60001, 79142, "rank=1",
			"supply_fold_deficit_ms=5.000", "supply_fold_ideal_ms=15.000",
			"fold_basis=known=20.000ms,unknown=0.000ms"),
	}
	md := audit730Render(t, audit730Bus(""), records, "")
	if !strings.Contains(md, "3次") || !strings.Contains(md, "42.000") {
		t.Fatalf("the 3次 member SUM row must render:\n%s", md)
	}
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: banned clause words
	// migrate to the new forms (see TestSFDRunningTwinBareWithoutSibling).
	for _, banned := range []string{"供给折算缺口", "已按全域最大核最高频", "频率数据不全", "机制构成"} {
		if strings.Contains(md, banned) {
			t.Fatalf("a member SUM must carry no single-member fold clause (%q leaked):\n%s", banned, md)
		}
	}
}

// Pin ④: AllKnown=false travels with the join — the running row renders the
// honest 频点数据不全 branch (deficit under the floor + unknown coverage),
// never the affirmative claim. The count proves the TWIN carries it too: the
// donor alone renders it on 2 surfaces (fence + detail table), the joined
// primary twin adds the conclusion line + its own fence row + detail row.
func TestSFDRunningTwinJoinUnknownBasisBranch(t *testing.T) {
	records := []types.ObservationRecord{sfdQ6Anchor(), sfdQ6TwinObs(), sfdQ6DonorObs(
		"supply_fold_deficit_ms=0.400", "supply_fold_ideal_ms=30.000",
		"fold_basis=known=30.400ms,unknown=28.519ms")}
	md := audit730Render(t, audit730Bus(""), records, "")
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: "频点数据不全" →
	// "CPU 频率数据不全" (供给折算族); count moves to the despaced surface (the
	// new clause carries an ASCII space the wrap/collapse may not preserve);
	// negative pin migrates to the new affirmative form 已按大核满频.
	// PTV8-RCR-C (§24.9 G4 co-repair, 2026-07-08). EVOLUTION RECORD: this
	// shape MINTED a deficit (0.400ms) — the unknown-basis clause now states
	// the lower bound instead of denying the published number ("无法折算"
	// beside 缺口 0.400ms was the F3 contradiction family).
	despaced := vs2Despace(md)
	if got := strings.Count(despaced, "CPU频率数据部分缺失,已计部分按全域最大核最高频折算:缺口0.400ms为下界(运行频点非最高)"); got < 3 {
		t.Fatalf("joined twin must render the unknown-basis branch too (donor alone = 2 surfaces, got %d):\n%s", got, md)
	}
	if strings.Contains(despaced, "无法折算") {
		t.Fatalf("the incomplete-data sentence must not deny the published deficit:\n%s", md)
	}
	if strings.Contains(despaced, "已按全域最大核最高频") {
		t.Fatalf("partial coverage must never make the affirmative claim:\n%s", md)
	}
}
