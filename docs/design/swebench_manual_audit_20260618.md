# SWE-bench Lite 137 Instance Manual Audit

Date: 2026-06-18
Branch: main
Status: complete

## Scope

This audit reviews the 137 historical SWE-bench Lite instances that had been
run through Codrax write mode and collected under the local SWE-bench result
artifacts. The purpose is to correct the reporting boundary:

- non-empty patch rate is an export-compatibility signal, not correctness;
- local Codrax verification is useful but can be incomplete;
- a typed per-instance manual audit verdict is required before reporting a
  manual-audit pass rate.

The official SWE-bench harness remains the only authority for official
`resolved/total` scoring. This document records a conservative internal audit
against the official dataset gold patches so we can reason about system gaps
without pretending the result is the official score.

## Artifacts

Generated local artifacts, intentionally not committed:

- `eval/results/swebench/manual-audit-20260618/audit_input.jsonl`
  - 137 rows of Codrax run telemetry joined with official SWE-bench Lite
    metadata.
  - Includes `status`, `verify_status`, patch path overlap, similarity, run
    directory, and prediction path.
- `eval/results/swebench/manual-audit-20260618/manual_audit.jsonl`
  - 137 typed audit rows keyed by `instance_id`.
  - Fields: `manual_audit_verdict`, `manual_audit_reason_code`,
    `manual_audit_source`, and `manual_audit_notes`.
- `eval/results/swebench/manual-audit-20260618/packs/*.md`
  - Per-instance review packs containing problem statement, official gold
    patch, and Codrax patch.
  - These packs include gold patches and must remain ignored local audit
    material.

Gold patch data was loaded from the cached Hugging Face
`SWE-bench/SWE-bench_Lite` test split using the existing
`eval/results/swebench/.venv` environment. Gold data was used only for human
audit material and was not injected into Codrax write-mode prompts or runtime
control.

## Method

Each instance was reviewed with the problem statement, the official gold patch,
and the Codrax exported source patch.

Verdict rules:

- `pass`: Codrax patch is exact, near-exact, or clearly issue-satisfying and
  behavior-equivalent to the gold fix.
- `fail`: Codrax patch is empty, obviously wrong-layer, misses a required
  behavior, changes the wrong owner boundary, or is contradicted by typed
  evidence and gold review.
- `unknown`: gold-diff review alone did not prove correctness or incorrectness.
  These rows need official harness execution or deeper project-specific
  execution.

Free-form notes are evidence only. Runtime logic and aggregate reporting must
consume only typed fields such as `manual_audit_verdict`, `verify_status`,
and `prediction_blocks_local_acceptance`.

## Results

Total audited instances: 137.

| Metric | Count | Rate | Meaning |
| --- | ---: | ---: | --- |
| Non-empty exported patch | 130 / 137 | 94.9% | Export compatibility only. |
| Local authoritative verify passed | 18 / 137 | 13.1% | Codrax normalized typed `verify_status=passed`. |
| Manual audit pass | 30 / 137 | 21.9% | Strict conservative human audit pass. |
| Manual audit fail | 28 / 137 | 20.4% | Definite incorrect or empty/wrong-layer patches. |
| Manual audit unknown | 79 / 137 | 57.7% | Not counted as pass. Needs harness/deeper execution. |
| Audited internal acceptance | 30 / 137 | 21.9% | Local verify plus manual audit, with typed manual `fail` overriding local verify pass. |

The previously discussed raw OR proxy
`verify_status=passed OR manual_audit_verdict=pass` would produce
33 / 137 = 24.1%, but it is not the corrected audited acceptance rate because
three local verify-passed rows were manually audited as fail:

- `django__django-11742`
- `mwaskom__seaborn-3190`
- `pydata__xarray-4248`

The adapter has therefore been tightened so typed manual audit `fail` rows
override local verify pass in `local_acceptance_verdict`. This is a reporting
and eval-telemetry boundary; it does not route write-mode runtime behavior
from manual notes or model prose.

