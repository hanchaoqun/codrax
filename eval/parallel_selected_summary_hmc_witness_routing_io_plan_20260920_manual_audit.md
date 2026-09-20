# 固定双例人工审计：IO分布读＋C++窄改计划

- 日期：2026-09-20；sweep `20260920-042031`。
- 干净构建：`bc90f5aaae0b`；快照 `.codrax/tmp/codrax-selected-20260920-042031`。
- 恰好2并行×1，单例1800秒；未改case、fixture或oracle，未追加第三例追绿。
- 结果根：`eval/results/hmc_witness_routing_io_plan_20260920`。
- 机器1/2、人工1/2。C++的PASS只限计划；IO历史FAIL不销账。

| 场景 | 机器 | 人工 | 墙钟/上下文 | 裁定 |
|---|---|---|---|---|
| `trace_query_io_request_latency_distribution` | FAIL | FAIL | runner382秒/原生380秒，30% | 未知统计视图被静默执行为事件搜索；最终总体、分位数、层级及缺端解释错误 |
| `patch_cpp_typo` | PASS | PASS（计划） | runner52秒/原生51秒，28% | 一行patch可应用，源码未应用；不作为运行时或逐合同证明 |

## 1. IO：不是仅模型波动，先修确定性入口缺陷

报告：`.codrax/output/20260920-042651.852-43371.md`，同名HTML与必选根因旁路齐全。日志：`trace_query_io_request_latency_distribution-20260920-042031/run-1.logs/codrax-20260920-042033-000-43371.log`。

### 1.1 新系统gap：未知view静默成功为另一种查询

日志1306：`trace_query`实参为`view=storage_latency_by_layer,time_start=1,time_end=14`。该名字是输出区段，不在公开view枚举。1307–1311运行标签仍写原view，1312工具却`ok=true`，1313起实际报告`view=event_search`。原生blob `.codrax/blob/20260920-042033-000-43371/trace-query-result-30320c09.json`的`view`也为`event_search`，没有`window_stats`或IO分布。

这不是正确分布到了模型后被忽略，而是调用入口允许未知枚举进入引擎默认分支，静默改变查询语义。模型后续转去多个event_search手算。应从现有canonical视图注册表单源校验，在明确参数边界报告未知值及合法选择；不按用户原文判断、不为单个输出字段增造统计内核或猜别名。合法alias/空默认/显式时间窗/自动补齐必须保护，engine直调同类行为也须核对。

本轮analyzer确实声明`bounded_fact_set`＋`io_latency,count_or_duration`（850、853），不同于前两批宽分类。新宽分类IO释义helper未命中；由于从未真正运行统计视图，专用释义也没有原生IO统计可解释。不得称02cea新helper已在live签收。

### 1.2 最终答案人工FAIL

夹具`eval/fixtures/hmosperf_io_request_latency_distribution/expected.json`的全体配对为17：RQ读11、BIO读3、RQ写3；分组均为设备12,80，RQ读一组2请求因配对歧义被抑制，另1请求缺完成。

- 答案却将RQ读/BIO读都写成8对、1–8ms，写请求/未完成/歧义均误称0，丢失三个总体的对应关系。
- RQ读正确均值/P50/P90/P95/P99应为6/6/10/10.5/10.9ms；最终为4.5/4/7/7.5/7.9ms。BIO与写组亦没有交付要求的真实分布。
- 将BIO端点误作rq issue→complete，又说RQ=BIO＋队列等待及两层完全重叠；没有跨层请求映射证据，不能构造这类包含或减法关系。
- 直接称reader在等IO时被阻塞，但没有调度/等待链证明；虽然结论正确说“仅凭分位数不能判断响应根因”，前文仍过度主张。没有系统生成链上根因，不能把它描述成根因投影选举错误。
- 将时间戳小数位/当前样本步长误当采集精度1ms、又以稀疏/显示样本数推捕获截断。原始trace有六位小数，样本不能单独证明捕获精度或完整性。

也有模型错误交接：2437第二次completion已写11/3/3成员集，但复合scalar值内含错误手算，grouped_count用非整数串。最终仍写8/8/0。不能以入口修复已规划就声明其余模型错答全部根治；也不能把模型手写数值当引擎已算出的测量。

### 1.3 过程、范围与保护项

1次analysis接受；6次trace_query（实际均事件搜索）；2次completion均接受，零completion硬拒绝；1次不可用read_file尝试。1次完整answer_document首次接受，零成文拒绝、零JSON恢复/降级、无图（用户不要求图）。finalizer持续96秒输出后正常完成；不是无答案/活跃流被提前中止。

1385探查时模型另加`line_start=1,line_end=46`，既有行窗优先语义使事件库存包含0.5..16s；其库存同时披露query与实际scan范围。此处不能把43总事件当指定时间窗的账户，指定1..14内为39。该模型加行窗行为与未知view失败分开留档；本批不据此扩大用户显式窗或变更既有行窗语义。

最终上下文有123条runtime observations、122条确定性查询记录（不同发布视图计数口径不等于122次查询），原有预处理自由文字8条被抑制。消息没有新IO measurement card、没有原生三组八值；因此不能以“有大量trace行”冒充“已交付分布”。

旁路schema2、`root_causes=[]`、`status=unavailable`、`reason_code=trace_root_cause_contract_not_active`正确：本题统计/证据边界没有激活根因选择合同。文件没有丢失，也不应把无调度证据的请求驻留提升为链上阻塞。

## 2. C++：普通窄改计划通过，未自动应用

证据目录：`patch_cpp_typo-20260920-042031`。原生日志`run-1.logs/codrax-20260920-042033-000-43380.log`，计划`plan-1789903281977952000-43380`。

- JSON计划仅`main.cpp:19`一处`retrun`→`return`，kind=patch；按JSON原始字节提取，`git apply --check --recount`退出0。
- 原repo HEAD仍seed `a853302f1cb3869e379ef7a6741851a02eb3d4b9`；源码与fixture哈希相同、tracked diff为空，仅harness `.gitignore`未跟踪。
- workflow完成不等于已应用：batch=planned、plan=pending_approval、无applied_commit_sha。没有apply、verify或编译执行。
- 日志932教原生JSON数组，1151提交合规数组、1158一次接受；943、966–969的窄改及无需额外probe教学一致。零工具拒绝、JSON修复、重试或降级。
- P2观察保留：验收文字“Hello,+name”没说明空输入取world，属于`planning_only_ungrounded`，不是已执行的合同证明。本例核心一行修复需求已满足，不因此伪称完整业务验收。

## 3. 排期与不误销账

优先修未知view静默回退，属于HMC-01.1/01.2及08.1的公共入口缺陷；再验证同一canonical注册表贯通schema/公开工具/engine/stream/script，保合法别名。同期容量教学审计已证伪“3×8原生测量必须重复写入cap16”的硬死局；存在“删任一条目永久丢值”的提示不准确，另片只修教学，不降容量或类型门。本次没有触发cap拒绝，不作为该提示修复live证据。

WRITE-PROOF-IDENTITY与B1仍以独立回归/全仓收据交付；本例没有进入补证分支，不能当B1命中证明。B2–B6、HMC-02.4业务实例焦点、完整图表达矩阵及其它66项开放不变。没有第三次回放，没有改历史verdict。
