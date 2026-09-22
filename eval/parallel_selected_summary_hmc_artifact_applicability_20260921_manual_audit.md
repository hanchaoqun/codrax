# Selected Eval Manual Audit

- date: 2026-09-22T06:51:20Z
- sweep_start_ts: 20260921-235120
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_artifact_applicability_20260921

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_go_typo | PASS | eval/results/hmc_artifact_applicability_20260921/patch_go_typo-20260921-235120 | write_apply,write_patch_oracle,answer_contains | none | 86s | 28 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | PASS | 实际一行patch，TestGreet原生断言及go test -json命令exit0；seed HEAD不动，无自动合入。普通写路径不代销B2–B6。 |
| 1 | real_trace_g1_english_dstate | PASS | eval/results/hmc_artifact_applicability_20260921/real_trace_g1_english_dstate-20260921-235120 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 112s | 32 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | D+IO三段及0.635ms正确，caller机理越界；系统英文附录违反项目中文锁，正文中文无错。profile为false，仅触达新分支非旧缺value真实命中。 |

## 冻结身份及完整审计

revision `fd000269c58e`，buildTime `2026-09-22T06:50:29Z`；runner12571正式exit0。严格2并行×1、不改oracle、不跑第三例；机器2/2，完整人工1/2。主审逐读Go计划、实际diff、验证/最终交付记录及可见终端交付；G1主审与独立只读复核均完整读最终答案、旁路、原生事件和关键成文输入。

### Go写入

计划`plan-1790059926969991000-26837`，真实apply commit `9db8ab98d6a45941529f99a9f3ce677159b334dc`，仅main.go一行retrun→return，无其它变更。实际`go test -json ./...`退出0、TestGreet断言通过、changed-path被真实project runner覆盖，工作树审计clean。不是只读规划PASS；最终报告complete/verified。自然语言清单中go build未有独立命令收据，终端明确说明自然语言清单不代表逐项独立执行，未虚称全部命令都跑过。

fixture主仓HEAD仍`2643ad38530b35f6f5215e3a9be962d02900dfe5`，等于seed；修改保留隔离工作树及recovery ref，没有自动合并。未发生B2–B6补证/恢复，不能代销那些任务。本例无runtime profile，新代码只提供非Trace/写模式不退化对照。

### G1读取

全读`.codrax/output/20260921-235310.204-26813.md`：原始D+IO三段0.138/0.147/0.350ms、合计0.635ms，完整34579.450627..34579.595184秒捕获窗和目标59566正确。系统补采窗口同请求、未用小窗替代；空schema2根因旁路为有限事实的`trace_root_cause_contract_not_active`，可选图未输出合理，均不判失败。

正文仍从`sync_buffer_read_wi`断言同步缓冲读取等待并展开磁盘/文件系统机制，最终输入1284/1542已明确禁止此推断，属于模型未遵从而非缺教学。`io_wait=1`也不是原始`iowait=1`拼写。此片不据一个样例加关键词门，不宣称已证随机波动。

系统语言gap与模型错误分账：日志1555–1557明确项目锁中文，中文正文正确；等待附录和补采说明却为英文。`requestedAnswerDocumentLanguage`优先旧AnswerContract语言，现有系统展示消费者和模型消息未使用同一有效语言权威，登记后续公开回归。同步更正前批人工记录中“英文问题得到中文”这一不充分理由，原FAIL仍由机理越界支撑。

两次分析提交仅一次拒绝：515为`bounded_effect_verdict`缺`target_effect_verdict`维度，第二次改finite并在544–545接受；不是artifact value错误或超时。两次profile均`is_artifact_value_lookup=false`，新drop warning被触达，旧parser亦会nil，因此不能称旧缺value缺陷已在本轮live复现并被消除。公开红绿负责该修复验收。最终首次成文接受，无成文patch；活跃流未因无答案4ms降级。

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
