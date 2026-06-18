# SWE-bench Historical Patch Audit

Generated: 2026-06-18 23:22:18 UTC

Scope: latest historical Codrax result per SWE-bench instance under `eval/results/swebench`.

Important: this is an oracle-assisted post-hoc audit, not an official SWE-bench score. The public dataset gold patch is used only to review whether Codrax touched the expected source surface and made a plausibly equivalent change.

## Summary

- Dataset: `SWE-bench/SWE-bench_Lite` split `test`.
- Input result rows: 3; unique latest instances: 3.
- Theoretical audit `pass`: 2/3 (66.7%).
- Theoretical audit `fail`: 1/3 (33.3%).
- Theoretical audit `unknown`: 0/3 (0.0%).
- Official resolved rate: not computed here; run the official SWE-bench harness for authoritative scoring.

## Reason Counts

| reason | count |
| --- | ---: |
| `typed_verify_failed` | 1 |
| `oracle_source_and_token_overlap_high` | 1 |
| `verify_passed_with_oracle_overlap` | 1 |

## System Shortfalls From The Audit

- Exploration/localization is still the largest product gap: many failed rows produce a non-empty patch, but the changed source files do not intersect the oracle source surface. That means the system often writes code before it has pinned the causal file/symbol.
- Verify evidence is frequently not decisive in historical artifacts: legacy rows lack the current local-acceptance fields, and several rows end in unverified/failed states while still exporting patches. The adapter now exposes this as an unevaluable denominator rather than a pass rate.
- Handoff quality needs to be measured by whether failure observations change the next patch surface, not only by whether context packs exist. Rows with source overlap but low token overlap should feed patch critic, impact analysis, and re-exploration.
- The write loop should prefer online edit-run-observe batches: small source-surface changes, typed observation, then continue. Large one-shot patches make wrong localization expensive and make verifier failures harder to attribute.
- Final answer quality cannot be audited from old result rows alone. Future SWE runs should persist a typed final-answer artifact with patch intent, touched contracts, observed failures, and residual risk.
- All audited instances have `codrax.out`, but none has a typed final-answer/report artifact. This audit stores log tails in JSONL for human spot checks; production eval should not rely on free-form terminal logs as the answer contract.

## Per-Instance Audit

| instance | repo | verdict | reason | model source | oracle source | overlap | token | verify |
| --- | --- | --- | --- | --- | --- | ---: | ---: | --- |
| `django__django-14534` | django/django | `fail` | `typed_verify_failed` | django/forms/boundfield.py | django/forms/boundfield.py | 1.0 | 1.0 | `failed` |
| `pytest-dev__pytest-5227` | pytest-dev/pytest | `pass` | `oracle_source_and_token_overlap_high` | src/_pytest/logging.py | src/_pytest/logging.py | 1.0 | 1.0 | `unavailable` |
| `sympy__sympy-23117` | sympy/sympy | `pass` | `verify_passed_with_oracle_overlap` | sympy/tensor/array/dense_ndim_array.py, sympy/tensor/array/ndim_array.py | sympy/tensor/array/ndim_array.py | 1.0 | 0.2 | `passed` |

## Reproduction

```bash
eval/results/swebench/.venv/bin/python eval/swebench/audit_historical_results.py --results-glob 'eval/results/swebench/*/results.jsonl' --dedupe latest-by-file-mtime --dataset-name SWE-bench/SWE-bench_Lite --split test --output-jsonl docs/design/swebench_historical_patch_audit_20260618.jsonl --output-md docs/design/swebench_historical_patch_audit_20260618.md
```
