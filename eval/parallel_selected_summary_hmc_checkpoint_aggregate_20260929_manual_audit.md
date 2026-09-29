# HMC §195 双例人工审计

固定生产/测试revision `9034f8142c05`，87295正式exit0；恰好2并行×1、每例1200秒总预算，没有第三例追绿。机器2/2 PASS，完整答案人工0/2 PASS。实现子能力与完整答案分开验收，不将本次机器PASS覆盖§194原始TIMEOUT/FAIL。

| case | 机器 | 完整人工 | 总时间 | 已验证与未通过部分 |
| --- | --- | --- | ---: | --- |
| trace_sqlite_checkpoint_records | PASS | FAIL | 129s | checkpoint-only接入、两条8ms记录及进程正确；额外系统事件的数量和名称角色误述。 |
| read_combo_command_current_source_explanation | PASS | FAIL | 276s | 389完整统计、聚合路由及及时完成通过；源码解释把分析预扫描辅助函数写成探索结果的后续答案链路。 |

## 1. Trace：无提交帧的已提交状态可见性

新自然问句只要求1.040–1.080秒窗口的启动记录名称、区间、耗时及进程，未要求用户理解WAL/checkpoint或照顾未提交数据。只附`eval/fixtures/hmosperf_checkpoint_wal/capture.data`，系统自动发现伴随日志；README/oracle/生成器不进入模型上下文。

真实SQLite夹具先提交两段8ms并checkpoint，再开启未提交事务：把第一段改成12ms，加入PendingOnly及大表溢写。独立SQLite只读连接只能看到已提交的两个8ms区间。输入main=61,440字节、WAL=943,512字节；回放后SHA保持main=`1baae92914352f6c55535415665aabba5cf90e0f792fcb774903870933bbcd5b`、WAL=`6a2755891fff2871b34d1f648a2f6023f778d775e3200ef00a97128c6bd4d8d6`。

准备收据`.codrax/trace-input-ab9a643ea70f92a1d493638355505e9b/preparation.json`明确CommitFrame=0、CheckpointOnly=true；私有镜像61,440字节、SHA=`3d7e3c502044604e79e2381017bea090110fc51a7674dafd1561a54a6ceca16a`。未提交page 1、新区间和大表均未泄漏。这证明稳定复制捕获的无提交WAL分支，不证明持续写入在线快照、WAL缺失或普通viewer兼容。

实际仅1次trace_query，范围1.04–1.08；零源码读取、repo_map、list_files。预分析预览只到0.980秒，但完整附件可查询的合同保留，后续正常查到所求窗；不能把有限预览当作全源截断。分析器第一次将不分析源码的推断写成非逐字用户引用，被现有精确校验拒绝，后续正常发射；没有放宽引用门。

实际finalizer完整日志1911行的display_rows有8个去重源行：两段启动的4端点和4条HiSys事件。启动分别LoadPreferences=1.040–1.048、RestoreTabs=1.064–1.072，均8ms、owner_pid=27599。四条系统事件分别为：

| 秒 | 源域 | 源事件名 | 另有内容字段 |
| ---: | --- | --- | --- |
| 1.046 | APP_LAUNCH | UI_READY | scene=document、status=ok |
| 1.050 | 未知/缺测 | STAGE_READY | stage=repair |
| 1.052 | APP_LAUNCH | 引用50未解析，不能从内容补名 | stage=draw |
| 1.058 | APP_LAUNCH | BOOTSTRAP | stage=bootstrap |

角色、名称状态、内容及完整纳秒区间已经供给，并非转换丢字段。Explorer交接曾把8行写成“两段+6事件”；终稿两段启动完全正确，但额外添加用户未要求的即时事件，写成“5条”而实际只列4条，又以draw/bootstrap内容值代替源事件名、未披露1.052的名称未解析。内部`codrax_process_interval`/`hi_sysevent`术语也增加阅读负担。完整答案FAIL，不能以所求主表正确抵消额外事实错误，也不让这份已供证误述独占下一能力批次。

本例没有图。根因旁路`.codrax/output/20260929-052231.404-68211.root-causes.json`为schema2空root_causes，原因trace_root_cause_contract_not_active；未将同期背景升为根因。最大上下文76,921/200,000（38%），2个探索轮次、1次完成尝试、0次完成拒绝；机器指标finalizer_reject=1、patch=2不直接等价于因果证据硬门拒绝。

## 2. 只读：路由修复通过，源码链路仍未通过

使用§194原问题、相同动态完整计数oracle，不改问题或判分追绿。实际命令与独立`find ... -type f ... ! -name '*_test.go'`均得到389；本批没有新增internal/tool生产文件。实际命令用`grep -v '_test.go'`而非严格后缀过滤且没有-type f，本仓结果相同，不将这种语法描述为任意仓均等价。

本轮分类仍给is_count_question/is_scalar_answer，但保留统计、递归口径、源码解释及边界的独立维度；没有生成活跃source_inventory_profile，旧枚举义务被聚合边界排除。只读工具可用，repo_map/source_inventory_lens均0，没有再要求输出389个文件或截成200项。第一轮分类的递归list_files被既有浅预扫描边界拒绝，随后发射；错误artifact_value_profile也按非运行时附件上下文被丢弃，这些警告没有被隐去。

