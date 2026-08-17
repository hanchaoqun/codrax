# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T10:55:09Z
- sweep_start_ts: 20260817-035508
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260817-035510 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 273s | 35 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=0 | partial | 显式窗、3 次 typed 查询、Trace 因果投影、窗内可消除量、链上/邻近/背景分区均完整；但模型仍把候选写成“主要阻塞原因/典型优先级反转/低优先级线程阻塞高优先级线程”，超过无 holder/waiter 证据的 typed 机理上限。新增 B967：principal summary 把 JSON 控制枚举 `bounded_window_candidate` 逐字泄漏给中文用户。首稿还把该字段错误复制到 3 个非 summary block，结构门正确拒绝一次。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260817-035509 | answer_regex,answer_contains,mermaid_edge_count | none | 590s | 45 | read=24,repo_map=6,list=0,trace=0,source_lens=0 | midloop=14,inv=5/0,fin_reject=2,unavail=0,prune=7 | partial | B966 首版生产未闭环：第二个 direct-read 仍选到 `answer_document_evaluator.go` 的无关 BusContext helper。代码审计确认 carrierRank 在所有普通 parser relation 之前，导致真正同时触达 extractor+Mutable 的 receiver operation 被压后。第二个 explorer 批最终读到 41 条证据；终图仅给 stage precedence，Mutable/BusContext 可见断开且 unproven，视觉边界诚实。但正文继续宣称完整跨阶段字段流，图还使用大量内部函数/node id 及若干隐式节点，业务表达与证据服从仍 partial。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `B966-TYPEDCARRIERNAVIGATIONOWNER1=production-failed-r610/revised`：首版只在 carrier 候选之间比较 owner，无法让普通 parser relation 超过 carrier。修向升级为 operation-level `participantTouchRank`：先比较一个真实 operation 命中多少个独立请求 participant，再比较 carrier/handoff/match；仍只排序下一段软补读。
- `B967-CONTROLMETADATALEAK1=confirmed`：`trace_causal_claim_caliber` 的 enum 是 JSON 控制字段，不是客户术语。通用 JSON 教学应明确所有 field/enum literal 只放结构字段，正文/标题/表/图用当前答案语言表达含义；不扫描、删除、翻译或改写模型正文。
- `B965-MODELBOUNDARYADHERENCE1=observed-again`：Trace 与 QF 的最终上下文均包含精确 unproven/authority 边界，模型仍在正文越权。继续作为模型服从观察，不用关键词硬门或系统代写结论。
- Trace 显式窗因果投影、自动补齐、链上-only 主因候选、邻近/背景隔离和 4ms 活跃流禁降级均未受影响。
