# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T15:58:07Z
- sweep_start_ts: 20260807-085806
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260807-085807 | log_regex,answer_regex | none | 48s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Final output is exactly `{"ids":["u1","u3"]}`. The first plan declared both materials but consumed only `users.json`; typed `required_material_scheduling` correctly required `instructions.md`, the repaired action consumed both, and the strict JSON-only projection completed without malformed-output recovery or explanatory leakage. |
| 1 | qf_sequence_analyzer_gate | FAIL | eval/results/qf_sequence_analyzer_gate-20260807-085807 | answer_regex,answer_contains | none | 295s | 27 | read=2,repo_map=3,list=0,trace=0,source_lens=0 | midloop=4,inv=5/1,fin_reject=1,unavail=1,prune=0 | fail | S37ab is production-positive: Analyzer emitted `predicate_axis=call` and retained the ordered endpoint profile. Explorer grounded 22 direct sibling calls from `buildAnalysisIR`, but never read `gate.go` or proved the exact `gate.Run` endpoint. The exact reachability gate initially rejected closure, then an endpoint-existence repair requested repo_map/grep/read_file; repair de-duplication retained the older emit-only tool set, so the model's grep was unavailable. Generic contract-chain low-delta convergence force-completed after four unchanged attempts. The final answer falsely calls 22 sibling calls a complete directed path to `gate.Run`, and required-anchor patching mechanically relabels the cited `gate.RunWith` row as `gate.Run`. Runner FAIL and human FAIL. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
