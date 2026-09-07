# r1034 人工审计：机器2/2通过，不能据此收为正确答案

- date: 2026-09-07T12:40:21Z
- sweep_start_ts: 20260907-054019
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

源码与二进制：干净`224e47cffc77`。严格并行2例，未提高预算、未改旧oracle/fixture，最终全仓回归与本轮部分时间并行，不将运行时长用作性能加速对照。以下保留原机器评分；人审依据完整日志、源码、最终答案及持久化应用结果，不回填旧成绩。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260907-054021 | answer_regex | none | 115s | 39 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=6,inv=2/0,fin_reject=3,unavail=0,prune=0 | fail | 主要调用边和walker角色正确；导语仍把顺序工作说成并行。第3轮整块修补拒绝有系统自相矛盾教学，B1609；最终图保留5条调用边，无整图丢失 |
| 2 | github_issue_nlohmann_long_double_symptom | PASS | eval/results/github_issue_nlohmann_long_double_symptom-20260907-054021 | write_apply,answer_regex | none | 143s | 29 | read=6,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 两头均lg→Lf，修类型却改g格式语义；原生非空测试放过，eval从未改README匹配正确Lg假绿，B1610；未发现伪造逐断言执行 |

## Rust：关系方向、执行顺序与修补过程分开判断

日志`run-1.logs/codrax-20260907-054023-000-96309.log`；最终答案`/Users/han/opt/codrax/.codrax/output/20260907-054214.597-96309.md`及同名HTML。实际先读3个源文件；源码为run先选唯一匹配器，再collect_files→walk收集返回，然后run逐文件调用index_file→is_match，非并行。README线性简写不能证明collect_files直接调用index_file；本轮最终图没有造这条错误边。

最终导语“两条并行的执行路径”与已提供源码/精确证据矛盾，是r1006已有模型质量观察，不新增关键词门或系统改正文。图的5条调用方向正确、节点仍是业务代码名，未被删空；未展开返回、构造分支和循环，不能把可用子集图说成完整执行证明。字段/引用层通过不等于自然语言推理正确。

B1609确证：第2轮修补后只剩cf→w缺正确锚；已有当前原子relation lease时，系统辅助hint仍教replace_blocks和保持body逐字，第3轮照教随后被whole_replace_not_authorized拒。第4轮按合法failure_ref原子replace可以通过，所以不是证据不可用或必须放松门。修向是活跃能力与教学同源，非允许整块覆盖保留边。

另外相同“概念目标核对”出现两次：首稿diagram已有receipt，后续系统soft要求summary再补同receipt，每block渲染分别发射。列B1611/P2同一绑定披露去重设计；不扫描/删除模型原文，不同结论/来源保留。

## C++：真实持久化补丁错误，验证范围不能冒充业务保持

`run-1.plan.json`与`plan-1788784930733223000-96307.report.json`完整读取；原生执行在第二日志`...054210-000-96618.log`。durable提交`6d0a351e9057e5c5251aa2d356ed169ca3c46d7e`相对base`f6841c3cee9fb350112940ae577574582487f186`只改两头第10行，均为`%.*Lf`，tests/Makefile/README未变，主scratch仍base。原计划就选Lf，未见系统代改补丁。

正式make check退出0、1182ms；测试同编译单元包含真实两头，检查1.25L输出非空。报告仅aggregate/make-test，无逐assertion receipt；全部自然语言验收项与签名项是planning-only。最终明确披露未逐项独立执行，故不指控伪造凭证，但行为保持确实未覆盖。规划log2232曾提交不include真头的模拟C++ probe，被不支持语言门拒，随后模型删probe；不把这段源码字符串当执行验证。既有B1561原生细粒度证明能力债仍在。

独立人审程序只include实际`run-1.applied-tree`两头，严格clang++编译后比较general-format结果，3组均不保持：1.25→1.250000、1e+12→1000000000000.000000、1e-12→0.000000。日志`.codrax/tmp/20260907-r1034-native-format-audit.log`；它是额外人工见证，不回填产品report。首次审计程序缺initializer_list导致未运行已纠正，不把该编译错误当行为反例。

B1610确证：MODE=apply无POST_APPLY_FILE时，run.sh汇总应用树所有跟踪文本；当前`%.*Lg`唯一匹配在未改的README.md:9，两实际目标均未匹配。修向是可声明的多文件post-apply域及逐文件断言，保单文件兼容、文档任务可以显式选文档，不按代码扩展名猜范围、不把多个文件拼接后由一个好文件遮住坏文件。原r1034机器PASS永久保留，只另记录严格作用域的人审失败。

## 处置顺序

1. B1609/P1：有效原子修补能力与提示一致，真实入口先红后绿。
2. B1610/P1：多工件逐文件eval域与假绿反例；不降低预期、不加产品格式关键词门。
3. B1611/P2：系统自有同绑定概念目标披露去重；模型正文不动。
4. B1607a/b、qualified-owner定位回退审计、B1561原生行为证明继续在统一台账，不因本轮机器PASS关闭。

## 修复后的独立核验（不改原回放评分）

B1609 已推送`ce3c18994`：真实完整提交/patch、必需/可选图的提示与原子执行能力统一，冻结后完整agent测试通过。B1611 已推送`c79f104a8`：仅同完整绑定的系统code receipt附注去重，中英真实渲染、完整render及race通过；模型正文与图不动。两项均尚无新LLM回放，不据单元测试写“本例已零重试”。

B1610 以**原r1034同一durable树**做显式双文件后验，两头分别`FAIL no_regex_match:%[.][*]Lg`，总判定FAIL。原机器`run-1.verdict=PASS`及正式report哈希未改，没有重跑产品LLM或原生验证。记录`.codrax/tmp/20260907-b1610-r1034-posthoc.log`与`.codrax/tmp/b1610-r1034-posthoc.lOrh42/run-1.post-apply-scopes.tsv`。新case声明只供今后回放，不能改写当时未声明scope的历史事实。完整处置/兼容边界见统一台账§123.1667–1669。
