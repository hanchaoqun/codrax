# Constructed capture-directory evaluation

The captures/ directory contains two independently generated SQLite captures
with the same basename in different directories. It is not a device recording.
Both use the native resource-stack producer protocol from
eval/fixtures/hmosperf_native_resource_stack/capture.sql; the nested capture applies
eval/cases/trace_catalog_objects.nested.sql after creation. Discovery must retain both file identities,
including the nested member, without treating the .data suffix as admission.

Build new, absent output files from the repository root:

    sqlite3 eval/fixtures/hmosperf_catalog_objects/captures/capture.data < eval/fixtures/hmosperf_native_resource_stack/capture.sql
    sqlite3 eval/fixtures/hmosperf_catalog_objects/captures/nested/capture.data < eval/fixtures/hmosperf_native_resource_stack/capture.sql
    sqlite3 eval/fixtures/hmosperf_catalog_objects/captures/nested/capture.data < eval/cases/trace_catalog_objects.nested.sql

Independent oracle for [10.000, 10.050) seconds:

This file and the build SQL are outside the model-visible fixture. They are
audit inputs only; the user question does not teach the expected answers or
the system's validation rules.

| Capture | Thread | Resource events | Source frame rows | Meaning |
| --- | ---: | ---: | ---: | --- |
| captures/capture.data | 101 | 3 | 8 | Two allocations and one free; retain missing symbol/depth and duplicate-depth distinctions |
| captures/capture.data | 202 | 1 | 3 | One allocation owned by the other thread |
| captures/nested/capture.data | 101 | 1 | 3 | One allocation; independent editor symbols, not the first capture's gallery symbols |
| captures/nested/capture.data | 202 | 0 | 0 | A completed empty query, not unexecuted work |

The event at exactly 10.050 in the first capture is outside the explicit
window. There are no scheduler witnesses: resource ownership does not establish
CPU execution, timing causality or a leak. Query windows and identities must
remain per capture; do not sum captures into one measurement.

Manual acceptance also inspects discovery, optional frozen query inventory,
individual native-query results and the final catalog status. If the model
fails to register expected queries, do not infer completion from successful
members alone. Failed/unexecuted/stale-member isolation is covered by the
deterministic public-entry tests, not by a fabricated failure in this live pair.
