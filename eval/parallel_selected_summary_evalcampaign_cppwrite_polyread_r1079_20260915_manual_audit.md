# r1079 人工审计：C++ 症状写修复与跨语言调用链读取

- date: 2026-09-16T02:59:46Z
- sweep_start_ts: 20260915-195946
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

基线`285f08729e12`已推送，冻结86包全测通过后清洁构建（2026-09-16T02:59:09Z）。02:59:46Z–03:01:47Z恰好两路、各一次，CAP5/read15/apply24/1200s，case/fixture/oracle不改。机器1PASS/1FAIL；以下区分业务代码后验、正式证明、答案事实、图语法与图语义，不改原机器结果、不追加第三例追绿。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | github_issue_fmt_tm_year_overflow_symptom | FAIL | eval/results/github_issue_fmt_tm_year_overflow_symptom-20260915-195946 | write_apply,answer_regex | none | 120s | 28 | read=4,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | business-code-pass / formal-unverified | 原测试及336边界/UBSan后验通过；Make聚合能力unknown，正式proof_weak保留 |
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260915-195946 | answer_regex | none | 121s | 41 | read=3,repo_map=1,list=0,trace=0,source_lens=1 | midloop=4,inv=1/0,fin_reject=3,unavail=0,prune=0 | fail / core-chain-partial | 语法可渲染，图断链/encode顺序/回退参数/函数关系错误；精确供给未被模型正确采用 |

## 1. C++：业务补丁正确，正式证明未闭合

1. 最终plan=`plan-1789527678759829000-46766`，实际apply/verify进程46961，正式final在20:01:44（本地）完成。case自身118s、父runner120s属不同计时边界。最终durable提交`fa90c76d2bf57dd548bff23c718255078cee0c69`，scratch main仍`019a4f320db5cad9c37ce99295ab226a1fff106f`，未自动合主fixture。
2. 仅`include/tmfmt.hpp`三行修改：加法前将year_offset升为long long，render_year参数全链保宽，to_string处重复cast无害。原tests/Makefile/README逐字未变，交付树、durable提交与保留worktree四源文件一致且clean。
3. 独立后验目录`.codrax/tmp/r1079-cpp-posthoc.PUn4Ru`：原测试在原实现负基线准确失败INT_MAX（-2147481749 vs2147485547）；原336项矩阵239过97败。最终交付原测试通过、同矩阵336/336、无-fwrapv的UBSan矩阵336/336且零诊断。探针未给模型、未改正式工作树或回填proof；本机有限验证不代表全部ABI/标准库/平台。
4. 正式验证确实执行`make check`，真实日志`codrax-20260915-200118-000-46961.log:668–670`记录编译、运行、退出0及880.549ms，不能说产品完全没跑测试。但report只记录`make-test/check`一项aggregate结果，changed-path capability为unknown，原因`aggregate_project_target_execution_unobserved`，没有目标执行身份的结构凭证。
5. 五个behavior_contracts均为明确planning-only；hard/soft required均0，不能把本轮误写为“有五个必需合同被漏验”。本轮低置信不是旧累计失败未清，而是原生执行能力缺精确凭证。控制器模型选择all_verified后，确定性终局仍`unverified:proof_weak`，最终用户面明确“未完全验证”，没有假绿。
6. B1319/B1561/B1575及B1678继承的原生runner证明问题继续OPEN。不得用编译命令/输出文字、Make target退出0、函数名称扫描或本次后验自动升为target execution。另留B1678诊断P2：`verification_proof_profile.go:178–180`将所有缺目标执行凭证概括成`production_verification_source_static_only`，本例实际unknown而非已证只有静态检查；资格降级合理，原因名称偏窄，不因此放宽证明门。
7. 初始read classifier曾将diagnostic=true与generic组合拒绝，模型下一轮改root_cause成功；未证实“同一合法声明同时必带必拒”。无JSON畸形或成文重试，本轮不触发写修复失败详情/累计顺序等旧臂，不能替它们补生产正证。

## 2. 跨语言读：宽oracle通过，完整答案人审失败

正式产物`.codrax/output/20260915-200145.423-46797.{md,html}`，以下读侧行号均指原单日志`run-1.logs/codrax-20260915-195947-000-46797.log`（合并日志另有两行头）；case119s、父runner121s。源码提供了Python facade、Rust core及PyO3 wrapper，最终正文保留`_fastlex`模块、注册、wrapper→core与回退入口，不能称整份答案或全部关系消失。

