# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T02:35:52Z
- sweep_start_ts: 20260811-193551
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | hilog_mixed_arkts_cangjie | PASS | eval/results/hilog_mixed_arkts_cangjie-20260811-193552 | log_attachment,answer_contains | log_triage | 76s | 21 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Four literal frames were retained and the two requested language locations were named, but log_triager nested two adjacent hilog error occurrences under `errors[].cause` solely from similar wording/time adjacency. No explicit Caused-by marker exists. Downstream rendered that model-authored pointer as authoritative `caused by`, so the final first asserted a proven propagation chain and only later called it inference. B595 confirmed. |
| 1 | github_issue_gson_lazy_number_symptom | FAIL | eval/results/github_issue_gson_lazy_number_symptom-20260811-193552 | write_apply,write_patch_oracle | none | 146s | 23 | read=6,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | uncertain | The applied equals/hashCode patch is minimal and `make check` passed with exact changed-file coverage. The repository also contains a manifestless Java main behavior test; the host has no javac/java, so the independent behavior lane correctly remained runner_missing/unverified. Source-static success must not be promoted to Java runtime behavior. This is an eval-environment limitation, not a reason to weaken verification. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

### B595 — model-authored log cause pointer escaped as typed causal authority

The attached artifact has two explicit occurrences: one ArkTS `Error` stack and one Cangjie `panic` stack. It has no structural exception-chain separator. The log triager nevertheless emitted the Cangjie panic as the ArkTS error's recursive `cause`. Existing validation checked that both messages were verbatim and bounded error cardinality, but never checked the relationship. Context rendering then converted the pointer into `↳ caused by`, granting a hard relation from a noisy model grouping decision.

The generalized repair is not an ArkTS/Cangjie special case: every recursive cause edge must carry a sibling `cause_relation` with the closed authority `explicit_artifact_marker` and one exact artifact separator line. Runtime validation verifies both verbatim presence and a closed producer-defined separator family. Adjacent timestamps, similar messages, common tags, or shared IDs remain peer occurrences with an unproven cross-error relationship. Downstream context exposes the marker and explicitly teaches the peer boundary; the system neither creates an alternative edge nor rewrites the model answer.

### Write verification boundary

The write controller preserved the applied diff and one passed project-declared source check, then honestly reported that Java execution was unavailable. The behavior test would exercise Map/equality semantics more strongly than the Python source oracle. Keeping the result unverified is the correct safety behavior; this run should be replayed on a JDK-capable host or replaced in the local campaign by a write fixture whose required runtime is installed. No `runner_missing` retry loop occurred.

### Recovery / duration audit

Both streams completed below four minutes. There was no malformed answer JSON recovery, no system-authored answer substitution, and no duration-triggered degradation. The standing invariant remains: elapsed time alone cannot authorize fallback; an active stream with real bytes stays active, and any recovery may publish only model-generated carriers with explicit disclosure.
