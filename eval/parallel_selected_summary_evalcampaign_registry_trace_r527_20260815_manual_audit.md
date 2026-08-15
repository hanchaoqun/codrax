# Selected Eval Manual Audit Scaffold

- date: 2026-08-15T20:23:21Z
- sweep_start_ts: 20260815-132320
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_relation_subagent_registry | PASS | eval/results/qf_relation_subagent_registry-20260815-132321 | answer_regex,answer_contains | none | 146s | 27 | read=3,repo_map=2,list=0,trace=0,source_lens=1 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 成员、总数、注册调用与 `Name()` 返回值均正确。模型采用同一 item 的 `citation_ref + citation_refs`，说明 B848 的低心智载体已进入生产；但注册调用 citation pool index 0 没有被绑定，最终被正确 prune，注册这一可见事实仍只有正文而无正式引用。不得再增加按本题固定引用数的硬门或提示。B851 的错误 candidate role 与 B852 的泛化 caveat 本轮均未复现。 |
| 2 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260815-132322 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 214s | 29 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass-with-caveat | 显式 5.000–5.007s 窗、四态、CPU2→CPU1 唤醒、0.800ms runnable 调度供给、5.000ms `VerifyClass` 原始业务占用、规则可消除 0、frame/deadline 未证边界和完整「Trace 因果投影」均保留；邻近/背景未晋升主因，B829/B835 双轴语义正确。模型先明确“无法证明实际丢帧”，后又写“此帧的主要瓶颈”，词面略强；同文已将其限定为 selected-window 最强候选，记模型措辞波动，不加答案原文硬门、不由系统改写。中文答案仍夹带 `frame evidence/causal_conclusion/tier/channel` 等内部枚举，属既有展示债。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- B848 获得生产正向采用证据，但仍是 authoring partial：多引用运输合同已被模型消费，未选择的注册锚没有被系统猜补。按既定止损规则停止围绕单一 registry case 叠加教学，转入异构高优先案例。
- B851、B852 本轮未复现，分别降回异构观察与非持续观察；不能据单轮宣称全系统闭环，也不能按 `subagent` 或 caveat 可见文本特判。
- Trace 正对照通过：实际占用/业务修向与规则计价可消除量分账，确定性语义线索未丢，缺 target wait/completion binding 时不获正 effective；显式窗、因果投影和自动补齐未被有限查询/引用改动影响。
- 两案均在持续活跃字节流下跨过 4ms/4s 正常完成；没有固定年龄降级、旧稿恢复、空答案、系统删改模型结论或畸形 JSON 恢复。
