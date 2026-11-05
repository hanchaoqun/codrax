# HMC350：完整账户预算、整件元宝与原有限IO验收

本批日期按America/Los_Angeles为2026-11-04；原始过程UTC为2026-11-05。全部在main，基线1eb828c87f1da30dc7091f349ccb49a9832afa16，源码提交62dbf18204cab2c42f2757a3401de032b30980c3，5Go/400增8删（2生产/3测试）。源码/测试/教学/helper/case/oracle冻结后停写，仅终态审计和五文档收口。前349正常Git推送和原publication已成功，历史失败/待传收据保原；分支归并不回签人工FAIL。

## 结论与分账

79唯一＝28完整＋51开放（18部分/29待实施/2待验收/1验收中/1持续），checkbox不变。完整能力新增0；已交付子能力1共享组：原16完整账户预算中保query公平并软优先不同完整内容。剩余本批验收：两例完整人工FAIL、08.3原有限FAIL、新同窗自然分支NOT_TRIGGERED；不销账、不追第三例。旧采样/IO、代表/权限/图、只读正负终态与更多二进制材料覆盖保持原账。

## 一组系统修复

旧公开双query/同窗路径16槽只有8不同物理实例，复现§349实际缺口。每组首个精确owner/root仍单独占自己的槽，后续公平轮次优先没展示过的完整内容；没有新内容仍可选重复，不合并query或删除原件。原16条/16+4扩展、窗口/钟/目标/行/Notes、独立population和权限不改。Novelty只排序，不授来源、完整性、物理关系、根因或完成权。

完整真实源暴露外层payload因索引/扫描元数据而不同、账户体相同。沿既有私有来源API复验物理代次，才把payload定位与完整体分开；JSON恢复/陈旧/冲突ID不能借key，无凭证保payload。纯ledger重放关闭RowSetWriter，非新查询/I/O。完整内容比较仅去发布ID/观测时间/call/query/text定位，目标/范围等保留；不是按业务名拟合或从问答关键词硬判。未改prompt/schema，不增加模型字段。

参考仓`core/preprocess/context_index.py`的逐步索引/产物分离与精确键设计。发现其path-only索引、同step/type value覆值和全局must_read不适合独立证据：本项目只采用职责分离，保物理代次、逐query资格和完整Notes。参考仓无修改。

## 原始材料、失败原件与测试

基线目录75份ftrace，100版抖音极速/105版网易云、元宝、蜂鸟、抖音极速各15份。使用完整网易云82838676B/559592行，SHA12349db5bbdc5a4343360d9633c7d6c36f18c8be5f39fb64e78b53c9e345bfe4；完整元宝63659046B/412546行，SHAbe59c0648ac7e1d8a2d117f5dc36efa0c92b5f3686671719ada435c16713a20c。两份均公开实际query→Finalizer双query测试16条/15物理/2独立query，原件/ledger/因果投影不变；不是把裁剪夹具冒原件或把零模型测试算第三评测。

新测试green-v1因错比stage outbox而FAIL，改比Turn A原件；完整真实v1/v2因外层payload差异FAIL，v1仅log、v2完整native原件保留，不能伪称v1载荷齐全。修复用既有物理代次owner，非放宽payload/权限断言。工装IO常量遮蔽Ruby类的错误在冻结前修好，原部分文件及错误收据保留。旧测试断言不动。最终相关4包40顶层/98子、race×2两包42/66、原因果4包21/141、双原件1顶层/2子PASS；常规测试的实物选择SKIP与实际env整件PASS分列。

冻结SHA97bb522a5cea8a1bcde1f20b9ad5980cd288e17aa7ea8721094ea465e8a7e638：7632源码/用例输入、395外部、15仓外快照。工具链SDKROOT固定MacOSX26.5.sdk。实际runner argv先以0模型/0查询捕获，15正负身份检查PASS，问题/15步/同物理原件不换；同字节不同原件负控拒绝。providers内容不读取/哈希/复制。

唯一全仓`go test -json ./... -count=1 -timeout=30m`，UTC02:00:27.962720→02:14:17.916581，OS0，冻结前后校验true。98包PASS/12无测试，28763顶层PASS/55SKIP，34576子PASS/14SKIP，268401纯JSON；FAIL/重复终态/未结束0，log SHA96f1497e16985f802620dac660c85bd9fb224c1f283d38f26b1d8ad59a1e1cd8。无第二full。

## 恰好两例原自然评测

| 原例，各15步/n1 | 进程/原smoke | 完整主审 | 有限/新分支 |
|---|---|---|---|
| A `donghu_v105_yuanbao_activity_instances`，完整用户采集 | exit0/PASS | FAIL | 同窗重复内容自然NOT_TRIGGERED；正确业务四量测不代签整答 |
| B `trace_io_inflight_overview`，原1839B有限夹具 | exit0/PASS | FAIL | 原08.3有限FAIL，仍验收中 |

A/B实际exec UTC02:00:30.473062/30.473063，同一冻结CLI；机器并行另由精确请求/响应证明：A34/B7唯一完整区间，歧义0，最大重叠27.486秒，不从barrier推断。34/7是请求次数，不是评测次数。两例各只一次。

