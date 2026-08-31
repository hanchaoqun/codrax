# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T03:53:14Z
- sweep_start_ts: 20260830-205312
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260830-205314 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 153s | 45 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 10ms 窗、6 次 typed 查询、1 个最终投影；NetworkService-60595 的 5.951ms 已证链上候选居首，实际占时与规则可消量分列，非链 IO/D/调度观测保持背景；零成文拒绝、零固定耗时降级。 |
| 1 | read_combo_pipeline_sequence_table | FAIL | eval/results/read_combo_pipeline_sequence_table-20260830-205314 | answer_regex,answer_contains | none | 447s | 40 | read=13,repo_map=3,list=0,trace=0,source_lens=0 | midloop=5,inv=1/0,fin_reject=6,unavail=0,prune=0 | fail | 模型连续提交 Mermaid-safe `node_id=readmode` 与精确 `visible_label=read mode`，系统却因代码身份过滤拒绝；直接使用带空格 node id 又必然不安全，形成确定性合同自锁，耗尽 6 次重试并恢复旧稿。旧稿仍有完整阶段表和详细图，但 runner 正确拒绝把 degraded answer 判绿。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human audit conclusion

- `B1466` 的 participant ID 大小写修复未出现回归；本轮模型选择了另一组参与者，没有形成同一大小写分裂生产形，因此仍以确定性回归为直接闭环证据。
- 新确认 `B1467-MULTIWORDPARTICIPANTDISPLAY1/P1`：请求作用域内 typed participant 可以是多词业务身份，但 visibility executor 只把代码身份候选当作合法显示载体。`read mode` 因而同时受到“node id 不得有空格”和“安全 node id 不承载精确身份”两条互斥合同约束。
- 最优修复是让完整 parsed display label 参与精确整值身份匹配，再保留现有代码身份候选解析；禁止子串、限定词或近似词获得权限。系统仍不生成关系、不选择 participant、node id、label、布局或结论。
- Trace 护栏通过：显式窗、自动补采、链上根因、背景隔离、双账户与因果投影均存在；活动流没有因 4ms、4m、轮次、上下文比例或等待时间降级。
