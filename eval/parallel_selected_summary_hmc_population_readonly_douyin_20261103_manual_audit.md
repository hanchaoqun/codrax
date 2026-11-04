# HMC343人工审计：充分原生事实与整答、只读负控分账

核账79/28/51，完整新增0、交付2子能力/1共享组，剩余验收3组；源码9758d3628b7aa27b0687c4bb912985949d2cb4b4。两例各n1、实际精确请求A14/B2、0歧义、最大重叠3.895秒。全仓OS0/98包PASS只证明本代代码，不替完整人工判决。无第三次真实模型评测、冻后没有源码/测试/教学/helper/case/oracle修改。

## A：完整用户抖音原题，smoke PASS、整答FAIL

原15steps问题/case未变；使用完整235422121B/1696481行用户ftrace，SHA93b45bcca5c9e9b415478d81e176b633d70886328c49731b5258c806695de1ef。目标header TID57874与marker payload PID10266分开，不按payload PID改线程。物理原件→原生query→typed账户→实际INIT→提交→MD/HTML逐层核对，原过程见[完整A审计](results/hmc_population_readonly_douyin_20261103_v1/sealed-v1/0202-codrax-hmc343-A-audit-v1.md)。

6trace_query日志声明、5唯一JSON；20工具声明/20参数文件全部可解析，不相加冒40次工具执行。一次exec导航被拒绝，其余grep/read_file针对本轮query TXT/JSON，不是实现分析。原生包含thread_timeline、window_stats、span_window、两份event_search；重复声明不冒独立成功查询。

### 可信数据已到场

- window_stats旧全库存25139/原生省25107/保留32不改。新population显式全线程，目标TID57874库存2611/展示32/省2579；实际INIT03唯一business_population定义先于引用，16完整账户及上下文省略单列。旧数据缺失已局部补齐，不能仍一概归因“没有原生事实”。
- 32节点按name、物理行、起止微秒、直接子数、inclusive/self墙钟、header TID共256项对旧338完整独立整数B/E账零差异。[只读交叉账](results/hmc_population_readonly_douyin_20261103_v1/sealed-v1/0223-codrax-hmc343-A-native-wall-readonly-v1.json)不重跑旧parser，不冒独立调度/因果oracle。1429确为root直接子，2611才是目标库存，32是保留树节点。
- 原生状态2783段：running959/487.440ms、runnable956/8.714ms、S837/170.918ms、D30/2.245ms、IO1/.034ms。主要16阶段数值与当前原生账户一致，attachBaseContext inclusive运行143.636与self运行103.841分开，旧混写本例未复现。
- D未标记IO与调度标记IO的互斥2.279ms小计、三处非重叠锁标记内sleep86.562ms均为合法同源计算，不误加硬门；它们不证明设备饱和、持有者、唤醒依赖或唯一根因。业务线索保留，解释权限不提升。

### 整答失败而不是数值被篡改

| 失败 | 实际证据 | 判决边界 |
|---|---|---|
| 小runnable比例排除CPU争抢/供给拖慢 | 正文先排除，尾注才说不能推整机利用率 | 单线程累计状态不排除链上其它线程及运行期算力不足；尾注不抵销 |
| “按时间顺序”列表倒序 | base.apk起236627.871880在Connectivity起236627.836554之前 | 端点正确、顺序错误；不冒图时序验收 |
| 32保留节点错套直接子集合 | c1说1429直接子“只展示其中32条” | 全树、目标树、直子数与保留数分开；正确新口径到场仍误述 |
| 非零运行区间说全睡眠 | 16.688中S16.639；50.721中S50.650 | 原生保其余运行/就绪，不能抹零 |
| 有子项节点称叶子 | MapImageFile直子43、RelocateImage直子7 | self较小不证明叶子、调用机理或因果；二者同属AppImageLoading |

没有图/局部patch，不能签图身份/关系/像素。原16阶段表在summary和table各一份；原生timeline又各2783行两份。两查询索引padding .25s/.5s和覆盖Notes不同，完整publication仍须独立；下一修复应分离内容体与查询凭证，不以行值相同放宽来源/总体/完成门。

### 实际上下文、初稿纠错与成本

三段实际INIT为100509/14102/314046B（采集含5B分隔换行），不是provider wire/token计量。本例未提交native_fact_handoff/interpretation_units，两次complete走旧aggregate_facts/reason；reason里确有全库存/目标及CPU错误解释。但3396/3663B两份完整reason及JSON转义，在三段INIT六组比较均不存在；逐段检查也未发现上述错误解释。不能由最终答案相似误述推断它们被转发或形成污染因果。当前应记“充分事实到场后的解释失败”。

