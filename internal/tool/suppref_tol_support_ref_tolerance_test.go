package tool

// SUPPREF-TOL pins (§29.104.13, docs/design/real_trace_campaign_20260705.md).
//
// Witness: eval/results/real_trace_h9_conversion_single_basis-20260715-201207/
// run-1.logs.all.log — emit#2 (log line 1576, accepted) minted the member_set
// "链路节点" with the decorated support ref "attached_trace.txt: wakeup_chain
// path"; emit#4 (log line 2862, the emit carrying keva-1 effective=3.429ms and
// the correct supply-fold conversion basis) was wholesale DOWNGRADED at the
// member_set_support_refs gate (log line 2867) after verification-phase reads
// of the engine's own trace-query-result blob flipped the runtime bypass off.
// The emit payloads below are VERBATIM from that run's logs (engine-minted /
// production-emitted forms; never synthesize witness fixtures).
//
// CSP63-FIX (§29.104.20): the blob-read pollution was root-fixed at
// runtimeSourceAuthorityCurrentSourceRecord (engine blob-session paths no
// longer mint current-source authority, internal/types — IsCodraxBlobSession-
// Path), so the strangled faithful-state replay below flipped from DOWNGRADE
// to acceptance and was rewired to the accepted-exclude pin's assertions per
// its forward-assertion note.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// supprefTolWitnessEmit2Params is the accepted turn-2 emit (log line 1576)
// whose "链路节点" fact carries the decorated prose support ref.
const supprefTolWitnessEmit2Params = `{"reason": "基于 trace 数据分析完成。RenderThread-17597 在 13762.791708s 至 13763.024898s（233.19ms）窗口内卡顿根因如下：\n\n**链路结构**：RenderThread-17597（depth=0）← .ugc.aweme.lite-17267（depth=1）← keva-3-17439/keva-1-17437（depth=2，优先级20/ohos_cfs，低于目标的53/ohos_rt，构成优先级反转候选）\n\n**running 时间折算基准**：running 折算值 = 链路上该线程 measured running 时间（基于目标线程的 CPU 频率基准进行折算）；当 measured running 时间对应的 CPU 频率低于目标线程频率时，差值计入 supply_fold_deficit（供给缺口），形成 effective attribution。\n\n**各线程 effective attribution 汇总**：\n\n1. **.ugc.aweme.lite-17267**：running 折算 143.499ms（4次唤醒事件聚合，running=143.499ms，runnable=3.400ms，无供给缺口）；无优先级反转。\n2. **RenderThread-17597 自身**：running 折算 4.958ms。\n3. **keva-1-17437**（优先级反转候选）：runnable 2.181ms + running 1.419ms（供给缺口主导）= effective 3.437ms；另有 io_wait 0.922ms。\n4. **keva-3-17439**（优先级反转候选）：running 2.579ms（供给缺口 2.286ms 下界）+ io_wait 1.354ms + runnable 1.319ms = effective 3.309ms。\n5. **binder:496_9-10961**：running 1.409ms（supply_fold_deficit=0.933ms，ideal=0.476ms）。\n6. **.ugc.aweme.lite-17267 runnable**：3.437ms（聚合 runnable 贡献）。\n\n**同时有 runnable 和 running 贡献的席位构成**：\n- keva-3-17439 优先级反转候选·供给缺口主导：反转等待(全额) 1.023ms + running 折算 2.286ms（供给缺口 2.286ms 下界为主，热限压 1.53GHz）——两因并提。\n- keva-1-17437：runnable 2.181ms + running 1.419ms（供给缺口构成有效 attribution）。\n\n**根因排序**：\n1. .ugc.aweme.lite-17267 running 143.499ms（primary，自身 CPU 密集型任务占用链路上游）\n2. RenderThread-17597 running 4.958ms（secondary，自身执行）\n3. .ugc.aweme.lite-17267 runnable 3.437ms（同优先级竞争）\n4. keva-1-17437 优先级反转候选 3.437ms（反转等待+供给缺口）\n5. keva-3-17439 优先级反转候选 3.309ms（反转等待+io_wait+供给缺口）\n6. keva-3-17439 io_wait 1.354ms\n7. keva-3-17439 runnable_wait 1.319ms\n8. binder:496_9-10961 running 0.933ms（供给缺口折算）", "confidence": "high", "result_kind": "resolved", "aggregate_facts": [{"kind": "scalar_value", "label": "窗口时长", "value": "233.19ms", "role": "principal_answer", "provenance": "trace_query: time_end - time_start"}, {"kind": "member_set", "label": "链路节点", "value": "3", "members": ["RenderThread-17597 (depth=0)", ".ugc.aweme.lite-17267 (depth=1)", "keva-3-17439 (depth=2)", "keva-1-17437 (depth=2)", "binder:496_9-10961 (depth=2)"], "support_refs": ["attached_trace.txt: wakeup_chain path"]}, {"kind": "grouped_count", "label": "running 时间折算（effective attribution）", "value": "5", "dimensions": [{"name": "thread", "value": ".ugc.aweme.lite-17267"}, {"name": "thread", "value": "RenderThread-17597"}, {"name": "thread", "value": "keva-1-17437"}, {"name": "thread", "value": "keva-3-17439"}, {"name": "thread", "value": "binder:496_9-10961"}], "members": ["143.499ms (.ugc.aweme.lite-17267)", "4.958ms (RenderThread-17597)", "3.437ms (keva-1-17437, priority_inversion)", "3.309ms (keva-3-17439, priority_inversion)", "0.933ms (binder:496_9-10961, supply_deficit)"]}, {"kind": "member_set", "label": "同时有 runnable 和 running 贡献的席位", "value": "2", "members": ["keva-3-17439 (runnable 1.023ms + running supply折算 2.286ms)", "keva-1-17437 (runnable 2.181ms + running 1.419ms)"], "member_notes": ["keva-3-17439: 反转等待(全额) 1.023ms + running折算 2.286ms（供给缺口 2.286ms 下界为主，热限压 1.53GHz）", "keva-1-17437: runnable 2.181ms + running 1.419ms（供给缺口构成有效 attribution）"]}, {"kind": "scalar_value", "label": "折算基准", "value": "目标线程 CPU 频率基准（supply_fold_deficit = chain_running × (target_freq / chain_actual_freq - 1)）", "role": "principal_answer"}, {"kind": "member_set", "label": "根因排序", "value": "8", "members": ["#1: .ugc.aweme.lite-17267 running 143.499ms (primary)", "#2: RenderThread-17597 running 4.958ms (secondary)", "#3: .ugc.aweme.lite-17267 runnable 3.437ms", "#4: keva-1-17437 priority_inversion 3.437ms", "#5: keva-3-17439 priority_inversion 3.309ms", "#6: keva-3-17439 io_wait 1.354ms", "#7: keva-3-17439 runnable_wait 1.319ms", "#8: binder:496_9-10961 running supply_deficit 0.933ms"]}], "evidence_floor_waiver": {"reason": "external_only_trace", "rationale": "分析基于 attached_trace.txt（donghu.ftrace），trace_query 已完成解析，无 current_source 代码需要分析。"}}`

