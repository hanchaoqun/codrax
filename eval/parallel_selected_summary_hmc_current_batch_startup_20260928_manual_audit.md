# HMC §187：当前写批次与启动名称来源人工审计

日期：2026-09-28（本地）。生产快照 `e3f78e4b56d2`，构建时间 `2026-09-29T02:59:09Z`；后续 `8d06ccc1e` 迁移兼容测试、`ace0601e7` 加固候选子进程fixture，均没有改动本次模型输入或生产实现。48682 固定2并行×1、正式exit0；机器0/2、完整人工0/2。没有第三次追跑，原机器FAIL不改签。

| 场景 | 自动结果 | 完整人工 | 实际命中与剩余缺陷 |
| --- | --- | --- | --- |
| `empty_python_module_apply` | FAIL，327秒，`verification_proof_incomplete` | FAIL | 当前proof批次、只读登记、完整读取与新执行均命中；登记类名与真实测试身份不匹配 |
| `trace_existing_sqlite_dictionary` | FAIL，247秒，8/5ms正则 | FAIL | 四段8/4/4/5ms正确，逐行名称状态完整送达；正文总数串窗、未知名称及实例关系仍误述 |

## 1. 写模式：当前批次投递已命中，完整凭证仍未闭合

结果目录 `eval/results/empty_python_module_apply-20260928-200009`。自然问题只要求实现空的 `totals.py`、不改既有测试/配置/依赖、运行测试并说明结果；没有告诉模型内部登记字段或类名。

- 实际落地仅 `def total(values): return sum(values)`；两份既有测试文件逐字节比较相同。新报告的四条Python unittest断言全部通过，语法预检通过，worktree audit clean。实现与真实测试成功不能替代“声明和断言身份相等”的证明。
- 最终计划 `plan-1790651100436691000-32523` 无源码修改、无verification_probes，含四个existing-test登记；登记前实际读取了当前实现和完整 `tests/test_totals.py`。这是新只读登记分支实际命中，不是只在测试中模拟。
- 日志4338–4355的实际planner消息只显示当前 `batch-1-cumulative-review-proof-probe-plan`、当前goal、`ready_to_plan`及 `verification_proof_followup`，不再把初始IR当下一批。4354的教学要求两种方案分开；首提交仍混合两种形状，4605–4606被精确schema拒绝，随后修复形状。不能为了这一拒绝放宽权限或扫描正文。
- 但新登记把 `assertion_suite` 写成 `unittest.TestCase`；新执行真实身份为 `tests.test_totals.TotalTest`。旧计划的 `unittest@tests::TotalTest` 同样不匹配。四条assertion_id本身均正确，未覆盖的强义务为 `empty-returns-zero`。最终精确证明门保留unverified，未将“测试全过”误当“本计划证明闭合”。报告passed=true与final unverified不是自相矛盾，描述的是不同事实。
- 最终用户输出明确“未完全验证”，没有假称全部验收；控制器尝试all_verified没有绕过证明门。完整人工FAIL针对任务仍未达到完整退出，不针对函数结果错误。
- 系统接缝：控制器3861–3868/4090–4097收到了精确native身份；本次planner初始消息没有 `Current native test identity snapshot`。源码已确认调用顺序为`runControllerPlanBatch`先准备私有登记授权，再调用`prepareControllerPlanningState`清空plan/report；`buildWriteNativeTestIdentitySnapshot`随后因缺当前plan而返回空。清空旧执行状态和拒绝任意历史pack是正确防护，但用于登记的来源身份需要独立、只读且绑定私有授权的展示通道，不能靠保留旧report让新计划复用旧PASS。下一回归必须经真实controller dispatch及恢复，而不是仅向planner测试手动塞plan/report；包括错误run/源计划/指纹、文件换代、撤销授权、不支持的runner、缺报告、过期执行及展示不授证明等负控。
- 另有原请求“目前空文件”仍影响首次规划判断（读取后已纠正）、同一proof义务多处重复并携 `verification_probe_required=true` 内部标记。保留为01.3/16.4/18.5投递与教学债，不据一次输出宣称纯模型波动，也不再添加案例专属提示。

## 2. Trace：源名称保留成功，作用域和语义表述仍失败

