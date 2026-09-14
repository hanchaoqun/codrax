# r1066 人工审计：显式窗 Trace 与 C++ 写模式

- date: 2026-09-14T02:37:54Z
- sweep_start_ts: 20260913-193754
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

基线 `32b7c70e7fb1`，提交后清洁构建于 2026-09-14T02:37:28Z。原 case/oracle、read15/apply24/CAP5/1200s；并行2、各一次，无第三例或重刷。机器0/2不等于两个最终代码/答案都相同性质地失败；以下独立审计保留原机器裁定，不回填正式 proof。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h6_channel_mixed_display | FAIL | eval/results/real_trace_h6_channel_mixed_display-20260913-193754 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 189s | 63 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 旧glyph oracle失败；正文错误合计/集合和IO证明解释不被投影正确抵销；图语法通过，关系语义另审；空旁路为选择格式拒绝 |
| 2 | github_issue_fmt_tm_year_overflow_symptom | FAIL | eval/results/github_issue_fmt_tm_year_overflow_symptom-20260913-193754 | write_apply,answer_regex | none | 263s | 29 | read=8,repo_map=4,list=0,trace=0,source_lens=2 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | code-pass / proof-incomplete | 最终源码及原测试、独立8016UBSan输入通过；正式累计义务未闭，不能冒称all_verified |

## 1. Trace：数值证据与模型结论分开

最终文件 `.codrax/output/20260913-194101.304-52663.md`（120002B）、同名HTML（3242892B）和root-causes.json（130B）。完整日志为结果目录 `run-1.logs.all.log`。

- 原机器失败仅 `missing:根因排序#1 missing:❶`。保留旧显示字形oracle失败，归B1622维护债；正文另有真错，不能以旧oracle过时替它签绿。
- 显式窗13762.791708..13763.024898（233.190ms）五次探索查询均带目标及时间窗；自动补采另有一份。投影一份（MD120起），四态157.248/5.604/70.338/0ms、running折算58.320ms、IO墙钟12.658ms及链上/背景分界保留。实际占用/可消除两轴、绘制/提交fence/traversal/measure业务族和语义工作边界仍在（MD88起），不能因模型摘要未深入解释就说系统没有供给。
- MD15、84把IO两个席12.658+3.602写成16.260ms方向leader，系统总览MD158仍为最大单席12.658ms。锁与优先级12.115ms本身是两个互斥席7.405+4.710的有效小计（MD161），模型MD64却称四席合计，扩张了该值的成员集合；不能误审成12.115这个值自身没有证据。
- MD15把65次总唤醒都归给IRQ、声称直接阻塞仅Binder1.409ms，与自身后文MD58已有IO完成闭合4次≥4.384ms冲突。D/IO调度标记为零不能否定独立IO完成证据。以上都是模型首稿已经出现的解释/数值错误，patch没有被系统重写正文。
- 模型序列图不是严格树：重复APP→T统计边，跨不同发生段的依赖画成连续时间序列，没有各事件时间边界。原文经仓内 `internal/preview/assets/mermaid.min.js` 真parse/render成功，1张sequence、SVG31919B，记录 `.codrax/tmp/20260913-r1066-h6-mermaid-render.json`；语法修复成功不授予时序真实性。未自动改向/删边来替模型回答。
- B1669来源自然正证：log912–916、1759–1763只有真实donghu.ftrace一项，载体attached_trace+attachment，不再把格式hint拆成第二文件；Perf预处理因尺寸跳过，不能销Perf绑定分支live。log1253–1262自动补采frame_root_cause_bundle耗时2.704s。附注MD1071显示单来源单窗状态账户，B1618此可见分支有正证，多源/多窗仍未自然覆盖。
- **B1670/P1上下文范围确认**：log2852尾卡只根据projection waiter/holder是否存在，却指示“该目标没有已建立的typed direct blocker”。同一上下文2472–2477、2494已有完成闭合IO与独立Binder正证。源码 `answer_document_final_decision_boundary.go:1732`只读投影字段，缺持有者对象不等于缺全部闭合等待证明；应缩窄否定范围。属于软指导过宽，不是已证必带/必拒硬合同，也不能断言它单独导致模型错误。独立小批修订，不扫描或修改模型正文。

## 2. 空根因旁路：本例是 rejected，不是客户的 omitted

