# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T19:06:31Z
- sweep_start_ts: 20260801-120630
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | patch_c_typo | PASS | eval/results/patch_c_typo-20260801-120632 | write_apply,write_patch_oracle,answer_contains | none | 93s | 19 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | plan/apply/verify 与单行 typo patch 一致；新增 diagram authority 仍未影响写模式。 |
| 2 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260801-120632 | answer_regex,answer_contains | none | 458s | 28 | read=8,repo_map=3,list=0,trace=0,source_lens=0 | midloop=9,inv=1/0,fin_reject=12,unavail=0,prune=0 | fail | 空 anchor 绕行已关闭，模型随后构造了正确 star；但 matcher 只认 typed Object 的包限定名，不认同一 grounded call record 的 exact callee AnchorSymbol，导致 12 次误拒和降级。降级恢复 doc 已有 star 图时，旧 rejected diagram attachment 仍以“系统保留内容”回流；`gate.Run` 精确终点仍未裁定为 absent/RunWith 邻近项。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
