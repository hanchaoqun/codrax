# HMC324：严格兼容修复、只读登记与真实二进制轮换

2026-10-30（UTC），仅在main工作。基线 `395ca294542ef47bde85746b5e06df1263fff7a2`，实现 `01ee615a9ab975923e0403ea37ceea8a5d1bd5f8`。79个唯一稳定任务＝27已交付＋52开放；18部分实施／29待实施／3待验收／1验收中／1持续。完整能力新增0；本批交付两组有界共享改动，并修复上一代4个全仓兼容失败，不关闭任何稳定ID。

## 实现与参考

承接§323的业务原子账户公平预算和残余D三值语义：精确来源/查询/主体/时间域/窗分组，主lane保16条，补全仅排除实际发布记录；完整记录去重、16+4、128行、严格JSON及完整双库存门不变。D残余桶用“未标记为IO等待”而非已证非IO，保所有原始状态、数值和链上因果权限。已有墙钟诊断仍为离线用途，未重接正文appendix。

§323唯一全仓实际失败4项，本批只在3个已有测试文件修复兼容遗漏：严格类型检查引入真实tracefence模块；生成卡片区分完整限定D标签与裸D项；两个原生量测快照更新精确共享标签。新增6个限定/裸项/混合/错误限定正反子例，原数值4.039、1.340、5.379及身份、来源断言不放宽。共32个源/测试文件相对基线变化，其中11生产、21测试（5新增）；相对封存§323只有3个测试文件变化。

实际对照参考仓marker实例/self/状态求交设计及hiperf二进制解析路径。借鉴字段独立与索引/解释分离，不照搬缺字段补零、首attr假设或把周期权重解释为执行毫秒。读取参考extensions说明及解析器实际代码；头注释指向的设计文档在本地缺失，不声称读过。参考仓未修改，详细发现保在本批原件中。

## 冻结与验证

四项失败复验：3包、4顶层、6子例通过；相邻及因果保护：6包、96顶层、356子例通过。保原4根因、闭合睡眠、量尺、自动补齐等断言。helper自测和预检通过，CLI/testbin各仅编译一次；标准CLI SHA `3d6f214e1515c8a2b1b0369611c554eb383d3a81507b08bf6ede40d1cb4d1a58`，testbin SHA `f18ba2ada819c8bba471da07dbde478a19b04648c1dd3ab47401d4f8d48657b0`。

冻结7536仓库输入、172外部依赖、44仓外快照，SHA `f5f0b13587e3ecfadbeea2a01c559937fb1a40b14a76da59a39aca0c4a65cc06`。之后源/测试/注释/helper/模型输入停写；文档与独立审计另行汇总。唯一全仓pid71665，12:02:23.022579Z→12:13:31.917822Z，实际exit0，输入前后不变：98包PASS/12无测试，28609顶层PASS/43SKIP，34041子例PASS/14SKIP，零FAIL/非JSON/重复或未终态。日志SHA `97d66d30d1094d2ccf3885b6a3d70bd4e8b4023414046fc1455c4f213915f913`。

## 两例各一次，真实并行

| 例 | 原合同及材料 | 进程／原机器判定 | 人工验收范围 |
| --- | --- | --- | --- |
| A | 原缺来源终态负控；真实planner最多12轮，scripted controller，独立恢复公开RunTests | exit0／PASS | 限定PASS；当前proof与来源缺终态分开，不冒自然controller/full CLI |
| B | 原sample-only问题；完整30,062,585 B PERFILE2，15steps/n1 | exit0／PASS | 完整正文FAIL：未知样本类型仍被肯定解释为on-CPU执行 |

仅A显式开启missing-terminal opt-in，full清空live开关。A并非自然controller/full CLI；B只附原二进制，不注入旧catalog/oracle/答案。独立历史七文件标尺明确复用，不冒本轮重测。实际请求A3/B13对、0歧义，最大可证模型请求重叠3.490秒，不以barrier或进程重叠代证。

