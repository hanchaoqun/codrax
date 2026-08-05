# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T19:26:56Z
- sweep_start_ts: 20260805-122654
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260805-122656 | log_regex,answer_regex | none | 71s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Final artifact is the exact strict JSON object `{"ids":["u1","u3"]}` with no fence or prose. Process audit still found two avoidable planner repairs hidden by the summary counters: the initial plan did not consume required `instructions.md`; the first repair then emitted `custom_transform` without its required script. Track as generic initial-plan/schema teaching debt, not answer failure. |
| 1 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260805-122656 | answer_regex,answer_contains | none | 190s | 20 | read=4,repo_map=1,list=0,trace=0,source_lens=1 | midloop=9,inv=2/0,fin_reject=2,unavail=0,prune=0 | fail | B125 fixes worked: unnamed sink no longer drives an exact-path gate and the diagram is optional. Final answer nevertheless reverses the runtime MRO (`ValidationMixin` before `TimestampMixin`) and cites unrelated `runner.py:21` for `plugin.handle`. Explorer had later read the complete decorated class, but Finalizer's deterministic relation capsule retained only the earlier line-9 `CsvPlugin` relation: `getConcreteValuesCached` ignored exact read-range expansion. This is a cross-language stale authority-cache gap. The optional diagram also cost two rejects before the model removed it; keep this as soft-guidance/model-variance evidence, not a new prose hard gate. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusion

- `EVAL-B126-RELREADSTALE1` — confirmed, P1: deterministic structural evidence is read-line gated but was cached across later read-range expansion. This can withhold inheritance, embedding, registration-adjacent, or other relation rows from any supported language after a later `read_file`.
- `EVAL-B126-DATAJSONPLAN1` — confirmed, P1: the initial data-plan contract makes the model discover two executable invariants by failure instead of teaching them atomically.
- B125 authority changes materially reduced the Python replay from 469s/13 explorer rounds/5 final rejects to 190s/9/2. The remaining wrong answer is not evidence that the retired exact-sink gate should return.
