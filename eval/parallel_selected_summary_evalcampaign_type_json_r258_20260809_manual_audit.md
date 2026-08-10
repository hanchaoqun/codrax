# Selected Eval Manual Audit Scaffold

- date: 2026-08-10T01:59:33Z
- sweep_start_ts: 20260809-185932
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260809-185933 | answer_regex,answer_contains | none | 171s | 22 | read=4,repo_map=3,list=0,trace=0,source_lens=1 | midloop=3,inv=2/0,fin_reject=1,unavail=0,prune=0 | fail | Runner 只检查名称/文件命中而假绿。首稿把 `LoopController -> implementer` 标成 implements，typed type-relation 门正确拒绝；patch 仅删除 `edge_anchors`，同方向可见边仍出厂，暴露“删权威元数据即可逃逸”。正文还把唯一方法 `Observe(... ) LoopSignal` 误写成 Observe、LoopSignal 两个方法。图可渲染但语义不合格。 |
| 2 | data_json_strict_ids | FAIL | eval/results/data_json_strict_ids-20260809-185933 | log_regex,answer_regex | none | 283s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 正确 answer=`{"ids":["u1","u3"]}`、5 decisions、3 source-backed rules、2 contributions、reconcile=pass 已存在，但 rules 晚于 item ledgers 生成且无 `rule_refs`。计数式 reducer 同时报 `next_stage=complete`、`allowed_next_actions=[]`，终验却报 `unlinked_source_rule_coverage`；5 次 repair 只能反复尝试已禁 custom_transform，最终预算耗尽。属 ledger 代次/依赖闭包 split-brain，不是 JSON 畸形或答案计算错误。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
