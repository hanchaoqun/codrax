# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T04:00:17Z
- sweep_start_ts: 20260821-210015
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_ts_workspace_chain | PASS | eval/results/sr_ts_workspace_chain-20260821-210017 | answer_regex,answer_contains | none | 235s | 27 | read=11,repo_map=5,list=0,trace=0,source_lens=1 | midloop=4,inv=3/0,fin_reject=1,unavail=0,prune=0 | pass | Analyzer 直接把“完整调用链”声明为 `relation_path`，没有附带伪 `member_set`。终稿完整给出 run → ApiClient.fetchUser → HttpTransport.send → HttpTransport.dispatchOnce → fetch，并准确定位 `@app/core` 到 tsconfig.base.json:8。唯一成文拒绝是第一稿仍手填 citation pool index，随后整块替换为 stable `evidence_ids` 并补齐 edge identity；答案事实与关系未缩水。 |
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-210017 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 262s | 36 | read=0,repo_map=0,list=0,trace=9,source_lens=0 | midloop=1,inv=3/0,fin_reject=1,unavail=0,prune=0 | uncertain | B1323/B1324 的核心验收通过：第一稿运行时路径块使用 `relation_path + observed_artifact_fact + external_observation`，没有伪造 `principal_path_edge`/源码 `call`；typed 时序软提示进入成文上下文，终稿不再把睡眠区间写成醒后等待，也没有跨线程时长相加。显式窗、11ms 链上 IO 第一席、三个独立 1ms 调度/优先级候选、实际占时/可消双账户、业务下钻、自动补采和完整 Trace 因果投影均保留。人工仍不能判完全通过：模型把“IO 等待在 irq wakeup 处结束”加强为“完成 IO”，而现有证据不证明具体 completion 机制；并泄漏 `trace_query`、`root_cause_rank`、`priority_inversion_candidate` 等内部词。唯一成文拒绝是缺 principal summary/caliber，模型一轮补齐，raw caliber 未进入可见正文。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case findings

1. `B1323-RELATIONPATHFAMILYAUTH1` 获生产正证：运行时关系路径不再为满足展示合同伪装成源码调用锚；TypeScript 源码路径仍保留逐跳 source call authority。
2. `B1324-TRACEINTERVALSEMANTICSGUIDANCE1` 获生产正证：醒前状态区间、醒后 runnable 起点和跨线程不可直接求和三条语义均进入成文，r838 的倒置/叠加错误未复现。
3. `B1325-REPLACEBLOCKMETADATALOAD1` 获第二语言复现，但强度仍为一次局部结构修补：r838 Rust 与 r839 TypeScript 都先提交手工 citation index，随后才改用 stable evidence ID；需要继续审计 stable evidence handoff 与 block-replacement 教学是否重复或竞争，不能按单个函数名加硬门。
4. Trace 读者语言与调用点机理边界仍有 P1 残余：系统已有 typed reader-safe 事实与软边界，模型仍复制机器枚举并把“等待结束”写成“完成 IO”。后续只能从同一 typed 席位把读者词与负权限更靠近最终 recipe，不能扫描、拒绝或改写模型正文。
