# HMC327 两例人工审计汇总（均FAIL）

原问题、材料、预算不改，每例各一次。A/B实际进程exit0；唯一full真实PASS且源未变。完整能力新增0，79/27/52及closure=none。A原smokePASS而完整人工FAIL；B人工FAIL、执行后同代终态NOT_TRIGGERED，operator未运行。552原件已封存，34文件实现776c8683940a777ec99cfac94bc51ff2e5f42afa的交付范围为共享子能力3组，不是完整父任务；代码/9文档及远端由本批publication确认。全仓、子能力、原问题完整验收分别计数。

## A：真实105元宝——完整人工FAIL，实收表格保真

原15steps/n1，完整63659046B ftrace，SHA256 `be59c0648ac7e1d8a2d117f5dc36efa0c92b5f3686671719ada435c16713a20c`；本代收集11query/3INIT/1HTML，非§326的26query/3493展示行。独立oracle不向模型注入；原件、窗口与线程身份不改。

[A独立原报告](results/hmc_native_publication_compat_20261030_v1/sealed-v1/0265-codrax-hmc327-A-audit-v1.md)23548B、SHA `13a6a0ece9e0c9d6752de8ceca68071ddd69cdb1f85b45179d01893f37249418`，已完整复读。原合同要求两次activity及主要业务子阶段的总量/self与线程状态；没有追加画图、唤醒链或根因要求。以下三层分别记账，不把缺证与充分证据后的误述混为一类。

### A-F1：两个主要child self在producer端未产出

| 实际window_stats payload前缀 | 范围 | 已发现/已发/省略节点 |
|---|---|---:|
| 45994f92 | 用户全窗 | 3718/32/3686 |
| e461b2f1 | 第一次activity窗 | 460/32/428 |
| 6618b5f1 | 第二次activity窗 | 1548/32/1516 |

三树均partial_topology。`trace_marker_tree_collect.go`完整统计后按起始行排序取NodeLimit32前缀，`trace_query_business_tree.go`只消费已发Nodes；目标login/home self21.605/83.143在所有实际原生结果中均无产物。完整activity总量/self59.695/6.770和111.487/3.938正确，不能据此代签主要子阶段。下游16账户公平预算无法恢复未产出的账户，oracle值不得回填给模型，不增32/16/128常数追绿，也不能声称本次整个完整树已经进入留存池后被丢。

### A-F2：已经真实查询的登录页total没有到成文

原span_window payload `453bca28339fa703b9d33391521d3f7e73b07f8d8412a48779013ff892d8b3ca` 给完整HYLoginMainActivity名、行155107–158588和46.600ms；home原查询1ae1f000…给103.315ms，activity查询b91f2d0a…给两实例。实际10次模型trace_query另加1次确定性补采，共11payload，不混算11次模型调用。

受理调查原参b4a390…/d64dc320含正确login unit[1]及aggregate；3实际INIT均无完整名和46.6/46.600账户，INIT02只有截短事实/引用槽。INIT03显示16业务记录/省略22、3查询组全覆盖，不表示底层3718/460/1548节点全可用；调查只投递unit[0]/5，省略4531B。这个已证跨通道缺口应恢复真实原生账户导航，不把模型aggregate/解释单元升native。无Mutable/pool快照，不能把整ToolResult历史裁剪当已证唯一原因；161767/200000估算和8次history prune本身不构成该证明。

### A-F3：原生证据充分后的成文失败

- 全窗D未标记3.610与IO标记1.584互斥，正文却写前者“含”后者，导致分区只和为302.467而非304.051ms。0.006ms未给IO标记不能升为已证非IO。
- 两activity自身段总数实际16展示+15省略=31、16+2=18，正文写16与17；第二实例direct-child数17不是self段数。
- hintActivityLaunch与0.001ms ResourcesManager节点始于3733.684623/3733.684573，早于home起点3733.692357，实际父为activity2；正文误绑到home之下。INIT已给精确父实例和时点。
- ResourcesImpl#updateConfiguration total0.044/self0.038，模型表在其它行用inclusive状态的同列填self Running0.038，原inclusive Running为0.044；应保分母，不是拒绝合法self计算。
- 将第一activity概括为许多短未展开child并以无时长账户排除login，遗漏已查46.600主要子阶段。答案承认partial且不编造缺失self是正项，但不满足原题主要阶段对照。

