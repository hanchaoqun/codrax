# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T15:30:56Z
- sweep_start_ts: 20260817-083053
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260817-083056 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 196s | 34 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | B986 生产正臂：finalizer 早期 `Measured wakeup edges` 已携带 `waker_cpu=2; wakee_target_cpu=1; cpu_relation=cross_cpu` 及无同核竞争权限。模型正文正确写成跨 CPU 唤醒，不再宣称同核占用、抢占或直接竞争；同时明确“未建立同步阻塞或锁持有者/等待者关系，反转只是验证候选”。显式窗、worker-200 链上 #1 8.300ms、10.000ms target sleep、两维占用/可消账、Trace 因果投影和背景 support-only 全部保留。`依赖的资源` 一词略宽，但同段立即给出未证边界，暂按模型表述观察项，不以 prose gate 干预。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260817-083056 | answer_regex,answer_contains,mermaid_edge_count | none | 273s | 33 | read=10,repo_map=2,list=0,trace=0,source_lens=0 | midloop=5,inv=5/0,fin_reject=1,unavail=0,prune=0 | fail | B985 未闭环。第一次 completion 仍被导航到 `cgec_enforcers.go`；模型随后靠宽搜索找到 `dispatchStage -> BuildAgentContext(o.busCtx, agentName, stage)` 与 `applyStageOutput -> appendStageOutputEvidenceToMutable(o.busCtx.Mutable, ...)`，正文描述基本正确，但最终 Mermaid 只保留四阶段 precedence，BusContext/Mutable 仍作为断开的 `unproven` 节点。生产解析形揭示嵌套载体：`Mutable` 的 owner 是 `BusContext`，而实际实参使用点属于持有 `BusContext` 字段的 `Orchestrator`；同 owner 一跳不足。B987 已实现一跳静态类型桥接并覆盖真实 `FromEP` 为空的 owner 恢复形，待 r628。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case conclusions

- Runner 仍为 2/2 PASS，人工为 `QF=fail`、`Trace=pass`。关系图 oracle 只看必需词和边数，尚不能证明请求 participant 之间的数据流已经画全；本轮不得把 runner 绿当作 B985 闭环。
- QF 的成文第 1 轮在 30 秒仍明确收到语义输出并持续生成，最终完成两轮成文；Trace 也正常完成。活跃字节流没有、也不得因固定 4ms 年龄阈值降级。
- B986 只前置 typed 事实和权限边界，未生成或替换模型答案；B987 只沿 parser-owned 声明类型链给 explorer 一个 bounded read coordinate，仍须模型读取并发射关系证据。两者均无用户/模型原文关键词硬门。
