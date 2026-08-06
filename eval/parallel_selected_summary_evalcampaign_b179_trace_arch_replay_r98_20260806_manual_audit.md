# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T13:37:51Z
- sweep_start_ts: 20260806-063749
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_d4_demand_vs_supply | PASS | eval/results/real_trace_d4_demand_vs_supply-20260806-063751 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 168s | 38 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式 114.940ms 窗、四态账、四跳唤醒链、根因排序、实际占时/规则可消双轴及完整 Trace 因果投影均保留。统一 `overlapping_members` authority 在 finalizer 前精确给出 #1/#2、23.994/19.041ms、95.156ms overlap、`members_independent=false`、`addition=forbidden` 和 max-only comparison；模型没有再发布 43.035ms 总量，却仍写“23.994 + 19.041 > 10.331”并以“这三条修向合计”据此比较方向，仍违反同一 typed 算术关系。0 JSON/reject/recovery；不能继续堆提示、扫 prose 或由系统改结论，需有界 model-owned typed-relation review。 |
| 2 | qf_architecture | FAIL | eval/results/qf_architecture-20260806-063751 | answer_regex,answer_contains | none | 603s | 38 | read=6,repo_map=20,list=0,trace=0,source_lens=20 | midloop=23,inv=13/0,fin_reject=0,unavail=0,prune=1 | fail | 最终确有答案，runner 因 analyze 角色使用“全面分析”而未命中旧 regex，含 oracle 假阴性；但系统真 gap 更严重：analyzer 把概念 stage/职责的 architecture mechanism 错铸成 `source_inventory_profile(type,constant)`，高置信 profile 获得全仓完备权，强迫 236 type + 261 constant 的无关 census，触发 20 次 lens、13 次 completion、23 次 midloop、两次 explorer dispatch 和 603s。答案主体列出 7 个 read-mode stage，但又误称 `preStages` 注册 multi-repo selector，Mermaid 还把可同时触发的 log/perf pre-stage 画成互斥分支；故不能按 runner 假阴性反向签绿。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
