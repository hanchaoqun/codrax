package tool

// answer_document_projection_chainguard_test.go — CHAINGUARD-1 display pins
// (§29.204/§29.204.1 spec §3③/§5 P4, 2026-07-22): the census wire riding rank
// seats end-to-end, the census=none SECOND seat gate (board/badge/crown), the
// ◎ chip census enum mapping (engine 同源 — the zero-credential over-claim
// word dies), the P4 full-seat scan (every rendered ⛓ seat row wears a chip ∨
// typed foreignSelf), the progressive-compatibility arm (absent census keeps
// legacy behavior byte-identically), and the F-E Role="" default pin
// (§29.208 P3 记档).

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func chainguardSeatNode(evidence, subject, token, census string, rank int, eff float64, line int) types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: evidence,
		Subject: subject, Object: token, TypeToken: token,
		StateKind: "runnable", ChainRelevance: "on_chain", ChainDepth: 1,
		ChainCredentialCensus: census,
		ImpactMS:              eff, CumulativeImpactMS: eff, EffectiveImpactMS: eff,
		QueryWindowStartTs: 10.0, QueryWindowEndTs: 10.2,
		Rank: rank, Tier: "primary", Confidence: 0.72,
		LineStart: line, LineEnd: line + 10,
	}
}

// TestChainguardCensusWireRidesChainedRankSeats — 件2 wire half, engine-built:
// every chain-lane ranked rank-row record of a CHAINED board carries the
// chain_credential_census note; the chainless twin board carries NONE at all
// (population exemption by absence — 渐进兼容 is structural).
func TestChainguardCensusWireRidesChainedRankSeats(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chainguard_wire.systrace")
	if err := os.WriteFile(path, []byte(ispgapChainlessDTrace), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	chained := tracequery.Run(idx, tracequery.Query{View: "root_cause_rank", PID: 100, TimeStart: 1.0, TimeEnd: 1.15, MinDurationMs: 0.05})
	records := traceQueryTypedObservations(chained, "chainguard_wire.systrace", "payload-ref", "raw-ref", "", time.Unix(1753100000, 0).UTC())
	censusNotes := 0
	for _, record := range records {
		if !strings.HasPrefix(strings.TrimSpace(record.ClaimKey), "root_cause_") {
			continue
		}
		var relevance, census, rank string
		for _, note := range record.RichNotes {
			if v, ok := strings.CutPrefix(note, types.TraceNoteKeyChainRelevance+"="); ok {
				relevance = strings.TrimSpace(v)
			}
			if v, ok := strings.CutPrefix(note, types.TraceNoteKeyChainCredentialCensus+"="); ok {
				census = strings.TrimSpace(v)
			}
			if v, ok := strings.CutPrefix(note, types.TraceNoteKeyRank+"="); ok {
				rank = strings.TrimSpace(v)
			}
		}
		if relevance == "on_chain" && rank != "" && rank != "0" {
			if census == "" {
				t.Fatalf("chained-board chain-lane ranked seat must carry the census note: %s %s notes=%v", record.Subject, record.ClaimKey, record.RichNotes)
			}
			censusNotes++
		}
	}
	if censusNotes == 0 {
		t.Fatal("fixture premise: the chained board must publish ranked chain-lane seats")
	}
	chainless := tracequery.Run(idx, tracequery.Query{View: "root_cause_rank", TimeStart: 1.0, TimeEnd: 1.15, MinDurationMs: 0.05})
	chainlessRecords := traceQueryTypedObservations(chainless, "chainguard_wire.systrace", "payload-ref-2", "raw-ref-2", "", time.Unix(1753100060, 0).UTC())
	for _, record := range chainlessRecords {
		for _, note := range record.RichNotes {
			if strings.HasPrefix(note, types.TraceNoteKeyChainCredentialCensus+"=") {
				t.Fatalf("chainless board must carry no census note (§29.36.2 exempt universe): %s %v", record.Subject, record.RichNotes)
			}
		}
	}
}

// TestChainguardCensusNoneSecondSeatGate — 件2 board half: a census=none seat
// (the stale / cross-query merged artifact form: Rank>0 + on_chain relevance
// still riding the row) holds NO valid seat on a chained tree — no badge, no
// election-board membership, never the crown; the census-less legacy twin
// (progressive compatibility) keeps its seat byte-identically.
func TestChainguardCensusNoneSecondSeatGate(t *testing.T) {
	proven := chainguardSeatNode("cg-proven", "workerA-301", "runnable_wait", "interval_proven", 1, 8.0, 10)
	none := chainguardSeatNode("cg-none", "isplogd-1300", "d_state_or_io_wait", "none", 2, 150.0, 30)
	none.StateKind = "d_sleep"
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"waker-1", "app-100"},
		WindowStartTs: 10.0, WindowEndTs: 10.2,
		OnChainCauses: []types.TraceCausalProjectionNode{proven, none},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background} {
		for _, row := range rows {
			if row.Node.EvidenceID == "cg-none" && row.Badge != 0 {
				t.Fatalf("件2: a census=none seat must never wear a badge: %+v", row)
			}
		}
	}
	board := runtimeTraceProjRankBoard(runtimeTraceProjLeadElectionRows(model))
	for _, row := range board {
		if row.Node.EvidenceID == "cg-none" {
			t.Fatalf("件2: a census=none seat must never enter the election board: %+v", row.Node)
		}
	}
	if len(board) == 0 || board[0].Node.EvidenceID != "cg-proven" {
		t.Fatalf("件2: the credentialed seat must lead the board, got %+v", board)
	}
	// Progressive compatibility: the SAME shape WITHOUT a census note keeps
	// its legacy seat (pre-census artifacts must not lose faces).
	legacy := none
	legacy.EvidenceID = "cg-legacy"
	legacy.ChainCredentialCensus = ""
	legacyProjection := types.TraceCausalProjection{
		WakeupPath:    []string{"waker-1", "app-100"},
		WindowStartTs: 10.0, WindowEndTs: 10.2,
		OnChainCauses: []types.TraceCausalProjectionNode{proven, legacy},
	}
	legacyModel := buildRuntimeTraceProjTreeModel(legacyProjection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	legacySeated := false
	for _, row := range runtimeTraceProjRankBoard(runtimeTraceProjLeadElectionRows(legacyModel)) {
		if row.Node.EvidenceID == "cg-legacy" {
			legacySeated = true
		}
	}
	if !legacySeated {
		t.Fatalf("渐进兼容: the census-less legacy artifact must keep its board seat")
	}
}

