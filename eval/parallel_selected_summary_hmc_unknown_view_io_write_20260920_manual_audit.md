# 未知视图与重复测量教学修复后：IO读＋Python写人工审计（2026-09-20）

固定干净构建 `99556ae4b3da`，批次 `20260920-044644`，2并行×1、每例1800秒。快照 `.codrax/tmp/codrax-selected-20260920-044644`；未改case/oracle，未追加第三次追绿。**机器2/2 PASS，人工1/2 PASS**。机器只验声明的出现性/补丁出口，不能覆盖分位数总体语义。此前FAIL保留原判。

| case | 批次耗时 / 最大上下文 | 过程 | 人工结论 |
|---|---|---|---|
| trace_query_io_request_latency_distribution | 272秒 / 30% | 4次trace_query；completion 3次、1次缺成员集降格；最终一次成文，安全修复blocks字符串 | FAIL：3组24值正确，但明细/总体/搜索窗口混同，RQ/BIO端点被扩大解释 |
| github_issue_dateutil_relativedelta_float_symptom | 153秒 / 28% | 1次空change路径拒绝后修复；只改源文件；4探针＋原4测试通过，工作流verified | PASS：补丁正确、测试未改、交付可恢复；未命中补证恢复，不代销B1 live命中或B2–B6 |

耗时为runner端到端口径。机器repair/reject计数为0不代表写规划零拒绝，以下按实际toolresult计数。完整日志和模型输出仅存本机。

## 1. 审计材料

- IO目录：`eval/results/hmc_unknown_view_io_write_20260920/trace_query_io_request_latency_distribution-20260920-044644`；日志 `run-1.logs/codrax-20260920-044646-000-61778.log`，下文称IO日志。
- IO报告：`.codrax/output/20260920-045114.271-61778.md`、同stem HTML及root-causes.json；统计原件 `.codrax/blob/20260920-044646-000-61778/trace-query-result-59477dc3.json`。
- 对账：`eval/fixtures/hmosperf_io_request_latency_distribution/expected.json`，准入样本RQ读11、BIO读3、RQ写3。
- Python目录：`eval/results/hmc_unknown_view_io_write_20260920/github_issue_dateutil_relativedelta_float_symptom-20260920-044644`；规划日志 `run-1.logs/codrax-20260920-044646-000-61787.log`，应用日志 `run-1.logs/codrax-20260920-044830-000-62364.log`。
- Python计划及报告：`plan-1789904910695169000-61787.json`、同ID `.report.json` / `.final.json`；物化输出 `run-1.applied-tree`、`run-1.materialization.json`、`run-1.write-apply.json`。

## 2. IO：数字通过，总体与测量含义失败

| 组（设备12,80） | 样本数 | min / mean / max (ms) | P50 / P90 / P95 / P99 (ms) |
|---|---:|---|---|
| RQ读 | 11 | 1 / 6 / 11 | 6 / 10 / 10.5 / 10.9 |
| BIO读 | 3 | 2 / 4 / 6 | 4 / 5.6 / 5.8 / 5.96 |
| RQ写 | 3 | 5 / 15 / 25 | 15 / 23 / 24 / 24.8 |

报告25/26/32行三组八值逐一正确，设备、操作、表头及1..14秒主统计窗正确；BIO写未伪造零分位数，按用户要求不画图。以下语义错误仍构成人工FAIL：

1. **报告66行：明细上限误作总体不足。** 原生window_stats准入17对，只展开8条明细，9条超明细容量但分布已全部纳入。答案却说这9条是否影响分位数未知，又把搜索40/43截断合并为统计不全。IO日志3464已明确17/8/9和两种容量独立，3465–3467三组数字/端点/全部提交者/查询窗同记录交付；不能归为系统漏数据。
2. **同段混用查询窗与事件族。** 独立event_search实际0.5..16秒、四个RQ/BIO事件族（2275、2284），答案却称1..14秒RQ issue/complete共43条。三次window_stats始终1..14，后一次更宽搜索不替换统计边界。
3. **报告17、49–52行：端点越权扩义。** 把block_rq_issue当应用提交起点、RQ当应用到驱动全路径；把BIO queue→complete写成BIO调度器内部排队/处理，并推荐RQ代表总体延迟。日志3461–3462已禁止从层名猜硬件/驱动/应用阶段及无映射包含/相减；正确上下文已到达，未被遵循。
4. **次要错误。** 41行把歧义cohort写得像在11条已接受配对内部，实际14起点＝11接受＋2歧义抑制＋1缺完成；62行将P99称最坏情况，分位数不是最大值。聚合合同允许多提交者，但不证明每个实际组都已有多个提交者。

