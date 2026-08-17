# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T19:33:55Z
- sweep_start_ts: 20260817-123353
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260817-123355 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 221s | 33 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 末端 reader mechanism scope 已完整出现，明确 runnable 不是占用 CPU、候选/唤醒次序不证明等待工作完成或直接阻塞；模型仍在首段和时序表写“worker 完成工作前阻塞 app”“等待 worker 响应”。进一步审计发现确定性投影图例仍把 `下钻` 定义成“父行在等什么/子行就是直接原因”，反转图例把低优先级依赖与持有资源阻塞并列，形成系统自身冲突教学，不能归为模型波动。显式窗、链上 #1、因果投影、自动补齐与背景分权均保持。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260817-123355 | answer_regex,answer_contains,mermaid_edge_count | none | 292s | 34 | read=10,repo_map=1,list=0,trace=0,source_lens=0 | midloop=5,inv=3/0,fin_reject=1,unavail=0,prune=0 | partial | candidate carrier 获生产正证：repair 中逐候选带 exact participant node/side、anchor-only technical identity 与“随后进入/调用”等读者标签，模型一次 patch 后形成合法图；相较 r635 的 6 reject/622s 降为 1 reject/292s，终图无 raw `argument_flow`、无重复 BusContext。BusContext/Mutable 的请求级有向数据流本轮未获完整 typed component，图诚实保留无箭头 grouping + 未证边界；但正文仍声称全阶段借 BusContext/Mutable 传递，B999 重复 witness，禁止用 prose 硬门或系统改写。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case decision

- B1002 获生产正证：精确端点映射与自然关系标签降低修补心智和内部词泄漏；关系仍由模型选择，系统未铸边。
- B1003 新确认：Trace deterministic legend 与末端 exact mechanism ceiling 自相矛盾。`下钻` 只能表示沿已发布链向上游展开；除非另有等待/完成或 holder/waiter 关系，不得定义为“父行正在等子行完成/子行是直接阻塞者”。反转候选同理必须分开 runnable 调度供给、running 算力供给与资源持有关系。
- B999 再现：图诚实未证但模型正文仍写端到端共享数据流。先保留为上下文/模型一致性审计项，不以字符串门或 renderer 重写闭环。
- 两案活跃流均远超 4ms 正常结束；无固定年龄降级。