// supprefTolWitnessEmit4Params is the downgraded emit#4 (log line 2862): the
// engine-truth emit (keva-1 3.429ms + supply-fold basis) that never reached
// the handoff before SUPPREF-TOL.
const supprefTolWitnessEmit4Params = `{"reason": "RenderThread-17597 在 13762.791708s 至 13763.024898s（233.19ms）窗口内的卡顿根因分析完成。链路结构：RenderThread-17597（depth=0）← .ugc.aweme.lite-17267（depth=1）← keva-3-17439 / keva-1-17437（depth=2，优先级20/ohos_cfs 低于目标 53/ohos_rt，构成优先级反转候选）。\n\n**running 时间折算基准**：supply fold 折算。effective attribution = min(measured running, supply_fold_deficit)，其中 supply_fold_deficit = measured_running × (fmax_ideal / thermal_cap_freq - 1)，fmax 基准取 big class 全速（2.75GHz），thermal_cap 取被测核的实际热限频点。measured running 超出 supply_fold_deficit 的部分视为实际已达成，不计入 effective attribution。\n\n**各链路节点 effective attribution：**\n\n1. .ugc.aweme.lite-17267 running（primary, confidence=0.82）\n   - measured running：143.499ms；effective：51.735ms\n   - supply_fold_deficit：51.735ms；fold_basis：fmax=2.75GHz（big） / thermal_cap=1.88GHz（middle class），实际观测到 big 核在 2.075GHz 附近运行被压至 middle class 热限 1.88GHz；capability=default_table\n   - 4 次 wakeup 链路触发叠加，含 3.400ms runnable + 24.606ms sleep；优先级 53/ohos_rt（等于目标，等优先级依赖）\n\n2. RenderThread-17597 running（secondary, self_wall_clock, subject_is_analysis_target）\n   - measured running：7.305ms；effective：4.958ms\n   - supply_fold_deficit：4.958ms；fold_basis：fmax=2.75GHz（big） / thermal_cap=1.53GHz（small class）；实测该线程在 small 核热限 1.53GHz 附近执行，缺失 big 核全速算力\n\n3. .ugc.aweme.lite-17267 runnable_wait（tertiary, chain_anchored）\n   - effective：3.437ms；runnable 3.437ms 全部锚定在 wakeup 依赖窗口内（chain credential）\n\n4. keva-1-17437 priority_inversion_candidate（tertiary, confidence=0.91, chain_depth=2）\n   - measured：runnable 2.181ms + running 1.419ms + io_wait 0.922ms\n   - effective：3.429ms；构成 = gated_runnable 2.181ms（等优先级依赖竞争导致 ready 未上 CPU）+ gated_running_deficit 1.248ms（running 时受优先级反转压制）；supply_fold_deficit：无（running 1.419ms 未超 supply 基准）；优先级 20/ohos_cfs，目标 53/ohos_rt，低优先级依赖\n\n5. keva-3-17439 priority_inversion_candidate（tertiary, confidence=0.91, chain_depth=2）\n   - measured：runnable 1.023ms + running 2.579ms + io_wait 1.354ms\n   - effective：3.309ms；构成 = gated_runnable 1.023ms + min(running 2.579ms, supply_fold_deficit 2.286ms) = 1.023 + 2.286 = 3.309ms；supply_fold_deficit：2.286ms，fold_basis：fmax=2.75GHz（big） / thermal_cap=1.53GHz（small class）；优先级 20/ohos_cfs，目标 53/ohos_rt\n\n6. keva-3-17439 io_wait（tertiary, confidence=0.84）\n   - effective：1.354ms；blocked_reason：sync_buffer_read_wi（storage read wait）；2 段累加：cpu=4 合计 1.232ms（4段）+ cpu=1 0.122ms\n\n7. keva-3-17439 runnable_wait（tertiary, chain_anchored）\n   - effective：1.319ms；runnable 1.319ms 锚定在链内；相邻余量 0.017ms 计入 adjacent seat\n\n8. binder:496_9-10961 running（tertiary, confidence=0.75, chain_depth=2）\n   - measured running：1.409ms；effective：0.933ms；supply_fold_deficit：0.933ms；fold_basis：fmax=2.75GHz（big） / thermal_cap=1.88GHz（middle class）；priority_inversion_gated：0.933ms\n\n**同时含 runnable + running 的两席位数值构成：**\n- keva-1-17437：effective 3.429ms = gated_runnable 2.181ms（阻塞等调度）+ gated_running_deficit 1.248ms（running 受反转压制，无 supply fold）；不含 supply fold deficit（running 1.419ms 未触发频率折算）\n- keva-3-17439：effective 3.309ms = gated_runnable 1.023ms（ready 未上 CPU）+ supply_fold_deficit 2.286ms（running 在 small 核热限 1.53GHz 压制下折算，热限压为主因，gated_capability_source=default_table）；反转等待（全额）1.023ms + running 折算 2.286ms（供给缺口 2.286ms 下界为主，热限压 1.53GHz）\n\n**席位数值的口径构成（按行内发布值逐字引用）：**\n- #1 running折算 51.735ms = supply_fold_deficit；反转等待(全额) 0ms；下界已知 143.499ms\n- #2 running折算 4.958ms = supply_fold_deficit；下界已知 7.305ms\n- #4 反转等待(全额) 2.181ms + running折算 1.248ms；下界已知 running 1.419ms\n- #5 反转等待(全额) 1.023ms + running折算 2.286ms（供给缺口 2.286ms 下界为主，热限压 1.53GHz）；下界已知 running 2.579ms\n- #8 running折算 0.933ms = supply_fold_deficit + priority_inversion_gated\n\n**Adjacent seats（时间相邻，非链上）：**\n- .ugc.aweme.lite-17267 runnable_wait adjacent：2.167ms（runnable 全窗 5.604ms 中 3.437ms 已锚定在链内，余量 2.167ms 在依赖窗口外）\n- keva-3-17439 runnable_wait adjacent：0.017ms（runnable 全窗 1.336ms 中 1.319ms 已锚定，余量 0.017ms）\n\n**窗口全览**：RenderThread-17597 自身 running 仅 7.305ms（effective 4.958ms），但被 .ugc.aweme.lite-17267（effective 143.499ms，供给缺口 51.735ms 主因）通过低优先级依赖 keva-3/keva-1（priority_inversion_candidate，confidence=0.91）阻塞，导致累计 171.505ms 的 target_impact 未能及时 wakeup。", "confidence": "high", "result_kind": "resolved", "aggregate_facts": [{"kind": "scalar_value", "label": "窗口时长", "value": "233.19ms", "role": "principal_answer", "provenance": "trace_query: time_end - time_start", "evidence_origin": ["runtime_artifact"]}, {"kind": "member_set", "label": "链路节点", "value": "5", "role": "principal_answer", "evidence_origin": ["runtime_artifact"], "members": ["RenderThread-17597 (depth=0)", ".ugc.aweme.lite-17267 (depth=1, 4 occurrences)", "binder:496_9-10961 (depth=2)", "keva-3-17439 (depth=2)", "keva-1-17437 (depth=2)"]}, {"kind": "grouped_count", "label": "running时间折算 effective attribution（on-chain ranked seats）", "value": "8", "role": "principal_answer", "evidence_origin": ["runtime_artifact"], "dimensions": [{"name": "折算基准", "value": "supply_fold: effective = min(measured_running, supply_fold_deficit)，fmax=big-class 2.75GHz，thermal_cap取被测核实际热限频点"}, {"name": "tier", "value": "primary=1, secondary=1, tertiary=6"}], "members": ["#1 .ugc.aweme.lite-17267 running: effective=51.735ms (supply_fold_deficit, fold_basis: fmax=2.75GHz/thermal_cap=1.88GHz middle, measured 143.499ms)", "#2 RenderThread-17597 running: effective=4.958ms (supply_fold_deficit, fold_basis: fmax=2.75GHz/thermal_cap=1.53GHz small, measured 7.305ms)", "#3 .ugc.aweme.lite-17267 runnable_wait: effective=3.437ms (chain_anchored, no supply fold)", "#4 keva-1-17437 priority_inversion: effective=3.429ms (gated_runnable=2.181ms + gated_running_deficit=1.248ms, supply_fold=N/A)", "#5 keva-3-17439 priority_inversion: effective=3.309ms (gated_runnable=1.023ms + supply_fold_deficit=2.286ms, fold_basis: fmax=2.75GHz/thermal_cap=1.53GHz small)", "#6 keva-3-17439 io_wait: effective=1.354ms (sync_buffer_read_wi)", "#7 keva-3-17439 runnable_wait: effective=1.319ms (chain_anchored)", "#8 binder:496_9-10961 running: effective=0.933ms (supply_fold_deficit, fold_basis: fmax=2.75GHz/thermal_cap=1.88GHz middle, measured 1.409ms)"]}, {"kind": "member_set", "label": "同时含 runnable+running 贡献的席位（#4 和 #5）", "value": "2", "role": "principal_answer", "evidence_origin": ["runtime_artifact"], "members": ["keva-1-17437: effective=3.429ms = gated_runnable 2.181ms + gated_running_deficit 1.248ms；反转等待(全额) 2.181ms + running折算 1.248ms；supply_fold无（running 1.419ms未触发频率折算）；下界已知 running 1.419ms", "keva-3-17439: effective=3.309ms = gated_runnable 1.023ms + supply_fold_deficit 2.286ms；反转等待(全额) 1.023ms + running折算 2.286ms（供给缺口 2.286ms 下界为主，热限压 1.53GHz）；下界已知 running 2.579ms"], "member_notes": ["keva-1-17437 running折算用 gated_running_deficit（非supply_fold），因 measured running 1.419ms 未超 supply 基准", "keva-3-17439 running折算用 supply_fold_deficit（=gated_running_deficit），因 measured running 2.579ms > supply_fold_deficit 2.286ms，折算至 2.286ms，热限压为主因"]}, {"kind": "member_set", "label": "根因排序（on-chain）", "value": "8", "role": "principal_answer", "evidence_origin": ["runtime_artifact"], "members": ["#1: .ugc.aweme.lite-17267 running, effective=51.735ms, tier=primary, confidence=0.82, supply_fold_deficit=51.735ms（big核热限压至middle 1.88GHz）", "#2: RenderThread-17597 running, effective=4.958ms, tier=secondary, confidence=0.86, supply_fold_deficit=4.958ms（small核热限压至1.53GHz）", "#3: .ugc.aweme.lite-17267 runnable_wait, effective=3.437ms, tier=tertiary, chain_anchored", "#4: keva-1-17437 priority_inversion_candidate, effective=3.429ms, confidence=0.91, depth=2, priority=20/ohos_cfs vs target=53/ohos_rt", "#5: keva-3-17439 priority_inversion_candidate, effective=3.309ms, confidence=0.91, depth=2, priority=20/ohos_cfs vs target=53/ohos_rt", "#6: keva-3-17439 io_wait, effective=1.354ms, blocked_reason=sync_buffer_read_wi", "#7: keva-3-17439 runnable_wait, effective=1.319ms, chain_anchored", "#8: binder:496_9-10961 running, effective=0.933ms, confidence=0.75, depth=2, supply_fold_deficit=0.933ms"]}]}`

