# r1086：卡顿清单 read 与 C++ 双头 apply 人工审计

审计日期：2026-09-16（UTC 2026-09-17）。机评原表保持不变；独立后验不回填正式验证报告、不倒签原 case PASS。

## 范围与排序

- 244 cases：216 read、25 apply、3 plan。按新需求/修复验证价值、模式稀疏性、真实验证能力、最近覆盖排序，选 jank 清单 read15 与 C++ apply24；Python plan 为下一备选，未启动。
- 基线 `48ef7affe7dc`，清洁构建 `0.1.20260917 / 2026-09-17T03:12:27Z`。03:12:53Z 同时启动，03:15:47Z 完成；CAP5/PARALLEL2/TIMEOUT1200，恰好两例，各一次，没有第三例或追绿重跑。
- 启动前冻结 Go/build4800、fixtures8、case/runner5项；完成后冻结两 result 目录（排除临时 run-1.repo）及读模式三份输出。后验只在独立临时目录编译，不改 fixture、oracle、交付或原结果。
- 审后复核4800＋8＋5＋33结果文件＋3答案文件共4849项SHA全部一致；收据为 `.codrax/tmp/20260916-{b1659b-build,r1086-fixtures,r1086-cases,r1086-results,r1086-answer}-after-audit.log`。原机评未重写。

| Case | 机评 | 人工结论 | 过程 |
|---|---|---|---|
| trace_query_jank_field_inventory | PASS | partial：核心清单通过，来源/身份/措辞仍有错 | 174s；ctx32%；trace_query5；成文硬拒0；软提示后patch1 |
| github_issue_nlohmann_long_double_symptom | FAIL：unverified/proof_weak | partial：补丁和本机行为通过，正式证明仍不足 | 131s；ctx28%；read5、repo_map3；不可用工具2；正式make check通过 |

## 1. 卡顿清单：交接获得实际正证，不能据此宣称整份答案无误

结果目录：`eval/results/trace_query_jank_field_inventory-20260916-201253`。主日志 `run-1.logs/codrax-20260916-201255-000-99444.log`；终稿 `.codrax/output/20260916-201544.658-99444.md`（同名HTML与root-causes.json）。

### 核心需求通过

终稿L15、21–29正确给出3条、按7/4/2帧降序、头行时间5.040000/5.050000/5.010000、六个超过2^53的原始起止整数、70/40/20ms；70,000,000ns=70ms换算正确。1帧、appid621、包含相似子串的 `other jank_event_sync`、坏数字均未冒充匹配项。L33–41未把卡顿标记直接升格为根因，仍要求时钟域映射、身份和调度因果证据，也没有重复要求UTC日历映射。

这为B1713的类型化解析/数值过滤及B1714的查询绑定清单交接提供一轮live正证。r1085的错数/错序原失败不改（当时三条成员齐全，但总数写4、顺序2/4/7，另有成文供给缺失）；本轮仍是一次样本，不承诺模型始终正确。

### 模型实际上下文足以支持正确回答

- 日志L3195–3205有独立 `Trace Event Search Inventories`。筛选query自带完整3行；较宽appid query自带4行（含1帧）；子串干扰query单列1行且没有typed jank_event。教学要求按各query身份分别计算，不能混池。
- L3202/3205三行包含原始文本、精确起止整数、派生duration、来源行与头行时间，以及 `emitter_tid=101 / emitter_tgid=101 / marker_pid=201 / appid=620`。这些是不同身份轴。物化附件行4/10/12对应原fixture行3/9/11，不应据差一行误判来源漂移。
- 同处caveats明确坏字段1条排除、非法/缺失不是零、原生时钟域未证；应用字段过滤不得继承发出线程为应用目标。L3276另有用户措辞教学。
- explorer曾有错误临时计数/换算；L3301已标这些为模型暂定总结，不是事实权威。最终正确总数来自模型消费后续查询证据，不是系统改写正文。

### 仍开放的解释层债

1. 终稿L25把 `reported_duration_ns=70000000` 称为“记录中自带”。原打点只有四字段，时长由解析器根据end-start派生；这是来源表述错误。
2. L35将 `time_domain_status=unverified`、`trace_seconds` 当作记录字段，泄漏系统资格枚举并混用内部词。应解释为“标记时钟尚未与调度时间轴建立映射”等业务边界。
3. L37的 `writer-101 (TID=101, PID=201)` 将marker PID写成发出线程进程身份；正确emitter TGID为101。
4. 未披露一条坏字段记录被数值过滤排除。有效清单仍为3，不能把披露缺失重记成清单缺供。

这些属于成文的身份/来源/展示质量问题；上下文已供给正确信息。单轮不足以认定纯随机波动，亦不能为追绿新增正文关键词硬门或系统改写答案。后续异构回放继续观察，再按可执行证据选择泛化修向。

