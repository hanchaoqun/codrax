# r1024 人工审计：方向关系一致性与跨仓补验证

- date: 2026-09-07T01:32:28Z
- sweep_start_ts: 20260906-183218
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

基线 `c55a5a51d`，严格两路并行。人工阅读最终答案、实际模型上下文、工具拒绝过程、根因旁路、写模式计划/补丁/验证报告。机器通过不替代人工正确性；以下保留原机器结论。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_memoclaw_text_search_multirepo_py | FAIL | eval/results/github_issue_memoclaw_text_search_multirepo_py-20260906-183228 | log_regex,write_apply,write_patch_oracle | none | 245s | 28 | read=10,repo_map=3,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=3,prune=0 | partial | 实现与权限边界正确；无成功替代探针，保留未验证正确。proof-only禁读与教学要求读冲突，B1578 |
| 1 | real_trace_h11_cross_direction_overlap | PASS | eval/results/real_trace_h11_cross_direction_overlap-20260906-183228 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 291s | 53 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=5/2,fin_reject=0,unavail=0,prune=0 | partial | B1571/73/74入模与正文改善；投影/五项根因JSON齐。纯Trace被要求源码operation ownership，B1576；正文业务总结及口径仍有残余 |

## Trace

- 产物 `.codrax/output/20260906-183717.463-42435.md/html`，唯一因果投影、实际占时与既有规则可消双轴、链上业务族、背景隔离保留。根因 JSON 5711 字节、available、模型选择 5 项，候选机理/金额组成限定保持；不是系统自动代选。
- **B1571 生产正证**：最终上下文完整候选14项、展示8项、complete=false；#9–#14没有再被宣称无资格。**B1574 正证**：两份方向交接中四席严格2+2，headline `#3@b2891da3/#4@b790bf37`，额外仅#8/#9；同一Comp不再重复两个身份。
- **B1573 正证**：新共享教学真实入模；模型明确最大单项不等于方向总和，12.115ms仅为两项已证同向小计，跨方向物理关系未建立且不相加。相较r1023，不再把所有领头值一律称方向可回收上限。仍保留“独立候选”含混措辞、49.4%无分母、表格通用列名、业务修向总结弱及内部术语，不对正文增加扫描或改写。
- **B1576 / P1 确认**：analyzer维度3/4为function_or_purpose，但current-source明确excluded，且informational_runtime_only waiver已接受；operation ownership教学和completion门仍要求grounded源码证据。工具另一面却正确拒绝将runtime trace行当源码，并明确告知可直接结束。4轮无进展后靠low-delta force-complete脱困，不能称合同已一致；应共享typed source适用域，保留维度与模型role，不伪造源码证据、不改问题关键词路由。
- 原因统计与状态口径需保持区别：target状态标记IO=0，不等于链上没有完成闭合的IO响应延迟（47段12.658ms）。先核实模型措辞与已供给两个证据域，不为该固定窗补正文规则。B1568 rounded特定旧分支本次仍非live正证，真实fixture覆盖已保留。
- 4次trace_query，0成文硬拒，1次advisory patch；inv指标5/2外还有DOWNGRADED过程，不能只读摘要数字忽略多轮源码合同矛盾。上下文最高53%，不是预算耗尽，也没有活跃流4ms/4min固定年龄降级。

## 跨仓写模式

- 原实现提交 `81252d72` 仅改`memoclaw/client.py`，6增9删；sync/async均改POST `/v1/search`与JSON(query,limit,可选namespace)，方法签名和await保留。API reference、原测试逐字不变；只读api-docs/typescript-sdk干净。人工另执行sync/awaited async×namespace有无均通过，不写回原运行报告。
- 初计划无行为probe，仅声明不存在的断言ID `text_search_contract_checks`。项目make成功只给aggregate，0/1真实合同未覆盖；不能因B1572修复而无条件消解。后续3条probe均用错误的`python-sdk/memoclaw/client.py`父仓前缀且只是AST/字符串，全部顶层异常，未执行sync/async行为。最终ledger 6/15 covered、9项未覆盖，`unverified:verification_proof_incomplete`诚实；本次是B1572的真实负例，正臂待有成功替代凭证的live验证。
- **B1578 / P1 确认**：proof-followup已获系统授权，却从首次即只开放emit_change_plan/run_tests；同一planner上下文要求先read_file、实际evidence=0，3次read_file被拒。历史已定位不等于这次dispatch已拥有当前worktree源码，促使模型猜路径/参数。最优方案是复用既有授权和只读预算给出当前worktree读取能力，保持proof-only不能修改源码、不扩大主仓权限。
- 模型探针对`json={`的固定字符串检查也会拒绝合法`json=body`，属于测试设计不足；不能按AST/async/源码关键词加硬门或让系统重写探针。最终摘要偏重末次proof计划而少展示早批源码修复，另记呈现观察。

## 横向能力边界与下一批

- B1575：文件级通过的probe及模块耦合≠逐合同、逐方法独立执行证明。现有ContractRefs为模型声明，系统无逐ref执行器凭证；先诚实说明验证口径，再设计跨语言执行器证据，不用额外模型自报枚举铸硬权威。
- B1577：callable显示cap12先于完整身份唯一性计算，有隐藏同尾owner导致虚假proved的代码风险，正在执行反例确认。
- B1579：满池128/256/1024后跳过后续同ID纠正，造成旧definition盖过已grounded call，已做真实关系入口先红，按“限新增ID、不限已入池身份纠正”修复。
- B1580：动态selector核心证据/调用证据先截断后检查候选唯一性，存在丢失cap外反证风险；尚未执行反例，保留待核，不宣称已复现。
- 优先闭合B1576/B1578两个精确能力合同矛盾及已复现的容量问题。随后两路异构读：日志+当前源码边界、Java调用链；不反复围绕同一Trace的模型措辞拟合。
