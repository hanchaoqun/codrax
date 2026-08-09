# Selected Eval Manual Audit Scaffold

- date: 2026-08-09T03:21:50Z
- sweep_start_ts: 20260808-202149
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_frame_timeline_flow | PASS | eval/results/trace_query_frame_timeline_flow-20260808-202150 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 126s | 30 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | S37di 已使 pre-emit 接受同一 report-local 的 4 item/3 temporal-edge 图；但 post-finalizer V2 oracle 仍直接调用旧 source matcher，并套用 generic DiagramPlan 的 guard>=1，发出 diagram_call_edge_unproven + diagram_relation_label_only。finalizer 被迫 patch，重试预算耗尽后系统又保留整份第一稿并追加降级 caveat；runner rejects=0 没覆盖这条 post-check 拒绝。模型语义仍把 span name 扩写为 UI/RenderService/GPU 线程职能与消息/绘制机理，typed frame_flow 只证明时序邻接。 |
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260808-202150 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 173s | 44 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 正面：rank/channel/row_identity 与 rank_binding=not_provided 到达后，本轮模型未再把全窗 census 绑定到 #5，也未把 page_lock_timeout 扩成 holder/resource；on-chain #1..#7 与背景分层基本保持。失败：摘要称 34579.595130 VSync 在帧边界内，后文又准确写明它超出 34579.587805 上界；frame evidence 仍为 unproven/absent，故该窗不能把这条邻近 VSync 当帧内结论。模型还把 sleep 概括为普通帧节拍等待，typed 数据只排除了 D/io_wait，并未证明全部 sleep 的机理。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner: 2/2 PASS；human: 0/2 PASS。runner 的 `fin_reject=0` 只统计 pre-emit，漏记 post-finalizer V2 oracle 的确定性拒绝。
- `EVAL-B402` 不是生产闭环：pre-emit owner 路由已生效，但 post-check 仍重新分类 runtime temporal 图，形成同一答案先接受后拒绝的合同冲突。
- `EVAL-B403` 继续确认：模型把 typed span/temporal rows 扩写成线程角色与内部工作机理；在合同噪声清除前不继续堆同义 prompt。
- `EVAL-B404` 本轮没有复现跨席 census 绑定，按模型波动观察；typed unbound 信号已到达，不据单轮关闭。
- 新增 `EVAL-B406-WINDOWRELATIONCONSISTENCY1=P1`：同一答案把 typed 窗外事件先写成窗内、后写成窗外。最优方向应是给模型提供按 selected-window 归类的 typed event relation，而不是扫描答案原文做矛盾硬门。
- Trace root population/order 仍只来自 typed on-chain 席；邻近 VSync、pressure、IRQ、未绑定 census 只允许背景/补充排查，不得升为根因。
