# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T07:31:12Z
- sweep_start_ts: 20260820-003111
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-003112 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 335s | 41 | read=0,repo_map=0,list=0,trace=10,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000000..2.020000s 窗、已证 threadpool-400→network-300→cookie-200→app-100、11ms IO 首席、三条 1ms 链上 runnable、实际占时/现规则可消双轴、Trace 因果投影与自动补齐均完整；邻近/背景未加冕。B1219 新词面生产生效。正文把三条 runnable 等待写成“CPU 竞争存在”略强于跨核拓扑证据，留作模型措辞观察，不作 prose 硬门。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260820-003112 | answer_regex,answer_contains,mermaid_edge_count | none | 373s | 37 | read=9,repo_map=1,list=0,trace=0,source_lens=1 | midloop=11,inv=5/0,fin_reject=3,unavail=0,prune=0 | fail | 最终图只保留四阶段 precedence 与 BusContext/Mutable 无箭头归属，未回答用户要求的数据流。B1220 的 4.3KB 有界失败 delta 生效且系统未代写边；但上下文同时声称 Mutable 是 local_typed_incident_only，却既无 typed_candidate 也无 local_operation_binding，确认覆盖状态与候选权威域漂移。正文还存在多处陈述—引用错绑（例如 Analyzer 的 Mutable 写入引用 finalizer.go:26），需结构化 item-evidence 绑定，不能用正文语义扫描补洞。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Confirmed gaps and disposition

- `B1222-FLOWLOCALCANDIDATEPARITY1` (P1, implementation next): strict request-scope subset exists时，authoring coverage 用全 evidence 的宽谓词判 local-only，而 candidate/binding 用 request-scoped Explorer-authored operations。统一到同一个 typed relation-scope projection；无可发布候选时不得声称“有精确局部操作”。
- `B1223-ANSWERITEMEVIDENCECITATIONBINDING1` (P1, design next): generic list/section item 只有手写 citation index，block-level claim_use 无法区分多 item 的证据归属。优先复用或扩展结构化 item-local evidence identity，由 accepted evidence 确定性绑定 citation；禁止扫描 item 文本判断相关性。
- Analyzer 三次结构化重试属于 JSON/typed schema adherence churn；现有 prompt 已有互斥决策表，尚未发现教学—校验矛盾，先作为 P2 模型波动观察，不新增关键词门。
- 两路均为活跃流并正常完成；没有 4ms、4m、首字节、stall 或累计年龄降级。
