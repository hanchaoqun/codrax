# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T06:15:14Z
- sweep_start_ts: 20260811-231513
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_d4_demand_vs_supply | PASS | eval/results/real_trace_d4_demand_vs_supply-20260811-231514 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 130s | 37 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Typed explicit-window Trace surfaces remain complete: target-state partition, wakeup-chain/root-rank, on-chain vs background, occupancy/eliminable axes, positive compute-supply seat, business clue, causal projection, and supplements all survive. The model nevertheless adds two explicitly overlapping seats (23.994 + 19.041) as “about 43ms” and upgrades priority-inversion candidates to proven dependency blocking. The same precise context has passed prior replays, so this remains model variance; no answer-prose scan, hard reject, or system rewrite is authorized. |
| 2 | read_combo_trace_current_source_explanation | PASS | eval/results/read_combo_trace_current_source_explanation-20260811-231514 | trace_attachment,answer_regex | perf_triage+trace_query | 684s | 55 | read=15,repo_map=1,list=0,trace=2,source_lens=0 | midloop=16,inv=13/0,fin_reject=1,unavail=0,prune=0 | fail | B613/B614 production-positive: the model-owned mechanism list is no longer displaced by a system aggregate list, and no projected-enum/schema-description conflict occurs. New deterministic gap B616: the generic completion gate remains at 1/3 because it demands a repository anchor for external runtime and evidence-boundary sub-topics; the model repeatedly re-emits already-grounded lines and restarts from a checkpoint. B617: repair patch lists nested item ids h1/h2/h3/j1 as unchanged block ids and is rejected. B618: selector repair asks the model to print private `.codrax/blob` artifact paths. Final content still overstates H-prefix semantics and does not present the sync correlation/lifecycle/consumer mechanism accurately enough. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
