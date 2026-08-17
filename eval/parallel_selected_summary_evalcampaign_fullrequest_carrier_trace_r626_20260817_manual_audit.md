# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T15:13:01Z
- sweep_start_ts: 20260817-081300
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260817-081301 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 216s | 36 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | uncertain | 因果投影、链上排序和自动补采正确：worker-200 链上有效归因 8.300ms，app-100 自身 runnable 0.020ms，背景压力保持 support-only；但模型正文把 CPU2→CPU1 的跨核唤醒误述为“同一 CPU 占用/直接竞争”。系统末尾 typed 核对已明确否定该机理，因此核心数值通过、模型机理解释失败，不能按 runner PASS 直接签绿。B986 已把同一精确 CPU 拓扑提前带入 finalizer 证据上下文，待 r627 生产回放。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260817-081301 | answer_regex,answer_contains,mermaid_edge_count | none | 339s | 35 | read=9,repo_map=2,list=0,trace=0,source_lens=0 | fail | 第一次 completion 被导航到 `cgec_enforcers.go:767-791`，第二次又到 `contract_check_block.go:3501-3525`；最终只画阶段 precedence 与无关的 `validateEnumerationItemLabelExtractorMatch -> MutableState.EmittedAnswerSymbols`，BusContext/Mutable 对请求阶段的数据流仍为 unproven。根因是字段声明在 `orchestrator.go`，真实 `BuildAgentContext(o.busCtx, AgentExtractor, StageExtract)` 交接在同 owner 的 `extract_work.go`，旧导航只扫描声明文件。B984 生产否证，B985 已扩展为同语言、同静态 owner、同包/目录的跨文件导航且不铸证据，待 r627 生产回放。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case conclusions

- Runner 结果为 2/2 PASS，但人工结论为 `QF=fail`、`Trace=uncertain`；本轮再次证明 Mermaid 边数与答案包含词不能替代关系正确性和机理正确性的人工审计。
- QF 在成文第 1 轮收到模型语义字节后持续生成，30 秒状态仍明确显示“已收到模型语义输出，持续生成中”，随后完成 3 个成文回合；不存在活跃字节流被固定 4ms 降级。固定时限只允许作用于无首字节、字节停顿、取消、deadline、传输或解码终态，不能作用于仍活跃的流。
- 两项修复都不接管模型结论：B985 只给 explorer 精确的下一阅读坐标；B986 只把已经存在的 typed CPU 拓扑和权限边界提前写入 finalizer 上下文。二者都不扫描用户输入或模型正文、不改写最终答案，也不改变显式窗 Trace 因果投影和自动补采。
