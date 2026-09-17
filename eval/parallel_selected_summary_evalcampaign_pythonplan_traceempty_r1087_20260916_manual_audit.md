# r1087 人工审计：Python 写计划与链外等待隔离

- date: 2026-09-17T04:47:48Z
- sweep_start_ts: 20260916-214747
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

机器两例 PASS；人工不等同整份答案通过。构建为清洁 `a4aab7344d0a`，版本 `0.1.20260917` / `2026-09-17T04:47:16Z`。CAP5、PARALLEL2、每例1200秒、原15步，各一次，无第三例或追绿重跑。按模式稀疏性（244例中3 plan）、历史新鲜度和链上/背景混淆风险排序；源码/case/fixture在运行期间不改。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | patch_python_typo | PASS | eval/results/patch_python_typo-20260916-214748 | write_plan,write_patch_oracle | none | 49s | 28 | read=2,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | partial：精准补丁通过，observed合同语义错 | 计划不等于已应用或行为验证 |
| 2 | trace_query_wakeup_background_demotion | PASS | eval/results/trace_query_wakeup_background_demotion-20260916-214748 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 174s | 45 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial：链上主因/背景边界通过，测量与解释有错 | B1717：背景排序7ms误称实际等待；实际19.5ms |

## Python：只交付计划，没有伪称已应用

- `run-1.plan.json` L2–18：`plan-1789620516282630000-54576` 仅把 `main.py:20` 的 `retrun` 改成 `return`；structured edit与生成diff一致，strip/world/CLI均保留。`batch-1` / `slice-001`，pending_approval，workflow只有一次plan attempt。
- 正式 `.codrax/plans/<plan-id>.final.json` L160–196：patch/verification为空、proof unknown、runner none；materialization为no_applied_mutations、owners=[]。这是正确的plan-only边界，不把dry-build或计划中的probe当成已执行行为证明。
- 4 dispatch、6次模型请求均首次attempt；读文件2次，emit plan1次，无工具拒绝、JSON修复、replan。Python dry-build真实PASS（合并日志L1145–1146），但import/greet probe未执行；最终run-1.out L44起称“提议的ChangePlan”和验收测试，未冒称修复已应用。
- 模型合同误读：日志L663的真实read_file包含retrun，L704原emit_write_analysis却写`pre-fix-typo-observed / not_contains retrun`；错误原样保留到计划L67–74及planner上下文L1003。它不在required map（L1009只有fix-typo-return），现有observed非required规则未变，probe未引用它且无expects_baseline_failure，所以本次没有由错合同获得baseline权限。记录模型/上下文语义partial，不猜未来apply结果，也不加原文词扫描纠正器。
- 原fixture与scratch main.py哈希相等；plan与plan-1相等。机器oracle只是kind=patch及加减行匹配，人工另外核对范围/所有权/无应用事实。

## Trace：投影和根因侧车在，背景测量有系统漏口

答案为 `.codrax/output/20260916-215041.073-54558.md`（438行），同名前缀HTML与root-causes.json；原结果不改。工具2次模型trace_query，另有系统root_cause_rank/critical_blocking_calls补采。没有read_file/repo_map/list_files，未见读取case/oracle/源码污染。上下文峰值45%。

### 正确且实际命中的能力

- 20ms明确窗口，app sleep20/running0/runnable0。真实链为 threadpool-400 → network-300 → cookie-200 → app-100，唤醒点2.016/2.018/2.020；threadpool IO为2.003–2.014共11ms，另有2.014–2.015 runnable1ms。logger D19.5ms（2.0005–2.020窗口截断）无链证，只能背景。
- 答案L48起保留Trace因果投影，头行原裁定词形与11ms主因、三个1ms低优先级供给候选保留。背景19.5ms未因更长而入根因榜。schema2侧车status=available，4项（11ms IO、三个1ms候选），精确窗口与候选限定存在，未出现valid_model_root_cause_selection_unavailable。
- B1659b空供给路径实际触发：最终typed runtime_work_relation_requested=true、frame_causality_requested=false；finalizer日志L2021明确无精确业务工作记录则省略未发布receipt，允许模型caveat。patch完成合法caveat归属，不需要伪观测，未丢IO/投影。但原问题只有调度/唤醒关系，true是模型过度分类：L530教学只允许独立semantic-work子问，L784模型自行选true；L789结构重试保留既有决定并允许主动重分类，并非系统自动强置。只签空供给通路命中，不签分类正确或真实业务span能力全测。
- 首次成文硬拒0；一次覆盖软建议后patch补principal/facet归属。多余schema_version被隔离而未重试；patch遗漏claim_uses降软提示，无格式死循环、无答案降级。日志请求实际600/300/600秒，显示持续语义输出时仍等待；不冒称真实等待10分钟验收。

### 确认的系统gap：B1717，排序封顶量冒充状态测量

- 答案L27、87、151、196、423给logger“7ms D-state”，另有19.5ms。不是两个独立等待；也不是perf_triage模型伪造7ms。
- 原生blob `trace-query-result-8942c5f4.json` 的同一background行来自off_cpu_sweep partition：d_state_ms=cumulative_impact_ms=19.5，impact_ms=projected_impact_ms=7，summary仍19.5。`backgroundImpactMs`使用20ms×0.35作为背景排序上限；原生测量未丢。
- `trace_query.go`发布Value直接取ImpactMs，影响notes/JSON/投影与finalizer事实卡。日志L2090把7ms称measured-in-stated-unit，同时自身row_state_breakdown=19.5；L2510–2511又提供7与19.5两条“沿用证据原口径”数据。模型确实被矛盾供给误导，不能只归波动。
- 独立修复批应仅将背景纯状态的原始测量与排序数值分开，在共享发布复制层统一JSON/notes/Observation/投影；原engine排序、链上可消收益、根因资格不动。未知类型、IO活动指数、设备延迟、缺原始状态不能拿Cumulative/包络或模型文字猜实际状态。施工与验证另记§123.1851，不回写本轮原答案。

### 模型解释错及既有展示债，不能靠硬门改正文

- L15将2.020000唤醒同时说成切入CPU；实际Running=2.020020，在所选窗外。L23把threadpool唤醒发生CPU4写为CPU3；日志L2513–2515已经正确区分事件CPU4与投递CPU3，系统事实对照L384也正确。属于准确供给未被忠实消费，不是渲染器改坏。
- L19将内核wait callsite解释成文件系统缓存页面就绪，接着又限定未证资源身份；应保callsite线索与推论界限。L27、365泄漏channel/schema/runtime_work_relation；模型有明确禁止枚举复述的软教学仍照搬，先保留观察，不加字词拒绝或系统替换。
- L41–45同状态重复、L308/309“无直接边”与同块上游唤醒点措辞冲突，以及大量中英文内部状态词是既有显示压缩/词面债；不因本轮无Mermaid而宣称全语言图/关系语法已验。
- perf预检曾误述IO对链无直接作用、睡眠/唤醒时刻，后续确定性查询提供正确链与11ms，最终主因已改正；预检推断不能作为后续原生测量的权威。B1717的7ms已独立定位到原生排序发布，不误归此预检。

## 收据与后续

运行后4816项Go/build、5项case/runner、1项fixture全一致；65项原结果与3项答案工件冻结于 `.codrax/tmp/20260916-r1087-{results,answer}-audit.sha`，不改机器PASS、不重跑。B1715已a4aab7344推送；B1716语法工具缺失假覆盖另列待修；本轮新B1717优先独立修复。Python合同错误、Trace时间/CPU/枚举解释保留人工partial，不冒称全系统无gap。
