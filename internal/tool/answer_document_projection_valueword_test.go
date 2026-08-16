package tool

// answer_document_projection_valueword_test.go — 值词库教学批 C2 pins
// (§29.104.16.1 M5②, 裁定③ §29.104.17 ③, 2026-07-17).
//
// The batch bridges the wire vocabulary the model greps in the blob
// (impact= / cumulative_impact= / effective_impact= / actual_impact=,
// gated_runnable_ms / member_fold_caliber …) to the report's display words
// (窗口投影 / 链上累计 / 有效归因 / 全额 / 折算 / 合计(共N段,同线程) …).
// Four legend deltas are pinned verbatim here (zh + EN), plus lockstep
// probes that tie the taught spellings to their single sources so the
// teaching face can never drift from the minted face (B4 词条-行内词 双向):
//
//   ① 链上累计 no-sub-chain arm + anti-「直达」 clause (glossary line);
//   ② 有效归因 eff↔cum relation clause (same-value = ONE measurement;
//      direction deferred to the row's caliber word — verified multi-lane:
//      折算/链上计入 below, 发生段账目/承自等待区间 above, default equal);
//   ③ the wire-mapping glossary line (column word ↔ trace_query row field);
//   ④ 裁定③ confidence-tier sentence (glossary line + the 行2 identity
//      legend entry): tier = per-lane numeric confidence folded through
//      fixed thresholds, never a cross-row evidence-strength comparison.

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestValueWordGlossaryLinesVerbatim pins the four C2 glossary deltas on the
// rendered key-metric table legend, zh and EN.
func TestValueWordGlossaryLinesVerbatim(t *testing.T) {
	zhMD := audit730Render(t, audit730Bus(""), audit730ChainObs(), "")
	for _, want := range []string{
		// ① no-sub-chain arm + anti-直达 (C2②/C2① fuel cutoff).
		"- 链上累计 = 该节点及其下钻子链沿唤醒链累计到关注线程的投影时长;无下钻子链的行不含子链份额,该值即该行自身账目的窗内投影;「链上」指沿唤醒链累计的方向,不是「直达关注线程」的额外传导声明。",
		// ② eff↔cum relation clause.
		"- 有效归因 = 该行计入根因排序的影响时长;与窗口投影不同时,行内口径词(全额/折算/单次最大等)说明取值方式;与链上累计同值时为同一测量的两个名目(非两项单独证据),异值时取值方式见行内口径词/标注(如 折算/链上计入 取小,发生段账目/承自等待区间 可高于链上累计)。",
		// ③ wire-mapping line.
		"- 与 trace_query 行字段对照:窗口投影 对应 impact=(JSON impact_ms);链上累计 对应 cumulative_impact=(cumulative_impact_ms);有效归因 对应 effective_impact=(effective_impact_ms);实际状态 对应 actual_impact=(actual_impact_ms);一行内多字段同值 = 同一测量的多个名目,不构成相互印证。",
		// ④ 裁定③ confidence-tier line.
		"- 置信 = 置信档(高/中/低):各证据来源的数值置信按固定阈值折词,不同来源基准不同,不作跨行证据强度比较。",
	} {
		if !strings.Contains(zhMD, want) {
			t.Fatalf("zh key-metric glossary missing C2 line %q:\n%s", want, zhMD)
		}
	}
	enMD := audit730Render(t, audit730Bus("en"), audit730ChainObs(), "en")
	for _, want := range []string{
		"- chain total = the projected duration this node plus its drill-down sub-chain accumulate toward the focused thread along the wakeup chain; a row WITHOUT a drill-down sub-chain carries no sub-chain share — its chain total is the row's own in-window account, and \"chain\" names the accumulation direction, not an extra direct-conduction claim toward the focused thread.",
		"when it equals the chain total the two are one measurement under two names (never two separate proofs), and when they differ the row's caliber word/annotation governs (e.g. discounted / on-chain-counted sit below, occurrence-segment account / inherited-from-wait-interval may sit above the chain total).",
		"- field mapping to trace_query rows: window projection ↔ impact= (JSON impact_ms); chain total ↔ cumulative_impact= (cumulative_impact_ms); attribution ↔ effective_impact= (effective_impact_ms); actual state ↔ actual_impact= (actual_impact_ms); several fields of one row sharing one value = one measurement under several names, never mutual corroboration.",
		"- confidence = the confidence tier (high/mid/low): each evidence source's numeric confidence folded through fixed thresholds; sources use different baselines, so the tier never compares evidence strength across rows.",
	} {
		if !strings.Contains(enMD, want) {
			t.Fatalf("en key-metric glossary missing C2 line %q:\n%s", want, enMD)
		}
	}
}

