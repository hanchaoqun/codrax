package tool

// answer_document_projection_clusterfix2_test.go — freq_only wording-fork
// display pins. EVOLUTION RECORD (CLUSTERSTREAM-1 件3, §29.193.1, 2026-07-21;
// supersedes the CLUSTER-FIX-2 件1 single-fork boundary): the S1 batch forked
// ONLY the single-cluster cause and deliberately folded the other six causes
// into the generic 「簇结构不可判」 sentence; the CLUSTERDIAG dossier (§3.3)
// adjudicated that fold a self-diagnosis gap (test_trace_02: three candidate
// arms indistinguishable on the answer face), so the clause single point now
// forks PER ARM. Absence (reason-less pre-batch records) still keeps the
// ruled generic wording byte-identically.
//
// 并注 (件3, dossier §4 witness runnable_2.txt:225): the SUFFIX form — always
// embedded after a sentence already carrying a 折算 verb — renders the merged
// single note 「,<cause>,按频率比」 instead of splicing a second
// 「按纯频率比折算」 verb into the same parenthesis; the standalone CLAUSE
// keeps the full term (legend seat unchanged, both terms legend-taught).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Wire-token drift pins (the CAP mirror precedent) — the full closed set.
func TestClusterFix2ReasonTokenMirrorsEngine(t *testing.T) {
	pairs := [][2]string{
		{runtimeTraceCapabilityFreqOnlyReasonNoDomains, tracequery.CoreCapabilityFreqOnlyReasonNoDomains},
		{runtimeTraceCapabilityFreqOnlyReasonNoSampledCluster, tracequery.CoreCapabilityFreqOnlyReasonNoSampledCluster},
		{runtimeTraceCapabilityFreqOnlyReasonSingleCluster, tracequery.CoreCapabilityFreqOnlyReasonSingleCluster},
		{runtimeTraceCapabilityFreqOnlyReasonClusterOverflow, tracequery.CoreCapabilityFreqOnlyReasonClusterOverflow},
		{runtimeTraceCapabilityFreqOnlyReasonFmaxTie, tracequery.CoreCapabilityFreqOnlyReasonFmaxTie},
		{runtimeTraceCapabilityFreqOnlyReasonComoveFloor, tracequery.CoreCapabilityFreqOnlyReasonComoveFloor},
		{runtimeTraceCapabilityFreqOnlyReasonComoveFloorBurst, tracequery.CoreCapabilityFreqOnlyReasonComoveFloorSingleBurst},
	}
	for _, p := range pairs {
		if p[0] != p[1] {
			t.Fatalf("display reason token %q drifted from the engine constant %q (core_capability.go)", p[0], p[1])
		}
	}
}

