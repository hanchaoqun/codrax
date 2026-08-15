# Selected Eval Manual Audit Scaffold

- date: 2026-08-15T21:53:38Z
- sweep_start_ts: 20260815-145337
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260815-145339 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 211s | 47 | read=0,repo_map=0,list=0,trace=8,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=1 | fail | Trace projection and automatic supplement are present and correctly keep root seats on-chain, but the model prose adds overlapping same-direction seats (10.433+7.386+6.673=24.492ms) and then promotes that invented subtotal over typed rank #1=23.994ms. It also says all 36 target wakeups traversed one full upstream chain although the complete typed target census is 34+1+1 direct wakers. Both exact authorities were already in the pre-finalize context, so this is model non-compliance, not missing system evidence; do not hard-scan or rewrite the answer. |
| 2 | read_combo_pipeline_sequence_table | FAIL | eval/results/read_combo_pipeline_sequence_table-20260815-145339 | answer_regex,answer_contains | none | 283s | 42 | read=16,repo_map=3,list=0,trace=0,source_lens=0 | midloop=6,inv=2/0,fin_reject=2,unavail=0,prune=3 | partial | Prose and the four-row input/output/carrier table cover Analyze→Explore→Extract→Finalize. The final required sequence diagram contains only the Analyze call/data-flow slice and explicitly omits the other stages because the checkout-verified stage-precedence provider did not activate. Analyzer emitted a required sequence with an empty participant slate and its stage/workflow dimension was dropped after a one-character unanchored quote drift (`每 stage` vs `每个 stage`); consequently all four grounded stages were known but the shared precedence authority never reached prompt/validator. Runner's `missing:finalizer` is also a narrow literal oracle because the answer uses StageFinalize/finalize, but the diagram incompleteness remains a real system gap. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusions

### Trace case

- The deterministic `Trace 因果投影` is present. Its crown is the typed on-chain #1 seat, and its elimination overview explicitly states both same-direction overlap and `合计不可直加`; adjacent/background rows remain support-only.
- Finalizer context already contained the compact ordered rank roster, per-direction leader/subtotal authority, cross-row addition prohibition, and complete target wakeup census (`36 = 34 + 1 + 1`). The model nevertheless invented two subtotals and one exhaustive full-path statement. This replay therefore does not justify another prose keyword hard gate, system-authored conclusion, or system answer mutation.
- Human verdict is fail because those mistakes change prioritization and relation scope even though the runner's surface oracle passes. Keep as a heterogeneous model-adherence witness; only promote to a system batch if later models fail despite the same concise typed authority often enough to show a context-layout class issue.

### Read combo case

- This is not merely a `finalizer` spelling failure. The answer has useful prose and a complete four-stage table, but the explicitly requested sequence view is structurally incomplete.
- Checkout-verified read-stage precedence support already exists and proves exactly `Analyze → Explore → Extract → Finalize`. It did not activate because both existing relevance arms depend on either a surviving `stage_or_workflow` dimension or typed diagram participants. The analyzer supplied neither after its source-quote drift, although exploration grounded the canonical endpoint/stage evidence.
- Generalized fix direction: add a third shared admission arm based on two-or-more unambiguous, citable, grounded canonical stage identities from the verified read-pipeline authority sources. Select only their contiguous canonical span and continue to authorize `precedence` only. Do not inspect request/final prose, infer calls or data flow, author the diagram, or weaken unrelated diagram validation.

### Stream/recovery boundary

Both cases received active semantic stream bytes and completed normally. There was no fixed-age 4ms/4s/4m degradation, malformed-JSON salvage, stale-draft fallback, or system replacement of the model answer.
