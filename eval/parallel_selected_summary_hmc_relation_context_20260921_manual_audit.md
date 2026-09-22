# Selected Eval Manual Audit

- date: 2026-09-22T06:32:36Z
- sweep_start_ts: 20260921-233235
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_relation_context_20260921

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_g1_english_dstate | PASS | eval/results/hmc_relation_context_20260921/real_trace_g1_english_dstate-20260921-233236 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 166s | 43 | read=1,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 三段D+IO及0.635ms正确；调用点被越权解释为正常文件系统/块设备同步等待，非英语回答；新分析字段校验次序矛盾见下。 |
| 2 | sr_java_call_chain | FAIL | eval/results/hmc_relation_context_20260921/sr_java_call_chain-20260921-233236 | primary_answer | none | 247s | 47 | read=7,repo_map=1,list=0,trace=0,source_lens=0 | midloop=6,inv=3/0,fin_reject=3,unavail=0,prune=0 | FAIL | 5条源码调用关系和容量检查保留，但stdout冒称审计落库，6项清单混作6跳；图无容量失败分支，局部变量与存储文本复述失真。 |

## 完整主审结论

冻结revision `51840eecf7eb`，buildTime `2026-09-22T06:31:56Z`。runner87611正式exit0，严格2并行×1，无第三例、无改oracle。机器1/2，完整人工0/2；本次由主审逐读最终答案、真实代码/原生等待值和过程日志，未宣称本批有新增独立人审。

### G1

最终文件 `.codrax/output/20260921-233520.088-12636.md` 全读。整份捕获34579.450627–34579.595184秒，D原始状态3段0.138/0.147/0.350ms、合计0.635ms及caller原文正确；schema2空旁路原因 `trace_root_cause_contract_not_active` 符合有限事实请求。最终上下文不再重播被投影排除的模型聚合，旧错误近似168/183/371μs没有复活为计量，未出现小数伪关系；但本轮成员是工件定位串而非纯小数，不能宣称live独立命中全部数字语法反例。

正文末段将opaque caller翻译为同步缓冲区读取，并判为文件系统或块设备的正常等待行为，证据只支持调用点和iowait标记，不支持具体业务机制或“正常”。该人工FAIL保留，不能靠补提示或扫描正文硬门销账。英文问题得到中文回答，附注仍泄漏英文系统统计标签，另留展示债。

过程新P1：日志551–552首次emit拒绝 `artifact_value_profile.value is required`；第二次577提交 `pending_observation`，578立即按 `is_scalar_answer=false` 丢弃整个可选profile。系统先校验不适用字段再清理，迫使无用重试20秒并诱发占位值。应按已有typed适用范围先处理，保合法标量值的严格校验，且同审field_value兼容转换不能绕回此范围。仅记录，未在冻结版本中修复。

### Java

最终文件 `.codrax/output/20260921-233641.161-12644.md` 全读；对照fixture全部5个Java类。最终关系摘要和图保5条真实调用边：create→schedule，schedule→resolveMaxVisits/countOpenVisits/insert，insert→record。容量条件确在schedule，失败抛异常、阻止insert/audit；但链列表包含guard和分支调用，不能当串行6跳。countOpenVisits只按petId前缀计数，无未完成状态；返回值直接用于if，不是赋值给调用方n；存储串为冒号而非箭头。图虽可表达成功顺序，却无alt/opt表达容量失败，不能独立表示完整逻辑。

AuditLog.record:6确为System.out.println，无数据库写入；正文仍称审计落库，概念目标核对将模型错误“支持”选择展示出来。这是语义问题，不能由系统将println按名字硬归类或替模型改判。机器FAIL与人工一致，不修改oracle。源码关系投递正常不抵销结论失败。

成文先因缺边metadata/锚标签拒绝；patch两轮分别重复消费failure_ref、同块atomic与replace冲突，被事务层正确拒绝且未修改基底；第三轮patch接受。保持图和5条边，没有删除图规避校验。摘要列patch=4是runner指标，实际为1次full emit+3次patch；不将拒绝一概归为畸形JSON。首轮正确typed修复权限已经提供，暂不凭这次模型误用新增限制。

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
