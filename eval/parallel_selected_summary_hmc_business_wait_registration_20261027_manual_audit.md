# HMC300 人工审计：OpenDocument 时序与只读登记恢复

日期：2026-10-27。两例进程均exit0；完整人工A FAIL、B有限合同PASS，合计1/2。唯一全仓96包PASS/1FAIL/12无测试，2顶层/10子例FAIL；候选不进main。

本批恰好A/B各一次，原问题与原限制不变。操作员oracle不注入模型；对原生payload、实际成文上下文、正文/表/图及持久化旁路分别核对。机器exit0不等于内容PASS，确定性绿不等于新分支自然触发。

## A：业务时序

材料为仓库作者构造的ftrace而非实机采集；执行是真实CLI/模型/公开工具。应有：OpenDocument `[1.000,1.050)`50ms业务范围、main S `[1.001,1.045)`44ms；worker LoadDocumentIndex业务40ms；RQ `[1.005,1.040]`请求驻留35ms；worker闭合S `[1.009010,1.040010)`31ms，存储完成线程80/TGID2在1.040010唤醒worker，worker就绪1ms后运行；worker1.045唤醒main，main就绪1ms后运行。worker1.0455之后尾段未闭合，不能借早先唤醒。后台TID900写请求47ms不在目标因果链。

实际A保住31ms闭合睡眠、44ms主线程睡眠、35ms请求驻留及两条唤醒事实，业务窗5ms运行与整窗7ms运行区分；没有把后台47ms作为主因。新增等待区间载体确实进入实际Finalizer并被正文复用。宽/窄因果impact同时出现的投影折叠修复自然覆盖须另查，不能由这些正确事实倒签。

完整答案FAIL：LoadDocumentIndex结束是1.044500，唤醒main是1.045000；正文却称“结束动作正是把主线程唤醒”，并把与1.050000的5.5ms差写成5ms。图将IO完成1.040000和唤醒1.040010合称“完成并唤醒”，在要求精确先后时混了两个事件。全窗其它CPU累计idle37.5ms不能排除特定ready1ms的抢占/竞争机制；正确状态表与量尺限制不能抵消这些解释越权。

原首次emit有3个英文节点和精确wake anchors；实际修补先补中文角色unproven边界，再尝试复用原节点、加中文业务label，被身份门拒绝；第三次patch改用中文node_id新增孤立声明获受理。最终MD/HTML源均6个participant：英文3个承载真实边，中文3个重复主体并被附“未证关系边界”。这是实际修补交接冲突，不简单归为renderer自动凭空注入，更不能扫描错句或放宽身份门。下一批先公开复现精确身份/可见标签/原生lane的交接，再判断泛化修法。HTML实际9表/1Mermaid/0静态SVG；一次浏览器启动因缺本地可执行失败，未加载页面，未下载/重试，不签像素QA。

实际11次trace_query与16项Finalizer前工具结果、6内容JSON不混计；新等待载体TRIGGERED，原HMC232所需PID200 rank/wakeup组合未调用，跨域fold自然NOT_TRIGGERED。实际Finalizer最高估计161905 tokens，原精确事实到场；调查4条解释单元未证投递，不将上游出现错文等同传播。双例实际上下文/原件名册与A主审、投影独审分别封存。

## B：只读登记与独立恢复

实际parent/controller/planner与独立恢复child调用真实模型；原源码/历史test报告准备与read exploration为operator/脚本，verifier使用公开RunTests不是verifier LLM。此例只验有限自然终态，不声称完整CLI或自然创作源码。须核源/测试/config/HEAD/diff不变、changes=[]、新invocation不同历史、WriteFinalReport引用真实原件；当前1断言/2命令与累计2报告/2测试行/4命令区分，历史unverified不得被覆盖。

B有限合同PASS，父PID58893、独立child66466，真实模型9请求＝父控制器5＋规划器2＋子控制器2。规划器完整读2/2行源码与4/4行测试后，提交changes=[]和精确assertion_ref；工具受理并未算测试完成，恢复子另选验证，当前invocation `native:103a2:18e25401bbc26738:1`与三代旧执行不同。当前1断言/2命令，真正WriteFinalReport累计2报告/2测试行/4命令，不是两个独立测试；3批占预算3/5。中间proof-repair旧attempt保持unverified，后继由精确证明依赖关闭，不覆写为passed。

父子各7/7原件SHA及bytes、各3/3验证引用、各2/2final→report链接核验通过；14份拷贝对应7材料，不算14测试。五快照均6逻辑pack、6 scoped IDs、6 artifact IDs，逻辑内容SHA一致；首次memory→durable规范化ID不同、后四份稳定，不误称五份ID全同。现场child审计核源/测试/HEAD/diff readonly，保留源码与测试SHA独立重算一致；临时源仓已被harness清理，不声称事后重开。

真正最终报告仍有source localization weak及apply_checkpoint_missing，继续原债；原缺来源等确定性负控本批已通过但未进入本自然正向例，不签自然负向覆盖。完整B原审计 `/private/tmp/codrax-hmc300-B-audit-v1.md` 随封存保留。

## 总体边界

实际模型请求交叠至少4.737秒：A当地00:57:51.536–56.929、B父00:57:52.192–57.279；B恢复子00:59:15.161–27.809也处于A一段真实请求中。不是只凭barrier或进程寿命判并行。A/B原receipt冻结前后均true。

冻结后源码、测试、注释和原执行helper不变，不追第三例/二次全仓；另两份新只读审计脚本只提取原件/尝试HTML检查，未参与被评测执行且不属于原冻结集合，分别封存，不冒称全部辅助脚本预先冻结。全仓新增`TestSelfRunningScopePublicNativeWindows`10子例FAIL，混宽窗/精确窗下原4项旁路变5或6项，不能当旧pin；另有`TestTraceNoteKeysEmittedSubsetOfRegistry`新soft_consumer真实发射夹具覆盖缺口。共享投影与候选身份交接优先、统一发射覆盖次之，图精确身份修补及正文解释继续留账。旧HMC299失败完整保留，采样解释和更宽IO/栈/代表/权限覆盖不回签。完整能力新增0、累计25，本批4组验收缺陷，54稳定任务不减；本批不进main，WAL仍生产NO-GO。冻结/测试/封存/发布终态见[批汇总](parallel_selected_summary_hmc_business_wait_registration_20261027.md)。
