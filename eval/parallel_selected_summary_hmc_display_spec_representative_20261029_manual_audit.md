# HMC313 双例人工审计汇总

完整人工 **0/2**。两例真实并行各一次，CLI exit0不等于验收通过；不以正确表格抵消错误正文，不把恢复稿当最终受理。不改变原有限合同/问题/机器断言，没有第三例。

原始逐行审计：[A报告](results/hmc_display_spec_representative_20261029_v1/sealed-v1/0130-codrax-hmc313-A-audit-v1.md)、[B报告](results/hmc_display_spec_representative_20261029_v1/sealed-v1/0149-codrax-hmc313-B-audit-v1.md)、[执行完整性](results/hmc_display_spec_representative_20261029_v1/sealed-v1/0217-codrax-hmc313-integrity-audit-v1.md)。本页归纳，不替换原件。HTML为源/表格核查，没有浏览器像素QA。

## A：业务IO时序，完整FAIL

原件：[最终MD](results/hmc_display_spec_representative_20261029_v1/sealed-v1/0048-20261029-050519.269-83059.md)、[最终HTML](results/hmc_display_spec_representative_20261029_v1/sealed-v1/0047-20261029-050519.269-83059.html)、[根因旁路](results/hmc_display_spec_representative_20261029_v1/sealed-v1/0049-20261029-050519.269-83059.root-causes.json)、[表面旁路](results/hmc_display_spec_representative_20261029_v1/sealed-v1/0046-20261029-050519.269-83059.answer-surfaces.json)。

| 表面/合同 | 核对结果 | 判定 |
| --- | --- | --- |
| 原机器判据 | degraded_answer_checks_skipped:1，最终恢复而非正常受理 | FAIL |
| 本批新量尺说明 | MD191/226、HTML337/376为原始6/41.5ms合计及各自原始段包络，说明混状态可能性与口径边界 | 限定PASS |
| 原生因果边界 | 31ms闭合睡眠与35ms请求驻留分开；两就绪各1ms可合法合计2；47ms写请求只作背景 | 保留 |
| 原榜单与投影 | 原4正序位仍可追溯，恢复投影3物理候选未重复31；旁路却unavailable，不能当模型根因选择已受理 | 有界保留，非整答PASS |
| 业务窗/查询窗 | INIT已给50ms内运行5；摘要MD15仍将50ms业务与7ms宽查询运行混写，后文正确5不能抵消 | FAIL |
| 区间/事件 | MD285把1.041010–1.045500的4.49ms连到1.045唤醒；到唤醒仅3.99ms，另0.5ms在其后 | FAIL |
| 优先级解释 | worker120/main53已知边方向被反述；storage一侧unknown不能外推整链 | FAIL |
| 图交付 | 最终MD/HTML无Mermaid/SVG/canvas等图；未验证图已隔离，两个旁路均unavailable | 安全隔离有效，需求FAIL |
| 主表/单位 | 5张表与MD单元一致；64是sector count，raw另有32768bytes，本终稿未将64写字节；原件2312B与预览2408B分开 | 不新增该错误，不扩大单位验收 |
| 展示去重 | 本轮4查询、只显式选IO members，无多selector线程表对偶 | NOT_TRIGGERED |

### 图修补的真实链路

实际1次完整emit＋12次patch均未最终受理，但iter7曾staged，不能说全部没有写入重试基准。首稿把唯一当前worker `…70532…` 抄成 `…70572…`，均完整74字符，只错一位，源没有发布坏ID；这是起始模型错误，不是假设查询换代。

随后iter1/5已经传正确ID，但ensure_visible只增改Body，clone保留旧RuntimeParticipantsV1，完整身份验证继续拒绝旧值。iter2整个图替换又被local lease拒绝；iter7保存可见主体后仍未消除旧绑定。原始日志及源码指向一个共享修补交接候选，不能把循环全归模型继续抄错。尚未公开回放和枚举全部合法路线，故不宣称系统所有修补路线必死，也不放宽身份校验。

unknown `native_participant`字段来自一次模型patch，当前证据不证明当次schema允许它，不能用模型自己的推理认定教学矛盾。原始提交、拒绝、fresh refs与staged基准全保留，下一批用该原件公开复现。最终context压力是后果/停止条件，不作为首错原因。

## B：代表选择，结构及完整人工FAIL

