# Selected Eval Manual Audit Scaffold

- date: 2026-08-09T09:57:21Z
- sweep_start_ts: 20260809-025719
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260809-025721 | log_regex,answer_regex | none | 36s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 一批、零 repair/action failure，严格输出 `{"ids":["u1","u3"]}`。本轮走单个 `custom_transform`，证明修复后无回归且运行明显收敛，但没有生产命中 B427 的 `count + value_field` typed 红臂，故 B427 仍为 pending production replay。 |
| 2 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260809-025721 | answer_regex,answer_contains | none | 236s | 34 | read=11,repo_map=3,list=0,trace=0,source_lens=0 | midloop=5,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | 表格列出四个主 stage，但摘要错误声称完整四阶段均由 `runAnalyzePhase` 驱动；源码中该函数只 dispatch `StageAnalyze`。首稿把同一两条 call 证据重复成四轮并虚构 self-call/assignment，校验正确拒绝；patch 删除无证边后又在单条箭头标签里写 `StageAnalyze → Explore → Extract → Finalize`，图仍未表达各阶段真实执行边界。B422 的 ANY-edge 完成漏洞再次生产复现。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusions

- `data_json_strict_ids`: human PASS。严格 JSON、顺序和成员值均正确，一批完成；但本轮走 direct custom，并未触发 B427 新增的 typed admission 红臂，不能据此宣称 B427 已生产闭环。
- `read_combo_pipeline_sequence_table`: human FAIL。已有 `canonical_read_main_sequence=analyze -> explore -> extract -> finalize` 精确上下文和四阶段表格证据，但模型把“阶段 roster/order”错误提升成“同一函数逐阶段 dispatch”的调用结论。
- 本轮一次“成文校验未通过”不是合同冲突或过硬：它准确拒绝了重复复用单一 call-site、无证 self-call 和无证 assignment。不得通过调大 occurrence budget 或自动删边来消除这次重试。
- `EVAL-B422-FLOWCOVERAGEANYEDGE1` 的新增生产形是 label laundering：patch 保留两条真实 call anchor，却把未证的四阶段控制流藏进箭头显示标签。根因仍是完成条件只需任意一条 flow operation，而不是逐 requested member/attribute/relation 覆盖。
- 最优方案仍须 design-first：由证据生产者提供有来源、边界和 member identity 的 requested-member coverage；阶段 roster/order 与函数调用关系分轴，每个成员的请求属性分别是 `proved` 或 typed `unproven_boundary`。完成门只读该结构载体，不读取 request、thinking、summary、表格或 Mermaid 标签，也不由系统改写模型结论。
