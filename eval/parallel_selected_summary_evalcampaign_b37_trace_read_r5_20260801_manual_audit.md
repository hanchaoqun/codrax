# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T09:36:25Z
- sweep_start_ts: 20260802-023624
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | read_combo_member_set_closure_scope | PASS | eval/results/read_combo_member_set_closure_scope-20260802-023625 | answer_regex,answer_contains | none | 123s | 21 | read=1,repo_map=6,list=0,trace=0,source_lens=6 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | The model-owned table contains all 11 exact types and citations, with no system answer rewrite. However the required opening says “10 types (plus one CompletenessClaim)” and claims the verified range is lines 1–508 with a later extension, contradicting the 11-row exact set and full-file scope. Process improved from 503s to 123s but still ran six lenses, a rejected bounded lens, and a repo-wide lens: this analyzer correctly omitted source_scope_profile for a concrete file, so nil class scope fell back to repo-wide despite an exact high-confidence required file. |
| 1 | real_trace_h11_cross_direction_overlap | PASS | eval/results/real_trace_h11_cross_direction_overlap-20260802-023625 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 246s | 40 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | The deterministic fix worked: projection root is now 13762.791708..13763.024898 (233.190ms), the four-state account balances that window, and the bar says full-scale=233.190ms; no 0.113ms denominator or over-window arithmetic remains. Human answer still fails causal calibration: the model calls a measured frequency-supply fold “thermal governance,” sums overlapping/incompletely enumerated seats into an exact 98.01ms maximum, claims scheduling supply is orthogonal, and presents PI/lock and fscache remediation as causal despite typed candidate/unproven boundaries. This is a repeated model-guidance gap; the system must not rewrite those conclusions. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
