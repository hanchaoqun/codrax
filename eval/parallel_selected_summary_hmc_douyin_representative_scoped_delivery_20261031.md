# HMC330：物理成员业务量测与代表原合同复验

日期：2026-10-31 UTC。main基线 `116b367b32a1c05781bf4b753602f217d49298ad`；5文件实现 `d82037ac67286f34625f53cc0a6c5dc058d91aa8`。复算 **79唯一＝27完整交付＋52开放**，重复0；18部分、29待实施、3待验收、1验收中、1持续。**本批完整能力新增0，交付子能力1项；两例整答人工均FAIL，原代表operator FAIL，root明确closure=none。**

实现与下述5文档紧邻提交；远端main、冻结源码及封存的最终一致性以[publication收据](results/hmc_douyin_representative_scoped_delivery_20261031_v1/publication-v1.json)为准，不由本地提交推断。旧失败不回签，WAL零副作用前不进生产。

## 1. 一项有公开反例的共享修复

组合输入的SourceRef.Path保留查询索引，而原生业务配对属于物理成员；旧等值判断导致6个已存在配对/2个查询没有量测摘要。修复复用可逆来源映射，要求唯一成员、完整区间、有效本地行和唯一精确SupportRef；表中保物理来源，查询入口另列。不会以同名、同刻或同字节合并实例。单源显示不变，部分/歧义/隔离/异源/陈旧不能借权。

资格只限已有业务区间量测，不造self、调度状态、等待机制或根因，不放宽组合window_stats的调度身份。Trace因果投影、自动补齐、闭合睡眠、原4候选及write隔离/审批/合并红线保留。没有扫描用户问题或模型正文建硬门，没有新增提示词或扩大固定预算。

公开工具RED2实际复现6→0；[原失败日志](results/hmc_douyin_representative_scoped_delivery_20261031_v1/sealed-v1/0496-codrax-hmc330-span-composite-public-red-v2.log) SHA `acfd63927054611144c5e5903b43cf43e2d97ed226a9603cad7ebf4dbbbcf019`。RED1还含macOS路径别名夹具错误，原日志保留；v1源码逆向重建及v2事后快照均不冒运行前快照。修夹具未弱化断言。GREEN为4顶层/19子例；实际context/finalizer/emit桥接另1例，6实例形成6表，旁路权限不变。自然A/B没有触发复合配对发布，仍明确NOT_TRIGGERED。

参考仓实际审读marker_ops完整node_map、callid/线程实例、区间状态交集和closure/select的同规格/完整名册意图。复用本项目既有CMP物理成员映射，不另建内核，不照搬直接子时长求和、缺失补零或Top前缀。参考实现不是本代oracle，也不冒它已提供本项目的组合坐标合同。

## 2. 验证与严格冻结

| 验证 | 实际结果 | 限定 |
| --- | --- | --- |
| 定向 | 5包，204顶层/360子例PASS，1可选SKIP | SKIP不是PASS |
| race | 4包，40顶层/58子例PASS | 非全仓race |
| 唯一full | 98包PASS/12无测试包；28679顶层PASS/44SKIP；34146子例PASS/14SKIP | 266268 GoJSON；三层FAIL、非JSON、重复及未终态均0 |
| A | CLI0，原smoke PASS，完整人工FAIL | 原100版抖音自然题15steps/n1 |
| B | CLI0，原operator1，有限原合同FAIL、整答FAIL | 原代表自然题50steps/n1；不改规格断言 |

全仓PID69215，UTC08:03:53.685387（parent spawn前）至08:17:17.438425；signal=null，source before/after=true，log SHA `675bcf395884f80a8a17593ad04d089168e45c4f285c947cd7525bac7c592ee3`。[完整统计](results/hmc_douyin_representative_scoped_delivery_20261031_v1/sealed-v1/0316-codrax-hmc330-full-stats-v1.json)另存。

CLI与原operator各构建一次，同源码库存 `770107d2f7b17194c9c02153076172bf628b69ab8641ea258bbf8b6b79b2ad4f`；无隐藏重建或testbin。四准备阶段、自检26项及零模型preflight实际通过。冻结7568仓库输入/201外部/14仓外快照，SHA `4593c34fd90bc9ebf78e597af5844fabb8c2b4183049abb32b1ae2c554ac0104`；随后代码/测试/注释/教学/helper/计划/问题停写。

两例各一次。原日志识别A40/B16请求头及各40/16响应头；仅A6/B16区间能唯一配对，A另6歧义群组共34请求保留、不猜邻近配对。最大可证请求重叠12.533秒，非进程并存推断。各例3个INIT文件是同一次成文的system/user/user消息，不是三轮评测。invalidUTF8日志原字节保留，不宣称编码已修。

## 3. A：完整真实抖音材料，事实充分而解释仍失败

原件位于 `/Users/han/opt/donghu/基线数据/100版本/抖音极速版/应用后台销毁后冷启动/1、LOG_ftrace_20261019_9.55.40/ftrace-20261019_9.55.40.ftrace`，235422121B/1696481行，SHA `93b45bcca5c9e9b415478d81e176b633d70886328c49731b5258c806695de1ef`。实际回放完整原件，不是切片、预览或旧105元宝数据。

