# HMC247：恢复入口与原生事实交接双例

日期：2026-10-15。起点 `5c272a607`；真实评测冻结 `67bb965e0`，同一生产 binary SHA256 `40fd305a376ede1c07be1d5ef7c7faea879b060aee7b57df18694e11a5672269`。两例原自然问题各一次并行，未追加第三例。原始失败保持，不用确定性测试或退出码替代人工验收。

| 案例 | 输入与问题 | 机器结果 | 人工结果 |
| --- | --- | --- | --- |
| A：sample-only热点 | 完整30,062,585字节原件；原`trace_binary_sample_only.case`，486489.4–486489.6秒、poll_report/tid1609热点及停顿解释 | CLI exit0；原smoke PASS，161秒 | FAIL：unknown类型仍被写成CPU执行；权重占比误作样本占比 |
| B：独立进程恢复 | 原两份构造SQLite；自然问题引用checkpoint，要求两文件×线程101/202、10.000–10.050秒分配/释放/调用位置和本轮状态 | CLI exit0；真实resume已触发；精确index审计exit1 | FAIL：两条线程101计划未执行，已查询原生事件正文漏失；完整11.1未交付 |

## 已确认进展与未通过边界

A默认完整原件准备12000记录，窗口1393，目标164记录/259 cycles权重；两个热点各82记录，权重174/85，CPU1/0正确。实际finalizer完整收到热点首末点、记录占比、质量及“未确立（样本类型未知）”，28/128原生事实行、0省略。模型未使用新增可选`native_fact_handoff`；不能由此推断工具schema没有提供字段。旧typed规则已抑制当前综合reason进入finalizer，不能将终稿错误简单归咎于旧reason再次污染。正文在充分原生事实与明确负资格下仍越权，保留人工FAIL；本批没有证明它只是偶发波动。

B完整checkpoint读取发布历史导航，模型确实调用`trace_catalog resume`成功：4本轮计划全部not_executed、0本轮attempt、1历史批次。未恢复旧成功资格，入口较§246的NOT_TRIGGERED前进。随后没有使用返回的`catalog_query_ref`，普通查询为两个全文件查询和两次线程202查询；最终6计划/4attempt、3success+1empty+2not_executed，原线程101两计划仍待执行。原始checkpoint保持1历史success/3未执行，不能将本轮catalog状态称作checkpoint已经写回。

模型曾称工具不可用，但resume后实际工具数16→17，第二dispatch同样17工具成功查询；**不能据模型自述就签“工具仍隐藏”**。本批可复现的交接风险是旧不可用回执未随真实工具集合变化说明其时效，后续修复须只读取精确集合，不扫描模型原文或扩大权限。完整资源数据已进入当前结果，终稿却称只有schema、没有事件与调用位置，独立保留事实复用缺口。

后置修复单独提交：`0fde0ff21`以真实当前工具名称集合的变化提供临时时效提示，不改准入、不清掉旧失败；`7daeb6e17`将既有广义“采样=执行上下文”教学改为记录位置/栈，保unknown/off_cpu/on_cpu各自边界和合法链上支持。两处均有实际agent适配器公开RED→GREEN及相邻回归；不是已保存真实答案的修正版本，也不能据此将两例改签PASS。新原生事实分离合同本例未被选择，仍单列未触发。

## 准备、验证与证据

- A独立零模型oracle正式exit0/23.018秒；其内部preflight测试binary与生产binary分开记录。原件SHA `da713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632`，未用预转换文本替代attachment。
- B零模型operator只执行一项历史查询后退出，新模型进程不继承旧Mutable。生产binary `/exit`预检正式exit0、无模型请求。构造SQLite不冒充实机采集或自然模型中断。
- 真实A session69492、B session71892正式exit0；B从12:40:35.511779Z至12:45:07.828535Z。精确实际`resume`返回的saved_index由独立helper核验，session25568正式exit1：`current catalog differs from the requested input roster`。
- 原生统计/Trace因果投影仍保留，没有可用链上根因；本例无模型图，不代签所有图修复。未触发自然query失败后修参重试，不追加该路径的成功声明。
- 全仓收据、后置修复、提交与归档计数以[中央账本§247](../docs/design/hmosperf_capability_gap_audit_20260917.md)为准，不以中间PASS代正式退出。
- 后置版本末版全仓正式exit0：93包PASS/13SKIP、0失败事件；6944输入、binary和完整原件冻结复核通过。原全仓v1的catalog Outcome漏登记FAIL保留；后置binary与原live不同，没有第三例或重跑，不能代签真实答案。

原始证据根：`eval/results/hmc_resume_handoff_20261015/`（本地忽略产物，不声称已推远端）。完整人工过程与后续范围见[人工审计](parallel_selected_summary_hmc_resume_handoff_20261015_manual_audit.md)。
