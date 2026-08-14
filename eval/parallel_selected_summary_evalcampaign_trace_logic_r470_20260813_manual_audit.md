# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T06:13:06Z
- sweep_start_ts: 20260813-231305
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260813-231306 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 159s | 32 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | B766 生产正证：目标线程逐 CPU Running 名册完整，8 核、90 段合计精确回到 157.248ms，模型不再把 top-2 当全量。频率结论也守住“策略上限存在、目标绑定未证”。但 exact target_window_states 已给出 Sleep=70.338ms，正文却近似为 70.3ms/30.2%并称为“fscache 等待”；blocked_reason caller census 只覆盖 16.358ms，不能命名全部 70.338ms 的 S-sleep。Runner 因缺 exact 70.338 失败；这不是 oracle 假阴性。Analyzer 仅重发 2 次，较 r469 的 7 次明显改善，但未命中 B767 canonical_field_target 生产臂，仍不能据此收该臂。 |
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260813-231306 | answer_regex,answer_contains,mermaid_edge_count | none | 369s | 38 | read=11,repo_map=2,list=0,trace=0,source_lens=0 | midloop=10,inv=5/0,fin_reject=1,unavail=0,prune=0 | fail | Runner 假绿。首稿已尝试 BusContext 包含 Mutable、阶段向载体写入以及 BuildAgentContext 关系，但 validator 只接受四阶段 precedence 与一条 local call，要求 BusContext/Mutable 走 unproven boundary；patch 后二者成为断开节点。正文仍宣称二者传递阶段数据，并误称 BuildAgentContext 构建 BusContext，图文不一致。Explorer 读取真实调用点后只发 call row，没有发已支持的 argument_flow；B744 表达/验证已存在但发现/选择仍未闭环。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

1. `B766-TARGETCPURUNNINGROSTER1` 可按 r470 生产正证收账：名册的完整性、未知 CPU 口径和求和均正确，且只作为目标线程状态支持证据，没有晋升为 Trace 根因席。
2. `B767-ANALYZERJSONCONVERGENCE1` 只有间接性能正证（Analyzer emission 7→2）；本轮模型没有形成需要 canonical repair 的同构 tuple，因此该修复臂仍待生产命中。
3. 新登记 `B768-TARGETSTATEEXACTCALIBER1/P1`：当 exact、complete 的 `target_window_states` 已存在时，模型仍以窗口相减近似状态值，并把 blocked_reason 的子集调用点口径外推成整个 S-sleep 机制。修复只能加强 typed context 的权威/子集边界，不得扫描或改写终稿。
4. `B744-CALLARGUMENTCARRIERRELATION1` 维持 production-partial：跨语言 argument_flow 的表达与 validator 已闭环，但 Explorer 在已读取 callsite 上仍只发 call relation。新登记 `B769-RELATIONOPERATIONDERIVATION1/P0`，应从同一已验证 operation source/call row 提供 parser-owned argument/assignment/return 候选，仍由模型选择和引用，系统不画边、不写结论。
5. 两案在 159s/369s 内持续活跃并最终产出结构化答案；未观察到“连接仍有字节、4ms 尚无完整答案便降级”。该路径继续明确禁止。