初稿曾错误断言INIT03保留旧reason，现撤回；[原初稿](results/hmc_population_readonly_douyin_20261103_v1/sealed-v1/0201-codrax-hmc343-A-audit-initial-v1.md)、[精确只读更正证据](results/hmc_population_readonly_douyin_20261103_v1/sealed-v1/0224-codrax-hmc343-A-reason-init-readonly-v1.json)与修正审计并存，初稿不得用于后继因果归账。KnownFacts明确未验证预览；其中query产物grep被通用标为current_source、相应read_file为runtime_artifact是来源分类线索，未证明它制造源码义务或引发本例错误。

MD965741B/SHAfdb059d15122bb7be0b0239db865cc08d06375d3e477cdde0dea69b565400366；HTML4269117B/SHAd4eaa4c06f0d79cc09ee28aeb3d606de8d47088aee286144974dc23bb914eac9。12main表行数为2/17/17/7/2784/2784/9/14/14/14/14/14，MD/HTML cell诊断一致，非像素QA。原日志2848077B/SHAbe20cf64732499bc1a8ddcc4746ba3d2a8be506cc946528dde764ce9f3633496含无效UTF8；原字节保留，一处只读诊断编码错误后用二进制按日志头检查，没有修原日志。

结论：新库存producer/同源说明自然触发且公开模型外路径通过；整答仍FAIL，不关闭任务，不以原生表正确、smoke绿或尾注补签。

## B：原只读登记缺来源终态，限定PASS

原`TestNativeRegistrationLiveRestoredMissingTerminalReceipt`、问题、planner12步、20分钟测试时限未变。真实planner为MiniMax-M2.7-highspeed、2个实际请求；先read_file value.py/test_value.py，再emit_change_plan使用当轮assertion_ref、changes=[]。登记不自动等于执行或通过，不新增探针文件。详[完整B审计](results/hmc_population_readonly_douyin_20261103_v1/sealed-v1/0226-codrax-hmc343-B-audit-v1.md)。

新计划plan-1793755566537048000-26624来源plan-1793755558383479000-26624；temp fixture HEAD562e99577df9cb464dba0403ef7b034844362a42，不是main HEAD。源SHA833556db828032d49efc4c58a380411c53465c917da89bf613a607a10cee2ba6、测试SHAec7d5c37376fa9c583ba402a196e94c9fa3d1c12c734195ccd18e8e7386a7864，独立恢复子26747与父26624检查一致、git status/diff空、授权未泄漏。不冒Python执行完全无临时缓存副作用。

| 维度 | 实际结果 |
|---|---|
| 历史/新执行 | native:6800:18e4b32d43f9bac0:1 / native:687b:18e4b32f350ccc30:1，确实重新执行 |
| 计数 | 1断言通过、2命令（编译检查/原生unittest）、4/4义务；不混为2个探针文件 |
| source batch | 只有apply attempt/apply_ref，无verify attempt/verify_ref/completion |
| proof batch | verify_only、verified |
| overall | run complete但completion unverified，reason missing_terminal_verify_verdict |
| 真实最终报告 | WriteFinalReport保持1断言/2命令及相同unverified；不拿兼容ChangeReport别名代签 |
| 留存 | 父历史及恢复子各4工件；子4份bytes/SHA重核；真实final→同代report links matched |

report SHA6d86dd156c5c59c6e1aba60be4fe5c3d88e6312133188c3911449a02d88c38d2；子真实final SHA75f299d7dbfebccf12e03517a3e771e0f8c59b64c15c2ddb43d983a3572ea5ae。原日志50921B/SHAf0c7c8debaa52feb837fd04e1067ad5546c74102a27293835b1f63e56a1193f3、有效UTF8。

这是“真实planner＋脚本controller＋公共原生执行＋新进程恢复”的负控；natural_controller=false、full_cli_run=false、context_pack_durability=false。只签此负向终态，不外推自然路由/模型控制器/完整CLI/其它语言平台，不算新完整交付。

## 全批判决与未覆盖

恰好2并行×1、两进程OS0、A原smoke PASS/B原负控PASS；完整人工A FAIL、B限定PASS。唯一full98测试包/28739顶层/34414子PASS、零失败，7608冻结输入匹配；624件封存、closure none。3组剩余验收：完整解释；内容体/独立凭证/整组预算与来源分类；未触发有限验收。详细51项ROI见[批汇总](parallel_selected_summary_hmc_population_readonly_douyin_20261103.md)。

本代未验sample-only、代表、图/局部修补、其它真实二进制、WAL生产或完整自然只读CLI。旧IO/资源/采样/权限FAIL按原ID保留，旧18.3有限关闭不重复计数。充分事实后的偶发误述不无限阻塞下一高ROI系统修复；保Trace因果投影、原4根因/闭合睡眠、链上业务线索、自动补齐及合法计算重试。WAL零副作用未证前生产NO-GO。
