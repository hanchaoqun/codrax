# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T09:22:55Z
- sweep_start_ts: 20260828-022254
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_python_typo | PASS | eval/results/patch_python_typo-20260828-022255 | write_plan,write_patch_oracle | none | 51s | 26 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Plan-only 输出只修改 main.py 第 20 行，kind=patch 的统一 diff 与结构化 replace edit 均精确把 retrun 改为 return；Python dry-build 通过，并提供一个导入后调用 greet 的有界 probe。无 JSON 修复、replan、finalizer 拒绝或额外文件扩域。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260828-022255 | answer_regex,answer_contains,mermaid_edge_count,mermaid_incident_node_count | none | 527s | 37 | read=33,repo_map=3,list=0,trace=0,source_lens=1 | midloop=14,inv=9/0,fin_reject=2,unavail=0,prune=0 | fail | 最终 Mermaid 不合法：首稿把物理换行放入 `-->|...|` 标签，系统兼容层随后把三个续行误铸为 `codraxNode1["调用"] Set...| M`，它们既无箭头又有悬空 pipe。runner 因 6 条可解析边、10 个 incident node 误签 PASS；新增 incident-node 下界仍可被与请求主参与者无关的技术端点满足。模型已完成两轮关系修补，最后损坏由系统 source repair 引入，不能归为模型波动。33 次 read、60 explorer iteration、14 次 midloop、9 次 completion 调用和 2 次 finalizer reject 也显示读路径仍有高 churn。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
