# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T10:36:29Z
- sweep_start_ts: 20260802-033628
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_text_filter_count | PASS | eval/results/data_text_filter_count-20260802-033629 | log_regex,answer_regex | none | 41s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Final literal `2` is correct and both files are eventually consumed, but the first terminal custom transform demoted explicitly named `instructions.md` to optional and executed before result validation forced one repair (`data_rounds=2`, `repair_rounds=1`, `action_failed=1`). |
| 1 | trace_query_binder_ipc_peer | PASS | eval/results/trace_query_binder_ipc_peer-20260802-033629 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 78s | 28 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Target proc/thread, transaction id, and direct waker are correct, but the fixture row says `reply=1` while its scenario claims the emitter issues a synchronous request. Deterministic `call_semantics=reply` was correct; the answer then reverses emitter direction and invents a completed client request/server return story. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual findings

### Data terminal material coverage

The exact user-material floor correctly retained both `instructions.md` and
`notes.txt` at workflow scope, but current-batch scoping demoted the unscheduled
instruction file to optional. Because the initial `custom_transform` also set
`continue_after=false`, it was allowed to execute as if it were the final batch.
Only post-execution result validation restored the workflow obligation and
requested repair. This is an avoidable action failure, not a wrong final literal.

The generic repair point is the pre-execution current-batch contract: a batch
that claims it may produce the final answer must restore every still-uncovered,
explicitly named user material from the workflow contract before the staging
guard runs. Genuine non-terminal batches and already-consumed materials must
remain deferrable.

### Binder emitter direction and fixture authority

The deterministic query did not fail: for the original `reply=1` event it
correctly emitted `call_semantics=reply`. In a native Binder transaction row,
the row's thread is the emitter and `dest_proc`/`dest_thread` are the receiver.
Therefore `reply=1` means that source is sending a reply; it cannot simultaneously
be evidence that the same source is issuing a synchronous request and waiting
for the destination to return.

The fixture's scenario requires a client synchronous request, so the fixture
must use `reply=0 flags=0` and expect `call_semantics=sync_request`. Separately,
both perf pre-triage and final answer guidance should state the general direction
rule. This remains soft semantic guidance: it does not inspect user/output prose,
reject an answer, or replace the model's conclusion.

## GAP decision and task status

- `EVAL-B41-DATATERM1` (P1): terminal current-batch scoping can hide an
  unresolved exact user-material obligation until after execution.
- `EVAL-B41-BINDERSEM1` (P1): model guidance did not make the Binder emitter /
  receiver direction boundary explicit enough.
- `EVAL-B41-EVAL1` (P1): the eval scenario and raw `reply` bit contradicted one
  another, so its prose ground truth demanded an impossible interpretation.

Tasks:

- [x] B41-T1: run the data/trace pair with parallelism exactly 2 and audit full
  logs plus answers;
- [x] B41-T2: restore unresolved explicit user materials only for typed
  final-answer-producing batches; preserve intermediate-batch deferral;
- [x] B41-T3: add generic Binder direction soft guidance at pre-triage and
  finalization, and correct the fixture to a true synchronous request;
- [x] B41-T4: relevant full-package tests and CGO build pass;
- [ ] B41-T5: replay the same pair from the committed fix and manually audit it.