// The clause single point: every typed arm names itself; absence keeps the
// legacy bytes EXACTLY (zh + EN).
func TestClusterFix2PerArmClauseFork(t *testing.T) {
	const freqOnly = runtimeTraceCapabilitySourceFreqOnly
	wantZH := map[string]string{
		"": "簇结构不可判,按纯频率比折算",
		runtimeTraceCapabilityFreqOnlyReasonSingleCluster:    "仅单簇有频点采样,无跨簇算力信息,按纯频率比折算(单簇内等价)",
		runtimeTraceCapabilityFreqOnlyReasonFmaxTie:          "簇最高频并列,核类排序不可判,按纯频率比折算",
		runtimeTraceCapabilityFreqOnlyReasonClusterOverflow:  "簇数超出核类表(>4),按纯频率比折算",
		runtimeTraceCapabilityFreqOnlyReasonComoveFloor:      "簇合并证据不足(共见证变迁<2),按纯频率比折算",
		runtimeTraceCapabilityFreqOnlyReasonComoveFloorBurst: "簇合并证据不足(共见证变迁<2),按纯频率比折算",
		runtimeTraceCapabilityFreqOnlyReasonNoDomains:        "无频点采样,按纯频率比折算",
		runtimeTraceCapabilityFreqOnlyReasonNoSampledCluster: "声明簇均无频点采样,按纯频率比折算",
	}
	wantEN := map[string]string{
		"": "cluster structure unjudged, frequency-ratio fold only",
		runtimeTraceCapabilityFreqOnlyReasonSingleCluster:    "only one cluster carries frequency samples — no cross-cluster capability information, frequency-ratio fold only (equivalent within the single cluster)",
		runtimeTraceCapabilityFreqOnlyReasonFmaxTie:          "cluster peak frequencies tie — class order unjudgeable, frequency-ratio fold only",
		runtimeTraceCapabilityFreqOnlyReasonClusterOverflow:  "cluster count exceeds the class table (>4), frequency-ratio fold only",
		runtimeTraceCapabilityFreqOnlyReasonComoveFloor:      "insufficient cluster-merge evidence (co-witnessed transitions <2), frequency-ratio fold only",
		runtimeTraceCapabilityFreqOnlyReasonComoveFloorBurst: "insufficient cluster-merge evidence (co-witnessed transitions <2), frequency-ratio fold only",
		runtimeTraceCapabilityFreqOnlyReasonNoDomains:        "no frequency samples, frequency-ratio fold only",
		runtimeTraceCapabilityFreqOnlyReasonNoSampledCluster: "declared clusters carry no frequency samples, frequency-ratio fold only",
	}
	for reason, want := range wantZH {
		if got := runtimeTraceProjCapabilityCaliberClauseReason(freqOnly, "", reason, true); got != want {
			t.Fatalf("zh clause for %q = %q, want %q", reason, got, want)
		}
	}
	for reason, want := range wantEN {
		if got := runtimeTraceProjCapabilityCaliberClauseReason(freqOnly, "", reason, false); got != want {
			t.Fatalf("EN clause for %q = %q, want %q", reason, got, want)
		}
	}
	// An unknown FUTURE token fails open to the generic wording (never "").
	if got := runtimeTraceProjCapabilityCaliberClauseReason(freqOnly, "", "future_token", true); got != wantZH[""] {
		t.Fatalf("unknown reason must fail open to the generic clause, got %q", got)
	}
	// The judged/default arms are reason-transparent (no fork outside
	// freq_only) — spot-pin the default form.
	if runtimeTraceProjCapabilityCaliberClauseReason(runtimeTraceCapabilitySourceDefault, "", runtimeTraceCapabilityFreqOnlyReasonSingleCluster, true) !=
		runtimeTraceProjCapabilityCaliberClauseTopo(runtimeTraceCapabilitySourceDefault, "", true) {
		t.Fatalf("a stray reason on a judged record must not fork the default wording")
	}
	// The legend seat is unchanged and both taught terms stay in play: full
	// clauses keep 按纯频率比折算, the joined suffix keeps 按频率比.
	if mark, ok := runtimeTraceProjCapabilityCaliberMarkTopo(freqOnly, ""); !ok || mark != runtimeTraceProjMarkCaliberFreqOnlyCapability {
		t.Fatalf("the freq_only legend seat must be unchanged")
	}
}

// The joined SUFFIX single point (件3 并注): one merged note — cause + the
// legend-taught 按频率比 short term — for EVERY freq_only arm; the second
// 折算 verb never splices into the host sentence's parenthesis again.
func TestClusterStreamJoinedSuffixMergesFreqOnlyNote(t *testing.T) {
	const freqOnly = runtimeTraceCapabilitySourceFreqOnly
	cases := map[string]string{
		"": ",簇结构不可判,按频率比",
		runtimeTraceCapabilityFreqOnlyReasonSingleCluster:    ",仅单簇有频点采样,按频率比",
		runtimeTraceCapabilityFreqOnlyReasonFmaxTie:          ",簇最高频并列,核类排序不可判,按频率比",
		runtimeTraceCapabilityFreqOnlyReasonClusterOverflow:  ",簇数超出核类表(>4),按频率比",
		runtimeTraceCapabilityFreqOnlyReasonComoveFloor:      ",簇合并证据不足(共见证变迁<2),按频率比",
		runtimeTraceCapabilityFreqOnlyReasonComoveFloorBurst: ",簇合并证据不足(共见证变迁<2),按频率比",
		runtimeTraceCapabilityFreqOnlyReasonNoDomains:        ",无频点采样,按频率比",
	}
	for reason, want := range cases {
		got := runtimeTraceProjCapabilityCaliberSuffixReason(freqOnly, "", reason, true)
		if got != want {
			t.Fatalf("zh joined suffix for %q = %q, want %q", reason, got, want)
		}
		if strings.Contains(got, "折算") {
			t.Fatalf("并注: the joined suffix must not re-splice a 折算 verb, got %q", got)
		}
	}
	if got := runtimeTraceProjCapabilityCaliberSuffixReason(freqOnly, "", "", false); got != ", cluster structure unjudged, frequency-ratio basis" {
		t.Fatalf("EN joined suffix = %q", got)
	}
	// Non-freq_only sources keep the unmerged clause suffix byte-identically.
	if got := runtimeTraceProjCapabilityCaliberSuffixReason(runtimeTraceCapabilitySourceDefault, runtimeTraceCapabilityTopologyComovement, "", true); got != ",按实测频点共动分簇折算" {
		t.Fatalf("judged suffix must stay unmerged, got %q", got)
	}
}