- 本例log3033：模型提供5个候选，但把 `trace_root_causes` 写成数组，`schema_version:"2"`放在答案外层。3034外层未知字段被隔离；3035选择器对象解析拒绝（要求 `{schema_version,root_causes:[{candidate_id}]}`），其余正文接受。
- log3057保留可选字段失败诊断和候选表；3060–3063因requested_dimensions进入已有单次软建议。模型下一patch只补runtime_work_relation，未补根因选择；log3107仍提醒缺选择，最终停止。不是磁盘丢失，也不是完全无模型选择意图。
- 实际130B合法JSON为 `schema_version:2,root_causes:[],status:unavailable,reason_code:model_root_cause_selection_rejected`。这个数组不得读成“无根因”。本例没有取得B1548有效选择正证；默认必产空旁路分支正常。
- 客户 `json_root_err.txt` 是另一情况，独立诊断见统一账本§123.1788：模型从未提交字段，原因应为 `valid_model_root_cause_selection_unavailable`。不能将这两条生产证据合并为一个解析错误。

## 3. C++：代码后验与程序证明不互相替代

保留worktree：`eval/results/github_issue_fmt_tm_year_overflow_symptom-20260913-193754/run-1.repo/.codrax/worktrees/trace-1789353595571261000-52880`。两次应用提交 `da6d7e9` → `86db2b2`，相对seed `2b1b2ba`只改 `include/tmfmt.hpp` 两行，原 `tests/test_tmfmt.cpp` 和Makefile不变，worktree clean，未合入主fixture。

- 模型首计划把加法升为int64_t却又转回int传给render_year，真实测试检出 `large positive year: got -2147481749, want 2147485547`。之后通过真实当前工作区修复，把render_year参数与整个计算链改为int64_t；没有修改测试预期绕过失败。
- B1122再现：run_tests/验证器摘要只展示Make输出的首条编译命令，验证器误称编译失败，完整反例后续才供给planner。原失败退出和结果没有被吞，缺口在摘要供给；统一账本§1787.12已列真实公共测试及有界原文方案，仍未施工。
- 主审独立编译执行原测试通过；临时 `/tmp/codrax-r1066-native.cQJYXN/year_matrix.cpp` 以交付头文件为被测对象，8016项（INT_MIN/INT_MAX附近、负值、0及普通年份）在UBSan且不恢复错误模式下全过。记录为独立后验，不回填产品报告或认为一次Make aggregate成功等于每个合同都已执行。
- 最终仍为 `verification_proof_incomplete`，未闭 `batch-1-cumulative-review`。log4617把无新失败证据的继续探索归为accept_unverified；没有强行all_verified。B1668完成态同批非法探索循环未再现，但该次结束走的是既有verification-only后续分支，不能据此单独销账所有B1668自然联接臂。
- 末计划 `no-overflow` 为 required=true/invariant/satisfies；当前声明test_tmfmt.cpp/expect_year:ordinary year等，历史声明main/expect_year；实际末报告只有make-test/check aggregate（366ms），没有逐断言执行身份/contract_refs。两代各有声明未观察和合同缺观察，共4项累计未闭；控制器当前计划显示2项有明确scope，不是累计丢账。B1561/B1575防冒证生效，但原生逐合同收据未闭。
- **B1671/P2上下文口径确认**：`write_controller.go:339/459`只计HardRequired，却显示required_typed_contracts=0；证明账仍包含soft-required。本例apply log2457/2465模型据此误认为soft-required可忽略，后续系统正确未签all_verified。修向是显示hard/soft/planning三者准确范围，复用现有分类/覆盖谓词，不把soft改hard、不改proof门。
- 代码使用int64_t却未显式包含<cstdint>，本机header-first编译可用；未证明所有标准库实现上的自包含/可移植性。保留有限验收边界，不扩成已证系统P1。

## 4. 保持不变与后续

JSON教学不增硬门；没有对用户问题、模型正文或图的业务词做关键词拒绝，也没有系统代替根因选择/重写答案。600s首响应、300s真实静默、600s非流默认不变，专项count3通过；两例未触及10m/5m阈值，不把短运行当长流实测。用户中途追加日志未造成重启eval或第三例。

按ROI继续：B1122复合执行失败详情供给 → B1561/B1575原生逐合同执行能力；B1548省略/拒绝补充机会与统一JSON教学为策略升级提案，旧合法省略/无额外轮次裁定不擅自删除。新旧机器/人工失败均留存，不追同题绿。
