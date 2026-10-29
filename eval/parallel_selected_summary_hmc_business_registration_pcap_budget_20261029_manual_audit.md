# HMC310 人工审计：读取兼容、图覆盖与PCAP整组展示

## 裁定与覆盖

本批恰好两例并行各一次：**A机器PASS／完整人工FAIL；B机器PASS／限定负向人工PASS**。另一次零模型真实PCAP静态公共CLI人工PASS，不是第三次模型评测。唯一普通全仓98包PASS、28514顶层／33670子例PASS，零FAIL；扩大race的分配断言FAIL及包级超时独立保留。源码、测试、helper冻结后没有修改或重跑。

HMC-06.1原有限合同可独立完整候选交付1，普通推送后计26已交付／53开放；main新增0、整批不合入。A失败不阻止无关PCAP原合同闭合，PCAP通过也不抵消旧自然正文失败。细节及收据见[批汇总](parallel_selected_summary_hmc_business_registration_pcap_budget_20261029.md)。

| 面向 | 实际结果 | 不扩签边界 |
| --- | --- | --- |
| 读取成功与可见权限 | 原旧预览回归、真实ReadFile→extractor/观察账/调查完成正负回归通过；旧实现overlay真实错误闭合已公开 | A未读文件、B只读正常小文件；自然超长行分支未触发 |
| 图角色与原生边 | 最终新说明区分业务映射未知与两条原生唤醒已证，局部PASS | atomic修补拒绝教学允许的多行身份，完整图FAIL |
| PCAP静态交付 | 64记录／64流成员全部字段、分类及未归组摘要、原预算和实际显示计数PASS | 真实材料SLL2/IPv4；IPv6/坏尾/容量来自公开合成测试，不冒新实采或协议会话 |
| 只读登记终态 | 新proof verified，旧source缺终态，总体unverified；同代原件/链接和只读保真PASS | scripted controller＋真实planner＋独立公共执行，不是完整CLI／自然controller |

## A：原生事实正确不等于最终解释正确

正式目录：eval/results/hmc_business_registration_pcap_budget_20261029_v1/A/trace_business_io_sequence-20261029-012645。原QUESTION、EXPECT及15步不改，275秒，实际13次trace_query／9份不同原生JSON／3个finalizer INIT。最终产物为 .codrax/output/20261029-013120.413-76896.md、html、root-causes.json、answer-surfaces.json，全部列入封存，不只看verdict。

本轮业务窗[1.000,1.050)=50ms，主查询[1.000,1.051]参数=51ms，附件[0.999,1.051]=52ms；不能照抄旧批“52ms查询窗”。主查询6running＋1runnable＋44sleep=51，业务交集5＋1＋44=50。31ms闭合提交线程睡眠与35ms issue→complete请求驻留保留、不可相加；两个不同时间/线程的1ms合法小计2ms保留；backup900的47ms仍仅背景。原4rank完整到场，3投影候选是另一集合，不是丢根因。storage80/TGID2→worker200/TGID100@1.040010、worker200→main100/TGID100@1.045真实方向正确；storage无调度区间不再说缺唤醒，也未推优先级反转已证。

### 已证共享缺口：修补入口与教学不一致

实际INIT03:341允许业务角色首行＋完整线程次行；首稿拒绝也重复该教学。模型随后提交真实patch：tool-call_5ab11ead14234137ad2730fc-emit_answer_document_patch-params-08a9ed46.json，以同node的“主线程&lt;br/&gt;app-main-100”等三标签调用ensure_visible。结果6ca4ace9拒绝精确身份，未staged；后续仅业务角色标签才被c461e9e6接纳。

静态链路是answer_document_diagram_edge_patch.go:1087→diagramParticipantEndpointExplicitlyDisplaysIdentity，只取完整标签或代码身份候选；full Emit检查则使用DiagramPrimaryVisibleIdentity首行。公开新测试覆盖full Emit而未覆盖原子patch。这是实际模型遵从教学仍被拒的系统接缝，不能归模型波动或降低精确身份门。下一批先以原件走公开atomic入口RED，再统一精确可见身份投影。

最终MD有两份3lane／2正确箭头的Mermaid：文本块保线程名，结构图仅业务角色；不是单图6lane，不是语法失败。两图未复用唤醒后的runnable与上CPU点1.041010/1.046。新边界说明“业务身份未核实，但两线程关系已验证”确已自然生效；仅该窄片PASS，不签完整图。

### 仍失败的成文内容

