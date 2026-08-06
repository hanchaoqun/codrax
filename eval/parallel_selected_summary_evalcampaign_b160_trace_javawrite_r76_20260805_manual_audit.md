# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T03:53:58Z
- sweep_start_ts: 20260805-205356
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260805-205358 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 166s | 40 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=0 | partial | 显式 34579.472865..34579.587805 窗、6 次 typed trace query、根因排序、唤醒链、窗内可消除量、Trace 因果投影、自动补齐及“实际占用/规则可消除”两轴均在。系统披露 frame_evidence_status=absent、causal_conclusion=unproven 且未改写模型结论；但模型仍把 #1 窗内候选称为“本帧根因”，并把同一 ThreadPoolForeg 的 D-state/io_wait/io_latency 重叠方向相加为 24.492ms，违反投影中的 max/不可相加边界。先作为模型越界 witness，不能扫描答案 prose 建硬门。Analyzer 因 runtime_question_profile.source_quote 缺失修复 1 次，属 P2 心智负担。 |
| 2 | github_issue_commons_lang_random_ascii_symptom | PASS | eval/results/github_issue_commons_lang_random_ascii_symptom-20260805-205358 | write_apply,answer_regex | none | 305s | 20 | read=13,repo_map=4,list=0,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | partial | 两轮计划后补丁最终形正确，但首轮测试方法落在 class 外且 fast-path 漏 `end <= 0x7f`；Python regex checker 只抓到后一项。第二轮修正后 `make check` 仍只是 source_static，两个 Java behavior probe 都为 verification_probe_runner_missing，JUnit 方法也无 @Test。最终工件同时出现 proof.status=weak、proof_ledger=low_confidence 与 completion=verified/all_batches_verified，属于确定性 typed 终态矛盾。通用根因是 active-plan localization 未回补到 controller view，加上 static-only + unavailable behavior probe 没有独立终态降级。已进入 B160-S1 修复。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
