# §206 原生CPU区间与只读测试登记：人工审计

日期：2026-10-07。产品冻结 `f6c2e27bd2510f09f39947fbc3809e1042d90132`，构建 revision `f6c2e27bd251-dirty` 的 dirty 仅为架构文档。恰好两例同时在途、各一次，无第三例：CPU 为实际 CLI；只读登记为真实模型 planner + 默认持久化 + 独立进程执行，controller 的 verify/finish 决策为脚本，不冒充完整 CLI/controller 验收。

| 场景 | 自动 | 人工 | 耗时 | 已验证范围 | 未通过/未覆盖 |
|---|---|---|---:|---|---|
| `trace_cpu_native_intervals` | PASS | **FAIL** | 180s（runner；pipeline 177s） | 普通 `.data` SQLite 默认只读接入、显式前40ms原生区间查询、负起点/洞/尾裁剪、准确统计与实际最终上下文 | 最终明细重复/遗漏、列名和单位缺失、整体分布与时序遗漏，不能签完整答案 |
| `TestNativeRegistrationLiveRestoredFollowup` | PASS | **限定范围 PASS** | 测试14.53s/包15.504s | 真实读取→只读登记→默认store→独立进程→新执行→原行为合同补证 | 全CLI/真实controller、原来源终态和多框架组合未覆盖；原整体workflow仍unverified |

机器2/2、按各自声明范围人工1/2；完整用户答案仍有1份本批FAIL。79个稳定父任务仍16已交付、63开放，五验收父项不变。不得把限定范围的登记PASS解释为整个18.5完成，也不重签§203/204/205的原始FAIL。

## 1. CPU：数据与上下文通过，成文失败

结果目录：[原始结果](results/hmc_native_sql_cpu_20261007/trace_cpu_native_intervals-20261007-233918/summary.md)。自然问题只要求前40ms每核组合、时序、占比、整体分布和覆盖率，不给模型独立oracle或内部约束清单。

独立oracle：窗口 `[0,0.040)` 秒，40ms墙钟、两个观测核、80 CPU-ms；联合已知35/未知45 CPU-ms（43.75%/56.25%）。CPU0：状态已知20、频率35、联合15、未知25ms；CPU1：40、20、20、20ms。CPU0五段为码0×1GHz 10ms、未知×1GHz 10ms、码1×未知5ms、码1×2GHz 5ms、未知×2GHz 10ms；CPU1为码2×未知20ms、码2×800MHz 20ms。源状态码的物理含义未经证明，不命名为Running/C状态。

实际日志 `run-1.logs/codrax-20261007-233920-000-44476.log:2505` 起的最终上下文完整列出80/35/45、每核全部组合、七个有端点区间、单位和未省略数量。2537附近的query_scope正确为用户窗与实际窗均0..40ms，不再出现单查询“实际范围未知”的矛盾。两个公开查询原件保存在结果目录 `blobs/`；未通过修改输入追绿。

过程：4次trace_query（两次cpu_state_frequency、两次window_stats），0源码读取/目录扫描，2次调查完成、0次调查完成拒绝；成文一次缺summary拒绝、一次局部patch，0JSON字符串恢复，最大上下文41%。本例没有§205的22次源码operation降级；旧例535秒与本例180秒并非同一输入/对照实验，不据此声称普遍提速比例。预诊断自行解读编码数据的错误未获得最终统计权威，最终使用确定性查询。

人工FAIL证据见[最终正文](results/hmc_native_sql_cpu_20261007/trace_cpu_native_intervals-20261007-233918/run-1.primary.md)：

- CPU0最终有6行、50ms/125%，与同文5组合/40ms矛盾；重复码0×1GHz和未知×2GHz，漏未知×1GHz。
- 第一表为“列1…列5”，时长和比例列没有单位；正文又说“频率单位kHz”而表格已是GHz/MHz。
- 缺整体80/35/45及43.75%/56.25%分布，缺区间端点；“数据完整性：完整，无省略”混淆展示无省略与真实缺测。
- 频率具体换算1GHz/2GHz/800MHz、每核覆盖率及状态码含义未核实正确；没有把这些统计当响应根因。

