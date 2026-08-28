# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T14:36:33Z
- sweep_start_ts: 20260828-073631
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260828-073633 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 241s | 40 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | 显式 2.000..2.020s 窗、四跳唤醒链、11.000ms 链上 IO 第一根因、三个各 1.000ms 的优先级反转候选、实际占时/规则可消双账户、业务下钻、邻近/背景隔离、系统自动补采与完整 Trace 因果投影全部保留；成文零拒绝。模型也明确跨 CPU 不证明竞争、优先级候选不证明持锁。但下钻仍把 fscache 等待点扩写成匿名页回写、预读失效、缓存空间不足和下层块设备等具体可能机理，typed 证据只证明等待调用点；后文 caveat 已收窄，按模型推理候选留观，不增加正文关键词硬门。B1380 的等价双写自然形本轮未触发：唯一 patch 是给重复 id 的 section 做 add/replace 合并，不能冒充生产正证。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260828-073633 | answer_regex,answer_contains,mermaid_edge_count,typed_diagram_participant_coverage | none | 427s | 42 | read=22,repo_map=3,list=1,trace=0,source_lens=0 | midloop=9,inv=7/0,fin_reject=1,unavail=1,prune=1 | partial | B1381 生产/eval 正证：accepted receipt 精确记录 required=6、covered=6、unproven_boundaries=2，runner 由同源 typed 收据签绿，不再逼两个未证边界伪造箭头；最终 Mermaid 语法和关系证据门均通过，只用一次局部 label/ref patch 收敛。答案的四阶段职责与 Analyzer→Explorer→Extractor→Finalizer 主链正确，但图被 30 余条 read retry/checkpoint 的方法、len、TrimSpace 和字段级 argument/data-flow 关系淹没，远超用户要的阶段逻辑视图。日志证明系统同时教“不要画全部候选”，又发出包含完整 edge_anchors 的 topology carrier 并要求两数组整份复制，属于 typed authoring 合同粒度自冲突（B1382），不是单纯模型波动。探索首个 probe dispatch 已读 10 个文件、形成 61 条证据并三次低增量才闭环，随后 evidence dispatch 又运行且因一条陈腐 schema-invalid relationship 多次拒绝完成，另立 B1383 审计，不用固定轮数/时长截断。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
