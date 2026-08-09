# Selected Eval Manual Audit Scaffold

- date: 2026-08-09T03:35:45Z
- sweep_start_ts: 20260808-203542
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260808-203545 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 137s | 41 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 根因人口与排序仍为 typed on-chain，pressure/其他进程 IO 留在背景，frame_evidence_status=absent 也正确披露；但模型再次把 rank_binding=not_provided 的 ThreadPoolForeg 全窗 caller census（×17/×1/×1）贴到 #3 cause-unproven 席后，并在摘要把 #3 10.433ms 与 #5 7.386ms 重组为“D/IO 合计有效归因 18.119ms”。两席明确不可相加，且即便机械相加也应为 17.819ms。代表窗又写成“主线程同步等待 CookieMonsterCl 响应”，与 direct_blocking_decision=not_established 冲突。 |
| 1 | trace_query_frame_timeline_flow | PASS | eval/results/trace_query_frame_timeline_flow-20260808-203545 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 146s | 31 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | B402 已闭环：一次 emit 即接受，pre/post 均无 diagram_call_edge_unproven、diagram_relation_label_only、重试、第一稿保留或图 caveat，Mermaid 与 3 个 temporal anchors 完整。人工仍失败：模型把 stage/span identity 扩写成 UI 调度帧渲染、RenderService 光栅化/命令下发、GPU 画面合成，并称 RenderFrame 是提交入口；typed authority 只证明 item stage、thread identity、interval 与 temporal adjacency，owning_thread_role/internal work/call relation 均未提供。显式窗仍触发完整报告/补采合同，未执行钻取 caveat 属现有显式窗优先设计，本轮不擅自收窄。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner: 2/2 PASS；human: 0/2 PASS。
- `EVAL-B402` production closed：runtime temporal block 的 pre-emit 与 post-finalizer owner routing 已一致，source sibling 门未放宽。
- `EVAL-B403` 在无合同噪声下再次复现，且不仅是图层：正文把 item-stage/span name 升级成 owning-thread role、内部工作和提交/完成机理。下一步应把每个 item 的 `stage_role` 与 `owning_thread_role_authority` 同位封装，减少模型跨字段迁移；不扫描最终原文硬拒。
- `EVAL-B404` 再次复现。当前 wait context 虽逐行写了 rank_binding=not_provided，但仍把 unbound census 紧邻同 subject 的 ranked seats，模型继续跨席。下一步按 typed carrier 把 ranked seats 与 unbound window inventory 分区发布，保留完整证据但降低错误邻接。
- 新确认 `EVAL-B407-TRACESEATRECOMPOSITION1=P1/HIGH`：模型对明确 non-additive 的 ranked seats 自行重组，并产生 18.119/17.819 算术漂移。它不是单数字问题；根修应让 ranked seat capsule 自带 aggregation_authority=forbidden，并把 unbound inventory 移出 seat 区，模型结论仍由模型形成。
- `EVAL-B406` 本轮未复现，继续观察，不据单轮关闭。
- Trace 根因仍只能来自 typed on-chain seats；邻近事件、unbound census、pressure/IRQ 等只能作背景支撑或额外排查方向。