模型原始 `emit_answer_document` JSON（日志2675、归档 `blobs/tool-call_function_4m4c9uajo1u6_1-emit_answer_document-params-be4e582d.json`）本来就有8行且第一表没有columns：不能说系统丢了已有表头或凭空添加错误数据。首稿预览只显示7行、最终8行的原因已确证为非TTY相邻相同行去重；`728aa2efc`在live后修复内容批次无损输出，状态notice继续去重。成功emit会清除恢复attachment，此处不是JSON恢复/patch加行。原FAIL不倒签、不按数值或问答关键词硬改模型结论。通用表结构/证据绑定优先于CPU特例提示，已有RuntimeMeasurementContract可让模型仅选择已发布observation_id/view，不必再发明逐格JSON或代写模型表。

## 2. 只读登记：新执行与原合同补证通过，不补造来源终态

目录：[receipt](results/hmc_native_sql_cpu_20261007/native_registration_restored/receipt.json)、[evaluation](results/hmc_native_sql_cpu_20261007/native_registration_restored/evaluation.json)、[原始stdout](results/hmc_native_sql_cpu_20261007/native_registration_restored/go-test.stdout.log)。自然问题：“实现已修好，请运行保留的现有测试，确认返回值正确。不要再修改源码、测试、配置或依赖。”

独立人工复核：

1. 日志 `logs/codrax-20261007-233925-000-44867.log:194–200` 实际读取test_value.py全部4行；204再次进入模型，233才单次登记；236–237工具明确changes=0且登记不是测试通过。
2. 新进程PID45788、父PID44867；旧invocation `native:af43:18dc7a9ccc41bd50:1`，新 `native:b2dc:18dc7aa011915138:1`。实际测试重新执行，未复用旧PASS；保留HEAD `7333cd8e1c4e530bcedb1141a124baa804130da8`。
3. final-report绑定原source plan、提交、登记digest、测试字节SHA和实际执行索引；原 `increment-result` 合同得到assertion级见证，proof batch为verified。保留测试SHA `ec7d5c…7864` 与计划、报告一致，源码/测试/配置和HEAD均未改。
4. workflow:53–57只把新proof标verified；17–20整体仍 `unverified/missing_terminal_verify_verdict`。旧fixture来源缺最终verdict不能由新测试倒签。恢复记录在359–360，默认store已被新进程读取并继续执行。

限定：controller仍是 `scripted_verify_then_finish`、`full_cli_run=false`；既有 `persistence_degraded/no_durable_store` 是fixture挂store前留下的历史sticky标记，不代表本次恢复失败。模型登记摘要提前称“verified witness”，工具明确纠正且typed门未受影响，保留低ROI措辞观察，不把它当执行证据、不重跑求绿。

## 3. 留项与证据保留

高ROI后续：16.4通用数据表结构/证据身份与拒稿恢复一致性；08.5/17.7无独立采集边界的纯measure输入不能用起点包络代表全采集；逐物理来源/簇/线程明细；18.5完整CLI/controller及来源终态。旧HiSys ID、关系/图、优先级反转和D/IO业务说明等未通过项均保留。目录＋精确源码混合场景的explorer软operation教学仍有冗余，正确硬门不倒退，不作为本批阻断。

原始日志、receipt和FAIL未改；本次相关测试首轮分配回归FAIL、旧教学词针FAIL及修后结果均在 `results/hmc_native_sql_cpu_20261007/validation/`，完整回归与发布收据见统一账本§206。

| 原始证据 | SHA-256 |
|---|---|
| CPU最终正文 | `c4ef536cb20e7fbe5af2e4a6ecd739166bd4fc373005f51475d3aa483fc4f5bd` |
| CPU原始完整日志 | `a9201dc0f31aa2f6cf6128880128259603e2c45904d2c5118ff0f669c7f2b91b` |
| 只读登记go test stdout | `dc340f908b3bc755373e824ae3b25318f3b08eb67f9d90011003b6265d706d31` |
