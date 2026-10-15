# HMC245 fixed parallel evaluation batch

- Date: 2026-10-15; frozen code: `9ceaca54d363418c6d3af7c0fc314537c56a3a6b`.
- Exactly two real cases, started concurrently, one attempt each. No third model replay.
- Binary SHA-256: `c4c94e0669572cd995837983f4678a296aca55a66d370435dc6f5aabfcba6758`.
- Results root: `eval/results/hmc_sampling_aggregate_registration_20261015/` (local ignored evidence).

| Case | Machine | Manual | Duration | Accepted scope / residual |
|---|---|---|---|---|
| `trace_binary_sample_only` | PASS | FAIL | 118 s | Full original preparation and 164 records / 259 weight; CPU 0 and CPU 1 now distinguished. Preview timestamps are still generalized to the full population and unknown sampling state is treated as execution. |
| `native_registration_natural_controller_terminal` | PASS | PASS within bounded entry | Test 92.84 s; package 93.929 s | Real parent/child controllers and planner, actual registration, independent-process restoration, fresh test execution, source/proof/overall verified. Not full CLI or model-authored source preparation. |

Machine acceptance **2/2**; full manual acceptance **1/2**. The prior failed answers remain unchanged; no parent capability is closed from smoke assertions or an untriggered branch.

The registration entry uses public fixture preparation and `RunTests`, not a verifier model. Parent and restored-child workflow decisions are real model calls with the default controller skill. Its continuation is part of the same case, not a third case. Source, test, configuration and HEAD remain unchanged during registration/reverification.

Detailed conclusions and remaining obligations: [manual audit](parallel_selected_summary_hmc_sampling_aggregate_registration_20261015_manual_audit.md), [central ledger §245](../docs/design/hmosperf_capability_gap_audit_20260917.md). Raw source captures, logs, payloads, failures and frozen receipts are retained locally, not claimed as pushed files.

Final frozen repository regression: formal exit 0, **92 package PASS / 13 SKIP**, no failure events or non-JSON errors. All 6915 frozen build/eval inputs, binary and original capture reverified after formal exit. Related/race, skill and Trace/read-mode protection tests passed before the freeze. The test-command budget is not a production timeout change.