19、56–64行因果边界基本守住：未将设备级分布加冕为具体线程根因，指出还需阻塞/唤醒及响应链证据；跨层不能直接合并的结论也正确，但不能抵销其余错误。

### 调用、上下文和恢复

- 1339、2180、2225均直接调用`window_stats(1..14)`；2275为`event_search(0.5..16)`。**本轮未触发未知view拒绝**，正常调用不是live拒绝修补见证。新共享教学在1065/1294已明确正确view、完整准入总体与非Top-N。
- completion 3次：1411接受；2335因缺成员集在2338降格；2380修后2389接受。末次18→16可选摘要压缩，再丢1个count4/member3不一致的可选条目，15项入账；无principal超cap拒绝，不宣称本轮直接命中新容量拒绝教学。
- completion仍传播“17中只有8含完整分位数”的错误；最终原生交付已纠正，答案仍未遵循。此类正确上下文到达后的错误留为模型质量债，不加本例词句硬门或系统代写。
- 3606安全恢复blocks字符串为数组；3636一次成文、3641–3646接受，零成文重试或旧草稿降级。旁路正常：schema_version=2、root_causes=[]、status=unavailable、reason_code=trace_root_cause_contract_not_active。本题无根因合同，不是文件漏写或根因选择失败。
- **另一个确定性系统缺口：** 报告70行“建议结合源码进一步核对相关组件”不在模型blocks内，是`CaveatFamilyAnswerCoverage`通用模板追加。上下文1414–1417/2382–2385已明确current-source excluded。应改共享来源中性文案，不引入原文关键词分支。

## 3. Python：交付通过，补证恢复能力仍开放

最终仅改`relativedelta.py`：years/months的float值经is_integer检查，整数值转int，非整数值抛ValueError，原int不变；新增一个辅助方法。原4个回归测试及期望逐字不变。人工独立重跑4测试通过，另验证零/负整数浮点、两种属性类型、年份/负月份日期运算、正负非整数及inf/nan拒绝，全部通过（`/tmp/hmc-unknown-view-python-extra-audit-20260920.log`）。

系统执行4个plain Python probe＋4个原生测试、8结果绿，源码编译及变更目标覆盖具备；`run-1.write-apply.json`记录allow_unverified=false、final_run_status=complete、final_verdict=verified、verify_authoritative=true。原仓HEAD仍为种子`3e003569db62e7091a0eb78b9734b684b7ada2b1`；补丁保留于`refs/codrax/applied/plan-1789904910695169000-61787`，提交`c1b751fef5f34a5b8c800e6c984f5cbe0bdc3091`，没有自动合入原仓。原仓仅runner生成的未跟踪.gitignore。

规划曾把路径写成dateutil/relativedelta.py，读失败后自行定位根目录；首个emit_change_plan混入无path条目，1338–1340精确拒绝，1362修后接受。只有一次结构修补，没有循环，不借此放宽路径门或静默接受空计划。

**不能以本次PASS销账原补证缺口**：初始计划已有4个原生测试声明；更重要的是12个behavior_contract均为planning_only_ungrounded、required=0。应用日志837–838的local proof scope本来没有active required待补义务，没有触发旧失败的缺断言补登记/ID恢复/B1止损。因此本次普通apply交付PASS，B2–B6仍开放；终端也明确自然语言验收项不等于逐项独立证明。

## 4. 下一步与保护边界

本批没有缩短600/300/600秒等待，没有按4ms或连接总年龄截断活跃流；两例在1800秒外层上限内自然结束。读例保答案与必选旁路，写例保隔离应用与原测试。收未知view/教学修复后另修来源中性提醒，再推进accepted业务焦点和原生断言补证，不以本次模型误读追加散文门。稳定清单13/79实现已交付、66开放不变。
