# HMC326：显式证据生命周期、整请求预算与代表自然验收

2026-10-31 UTC（批名按本地2026-10-30），持续main。基线仍为`a8846881aaa1ba6cea42aeaad7dd86988165c553`；承接§325未发布候选，本批也仅失败封存、未独立提交发布。79个唯一稳定任务＝27已交付＋52开放，状态仍18部分实施／29待实施／3待验收／1验收中／1持续。**完整能力新增0，正式交付子能力0，closure=none。** B原代表选择自然合同、唯一operator及独立人工PASS，但唯一全仓FAIL，11.2不销账。

后继状态注记（§327）：新代唯一full已PASS，34文件修复代码776c8683940a777ec99cfac94bc51ff2e5f42afa独立提交，交付范围仅3组有界共享子能力；552原件已封存，代码/文档相邻两笔及main远端由[§327 publication](results/hmc_native_publication_compat_20261030_v1/publication-v1.json)确认。本报告的8顶层/2子例FAIL、A人工FAIL与B原合同/operator人工PASS均保持§326原判，889封存原件不重写、不补§326 publication。§327 B没有生成执行后同代交付，不能借本代PASS替它触发；11.2仍不销账。见[§327汇总](parallel_selected_summary_hmc_native_publication_compat_20261030.md)。

## 候选实现与参考

两个共享候选轨分别处理当前证据生命周期和整份成文请求预算。显式替换/清空当前证据与普通历史压缩不再共用撤销含义；非撤销发布使用typed kind，追加和fork合并检查集合身份，物理源epoch、当前查询资格及各分支可见性保持独立。候选保128查询/16MiB encoded核心边界；不是heap或提示容量承诺，也不从缓存授予完成、根因或旧来源权限。

成文预算统一静态消息、实际过滤后工具schema、尾部提示及动态事实；必需完整单元先留，可选原生依赖组整组发布/省略，实际Chat前再核内容字节。已知输出上限才计算输出保留；未知不造数值。预算收据明确是本地字节策略，不能声称精确tokenizer或provider容量保证。来源/查询/实例/窗口绑定、原16+4/128行及原生完整库存不放宽，没有问句/答案关键词硬门，Trace因果投影、链上根因和自动补齐仍按原合同保护。

本次实际参考阅读包括marker_ops的完整node_map、itid、各节点状态交集及缓存对象身份，以及query_engine的summary_only/data、集中legend/empty/truncated分层。借鉴完整结构先行、展示缩减后置；不照搬扁平legend.update混跨查询字典、缺状态补零、children直接求和或名字经验规则。原生准确self本来存在，修复目标是传输/生命周期和展示交接；不倒签§323每一个实际丢弃时点。具体阅读范围、root与准备轨归属见[参考发现原件](results/hmc_native_lifecycle_prompt_budget_20261030_v1/sealed-v1/0693-codrax-hmc326-reference-findings-v1.md)。

## 冻结、验证与全仓失败

最终定向3包119顶层/298子例PASS；有限race2包16顶层/15子例PASS。原失败、修复过程、收据均保留，不以定向选集代签全仓。普通CLI与原代表operator各构建一次、同源码inventory `771eeb9041e52fd24312e82f7fb790e4d74952663ec6d207aa67e814f8d96597`：

- CLI SHA256：`2377963bb6701f010a40f332efb0305b2886682d31c9845edea76d140473cf9d`。
- operator SHA256：`d94b8cdb263ce3552981fb3b01305a576bd59911be3269e256d5ee769fd39efb`。
- 冻结7549仓库输入、233外部依赖、40仓外快照；SHA256 `b7f58716737427676120d4b5db9a558f56fb39da4306fe6699b7fc886c671b11`。此后源码、测试、helper、输入停写。

唯一full PID40988，UTC02:47:35.740910父进程spawn前记录至03:01:05.037145真实终态，exit1、signal=null、源前后核验true。95包PASS/3FAIL/12无测试，28633顶层PASS/8FAIL/43SKIP，34076子例PASS/2FAIL/14SKIP；265878条GoJSON，非JSON/重复终态/未终态均0。完整日志SHA256 `1a262adc07417b2f515c45aa7882635de907aa8995fcb216d9a7c921fd94bfb3`，见[原统计](results/hmc_native_lifecycle_prompt_budget_20261030_v1/sealed-v1/0639-codrax-hmc326-full-stats-v1.json)与[实际进程凭证](results/hmc_native_lifecycle_prompt_budget_20261030_v1/sealed-v1/0640-codrax-hmc326-full-v1-receipt.json)。

8项顶层失败如下；两个子例FAIL均属于repl一项，不重复加为10项顶层失败。

| 包 | 原失败测试 |
|---|---|
| types | `TestB1624bLateForkCannotCreateFreshNavigationForOldOriginal` |
| agent | `TestTraceEventInventorySourceSharingKeepsGenericFactsHMC251` |
| repl | `TestHMC274SourceOutcomeConflictPublicRepair`（artifact、mixed两个子例） |
| agent | `TestPostEmitAdvisoryIsOneTableOneLatchOneArm` |
| agent | `TestTraceEventInventoryOverlappingPublicQueriesKeepCompletionEndpoints` |
| agent | `TestNoInternalTermsInHints` |
| agent | `TestHMC275PublicPartialTimelineAliasNeverClaimsComplete` |
| agent | `TestHMC269SourceTitleNavigationActualOverlappingWindows` |

这是确定性回归，不是8个模型误述，也不能未经公开复现就统称旧标签兼容。`TestBusContextCopylocksVetClean`在本次full已PASS，§325的mutex复制原FAIL仍按旧代保留，不误记成当前仍坏。冻结后没有第二次full或源码修补。

