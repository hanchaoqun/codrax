# r1053 人工审计：显式窗根因与代码时序

- date: 2026-09-10T11:23:57Z
- sweep_start_ts: 20260910-042356
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

基线 `8aef20864155`，11:23:57Z 开始、11:35:29Z 两题全部结束。严格并行2；原case、预算和oracle未改。机器0/2通过不等于两题都没有答案；下列人工结论分别核对正文、上下文、查询/源码及真实修补过程。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h6_channel_mixed_display | FAIL | eval/results/real_trace_h6_channel_mixed_display-20260910-042357 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 208s | 60 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail（局部） | 投影/旁路/树完整；旧图符机针未命中，另有真实跨行相加、机制误称和系统口径标签缺项 |
| 2 | qf_sequence_analyzer_gate | FAIL | eval/results/qf_sequence_analyzer_gate-20260910-042357 | answer_regex,answer_contains | none | 692s | 40 | read=6,repo_map=0,list=0,trace=0,source_lens=0 | midloop=17,inv=5/0,fin_reject=20,unavail=0,prune=0 | fail | 恢复旧模型草稿仍含虚构同级互调；合法孤儿收尾被系统后置规范化误拦，B1647有效RED |

## Trace：什么保留、什么未通过

工件 `.codrax/output/20260910-042723.261-2448.{md,html,root-causes.json}`；旁路9386字节、5个模型所选条目。MD105起投影仍在，109头行及后续链/背景分区完整；Mermaid经离线真实库渲染成功，SVG29190字节（`.codrax/tmp/20260910-r1053-h6-mermaid-render.json`）。机评缺的是旧`根因排序#1`/`❶`针，现文为`➊`；不修改oracle冒称通过。

原目标17267，13762.791708..13763.024898，233.190ms。原四态Running157.248/Runnable5.604/S70.338/D0，折算58.320保留；本轮实际IO集合为47条/12.658ms，不是旧默认45/11.141或其它查询49/14.923。实占、可消量、请求驻留、完成闭合阻塞各保自身尺；frame未证限定与业务线索在。仅两次trace_query，系统补齐保留。第一完整模型稿接受，随后只有一次summary关系说明patch；成文零拒绝，最大上下文60.4%，不存在预算耗尽丢图。

未全过的真实问题：MD17自行把IO3.602与D3.598相加成约7.2ms（日志1876/1883/1886已有不可跨行求和引导）；MD61把某Jank项Running3.299当整项实测墙钟，其整项折算4.710来自1.759+2.951；旁路把2.34GHz热限制说成policycap，以及把`dma_fence_default_w` caller升成具体资源对象，输入已明确相应边界。这些先记模型数值/机制解释残余，不追加正文扫描硬门。有限IO事实B1644读者未自然命中，不能倒签该生产分支通过。

系统确认两项：共享技能实际入模（log1435）的IO-LATENCY ROLE WORDS把所有IO项硬说成请求issue→complete，漏掉本次完成闭合S阻塞尺，属B1645；MD81/586两处系统状态面将标记分类器泛称“S态IO等待”，属B1646。MD584已有一般口径说明，raw查询也有scheduler_marked_only，不称全链零披露，但局部标签确实不足。

## 代码时序：先分清模型错与系统错

工件 `.codrax/output/20260910-043527.206-2436.md`。源码builder在analyzer.go:2725调用RunWith，gate.go:134的Run在135也调用RunWith，不存在builder→Run；Normalize2323、Amplify2348、Compile2530均由builder同级调用。模型导语正确识别汇聚，但首稿MD23–25把同级函数画成互调，Compile引用2651注释而非2530，函数清单未保实际顺序且缺早退/risk/hdp/bind。旧草稿恢复后这些错误仍出厂，人工FAIL独立于机评的degraded标志。

系统正确行为：日志1834–1838将2651证据降为text_reference_fact/visible_text_only，未授call权；1839–43对RunWith错行做精确邻近恢复；3652–55拒绝首稿虚构调用。模型此前错引用片段、孤儿名单误选n3、重复已消费failure_refs、畸形JSON均有独立精确拒绝依据，不能全归系统。

独立系统P1 B1647：iter1（3793–97）有效关系patch已进入未发布retry base；iter5/6（4004/4041）只执行n4的remove_if_isolated，却报未修改n1→n2的unlisted_relation_removed/added。公开Execute最小测试已复现：同组旧空方法身份与新增完整身份并存，后置recipe补齐旧身份，lease补偿器对混合基准未覆盖，错误触发越界门。`.codrax/tmp/20260910-b1647-public-overlay-red-verified.log`为有效RED1.674s；不是凭日志猜测。修向仅稳定系统自行补齐的身份变化，真实跨方法/新增删除边仍拒绝。

末次20轮重试耗尽后log4632–36恢复旧草稿并披露降级；不是活跃SSE经过4ms/4分钟即降级。本轮1200s显式eval上限未触发。不能靠扩大20轮上限或改写模型图来修补B1647。

## 后续优先级

P1先修B1647阶段基准自冲突、B1648现代IO已评估false借旧target-self路径上链；P2修B1645三尺教学、B1646标记等待局部名称。B1648来自同轮原生负控，非H6正样本自行证明缺陷。生产修复、回归收据和剩余边界统一记录在 `docs/design/eval_priority_campaign_audit_20260730.md` §123.1733–1735及后续收账；不改原模型正文、不按类型拟合，不将本轮工程修复提前称live复验通过。
