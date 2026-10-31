# HMC329：严格教学兼容修复与二进制／只读登记轮换

日期：2026-10-31 UTC。基线 main `41274552a308ce14f1a87e245cf741ae59154a29`，30文件实现提交 `81cc27c5f1f51ab6e83f606cbda251c4d126c5d7`。根与独立代理均复算 **79唯一＝27已交付＋52开放**：18部分实施、29待实施、3待验收、1验收中、1持续。**本批完整能力新增0；有界产品子能力3组；本批两例人工1限定PASS／1完整FAIL**。旧验收欠项不消失，closure=none。

代码与7份文档紧邻分笔提交，远端main与冻结源码的一致性交付以[publication收据](results/hmc_teaching_compat_binary_registration_20261031_v1/publication-v1.json)确认，不由本地commit推断。§328保持独立失败代，不给旧代补publication；其完整人工两份FAIL和全仓7项FAIL均保原结论。

## 1. 先闭环确定性回归，不修改旧断言

§328唯一full发现两类原因、7个顶层失败。本代仅修测试接缝及canonical golden，继承的生产逻辑不再改动：

- 新代表交付夹具的3次含mutex结构体值复制改用既有 `ShallowClone()`；保原fork、查询、合并、撤权及终态断言。
- 业务树“完整统计后按精确物理线程优先、有效闭合inclusive降序选择”的教学已合法改变，但6个历史说明校验未登记该演进。新增独立旧／新固定文本的单次精确逆变换；旧全串SHA `9575f64e48d3f7de1022e4059b181028b1eb28d8c497835d94c6fb790e41be8e` 不变。缺失、重复、混入旧段、仅旧段、权限改写及段外变化均保严格负控，三处公共教学面一致。
- canonical golden只经原有授权生成入口更新一次：41687→42001B，精确314B已批准差异，SHA `353d6b62c34e7be306f114a9c4f18361c56f4dd73c70637a1d371764568cb347`。生成不是验证；后续定向与唯一全仓另行通过。没有更新旧SHA追绿或删负控。

3组产品子能力为：①完整业务树统计后，在原32节点和16账户预算内按精确目标选择，保完整节点总数／省略数、无目标旧序及未知不当零；②真实 `span_window` 的完整命名区间进入typed原生事实，查询窗与完整实例测量范围分开，不造self、调度、机制或因果权；③代表查询汇合后自动交付同代不可变index＋selection＋SHA，覆盖真实分支尝试、失败和未执行，重复终态复用同字节，不读latest恢复权限、不代查询或诊断完成。

## 2. 参考实现与真实材料

§328已直接对照参考仓 `core/preprocess/marker_ops.py` 的完整树/区间统计，以及 `core/batch/closure` 的成员消费前核验与原子交付意图。迁移的是先完整取数、再有界展示和来源/代次边界，不复制前缀截断、近邻因果或第二执行内核。其105版元宝原始63659046B ftrace已实际回放；四账户及主要child self在真实查询和成文INIT到场，完整解释仍FAIL，详见[失败代汇总](parallel_selected_summary_hmc_terminal_business_publication_20261030.md)。

本代根与准备代理均完整读取参考 `core/extensions/AGENTS.md`，并读 `hiperf_binary_parser.py` 1–110、156–235：结构解析与符号解析分开，保精确整数时间/PID/TID/CPU/period；BRBE cycles=0还须看unknown/valid，不能凭默认零造零成本。CPU号或period不授Running、耗时或BRBE能力。见[参考发现](results/hmc_teaching_compat_binary_registration_20261031_v1/sealed-v1/0322-codrax-hmc329-reference-findings-v1.md)。仅列入冻结不等于已经语义审读；参考parser本代未重跑。

## 3. 实际验证

| 验证层 | 实际结果 | 边界 |
|---|---|---|
| 定向 | 5包、199顶层／341子例PASS，1可选真实材料例SKIP | SKIP不算PASS；§328真实材料执行另有原件 |
| race | 4包、35顶层／39子例PASS | 不是完整全仓race |
| 唯一full | 98包PASS／12无测试；28674顶层PASS／44SKIP；34127子例PASS／14SKIP | 266172条GoJSON，三层FAIL、非JSON、重复终态、未终态均0 |
| A | OS0，原smokePASS，完整人工FAIL | 原自然sample-only题15steps/n1 |
| B | OS0，原机器及限定人工PASS | 原缺来源终态负控，真实planner＋脚本controller＋新进程，不冒fullCLI |

