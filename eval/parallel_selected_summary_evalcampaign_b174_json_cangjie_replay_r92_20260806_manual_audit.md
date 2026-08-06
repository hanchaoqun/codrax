# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T11:07:02Z
- sweep_start_ts: 20260806-040701
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | cangjie_repomap | FAIL | eval/results/cangjie_repomap-20260806-040703 | typed_inventory_rowset,dimension_substring,answer_contains | none | 157s | 21 | read=8,repo_map=4,list=0,trace=0,source_lens=4 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Final answer contains the exact 2 extend, 2 foreign func, and 8 public class rows with correct file/line/package and 12 matching citations. Runner false-reds because the answer uses one valid combined table after three prose headings: section scoping sees zero rows under the first two headings and all 12 under the last. The existing explicit row-marker oracle needs a mixed-table route. Finalizer also string-wrapped the whole blocks array once; lossless recovery preserved the answer, so this is cognitive-load evidence rather than answer loss. |
| 1 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260806-040703 | log_regex,answer_regex | none | 196s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Final JSON is exactly `{"ids":["u1","u3"]}`. Process remains unhealthy: 7 batches, 3 repair rounds, and 4 failed actions. The user-material floor overwrote the model's valid planner_distilled mode for instructions.md with script_consumed, creating a guard/teaching contradiction; later filter_records carried output_artifact both as the typed top-level field and as an unsupported params key, despite identical values. Strict carrier no longer directly mints ledgers, but the repair escalated into derive/filter/contribution/reconcile after these two deterministic failures. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
