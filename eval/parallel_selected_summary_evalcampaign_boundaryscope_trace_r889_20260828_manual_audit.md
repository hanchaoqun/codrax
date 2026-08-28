# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T14:59:02Z
- sweep_start_ts: 20260828-075901
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260828-075902 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 152s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | partial | 系统面完整保留显式 2.000..2.020s 窗、四节点/三边唤醒链、11.000ms 链上 IO 第一席、三个独立 1.000ms 优先级反转候选、实际占时/规则可消双账户、背景隔离及完整 Trace 因果投影。模型正文却先写“四次唤醒”后又列三次，并把等待调用点扩写为页缓存层和整链直接阻断；同页 caveat 又承认等待对象、持有者、文件系统/后端均未证，形成口径矛盾。唯一 patch reject 是模型向只发布字符串闭集分支的 block_field_edits_v1 自造 facet_ids 数组分支；不是 schema 必带/必拒冲突，但底层反序列化报错缺少合法通道提示。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260828-075902 | answer_regex,answer_contains,mermaid_edge_count,typed_diagram_participant_coverage | none | 261s | 37 | read=8,repo_map=3,list=0,trace=0,source_lens=1 | midloop=7,inv=4/0,fin_reject=2,unavail=0,prune=0 | partial | B1382 生产正证：finalizer 只收到 boundary-only carrier，最终图收敛为 analyzer→explorer→extractor→finalizer，未再复制 30 余条内部 retry/checkpoint 关系；accepted typed coverage receipt 为 required=6/covered=6/unproven_boundaries=2，Mutable/BusContext 未证边界诚实保留。相对 r888，read_file 22→8、墙钟 427s→261s，但单次回放只证明本轮收敛，不能独立归因性能收益。图中四个阶段 subgraph 为空，Mutable/BusContext 为孤立节点，且可见标签仍带内部路径，关系表达仍 partial。首个 patch 同时使用局部 diagram 操作与未获授权整块替换，被精确拒绝；第二个局部 patch 通过，属于模型未遵循已发布 patch 能力，未发现系统合同冲突。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
