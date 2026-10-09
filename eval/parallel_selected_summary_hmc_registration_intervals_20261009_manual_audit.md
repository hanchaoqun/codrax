# §216 原生区间导航与只读登记人工审计

冻结代码 `62cfa7ddb936`，批次 `20261009-044954`；两例各一次、并行2，runner 60809正式exit0。自然QUESTION未改，未追加第三例。机器 **2/2**，完整人工 **1/2**；旧失败不回写。

| case | 机器 | 完整人工 | runner耗时 | 结论 |
| --- | --- | --- | ---: | --- |
| trace_measurement_records | PASS | FAIL | 175s | 首轮正确选区间视图，原始13行齐全；最终手写预览子集、改错持续时间并猜GPU语义 |
| native_registration_commandless | PASS | PASS（普通源码修复） | 158s | 单文件修复、既存3断言及3探针真实执行、来源批独立verified；未命中新只读登记分支 |

## 量测：查询改善，完整事实未成为最终表格

[主答案](results/hmc_registration_intervals_20261009/trace_measurement_records-20261009-044955/run-1.primary.md)、[原日志](results/hmc_registration_intervals_20261009/trace_measurement_records-20261009-044955/run-1.logs/codrax-20261009-044957-000-44110.log)、[oracle](fixtures/hmosperf_measurements/README.md)。下列日志行号均对应单份原日志。

- 174–176：完整manifest的 `measure_interval→measurements` 导航真实到场；208紧凑语义保留filter/source_arg及值的SQLite类型。
- 1237–1238：首轮正确查询 `[1,2)`，另查 `[0,2)`；不再错选process/event_search。主结果 `trace-query-result-6e40d6cf.json` 为13匹配、0省略、1时间未知，保carry-in、7个显示分组、REAL/TEXT/BLOB/精确大整数、NULL与-1持续时间及右界排除。
- 2319–2333：explorer已复述完整原值和正确持续时间。工具Summary 23023字节实际完整投递；诊断日志的2000字节截断不能冒称模型未收到数据。
- 3045–3073：finalizer收到两组完整三表的选择入口。主窗summary为7行、members/timeline各13行，但每表固定仅预览首4行，总计只用24/128行预算；省略显式披露，完整原生表仍可按ID/view渲染。
- 3211–3216：模型不选任何 `runtime_measurement`，把Markdown表写进summary，再加caveat；被接受且没有后续修补。不是底层导出丢失，也不是renderer删行。

最终primary的明确错误：

1. 第7行称4种过滤器，漏30、999、40对应的大整数、BLOB、缺引用与歧义记录。总称13条却未完整交付。
2. 第20行把已知200ms持续时间写成 `dur=-1`；21因value=NULL省略已知终点；22把dur=NULL写成-1。没有完整保原0.9秒起点、时间未知记录及选择边界。
3. 11–13、25、29把未知单位转换为MHz、0/1及TEXT "1"解释为活动状态；38又断言至少2–3次GPU切换。3049–3050的typed边界已明确单位/状态/物理资源均未确认，不能称只是缺一句教学。
4. 第3行将0–2秒称完整捕获范围；第16行将主窗内仍存在的记录表述成额外全窗数据。附录的“不确定”并未撤回正文无依据结论。

系统剩余接缝按01.3/16.4/17.7/18.4留账：原生表与模型手写表的结构化交接、主窗/对照查询用途、覆盖范围与预览分配、错误closure/预分诊的语义权威。不要扫描答案关键词硬门、替模型写结论或无限加相同禁止句；优先利用已存在的来源绑定数据和精确结构。

另外两项上下文问题：815将明确1到2秒归为full_artifact，将频段说明归成function_or_purpose，产生多余源码软义务；1278等已有current_source_required噪音。主payload `Compactions=null` 且量测省略0，但通用 `traceQueryResultCompacted` 将元数据coverage摘要压缩说明误作查询结果截断，3097–3103重复建议缩到80–150ms。完整事实总体与元数据摘要容量必须分开；均挂原ID，不新建重复任务。

2445补查状态为families_present；本次是模型直接选对视图，新自动恢复路径没有live命中，公开正反/race通过不能冒称本例触发。未产生图，不能验收旧图FAIL。最终原生行完整与答案FAIL分别计账，不认定为已证偶发波动。

## Python：普通源码修复通过，不冒称登记分支命中

[用户输出](results/hmc_registration_intervals_20261009/native_registration_commandless-20261009-044955/run-1.out)、[原日志](results/hmc_registration_intervals_20261009/native_registration_commandless-20261009-044955/run-1.logs/codrax-20261009-044957-000-44123.log)、[实际交付](results/hmc_registration_intervals_20261009/native_registration_commandless-20261009-044955/run-1.applied-tree/packages/widget/widget.py)、[结构化执行摘要](results/hmc_registration_intervals_20261009/native_registration_commandless-20261009-044955/run-1.write-apply.json)。

普通CLI `--mode=write` 自动路径完成，不使用旧恢复harness。一个source plan、一个batch，唯一交付patch将 `return value` 改成 `return value + 1`；既有tests与setup.py逐字节保持，原seed HEAD不变。原日志2123/2125/2127/2189记录既有3个unittest断言与3个当前探针执行，负数/零/正数通过；根目录零测试记录未冒充有效测试。来源批自己的post_apply_verify及最终all_batches_verified齐全。输出76–82正确表述“6条验证结果”，未伪称6个原生测试。工作树没有自动合并到主仓。

本例 `native_test_registration` 缺省，没有补证批或跨进程登记恢复；因此只验收普通修复回归。§216新完整登记能力另由公开真实controller/工具/独立进程正控及负控验证，不能用这条自然live代签未触发分支。旧模型书写的suite/assertion标签仍与原生身份不符，没有被升级为新登记凭证；历史该类失败继续保留。

附带保留三项非本批新回归：scratch repo的既有EnsureCodraxGitignore追加 `.codrax/`，不属于模型交付patch；原日志556/558非runtime问题的not_applicable配置曾被交叉profile校验拒绝，模型后续自修，需另审教学与typed兼容，不能仅按模型解释认定误拒；final.json的probe_count=2实际来自置信条目长度，而probe_commands=3才是三次探针命令，不能把前者当探针数。没有虚假测试成功，不影响本例人工PASS，但元数据命名问题仍应记录。

## 计账及后续

79唯一ID仍19完整交付＋60开放，本批父项新增0；两组实现子能力为原生区间发现/保真交接及有界恢复、只读登记完整来源终态/原件安全保留，推送及统一回归收据见账本§216。5稳定验收父项与本次1份人工FAIL分列、重叠不相加。图空边/中文ID与inline/空白成功的旧公开RED仍P1；未覆盖的17.6真机、17.7持续写入/多来源/viewer、GPU生产者协议及跨语言登记不能代销。
