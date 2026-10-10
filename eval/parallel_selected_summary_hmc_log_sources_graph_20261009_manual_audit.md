# HMC §217：完整日志来源与事务交接图人工审计

日期：2026-10-09（本地），固定源码 dbb3b42753e5。两例各一次、并行2；runner session8665正式exit0。机器2/2，完整人工0/2；不追加第三例。主代理与两位独立代理核对原件、实际工具载荷、模型上下文与终稿。

| 用例 | 机器 | 人工 | 耗时 | 实际通路 | 结论 |
| --- | --- | --- | ---: | --- | --- |
| log_shared_sources | PASS | FAIL | 152s | log_triage→2次log_query→finalizer | 原生6+3记录正确；终稿漏内核记录、错来源行、把非法日期改为有效时间 |
| trace_transaction_handoffs | PASS | FAIL | 176s | perf_triage→2次trace_query及确定性主窗补采→finalizer | 非空图但虚构回复/交接、时序倒置；普通关系问题被root_cause_trace族跳过完整边核验 |

机器PASS只证明runner声明的工具调用/产物条件，不证明事实、范围、图或答案完整性。原始失败答案不回写，不倒签修后GREEN。

## 1. 完整日志：底层正确，终稿与交接失败

原件为eval/fixtures/hmosperf_log_sources/app/session.log及kernel/session.log.gz；内核解压内容与旁置kernel/session.log逐字节相同。输入前后SHA不变。QUESTION只提出事件、记录者及关系，不包含系统护栏答案。

结果目录：eval/results/hmc_log_sources_graph_20261009/log_shared_sources-20261009-185501/，审计run-1.primary.md及run-1.logs/codrax-20261009-185503-000-10847.log。

### 原生结果

实际两次log_query（日志1311–1317）完整返回应用6条/7物理行、内核3条/4物理行，均omitted=0。两个full-page JSON保原source、decoded行范围、原字节与SHA。应用健康AssetLoad跨L2–3，坏日期L4、孤立续行L5、unknown L6不串owner。内核首条跨L1–2；L3 not_found没有PID/TID及墙钟；L4 snapshot PID91/TID92。三条boot_ns为精确字符串9007199254740993、9007199254741001、9007199254741010。不存在解析器/查询漏行、浮点损精度或压缩源被改写。

### 终稿FAIL

- primary.md:3及表格只列6个有归属事件+两条未知文本，遗漏内核L3 not_found独立行及其未知身份；只在假设性因果段提到not_found不能代替事件清单。
- primary.md:5/16/19把内核首条L1–2写成L2，snapshot L4写成L3。
- primary.md:18剥去非法99-09，把时间显示为合法-looking 09:00:00.004；L7随后断言应用四条时间严格递增。保PID/TID不等于时间有效。
- L23将“未见跨源同步见证”改成“不存在同步机制”；未知不等于不存在。
- AssetLoadError和MalformedTimestamp为triager自拟类别，不是原日志声明的异常类型；原文分别是标签/消息与解析失败内容。

最终仍保住跨源因果未证、相同request仅调查线索、孤立文本不继承PID/TID。不能因此签整答PASS。

### 系统上下文实证

预分诊第一次emit（L295）错误因果/重复错误/非原文证据被拒，第二次（L334）仍有未证墙钟、归属及因果摘要。最终L1908明确过滤部分自由摘要与身份，不能声称全部透传；但L1914–1915将snapshot错误log_line=3随原文保留为事实，终稿复现该错坐标。explorer closure L1354另编造not_found墙钟.002和跨源排序；L2124已排除自由closure文字，其错误不能一概视为获权威支持。

确定性交接缺口：

1. Observation Ledger L2110只投7/19：3条旧triage+4条应用native。整个内核源3条native和两个coverage被裁掉，来源覆盖无保障。
2. native Value截为约100字，前置空boot字段/字节坐标占满；PID/TID、解析有效性等关键字段不可见。每条重复三个长hash，耗预算而没有增加事实。
3. Known Facts L1997–1998只剩boundary与locator；Typed Handoff L2134–2140声明11 refs，却仅展开前6个应用refs。
4. Claim Binding L2082–2088均来自triage；L2104实际已有两次query仍写deterministic_runtime_queries=0。
5. L2159–2166将analyzer自动生成且无source的行号称作“用户指定行号”。L1956在只有回答工具的最终阶段仍建议留给后续查询阶段。

