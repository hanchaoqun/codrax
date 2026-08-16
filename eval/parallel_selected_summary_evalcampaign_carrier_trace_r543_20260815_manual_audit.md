# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T01:48:11Z
- sweep_start_ts: 20260815-184810
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h11_cross_direction_overlap | PASS | eval/results/real_trace_h11_cross_direction_overlap-20260815-184812 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 152s | 41 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B869 的 shared roster 已进入详细/末尾上下文，模型首段正确转录 lock_priority 的 #2+#3 互斥小计 12.115ms，并把跨方向关系定为 unresolved、禁止跨方向相加；Trace 显式窗、因果投影、自动补齐、链上主因、实际占用与现规则可消双轴均保留。但核心关系问答仍自相矛盾：表格把四个锁候选写成 `7.405+4.710+3.429+3.309`，而 typed authority 只授权前两席的小计；又称“其余候选独立”，并把三个 IO 席用 `+` 串联，均属 roster 明确未授权的 unlisted-pair 关系。Runner 未校验主答案关系作用域而假绿。新立案 B870：从同一 typed roster 构造按方向的 bounded presentation plan，明确 headline value、可参与 subtotal 的 refs 与必须单列的 unresolved refs，继续只作模型输入、不扫描/改写答案。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260815-184812 | answer_regex,answer_contains,mermaid_edge_count | none | 274s | 40 | read=11,repo_map=5,list=0,trace=0,source_lens=1 | midloop=6,inv=3/0,fin_reject=2,unavail=0,prune=1 | fail | B868 生产正证：finalizer typed coverage 明确输出四 stage=request_scoped_incident、BusContext=local_typed_incident_only、Mutable=no_incident；BusContext 两条精确 local binding 不触发重复搜索却保留 requested-relation boundary。最终图诚实保留三条 stage precedence、两个局部 dispatch/BuildAgentContext 事实，并让 BusContext/Mutable 断开，未再把局部调用冒充完整 carrier 流；拒绝由历史 4–14 次降为 2 次。中央请求仍未完成：Explorer 首轮只把 extractor/BusContext 列为 source_operation_required，把 Mutable 作为不可搜索 display boundary；没有取得 producer→carrier→consumer 的 stage-scoped transfer，最终正文却继续宣称四阶段经 BusContext/Mutable 传递完整产物，还错误称 BusContext 不可变。新立案 B871：只从 parser/grounded declared binding 升级 participant 的 source identity，并让 request-scoped relation closure 搜 producer/transfer/consumer，而非按裸名称或答案文字造边。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
