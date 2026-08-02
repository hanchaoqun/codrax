# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T08:54:55Z
- sweep_start_ts: 20260802-015454
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h11_cross_direction_overlap | PASS | eval/results/real_trace_h11_cross_direction_overlap-20260802-015455 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 211s | 38 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | All four model trace queries used the explicit 13762.791708..13763.024898 window and the state table used its 233.190ms duration, but the system causal-projection root was narrowed to 13762.795456..13762.795569 (0.113ms) while still carrying full-window durations. That makes the denominator and overlap arithmetic invalid. The model also upgraded neutral frequency/policy evidence to thermal governance, stated an exact 12-factor count despite incomplete enumeration authority, and proposed PI/fscache causal fixes without typed proof. Projection presence is not projection correctness. |
| 2 | read_combo_member_set_closure_scope | PASS | eval/results/read_combo_member_set_closure_scope-20260802-015455 | answer_regex,answer_contains | none | 503s | 39 | read=27,repo_map=15,list=0,trace=0,source_lens=15 | midloop=28,inv=14/0,fin_reject=0,unavail=0,prune=1 | pass | The final answer is model-authored and contains all 11 exact enum types with responsibilities and citations; no system replacement list, blanked descriptions, or `[excluded]` rewrite remains. Minor wording calls a variable a private type. Process health fails badly: an exact one-file request expanded into repo-wide/irrelevant scopes (27 reads, 15 repo maps, 15 source lenses, 503s) because the analyzer emitted both an exact required file and `source_scope_profile=requested_scope:production`; class authority overrode exact-file authority. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
