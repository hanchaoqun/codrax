# HMC319：业务实例身份交接与只读登记负控

日期：2026-10-30 UTC；本地运行跨2026-10-29/30。基线100e86c98ab6484f8ab3c205a5f527163f8293a9，继续main。

## 核账与交付

79个唯一稳定任务＝26已交付＋53开放，18类；18部分／29待实施／3待验收／2验收中／1持续。逐类开放数仍2/1/1/5/2/2/4/6/3/2/1/5/3/5/3/2/2/4。本批不改变任何稳定任务勾选。

- 完整能力新增：0，累计26；04.2业务树完整成文仍FAIL。
- 已交付有界子能力：2组，当前原生业务实例到请求图主体的精确桥，以及包含子层总量/自身互斥分段的统一教学适用域。前者自然触发并受理，后者已实际到场但不等于模型语义全通过。
- 本批剩余验收/修复缺陷：4组，A三组（发布次数升为实例数、背景self字段未传全、跨层状态小计表述冲突），B一组（错误计划形状先报身份错误）。不是新增4个稳定任务。

机器2/2；完整人工A FAIL、B原有限负控PASS，不签整批无缺陷。Async指代仅记歧义，不升级为确定错归。详见[人工审计](parallel_selected_summary_hmc_business_registration_binding_20261029_manual_audit.md)。

## 参考、实现和保护

主代理实际阅读参考仓marker_ops.py:290–680：节点id/parent_id及callid=itid确定同线程树，cookie异步与同步分开，self扣直接子层，状态按同一itid求交，postorder cache绑定当前node_map对象。采用实例/来源/当前代次优先，不照搬简单子时长相加或缺状态返回零；Codrax仍用直接子区间并集和未知覆盖。参考AGENTS、GOLDEN_RULES、architecture及marker源码共6件只读冻结，未运行参考服务器。

真实trace_query在原来源完成后一次性密封当前predicate全量行摘要，挂在既有私有replay引用，不进JSON。沿原文件/查询同代校验运输；编译器只消费，不从被裁剪或修改的公开结果重新签发。4096行是新桥资格上限，超限保旧事实/已证边但不授新身份桥，不冒称总内存硬上界。

业务provider先核完整未裁库存，再按已验证的显式线程目标及请求业务字面身份选择唯一物理实例；孤立同名根也参与歧义。跨查询可识别同一物理实例，但各查询关系/量测凭证仍独立。无目标、多目标、同名多实例、旧代、JSON、修改后重签、部分库存和裁剪不借权限。完整发射与局部修补共用原校验路径，合法部分contain关系不删除，不授调用/唤醒/根因权限。

量测表、业务上下文、图关系教学同步区分：重叠inclusive不能直接加；同源同线程同窗同单位、已证互斥self及同状态交集可作有限小计。未知/省略不补零，不把采样权重换毫秒。未加问答原文关键词门、旧断言未放宽；Trace投影、自动补采、原4候选、31闭合睡眠/35请求驻留、合法2ms及47背景保持原保护。

## 公开验证与冻结

| 验证 | 实际结果 |
| --- | --- |
| related-v1 | 4包，397顶层/418子例PASS，零FAIL/SKIP |
| causal-protections-v1 | 3包，64顶层/293子例PASS，零FAIL/SKIP |
| race-v1 | 4包，47顶层/77子例PASS，零FAIL/SKIP |
| 唯一全仓 | exit0，98包PASS/12无测试；28582顶层/33932子例PASS；原43/14 SKIP；零FAIL、非JSON、重复或未终态 |

首轮graph夹具没声明图需求，PASS但NOT_TRIGGERED，未冒作RED；补真实图前提后公开FAIL。草稿编译、新测试查询等价前提错误、teaching初次async前提错误均原件保留，只改新测试；不同查询发布测试最终用不同窗口证明查询身份独立，未改旧断言。graph-public-red-v2期间仅另一轨新增测试文件并发写入，收据source_unchanged=false如实保留，不冒称该轮冻结。

CLI与testbin各正式编译一次，分别SHA692152ec88e0277028b57c112f5a32443dde0eb1633ba66a07de3e3dc99cd3cb、623cfdc167c2bbde3120f4bfaa6b4d90cadd1ebb1ff5b237da396baf57de1b3e。冻结7522仓库输入/271外部输入/22仓外快照，SHAc80db935883d3ce9348bf3aedfa4ca7ab1058352a5d9a7992db2867eafaacadd，之后源码、测试、注释、helper和模型输入停写。

