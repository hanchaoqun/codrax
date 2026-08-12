# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T11:10:32Z
- sweep_start_ts: 20260812-041031
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h11_cross_direction_overlap | PASS | eval/results/real_trace_h11_cross_direction_overlap-20260812-041032 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 133s | 41 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Runner 只钉到 overlap 字样和四方向。人工核验发现正文称 IO #5=3.670ms 与 #6=3.598ms 有 42.193ms 物理重叠，随后又称四方向没有 typed overlap/dependency。42.193ms 来自系统把两个 broad row envelope 的交集铸成 physical_relation=overlap，且大于任一席位值；是 typed 上下文自冲突，不是模型随机波动。双轴仍保留：模型正文列出实际占用/业务 span 与规则计价方向，系统投影也严格区分链上、邻近和背景。 |
| 2 | real_trace_h10_spantop_member_subrows | PASS | eval/results/real_trace_h10_spantop_member_subrows-20260812-041032 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 255s | 47 | read=0,repo_map=0,list=0,trace=13,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 连接持续活跃 255s，越过 4 分钟后仍正常完成，未按年龄降级，正向验证 active-stream 不变量。事实层却冲突：Explorer 已找到同一应用进程 Jit thread pool 的 2 个 JIT span（1.781ms 行5969..6114；0.607ms 行12611..12664），通用 Axis-A TOP8 将小 semantic family 挤掉，模型正文误称窗口内没有 JIT；系统附录随后又发布 JIT 2次合计2.388ms。Runner oracle 未比较模型正文与补齐事实，故假绿。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
