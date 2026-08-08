# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T09:08:21Z
- sweep_start_ts: 20260808-020820
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_text_filter_count | PASS | eval/results/data_text_filter_count-20260808-020821 | log_regex,answer_regex | none | 36s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | One-round data workflow. `instructions.md` is independently represented as `planner_distilled` with concrete rule notes; `notes.txt` is `script_consumed` and the executable action actually calls `read_text('notes.txt')`. Material/output contracts are satisfied, repair_rounds=0, warnings=0, and the visible answer is exactly the plain number `2`. |
| 1 | patch_python_typo | PASS | eval/results/patch_python_typo-20260808-020821 | write_plan,write_patch_oracle | none | 59s | 20 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | The write analyzer grounds `main.py:20`; the controller selects the mode-valid `plan_batch` action; the plan emits one `kind=patch` replacement from `retrun` to `return` and no unrelated edit. No action-mode conflict, unavailable tool, repair pack, or validation reject occurred. The preliminary classifier still uses broad `explain/mechanism` labels, but emits no read-only inventory/diagram/trace profile and does not alter the write plan; retain B337 as P2 observation rather than hard-rewriting enum labels. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
