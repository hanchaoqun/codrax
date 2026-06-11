#!/usr/bin/env bash

# Shared helpers for eval runners. Kept bash-3.2 compatible because
# macOS still ships that shell.

eval_run_with_timeout() {
  local seconds="$1"
  shift
  # Prefer one cross-platform implementation over platform-specific
  # timeout(1) variants. The Python path starts the command in a fresh
  # process group and always tears down the whole group on timeout, so
  # a timed-out eval cannot leave LLM/tool grandchildren occupying a
  # parallel slot after the worker shell exits.
  if command -v python3 >/dev/null 2>&1; then
    python3 - "$seconds" "$@" <<'PY'
import os
import signal
import subprocess
import sys
import time

timeout = float(sys.argv[1])
cmd = sys.argv[2:]
p = subprocess.Popen(cmd, start_new_session=True)
deadline = time.time() + timeout

def kill_group(sig):
    try:
        os.killpg(p.pid, sig)
    except ProcessLookupError:
        pass

try:
    while True:
        rc = p.poll()
        if rc is not None:
            sys.exit(rc)
        if time.time() >= deadline:
            break
        time.sleep(min(0.5, max(0.0, deadline - time.time())))
    kill_group(signal.SIGTERM)
    grace_deadline = time.time() + 10
    while True:
        rc = p.poll()
        if rc is not None:
            sys.exit(124)
        if time.time() >= grace_deadline:
            break
        time.sleep(min(0.25, max(0.0, grace_deadline - time.time())))
    kill_group(signal.SIGKILL)
    p.wait()
    sys.exit(124)
except KeyboardInterrupt:
    kill_group(signal.SIGTERM)
    raise
PY
    return $?
  fi
  if command -v timeout >/dev/null 2>&1; then
    command timeout -k 10 "$seconds" "$@"
    return $?
  fi
  if command -v gtimeout >/dev/null 2>&1; then
    command gtimeout -k 10 "$seconds" "$@"
    return $?
  fi
  "$@"
}

eval_latest_result_dir() {
  local results_root="$1"
  local case_id="$2"
  local sweep_start="$3"
  local d ts
  for d in $(ls -dt "${results_root}/${case_id}"-* 2>/dev/null); do
    ts=$(basename "$d" | sed "s/${case_id}-//")
    if [[ "$ts" > "$sweep_start" ]] || [[ "$ts" == "$sweep_start"* ]]; then
      echo "$d"
      return 0
    fi
  done
  return 1
}

eval_metric_field() {
  local file="$1"
  local key="$2"
  local line
  line=$(grep -a -m1 "^${key}=" "$file" 2>/dev/null || true)
  if [[ -z "$line" ]]; then
    echo "-"
    return 0
  fi
  printf '%s\n' "${line#*=}" | tr -d '\000'
}

eval_count_pattern() {
  local pattern="$1"
  local file="$2"
  local n
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  n=$(grep -aE -c "$pattern" "$file" 2>/dev/null) || n=0
  echo "${n:-0}"
}

eval_max_context_tokens_estimate() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  LC_ALL=C awk '
    {
      if ($0 !~ /^20[0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[^ ]+ DEBUG \[diag [^]]+\][^:]*phase=llm_request/) {
        next
      }
      for (i = 1; i <= NF; i++) {
        if ($i ~ /^context_tokens_est=[0-9]+$/) {
          split($i, a, "=")
          if (a[2] + 0 > max) {
            max = a[2] + 0
          }
        }
      }
    }
    END { print max + 0 }
  ' "$file"
}

eval_max_context_window_tokens() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  LC_ALL=C awk '
    {
      if ($0 !~ /^20[0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[^ ]+ DEBUG \[diag [^]]+\][^:]*phase=llm_request/) {
        next
      }
      for (i = 1; i <= NF; i++) {
        if ($i ~ /^context_window=[0-9]+$/) {
          split($i, a, "=")
          if (a[2] + 0 > max) {
            max = a[2] + 0
          }
        }
      }
    }
    END { print max + 0 }
  ' "$file"
}

