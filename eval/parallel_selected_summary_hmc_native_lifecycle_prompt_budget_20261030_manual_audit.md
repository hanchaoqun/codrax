# HMC326 两例独立人工审计汇总

两例原问题、材料和预算均未改，各运行一次。A进程0/原smoke PASS/完整人工FAIL；B进程0/唯一原operator PASS/原合同自然人工PASS。唯一全仓8顶层/2子例FAIL，所以完整能力新增0、正式交付子能力0，27已交付/52开放保持，11.2交付待全仓。不能把原生正确、自然验收、全仓和正式交付混为一层。

后继注记（§327）：新代全仓PASS及代码776c8683940a777ec99cfac94bc51ff2e5f42afa不改变本报告失败终态；本代B已通过的8次查询/同代status/operator合同不搬用到§327仅4次查询且终态入口NOT_TRIGGERED的运行。§327 A/B完整人工均FAIL，当前A父账户窄窗已有明确限定，不重复认定本报告旧范围错误；生产产出/跨通道遗漏另按新代证据记账。552件新封存不替换本代889原件，也不授旧代publication；详见[§327手审](parallel_selected_summary_hmc_native_publication_compat_20261030_manual_audit.md)。

## A：真实105版元宝启动实例——完整FAIL

原`donghu_v105_yuanbao_activity_instances.case`，15steps/n1，只分析附加trace的TID46571及3733.491839–3733.795890s启动，比较两次activityStart及主要子阶段的总量、自身量和线程状态。没有新增必须画图、给根因或列尽所有子节点的要求。完整ftrace为63659046B/412546行，SHA256 `be59c0648ac7e1d8a2d117f5dc36efa0c92b5f3686671719ada435c16713a20c`；物理头线程46571与marker载荷2227分开，预览封装63659205B没有冒原件大小。独立整数μs oracle沿用原封存版本、未重算或注入模型。

四个完整业务账户原生正确、实际成文INIT可见、最终正文数值也正确（单位ms）：

| 业务账户 | 总耗时 | 自身墙钟耗时 | 全区间Running / Runnable / Sleeping / D总计 |
|---|---:|---:|---|
| 第一次activityStart | 59.695 | 6.770 | 35.932 / 1.158 / 19.120 / 3.485 |
| 第二次activityStart | 111.487 | 3.938 | 83.926 / 1.971 / 24.234 / 1.356 |
| 登录页performCreate | 46.600 | 21.605 | 26.886 / 0.818 / 15.442 / 3.454 |
| 首页performCreate | 103.315 | 83.143 | 80.727 / 1.743 / 19.489 / 1.356 |

此表D总计是D未标记桶加调度IO桶，不代表全部已证IO。用户304.051ms窗口的174.935 Running、4.620 Runnable、119.302 Sleeping、5.194 D总计也正确；D细分3.610与1.584没有被抹掉。

三类实质错误仍在最终Markdown与HTML可见正文/手写表中：

1. **D/IO集合错配。** 实例1 D未标记3.333与IO0.152、实例2 D未标记0.006与IO1.350是互斥桶，正确总D分别3.485和1.356；模型却写“不可中断等待／其中IO”。主要子阶段表又遗漏独立IO项，导致登录页状态合计46.448而非46.600、首页101.965而非103.315。已知IO不是未知或漏采，不能因表头改含义丢掉。
2. **self/Running量尺越权。** 首页self83.143被说成纯CPU执行而非等待，实际self包括70.213 Running、10.413 Sleeping、1.161 Runnable、1.356 D/IO。第二实例Running83.926占111.487约75.3%，不是几乎全部；“运行占主导/比第一次更高”和合法51.792ms总量差可保，不应否定已知等待。
3. **父账户适用域外推。** clientTransactionExecuted在activity窄窗内self0合法，但原生广窗账户是[3733.491839,3733.615757)、总123.918/self2.375。正文未限定窄窗就称父整体self0、时间全来自子层。受理调查unit[0]已这样叙述并实际进入INIT，与正确广窗账户同包；是具体共享解释冲突，不是本轮缺原始数据或父账户被预算裁掉。

26份独立query payload不等于43次真实trace_query声明/成功回执。原失败均保留，包括四次调查提交前三次的空选择、显示资格和member_notes形状拒绝，最终第4次受理；只一次答案replace_all成功，没有局部patch。四完整self及独立状态在INIT03，D互斥桶/未知分类教学也在。6个调查解释单元仅[0,1]实际发布；另一个错误D单元[3]未进入本次预览，不能虚构它污染了最终回答。

动态预算实际allocated390257B、published316513B，省略97252B/1个预览依赖组、未执行组0；没有据此判关键账户缺失。实际首发156564/200000只是估计，预算也不是精确provider容量保证。前两类有足量正确原生输入，有限留账；第三类适合后继按typed账户范围公开复现，不删除合法窄窗事实、不用业务名关键词硬门。

最终HTML42表＝附件1＋模型2＋原生39。7张时序表分别217/322/155/836/836/291/836行，共3493展示行，逐行对照既有oracle无缺失、倒序或状态/时间不符；三份836行全窗表只是同源重复，不是三个独立总体。实际起止和按业务窗裁切的量测起止分开，Sleeping未误绑邻近唤醒、Runnable与Running未混写。无模型图或patch；root-cause合同未激活，空根因列表不证明没有根因。未做浏览器像素验收。

