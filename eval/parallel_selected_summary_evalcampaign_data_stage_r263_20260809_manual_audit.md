# Selected Eval Manual Audit Scaffold

- date: 2026-08-10T04:40:17Z
- sweep_start_ts: 20260809-214013
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_multifile_reference_projection | FAIL | eval/results/data_multifile_reference_projection-20260809-214017 | log_regex,answer_regex | none | 56s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | Typed state remained `ledger_graph.first_missing=contributions` with sole producer `compute_contributions`, but post-result control bypassed evaluator/continuation planning and deterministically emitted 18 non-producer batches (`normalize/apply/mapping/value_distribution/join`). New artifact aliases changed idempotency keys, so repeated auxiliary work escaped replay detection; terminal had 22 consumed artifacts, 12 resolutions, zero contributions and failed on the same first missing ledger. This disproves B450 production closure and establishes P0 `EVAL-B451-DATAFIRSTPRODUCER1`. |
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260809-214017 | answer_regex,answer_contains,mermaid_edge_count | none | 285s | 31 | read=12,repo_map=2,list=0,trace=0,source_lens=0 | midloop=10,inv=2/0,fin_reject=4,unavail=0,prune=0 | fail | Stage order was correct, but four Finalizer rejects consumed four patches. The accepted diagram removed every explicitly requested `Mutable/BusContext` data-flow edge and retained only stage precedence, so runner edge-count oracles missed requirement loss. Typed `AllMainStages` authority can validate `precedence` anchors once emitted, but the recipe/context did not make that construction easy and ordinary stage-output/BusContext relations lacked a ready typed carrier. Prose also incorrectly says Analyzer output is wholly LLM-derived; the architecture contract says the model classifies while deterministic packages construct TaskGraph/EvidencePlan/hypotheses/quality gate. Record P1 `EVAL-B452-STAGEDIAGRAMAUTH1` and `EVAL-B453-ANALYZERPROVENANCE1`; no prose hard gate is proposed. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human verdict

- Runner result `1/2` overstates quality: after semantic review both cases are human-fail.
- Data is a deterministic control-plane defect, not model variance: the model was never consulted after the first batch because post-result fallback always found another executable auxiliary scaffold.
- QF is not an impossible validator contract: exact `precedence` anchors passed on the fifth attempt. The gap is authority transport/teaching and oracle coverage—an answer can delete the requested BusContext flow and still pass the current case.
- None of these findings affects Trace explicit windows, automatic supplement, causal projection, or the on-chain-only root-cause rule.