A真实读取2行实现与4行测试；错误字符串数组首次被拒，正确结构后受理。当前proof verified不产生来源verify/Completion，整体仍unverified/missing_terminal_verify_verdict。B经真实准备与10份原生结果交接：目标164条/权重259、两个热点82/174及82/85、PID1514/TID1609、CPU0/1记录范围与完整包络正确；权重不当时间。原表写“执行状态未确立”，正文却肯定在CPU上运行。正确表格不能抵销正文错误，不追跑第三例或加全文关键词门。

B三份实际INIT均保关键未知类型、权重边界和正确包络；展示的1/5独立解释单元也明确不得断言on-/off-CPU。受理调查中的错误扫描终点.469586和宽窗链路未重传到成文，未发现关键事实预算丢失；110571tokens或发生工具历史裁剪本身不判FAIL。本轮属于证据充分后的解释误述，区别于§323已证账户缺失。trace_supplement因families_present跳过，不冒实际新增隐藏查询。

## 未销账与下一ROI

§323 IO/Donghu两个人工FAIL和四项原全仓FAIL保持原代封存、不独立发布、不倒签。新的实际证据证明：四个完整业务self虽被原生查询和两次受理调查保留，成文INIT仍只剩窄窗组；这不是“证据充分后的模型波动”。先公开复现来源生命周期与非根业务实例账户选择，再修等待完整聚合与预览小计交接。整工具结果历史裁剪是代码路径假说，尚无该次实际丢弃收据，不先断言唯一根因。

重点9组继续留账：业务账户交接；等待总体/预览；D/IO未知分类完整验收；同源重复和预算；采样执行状态越权；IO物理/驻留/集合解释；业务等待/caller/比例解释；JSON修补终态；代表选择自然规格。旧权限、图、Windowed和扩大race未触发/欠项不由本次限定通过关闭。105版元宝真实ftrace已用于上一代实测，100版抖音例仍NOT_RUN；WAL零副作用前生产NO-GO。

全部52项按ROI九组无重漏排序：共享交接7、已有原件/验收链8、业务及代表5、跨域可比5、SQLite安全1、IO资源/图形9、硬件PMU/平台6、新业务媒体网络8、建议写链3；持续任务18.5与高ROI实现并行。精确ID/状态见[完整核账](results/hmc_registration_sample_compat_20261030_v1/sealed-v1/0234-codrax-hmc324-census-plan-v1.md)，分组不是新增任务。

原始结果根：`eval/results/hmc_registration_sample_compat_20261030_v1`。[封存索引](results/hmc_registration_sample_compat_20261030_v1/sealed-index-v1.json)397件，SHA `2d94635aafdad3d0ee35ea412a8b13dad124cca4c3a7d366a644b3b19b5a3412`；三正式进程exit0，封存本身不推人工PASS。旧25档8919件、562快照身份已核验（18旧污染为子集，另544仓外身份）。大原件及同字节全文副本按身份引用，完整query/INIT/答案不按大小截掉；仓外Go快照不回流可编译目录。凭据只由正常运行时加载，审计/归档不读取、哈希或复制凭据内容。HTML仅源审，不冒像素QA。

32源/测试实现与精确7文档分两笔相邻main提交，普通push；实际文档提交、远端HEAD、冻结源及原件一致性由后置[publication收据](results/hmc_registration_sample_compat_20261030_v1/publication-v1.json)核验，closure=none。原始397件本地留存，不冒Git上传。[独立代码复核](results/hmc_registration_sample_compat_20261030_v1/sealed-v1/0263-codrax-hmc324-final-code-review-v1.md)、[根复核](results/hmc_registration_sample_compat_20261030_v1/sealed-v1/0345-codrax-hmc324-root-review-v1.md)和[上游交接发现](results/hmc_registration_sample_compat_20261030_v1/sealed-v1/0270-codrax-hmc324-handoff-followup-v1.md)分别限定已修和未修范围。

参见[人工审计](parallel_selected_summary_hmc_registration_sample_compat_20261030_manual_audit.md)及[上一代失败](parallel_selected_summary_hmc_io_donghu_handoff_20261030.md)。