// TestChainguardChipCensusMapping — 件3: with the engine census riding the
// row the ◎ chip is a pure word-face mapping of the enum — member_inherited /
// interval_proven / target_self / wakeup_anchored map to their family words
// on both language faces (the last two explicit since dual-review F-2: the
// basis-less residue must not fall to the completeness arm's 交集证明), and
// the census=none violation seat wears NO credential word (the pre-CHAINGUARD
// completeness arm minted 交集证明 on exactly this zero-credential shape —
// the §29.202 chip 穿透 over-claim dies here). The ②形 divergence pair
// (inheritance bit ∧ non-empty segments — engine-unreachable today: segments
// mint only on the VIEW face and the inheritance bit retires on HULL-CRED
// keeps) is pinned on BOTH sides so any future flip is a deliberate red.
func TestChainguardChipCensusMapping(t *testing.T) {
	for _, zh := range []bool{true, false} {
		inherited := runtimeTraceProjTreeRow{HasData: true, Kind: runtimeTraceProjTreeRowChain,
			Node: chainguardSeatNode("cg-m1", "workerA-301", "runnable_wait", "member_inherited", 1, 8.0, 10)}
		proven := runtimeTraceProjTreeRow{HasData: true, Kind: runtimeTraceProjTreeRowChain,
			Node: chainguardSeatNode("cg-m2", "workerB-302", "runnable_wait", "interval_proven", 2, 6.0, 30)}
		violation := runtimeTraceProjTreeRow{HasData: true, Kind: runtimeTraceProjTreeRowChain,
			Node: chainguardSeatNode("cg-m3", "workerC-303", "d_state_or_io_wait", "none", 3, 4.0, 50)}
		wantInherited, wantProven := tracefence.CredentialTierMemberInheritedZH, tracefence.CredentialTierIntervalProvenZH
		if !zh {
			wantInherited, wantProven = tracefence.CredentialTierMemberInheritedEN, tracefence.CredentialTierIntervalProvenEN
		}
		if got := runtimeTraceProjElimQualifier(inherited, runtimeTraceProjOrdinalChannelChain, zh, nil); got != wantInherited {
			t.Fatalf("件3 (zh=%v): census member_inherited must map %q, got %q", zh, wantInherited, got)
		}
		if got := runtimeTraceProjElimQualifier(proven, runtimeTraceProjOrdinalChannelChain, zh, nil); got != wantProven {
			t.Fatalf("件3 (zh=%v): census interval_proven must map %q, got %q", zh, wantProven, got)
		}
		if got := runtimeTraceProjElimQualifier(violation, runtimeTraceProjOrdinalChannelChain, zh, nil); got != "" {
			t.Fatalf("件3 (zh=%v): a census=none seat must wear NO credential word (the over-claim killer), got %q", zh, got)
		}
		// Dual-review F-2 explicit arms: the basis-less census=target_self
		// residue (the R8 SubjectIsAnalysisTarget mint without a display
		// self-qualifier) and the basis-less census=wakeup_anchored residue
		// wear their OWN family words — never the completeness arm's 交集证明.
		wantSelf, wantWakeup := tracefence.CredentialTierTargetSelfZH, tracefence.CredentialTierWakeupAnchoredZH
		if !zh {
			wantSelf, wantWakeup = tracefence.CredentialTierTargetSelfEN, tracefence.CredentialTierWakeupAnchoredEN
		}
		self := runtimeTraceProjTreeRow{HasData: true, Kind: runtimeTraceProjTreeRowChain,
			Node: chainguardSeatNode("cg-m4", "CookieMonsterCl-59843", "scheduler_latency", "target_self", 4, 3.0, 70)}
		if got := runtimeTraceProjElimQualifier(self, runtimeTraceProjOrdinalChannelChain, zh, nil); got != wantSelf {
			t.Fatalf("件3 (zh=%v): basis-less census target_self must map %q, got %q", zh, wantSelf, got)
		}
		// XLANE-1 定谳⑤ still holds through the explicit arm: a foreign-subject
		// fused self row never wears 目标自身, census or no census.
		foreignSelfRow := self
		foreignSelfRow.SelfQualifierForeignSubject = true
		foreignSelfRow.SelfWallClockQualifier = true
		if got := runtimeTraceProjElimQualifier(foreignSelfRow, runtimeTraceProjOrdinalChannelChain, zh, nil); got != "" {
			t.Fatalf("件3 (zh=%v): the foreignSelf exception must survive the census target_self arm, got %q", zh, got)
		}
		wakeup := runtimeTraceProjTreeRow{HasData: true, Kind: runtimeTraceProjTreeRowChain,
			Node: chainguardSeatNode("cg-m5", "workerD-304", "runnable_wait", "wakeup_anchored", 5, 2.0, 90)}
		if got := runtimeTraceProjElimQualifier(wakeup, runtimeTraceProjOrdinalChannelChain, zh, nil); got != wantWakeup {
			t.Fatalf("件3 (zh=%v): basis-less census wakeup_anchored must map %q, got %q", zh, wantWakeup, got)
		}
		// ②形 负臂 pin (dual-review F-2): census=member_inherited ∧ non-empty
		// ChainCredentialSegments — the ENUM is the authority, so the census
		// word wins and the legacy arm's segments==0 negative gate must not
		// fork the word face.
		forked := inherited
		forked.Node.ChainIdentityInheritance = true
		forked.Node.ChainCredentialSegments = [][2]float64{{10.0, 10.1}}
		if got := runtimeTraceProjElimQualifier(forked, runtimeTraceProjOrdinalChannelChain, zh, nil); got != wantInherited {
			t.Fatalf("②形 (zh=%v): census member_inherited ∧ segments — the enum word must win, got %q", zh, got)
		}
		// …while the census-less twin keeps the legacy fork (inheritance ∧
		// segments falls past the segments==0 gate to the completeness arm).
		legacyForked := forked
		legacyForked.Node.ChainCredentialCensus = ""
		if got := runtimeTraceProjElimQualifier(legacyForked, runtimeTraceProjOrdinalChannelChain, zh, nil); got != wantProven {
			t.Fatalf("②形 渐进兼容 (zh=%v): census-less inheritance∧segments keeps the legacy completeness word %q, got %q", zh, wantProven, got)
		}
		// Legacy re-derivation stays byte-identical when the census is absent:
		// the same typed bits still elect the same words.
		legacyInherited := inherited
		legacyInherited.Node.ChainCredentialCensus = ""
		legacyInherited.Node.ChainIdentityInheritance = true
		if got := runtimeTraceProjElimQualifier(legacyInherited, runtimeTraceProjOrdinalChannelChain, zh, nil); got != wantInherited {
			t.Fatalf("渐进兼容 (zh=%v): the census-less inheritance bits must keep %q, got %q", zh, wantInherited, got)
		}
	}
}

