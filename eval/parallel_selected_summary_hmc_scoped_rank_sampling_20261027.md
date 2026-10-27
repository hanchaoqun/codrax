# HMC301：量测保真、主查询候选资格与采样轮换

日期：2026-10-27。候选分支`codex/hmc299-wait-interval-candidate`，基线`cf8d13450cafbd71be01444e0d56e9b1d065cc26`；main保`788588e365d40a48b30827ffd8db08dae4e2ae9d`。本批不合main。

## 1. 对账与交付范围

79唯一稳定任务＝25已交付＋54开放，18类；开放17部分、31待实施、3待验收、2验收中、1持续。**完整能力新增0；候选共享修复2组；完整自然答案0/2通过。** 不将独立量测保真、定向测试或机器PASS代签父能力。

1. 当前查询根因资格：原公开四项合同在宽窗加入后变5/6，新增者均为同一物理11ms IO的独立宽窗量测。仅候选编译复用已有主窗权威/榜单匹配，不合并或删除原量测，不改CandidateID、查询分母、Trace因果投影、自动补齐或闭合睡眠实例。
2. 真实发射覆盖：原严格census追加公共TraceQuery wait实例，各保原件/查询；补旧`root_cause_eligibility`同字节常量与io/display_only登记。`wakeup_measured_wait`保持soft_consumer，原断言不放宽。

参考实际源码：`wakeup_chain_index.py:117–145`真实线程实例与精确wake端点；`sleep_ops.py:558–605`状态段和动态量测范围；`llm_contract.py:112–146`按真实成员/字段回查且零值有效。吸收身份/量尺/来源分离，不迁参考近邻或文本启发式为硬门。详细设计见统一账本§301。

## 2. 修复与确定性证据

所有运行的argv、PID、UTC、exit、日志SHA均保在本批封存；文件名含green不代表整体PASS。

| 验证 | 结果 |
|---|---|
| 原四项合同、未改断言 | RED：10子例FAIL/2PASS；精确窗单独通过，宽窗组合重复候选 |
| 新候选矩阵与原四项/闭合睡眠 | 最终3包5顶层/53子例PASS |
| 候选相关＋相同race | 各3包662顶层/922子例PASS，无FAIL/SKIP |
| 发射末版分包normal | 4包29顶层/36子例PASS |
| 发射末版分包race | 4包28顶层/36子例PASS；分类AST另有同生产字节race通过 |
| Trace/自动补齐/活跃流/超时预算/P3等保护 | 3包297顶层/64子例PASS |
| 标准CGO make | PASS，原生工具嵌入校验通过 |

发射v3/v4整体FAIL保原：新golden排序/并行测试编译施工错误与当时未修候选RED分开，不冒充产品故障或整轮绿色。新增测试覆盖三类typed候选×12边界、同窗多board、独立authority、未知、0起点、原2µs容差、rank donor优先；真实TraceQuery反发布顺序及同名双源各4项、全部ledger IDs/MeasurementOrigins保留，编译前后projection字节不变。原HMC232闭合`[1.009010,1.040010)`31ms与尾段合同未改。

## 3. 严格冻结与真实双例

冻结清单SHA `c13451db79eb8a083eedcd7edb5deae5b525166d0975dbeed5358c99be79802c`；冻结后源码、测试、注释及全部执行/提取helper停写。7422仓库输入＋38外部＋13仓外快照，源码快照使用`.go.txt`，不重新污染`./...`。凭证文件不读、不hash、不复制。

标准CLI SHA `9b494d874d3c913f9569242441d3c0bfe85111c28cbe3256c5bd6879df011891`。原case/QUESTION与15步不变，n=1，各一次，共同屏障放行；无第三模型、无冻结后重建或二次全仓。

| 侧 | 原case | 进程UTC开始→结束 | 机器/完整人工 |
|---|---|---|---|
| A | trace_query_wakeup_background_demotion | 09:26:44.063315→09:31:10.937733 | exit0/PASS；FAIL |
| B | trace_binary_sample_only | 09:26:44.065288→09:30:06.909124 | exit0/PASS；FAIL |

A为15事件构造ftrace，不冒称实机；B为真实完整30,062,585字节PERFILE2，SHA `da713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632`，未预注入准备产物。实际请求重叠及完整审计见[人工报告](parallel_selected_summary_hmc_scoped_rank_sampling_20261027_manual_audit.md)。两侧正式receipt冻结前后均true。

A的5次trace_query全在2–2.02，保IO11与三项1ms、logger背景；新宽窗分支**NOT_TRIGGERED**。sidecar把runnable相邻段错称同一IO等待区间，原生占用表重复，完整FAIL。其已投2/3调查解释有“唯一席位”与四项榜单、关联重叠0与物理同窗的概念冲突；正确事实也已到场，不能认定冲突就是错句唯一成因。B完整12000准备、目标164/259及两热点82/174、82/85正确，自然补查采样树；unknown仍被终稿断言on-CPU/最外帧，完整FAIL。B调查错误reason未投实际Finalizer，不能臆称传播。正确原生数据和后文边界不能抵消前文越权。

## 4. 全仓、封存及发布终态

唯一全仓`go test -json ./... -count=1 -timeout=30m`实际UTC09:26:44.718039–09:41:35.213498，exit0：**97包PASS、12包无测试，28,373顶层/33,304子例PASS，零FAIL**；41顶层/14子例SKIP保原，不算通过。251,634行均JSON，无重复终态或未结束项。日志SHA `d6a230536a8ffa6546e214dcff2a88bf7dbd09e579e7f2b2b9279f0b9026a7b5`，正式receipt冻结前后均true；未跑第二次全仓。

结果根：`eval/results/hmc_scoped_rank_sampling_20261027_v1`。两侧完整工具payload、三个实际Finalizer INIT、最终MD/HTML/sidecar/表面清单、错误和原件索引均封存；HTML只做源码/内容审计，不冒称像素渲染验收。旧HMC299 209件＋HMC300 232件和18个隔离快照逐SHA保原，不倒签旧全仓FAIL。

本批**235份独立原件版本**已封，canonical realpath＋SHA去掉路径别名重复，但同字节不同来源保留。`sealed-index-v1.json` SHA `bf2f9bfa5092d119a7daa234c74e923ee8f4c3336a62243903589122733cca60`，封存时间UTC09:42:26.983834；三运行正式终态均exit0/冻结前后true。原RED、施工失败、两个完整人工FAIL、未触发结论均原样保存。

代码提交`e3aa051b23f445822411d12cb4c07305004c3e1f`；五文档另笔，二者相邻并普通推送同候选分支。最终提交树与7422输入、38外部、13快照、235封存原件、远端/clean核验由本结果根`publication-v1.json`记录；不另改冻结helper，不倒签原失败。

## 5. 剩余验收与ROI

- 先复现既有图主体/业务角色/原生lane的精确绑定及同源呈现重复；不在量测层重新粗合并。
- 再核成文/sidecar使用的状态区间、量尺与已选候选身份，保相邻与重叠区别；对已到场unknown不新增原文关键词门。
- 能力槽轮换只读登记或代表选择原合同；宽窗新资格自然覆盖欠项明确留账，不无限重复原题求绿。
- 旧采样、IO、栈关系与权限未触发保持原项；17.6欠新的真实格式/工具/宿主材料，同采集包装不充新能力；17.7活跃WAL未满足零副作用，生产NO-GO。

代码与五文档分笔普通推送候选。最终`publication-v1.json`只签冻结字节/提交树/封存/远端/clean完整性，不签main准入或人工通过。
