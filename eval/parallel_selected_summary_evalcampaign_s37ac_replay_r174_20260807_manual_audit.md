# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T16:17:54Z
- sweep_start_ts: 20260807-091752
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_cpp_typo | PASS | eval/results/patch_cpp_typo-20260807-091754 | write_plan,write_patch_oracle | none | 93s | 21 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Read-mode call-chain changes did not leak into write mode. The controller produced one micro batch, read the exact C++ source, and emitted a one-line `retrun` to `return` patch plan with compile/behavior acceptance criteria; no unavailable tools, investigation churn, or finalizer repair. |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260807-091754 | answer_regex,answer_contains | none | 597s | 37 | read=12,repo_map=1,list=1,trace=0,source_lens=0 | midloop=16,inv=24/0,fin_reject=1,unavail=0,prune=0 | pass | Final answer is directionally correct: `buildAnalysisIR -> gate.RunWith` and `gate.Run -> gate.RunWith` are separate incoming edges, with an explicit no-directed-path boundary; the list and Mermaid agree. S37ac is production-positive: endpoint repair exposed grep/read_file, `gate.go:134-135` was read, no unavailable tool occurred, and repeated exact blockers never force-completed. Churn remained severe: the first explorer grounded `anchor_symbol=gate.Run` with `subject=gate`, but endpoint existence ignored the qualified anchor whenever Subject was non-empty, causing a second explore attempt and 24 completion attempts. One finalizer rejection was a model JSON block missing `id`, repaired on the next emit. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