// End-to-end through the supply-fold clause body: the dominant deficit form
// and the compressed no-deficit form both fork on the node's typed reason —
// and keep the ruled generic wording without it.
func TestClusterFix2SupplyFoldClauseForkEndToEnd(t *testing.T) {
	node := capClauseNode(5, 15, 20, 0, 5, runtimeTraceCapabilitySourceFreqOnly)
	node.SupplyFoldCapabilityFreqOnlyReason = runtimeTraceCapabilityFreqOnlyReasonSingleCluster
	clause, _, ok := runtimeTraceProjSupplyFoldClause(node, 0, false, true)
	if !ok || !strings.Contains(clause, "仅单簇有频点采样,按频率比") {
		t.Fatalf("the dominant deficit clause must carry the single-cluster joined note:\n%s", clause)
	}
	if strings.Contains(clause, "簇结构不可判") {
		t.Fatalf("the single-cluster form must not also claim 簇结构不可判:\n%s", clause)
	}
	// UXR-1 §29.36.4 ② stands: still no core-class word, still the
	// 全域最高频 basis word.
	if strings.Contains(clause, "大核") || !strings.Contains(clause, "按全域最高频折算") {
		t.Fatalf("core-class honesty gate / basis word must be unchanged:\n%s", clause)
	}
	// 并注: exactly ONE 折算 verb inside the deficit parenthesis (the lead
	// 按全域最高频折算) — the suffix contributes 按频率比 only.
	paren := clause[strings.Index(clause, "("):]
	if strings.Count(paren[:strings.Index(paren, ")")+len(")")], "折算") != 1 {
		t.Fatalf("并注: the deficit parenthesis must carry one 折算 verb:\n%s", clause)
	}
	// Compressed no-deficit form, per-arm (fmax_tie names itself now).
	noDeficit := capClauseNode(0, 2.641, 2.641, 0, 0, runtimeTraceCapabilitySourceFreqOnly)
	noDeficit.SupplyFoldCapabilityFreqOnlyReason = runtimeTraceCapabilityFreqOnlyReasonFmaxTie
	compressed, _, ok := runtimeTraceProjSupplyFoldClause(noDeficit, 0, false, true)
	if !ok || !strings.Contains(compressed, "已按全域最高频(或接近)运行·无供给折算(簇最高频并列,核类排序不可判,按频率比)") {
		t.Fatalf("the compressed no-deficit form must fork per arm:\n%s", compressed)
	}
	noDeficit.SupplyFoldCapabilityFreqOnlyReason = runtimeTraceCapabilityFreqOnlyReasonSingleCluster
	compressed, _, ok = runtimeTraceProjSupplyFoldClause(noDeficit, 0, false, true)
	if !ok || !strings.Contains(compressed, "已按全域最高频(或接近)运行·无供给折算(仅单簇有频点采样,按频率比)") {
		t.Fatalf("the single-cluster compressed form keeps its S1 bytes:\n%s", compressed)
	}
	compressedEN, _, ok := runtimeTraceProjSupplyFoldClause(noDeficit, 0, false, false)
	if !ok || !strings.Contains(compressedEN, "no supply fold (single-cluster samples only, frequency-ratio basis)") {
		t.Fatalf("the EN compressed form must fork too:\n%s", compressedEN)
	}
	// Without the reason both forms keep the pre-batch bytes (the CAP pin's
	// exact strings — absence preserves the ruled generic surface).
	legacy, _, ok := runtimeTraceProjSupplyFoldClause(capClauseNode(0, 2.641, 2.641, 0, 0, runtimeTraceCapabilitySourceFreqOnly), 0, false, true)
	if !ok || !strings.Contains(legacy, "已按全域最高频(或接近)运行·无供给折算(簇结构不可判,按频率比)") {
		t.Fatalf("the reason-less compressed form must keep the legacy bytes:\n%s", legacy)
	}
}