// —— Pin ④: the strip closed set, one unit case per decoration family, plus
// the negative (never-strip) families. 宁窄勿宽: only census-observed
// decorations strip; everything else must come back unchanged.
func TestSupprefTol_StripDecorations_ClosedSet(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		changed bool
	}{
		// census A (h9 witness, verbatim): artifact head + colon-prose tail.
		{"witness_artifact_colon_prose", "attached_trace.txt: wakeup_chain path", "attached_trace.txt", true},
		// census B (h6 run, verbatim): file:line + trailing paren annotation.
		{"trailing_paren_after_line", "attached_trace.txt:603 (caller=fscache_page_get_an+0xd0c/0x1b44)", "attached_trace.txt:603", true},
		// census C (h2 run, verbatim): artifact id + trailing paren annotation.
		{"trailing_paren_after_id", "runtime_artifact:ae9bd2562e129fa7 (wakeup counts census)", "runtime_artifact:ae9bd2562e129fa7", true},
		// Wrapped LOCATION cores report changed=false: the unchanged exact
		// parser already absorbs a symmetric wrap for location surfaces
		// (ParseAnswerSourceLocationSurface trims `'" itself), so the K2
		// parse-stop halts before peeling — those refs stay byte-identical.
		{"backtick_wrap_location_core_absorbed_by_parser", "`internal/agent/analyzer.go:1903`", "`internal/agent/analyzer.go:1903`", false},
		{"double_quote_wrap_location_core_absorbed_by_parser", `"internal/agent/analyzer.go:1903"`, `"internal/agent/analyzer.go:1903"`, false},
		{"single_quote_wrap_location_core_absorbed_by_parser", "'internal/agent/analyzer.go:1903'", "'internal/agent/analyzer.go:1903'", false},
		// Wrap peeling still functions where the core is NOT
		// location-parseable — evidence-id cores need it for the exact-match
		// byID lane.
		{"backtick_wrap_id_core", "`runtime_artifact:ae9bd2562e129fa7`", "runtime_artifact:ae9bd2562e129fa7", true},
		{"double_quote_wrap_id_core", `"runtime_artifact:ae9bd2562e129fa7"`, "runtime_artifact:ae9bd2562e129fa7", true},
		// Whitespace-only decoration reports changed=false: every caller
		// already TrimSpaces the ref, so a whitespace-only delta would only
		// mint a duplicate lookup candidate.
		{"outer_whitespace", "  internal/agent/analyzer.go:1903  ", "internal/agent/analyzer.go:1903", false},
		{"cjk_fullwidth_paren", "internal/agent/analyzer.go:1903（初始化）", "internal/agent/analyzer.go:1903", true},
		{"layered_backtick_then_paren", "`internal/agent/analyzer.go:1903 (init)`", "internal/agent/analyzer.go:1903", true},
		// Never-strip families (pin ⑤ ambiguity + census-absent decorations).
		{"legal_labeled_colon_form", "Foo: internal/agent/analyzer.go:1903", "Foo: internal/agent/analyzer.go:1903", false},
		{"artifact_colon_with_parseable_tail_is_ambiguous", "attached_trace.txt: other.go:10", "attached_trace.txt: other.go:10", false},
		{"ordinal_prefix_not_in_census", "1. internal/agent/analyzer.go:1903", "1. internal/agent/analyzer.go:1903", false},
		{"cjk_line_form_has_no_legal_core", "日志第2行", "日志第2行", false},
		{"empty_paren_group_kept", "Gate.Run()", "Gate.Run()", false},
		{"asymmetric_backtick_kept", "`internal/agent/analyzer.go:1903", "`internal/agent/analyzer.go:1903", false},
		{"paren_without_space_kept", "analyzer.go:1903(init)", "analyzer.go:1903(init)", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, changed := stripAggregateSupportRefDecorations(tc.in)
			if changed != tc.changed || got != tc.want {
				t.Fatalf("strip(%q) = %q changed=%v, want %q changed=%v", tc.in, got, changed, tc.want, tc.changed)
			}
		})
	}
}