父账户在本轮对123.918/self2.375已明确窗口裁切限定，不能当§326未限定窄窗外推同错；两次transaction是否独立冷启动仅保语义提醒，不新增退出门。合法132.869ms实例间隔与171.182ms实例区间union可保，不加算术硬门。

### A原生展示与真实上下文的已证边界

4张时序表为836+836+217+322＝2211行，独立整数μs oracle逐行端点/时长/owner/状态/行号/CPU核对0错配；两张836为同源重复，不相加成新总体。24业务表每表13字段、共312字段均与各自原生payload一致；21张明确标其它线程，源/范围标签正确，是主体选择噪声而非已证因果越权。HTML30表＝附件1+模型1+原生28；原件63659046B与格式预览63659205B不混用。合法S行未误绑定后续wake；五分状态量尺保真，不因正文错误抹掉这些正项。

实际INIT正文分别97597/35807/306062B，独立报告保SHA；只是日志记录，不冒未观察provider wire。一次调查受理、一次答案受理、一次局部patch受理（3不变/1替换），patch未修正主要成员和量尺问题；不存在被隐藏的finalizer拒绝。最终MD442051B，SHA `22c6ef82c379af4b94920df2dca92a0dcb6e47568e42fded31aff9c8bf32c404`；HTML3428863B，SHA `c75a0eaeb801db266bdf2da5ea560ee77ea07a61c4b8bb8d59a49a7bcdfcea96`。表格MD/HTML逐格等值；仅源/语义审计，不冒浏览器像素QA。

没有根因排序/唤醒链或图调用，不签相关能力也不新设要求。确定性event_search补采62 observations真实发生，不表示所有缺失business self已自动补齐。A完整结论仍FAIL，实收原生表单项PASS不抵销问句缺口。

## B：原代表自然入口——人工FAIL，终态交付未触发

原50steps/n1，原题按线程和窗口各选两代表，比较101/202的资源申请/释放及调用位置、概括进度；不追加内部view或期望值。四个53248B SQLite物理文件来自两份夹具各两份复制，a/c与b/d分别同字节，不冒四次实机捕获。原材料标尺仅供审计，未选c/d的预期不冒本轮取数。

真实discover为`{"request_plan":{"view":"resource_stack"},"root":"captures"}`；返回8计划/2组/4选/4未选。四个trace_query使用原catalog_query_ref，每计划各1次尝试：

| 当前原件/请求线程 | outcome | 原生事件/源帧 | 资源事件流量 |
|---|---|---:|---|
| a/101 | success | 3/8 | Alloc2、768bytes；Free1、512bytes |
| b/101 | success | 1/3 | Alloc1、512bytes；无释放事件 |
| a/202 | success | 1/3 | Alloc1、128bytes；无释放事件 |
| b/202 | empty | 0/0 | 完整所选查询0匹配，不证明完整采集或目标存在 |

c/d在两组均not_executed、attempt0，选择身份不改变。正确保留原查询半开窗、原件范围、源-local主体、事件/帧/组/bytes各分母、重复/缺位深度、未知符号和signed地址；不由同callchain_id合并跨源线程。不要求补查未选成员，也不能将未选等同真正零。

### 完整FAIL的两项正文冲突

1. 原生教学与表已明确源帧不证明有序调用链、未证明完整展开，正文仍写未限定含义的malloc→业务函数箭头及free“栈顶”。最终正确caveat不补成关系凭据；本轮未生成Mermaid或断言泄漏/根因，不扩大失败范围。
2. 最终caveat写“两次窗口查询的4条资源调用栈结果”，实际是两来源×两线程＝4次独立查询、一个共同数字窗口。前文和INIT正确不能把该处混用自动判过。