准备阶段另有编排偏差：额外只读Ruby诊断语法失败，工具报告exit1，却因编排未按失败分支停止而继续执行了原guarded prepare-v1（实际exit0）。额外诊断没有独立intent/preexec/OS终态收据，不能补造或把它当成功核验，也不能声称prepare未执行。报告root后，仅按明确授权继续原chmod→preflight→freeze，未重跑已完成label。原命令、错误、顺序和凭证界限保存在[偏差原件](results/hmc_native_lifecycle_prompt_budget_20261030_v1/sealed-v1/0685-codrax-hmc326-preflight-diagnostic-deviation-v1.md)。

## 两例各一次，过程与验收分开

| 例 | 原入口与预算 | 进程／原结构检查 | 独立人工 |
|---|---|---|---|
| A | 原105版元宝完整ftrace；15steps/n1 | exit0／原smoke PASS | FAIL：三类状态/量尺/范围冲突 |
| B | 原代表选择自然题；50steps/n1 | exit0／唯一原operator exit0 | 原合同自然PASS；交付待全仓 |

A运行PID40997、产品CLI41040，B产品CLI40998。两个进程同时存在不是并行证明；按真实日志唯一配对完成区间A30/B19，26歧义单列不猜，最大可证重叠22.459秒。原始日志分别有A124request/124response、B37/36；B一个未配对请求不说明进程未终态。完整[请求配对原件](results/hmc_native_lifecycle_prompt_budget_20261030_v1/sealed-v1/0545-codrax-hmc326-actual-request-overlap-v1.json)保留这些不同口径。

A认真复用`/Users/han/opt/donghu/基线数据/105版本/元宝/容器重启后冷启动/1、LOG_ftrace_20261019_17.17.44/ftrace-20261019_17.17.44.ftrace`，63659046B，SHA256 `be59c0648ac7e1d8a2d117f5dc36efa0c92b5f3686671719ada435c16713a20c`。原问句、范围及材料未改，独立整数μs oracle仅供审计，不注入模型。两次activityStart的total/self为59.695/6.770、111.487/3.938ms；两次主要performCreate为46.600/21.605、103.315/83.143ms，全部连同独立状态进入实际INIT并保留到最终正文。7张系统时序表3493展示行逐行匹配oracle，含3份重复全窗表而非三个独立总体；物理线程46571与marker payload2227、原件与预览大小正确分离。

但A正文/手写表把D未标记桶与IO桶写成包含关系，将self墙钟升为纯CPU执行，又把父实例子窗self0外推到整体。前两类原生事实已经足量；第三类实际受理解释unit[0]和同包广窗父self2.375冲突，属于可定位的共享解释交接欠项。完整原生取数不覆盖成文错误。A收集26唯一query payload/3INIT/1HTML，日志43次trace_query均成功，次数与物件数不混算。

B自然选择真实`resource_stack`规格，8计划/2组/4选/4未选；先查四入选，再补查四未选但不改选择身份。8计划各一次尝试，6success/2empty、0failed/0not_executed；每组4成员、2选2未选。原status同一次返回的index/selection snapshot实存hash一致，唯一operator与独立人工均PASS。四份SQLite来自两份构造夹具各拷两次，不冒四次实机采集；同字节物理文件仍分来源。最终保半开窗、线程、事件/帧/字节口径、合法深度排序及未知栈质量，没有用额外查询替代错误选择规格。

## 未销账、后继ROI与封存

优先闭环8项全仓系统回归，再公开复现父账户范围与解释复用冲突，继续同源事实与正文/表/图一致性；前两类足量证据后的解释误述保留，避免无限追绿。B自然原合同已通过，但11.2待全仓及交付门；不改原题追加出口门，也不以本次通过回签§325。随后轮换既有只读登记缺来源终态负控和具备真实材料的二进制。原52项与旧9组账目保持；图、repair/no-fit、全部fork/容量边界、sample-only、旧IO/权限等未触发不销账，100版抖音仍NOT_RUN，WAL未零副作用继续生产NO-GO。

本批889原件于UTC03:18:09.995701封存，[索引](results/hmc_native_lifecycle_prompt_budget_20261030_v1/sealed-index-v1.json)SHA256 `1573ed5fb73eb7cfa3a1170ec483b14d7c6957505d1ff0edd79ad64f6a16c07a`；[收据](results/hmc_native_lifecycle_prompt_budget_20261030_v1/sealed-receipt-v1.json)保A/B/full实际退出0/0/1。旧27档10137件/632独立快照来源身份核验，原失败不变。大ftrace及同字节全文副本仅登记外部身份，不复制入Git；完整query/INIT/答案不按大小删减，Go快照不回编译树，provider内容未读/hash/copy。封存不授PASS，本代无publication，后继另代修复，不重封本批。

参见[人工审计汇总](parallel_selected_summary_hmc_native_lifecycle_prompt_budget_20261030_manual_audit.md)、[A独立原报告](results/hmc_native_lifecycle_prompt_budget_20261030_v1/sealed-v1/0503-codrax-hmc326-A-audit-v1.md)、[B独立原报告](results/hmc_native_lifecycle_prompt_budget_20261030_v1/sealed-v1/0524-codrax-hmc326-B-audit-v1.md)和[52项核账计划](results/hmc_native_lifecycle_prompt_budget_20261030_v1/sealed-v1/0600-codrax-hmc326-census-plan-v1.md)。本次只审HTML源结构/事实，不冒浏览器像素QA。
