# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T13:27:22Z
- sweep_start_ts: 20260821-062722
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-062722 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 211s | 41 | read=0,repo_map=0,list=0,trace=12,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | 精确 2.000–2.020s、四节点链、11.000ms 链上 IO 第一席、三个 1.000ms 调度候选、主要占时/规则可消双账、邻近/背景隔离、完整 Trace 因果投影和补采均保留；无固定 4ms/4m 或旧稿降级。模型正文明确披露未建立目标与上游的同步阻塞/资源持有关系，较 r810/r811 收敛；但下钻仍从“跨 CPU 唤醒”扩写 NUMA 延迟，从 fscache 调用位扩写网络/本地缓存选择，均只是待查假设。继续归 B1269/B1271 软引导，不扫描或改写答案。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260821-062722 | answer_regex,answer_contains | none | 904s | 51 | read=43,repo_map=7,list=0,trace=0,source_lens=4 | midloop=20,inv=6/0,fin_reject=3,unavail=0,prune=3 | partial | 正文与四阶段表整体可用、引用充分，三条阶段 precedence 边有 typed anchor；但图最终留下 Orchestrator、runAnalyzePhase、dispatchStage、BusContext/AnalysisIR 等 10 个无连线 technical participant，真正关系只剩 analyze→explorer→extractor→finalizer 三条隐式 participant 边，形成“语法合法但关系奇怪/图文割裂”。B1294 教学在错误后被模型准确消费，却未改善首次提交：第一次仍漏 call_chain_endpoints，第二次仍含无 CURRENT-request provenance 的 Orchestrator，analyzer 5 轮；故 production 未闭环。finalizer 另有一次 JSON action enum 手误和两轮关系清理，关系删除后没有同步清理孤立 participant declaration，确认 B1295。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner 2/2 PASS，人工均为 `partial`；read 904s、43 次 read、20 次 midloop、3 次 finalizer reject，不能按 runner PASS 收账。
- `B1294` 文字教学仅获得 retry-consumption 正证，首次 emit 仍重复原两类错误；状态回退为 `implemented-soft-teaching/production-not-closed`。下一步应用 schema-validated question kind/axis 与逐行 CURRENT-request provenance 做确定性、可审计归一化：非调用链缺失端点补空 inert shape；无来源推断 participant 丢回调查 entity，保留所有真正用户点名身份与现有来源门。
- 新建 `B1295-SEQUENCEORPHANPARTICIPANT1/P1`：关系修复删除无证边后，可见 sequence participant 声明失去全部 incident edge，却没有相邻修复提示要求模型一并删掉非请求、非边界的孤立声明；终图因此合法但误导。最优形是基于解析后的 diagram structure 发布 soft/local cleanup candidate，由模型显式选择删除声明；不得由系统补造边或扫描可见 label/message 判业务语义。
- Trace 全链、根因类型、双轴与自动补采继续作为每批守护面，B1294/B1295 施工不得进入其 query/ranking/projection 路径。
