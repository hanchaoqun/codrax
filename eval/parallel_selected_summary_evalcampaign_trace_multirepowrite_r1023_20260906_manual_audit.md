# r1023 人工审计：Trace 事实卡与跨仓 Python 写模式

- date: 2026-09-06T07:54:04Z
- sweep_start_ts: 20260906-005402
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

基线 `947d5916e`，严格并发两路。已检查真实模型上下文、成文参数、最终答案/投影/旁路，以及写模式补丁、测试与累计证明。机器结论不代替人工正确性。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_memoclaw_text_search_multirepo_py | FAIL | eval/results/github_issue_memoclaw_text_search_multirepo_py-20260906-005404 | log_regex,write_apply,write_patch_oracle | none | 128s | 28 | read=5,repo_map=3,list=0,trace=0,source_lens=1 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | partial | 修复/权限边界正确；旧原因码清单留下proof_weak，另probe异步行为覆盖不足，不能将两者混为代码失败 |
| 1 | real_trace_h11_cross_direction_overlap | PASS | eval/results/real_trace_h11_cross_direction_overlap-20260906-005404 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 136s | 45 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B1570事实卡命中、◎双席在；答案仍有上限/单位/计数误述，且系统存在截断名单冒充全集与旧ceiling教学；模型未选旁路，空JSON诚实生成 |

## Trace

- 最终 `.codrax/output/20260906-005618.609-8399.md`：有模型总结、唯一因果投影、主要时间占用/业务族/邻近背景分层，四方向总览完整。Comp7.405、Jank4.710及互斥合计12.115在◎，本轮调度发布值是3.956，不沿用上一轮6.797。
- B1570生产命中：实际finalizer上下文的Comp measured8.294/effective7.405、Jank measured3.299/effective4.710及读者卡一致；不再把复合量称实测running。B1568本轮使用精确查询窗，未触发rounded→exact特定故障臂；只能算总览回归通过，真正旧缺陷由生产fixture先红后绿覆盖。
- 同名根因JSON确实生成139字节，但 `status=unavailable`、`root_causes=[]`。唯一emit仅有blocks，模型未提交根因选择；工具明确提供14候选与可补选advisory，没有打包失败/删除选择/错误schema证据。默认必有文件正常，程序化根因内容本轮缺席；不得自动替模型选项或把空JSON说成available。
- B1571：`renderTraceFinalPrincipalRankPopulation`只取8项，却宣称全部ranked_row_count=8、其余均unranked；同一上下文的方向计划仍列#9–#14。显示cap冒充资格全集，应全量typed census和emitted/total/complete分开，未列不降资格，保留主窗/链门。
- B1573：`internal/skill/defaults.go:987`仍教“largest seat … direction's recoverable ceiling”，与本轮同向可加12.115大于最大单席7.405及供给估算边界冲突。“消除上限”多次出现不能纯归模型波动；需修单项/方向/已证合计/估算口径的共享教学，不改模型正文。
- B1574：两份原始blob与ref哈希独立复算证明同一Comp7.405席因rank/host的Object不同成为两个ref；direction member_count=4却有2个headline+3个additional。不是合法异窗/异板，需精确donor别名单源；禁止按ordinal粗去重。0.598实际属于邻近rank11，系统没有给#3直接赋该值；重复#3由系统提示诱发，模型再错配0.598，必须分开说明。
- 答案仍有558kHz单位误述、2.1/2.34GHz治理来源混用、4次可见完成片段与47段全族混用及“未证关系”旁边称独立的含混语言。模型收到的相交/不可加限制不允许这些结论；先记录质量问题，不加正文扫描硬门。
- 本轮没有成文硬拒/patch，上下文45%，非预算耗尽，无活跃流固定年龄降级。B1569不在本轮二进制，不能记live命中。

## 多仓写模式

- 只修改授权 `python-sdk` 内的 `memoclaw/client.py`；sync/async均改为POST `/v1/search`及query/limit/可选namespace的JSON体。测试、API reference未改，只读兄弟仓库干净。
- 第一纯静态probe遭耦合拒绝合理；第二动态文件导入未被静态imports联接识别，且该版另有新建transport后读空结果的模型错误；第三版导入/实例修正后执行。JSON字符串包裹的结构数组被兼容层恢复，没有丢变更。
- Codrax只执行一个probe，项目make被 `probe_primary_suite_skipped` 跳过。probe动态覆盖sync+namespace，async仅AST，却声明两个行为合同；此为模型验证设计不足，不能把carrier覆盖数量等同于充分行为验证。
- B1572：已有按同contract-ref替代证据消解缺失的实现，且28/28累计义务已covered；producer发新码 `project_test_assertion_not_observed`，汇总清单只认旧码 `project_test_observation_not_executed`，过期reason使结果留proof_weak且无未覆盖义务可排补验。不是“4/4即应无条件绿”的推断，需按原精确替代证明语义统一新旧码；失败、异ref、部分覆盖仍不能消解。
- 人工另在applied snapshot运行sync/async × namespace有/无，核对方法/地址/body/返回和await全通过，原测试也通过。这些是人工审计结果，不冒充本次Codrax自身凭证；保留原runner FAIL与未完全验证事实。
