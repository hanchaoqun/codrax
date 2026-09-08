# Selected Eval Manual Audit — r1042

- date: 2026-09-08T15:48:48Z
- sweep_start_ts: 20260908-084848
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | cangjie_repomap_fixture | PASS | eval/results/cangjie_repomap_fixture-20260908-084849 | dimension_substring,answer_contains | none | 86s | 28 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=1,inv=3/0,fin_reject=0,unavail=0,prune=0 | pass | 五项声明、位置、包名正确；0成文拒绝。但上下文仍把模型备注与已证声明整项混用，B1620有新witness，未因答案正确销账。 |
| 2 | github_issue_dateutil_relativedelta_float_symptom | PASS | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260908-084849 | write_apply,write_patch_oracle | none | 198s | 28 | read=5,repo_map=1,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 修复正确，原测试不改；系统真实执行1个动态probe及4个原生unittest。人工交付树原4项和23组补验均过，另留规划文字/修复提示观察。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 基线与选题

干净main@1290d1a73233，build=2026-09-08T15:37:20Z，固定快照`.codrax/tmp/codrax-selected-20260908-084848`，严格两路、单例1200s上限。该版本包括B1625/B1627a+b/B1623/B1628，不含随后施工的B1624c。case/oracle/历史答案不改。runner86/198s与内部wall84/195s是不同计时边界，不互换。

从243现有case（215读、25apply、3plan）按语言异构、读写轮换、近期覆盖老化及原生验证可用性选题：仓颉隔离fixture距上次三批；dateutil距上次九批，其原4项stdlib unittest本机可运行。没有因为机器FAIL反复单题追绿，也没有要求本题未请求的图。

## 仓颉：答案通过，字段权限问题继续开放

最终`.codrax/output/20260908-085013.102-82047.md`及同名HTML。独立源真值为：`extend Cart`（cart/Cart.cj:30/demo.cart）；foreign `native_add`（bridge/Bridge.cj:6/demo.bridge）；三个public class为App（main.cj:11/demo.app）、Cart（cart/Cart.cj:14/demo.cart）、Bridge（bridge/Bridge.cj:15/demo.bridge）。最终五项全部正确，Item struct和ohSum普通包装函数不混入。包均来自package声明，不从目录推断；extend没有被称为继承。

日志`run-1.logs/codrax-20260908-084850-000-82047.log`：2425一次合法结构化成文，4个block/5个精确row ID，2427–2430系统按ID补齐引用，2435持久化、2436接受；0次成文拒绝/0patch。三次调查完成891/1205/1548均被接受，不是成文重试。Analyzer先用不合本题的属性角色得到零成员，但结果如实为零，后续完整source_inventory自纠；未发现本轮JSON schema把合法答案同时必带必拒。无read_file，不能冒称本题覆盖B1628的真实读取→注释资格路径。

**B1620/P1新witness**：1205模型备注说“在Cart类定义体中…extend”，实际Cart类于28行结束，extend独立位于30行。2158/2285仍保旧错误备注，同时aggregate整项带current_source/principal/hard/independently_proven；一般note支持上限教学虽在2250/2253，本行仍缺字段级封顶。2262要求展示每条非空note，与2270–2272本题只请求name/location/package且声明不证明ownership/继承等关系的教学有张力。本次最终没有采用错误备注，是正确取舍，不是该系统问题已经关闭。最优下一批是拆开精确声明字段与模型说明的来源/支持上限、统一字段选择教学；保留有用说明，不删除模型答案、不扫描“继承”等词造门、不做仓颉特判。

## Python写：实际行为通过，规划文字不冒充测试

结果目录`eval/results/github_issue_dateutil_relativedelta_float_symptom-20260908-084849`。交付`run-1.applied-tree/relativedelta.py` commit=c51f681685bc，仅实现+15/-2。两个参数都先校验整数值浮点再转int，非整数浮点拒绝；`__radd__`及公开构造参数不改。原测试和README字节不改，run-1.repo原实现/测试SHA256仍等于fixture且HEAD保持5c02d7ec8ee2；运行生成的未跟踪.gitignore不能被描述成整个目录完全无变化。审批auto_safe/medium/auto_execute，修改保持在隔离worktree，没有自动合入主仓。

正式apply日志`run-1.logs/codrax-20260908-085128-000-83768.log:671–678`：实际1个动态probe（38ms）之后，以impact_related_test_surface继续执行`python3 -m unittest "test_relativedelta.py" -v`（51ms），原4项全绿，总test_results=5。suite_continued/syntax_preflight是审计行，不能把command_count=4说成4次真实行为命令。与r1032只正式跑probe不同，本次原生suite是真实执行。正式`plan-1788882688570480000-82043.report.json`与同计划final.json一致，post_apply_verify=passed、complete/verified/strong，4个原生assertion ID有实际assertion scope；最终明确自然语言清单不代表每条独立执行证明。

人工另在交付树以/usr/bin/python3 -B重跑原4unittest，并stdin执行23组边界（双字段正负整数浮点/零/与整数同值、分数拒绝、年/月组合、默认值、非date原TypeError），全部退出0；这两次补验仅在审计工具输出中，未另存日志文件，也未回写正式report，不能假冒系统原运行的验证范围。

重试/教学观察：write analysis log799–826曾把测试selector当文件路径被拒，下一轮修为已读`test_relativedelta.py`；错误提示又要求“read that exact file”，但当轮只开放emit_write_analysis。本次恢复成功，记P2工具能力与修复提示一致性观察，不能自动剥selector或拓权。ChangePlan有一次contract_refs超过10条被如实拒并缩减，未证合同冲突。模型有“divmod抛TypeError”（实际是后续date.replace）及两条delta+date倒向规划文字，正确源码和正式测试皆为date+delta；不因软文字错误造新的prose硬门或宣称无效实现。

## 后续优先级与红线

B1620字段权限/教学一致性优先；B1624c独立修复流式预览统计提示（本轮binary尚未包含）；B1624b派生阅读谱系须指向原已获准引用，不扩大escape许可；B1626多请求窗、B1622物理次数与统计分组、B1616b/B1561原生验证闭包继续开放。两例通过不销整套系统账。Trace链上根因、两轴、业务线索、显式窗/投影/补齐未改；结果只提供准确事实，不接管模型结论。活跃流不因4ms或旧4分钟无最终正文降级，真实停滞、取消及显式任务截止保持。
