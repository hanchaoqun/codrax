# HMC327：共享发布与预算兼容收口

2026-10-31 UTC，批名按本地2026-10-30；持续main，基线`a8846881aaa1ba6cea42aeaad7dd86988165c553`，34文件代码提交`776c8683940a777ec99cfac94bc51ff2e5f42afa`。A/B完整人工均FAIL，唯一full实际PASS，552原件已封存；代码/文档相邻提交及main远端交付以[publication凭证](results/hmc_native_publication_compat_20261030_v1/publication-v1.json)确认，不从本地commit推断push。79唯一稳定任务＝27已交付＋52开放（18部分、29待实施、3待验收、1验收中、1持续），**完整能力新增0，closure=none**。

代码交付范围为3组有界共享子能力：当前代原生留存与历史只读隔离；exact-owner完整账户交接已有产出时的已证边界；整请求预算、整组来源发布与受理答案保留。不是三个完整父任务，也不声称四业务账户整答完整。§325/326均为失败未独立发布代，原FAIL、原封存与§326 B自然PASS各保历史口径，不给旧代补publication。

## 真实验证与冻结身份

最终定向4包152顶层/346子例PASS，有限race2包25顶层/32子例PASS，分别3714/236条GoJSON，零非JSON/重复/未终态。前置publication-compat-v1/v2及soft-premise失败原件保留：两次新正控的完整发布实际更大，最终只将相应夹具明确为严格“省略较小”负控；保roomy完整发布正控，lower-cost-full公共入口仍NOT_COVERED。不以修改断言偷换数值或降低原生完整性/权限门。

普通CLI与原代表operator各唯一构建、同源码inventory `65fd2c0f748680d7f884dca926cc7e88efe5892e8e0e844585ade576839c44c5`；operator仅构建/准备不等于本代执行终态审计。

- CLI SHA256：`a29d35b628c022f6fda7467e81befd087d02e338f6d0c898fe75f2f217c1069e`。
- operator SHA256：`d94b8cdb263ce3552981fb3b01305a576bd59911be3269e256d5ee769fd39efb`。
- freeze SHA256：`675c4ac2f005e24840d904c4fded019bd6724c96812fb47262efdbaad0386bc9`，7553仓库输入/210外部/44仓外快照。此后源、测试、注释、helper、计划及模型输入停写。

唯一full PID73486，UTC04:05:09.136686父进程spawn前时间至04:18:28.695073真实终态，exit0、signal=null，source_verified_before/after均true。98包PASS/12无测试；28650顶层PASS/43SKIP，34095子例PASS/14SKIP；265944条GoJSON，0非JSON/重复终态/未终态。日志SHA256 `0c0806d12ad9290d525bfb5e2fd580b60d31168c0efa842bc8ca485d417ed90e`。见[原统计](results/hmc_native_publication_compat_20261030_v1/sealed-v1/0344-codrax-hmc327-full-stats-v1.json)、[实际进程收据](results/hmc_native_publication_compat_20261030_v1/sealed-v1/0345-codrax-hmc327-full-v1-receipt.json)与[冻结清单](results/hmc_native_publication_compat_20261030_v1/sealed-v1/0343-codrax-hmc327-frozen-v1.json)。

本代参考实读范围与归属见[原始发现](results/hmc_native_publication_compat_20261030_v1/sealed-v1/0415-codrax-hmc327-reference-findings-v1.md)：root重读query_engine 371–429及core/batch AGENTS、closure/select全文；吸收display_name/unit、真实empty/truncated与manifest驱动稳定代表排序，保规格→计划→独立未选名册，不复制flat legend跨查询混用、缺值or0或每簇5/Top50常数。选择身份不是执行完成；参考清单不等同逐文件独审。此前marker完整node_map/itid/状态交集的发现继续用于精确账户交接，而不是补未产出的数值。

## 两例各一次，原合同未改

| 例 | 入口与预算 | 实际过程 | 人工/完整合同状态 |
|---|---|---|---|
| A | 原105元宝完整ftrace；15steps/n1 | exit0；原smokePASS；11query/3INIT/1HTML | 完整人工FAIL；实收原生表保真不等于问句完成 |
| B | 原代表选择自然问题；50steps/n1 | exit0；4query/3INIT/1HTML | 人工FAIL；同代终态NOT_TRIGGERED；operator未运行 |

真实模型完成请求配对A43/B18，歧义0，最大可证重叠21.865秒；不是依据进程同时存在推算，配对区间数不当作查询数。见[完整请求配对原件](results/hmc_native_publication_compat_20261030_v1/sealed-v1/0307-codrax-hmc327-actual-request-overlap-v1.json)。

A继续使用63659046B真实原件，SHA256 `be59c0648ac7e1d8a2d117f5dc36efa0c92b5f3686671719ada435c16713a20c`，路径`/Users/han/opt/donghu/基线数据/105版本/元宝/容器重启后冷启动/1、LOG_ftrace_20261019_17.17.44/ftrace-20261019_17.17.44.ftrace`。原问句/预算、物理头线程46571及marker载荷2227、业务窗口保持；独立整数μs oracle只供operator审计、不注入模型。独立人工FAIL：实际三个window_stats树3718/460/1548节点均只发起始行前32，省略3686/428/1516，两个主要performCreate self21.605/83.143未进入typed产物。两activity总量/自身59.695/6.770与111.487/3.938正确，不可据此声称四账户齐全；下游公平展示无法恢复未产出的child self。

