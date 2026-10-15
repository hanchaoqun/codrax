# HMC246 fixed parallel evaluation batch

- Date: 2026-10-15; frozen code: `5c71656883ec71ef62ad8f44a552e1b3b588a07f`.
- Exactly two real cases, started concurrently, one run each; no third replay.
- Binary SHA-256: `ec919ac9ff120f3c1b0f009c666d257bc181e9ad59a8f2c751e170f54a0faea2`.
- Results root: `eval/results/hmc_manifest_sampling_20261015/` (local ignored evidence).

| Case | Machine / capability check | Manual | Accepted scope / residual |
|---|---|---|---|
| `trace_binary_sample_only` | Smoke PASS; CLI exit 0; 124 s | FAIL | Full original and exact 164 records / 259 weight, CPU distribution and complete hotspot bounds reached the finalizer. Prose still promotes unknown sample kind to active execution. |
| Fresh-process capture checklist | CLI exit 0; exact current-index audit FAIL; resume NOT_TRIGGERED | FAIL | Three missing source/thread tuples were really queried, including known-empty. Model rediscovered files instead of resuming, reused an old success, mixed source-specific frames and invented completion counts. |

Machine acceptance **1/2**; full manual acceptance **0/2**. A clean CLI exit is not answer acceptance. B's correct new partial queries do not sign the untriggered resume capability. Historical failures remain unchanged.

B starts after zero-model public-tool operator preparation of one old success and three pending items. The natural question names the checkpoint and business scope without tool syntax, expected counts or hidden guards. The real CLI is a separate process with current repository authorization, not injected old mutable evidence. Its two SQLite originals are constructed fixtures, not device recordings or a naturally interrupted LLM run.

Detailed findings: [manual audit](parallel_selected_summary_hmc_manifest_sampling_20261015_manual_audit.md), [central ledger §246](../docs/design/hmosperf_capability_gap_audit_20260917.md). Source, logs, native payloads, all answer surfaces, failed intermediate checks and frozen receipts remain local; no credentials are archived.

The first full suite formally failed on the new tool-local parameter's missing mirror-registry entry. A test-only exact registration and strict script-rejection regression fixed that omission without changing production code, the binary or live-evaluation inputs. The second frozen full suite formally exited 0: **93 packages PASS / 13 SKIP / zero failure events**. All 6,929 refrozen inputs, binary and original capture hashes verified after exit. Both full logs remain archived; deterministic test success does not change either manual verdict.