// —— Pin ② + semantic equivalence (同 ref 集): a stripped decorated ref must
// parse EXACTLY like its directly-emitted bare form — never better, never
// worse — and legal bare/labeled forms must parse byte-identically through
// the tolerant entry point.
func TestSupprefTol_TolerantParts_EquivalenceAndZeroChange(t *testing.T) {
	equivalents := []struct {
		decorated string
		bare      string
	}{
		{"`internal/agent/analyzer.go:1903`", "internal/agent/analyzer.go:1903"},
		{"internal/agent/analyzer.go:1903 (init path)", "internal/agent/analyzer.go:1903"},
		{"internal/agent/analyzer.go:1903（初始化）", "internal/agent/analyzer.go:1903"},
		// The witness ref strips to a line-less artifact file: its bare form
		// does not parse as a location either — equivalence includes shared
		// failure (fail-open, not decorated-beats-bare).
		{"attached_trace.txt: wakeup_chain path", "attached_trace.txt"},
		{"attached_trace.txt:603 (caller=fscache_page_get_an+0xd0c/0x1b44)", "attached_trace.txt:603"},
	}
	for _, pair := range equivalents {
		dl, dloc, dok := aggregateMemberSupportRefPartsTolerant(pair.decorated)
		bl, bloc, bok := aggregateMemberSupportRefParts(pair.bare)
		if dl != bl || dloc != bloc || dok != bok {
			t.Fatalf("tolerant(%q)=(%q,%q,%v) != exact(bare %q)=(%q,%q,%v)",
				pair.decorated, dl, dloc, dok, pair.bare, bl, bloc, bok)
		}
	}
	legal := []string{
		"internal/agent/analyzer.go:1903",
		"Foo: internal/agent/analyzer.go:1903",
		"Foo @ internal/agent/analyzer.go:1903",
		"Foo (internal/agent/analyzer.go:1903)",
	}
	for _, ref := range legal {
		el, eloc, eok := aggregateMemberSupportRefParts(ref)
		tl, tloc, tok := aggregateMemberSupportRefPartsTolerant(ref)
		if !eok || el != tl || eloc != tloc || eok != tok {
			t.Fatalf("legal form %q must parse unchanged: exact=(%q,%q,%v) tolerant=(%q,%q,%v)",
				ref, el, eloc, eok, tl, tloc, tok)
		}
	}
}

// —— Witness-ref explicit runtime-artifact origin: the decorated prose ref
// and its bare form must carry the SAME explicit runtime-artifact origin
// (equivalence), while plain source refs stay non-runtime (zero change).
func TestSupprefTol_SupportRefExplicitRuntimeArtifactOrigin(t *testing.T) {
	positives := []string{
		"attached_trace.txt: wakeup_chain path",                    // witness, verbatim
		"attached_trace.txt",                                       // its bare form
		"runtime_artifact:ae9bd2562e129fa7 (wakeup counts census)", // census C, verbatim
		"runtime_artifact:ae9bd2562e129fa7",
		"trace_query: window_stats",
	}
	for _, ref := range positives {
		if !aggregateSupportRefCarriesExplicitRuntimeArtifactOrigin(ref) {
			t.Fatalf("ref %q must carry explicit runtime-artifact origin", ref)
		}
	}
	negatives := []string{
		"internal/agent/analyzer.go:1903",
		"Foo: internal/agent/analyzer.go:1903",
		"https://example.com/doc",
		"",
	}
	for _, ref := range negatives {
		if aggregateSupportRefCarriesExplicitRuntimeArtifactOrigin(ref) {
			t.Fatalf("ref %q must NOT carry explicit runtime-artifact origin", ref)
		}
	}
	fact := types.AnswerAggregateFact{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "链路节点",
		Value:       "5",
		Members:     []string{"RenderThread-17597 (depth=0)"},
		SupportRefs: []string{"attached_trace.txt: wakeup_chain path"},
	}
	if !aggregateFactHasExplicitRuntimeArtifactOrigin(fact) {
		t.Fatalf("witness fact with artifact-naming support ref must count as explicit runtime origin")
	}
}

// supprefTolFaithfulH9Policy is the h9 run's FAITHFUL analyzer terminal state
// (log lines 496-499): the model emitted current_source_mode=exclude WITHOUT
// exclusion_kind=explicit_user_exclusion, so emit_analysis IGNORED the
// exclusion and auto-softened the lane to allow/external_only ("external
// _observation_policy=allow/artifact_citation=external_only"). Anchored
// source quotes survive that softening. Never mint the refused exclude state
// for this run (P0-B: fixtures carry the engine's actual terminal state).
func supprefTolFaithfulH9Policy() *types.ExternalObservationPolicy {
	return &types.ExternalObservationPolicy{
		CurrentSourceMode:    types.ExternalObservationCurrentSourceAllow,
		ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
		SourceQuotes:         []string{"只分析这份 trace，不分析代码"},
		Confidence:           1.0,
	}
}