最终MD662752B，SHA256 `c95c612fa54ebd59a7b7a7af6cad92c312225a0a573dcb1b0417d99e0280ee73`；HTML3767943B，SHA256 `b07461a574ff8f928957bf96f3097991e7970ee893edae8cb713f0b41c64d75d`。原日志无效UTF-8字节原样保留，不用scrub版覆盖。完整审计及原件坐标见[A独立报告](results/hmc_native_lifecycle_prompt_budget_20261030_v1/sealed-v1/0503-codrax-hmc326-A-audit-v1.md)。其局部正项在批次交付口径下仍是候选验收证据，不能计正式交付子能力。

## B：原代表选择自然入口——原合同PASS，交付待全仓

原题50steps/n1，按线程和检查窗每类选两份代表，比10.000–10.050s内线程101/202的资源申请/释放及调用位置、概括进度。没有增加内部view、预选ID、期望值或补触发提示。四个53248B SQLite物理文件是两份既有夹具各复制两次，不冒四次实机采集；a/c与b/d各自字节相同，物理来源、查询和准备身份仍独立。

模型真实discover带`request_plan.view=resource_stack`，得到8计划/2组/4选/4未选，先执行入选c101/d101/a202/b202，再补查未选a101/b101/c202/d202；补查不改变未选身份。每个计划恰好一次尝试，终态8尝试＝6success＋2empty，0failed/0not_executed；入选及未选各3success/1empty。一次原status返回的完整index与selection snapshot同代实存hash一致，不拼latest alias或另一次返回。唯一原operator exit0，随后独立人工通过。

| 物理来源／线程 | 代表身份 | 资源事件／源帧 | 分配次数／bytes | 释放次数／bytes |
|---|---|---:|---:|---:|
| c／101 | 入选 | 3／8 | 2／768 | 1／512 |
| d／101 | 入选 | 1／3 | 1／512 | 0／无事件 |
| a／202 | 入选 | 1／3 | 1／128 | 0／无事件 |
| b／202 | 入选，empty | 0／0 | 0／无事件 | 0／无事件 |
| a／101 | 未选、已补查 | 3／8 | 2／768 | 1／512 |
| b／101 | 未选、已补查 | 1／3 | 1／512 | 0／无事件 |
| c／202 | 未选、已补查 | 1／3 | 1／128 | 0／无事件 |
| d／202 | 未选、已补查，empty | 0／0 | 0／无事件 | 0／无事件 |

8个原生payload逐个核对既有oracle的事件/线程/操作/大小/地址/栈帧字段，未发现差异。半开右界10.050排除；原件10.000–10.100只作背景，不变查询分母。保重复深度0、缺深度1、未知/歧义符号及原地址；同callchain_id不合并跨源/线程实例。正文符号箭头所在块明确“按原深度顺序，不代表函数执行顺序”，没有额外谁调用谁或业务根因断言，不能靠箭头字符判越权。

3条实际INIT保目录/分组/selected状态、8查询的测量选择及原分母/owner；页读取incomplete不覆盖独立原生集合。调查解释发布2/5（indices0、3），完整边界明示；早期“source_revision互异等于文件字节互异”等解释最终没有传播。当前源码义务未误升，citations=[]。唯一答案首次受理，两张distribution绑定c101/a202，其余自动原生补齐不算模型又查询。此例没有自然触发预览省略/no-fit/repair预算。

最终MD39284B/314行，SHA256 `f395f30ba7268918f31cd73f02a6619a30878d5884da9a8e054dd94da5faca57`；HTML2821523B，SHA256 `fd29ed66b771b2a2c2ba081d301863c6143eb01a047c4da12cae44d017a416eb`。正文及16表全文审核；原collector的table_cells_equal_diagnostic=false来自首表5个Markdown代码定界符与HTML code标签序列差异，逐个核对字段相同，原false报告没有改写。无图体，不签图分支或像素验收；同源解释重复作为展示质量欠项，不额外阻断原合同。

完整原件、真实调用顺序和独立审计见[B报告](results/hmc_native_lifecycle_prompt_budget_20261030_v1/sealed-v1/0524-codrax-hmc326-B-audit-v1.md)及[唯一operator凭证](results/hmc_native_lifecycle_prompt_budget_20261030_v1/sealed-v1/0694-codrax-hmc326-representatives-audit-v1-receipt.json)。历史失败保持原代；B通过不授只读登记、sample-only、坏源恢复、完整硬件、WAL或全部图/预算边界覆盖。

## 批次终态

唯一full确为8顶层/2子例FAIL，不由A/B过程0、B自然PASS或定向/race绿覆盖。详见[批汇总](parallel_selected_summary_hmc_native_lifecycle_prompt_budget_20261030.md)与[原full统计](results/hmc_native_lifecycle_prompt_budget_20261030_v1/sealed-v1/0639-codrax-hmc326-full-stats-v1.json)。889原件已不可变封存，index SHA256 `1573ed5fb73eb7cfa3a1170ec483b14d7c6957505d1ff0edd79ad64f6a16c07a`；本代不独立发布、不重封、不追跑。下一代先闭环已证系统缺口；旧IO/采样/权限/未触发仍留账，WAL零副作用未满足前生产NO-GO。
