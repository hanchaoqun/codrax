# Selected Eval Manual Audit Scaffold

- date: 2026-08-09T02:03:57Z
- sweep_start_ts: 20260808-190356
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260808-190357 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 186s | 35 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | S37de 的独立 rows 原样进入 Finalizer：10.433ms 席 caller=not_provided，7.386ms 席 caller=fscache，census seat_binding=not_provided。但 rows 没有 rank/channel/row_identity，模型仍把 census 17+1 与 #5 caller 绑到 #3，写成“10.433ms=fscache/hmfs 文件缓存页等待”；又把 page_lock callsite 升为系统级页缓存竞争。B399 partial：需要稳定 seat_ref 与 root-cause rank 一一关联，census 明确 rank_binding=not_provided。根因人口本身仍只有链上，frame causality unproven 已披露。 |
| 1 | trace_query_frame_timeline_flow | PASS | eval/results/trace_query_frame_timeline_flow-20260808-190357 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 187s | 30 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=1,inv=2/1,fin_reject=1,unavail=0,prune=0 | fail | S37de gap/temporal 教学原样进入 prompt，但模型仍把三个 1ms gap 写成正常通信/无阻塞/无排队，并由 span 名扩写输入、布局、光栅化、GPU 提交。上游 perf-triage 的 model-authored cross_thread_flow 叙事仍作为 repairable Observation Ledger 与精确 trace_query 竞争，确认 B400。模型忽略 sequence Note 教学，画无 edge_anchors 的 flowchart；被正确拒绝后直接删图，确认 B401 需从 typed temporal rows 给 copy-ready body+anchors，不能再堆软句。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner `2/2 PASS`，人工 `0/2 PASS`；系统没有替换模型答案，但精确提示被重复、低权限叙事稀释。
- `EVAL-B398` 保持 partial：口径已正确提供，模型仍越权；根因不是再缺一句提示，而是 pre-triage model narrative 与 typed trace_query 同席竞争。
- `EVAL-B399` 保持 partial：独立行有效但 identity 不足；下一小批给 fact row 携带 board-local rank/channel/row_identity，并给 census 固定 `rank_binding=not_provided`。
- 新增 `EVAL-B400-PRETRIAGENARRATIVEAUTHORITY1=P1/HIGH`：当同一 runtime artifact 已有 deterministic trace_query authority 时，pretriage model-extracted 自由叙事只能作导航，不应在 Finalizer Observation Ledger 继续与 exact rows 争夺机理/因果权限；实测原始字段仍保留。
- 新增 `EVAL-B401-RUNTIMETEMPORALCAPSULE1=P1/HIGH`：pure-temporal frame 报告需要由 typed frame items/edges 提供 copy-ready Mermaid body 与完整 temporal edge anchors；这是 authoring aid，不是系统代写或答案 mutation。
- 根因资格仍为 `typed_on_chain_only`，adjacent/background 只作 support/额外排查；显式窗、自动补齐、唤醒链、排序、投影、窗内可消量与双轴均在。
