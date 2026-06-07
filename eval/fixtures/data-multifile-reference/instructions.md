Use observations.csv as the source record set.

Use labels.csv to map each observation raw_label to canonical_label. Exclude
records whose active field is false. For every included source record, add
value to the total for its canonical_label.

Use targets.csv as the required output reference set. Output one total per
target row, in target order. If a target canonical_label has no included
records, output zero for that target.

The final answer must be only one comma-separated line of numbers. Keep the
calculation auditable with contribution records and a reconciliation check.
