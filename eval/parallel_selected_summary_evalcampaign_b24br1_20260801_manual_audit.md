# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T18:46:50Z
- sweep_start_ts: 20260801-114648
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | patch_c_typo | PASS | eval/results/patch_c_typo-20260801-114650 | write_apply,write_patch_oracle,answer_contains | none | 119s | 19 | read=2,repo_map=1,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | plan/apply/verify 与单行 typo patch 一致，无 read-mode hard-authority 回归。 |
| 2 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260801-114650 | answer_regex,answer_contains | none | 295s | 29 | read=4,repo_map=2,list=0,trace=0,source_lens=0 | midloop=7,inv=1/0,fin_reject=6,unavail=0,prune=0 | fail | `diagram_hint=sequence` 已正确；硬门 6 次拒绝并促使模型识别 sibling calls 不是链。但 `(analyzer.go:1820)` 同行后缀使正确 star 边误拒，模型最终清空 `edge_anchors` 绕过硬门，系统又把首个被拒旧图作为保留附件追加；最终仍含两张伪串联图且以 `gate.RunWith` 替代精确终点。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
