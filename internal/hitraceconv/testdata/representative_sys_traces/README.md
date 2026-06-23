# Representative sys trace fixtures

This directory is the committed fixture gate for Batch 6C3.

Do not place customer-private captures here unless redistribution is explicitly
approved. A fixture must include a manifest JSON next to the capture:

```json
{
  "id": "vendor-no-perf-001",
  "description": "Short description of the captured scenario.",
  "input": "vendor-no-perf-001.sys",
  "input_sha256": "<sha256 of vendor-no-perf-001.sys>",
  "trace_db": "vendor-no-perf-001.trace.db",
  "trace_db_sha256": "<sha256 of vendor-no-perf-001.trace.db>",
  "capture_class": "redistributable_real_capture",
  "redistribution": "approved_internal",
  "approval_ref": "approval record or ticket id",
  "trace_kind": "no_perf_sys",
  "expected": {
    "min_events": 1,
    "builtin_parity": true,
    "coverage": [
      {"family": "scheduler", "table": "sched_slice", "min_rows": 1}
    ],
    "event_types": ["sched_switch"]
  }
}
```

Rules:

- `input` and `trace_db` must be relative to this directory.
- `capture_class` must be `redistributable_real_capture`; synthetic fixtures
  belong in package tests and cannot satisfy the representative retirement gate.
- `redistribution` must be one of `public`, `approved_internal`, or
  `approved_customer`.
- `approval_ref` must identify the approval/license/customer record that makes
  the fixture redistributable.
- `input_sha256` is required and must match the committed `.sys` file.
- `trace_db_sha256` is required whenever `trace_db` is present and must match
  the committed SQLite sidecar.
- `trace_db` is the deterministic trace_streamer SQLite export used by normal
  Go tests. Manual verification with a real trace_streamer binary should be
  recorded in the delivery plan before using the fixture to retire parser code.
- `trace_kind=no_perf_sys` may request `builtin_parity=true`; trace+perf
  fixtures should validate SQL-first auto behavior plus built-in raw fallback
  when SQL is unavailable or fails.
- Text systrace files such as `../customlogs/xxx_all.systrace` belong to
  trace_query evals, not this converter parity gate.
