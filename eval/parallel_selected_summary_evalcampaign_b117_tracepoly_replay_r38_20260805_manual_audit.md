# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T15:09:54Z
- sweep_start_ts: 20260805-080953
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260805-080954 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 184s | 40 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B116 生效：`Structured Aggregate Facts` 不再携带 explorer 的错误综合，最终尾界位于最后且明确写出 `causal_conclusion=unproven`、跨 row 不可加、wakeup 不证明阻塞；显式窗、投影、补齐和双轴均保留。但模型仍把 CookieMonsterCl wakeup 候选叫“直接阻塞原因”，把 D-state/io_wait/io_latency 三个不同 row 合成 24.5ms，并把 wakeup 往来升级为紧密交互/延长链路。runner 未覆盖这些语义越证。 |
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260805-080954 | answer_regex | none | 226s | 20 | read=3,repo_map=2,list=0,trace=0,source_lens=1 | midloop=10,inv=4/0,fin_reject=2,unavail=0,prune=0 | fail | B117 两项生效：完整成员集没有第二份系统副本；diagram reject 从 9 降至 2，模型第 3 轮按 typed 提示主动 `remove_block_ids=["d1"]`。但接受后旧 rejected 图仍被系统作为“系统保留内容”附回，直接否定模型的显式删除并重新展示无 typed edge authority 的图；确认 mutation-aware attachment gap。文字主链与 fallback 基本正确。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Gap decisions

- `EVAL-B115-TRACEAGGAUTH1=production-covered`：完整 Trace 的 runtime-only model aggregate 已退出 Finalizer 事实合同；本轮错误不再来自该入口。
- `EVAL-B115-ENUMACCUM1=production-covered`：最终没有重复六成员清单。
- `EVAL-B115-DIAGRAMCHURN1=production-covered`：拒绝 9→2，第二次失败后模型立即走合法删除车道；不需要降低调用边 hard authority。
- `EVAL-B117-ATTACHREMOVE1`（P0，confirmed/implemented-next）：accepted patch 明确删除 diagram 后，旧 rejected-model diagram 仍因“最终 doc 无图”而通过恢复附件过滤。应以 typed `remove_block_ids` + prev block kind 作为删除意图，只清除 model-rejected diagram attachment；system-authored attachment 保留独立权限。
- `EVAL-B117-TRACEDECEXEC1`（P1，confirmed third witness）：精确尾界存在且旧 aggregate 已移除，模型仍跨 row 求和并混淆 wakeup/阻塞。不能新增答案关键词 hard gate；下一步应把 typed 直接阻塞权限和每个 fix-direction 的单席 leader/ceiling 收敛为尾部小型 decision ledger，减少模型从 55K-token 明细自行推导关系的自由度，仍由模型给结论。
- `EVAL-B107-ENDPOINTAMBIG1`（P1，confirmed）：Python FFI 名、PyO3 wrapper 与 Rust 同名 core function 之间仍缺 typed identity bridge；本轮删图是诚实降级，不等于 endpoint gap 已解决。