审计侧独立整数微秒标尺重新流读全文件，不调用生产/参考查询器，也不进模型。根与1429直接子共1430账户；根账户总669.351ms/self227.101ms/子并集442.250ms。header TID/TGID57874与marker payload PID10266分开；自身墙钟不等CPU执行。整体75个ftrace/15场景/9995542762B的材料清点只证明可用库存，不冒全部已评测。

9个成功query回执对应4份唯一载荷；主要账户已进成文上下文。最终HTML4表：1行原件信息、16行主要阶段、6行线程状态（含合计）、2783行完整原生时序。16组业务起止及64量测格、时序全部12列同源核对通过。完整深层账户与原生核对；独立标尺只序列化根/直接子，不能声称所有深层self均独立复算。

完整人工FAIL：小Runnable比例被扩大为排除Running阶段算力供给效应；5509匹配/200预览且最终无事件明细表，却说“全部打点行已展示”；0.634ms被说小于0.6、三处self概括与同答锁段全self冲突。self墙钟写成自己执行的风险继续留账，但合法Running487.440ms/72.8%及子并集差值不误罚。实际INIT有16账户及状态完整账；强错误调查句并非全部重传，投递[0,2,3]/7，不虚构关键事实丢失。

另留同时间戳政策差异：118835/118836同刻out/wake后6µs，161365/161366同刻后10µs，原生保S/D而独立行序标尺记Runnable。Running及总窗一致；原生8.698/170.924/2.289ms与标尺8.714/170.918/2.279ms只差这16µs。跨CPU行序不暗定真实先后，不定bug、不称模型发明，不冻后修闭合睡眠。

## 4. B：同代交付通过，但原代表规格没有通过

原题、原operator不变。四输入为两类完整构造SQLite的复制件，不冒四台设备捕获。自然流程选择event_search声明：8计划、2组、4选/4未选；随后4次合法resource_stack补查。当前12记录、8真实尝试7success+1empty、4未执行，同代index `ec55f086…`/selection `464b1420…`及两个SHA在正常终态和最终答案一致，不由latest或事后status补造。

原operator要求resource_stack代表声明，唯一实际FAIL：`cluster specification or full membership differs`。这不是无关栈措辞阻塞11.2，而是原有限用例的规格自身偏移。通用选择/完整名册/同代交付的有限PASS保留；额外查询不能替换原计划，也不把合法补查算作错误。root明确本代不关闭11.2，不重跑追绿。

原生5资源事件、14帧和独立分组保真，18张MD/HTML表一致（仅代码标记渲染差异）。整答仍FAIL：含10.050端点的原查询5匹配附在半开窗标题，4显示成员是排除端点而非预算漏行；d202零事件/零帧却被首段说有堆入口；每来源2声明/1代表混写。当前event_search d202零匹配实际登记success，资源补查才是empty，两次状态不互改。实际INIT中“统一窗口”错误unit[0]确实投递；其它错误被拒/省略，不能冒全部污染。箭头、深度词本身不是硬失败门，旧栈关系误述不倒灌本轮。

## 5. 封存、剩余验收和下一ROI

本代504件封存，[index](results/hmc_douyin_representative_scoped_delivery_20261031_v1/sealed-index-v1.json) SHA `111ec43148554d35614bab3c960f0297581a68c6fa4904abef65a312ca7c1523`；旧31代12509件/790来源快照身份保真。完整查询、实际INIT、MD/HTML、原拒绝/失败与独立报告均保留；235MB原件按完整身份引用，不把全量查询按大小丢弃；Go快照在编译树外。providers密钥不读/hash/copy。

[A完整审计](results/hmc_douyin_representative_scoped_delivery_20261031_v1/sealed-v1/0235-codrax-hmc330-A-audit-v1.md) SHA `8ea9c44560fbcb055db16326217b7a310806bccf494da57f182cfda1d5188702`；[B完整审计](results/hmc_douyin_representative_scoped_delivery_20261031_v1/sealed-v1/0257-codrax-hmc330-B-audit-v1.md) SHA `4f0d1f574465718bd0b509fa36e959e34f1d53f72936f95014d8a88ab976eeba`。两例均无图/patch，未做像素QA；根因合同未激活不增加原题退出门。

剩余52稳定项的完整无重漏[ROI名册](results/hmc_douyin_representative_scoped_delivery_20261031_v1/sealed-v1/0352-codrax-hmc330-plan-v1.md)仍为共享7→已有材料验收7→原生扩展11→协议/来源21→新域6；规划阶段“尚未运行”不冒当前终态。下一双轨优先：结构化目标到正确查询规格/声明/补查归属，以及查询身份/端点/原总体/投影到解释和表格的绑定。随后轮换IO原有限合同、只读登记负控及二进制验收，不能把已验证入口重复列成未施工。

本批两个整答失败及代表有限合同失败未销账；旧9组债、采样/IO/权限、图/patch、h2/h3、lower-cost-full、复合自然分支和更多平台材料继续开放。16µs先独立核规则；充分事实后的偶发误述记录即可，不追加第三例、问题提示答案或业务名硬门。WAL未满足零副作用不接生产。
