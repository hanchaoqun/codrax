# Native resource metadata: exported-text acceptance

This is authored synthetic text shaped like the public native-hook exporter.
It is not a raw binary/database admission test. The field combinations reuse
`TestNativeHookMetadataPublicExportAndSearch` in
`internal/hitraceconv/streamerdb_native_hook_metadata_test.go`; the fourth,
out-of-window resource is a boundary sentinel.

Only `events.systrace` is attached. The analysis repository is `stub_repo`,
so this oracle and the case file are not available as answer evidence.

Independent facts for the explicit 0.0005–0.0035 second window:

| Occurrence | Event | Source quantity | Source stack key | Source resource end (ns) | Heap counter |
|---|---|---:|---:|---|---:|
| 0.001 s | AllocEvent | 9007199254740993 | 9007199254740995 | 9223372036854775807 | 8192 |
| 0.002 s | FreeEvent | 0 | -1 | 0 | 4096 |
| 0.003 s | MmapEvent | NULL | NULL | NULL | 4096 |

- Exactly three resource observations are in the requested window. FD_Open_Event
  at 0.004 s is outside it, even though its resource end is later.
- The source quantity is independent of the cumulative HeapSize counter. Do not
  reconstruct one from the other, negate FreeEvent's reported zero, or round the
  integers through floating point.
- NULL means no value, not zero; end=0 does not independently prove a release.
- A stack key is an unresolved source identifier, not a resolved function.
- All NativeHook events are instants. Their source resource end is not an
  execution span or a proved leak, wait, or chain root cause.
- The primary-answer assertions are a floor. Human review must check each
  value's row association, missingness, units, citations, selected window,
  finalizer context, and ordinary user-facing explanations. Do not require a
  prescribed query sequence or count of tool calls.