eval_max_context_window_pct() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  LC_ALL=C awk '
    {
      if ($0 !~ /^20[0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[^ ]+ DEBUG \[diag [^]]+\][^:]*phase=llm_request/) {
        next
      }
      tok = 0
      win = 0
      for (i = 1; i <= NF; i++) {
        if ($i ~ /^context_tokens_est=[0-9]+$/) {
          split($i, a, "=")
          tok = a[2] + 0
        } else if ($i ~ /^context_window=[0-9]+$/) {
          split($i, b, "=")
          win = b[2] + 0
        }
      }
      if (tok > 0 && win > 0) {
        pct = int((tok * 100 / win) + 0.5)
        if (pct > max) {
          max = pct
        }
      }
    }
    END { print max + 0 }
  ' "$file"
}

eval_count_control_pattern() {
  local pattern="$1"
  local file="$2"
  # Control-plane metrics must not match source snippets, model answers, or
  # customer-log payloads. Codrax debug logs are timestamped; requiring that
  # prefix keeps quoted control-looking text out of retry/rewrite counters.
  eval_count_pattern "^20[0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[^ ]* ${pattern}" "$file"
}

eval_count_finalizer_rejects() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  local tool render n
  # Count control-plane events only. Whole-log grep of these strings is unsafe:
  # source snippets and model answers may discuss `finalizer_rejects` or quote
  # customer logs that contain "成文校验未通过".
  tool=$(eval_count_control_pattern 'DEBUG \[diag finalizer\][^:]*phase=toolresult TOOLRESULT emit_answer_document(_patch)? ok=false' "$file")
  # Multibyte bracket expressions ([校交]) are locale-dependent: in a
  # non-UTF-8 grep locale the class decomposes into single BYTES and the
  # pattern can never match, silently zeroing the counter. Literal
  # alternation is byte-exact in every locale.
  render=$(eval_count_control_pattern 'INFO \[render\][[:space:]]+•[[:space:]]+(成文校验未通过|成文交验未通过)' "$file")
  n=$((tool + render))
  echo "$n"
}

eval_count_finalizer_rewrites() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern 'INFO \[render\][[:space:]]+⟳ 4/4 .*(答案待完善|正在重写答案|检测到 .*前后不一致)' "$file"
}

eval_count_answer_document_patch_calls() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern 'DEBUG \[diag finalizer\][^:]*phase=toolcall [^:]*tool=emit_answer_document_patch( |$)' "$file"
}

eval_count_midloop_injects() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern 'DEBUG \[diag [^]]+\][^:]*phase=midloop_inject' "$file"
}

eval_count_tool_calls() {
  local file="$1"
  local tool="$2"
  if [[ -z "$file" || ! -f "$file" || -z "$tool" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern "DEBUG \\[diag [^]]+\\][^:]*phase=toolcall [^:]*tool=${tool}( |$)" "$file"
}

eval_count_repeated_mcp_resource_reads() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  LC_ALL=C awk '
    /^20[0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[^ ]+ DEBUG \[diag [^]]+\]/ &&
    $0 !~ /ASSISTANT content/ &&
    $0 ~ /phase=toolcall .*tool=mcp_read_resource( |$)/ {
      idx = index($0, " params=")
      key = $0
      if (idx > 0) {
        key = substr($0, idx + 8)
      }
      count[key]++
    }
    END {
      dup = 0
      for (k in count) {
        if (count[k] > 1) {
          dup += count[k] - 1
        }
      }
      print dup + 0
    }
  ' "$file"
}

eval_count_source_inventory_tool_calls() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  # Count actual repo_map tool calls only. Prompt text, model answers, and
  # customer logs can mention source_inventory without the model using it.
  eval_count_control_pattern 'DEBUG \[diag [^]]+\][^:]*phase=toolcall [^:]*tool=repo_map params=.*"view"[[:space:]]*:[[:space:]]*"source_inventory"' "$file"
}

eval_count_repo_lens_discovery_hints() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern 'DEBUG \[repo_lens\] discovery_hint ' "$file"
}

eval_count_transient_retry_checkpoints() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  # Counts only the orchestrator control-plane event emitted when a preserved
  # explore-state checkpoint is installed after a stream-level retry. Prompt
  # text and quoted customer logs can mention the same phrase and must not
  # inflate this metric.
  eval_count_control_pattern 'DEBUG \[diag orchestrator\][^:]*phase=transient_retry_checkpoint [^:]*installed=true' "$file"
}

