# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T16:03:33Z
- sweep_start_ts: 20260819-090331
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h3_iofam_one_seat | FAIL | eval/results/real_trace_h3_iofam_one_seat-20260819-090333 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 380s | 41 | read=7,repo_map=0,list=0,trace=5,source_lens=0 | midloop=6,inv=4/0,fin_reject=0,unavail=0,prune=0 | fail | 查询和 Finalizer 上下文都有 emitted=8/total=198/hidden=190 与 41.329 request·ms 非墙钟不可相加的精确 typed 标尺，终稿却完全漏掉 41.329，并把 6 条 target-owned 可见 witness 写成目标“共发起 6 个请求”。正文还先把 scheduler-marked IO-wait 放进“非墙钟”分组、表格又写“是”，并把 selected-window storage 最大值误称为非墙钟。1.347ms 单请求驻留、4 次 issuer-blocked 并集 ≥4.384ms、S 状态与 D/io_wait=0 的并置正确；有限事实 scope 无因果投影正确。确认 B1172：facet_ids=member_set 只验块标签，不验 producer-owned typed measurement facet 成员覆盖；禁止用答案关键词门或系统代写修复。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260819-090333 | answer_regex,answer_contains,mermaid_edge_count | none | 636s | 50 | read=25,repo_map=2,list=0,trace=0,source_lens=0 | midloop=21,inv=7/0,fin_reject=3,unavail=0,prune=2 | partial | B1167 新正臂命中：模型提交 `o.busCtx -> BuildAgentContext` 后，系统只要求补交 parser-proved `BuildAgentContext result -> ac` assignment，模型自行发证据，系统未铸边。最终 Mermaid 合法，但仍是四阶段 precedence 子图与 `BusContext -> BuildAgentContext -> Mutable` 局部技术子图，二者没有形成用户请求的共享状态业务数据流。导航选中 `extractStageHasRequiredWork` 的预检查调用；`ac` 后续只有成员投影而无 whole-value consumer，真正 `dispatchStage` 的 `agentCtx -> Execute -> output -> applyStageOutput` 未进入证据链。25 reads 虽低于 r734 的 33，Explorer 仍 39 iter，reject 1→3、墙钟 508→636s、上下文 80k→99k；机器只看名词/最小边数而假绿。确认 B1171：同一 callee 多调用点应按 parser-proved downstream whole-value continuation 深度作软导航排序。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工结论与下一批

1. `B1167-DIAGRAMCROSSCOMPONENTDATAFLOW1` 只能记为 implementation-positive / production-partial，不能关闭。新增 assignment companion 的方向正确，但调用点选择仍停在 liveness/precheck helper。
2. `B1171-CALLSITEWHOLEVALUECONTINUATIONRANK1` 为下一批最高杠杆项：在同一 parser call 候选集中，用 assignment result 被后续完整实参消费并继续产生新 result 的有界深度作软排序；仍只返回读取坐标，不制造 EvidenceItem、边、图或结论。需覆盖所有支持语言、歧义 fail-closed、Trace/写隔离。
3. `B1172-RUNTIMEFACETMEMBERSHIPAUTHORITY1` 新确认：运行时“不同口径/逐项对比”请求需要 producer-owned facet ID 清册；模型通过结构化 block/item 引用声明覆盖，validator 只核精确 ID/owner/caliber，不扫描可见 prose。系统不得把清册渲染成答案或替模型补结论。先完成 B1171，再单独设计/施工该 schema 批。
4. Trace 本轮 `final_projection=0` 正确：问题是有限 IO 事实与口径对比，不是根因/唤醒链请求。显式窗、typed 查询、S-state completion-closed IO 等待与 D/io_wait=0 并置均在；失败不能通过强塞因果投影掩盖。
5. 两案均有完整模型答案，无畸形 JSON 恢复或固定 4ms/累计 age 降级；系统没有替换模型答案、关系或结论。