全仓PID38051，UTC07:18:31.278003（parent spawn前）至07:31:39.666048真实终态，signal=null，冻结源前后均true；log SHA `6987546e3c3a1033fe6e0a8d4ab9218d2e65bea019f4fc5a80e08670037eb9de`。CLI与原orchestrator testbin各构建一次，绑定相同源码库存 `08ce1c518cb1ea8d11db060cc9032ac07523e7246977239d1db85671e091c6c5`，无隐藏重建。两例真实请求A8/B2完整配对，歧义0，最大可证重叠1.109秒；不以进程并存替模型并行。

## 4. 两例完整人工结论

A完整PERFILE2原件30062585B，SHA `da713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632`，未预转换／裁样本。5次真实查询、3INIT、1HTML；独立核全部12000条已存scalar与本代生成记录。用户半开窗内1393条跨11线程；PID1514/TID1609目标164条／259权重，两热点各82条，分别174和85权重，CPU1/0与完整包络正确。40/164事件预览、宽窗515/1156、原件／预览／bundle大小均分开；最后window_stats自动继承遗漏的TID1609，原生5表同源保真。

完整人工仍FAIL：正文3处将unknown样本类型断言为线程已在CPU执行，实际INIT与同答表均明确状态未确立。两次调查reason有错误但未重传到INIT；预算省略0，目标核心事实完整，不归因为丢事实或摘要污染。122.483125ms记录间隔是合法有限算术，未被写成等待；单帧depth0为原件实况，也不另加失败。原文保留、不追第三例。较多内部quality字段／多查询重复说明仅为降噪候选，不能冒本次已证高影响根因。

B保原问题、12planneriterations/n1：2次真实provider请求，2次完整读入，唯一 `changes=[]` 登记受理；持久化null仍是0变更，不混原始JSON。fresh子PID38198≠父38053，真实公共RunTests本轮1断言、1原生执行＋1语法检查＝2命令，invocation不同于旧结果。来源始终只有apply，没有verify attempt／VerifyRef／Completion；proof verified，但整体unverified／missing_terminal_verify_verdict。源码、测试、HEAD均不变；父子真正 `.final.json` 及4个原件同字节，精确指向同代内容寻址report。workflow仅context-pack索引回填有差异，不能称全字节一致或durability验收。无图、无Markdown自然收尾要求，不暗加或补签。

完整独审：[A](results/hmc_teaching_compat_binary_registration_20261031_v1/sealed-v1/0178-codrax-hmc329-A-audit-v1.md) SHA `7213328bbaa289ade67253fba93ecfea4004f9fba769acf1485ada3aa963726e`；[B](results/hmc_teaching_compat_binary_registration_20261031_v1/sealed-v1/0200-codrax-hmc329-B-audit-v1.md) SHA `8dd180eab467ce6f2d3ed8215f3107002a9f65584ec8b5a4ab21a9fe76af1b70`。仅HTML源审，不冒像素QA。

## 5. 封存、开放项与下一ROI

冻结7565仓库输入、200外部输入、42仓外快照，manifest SHA `c13f8601a77a37f6a60d97c5be95bbaa521ac7284a8d0d66fc2c39474665f478`。冻结后源/测试/教学/helper/计划/输入停写。旧30代12103件、748来源快照身份逐件保真；本代406件封存，[index](results/hmc_teaching_compat_binary_registration_20261031_v1/sealed-index-v1.json) SHA `6c60219735778fefc898f8240051fe404bc89c146eb4c1bc20b71090048b51e1`。全部原始失败、完整查询、INIT、输出及审计保留；原始大文件仅按完整身份引用，Go快照在编译树外，provider密钥不读/hash/copy。

全部52项的[无重漏ROI名册](results/hmc_teaching_compat_binary_registration_20261031_v1/sealed-v1/0224-codrax-hmc329-census-plan-v1.md)保留；该文件的阶段状态是冻结前规划快照，终态以本汇总和真实收据为准。排序为：共享准确性7项→具备材料验收7项→原生共享扩展11项→依赖真实协议/来源21项→新域集成6项，合计52，不表示都已实施。

后继完整能力轨先逐条核11.2原有限出口：§328真实代表选择/同代交付/operator已局部PASS，不能因无关栈解释无限阻塞，也不暗加聚类/回写/WAL条件；本代冻结closure=none，不事后改签。共享轨优先统一原生账户与解释的来源/查询/实例/范围及parent/child限定；已有足量事实的self墙钟→CPU、D/IO、栈深度→调用边和采样状态误述继续留账，不能通过关键词硬门改文。原9组重点验收债仍保留。

跨视图复合来源、多实例自然发现、h2/h3教学live、lower-cost-full公共分支、其它二进制格式/设备、旧IO/权限/图分支未覆盖；100版抖音仍NOT_RUN。继续使用已登记真实Donghu材料，不把105元宝一次成功取数泛化为全域完成。WAL零副作用前生产NO-GO；Trace因果投影、闭合睡眠、原4候选、自动补齐与write隔离/审批/主仓合并红线保持。
