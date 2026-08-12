# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T03:30:40Z
- sweep_start_ts: 20260811-203039
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260811-203041 | typed_inventory_rowset,dimension_substring,answer_contains | none | 133s | 24 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=1,inv=2/1,fin_reject=0,unavail=0,prune=0 | pass | Typed inventory and final answer contain exactly 2 extend rows, 2 foreign-func rows and 8 unique public-class rows with the requested symbol, file/line and package fields. Animal/Service correctly remain members of public class while retaining sealed/abstract modifiers. The first completion incorrectly added the two modifier-family counts to 8 and claimed 10; the exact member-set cardinality gate rejected it, and the model corrected the same carrier to 8 without deleting evidence. This is a healthy precise correction, not a contradictory contract. No finalizer retry, malformed recovery, diagram repair or answer takeover occurred. |
| 1 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260811-203040 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 157s | 30 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Explicit 7ms window, target five-state account, worker-200→app-100 typed wakeup chain, CPU2→CPU1 topology, VerifyClass business clue, raw-occupancy/rule-eliminable axes, 0.800ms scheduler-supply seat, on-chain-only ranking, background demotion, causal projection and supplement all survive. Unlike r355, no analyzer-authored causal subtopic entered the retained plan. The model calls VerifyClass the strongest bounded-window candidate/direct upstream factor, while the same model answer and system boundary explicitly keep frame causality unproven/absent and do not claim a proven dropped-frame cause. No finalizer reject, malformed recovery, diagram repair, degraded draft or system-authored answer occurred. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit disposition

- `B598-RUNTIMESUBTOPICAUTHORITY1`: production replay is clean. The analyzer happened not to emit a subtopic in this run, so the production witness proves the polluted carrier is absent rather than exercising the drop warning itself; the positive drop arm remains pinned by the targeted test, with five preservation arms for wider runtime shapes.
- Trace retains both requested diagnostic axes: significant on-chain work as a new optimization direction, and rule-priced eliminable scheduling supply. Non-chain pressure remains background only. The model owns the bounded-window conclusion; the deterministic projection supplies facts, ordering and evidence caliber without replacing it.
- Cangjie confirms the source-inventory row set across language-specific declaration modifiers. The one retry was triggered by a typed value/member mismatch and converged by correcting the value, not by deleting a row or weakening the gate.
- Both runs remained under four minutes. Neither run exercised elapsed-time recovery; the independent active-stream regression and earlier 351s/328s production witnesses continue to pin that real model progress cannot be downgraded on elapsed age alone.