// TestValueWordIdentityEntryCarriesConfidenceRuling pins the 裁定③ clause on
// the 行2 identity legend entry in the catalog (zh + EN), and the lockstep
// probe: the taught tier words are exactly what the single tier implementation
// mints (runtimeTraceProjConfidenceTier — one implementation, three faces).
func TestValueWordIdentityEntryCarriesConfidenceRuling(t *testing.T) {
	var entry runtimeTraceProjLegendEntry
	found := false
	for _, e := range runtimeTraceProjLegendCatalog() {
		if e.Mark == runtimeTraceProjMarkCauseIdentityRow {
			entry, found = e, true
			break
		}
	}
	if !found {
		t.Fatal("identity-row legend entry missing from the catalog")
	}
	wantZH := "置信档(高/中/低)=各证据来源数值置信按固定阈值折词,不同来源基准不同,不作跨行证据强度比较(置信档差异不作为推翻榜位次序的依据)。"
	if !strings.Contains(entry.ZH, wantZH) {
		t.Fatalf("zh identity entry missing 裁定③ clause:\n%s", entry.ZH)
	}
	wantEN := "The confidence tier (high/mid/low) = each evidence source's numeric confidence folded through fixed thresholds; sources use different baselines, so the tier never compares evidence strength across rows (a tier difference is no basis for overturning seat order)."
	if !strings.Contains(entry.EN, wantEN) {
		t.Fatalf("en identity entry missing 裁定③ clause:\n%s", entry.EN)
	}
	// Lockstep probe: the taught 高/中/低 triple composes from the tier
	// single source — a threshold-word change breaks this pin before it can
	// silently orphan the legend sentence.
	zhTriple := "置信档(" + runtimeTraceProjConfidenceTier(0.9, true) + "/" +
		runtimeTraceProjConfidenceTier(0.7, true) + "/" + runtimeTraceProjConfidenceTier(0.3, true) + ")"
	if zhTriple != "置信档(高/中/低)" || !strings.Contains(entry.ZH, zhTriple) {
		t.Fatalf("zh tier triple drifted from the single source: %q vs entry %q", zhTriple, entry.ZH)
	}
	enTriple := "(" + runtimeTraceProjConfidenceTier(0.9, false) + "/" +
		runtimeTraceProjConfidenceTier(0.7, false) + "/" + runtimeTraceProjConfidenceTier(0.3, false) + ")"
	if enTriple != "(high/mid/low)" || !strings.Contains(entry.EN, enTriple) {
		t.Fatalf("en tier triple drifted from the single source: %q vs entry %q", enTriple, entry.EN)
	}
}

