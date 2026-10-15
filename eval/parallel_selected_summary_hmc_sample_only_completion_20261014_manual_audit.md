# HMC244 — sample-only binary and commandless completion manual audit

- date: 2026-10-15T06:33:05Z
- sweep_start_ts: 20261014-233303
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_sample_only_completion_20261014

The runner records typed metrics and declared oracle surfaces only. The verdicts below follow review of actual native payloads, finalizer input, final answers, applied files and persisted workflow/report records. Machine PASS is not manual acceptance.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_binary_sample_only | PASS | eval/results/hmc_sample_only_completion_20261014/trace_binary_sample_only-20261014-233305 | successful_tool_result,trace_attachment,primary_answer | perf_triage+trace_query | 119s | 43 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | Correct native input/counts; wrong CPU scope, sample-kind authority and count/weight/time prose |
| 2 | native_registration_commandless | PASS | eval/results/hmc_sample_only_completion_20261014/native_registration_commandless-20261014-233305 | write_apply,write_patch_oracle,answer_contains | none | 141s | 31 | read=4,repo_map=1,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | PASS within ordinary delivery | Actual plan/source summary and durable 3-result count agree; registration NOT_TRIGGERED |

## Frozen execution and evidence

Both natural cases ran once in parallel against code `0577b8b6cadf24716069ca453db03ce859ada9de`, binary SHA256 `74daa48b879e098aacaa3bcc92a2b1de12e1c4edc2c201e4c0a5a2ee6a0516bb`. No question constraints or fixtures were added to coach these answers. The build was marked dirty only for the batch documentation. Detailed per-case audits, complete native blobs, final MD/HTML/sidecars and original input hashes are archived beneath the results root; the selected summaries and central documentation are the tracked delivery, not the ignored raw archive.

## A — binary path passed, full answer failed

The original `test_brbe.data` is 30,062,585 bytes, SHA256 `da713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632`. Fresh reference-parser counts and the separate production preparation/CLI oracle agree: 12,000 total samples; 1,393 samples in `[486489.4,486489.6)`; thread 1609 / poll_report contributes 164 samples and 259 cycles weight. The actual model used `event_search` then `perf_stats` with the intended thread and window.

The two hotspots each have 82 samples: kernel address `0xffffff800b829d24` has weight 174, and `SensorCallbackProxy::OnDataEvent_` has weight 85. Count shares are 50%/50%; weight shares are 67.18%/32.82%; neither is elapsed time. CPU 0 contributes the callback's 82 samples/85 weight, CPU 1 the kernel's 82/174. CPU 0's omitted zero JSON value is backed by explicit `perf_cpu_known=true` and original records, not an inferred default. All 164 sample kinds remain unknown; there is no scheduler-state/wakeup/blocking authority.

The primary answer and final output nevertheless say all samples were on CPU 0, infer on-CPU rather than off-CPU, treat this as contrary evidence against a stall, and describe 33% as time and 67%/32% as sample counts. The numeric hotspot table is correct; a correct appended quality section does not cancel contradictory prose. The finalizer had complete hotspot count/weight/denominator fields and unknown-kind quality; it did not select the available native presentation selectors. There was no failed sampling query or final diagram, so neither natural retry nor diagram behavior is covered here.

The current investigation synthesis contained these overstatements and remained visible, but it was explicitly labeled advisory rather than verified fact. The event inventory was a partial preview (164 total, 40 retained, 124 omitted; at most 32 shown in that prompt), and tree reference previews were incomplete (82 references with 78 omitted per node). Those precise completeness flags can prevent replacement of that synthesis; they do not prove the sole executed cause. The accepted request profile was bounded_fact_set with no runtime-work/frame-causality flag, but not every in-memory replacement precondition is serialized. Record this as an observed model failure plus a shared handoff/preview-scope improvement candidate, not proven random fluctuation or permission to delete all diagnostic notes. Further system changes need a public causal/cohort/quantity-boundary counterexample first.

## B — ordinary delivery and both fixed handoffs passed

The sole source change is `return value` → `return value + 1`. Both existing test files and `setup.py` are byte-identical across fixture, scratch, worktree and materialized output; scratch HEAD remains the seed. The real native unittest execution ran three methods (negative, zero, positive), with three passed verification-result rows and no independent probe in this run. Root-directory discovery reports `zero_tests`; syntax preflight is not counted as a behavioral test.

The terminal report, current verify attempt, plan and persisted display snapshot agree on three recorded results. The finish message uses those facts; the model reason is preserved separately as advisory. This run's model reason was also accurate, so adversarial-count handling is covered by the batch's public deterministic regression, not claimed as naturally triggered. Restored display is covered by real Store.Save → fresh Store.Load → REPL rendering tests; no live `/workflow show` was performed in this case.

Summary plan `plan-1792046090963382000-85041` is the actual delivered plan, with its one-file patch, existing materialized source preview, exact diff and verify-command provenance. No `run-1.plan.json` mirror was fabricated. Ordinary workflow completion is verified; read-only test registration is **NOT_TRIGGERED**, so this case cannot close that separate branch. The existing internal `proof.probe_count=2` derives from a confidence-entry count, not two executed probes; the verified execution report remains the count source. A discarded model strategy string mentioned pytest, but neither persisted workflow facts nor final verification claimed pytest execution.

There were three analyzer contract retries before the accepted classification; the runner's `rejects` column counts finalizer rejects and does not mean there were no analyzer retries. This efficiency observation stays recorded without adding a third repair track or weakening contracts. Historical HMC243 failures retain their original verdicts.

## Outcome

Machine 2/2, full manual 1/2. Two shared implementation fixes delivered; no capability parent closes solely from these results. Remaining task count stays 58 (21 delivered of 79). The unresolved sampling prose/cohort issue stays under existing HMC-09.4 / HMC-01.3 / HMC-16.4 / HMC-18.3 / HMC-18.4, not a new duplicated stable task. See [central ledger §244](../docs/design/hmosperf_capability_gap_audit_20260917.md) for formal suite, archive and publication receipts.
