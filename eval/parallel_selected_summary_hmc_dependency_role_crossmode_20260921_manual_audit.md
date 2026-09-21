# f29750f58 Trace / JavaScript 双例人工审计

- date: 2026-09-21T08:48:13Z
- sweep_start_ts: 20260921-014813
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_dependency_role_crossmode_20260921

固定干净二进制f29750f5862c（buildTime 2026-09-21T08:47:33Z），2并行×1，runner session10387正式exit0。机器0/2、完整人工0/2；运行器退出0表示跑批完成，不表示案例通过。无第三例追绿，不回写旧FAIL。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_dayjs_duration_nan_symptom | FAIL | eval/results/hmc_dependency_role_crossmode_20260921/github_issue_dayjs_duration_nan_symptom-20260921-014813 | write_apply,answer_regex | none | 136s | 28 | read=6,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail（行为验证未完成） | 修复源码与保测试正确；缺npm，静态检查通过不等于JS断言执行通过，诚实unverified |
| 1 | trace_query_wakeup_background_demotion | FAIL | eval/results/hmc_dependency_role_crossmode_20260921/trace_query_wakeup_background_demotion-20260921-014813 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 234s | 46 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 机器仅词序regex未命中；本片修复命中，但模型无凭证合计3ms，系统扩窗背景借主窗分母 |

## Trace：修复命中与未闭环项分账

以下行号相对Trace结果目录。`run-1.verdict:1`仅要求IO相关词在前、threadpool-400在后；`run-1.principal.md:13`实际明确threadpool、11ms IO及fscache调用点，但词序相反。记为oracle词面局限，原机器FAIL不改；不是IO主因遗漏。

实际主日志`run-1.logs/codrax-20260921-014815-000-1926.log:2236,2664–2665`已投递中性依赖阶段/最终机理上限及独立Binder/IO例外。`run-1.answer-transcript.md:333–349`的两个邻近明细均“不计入链上影响”，且保准确上游唤醒点。二者有live见证，不再靠单元测试代签。

4次模型查询外另有2次系统补采：探索先取2.000–2.025（日志1327–1328），1554–1568按请求范围补采，2627–2643最终供给明确2.000–2.020及20ms sleep/0运行/0runnable、cookie17ms/network14ms及定位窗非连续状态段。最终正文未再把依赖窗写成连续sleep起止；唤醒pool→network 2.016、network→cookie 2.018、cookie→app 2.020准确。正文“随后切入运行”未给错时间，但没有明确2.020020的sched-in在窗外，记精度披露不足而非本次阻断项。

**人工阻断1：** principal21将三个1ms候选合计为3ms。实际输入2646–2659明确缺关系/折叠授权，只能分别列值；系统图91–94也写不可直加。不能从三个数值相等或相近定位范围推物理互斥/可加。本次是收到正确输入后的模型误用，未证明只是随机波动，不以新散文硬门修补。

**人工阻断2：** transcript156–158将扩窗logger20ms绘成100%及“整窗等待”，372称疑似空闲；主树仍以20ms请求窗为尺（58/62）。原生`.codrax/blob/20260921-014815-000-1926/trace-query-result-9b6ab2aa.json:2222–2270`声明查询2.000–2.025、状态2.0005–2.0205。请求窗内19.5ms同时正确存在（transcript159/243/494，正文亦正确）。`runtimeTraceProjWholeWindowIdleRow`只比较值与主树窗长，不检查行的查询身份；这是确定系统显示缺口，下一批优先修分母/标签，不能裁原值或升背景为根因。

Harmony来源的优先级语义正确；正文保候选/未证持锁边界，fscache保调用点未推设备；无新模型控制枚举泄漏。正常text图、零finalizer拒绝；不存在空答案或活跃流被短总时长切断的本例迹象，但不据此签所有超时情形。

必选`.codrax/output/20260921-015205.360-1926.root-causes.json`为schema2/available，仅threadpool IO一项0.011s，requested/query均2.000–2.020，未混入logger或三个未证反转候选。描述21的“第一跳”与24的“目标链第3级”观察方向未说明，记措辞歧义，不是坐标/排名错误。

## JavaScript：真实应用成功，缺运行器不能签行为通过

唯一差异`src/duration.js:15`改为`value != null ? Number(value) : 0`；完整树对照确认tests、Makefile、package及其余源文件不变。应用提交`6b65d310798d98de63b95c2b64841e1878cdffa0`；原fixture仓HEAD仍`8057c36333698365814dcb5631f3cab1d9e6091c`，`run-1.materialization.json`为resolved，非主仓自动合并。

实际report的executed_commands记录`make check` exit0（70ms），它执行Python源码形状检查；随后`npm test --` exit127，node语法预检也未启动。主日志2166–2189及report明确runner_missing，final.json:8–12为unverified，run-1.out:152–166向用户披露。没有把静态PASS当JS运行成功，也没有因环境缺失重改已正确源码；本机未找到Node/npm，未安装依赖或回写历史报告追绿。

上下文：日志1024/1121已正确教未修改测试只需PTO引用，不要求测试改动。模型的3条PTO（plan.json:44–75）使用自行起名的assertion_id；原始JS是顶层assert脚本，不包含这些具名suite/assertion，当前npm_script_exit_status也不提供逐断言身份。缺npm是本轮直接失败原因，不能声称安装后这些绑定即可成为证明。全部行为合同因未携精确来源在日志795降为planning-only（plan.json:77起），不代销B2–B6或旧required合同失败。

一次workflow双重编码且内层畸形JSON在日志939被精确拒绝，下一次原生对象恢复；提示已要求native object且保全部字段，未确认教学自相矛盾。规划仅一次emit_change_plan，无成文重试；普通读取failure_rounds=0，不能据此宣称§71新失败轮计费降低了本例重试。

## 后续排序

优先修精确可复现的跨查询背景分母/整窗标签；B2原生只读补证授权按源快照、批次/合同身份推进。模型违反已给不可加规则、JS缺环境与无原生assertion身份、oracle词序局限分别留账；不扩权、不降低验证门。HMC13/79已交付、66开放不变。
