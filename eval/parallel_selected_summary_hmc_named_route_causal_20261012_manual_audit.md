# §228 命名原件交接与正常答案因果资格：真实双例人工审计

2026-10-12。固定代码 `a9c930d8efc7`，构建 `2026-10-12T08:12:02Z`，dirty仅文档。两例于本机01:12:20同时启动，各一次、并行2，timeout1200s。未改自然问题、未追加第三例或追绿重跑。[机器摘要](parallel_selected_summary_hmc_named_route_causal_20261012.md)仅判工具/附件/投影烟测，不能代签答案正确。

| 案例 | 机器 | 完整人工 | 耗时 | 关键结果 |
| --- | --- | --- | ---: | --- |
| trace_dual_common_window_measurements | PASS | **FAIL** | 121s | 原件交接和双侧原生取数成功；正文重抄错精度、分组、区间和未知数量 |
| trace_business_io_sequence | PASS | **FAIL** | 555s | 正常模型图开始受核验，私有投影保留；正文因果/量尺错误，模型图漏真实唤醒 |

账本仍79唯一ID=20完整交付+59开放、重复0。准确附表不抵消正文错误，确定性回归不倒签原FAIL。

## 1. 共同窗口双侧量测

结果目录：[trace_dual_common_window_measurements-20261012-011220](results/hmc_named_route_causal_20261012/trace_dual_common_window_measurements-20261012-011220/)。[独立oracle](cases/trace_dual_common_window_measurements.oracle.md)。

实际命中`route=data → needs_native_reader → analyze`，首次分析保两原路径和共同`[1,2)`窗。Explorer初始上下文已给两側available准备结果；两个实际trace_query由原路径自动定位各自bundle，取到基线13条/7组、当前8条/4组。原库15/10条，各1条NULL时间不计窗口，右边界正确排除；最终两侧available。不能再解释为“没准备”或“路由丢原件”。日志`run-1.logs/codrax-20261012-011224-000-11738.log:806`、`:955`及载荷`trace-query-result-df998266.json`/`trace-query-result-8d51a854.json`可复核。额外read_file×2和sqlite exec×8是可削减绕路，不增加权威。

| 原生事实 | 最终正文问题 | 定位 |
| --- | --- | --- |
| 基线7独立组，5明确引用+999未解析+40歧义 | 合称6种，混并两个未知组 | [primary:7](results/hmc_named_route_causal_20261012/trace_dual_common_window_measurements-20261012-011220/run-1.primary.md:7) |
| 基线334200000.5 | 写3.342×10⁸，丢0.5 | [primary:9](results/hmc_named_route_causal_20261012/trace_dual_common_window_measurements-20261012-011220/run-1.primary.md:9) |
| 基线gpu_state最后TEXT记录结束于1.6s | 写覆盖至1.5s | [primary:11](results/hmc_named_route_causal_20261012/trace_dual_common_window_measurements-20261012-011220/run-1.primary.md:11) |
| 当前250000000.25，NULL/-1两条持续未知 | 写2.5×10⁸，未知写1条 | [primary:20](results/hmc_named_route_causal_20261012/trace_dual_common_window_measurements-20261012-011220/run-1.primary.md:20) |

正确明细在[transcript:43](results/hmc_named_route_causal_20261012/trace_dual_common_window_measurements-20261012-011220/run-1.answer-transcript.md:43)、`:50`、`:75`、`:78`；13+8行、NULL/TEXT/BLOB/大整数、原始/裁剪区间、各1条无法定位时间均保全。原生保真不等于正文复用正确，名称也不独立证明GPU身份、单位或可比性。本例无图，不计图验收。

唯一patch拒绝（日志`:1915`）是模型选未发布的summary `s1.add_facet_id`。`answer_document_evaluator.go:17194`明确仅用已发布精确分支，否则replace_blocks，不是系统强制schema不支持的操作。已有完整原生成员表，后续member_set提示仍增加重复填写负担，是共享展示改进点；应基于同源事实ID复用成员/范围/数值、解释单列，不扫描正文数字硬门。

## 2. OpenDocument业务等待与唤醒时序

结果目录：[trace_business_io_sequence-20261012-011220](results/hmc_named_route_causal_20261012/trace_business_io_sequence-20261012-011220/)。[独立oracle](cases/trace_business_io_sequence.oracle.md)。

本轮仅event_search和wakeup_chain(pid100)，没有root_cause_rank，不能为新的无目标榜显示资格记live覆盖。Finalizer日志`run-1.logs/codrax-20261012-011224-000-11739.log:2483`起已有：OpenDocument`[1,1.05)`50ms内running5/runnable1/sleep44ms；LoadDocumentIndex`[1.0045,1.0445)`40ms内running8/runnable1/sleep31ms。`:2513`区分35ms请求驻留与31ms完成闭合等待，`:2414`披露背景活动指数不授因果。

