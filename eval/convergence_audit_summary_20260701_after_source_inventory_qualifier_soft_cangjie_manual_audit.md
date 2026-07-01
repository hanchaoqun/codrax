# Selected Eval Manual Audit Scaffold

- date: 2026-07-01T06:28:01Z
- sweep_start_ts: 20260701-142801
- total cases: 1
- parallel: 1
- timeout: 900s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260701-142802 | typed_inventory_rowset,dimension_substring,answer_contains | none | 180s | 21 | read=4,repo_map=4,list=0,trace=0,source_lens=4 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | uncertain | The previous unrelated Java supplement pollution is gone after the qualifier-soft slice. The answer table visibly lists the expected 2 extend rows, 2 foreign func rows, and 8 public class rows, but the deterministic rendered count text says `public class 9` / `public class（9）`. Logs show `emit_answer_document` originally emitted 8 rows and a pre-emit structural advisory detected expected_count=8 versus visible_count=9, but the advisory was accepted as soft and the final renderer still surfaced the mismatched count. Commercial correctness is therefore not fully proven: content rows look correct, but visible count/header repair still needs a typed count-normalization or bounded repair slice; runner PASS did not catch that presentation-count mismatch. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
