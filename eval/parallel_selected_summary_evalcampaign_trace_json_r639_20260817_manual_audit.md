# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T20:08:03Z
- sweep_start_ts: 20260817-130802
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260817-130803 | log_regex,answer_regex | none | 128s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 最终答案严格为 `{"ids":["u1","u3"]}`，没有畸形 JSON、恢复稿或正文污染。流程用了 6 轮且 `data_action_failed=4`：首轮先发 `custom_transform` 而非必需的 `derive_rules`；后续又把当前 rank 与未来 rank 的动作放在同批，被动态 schema 正确拒绝并由 compact repair 收窄。正确性闭环，另记 B1009/P2 教学心智与效率债，不降低 rank 门。 |
| 1 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260817-130803 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 228s | 36 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | B1006 获生产正证：Finalizer 不再收到 raw trace，主窗恢复精确 20.000ms，节点/边计数也正确；显式窗、因果投影、自动补齐与四席双轴完整。仍把仅证实为 pre-wakeup on-chain work 的 IO 席描述为直接级联根因。精确上下文存在两处自冲突：raw 已隔离但 perf-triage 仍称“下一节有 raw/请读 raw”；排序板无条件要求“State root causes”，却与席位级 bounded/no-conclusion caliber 冲突。已按 typed stage/caliber 做软合同根修，待下一轮回放。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
