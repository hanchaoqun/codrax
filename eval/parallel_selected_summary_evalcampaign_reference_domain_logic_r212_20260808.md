# Selected parallel eval sweep

- date: 2026-08-08T13:10:28Z
- sweep_start_ts: 20260808-061027
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | data_multifile_reference_projection | PASS | - | 182s | 0 | 0 | 0 | 0 | 2 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_multifile_reference_projection-20260808-061028 |
| 2 | qf_logic_view_read_pipeline | PASS | - | 236s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260808-061028 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**

## Human audit

- `data_multifile_reference_projection`: PASS。最终 `17,0,5`、贡献、补零与对账均正确；S37cb compatible-field candidate 与 B349 参数 schema 均未命中生产修复臂。
- `qf_logic_view_read_pipeline`: FAIL。四阶段/职责大体正确且 Mermaid 可渲染，但核心数据流把 `FinalAnswer` 错画为经 `applyStageOutput` 写入 `BusContext`，还把 `StageBinding.Terminal=true` 错述为 BusContext 写入；runner 的 contains/regex oracle 未覆盖关系真值。
- Filed `EVAL-B353-ARCHROLEFLOWAUTH1=P1`: separate conceptual component responsibility/data-flow from single-winner role lookup, and require field-transfer/exclusion guidance from typed source relations rather than carrier field membership.