// supprefTolAcceptedExcludePolicy is the ACCEPTED explicit-user-exclusion
// terminal state, verbatim from eval run
// real_trace_c4_freq_supply_evidence-20260706-012258 (same forbidding quote
// as h9; that run's analyzer emitted exclusion_kind=explicit_user_exclusion
// and emit_analysis accepted it: "external_observation_policy=exclude/
// artifact_citation=external_only", no exclude-ignored WARN). 309 eval runs
// carry this accepted-exclude terminal state, so the lane is a real
// production state, not a synthetic construction.
func supprefTolAcceptedExcludePolicy() *types.ExternalObservationPolicy {
	return &types.ExternalObservationPolicy{
		CurrentSourceMode:    types.ExternalObservationCurrentSourceExclude,
		ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
		ExclusionKind:        types.ExternalObservationSourceExclusionExplicitUserBoundary,
		SourceQuotes:         []string{"只分析这份 trace，不分析代码"},
		Confidence:           1.0,
	}
}

// supprefTolWitnessBus reconstructs the h9 witness run context from its
// logged emit_analysis (log line 495) and route classification: route hint
// route=repo/source=artifact plus one deterministic trace_query runtime
// observation. The external-observation policy is injected so the faithful
// h9 terminal state and the c4-attested accepted-exclude state stay clearly
// separated.
func supprefTolWitnessBus(policy *types.ExternalObservationPolicy) (*types.BusContext, *types.MutableState) {
	mut := types.NewMutableState("q")
	const rawRequest = "只分析这份 trace，不分析代码。请分析 RenderThread-17597 在 13762.791708s 到 13763.024898s 窗口内的卡顿根因：链路上各线程的 running 时间折算后各计入多少、折算的基准是什么？对同时有 runnable 和 running 贡献的线程，请说明席位数值的构成。请给出根因排序。"
	bus := &types.BusContext{
		Mutable: mut,
		TurnRouteHint: types.TurnRouteHint{
			Route:           "repo",
			Source:          "artifact",
			Operation:       "investigate",
			NeedsRepoAccess: true,
			Confidence:      0.9,
		},
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: rawRequest,
			Intent:     "root_cause",
			DiagnosticProfile: types.DiagnosticIntentProfile{
				IsDiagnostic: true,
				Confidence:   0.95,
			},
			ExternalObservationPolicy: policy,
		}},
	}
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "trace_query",
		Success:  true,
		Summary:  "root_cause_rank rows",
		Observations: []types.ObservationRecord{{
			ID:              "trace_query:trace-query-result-25d95bf1.json#root_cause_rank:1",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Subject:         ".ugc.aweme.lite-17267",
			Object:          "running",
			Value:           "143.499ms",
			Summary:         ".ugc.aweme.lite-17267 aggregated on wakeup chain impact=143.499ms",
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef: types.ObservationSourceRef{
				Kind: types.ObservationSourceRuntimeArtifact,
			},
			Span: types.ObservationSpan{LineStart: 1043, LineEnd: 20773},
		}},
	})
	return bus, mut
}

// supprefTolAppendVerificationPhaseReads mirrors the turn-3 verification
// phase of the witness run (log lines 2111..2729): read_file pagination over
// the engine's own trace-query-result blob plus one read of the attached
// trace. Before CSP63-FIX these reads flipped CurrentSourceSatisfied on and
// strangled the runtime bypass at emit#4 time; the root fix made engine
// blob-session reads authority-inert, and the replays below pin exactly that.
func supprefTolAppendVerificationPhaseReads(mut *types.MutableState) {
	const blobTrace = "/Users/han/opt/claude/codrax/.codrax/blob/20260715-201207-000-89609/attached_trace.txt"
	const blobJSON = "/Users/han/opt/claude/codrax/.codrax/blob/20260715-201207-000-89609/trace-query-result-25d95bf1.json"
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName:     "read_file",
		Success:      true,
		Summary:      "read trace-query result",
		ReadCoverage: &types.ToolReadCoverage{Path: blobJSON, LineStart: 1, LineEnd: 200},
	})
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName:     "read_file",
		Success:      true,
		Summary:      "read attached trace",
		ReadCoverage: &types.ToolReadCoverage{Path: blobTrace, LineStart: 1, LineEnd: 120},
	})
}

// —— Faithful h9 witness pin, rewired per its forward assertion (CSP63-FIX):
// under the run's REAL analyzer terminal state (allow/external_only — the
// exclude was refused, log line 496) plus the verification-phase reads of the
// engine's own trace-query-result blob, emit#4 now LANDS. The blob reads no
// longer mint CurrentSourceSatisfied (root fix at runtimeSourceAuthority-
// CurrentSourceRecord), so the runtime origin bypass protects the decorated
// members at the support_refs gate and the engine truth (keva-1
// effective=3.429ms plus the supply-fold conversion basis) reaches the stable
// handoff. Mid-flight assertions pin the two mechanism flips (origin lanes
// protect / no polluted current-source requirement) plus the unchanged
// SUPPREF-TOL invariant (the junk prose ref stays non-consumable).
func TestSupprefTol_WitnessReplay_FaithfulStateEmitFourLandsAfterCSP63(t *testing.T) {
	bus, mut := supprefTolWitnessBus(supprefTolFaithfulH9Policy())
	tool := &EmitInvestigationComplete{}

	res2, err := tool.Execute(bus, json.RawMessage(supprefTolWitnessEmit2Params))
	if err != nil {
		t.Fatalf("emit#2 replay error: %v", err)
	}
	if !res2.Success || !strings.HasPrefix(res2.Summary, "Investigation marked complete") {
		t.Fatalf("emit#2 replay must stay accepted as in the real run: %s", res2.Summary)
	}

	supprefTolAppendVerificationPhaseReads(mut)

	var witnessFact types.AnswerAggregateFact
	for _, fact := range mut.StableInvestigationAggregateFacts() {
		if fact.Kind == types.AnswerAggregateMemberSet && strings.TrimSpace(fact.Label) == "链路节点" {
			witnessFact = fact
			break
		}
	}
	if len(witnessFact.Members) != 5 || len(witnessFact.SupportRefs) != 1 {
		t.Fatalf("witness fact not staged as in the run: %+v", witnessFact)
	}
	support := buildAggregateMemberSupportIndexWithEvidence(bus, mut.EmittedEvidence())
	if aggregateFactAnySupportRefConsumable(witnessFact.SupportRefs, support) {
		t.Fatalf("SUPPREF-TOL invariant: the decorated prose ref must stay non-consumable (no repair veto)")
	}
	// CSP63-FIX mechanism flip 1: with the blob reads authority-inert, the
	// runtime landing snapshot is allowed again and the origin bypass
	// protects every decorated member of the witness fact.
	if !aggregateFactOriginLanesProtectAllDecoratedMembers(bus, witnessFact) {
		t.Fatalf("post-CSP63 the origin bypass must protect the decorated members")
	}
	// CSP63-FIX mechanism flip 2: the polluted current-source requirement bit
	// is gone — blob reads no longer mint CurrentSourceSatisfied.
	rm := requestModelForAggregateSupport(bus)
	if aggregateMemberSetOriginRequiresCurrentSource(bus, rm, []types.AnswerAggregateFact{witnessFact}) {
		t.Fatalf("post-CSP63 blob reads must not mint a current-source requirement")
	}
	// With the bypass alive the bare-surface form repair correctly steps
	// aside (the per-member loop would no-op on protected members); the
	// repair lane itself stays pinned by the synthetic no-runtime-context
	// pins below.
	if decoratedMemberSetFormRepairEligible(bus, "resolved", witnessFact) {
		t.Fatalf("live origin bypass must supersede the bare-surface form repair")
	}

	res4, err := tool.Execute(bus, json.RawMessage(supprefTolWitnessEmit4Params))
	if err != nil {
		t.Fatalf("emit#4 replay error: %v", err)
	}
	if !res4.Success {
		t.Fatalf("emit#4 replay failed outright: %s", res4.Summary)
	}
	if strings.Contains(res4.Summary, EmitInvestigationCompleteDowngradePrefix) ||
		strings.Contains(res4.Summary, "support_refs do not resolve to the cited member") {
		t.Fatalf("emit#4 must land instead of the DOWNGRADE, got: %s", res4.Summary)
	}
	if !strings.HasPrefix(res4.Summary, "Investigation marked complete") {
		t.Fatalf("emit#4 must be accepted, got: %s", res4.Summary)
	}
	assertSupprefTolEmitFourHandoffLanded(t, mut)
}

