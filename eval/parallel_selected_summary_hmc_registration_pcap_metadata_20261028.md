# HMC305：共享结构演进、附件元数据与合法小计边界

日期：2026-10-28。基线`8e2c14bb7ec63058c9dc4047fdae3cba833f87eb`，候选`codex/hmc299-wait-interval-candidate`；main保持`788588e365d40a48b30827ffd8db08dae4e2ae9d`。本批冻结后源码、测试、注释和helper停写，失败不追改、不进main。

## 对账与交付

逐稳定ID重核79唯一＝25已交付＋54开放，18类；19部分实施/29待实施/3待验收/2验收中/1持续。

- 完整能力交付：本批新增0、累计25。06.1仍欠流身份、协议会话/校时等退出条件，不能把包清单子能力签成整项完成。
- 已实施子能力：候选3组——共享Result精确演进与包渲染、原件/查询/预览同源元数据、定位窗口与合法小计的权限分层。未获main交付资格。
- 验收缺陷：新只读诊断导入边界失败、既有包络重叠限定丢失、PCAP自由解释及分组口径、未触发的自然覆盖分别记录；旧Trace角色/IO/采样/代表及权限欠项不重复新增或误销。

## 双轨实现与参考取舍

结构轨先公开复现§304的8项历史schema失败。只逆转唯一精确可选`PacketInventory|*tracewire.PacketInventoryResult|packet_inventory,omitempty`后，必须恢复旧完整字段类型、标签和顺序哈希；历史expected hashes不改。当前完整哈希`068194140d8edb5f26727356721b8640eeb1f2a1cd2a2743f1b3ccedb194cd20`，逆转后`aefd0aaa1adde3b00685730bd355eb8f435e1b7d78d3ea416707a6bf542e0bd1`。缺失、重复、错类型/标签及旧字段漂移仍失败。六类包DTO单独锁结构，真实PCAP字节→流式查询→渲染验证必需0/false、nil未知、纳秒epoch、顺序/预算/无重复和partial；未执行Trace计数不伪装成包零值。

能力轨针对§304完整PCAP已成功查询但附件卡把462B预览内容当3499182B原件的共享缺口。`TraceMaterial`保留准备阶段的内容分类和已绑定文件代次，CLI/REPL/最终MD/HTML共用原件、查询材料和预览封装的展示。预览字符串没有凭证只能说明预览大小，不能补原件零值。交叉审查又公开复现转换原件因派生Kind不同重复计数、真实空成员变unknown：只合并同代原件的请求卡/准备卡，保派生Kind/Carrier和查询信息；源/查询改代、异源、无凭证、bundle子项不借身份。真零须受信代次或成功有界只读，清单缺省或声称零不能自证。跨派生格式回归是公开凭证/渲染测试，不冒充真实转换器验收。

共享代表时间窗表不再一概禁止相加：定位包络不决定算术资格，小计服从已发布明确成员和量尺，不能外推其它成员/方向或当作单窗时长。真实业务Trace公共查询→发射→修补保两成员2ms合法小计、完整ledger、查询和权限；只修确定性系统说明，不扫描/改写模型正文，也不声称已经修好31ms直接等待误述。

参考`core/skill_executor.py:1404`把大小绑定实际文件、摘要单列，`query_engine.py`保总体统计先于明细降采样，`llm_contract.py`用明确成员/字段/值约束图。本批复用本仓已有代次与权限，不移植参考的源DB重写、全量list、文件名校时或自由文本关键词/重复硬门。WAL生产NO-GO保持。

## 冻结前验证

| 验证 | 结果 |
|---|---|
| schema/真实包渲染相关及race | 各1包93顶层/59子例PASS，零FAIL/SKIP |
| 小计/窗口/关系相关及race | 各3包41顶层/133子例PASS，零FAIL/SKIP |
| 原四根因、闭合睡眠、优先级、语义及自动补齐保护 | 5包35顶层/65子例PASS |
| 元数据相关完整包 | outputdump/attachment/traceinput PASS |
| 元数据末版聚焦race | outputdump/cmd/repl/orchestrator PASS；attachment/traceinput无匹配测试，不计已覆盖 |
| helper自测 | 原始非法UTF-8/NUL、歧义请求、实际links与错代负控、旧1435封存/18快照PASS；零模型 |

原始RED、施工期遗漏import编译错误、辅助fixture预期纠正等全部保留，不把测试构造错误冒充产品缺陷。标准CLI与orchestrator测试可执行文件各构建一次，后者只是编译，不增加模型例数。CLI SHA`91da74c03e2639792a0c3a47d72c83dc5c3a95ab7ea4586fae2f8680040873bf`。冻结7451仓库输入＋59外部输入＋26仓外快照，SHA`ae707a28c733d69f85cb146be177aa62cf507ebb29ad4a8452d14ac0005b5207`；源码快照用`.go.txt`，不污染Go包发现。凭证内容未读/hash/复制。

## 两例真实执行，各一次

| 侧 | 入口及范围 | UTC外层起止 | 机器判定 |
|---|---|---|---|
| A | 原缺来源终态只读登记；真实planner、脚本controller、独立恢复真实run_tests，非自然controller/完整CLI | 01:19:06.885866–01:19:27.032117 | PASS |
| B | 原普通PCAP自然问题，真实--htrace，15步 | 01:19:06.888508–01:22:47.277543 | PASS |