另一已证交接缺口是span_window原生查询453bca…已经发布登录页完整HYLoginMainActivity名及46.600ms，3实际INIT却没有该账户；home总量103.315已到账。受理调查正确unit[1]没投递，只投递[0]/5、省略4531B；调查aggregate不授原生权限，修向须回到原生source/query/instance/window导航。没有实际Mutable快照证明整个原生ToolResult被历史预算丢弃，不能扩大归因。A实收4时序表2211行与24业务表312字段全部0错配，其中21张明确是其它线程背景，噪声不等于因果越权；D互斥桶、自段数31/18、早于home开始的两个child父身份及inclusive/self Running列仍被正文误述，独立留账。§326父账户窄窗问题本轮带限定，不误挂同一错误。[A原审计](results/hmc_native_publication_compat_20261030_v1/sealed-v1/0265-codrax-hmc327-A-audit-v1.md)23548B/SHA `13a6a0ece9e0c9d6752de8ceca68071ddd69cdb1f85b45179d01893f37249418`，不拿§326四账户及3493行旧证明充本批。

B原问题未添内部view或答案提示：按线程和10.000–10.050s窗口每类选两份代表，比较线程101/202的资源申请/释放及调用位置、概括进度。四份SQLite为两份完整夹具各复制两次，不冒四次硬件采集。discover正确使用request_plan resource_stack：8计划、2组、4选中、4未选；四选中真实查询各一次，终态3success＋1empty，4未选保not_executed。原生5事件/14帧、单位、独立组、未知字段和自动补齐正确。没有要求把未选成员补查才算完成。

完整人工仍FAIL：正文把原深度排列写成未限定含义的调用箭头/“栈顶”，并把4次查询写成“两次窗口查询”；正确原生表和后置caveat不能消除前文冲突。原生证据已足量，有限留账，不追跑此题求绿。[B独立报告](results/hmc_native_publication_compat_20261030_v1/sealed-v1/0286-codrax-hmc327-B-audit-v1.md) SHA256 `a716ee220ee32091dae0bda74418db737337fabce88e5629e88d2160ed000f99`；原机器结构验收not_available，不能把exit0或operator已构建写成机器PASS。

## P0系统缺口与同代交付边界

B成文INIT02第9–26行已完整带8计划/4尝试、3success＋1empty/4未执行及两侧各4成员、omitted0。3份INIT均没有saved_index/selection_snapshot及两SHA；唯一discover immutable index cd0ada…与selection e3b676…还是0次尝试/8未执行。没有执行后status/representative_status返回，原operator未运行，11.2不销账。

只读代码核证：`traceCatalogFinishQuery`完成真实记账后，`saveTraceCatalogAfterAttempt`→`saveTraceCatalog`→`Catalog.Save`使用immutable=false；它保存legacy alias和同run内存publication union，不产生fresh index/selection/SHA。`SavePublication`只由显式trace_catalog动作调用；finalizer目录仅取Mutable.CurrentExecution。故不是“fresh凭证已存在但模型遗漏”，也不是本次预算丢目录。后继将这一共享生命周期缺口升P0，确定性终态组件应复用同run受信Publication一次交付同代状态与不可变链接；status只是可选分页，不依赖模型额外记忆动作。不可读latest重建权限或给本批补验收，不把保存/选择当诊断完成。

另一P0是业务树目标完整产出与已产出账户跨通道交接；它优先于足量证据后的措辞调整。后继按typed来源/查询/实例/窗口公开复现，不放宽16+4/128行或完整来源门、不增大常数追绿、不以名字/原始问答关键词设硬门。Trace因果投影、自动补齐、合法计算与链上根因保留。

## 封存、发布范围与后继

本代无稳定ID关闭。[旧28档11026件/672来源快照核验](results/hmc_native_publication_compat_20261030_v1/sealed-v1/0356-codrax-hmc327-legacy-v1-receipt.json)保真，本代552原件于UTC04:27:40.009911封存；[索引](results/hmc_native_publication_compat_20261030_v1/sealed-index-v1.json) SHA256 `ba1e205759df90d3ebf9d7124d7e8a3a8901c6f915083ce9cf8fda72e7c8ae7f`，[封存收据](results/hmc_native_publication_compat_20261030_v1/sealed-receipt-v1.json)记录A/B/full真实退出0/0/0及独立机器/人工边界。封存不授通过，旧325/326不重封或补publication。

34文件代码776c8683940a777ec99cfac94bc51ff2e5f42afa与本批9文档按两相邻main提交交付；文档commit、远端main、清洁树与冻结/封存完整性只由[后置publication](results/hmc_native_publication_compat_20261030_v1/publication-v1.json)确认，不把本地代码提交先写成已推送，也不在文档内嵌自身commit SHA造成循环。完整[52项核账](results/hmc_native_publication_compat_20261030_v1/sealed-v1/0317-codrax-hmc327-census-plan-v1.md)与原勾选保持。大ftrace及同字节副本仅登记不入Git；query/INIT/答案不按大小裁剪，Go快照不进入编译树，providers内容不读/hash/copy。

下一批双轨优先处理上述两个系统问题，并轮换既有只读登记缺来源终态负控；未执行不能写PASS。sample-only、旧IO/采样解释、权限分支、图、未选补查后身份以及lower-cost-full公共入口均保留未覆盖/原FAIL。100版抖音NOT_RUN，WAL零副作用前生产NO-GO。见[本批手审](parallel_selected_summary_hmc_native_publication_compat_20261030_manual_audit.md)。
