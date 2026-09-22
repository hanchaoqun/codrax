# Native execution / reader-field 固定双例人工审计

日期：2026-09-22。机器结果见[原始汇总](parallel_selected_summary_hmc_native_reader_20260922.md)。不修改机器verdict，不回签历史FAIL。

## 1. 版本、范围与结论

- 冻结构建：`e2f3931ded53-dirty`，buildTime=`2026-09-22T14:42:18Z`；dirty仅文档，Go/构建输入已提交并由runner复验。
- runner1192正式exit0，开始14:43:02Z，快照`.codrax/tmp/codrax-selected-20260922-074302`；严格2并行×各1次，无第三例或单例追绿。
- 机器 **1/2**；完整用户任务人工 **1/2**。快照包含409448e7e/5722d6b39/e2f3931de，不包含随后CTest测试适配、完整system教学/标签分离的增量。
- 用户任务通过不代表所有能力通道验收，原始数字正确不抵销成员/总数错误。

| case | 机器 | 人工 | 时间/上下文 | 本轮边界 |
|---|---|---|---|---|
| github_issue_dateutil_relativedelta_float | PASS | PASS（用户任务）；补绑定未验收 | 外层148s/案例145s，28% | 源码正确、4原测试通过且未改；新执行ID命中。模型probe预期错误并实际失败，警告保留；7合同均planning-only，不代销旧required缺绑定 |
| trace_query_jank_field_inventory | FAIL | FAIL | 外层181s/案例179s，31% | 六个大整数及3时长正确；99帧干扰被纳入、总数4、未降序，同轴换算说明漏答 |

## 2. Python：改动、测试、报告与上下文

目录：[本轮产物](results/hmc_native_reader_20260922/github_issue_dateutil_relativedelta_float-20260922-074302)。完整读取run-1.out、两阶段日志、计划/最终报告、交付源树/原测试及验证收据；本例plan→apply，不验收read finalizer、Trace图或根因旁路。

1. 交付`e54e824e7d770e533f89f9c63e43652fd5d7368f`只改relativedelta.py，+8/-2。仅对float做整数值规整，不损失原有大整数；非整数值float抛ValueError，日期运算保原路径。seed HEAD未合并交付；未跟踪.gitignore是运行器产物，不笼统称工作目录全净。
2. 原fixture、seed、applied tree的test_relativedelta.py SHA256同为`8e7ebfe79df13b1c19d25d7ba7b9d5042b4dac4c05bef45e43710114f767c484`，交付diff无测试/README/配置改动。真实执行python3 -m unittest discover -v，4个RelativeDeltaFloatTest方法通过，不是只编译/零测试。
3. native命令及4断言同带`native:35c6:18d7abd6726caf90:1`；语法检查/probe/恢复记账未伪装原生断言。新producer命中，但无PTO，不冒称跨执行consumer或只读补绑定live命中。
4. 模型probe第18行错误断言2020-01-15加2年和12个月应为2021，实际应为2023。失败是预期写错，不是产品日期算错。diagnostics保AssertionError、完整输出引用及“不代表已证实产品缺陷”限定，最终卡没隐藏失败或称probe成功。不应为错误probe改产品；本例没有逐合同断言权威。
5. planner日志1053–1075保原测试约束及7条planning-only合同，没有run_existing_test（原题仅禁止改测试）。异常expected仍为自然语言句而非单一异常类型，被原有质量修复留为planning-only，不是本次required被新代码豁免。最终verified符合本轮typed义务和真实4测试，但不代销§131 required异常缺绑定的旧FAIL；分析契约质量仍留账。
6. 首次emit把changes写为字符串碎片，兼容层还原后第二个成员只有verification_probes无path，精确拒绝change_path_empty；下一轮正确对象提交，保1probe/4验收项。见日志1220、1250–1253、1322–1332，没有静默删probe、推断源码路径或关键词硬门。修补成功不等于全部JSON教学无歧义。

可观测性边界：apply独立DEBUG日志仅5行；其controller过程来自run-1.out，执行事实来自落盘报告/交付树，不能冒称已拿到apply每次完整system/user消息。plan阶段有完整保存的上下文，这两种来源分开审计。

## 3. Trace：完整答案与原生7行宽查询

目录：[本轮产物](results/hmc_native_reader_20260922/trace_query_jank_field_inventory-20260922-074302)。完整读取primary、principal/transcript、日志、原16行Trace、查询blob和schema2旁路；独立第二审一致。

- 正确集合是源行9/11/3：appid620、帧数7/4/2，头时间9007199.54/.55/.51秒；六个>2^53原纳秒整数、终点减起点70/40/20ms均正确。
- primary12额外纳入other jank_event_sync的99帧，3/14/30写成4条，28要求用户再确认排除。用户要jank_event_sync记录，不是包含字符串的任意标记；7/4/2/99连扩大集合也非降序。计数机器FAIL有效，不是仅regex误报。
- 日志1181仅event_types=[trace_mark],pattern=jank_event_sync宽查询，未调用已有event_field_filters。原生如实返回7条文本匹配及total/emitted=7，1944完整呈现。第13行只有raw/span_name，**无jank_event解析字段**，第15行有invalid issue；原生未错误注册99。explorer1231自行从raw拆值并误称parser仍解析；其aggregate/clock closure在最终视图1961明确省略，不当最终权威来源。
- typed行足以筛出3条，前缀负控存在。错误是模型将宽查询raw候选升为所求语义成员，不是原生字段过滤失效或上下文缺行；不能据单次认定纯模型波动，更不加问题/答案关键词硬门。
- appid仅称应用级标识，未冒充PID；目标线程、调度事件和因果链不足基本正确。未再宣称异时钟，但漏解释同源轴、ns↔s单位换算、报告头不等于症状端点。不套用上一轮“否认同轴”的错误描述。
- 1次query、1次调查完成、1次成文、1次接受patch，无成文硬拒。清单不要求Mermaid/Trace因果投影，本轮不验收显式因果窗、分片/调度链。`.codrax/output/20260922-074601.312-13508.root-causes.json`正常生成：schema2、空根因、trace_root_cause_contract_not_active符合未启用根因合同。

## 4. 新发现及未销边界

1. 新reader教学2015/2039/2041真实到场，允许原始字段和非封闭卡名；system1545却仍全称字段名只能作引文旁键、不能进入句子，对应internal/skill/defaults.go。§135初始消息公开验收有效，不等于完整system+user无冲突；另一最终边界段有同类旧句，后续完整Finalizer公开测试须覆盖。未证该残留直接导致99误收。
2. 软提示2118–2120将“第4维：匹配条数”当用户label，2151patch照搬并重复。typed输入本身label=匹配条数/index=4（779）；序号由系统包装，是共享标签/内部顺序混用的泛化展示债，不是模型最初写坏label。不得扫答案改词或改维度数值。
3. §131/133历史FAIL不改，本次结论独立；后续生产增量不借本轮live签收。原生只读登记/新执行代次、caller双轴、业务局部补齐、其它平台产物新鲜度及66父开放项不代销。
