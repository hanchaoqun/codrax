# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T07:18:55Z
- sweep_start_ts: 20260806-001854
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | operation_system_inventory | PASS | eval/results/operation_system_inventory-20260806-001855 | log_regex,answer_regex | none | 40s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Five read-only commands executed once and every reported value matches command output: macOS 26.5.2/25F84, 18 logical and physical CPUs, 137438953472 bytes = 128 GiB, Apple M5 Max 40-core GPU and Metal 4. Full-output artifact is disclosed. Native operation planning arrays passed with zero repair; no answer mutation or hidden partial-material state. |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260806-001855 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 298s | 42 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=1,inv=2/1,fin_reject=0,unavail=0,prune=0 | fail | B168-S1 production replay succeeded: zero-confidence completion prose is absent from Known Facts, and the previous false lock-holder/direct-wait/deadline conclusion is gone. Final prose now says `wakeup_path_blocking_authority=not_implied`, no confirmed holder/waiter and frame causality unproven; deterministic explicit-window projection/two-axis ranking remains complete. Remaining accuracy gaps: a 2.978ms representative occurrence is described as containing the full-window 23.994ms aggregate seat; CPU/IRQ rows are called below a “significant threshold” without a typed threshold; model aggregate_facts falsely asserts `priority_inversion_authority=confirmed_holder_waiter`, which conflicts with typed rows and creates a soft visible-value advisory even though the final model correctly refuses it. Analyzer also spent 4 rejects on an audit-only runtime-question source quote. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch findings

- `EVAL-B168-EMITFACTAUTH1`: production replay closed; the fix removes only zero-authority control prose and preserves structured model exploration plus deterministic Trace projection.
- `EVAL-B169-RUNTIMEQUOTE1` (P1): `runtime_question_profile.source_quote` is unused by downstream consumers but was a hard reject, causing four redundant analyzer rounds. The teaching simultaneously said `emit_analysis exactly once`, contradicting required repair after a rejection.
- `EVAL-B169-AGGAUTHCLAIM1` (P1): a model-authored runtime aggregate can self-assert a deterministic authority dimension that contradicts typed trace rows. The pre-emit aggregate visibility checker then asks the final answer to expose it. This run survived because the advisory was soft and the model followed later typed authority; the carrier boundary still needs a generic authority join.
- `EVAL-B169-AGGJSONMETRIC1` (P2): `aggregate_facts` arrived as a JSON-encoded string and was losslessly recovered, but all current carrier/recovery metrics remained zero.
- `EVAL-B169-OCCWINDOW1` (P2): the final answer still conflates an occurrence window with a full-query aggregate despite the deterministic appendix carrying the correct boundary. Observe across another Trace family before changing soft guidance; no final-prose hard gate.
