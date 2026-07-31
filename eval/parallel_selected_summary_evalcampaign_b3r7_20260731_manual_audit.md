# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T11:13:22Z
- sweep_start_ts: 20260731-041322
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_log_current_code_boundary | PASS | eval/results/read_combo_log_current_code_boundary-20260731-041322 | log_attachment,answer_regex | log_triage | 214s | 31 | read=5,repo_map=2,list=0,trace=0,source_lens=0 | midloop=5,inv=1/0,fin_reject=0,unavail=3,prune=0 | fail | S8 权威分层已真实呈现为 observed_evidence / advisory interpretation，S9 后没有物理越界 citation；但答案仍把 render 的阶段 ordinal 4/4 与 finalizerIdenticalErrorStreak=4 合并为同一控制链。S5 producer-chain 指令在 explore/finalize 均发布却被模型忽略，现有结构化完成门只要求若干源码行，未要求每条机制 edge 有 direct call/assignment/parameter-flow proof。 |
| 2 | logtri_oversized | PASS | eval/results/logtri_oversized-20260731-041322 | log_attachment | log_triage | 237s | 30 | read=17,repo_map=0,list=1,trace=0,source_lens=0 | midloop=5,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | runtime principal main.crashy → main.main 保留，但 route 明确 current_source=optional 后，analyzer 仍用 artifact-only quote 铸 current_source_explanation_profile/current-version verdict，触发 17 reads。最终以“当前 checkout 无 crashy”为由判 fixed/风险已消除；decision 只引用 runtime artifact，零匹配不能证明不可验证历史构建已修复。两个 frame item 仍错位引用下一行，C1 持续开放。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