调查摘要仍传调用箭头，实际INIT03同时带正确原生边界与这一advisory解释，不能把所有成因定为模型随机波动；原生事实没有在本轮丢失，数值/源表完整是独立正向子证据。首次答案受理后，一个facet_ids对象形状的patch被strict decoder拒绝且未staged，最终保留原已受理答案与自动补齐，不能伪称修补成功。读取SQLite字节带来的“源码定位”附表为材料展示噪音，current_source_required=false，未升级用户源码义务。

### NOT_TRIGGERED：缺的是执行后的不可变交付，不是内存状态

实际INIT02完整保8计划、4尝试、3success＋1empty、4未执行，2组/4选/4未选，双方展示均omitted0。3份INIT里saved_index/selection_snapshot及两SHA皆无。唯一discover的index cd0ada…与selection e3b676…记录0尝试/8未执行，不能作执行终态。本轮没有status/representative_status，operator未运行；结构机器验收not_available，不由CLI exit0推PASS。

生产代码真实Complete后使用Save(false)更新alias和同run内存union，不生成fresh Publication；只有显式trace_catalog动作生成不可变同代index/selection/SHA。成文目录正确显示当前Mutable状态，却无自动物化交付。这一已证共享缺口升P0，不能记作预算截断或“模型已有终态凭证仍漏写”。本次没有读取latest补证明，也没有事后运行status/operator。11.2不销账，§326的8查询/operator PASS不搬到本批。

### 原始定位与展示边界

[B独立原报告](results/hmc_native_publication_compat_20261030_v1/sealed-v1/0286-codrax-hmc327-B-audit-v1.md)，SHA256 `a716ee220ee32091dae0bda74418db737337fabce88e5629e88d2160ed000f99`。原收集`/private/tmp/codrax-hmc327-B-materials-v1`及全部可归档原件由[sealed索引](results/hmc_native_publication_compat_20261030_v1/sealed-index-v1.json)逐一映射：4payload/3INIT/1HTML；21参数原件与21日志声明是对应关系，不相加成42调用。日志含非UTF-8字节，原件保留；INIT仅代表实际日志体，不冒未记录provider wire。

最终MD33957B，SHA `d3c2b76626e18168294fca4e8a94d7f8528b99494f95593c603c0e537e3a50e7`；HTML2815425B，SHA `cf63adcebe5a4295e39a4bf23ab054186e0594f22d1a4c2ba4678a17388f51b7`。15张表源审字段一致；collector原false等值诊断来自Markdown ticks/HTML code语法，不覆盖原报告或误升事实错配。无图体，不签图修复或浏览器像素验收。原生5事件/14帧和7个模型选择器、系统补齐保真不抵销正文FAIL。

## 批次边界与封存

唯一full 98包PASS/12无测试、28650顶层PASS/43SKIP、34095子例PASS/14SKIP，265944JSON、零非JSON/重复/未终态。两例各一次，明确完成请求A43/B18、0歧义、最大重叠21.865秒。定向152/346及race25/32通过；旧失败、新soft前提失败原件均保留，lower-cost-full公共分支NOT_COVERED。

本代552件已封存，[index](results/hmc_native_publication_compat_20261030_v1/sealed-index-v1.json) SHA ba1e205759df90d3ebf9d7124d7e8a3a8901c6f915083ce9cf8fda72e7c8ae7f；[收据](results/hmc_native_publication_compat_20261030_v1/sealed-receipt-v1.json)的A/B/full 0/0/0与A/B完整人工FAIL分别保存。34文件代码提交776c8683940a777ec99cfac94bc51ff2e5f42afa不授整答PASS；9文档提交及main远端交付只据[后置publication](results/hmc_native_publication_compat_20261030_v1/publication-v1.json)确认，不在本文内嵌自身SHA。§325/326失败不回签、不补旧publication。下一批优先两个系统缺口并轮换只读登记负控，当前未执行；旧IO/采样/权限、图及其它未触发保持原账，WAL生产NO-GO。详见[本批汇总](parallel_selected_summary_hmc_native_publication_compat_20261030.md)。
