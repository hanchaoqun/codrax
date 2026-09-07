# r1026：动态选择关系与多仓写模式人工审计

- 快照：`fbc15596cd74`，干净构建；`PARALLEL=2 TIMEOUT=1800`，仅本次两路，没有第三路或失败重跑。
- 机器结果：2/2 PASS；**人工结果：write修复正确但生产验证粒度有限；read部分正确、关系表达与修补过程未闭环。**
- 原始结果、答案、图、计划、报告及oracle均未修改。

| case | 机器结果 | 人工判断 | 实际验证边界 |
|---|---|---|---|
| `sr_py_registry_dispatch` | PASS，160s | partial；主类正确，部分解释混淆，图过简，6次成文拒绝 | 3次read、2次repo_map、1次source lens；最大context48%；原图parse+render通过不代表关系完整 |
| `github_issue_memoclaw_text_search_multirepo_py` | PASS，129s（case内部126s） | 补丁pass；生产行为执行覆盖未证明 | 12次read、3次repo_map、3次list，context28%；原make check为AST/source检查，独立人工四格动态补验4/4通过，不回写产品凭证 |

## Read：答案、证据和六轮修补分别核验

工件：`.codrax/output/20260906-195339.469-98643.md`；日志：`eval/results/sr_py_registry_dispatch-20260906-195101/run-1.logs.all.log`。

- 主结论`kind=json → JsonPlugin`正确；正文保留注册发生在类定义/导入阶段、运行时查询与实例化、线程池callback，不把callback说成普通直接调用。
- 第3步把`cls`称为实例不准确：源码中`cls`是类，`cls()`才产生实例。注册工厂`register(name)`与返回的`bind(cls)`作用混叙。正文已经正确说未知键抛KeyError，源码也是重新抛KeyError，此点不另立错误。
- 最终图仅2个participant及调用/返回两箭头，没有注册、字典选择或callback路径。模型自行删除后的结果，不是系统替换答案或删图。原文通过本仓`internal/preview/assets/mermaid.min.js`的真实浏览器parse及render，生成20,173字节SVG；日志`.codrax/tmp/r1026-python-mermaid.json`。
- 第4轮不是系统执行attach后清锚：日志2880明确是模型提交4个`action=remove`。第5轮把`from_node`放在操作顶层，schema一直要求`edge`对象；第6轮按schema嵌套后成功。第7轮整块替换又遗漏已补齐identity，重新被拒，最终保留此前可用草稿。不能把这些都归为一项系统自冲突。
- 可确定的教学缺口：实际动态description及高频missing-anchor提示只说字段名，未给`diagram_edge_edits[i].edge.*`路径；参数schema正确。另attempt-only升级提示在问题类型已变化且上一修补已staged后，仍说SAME issue/previous fix无效/FINAL RETRY。登记B1584，仅修软提示准确性，不改变拒绝谓词、重试预算或答案。
- 完整候选入口日志1550为`candidates=0 rejected=2 reasons=ambiguous_lookup=2`。同一`pipeline/registry.py:31`赋值由模型与解析器两条合法载体表示，原发生点key可能因predicate/object表面形不同将其算作两个lookup。B1580完整索引忠实保留了这个原判据，不是新cap遗漏。登记B1585，必须用精确发生点及现有规范化关系证明去重，保留真实冲突，待可执行反例与独立否证；不以ID、相似度或同一行粗合并。

## Write：实际修改与证明强度分开

工件目录：`eval/results/github_issue_memoclaw_text_search_multirepo_py-20260906-195101`。

- 保留工作树HEAD`9af90b2f6c5761fb8736a91ba23c7e8103e01f31`，仅`memoclaw/client.py` **+8/-11**。两方法均从GET旧路由+querystring改成POST`/v1/search`+JSON body；保留签名、默认参数、namespace原判定及async的await。
- 测试/API文件字节不变；原Python仓仍seed`e384fcde`，只读TypeScript/API兄弟仓及原仓均干净。两份计划同ID且代码、合同、验收、probe字段相同，不把状态推进当replan。
- 生产`run_tests`只执行`make check`，46ms；原`tests/check_search_client.py`检查AST/source，不调用sync/async。零verification probe、零逐方法动态执行凭证。报告3/3是2个文件影响项+1个advisory，必需行为合同0，12个合同为planning-only；不能解释成12项行为均独立执行。
- 实际上下文日志3674披露`required_typed_contracts=0 / planning_only_contracts=12`及all_verified范围；最终交付说明自然语言验收不代表逐项独立执行。未见模型声称执行了本席的四格动态测试。
- 本席独立无网络mock transport补验：sync/真正awaited async × namespace缺省/提供 **4/4通过**；检查POST、URL、完整JSON及特殊字符、恰一次调用、返回值透传。它是人工审计证据，不是原产品报告的凭证。
- B1575无合格probe confidence而不发射限定是正确负臂，不是接线丢失；B1578无proof-followup，B1572无同ref证明债，均不能称本轮取得live正证。完整离线接线回归仍有效。

## 收账与优先级

1. B1584：共享JSON路径教学与中性重试进度提示，高ROI小批；不加硬门、不代写模型图。
2. B1585：同一操作多载体被当冲突的精确复现/根修，优先于继续磨Python措辞；先覆盖不同来源、owner、发生点及真正冲突负例。
3. 下一轮轮换Cangjie/ArkTS及C读写；既有Trace投影/自动补齐、JSON恢复、模型正文所有权与活跃流正负回归持续保留。
4. 逐义务/逐方法执行器拥有的运行凭证仍是独立能力债；不得拿模型自报、词法匹配或本席补验填回原机器报告。