// TestValueWordWireMappingLockstep ties the glossary/skill-taught wire
// spellings to the actual wire single sources: the rank-row JSON tags and the
// display fold-caliber word mint. A wire key rename or a display word change
// must land here before the teaching faces can drift.
func TestValueWordWireMappingLockstep(t *testing.T) {
	// (a) The four duration JSON tags taught by the glossary mapping line
	// exist verbatim on the rank row struct.
	rankType := reflect.TypeOf(tracequery.RootCauseRankItem{})
	tags := map[string]bool{}
	for i := 0; i < rankType.NumField(); i++ {
		tag := rankType.Field(i).Tag.Get("json")
		if comma := strings.IndexByte(tag, ','); comma >= 0 {
			tag = tag[:comma]
		}
		if tag != "" && tag != "-" {
			tags[tag] = true
		}
	}
	for _, want := range []string{"impact_ms", "cumulative_impact_ms", "effective_impact_ms", "actual_impact_ms",
		"gated_runnable_ms", "gated_running_deficit_ms"} {
		if !tags[want] {
			t.Fatalf("taught wire key %q missing from the rank row JSON tags", want)
		}
	}
	if tags["projected_total_ms"] {
		t.Fatal("projected_total_ms appeared on the rank row JSON struct — the C3② Description teaching (wakeup_chain field; rank notes only echo cumulative) must be revisited")
	}
	// (b) The fold-caliber display words taught by the C1 skill directive are
	// exactly the single-source mints (digit → N template form).
	sk := registeredExploreSkillForValueWordTest(t)
	body := ""
	for _, item := range sk.WorkflowTierB {
		if strings.HasPrefix(item.Body, "TRACE VALUE WORDS:") {
			body = item.Body
			break
		}
	}
	if body == "" {
		t.Fatal("explore-skill missing the TRACE VALUE WORDS directive")
	}
	node := func(caliber string) (zh, en string) {
		// Member count 3 keeps the digit→N template replacement exact (no
		// second digit can appear in the minted word).
		n := types.TraceCausalProjectionNode{FamilyMemberCount: 3, FamilyFoldCaliber: caliber}
		zhWord, _, ok := runtimeTraceProjFamilyCaliberWord(n, true)
		if !ok {
			t.Fatalf("family caliber word missing for %s", caliber)
		}
		enWord, _, ok := runtimeTraceProjFamilyCaliberWord(n, false)
		if !ok {
			t.Fatalf("family caliber word missing for %s", caliber)
		}
		return strings.Replace(zhWord, "3", "N", 1), strings.Replace(enWord, "3", "N", 1)
	}
	for _, caliber := range []string{
		tracequery.RootCauseMemberFoldCaliberSumDisjoint,
		tracequery.RootCauseMemberFoldCaliberIntervalUnion,
		tracequery.RootCauseMemberFoldCaliberMaxOverlapFallback,
		tracequery.RootCauseMemberFoldCaliberCountSum,
	} {
		zhWord, enWord := node(caliber)
		if !strings.Contains(body, zhWord) {
			t.Fatalf("skill directive missing the %s zh display word %q:\n%s", caliber, zhWord, body)
		}
		if !strings.Contains(body, enWord) {
			t.Fatalf("skill directive missing the %s en display word %q:\n%s", caliber, enWord, body)
		}
	}
	// (b') 修补轮 件2: the CAPPED count_sum fork (published seat below the
	// member Σ — the engine's off-chain window cap, rcm.go F-7) mints the
	// honest capped word, and the directive teaches exactly that mint.
	clamped := types.TraceCausalProjectionNode{
		FamilyMemberCount: 3,
		FamilyFoldCaliber: tracequery.RootCauseMemberFoldCaliberCountSum,
		FamilyMemberSumMS: 5.0,
		EffectiveImpactMS: 4.0, // published 4.000 != Σ 5.000 → capped fork
	}
	clampedZH, _, okZH := runtimeTraceProjFamilyCaliberWord(clamped, true)
	clampedEN, _, okEN := runtimeTraceProjFamilyCaliberWord(clamped, false)
	if !okZH || !okEN {
		t.Fatal("capped count_sum caliber word missing from the single source")
	}
	if !strings.Contains(clampedZH, "超上限截断") || !strings.Contains(clampedEN, "capped") {
		t.Fatalf("capped fork fixture did not take the capped arm: %q / %q", clampedZH, clampedEN)
	}
	for _, want := range []string{
		strings.Replace(clampedZH, "3", "N", 1),
		strings.Replace(clampedEN, "3", "N", 1),
		"the Σ-identity claim must not be restated beside the capped value",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("skill directive missing the capped count_sum teaching %q:\n%s", want, body)
		}
	}
	// (c) 修补轮 件3: the gated component words compose from the 行3 component
	// single source (runtimeTraceProjInversionComponents Word+CaliberShort —
	// the exact "%s(%s)" 行3 form), and the zh canonical word 「有效归因」 pins
	// to the key-metric table column-name single source. Honest scope note:
	// the 「…」 quoting brackets are teaching-face decoration with no single
	// source; the WORDS inside are single-source-composed.
	gated := types.TraceCausalProjectionNode{
		PriorityInversionCandidate: true,
		GatedRunnableMS:            2.0,
		GatedRunningDeficitMS:      1.0,
		EffectiveImpactMS:          3.0, // identity: 2.000 + 1.000 == 3.000
	}
	components, _, okGated := runtimeTraceProjInversionComponents(gated, false, true)
	if !okGated || len(components) != 2 {
		t.Fatalf("gated composite fixture did not build both components: ok=%v n=%d", okGated, len(components))
	}
	runnableWord := components[0].Word + "(" + components[0].CaliberShort + ")"
	runningWord := components[1].Word + "(" + components[1].CaliberShort + ")"
	if runnableWord != "runnable(全额)" || runningWord != "running(折算)" {
		t.Fatalf("行3 component words drifted from the single source: %q / %q", runnableWord, runningWord)
	}
	columns, _ := runtimeTraceProjDetailTable(runtimeTraceProjTreeModel{}, true)
	if len(columns) < 4 || columns[3] != "有效归因" {
		t.Fatalf("key-metric attribution column drifted: %v", columns)
	}
	for _, want := range []string{runnableWord, runningWord, "「" + columns[3] + "」"} {
		if !strings.Contains(body, want) {
			t.Fatalf("skill directive missing the single-source display word %q:\n%s", want, body)
		}
	}
	// (d) The glossary wire-mapping line and the skill directive agree on the
	// text-face spellings.
	for _, want := range []string{"impact=", "cumulative_impact=", "effective_impact=", "gated_runnable_ms", "gated_running_deficit_ms", "member_fold_caliber"} {
		if !strings.Contains(body, want) {
			t.Fatalf("skill directive missing wire spelling %q:\n%s", want, body)
		}
	}
}

