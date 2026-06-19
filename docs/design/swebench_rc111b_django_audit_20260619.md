# SWE-bench Historical Patch Audit

Generated: 2026-06-19 00:39:21 UTC

Scope: latest historical Codrax result per SWE-bench instance under `eval/results/swebench`.

Important: this is an oracle-assisted post-hoc audit, not an official SWE-bench score. The public dataset gold patch is used only to review whether Codrax touched the expected source surface and made a plausibly equivalent change.

## Summary

- Dataset: `SWE-bench/SWE-bench_Lite` split `test`.
- Input result rows: 1; unique latest instances: 1.
- Theoretical audit `pass`: 1/1 (100.0%).
- Theoretical audit `fail`: 0/1 (0.0%).
- Theoretical audit `unknown`: 0/1 (0.0%).
- Official resolved rate: not computed here; run the official SWE-bench harness for authoritative scoring.

## Reason Counts

| reason | count |
| --- | ---: |
| `verify_passed_with_oracle_overlap` | 1 |

## System Shortfalls From The Audit

- Exploration/localization is still the largest product gap: many failed rows produce a non-empty patch, but the changed source files do not intersect the oracle source surface. That means the system often writes code before it has pinned the causal file/symbol.
- Verify evidence is frequently not decisive in historical artifacts: legacy rows lack the current local-acceptance fields, and several rows end in unverified/failed states while still exporting patches. The adapter now exposes this as an unevaluable denominator rather than a pass rate.
- Handoff quality needs to be measured by whether failure observations change the next patch surface, not only by whether context packs exist. Rows with source overlap but low token overlap should feed patch critic, impact analysis, and re-exploration.
- The write loop should prefer online edit-run-observe batches: small source-surface changes, typed observation, then continue. Large one-shot patches make wrong localization expensive and make verifier failures harder to attribute.
- Final answer quality cannot be audited from old result rows alone. Future SWE runs should persist a typed final-answer artifact with patch intent, touched contracts, observed failures, and residual risk.
- Typed final-report artifacts are present for 1/1 audited instance(s). Prefer these structured artifacts over terminal logs for delivery audit and residual-risk analysis.

## Per-Instance Audit

| instance | repo | verdict | reason | model source | oracle source | overlap | token | verify |
| --- | --- | --- | --- | --- | --- | ---: | ---: | --- |
| `django__django-14534` | django/django | `pass` | `verify_passed_with_oracle_overlap` | django/forms/boundfield.py | django/forms/boundfield.py | 1.0 | 0.5 | `passed` |

## Reproduction

```bash
eval/results/swebench/.venv/bin/python eval/swebench/audit_historical_results.py --results-glob 'eval/results/swebench/*/results.jsonl' --dedupe latest-by-file-mtime --dataset-name SWE-bench/SWE-bench_Lite --split test --output-jsonl docs/design/swebench_historical_patch_audit_20260618.jsonl --output-md docs/design/swebench_historical_patch_audit_20260618.md
```