eval_count_unavailable_tool_attempts() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  local global restricted
  global=$(eval_count_control_pattern 'WARN \[agent\] tool "[^"]+" rejected before execution: not in current tool schema' "$file")
  restricted=$(eval_count_control_pattern 'WARN \[explorer\] tool "[^"]+" rejected: tool "[^"]+" is not available in the current explorer repair state' "$file")
  echo $((global + restricted))
}

eval_count_checkpoint_continuation_broad_hint() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern 'DEBUG \[orchestrator\] window hint applied .*Checkpoint summary.*DAG-scheduled investigation window' "$file"
}

eval_count_closure_only_repeated() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  # Counts the old anti-pattern where closure-only hints carried an
  # iteration suffix, allowing the same closure instruction to look
  # like a fresh hint every round. Stable per-window keys should keep
  # this at zero.
  eval_count_control_pattern 'DEBUG \[diag [^]]+\][^:]*phase=midloop_signal .*key="explorer\.mid-loop\.[^"]*closure-only\.[0-9]+"' "$file"
}

eval_count_mermaid_source_repairs() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern 'DEBUG \[mermaidcompat\] source repair applied' "$file"
}

eval_count_repair_debt_checkpoints() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern 'DEBUG \[repair_debt\] checkpoint ' "$file"
}

eval_count_repair_debt_close_ready_filters() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern 'DEBUG \[repair_debt\] close_ready filtered_advisory=' "$file"
}

eval_max_repair_debt_checkpoint_class() {
  local file="$1" class="$2"
  if [[ -z "$file" || ! -f "$file" || -z "$class" ]]; then
    echo 0
    return
  fi
  LC_ALL=C awk -v cls="$class" '
    /^20[0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[^ ]+ DEBUG \[repair_debt\] checkpoint / {
      n = split($0, fields, /[[:space:]]+/)
      for (i = 1; i <= n; i++) {
        if (fields[i] ~ ("^" cls "=[0-9]+$")) {
          split(fields[i], kv, "=")
          if ((kv[2] + 0) > max) {
            max = kv[2] + 0
          }
        }
      }
    }
    END { print max + 0 }
  ' "$file"
}

eval_sum_answer_contract_violations() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  LC_ALL=C awk '
    /^20[0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[^ ]+ DEBUG \[diag finalizer\][^:]*phase=answer_contract_check / {
      for (i = 1; i <= NF; i++) {
        if ($i ~ /^violations=[0-9]+$/) {
          split($i, a, "=")
          sum += a[2] + 0
        }
      }
    }
    END { print sum + 0 }
  ' "$file"
}

eval_sum_answer_contract_violations_for_section() {
  local file="$1"
  local section="$2"
  if [[ -z "$file" || ! -f "$file" || -z "$section" ]]; then
    echo 0
    return
  fi
  LC_ALL=C awk -v section="$section" '
    /^20[0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[^ ]+ DEBUG \[diag finalizer\][^:]*phase=answer_contract_check / {
      matched = 0
      for (i = 1; i <= NF; i++) {
        if ($i == "section=" section) {
          matched = 1
        }
      }
      if (!matched) {
        next
      }
      for (i = 1; i <= NF; i++) {
        if ($i ~ /^violations=[0-9]+$/) {
          split($i, a, "=")
          sum += a[2] + 0
        }
      }
    }
    END { print sum + 0 }
  ' "$file"
}

eval_count_agent_iterations() {
  local file="$1"
  local agent="$2"
  if [[ -z "$file" || ! -f "$file" || -z "$agent" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern "DEBUG \\[diag ${agent}\\][^:]*ASSISTANT content_len=" "$file"
}

eval_count_agent_dispatches() {
  local file="$1"
  local agent="$2"
  if [[ -z "$file" || ! -f "$file" || -z "$agent" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern "DEBUG \\[diag ${agent}\\][^:]*DISPATCH stage=" "$file"
}

eval_count_semantic_quality_dispatches() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern 'INFO \[semantic_quality_reviewer\].*verdict' "$file"
}

eval_count_semantic_quality_concerns() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern 'INFO \[semantic_quality_reviewer\].*(emitted [1-9]|verdict sufficient=false)' "$file"
}

eval_count_self_consistency_concerns() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  local reviewer orchestrator n
  reviewer=$(eval_count_control_pattern 'INFO \[self_consistency_reviewer\].*(emitted|consistent=false)' "$file")
  orchestrator=$(eval_count_control_pattern 'DEBUG \[orchestrator\].*self_contradiction' "$file")
  n=$((reviewer + orchestrator))
  echo "$n"
}

eval_detect_provider_blocked() {
  local reasons="" file
  for file in "$@"; do
    if [[ -z "$file" || ! -f "$file" ]]; then
      continue
    fi
    # Provider-blocked is an eval classification, not product logic. Match only
    # timestamped control-plane log lines so customer/source text that quotes an
    # LLM error does not turn a product regression into an external outage.
    if LC_ALL=C awk '
      /^20[0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[^ ]+ / &&
      $0 !~ / DEBUG \[diag [^]]+\].*ASSISTANT content/ &&
      $0 ~ /(LLM API error \(status 402\)|insufficient_balance_error)/ {
        found = 1
      }
      END { exit found ? 0 : 1 }
    ' "$file"; then
      case ",$reasons," in
        *,insufficient_balance,*) ;;
        *) reasons="${reasons:+$reasons,}insufficient_balance" ;;
      esac
    fi
    if LC_ALL=C awk '
      /^20[0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[^ ]+ / &&
      $0 !~ / DEBUG \[diag [^]]+\].*ASSISTANT content/ &&
      $0 ~ /(LLM provider is not configured|没有可用的模型 provider 配置|providers\.yaml: llm\.default\.provider is required)/ {
        found = 1
      }
      END { exit found ? 0 : 1 }
    ' "$file"; then
      case ",$reasons," in
        *,provider_unconfigured,*) ;;
        *) reasons="${reasons:+$reasons,}provider_unconfigured" ;;
      esac
    fi
  done
  echo "$reasons"
}