UTC日期均2026-10-29。实际请求可唯一配对A2/B15，另1个歧义事件保留不猜配，至少11.229秒真实请求重叠；不是进程重叠推断。无第三例。B实际3次trace_query均成功（两次attached、一次path），同源同limit20只有1份去重结果原件，不能写成1次执行。明细20/省略6081与上批limit10分别记账。

A本轮1结果/2命令、历史独立1/2，新invocation不同；来源verify attempt=0且VerifyRef/Completion缺失，本轮proof verified不能覆盖总体unverified/missing_terminal_verify_verdict。父与恢复子各4份真实原件、1条实际links，同代引用逐SHA/字节校验。有限负向PASS不代签自然controller/fullCLI或其它未触发能力。

B原件3499182B、查询材料同原件、预览封装542B在终端/MD/HTML已分开，§304具体大小错误未复现。完整原生6101包、协议4336/1627/138、帧长各3401542B、精确epoch、4倒序及20成员与独立Ruby字节遍历/tcpdump核对。最终正文却把443端口升为加密流量、帧长升为载荷/请求/控制类，并把同一协议维度内三个类别说成不可相加；后文正确边界不能抵消前文。完整人工结果及实际INIT、解释引用与重复展示详见[逐面审计](parallel_selected_summary_hmc_registration_pcap_metadata_20261028_manual_audit.md)。

## 全仓、封存和候选发布

唯一全仓UTC 2026-10-29 01:19:06.323197–01:30:47.619385，exit1：96包PASS/2FAIL/12无测试，28421顶层/33389子例PASS、3顶层/2子例FAIL、42顶层/14子例SKIP。252228行全JSON，无重复终态或未结束；原始日志SHA `991642b3b3504f4e3fbdbb3a7a424b5fde428160d749506f010909aaa0b6ae27`。完整冻结输入在运行前后及独立复验均不变。失败为两个接缝，不是5个独立故障：

- `TestTraceDiagImportBoundary`：render.go新增直接tracewire依赖，不在原闭合白名单内。
- `TestB1688PublicTraceQueryAnswerEnvelope`（zh/en）和`TestB1688BareIntervalDoesNotInventOccurrenceIdentity`：改小计说明时丢掉包络可能重叠、包络时长不可直接相加的原限定，英文还改变了固定措辞。真实公共Trace查询仍保原投影/ledger/模型块，但不能将缺失的边界教学一概称测试噪声。须同时保“定位包络不可直接相加”与“明确成员、同量尺的合法小计”，不是恢复全面否定算术。

旧8项schema检查本轮全部PASS，不掩盖新失败。冻结前按新/相关测试名筛选漏掉导入边界和B1688历史消费者，是本批测试影响面选择不足；下批将这两个已证接缝纳入冻结前完整相关集。未改原白名单/历史断言、未冻结后补写源码/测试/注释/helper、未二次全仓，绝不据机器2/2签main。

UTC 2026-10-29 01:37:19.884909原始材料封存完成，共257件，按canonical realpath＋SHA去重，同字节不同来源保留。索引SHA `7bfb62da5ea9442353e63417eb13eb8a3c589be6b4c0a37896ddc140d72ffd40`；两份独立报告、实际INIT、原始非法UTF-8日志、PCAP原件/oracle、验证报告/links、失败及19个变更源码/测试均留存。旧1435件/18快照与冻结输入复验不变。收据：[封存索引](results/hmc_registration_pcap_metadata_20261028_v1/sealed-index-v1.json)、[原始终态](results/hmc_registration_pcap_metadata_20261028_v1/sealed-receipt-v1.json)。

候选代码提交`84639db132d757f84d712e6c55ca0050a17b0efb`（19文件），本批五文档单独紧邻提交，普通推送该候选分支；最终远端同代、两提交源码与冻结字节一致、clean/main不动及54开放核验以[发布收据](results/hmc_registration_pcap_metadata_20261028_v1/publication-v1.json)为准。发布完整性不是验收PASS；本批完整能力0、main新增0、失败留账。

## 下一ROI

1. 先闭环两项全仓接缝：保原导入边界，从已有`tracequery.Result.PacketInventory`及已审查字段取得六个精确DTO类型，不扩白名单、不按类型名/包名猜测、不自动放行未来DTO；同时分开定位包络时长与明确成员小计，恢复重叠限定但保合法算术。冻结前跑整个受影响tracediag包、原B1688与新增小计正反回归，不再只按新测试名筛选。
2. 共享解释维度/成员/量尺的绑定与高质量原生事实复用；现有完整数字和限制已到场的误述仍记录，不以第三例追绿或关键词改文。图业务角色、Trace等待、旧采样/IO债保持独立。
3. 能力轨推进06.1有真实材料的流身份/方向剩余合同，与05.2跨源校准、06.2会话分开；轮换11.2代表或sample-only验收。只读登记本次仅补缺来源终态负向，不扩大签署范围。
4. 17.6新格式/宿主真实材料到位提级；17.7 WAL零副作用不满足不接生产。18类54个稳定开放ID不因局部子修复减少。
