# HMC IO载体 / 异状态图关系：固定版本人工审计

- date: 2026-09-21T04:20:30Z
- sweep_start_ts: 20260920-212027
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_io_state_account_20260920

固定干净版本 `de47561c847d`，快照 `.codrax/tmp/codrax-selected-20260920-212027`；2并行×1，未修改case/oracle，未补第三次追绿。机器2/2不代替人工验收：**人工0/2，两例FAIL保留**。后续导航修复不在本版内。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/hmc_io_state_account_20260920/trace_query_wakeup_causal_io_chain-20260920-212030 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 160s | 45 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 错把可消除候选最大说成实际占时最长；调用点推资源机理；Binder零匹配越权排除；系统PIC段头另有定位包络误述 |
| 1 | trace_query_business_marker_io_chain | PASS | eval/results/hmc_io_state_account_20260920/trace_query_business_marker_io_chain-20260920-212030 | log_regex,typed_trace_projection_count,trace_attachment,primary_answer | perf_triage+trace_query | 354s | 43 | read=0,repo_map=0,list=0,trace=16,source_lens=0 | midloop=1,inv=2/0,fin_reject=1,unavail=0,prune=0 | FAIL | 子业务恢复但50/53ms混账；请求/等待/运行量仍拼算；bytes误写扇区；旁路存在但无可选候选 |

## 明确时间窗（独立复审）

primary 为结果目录 `run-1.primary.md`，最终报告 `.codrax/output/20260920-212308.071-91783.md`，日志 `run-1.logs/codrax-20260920-212033-000-91783.log`。

- primary:1/3称IO11ms是链上实测最长；同轮成文输入:2519–2524与最终:126/132明列cookie S17、network S14。11只是在已确认可消除候选中最大，不是全部实际占用最大。
- primary:36从内核调用点断言文件系统缓存页读取并推预读/缓存优化，越过primary:21及日志:2537的机理边界。:38以verified Binder=0排除主要等待机制，但日志:2292/2433明确未关联sleep不等于non-Binder证据。
- primary:1四跳实际三边四节点，:13三CPU实际列出四CPU；:40仍露出schema/runtime_work_relation内部词。
- 改善：目标S20、IO2.003..2.014=11ms与链准确；四候选完整保留，旧S与PIC“同状态物理重叠”说明消失。旁路available且11ms/查询窗保持，旧虚构再睡/再唤醒消失；不以此将整答判PASS。
- 新确认的系统残留：最终:105确定性发射“3席·成员区间重叠”。真正三段Runnable在2.014..15、2.016..17、2.018..19互斥；仅E4/E6/E8定位包络重叠。源头`TraceAnswerDirectionSectionArithmetic`→`TraceCausalProjectionFaithfulEnvelopeOverlapMS`未区分计量分量和定位包络。单独修显示边界，不与本版SMR1窄修混账。

## 业务窗口（主线完整审计）

primary 为结果目录 `run-1.primary.md`，最终报告 `.codrax/output/20260920-212622.494-91794.md`，日志 `run-1.logs.all.log`。

- primary:13恢复LoadDocumentIndex，:25–27正确8/1/31ms；IO完成→worker→app链保持，后台47ms写未当主根因。
- primary:3/51仍把OpenDocument50ms运行写5–7ms并虚构首帧热身；:17–22将53ms查询窗7/1/44（加1ms未知）冠以业务窗。正确marker-local5/1/44已进入探索日志:1598/2529/2620/2652，不能简单归为工具未供给，也未证明全是模型波动。
- primary:5把业务B开始1.0045当IOissue，:44却正确1.005；:30/52将重叠的31/44说相邻；:40说1.049在1.050之后，并越权称后台为并发干扰；:44/46把32768/65536bytes写为扇区。
- primary:55拼出44=35+8≈43并虚构1ms容差；请求存续35ms与worker运行部分重叠，不能如此串加。:57保留同步对象未证限定，不抵销前述错误。
- 系统Trace投影、观测与IO中性载体解释仍在。日志:3882/3883命中新的次生观测解释；本轮是背景载体，不冒充特定31ms载体的live正针。
- mandatory根因旁路确实生成，但schema2为`unavailable/no_selectable_typed_on_chain_candidates`和空数组；本轮没查root_cause_rank，不能称内容已通过，也不是文件丢失。
- 成文仅一次拒绝：日志:4302–4304，额外patch的replace_blocks[1]缺id，原完整答案保留；JSON教学/事务状态继续只读核对，不因最终有答案就抹去拒绝。16次查询、两次完成交接的过程成本保留。

两例无空答案、无活跃流强制超时；均有摘要绑定的正文归属收据及Trace投影。测试通过仅签子修复，HMC父项和上述人工FAIL继续开放。