本fixture完整原预览仍可见，模型本可答对，故不能证明预算单独导致所有错误；但精确事实被弱摘要/无用元数据挤出已成立，不能按纯模型波动搁置。

## 2. 事务图：原生关系正确，身份与语义校验有绕过

结果目录：eval/results/hmc_log_sources_graph_20261009/trace_transaction_handoffs-20261009-185501/，主日志run-1.logs/codrax-20261009-185503-000-10845.log。

独立oracle：主窗[1,1.05)有5提交/4物理消费/7协议键/3唯一匹配。[101,7]提交0.999在窗外，[101,12]消费1.050为右边界背景；[101,9]两提交有歧义，不能靠近邻择一；[101,10]缺消费、[103,11]缺提交不等于丢失/阻塞；[999,13]中的999是协议键字段，实际发出TID101，不能造线程999。

终稿开头虽写5/4，却误称“含1条边界事件”，无法确认的总述只列3项而漏[101,10]，后文才补第四项。结论未直接宣称根因，主窗补采帮助修回部分计数，但这不是完整PASS。

图有11条箭头，不再是旧零边图，却把缺消费/歧义/身份未确认的提交画成已发往RSMain；把消费观察画为RSMain→AppUI回复，原Trace没有该返回消息证据。图中1.020之后回到1.010、再到1.004，未标明倒叙；activation与箭头仅表达绘图语法，不能替代线程执行或处理驻留。不能只因可解析/存在一条边就通过语义审计。

系统缺口已精确定位：实际analyzer声明runtime_question_profile.scope=relation_analysis且frame_causality_requested=false，但下游Family仍是root_cause_trace。本批minimum coverage及既有完整edge证据核均按Family直接退出，造成普通原生关系图未获边身份核验。最终4个node-pair anchors都没有from_identity/to_identity，却覆盖11条箭头，emit和post均放行。应根据既有精确关系请求与来源资格统一图核验通道；不能全面撤销根因图专属通道或扫描用户/答案关键词。

## 3. 台账与后续ROI

- 05.1完整材料保真、公开查询/原文分页、来源收据及运行通路满足原输入任务范围；末版全仓与发布收据齐后记实现交付。初审因最终回答失败暂不销账的判断，经回读原退出条件纠正（统一账本§217.9）；实际最终来源绑定和全题验收仍FAIL，归16.4/18.4，不新增输入任务退出条件。
- 12.5/18.4原公开空图、中文ID、inline丢边及空白冒成功已修；本次又暴露族分流造成的边核验绕过，属于同一图缺陷轨，应优先按typed信号修复。完整业务角色→原生实例仍开放。
- 16.4/18.4下一高ROI为统一原生事实投影：按source保coverage/事实多样性、原生精确字段先于重复triage、来源hash每源一次、refs实际解析到事实，不直接加大全局预算。范围/身份/无效时间/未对齐保结构化传递，不用用户加限制语或模型输出关键词门。
- §216量测完整三表未选用、各表固定4行而128预算闲置、metadata压缩误报查询截断，与本次同属“原件完整不等于模型实际拿到有效事实”；应共用方案，避免为log单造补丁。
- 保留原始机器/人工FAIL和未命中的写模式/二进制能力验收；本批无写例，不补签旧写FAIL。

## 4. 评测后修复的证据边界

`a14f1221e`已修普通非帧关系请求被旧root_cause_trace族豁免的确定性旁路：family保持不变，compile/pre/patch/post统一按精确typed资格使用原生身份关系核验。原公开RED同源断言已GREEN，相关普通/精确race四包通过，实际局部修补保留原说明文字；真正根因/帧因果投影与自动补齐回归保留。详见统一账本§217.7。

这只能证明已修旁路及公开修补路径，不能证明本份模型回答已经正确。两个原live人工FAIL保持；同一actor pair多消息绑定、全题时间顺序和日志事实交接仍开放。没有第三例或回写原答案。
