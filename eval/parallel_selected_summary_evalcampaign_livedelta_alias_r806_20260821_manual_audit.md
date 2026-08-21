# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T09:56:05Z
- sweep_start_ts: 20260821-025604
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-025605 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 197s | 37 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | 系统面完整：显式 2.000..2.020s、四节点唤醒链、11.000ms 链上 IO 第一席、三个独立 1.000ms runnable/优先级候选、实际占时/规则可消双账、背景隔离与 Trace 因果投影均在；活动流未因固定耗时阈值降级。模型仍把 `fscache_page_wait_on_page_bit` 扩写成“文件系统缓存页 IO 完成”，而 typed 证据只证明调用点/IO 等待，后文 caveat 又正确否认具体对象/后端，属于既有 B1269/B1271 软引导项。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260821-025605 | answer_regex,answer_contains | none | 1395s | 79 | read=25,repo_map=3,list=1,trace=0,source_lens=0 | midloop=19,inv=4/0,fin_reject=12,unavail=0,prune=7 | partial | 表格覆盖四阶段输入、输出与 BusContext/Mutable 载体，最终 Mermaid 语法有效并出现三条阶段 precedence；但 12 次 finalizer reject 中 7 次围绕同一新增关系合同，`allowed_additions` 教 `analyzer→explorer`，recipe 归一器却按所选 node dialect 回填另一身份，租约再判 `unlisted_relation_added`。最终靠猜中 `StageAnalyze→StageExplore→StageExtract→StageFinalize` 才通过，图内保留 6 个无边 participant，时序表达偏薄；第 9 轮写入的“patch 无法添加”临时 caveat 在关系已成功添加后仍泄漏到终稿并与可见图矛盾。B1286 已让每次失败继续发布完整 live delta，B1287 未再制造 exact-node/label 漂移；新增 B1288/B1289。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