- 原事件32768字节／64扇区，INIT03:457/472/562保sectors×512和32768；MD60却写64字节并推设备服务时间。请求驻留与闭合睡眠不自动证明硬件服务时长，正确表/侧车不能抵消正文越权。
- 原4排名与去重3候选均在上下文，正文却称排名仅三条；不得据此删旧rank或称系统遗漏。
- 0.999→1.001完整运行2ms与业务交集1ms混写；worker打点后4.510ms与完整首段5.010ms混写。保范围事实，后续不扫描正文关键词设门。
- 调查unit[0]的44/50=86.27%错误到场，实际只有unit[0]/[3]两个整单元进入解释预览；不能声称所有历史错误都被重传。最终未照抄该百分比。native_refs只验证地址，不验证算术或因果。

4次finalizer拒绝与179047上下文估算保留，其中一次是facet_ids对象/数组JSON形状错误；不能把全部拒绝计作图问题或臆测预算定因。HTML只审源码和表图语义：4表与MD一致、两图均在，不宣称像素QA。read_file=0、name参数未用；超长行和PID/name同实例去重自然未触发。

## B：缺来源终态的只读登记

正式根 /private/tmp/codrax-hmc310-registration-v1；原TestNativeRegistrationLiveRestoredMissingTerminalReceipt，Go合同7.33s、独立子进程0.43s。两次真实planner请求，实际read_file两次和emit_change_plan一次；changes为空，原assertion绑定成功。辅助submitted-tool-parameters.json为空数组是collector不识别该日志形态，原日志171/172/200保完整参数及落盘计划，不得记零调用。

父76851／恢复子76966、历史native:12c33…与当前native:12ca6…不同。当前1个assertion结果、2命令（语法预检与unittest）、1个执行凭证；历史1/2独立，审计合计2/4不是本轮两测试。source前后完全相同：1apply、0verify、无VerifyRef/Completion。proof当前verified，总体仍unverified/missing_terminal_verify_verdict，缺来源终态未被新证明覆盖。

真终态报告为history-report-artifacts/originals/plan-1793262415153474000-76851.final.json；根final-report.json只是ChangeReport兼容名。父/子索引各4原件／1attempt／1link，逐字节SHA和实际verification.report_id、artifacts.report_path均指向本轮。HEAD、源码/测试SHA和工作树保持只读。materialization unavailable、自然controller、完整CLI、源码终态正向及context-pack durability均不扩签。

## PCAP：原06.1有限合同收口

唯一真实CLI执行出货collect_packet_capture.yaml两视图，无模型。原件3499182B，6101记录／503端点组；5963归组＋138未归组，捕获帧字节3401542，4次时间逆序，原生预览各64。173物理行报告实际展示79/79和73/73行，**64记录和64流组全字段完整**；7项分类和1项未归组原因先于大列表，138条／45268B及原生时间保真，450行预算未上调。

独立旧封存字节oracle与本批副本绑定后核1111路径／6167检查、零差异。每组身份、总计、双方向、同端点及各自时间齐全；未知不补0，接口合法0不丢。A/B规范端点不是本机/远端，443不升级为加密，帧字节不改载荷，epoch不映射Trace时钟。下标0–63的全部记录/流都逐字段核，不只检查尾成员。技术报告一行更长且字节更多，不能宣传token同步变小。

既有公开正反涵盖IPv6/分片、重复物理记录、坏尾partial/真零、源陈旧、取消、1GiB稀疏输入和组限；本实采只SLL2/IPv4。05.2完整校准、06.2会话、PCAPNG和自然选脚本不是本项新增退出条件。四包race3PASS/1FAIL、分配delta8>5及10分钟超时仍留18.4，不叫环境波动；普通全仓同断言PASS也不倒签race。若后续证实直接影响06.1则重开。

## 留账、原件与发布边界

本批剩余4共享原因组：图原子修补/双表面交接、成文单位/量尺/集合、B采集参数索引、race分配/超时；另超长行自然未触发。均归原16.4/16.5/18.2/18.4，不增稳定ID。旧采样、IO、栈关系、代表选择及权限未触发仍原账；WAL零副作用未满足，生产NO-GO。

三个独立逐项审计原件 /private/tmp/codrax-hmc310-A-audit-v1.md、-B-audit-v1.md、-pcap-manual-v1.md 全文随封存索引保存；本文件只作综合记录，不改其原始裁定。真实请求重叠4.495秒，未唯一配对者排除。失败日志、临时fixture错误、拒绝patch、旧实现overlay、原始产物和真实INIT均保留。候选相邻分笔发布及封存数字见批汇总，A未过所以不进main。