真实唤醒为80→200@1.040010、200→100@1.045，上CPU为1.041010/1.046。系统私有Trace投影仍保两边：[transcript:135](results/hmc_named_route_causal_20261012/trace_business_io_sequence-20261012-011220/run-1.answer-transcript.md:135)、`:149`及确定性附件，未被新模型图核验删除。不能把以下错误全归因于事实未投递。

- [principal:3](results/hmc_named_route_causal_20261012/trace_business_io_sequence-20261012-011220/run-1.principal.md:3)：业务50ms内运行写7ms、44ms占84.6%；正确5ms/88%，首次入睡前仅1ms。7ms/84.6%来自较宽观察窗，不是业务窗。
- `:7/:17`：将wakee的120/ohos_rt挂到storage-irq，waker实际未知；又把53/ohos_rt称CFS，不能据此构造RT>CFS差。
- `:15/:19`：低压力写成CPU完全空闲，47跨单位活动指数写成47ms且称唯一活跃请求。`trace-query-result-3afdd2c0.json:2284`的score_caliber为cross_unit_activity_index，47来自background backup请求，不是目标线程阻塞或硬件带宽。
- `:13`保35/31ms且未相加是局部正确；`:19/:21`仍以35ms请求驻留当响应根因主量尺，保证换更快存储等显著提速。应优先定位链上worker的31ms闭合等待和LoadDocumentIndex业务线索，不能由驻留单独证明硬件瓶颈或收益。

首次正常模型图的无证派发、自调用和wakeup使用reply操作符不再绕过核验，正常接受边界确实命中。最终[principal:25](results/hmc_named_route_causal_20261012/trace_business_io_sequence-20261012-011220/run-1.principal.md:25)仅保200→100，缺80→200，还有重复/孤立参与者，完整图FAIL；私有图保真不能代签所求模型图完整。

13次拒绝/13次patch包含缺摘要/图、错误关系、整块越权替换、字段层级错误、旧refs重放、错误调用身份、孤立节点处置和接受后未发布facet分支。迭代12接受，迭代13附加维度patch失败后仍保原接受稿，不是恢复稿泄漏。未见旧错误数组坐标或private sibling carrier_added回归，不能把全部拒绝统称系统死循环。

确认通用系统接缝：同端点多个语句共用anchor，删除失败reply连带删去合法forward仍需的anchor。日志`:3124`只余reply冲突，`:3317`仅remove该ref，`:3324`合法剩边变missing_call_anchor；`answer_document_diagram_edge_patch.go`删除分支原来只要非bodyOnly就删匹配anchor。两个独审一致，本批同轨补公开RED和精确所有权保留；实现/末版收据记账本§228。不据此声称全部后续模型删边都由它造成，不修改原FAIL。

另有旧participant覆盖检查按root_cause_trace家族早退：`answer_document_diagram_participant_coverage.go:917/4078`，本次三参与者incident_required已被接受（日志`:1007/:1011`）但未执法。retain_as_context不能证明所求关系完整。此项留12.5/18.4，不在收尾另开第三实现线。

## 3. 留账与后继

1. 完整能力10.1：converter已有设备/负载元数据→来源代次→single/pair消费；有见证不恒写未知，无见证不猜。跨路由已真实取数，不重复施工，不把全部正文正确性无限加到10.1。
2. 共享缺陷16.4/18.4/12.5：业务实例窗与观察窗分开；正文/表/图按同一事实身份复用，成员维度不迫使重抄值；参与者覆盖和关系修补依精确typed所有权，不扫描问答关键词。
3. 完全空目录、无runtime附件而仅在问题中命名外部文件的既有早退，以及运行中data/native混编留18.4/01.3。不是所有无可解析源码仓都失败，本次README/docs仓已进入分析。

证据充分后的误述保为验收缺陷，未证明稳定波动，不宣布波动、不追跑。两份FAIL与原五稳定验收父项交叉，不相加成新任务；未跑写模式、只读登记和其它二进制分支不补签。

原始工具载荷、实际准备产物、出厂Markdown/HTML/来源表及验证日志已复制到本批results的`tool-payloads`、`prepared-inputs`、`output-artifacts`、`verification`并逐文件核对SHA-256。题目/oracle/输入7份哈希未变，原FAIL及中间拒稿不覆盖。

同轨收尾补丁`8c74418f4`已修共享锚点局部删除与替换迁权，公开相关/race通过，详见账本§228.7。它在本次live之后实施，不能倒签这两份FAIL。任意多replace共享锚点整体迁移仍是旧执行器边界，留12.5；本批仅证明最后使用者退休及部分替换保护，不夸大为全部图修补已完成。
