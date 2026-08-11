# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T21:51:02Z
- sweep_start_ts: 20260811-145101
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260811-145102 | answer_regex,answer_contains | none | 160s | 28 | read=5,repo_map=2,list=0,trace=0,source_lens=0 | midloop=3,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | B572a/b 明显降 churn（827s→160s；parser call 已投影且旧抄送循环消失），但 analyzer 首次提交精确 source/sink 时把 mode 错写为 discover，被拒后改成空端点 discover_path；这使精确 reachability authority 消失。最终图的虚构反向边被关系门正确拒绝并删除，但正文仍错误宣称 gate.RunWith 调 gate.Run，runner oracle 假绿。立 B574。 |
| 1 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260811-145102 | answer_regex,answer_contains | none | 266s | 30 | read=14,repo_map=1,list=0,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=7,unavail=0,prune=0 | fail | exact provider 在场，但 flowchart 的 `AE[analyzerEvaluator…]` 后又出现 subgraph 裸 `AE`；标签汇总把裸引用误当冲突声明，端点退回 AE，12 条正确 implementer→LoopController 边反复报 unproven，模型最终删光关系。立 B573；runner 仅看名册文字而假绿。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- B571 production positive：post-finalizer 已消费 exact typed relation provider；本轮失败发生在可见 flowchart alias identity 层，不是 provider 再断桥。
- B572a/b production positive：endpoint role 假声明和 already-read parser 手工抄送循环均消失；sequence 用时从 r337 的 827s 降到 160s。
- B573（P1-high）：同一节点的显式标签与后续裸引用被错误判为两个标签。共享节点身份归并应让裸引用继承唯一显式标签；两个不同显式标签继续 fail-closed。不得按语言、类型名或本 case 特判。
- B574（P1-high）：`discover + non-empty source/sink` 是结构化枚举与字段的局部矛盾。当前先硬拒，再诱使模型把两个端点清空为 discover_path，丢掉精确方向门。最小泛化方案是在 normalization 前把该唯一可判定形规范化为 exact；端点仍须通过当前请求 typed provenance，无法证明时继续由既有 normalization 降权，不能从原始 prose 或提及顺序推断。
- 两条活动模型流均由原模型完成。266s 超过四分钟但没有按累计时长触发草稿替换、系统代答或结论降级；该不变量必须继续保持。恢复只可由 transport 断链、真实无进展、调用方取消或独立安全边界触发，并只发布模型已有载体且明确披露。
