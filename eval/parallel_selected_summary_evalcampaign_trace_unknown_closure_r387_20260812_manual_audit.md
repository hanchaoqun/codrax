# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T13:13:26Z
- sweep_start_ts: 20260812-061325
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h10_spantop_member_subrows | PASS | eval/results/real_trace_h10_spantop_member_subrows-20260812-061327 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 193s | 46 | read=0,repo_map=0,list=0,trace=12,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | B647 生效：不再封闭“全部等待”；target/peer 边界正确。模型列表漏写成员数值/行号，但同一最终答案的确定性附录完整发布 1.781/0.607ms 与两段行号，既有逐成员软教学已在，记模型展示波动。 |
| 1 | real_trace_h11_cross_direction_overlap | PASS | eval/results/real_trace_h11_cross_direction_overlap-20260812-061327 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 321s | 50 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=1 | fail | B646 对方向关系生效；但摘要用“typed 直接阻塞未建立”反推“并非来自外部阻塞”，是缺证据否定事实的同类泛化 gap。B648。321s 活跃流正常出答，未按年龄降级。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human Findings

### H11 — fail；方向关系已保守，但“未建立”又被反推成“不存在”

- B646 正向：正文明确 `cross_direction_physical_relation=unresolved`，不再声称方向相互独立或没有重叠；跨方向总和未发布，锁方向内部精确小计仍为 12.115ms。
- 新残余 B648：摘要写“主要卡顿并非来自外部阻塞”。typed authority 只说 `target_direct_blocking_authority=not_provided_by_projection` / `direct_blocking_decision=not_established`；它不能证明物理上没有外部阻塞。后文又正确承认同步阻塞/锁持有者尚未证明，摘要与证据边界矛盾。
- 这是 B646 的更一般形：缺少某类证据意味着 unknown，不意味着该机制为 false。应将最终 capsule 从 overlap/independence 特例提升为通用 `evidence_absence_implication=unknown_not_false`，仍只做模型上下文，不扫或改写正文。
- 本案 321s，连接持续活跃，最终答案由同一模型轮正常产生；没有四分钟年龄降级、旧稿替代或系统代写。

### H10 — pass；原因封闭修复生效，成员展示波动由确定性附录兜住

- B647 正向：模型不再声称 CompThread 的全部等待只由 GPU fence 与算力供给构成，也没有把 running 折算量叫等待分量。
- 模型正确区分目标 CompThread 与 peer `Jit thread pool-17284`，保留 TextView 与 DecimalQuantity 两个不同成员，未将 peer JIT 提升为目标因果。
- 模型自写列表本轮省略 1.781/0.607ms 与行号，但同一最终答案的确定性 Trace 投影逐成员完整发布 `1.781ms 行5969..6114`、`0.607ms 行12611..12664`。Finalizer typed context 也逐字携带四字段，且已有“每成员复制自己的 name/duration/lines”软教学；没有系统信息丢失、合同冲突或硬拒绝。按模型展示波动留观，不为一次波动增加词面门或系统代写。

结论：runner 2/2，人工 1/2。`B646/B647=production-positive`；
`B648-TRACEEVIDENCEABSENCENEGATION1=confirmed/P0`；`H10-member-visible-detail=model-variance/advisory`；
`active-stream-over-4m=production-positive/no-age-degrade`。
