# r1044 Rust读 / C++写人工审计

- date: 2026-09-09T03:36:10Z
- sweep_start_ts: 20260908-203610
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

基线 `main=cf5dc859694a`（B1630c已推送），clean binary built `2026-09-09T03:35:26Z`。全仓86包测试通过；运行前无未提交Go输入。243个case（215读、25 apply、3 plan）按客户风险、异构覆盖和老化选择本批；相同组合上次r1034，间隔9批。严格两路，无第三例live，无case/oracle/fixture更改。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_nlohmann_long_double_symptom | PASS | eval/results/github_issue_nlohmann_long_double_symptom-20260908-203610 | write_apply,write_patch_oracle | none | 119s | 28 | read=7,repo_map=2,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=2,prune=0 | pass within tested platform | 双头修为%.*Lg；真实树严格原生编译+原测试+通用格式对照均过；不外推其他ABI或未逐条执行的自然语言验收 |
| 1 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260908-203610 | answer_regex | none | 146s | 43 | read=6,repo_map=3,list=0,trace=0,source_lens=0 | midloop=4,inv=3/0,fin_reject=2,unavail=0,prune=0 | fail | 主路径/职责正确，但多写了不存在的regex能力，图中返回与递归顺序错；没有确认系统矛盾硬门 |

## Rust：正确主链不能掩盖额外错误

最终工件：`.codrax/output/20260908-203834.987-67421.{md,html}`；日志：该result目录 `run-1.logs/codrax-20260908-203611-000-67421.log`。根席及独立席均读最终MD、实际source和修补过程。

- main→run，run先collect_files、返回后逐文件index_file，再逐行is_match的核心解释正确；walker只收集路径、过滤隐藏/target、递归目录，不参与内容读取/匹配。r1034的“并行”错误本次未复现。
- MD第20行称RegexLikeMatcher支持“跨行连续匹配和分组提取”，不受源码支持。`matcher.rs`只按`.*`拆分后顺序find，调用端又按content.lines逐行传入，没有跨行或捕获组实现。该句在模型首次成文payload3620及预览日志3646已在场，不是系统替换正文；入模3297只给“用find分段匹配”的候选说明及支持上限，3070–3077明确dispatch候选不能直接画成调用。暂记模型过度推断，不加识别本句/本语言的硬门或自动代写。
- 图第46–47行先递归walk，后read_dir返回，顺序与同步调用不符；主循环源码的先后关系本身清楚。关系证据证明端点/调用，不自动证明模型排列的时间顺序。模型选择保留两个孤立匹配器participant的信息量弱，但不能据此编造dispatch边。
- 首次成文拒绝（日志3633–3706）含8条standalone关系行缺from_identity/to_identity，以及图中两个把接口/实现关系当直接调用的边、递归误用reply箭头。提供了精确关系备选和合法局部修补；模型在3741选择补关系身份、删除两条图边并把递归改为call，系统按模型选择执行。
- 第二次拒绝（3745–3754）是上述删边后两个孤立participant需要明确处置。模型3775选retain_as_context并提供visible_label，现合同只要求visible_label，不存在被吞掉的context_note（`emit_answer_document_patch.go`描述/schema/实际hint已核）。最终两个label完整保留，不成立“系统丢注释”新gap。
- 3775的participant patch把unchanged_block_ids编码为JSON数组字符串；当前机械兼容保留原IDs，原生数组教学已在场。最后3848–3863只补概念终点绑定，先前内容及修补保留。MD18的详细函数行为还引用main.rs23调用行，源码29–30已读取并进入支持上下文，属模型选证范围偏窄。没有发现同字段同时必带/必拒，亦未再出现whole_replace_not_authorized。本批不改模型图/正文，不因有重试便裁为系统缺陷；继续保通用修补心智和图时序表达观察。

## C++：正式验收与独立审计分开

已读实际 `plan-1788925061374364000-67427.report.json`：正式系统只执行`make check`，exit0、1386ms；一个test_result、`observation_scope=aggregate`。两头文件changed-path覆盖为project_runner，不能据此认定每条自然语言验收都有独立assertion。交付说明也明确披露此边界，B1616b/B1561不因本次PASS销账。

实际过程还包含两个planner read_file失败（日志2025/2028）及一次emit_change_plan不支持C++ probe的schema拒绝（2070）；runner的fin_reject=0不代表全流程零拒绝。planner教学1723–1725已明确C/C++走原生项目验证、不用受限inline probe；模型随后删除probe再提交，不是同一合同既要求又禁止C++ probe。原有5条自然语言验收不自动等于5次执行。

独立审计对象为实际durable worktree：`run-1.repo/.codrax/worktrees/trace-1788925062014746000-67577`，对应`refs/codrax/applied/plan-1788925061374364000-67427`。两份发布头均修为`%.*Lg`，未降告警/改测试或将long double强转double。

原fixture非空输出测试偏弱；本次独立席以本机clang++、`-std=c++17 -Wall -Wextra -Wformat -Werror`在真实交付树上编译执行原tests，exit0；再执行既有`.codrax/tmp/r1034-nlohmann-format-audit.cpp`，两头均与`%.*Lg`参考相符：

| 输入 | 参考/普通头/single头 | 保持通用格式 |
|---|---|---|
| 1.25L | 1.25 / 1.25 / 1.25 | 是 |
| 1e12L | 1e+12 / 1e+12 / 1e+12 | 是 |
| 1e-12L | 1e-12 / 1e-12 / 1e-12 | 是 |

原生验证和对照日志位于`.codrax/tmp/r1044-nlohmann-delivery.lJmeUQ/{native-tests,general-audit}.log`，均exit0。未运行会写固定/tmp路径的fixture Makefile、未修改fixture/test/oracle/交付树、未将额外人工验证回填正式report。r1034的`%Lf`语义回退本次未复现。本机Apple arm64下long double精度等同double，此矩阵不证明其他ABI的扩展精度；也不存在足以单独验证float/double重载的fixture实现，不外推。

## 队列与边界

B1630c在本批前已完成真实查询/发布/成文入口与全仓回归；本批无Trace，不冒充其客户live验证。后续优先B1631来源隔离的真实入口复现与共源关联（缺来源不伪造），再B1624b派生结果纯导航谱系及其余未结债。Rust两项回答错误本次只记人工fail/模型观察，优先级不挤掉有静态构造路径的跨捕获关联问题。

Trace精确窗/因果投影/自动补齐/链上占用与可消除双轴未改；背景不升根因，模型拥有正文/结论/图。活跃流5族+agent预算专项已复验，不能因4ms或旧4m无最终正文降级，真正停滞/显式deadline/取消仍在。所有机器结果保原判，不靠改oracle提升成绩。