A：11原生JSON、40声明/40参数分列，实际INIT409919B。16完整query记录/12物理/3不同窗覆盖3/3组；两activityStart59.695/self6.770、111.487/self3.938及主要performCreate46.600/self21.605、103.315/self83.143已到场。96原生业务账户与本轮独立整件整数B/E标尺0差异；2211最终时序行逐native精确匹配。仍错将父事务完整末点3733.863427接给activityStart，后代称直子，过宽否认既有等待位置绑定；MD389726B、HTML3373513B，836行整窗时序重复两份。新自然分支未触发不能冒15物理的零模型测试为自然结论。

B：2原生JSON、7声明/7参数分列，实际INIT274810B。原4分组按family/dev/op的peak/mean/busy/area/pairs/issues全部正确，并保未配对1/歧义cohort1/抑制2与采集完整性未知。正文由低深度断言设备无排队/很闲、把issues与pairs差归窗外完成、将request·ms说成设备槽位时间，有限FAIL。9RQ+1BIO明确标为端点事件合计合法，不禁止一切跨层字面算术。HTML2800712B，两个4行RQ明细表重复。

A受理8解释单元仅0/1完整进INIT，坏unit7未投；B完整坏reason及34面积/41.2%未投，不冒直接污染。A一次1004B真截断emit_analysis被执行前拒绝，后自然恢复；1参数blob不可解，A/B日志非UTF-8行3/1保原，不能抹成全程成功，也不冒充新原生事实截断。无图/Mermaid，HTML仅源级人工核验，未做像素验收；图不暗加为本题退出条件。详[手审](parallel_selected_summary_hmc_account_novelty_yuanbao_io_20261104_manual_audit.md)。

## 全部51项：ROI高到低，无重漏

以下是排队优先级，不是新增稳定任务；原状态/依赖/退出条件见[实施清单](../docs/design/hmosperf_implementation_tasks_20260917.md)。完整逐ID机器核账见[冻结census](results/hmc_account_novelty_yuanbao_io_20261104_v1/sealed-v1/0386-codrax-hmc350-census-roi-v1.json)。

| 顺序/数量 | 原稳定ID | 推进理由 |
|---|---|---|
| P0共享7 | 16.4、01.3、02.4、16.5、01.2、18.4、12.5 | 量测/解释适用域、来源和整组预算、统一JSON/图；优先系统缺口，充分事实偶发误述独立记账 |
| P1有材料验收6 | 08.3、18.5、11.2、03.2、14.1、18.2 | 在途/轮换、代表选择、资源位型/统计与只读测试登记；逐项原出口验收，不暗加WAL/回写要求 |
| P2原生复用11 | 04.3、04.5、08.1、08.5、12.2、12.3、17.6、17.7、05.2、10.2、14.2 | 业务/跨层时钟、IO/CPU、帧连接、真实二进制/安全SQLite、可比性/资源代次 |
| P3协议/材料依赖21 | 04.4、04.6、05.3、06.2、06.3、08.6、08.7、08.8、09.1、09.2、09.3、10.3、12.4、12.6、13.1、13.2、13.3、14.3、14.4、14.5、15.1 | 通用底座完成后按版本化协议/真实材料推进，不由邻近/名称/经验模型授硬权 |
| P4新域6 | 07.1、07.2、07.3、07.4、15.2、15.3 | 媒体生命周期、可选静态适配和建议写链，保持风险/审批/worktree/验证 |

后继三组失败/欠项挂原ID：①主体/原窗/完整实例与关系到解释；②IO群体/量尺和物理权限；③最终输出重复/跨块整组预算。另记自然同窗偏好、代表/权限/图、只读正负终态、更多真实格式未触发覆盖。下一双轨优先共享整输出复用/预算，完整轨轮换已有材料的资源/二进制与只读终态；不让本轮充分事实IO误述无限阻塞其它ROI，不重复计18.3已完整交付。

## 原件封存与发布

[621件索引](results/hmc_account_novelty_yuanbao_io_20261104_v1/sealed-index-v1.json) SHA00d6a64040896ff2de6e1edd065f9065abc2f3d4331eb1b459f853cd817678d1；[seal](results/hmc_account_novelty_yuanbao_io_20261104_v1/sealed-receipt-v1.json) SHA1988eac37dbe5322938dfa2a4782c0e803817c08b148128e7688e407cd840326；[root acceptance](results/hmc_account_novelty_yuanbao_io_20261104_v1/sealed-v1/0335-codrax-hmc350-acceptance-v1.json) closure=none。前51代22924＋621＝52代23545件按代非全局去重；物理重核349517与失败344554，其余49代只链头。原件、失败、原始JSON/actual INIT/HTML/MD均本地ignored证据树保留，整件大trace只引用完整身份，124MB编译物不塞Git。

源码与本批五文档为main相邻两提交，远端/清洁状态另以[publication guard](results/hmc_account_novelty_yuanbao_io_20261104_v1/publication-v1.json)为准，不以本地commit或历史候选同步代签。Trace因果投影/自动补齐、原4根因、闭合睡眠、链上业务/语义优化与合法计算/重试保持；WAL未满足零副作用前生产NO-GO。
