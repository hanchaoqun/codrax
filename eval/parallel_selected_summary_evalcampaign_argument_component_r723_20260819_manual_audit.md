# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T07:45:51Z
- sweep_start_ts: 20260819-004550
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260819-004551 | answer_regex | none | 210s | 28 | read=6,repo_map=5,list=0,trace=0,source_lens=0 | midloop=3,inv=3/0,fin_reject=2,unavail=0,prune=0 | partial | 主叙述与八段调用链事实、方向、引用和 walker 的遍历/收集角色正确。首稿把 blocks 整体 JSON 字符串化，数组内仅有 summary/list，diagram 字段意外落到数组外；list 缺 edge_anchors 的拒绝正确。模型第一次 patch 把不存在于 patch base 的 diagram id 当 unchanged，第二次只修 list，最终靠 rejected-draft attachment 保图。系统恢复面随后显示两份同图：一份正确，另一份尾行混入 `\", \"kind\": \"sequence\", \"language\": \"mermaid`，说明 lossy candidate 清洗未识别 kind/language JSON 尾界，canonical hash 也未去重。保留图另含未校验 reply 箭头，虽明确位于非主体“系统保留内容”面，仍不能视为完整合格答案。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260819-004551 | answer_regex,answer_contains,mermaid_edge_count | none | 277s | 34 | read=10,repo_map=2,list=0,trace=0,source_lens=1 | midloop=7,inv=5/0,fin_reject=2,unavail=0,prune=0 | partial | B1150 的 code seam 本轮未被生产触发：模型没有选择含 `o.busCtx` 完整实参的调用点，所以账本无 argument_flow。completion 正确识别 source_operation_missing=[BusContext]，但导航把 analyzer.go 的局部 `BusContext{...}` 构造排在 carrier-as-argument handoff 前；该行是真证据却不能连接用户指定的 BusContext/Mutable 数据流，三次低增量后诚实收敛。终图仅有四阶段 precedence 与 BusContext→Mutable 无箭头 containment，明确披露有向关系未证，仍未完成问题主体。调查虽较 r721 大幅收敛，但 explorer=17/read=10，出现 6/6/7 aggregate 对齐重试和 2 次成文拒绝。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case conclusion

- B1150 的“auto-paired call 不再绕过 argument repair”单元/组件层实现成立，但 production 仍待正见证；当前更早的 blocker 是 carrier handoff 导航排序。
- 新记 P1：关系补证坐标应优先“请求 carrier 的完整实参/赋值/返回操作”，不能被只出现同类型的构造/声明旁支压过。
- 新记 P1：畸形结构化答案中的 recovered diagram 必须先剥离 JSON sibling 尾界，再做 canonical dedupe；同一模型图不能重复、不能把协议尾片放进 Mermaid fence。
- 两案均未发生 active-stream 固定 4ms/总年龄降级；无 Trace 路径改动、无系统代写模型结论。
