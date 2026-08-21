# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T04:24:25Z
- sweep_start_ts: 20260820-212425
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-212425 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 173s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 显式 2.000..2.020s 窗、4 次 typed 查询与自动补采均生效；四跳唤醒链、11.000ms 链上 IO 第一席、三个独立 1.000ms 调度/优先级候选、实际占时/规则可消双账户和 Trace 因果投影完整，背景未升为主因。trace_causal_claim_caliber 只出现在 principal summary，普通运行时行没有 candidate_role 污染，CPU 位置也没有再推成 NUMA/迁核/直接竞争。仍有 P1：模型把调用点名 fscache_page_wait_on_page_bit 释义成“等待文件系统缓存页位图就绪/缓存页位图就绪前”；虽然同段承认对象、后端和直接目标因果未知，仍属于从标识符词面外推语义，B1271 只部分闭环。 |
| 1 | read_combo_pipeline_sequence_table | FAIL | eval/results/read_combo_pipeline_sequence_table-20260820-212425 | answer_regex,answer_contains | none | 1794s | 95 | read=33,repo_map=2,list=0,trace=0,source_lens=0 | midloop=25,inv=5/0,fin_reject=20,unavail=1,prune=0 | fail | 首稿正文、表和 sequenceDiagram 基本回答了问题；B1274 阶段行 authority 未再复现，活动流多次超过 4m 仍因上游字节活性顺延，没有固定短阈值降级。随后关系校验同时产生标签不一致、无证 return/assignment/precedence、正文边缺锚和锚缺正文等异质失败；失败引用没有声明可执行动作/载体，模型把 body-only 失败当 prior-anchor remove、把 relabel 问题当 remove/add，并反复撞 exact-prior-anchor、unlisted relation 与 unchanged-block 冲突。20 次 reject 后只回退展示第一稿并披露恢复说明，runner 因 degraded_answer_checks_skipped 判 FAIL。确认新 P0 B1276：failure_ref 必须携带由同一执行器编译的 allowed_actions 与 target_carrier，禁止让模型猜。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
