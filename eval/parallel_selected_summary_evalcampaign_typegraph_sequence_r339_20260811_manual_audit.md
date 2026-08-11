# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T22:09:24Z
- sweep_start_ts: 20260811-150922
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260811-150924 | answer_regex,answer_contains | none | 245s | 28 | read=3,repo_map=0,list=0,trace=0,source_lens=0 | midloop=12,inv=4/0,fin_reject=2,unavail=0,prune=0 | fail | B574 生效：analyzer 首次即保留 exact buildAnalysisIR→gate.Run，completion 正确触发并接受 no_directed_path。错误的 RunWith→Run 图边也被 typed relation gate 拒绝。但最终 prose/list 仍说 RunWith 内部调用 Run，实际源码是 Run→RunWith；已读 parser 反向 wrapper 边没有投影给 finalizer，立 B575。 |
| 1 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260811-150924 | answer_regex,answer_contains | none | 248s | 28 | read=14,repo_map=2,list=0,trace=0,source_lens=0 | midloop=7,inv=2/0,fin_reject=1,unavail=0,prune=0 | pass | 12 个 production implementer 名册、文件位置和 implementer→LoopController 图边均保留；B573 生效，未再出现 alias(AE) unproven 风暴。首稿把 class relation operator 写进 flowchart 而被拒，模型一次 patch 改为有锚的 flow relation 后通过；记录 P2 syntax-family 教学观察，不以 case 硬修。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- B573 production positive：flow alias 后续裸引用不再覆盖显式身份，类型关系案从 r338 的 7 次 reject/无边降到 1 次 reject/12 条完整边。
- B574 production positive：sequence analyzer 直接发射 exact ordered endpoints，`no_directed_path` 精确 gate 已实际运行；不存在 discover_path 逃逸。
- B575（P1-high）：`no_directed_path` 的 endpoint-existence gate 接受了边界，但已读 `gate.Run` 函数体中的 AST call `gate.Run -> gate.RunWith` 没有进入 finalizer typed evidence。现有 parser handoff 只接 principal member-set 内的双端关系；边界 endpoint 与 roster 邻点之间的反向/平行解释边漏接。最优方案是在 active typed no_directed_path 下，仅投影“exact requested endpoint ↔ principal member-set 唯一节点”的已读 AST/Cangjie call；regex/unread/任意邻点继续不投影。模型仍决定如何解释和画图。
- 两路均约 4m 后由原模型完成，无固定时长降级、系统代答或删答。
