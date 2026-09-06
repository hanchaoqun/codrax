# r1022 人工审计：Trace 候选旁路与类型关系图

- date: 2026-09-06T07:20:53Z
- sweep_start_ts: 20260906-002042
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

基线 `25a4f4362`，严格两路，已读模型 full/patch 参数、最终答案、系统上下文、Trace根因JSON与源码关系。机器2/2保留，人工不能据此重签全部正确。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260906-002053 | answer_regex,answer_contains | none | 182s | 35 | read=13,repo_map=2,list=0,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass with caveats | 12生产实现/文件/方向正确，官方Mermaid渲染通过；阶段结束措辞粗略；单次仅补member_set却误称可见文件清单遗漏，B1569 |
| 1 | real_trace_h11_cross_direction_overlap | PASS | eval/results/real_trace_h11_cross_direction_overlap-20260906-002053 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 451s | 54 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 答案/投影/根因JSON恢复且B1562生产正证；但跨向关系臆断、总览漏优先级方向、事实卡价格冒充实测，不能只归咎模型 |

## Trace 事实与问题分层

- 最终 `.codrax/output/20260906-002822.086-79290.md` 和同名 `.root-causes.json`（5723字节）生成。仅一份因果投影，58.320ms供给折算、12.658ms/47段完成闭合IO、原始耗时轴、链上业务族及背景JIT分层可见。根因JSON5项顺序来自模型；Comp7.405=0.109+7.296ms、Jank4.710=1.759+2.951ms，候选限定/算力来源/独立帧资格完整，B1562 production-positive。
- 模型正文仍错误宣称频率/优先级运行缺口“物理重叠”、IO与频率“独立”、优先级等待与频率“互不影响”；未发布相交/相加凭证不能推出这些关系。另将折算估计下界表述成上限，调度代表3.956ms与最终合并6.797ms未对齐。不能系统替写正文来通过验收。
- **B1568**：◎总览只有频率/IO/调度，锁与优先级方向消失；树与JSON却保留Comp/Jank排名。查明same-segment fold容许原查询窗±1ms、继承Rank/板身份却漏RankQueryWindow成对值，精确主窗门在fold后把幸存chain节点拒掉。不是非链候选本应退域，也不是正常节拍裁剪。修向是完整同源排名身份贯穿，门阈值不放松。
- **B1570**：给模型的读者事实卡把7.405/4.710复合可消量说成实测running，甚至禁止使用真正raw running8.294/3.299。源数据有原始状态量，node/helper丢失口径；需修系统信息，不能仅判模型波动。
- **B1567**：analyzer9轮才完成，第4–8轮有required causal role，却因source_quote拼接/省略号不逐字锚定被丢弃。工具缺role诊断未带真正归一化原因，模型反复改错字段。单独9臂入口及教学已修，保持语义决定由模型作出。第一轮两个独立错误串行返有成本；第三轮excluded roles变空则是模型重发新错，不混算。
- 模型的最后summary patch只给runtime_work_relation、没有text，最终前置背景JIT判断不是系统删除正文后代写；但意图/摘要表达仍欠佳，不能称最终分析已有良好总结。未新增正文词扫描、强制排序或替换结论。
- 上下文最高54%，非预算不足；保持流式字节活动，没有4ms/4min无可见答案强制降级。

## 类型关系图

- `.codrax/output/20260906-002353.430-79278.md`：13节点，12条实现边同向指向LoopController；逐一核对12个生产Observe方法和文件位置，另3个测试实现有明确区分。
- 仓内官方Mermaid包+Chrome直接解析/渲染原始唯一fence成功，SVG 26,604字符，证据 `.codrax/tmp/r1022-loopcontroller-mermaid-20260906.json`。本例模型直接写flowchart，因此不是分段class shim新分支的live见证；该新分支已有独立fixture及官方验证。
- 首次作者section已经有12项文件清单；advisory声称“最终可见答案遗漏实现类型所在文件”，第二次patch实际只加member_set元数据，所有作者文字/图保持。**B1569/P2** 应将诊断改为“结构化绑定未声明”，不得从标记缺失断言可见内容不存在，也不扫描正文替模型自动归类。
- 模型将Observe概括为每阶段结束后调用略粗；真实接口在工具执行后和无工具soft-stop两类循环节点观察。本题核心关系/位置正确，不为该附带措辞新增硬门。
