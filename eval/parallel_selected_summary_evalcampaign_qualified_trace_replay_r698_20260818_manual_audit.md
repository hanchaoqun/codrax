# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T18:24:23Z
- sweep_start_ts: 20260818-112421
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260818-112423 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 189s | 35 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=0 | partial | 精确窗、链上根因、8.300/9.000ms 双口径与投影均正确；模型正文仍把候选扩写成“持有资源/等待完成/CFS 抢 RT”，与 typed 未证边界冲突。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260818-112423 | answer_regex,answer_contains | none | 307s | 32 | read=8,repo_map=1,list=0,trace=0,source_lens=0 | midloop=9,inv=2/0,fin_reject=3,unavail=0,prune=0 | fail | 无路径结论、两条汇聚边与 Mermaid 均正确；补丁重试丢失用户明确要求的“关键中间函数”清单，端点边界列表却被宽泛 shape receipt 冒名签绿。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工审计结论

### `trace_query_wakeup_causal_runnable`

1. Runner PASS 的核心面可信：查询窗严格保持 `1.000000..1.010000`；目标 `app-100` 的 sleep
   10.000ms 是自身等待症状，唯一主因席来自链上 `worker-200` 优先级反转候选；有效归因
   8.300ms 与链上累计 9.000ms 没有混算。邻近 sleep 0.200ms 和背景调度压力 3.500ms 均未晋升主因，
   `Trace 因果投影`、窗内可消除量与确定性补采完整在场。
2. 人工只能判 partial。模型正文写出“worker-200 持有 app-100 依赖的资源”“等待 worker-200 完成
   work”“CFS 线程抢占 RT 线程调度资源”；同页边界和 typed 指引却明确只有唤醒链+runnable 候选，
   没有 holder/waiter、锁或等待完成证据。系统没有改写这些模型结论，后置投影也诚实披露未证边界。
3. 上下文审计发现一条独立高 ROI 风险：前置 `perf_triage` 在确定性 `trace_query` 之前发布了
   `kind=dependency` 的模型抽取行（“app-100 依赖 worker-200 执行后唤醒”），Finalizer 仍看到该行，
   虽然提示称其只作 navigation。精确查询已覆盖同一窗/线程/机制后，旧模型抽取不应继续与
   deterministic rows 并列成为答案材料；记为 `B1102-PRETRIAGECAUSALSHADOW1`，后续按 typed
   producer/subject/window 覆盖关系收窄，不扫描答案词句，也不删除模型结论。
4. 活跃字节流没有 4ms 固定年龄降级、旧稿恢复或空答案。显式窗、自动补齐、链上-only 主因和
   实际占用/规则可消双轴均正常。

### `qf_sequence_analyzer_gate`

1. 最终结论与图关系正确：不存在 `buildAnalysisIR -> gate.Run` 有向路径；真实边是
   `buildAnalysisIR -> gate.RunWith` 与 `gate.Run -> gate.RunWith` 独立汇聚。Mermaid 保留
   `ParseOutput -> buildAnalysisIR`、`Normalize`、`Compile` 以及两条端点边，方向与源码一致。
2. B1098 获生产正证：Explorer 由 r697 的 30 轮降到 13 轮，日志中不再出现“已读 callable body
   仍需手抄实现证据”的补证要求。B1100 也获生产正证：模型列表出现的关系没有再被 renderer
   从 anchors 复写第二套 bullet。B1099 本轮未再次生成 `risk.Evaluate/hdp.Plan` 两个限定调用，
   因而只保留单测闭环，不能虚报生产闭环。
3. Runner PASS 是假绿。首稿有 11 项“关键中间函数”清单；三轮结构修补后终稿只剩摘要、图和两项
   “有向路径边界”，用户要求的清单完全消失。旧 `member_set` 覆盖收据在只有一个该维度时，允许
   任意有 item 的 list/table 通过，于是无关的 `principal_path_edge` 列表冒充成员清单。
4. `B1101-REQUESTEDMEMBERSETCARRIERALIAS1` 已根修：每个 required member-set 维度都必须由模型在
   真正成员列表/表格上设置隐藏 `facet_ids:["member_set"]`；校验不再用可见标题或任意结构列表推断。
   系统不选择成员、不补写清单、不读取用户原文或最终 prose；可见标题继续使用业务语言。新增生产形
   pin 覆盖“端点边界列表标题恰含关键中间函数仍不可冒名”以及显式 roster carrier 正向形。
5. 三次成文拒绝中，第一轮是 endpoint facet 混入 off-facet calls；第二轮 patch 把数组对象串行化且
   重复 `replace_blocks`；第三轮只补 diagram 后仍缺端点块。它们解释了清单为何被模型 patch 丢失，
   但本批不按该次 JSON 字符串做 case-specific 修复；已有结构化维度收据会在任何 patch 形态下阻止
   这种丢失被签绿。
