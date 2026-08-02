# Selected Eval Manual Audit

- date: 2026-08-02T15:58:05Z
- baseline: `main@fb49ccd9c`
- total cases: 2
- parallel: 2
- runner result: 1 PASS / 1 typed FAIL
- human result: read FAIL / write PASS with verification boundary

| case | runner | human | context and answer audit |
|---|---|---|---|
| `read_combo_log_current_source_explanation` | PASS, 228s | FAIL | `ROUTEREPAIR1` is covered: after the symmetric typed rejection the analyzer explicitly chose `explain / architecture_explain` and cleared diagnostic flags; there was no system semantic replacement. The final answer nevertheless invented a single `errors.As` dispatcher between dispatch errors and `Violation` lists, claimed first-byte timeout enters the no-visible-output-only fallback, and treated render retry wording as shared validation control. The six source reads and independent-mechanism guidance were sufficient to avoid this; keep as model-variance watch and do not add prose gates or answer replacement. |
| `github_issue_napi_force_wasi_env_symptom` | FAIL (`unverified/verification_proof_incomplete`), 179s | PASS with honest boundary | Production patch is correct. `LOCALIZE1` is covered: final state is `localization_status=supported`, owner path is supported, and the old missing-owner contradiction is absent. Node/npm are unavailable, so the JavaScript behavior probe and required behavior contracts remain genuinely unverified. This typed FAIL must not be weakened. |

## Context precision audit

- Analyzer correction context now presents both legal typed repairs and leaves the choice to the model.
- Read routing stays `explain / architecture_explain`; no root-cause family or Trace contract is injected.
- The runtime log proves a first-byte timeout attempt. It does not prove the separate post-success answer-validation control path.
- The model had independent-mechanism, producer-role, and runtime-rule-instantiation guidance plus six source reads, but did not open the actual successful-finalize `runContractCheck` / violation / requeue join. Additional answer-side hardening would fit one output instead of repairing a system information gap.
- Write localization context is internally consistent after the fix. Verification remains strict because missing runners and uncovered behavior contracts are precise typed facts.
- No Trace resolver/query/authority/mutation path changed; explicit-window causal projection and auto-supplement behavior remain untouched.

## Disposition

- `EVAL-B42-ROUTEREPAIR1`: covered.
- `EVAL-B42-LOCALIZE1`: covered.
- Read mechanism error: model-variance watch; no code action.
- Write `unverified`: correct fail-closed boundary; no code action.
- Move to the next higher-priority cross-mode pair.
