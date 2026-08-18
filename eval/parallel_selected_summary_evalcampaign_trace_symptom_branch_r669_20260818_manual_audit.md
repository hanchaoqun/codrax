# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T07:37:32Z
- sweep_start_ts: 20260818-003732
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260818-003732 | answer_regex | none | 135s | 28 | read=2,repo_map=1,list=0,trace=0,source_lens=1 | midloop=4,inv=1/0,fin_reject=2,unavail=0,prune=0 | partial | B1050/B1049 production positive: first patch preserved all four sibling blocks and the complete ordered-list body while adding four typed relations; final patch removed only the unsupported optional diagram. The relation list survived, but its reader labels were contaminated by diagram aliases (`Wr → Fn`), confirming B1053. |
| 1 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260818-003732 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 169s | 35 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | B1052 production positive: target app sleep remains only in the target-state symptom line; the final reader card's actual-occupancy and eliminable rosters contain only on-chain worker-200 and background stays support-only. The model nevertheless copied two raw control tokens and independently overclaimed completion/direct resource blocking despite explicit typed ceilings; classify as model adherence/context-load debt, not authority or calculation regression. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual findings

### Trace

- Exact selected window `1.000000..1.010000`, deterministic supplement, wakeup path `worker-200 -> app-100`, cross-CPU tuple, worker rank #1, measured/effective 9.000/8.300ms, target state partition, causal projection, and adjacent/background separation are all present.
- The last reader decision card no longer places target-self `sleep 10.000ms` in the cause roster. It lists the sleep only as the target symptom, lists worker-200 alone under measured on-chain occupancy and existing-rule eliminable impact, and keeps the two context rows outside root-cause ordinals. This is the production witness that closes B1052.
- The model still emitted `bounded_window_candidate` and `dominant_state=runnable`, then claimed worker-200 “completed work”, “occupied scheduling resources”, and “directly” delayed the target. The same prompt explicitly says no work-completion, synchronous-blocking, direct-blocker, lock-holder, same-CPU, or post-wakeup authority is provided. The system appendix remains within that ceiling and does not rewrite the model conclusion. Do not add a final-prose keyword gate or deterministic conclusion replacement; continue heterogeneous replay and reduce duplicated raw context separately.

### Polyglot relation chain

- First reject correctly requested typed ownership for a principal relation list that declared call/register claims. The sparse `replace_blocks={id, edge_anchors}` repair inherited the previous model-authored kind, role, items, facet claims, and citations; four visible typed relations remained after the optional diagram was removed. This is the missing production witness for B1050 and B1049.
- Second reject was legitimate: the optional sequence diagram asserted self-call, bridge, reply, and helper arrows beyond the exact typed recipe set. The model removed only that optional diagram on the third turn; relation prose/list content was preserved.
- New B1053 is deterministic: `normalizeDiagramEdgeAnchorMetadata` rewrote the standalone list's reader-facing endpoints to the sibling Mermaid aliases solely for validation. When the next patch removed the diagram, `Wr`/`Fn` remained visible. The fix keeps standalone labels immutable and supplies an ephemeral alias-resolved copy only to diagram validation.
