# Codrax Telemetry Aggregator

`eval/telemetry` summarizes runtime logs into the decision signals needed before changing analyzer/finalizer gates.

Usage:

```bash
go run ./eval/telemetry --format markdown ../customlogs
go run ./eval/telemetry --format json eval/results ../.codrax/logs
```

When no path is passed it scans:

- `eval/results`
- `.codrax/logs`
- `../.codrax/logs`
- `../customlogs`

The command accepts files or directories. Directory scans include `.log`, `.txt`, and `.out` files so customer snippets can be analyzed without renaming them.

Important counters:

- `Analyzer / provenance totals`: distribution from `[analyzer] entity provenance summary`.
- `Analyzer / blocklist shadow`: dropped generic entities and whether the deferred drop-to-noise path would have allowed search/shape usage.
- `Finalizer / contract violations`: `answer_contract_check section=... violations=N`, including non-tool-reject repairs.
- `Finalizer / repair kinds`: typed repair root causes from `repair_plan`.
- `Compatibility And Transport`: tool-parameter repair and LLM timeout/error lines.

Decision rule for the deferred blocklist downgrade:

Do not switch `GenericEntityBlocklist` from hard drop to typed noise until a representative log sweep shows that shadowed dropped entities are not needed as principal answer/search/shape surfaces. This tool supplies the aggregate evidence; it does not change runtime behavior.
