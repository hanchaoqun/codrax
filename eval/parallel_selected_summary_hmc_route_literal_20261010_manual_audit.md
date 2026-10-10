# §222 固定双例完整人工审计

2026-10-10；冻结实现 `f203716ec013`（路由 `0f15c7166`），构建正式通过；snapshot `codrax-selected-20261010-004343`。两个自然问题各一次、并行2，无改问句、无第三例追绿。runner57826正式exit0。机器2/2不等于答案正确；主审与各领域独审核对完整transcript、工具过程、结构化输出和oracle，**完整人工0/2**。旧§220/221失败未覆盖或改签。

| 场景 | 机器 | 完整人工 | 本批可确认的子结果 |
| --- | --- | --- | --- |
| 双侧原生量测 | PASS，359秒 | FAIL | 正确route=repo，两个窗口自动修复保留；实际原生工具未开放，取数/成文失败 |
| 多源日志 | PASS，266秒 | FAIL | 两张原生表保全9个唯一记录；关联被升为确定因果，主文仍误述 |

## 双侧量测：路由修复命中，动态工具交接未通

结果目录：[trace_dual_measurement_records-20261010-004345](results/hmc_route_literal_20261010/trace_dual_measurement_records-20261010-004345)。[完整答案](results/hmc_route_literal_20261010/trace_dual_measurement_records-20261010-004345/run-1.answer-transcript.md)和[原日志](results/hmc_route_literal_20261010/trace_dual_measurement_records-20261010-004345/run-1.logs/codrax-20261010-004349-000-43324.log)已逐项审计；以下量测日志行号以此原文件为准，合并all.log多两行头。

- 日志43–44：分类thinking明确识别两个受支持schema的`native_reader=trace_query`候选，并选择repo/investigate。不是超时回退；这次支持原生非诊断路由子能力改善，不代表query-ready。
- 日志519–520首个analyze把两个窗错误拼成单窗且source_quote不实，校验拒收；547自动修复保留1–2与4–4.5两窗。仍未证明逐来源窗口绑定完整。
- 日志923–934先用read_file读取SQLite二进制形成每侧约16KB、合计约33KB噪声，后尝试shell sqlite被拒；trace_catalog对父目录授权拒绝不能称工具query对象schema错误，也不应扩大目录权限绕过。
- 日志1054–1063及后续共7次trace_query尝试均因不在当前16工具schema中拒绝，成功0。只有catalog/capability导航不等于量测。随后读邻近`current.sql`，没有源相等或生成关系凭证。
- 日志3426的最终任务另带`requested_result_kinds=[measurement,source_explanation]`，无用户源码解释目标。这是请求义务与原生输入交接的共享接缝，不按具体数据词修补。
- 首稿4028存在tool:30伪citation被拒，不把它当最终答案；最终稿本身仍错误，见下表。

机器门的现有限制：case中的`phase=toolcall .*tool=trace_query`只匹配调用尝试，执行前拒绝也会命中，因此原机器PASS不代表成功查询。保留本轮原始PASS，不事后重写runner结果；后继18.4验收需采用准确的成功结果及原生发布凭证，同时继续人工核对完整事实，不能只放宽/收紧答案关键词。

| 完整答案位置 | 实际问题 | oracle |
| --- | --- | --- |
| 11、17、27 | current称7条/3序列，vendor.sample称4条 | 8条/4个独立filter序列，vendor.sample仅1条 |
| 15、25 | 只展示filter10两条，漏carry-in零和负duration记录 | 3.9–4.1区间与窗口相交，选中4–4.1；4.45负duration的未知终点记录仍需保留 |
| 13–17、25–27 | 同名filter20不见；gpu_state的INTEGER 1和精确大整数不见 | filter20的750000000、state INTEGER 1、9007199254740995均为独立原值 |
| 19 | baseline13条全部缺失，声称工具不能读取 | 输入受支持；实际问题是动态工具清单没有开放，不是reader不支持 |
| 11 | 从current.sql反推current.data作为事实 | 相邻文件并非该二进制当前代次的数据证明 |
| 25–26 | 名称直接解释为GPU频率/状态，未披露语义/单位未知 | 序列名保留但不能自行授硬件、频率单位或状态码解释 |