// The gated lane (DISPHYG-3 件7 reason twin) consumes the same single points:
// reason-less records keep the generic cause (joined form), the typed tokens
// fork, and the full inversion composition mirror carries the fork.
func TestDisphyg3GatedLaneReasonTwinForksPerArm(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		GatedCapabilitySource: runtimeTraceCapabilitySourceFreqOnly,
	}
	suffix := runtimeTraceProjCapabilityCaliberSuffixReason(node.GatedCapabilitySource, node.GatedTopologySource, node.GatedCapabilityFreqOnlyReason, true)
	if suffix != ",簇结构不可判,按频率比" {
		t.Fatalf("the reason-less gated lane must keep the generic cause (joined note), got %q", suffix)
	}
	node.GatedCapabilityFreqOnlyReason = runtimeTraceCapabilityFreqOnlyReasonSingleCluster
	suffix = runtimeTraceProjCapabilityCaliberSuffixReason(node.GatedCapabilitySource, node.GatedTopologySource, node.GatedCapabilityFreqOnlyReason, true)
	if suffix != ",仅单簇有频点采样,按频率比" {
		t.Fatalf("the single-cluster gated lane must fork, got %q", suffix)
	}
	if strings.Contains(suffix, "簇结构不可判") {
		t.Fatalf("the single-cluster gated lane must not claim 簇结构不可判: %q", suffix)
	}
	suffixEN := runtimeTraceProjCapabilityCaliberSuffixReason(node.GatedCapabilitySource, node.GatedTopologySource, node.GatedCapabilityFreqOnlyReason, false)
	if !strings.Contains(suffixEN, "single-cluster samples only") {
		t.Fatalf("the EN gated lane must fork too, got %q", suffixEN)
	}
	// The fail-open lossless composition mirror reads the same twin.
	node.GatedRunnableMS, node.GatedRunningDeficitMS = 1.5, 0.5
	composition := runtimeTraceProjInversionCompositionText(node, true)
	if !strings.Contains(composition, "仅单簇有频点采样") || strings.Contains(composition, "簇结构不可判") {
		t.Fatalf("the composition mirror must fork with the twin:\n%s", composition)
	}
	// EMISSION-PATH arm (突变复核 M6 幸存实证 2026-07-20: a call-site revert to
	// the reason-less wrapper survived the unit pins — the primary 拆解子行
	// builder must consume the twin itself, not just the shared clause).
	node.EffectiveImpactMS = node.GatedRunnableMS + node.GatedRunningDeficitMS
	components, _, ok := runtimeTraceProjInversionComponents(node, false, true)
	if !ok {
		t.Fatalf("the inversion components builder must accept the gated composite")
	}
	joined := ""
	for _, component := range components {
		joined += component.CaliberFull + "\n"
	}
	if !strings.Contains(joined, "仅单簇有频点采样") || strings.Contains(joined, "簇结构不可判") {
		t.Fatalf("the 拆解子行 caliber words must fork with the twin:\n%s", joined)
	}
	// The fmax_tie token names itself on the gated lane too (per-arm fork).
	node.GatedCapabilityFreqOnlyReason = runtimeTraceCapabilityFreqOnlyReasonFmaxTie
	if got := runtimeTraceProjCapabilityCaliberSuffixReason(node.GatedCapabilitySource, node.GatedTopologySource, node.GatedCapabilityFreqOnlyReason, true); got != ",簇最高频并列,核类排序不可判,按频率比" {
		t.Fatalf("the gated fmax_tie arm must name itself, got %q", got)
	}
}

// The DISPHYG-3 件7 wire parse pin lives beside the CAP-2 gated-topology
// parse pin (internal/types/trace_causal_projection_cap2_test.go — the same
// note→node fixture family the twin key extends).

// --- 图例承诺面 completeness (复核 F2/F3/F4, 2026-07-21) ----------------------

