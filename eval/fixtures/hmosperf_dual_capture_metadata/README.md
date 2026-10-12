# Two capture metadata sources

Run `bash generate.sh` once with SQLite available. The generator refuses to
overwrite either existing output. It copies the existing common-window
measurement fixtures, then adds only the adjacent SQL metadata. Existing
measurement populations and original fixtures are unchanged.

Both inputs are healthy SQLite files despite their `.data` suffix. Baseline
has three valid INTEGER device_info values. Current has one valid integer,
one NULL and one REAL; its REAL 60.0 is intentionally not SQLite INTEGER 60.
Both contain bounded parser metadata, and current contains a duplicate name.
Arbitrary meta keys are literal collector diagnostics, not certified hardware
or workload identities. The independent case oracle is not attached to the
model.
