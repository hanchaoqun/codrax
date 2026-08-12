# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T02:58:21Z
- sweep_start_ts: 20260811-195820
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | github_issue_dateutil_relativedelta_float_symptom | PASS | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260811-195821 | write_apply,write_patch_oracle | none | 193s | 23 | read=4,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Patch correctly normalizes integral float years/months to int and rejects fractional/non-integer input. Syntax preflight and `python3 -m unittest discover -v` both executed successfully; 4 behavior tests passed. No recovery draft or system-authored answer. |
| 2 | hilog_mixed_arkts_cangjie | PASS | eval/results/hilog_mixed_arkts_cangjie-20260811-195822 | log_attachment,answer_contains | log_triage | 328s | 23 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=4,inv=3/0,fin_reject=2,unavail=0,prune=0 | fail | B595 production positive: first log-triage draft tried an unsupported cause marker and was rejected; second emit preserved both ArkTS and Cangjie errors as peer occurrences. Final model nevertheless promoted 1ms adjacency into an established cross-error root cause despite the peer boundary. This is model overclaim over an accurately typed carrier, not permission to scan/rewrite its prose. Two finalizer rejects came from the three compatible structured-table row conventions; teaching should prefer the unambiguous columns + cells-only form. Runtime was 328s with live model progress and two genuine server 529 retries; no elapsed-time fallback, old-draft recovery, or system answer occurred. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit disposition

- `B595=production-positive`: unsupported log `cause` edges are now rejected before typed context construction.
- `B596=soft-context-follow-up`: publish a compact typed peer-error authority header (`cross_error_relation=unproven`, `observed_scope=peer_error_occurrences_only`) and keep any bridge/propagation statement explicitly hypothetical. This remains soft model guidance; it does not inspect or replace answer prose.
- `B597=json-mind-reduction`: keep all historical structured-table forms compatible, but teach one preferred form only: `columns[]` plus `items[].cells[]`, with `label` and `text` omitted and one cell per column.
- `B560=production-positive@328s`: a live stream remained active beyond four minutes and still returned the model-authored answer. Total elapsed time did not authorize degradation. The two retries were exact provider `529` responses, not a local duration watchdog.