Run telemetry distribution:

- Prediction status: 130 `predicted`, 7 `empty_patch`.
- Verify status: 18 `passed`, 13 `failed`, 87 `unavailable`, 19 empty/missing.
- File overlap: 117 full-overlap rows, 20 zero-overlap rows.
- Patch similarity buckets: 16 at or above 0.9, 8 in 0.7-0.9, 40 in 0.4-0.7,
  66 in 0-0.4, and 7 zero.

## Pass Rows

- `astropy__astropy-12907`
- `django__django-10924`
- `django__django-11049`
- `django__django-11099`
- `django__django-11133`
- `django__django-11179`
- `django__django-14017`
- `django__django-14534`
- `django__django-14752`
- `matplotlib__matplotlib-23314`
- `psf__requests-2148`
- `pydata__xarray-5131`
- `pylint-dev__pylint-6506`
- `pytest-dev__pytest-11148`
- `pytest-dev__pytest-5227`
- `pytest-dev__pytest-7373`
- `pytest-dev__pytest-7432`
- `pytest-dev__pytest-7490`
- `pytest-dev__pytest-8365`
- `scikit-learn__scikit-learn-13142`
- `scikit-learn__scikit-learn-13496`
- `scikit-learn__scikit-learn-14894`
- `scikit-learn__scikit-learn-15535`
- `sphinx-doc__sphinx-10325`
- `sphinx-doc__sphinx-8474`
- `sphinx-doc__sphinx-8595`
- `sphinx-doc__sphinx-8713`
- `sympy__sympy-13480`
- `sympy__sympy-18199`
- `sympy__sympy-18532`

## Fail Rows

- `astropy__astropy-14365`
- `astropy__astropy-6938`
- `django__django-10914`
- `django__django-11001`
- `django__django-11019`
- `django__django-11742`
- `django__django-11848`
- `django__django-11964`
- `mwaskom__seaborn-2848`
- `mwaskom__seaborn-3190`
- `mwaskom__seaborn-3407`
- `pydata__xarray-4094`
- `pydata__xarray-4248`
- `pydata__xarray-4493`
- `pytest-dev__pytest-11143`
- `pytest-dev__pytest-5692`
- `sphinx-doc__sphinx-10451`
- `sphinx-doc__sphinx-11445`
- `sphinx-doc__sphinx-7738`
- `sphinx-doc__sphinx-8506`
- `sphinx-doc__sphinx-8801`
- `sympy__sympy-11870`
- `sympy__sympy-12171`
- `sympy__sympy-12236`
- `sympy__sympy-12454`
- `sympy__sympy-13177`
- `sympy__sympy-18057`
- `sympy__sympy-23117`

Representative fail patterns:

- In-place mutation vs rebinding: `astropy__astropy-6938`.
- Missing required branch of issue behavior: `astropy__astropy-14365`.
- Local verify false positive with wrong check id or wrong owner layer:
  `django__django-11742`, `mwaskom__seaborn-3190`,
  `pydata__xarray-4248`.
- Config-guarded behavior replaced by unconditional escaping/suppression:
  `sphinx-doc__sphinx-7738`.
- Narrow mathematical condition missing required assumptions:
  `sympy__sympy-13177`.

## Unknown Rows

