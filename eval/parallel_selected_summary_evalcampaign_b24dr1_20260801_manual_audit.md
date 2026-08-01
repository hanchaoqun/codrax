# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T19:25:20Z
- sweep_start_ts: 20260801-122519
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | patch_c_typo | PASS | eval/results/patch_c_typo-20260801-122521 | write_apply,write_patch_oracle,answer_contains | none | 102s | 19 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | plan/apply/verify 完成；applied tree 只有 `main.c` 的 `retrun`→`return` 一行，编译与 diff oracle 均通过。 |
| 2 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260801-122521 | answer_regex,answer_contains | none | 252s | 30 | read=6,repo_map=2,list=0,trace=0,source_lens=0 | midloop=11,inv=4/2,fin_reject=4,unavail=0,prune=0 | fail | 最终只保留一张 evidence-backed star sequence 图，无旧 rejected 图回流；但精确请求终点 `gate.Run` 未被裁定，答案静默改成 `gate.RunWith`。runner 的 substring oracle 把后者误判为前者，故机器 PASS 不代表用户要求满足。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `EVAL-B24-EDGEAUTH1`、`EVAL-B24-CALLEEALIAS1`、`EVAL-B24-ATTACH1` 在真实复放中均已覆盖：首稿伪 sibling chain 被 hard authority 拒绝，最终图为 `buildAnalysisIR -> each grounded callee`，且无旧图附件回流。
- read case 从上一轮 458s / 12 rejects 降到 252s / 4 rejects；剩余拒绝集中在真实的 edge authority 修正，不再是 alias/attachment 重试环。
- `EVAL-B24-ENDPOINT1/P1` 仍是人工失败主因：`required_mechanism_anchors=0`，且 qualified required anchor 的 owner/member 展开允许 `gate.RunWith` 通过共享 owner `gate` 冒充 `gate.Run`。
- `EVAL-B24-KEYSET1/P2` 仍开放：模型主列表 17 项，系统补充清册 19 项，summary 使用 19；这是 key subset 与 complete roster 的 typed scope 差异，不在本批通过答案原文做删减。
