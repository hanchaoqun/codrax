# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T11:28:50Z
- sweep_start_ts: 20260731-042850
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_log_current_code_boundary | PASS | eval/results/read_combo_log_current_code_boundary-20260731-042850 | log_attachment,answer_regex | log_triage | 162s | 30 | read=4,repo_map=2,list=0,trace=0,source_lens=1 | midloop=3,inv=1/0,fin_reject=0,unavail=1,prune=0 | fail | 正确解释 4/4 是 read pipeline 的 finalize ordinal，证明前轮错误有模型波动；但又把即时 retry notice 与“重试耗尽后才适用”的 no-visible-output fallback/SkipAnswerChecks 串成必然调用链，并声称系统校验失败对应另一固定文案。真实 source 行存在但 direct producer/control flow 未证明，R1 持续。 |
| 2 | logtri_oversized | PASS | eval/results/logtri_oversized-20260731-042850 | log_attachment | log_triage | 202s | 24 | read=8,repo_map=0,list=0,trace=0,source_lens=0 | midloop=6,inv=3/1,fin_reject=0,unavail=0,prune=0 | fail | 主结论已从错误 fixed 改为 not_enough_evidence，明确不能由 checkout absence 推出历史问题已修；但本轮 route classifier 自身漂为 current_source=required，未直接覆盖 S10 optional 分支并仍读 8 次。frame/item 又把外部 main.crashy 映到当前测试 fixture/同名 main，甚至正文称“位于外部 internal/releaseartifact/cmd/verify/main.go:42”，artifact-local identity 仍错，C1 持续。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