// TestValueWordDescriptionTeachingScoped — C3 pins (§29.104.16.1 M5③④+M21):
// the byte golden already freezes the full Description; these substring pins
// name the three deltas so a future regeneration cannot silently revert them,
// and cover the Parameters schema face the golden does not reach.
func TestValueWordDescriptionTeachingScoped(t *testing.T) {
	desc := (&TraceQuery{}).Description()
	for _, stale := range []string{"bounded ranking/hidden-cost signal", "the top-ranked state", "projected_impact_ms/projected_total_ms"} {
		if strings.Contains(desc, stale) {
			t.Fatalf("stale Description teaching resurfaced: %q", stale)
		}
	}
	for _, want := range []string{
		"effective_impact_ms as the row's ranking-attribution value taken under its stated caliber",
		"a priority-inversion row may publish a multi-component composite (runnable in full plus discounted running)",
		"projected_total_ms is a wakeup_chain causal_impact/aggregated_impact row field, not a rank-row JSON key",
		"only echoes cumulative_impact_ms — one value under two names, never a second measurement",
		"the drill_rank=1 state is always significant, and states further down the drill_rank ordering",
	} {
		if !strings.Contains(desc, want) {
			t.Fatalf("Description missing C3 teaching %q", want)
		}
	}
	schema := string((&TraceQuery{}).Parameters())
	if strings.Contains(schema, "projected_impact_ms/projected_total_ms") {
		t.Fatal("schema still attributes projected_total_ms to rank rows as a slash pair")
	}
	if !strings.Contains(schema, "cumulative_impact_ms (a rank note may echo it as projected_total_ms — one value, two spellings)") {
		t.Fatal("schema missing the projected_total_ms echo disclosure")
	}
	// 修补轮 件4 (对抗 P3-4 + K1, 2026-07-17): the two LLM faces of ONE tool
	// must speak ONE effective_impact_ms geometry, and that geometry must be
	// the engine's (rootCauseEffectiveImpactMsUncapped keeps a residual
	// terminal arm that falls back to cumulative for rows outside every
	// matrix arm — so neither the retired generic default NOR the former
	// blanket "never falls back generically" claim is honest).
	geometry := "effective_impact_ms is assigned by the closed typed matrix, never as a generic default; only a residual row outside every matrix arm and without a published effective falls back to cumulative_impact_ms"
	for face, text := range map[string]string{"Description": desc, "Parameters": schema} {
		if !strings.Contains(text, geometry) {
			t.Fatalf("%s missing the shared effective_impact_ms geometry sentence", face)
		}
		for _, stale := range []string{
			"never falls back generically",
			"defaults to cumulative_impact_ms",
			"default effective_impact_ms to cumulative_impact_ms",
		} {
			if strings.Contains(text, stale) {
				t.Fatalf("%s retained the stale effective_impact_ms claim %q", face, stale)
			}
		}
	}
}

func registeredExploreSkillForValueWordTest(t *testing.T) *skill.Config {
	t.Helper()
	r := skill.NewRegistry()
	skill.RegisterDefaults(r)
	sk, err := r.Get("explore-skill")
	if err != nil {
		t.Fatalf("Get(explore-skill): %v", err)
	}
	return sk
}