### count提示不是新的矛盾硬合同

日志L3358–3374只有一次展示软提示，随后模型加“匹配条数”标题，3条与清单保留。count可用scalar，也可用可见标签完成展示；不是只认scalar的硬门。模型说“没有add_blocks”与无关系租约路径的代码/公共测试不符；scalar在L3100–3105也明确可选。日志没有完整wire参数schema，故只作代码/测试与教学交叉核验，不冒称直接看过本轮完整schema。未确认新增权限冲突，不开重复修复项。

### 不在本轮覆盖范围

最终分析为bounded_fact_set，runtime_work_relation_requested=false / frame_causality_requested=false。因此B1659b空业务工作关系出口live为N/A，不得签绿；没有Trace根因投影也不是投影丢失。必有侧车存在：schema_version2、root_causes空、status unavailable、reason `trace_root_cause_contract_not_active`，准确说明未启动根因合同。无图、无JSON降级，本轮不能验证Mermaid或完整因果链。600/300/600s请求默认在日志中存在，模型两次成文约21s；活跃流长等待保护由本批单测验证，不冒称本轮有长静默live证据。

## 2. C++：业务补丁正确，正式验证保持不充分

结果目录：`eval/results/github_issue_nlohmann_long_double_symptom-20260916-201253`。主日志 `run-1.logs/codrax-20260916-201255-000-99454.log`；应用/验证日志 `run-1.logs/codrax-20260916-201430-000-99535.log`。计划 `plan-1789614870729244000-99454`，交付 `run-1.applied-tree`。

### 补丁与交付归属

两份发布头各一行 `%.*lg → %.*Lg`，未通过窄化转换消除警告，也未削弱测试。唯一owner `4cccfd7` 直接继承seed `95fdb3`；五个交付文件与owner commit精确一致。scratch HEAD/tree仍为seed，交付来自对应隔离工作树，未混入宿主源码。

### 原始失败与独立本机后验

预跑原测试及2352格矩阵严格编译均RED，原因均为两头L10格式参数类型不符，非SDK环境故障。交付原测试严格编译/运行通过；C、de_DE.UTF-8、fr_FR.UTF-8各2352项，共 **7056/7056**。本机clang21/SDK26.5/arm64的double与long double均8字节、53位精度；不冒称验证扩展精度平台。薄fixture只有long double wrapper，矩阵中的double实参也经过该签名转换，不能扩张为真实库独立 `%g` serializer已执行。

收据：`.codrax/tmp/20260916-r1086-nlohmann-audit.Ft8q2y` 与 `20260916-r1086-nlohmann-delivery.hSEURh`。后者status.tsv、ownership-status.tsv、owner-diff.patch、三locale日志及输入SHA保留。30项后验输入前后不变，原baseline7项也未变。这是人工独立验收，不回填成自动系统已观察到的凭证。

### 正式proof_weak并非错误拒绝

- 应用日志L703–707：make check确实编译并运行，945ms、exit0。report.json L4–12只有aggregate/check/make-test；L35–57两生产路径covered，但capability unknown。不能把reason名称 `production_verification_source_static_only` 误译成本轮没执行或只做静态检查。
- plan.json L66–79的 `long_double_format / main → outcome-1..5` 是模型声明，不是runner观察到五个断言。七合同均planning_only_ungrounded；零unresolved_items不等于生产行为证明完整。Make适配器未提供结构化assertion协议，未知能力没有被悄悄签绿。
- 应用日志L827–838已供给low_confidence、unknown及“不得仅凭passed选all_verified”。模型仍于L932提交all_verified，L943正确降为accept_unverified。最终用户输出区分“一条测试通过”与“未完全验证”。机器FAIL因此保留。
- planner先尝试用于C++的Go静态探针，语言门准确拒绝；随后移除该探针，没有执行不兼容检查。5次读取预算后两次额外read_file被拒，未见互相矛盾的硬合同。

保留既有 **B1561/P1原生C/C++精确行为凭证能力债**，以及unknown使用source_static_only reason的命名债；不将已有边界重复记成新P0。模型对独立double路径未受影响的表述过宽另记展示边界，补丁本身不含该过宽修改。

## 收口与后续

本批系统修改仅B1659b，已提交推送 `48ef7affe7dc`；公共红绿、count3、race3、冷审、无缓存全仓86个有测试包全部通过。r1086未触发该空供给请求，因此只有确定性验收，不写生产已闭环。B1714清单交接获得核心live正证，但身份/时钟/派生值解释债仍开放；B1561继续按跨语言验证能力补齐评估，不降低proof门。

下一批优先考虑尚未覆盖的plan模式与实际触发空供给的异构read；仍先冻结输入、恰好两例、各一次。Trace显式窗、链上根因/链外背景边界、自动补齐、活动流等待均不改；禁止为本轮措辞问题增加raw prose硬门或代写结论。