// 图例是承诺面 (§29.183 G9 同族纪律): every closed-set freq_only arm's row
// cause word must be taught by the …FreqOnlyCapability legend enumeration.
// The pin walks runtimeTraceCapabilityFreqOnlyReasonClosedSet — for each arm
// there must be an enumerated legend term that is a VERBATIM substring of the
// arm's row phrase AND of the legend entry (zh + EN). A future arm added to
// the closed set without its legend term goes red here (the cold-read F2
// witness: the fifth arm 声明簇均无频点采样 rendered on rows while the legend
// enumerated only four causes).
func TestClusterStreamFreqOnlyLegendEnumerationComplete(t *testing.T) {
	// Roster completeness first: the closed set must cover the generic arm
	// plus all seven typed constants (the var is the single roster both this
	// pin and the wrap-atom derivation walk).
	wantRoster := []string{"",
		runtimeTraceCapabilityFreqOnlyReasonNoDomains,
		runtimeTraceCapabilityFreqOnlyReasonNoSampledCluster,
		runtimeTraceCapabilityFreqOnlyReasonSingleCluster,
		runtimeTraceCapabilityFreqOnlyReasonClusterOverflow,
		runtimeTraceCapabilityFreqOnlyReasonFmaxTie,
		runtimeTraceCapabilityFreqOnlyReasonComoveFloor,
		runtimeTraceCapabilityFreqOnlyReasonComoveFloorBurst,
	}
	inRoster := map[string]bool{}
	for _, token := range runtimeTraceCapabilityFreqOnlyReasonClosedSet {
		inRoster[token] = true
	}
	for _, token := range wantRoster {
		if !inRoster[token] {
			t.Fatalf("closed-set roster lost token %q", token)
		}
	}
	var entry runtimeTraceProjLegendEntry
	found := false
	for _, e := range runtimeTraceProjLegendCatalog() {
		if e.Mark == runtimeTraceProjMarkCaliberFreqOnlyCapability {
			entry, found = e, true
			break
		}
	}
	if !found {
		t.Fatalf("freq_only capability legend entry missing from the catalog")
	}
	terms := map[bool][]string{
		true: {"簇结构不可判", "仅单簇有频点采样", "簇最高频并列", "簇数超出核类表",
			"簇合并证据不足(共见证变迁<2)", "无频点采样", "声明簇均无频点采样"},
		false: {"cluster structure", "single-cluster samples only", "cluster peak frequencies tie",
			"cluster count exceeds the class table", "insufficient cluster-merge evidence (co-witnessed transitions <2)",
			"no frequency samples", "declared clusters carry no frequency samples"},
	}
	for _, zh := range []bool{true, false} {
		text := entry.ZH
		if !zh {
			text = entry.EN
		}
		// Every enumerated term must stand in the legend entry…
		for _, term := range terms[zh] {
			if !strings.Contains(text, term) {
				t.Fatalf("legend entry (zh=%v) lost the enumerated cause term %q:\n%s", zh, term, text)
			}
		}
		// …and every closed-set arm's row phrase must be covered by a term.
		for _, token := range runtimeTraceCapabilityFreqOnlyReasonClosedSet {
			phrase := runtimeTraceProjFreqOnlyCauseShort(token, zh)
			covered := false
			for _, term := range terms[zh] {
				if strings.Contains(phrase, term) {
					covered = true
					break
				}
			}
			if !covered {
				t.Fatalf("arm %q row phrase %q (zh=%v) has no legend enumeration term — 图例是承诺面, add the term to the legend AND this pin", token, phrase, zh)
			}
		}
	}
	// 复核 F3: the 下界 entry's exception key names BOTH taught row terms —
	// the 并注 suffix rows carry the short 按频率比 form, and a single-term
	// key would misread them as class-priced.
	for _, e := range runtimeTraceProjLegendCatalog() {
		if e.Mark != runtimeTraceProjMarkCaliberLowerBound {
			continue
		}
		if !strings.Contains(e.ZH, "标注「按纯频率比折算」/「按频率比」的行除外") {
			t.Fatalf("下界 exception key must name both row terms:\n%s", e.ZH)
		}
		if !strings.Contains(e.EN, "「frequency-ratio fold only」/「frequency-ratio basis」 excepted") {
			t.Fatalf("EN 下界 exception key must name both row terms:\n%s", e.EN)
		}
	}
}

// 复核 F4 (DISPLAY-HYG 主张词不可断): every closed-set arm's zh cause phrase
// joins the wrap-atom table clause by clause (the zh comma stays a legal
// break boundary; a wrap must never bisect 声明簇均无频点采样 or
// 核类排序不可判 mid-claim on a long thread-name row). The table entries are
// DERIVED from the closed set in init(), so this pin reds only if that
// derivation is unwired.
func TestClusterStreamFreqOnlyCausePhrasesAreWrapAtoms(t *testing.T) {
	atoms := map[string]bool{}
	for _, atom := range runtimeTraceProjWrapAtomCompounds {
		atoms[atom] = true
	}
	for _, token := range runtimeTraceCapabilityFreqOnlyReasonClosedSet {
		for _, clause := range strings.Split(runtimeTraceProjFreqOnlyCauseShort(token, true), ",") {
			if clause == "" {
				continue
			}
			if !atoms[clause] {
				t.Fatalf("cause clause %q (arm %q) missing from the wrap-atom table — the init() derivation is unwired", clause, token)
			}
		}
	}
}
