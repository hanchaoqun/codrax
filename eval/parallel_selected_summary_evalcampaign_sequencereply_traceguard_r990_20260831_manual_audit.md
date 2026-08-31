# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T20:58:12Z
- sweep_start_ts: 20260831-135811
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260831-135813 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 198s | 41 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=2/2,fin_reject=0,unavail=0,prune=0 | fail | 显式 34579.490..34579.500s 主窗、4 次 typed query、链上根因排序、实际占时/规则可消双账户、业务线索/邻近/背景隔离、自动补齐和最终 `Trace 因果投影` 均完整，未发生固定 4ms/4m 或活动流降级。模型主回答却没有独立回答核心的 VerifyClass 工作关系：typed 行已给出 T7 上 0.285ms、宿主随后直接唤醒目标、semantic-completion/target-wait binding 未证、有效归因 0；模型只在占用表列 span，关系结论留给系统投影。分析器首稿本来把 `runtime_work_relation_requested=true` 选对，但因同一对象多发 `fact_families` 被拒，完整重发时把该独立布尔误改为 false，确认 B1514。 |
| 1 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260831-135813 | primary_answer | none | 344s | 49 | read=6,repo_map=1,list=0,trace=0,source_lens=0 | midloop=13,inv=3/0,fin_reject=10,unavail=0,prune=0 | fail | 首稿 sequence 图包含五条正确源码调用、容量分支和回复，但 presentation-only `visible_label` 与 Mermaid message 的精确字节不一致触发硬拒；叠加独立 relation repair 后共 10 次 reject，模型最终删除整张图才签绿。B1512 的 reply-before-future-call 负臂本轮没有再次出现，因而仍无生产触发正证。最终列表保住调用边，但仍把内存 `rows.add` + `System.out.println` 称为“落库”，尽管 typed context 精确披露终点是控制台；该事实错误继续按模型消费波动观察，不加正文关键词硬门。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual findings

1. `B1512-SEQUENCEREPLYORDER1` 的结构测试与全量套件已通过，但 r990 的模型没有再产生“先 reply、后 invocation”形，因此本轮只能证明普通 sequence/Trace 无回退，不能把未命中的分支冒充生产闭环。
2. 新确认 `B1513-DIAGRAMDISPLAYHARDGATE1/P1`：`visible_label` 与 Mermaid message 都是模型展示面，既不创造 relation authority，也不影响 Mermaid 语法；当前却把二者任意字节差异放进 typed call-edge 硬车道。Java 的 `schedule` 对 `schedule(petId, reason)` 等五处差异因此烧掉修补预算并诱导删图。最优方案是把这类展示一致性检查降为 soft advisory/retry companion；Mermaid 可解析性、typed relation、方向、endpoint identity 仍保持硬门，系统不复制、翻译或选择任何可见措辞。
3. 新确认 `B1514-ANALYSISRETRYINDEPENDENTDECISION1/P1`：分析器首稿只需删除 causal scope 禁止的 `fact_families`，但完整重发允许无关的、已正确的 `runtime_work_relation_requested=true` 漂成 false。该布尔正是激活模型拥有的 runtime-work relation 席位的 typed 决策。下一批先在精确冲突错误中明确“只修冲突字段并保持该独立决策”，并加 true-through-retry 教学 pin；不从请求/答案关键词推断或替模型回填。
4. Trace 系统投影仍是证据补齐面，不替代模型回答。r990 的最终投影完整不等于模型已回答 VerifyClass 子问；人工结论据此保持 fail。
