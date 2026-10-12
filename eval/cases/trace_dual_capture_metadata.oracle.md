# Independent oracle: two captures and collection metadata

This oracle is not attached to the model. The natural request only states the
two captures, shared [1,2) second window and desired measurements/context.
It does not prescribe tools, JSON, missing-data rules or causal guardrails.

## Measurements and source identity

The new files copy the exact measure/measure_filter/trace_range tables from
the [common-window case](trace_dual_common_window_measurements.oracle.md).
All its row/value/type/window expectations apply: baseline 13 selected rows
in 7 independent groups, current 8 rows in 4 groups; each has one unpositioned
row. Keep 334200000.5, 250000000.25 and both integers above 2^53 exact.
Both healthy inputs must really prepare and read, with independent available
states. No source or source generation can come from the other file.

## Metadata from independent SQL

| Source | Field | Recorded storage/value | Expected qualified state |
|---|---|---|---|
| baseline | device_info.physical_width | INTEGER 1176 | observed raw integer |
| baseline | device_info.physical_height | INTEGER 2520 | observed raw integer |
| baseline | device_info.physical_frame_rate | INTEGER 120 | observed raw integer |
| current | device_info.physical_width | INTEGER 1080 | observed raw integer |
| current | device_info.physical_height | NULL | missing, not zero or baseline height |
| current | device_info.physical_frame_rate | REAL 60.0 | invalid for the exact INTEGER decoder; not validated 60 Hz |

Parser metadata contains exact collector values capture-kit-A / capture-kit-B,
workload_cmd strings `open-document --set alpha` / `open-document --set beta`,
and device_name strings board-A-lab / board-B-lab. These are arbitrary
meta(name,value) fields, not native perf feature identity witnesses. Preserve
them as explicitly sourced literal diagnostic entries if surfaced; do not
upgrade them to a verified physical device, chip model, same device, same
workload, or matched scenario. Current's duplicate revision key is omitted
with a duplicate/invalid diagnostic, not resolved by first/last wins.
The capture records no physical display units or deadline authority from
device_info alone. Metadata is capture-level, not a measurement occurring
in the selected second, and cannot establish clock alignment or causality.

## Manual acceptance

Audit actual preparation, source-generation checks, single and/or comparison
query results, model-visible metadata, final native acquisition tables and
main narrative. Metadata must not be irretrievably replaced by constant
unknown when the exact source supplies these diagnostics; unknown identities
remain unknown. A present metadata table does not cure wrong narrative or
lost measurement rows. A tool-call attempt is not proof of successful read.

The pair need not use a particular query order or both query syntaxes. Unit
and public deterministic tests separately cover raw perf host/CPU/workload
features, missing/invalid/truncated feature strings, query failures/no query,
side reversal and stale/WAL/JSON/fork boundaries. Do not claim those branches
were exercised merely because this SQLite live case passes.
