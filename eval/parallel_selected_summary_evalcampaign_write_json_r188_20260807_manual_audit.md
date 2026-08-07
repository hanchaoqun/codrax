# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T21:45:50Z
- sweep_start_ts: 20260807-144549
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260807-144550 | log_regex,answer_regex | none | 51s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Final bytes are exactly `{"ids":["u1","u3"]}` with source order preserved and no prose/fence. The initial planner already quoted the complete rule material but declared `instructions.md` as `script_consumed` while only reading `users.json`; the precise material guard forced one repair. This is the repeatedly reproduced B298 usage-mode teaching burden, not malformed JSON or an answer-recovery event. |
| 1 | github_issue_nlohmann_long_double | PASS | eval/results/github_issue_nlohmann_long_double-20260807-144550 | write_apply,answer_regex | none | 157s | 21 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=2,prune=0 | pass | Applied tree changes only `%.*lg` to `%.*Lg` in both production headers; the test file is byte-identical. `make check` compiled and ran the C++ test and the report covers both changed paths. Process debt is deterministic: the mode-projected tool schema hid `apply_plan`, while the static controller skill still advertised the full action set, causing one rejected controller decision; planner then spent two unavailable grep attempts after its read surface was withdrawn. B317 addresses the contradictory controller authority; grep misuse remains model/tool-efficiency observation. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

- Runner/human verdict: 2/2 PASS. Neither answer was system-rewritten and no answer-contract/finalizer retry occurred.
- Confirmed `EVAL-B317-WRITECONTROLLERMODEAUTH1=P1`: the JSON schema was correctly projected for `ModePlan`, but static skill prose enumerated `apply_plan`; the same dispatch therefore both offered and rejected that action through different authorities.
- `EVAL-B298-DATATEXTUSAGEMODE1` remains open: exact rule text already present in candidate context still receives a script-consumer-first floor, repeatedly creating a repair solely to read the rule file again.
- C++ fixed-string/regex escape misuse and post-budget grep calls are observable efficiency debt, but the typed edit repair and final verification remained fail-closed; do not add `%.*lg`/C++ special cases.
- Trace was not entered. No change in this batch may touch explicit-window causal projection, automatic supplementation, root ranking, wakeup chains, eliminable-window math, or dual root-cause dimensions.