// assertSupprefTolEmitFourHandoffLanded pins the emit#4 handoff surface both
// terminal-state lanes share after CSP63-FIX: the keva-1 effective=3.429ms
// seat fact, the supply-fold conversion-basis fact, and the principal
// 链路节点 member_set landing LOSSLESSLY through the runtime origin bypass —
// decorated member surfaces intact, no form_repair rewrite (the repair was
// the polluted-state fallback landing; the bypass is the designed primary
// lane, emit_investigation_complete.go normalizeDecoratedMemberSetFormDebt).
func assertSupprefTolEmitFourHandoffLanded(t *testing.T, mut *types.MutableState) {
	t.Helper()
	facts := mut.StableInvestigationAggregateFacts()
	var sawSeatFact, sawBasisFact, sawIntactChainFact bool
	for _, fact := range facts {
		joinedMembers := strings.Join(fact.Members, "\n")
		if fact.Kind == types.AnswerAggregateMemberSet &&
			strings.Contains(fact.Label, "同时含 runnable+running") &&
			strings.Contains(joinedMembers, "keva-1-17437: effective=3.429ms") {
			sawSeatFact = true
		}
		if fact.Kind == types.AnswerAggregateGroupedCount &&
			strings.Contains(fact.Label, "running时间折算 effective attribution") &&
			strings.Contains(joinedMembers, "supply_fold_deficit, fold_basis: fmax=2.75GHz/thermal_cap=1.88GHz middle, measured 143.499ms") {
			sawBasisFact = true
		}
		if fact.Kind == types.AnswerAggregateMemberSet && strings.TrimSpace(fact.Label) == "链路节点" &&
			fact.Role == types.AnswerAggregateRolePrincipalAnswer {
			if len(fact.Members) != 5 || !strings.Contains(joinedMembers, "(depth=") {
				t.Fatalf("principal 链路节点 fact must keep its five decorated member surfaces intact, got %+v", fact.Members)
			}
			if strings.Contains(fact.Provenance, "form_repair:decorated_member_base") {
				t.Fatalf("origin-bypass landing must not rewrite members via form repair, got provenance %q", fact.Provenance)
			}
			sawIntactChainFact = true
		}
	}
	if !sawSeatFact {
		t.Fatalf("keva-1 effective=3.429ms seat fact missing from handoff: %+v", facts)
	}
	if !sawBasisFact {
		t.Fatalf("supply-fold conversion-basis fact missing from handoff: %+v", facts)
	}
	if !sawIntactChainFact {
		t.Fatalf("principal 链路节点 fact missing from handoff: %+v", facts)
	}
}

// —— Accepted-exclude lane pin (真形): the same emit payloads land end-to-end
// when the run carries the ACCEPTED explicit-user-exclusion terminal state
// (policy shape verbatim from the c4 run, same forbidding quote; fact
// payloads verbatim from h9 — every ingredient engine-attested, and the
// conjunction is reachable in any accepted-exclude trace run whose explorer
// pages the trace-query result blob). Post-CSP63 the blob reads are
// authority-inert in this lane too, so the runtime origin bypass protects
// the decorated members and the emit lands LOSSLESSLY (the pre-fix pin
// asserted the bare-surface form-repair landing — the fallback the polluted
// authority forced; that fallback lane stays pinned by the synthetic
// no-runtime-context pins below). The engine truth (keva-1 effective=3.429ms
// plus the supply-fold conversion basis) reaches the stable handoff.
func TestSupprefTol_AcceptedExcludeLane_EmitFourLandsInHandoff(t *testing.T) {
	bus, mut := supprefTolWitnessBus(supprefTolAcceptedExcludePolicy())
	tool := &EmitInvestigationComplete{}

	res2, err := tool.Execute(bus, json.RawMessage(supprefTolWitnessEmit2Params))
	if err != nil {
		t.Fatalf("emit#2 replay error: %v", err)
	}
	if !res2.Success || !strings.HasPrefix(res2.Summary, "Investigation marked complete") {
		t.Fatalf("emit#2 replay must stay accepted (healthy-path zero change): %s", res2.Summary)
	}

	supprefTolAppendVerificationPhaseReads(mut)

	res4, err := tool.Execute(bus, json.RawMessage(supprefTolWitnessEmit4Params))
	if err != nil {
		t.Fatalf("emit#4 replay error: %v", err)
	}
	if !res4.Success {
		t.Fatalf("emit#4 replay failed outright: %s", res4.Summary)
	}
	if strings.Contains(res4.Summary, EmitInvestigationCompleteDowngradePrefix) ||
		strings.Contains(res4.Summary, "support_refs do not resolve to the cited member") {
		t.Fatalf("emit#4 must land instead of the DOWNGRADE, got: %s", res4.Summary)
	}
	if !strings.HasPrefix(res4.Summary, "Investigation marked complete") {
		t.Fatalf("emit#4 must be accepted, got: %s", res4.Summary)
	}
	assertSupprefTolEmitFourHandoffLanded(t, mut)
}