NULL与TEXT `"0"`局部保留，未见跨源共同时间轴、差值或因果断言；不能代替全部原值/来源/区间交付。完整答案无系统原生量测表、无双侧状态表。输入文件未改动。10.1、17.7、16.4/18.4保持开放。

下一优先修复点：复用既有preflight，把命名内容候选经真实Coordinator准备后交给动态schema与executeTool；不能给候选直接授来源/窗口权限。现有直接调用TraceQuery的接续测试绕过动态schema，不能再作为该接缝验收。

## 日志：事实表完整，但正文违反因果边界

结果目录：[log_shared_sources-20261010-004345](results/hmc_route_literal_20261010/log_shared_sources-20261010-004345)。审计[完整答案](results/hmc_route_literal_20261010/log_shared_sources-20261010-004345/run-1.answer-transcript.md)，不能只看不含系统原生表的primary投影。

- 正文18–26列齐9个成员；原生表44–49为app6，67–69为kernel3，合计9个唯一记录。未知身份、原始消息/续行、malformed/orphan/unknown、精确boot纳秒、来源和代次53/73均保留。无第三张重复筛选表；这一默认展示子路径通过，不声称全答案通过。
- 28、30–34、38把共同`request=rx17`升级成确定“因果链”，写出触发内核查询、上游获知失败、资产缺失导致中止。36虽承认无校准，却仍称因果顺序来自标识符。原表51/71明确共同标识不证明因果；免责声明没有撤回主结论。不能把候选业务关联、相邻记录或背景变成根因链。
- 22将`99-09`错称无效年份，原格式中是无效月份；25无证称observer“并行”。38声称“5条链+其余3条”，漏算一条非结构化diagnostic，尽管前面9项/原生表均保留。
- 20/32给boot原始十进制秒但未注明单位。本次没有旧例把该值标成ns或把8ns说成8μs，不能把旧失败复制成此次命中。
- 本次`log_triager`在all.log275发出`errors=[]`、8条observations，**未实际触发Type来源资格分支**。新Type子能力只引用公开实际工具正反和race结果，不以此live验收倒签。
- all.log3008–3015实际接受4个模型块+2张原生表，3020要求维度修补；3055提交field patch，3058因`block_id_not_published`拒绝，随后使用第一稿结束。该修补对象交接问题留案，不与第一稿语义正确性混同，也不按错误正文词句加门。
- 未输出图，不声称视觉或图关系验收通过。无写模式场景，不补签旧写例/只读登记未命中分支。

## 结论与证据保存

仍为79唯一任务=20完整交付+59开放。路由教学和日志字段来源是本批子能力；双侧完整读取及最终人工答案未通过，不减开放数。当前高ROI是输入内容→准备→工具资格/请求义务交接，以及模型观察摘要/关系表述的准确证明范围；不围绕个别年份、单位词或sample名称做硬门。

本批全仓在`f203716ec`发现无可信Type/Message但有栈时帧事实丢失，属于真实回归；另有历史Type-only fixture及文案pin。完整全仓收齐后再修并冻结验证，不以本次live为后续补救版本的运行收据。首轮失败、相邻失败、原生continuation公开RED和测试装配失败分别保留在`results/hmc_route_literal_20261010/validation`；最终全仓/补救提交见统一审计账本§222后续。

最终收尾：原首轮另有提示词静态拼接审计失败，前次汇总漏收导致补救全仓再失败，已如实记录。栈补救`62aaa1899`与文案逐字不变的静态拼接修复`081d5c81d`之后，全部9个失败根测试定向通过，末版完整回归90测试包PASS、13无测试包、零FAIL，构建/race通过，四笔实现已推送。没有重跑live或修改本页两项人工FAIL；46份验证文件和95+28份工具载荷原字节保存。
