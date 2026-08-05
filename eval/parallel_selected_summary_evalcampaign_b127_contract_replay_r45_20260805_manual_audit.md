# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T19:45:47Z
- sweep_start_ts: 20260805-124545
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260805-124547 | log_regex,answer_regex | none | 50s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Final bytes are exactly `{"ids":["u1","u3"]}` and both required materials are consumed, so B126's executable-carrier/material-floor contract is effective. One avoidable repair remains: the first custom transform writes `from read_text import read_text`; the runner correctly blocks it because `read_text` is a prebound helper, not a module. Confirms `EVAL-B127-PREBOUNDHELPER1`; fix the schema/system lesson instead of weakening import safety or repairing output prose. |
| 2 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260805-124547 | answer_regex,answer_contains | none | 98s | 20 | read=4,repo_map=1,list=0,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | The class and MRO are correct, but the ordered chain cites `runner.py:15` twice, loses the verified `registry.py:31` lookup and `runner.py:17` handle citation, and ends below the typed citation floor. Production parsing retains all decorated `JsonPlugin` bases; the loss occurs later because evidence diversity caps rows sharing source+subject+anchor, and explain+typed-call-chain is routed to QFGeneric so the dynamic-dispatch handoff lesson is absent. Sparse registration emits also omit `subject`, making an otherwise grounded binding invisible to the typed capsule. Confirms `EVAL-B127-RELROSTERCAP1`, `EVAL-B127-CALLFAMILY1`, and `EVAL-B127-REGENDPOINT1`. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch judgement

- Do not fit Python or `JsonPlugin`: preserve distinct typed relationship endpoints for any language that emits multiple inheritance, implements, embedding, or fan-out rows from one declaration.
- Route an explain request carrying a normalized call-chain endpoint profile/`ReqCallChain` through the call-chain family after architecture precedence and before generic single-topic handling. This uses typed IR only and leaves explicit-window Trace/root-cause routing untouched.
- Make `subject` and `object` conditionally required in the provider-visible schema for relationship/registration rows; retain a typed `anchor_symbol` display fallback for older sparse accepted rows. No summary/request/final-answer prose becomes a hard signal.