276秒完成（管线约274秒），17个Explorer轮次、4次完成尝试、0次完成硬拒绝、12次read_file、7次中途提示；最大上下文63,167/200,000（32%）。上一轮是1201秒评测总预算TIMEOUT、48轮探索/12次完成尝试、最大109,252。这里只比较两次实际观察，不将单次模型差异全归因于代码，也不宣称获得稳定性能倍数。

完整答案FAIL的源码证据：

1. `internal/tool/builtin.go:677/692/1458`支持从stdout提取确定性整数并写入ToolResult.CommandMeasurement；389及此段解释正确。
2. 答案写“随后在prescan阶段”由`appendPrescanCommandMeasurementCorpus`注入后续证据。该函数在`internal/types/context.go:5996`只是序列化辅助函数，调用于5928的prescanToolResultCorpus；生产入口AppendPrescanToolResult的调用者仅`internal/agent/analyzer.go:1333`，且1822的isPrescanTool只允许repo_map/grep/list_files，不包括本例探索阶段执行的exec_command。辅助函数能序列化某字段，不能证明实际执行结果经过它。
3. 探索后的通用已实现载体路径在`internal/types/observation_ledger.go:2009/2131`，将CommandMeasurement转为带来源、命令、整数值的ObservationRecord。模型没有读此消费者或上面的真实调用者；最终也没有说明这条后续路径。12次读取集中在同几个定义/构造位置，未完成生产者→消费者关系取证，不能把答案“看起来有源码引用”视为调用链证明。
4. 第3个链路节点引用builtin.go:1449，但该处是VCS history分支；普通command_measurement构造实际在1459。这是独立错误锚点，不将“同文件邻近”当作精确证明。

供给审计：分析器未发current_source_explanation_profile，intent=return_value；现有测量机制软提示fallback要求mixed route+intent=explain等条件，本次实际消息没有该提示。不能声称“完整机制源码及专门提示都已给到，只是随机波动”。已给的文件发现摘要提到observation_ledger.go及结构关系查询方向，但没有被继续读取。后续应统一混合维度适用域、关系取证与结构化交接，而不是在提示中硬编码本仓正确函数链。

另外，日志4704行显示软数量提示把“7个链路节点”与独立scalar块389比较（expected_count=7、visible_count=389）；未阻断，但增加噪声。该问题挂01.3/16.4/18.4的多载体归属，下一步应依据精确facet/aggregate引用隔离，而非把提示升硬门或扫输出原文。一次metadata patch已按facet_ids补源码/路径归属，没有重写正确计数；这不证明机制解释因此正确。

## 3. 退出边界与后续优先级

- 本批可复用子能力2：聚合输入覆盖与答案成员分离；稳定checkpoint-only WAL默认只读接入。完整稳定任务仍16已交付/63开放，5个验收父项仍开放。
- 两份完整人工FAIL分别记入16.4/18.2与01.3/16.4/18.4/18.5。前者事实字段已供给，后者关系取证不完整及提示适用域未贯通；不统一标成模型偶发波动，不倒签旧失败。
- 下一能力轨优先17.7缺日志状态的来源权威与安全复用，之后按可独立交付收益选择03.3原生资源栈/04.5进程概览，避免连续只打磨启动事件措辞。缺陷轨优先混合维度/关系证据闭环和多载体数量归属，不新增案例专用函数名/事件名规则。
- 两例均为读模式，不冒充写模式验收。只读测试登记的完整CLI/controller收尾已在§192通过，不重复列为未交付；重启恢复和其他开放退出保留。600/300/600秒策略未改，实际日志首响应10分钟、静默5分钟；分析器另有既有3分钟terminal emit预算，评测另有1200秒总预算，不能混称流活跃4分钟即降级。

## 4. 原始凭证

结果目录为`eval/results/trace_sqlite_checkpoint_records-20260929-052025`与`eval/results/read_combo_command_current_source_explanation-20260929-052025`；完整日志分别为run-1.logs/codrax-20260929-052027-000-68211.log和68195.log。小型原判/metrics/wall与两份最终正文随批登记，完整日志、结构化输出和准备材料留本地并固定指纹。

| 工件 | SHA-256 |
| --- | --- |
| Trace完整日志 | `f75a579192763ec87d9b6e94071606f8a4d66b498f171eb356510a05c646e2bd` |
| Trace最终正文 | `ec0a7d5e451e3bc77042abfc4a63d4d545630d9f6ee535eddae094a4235b2ba3` |
| Trace准备收据 | `669851d1c7533ba3ab29e7e7b4a97a413e8e68020480cfcbf042b74ed176fb12` |
| Trace根因旁路 | `5f8063f5882d6f6c5da9ef59d8ba0282f0fb2ef8c728bdcc77ef0249d2287e39` |
| 只读完整日志 | `7258fe2feeaed177e7acb53290061aa71d28b9c8a09ea4ecc7c1fcfa87e6c49f` |
| 只读最终正文 | `b84eac8b00ee30956b2221f6e48ef6385e75109226bdd90e3bd4f2dd4f625e71` |
| runner日志/tmp/codrax-hmc195-eval.log | `64fcb46cfb67538a20e5224cfcfa08629f41e4a03b5ac851ee1ff218cb13af0d` |

全仓与推送正式收据见统一账本§195；机器summary保留原始2/2 PASS，不篡改成机器FAIL替代人工结论。