// chainguardScanChipCompleteness is the P4 full-seat scan (§29.204.1 spec §5
// P4 — the §29.187① 「入 gate 者恰佩其一」 pin upgraded to 「凡渲染 ⛓ 席行
// chip 非空 ∨ typed foreignSelf」): every individually-seated rendered row on
// the ⛓ channel wears a non-empty credential chip, with the XLANE-1 定谳⑤
// foreign-subject fused self row as the ONE whitelisted suppression. Merged
// representatives (MergedCount>1) are IN the net (dual-review F-3,
// 2026-07-22): merged rows hold real seats — runtimeTraceProjRowValidSeat
// never excludes them, and the historical §29.202 witness carrier was
// exactly the E10(+1) aggregation face — so they answer to the
// aggregate-aware predicate chip 非空 ∨ typed foreignSelf ∨ valueless-mirror
// members (MergedValuelessCount>0: the ISPGAP F-A exempt mirror form may
// legitimately publish a bare representative). Only the two counted-roster
// fold kinds stay out. Returns the merged population count so the fixture
// tests can prove the arm is armed, not vacuous.
func chainguardScanChipCompleteness(t *testing.T, model runtimeTraceProjTreeModel, zh bool, label string) int {
	t.Helper()
	scanned, mergedScanned := 0, 0
	for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background} {
		for _, row := range rows {
			if !runtimeTraceProjRowSharedSeatArm(row) ||
				strings.TrimSpace(row.Node.ChainRelevance) != "on_chain" ||
				runtimeTraceProjRowOrdinalChannel(row) != runtimeTraceProjOrdinalChannelChain {
				continue
			}
			if row.Node.OnChainOverflowFold || row.Node.MicroAnchorFold {
				continue // counted rosters / fold aggregates: not individually-seated rows
			}
			scanned++
			foreignSelf := row.SelfQualifierForeignSubject &&
				(strings.TrimSpace(row.Node.OnChainBasis) == "self_deterministic_span" ||
					strings.TrimSpace(row.Node.OnChainBasis) == "self_wall_clock_interval" ||
					row.SelfWallClockQualifier)
			chip := runtimeTraceProjElimQualifier(row, runtimeTraceProjOrdinalChannelChain, zh, nil)
			if row.Node.MergedCount > 1 {
				mergedScanned++
				if chip == "" && !foreignSelf && row.Node.MergedValuelessCount == 0 {
					t.Fatalf("P4 merged-代表行臂 (%s zh=%v): rendered merged ⛓ representative wears no credential chip, is not the foreignSelf exception and carries no valueless-mirror members: %+v", label, zh, row.Node)
				}
				continue
			}
			if chip == "" && !foreignSelf {
				t.Fatalf("P4 全席扫描 (%s zh=%v): rendered ⛓ seat row wears no credential chip and is not the foreignSelf exception: %+v", label, zh, row.Node)
			}
		}
	}
	if scanned == 0 {
		t.Fatalf("P4 (%s): the scan population must not be empty", label)
	}
	return mergedScanned
}

