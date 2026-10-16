# HMC255：双例完整人工审计

日期2026-10-16。两例各一次并行，正式exit0；完整答案人工 **0/2 PASS**。11.3有限最终交付 **A FAIL / B PASS**。物理helper、当前执行、完整正文/Primary/Principal、上下文与独立oracle分开核对，不把任何单轴成功当作整答成功。§254原FAIL保持。

## 原件与审计范围

本地根`eval/results/hmc_state_fact_handoff_20261016/`，A=`evidence-export`，B=`report-writeback`。准备是构造SQLite、零模型历史种子；两例使用同一自然问题和冻结binary，不冒称实机/自然中断。父代理通读最终MD及两份surface，另由两个独立代理逐工具/载荷/执行集合审计；没有追跑第三例。

| 项目 | A | B |
|---|---|---|
| 原始日志 | `logs/codrax-20261016-014339-000-60654.log` | `logs/codrax-20261016-014340-000-60889.log` |
| 默认MD | `workspace/.codrax/output/20261016-014848.527-60654.md` | `workspace/.codrax/output/20261016-014814.611-60889.md` |
| MD SHA | `194cb96892920e69cb8c19982800b8af691b7bae50c8720157103debdacf0160` | `f513a3b5e6a7d024143dd363edf379bb03bcf4de63df1542b036a40e8a566ed8` |
| 正式过程 | PID60654，UTC08:43:38.997111–08:48:48.592266，exit0 | PID60889，UTC08:43:40.114154–08:48:14.695966，exit0 |
| run.out SHA | `6ad45c2266dd5090bfe43b7f297cb7c8fea3902e1ad46901024d7ad02c6d1261` | `b7537d220436663c53d9cc1196cf698a691283ea7f354ad9dc30591f4951f774` |
| 精确终态receipt日志行 | 3623 | 4773 |
| manifest SHA | `69683fcc15577c18997a43efb6dc6cbad2a52f7fd879afc95a2cd9bbede369f3` | `b81926f01959692bb512641c0ca21f9e58c3f6767485e5058cb8377d5d3c30af` |

终态receipt由原logger逐字节提取并保存来源偏移/行SHA，不从manifest重造。当前index从实际成功resume/status返回取，不按mtime猜文件。`export-delivery-audit.json` A六轴verified/显式报告N/A，B七轴verified；状态仍是`physical_delivery_smoke_pass_human_pending`，不冒充人工结论。详细独审与正式命令收据复制入本批`verification-receipts/`。

原件SHA：主采集`a938230f8ec8bceaafd8038d16058057f11407e8ce1846ee43f4e307f6d42bf5`、nested采集`f912c23433825e0f25188ffef226c296d3515a2550d6a9be56cbc87880e4f03e`。两例源文件与原checkpoint均未改变；B保存的旧报告SHA `0464b367df35fe5e4e0d8577e5cbb136f630d7987b8eefbb30fb2b7e71468047`保原，显式final-report.md与本次默认MD逐字节相同。

## 独立事实基准

只读SQLite oracle与四份实际原生查询逐事件/帧核对。窗口`[10.000,10.050)`秒，右端点不计。原生载体没有声明size单位；不能擅补B/字节。事件时刻不是执行耗时。

| 原件/请求线程 | 本轮结果 | 事件/物理帧 | 原始事件 |
|---|---|---|---|
| `capture.data` /101 | success | 3/8 | 10.005 Alloc 512；10.025 Free 512；10.040 Alloc 256；gallery-main |
| `capture.data` /202 | success | 1/3 | 10.015 Alloc 128；background |
| `nested/capture.data` /101 | success | 1/3 | 10.005 Alloc 512；editor-main |
| `nested/capture.data` /202 | empty | 0/0 | 保请求身份与窗口，无观测主体，不证明线程不存在/不活动 |

全集5事件=4分配+1释放，14物理帧，0事件省略。Free的源帧是depth0 free、depth2未知符号，缺depth1；未知符号不等于缺层。另一个Alloc有两个不同depth0帧和一个depth1歧义帧，同深度没有先后调用关系。所有真实展开完整性均unknown；源帧载体完整不等于真实展开完整。同数字TID跨原件不构成同线程身份。

## A：所有查询、交接和最终表面

全部44个工具调用：list_files3、repo_map1、emit_analysis1、read_file6、trace_catalog5、trace_query12、emit_investigation_complete9、grep2、exec_command1、emit_evidence2、emit_answer_document1、patch1。exec的strings读取被拒，无客户写入。

12次trace_query请求：4次`if`坏容器在schema前置拒绝；4次ref-only原件真实查询；4次误把当前结果JSON当采集输入。后四次省略窗口，原生明确unavailable/requires_finite_positive_window，不能将matched_events=0当真实empty，也不具有原件/准备来源身份，不能为计数强绑到catalog；四次真实原件尝试均在当前登记，3success+1empty。自然恢复实际发生，不是用历史success充当本轮执行。

