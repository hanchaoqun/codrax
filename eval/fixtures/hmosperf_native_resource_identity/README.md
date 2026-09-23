# Native resource identity: exported-text acceptance

This authored synthetic text follows the native-hook export wire. It checks
model-visible querying and explanation of resource metadata, not admission of a
raw binary or SQLite database. Only `events.systrace` is attached; the analysis
repository is `stub_repo`, so this README and the case oracle are not evidence
available to the model. Public converter tests must independently verify the
same source values through database export and parser/query round trips.

## Independent expected observations

The requested interval is 0.0005–0.0055 seconds. It contains five resource
operation instants and five separate cumulative-counter observations.

| Time (s) | Operation | Source signed address | Address bits | Subtype ID | Source subtype name | Counter observation |
|---|---|---|---|---|---|---|
| 0.001 | AllocEvent | 9007199254740993 | 0x0020000000000001 | 7 | `缓存\|纹理` | HeapSize=8192 |
| 0.002 | FreeEvent | 0 | 0x0000000000000000 | 0 | empty TEXT (`""`) | HeapSize=4096 |
| 0.003 | MmapEvent | -1 | 0xffffffffffffffff | 8 | SQL NULL (`null`) | MmapSize=4096 |
| 0.004 | AllocEvent | NULL | NULL | 99 | not resolved; name field absent | HeapSize=4128 |
| 0.005 | MmapEvent | -9223372036854775808 | 0x8000000000000000 | NULL | no subtype reference or name | MmapSize=8192 |

The source fixture corresponds to a dictionary with unique entries
`7 -> "缓存|纹理"`, `0 -> ""`, `8 -> SQL NULL`, and
`10 -> "窗外文件描述符"`; ID 99 has no entry. The string at ID 7 is encoded as a
JSON string with its pipe escaped to `\u007c`, so it cannot add another marker
field or physical trace line. The decoded name contains a literal pipe, not the
six characters `\u007c`. A model may also show the exact escaped source string
if it explains that representation correctly.

The FD_Open_Event at 0.006 seconds and NativeHook_FD_Active=1 are outside the requested
window. They must not be counted as in-window observations. The scheduler lines
only provide a plausible trace envelope; they do not prove a resource operation
caused a scheduling delay.

## Required human audit

- Preserve signed source integers and their 64-bit bit patterns exactly. No
  floating-point rounding, absolute value, or reinterpretation of the bit
  pattern as a negative magnitude. In particular, -1 maps to all-one bits,
  not to 1. This preservation alone does not establish that the address is a
  valid allocation: the underlying source can also carry sentinel values.
- Keep zero distinct from NULL. A negative source address is not by itself
  proof of corruption, but the fixture does not prove it is usable memory.
- Keep empty TEXT, explicit dictionary SQL NULL, an unresolved reference, and
  an absent reference distinct. For ID 99 the exported text supports only
  “name not resolved”; it does not reveal why resolution failed. Do not invent
  a name, treat an unknown subtype as anonymous memory, or apply an ID from a
  different capture's dictionary.
- Count five I observations, not ten operations. HeapSize is 8192 -> 4096 ->
  4128; MmapSize is 4096 -> 8192. Do not combine those into a single cumulative
  series, add the sampled levels, or count C records as additional operations.
- A source stack ID is not a resolved function, a resource end timestamp is
  not an execution interval, and the recorded quantities are not independently
  proved allocation sizes for every resource family. These records alone do
  not prove a long-running function, leakage, or a causal-chain root cause.
- Audit the complete answer, row/value associations, finite-window coverage,
  citations, and actual finalizer context. The case's string-based smoke
  checks are only a floor, not a semantic PASS. No fixed tool order, number of
  calls, root-cause selection, or diagram is required.