eval_provider_preflight() {
  local codrax_bin="$1"
  local repo_root="$2"
  local outdir="$3"
  local timeout_seconds="${4:-120}"
  if [[ -z "$codrax_bin" || ! -x "$codrax_bin" ]]; then
    echo "provider_unconfigured"
    return 0
  fi
  mkdir -p "$outdir/logs"
  local providers=()
  if [[ -f "$repo_root/providers.yaml" ]]; then
    providers=(--providers "$repo_root/providers.yaml")
  fi
  local rc=0
  eval_run_with_timeout "$timeout_seconds" "$codrax_bin" \
    "${providers[@]}" \
    --repo "$repo_root" \
    --multi-repo=false \
    --chitchat-classifier=true \
    --log-dir "$outdir/logs" \
    --log-level debug \
    --request "你好" \
    >"$outdir/preflight.out" 2>"$outdir/preflight.err" || rc=$?
  local logs=()
  while IFS= read -r f; do
    logs+=("$f")
  done < <(ls "$outdir"/logs/codrax-*.log 2>/dev/null || true)
  local blocked
  blocked="$(eval_detect_provider_blocked "${logs[@]}")"
  if [[ -n "$blocked" ]]; then
    echo "$blocked"
    return 0
  fi
  if LC_ALL=C grep -aqE '(insufficient_balance_error|LLM API error \(status 402\))' "$outdir/preflight.err" "$outdir/preflight.out" 2>/dev/null; then
    echo "insufficient_balance"
    return 0
  fi
  if LC_ALL=C grep -aqE '(LLM provider is not configured|没有可用的模型 provider 配置|providers\.yaml: llm\.default\.provider is required)' "$outdir/preflight.err" "$outdir/preflight.out" 2>/dev/null; then
    echo "provider_unconfigured"
    return 0
  fi
  if [[ "$rc" -eq 124 ]]; then
    echo "provider_preflight_timeout"
    return 0
  fi
  echo ""
}

eval_running_jobs() {
  jobs -rp | wc -l | tr -d ' '
}

eval_wait_for_slot() {
  local parallel="$1"
  while [[ "$(eval_running_jobs)" -ge "$parallel" ]]; do
    sleep 1
  done
}

# eval_count_tool_rejects <log> <tool> — control-plane count of
# explicit tool-level rejections (phase=toolresult ok=false) for one
# emit tool. Loop-churn diagnosis: each reject costs one agent round.
eval_count_tool_rejects() {
  local file="$1" tool="$2"
  if [[ -z "$file" || ! -f "$file" || -z "$tool" ]]; then
    echo 0
    return
  fi
  eval_count_control_pattern "DEBUG \\[diag [^]]+\\][^:]*phase=toolresult TOOLRESULT ${tool} ok=false" "$file"
}