原名`capture-review`与最终4计划/4attempt一致，4份唯一当前载荷及report完整、逐字节匹配，退出后真实渲染链接可读；物理通过。但最终MD第57行及Primary/Principal明确宣称包中有“5条内存分配事件…1条内存释放事件”，实际材料为4+1。**A有限交付FAIL**是包内容陈述错，不是把所有自由正文错误无限并入11.3。

完整人工FAIL还包括：Free重复列入Alloc，101分配总数多算；原生未知单位被加B；同深度帧串箭头；真实展开unknown写完整；empty查询借另一来源已知线程名填入观测身份；手抄细节与系统正确原生表重复且冲突。时间/主要非空主体保留、无根因越权是正证，但不能抵消上述错误。

实际上下文有两阶段差异：先前完成接受原生交接；后续回探选择器猜测失败，泛化`managed_delivery_native_handoff`提示不定位具体ID/view错误，最终重复错误熔断（日志2392–2393）。日志2367旧soft-stop仍要求把结论和边界放reason，和schema分离合同相矛盾。最终器保有独立解释，同时fallback仍携pending旧阶段状态；终稿恰未复述pending不说明系统接缝已闭环。四查询/5事件/14帧原生事实和未知量尺完整送达，不是被预算截掉。

四个结果JSON的误查询还生成额外跨工件关系噪音和长内部标签；不是原件重试，也不应由默认关系表替其补因果权。没有Mermaid，不能签图修复/图布局通过。

## B：所有查询、交接和最终表面

全部48个工具调用：repo_map1、list_files4、emit_analysis5、read_file12、trace_catalog3、trace_query12、emit_evidence4、emit_investigation_complete5、emit_answer_document1、patch1。无源码修改工具。

12次trace_query中，4次if容器拒绝、4次ref与view并用拒绝；随后4次普通原件路径查询真正执行并自动准备/登记。这8次拒绝不计本轮原生attempt。两分支同名export合为同一意图，最终4计划/4attempt、3success+1empty，8个引用角色对应4份唯一当前载荷，与实物逐字节一致。原名包、返回后链接及显式报告回写成立，无最终pending/已发布矛盾，**B有限最终交付PASS**。中间解释过早声称已导出仍是教学问题；本例最终实际发布成功不为将来失败路径兜底。

完整答案仍FAIL：

1. MD第34行称5次分配/线程101四次，实际4次/101三次；错误已在接受的独立解释中，原生表未错。
2. 第21行称历史有两个101计划success，原checkpoint只有主文件101曾success，nested101为not_executed。当前四次执行真实且计数正确，这是历史叙述错误，不是本轮漏执行。
3. 第43行将两个depth0帧串为调用边；第53行将真实depth2未知符号移成depth1，并漏其物理帧；单一事件链被推广至其它事件。
4. 完整源帧/深度连续误升真实展开完整，所有malloc/free质量一概写完整；大小未有载体单位仍全部字节化。
5. “只分析采集文件”仍携current-source required；日志1632以`source_location`维度要求源码引用，最后用checkpoint.json:1的`{`作锚点，不能支撑量测事实。

时间、非空主体、不同原件未错配，empty限制保留，无泄漏/CPU/等待/响应根因断言。最终器已收到4查询及summary/members、0预算省略；Current model synthesis旧reason不再重传，证明结构化分离通道在本例生效。但独立解释仍复制错数值，终稿没用原生选择器，故“原始事实齐全”不等于“同源成文闭环”。

## 共用系统接缝与优先级

1. **精确可恢复的结构化交接（11.3/16.4）**：缺失字段、错误选择器、过时来源应各报当前准确失配点及可复制选择，所有软停止/重试教学服从同一已生效合同。不能只把字段改必需、却让模型在多个ID体系间猜测。
2. **精确引用失败与角色（16.4）**：B日志633合法pid101/202，636真正失败是第二引文`线程 202`不连续出现于原句`线程 101 和 202`；667/701误删合法pid，670/703仍同一错误，733才恢复合法共同引文。需要定位坏字段并保合法兄弟字段，不放松逐字引用、不从原文数字硬补目标。运行时调用位置/事实主体/仅定位游标/当前源码位置需要同源短教学。
3. **原生三surface复用与产物角色（16.4/16.5/18.4）**：量值/时间/身份/深度来自原生事实选择；独立解释保业务判断、边界与已验证关系，不做第二套数值输入。当前已发布结果应导航到结果阅读，原件查询导航仍须私有来源；合法JSON trace不得按后缀误挡。

补充事实lane自然未触发：A成功保两个正式目标，B最终空RuntimeTargets但有双lookup，均未发生“单目标排他+合格补充”。实际公开回归覆盖该分支及source/window/预算负控，不能冒充自然验收。sample-only二进制、只读登记、代表选择及图轮换也未由本批代签。

结算：完整能力新增0，三组有界实现；23/79已交付、56开放不变。三组当前验收欠项归原稳定编号，不新增重复任务。不将已证系统接缝统称模型波动，不按词删正文，不以正确附表抵消错误主文。正式测试、代码/文档提交及归档信息见[批摘要](parallel_selected_summary_hmc_state_fact_handoff_20261016.md)与统一审计§255。
