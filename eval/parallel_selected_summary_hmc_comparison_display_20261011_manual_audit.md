# HMC §224 双侧输入与原生展示人工审计

- date: 2026-10-12T01:45:45Z
- sweep_start_ts: 20261011-184544
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_comparison_display_20261011

运行版本`487d6ab3c353`，snapshot `codrax-selected-20261011-184544`；两例各一次、并行2，runner正式exit0。机器1/2，完整人工0/2；两位独审及主审均检查实际工具结果、模型上下文与最终接受稿，不以草稿或宽松机器门代签。后续入口补救不重跑live，不倒签本结果。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_dual_measurement_partial | FAIL | eval/results/hmc_comparison_display_20261011/trace_dual_measurement_partial-20261011-184545 | successful_tool_result | none | 90s | 35 | read=1,repo_map=0,list=0,trace=2,source_lens=0 | midloop=1,inv=2/2,fin_reject=1,unavail=0,prune=0 | FAIL | 独立准入被源码策略挡住；健康侧未执行却被说成SQLite不支持 |
| 2 | log_shared_sources | PASS | eval/results/hmc_comparison_display_20261011/log_shared_sources-20261011-184545 | log_regex,log_attachment | log_triage+log_query | 391s | 30 | read=3,repo_map=2,list=0,trace=0,source_lens=1 | midloop=4,inv=7/1,fin_reject=0,unavail=0,prune=0 | FAIL | 9条保全但为重抄；时间单位、跨源同一性与正文关系错误 |

## 量测：真实准备的默认策略接缝

证据：[最终接受稿](results/hmc_comparison_display_20261011/trace_dual_measurement_partial-20261011-184545/run-1.primary.md)、[完整过程](results/hmc_comparison_display_20261011/trace_dual_measurement_partial-20261011-184545/run-1.logs.all.log)、[独立oracle](cases/trace_dual_measurement_partial.oracle.md)。

- 516–520行：分析器已正确声明两侧原路径和1–2/4–4.5秒窗口，当前源码策略由route合成为allow。这是自然问题，不要求用户额外说“不要看源码”。
- 934–942行：两个single trace_query中空基线先失败；健康current被全局terminal执行前拒绝，并非SQLite原生解析失败。无真实健康侧量测结果，机器成功结果门正确判FAIL。
- 1014–1019行：model absence把未执行误说成格式不支持并被接受；1644–1663行后续交接重复了这一错误闭环。工具修复摘要仅按tool给reason，未清楚区分失败路径和被跳过的健康路径。表中inv计数为runner启发式统计，真实第二次completion的ok=true以日志为准。
- 1704–1714行接受错误正文；后续patch被拒，最终稿仍断言SQLite不支持。健康侧8条、4序列、精确整数/NULL/0/未知区间、逐源窗口和原生展示都未命中，不能给它们签live通过。
- 系统根因：合法comparison和原生准备仍被旧“排除源码”条件耦合；运行时输入接入应与是否另看源码正交。目录discover拒绝是正确边界，不能为此开放父目录。入口补救挂10.1/17.7；无执行却成absence的交接权限挂16.4/18.4，不称模型波动。

## 日志：完整事实到场，默认展示入口与正文仍失败

证据：[最终接受稿](results/hmc_comparison_display_20261011/log_shared_sources-20261011-184545/run-1.primary.md)、[完整回答含系统补充](results/hmc_comparison_display_20261011/log_shared_sources-20261011-184545/run-1.answer-transcript.md)、[过程](results/hmc_comparison_display_20261011/log_shared_sources-20261011-184545/run-1.logs.all.log)。

- 实际log_query取得app6+kernel3逻辑记录，合11物理行，匿名PID/TID、invalid wall、精确boot_ns及原文均保全。最终模型表也保9条，包含孤儿L5和未知L6；不重报§223的8/9遗漏。系统handoff约4390行供给完整原生表，输入保真这一层有效。
- 846–851行：模型未声明source_inventory_profile，系统因typed枚举形状合成function/method/type源码清单。route的allow与无引用source_scope=all互相自证，使事件整理误变源码清单，增加repo_map/源码read/重复completion修补，并让原生展示维度保守不发布。应修正默认合成依据，不放宽原生展示门或禁止真实混合源码问题。
- 最终6块没有runtime_measurement或requested_dimension_indices，table-events仍是模型重写九行；本批selector复用和9+3+6默认子集抑制没有被这份最终答案验收，不能只凭确定性例或机器PASS声称闭环。
- 最终正文1/3/89/98行把共享rx17升级为同一次请求，虚构“两源共享catalog.bin”；kernel没有asset字段，observer及未知行甚至没有rx17。弱关联可作排查线索，不能证明同一业务实例。
- 53/60/68/114–116行把原始boot秒串当成纳秒；53/92行进一步错算约9.007秒，实际原始秒为9007199.254740993、精确ns为9007199254740993。28行把99月误说成年份；118行又说malformed记录线程未知，与同稿27行及原生PID/TID矛盾。
- 正文虽多处否认已证因果，仍断言同一请求上下文和已校准隐含路径，不能靠免责声明覆盖无证同一性。误述挂16.4/18.4，数据已足与系统错误引导分别记，不新增答案数字/关键词硬门，也未证明偶发波动。

## 过程与保护项

实际Finalizer上下文量测首轮33973、后轮43834 tokens，日志47179 tokens；这些仅本轮收据，不从一次运行推导普遍降噪收益。模型分类修复文字、源码广度任务、合成源码清单与重复completion均属后继降噪线索。无图、写模式、只读登记自然分支，不能给旧任务补签；Trace因果投影/自动补齐本live未触发，保护依赖确定性回归。固定双例之外未追加第三例追绿。

原始载荷与测试收据随批归档；末版修复/全仓/推送记录见统一账本§224，历史失败原文不改。
