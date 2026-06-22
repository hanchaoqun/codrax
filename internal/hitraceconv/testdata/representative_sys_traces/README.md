# Representative sys trace fixtures

This directory is the committed fixture gate for Batch 6C3.

Do not place customer-private captures here unless redistribution is explicitly
approved. A fixture must include a manifest JSON next to the capture:

```json
{
  "id": "vendor-no-perf-001",
  "description": "Short description of the captured scenario.",
  "input": "vendor-no-perf-001.sys",
  "trace_db": "vendor-no-perf-001.trace.db",
  "redistribution": "approved internal fixture",
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
- `trace_db` is the deterministic trace_streamer SQLite export used by normal
  Go tests. Manual verification with a real trace_streamer binary should be
  recorded in the delivery plan before using the fixture to retire parser code.
- `trace_kind=no_perf_sys` may request `builtin_parity=true`; trace+perf
  fixtures must stay SQL-only.
- Text systrace files such as `../customlogs/xxx_all.systrace` belong to
  trace_query evals, not this converter parity gate.
