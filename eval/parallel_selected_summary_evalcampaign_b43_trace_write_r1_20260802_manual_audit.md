# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T16:05:45Z
- sweep_start_ts: 20260802-090544
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260802-090545 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 158s | 32 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 系统保留显式 5.000..5.007s 窗、两次 windowed trace_query、因果投影与自动补采；typed 上下文明确分开 5.000ms span 墙钟、4.600ms CPU/规则可消除量、0.800ms runnable，并披露 frame_causality=unproven。模型仍把 5ms 写成全程 CPU 运行并把候选写成已证根因；证据已足，归 model-variance-watch，不加 prose gate/rewrite。 |
| 2 | github_issue_chrono_duration_min_symptom | PASS | eval/results/github_issue_chrono_duration_min_symptom-20260802-090545 | write_apply,answer_regex | none | 391s | 20 | read=11,repo_map=4,list=2,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=2,prune=0 | fail | cargo 缺失，唯一成功的 make check 只是 Python 文本扫描，却被 declared-input roster 升成 Rust project_runner，终态错误签为 verified；生成代码关于 i32 下溢的注释错误，const fn 内格式化 panic 也从未由 Rust 编译器验证。登记 METAAUTH1/P0 与 PROBEBIND1/P1。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Context audit

- Trace：上下文宽度与显式窗问题一致，数值轴、权限边界、唤醒链和自动补采均足以支撑正确回答；系统没有替换模型结论。
- Write：本地 runner 可用性披露准确，但验证 authority 错把“跨语言脚本读取目标文本”当作“目标语言行为被执行”，导致证据账本与真实能力边界矛盾。
- 两项修复都只能消费 typed plan、runner、language family、path、probe identity 与执行结果；禁止读取用户原文或最终答案措辞。
