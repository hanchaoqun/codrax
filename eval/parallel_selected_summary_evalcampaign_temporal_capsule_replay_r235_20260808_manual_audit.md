# Selected Eval Manual Audit Scaffold

- date: 2026-08-09T03:00:30Z
- sweep_start_ts: 20260808-200028
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_frame_timeline_flow | PASS | eval/results/trace_query_frame_timeline_flow-20260808-200030 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 165s | 30 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | B400 生效：Finalizer 无 pretriage 自由叙事。B401 胶囊完整到达，模型首稿复制了 Note shape 与 3 个 temporal anchors；但 semantic family=generic+AxisFlow 的 source-call gate 仍把 temporal arrows 判成 missing_call_anchor，造成系统教学/校验自冲突，patch 删除图。正文仍把未授权的 UI/main/render-service/hardware 角色、消息/同步触发和 gap 调度成因写成事实；temporal-unproven 限定虽在，但语义越权未闭环。 |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260808-200030 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 180s | 37 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | rank/channel/row_identity 与 census rank_binding=not_provided 均到达，但模型仍把 unbound ×17 census 绑定给 #5、把 page_lock_timeout callsite 扩写成持有/等待页锁，并在 frame_evidence_status=absent 下断言 >8ms wakeup 导致错过 Vsync。链上主因人口/排序与链外背景分区总体正确，系统未改写模型正文；语义权限仍失败。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
