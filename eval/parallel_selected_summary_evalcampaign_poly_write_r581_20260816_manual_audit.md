# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T19:22:12Z
- sweep_start_ts: 20260816-122210
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260816-122212 | answer_regex | none | 191s | 27 | read=5,repo_map=3,list=0,trace=0,source_lens=0 | midloop=5,inv=2/0,fin_reject=1,unavail=0,prune=0 | partial | Python fast/fallback 与 Rust wrapper/core/helper 的文字链基本正确；首稿未证 Native→Wrapper 等边被正确拒绝。但 analyzer 把“原生模块名”漂成 source_location，触发冗余第三轮并借错 citation；模型把 `_fastlex` 定义载体发成 mechanism，精确 add_function 注册债未启动，最终图只剩 Python component，标题却称完整 Rust 序列。 |
| 2 | github_issue_gson_lazy_number_symptom | FAIL | eval/results/github_issue_gson_lazy_number_symptom-20260816-122212 | write_apply,write_patch_oracle | none | 224s | 25 | read=7,repo_map=2,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | partial | equals/hashCode 补丁和既有回归面正确，make check 通过；Java runtime 缺失使 target behavior 诚实保持 unverified，不能降杆签绿。系统随后对同一不可用 probe 追加 batch-1-proof-repair 并再次运行，最终 unverified 块重复两次，属于 typed stable-unavailable 去重和展示组合 gap。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch findings

- B925 的结构化 member-set 收据没有获得生产触发：本轮 analyzer 将相同“原生模块名”维度标为 `source_location`，相邻回放中的 typed role 不稳定。
- B926 confirmed：单一普通 `source_location` 维度没有“带有效 file:line citation 的 principal structured item”收据，正文已经给出模块和出处仍被要求补内部维度块；补块又复用了无关 citation。
- B927 confirmed：精确注册 binding 债只从 `EvidenceRegistration` 启动；模型选择了 parser 可定位的定义载体、却标作 `mechanism` 时，`m.add_function(...)` 只成为普通 body call，注册 handoff 不进入 verified component。
- B928 confirmed：post-apply report 已以 typed `verification_status=unavailable`、`runner_missing` 和唯一 Java probe 证明稳定环境不可用，现有 suppression 却只接受 report-level passed，导致 identical verify-only proof batch；最终报告又组合了两份相同 unverified section。
- 本批没有运行 Trace 案例，也没有修改 Trace 查询、投影或自动补齐；既有显式窗和链上-only 权威不作状态外推。
