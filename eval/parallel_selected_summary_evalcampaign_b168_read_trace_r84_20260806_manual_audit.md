# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T07:03:33Z
- sweep_start_ts: 20260806-000332
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260806-000333 | answer_regex,answer_contains | none | 123s | 23 | read=2,repo_map=2,list=0,trace=0,source_lens=1 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | B167 production replay closed: Explorer read `internal/types/stage_binding.go` and supplied 4 members / 4 responsibility notes / 4 grounded refs in matching order; every note matches the cited initializer and the old false `dataflow.Analyze` entry claim is absent. Final four identities, order and responsibilities are correct. P2 presentation variance remains: explanation precedes Mermaid although the request asked for explanation after it; observe only, no final-prose hard gate. Finalizer also sent `blocks` as a JSON-encoded string; the existing lossless flat-mode recovery succeeded, but runner metrics reported zero repair/recovery, exposing a telemetry gap rather than an answer loss. |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260806-000333 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 236s | 45 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | fail | Typed Trace execution is rich and correct: all 5 queries are window-bound; root ranking, wakeup chain, target state partition, system two-axis projection, non-additive boundary and enumeration incompleteness are present. Model prose nevertheless overclaims that `ThreadPoolForeg-60555` held the suspend lock and blocked the main thread even while typed context says `holder_authority=not_provided_by_caller`, no confirmed holder/waiter and target_wait_occurrences=0. It also mixes the 34-edge full-window aggregate into a 2.978ms representative occurrence, and treats an outside-window vsync marker as proof of meeting a frame deadline despite absent frame/deadline binding. Root context gap: successful zero-confidence `emit_investigation_complete` control text was injected into Finalizer Known Facts and contradicted later typed authority. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch findings

- `EVAL-B167-MEMBERNOTEAUTH1`: production replay closed.
- `EVAL-B168-EMITFACTAUTH1` (P1, confirmed): a successful orchestration/emit result with typed confidence `0.0` was still projected as `RepoFact` and rendered under Known Facts. This gives model-authored completion prose a false factual channel and can conflict with typed Trace evidence. Fix must use typed tool confidence only; do not inspect request or answer prose.
- `EVAL-B168-JSONMETRIC1` (P2, confirmed): lossless `blocks[]` JSON-string recovery is logged but absent from runner metrics. Add observability before deciding whether JSON teaching needs compression; do not add another repetitive instruction now.
- `EVAL-B168-QFPRESENT1` (P2, observe): diagram/explanation order is a model adherence fluctuation. It did not lose facts and does not justify a raw-output hard gate.
