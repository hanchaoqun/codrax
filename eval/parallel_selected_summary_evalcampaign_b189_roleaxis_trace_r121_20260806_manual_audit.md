# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T19:50:43Z
- sweep_start_ts: 20260806-125041
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_java_handler_impls | FAIL | eval/results/sr_java_handler_impls-20260806-125043 | typed_inventory_rowset,answer_regex,answer_contains | none | 108s | 20 | read=5,repo_map=3,list=0,trace=0,source_lens=2 | midloop=4,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | S20 生产臂命中：`required_candidate_roles=0`，每行 `candidate_role=type`，零 role advisory、零虚假 caveat，关系/表格形保留。三个实现与路径事实正确；严格判 fail 是因模型本轮选择 class definition 行支撑同时展示路径的二轴行，最终引用不证明 `/echo|/stats|/upper`，runner typed rowset 正确失败。系统未改写 citation；与 r118 Rust 合并为通用“definition 与 relation proof 被教学称作等价”问题。 |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260806-125043 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 148s | 31 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 两次 trace_query 均携带精确窗 34579.472865..34579.587805 与 pid=59566；Trace 因果投影、系统补采、根因排序、唤醒链、双轴时间占用/可消除量完整，S20 无回归。但模型再次无视 final typed tail 的 `cross_row_addition=not_authorized` 与 `aggregate_absolute_level_authority=not_provided`：跨席合计 43.035ms/24.5ms，并称 IO“高负载”、CPU“中等偏高”；还引用超窗 print 范围讨论帧标记。按红线记为重复模型/路由合成失配，不扫描正文硬拒、不由系统重写结论。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
