# Selected Eval Manual Audit

- date: 2026-09-18T08:39:02Z
- sweep_start_ts: 20260918-013901
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results_hmc_window_stats_20260918

Reviewed final Markdown, complete run logs, typed query payloads, raw scheduler/IO rows, and root-cause sidecars. Binary revision: `9a5c95467`. This is a repair-behavior replay, not a matched baseline A/B or an explicit-false live test. Original machine failures remain unchanged; no third run was used to seek green.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h2_dstate_dma_fence_triform | FAIL | eval/results_hmc_window_stats_20260918/real_trace_h2_dstate_dma_fence_triform-20260918-013903 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 149s | 40 | read=2,repo_map=0,list=0,trace=2,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=1,prune=0 | fail | D-segment count/duration preserved, but unsupported GPU/HAL/IO-exclusion and uniform-spacing claims; system also labels the unmarked D partition as non-IO. |
| 2 | real_trace_h3_iofam_one_seat | FAIL | eval/results_hmc_window_stats_20260918/real_trace_h3_iofam_one_seat-20260918-013903 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 258s | 50 | read=7,repo_map=0,list=0,trace=14,source_lens=0 | midloop=3,inv=3/0,fin_reject=0,unavail=1,prune=3 | fail | Core S-state IO facts preserved; completion mistaken for wakeup on unproved rows, displayed rows look like a census, internal labels leak, layer measurements called sampling windows. |

## Evidence and findings

- H2 answer: `.codrax/output/20260918-014130.455-82989.md`, SHA256 `2b9eebd6a6e6784b7ac31f3673bfcb7465874e854b8e03a71e5ed3b3fdb73da0`.
- H3 answer: `.codrax/output/20260918-014319.125-82990.md`, SHA256 `5d56bf81c6ab9adfa68bfec27b2a246b70295f3c62ab32ab95c046362d6edd52`.
- Both matching `.root-causes.json` files: schema 2, empty roots, `trace_root_cause_contract_not_active`; SHA256 `5f8063f5882d6f6c5da9ef59d8ba0282f0fb2ef8c728bdcc77ef0249d2287e39`. These are bounded factual requests; zero whole-root projections and inactive root-selection contracts are correct, not missing-answer regressions.
- H2 machine failure is a narrow wording mismatch (`内核等待调用点` vs its older regex), but human failure is substantive: `devhost.elf` is asserted to be Android HAL, a call-site name is promoted to GPU fence/command causality, and `iowait=0` is used to exclude IO. Finalizer context already said the call-site has no mechanism authority and zero marking does not exclude IO. The system's own `非 IO D-state` state-account label is nevertheless misleading and must be audited at its typed producer, not corrected by scanning answer prose.
- H2 typed facts: 11 D intervals / 36.757 ms; 12 blocked-reason records / Σ39.157 ms are a separate ruler; 1.356 ms remains unaccounted. Final answer retains these core distinctions. Early exploration confused emitter PID with payload target and record counts; later typed recap corrected the core count, but did not prevent unsupported mechanism claims. No source tools were needed or used.
- H3 raw example: issue L9756 at 13762.872568 → complete L9936 at 13762.873915 = 1.347 ms request residence. S switch-out L9758 at 13762.872586 → wakeup L9938 at 13762.873923 = 1.337 ms target blocking. The answer preserves four completion-closed S waits / ≥4.384 ms and does not require D state to recognize proved IO blocking.
- H3 globally displays 8 of 198 requests, including 6 target requests; 190 hidden requests contribute 41.329 request·ms (not target blocking wall-clock). The answer discloses this global cap, but its “共捕获6条有效目标请求” needs displayed-subset qualification, and two unproved rows still name a “完成唤醒方”. Per-layer paired-event maxima are not measurements over “多个采样窗口” (`query.go` storage latency accumulator groups paired events by layer). These are separate semantic issues from the two regex misses.
- H3 user-facing leaks include `request_residence_caliber`, `issuer_blocked_caliber`, `Trace Target Blocking Wall-Clock Authority`, and `storage_latency_by_layer`. Do not add a keyword hard gate; improve shared measured-fact presentation and teaching instead.
- Process/context: no finalizer hard rejects or repair rounds in either run; H2 one facet-metadata patch, H3 none. Peak contexts 80,887 / 99,204 tokens (40% / 50% of 200k). H3 did three explorations / 17 iterations with three history prunes; the summary's explorer dispatch counter of zero is not proof of no exploration. Both analyzers attempted an unavailable grep once. The no-summary attachment preamble still says “derive hotspots, stalls … yourself” while another actual-stage instruction says there is no query/raw-file reader. This is a confirmed system teaching conflict, not proof that it caused every answer error.

Follow-up ownership and priorities: unified audit §15; HMC-01.3/16.4 for stage-appropriate attachment teaching, HMC-01.2/16.4 for factual IO/state labels, HMC-18.2/18.5 for heterogeneous answer-quality validation. A single run cannot prove model fluctuation or that all semantic errors are deterministic system bugs.

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
