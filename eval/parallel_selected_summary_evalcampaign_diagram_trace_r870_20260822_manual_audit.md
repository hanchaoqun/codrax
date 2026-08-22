# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T17:37:19Z
- sweep_start_ts: 20260822-103717
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260822-103719 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 174s | 36 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 精确 2.000000..2.020000s 窗、四态、四跳唤醒链、逐跳 CPU、11ms 链上 IO 首席、三个独立 1ms 调度/优先级候选、实际占时与规则可消双账户、背景隔离、业务下钻及完整 Trace 因果投影均在；没有固定 4ms/4m/流年龄降级。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260822-103719 | answer_regex,answer_contains,mermaid_edge_count | none | 312s | 38 | read=9,repo_map=6,list=0,trace=0,source_lens=0 | midloop=7,inv=4/0,fin_reject=3,unavail=0,prune=0 | partial | B1354 生产正证：首次缺 BusContext 直接导航到 extract_work.go，随后继续到 builder.go:59 并得到精确 Mutable initializer；但新增 typed 局部关系未改变只按 missing-set 计算的收敛键，系统提前 force-complete。正文已明显改善，最终图仍用两个隐式技术节点且没有把共享载体清楚接入四阶段。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

1. Trace 人工通过。主根因只来自 typed 链上席位；邻近/背景没有被提升为主因。系统自动补齐保留精确窗、链上 IO、调度与算力候选、业务线索和完整因果投影，且 finalizer 零拒绝。
2. B1354 获得生产正证。`source_operation_missing=[BusContext]` 后，导航不再回到词典序靠前但无关的 `cgec_enforcers.go`，而是命中模型已经落证的 `internal/orchestrator/extract_work.go:3-27`；模型由此发出 `o.busCtx -> BuildAgentContext` 的 exact argument flow。下一轮又精确读取 `internal/context/builder.go:47-71`，发出 `Mutable: bus.Mutable` initializer。
3. 新 P1 `B1355-FLOWTYPEDPROGRESSCONVERGENCE1` 不是模型波动。第二次 completion 的缺失集合为 `[Mutable]`；模型随后新增了可引用的 initializer，使最终 typed 分类由 `source_operation_missing` 进到 `local_typed_incident_only`。旧 `flowParticipantCoverageBlockerKey` 只哈希缺失名称集合，故仍把这次真实进展算成连续无进展，并在下一次 close 提前 force-complete。
4. 最优根修是有限 typed 阶段，而不是证据计数或轮数放宽：blocker key 只在缺失集合变化，或同一集合中每个缺失参与者从“无局部操作”单调进到“已有可引用局部操作”时重置一次。搜索坐标、read 次数、无关 evidence、请求/模型/答案 prose 与 Mermaid label 均不参与。进到局部操作阶段只获得一次桥接调查机会；下一次仍无 typed 请求范围连接就按原合同带 `unproven` 收敛。
5. 三次 finalizer reject 均来自真实的结构/关系约束，未发现互相矛盾的合同；不过模型删除不获证的 Mutable→阶段边后，只留下 `busctx -> ctxbuilder` 与 `orchestrator -> mutable` 两个局部技术关系，导致用户想看的跨阶段业务数据流仍不够清楚。B1355 只恢复调查机会，不授权系统代写这条关系。