// —— Pin ③ (fail-open): a decorated member_set whose support ref is
// CONSUMABLE (parses to a real location) but does not resolve to the member
// keeps today's downgrade byte-identically — consumable refs still veto the
// bare-surface repair and the gate message format is pinned verbatim.
func TestSupprefTol_ConsumableButUnresolvingRefKeepsDowngradeByteIdentical(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"code members collected",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[
			{
				"kind":"member_set",
				"label":"code members",
				"value":"1",
				"members":["Gate.Run (8个独立检查)"],
				"support_refs":["internal/tool/zz_other.go:12"]
			}
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("downgrade must stay a successful tool result: %s", res.Summary)
	}
	want := `aggregate_facts: member_set "code members" has 1 member(s) shaped as "<code identifier> (<qualifier>)" ("Gate.Run (8个独立检查)") but support_refs do not resolve to the cited member. Decorated code-shape members never auto-resolve against evidence anchors, so downstream answer rendering cannot align row item citation_ref values without explicit per-member grounding. Re-emit with support_refs in either of these forms: labeled ["<member-leading-symbol>: <file>:<line>", …] where the label is the member's bare leading identifier (no decorator), or positional ["<file>:<line>", …] with one entry per members[] in the same order. If you cannot ground a member, drop the decorator so the bare symbol can auto-resolve, or remove the member entirely.`
	if !strings.Contains(res.Summary, EmitInvestigationCompleteDowngradePrefix) || !strings.Contains(res.Summary, want) {
		t.Fatalf("consumable-but-unresolving ref must keep the byte-identical downgrade, got: %s", res.Summary)
	}
}

// —— Pin ⑤ (ambiguity negative arm): the artifact-colon strip refuses shapes
// whose tail itself parses as a location (legal labeled grammar), so a
// decorated-looking-but-legal ref keeps today's labeled-form parse and the
// witness-junk shape never resolves a member it does not name.
func TestSupprefTol_AmbiguousArtifactColonShapeStaysLabeledForm(t *testing.T) {
	label, loc, ok := aggregateMemberSupportRefPartsTolerant("attached_trace.txt: other.go:10")
	if !ok || label != "attached_trace.txt" || loc != "other.go:10" {
		t.Fatalf("ambiguous artifact-colon shape must keep the legal labeled parse, got (%q,%q,%v)", label, loc, ok)
	}
	if _, _, ok := aggregateMemberSupportRefPartsTolerant("attached_trace.txt: wakeup_chain path"); ok {
		t.Fatalf("witness prose ref must still fail location parsing after stripping (fail-open)")
	}
}

// —— K2 stop-rule pin: one peeled layer must never strip THROUGH a legal
// core. A backtick-wrapped legal labeled-paren ref stops at the parseable
// form (payload-bearing paren group intact) and parses exactly like its
// directly-emitted form.
func TestSupprefTol_StripStopsAtParseableLabeledParenCore(t *testing.T) {
	got, changed := stripAggregateSupportRefDecorations("`Foo (src/foo.go:10)`")
	if !changed || got != "Foo (src/foo.go:10)" {
		t.Fatalf("strip must stop at the parseable labeled-paren core, got %q changed=%v", got, changed)
	}
	dl, dloc, dok := aggregateMemberSupportRefPartsTolerant("`Foo (src/foo.go:10)`")
	bl, bloc, bok := aggregateMemberSupportRefParts("Foo (src/foo.go:10)")
	if !dok || dl != bl || dloc != bloc || dok != bok {
		t.Fatalf("wrapped labeled-paren ref must parse like its bare form: (%q,%q,%v) vs (%q,%q,%v)", dl, dloc, dok, bl, bloc, bok)
	}
}

// —— P0-A A/B probe promoted to a pin: a decorated location-only positional
// ref that only parses via strip retry must NOT cut off the ref-loop
// fallback. In the baseline tree the paren-annotated positional ref fails to
// parse (the exact parser absorbs symmetric wraps but not trailing paren
// annotations), so the member fell through and resolved via its labeled
// second ref — a baseline-ACCEPTED form. Tolerant positional parsing must
// not convert that mismatching location into a rejection: the positional
// branch is success-only, every failure leg falls through to the ref loop
// (same philosophy as the aggregateMemberSetMemberUsableAt positional arm).
func TestSupprefTol_DecoratedPositionalRefDoesNotCutRefLoopFallback(t *testing.T) {
	fact := types.AnswerAggregateFact{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "code members",
		Value:   "2",
		Members: []string{"Gate.Run (8 checks)", "helper"},
		SupportRefs: []string{
			"internal/agent/other.go:99 (init)",   // decorated location-only positional ref; strip-parses to a location that does NOT name the member
			"Gate.Run: internal/agent/gate.go:10", // labeled ref that resolves the member via the ref loop
		},
	}
	if _, _, ok := aggregateMemberSupportRefParts(fact.SupportRefs[0]); ok {
		t.Fatalf("probe invariant broken: the decorated positional ref must NOT exact-parse (baseline fell through here)")
	}
	if _, loc, ok := aggregateMemberSupportRefPartsTolerant(fact.SupportRefs[0]); !ok || loc != "internal/agent/other.go:99" {
		t.Fatalf("probe invariant broken: the decorated positional ref must strip-parse, got (%q,%v)", loc, ok)
	}
	evidence := []types.EvidenceItem{{
		ID:           "ev-gate-run",
		Source:       "internal/agent/gate.go",
		LineStart:    10,
		AnchorSymbol: "Gate.Run",
		Snippet:      "func (g Gate) Run() error {",
	}}
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	support := buildAggregateMemberSupportIndexWithEvidence(bus, evidence)
	if !aggregateMemberSetSupportRefsResolveMember(fact, fact.Members[0], support) {
		t.Fatalf("ref-loop fallback must still resolve the member when the decorated positional ref mismatches")
	}
}