唯一全仓go test -json ./... -count=1 -timeout=30m，pid3476，06:59:17.632466Z至07:10:33.151509Z；原日志SHAf8642dd13729f84467dd0d8c28c861dad66d305636d8a36b698707c4f6938130。live与全仓前后冻结输入一致，原opt-in跳过不冒通过。

## 恰好两例并行真实评测

| 侧 | 原合同与执行范围 | 机器/进程 | 人工 |
| --- | --- | --- | --- |
| A | 原trace_query_business_tree题/EXPECT、1481B authored导出文本、15步/n1；标准CLI一次 | PASS，shell3479 exit0，07:02:38.835654Z | 图桥有限PASS；完整成文FAIL，04.2不销 |
| B | 原TestNativeRegistrationLiveRestoredMissingTerminalReceipt，n1；真实planner＋scripted controller＋独立恢复公开RunTests | 原测试PASS，pid3480 exit0，06:59:38.855613Z | 原只读登记负向合同PASS；非自然controller/full CLI |

A原专家显式窗不代默认自然发现；原operator oracle保留，另独立校准其“跨行不可加”歧义，不强制输出新小计、不注入模型。B测试的恢复子进程不是第三个案例。无追跑、无新增二进制实采主张。

实际模型请求最大可证重叠2.853秒，非barrier推定。A日志13请求/12响应，4唯一闭合对＋9请求/8响应歧义组；B4请求/4响应均可唯一配对。原非法UTF8及未闭合日志证据保留，不猜配，也不据此否定真实进程终态。

A九次模型trace_query＋一次系统window_stats补采，10独立payload；complete两次一拒一受理，emit一次受理、patch零次。三份实际INIT；15参数blob与15内联声明是相同调用的两载体，不能相加。B五工具声明＝2 read_file＋3 emit，2拒1受理；参数blob零但内联参数齐，不等于零调用。

## 留账与下一ROI

1. 优先公开复现共享发布行数与物理实例数分离、字段级可用性到成文的交接。保各查询来源/总体，不能用删查询或按名合并解决“9行称9实例”；背景self墙钟已知与状态未知必须分开。归16.4/16.5/18.2。
2. 受信IO端点到共享标题仍为已证系统缺口；同时保合法小计/关系权限，不反复追跑已充分给证的自由误述。B错误JSON形状先报身份错误的修复提示债归16.4/18.5。
3. 完整能力槽轮换11.2代表规格自然入口，先核观察意图→真实view/线程/窗/每组预算，按原代表/未选/执行状态合同验收；只读登记继续负控轮换。17.6仅有新增真实格式/工具/宿主材料才扩验，旧ZIP/gzip包装不算新覆盖。

旧IO、采样、权限、代表选择、§310扩大race和Windowed自然未触发均不销。活跃WAL未满足零旁车副作用前生产NO-GO。

## 证据与发布

本批[留存根](results/hmc_business_registration_binding_20261029_v1/)封存443件，[索引](results/hmc_business_registration_binding_20261029_v1/sealed-index-v1.json)SHA115e14a90e811f8a4d8a7f3a751d7b058221944bde04069f5c7cd1caea903ab2；[收据](results/hmc_business_registration_binding_20261029_v1/sealed-receipt-v1.json)保实际进程、全仓统计与原始路径。旧20档6285件/428来源快照身份逐字节保真，18历史污染为其子集。原失败、完整输入、INIT/参数/payload、最终MD/HTML与独立双审计均保留；HTML源审不冒像素QA。凭据不读/哈希/复制，仓外Go快照不回流成仓内源码。raw results是本地留存，不宣称随Git上传。

实现dbc3ea6d74ef5b7521f7f225f9cbd10d331d76fa与随后5文档紧邻分笔提交、普通推送main；最终精确文档SHA、远端相等、工作区清洁及源/封存字节检查记入[publication](results/hmc_business_registration_binding_20261029_v1/publication-v1.json)。closure=none；发布完整性不倒签完整人工FAIL。

代码提交后的独立verify命令因仍要求冻结基线HEAD而拒绝，发生于字节检查之前；[中间检查收据](results/hmc_business_registration_binding_20261029_v1/intermediate-head-check-v1.json)保原诊断，不称源已变或测试失败。最终publication显式传两个相邻SHA重核提交字节。此为封存后的行政收据，不混入443件不可变索引。
