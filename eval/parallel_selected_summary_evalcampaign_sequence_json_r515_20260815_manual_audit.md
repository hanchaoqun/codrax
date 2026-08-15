# Selected Eval Manual Audit Scaffold

- date: 2026-08-15T16:25:25Z
- sweep_start_ts: 20260815-092523
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260815-092525 | log_regex,answer_regex | none | 220s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 精确输出 `{"ids":["u1","u3"]}`；6 个数据批次完成 rule coverage、2 条 contribution、reconcile 和 final projection。模型首批把未来阶段的 reconcile/assemble 动作放入当前枚举，并一度把输出名 `ids` 当输入字段；动态 schema、阶段门和 typed deferred queue 一致地拒绝/拆批，后续按真实字段 `id` 恢复。无矛盾合同、无解释文字泄漏。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260815-092525 | answer_regex,answer_contains | none | 283s | 30 | read=7,repo_map=1,list=0,trace=0,source_lens=0 | midloop=10,inv=7/2,fin_reject=1,unavail=0,prune=0 | partial | B841 生产正证：Explorer 读取 `gate.go:131-150`，终稿保留两条真边 `buildAnalysisIR -> gate.RunWith`、`gate.Run -> gate.RunWith`，并明确两入口汇合、二者间无直接调用；Mermaid 可渲染。唯一 finalizer reject 正确拦下把 13 条 sibling call 冒充 principal path。末轮因 requested member-set 标签不够显式而新增一张 13 行表，与已有 supporting list 重复，属于模型成文冗余；不得据此放宽关系 authority 或由系统代写。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
