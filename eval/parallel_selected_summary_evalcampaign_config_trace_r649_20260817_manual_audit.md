# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T23:04:05Z
- sweep_start_ts: 20260817-160403
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_config_two_knobs_precedence | FAIL | eval/results/read_combo_config_two_knobs_precedence-20260817-160405 | answer_regex,answer_contains | none | 174s | 27 | read=10,repo_map=1,list=0,trace=0,source_lens=1 | midloop=8,inv=1/0,fin_reject=2,unavail=1,prune=0 | fail | 第一批多 mentioned-key 修复未命中生产：Analyzer 同时发 `config_mapping`、两个用户 bucket、`has_per_member_table=true` 与 `is_category_enumeration=true`，旧 set-valued 前门先关闭 exact-resolution，semantic view 仍为 false。Explorer 第 6 轮已主动请求搜索 `PipelineSettings\\s*\\{|MaxRetriesPerStage:\\s*[0-9]`，但当轮工具面已只剩 completion/evidence，grep 被拒；最终把 CLI sentinel 0 当代码默认值，并把示例注释 50/2 错写成 100/3。两次 finalizer 拒绝只是表格列数不齐，合同正确；根因是有限命名配置集合与广义枚举未分型。 |
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260817-160405 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 188s | 36 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式窗 2.000..2.020s、四节点三边唤醒链、链上 #1 threadpool-400 IO wait 11ms、三段互斥 runnable 各 1ms、目标自身 20ms S 态与 frame/deadline 未证边界均保留。主因只来自 typed 链上席，邻近/背景未晋升；Trace 因果投影和自动补齐正常，无固定 4ms 活跃流降级。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner/human均为 `1 PASS / 1 FAIL`。
- r649 精确否证第一批只修“多候选分支”还不够：有限配置表会被上游 set-valued predicate 提前关闭，必须在同一 exact-resolution authority 中先识别 typed finite named config set。
- Trace 是反向保护正证，读模式配置修复未影响显式窗、因果投影、链上-only 根因或系统补齐。
