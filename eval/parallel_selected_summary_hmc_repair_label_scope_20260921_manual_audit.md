# Scope-repair teaching and IO labels: fixed-version manual audit

- date: 2026-09-21T11:10:58Z
- sweep_start_ts: 20260921-041057
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_repair_label_scope_20260921

Frozen revision `86679d36ba5e`, built `2026-09-21T11:10:28Z`. Runner session62896 formally exited 0; its batch verdict is machine 1/2, not two passing cases. H4's core requested answer passes with an IO disclosure omission retained; business IO fails manual review. No oracle changes, third same-version run, or retroactive closure of earlier failures.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h4_supply_thermal_witness | PASS | eval/results/hmc_repair_label_scope_20260921/real_trace_h4_supply_thermal_witness-20260921-041058 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 170s | 44 | read=2,repo_map=0,list=0,trace=10,source_lens=0 | midloop=2,inv=3/0,fin_reject=1,unavail=0,prune=0 | PASS for core question; P2 disclosure omission | Four states, explicit finite window and policy-limit boundary preserved. Independent IO lower bound supplied but omitted. |
| 1 | trace_query_business_marker_io_chain | FAIL | eval/results/hmc_repair_label_scope_20260921/trace_query_business_marker_io_chain-20260921-041058 | log_regex,typed_trace_projection_count,trace_attachment,primary_answer | perf_triage+trace_query | 240s | 48 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=2,inv=1/0,fin_reject=2,unavail=0,prune=0 | FAIL | Repair hint and IO label fixes hit, but model still mixes request, blocking and business-window measures. |

## 1. H4: finite facts preserved, separate IO measure omitted

The question asks for CPU running time, four scheduler-state durations, and whether a policy limit affected the target in 13762.791708–13763.024898s. The principal answer preserves 233.190ms = 157.248ms Running + 5.604ms runnable + 70.338ms S + 0ms D. CPU4 has a directly recorded 2100000kHz policy maximum, 35.960ms target running and a representative observed frequency of 558000kHz. The answer does not infer binding performance restriction from the maximum or the observed low frequency. The actual fixture's limit row is 17112; do not copy the older case-comment row17113 as current evidence.

The accepted analysis is `bounded_effect_verdict`. Zero causal projections is correct for this finite question, not a lost full-diagnosis capability. `.codrax/output/20260921-041345.927-15575.root-causes.json` exists with schema2, empty root list and reason `trace_root_cause_contract_not_active`.

The final input also supplies a completion-closed IO lower bound: at least four target waits with disjoint S intervals of 1.337/1.238/1.027/0.782ms, union at least 4.384ms. Typed source is `91a8e3cc.json` window_stats; issue/complete/wakeup rows are 9756/9936/9938, 8188/8318/8321, 4033/4158/4160 and 547/600/601. Eight visible IO rows plus190 overflow rows are a result-capacity boundary, not a complete target census or evidence of capture loss. Do not promote 8/198 or request-duration totals into exhaustive target blocking.

Merged final log3032 preserves that fact in its last reader card; earlier teaching says the two measures must be reported separately when present. The answer only reports scheduler-marked D/IO zero. Its nearby “调度器标记” qualification prevents interpreting this as an explicit denial of all IO, and this question does not independently ask for all IO root causes. Thus core answer PASS, with a P2 model-consumption/presentation omission. This does not establish collection or scope failure and does not close broader IO reporting work.

The first finalizer draft submitted `trace_causal_claim_caliber=unproven`, which is not an allowed enum. The schema did not teach it; the model's belief is not evidence of a contradictory contract. Removing it restored delivery. No downgrade, premature active-stream timeout, or causal authority upgrade occurred.

## 2. Business IO: accurate supplied evidence, inaccurate principal answer

Source measurements remain separate: `OpenDocument` 1.000–1.050s is 50ms=5 Running+1 runnable+44 S; broader query1.000–1.051s is 51ms=6+1+44; `LoadDocumentIndex`1.0045–1.0445s is 40ms=8+1+31. Request issue→complete1.005–1.040s is35ms; completion-closed worker sleep1.009010–1.040010s is31ms. Worker/main runnable intervals are separately1ms. Backup's47ms request is background, not an on-chain cause.

The finalizer receives all of these facts: merged log2333–2334 publishes named business50/40ms with their own partitions,2347 separates request35/blocking31 and endpoints,2617 publishes query51. Initial messages survive4→7→10 to the final patch request (2647→2747→2790), prune0. The needed input was not lost.

Manual failures in `run-1.principal.md`:

- Lines25–33 put the51ms query's6+1+44 into the stated50ms business window.
- Line13 calls sleep-entry1.009010 the request issue timestamp; actual issue is1.005.
- Lines41/69 describe31ms as issue-to-complete/request duration and omit35ms, matching machine FAIL.
- Lines1/35/57 infer handoff/communication overhead from44−31=13ms without a mechanism witness.
- “随即/立即” at1/41/51 hides4.99ms between worker wake and main wake; line71 calls the52ms capture50ms.

LoadDocumentIndex/40ms are now retained, but this does not erase other failures. Repeated misuse of adequately supplied measures is a persistent model-consumption problem, not automatically random fluctuation. No answer scanner or system-authored replacement conclusion is added.

## 3. Narrow repairs actually exercised

New analyzer repair guidance is hit at merged log770: the model removes `fact_families` from a causal profile while preserving all seven required dimensions and work=false/frame=false. This proves actual hint delivery and multi-dimension preservation; this run never declares `target_effect_verdict`, so it does not prove the live effect+causal dual-role branch. The latter retains public positive/negative regression evidence, not invented live coverage.

The IO overview at `run-1.answer-transcript.md:134` now says `31.000ms · IO阻塞`, without the unsupported device-latency suffix. Original values, ranking, chain,50/40ms business clues and47ms background survive. Two1ms scheduling seats have typed disjoint intervals and may form their published2ms subtotal; this is not double-counting.

`.codrax/output/20260921-041455.870-15574.root-causes.json` is schema2/available with three on-chain selections, impact_seconds0.031/0.001/0.001 and query scope1.000–1.051. Its model-owned description now correctly says31ms waiting for IO completion. The prior description was not overwritten by a rule; current principal prose remains independently wrong about request duration.

First finalizer call placed summary-only `trace_causal_claim_caliber` in five sections and was rejected; second document was accepted. Final metadata patch repeated that error and was rejected atomically (merged log2819–2824): previous accepted document delivered without partial mutation. Correct placement teaching existed at2552. This is not a newly confirmed impossible contract; retain the JSON-consumption retry observation.

## Disposition

Reference `core/preprocess/sleep_ops.py:198–216,240–255,527–528,558–625` clips each state segment to its own window and follows direct wake evidence. Keep those distinctions; do not copy unknown/interrupt-waker dropping or infer mechanisms from duration remainders. HMC remains13/79 delivered,66 open; native identity teaching/visibility and B2–B6 stay open. Finish exact structural-census owner synchronization and replacement full suite before signing the three narrow repairs; preserve both full-suite failures and all manual observations.

Final follow-up: exact census-owner migration is committed as `7cfd422bf`; replacement full suite `/tmp/hmc-repair-label-final2-full-20260921.log` (session15057) formally exited0:87 tested packages,13 without tests,0FAIL; tool406.290s/agent95.936s/tracequery120.921s. This completes the three narrow code-repair slices without changing these live/manual verdicts. No post-86679 production change or third live run was made.