结果目录 `eval/results/trace_existing_sqlite_dictionary-20260928-200009`。沿用自然问题“1.000到1.080秒启动阶段与同窗系统事件两张表”，原问句和oracle未改。

- 最终上下文日志3126–3133有7份独立inventory、15个共享展示对象，成员零省略；窄窗13条（8个启动端点+5个系统事件）全到场，另有扩窗查出的0.980和1.090两条事件。每份query/coverage/row_refs独立保留，没有系统层相加人口。合法0引用的BOOTSTRAP、NULL引用、重复字典50的unresolved、LoadPreferences均有逐行typed状态；E端点也保源业务名状态。
- 最终四个阶段起止和耗时全部正确：1.010–1.018=8ms、1.022–1.026=4ms、1.030–1.034=4ms、1.040–1.045=5ms。5行系统事件表也完整，未知域/事件名均写未解析，没有再冒用物理载体名；单点BOOTSTRAP未混入启动阶段表。这些局部改善不能代替完整人工PASS。
- 自动8/5ms正则未接受表头“耗时(ms)”与单元格数字分开的合法表示，属于oracle误报；保留机器原FAIL，不能以修改断言回写成绩。人工仍有独立错误。
- 正文却说“同期系统事件7条、分别来自APP_LAUNCH”，与同窗5条及未知域不符；结尾又声称窗外两条“虽属于同一启动序列”，没有实例证据。四段包络35ms被称“总窗口耗时”，用户窗口实际80ms，不能代替阶段耗时或完整启动实例时长。
- 两个名称缺失的区间仍以“startup子阶段一/二”作业务名；附注把NULL/重复引用解释为“解析表未完整下发”，并猜测嵌套子阶段。实际证据只是源字段NULL与引用无法唯一解析，没有下发失败或阶段归属证据。缺失状态已送达，不再归因该字段丢失，但尚不足证明只是偶发模型波动。
- 过程有一次read_file读取生成的trace材料，不是源码；trace_query=14，repo_map/list/source_lens=0。一次event_types=`hisysevent`返回空，正确族为`hi_sysevent`，event_names=`print`又只得到3个可打印成员；typed事件族目录/预览与路由值得审计。pre-stage和explorer仍面对编码载体，出现把1010000000ns读成1.012秒等错误描述；最终typed数值没有此误算。后续应提供结构化预览、业务名/角色、精确用户窗与查询扩窗的不同用途，不要求模型从base64和原始预览自行恢复已知事实。
- Markdown/HTML产物均为两张表和文字，无关系图；只检查产物内容/结构，不声称截图视觉验收。root-causes旁路schema2、空列表、`trace_root_cause_contract_not_active`，没有把背景系统事件升级根因。

## 3. 原始证据摘要

| 文件（结果目录见上） | SHA-256 |
| --- | --- |
| write `run-1.out` | `0fedc87f2ef753b2b36e6f4fcec7f54bc06ee0f2bdef7276d8558c079d452f98` |
| write `run-1.logs.all.log` | `7bec8110a58b4dd86d58e08e2f7d6b5a68912681651436df94e8cf8022d45f92` |
| write `run-1.plan-2.json` | `78aed907ca12be4029d9fc4b1a2417dcac1665255b8afdd60c556f317181538c` |
| write `plan-1790651100436691000-32523.final.json` | `688036359e08c29e5ab112edfe0aabd145f7bd56b34f0d6a86986aa9f641b915` |
| dictionary `run-1.primary.md` | `410fbd7c977c14150c4b328229de6775cd3cf52185cb484e35dd84da94c571f0` |
| dictionary `run-1.logs.all.log` | `bf1bedccd731527a21598268dac4ebfa5b9fc2aaec18881955cb5e524bd3572f` |
| `.codrax/output/20260928-200414.022-13438.root-causes.json` | `5f8063f5882d6f6c5da9ef59d8ba0282f0fb2ef8c728bdcc77ef0249d2287e39` |

下一批优先做有授权/来源绑定的proof规划身份交接，以及04.3启动实例/主体身份能力；结构化预览、查询作用域投递、普通viewer的typed-only可见性边界分别保留原ID。稳定03.2/04.2/08.3/08.4/18.2五项验收父项仍开放，本批两份FAIL不另造重复任务。
