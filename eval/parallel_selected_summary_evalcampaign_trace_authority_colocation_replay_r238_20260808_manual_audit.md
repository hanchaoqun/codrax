# Selected Eval Manual Audit Scaffold

- date: 2026-08-09T03:53:07Z
- sweep_start_ts: 20260808-205305
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260808-205307 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 161s | 44 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | ranked/unbound 分区与新 authority 字段真实到达，但模型仍把 ×17 census 搬进 #3 bullet。#3/#5 的 18.119 错和被避免，却改为把 #1+#2 写成“合计约43ms/锁与优先级43ms级候选”，再把 #3+#5+#6 写成 IO 约24ms；root board 早已说明同修向取最大、所有 seats 不相加，说明仅 wait-callsite 行携带 cross-seat authority 覆盖不全，主 root-seat 行也必须同位带 aggregation authority。另称 page_lock_timeout“引起”#7 io_wait，typed 只授权该席 kernel wait callsite。 |
| 1 | trace_query_frame_timeline_flow | PASS | eval/results/trace_query_frame_timeline_flow-20260808-205307 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 168s | 31 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=3,inv=2/0,fin_reject=2,unavail=0,prune=0 | fail | 将三项权限串塞进每条 Mermaid Note 增加了机械负担：模型首稿没有复制 edge_anchors；第一次 patch 的可见 body 只有两条 cross-thread arrows，却复制三条 anchors（多出 t1 self-edge），连续两次成文校验未通过后才成功。最终图可渲染且 post-check 无旧 source-call 冲突，但正文继续把 stage 扩成输入/动画回调、RenderService 提交、GPU 硬件绘制/合成。权限同位方向保留，载体应改为同胶囊紧邻 compact typed JSON，恢复短 Mermaid Notes。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner: 2/2 PASS；human: 0/2 PASS。
- `EVAL-B409-TEMPORALCAPSULECOGNITIVEOVERLOAD1=P0/HIGH` 新确认：语义 ceiling 直接进入每条 Mermaid Note 造成 copy burden，2 次 pre-emit reject。应保持权限与 capsule 同位，但从 Mermaid body 移到紧邻 compact typed JSON；这也是 JSON 教学“单源、少心智”的更优载体。
- `EVAL-B403` 未改善；正文仍越过 item-stage/thread-role/internal-work 边界。
- `EVAL-B404` 分区已真实接线但单独不足，census 仍被模型搬进 #3。下一步 causal-diagnosis Finalizer 不再消费 unbound census 的具体 roster；Explorer 与 deterministic report/evidence index 继续保留，显式请求 recorded_reason/count 的 bounded fact 查询仍可见。
- `EVAL-B407` 继续确认且扩域：不仅 wait seats，所有 root-cause ranked seats 都必须同位携 `cross_seat_aggregation_authority=forbidden`；同修向 lane 的可恢复值是最大 seat，不是和。
- B402 仍闭环；新拒绝是 capsule copy/anchor 基数，不是 post source/runtime owner 冲突。
- Trace 根因人口仍为 typed on-chain only；背景信息没有进入系统 root population，但模型正文仍需通过更精简、精确的上下文约束改善。
