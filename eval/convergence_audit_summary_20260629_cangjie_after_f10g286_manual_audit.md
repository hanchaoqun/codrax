# Selected Eval Manual Audit Scaffold

- date: 2026-06-29T09:02:59Z
- sweep_start_ts: 20260629-170259
- total cases: 1
- parallel: 1
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260629-170259 | typed_inventory_rowset,dimension_substring,answer_contains | none | 237s | 31 | read=13,repo_map=2,list=3,trace=0,source_lens=0 | midloop=8,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | Final answer lists both extend blocks, both foreign func declarations, and all eight public class declarations with correct files and package information. Manual audit still found a presentation/projection gap: some package values were rendered in the note column while the package column was blank; this is tracked and fixed by D1-F10g.287 as a row-local typed-attribute projection issue, not as a functional-answer failure. Efficiency remains a P1 concern: wall=237s, read_file=13, midloop=8, and source_lens metric shows 0 despite repo_map use, so future batches should continue cost/telemetry audit. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
