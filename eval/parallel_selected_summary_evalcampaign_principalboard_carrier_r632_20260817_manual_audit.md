# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T17:41:34Z
- sweep_start_ts: 20260817-104132
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260817-104134 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 202s | 36 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B993 的系统投影已正确把 1.011 扩窗 #2 移到背景，主窗方向小计/代表窗均只剩 worker #1=8.300ms；但模型正文仍把 app-100 0.020ms 写成“根因排名 #2/次要候选”并臆述同 CPU 竞争。确认是最终 typed authority 显著性/上下文负担 GAP，而非投影再次算错。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260817-104134 | answer_regex,answer_contains,mermaid_edge_count | none | 533s | 45 | read=21,repo_map=3,list=0,trace=0,source_lens=1 | midloop=6,inv=9/0,fin_reject=3,unavail=0,prune=1 | fail | B992 已找到真实 `o.busCtx.Mutable` 参数交接；但精确 call 修补提示要求模型提交 grounder 不接受的 dotted anchor，形成确定性重试环。修复后仍暴露更深层 typed component GAP：阶段业务参与者、实现 owner 与 BusContext/Mutable 分组不能在同一关系组件中对齐，终稿只剩阶段顺序和两个断开载体。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

1. `B992-OUTERTYPEDCARRIERSEGMENTNAV1` 获生产正证：Explorer 精确读取并发出了
   `o.busCtx.Mutable -> appendStageOutputEvidenceToMutable` 的 `argument_flow`，证明外层 typed
   carrier segment 可以把导航带到真实交接点；它仍只负责导航，没有系统铸边。
2. 新确认 `B994-CALLREPAIRINPUTGRAMMAR1`：call repair 的 durable obligation 保存完整 canonical
   endpoint 是正确的，但提示也要求模型把完整 dotted callee 同时放进 `anchor_symbol`。grounder 的
   model-input 语法只接受 parser call token，导致模型逐字照做仍落为 `text_reference`，再次重发又被
   dedupe。`4e3bfa59a` 将模型输入修补目标改为 exact tail token，parser 验证后恢复完整 canonical
   callee；关系债仍按完整 endpoint 清账，不降低证据杆。
3. `B993-SELECTEDWINDOWRANKBOARDISOLATION1` 的确定性五面均生产生效：主窗根因名册、方向算术、可消除
   榜、树徽章和代表窗只含 `worker-200 #1 8.300ms`；扩窗的 app-100 0.020ms 保留在背景层。模型仍从
   较早/raw rank 行复制本地 `#2`，说明仅修系统投影不够，最终合成尾部还需要同一 typed 谓词给出紧凑
   的主窗 ordinal roster 与异窗排除名单。
4. `b6388664e` 已增加该尾部 roster：只有选中窗 on-chain 可消除席位可在本次结论使用 `#N`；已知异窗
   rank 行明确为 supporting context 且 selected-window ordinal forbidden。它不扫描或修改模型答案，
   不删除原始证据，也不影响 trace query、投影编译或自动补齐。`agent/types/tool` 全套通过。
5. QF 的剩余 GAP 不是“没有任何关系证据”：stage precedence、Extractor/Finalizer 对 Mutable 的真 call、
   `o.busCtx.Mutable` 参数交接和 `BusContext.Mutable : *MutableState` ownership 均已存在。缺口在 typed
   关系组件无法把业务 stage participant、实现 callable owner 与嵌套 carrier grouping 对齐；继续增加
   source read 只会重复发现局部真边。最优后续必须是 parser/typed owner-or-group bridge，不能把两个
   断开 local operation 强并成一条不存在的 data-flow 边。
6. 两案均再次看到内部枚举进入客户文案（包括 `bounded_window_candidate` /
   `priority_inversion_candidate`）。控制 JSON enum 必须保持稳定；修向仍是缩短并后置 JSON 教学、提供
   客户语言语义别名，不以字符串扫描拒绝或渲染器改写模型正文。
7. 两案活跃字节流均远超 4ms 而正常完成；没有固定 4ms、固定累计年龄或“尚未形成完整答案”触发的
   降级。终止/恢复权仍只属于 caller cancel/deadline、无首字节、byte stall、transport/decode failure。