// TestChainguardChipFullSeatScanEngineBoard — P4 over an ENGINE-built chained
// board (full production chain: trace → Run → observations → compile → tree
// model), zh + EN.
func TestChainguardChipFullSeatScanEngineBoard(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chainguard_scan.systrace")
	if err := os.WriteFile(path, []byte(ispgapChainlessDTrace), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	result := tracequery.Run(idx, tracequery.Query{View: "root_cause_rank", PID: 100, TimeStart: 1.0, TimeEnd: 1.15, MinDurationMs: 0.05})
	records := traceQueryTypedObservations(result, "chainguard_scan.systrace", "payload-ref", "raw-ref", "", time.Unix(1753100000, 0).UTC())
	set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: records})
	if len(set.Projections) != 1 {
		t.Fatalf("expected one projection, got %d", len(set.Projections))
	}
	for _, zh := range []bool{true, false} {
		model := buildRuntimeTraceProjTreeModel(set.Projections[0], newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		chainguardScanChipCompleteness(t, model, zh, "engine-chained")
	}
}

// TestChainguardChipFullSeatScanTiebaCarve — P4 over the committed customer
// carve (the §29.202 isplogcat replay board; skipped when the golden fixture
// is not checked out). This board carries real merged representatives
// ((+4)/(+3) aggregation rows — the E10(+1) witness carrier shape), so the
// merged-representative arm's population is asserted non-empty here: the net
// is provably armed on the witness's own carrier form.
func TestChainguardChipFullSeatScanTiebaCarve(t *testing.T) {
	if _, err := os.Stat(ispgapTiebaCarve); err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), ispgapTiebaCarve)
	if err != nil {
		t.Fatal(err)
	}
	result := tracequery.Run(idx, tracequery.Query{View: "root_cause_rank", PID: 59843, TimeStart: 34579.450627, TimeEnd: 34579.595131, MinDurationMs: 0.05})
	records := traceQueryTypedObservations(result, "donghu_tieba_frame.systrace", "payload-ref", "raw-ref", "", time.Unix(1753100000, 0).UTC())
	set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: records})
	if len(set.Projections) != 1 {
		t.Fatalf("expected one projection, got %d", len(set.Projections))
	}
	for _, zh := range []bool{true, false} {
		model := buildRuntimeTraceProjTreeModel(set.Projections[0], newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		if merged := chainguardScanChipCompleteness(t, model, zh, "tieba-carve"); merged == 0 {
			t.Fatalf("P4 merged-代表行臂 (zh=%v): the tieba carve must seat merged representatives — empty population means the arm is vacuous", zh)
		}
	}
}

