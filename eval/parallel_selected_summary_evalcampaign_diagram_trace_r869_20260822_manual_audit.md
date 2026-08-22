# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T17:17:14Z
- sweep_start_ts: 20260822-101713
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260822-101714 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 127s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 精确 2.000000..2.020000s 窗、app-100 四态、threadpool-400→network-300→cookie-200→app-100 唤醒链和逐跳 CPU 全在；11.000ms 链上 IO 第一席、三个独立且不可加的 1.000ms runnable/优先级候选、实际占时/规则可消双账、背景隔离、业务下钻与完整 Trace 因果投影均在。四次 target/window-filtered typed 查询，成文零拒绝；没有固定 4ms/4m、流年龄或上下文比例降级。模型对 fscache 机理的措辞略强，但同段明确披露等待对象/持有者未证，不构成系统硬化。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260822-101714 | answer_regex,answer_contains,mermaid_edge_count | none | 223s | 32 | read=8,repo_map=4,list=0,trace=0,source_lens=1 | midloop=3,inv=3/0,fin_reject=0,unavail=0,prune=0 | partial | B1353 无回归：最终只保留一个 BusContext subgraph，无同 ID node/subgraph 冲突、无第二个隐式 BusContext 节点，Mermaid 合法且成文零拒绝；但用户明确要求的 Mutable/BusContext 与四阶段数据流仍未画出。完成门正确识别 BusContext 缺关系，却两次把精确导航固定到无关 `cgec_enforcers.go:767-791`；模型随后已在 `orchestrator.go:55` 发出同参与者可引用证据，导航仍未切到真实 `BuildAgentContext(o.busCtx, ...)`，连续三次低增量后以 unproven 收口。不是单纯模型波动，确认 B1354 自适应候选预算 gap。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner 2/2 PASS；人工为 Trace pass、read partial。自动 `mermaid_edge_count` 只能证明三条阶段顺序边存在，不能证明显式要求的共享载体数据流已经回答。
- B1353 本轮只能签“生产无回归”：自然回答没有再出现重复 BusContext 身份，但精确 alias-repair 正臂未触发，不能把它误记为生产正证。
- 新确认 `B1354-GROUNDEDSOURCEFRONTIER1/P1`：participant repair 的声明绑定在候选质量比较前先受六文件预算裁剪；即使 Explorer 后来已经在一个被裁文件中提交同一 missing participant 的可引用证据，该文件仍无法重返候选池。修复只让 exact source + citable participant incidence 成为软导航排序信号；关系、方向、节点、标签和结论仍必须由模型读取源码后提交。
