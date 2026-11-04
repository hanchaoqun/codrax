# HMC343：库存主体、同源成文交接与只读负向终态

2026-11-03（执行UTC已至11-04），main基线08d1defbbc23bb0f402914482115b238425d4d9e，源码9758d3628b7aa27b0687c4bb912985949d2cb4b4。核对统一账：79唯一＝28完整＋51开放（18部分/29待实施/2待验收/1验收中/1持续），原勾选不变。完整能力新增0；已交付2项子能力、1组共享修复；剩余验收欠项3组，未触发另列。不是旧53项，不按文档出现次数计任务。

## 双轨与已交付范围

完整轨轮换原有只读登记“缺来源验证终态”负控，与完整用户抖音原题并行；共享轨解决§342已证的全线程库存/目标线程/展示集合缺少明确交接。13Go/499增21删（8生产/5测试）：

1. producer在原32节点截断前发布分层库存；保旧全线程NodeCount，明确物理TID才有目标库存、已展示、未展示。合法数字目标测得0不是未知；旧JSON缺metadata、进程/不能解析物理TID的名字不补0。SourceRef绑定复用原name-TID/纯数字/name (TID)解析器，精确整数校验，不猜名字/线程代次。
2. 查询预览与实际Finalizer共用库存说明，逐源/query先定义、完整账户引用；脚本renderer保必需0、可选nil和新增字段分类。全局/目标计数不再逐行重复，但不同查询仍独立。原inclusive/self墙钟、状态、离散分段、物理端点/行号、完整来源/查询凭证不减，不授完整成员/完成/因果权限。

参考`core/preprocess/context_index.py`与`core/llm_contract.py`的原件/导航分离、字段适用域及先定义后引用；原件路径不等于查询身份，不复制同step/type覆值、缺值补0、固定5/50、文本/容差/fingerprint权限门。保Trace因果投影、原4根因、闭合睡眠、链上业务线索、自动补齐和合法计算重试，不扫描问答原文关键词硬门。[设计边界](results/hmc_population_readonly_douyin_20261103_v1/sealed-v1/0322-codrax-hmc343-design-v1.json)为冻前原件。

两份各2783行timeline的内容相同，但索引padding为.25s/.5s且覆盖Notes不同；不能按值合并整publication或删除一份查询凭证。完整内容体与独立查询凭证分离复用、整组预算仍未实现，不将库存字典复用冒称通用展示去重完成。

## 冻前验证、真实材料与唯一全仓

旧码公开RED保原；新字段分类遗漏、夹具继承目标/误期望行数、合法name-TID被拒的真实兼容回归均保原。修producer/共享parser与精确夹具前提，不降低旧断言、原预算或问题/oracle。最终同一源码：

| 检查 | 结果 |
|---|---|
| 新库存与字段处置4包 | 68顶层/205子PASS |
| 相关7包 | 236顶层/678子PASS |
| race×2四包 | 56顶层/110子PASS |
| 原4根因、闭合睡眠等四包 | 21顶层/141子PASS |
| 完整实采公开工具到actual Finalizer | 1query/0模型PASS，非第三次模型eval |
| 正常CLI build、零模型/零工具startup | PASS |

[最终冻前收据](results/hmc_population_readonly_douyin_20261103_v1/sealed-v1/0511-codrax-hmc343-prefreeze-final-checks-v1.json)逐项绑定源码和原始日志。认真使用完整用户材料：`/Users/han/opt/donghu/基线数据/100版本/抖音极速版/应用后台销毁后冷启动/1、LOG_ftrace_20261019_9.55.40/ftrace-20261019_9.55.40.ftrace`，235422121B、1696481行，SHA93b45bcca5c9e9b415478d81e176b633d70886328c49731b5258c806695de1ef。不是裁片或模拟录制。模型外工具原件/raw清理前留存；复用旧338独立整数B/E账2611，不重跑旧parser，不冒独立调度/因果oracle。

冻结7608输入/441外部/22快照，SHAa9ee77a1323a622383f4815b99b28abb41e9e939feb182b2ccb40e583158f393，之后源码/测试/教学/helper/case/oracle停写，仅只读采集审计输出与批文档。唯一`go test -json ./... -count=1 -timeout=30m`于UTC11-04 01:25:54.590734→01:40:03.911392，实际OS0、源前后true：98包PASS/12无测试，28739顶层PASS/49SKIP、34414子PASS/14SKIP，267717纯JSON，零FAIL/非JSON/重复/未终态。log SHA352f042ca9b93bcab696dca1ee838551d98225edbb5ba1eb38432ff314b77564。[正式收据](results/hmc_population_readonly_douyin_20261103_v1/sealed-v1/0372-codrax-hmc343-full-v1-receipt.json)、[独立统计](results/hmc_population_readonly_douyin_20261103_v1/sealed-v1/0371-codrax-hmc343-full-stats-v1.json)不借用前代结果；没有第二次full追绿。

## 恰好两原例各n1

| 例 | 原入口与范围 | OS/原smoke | 人工 |
|---|---|---|---|
| A | 完整235422121B抖音原题、15steps | 0/PASS | 整答FAIL；新库存交接自然触发 |
| B | 原只读缺来源终态测试、planner12步 | 0/原负控PASS | 限定PASS；非自然controller/fullCLI |