1. MD14把`list(data)`说成也传给回退函数；真实tokenizer.py:22传bytes，回退内部:25才转list。MD35–40把encode画到原生条件分支内；实际:19在:20判断之前，日志1983–1995已供同owner源码顺序。两处是模型错误，不是对应数据未给。
2. MD46将best_merge称为“非独立函数跳转”，但lib.rs:20定义独立函数、:13调用它，日志1969/2370已有call edge。fan-out只表示兄弟调用不能自动串成连续调用链，不等于不是函数调用。
3. 最终图Wrap/Core孤立，不能表达完整Python→导出面→注册绑定（非源码call）→wrapper→core。模型还将已有真实局部调用的Encode/List写成unproven。回退算法仅有入口，没有最低rank选择/替换删除/无匹配返回；merge.rs本轮未读，不把完整末端未探索一概称系统丢已供证据。
4. 原图经仓内`internal/preview/assets/mermaid.min.js`真解析+render通过，1张sequence、SVG27130B，收据`.codrax/tmp/20260915-r1079-read-mermaid-render.json`，源字节未修；不是Mermaid语法失败。没有做完整浏览器视觉/布局验收，语法通过不抵销顺序与关系错误。

## 3. 三次拒绝、JSON教学与答案所有权

1. 第一次log2479–2555：principal list缺from_identity/to_identity，图未带任何edge_anchors；已有log2231–2236提供guard、wrapper→core、encode/list的完整recipe。不是系统把正确模型关系改坏后再拒绝。
2. 第二次log2591–2595：attach缺必要嵌套edge，未staging；第三个调用log2631–2655成功。该模型补丁主动提交三个remove，并将Core/Encode/Guard/List/Wrap写为unproven，原参数保存在`.codrax/blob/20260915-195948-000-46797/tool-call_function_b41o2375cukn_1-emit_answer_document_patch-params-e159c6c6.json`。图边是模型删除，不是系统擅自改答案。
3. 最后一次log2697–2716：模型把definition证据`ev-6cb7c2b916f4ed33`选作terminal-operation receipt；日志2423–2432已给真实operation候选，包括wrapper→core的`ev-1d8f08ac73c376d0`。精确拒绝成立，最终保留前一成功稿。未发现同一个可用证据被同时要求必带/必拒，不应为追绿撤证明门。
4. 注册行模型误写`anchor_kind=registration`，兼容层降为text_reference；log806已明确registration不是anchor_kind，且真实绑定应使用syntax-matching call/assignment/initializer。当前缺registered-export handoff不能据此自动升级AnchorCall权威。保留后续公共对照机会，但本轮不足以确认独立parser绑定通道断路或新的系统P1。
5. **B1694生产命中N/A**：日志1266–1267扫描3文件、实际concrete值提取0；只有独立branch_effect，没有同坐标definition/concrete-return碰撞。metrics的`concrete_values=2`是日志命中数，不是恢复了两条返回证据。修复有效性依本批新公共五语红转绿+count3/race/全仓收据，不拿该live虚报命中。

## 4. 收账与下一优先级

- 原机器汇总、MD/HTML、两例out/合并日志/verdict/metrics及所选正式JSON均已指纹保全，清单`.codrax/tmp/20260915-r1079-{output,formal}-before-audit.sha`；case及两fixture前后SHA一致。后验另有独立原工件保真收据。
- 优先继续B1694点/范围引用分支（full与patch均需精确范围和模型池所有权）；随后原生执行证明能力B1319/B1561/B1575，不以stdout词面授能力。B1678未知/静态诊断词留P2，不丢账。
- 本轮图错误保留模型质量/合法教学采用观察；有明确源码/recipe但模型选错，不再给该case加专属硬门。单次不能证明随机波动分布，后续用其他语言/模式观察，不追加本题追绿。
- Trace本轮N/A，不代销显式窗、因果投影、自动补齐、IO/供给/语义/业务双轴或root侧车。代码默认600/300/600s和活跃字节续期不变，本批无超时；短运行不等于十分钟客户流实测。

状态：`exact2-once/machine1PASS1FAIL`；`write-business-local-pass/formal-unverified`；`read-human-fail/mermaid-syntax-pass/semantic-fail`；`B1694-claim-live=N-A`；`new-system-P1=not-confirmed`；`original-model-artifacts+oracles=preserved`。