// TestChainguardRankLaneRoleEmptyRoleDefaultPinned — F-E (§29.208 P3 记档③):
// runtimeTraceProjRankLaneRole("") is DELIBERATELY false — an empty Role is
// not a rank-lane role, so an empty-role empty-relevance node keeps the
// by-construction chain-view fallback on the display channel classifier.
// This default-open is bounded on two sides: (a) the projection compile
// stamps a typed Role on every root_cause_* record, so no engine-minted rank
// row can reach the display with an empty Role; (b) the census second seat
// gate (件2) still bars a census=none seat regardless of Role. Pinning the
// semantics makes any future flip a deliberate red, not a silent drift.
func TestChainguardRankLaneRoleEmptyRoleDefaultPinned(t *testing.T) {
	if runtimeTraceProjRankLaneRole("") {
		t.Fatal("F-E: empty Role is pinned NOT-rank-lane (by-construction chain-view default)")
	}
	node := types.TraceCausalProjectionNode{Role: "", ChainRelevance: ""}
	if got := runtimeTraceProjNodeOrdinalChannel(node); got != runtimeTraceProjOrdinalChannelChain {
		t.Fatalf("F-E: empty-role empty-relevance keeps the chain-view fallback channel, got %q", got)
	}
	// Both rank-lane roles keep the ISPGAP-1 background resolution.
	for _, role := range []string{types.TraceCausalRolePrimaryRootCause, types.TraceCausalRoleRootCauseContext} {
		node := types.TraceCausalProjectionNode{Role: role, ChainRelevance: ""}
		if got := runtimeTraceProjNodeOrdinalChannel(node); got != runtimeTraceProjOrdinalChannelBackground {
			t.Fatalf("F-E: rank-lane role %q with empty relevance must resolve background, got %q", role, got)
		}
	}
}