A 6query声明对应5唯一JSON；20工具声明/20参数文件不是40次执行，一次exec导航明确拒绝。实际INIT03含全库存25139、目标TID57874库存2611、原生展示32、目标省2579，16完整账户及上下文省略分别说明。32节点对旧整源整数wall/端点/身份/层级账256项零差异；16主要阶段和2783状态区间原生保真。整答仍排除CPU争抢、时序列表倒序、32节点错套1429直接子集合、非零运行说全睡眠及有子项说叶子；不将正确表/尾注当人工通过，不改正确原生值迎合正文。

A三INIT100509/14102/314046B含提取分隔换行，不冒provider wire/token成本。两次旧reason提交存在错误，但三INIT逐字/JSON转义均不存在，不能臆定直接污染；初稿误判已撤回，原稿与更正证据同时封存。MD965741B/HTML4269117B、12main表cell诊断一致；模型重复16阶段表，两份各2783行时序亦重复，仍为共享P0。日志含无效UTF8，原字节保留；一处只读文本诊断编码错误不修原件。无图/patch/像素QA。

B真实planner、脚本controller、公共原生执行与独立进程恢复，源码/测试/HEAD不变。1断言/2命令/4义务分列；来源只有apply，proof verified而overall unverified/missing_terminal_verify_verdict。真实WriteFinalReport及同代final→report链接一致，父子4原件留存；新invocation不复用旧结果。非自然controller/fullCLI、context-pack耐久false，不以有界负控补签整个只读能力。

精确唯一区间A14/B2、0歧义，最大真实请求重叠3.895秒，不由barrier/进程重叠推断。[真实请求证据](results/hmc_population_readonly_douyin_20261103_v1/sealed-v1/0235-codrax-hmc343-actual-request-overlap-v1.json)；无第三例追跑。详[逐层人工审计](parallel_selected_summary_hmc_population_readonly_douyin_20261103_manual_audit.md)。

## 全部51项ROI与下一双轨

| ROI | 数量 | HMC稳定ID（省前缀） | 推进依据 |
|---|---:|---|---|
| P0共享 | 7 | 16.4、01.3、02.4、16.5、01.2、18.4、12.5 | 完整内容体/独立凭证复用、整组预算、运行时来源分类；原生与成文适用域 |
| P1现有材料 | 6 | 08.3、18.5、11.2、03.2、14.1、18.2 | 轮换原有限IO/资源、自然只读正负终态及实际材料验收，不无限追同题 |
| P2复用 | 11 | 04.3、04.5、08.1、08.5、12.2、12.3、17.6、17.7、05.2、10.2、14.2 | 有真实格式/平台材料先做端到端，不把交叉编译冒本机运行 |
| P3协议/来源 | 21 | 04.4、04.6、05.3、06.2、06.3、08.6、08.7、08.8、09.1、09.2、09.3、10.3、12.4、12.6、13.1、13.2、13.3、14.3、14.4、14.5、15.1 | 先证明来源/协议/权限，不以参考代码或邻近信息补客户证据 |
| P4新域 | 6 | 07.1、07.2、07.3、07.4、15.2、15.3 | 依赖/真实材料齐备后 |

[核账快照](results/hmc_population_readonly_douyin_20261103_v1/sealed-v1/0317-codrax-hmc343-census-roi-v1.json)覆盖全部51ID无重漏。剩余验收3组：

1. 完整成文范围、时序、状态和机制解释；旧资源/采样/IO仍按原ID/原FAIL，不将充分事实后的偶发误述无限升级为新硬约束。
2. 共享内容体与独立查询凭证、跨块整组预算、运行时query产物来源分类与导航降噪。INIT中grep产物泛标current_source而read_file为runtime_artifact是具体线索，尚不证明制造源码义务或导致A失败；优先公开复现、泛化修复，不值同合query。
3. 代表规格、权限/图、其它真实二进制/平台以及自然只读CLI终态。未触发不冒FAIL已修，也不拿旧绿代签；本批sample-only/代表/图及其它二进制NOT_RUN。

下一共享轨优先第2组已证系统面，完整轨轮换有实物二进制与原有限IO/资源，随后自然只读正负终态；旧18.3不重复算交付。WAL未证明零副作用前生产NO-GO。

## 原件与main发布

624件index SHAdb1e52056a6fce2745674515dd63e5ae90c0399d4f8218c4d1983c0e242cc409、seal receipt SHA18ef923af80c7392a3d60184ac1c5c8e93420c90a8df3a81c7a865d3336425a4；[root closure none](results/hmc_population_readonly_douyin_20261103_v1/sealed-v1/0234-codrax-hmc343-acceptance-v1.json)。45代按代19709件非全局去重，仅物理重核最新342的519及更早43代链头；旧失败不回签、不补成功publication。大实采原件只引用，providers含synthetic不读/哈希/复制；忽略的本地证据不冒全部已上传。

源码与本批五文档相邻提交，main/远端/清洁工作树/7608冻结输入及封存原件由[publication](results/hmc_population_readonly_douyin_20261103_v1/publication-v1.json)独立核验；完整任务28、开放51保持，不把A人工FAIL重签。
