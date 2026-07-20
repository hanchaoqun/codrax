package types

// trace_note_keys_golden_test.go — NKR golden snapshot of the FULL rich-note
// key registry (key|family|carrier). The diff of this file is the review
// surface for ANY registry change, exactly like the causal-token registry
// golden. If this test fails you either (a) added/renamed/removed a key —
// update the golden IN THE SAME COMMIT and walk the trace_note_keys.go
// change protocol, or (b) typo'd a registry row — fix the row, not the
// golden.

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var traceNoteKeyGoldenRows = []string{
	// G1 跨车道对账 (§27.2-G1, 2026-07-09): absorbed-side markers on the
	// critical_blocking observation; the family-side rank_family_key rides
	// the causal_rank family below. All hard consumers (projection bucket
	// relocation + 链上并入 stanza join).
	"absorbed_by_rank_family|blocking|hard_consumer",
	"absorbed_into|blocking|hard_consumer",
	// DIAG A2 (§28.11-3(b) D-10, 2026-07-09): the typed two-caliber divergence
	// disclosure (state-segment actual vs thread-level actual total).
	"actual_caliber_note|impact|hard_consumer",
	"actual_d_state|state|soft_consumer",
	"actual_impact|impact|hard_consumer",
	"actual_impact_ms|impact|hard_consumer",
	"actual_io_wait|state|soft_consumer",
	"actual_runnable|state|soft_consumer",
	"actual_running|state|soft_consumer",
	"actual_sleep|state|soft_consumer",
	// EVOLUTION RECORD (DIAG A2, 2026-07-09): soft→hard — the projection
	// compile now parses the thread-total caliber into node.ActualTotalMS.
	"actual_total|impact|hard_consumer",
	"actual_total_ms|impact|hard_consumer",
	// EVOLUTION RECORD (CR-2 P7 + R-P2-2, 2026-07-12): soft→hard — compile
	// parses the interval into ActualWindowStartTs/EndTs (⚠ containment).
	"actual_window|anchor_window|hard_consumer",
	"adds|io|display_only",
	"advisory_pretriage|ledger_marker|soft_consumer",
	"allowed_core_classes|cpu_load|display_only",
	"allowed_cpus|cpu_load|display_only",
	"also_starved|occupancy|display_only",
	"ambiguous_cohorts|io|display_only",
	"avg_latency|io|display_only",
	// EVOLUTION RECORD (R-P2-2 反向臂首跑, 2026-07-12): soft→hard (compile
	// parses node.BackgroundRank since DCS §23.1 — column under-reported).
	"background_rank|causal_rank|hard_consumer",
	"block_dev|io|display_only",
	"block_max|io|display_only",
	// DSTATE-REFINE arm a (CAL-1 件③, 2026-07-12): unanimous caller disclosure.
	"blocked_reason_caller|state|hard_consumer",
	// 件1 census 根修 (修复轮, 2026-07-13): pid-keyed per-caller full census
	// (符号×count×Σms) + its caller-overflow count; the model evidence feed
	// consumes both deterministically.
	"blocked_reason_census|state|soft_consumer",
	"blocked_reason_census_overflow|state|soft_consumer",
	// CR-3 件② P10 (2026-07-12): unconsumed blocked_reason residual on the
	// unresolved D-family row (冷读案7 GPU-fence witness).
	"blocked_reason_window_caller|state|hard_consumer",
	"blocked_reason_window_count|state|hard_consumer",
	"blocking_candidate|blocking|display_only",
	// BLOCKFROM (§27.4 G13, 2026-07-09): waiter-side blocking call site
	// ("blocking from <sig>(<file:line>)" payload tail). EVOLUTION RECORD
	// (Wave-3.2 收尾): display→hard_consumer — the DISP-2 projection read-in
	// (等待点 detail line) landed the same wave.
	"blocking_from_site|blocking|hard_consumer",
	"blocking_kind|blocking|hard_consumer",
	// LOCKNS-FIX 件3 (§29.104.12, 2026-07-16): unknown-morphology fail-open
	// marker on a payload-less blocking_span row (明细持有者核查行
	// 「owner 未解析(形态未注册)」; 嘈声检测信号只作软披露,不作门).
	"blocking_owner_key_unregistered|blocking|hard_consumer",
	// XERR1-FIX 件1/件3 (§29.104.3/.4, 2026-07-15): payload-less blocking_span
	// value-basis (wait_segments/span_envelope word-face fork), converged Σ +
	// sleep component (互指 gate), the preserved envelope disclosure, and the
	// budget-sanity marker trio (⚠ 「span 包络 X > 窗内非 running Y」 line).
	"blocking_span_envelope_ms|blocking|hard_consumer",
	"blocking_value_basis|blocking|hard_consumer",
	// XERR1-FIX 修补 件F (冷读 P3-3, 2026-07-16): partial-coverage lower-bound
	// disclosure pair (明细覆盖核查行「收敛值为已证下界」).
	"blocking_wait_account_covered_ms|blocking|hard_consumer",
	"blocking_wait_budget_exceeded|blocking|hard_consumer",
	"blocking_wait_budget_non_running_ms|blocking|hard_consumer",
	"blocking_wait_budget_running_ms|blocking|hard_consumer",
	"blocking_wait_coverage_partial|blocking|hard_consumer",
	"blocking_wait_segment_ms|blocking|hard_consumer",
	"blocking_wait_sleep_ms|blocking|hard_consumer",
	// P0-E CHAIN-PATH (ledger §22.1): per-branch path record identity + the
	// rank/impact rows' owning-branch attach domain.
	"branch|chain_path|hard_consumer",
	"branches|chain_path|display_only",
	// SPANVIS-1 (2026-07-19): the pure-advisory business-span mention face —
	// all-or-nothing per-record parse into the projection-level side channel
	// (no node/seat/ordinal); tree ◈ advisory block + ◎ 旁栏 footnote consume.
	"business_span_basis|business_span|hard_consumer",
	"business_span_count|business_span|hard_consumer",
	"business_span_hidden|business_span|hard_consumer",
	"business_span_lines|business_span|hard_consumer",
	"business_span_max_ms|business_span|hard_consumer",
	"business_span_name|business_span|hard_consumer",
	"business_span_omitted|business_span|hard_consumer",
	"business_span_total_ms|business_span|hard_consumer",
	"bytes|io|display_only",
	"callstack|io|display_only",
	"candidate_count|causal_rank|display_only",
	"capacity_truncated|causal_rank|hard_consumer",
	"category|plugin|display_only",
	"causality|causal_rank|hard_consumer",
	// RSPA (§29.61.10a/b/c, 2026-07-14): the on-chain seat-value re-anchoring
	// bipartition trio — 全窗 = 锚定 + 余段 (同源二分,唯一可相加还原形);
	// the remainder marker gates the ◇ half's lane words.
	// RNB-1 (§29.88 R2/R4, 2026-07-14): + the case-A' ownership-divergence
	// double-Σ disclosure (census / chain_lane / ownership_divergent) and the
	// R4 whole-seat lane-demotion marker (credential_lane_demoted).
	"chain_anchor_census|state|hard_consumer",
	"chain_anchor_chain_lane|state|hard_consumer",
	"chain_anchor_full|state|hard_consumer",
	"chain_anchor_ownership_divergent|state|hard_consumer",
	"chain_anchor_remainder_seat|state|hard_consumer",
	// XLANE-1 件1 (§29.104.2, 2026-07-15): the represented-by-chain-seat
	// whole-seat ◇ demotion marker (honest sibling of credential_lane_demoted).
	"chain_anchor_represented_by_chain_seat|state|hard_consumer",
	"chain_anchored|state|hard_consumer",
	"chain_branch|causal_rank|hard_consumer",
	// HULL-CRED (§29.104 终判③, 2026-07-17): the keep-⛓ per-segment
	// credential trio (envelope_level / segment_disjoint / segments) around
	// the RNB-1 R4 lane-demotion marker — segment inventory (proof carriage),
	// all-disjoint demote marker, envelope-tier honest-word marker.
	"chain_credential_envelope_level|state|hard_consumer",
	"chain_credential_lane_demoted|state|hard_consumer",
	"chain_credential_segment_disjoint|state|hard_consumer",
	"chain_credential_segments|state|hard_consumer",
	"chain_credential_segments_truncated|state|hard_consumer",
	"chain_depth|causal_rank|hard_consumer",
	// ONCHAIN-FIX-1 件1 (2026-07-18): the interval-less identity-inheritance
	// admission marker (fail-open keep disclosure; fabricated overlap retired).
	"chain_identity_inheritance|state|hard_consumer",
	"chain_relevance|causal_rank|hard_consumer",
	"chain_required|causal_rank|hard_consumer",
	"churn|io|display_only",
	"clock_set_rate|supply_pressure|display_only",
	"completions|io|display_only",
	"constraint|cpu_load|display_only",
	"context|dma_fence|display_only",
	"core_class|cpu_load|display_only",
	"core_classes|cpu_load|display_only",
	"core_limited_cpu_ms|compute_supply|display_only",
	"count|io|display_only",
	"coverage_mode|causal_rank|display_only",
	"cpu|cpu_load|display_only",
	// RNB-2 件5 AFF-EVID (§29.88.6, 2026-07-15): affinity/cpuset judgment
	// payload quintet — the 行3/明细 constraint-description inputs.
	"cpu_constraint_allowed_cpus|state|hard_consumer",
	// R5a (§29.88.4 场景② 按核档, 2026-07-15): the tier-exclusion proof pair.
	"cpu_constraint_allowed_max_tier_khz|state|hard_consumer",
	"cpu_constraint_cpuset|state|hard_consumer",
	"cpu_constraint_excluded_cpus|state|hard_consumer",
	"cpu_constraint_global_max_tier_khz|state|hard_consumer",
	"cpu_constraint_kind|state|hard_consumer",
	"cpu_constraint_policy|state|hard_consumer",
	"cpu_count|compute_supply|display_only",
	"cpus|cpu_load|display_only",
	"cpuset|cpu_load|display_only",
	// AXIOM-V2 件2/件3 (2026-07-18): the cross-direction overlap pair roster
	// (display 互指句, hard) and the un-pointable pair type-token disclosure
	// (audit only).
	"cross_direction_overlap_undisclosed|causal_rank|display_only",
	"cross_direction_overlaps|causal_rank|hard_consumer",
	"cumulative_impact_ms|impact|hard_consumer",
	// RANKDIS-M18 (§29.104.17 裁定② 2026-07-16): composite-score twins of the
	// ms-semantic value keys — one row emits exactly one family; the
	// projection/board unions read both (hard), the projected pair mirrors
	// the display-only ms echoes.
	"cumulative_impact_score|impact|hard_consumer",
	"d_state|state|hard_consumer",
	"ddr|supply_pressure|display_only",
	"delay|sched_accounting|display_only",
	"deletes|io|display_only",
	"delivered_cpu_ms|compute_supply|display_only",
	"depth|causal_rank|hard_consumer",
	"detected_period_ms|periodic|hard_consumer",
	"deterministic_running|state|hard_consumer",
	"deterministic_runtime_query_present|ledger_marker|soft_consumer",
	// EVOLUTION RECORD (RCM §24.7.1 ①/§24.9-B F3, 2026-07-08): dev/inode
	// promoted display_only → hard_consumer — rank rows now carry them from
	// typed fields and the projection compile parses them into node fields.
	"dev|io|hard_consumer",
	// AXIOM-V2 件3 (2026-07-18): the direction-conservation violation finding
	// (公理 v2 违宪形 disclosure / 立案素材; audit face only).
	"direction_conservation_excess|causal_rank|hard_consumer",
	"domain|plugin|display_only",
	"dominant_state|state|hard_consumer",
	"drill_status|blocking|display_only",
	"driver|dma_fence|display_only",
	"dso|perf|soft_consumer",
	// DSTATE-REFINE arm a (CAL-1 件③, 2026-07-12): the refined-D coverage proof.
	"dstate_all_noniowait|state|hard_consumer",
	// §29.50.5 (v5 P1 批 件②, 2026-07-13): proof-partition honest remainder.
	"dstate_cause_unproven_remainder|state|hard_consumer",
	"duration|impact|display_only",
	"edge_count|causal_rank|display_only",
	"edges|chain_path|display_only",
	"effective_impact|impact|hard_consumer",
	"effective_impact_ms|impact|hard_consumer",
	"effective_impact_score|impact|hard_consumer",
	"event|io|display_only",
	"example|io|display_only",
	"file_bytes|io|display_only",
	"file_events|io|display_only",
	// AXIOM-V2 件1 (2026-07-18): the registry fix-direction attribute — the
	// display 行2 修向 word and the 互指句 direction qualifier fork on it.
	"fix_direction|causal_rank|hard_consumer",
	"flags|blocking|display_only",
	"fold_basis|supply_fold|hard_consumer",
	// CAP (§26 C3, 2026-07-08): the two typed capability-caliber keys — the
	// display wording forks (按默认算力比粗算 / 簇结构不可判) parse them into
	// node fields (hard_consumer).
	"fold_capability|supply_fold|hard_consumer",
	// CLUSTER-FIX-2 件1 (S1, 2026-07-20): the typed freq_only cause token —
	// the single-cluster wording fork (仅单簇有频点采样…) keys on it.
	"fold_capability_freq_only_reason|supply_fold|hard_consumer",
	"fold_cluster_freq_reuse|supply_fold|display_only",
	"fold_cluster_lane_caveat|supply_fold|display_only",
	// CAP-2 (§28.4/§28.5, 2026-07-09): cluster-structure source (wording
	// upgrade fork, hard) + the rail-family/roster audit note (display).
	"fold_cluster_topology|supply_fold|hard_consumer",
	"fold_fmax|supply_fold|display_only",
	"fold_fmax_finding|supply_fold|display_only",
	"fold_rail_basis|supply_fold|display_only",
	// CAP 复核 F1 (2026-07-08): the demoted-reference basis class (absence =
	// the nominated big-class basis).
	"fold_reference_class|supply_fold|hard_consumer",
	"folded_max_ms|causal_rank|hard_consumer",
	"folded_min_ms|causal_rank|hard_consumer",
	"folded_rows|causal_rank|hard_consumer",
	"folded_subjects|causal_rank|hard_consumer",
	"fragments|state|soft_consumer",
	"freq|cpu_load|display_only",
	"function|workqueue|display_only",
	"gated_aggregation_caliber|gating|display_only",
	// CAP (§26 C3): the gated twin of fold_capability above.
	"gated_capability|gating|hard_consumer",
	// DISPHYG-3 件7 (2026-07-20): the gated twin of
	// fold_capability_freq_only_reason — the CLUSTER-FIX-2 D5 batch boundary
	// closed.
	"gated_capability_freq_only_reason|gating|hard_consumer",
	// CAP-2: the gated twin of fold_cluster_topology.
	"gated_cluster_topology|gating|hard_consumer",
	// PARTSPLIT-1 (§29.150④, 2026-07-19): R4-mirror refusal record — seat-face
	// pre/post/anchor pair + side-channel account/seat_published.
	"gated_composite_edge_account|state|hard_consumer",
	"gated_composite_edge_anchor_ts|state|hard_consumer",
	"gated_composite_edge_anchor_via|state|hard_consumer",
	"gated_composite_edge_post_share|state|hard_consumer",
	"gated_composite_edge_pre_share|state|hard_consumer",
	"gated_composite_edge_seat_published|state|hard_consumer",
	"gated_runnable|gating|hard_consumer",
	"gated_running_deficit|gating|hard_consumer",
	// LEVELMERGE-1 件2 (方案 P 区间分账, 2026-07-18): the gated-share split
	// family — A/B decomposition floats, the constituent-row marker, the
	// claim-seat [E#] pointer roster and the fail-open overlap disclosure.
	"gated_share_claim_seats|state|hard_consumer",
	"gated_share_claimed|state|hard_consumer",
	"gated_share_constituent_seat|state|hard_consumer",
	"gated_share_full|state|hard_consumer",
	"gated_share_overlap|state|hard_consumer",
	// INODE (§28.6, 2026-07-09): top_io_inode whole-window fold row keys.
	"groups_total|io|display_only",
	// ANSWERFACE-1 件2 (§29.140 G6, 2026-07-19): target_window_states
	// boundary-fold disclosure quartet (head half).
	"head_carry_ms|state|hard_consumer",
	"head_carry_state|state|hard_consumer",
	"high_prio|cpu_load|display_only",
	"high_prio_overlap|cpu_load|display_only",
	"high_prio_running|cpu_load|display_only",
	"holder_handoff|blocking|hard_consumer",
	"holder_host_process|blocking|display_only",
	// EVOLUTION RECORD (LOCKNS-FIX 件6 / OM-10 关账, §29.104.12, 2026-07-16):
	// display→hard_consumer — the projection compile reads the ②×③
	// identity-unification declaration into
	// TraceCausalProjectionNode.BlockingHolderNsUnification and the detail
	// 持有者来历 line appends the 「发射对×收尾唤醒两道互证」 disclosure.
	"holder_ns_unification|blocking|hard_consumer",
	"holder_self_contradiction|blocking|hard_consumer",
	// G10-EN 根修 (QH2-A, 2026-07-14): the self-contradiction witness typed
	// component quintet — the compile assembles
	// BlockingHolderContradictionParts from them so the zh/EN detail lanes
	// each word their own sentence; the legacy zh string key keeps the
	// byte-frozen audit-verbatim value.
	"holder_self_contradiction_holder|blocking|hard_consumer",
	"holder_self_contradiction_lines|blocking|hard_consumer",
	"holder_self_contradiction_owner_tid|blocking|hard_consumer",
	"holder_self_contradiction_queued_ms|blocking|hard_consumer",
	"holder_self_contradiction_span_ms|blocking|hard_consumer",
	"holder_site|blocking|hard_consumer",
	"holder_source|blocking|hard_consumer",
	// R3-IMPL (§29.88.1, 2026-07-15): the host-edge-anchored semantic seat's
	// credential disclosure pair (行2 边锚定(宿主→目标) sentence inputs).
	"host_wakeup_edge_anchor_ts|causal_rank|hard_consumer",
	"host_wakeup_edge_anchor_via|causal_rank|hard_consumer",
	"idle_mismatch_ms|compute_supply|soft_consumer",
	"impact|impact|hard_consumer",
	"impact_ms|impact|hard_consumer",
	"impact_score|impact|hard_consumer",
	"inherited_target_blocked_ms|impact|display_only",
	"inode|io|hard_consumer",
	"io_wait|state|hard_consumer",
	"iowait_blocked|io|display_only",
	"kind|cpu_load|display_only",
	"l3|supply_pressure|display_only",
	"latency|chain_path|display_only",
	"lateness_ms|periodic|hard_consumer",
	"layer|io|display_only",
	"legacy_summary_fallback|ledger_marker|display_only",
	"line|io|display_only",
	"lock_twin_folded|blocking|soft_consumer",
	"low_freq_cpus|supply_pressure|display_only",
	"low_freq_loss_cpu_ms|compute_supply|display_only",
	"max|interrupt|display_only",
	"max_delay|sched_accounting|display_only",
	"max_latency|io|display_only",
	"max_runtime|sched_accounting|display_only",
	"max_segment|state|soft_consumer",
	// RCM 家族合并族 (§24.7.1/§24.10, 2026-07-08): engine same-thread family
	// merge carriers — the projection compile parses them into the isolated
	// FamilyMember* node lane (never MergedCount/MergedMaxMS).
	"member_count|causal_rank|hard_consumer",
	"member_fold_caliber|causal_rank|hard_consumer",
	// XLANE-2 件1 (2026-07-17): complete per-member line ranges of a semantic
	// family seat — the display 成员子集 subset-judgment input.
	// (件2's self_gap_semantic_overlaps rides further down in sort order.)
	"member_line_ranges|causal_rank|hard_consumer",
	"member_max_ms|causal_rank|hard_consumer",
	"member_min_ms|causal_rank|hard_consumer",
	"member_roster|causal_rank|hard_consumer",
	"member_sum_ms|causal_rank|hard_consumer",
	// SPANTOP-1 件1 (§29.131): complete per-member wall-clock list — the
	// display constituent top-3 sub-row input (µs identity gated).
	"member_wall_ms|causal_rank|hard_consumer",
	"metric|plugin|display_only",
	"migrations|cpu_load|display_only",
	"name|io|display_only",
	"nearest_block_thread|io|display_only",
	"nearest_chain_thread|causal_rank|display_only",
	"nearest_chain_window|anchor_window|soft_consumer",
	"next_step|guidance|soft_consumer",
	"next_step_kind|guidance|soft_consumer",
	"nodes|chain_path|display_only",
	"not_answer_grade|ledger_marker|display_only",
	"observed_core_class|cpu_load|display_only",
	"observed_cpu|cpu_load|display_only",
	"occupier_1|occupancy|hard_consumer",
	"occupier_2|occupancy|hard_consumer",
	"occupier_3|occupancy|hard_consumer",
	"occurrence_windows|anchor_window|soft_consumer",
	"occurrences|causal_rank|display_only",
	"offsets|io|display_only",
	// SELF-SEM (§29.61.1, 2026-07-13): on-chain proof-basis marker.
	"on_chain_basis|causal_rank|hard_consumer",
	"oneway|blocking|display_only",
	"op|io|display_only",
	"other_cpu_idle|cpu_load|display_only",
	// EVOLUTION RECORD (审计 #5/#62, §29.25 处置委托 + §29.26 待主会话落账,
	// 2026-07-10): display_only → hard_consumer — on-chain semantic-span
	// intersection carrier (SemanticChainProjectedMS).
	"overlap|causal_rank|hard_consumer",
	// LOCKNS-FIX 修补 件A (冷读 P2-F1+P3-F7, 2026-07-16): typed payload-owner
	// -tid presence verdict (absent/present_collision/present_comm_mismatch)
	// — 明细持有者来历 presence 分句 fork; 缺席 fail-open 保 legacy 句逐字节.
	"owner_tid_presence|blocking|hard_consumer",
	"owner_tid_raw|blocking|hard_consumer",
	"p95_segment|state|soft_consumer",
	"page_cache_churn|io|display_only",
	"paired|io|display_only",
	"pairing_suppressed|io|display_only",
	"path|chain_path|soft_consumer",
	"peer|blocking|hard_consumer",
	"peer_chain_blocker|blocking|display_only",
	"peer_chain_blocker_source|blocking|display_only",
	"peer_chain_blocker_state|blocking|display_only",
	"peer_chain_presumptive|blocking|display_only",
	"peer_chain_state|blocking|display_only",
	"peer_source|blocking|display_only",
	"peer_state_d_state|blocking|display_only",
	"peer_state_dominant|blocking|display_only",
	"peer_state_fragments|blocking|display_only",
	"peer_state_io_wait|blocking|display_only",
	"peer_state_runnable|blocking|display_only",
	"peer_state_running|blocking|display_only",
	"peer_state_sleep|blocking|display_only",
	"peer_state_total|blocking|display_only",
	"percent|perf|display_only",
	"perf_context|perf|display_only",
	"perf_contexts|perf|display_only",
	"perf_quality|perf|soft_consumer",
	"perf_quality_caveats|perf|soft_consumer",
	"periodic_source|periodic|hard_consumer",
	"policy|cpu_load|display_only",
	"pressure_density|occupancy|display_only",
	"prio|cpu_load|display_only",
	"priority|cpu_load|display_only",
	"priority_artifact_source|gating|display_only",
	"priority_inversion_candidate|gating|hard_consumer",
	"priority_inversion_edges|gating|display_only",
	"priority_inversion_gated|gating|display_only",
	"priority_relation|gating|display_only",
	"priority_relation_artifact_sources|gating|display_only",
	// TQ-PRIORITY-POINT-AUTHORITY (2026-07-17): NKR promotion after the
	// observation/projection compile began preserving the typed proof account.
	"priority_relation_caliber|gating|hard_consumer",
	"priority_relation_proven_lower_ms|gating|hard_consumer",
	"priority_relation_unknown_or_nonlower_ms|gating|hard_consumer",
	"priority_source|gating|hard_consumer",
	"process|cpu_load|display_only",
	// CR-3 件③ P11 (2026-07-12): rank-row process attribution (冷读案8).
	"process_comm|causal_rank|hard_consumer",
	// EVOLUTION RECORD (审计 #5/#62, 2026-07-10): display_only →
	// hard_consumer — the on-chain semantic FAMILY record's exact
	// intersection participation (SemanticChainProjectedMS); the rank-lane
	// projected_impact_ms display echo stays display-only.
	"projected_impact|impact|hard_consumer",
	"projected_impact_ms|impact|display_only",
	"projected_impact_score|impact|display_only",
	"projected_total|impact|display_only",
	"projected_total_ms|impact|display_only",
	"projected_total_score|impact|display_only",
	"rank|causal_rank|hard_consumer",
	// XLANE-3 件1 (§29.104.2 定谳③, 2026-07-16): the rank board identity
	// triple's params/target halves (multi-board split + chip anchor inputs).
	"rank_board_params_fingerprint|causal_rank|hard_consumer",
	"rank_board_target|causal_rank|hard_consumer",
	// G1 跨车道对账 (§27.2-G1, 2026-07-09): family-side canonical identity on
	// the absorbing rank observation (absorbed-side keys ride blocking above).
	"rank_family_key|causal_rank|hard_consumer",
	// RANKDIS-M18: renamed from rank_impact — the state-drilldown composite
	// weight (§7.30 S1 witness) joins the score vocabulary with its JSON tag
	// (rank_impact_ms → rank_impact_score); zero parsers, zero-compat rename.
	"rank_impact_score|impact|display_only",
	"reads|io|display_only",
	"recommended_sections|causal_rank|display_only",
	"recommended_views|causal_rank|soft_consumer",
	"recursive|causal_rank|soft_consumer",
	// RSPA M-IO (§29.61.10c, 2026-07-14): per-IO completion-closure credential.
	"resource_completion_closure|state|hard_consumer",
	"ret|io|display_only",
	"runnable|state|hard_consumer",
	// EVOLUTION RECORD (SYM-2 §24.17 R2, 2026-07-08): the typed below-RT
	// preemption disclosure on self runnable rank rows — new hard-consumer key.
	"runnable_below_rt_preempted|state|hard_consumer",
	"runnable_cpu|guidance|soft_consumer",
	"running|state|hard_consumer",
	"runtime|sched_accounting|display_only",
	"same_cpu_busy|cpu_load|display_only",
	"same_cpu_idle|cpu_load|display_only",
	// DIAG A1 (§28.11-3(a) G12, 2026-07-09): µs-tie fold-member roster on
	// cross-thread take-MAX fold records (huadong_79 E23 shape).
	"same_value_members|causal_rank|hard_consumer",
	"sample_weight|perf|display_only",
	"samples|perf|display_only",
	"score|causal_rank|display_only",
	"selected_frame_id|causal_rank|display_only",
	"selected_name|causal_rank|display_only",
	"selected_phase|causal_rank|display_only",
	"selected_role|causal_rank|display_only",
	"selected_window|anchor_window|anchor_window",
	// XLANE-2 件2 (2026-07-17): the self-gap seat's semantic-overlap
	// disclosure roster.
	"self_gap_semantic_overlaps|causal_rank|hard_consumer",
	// RULER2-1 (§29.150② / R-19-b, 2026-07-19): the self runnable two-ruler
	// accounting record (per-ruler seat values/ordinals + same-ruler
	// subtotals; NO cross-ruler total key — M3 禁混尺).
	"self_two_ruler_edge_effs|causal_rank|hard_consumer",
	"self_two_ruler_edge_ranks|causal_rank|hard_consumer",
	"self_two_ruler_edge_subtotal|causal_rank|hard_consumer",
	"self_two_ruler_wall_effs|causal_rank|hard_consumer",
	"self_two_ruler_wall_ranks|causal_rank|hard_consumer",
	"self_two_ruler_wall_subtotal|causal_rank|hard_consumer",
	"semantic_class|span|hard_consumer",
	"seqno|dma_fence|display_only",
	"signal|io|display_only",
	"significant|causal_rank|soft_consumer",
	"sleep|state|hard_consumer",
	"sleep_io_wait|state|hard_consumer",
	"source|causal_rank|soft_consumer",
	"span_category|span|hard_consumer",
	"span_kind|span|hard_consumer",
	"span_name|span|hard_consumer",
	"span_subcategory|span|hard_consumer",
	"starved_runnable_ms|occupancy|display_only",
	"state|state|display_only",
	// RANKDIS-EXT A3 (§29.104.16.1 M15, 2026-07-16): the state_drilldown
	// ordinal's dedicated lane — `rank` stays exclusively causal-board.
	"state_rank|state|display_only",
	"storage_max|io|display_only",
	"subject_chain_blocker|blocking|display_only",
	"subject_chain_blocker_source|blocking|display_only",
	"subject_chain_blocker_state|blocking|display_only",
	"subject_chain_presumptive|blocking|display_only",
	"subject_chain_state|blocking|display_only",
	"subject_is_lock_holder|blocking|hard_consumer",
	"subject_kind|causal_rank|hard_consumer",
	"subject_state_d_state|blocking|display_only",
	"subject_state_dominant|blocking|display_only",
	"subject_state_fragments|blocking|display_only",
	"subject_state_io_wait|blocking|display_only",
	"subject_state_runnable|blocking|display_only",
	"subject_state_running|blocking|display_only",
	"subject_state_sleep|blocking|display_only",
	"subject_state_total|blocking|display_only",
	"supply_fold_deficit_ms|supply_fold|hard_consumer",
	"supply_fold_ideal_ms|supply_fold|hard_consumer",
	"supply_ratio|compute_supply|soft_consumer",
	"switches|state|soft_consumer",
	"symbol|perf|display_only",
	"symbolization_status|perf|display_only",
	"sync_like|blocking|display_only",
	"system_or_kernel_competitors|cpu_load|display_only",
	"system_or_kernel_overlap|cpu_load|display_only",
	"system_or_kernel_running|cpu_load|display_only",
	// ANSWERFACE-1 件2 (§29.140 G6, 2026-07-19): target_window_states
	// boundary-fold disclosure quartet (tail half).
	"tail_open_ms|state|hard_consumer",
	"tail_open_state|state|hard_consumer",
	"target|chain_path|display_only",
	"target_cpus|interrupt|display_only",
	// EVOLUTION RECORD (COV 批, §24.9 D-1, 2026-07-08): target_impact family
	// display_only → hard_consumer — typed TargetImpactMS promotion for the
	// coverage-sentence numerator (永不读 §20.1 展示覆写后的 cumulative 通道).
	"target_impact|impact|hard_consumer",
	"target_impact_ms|impact|hard_consumer",
	"target_mask|interrupt|display_only",
	"target_prio|cpu_load|display_only",
	"target_priority|cpu_load|display_only",
	"target_priority_artifact_source|gating|display_only",
	"target_priority_source|gating|hard_consumer",
	// CR-3 件③ P11 (2026-07-12): rank-row process attribution (冷读案8).
	"tgid|causal_rank|hard_consumer",
	"thermal|supply_pressure|display_only",
	// THERM (§28.5-T7, 2026-07-09): in-window thermal/policy press on the
	// fold's dominant running cluster (窗内该簇受热限压至 X sentence).
	"thermal_cap_khz|supply_fold|hard_consumer",
	// CR-3 件⑥ F-10 (2026-07-12): the cap's in-window witness bit (冷读 D5).
	"thermal_cap_witnessed|supply_fold|hard_consumer",
	"thread|cpu_load|display_only",
	"threads|cpu_load|display_only",
	"throughput|supply_pressure|display_only",
	"tier|causal_rank|hard_consumer",
	"timeline|dma_fence|display_only",
	"top_background_process|cpu_load|display_only",
	"top_background_threads|cpu_load|display_only",
	"top_competitor|guidance|soft_consumer",
	"top_competitor_overlap|guidance|display_only",
	"top_competitor_running|guidance|display_only",
	"top_dev|io|display_only",
	"top_inode|io|display_only",
	"top_name|io|display_only",
	"top_thread|cpu_load|display_only",
	"top_thread_ms|cpu_load|display_only",
	"top_threads|io|display_only",
	// EVOLUTION RECORD (R-P2-2 反向臂首跑, 2026-07-12): soft→hard (compile
	// parses node.TotalMS — column under-reported).
	"total|impact|hard_consumer",
	"total_latency|io|display_only",
	// G2 判据 typed 化 (§27.2/§28.1, 2026-07-09): the trace_gap blind-spot
	// criterion enum (no_sched_data / no_eligible_wait). EVOLUTION RECORD
	// (Wave-3.2 收尾): display→hard_consumer — the DISP-2 ◇ wording fork
	// parses it in the projection compile; TraceNoteKeyTraceGapKind exported.
	"trace_gap_kind|causal_rank|hard_consumer",
	"type|causal_rank|hard_consumer",
	"unpaired_done|io|display_only",
	"unpaired_start|io|display_only",
	"value|plugin|display_only",
	"vector|interrupt|display_only",
	"verdict|cpu_load|display_only",
	// SA-F2 (DISPATCH-IND 批4, 2026-07-14): per-generator vsync/frame-pacing
	// census notes (display tier — no consumer parses them yet).
	"vsync_generator_census_caliber|vsync_census|display_only",
	"vsync_generator_census_events|vsync_census|display_only",
	"vsync_generator_census_first_ts|vsync_census|display_only",
	"vsync_generator_census_identified_by|vsync_census|display_only",
	"vsync_generator_census_last_ts|vsync_census|display_only",
	"vsync_generator_census_period_ns|vsync_census|display_only",
	"vsync_generator_census_period_prints|vsync_census|display_only",
	"vsync_generator_census_refresh_rate|vsync_census|display_only",
	"vsync_generator_census_trace_marks|vsync_census|display_only",
	"vsync_generator_census_woken|vsync_census|display_only",
	"wait_object|blocking|display_only",
	"waiters|blocking|hard_consumer",
	"wakee_priority|chain_path|display_only",
	"wakee_priority_artifact_source|gating|display_only",
	"wakee_priority_authority|gating|display_only",
	"wakee_priority_source|gating|display_only",
	"waker_priority|chain_path|display_only",
	"waker_priority_artifact_source|gating|display_only",
	"waker_priority_source|gating|display_only",
	// WAKE-CENSUS (§29.58, 2026-07-13): per-pair whole-inventory wakeup-edge
	// census (count folds pre-cap; overflow discloses the pair-cap trim).
	// WAKE-CENSUS-D 2A (§29.58.4, 2026-07-13): typed exit-state split trio.
	"wakeup_edge_census_d_exit|chain_path|soft_consumer",
	"wakeup_edge_census_first_ts|chain_path|soft_consumer",
	"wakeup_edge_census_last_ts|chain_path|soft_consumer",
	"wakeup_edge_census_other_exit|chain_path|soft_consumer",
	"wakeup_edge_census_overflow_edges|chain_path|soft_consumer",
	"wakeup_edge_census_overflow_pairs|chain_path|soft_consumer",
	"wakeup_edge_census_sleep_exit|chain_path|soft_consumer",
	// 修复轮 件2 (2026-07-13): per-result target-wakee completeness marker.
	"wakeup_edge_census_target_wakee|chain_path|soft_consumer",
	"wakeup_ts|chain_path|display_only",
	"weight_unit|perf|display_only",
	"window|anchor_window|anchor_window",
	"window_ms|anchor_window|soft_consumer",
	"window_proportion|anchor_window|display_only",
	"window_source|anchor_window|anchor_window",
	"work|workqueue|display_only",
	"writes|io|display_only",
}

