# 共享事实交接双例人工审计（§218）

- date: 2026-10-10T03:10:55Z
- sweep_start_ts: 20261009-201052
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_shared_fact_handoff_20261009

机器通过2/2，完整人工通过0/2。主代理与独立审计代理核对原始fixture、完整工具JSON、实际finalizer消息和最终正文。两例各一次并行2，使用冻结源码`b6c00f4d1`构建；不追加第三例，不改写原答案或§216/217失败。事实已完整交接与最终答案正确是两种不同验收结果。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | log_shared_sources | PASS | eval/results/hmc_shared_fact_handoff_20261009/log_shared_sources-20261009-201055 | log_regex,log_attachment | log_triage+log_query | 222s | 30 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | FAIL | 原生9记录已到场；最终身份/级别误归属、跨时钟排序及无证失败范围 |
| 2 | trace_measurement_records | PASS | eval/results/hmc_shared_fact_handoff_20261009/trace_measurement_records-20261009-201055 | log_regex,trace_attachment | perf_triage+trace_query | 237s | 46 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | FAIL | 原生33行已到场；最终擅自推导单位、GPU状态，仍受旧错误摘要影响 |

## 1. 日志：来源/字段交付通过，最终关系与身份未通过

以下路径均相对`results/hmc_shared_fact_handoff_20261009/log_shared_sources-20261009-201055/`。

- Oracle为`eval/fixtures/hmosperf_log_sources/README.md`：两来源9逻辑记录/11物理行；内核not_found无PID/TID/comm，不可借邻近记录补身份。
- 完整载荷`log-query-result-0e58aeff.json`为app的6记录/7行，`log-query-result-7db11e5a.json`为kernel的3记录/4行；SHA与原件一致，精确boot纳秒`9007199254740993`保留。载荷原目录`.codrax/blob/20261009-201057-000-41829/`，归档后同名字节保留。
- `run-1.logs/codrax-20261009-201057-000-41829.log:2508–2546`的实际最终上下文含全部9记录、完整身份/null/状态/原时间及来源元数据。本轮不再是§217的kernel整源遗漏或JSON前缀截断。较早`:725–751`显示系统用原件修正模型行坐标。
- `run-1.primary.md:25`却把not_found归给81/82/storage并沿用level4，原件该条为level3无actor；同答案`:17`又说无PID/TID，内部矛盾。level4标为INFO也错误。
- `run-1.primary.md:9–12`声称按wall升序，却把`.002`的lookup放在`.001`的begin前；未知时间/坏日期也混入排序。末尾时钟边界声明不能撤销前面的错误排序。
- `run-1.primary.md:7、33`无依据断言`per_item_rejection`、唯一失效点、其他请求/线程未受影响。日志只能显示missing/aborted，不证明其余请求的状态。

系统诱导链路：analyzer日志`:897`错误设置`error_granularity`和`per_item_rejection`，最终上下文`:2434–2443`把它变成必选决策，而用户仅问事件、执行者与关系；`:2584`的主答案support-lane指导与`:2607–2616`仍主要使用旧triage错误/帧的lane也不一致。高ROI后继为分类—契约来源校验及原生事实与模型摘要的分层；不能再记作输入缺行，也不能仅凭一次失败断定模型偶发波动。

终稿已列全9记录并分开orphan/unknown，属于子能力进步；原始物理行引用和精确boot值未在终稿展示，最终逐行引用/时间展示不签通过。

## 2. 量测：全部小表交接通过，最终单位与状态未通过

以下路径均相对`results/hmc_shared_fact_handoff_20261009/trace_measurement_records-20261009-201055/`。

- Oracle为`eval/fixtures/hmosperf_measurements/README.md`：主窗`[1,2)`，13选中记录/7组；右端2秒记录排除、1条NULL时间记录无法定位。全部单位未知；同名但不同filter不合并，state码无已知含义。
- 完整载荷`.codrax/blob/20261009-201057-000-41828/trace-query-result-9bae8582.json`保持13记录、原区间/窗内交集、单位未知及1条无法定位记录。
- `run-1.logs/codrax-20261009-201057-000-41828.log:3117、3122、3126`三份原生JSON可解析，分别7组、13原始成员和13窗内交集，预览省略均0；`:3130`确认33/128行。`:3118–3129`准确提供未知单位/身份、原区间与交集及未定位数量。
- `run-1.primary.md:3`写成0~2秒、4已识别+3未知，后文却用1~2秒；实际5组唯一解析、2组身份未确认。`:11–18`擅加Hz/MHz；其中科学计数值`3.342000005e+08`与原值`334200000.5`等值，不能把这一合法表达误记为精度错误（独审纠正了审计草稿的算术误判）。
- `run-1.primary.md:26`推算百分比，`:37、47`直接断言GPU曾运行于334.2~800MHz；`:49`承认单位与状态只是推测，不能消除前述确定性结论。仅有名字gpufreq不能证明单位或与GPU工作状态的对应关系。
- 原0.9~1.2秒区间在终稿未同时展示窗内1~1.2秒交集；右边界与NULL时间排除未披露。13行/7组、NULL不补0、文本"1"和整数1区分、大整数、BLOB及未知/无效时长则有保留。

系统噪音：同一实际消息`:3149`的模型closure仍带0~2秒、4.8GHz误码和GPU实际频段，`:3179`重复Hz/频段结论，`:3235`计划标签也仍0~2秒；正确原生33行与这些摘要并存，最终上下文约72737tokens。`:3281`最终emit只手写summary+caveat，没有用原生`runtime_measurement`表，`:3286`被接受。后继应以精确source/query/window/结构支持协调原生与模型摘要，保留调查方向及未决问题，减少冲突副本；不扫描用户/答案词汇硬拦单位。

## 3. 台账与后继

本批没有完整父任务达到退出条件：维持20已交付、59开放。来源定位、完整字段、预算补位/小表完整展示是16.4/18.4已交付子能力，不等于完整日志或量测答案验收通过。新确认的错误分类升权、旧模型摘要与原生事实重复冲突，以及最终字段/单位/时序归属纳入同一P1后继；跨源cause occurrence仍挂05.3/16.4。失败原件、工具结果、真实消息和正文均保留。只有确定性兼容回归修复后重新做相关及全仓测试，不以重跑模型得到偶然PASS替换本轮FAIL。
