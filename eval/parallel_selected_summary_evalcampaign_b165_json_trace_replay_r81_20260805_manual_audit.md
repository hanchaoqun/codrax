# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T05:35:08Z
- sweep_start_ts: 20260805-223506
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_d4_demand_vs_supply | PASS | eval/results/real_trace_d4_demand_vs_supply-20260805-223508 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 212s | 36 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=0 | fail | 主结论与双轴正确：显式 114.940ms 窗内 sleep=84.358ms、runnable=3.636ms，需求侧等待为主；算力折算 10.331ms 仅次要，Trace 因果投影/窗内可消除量均在。但模型把重叠席位做了无权限加法：23.994+19.041 说成约 43ms，并把 ThreadPoolForeg 10.433/7.386/6.673 说成约 24.5ms。trace_query 已明确 `same_direction_addition=not_authorized_without_exact_typed_subtotal`，该关系未随选中标量传到 Finalizer。 |
| 1 | github_issue_chrono_duration_min_symptom | FAIL | eval/results/github_issue_chrono_duration_min_symptom-20260805-223508 | write_apply,answer_regex | none | 903s | 20 | read=14,repo_map=3,list=0,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | JSON 教学获得正证：两次 emit_change_plan 都是 native arrays，strict decode/carrier/element-shape repair 全为 0。第二次计划保留原 7 行测试逐字并追加三项测试，却被保护测试 critic 因 replace 旧行而误判“削弱”；replan 随后用跨语言 Python 模拟 Rust 算术并在下一次模型响应撞 900s，总终态 plan_not_written/apply_not_run。登记 post-image critic P0；跨语言模拟 probe 作为独立 P1 深审。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