// —— Census family 3/4 end-to-end positive arms: engine-actually-emitted
// decorated support_refs forms strip and TRULY resolve, landing the emit
// with the decorated member surfaces INTACT (resolution lane, not the
// repair lane).
func TestSupprefTol_DecoratedRefsResolveEndToEnd(t *testing.T) {
	t.Run("family4_h2_verbatim_artifact_id_paren", func(t *testing.T) {
		// Ref verbatim from eval run real_trace_h2_dstate_dma_fence_triform-
		// 20260716-181114: the artifact registry id (prompt face
		// "id=runtime_artifact:ae9bd2562e129fa7") plus a paren annotation.
		// Stripping exposes the exact evidence id for the byID lane.
		mut := types.NewMutableState("q")
		bus := &types.BusContext{Mutable: mut}
		mut.AppendEvidence([]types.EvidenceItem{{
			ID:           "runtime_artifact:ae9bd2562e129fa7",
			Source:       "attached_trace.txt",
			LineStart:    12,
			AnchorSymbol: "CompThread_0-2955",
			Snippet:      "wakeup counts census row CompThread_0-2955",
		}})
		tool := &EmitInvestigationComplete{}
		res, err := tool.Execute(bus, json.RawMessage(`{
			"reason":"census members collected",
			"confidence":"high",
			"result_kind":"resolved",
			"aggregate_facts":[{
				"kind":"member_set",
				"label":"wakeup census members",
				"value":"1",
				"members":["CompThread_0-2955 (合成线程)"],
				"support_refs":["runtime_artifact:ae9bd2562e129fa7 (wakeup counts census)"]
			}]
		}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.Success || !strings.HasPrefix(res.Summary, "Investigation marked complete") {
			t.Fatalf("decorated artifact-id ref must resolve via byID strip retry, got: %s", res.Summary)
		}
		facts := mut.StableInvestigationAggregateFacts()
		if len(facts) != 1 || len(facts[0].Members) != 1 || facts[0].Members[0] != "CompThread_0-2955 (合成线程)" {
			t.Fatalf("resolution lane must keep the decorated member surface intact, got %+v", facts)
		}
	})
	t.Run("family3_h6_annotation_on_parseable_core", func(t *testing.T) {
		// Annotation verbatim from eval run real_trace_h6_channel_mixed_
		// display-20260714-111026 ("attached_trace.txt:603 (caller=
		// fscache_page_get_an+0xd0c/0x1b44)"), transplanted onto a
		// parseable source core: `.txt` cores are unparseable by the
		// location-surface extension registry by design, so the h6
		// instance itself can only ground through id/byID lanes — the
		// paren-annotation FAMILY is what this arm pins end-to-end.
		mut := types.NewMutableState("q")
		bus := &types.BusContext{Mutable: mut}
		mut.AppendEvidence([]types.EvidenceItem{{
			ID:           "ev-fscache",
			Source:       "internal/agent/gate.go",
			LineStart:    603,
			AnchorSymbol: "Gate.Run",
			Snippet:      "func (g Gate) Run() error {",
		}})
		tool := &EmitInvestigationComplete{}
		res, err := tool.Execute(bus, json.RawMessage(`{
			"reason":"code members collected",
			"confidence":"high",
			"result_kind":"resolved",
			"aggregate_facts":[{
				"kind":"member_set",
				"label":"code members",
				"value":"1",
				"members":["Gate.Run (8个独立检查)"],
				"support_refs":["internal/agent/gate.go:603 (caller=fscache_page_get_an+0xd0c/0x1b44)"]
			}]
		}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.Success || !strings.HasPrefix(res.Summary, "Investigation marked complete") {
			t.Fatalf("paren-annotated file:line ref must resolve via strip retry, got: %s", res.Summary)
		}
		facts := mut.StableInvestigationAggregateFacts()
		if len(facts) != 1 || len(facts[0].Members) != 1 || facts[0].Members[0] != "Gate.Run (8个独立检查)" {
			t.Fatalf("resolution lane must keep the decorated member surface intact, got %+v", facts)
		}
	})
}

// —— Synthetic form-repair pin (deliverable b): a NON-consumable junk ref no
// longer vetoes the bare-surface repair. Baseline behavior for this exact
// payload was the "support_refs do not resolve" DOWNGRADE; now the fact
// lands with bare members and the decorator qualifier moved losslessly into
// member_notes.
func TestSupprefTol_JunkRefNoLongerVetoesBareSurfaceRepair(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}
	res, err := tool.Execute(bus, json.RawMessage(`{
		"reason":"code members collected",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"code members",
			"value":"1",
			"members":["Gate.Run (8个独立检查)"],
			"support_refs":["参见上文分析"]
		}]
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success || !strings.HasPrefix(res.Summary, "Investigation marked complete") {
		t.Fatalf("junk-ref decorated member_set must land via bare-surface repair, got: %s", res.Summary)
	}
	facts := mut.StableInvestigationAggregateFacts()
	if len(facts) != 1 {
		t.Fatalf("expected one fact, got %+v", facts)
	}
	fact := facts[0]
	if len(fact.Members) != 1 || fact.Members[0] != "Gate.Run" {
		t.Fatalf("member must be repaired to the bare surface, got %+v", fact.Members)
	}
	if len(fact.MemberNotes) != 1 || fact.MemberNotes[0] != "8个独立检查" {
		t.Fatalf("decorator qualifier must move losslessly into member_notes, got %+v", fact.MemberNotes)
	}
	if !strings.Contains(fact.Provenance, "form_repair:decorated_member_base") {
		t.Fatalf("repair provenance marker missing: %q", fact.Provenance)
	}
}

// —— Shadowed-fact uniformity pin (accepted-surface delta, argued): facts
// shadowed by a source-inventory principal row set are exempt from the
// support_refs GATE, and before this batch a junk ref (unlike no ref) also
// kept them out of the bare-surface repair. The repair is lossless
// (decorator qualifier moves to member_notes) and an identical fact one junk
// ref away must not fork its accepted surface, so both variants now repair
// identically — pinned here with the refs-absent variant as the behavior
// authority (that lane predates this batch unchanged).
func TestSupprefTol_ShadowedFactJunkRefRepairsSameAsNoRef(t *testing.T) {
	shadowed := func(refs []string) types.AnswerAggregateFact {
		return types.AnswerAggregateFact{
			Kind:        types.AnswerAggregateMemberSet,
			Label:       "shadowed members",
			Value:       "1",
			Provenance:  "demoted:shadowed_by_source_inventory_principal_row_set",
			Members:     []string{"Gate.Run (8个独立检查)"},
			SupportRefs: refs,
		}
	}
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	noRef, _ := normalizeDecoratedMemberSetFormDebt(bus, "resolved", []types.AnswerAggregateFact{shadowed(nil)}, nil)
	junkRef, _ := normalizeDecoratedMemberSetFormDebt(bus, "resolved", []types.AnswerAggregateFact{shadowed([]string{"参见上文分析"})}, nil)
	if len(noRef) != 1 || len(junkRef) != 1 {
		t.Fatalf("unexpected fact counts: %d vs %d", len(noRef), len(junkRef))
	}
	if noRef[0].Members[0] != "Gate.Run" {
		t.Fatalf("refs-absent repair lane (pre-existing) must strip to the bare surface, got %+v", noRef[0].Members)
	}
	if junkRef[0].Members[0] != noRef[0].Members[0] ||
		strings.Join(junkRef[0].MemberNotes, "\n") != strings.Join(noRef[0].MemberNotes, "\n") {
		t.Fatalf("junk-ref variant must repair identically to the refs-absent variant: %+v vs %+v", junkRef[0], noRef[0])
	}
}

// —— Schema soft-guidance presence pin: the support_refs description keeps
// the bare-form teaching sentence added by SUPPREF-TOL.
func TestSupprefTol_SupportRefsSchemaTeachesBareForm(t *testing.T) {
	tool := &EmitInvestigationComplete{}
	if !strings.Contains(string(tool.Parameters()), "Write each ref bare") {
		t.Fatalf("support_refs schema description must keep the bare-form guidance sentence")
	}
}
