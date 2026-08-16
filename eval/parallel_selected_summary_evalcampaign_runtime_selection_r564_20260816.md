# Selected parallel eval sweep

- date: 2026-08-16T11:38:39Z
- sweep_start_ts: 20260816-043837
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | read_combo_answer_document_tools | FAIL | no_regex_match:(```mermaid|flowchart|graph[[:space:]]+(TD|LR)) | 113s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/read_combo_answer_document_tools-20260816-043839 |
| 2 | real_trace_h4_supply_thermal_witness | FAIL | no_principal_text_regex_match:((CPU ?=? ?4|cpu ?=? ?4).{0,240}(2\.10 ?GHz|2\.1 ?GHz|2100 ?MHz|2100000 ?kHz).{0,160}(上� | 216s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h4_supply_thermal_witness-20260816-043839 |

**Pass: 0 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 2**