func TestTraceNoteKeyRegistryGolden(t *testing.T) {
	rows := TraceNoteKeyRows()
	sort.Slice(rows, func(i, j int) bool { return rows[i].Key < rows[j].Key })
	got := make([]string, 0, len(rows))
	for _, row := range rows {
		got = append(got, fmt.Sprintf("%s|%s|%s", row.Key, row.Family, row.Carrier))
	}
	// The golden list is maintained in key-sorted order (the same order the
	// registry renders); compare positionally so the golden file itself is
	// the canonical layout.
	want := append([]string(nil), traceNoteKeyGoldenRows...)
	if len(got) != len(want) {
		t.Fatalf("registry has %d rows, golden has %d rows\nregistry:\n%s\ngolden:\n%s",
			len(got), len(want), strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("registry/golden diverge at sorted row %d:\n  registry: %s\n  golden:   %s", i, got[i], want[i])
		}
	}
}

// TestTraceNoteKeyRegistryStructure lints the table itself: unique keys,
// wire-safe charset, occupier prefix/cap lockstep, and the anchor-window
// whitelist stays EXACTLY the three adjudicated carriers.
func TestTraceNoteKeyRegistryStructure(t *testing.T) {
	keyPattern := regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	seen := map[string]bool{}
	var anchors []string
	for _, row := range TraceNoteKeyRows() {
		if seen[row.Key] {
			t.Errorf("duplicate registry key %q", row.Key)
		}
		seen[row.Key] = true
		if !keyPattern.MatchString(row.Key) {
			t.Errorf("registry key %q is not wire-safe (want ^[a-z][a-z0-9_]*$)", row.Key)
		}
		switch row.Carrier {
		case TraceNoteCarrierAnchorWindow, TraceNoteCarrierHardConsumer,
			TraceNoteCarrierSoftConsumer, TraceNoteCarrierDisplayOnly:
		default:
			t.Errorf("registry key %q has unknown carrier %q", row.Key, row.Carrier)
		}
		if row.Carrier == TraceNoteCarrierAnchorWindow {
			anchors = append(anchors, row.Key)
		}
		if strings.HasPrefix(row.Key, TraceNoteKeyActualPrefix) && row.Carrier == TraceNoteCarrierDisplayOnly {
			t.Errorf("registry key %q: actual_* keys are prefix-consumed dual-basis markers and can never be display_only", row.Key)
		}
	}
	sort.Strings(anchors)
	wantAnchors := []string{TraceNoteKeySelectedWindow, TraceNoteKeyWindow, TraceNoteKeyWindowSource}
	sort.Strings(wantAnchors)
	if strings.Join(anchors, ",") != strings.Join(wantAnchors, ",") {
		t.Errorf("anchor-window whitelist drifted: got %v want %v", anchors, wantAnchors)
	}
	// Occupier roster: prefix + the three registered ordinals stay in lockstep
	// with the producer cap of 3.
	for i := 1; i <= 3; i++ {
		key := fmt.Sprintf("%s%d", TraceNoteKeyOccupierPrefix, i)
		if !TraceNoteKeyRegistered(key) {
			t.Errorf("occupier roster key %q missing from registry", key)
		}
	}
	if TraceNoteKeyRegistered(TraceNoteKeyOccupierPrefix + "4") {
		t.Errorf("occupier_4 registered but the producer roster cap is 3 — raise both in lockstep or neither")
	}
}
