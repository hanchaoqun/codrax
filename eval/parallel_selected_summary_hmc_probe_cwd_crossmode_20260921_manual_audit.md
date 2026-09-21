# 固定双例人工审计：计划教学与 Python 探针执行目录

- date: 2026-09-21T10:19:13Z
- sweep_start_ts: 20260921-031913
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_probe_cwd_crossmode_20260921

固定干净二进制88be719dc396，2并行×1；runner35140正式exit0。机器1/2 PASS、完整人审0/2；Python源码功能/原生执行正确，但不等于完整验证交付通过。此文件保留新能力命中边界，不以自动结果代人审。

| case | 机器 | runner耗时 | 人工完整验收 | 本次重点 |
|---|---|---:|---|---|
| trace_query_jank_field_inventory | PASS | 134s | FAIL | 数值/身份正确；扫描覆盖解释与内部词汇仍不合格 |
| nested_python_increment | FAIL: proof_weak | 184s | FAIL | 源码/3原生测试正确；required断言身份未绑定，局部零测试被总体化；§82未live命中 |

## 1. jank 字段清单：数值正确，覆盖解释与读者词汇仍 FAIL

[模型正文](results/hmc_probe_cwd_crossmode_20260921/trace_query_jank_field_inventory-20260921-031913/run-1.primary.md)第7/9/11行正确列出3条7/4/2帧、5.040/5.050/5.010秒头行时间、六个超过2^53的原始纳秒整数及70/40/20ms。一个event_search实际以appid=620与jank_frames>=2的AND数值条件扫描完整材料；同名子串、帧数1、异appid及非法数字均未混入。没有凭marker生成调度根因。

旧发射身份误述本轮未复现：第19行TID=101/TGID=101正确，未将marker PID201冒充发射进程；appid未直接当受影响线程。时钟/目标身份/链证据不足的主结论正确。但第25行先称完整材料域查询，再称“未构建完整trace索引，如有更宽时间范围的marker可能未被纳入”，将执行策略误当扫描不完整；本次coverage是完整artifact、scope5.010000–5.070010、14个带时间戳事件、matched_total=emitted=3，无额外范围限制。5.01–5.05仅匹配行包络，不是扫描上界。第15/17/23/25行还泄漏symptom、time_domain_status/unverified、typed、streamed_event_search等内部词汇。完整答案维持人工FAIL，不倒签§62旧FAIL。

实际最终上下文日志1988–1994清楚区分扫描覆盖、枚举完整度、成员展示，明确要求普通用户语言；完整typed inventory含正确coverage、三条精确值及emitter_tid/emitter_tgid/marker_pid。日志1996也明确完整artifact域扫描。故事实和关键限制已经供给，尚未发现新的相互矛盾硬合同或缺字段系统故障；保留模型消费错误及实现细节噪声观察，不追加答案关键词门。原生streamed_event_search caveat仅说不构建/缓存全索引，并未说漏扫描，不能将模型附加推论记成引擎漏数据。

过程：预阶段候选一度误收同名子串99帧且误算时长，后续唯一确定性查询恢复3条；最终输入以原生库存为权威，不把预阶段猜测当测量值。analyzer初次not_applicable被现有typed合同拒绝后改bounded_fact_set/count_or_duration+occurrence_time；最终一次emit成功、零成文拒绝/降级。机器PASS，runner134秒，case内部132秒，最大上下文29%。有限事实请求无需因果投影，不是Trace能力消失。

必选旁路`.codrax/output/20260921-032125.750-79837.root-causes.json`已生成schema2、root_causes=[]、status=unavailable、reason_code=trace_root_cause_contract_not_active。最终Markdown SHA256为27a71d5e7a205bf69d90573a99444e2c9555e105a03dc153f2e72a200bc8b3f1，与归属收据一致；无Mermaid图，不将本例算作图渲染回归命中。

## 2. 子项目源码修复

实际seed HEAD12d664605510033ea3880714114a94b0d0f2e059不动，交付69bf91ec3eb7be3eb0dc1cf139c76b9c73628cc0只改widget.py一行return value→value+1。原测试、setup.py、测试包初始化与交付提交中的.gitignore均未修改；测试三面SHA256仍为504b2535413cafa883be2802d92a698036735de8883384530caec4fa8375f322。原仓工作区并非零变化：runtime向.gitignore追加.codrax/且未提交，非模型计划改动；隔离交付树干净、无主干合并，不隐去此边界。

两次真实unittest在packages/widget执行精确tests/test_widget.py，均3passed，日志2496等与[最终报告](results/hmc_probe_cwd_crossmode_20260921/nested_python_increment-20260921-031913/plan-1789986034752069000-79852.report.json)一致。PTO短suite IncrementTest符合既有suffix规则，不单独记错；真正不匹配的是短assertion_id=test_negative_integers等，真实身份为python/unittest@packages/widget::test_negative_integers。三项required合同均missing，最终诚实unverified/proof_weak，机器失败有真实证明依据。verify-only再跑相同声明不能补登记身份，B2–B6不闭合。首轮schema仍举裸method且未说明系统非根前缀，是供给教学接缝，不应全归模型猜错；不放宽精确matcher。

全程emit1次、拒绝0次、RunTests2次、verification_probes=0。本次没有探针cwd候选消费，§82未live命中；不据零拒绝宣称该修复已在生产降低重试。

另确认系统矛盾：root discover零测试与nested三个PASS合理并存，但合并NoTestsRunners=['python']被writeflow/observation_authority.go:68、stage_hooks.go:1193、write_controller_scheduler.go:3616按整批无测试消费；与types.NormalizeVerificationStatus的“且TestResults为空”相冲突。write_verify_render.go:345将局部提示写成总体“python没有发现任何测试”，实际最终out134/145两次出现并建议补环境，属于系统投影错误。write_retry_helpers.go:152还可能抑制混合报告中的真实失败重试，需要精确负控；不能仅修一句显示，也不能用另一个PASS抹掉局部缺测、未覆盖路径、runner缺失或required证明债。该新高ROI接缝独立留档，下一批先公开复现与修复，不回填本轮FAIL。
