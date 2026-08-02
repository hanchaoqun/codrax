# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T10:52:54Z
- sweep_start_ts: 20260802-035252
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_text_filter_count | PASS | eval/results/data_text_filter_count-20260802-035254 | log_regex,answer_regex | none | 48s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass (final) / fail (process) | Final `2` is correct, but initial plan declared both inputs while its executable script read only `notes.txt`; the incomplete batch ran and failed before repair (`data_rounds=2`, `repair_rounds=1`, `action_failed=1`). The evaluator then hallucinated `(3)` from planner-authored success criteria and proposed blocked; the deterministic completion gate correctly retained the valid typed result. |
| 2 | trace_query_binder_ipc_peer | PASS | eval/results/trace_query_binder_ipc_peer-20260802-035254 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 111s | 30 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=1,inv=3/1,fin_reject=0,unavail=0,prune=0 | fail | Direction semantics are now correct (`reply=0`, `sync_request`, target PID 100/TID 101, tx=42, direct waker `binder:100_1-101`). However, analyzer's correct `bounded_fact_set` was hard-rejected solely because `predicate_axis=call`; retry widened to `relation_analysis`, causing a 25,300-character full causal report, root ranking, eliminable amount, and deterministic supplementation for a three-fact lookup. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual findings

### EVAL-B41-DATAPREFLIGHT1 (P1)

The first B41 material-floor patch fixed the current-batch declaration but not
execution truth. `input_paths=[instructions.md,notes.txt]` was treated as proof
that both were scheduled, even though the script contained only
`read_text("notes.txt")`. The result validator caught the omission after
execution, but that is too late for a terminal batch.

The generalized repair must compare required script-consumed materials with
the executable structured script itself. A conservative Python AST proof over
the data runner's canonical reader helpers can establish literal path use.
Malformed source, aliases, dynamic paths, escaped/computed literals, or an
unknown call shape must remain fail-open to the declared input contract; this
keeps noisy inference out of a hard gate. No user request or answer prose is
scanned.

### EVAL-B41-RELWIDTH1 (P1)

The original analyzer payload accurately declared the request as
`runtime_question_profile.scope=bounded_fact_set` and quoted exactly the three
requested fields. `emit_analysis` rejected it because the same payload had a
typed call/relation shape. The retry was forced to `relation_analysis`, which
then authorized full trace materialization and the assembly-time
`root_cause_rank` / `critical_blocking_calls` supplement.

This is an architecture error: relation shape and answer breadth are
orthogonal. A finite request may ask for one peer, transaction id, and direct
waker without asking for a topology or causal diagnosis. The authority order
should be:

1. explicit user time window (full causal projection remains enabled);
2. explicit typed runtime breadth (`bounded_fact_set` narrows; causal/broad
   relation/overview widens);
3. legacy typed call/relation shape;
4. legacy fallback.

This preserves explicit-window root ranking, wakeup-chain construction,
eliminable-amount calculation, causal projection, and automatic supplementation
while keeping a direct relation fact lookup narrow. It does not inspect prose
or mutate the model answer.

### EVAL-B41-DATAEVAL1 (P2 watch)

The repaired plan invented `count ... (3)` in `success_criteria` while the
deterministic result was `2`. The evaluator treated that planner-authored text
as ground truth and returned blocked after mentally recounting compact
previews. The deterministic completion gate normalized the workflow to
complete, which is the correct control-plane decision because material
coverage and the executed result were valid. Add only soft evaluator guidance:
planner goals/criteria are workflow intent, not independent value authority;
user material, typed metrics, ledgers, reconcile reports, and executed results
carry value authority.

## Batch decision

- Implement `DATAPREFLIGHT1` with precise AST consumption proof and fail-open
  fallback.
- Implement `RELWIDTH1` by separating typed breadth from relation shape and
  keeping explicit windows first.
- Add `DATAEVAL1` soft guidance; keep the deterministic completion gate.
- Run relevant full package tests and build, commit/push, then replay the same
  pair strictly in parallel two for closure.