- `astropy__astropy-14182`
- `astropy__astropy-14995`
- `astropy__astropy-7746`
- `django__django-11039`
- `django__django-13933`
- `matplotlib__matplotlib-18869`
- `matplotlib__matplotlib-22711`
- `matplotlib__matplotlib-22835`
- `matplotlib__matplotlib-23299`
- `matplotlib__matplotlib-23476`
- `matplotlib__matplotlib-23562`
- `matplotlib__matplotlib-23563`
- `matplotlib__matplotlib-23913`
- `matplotlib__matplotlib-23964`
- `matplotlib__matplotlib-23987`
- `matplotlib__matplotlib-24149`
- `matplotlib__matplotlib-24265`
- `matplotlib__matplotlib-24334`
- `matplotlib__matplotlib-24970`
- `matplotlib__matplotlib-25079`
- `matplotlib__matplotlib-25311`
- `matplotlib__matplotlib-25332`
- `matplotlib__matplotlib-25433`
- `matplotlib__matplotlib-25498`
- `matplotlib__matplotlib-26011`
- `matplotlib__matplotlib-26020`
- `mwaskom__seaborn-3010`
- `pallets__flask-4045`
- `pallets__flask-4992`
- `pallets__flask-5063`
- `psf__requests-1963`
- `psf__requests-2317`
- `psf__requests-2674`
- `psf__requests-3362`
- `psf__requests-863`
- `pydata__xarray-3364`
- `pylint-dev__pylint-5859`
- `pylint-dev__pylint-7080`
- `pylint-dev__pylint-7114`
- `pylint-dev__pylint-7228`
- `pylint-dev__pylint-7993`
- `pytest-dev__pytest-5103`
- `pytest-dev__pytest-5221`
- `pytest-dev__pytest-5413`
- `pytest-dev__pytest-5495`
- `pytest-dev__pytest-6116`
- `pytest-dev__pytest-7168`
- `pytest-dev__pytest-7220`
- `pytest-dev__pytest-8906`
- `pytest-dev__pytest-9359`
- `scikit-learn__scikit-learn-10297`
- `scikit-learn__scikit-learn-10508`
- `scikit-learn__scikit-learn-10949`
- `scikit-learn__scikit-learn-11040`
- `scikit-learn__scikit-learn-11281`
- `scikit-learn__scikit-learn-12471`
- `scikit-learn__scikit-learn-13241`
- `scikit-learn__scikit-learn-13439`
- `scikit-learn__scikit-learn-13497`
- `scikit-learn__scikit-learn-13584`
- `scikit-learn__scikit-learn-13779`
- `scikit-learn__scikit-learn-14087`
- `scikit-learn__scikit-learn-14983`
- `scikit-learn__scikit-learn-15512`
- `scikit-learn__scikit-learn-25500`
- `scikit-learn__scikit-learn-25570`
- `scikit-learn__scikit-learn-25747`
- `sphinx-doc__sphinx-7686`
- `sphinx-doc__sphinx-7975`
- `sphinx-doc__sphinx-8273`
- `sphinx-doc__sphinx-8282`
- `sphinx-doc__sphinx-8435`
- `sphinx-doc__sphinx-8627`
- `sphinx-doc__sphinx-8721`
- `sympy__sympy-11400`
- `sympy__sympy-11897`
- `sympy__sympy-12419`
- `sympy__sympy-12481`
- `sympy__sympy-13031`

## System Implications

1. Non-empty patch rate must stay out of correctness reporting.
   It proves Codrax can export harness-consumable predictions, not that the
   patch resolves the issue.
2. Local verify pass is valuable but not sufficient as a final correctness
   number. The three local verify-passed/manual fail conflicts show that
   partial local environments and weak focused tests can accept wrong-layer
   patches.
3. Manual audit rows need typed precedence. A typed `manual_audit_verdict=fail`
   is a local audit blocker for internal acceptance telemetry, while free-form
   notes remain non-authoritative.
4. The largest remaining evaluation gap is official harness execution for the
   79 unknown rows. Treating unknown as pass would overstate quality; treating
   unknown as hard fail would understate unverified-but-plausible fixes.
5. Future commercial work should prioritize stronger owner-boundary tests,
   impact analysis, and authoritative project test execution over additional
   patch-export metrics.

## Current Reported Rate

For this 137-instance corpus, after conservative manual audit:

```text
strict manual audit pass = 30 / 137 = 21.9%
audited internal acceptance = 30 / 137 = 21.9%
official SWE-bench resolved = not measured in this audit
```
