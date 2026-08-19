# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T08:24:56Z
- sweep_start_ts: 20260819-012454
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260819-012456 | answer_regex | none | 191s | 29 | read=6,repo_map=3,list=0,trace=0,source_lens=0 | midloop=2,inv=2/0,fin_reject=1,unavail=0,prune=0 | partial | B1152 获生产正证：只显示一份合法 sequence diagram，没有 JSON sibling 尾片、重复恢复图或“系统保留内容”协议碎片。六条 typed call 边和 walker 的遍历角色均保留；首轮缺 exact identity、自调用误用 reply 箭头的拒绝正确，patch 后闭合。新发现引用归属断层：第 3 行 `walker::collect_files -> walk` 错引 `src/main.rs:23`，第 5 行 `run -> index_file` 错引 `src/walker.rs:19`；模型按可见行序递增 citation_ref，而 citations[] 是证据顺序。块级 edge_anchors 全部正确，现有 gate 未把每个可见关系行绑定到同一 typed 边。摘要还把分支调用压成 `walk -> index_file` 的线性相邻叙述，图本身则正确从 run 分叉；后者先作软教学观察，不以正文扫描硬门修。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260819-012456 | answer_regex,answer_contains,mermaid_edge_count | none | 400s | 41 | read=7,repo_map=3,list=0,trace=0,source_lens=0 | midloop=9,inv=5/0,fin_reject=3,unavail=0,prune=0 | partial | B1151 获生产正证：导航命中 `extract_work.go:15` 的 `BuildAgentContext(o.busCtx, AgentExtractor, StageExtract)`，模型按 repair 发出 grounded argument_flow；B1150 的 component join 也发布了 BusContext/Mutable/stage 的 typed recipe。终稿仍把四阶段、注册、context building 画成三个断开的子图，只显示 `BusContext -> BuildAgentContext -> bus.Mutable.Objective` 的局部技术路径，没有证明返回值 `ac` 的后续消费者，故业务数据流主体仍不完整。更严重的是前后合同分叉：pre-emit 接受 `Mutable` 的 unproven boundary，post-finalizer 又以 `available_typed_incident_edge_not_rendered` 拒绝同一草稿并耗尽重试；两阶段证据池/请求域口径不一致，属于确定性系统 gap，不是模型波动。正文若干“Analyzer 写子任务/Extractor 写 Mutable”等职责也强于实际逐行证据，未达到人工 pass。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case conclusion

- B1151 已由生产回放确认：parser operation-quality-first 导航确实把探索带到完整 carrier handoff；B1152 也确认恢复图只显示一次且无协议尾片。
- 新记 B1153/P1：principal relation list 的每行 citation 必须与同一块中模型已提交的 exact typed edge anchor 绑定。唯一 edge→evidence location 时可机械纠正 citation_ref；多调用点或别名歧义必须保持 fail-closed。不得按列表序号、答案关键词或 prose 相似度猜引用。
- 新记 B1154/P0：participant coverage 的 pre-emit 与 post-finalizer 必须消费同一 lossless evidence pool 和同一 request-scoped component 判定；不能让同一 boundary 在前门必带、后门必拒。
- 新记 B1155/P1：完整参数传递只是值流第一跳。关系型调查还需继续采集 call result assignment/return/consumer 的 parser-owned typed 路径，允许模型画真实多跳技术路径；系统不得直接合成 BusContext→Extractor 等抽象业务边。
- 两案都未出现 active-stream 4ms/固定总年龄降级，也没有 Trace 路径或模型答案所有权变化。
