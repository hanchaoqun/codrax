# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T22:20:34Z
- sweep_start_ts: 20260811-152032
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260811-152034 | answer_regex,answer_contains | none | 172s | 26 | read=16,repo_map=2,list=0,trace=0,source_lens=0 | midloop=6,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | 12 个 production implementer、文件位置和 implements 方向 classDiagram 首稿即通过，零 finalizer reject；B573 回归稳定。 |
| 2 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260811-152034 | answer_regex,answer_contains | none | 248s | 29 | read=4,repo_map=1,list=0,trace=0,source_lens=0 | midloop=11,inv=5/0,fin_reject=1,unavail=0,prune=0 | fail | B575 正证：finalizer context 与正文均正确保留 buildAnalysisIR→gate.RunWith、gate.Run→gate.RunWith 和 buildAnalysisIR→gate.Run 不可达。但图把同一 RunWith 画成 n9=gate.RunWith 与 n11=RunWith 两个 participant，文字“共享汇合点”与视觉拓扑不一致；立 B576 typed alias-family duplicate identity。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- B575 production positive：正确 wrapper 方向已进入 typed topology、finalizer prompt、正文和图边，r339 的反向 prose 错误消失。
- B576（P1-high）：既有 `duplicate_participant_identity` 只比较完全相同的 endpoint 字符串。一个 citable call row 同时携带 `Object=gate.RunWith` 与 `AnchorSymbol=RunWith`，已精确证明两者是同一 callee identity family；但重复检测未消费该同源别名，所以 n9/n11 分裂未被拦截。最优方案是从每条 typed call evidence 构造 endpoint alias→canonical family，只有 alias 唯一落入一个 canonical endpoint 时合并；同一短名映射多个 owner 时继续 fail-closed，不从标签/请求/prose 猜 owner。
- 类型关系 172s、时序 248s 均由原模型完成；后者超过四分钟无时长降级。
