# r1051 Java / C++ 人工审计

- date: 2026-09-10T08:59:17Z
- sweep_start_ts: 20260910-015917
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

代码基线 `f4bb3aee835c9b7ed377410568e7b041b99c80b1`，干净构建后快照 `.codrax/tmp/codrax-selected-20260910-015917`。08:59:17Z 两例同时开始，09:05:46Z 全部结束。机器 PASS 不等于人工全过；未修改问题、oracle、provider、模型预算或答案。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_java_config_precedence | PASS | eval/results/sr_java_config_precedence-20260910-015917 | answer_regex | none | 126s | 28 | read=4,repo_map=2,list=0,trace=0,source_lens=1 | midloop=3,inv=2/1,fin_reject=0,unavail=0,prune=0 | pass | 20→配置50→环境变量，未虚构运行时值；一轮JSON错误恢复；错误引用advisory另立B1642 |
| 2 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260910-015917 | answer_regex,answer_contains | none | 389s | 40 | read=6,repo_map=1,list=0,trace=0,source_lens=0 | midloop=7,inv=2/0,fin_reject=7,unavail=0,prune=0 | fail（局部精度/图） | 主链正确且答案保留；返回静态类型/默认flush不精确，最终残图弱；两轮alias冲突立B1641 |

## Java：核心回答通过，过程问题分账

终稿 `.codrax/output/20260910-020122.415-36204.md`；实际log `run-1.logs/codrax-20260910-015919-000-36204.log`。

1. 已完整核 `ClinicConfig.java`、`application.properties` 和 `VisitService.java`：默认20，资源属性50覆盖，非空环境变量最后覆盖；未观测环境变量实际值不冒充已设置或未设置。终稿表格与引用正确，首轮成文接受。非法非空环境变量会抛异常而不是回退；题目未专问异常，终稿也没声称全失败回退，不为该省略添加硬门。
2. Explorer一次把config/runtime填入错误枚举轴，随后按已有schema改正；一次把 `aggregate_facts` 编成带数组外残片的JSON字符串（log1274–1278），下一轮省略可选聚合并保留结论，7条grounded evidence继续供成文。不能只截取前面合法数组而静默丢掉后面的模型成员，也不能猜造括号关系。已有安全字符串数组恢复仍保留。本次不是最终答案消失。
3. 通用Completion Handoff仍教“value只能数字”和“prose-only summaries用scalar_value”（log856），与当前schema明确允许分类结果字符串、叙述放reason冲突。这不是本次畸形JSON的已证原因，但确为独立通用教学gap，记B1640。
4. log2169把正确 `application.properties:1` 的50引用软提示改到 `ClinicConfig.java:21` 的getProperty(key)。模型首稿部分claim metadata有误，但复核发现即使各form改正确，同表多role、同key候选仍会first-match抢位；两条校验车道共用错误匹配。记B1642，不把正确literal来源改成lookup。旧EVAL-B36-CITATION1的反向修复仍有效，B1606只修view可选form不涵盖此行级歧义。
5. 终稿出现的 `lookup` 英文是模型原 `caveat-1`（log2164），不是系统改写；作为表达质量观察保留，不全文替换、不重跑同题追词面。

## C++：主路径正确，未达人工全绿

终稿 `.codrax/output/20260910-020544.382-36202.md`；实际log `run-1.logs/codrax-20260910-015919-000-36202.log`。模型读到完整两header、registry和logger（log869–910、964、1072），不是缺文件；峰值上下文40%，不是上下文耗尽。

1. 正确保留初始化按kind选择ConsoleSink、Logger持有Sink、log经虚调用写stderr及换行；没有采用README过期的log→format_value关系，也没有把unknown返回nullptr说成抛错。
2. 工厂返回表达式构造的是ConsoleSink，但签名返回 `unique_ptr<Sink>`；终稿把传回调用方的类型写成 `unique_ptr<ConsoleSink>`。ConsoleSink无flush override，实际继承Sink空实现，终稿只说由具体sink决定策略。这两处记模型精度残差，源码已读且不以样例字符串硬补正文。
3. 8轮成文、7次patch。第1轮有缺return声明、虚构调用、reply误当call等真实结构错；第2轮同failure_ref重复操作；第5轮已暂存更改待关联边处理。第7轮思考声称6次attach，但log3684实际只交remove/boundary，故同六条identity错误仍在是正确；第8轮log3743真正6次attach后3753接受，未见系统再次丢失已交字段。
4. 第3–4轮（log3473–3477、3515–3519）raw新ID `Logger.log` 带显式label `Logger::log`，schema对新ID允许，执行器却先归一成现ID `Logger` 后要求label也为Logger。这是schema/executor不一致见证，记B1641，后续以任意语言方法/actor及新ID明确标签复现，不只加教学掩盖系统自动合并。
5. 最终可解析的时序图仅写入、条件flush及继承边；App/Registry/CStd孤立，已证具体输出只在旁边列表保留。图非必需，不以“少图”改硬门；但继承当时序消息及残图不能算完整优质解释。统一节点/关系/展示选择审计继续，禁止系统补造连线。

## 下一批排序与边界

先收B1640教学小批、B1641精确节点冲突；B1642引用行消歧独立排队。随后 `github_issue_tokenizers_newline_run_multirepo_py`（写模式、最近r1035）+ `real_trace_h3_iofam_one_seat`（显式窗D/IO、最近r1013）exact2；每例1200s，默认步数/oracle不变，人工核持久化写结果和不同IO统计域。此处仅计划，未声称运行。Trace主体、显式窗、自动补采和链上资格未因本轮read评测改变。
