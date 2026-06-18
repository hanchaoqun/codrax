# SWE-bench Historical Patch Audit

Generated: 2026-06-18 15:44:13 UTC

Scope: latest historical Codrax result per SWE-bench instance under `eval/results/swebench`.

Important: this is an oracle-assisted post-hoc audit, not an official SWE-bench score. The public dataset gold patch is used only to review whether Codrax touched the expected source surface and made a plausibly equivalent change.

## Summary

- Dataset: `SWE-bench/SWE-bench_Lite` split `test`.
- Input result rows: 278; unique latest instances: 137.
- Theoretical audit `pass`: 60/137 (43.8%).
- Theoretical audit `fail`: 43/137 (31.4%).
- Theoretical audit `unknown`: 34/137 (24.8%).
- Official resolved rate: not computed here; run the official SWE-bench harness for authoritative scoring.

## Reason Counts

| reason | count |
| --- | ---: |
| `oracle_source_and_token_overlap_high` | 48 |
| `source_overlap_requires_manual_review` | 21 |
| `plausible_overlap_unverified` | 13 |
| `wrong_source_surface_no_oracle_overlap` | 13 |
| `verify_passed_with_oracle_overlap` | 12 |
| `weak_semantic_overlap` | 12 |
| `typed_verify_failed` | 11 |
| `empty_patch` | 7 |

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
| `astropy__astropy-12907` | astropy/astropy | `pass` | `oracle_source_and_token_overlap_high` | astropy/modeling/separable.py | astropy/modeling/separable.py | 1.0 | 1.0 | `unavailable` |
| `astropy__astropy-14182` | astropy/astropy | `unknown` | `plausible_overlap_unverified` | astropy/io/ascii/rst.py | astropy/io/ascii/rst.py | 1.0 | 0.2353 | `unavailable` |
| `astropy__astropy-14365` | astropy/astropy | `pass` | `oracle_source_and_token_overlap_high` | astropy/io/ascii/qdp.py | astropy/io/ascii/qdp.py | 1.0 | 0.5 | `unavailable` |
| `astropy__astropy-14995` | astropy/astropy | `unknown` | `plausible_overlap_unverified` | astropy/nddata/mixins/ndarithmetic.py | astropy/nddata/mixins/ndarithmetic.py | 1.0 | 0.2727 | `unavailable` |
| `astropy__astropy-6938` | astropy/astropy | `pass` | `oracle_source_and_token_overlap_high` | astropy/io/fits/fitsrec.py | astropy/io/fits/fitsrec.py | 1.0 | 0.8333 | `unavailable` |
| `astropy__astropy-7746` | astropy/astropy | `unknown` | `plausible_overlap_unverified` | astropy/wcs/wcs.py | astropy/wcs/wcs.py | 1.0 | 0.2571 | `` |
| `django__django-10914` | django/django | `fail` | `typed_verify_failed` | django/conf/global_settings.py | django/conf/global_settings.py | 1.0 | 0.4444 | `failed` |
| `django__django-10924` | django/django | `pass` | `verify_passed_with_oracle_overlap` | django/db/models/fields/__init__.py, django/forms/fields.py | django/db/models/fields/__init__.py | 1.0 | 0.2857 | `passed` |
| `django__django-11001` | django/django | `fail` | `typed_verify_failed` | django/db/models/sql/compiler.py | django/db/models/sql/compiler.py | 1.0 | 0.1304 | `failed` |
| `django__django-11019` | django/django | `fail` | `typed_verify_failed` | django/forms/widgets.py | django/forms/widgets.py | 1.0 | 0.0154 | `failed` |
| `django__django-11039` | django/django | `pass` | `oracle_source_and_token_overlap_high` | django/core/management/commands/sqlmigrate.py | django/core/management/commands/sqlmigrate.py | 1.0 | 0.2903 | `` |
| `django__django-11049` | django/django | `pass` | `verify_passed_with_oracle_overlap` | django/db/models/fields/__init__.py, django/forms/fields.py | django/db/models/fields/__init__.py | 1.0 | 0.75 | `passed` |
| `django__django-11099` | django/django | `pass` | `verify_passed_with_oracle_overlap` | django/contrib/auth/validators.py | django/contrib/auth/validators.py | 1.0 | 0.8 | `passed` |
| `django__django-11133` | django/django | `pass` | `verify_passed_with_oracle_overlap` | django/http/response.py | django/http/response.py | 1.0 | 1.0 | `passed` |
| `django__django-11179` | django/django | `pass` | `verify_passed_with_oracle_overlap` | django/db/models/deletion.py | django/db/models/deletion.py | 1.0 | 0.4286 | `passed` |
| `django__django-11742` | django/django | `pass` | `verify_passed_with_oracle_overlap` | django/db/models/fields/__init__.py | django/db/models/fields/__init__.py | 1.0 | 0.4576 | `passed` |
| `django__django-11848` | django/django | `fail` | `typed_verify_failed` | django/utils/http.py | django/utils/http.py | 1.0 | 0.3 | `failed` |
| `django__django-11964` | django/django | `fail` | `wrong_source_surface_no_oracle_overlap` | django/db/models/fields/__init__.py | django/db/models/enums.py | 0.0 | 0.1282 | `failed` |
| `django__django-13933` | django/django | `pass` | `oracle_source_and_token_overlap_high` | django/forms/models.py | django/forms/models.py | 1.0 | 0.4 | `` |
| `django__django-14017` | django/django | `pass` | `verify_passed_with_oracle_overlap` | django/db/models/expressions.py, django/db/models/query_utils.py | django/db/models/query_utils.py | 1.0 | 0.4375 | `passed` |
| `django__django-14534` | django/django | `fail` | `typed_verify_failed` | django/forms/boundfield.py | django/forms/boundfield.py | 1.0 | 1.0 | `failed` |
| `django__django-14752` | django/django | `pass` | `verify_passed_with_oracle_overlap` | django/contrib/admin/views/autocomplete.py | django/contrib/admin/views/autocomplete.py | 1.0 | 0.697 | `passed` |
| `matplotlib__matplotlib-18869` | matplotlib/matplotlib | `fail` | `weak_semantic_overlap` | lib/matplotlib/__init__.py | lib/matplotlib/__init__.py | 1.0 | 0.0526 | `unavailable` |
| `matplotlib__matplotlib-22711` | matplotlib/matplotlib | `fail` | `weak_semantic_overlap` | lib/matplotlib/widgets.py | lib/matplotlib/widgets.py | 1.0 | 0.0741 | `` |
| `matplotlib__matplotlib-22835` | matplotlib/matplotlib | `unknown` | `plausible_overlap_unverified` | lib/matplotlib/artist.py | lib/matplotlib/artist.py | 1.0 | 0.2642 | `unavailable` |
| `matplotlib__matplotlib-23299` | matplotlib/matplotlib | `fail` | `weak_semantic_overlap` | lib/matplotlib/__init__.py | lib/matplotlib/__init__.py | 1.0 | 0.0 | `unavailable` |
| `matplotlib__matplotlib-23314` | matplotlib/matplotlib | `pass` | `oracle_source_and_token_overlap_high` | lib/mpl_toolkits/mplot3d/axes3d.py | lib/mpl_toolkits/mplot3d/axes3d.py | 1.0 | 1.0 | `unavailable` |
| `matplotlib__matplotlib-23476` | matplotlib/matplotlib | `unknown` | `source_overlap_requires_manual_review` | lib/matplotlib/figure.py | lib/matplotlib/figure.py | 1.0 | 0.0984 | `unavailable` |
| `matplotlib__matplotlib-23562` | matplotlib/matplotlib | `unknown` | `source_overlap_requires_manual_review` | lib/mpl_toolkits/mplot3d/art3d.py | lib/mpl_toolkits/mplot3d/art3d.py | 1.0 | 0.1765 | `unavailable` |
| `matplotlib__matplotlib-23563` | matplotlib/matplotlib | `unknown` | `source_overlap_requires_manual_review` | lib/mpl_toolkits/mplot3d/art3d.py | lib/mpl_toolkits/mplot3d/art3d.py | 1.0 | 0.1111 | `unavailable` |
| `matplotlib__matplotlib-23913` | matplotlib/matplotlib | `pass` | `oracle_source_and_token_overlap_high` | lib/matplotlib/legend.py | lib/matplotlib/legend.py | 1.0 | 0.875 | `unavailable` |
| `matplotlib__matplotlib-23964` | matplotlib/matplotlib | `unknown` | `source_overlap_requires_manual_review` | lib/matplotlib/backends/backend_ps.py | lib/matplotlib/backends/backend_ps.py | 1.0 | 0.1333 | `unavailable` |
| `matplotlib__matplotlib-23987` | matplotlib/matplotlib | `fail` | `weak_semantic_overlap` | lib/matplotlib/figure.py | lib/matplotlib/figure.py | 1.0 | 0.0769 | `unavailable` |
| `matplotlib__matplotlib-24149` | matplotlib/matplotlib | `unknown` | `source_overlap_requires_manual_review` | lib/matplotlib/axes/_axes.py | lib/matplotlib/axes/_axes.py | 1.0 | 0.0909 | `unavailable` |
| `matplotlib__matplotlib-24265` | matplotlib/matplotlib | `pass` | `oracle_source_and_token_overlap_high` | lib/matplotlib/style/core.py | lib/matplotlib/style/core.py | 1.0 | 0.6162 | `unavailable` |
| `matplotlib__matplotlib-24334` | matplotlib/matplotlib | `unknown` | `source_overlap_requires_manual_review` | lib/matplotlib/axis.py | lib/matplotlib/axis.py | 1.0 | 0.1379 | `unavailable` |
| `matplotlib__matplotlib-24970` | matplotlib/matplotlib | `unknown` | `source_overlap_requires_manual_review` | lib/matplotlib/colors.py | lib/matplotlib/colors.py | 1.0 | 0.098 | `unavailable` |
| `matplotlib__matplotlib-25079` | matplotlib/matplotlib | `fail` | `wrong_source_surface_no_oracle_overlap` | lib/matplotlib/cm.py | lib/matplotlib/colors.py | 0.0 | 0.1111 | `unavailable` |
| `matplotlib__matplotlib-25311` | matplotlib/matplotlib | `unknown` | `source_overlap_requires_manual_review` | lib/matplotlib/offsetbox.py | lib/matplotlib/offsetbox.py | 1.0 | 0.1132 | `unavailable` |
| `matplotlib__matplotlib-25332` | matplotlib/matplotlib | `unknown` | `plausible_overlap_unverified` | lib/matplotlib/cbook.py | lib/matplotlib/cbook.py | 1.0 | 0.2097 | `unavailable` |
| `matplotlib__matplotlib-25433` | matplotlib/matplotlib | `fail` | `wrong_source_surface_no_oracle_overlap` | lib/matplotlib/widgets.py | lib/matplotlib/figure.py | 0.0 | 0.375 | `unavailable` |
| `matplotlib__matplotlib-25498` | matplotlib/matplotlib | `pass` | `oracle_source_and_token_overlap_high` | lib/matplotlib/colorbar.py | lib/matplotlib/colorbar.py | 1.0 | 0.2857 | `unavailable` |
| `matplotlib__matplotlib-26011` | matplotlib/matplotlib | `pass` | `oracle_source_and_token_overlap_high` | lib/matplotlib/axis.py | lib/matplotlib/axis.py | 1.0 | 0.6111 | `unavailable` |
| `matplotlib__matplotlib-26020` | matplotlib/matplotlib | `pass` | `oracle_source_and_token_overlap_high` | lib/mpl_toolkits/axes_grid1/axes_grid.py | lib/mpl_toolkits/axes_grid1/axes_grid.py | 1.0 | 0.3529 | `unavailable` |
| `mwaskom__seaborn-2848` | mwaskom/seaborn | `fail` | `empty_patch` |  | seaborn/_oldcore.py | 0.0 | 0.0 | `` |
| `mwaskom__seaborn-3010` | mwaskom/seaborn | `unknown` | `source_overlap_requires_manual_review` | seaborn/_stats/regression.py | seaborn/_stats/regression.py | 1.0 | 0.1304 | `` |
| `mwaskom__seaborn-3190` | mwaskom/seaborn | `fail` | `weak_semantic_overlap` | seaborn/_core/scales.py | seaborn/_core/scales.py | 1.0 | 0.0435 | `passed` |
| `mwaskom__seaborn-3407` | mwaskom/seaborn | `fail` | `typed_verify_failed` | seaborn/axisgrid.py | seaborn/axisgrid.py | 1.0 | 0.0 | `failed` |
| `pallets__flask-4045` | pallets/flask | `pass` | `oracle_source_and_token_overlap_high` | src/flask/blueprints.py | src/flask/blueprints.py | 1.0 | 0.2857 | `unavailable` |
| `pallets__flask-4992` | pallets/flask | `pass` | `oracle_source_and_token_overlap_high` | src/flask/config.py | src/flask/config.py | 1.0 | 0.5185 | `unavailable` |
| `pallets__flask-5063` | pallets/flask | `pass` | `oracle_source_and_token_overlap_high` | src/flask/cli.py | src/flask/cli.py | 1.0 | 0.3302 | `` |
| `psf__requests-1963` | psf/requests | `pass` | `oracle_source_and_token_overlap_high` | requests/sessions.py | requests/sessions.py | 1.0 | 0.3333 | `` |
| `psf__requests-2148` | psf/requests | `pass` | `oracle_source_and_token_overlap_high` | requests/models.py | requests/models.py | 1.0 | 1.0 | `unavailable` |
| `psf__requests-2317` | psf/requests | `fail` | `weak_semantic_overlap` | requests/sessions.py | requests/sessions.py | 1.0 | 0.0357 | `unavailable` |
| `psf__requests-2674` | psf/requests | `fail` | `wrong_source_surface_no_oracle_overlap` | requests/exceptions.py, requests/packages/__init__.py | requests/adapters.py | 0.0 | 0.1364 | `unavailable` |
| `psf__requests-3362` | psf/requests | `unknown` | `source_overlap_requires_manual_review` | requests/utils.py | requests/utils.py | 1.0 | 0.1364 | `unavailable` |
| `psf__requests-863` | psf/requests | `pass` | `oracle_source_and_token_overlap_high` | requests/models.py | requests/models.py | 1.0 | 0.4118 | `unavailable` |
| `pydata__xarray-3364` | pydata/xarray | `unknown` | `source_overlap_requires_manual_review` | xarray/core/concat.py | xarray/core/concat.py | 1.0 | 0.1786 | `unavailable` |
| `pydata__xarray-4094` | pydata/xarray | `fail` | `wrong_source_surface_no_oracle_overlap` | xarray/core/concat.py, xarray/core/dataset.py | xarray/core/dataarray.py | 0.0 | 0.0 | `failed` |
| `pydata__xarray-4248` | pydata/xarray | `fail` | `weak_semantic_overlap` | xarray/core/formatting.py | xarray/core/formatting.py | 1.0 | 0.0556 | `passed` |
| `pydata__xarray-4493` | pydata/xarray | `fail` | `empty_patch` |  | xarray/core/variable.py | 0.0 | 0.0 | `` |
| `pydata__xarray-5131` | pydata/xarray | `pass` | `oracle_source_and_token_overlap_high` | xarray/core/groupby.py | xarray/core/groupby.py | 1.0 | 1.0 | `unavailable` |
| `pylint-dev__pylint-5859` | pylint-dev/pylint | `pass` | `oracle_source_and_token_overlap_high` | pylint/checkers/misc.py | pylint/checkers/misc.py | 1.0 | 0.5714 | `` |
| `pylint-dev__pylint-6506` | pylint-dev/pylint | `fail` | `wrong_source_surface_no_oracle_overlap` | pylint/__init__.py | pylint/config/config_initialization.py | 0.0 | 0.04 | `passed` |
| `pylint-dev__pylint-7080` | pylint-dev/pylint | `fail` | `wrong_source_surface_no_oracle_overlap` | pylint/lint/pylinter.py | pylint/lint/expand_modules.py | 0.0 | 0.0909 | `unavailable` |
| `pylint-dev__pylint-7114` | pylint-dev/pylint | `unknown` | `plausible_overlap_unverified` | pylint/lint/expand_modules.py | pylint/lint/expand_modules.py | 1.0 | 0.2414 | `unavailable` |
| `pylint-dev__pylint-7228` | pylint-dev/pylint | `unknown` | `plausible_overlap_unverified` | pylint/config/argument.py, pylint/config/option.py | pylint/config/argument.py | 1.0 | 0.2424 | `unavailable` |
| `pylint-dev__pylint-7993` | pylint-dev/pylint | `fail` | `weak_semantic_overlap` | pylint/reporters/text.py | pylint/reporters/text.py | 1.0 | 0.0556 | `unavailable` |
| `pytest-dev__pytest-11143` | pytest-dev/pytest | `fail` | `typed_verify_failed` | src/_pytest/assertion/rewrite.py | src/_pytest/assertion/rewrite.py | 1.0 | 0.3333 | `failed` |
| `pytest-dev__pytest-11148` | pytest-dev/pytest | `unknown` | `source_overlap_requires_manual_review` | src/_pytest/pathlib.py | src/_pytest/pathlib.py | 1.0 | 0.16 | `passed` |
| `pytest-dev__pytest-5103` | pytest-dev/pytest | `unknown` | `plausible_overlap_unverified` | src/_pytest/assertion/rewrite.py | src/_pytest/assertion/rewrite.py | 1.0 | 0.2483 | `unavailable` |
| `pytest-dev__pytest-5221` | pytest-dev/pytest | `pass` | `oracle_source_and_token_overlap_high` | src/_pytest/python.py | src/_pytest/python.py | 1.0 | 0.3333 | `unavailable` |
| `pytest-dev__pytest-5227` | pytest-dev/pytest | `pass` | `oracle_source_and_token_overlap_high` | src/_pytest/logging.py | src/_pytest/logging.py | 1.0 | 1.0 | `unavailable` |
| `pytest-dev__pytest-5413` | pytest-dev/pytest | `fail` | `wrong_source_surface_no_oracle_overlap` | src/_pytest/python_api.py | src/_pytest/_code/code.py | 0.0 | 0.3636 | `unavailable` |
| `pytest-dev__pytest-5495` | pytest-dev/pytest | `unknown` | `source_overlap_requires_manual_review` | src/_pytest/assertion/util.py | src/_pytest/assertion/util.py | 1.0 | 0.1385 | `unavailable` |
| `pytest-dev__pytest-5692` | pytest-dev/pytest | `fail` | `typed_verify_failed` | src/_pytest/junitxml.py | src/_pytest/junitxml.py | 1.0 | 0.1765 | `failed` |
| `pytest-dev__pytest-6116` | pytest-dev/pytest | `fail` | `weak_semantic_overlap` | src/_pytest/main.py | src/_pytest/main.py | 1.0 | 0.0 | `unavailable` |
| `pytest-dev__pytest-7168` | pytest-dev/pytest | `unknown` | `plausible_overlap_unverified` | src/_pytest/_io/saferepr.py | src/_pytest/_io/saferepr.py | 1.0 | 0.2727 | `` |
| `pytest-dev__pytest-7220` | pytest-dev/pytest | `unknown` | `source_overlap_requires_manual_review` | src/_pytest/_code/code.py, src/_pytest/nodes.py | src/_pytest/nodes.py | 1.0 | 0.125 | `` |
| `pytest-dev__pytest-7373` | pytest-dev/pytest | `pass` | `oracle_source_and_token_overlap_high` | src/_pytest/mark/evaluate.py | src/_pytest/mark/evaluate.py | 1.0 | 0.9143 | `unavailable` |
| `pytest-dev__pytest-7432` | pytest-dev/pytest | `pass` | `oracle_source_and_token_overlap_high` | src/_pytest/skipping.py | src/_pytest/skipping.py | 1.0 | 1.0 | `unavailable` |
| `pytest-dev__pytest-7490` | pytest-dev/pytest | `pass` | `verify_passed_with_oracle_overlap` | src/_pytest/skipping.py | src/_pytest/skipping.py | 1.0 | 0.4667 | `passed` |
| `pytest-dev__pytest-8365` | pytest-dev/pytest | `fail` | `weak_semantic_overlap` | src/_pytest/tmpdir.py | src/_pytest/tmpdir.py | 1.0 | 0.0312 | `passed` |
| `pytest-dev__pytest-8906` | pytest-dev/pytest | `fail` | `wrong_source_surface_no_oracle_overlap` | src/_pytest/outcomes.py, src/_pytest/skipping.py | src/_pytest/python.py | 0.0 | 0.1139 | `unavailable` |
| `pytest-dev__pytest-9359` | pytest-dev/pytest | `fail` | `wrong_source_surface_no_oracle_overlap` | src/_pytest/assertion/rewrite.py | src/_pytest/_code/source.py | 0.0 | 0.025 | `unavailable` |
| `scikit-learn__scikit-learn-10297` | scikit-learn/scikit-learn | `pass` | `oracle_source_and_token_overlap_high` | sklearn/linear_model/ridge.py | sklearn/linear_model/ridge.py | 1.0 | 0.5606 | `unavailable` |
| `scikit-learn__scikit-learn-10508` | scikit-learn/scikit-learn | `pass` | `oracle_source_and_token_overlap_high` | sklearn/preprocessing/label.py | sklearn/preprocessing/label.py | 1.0 | 0.3077 | `unavailable` |
| `scikit-learn__scikit-learn-10949` | scikit-learn/scikit-learn | `unknown` | `source_overlap_requires_manual_review` | sklearn/utils/validation.py | sklearn/utils/validation.py | 1.0 | 0.12 | `unavailable` |
| `scikit-learn__scikit-learn-11040` | scikit-learn/scikit-learn | `pass` | `oracle_source_and_token_overlap_high` | sklearn/neighbors/base.py | sklearn/neighbors/base.py | 1.0 | 0.3929 | `unavailable` |
| `scikit-learn__scikit-learn-11281` | scikit-learn/scikit-learn | `pass` | `oracle_source_and_token_overlap_high` | sklearn/mixture/base.py | sklearn/mixture/base.py | 1.0 | 0.9286 | `unavailable` |
| `scikit-learn__scikit-learn-12471` | scikit-learn/scikit-learn | `pass` | `oracle_source_and_token_overlap_high` | sklearn/preprocessing/_encoders.py | sklearn/preprocessing/_encoders.py | 1.0 | 0.2857 | `unavailable` |
| `scikit-learn__scikit-learn-13142` | scikit-learn/scikit-learn | `pass` | `oracle_source_and_token_overlap_high` | sklearn/mixture/base.py | sklearn/mixture/base.py | 1.0 | 1.0 | `unavailable` |
| `scikit-learn__scikit-learn-13241` | scikit-learn/scikit-learn | `pass` | `oracle_source_and_token_overlap_high` | sklearn/decomposition/kernel_pca.py | sklearn/decomposition/kernel_pca.py | 1.0 | 0.2889 | `unavailable` |
| `scikit-learn__scikit-learn-13439` | scikit-learn/scikit-learn | `pass` | `oracle_source_and_token_overlap_high` | sklearn/pipeline.py | sklearn/pipeline.py | 1.0 | 1.0 | `unavailable` |
| `scikit-learn__scikit-learn-13496` | scikit-learn/scikit-learn | `pass` | `oracle_source_and_token_overlap_high` | sklearn/ensemble/iforest.py | sklearn/ensemble/iforest.py | 1.0 | 0.9143 | `unavailable` |
| `scikit-learn__scikit-learn-13497` | scikit-learn/scikit-learn | `unknown` | `plausible_overlap_unverified` | sklearn/feature_selection/mutual_info_.py | sklearn/feature_selection/mutual_info_.py | 1.0 | 0.2222 | `unavailable` |
| `scikit-learn__scikit-learn-13584` | scikit-learn/scikit-learn | `pass` | `oracle_source_and_token_overlap_high` | sklearn/utils/_pprint.py | sklearn/utils/_pprint.py | 1.0 | 0.4615 | `unavailable` |
| `scikit-learn__scikit-learn-13779` | scikit-learn/scikit-learn | `pass` | `oracle_source_and_token_overlap_high` | sklearn/ensemble/voting.py | sklearn/ensemble/voting.py | 1.0 | 0.4444 | `unavailable` |
| `scikit-learn__scikit-learn-14087` | scikit-learn/scikit-learn | `unknown` | `plausible_overlap_unverified` | sklearn/linear_model/logistic.py | sklearn/linear_model/logistic.py | 1.0 | 0.2778 | `unavailable` |
| `scikit-learn__scikit-learn-14894` | scikit-learn/scikit-learn | `pass` | `oracle_source_and_token_overlap_high` | sklearn/svm/base.py | sklearn/svm/base.py | 1.0 | 1.0 | `unavailable` |
| `scikit-learn__scikit-learn-14983` | scikit-learn/scikit-learn | `pass` | `oracle_source_and_token_overlap_high` | sklearn/model_selection/_split.py | sklearn/model_selection/_split.py | 1.0 | 0.4667 | `unavailable` |
| `scikit-learn__scikit-learn-15512` | scikit-learn/scikit-learn | `fail` | `weak_semantic_overlap` | sklearn/cluster/_affinity_propagation.py | sklearn/cluster/_affinity_propagation.py | 1.0 | 0.061 | `unavailable` |
| `scikit-learn__scikit-learn-15535` | scikit-learn/scikit-learn | `pass` | `oracle_source_and_token_overlap_high` | sklearn/metrics/cluster/_supervised.py | sklearn/metrics/cluster/_supervised.py | 1.0 | 1.0 | `unavailable` |
| `scikit-learn__scikit-learn-25500` | scikit-learn/scikit-learn | `fail` | `wrong_source_surface_no_oracle_overlap` | sklearn/calibration.py | sklearn/isotonic.py | 0.0 | 0.0132 | `unavailable` |
| `scikit-learn__scikit-learn-25570` | scikit-learn/scikit-learn | `unknown` | `source_overlap_requires_manual_review` | sklearn/compose/_column_transformer.py | sklearn/compose/_column_transformer.py | 1.0 | 0.1 | `unavailable` |
| `scikit-learn__scikit-learn-25747` | scikit-learn/scikit-learn | `pass` | `oracle_source_and_token_overlap_high` | sklearn/utils/_set_output.py | sklearn/utils/_set_output.py | 1.0 | 0.3889 | `unavailable` |
| `sphinx-doc__sphinx-10325` | sphinx-doc/sphinx | `pass` | `verify_passed_with_oracle_overlap` | sphinx/ext/autodoc/__init__.py | sphinx/ext/autodoc/__init__.py | 1.0 | 0.3014 | `passed` |
| `sphinx-doc__sphinx-10451` | sphinx-doc/sphinx | `fail` | `empty_patch` |  | sphinx/ext/autodoc/typehints.py | 0.0 | 0.0 | `` |
| `sphinx-doc__sphinx-11445` | sphinx-doc/sphinx | `fail` | `typed_verify_failed` | sphinx/util/rst.py | sphinx/util/rst.py | 1.0 | 0.0952 | `failed` |
| `sphinx-doc__sphinx-7686` | sphinx-doc/sphinx | `unknown` | `source_overlap_requires_manual_review` | sphinx/ext/autosummary/generate.py | sphinx/ext/autosummary/generate.py | 1.0 | 0.0988 | `unavailable` |
| `sphinx-doc__sphinx-7738` | sphinx-doc/sphinx | `pass` | `oracle_source_and_token_overlap_high` | sphinx/ext/napoleon/docstring.py | sphinx/ext/napoleon/docstring.py | 1.0 | 0.3333 | `unavailable` |
| `sphinx-doc__sphinx-7975` | sphinx-doc/sphinx | `fail` | `weak_semantic_overlap` | sphinx/environment/adapters/indexentries.py | sphinx/environment/adapters/indexentries.py | 1.0 | 0.0667 | `unavailable` |
| `sphinx-doc__sphinx-8273` | sphinx-doc/sphinx | `unknown` | `plausible_overlap_unverified` | sphinx/builders/manpage.py | sphinx/builders/manpage.py | 1.0 | 0.2258 | `unavailable` |
| `sphinx-doc__sphinx-8282` | sphinx-doc/sphinx | `unknown` | `source_overlap_requires_manual_review` | sphinx/ext/autodoc/__init__.py, sphinx/ext/autodoc/typehints.py | sphinx/ext/autodoc/__init__.py | 1.0 | 0.1695 | `unavailable` |
| `sphinx-doc__sphinx-8435` | sphinx-doc/sphinx | `unknown` | `source_overlap_requires_manual_review` | sphinx/ext/autodoc/__init__.py | sphinx/ext/autodoc/__init__.py | 1.0 | 0.0909 | `unavailable` |
| `sphinx-doc__sphinx-8474` | sphinx-doc/sphinx | `pass` | `verify_passed_with_oracle_overlap` | sphinx/domains/std.py | sphinx/domains/std.py | 1.0 | 0.5417 | `passed` |
| `sphinx-doc__sphinx-8506` | sphinx-doc/sphinx | `fail` | `typed_verify_failed` | sphinx/domains/std.py | sphinx/domains/std.py | 1.0 | 0.2632 | `failed` |
| `sphinx-doc__sphinx-8595` | sphinx-doc/sphinx | `pass` | `oracle_source_and_token_overlap_high` | sphinx/ext/autodoc/__init__.py | sphinx/ext/autodoc/__init__.py | 1.0 | 1.0 | `unavailable` |
| `sphinx-doc__sphinx-8627` | sphinx-doc/sphinx | `fail` | `wrong_source_surface_no_oracle_overlap` | sphinx/domains/python.py | sphinx/util/typing.py | 0.0 | 0.0222 | `unavailable` |
| `sphinx-doc__sphinx-8713` | sphinx-doc/sphinx | `pass` | `oracle_source_and_token_overlap_high` | sphinx/ext/napoleon/docstring.py | sphinx/ext/napoleon/docstring.py | 1.0 | 1.0 | `unavailable` |
| `sphinx-doc__sphinx-8721` | sphinx-doc/sphinx | `pass` | `oracle_source_and_token_overlap_high` | sphinx/ext/viewcode.py | sphinx/ext/viewcode.py | 1.0 | 0.8571 | `unavailable` |
| `sphinx-doc__sphinx-8801` | sphinx-doc/sphinx | `fail` | `empty_patch` |  | sphinx/ext/autodoc/importer.py | 0.0 | 0.0 | `` |
| `sympy__sympy-11400` | sympy/sympy | `pass` | `oracle_source_and_token_overlap_high` | sympy/printing/ccode.py | sympy/printing/ccode.py | 1.0 | 0.2923 | `` |
| `sympy__sympy-11870` | sympy/sympy | `fail` | `empty_patch` |  | sympy/functions/elementary/trigonometric.py | 0.0 | 0.0 | `` |
| `sympy__sympy-11897` | sympy/sympy | `unknown` | `source_overlap_requires_manual_review` | sympy/printing/latex.py | sympy/printing/latex.py | 1.0 | 0.1316 | `unavailable` |
| `sympy__sympy-12171` | sympy/sympy | `pass` | `oracle_source_and_token_overlap_high` | sympy/printing/mathematica.py | sympy/printing/mathematica.py | 1.0 | 0.35 | `unavailable` |
| `sympy__sympy-12236` | sympy/sympy | `fail` | `empty_patch` |  | sympy/polys/domains/polynomialring.py | 0.0 | 0.0 | `` |
| `sympy__sympy-12419` | sympy/sympy | `pass` | `oracle_source_and_token_overlap_high` | sympy/matrices/expressions/matexpr.py | sympy/matrices/expressions/matexpr.py | 1.0 | 0.5 | `unavailable` |
| `sympy__sympy-12454` | sympy/sympy | `pass` | `oracle_source_and_token_overlap_high` | sympy/matrices/matrices.py | sympy/matrices/matrices.py | 1.0 | 0.9 | `` |
| `sympy__sympy-12481` | sympy/sympy | `unknown` | `plausible_overlap_unverified` | sympy/combinatorics/permutations.py | sympy/combinatorics/permutations.py | 1.0 | 0.2143 | `unavailable` |
| `sympy__sympy-13031` | sympy/sympy | `fail` | `wrong_source_surface_no_oracle_overlap` | sympy/matrices/common.py | sympy/matrices/sparse.py | 0.0 | 0.375 | `unavailable` |
| `sympy__sympy-13177` | sympy/sympy | `pass` | `oracle_source_and_token_overlap_high` | sympy/core/mod.py | sympy/core/mod.py | 1.0 | 0.9 | `unavailable` |
| `sympy__sympy-13480` | sympy/sympy | `pass` | `oracle_source_and_token_overlap_high` | sympy/functions/elementary/hyperbolic.py | sympy/functions/elementary/hyperbolic.py | 1.0 | 1.0 | `unavailable` |
| `sympy__sympy-18057` | sympy/sympy | `fail` | `empty_patch` |  | sympy/core/expr.py | 0.0 | 0.0 | `` |
| `sympy__sympy-18199` | sympy/sympy | `unknown` | `source_overlap_requires_manual_review` | sympy/ntheory/residue_ntheory.py | sympy/ntheory/residue_ntheory.py | 1.0 | 0.0986 | `passed` |
| `sympy__sympy-18532` | sympy/sympy | `pass` | `verify_passed_with_oracle_overlap` | sympy/core/basic.py | sympy/core/basic.py | 1.0 | 0.7647 | `passed` |
| `sympy__sympy-23117` | sympy/sympy | `fail` | `typed_verify_failed` | sympy/tensor/array/ndim_array.py | sympy/tensor/array/ndim_array.py | 1.0 | 0.25 | `failed` |

## Reproduction

```bash
eval/results/swebench/.venv/bin/python eval/swebench/audit_historical_results.py --results-glob 'eval/results/swebench/*/results.jsonl' --dedupe latest-by-file-mtime --dataset-name SWE-bench/SWE-bench_Lite --split test --output-jsonl docs/design/swebench_historical_patch_audit_20260618.jsonl --output-md docs/design/swebench_historical_patch_audit_20260618.md
```