原件：[最终MD](results/hmc_display_spec_representative_20261029_v1/sealed-v1/0391-20261029-050431.113-83015.md)、[最终HTML](results/hmc_display_spec_representative_20261029_v1/sealed-v1/0390-20261029-050431.113-83015.html)。同次status结果/params/index/snapshot的精确路径、字节和哈希见[B独审](results/hmc_display_spec_representative_20261029_v1/sealed-v1/0149-codrax-hmc313-B-audit-v1.md)。

| 表面/合同 | 核对结果 | 判定 |
| --- | --- | --- |
| 自然选择入口 | 8计划/2组/4选中/4未选，预算/线程/数字窗到场，但view是event_search | 已触发，原资源合同FAIL |
| 同代交付 | 最后普通status完整返回准确绑定index/snapshot/hash；原Go auditor实际exit1 | 交付局部有效，不代签规格 |
| 原生资源事实 | A1013事件8帧；A202/B101/D101各1/3；B202/D202两0/0；数值及未知字段正确 | 保留 |
| 表格复用 | 15原生表＋1边界表；16表73行519单元格与MD一致，来源/身份/窗口可追溯 | 有界PASS |
| 查询分母 | 17记录＝8声明＋9观察；25尝试＝23success＋2empty；13唯一payload不是调用数 | 原计数正确 |
| 完成说明 | 4选中event_search计划各3次均success；不能说第三次才有结果。未选原计划未执行不能覆盖另resource规格已查事实 | FAIL |
| 窗口规格 | event_search含10.050事件，resource_stack用[10,10.05)；数字边界相同不等于成员全集相同 | 合同未闭环 |
| 来源身份 | MD139把C的background202 rename借给D，用其解释D202空结果；D实际5行没有该身份行 | FAIL |
| 关系权限 | MD52以库名/数量差异断言非同负载，写成配对分配—释放—再分配；无原生生命周期凭证 | FAIL |
| 栈帧顺序 | 最终用列表/原深度，未照搬调查中的调用箭头；缺层/同层与未知保留 | 局部改善，旧债不倒签 |
| 图 | 原题不要求图，本例无图 | 不新增退出门 |

### 代表计划、实查与未选名册必须分开

| 原选组 | 选中event_search计划 | 未选event_search计划 | 另行资源查询 |
| --- | --- | --- | --- |
| TID101 | A101、B101，各三次success | C101、D101 | A101/B101/D101成功 |
| TID202 | B202、D202，各三次success | A202、C202 | B202/D202真实空；A202成功 |

不能拿另查过的D101/A202资源结果替代所选资源代表矩阵，也不能因此说这些来源/线程从未检查。普通status而非特定动作名就是合法同代审计入口；原结构失败不是因为动作名，也不是因新增完整栈/图要求。

### 实际上下文是否足够

六次调查提交两次受理、四次拒绝，原生数据本身已足。实际INIT3只完整展示五解释单元中的0/1：其中包含“成对”及不同view同规格措辞；虽标advisory、support refs未验，部分仍进入最终正文。最终保留了“成对”，但去掉调用链箭头。未展示的单元4中旧C202空/全覆盖错误没有成为本次终稿，不把内部任意坏草稿都归因成最终消费。

INIT2已有17记录/25尝试及event_search规格，资源原生表也已到场。当前失败不是缺原生值，而是解释/选择/执行集合的范围未一致绑定。来源、主体、窗口、量尺及关系适用域应在结构化交接中复用，不能靠扫描用户或模型原文设硬门。

## 统一留账与审计边界

完整能力新增0；候选有界子能力2组，53开放不变。当前3产品原因组（图修补绑定、代表规格/执行、同源正文解释）和1评测精度组（请求配对残留pending）归旧ID；不把每句误述新建稳定任务，也不无限追跑同题。

新元数据隔离的对偶由公开工具测试及唯一冻结全仓验收，不由本live缺少重复表代签；A新量尺说明是自然恢复出口限定PASS，B未触发该分支。只读登记终态、二进制新家族、旧采样/IO/权限覆盖本批未验，不误销。WAL保持生产NO-GO，main不动。

两个CLI真实并行成立；独审修正overlap工具过度配对的统计后，仅承认可证A20/B14和21.233秒交集，原文件20/32及全部原始日志不变。该证据精度欠项不抹掉实际重叠，也不借重叠证明答案通过。完整冻结、全仓、封存与候选分笔推送见[批汇总](parallel_selected_summary_hmc_display_spec_representative_20261029.md)。
