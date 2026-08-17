# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T10:18:17Z
- sweep_start_ts: 20260817-031816
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260817-031817 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 151s | 35 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | B962 生产闭环：route 不再把“调度状态/唤醒关系”合成为图/表权限，显式窗、三次 typed 查询、因果投影和自动补齐均保留。模型正文仍把 `measured_lower_priority_dependency_supply_candidate` 扩写成“持锁直接阻塞/典型优先级反转/实际阻塞 10ms”，而最终 typed handoff 明示 `not_authorized_mechanisms=priority_inversion_occurrence,post_wakeup_delay,lock_or_holder_waiter,synchronous_blocking`。系统投影仍保持 candidate 口径；这是模型证据口径服从观察项，不能靠扫描正文硬拒或系统改写答案。 |
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260817-031818 | answer_regex,answer_contains,mermaid_edge_count | none | 318s | 42 | read=16,repo_map=4,list=0,trace=0,source_lens=0 | midloop=10,inv=3/0,fin_reject=6,unavail=0,prune=1 | fail | Runner 假绿。Router 正确保留逐字 `Mermaid 架构图` 权威，Analyzer 首次也提交 required flow + 六名 incident participant，但该次因缺 `call_chain_endpoints` 被拒；完整重发时漏掉整个 `diagram_hint`，工具却接受。于是参与者覆盖和 required 修补失效，第 3 次修补甚至称图为 OPTIONAL。最终图只有三条局部 typed call，Analyzer/Finalizer/BusContext 等断开，正文却宣称完整四阶段数据流。根因是无关字段修补可丢当前轮硬展示合同，而非 Mermaid 语法或单次模型波动。B963 已改为：一旦 schema-valid required diagram dimension 在场，任何完整重发都必须保留 diagram_hint；不合成边，未证参与者只能以断开节点+typed boundary 呈现。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `B962-PRESENTATIONPROVENANCE1=production-closed-r608`：内容维度不再被 router 越权铸成展示硬门。
- `B963-REQUIREDPRESENTATIONREPAIRLOSS1=implemented/pins-pass/pending-replay`：无关字段重发不能再删除 required `diagram_hint`；JSON 教学同步说明该 carrier 在每次 complete repair 中必须保留。
- `B964-RELATIONSPINEFALSEGREEN1=causal-consequence-of-B963/pending-replay`：当前自动 oracle 只验“有图/有边”，无法识别局部真边冒充完整关系；生产修复先恢复 required participant/boundary 合同，回放后再决定是否需要泛化 evaluator receipt，禁止按本 case 节点名硬编码。
- `B965-TRACEMODELCALIBER1=model-adherence-watch/context-sufficient/no-prose-hard-gate`：typed handoff 已精确禁止 holder/waiter、同步阻塞和已证反转，系统不替模型改结论。
- Trace explicit-window、因果投影、自动补齐、链上-only 主因、实际占时与规则计价双轴均保持；邻近/背景仍为 support-only；未观察 active-stream 4ms 降级。
